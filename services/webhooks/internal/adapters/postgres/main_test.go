package postgres

// Integration-test bootstrap that provisions the REAL PostgreSQL role model
// (LC-002 / LC-014) so the webhooks integration + RLS tests run under
// production-shaped roles and RLS instead of a single permissive role. This is
// the SAME harness convention used by every LedgerCore service (see
// services/ledger-core/internal/adapters/postgres/main_test.go): given a
// SUPERUSER DSN in LEDGERCORE_TEST_ADMIN_URL, TestMain
//
//   * creates ledgercore_migrator (owner/DDL), ledgercore_maint (NOLOGIN, owner
//     of the SECURITY DEFINER maintenance functions) and ledgercore_webhooks_rt
//     (runtime, NOBYPASSRLS, DML only) idempotently;
//   * (re)creates schema `webhooks` owned by the migrator with the runtime
//     default privileges from infra/postgres/init/01-init.sql;
//   * runs the goose migrations AS THE MIGRATOR;
//   * applies the infra/postgres/migrate/grants.sql step: reassigns every
//     SECURITY DEFINER function to ledgercore_maint, grants EXECUTE to the
//     runtime role, and grants maint UPDATE on subscriptions/deliveries (the
//     sanctioned maintenance functions UPDATE those tables);
//   * points LEDGERCORE_TEST_DATABASE_URL at the RUNTIME role, so the suite runs
//     exactly as a deployed service would (NOBYPASSRLS -> RLS bites for real).
//
// One test-harness-only deviation from production: the runtime role is granted
// MEMBERSHIP in ledgercore_maint so the tests' maintenance helpers (withMaintTx,
// which does SET LOCAL ROLE ledgercore_maint to simulate the SECURITY DEFINER
// path for table cleanup and cross-tenant reads) can assume the maint identity.
// The RLS isolation assertions never SET ROLE — they go through the tenant-
// scoped repo — so this does not weaken what the RLS tests prove. Membership
// grants no BYPASSRLS. In production the runtime role is NOT a maint member and
// reaches maintenance only through the EXECUTE-granted SECURITY DEFINER funcs.
//
// With LEDGERCORE_TEST_ADMIN_URL unset the suite falls back to the legacy path:
// it reads LEDGERCORE_TEST_DATABASE_URL directly and self-migrates + self-grants
// (requires a DDL role that is a maint member). With neither variable set the
// integration suite skips.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/ledgercore/ledgercore/libs/go/pgxutil"
)

// roleModelProvisioned is true once TestMain has stood up the real role model,
// which means migrations and grants already ran as the migrator/superuser and
// the tests must NOT try to migrate or re-grant as the (DDL-less) runtime role.
var roleModelProvisioned bool

const (
	migratorPassword  = "ledgercore_migrator_dev"
	webhooksRTPassword = "ledgercore_webhooks_rt_dev"
	runtimeRole        = "ledgercore_webhooks_rt"
)

func TestMain(m *testing.M) {
	adminURL := os.Getenv("LEDGERCORE_TEST_ADMIN_URL")
	if adminURL != "" {
		if err := provisionRoleModel(adminURL); err != nil {
			fmt.Fprintf(os.Stderr, "role-model provisioning failed: %v\n", err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

// withUser returns rawURL with its userinfo replaced by user:password.
func withUser(rawURL, user, password string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

func provisionRoleModel(adminURL string) error {
	ctx := context.Background()

	admin, err := pgxutil.NewPool(ctx, adminURL, "public")
	if err != nil {
		return fmt.Errorf("admin pool: %w", err)
	}
	defer admin.Close()

	// 1. Roles (idempotent). Passwords are forced so a pre-existing role from an
	//    older bootstrap still matches the DSNs below.
	roleSQL := `
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_migrator') THEN
        CREATE ROLE ledgercore_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_maint') THEN
        CREATE ROLE ledgercore_maint NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_webhooks_rt') THEN
        CREATE ROLE ledgercore_webhooks_rt LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END $$;`
	if _, err := admin.Exec(ctx, roleSQL); err != nil {
		return fmt.Errorf("create roles: %w", err)
	}
	for _, stmt := range []string{
		fmt.Sprintf("ALTER ROLE ledgercore_migrator LOGIN PASSWORD %s", quoteLiteral(migratorPassword)),
		fmt.Sprintf("ALTER ROLE ledgercore_webhooks_rt LOGIN PASSWORD %s", quoteLiteral(webhooksRTPassword)),
		"GRANT ledgercore_maint TO ledgercore_migrator",
		// Fresh schema owned by the migrator — guarantees clean ownership even
		// if an older run left a `webhooks` schema owned by another role.
		"DROP SCHEMA IF EXISTS webhooks CASCADE",
		"CREATE SCHEMA webhooks AUTHORIZATION ledgercore_migrator",
		"GRANT USAGE ON SCHEMA webhooks TO ledgercore_webhooks_rt",
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA webhooks GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_webhooks_rt",
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA webhooks GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_webhooks_rt",
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("provision (%s): %w", stmt, err)
		}
	}

	// 2. Migrate AS THE MIGRATOR so every object is owned by it.
	migratorURL, err := withUser(adminURL, "ledgercore_migrator", migratorPassword)
	if err != nil {
		return err
	}
	migPool, err := pgxutil.NewPool(ctx, migratorURL, SchemaName)
	if err != nil {
		return fmt.Errorf("migrator pool: %w", err)
	}
	if err := Migrate(ctx, migPool); err != nil {
		migPool.Close()
		return fmt.Errorf("migrate as migrator: %w", err)
	}
	migPool.Close()

	// 3. Apply the grants.sql step (as admin/superuser). The webhooks maintenance
	//    functions (claim_due_deliveries, set_encrypted_secret) run as maint and
	//    UPDATE their tables, so maint needs UPDATE on top of SELECT/DELETE.
	for _, stmt := range []string{
		"GRANT USAGE ON SCHEMA webhooks TO ledgercore_maint",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA webhooks TO ledgercore_webhooks_rt",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA webhooks TO ledgercore_webhooks_rt",
		"GRANT SELECT, DELETE ON ALL TABLES IN SCHEMA webhooks TO ledgercore_maint",
		"GRANT UPDATE ON webhooks.subscriptions, webhooks.deliveries TO ledgercore_maint",
		// Test-harness only: lets withMaintTx do SET LOCAL ROLE ledgercore_maint.
		"GRANT ledgercore_maint TO ledgercore_webhooks_rt",
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("post-migrate grants (%s): %w", stmt, err)
		}
	}
	grantsSQL := `
DO $$
DECLARE fn text;
BEGIN
    FOR fn IN
        SELECT p.oid::regprocedure::text
        FROM pg_proc p
        JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'webhooks' AND p.prosecdef
    LOOP
        EXECUTE format('ALTER FUNCTION %s OWNER TO ledgercore_maint', fn);
        EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO ledgercore_webhooks_rt', fn);
    END LOOP;
END $$;`
	if _, err := admin.Exec(ctx, grantsSQL); err != nil {
		return fmt.Errorf("apply grants: %w", err)
	}

	// 4. Point the suite at the RUNTIME role.
	runtimeURL, err := withUser(adminURL, runtimeRole, webhooksRTPassword)
	if err != nil {
		return err
	}
	if err := os.Setenv("LEDGERCORE_TEST_DATABASE_URL", runtimeURL); err != nil {
		return err
	}
	roleModelProvisioned = true
	return nil
}

// quoteLiteral single-quotes a SQL string literal (passwords only, from a
// constant here, but kept safe regardless).
func quoteLiteral(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += "''"
			continue
		}
		out += string(r)
	}
	return out + "'"
}
