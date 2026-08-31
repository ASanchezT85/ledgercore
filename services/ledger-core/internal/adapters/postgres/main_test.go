package postgres

// Integration-test bootstrap that provisions the REAL PostgreSQL role model
// (LC-002 / LC-014) so the hardening tests — above all the sanctioned purge
// (R-005) — run under production-shaped roles and RLS instead of a single
// permissive role.
//
// Two ways to run the integration suite:
//
//   1. Role-model provisioning (recommended; exercises R-005 for real).
//      Set LEDGERCORE_TEST_ADMIN_URL to a SUPERUSER DSN, e.g.
//        postgres://postgres:postgres@localhost:5432/ledgercore_test
//      TestMain then, on that database:
//        * creates ledgercore_migrator (owner/DDL), ledgercore_maint (NOLOGIN,
//          owner of the SECURITY DEFINER purge fn) and ledgercore_ledger_rt
//          (runtime, NOBYPASSRLS, DML only) — idempotently;
//        * (re)creates schema `ledger` owned by the migrator with the runtime
//          default privileges from infra/postgres/init/01-init.sql;
//        * runs the goose migrations AS THE MIGRATOR;
//        * applies the infra/postgres/migrate/grants.sql step: every SECURITY
//          DEFINER function in `ledger` is reassigned to ledgercore_maint and
//          granted EXECUTE to ledgercore_ledger_rt.
//      It then points LEDGERCORE_TEST_DATABASE_URL at the RUNTIME role, so the
//      Store (and PurgeTenant) run exactly as a deployed service would.
//
//   2. Legacy single-role: leave LEDGERCORE_TEST_ADMIN_URL unset and set
//      LEDGERCORE_TEST_DATABASE_URL to any role that can also run DDL. The
//      suite still runs, but the purge test FAILS (not skips) because the
//      maint role/ownership cannot exist — see TestPurgeExpiredSandboxTenant.
//
// With neither variable set the whole integration suite skips, as before.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ASanchezT85/ledgercore/libs/go/pgxutil"
)

// roleModelProvisioned is true once TestMain has stood up the real role model,
// which means migrations already ran as the migrator and newTestStore must NOT
// try to migrate again as the (DDL-less) runtime role.
var roleModelProvisioned bool

const (
	migratorPassword = "ledgercore_migrator_dev"
	ledgerRTPassword = "ledgercore_ledger_rt_dev"
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
	//    older bootstrap still gets the DSNs below.
	roleSQL := `
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_migrator') THEN
        CREATE ROLE ledgercore_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_maint') THEN
        CREATE ROLE ledgercore_maint NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_ledger_rt') THEN
        CREATE ROLE ledgercore_ledger_rt LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END $$;`
	if _, err := admin.Exec(ctx, roleSQL); err != nil {
		return fmt.Errorf("create roles: %w", err)
	}
	for _, stmt := range []string{
		fmt.Sprintf("ALTER ROLE ledgercore_migrator LOGIN PASSWORD %s", quoteLiteral(migratorPassword)),
		fmt.Sprintf("ALTER ROLE ledgercore_ledger_rt LOGIN PASSWORD %s", quoteLiteral(ledgerRTPassword)),
		"GRANT ledgercore_maint TO ledgercore_migrator",
		// Fresh schema owned by the migrator — guarantees clean ownership even
		// if an older run left a `ledger` schema owned by another role.
		"DROP SCHEMA IF EXISTS ledger CASCADE",
		"CREATE SCHEMA ledger AUTHORIZATION ledgercore_migrator",
		"GRANT USAGE ON SCHEMA ledger TO ledgercore_ledger_rt",
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA ledger GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_ledger_rt",
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA ledger GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_ledger_rt",
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
	migPool, err := pgxutil.NewPool(ctx, migratorURL, "ledger")
	if err != nil {
		return fmt.Errorf("migrator pool: %w", err)
	}
	if err := Migrate(ctx, migPool); err != nil {
		migPool.Close()
		return fmt.Errorf("migrate as migrator: %w", err)
	}
	migPool.Close()

	// 3. Apply the grants.sql step: reassign every SECURITY DEFINER function in
	//    the `ledger` schema to ledgercore_maint and grant EXECUTE to the
	//    runtime role. Run as admin (superuser) so the ownership change is
	//    unconditional.
	// The purge fn is owned by ledgercore_maint and runs SECURITY DEFINER, so
	// ledgercore_maint itself needs USAGE on the schema and SELECT/DELETE on the
	// tables it clears (mirrors infra/postgres/init 01-init.sql for maint).
	for _, stmt := range []string{
		"GRANT USAGE ON SCHEMA ledger TO ledgercore_maint",
		"GRANT SELECT, DELETE ON ALL TABLES IN SCHEMA ledger TO ledgercore_maint",
	} {
		if _, err := admin.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("maint grants (%s): %w", stmt, err)
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
        WHERE n.nspname = 'ledger' AND p.prosecdef
    LOOP
        EXECUTE format('ALTER FUNCTION %s OWNER TO ledgercore_maint', fn);
        EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO ledgercore_ledger_rt', fn);
    END LOOP;
END $$;`
	if _, err := admin.Exec(ctx, grantsSQL); err != nil {
		return fmt.Errorf("apply grants: %w", err)
	}

	// 4. Point the suite at the RUNTIME role.
	runtimeURL, err := withUser(adminURL, "ledgercore_ledger_rt", ledgerRTPassword)
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

// runtimeStorePool builds a Store pool on the runtime DSN without migrating
// (migrations already ran as the migrator during provisioning).
func runtimeStorePool(t *testing.T, url string) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxutil.NewPool(context.Background(), url, "ledger")
	if err != nil {
		t.Fatalf("create runtime pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool), pool
}
