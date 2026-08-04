package policytest

// Anti-leak contract test for the `blog` schema and its runtime role.
//
// The console serves the public blog (post views + reader comments). That is
// the ONLY unauthenticated write path in the product: anyone on the internet
// can POST a comment, which reaches Postgres as ledgercore_blog_rt. This test
// pins the blast radius of that role:
//
//   - it can read and write its OWN schema (`blog`),
//   - it CANNOT touch ledger / identity / recon / webhooks — not even SELECT,
//   - it cannot run DDL in its own schema either (no CREATE, so a SQL-injection
//     foothold cannot create a helper function to escalate with).
//
// The money schemas are created here as empty stand-ins when absent, so the
// denial assertions can never pass for the wrong reason ("schema does not
// exist" instead of "permission denied"). The blog schema is built from the
// REAL migration in apps/console/migrations, so a migration that accidentally
// widens access fails this gate.
//
// Requires LEDGERCORE_TEST_ADMIN_URL (a superuser DSN). Skips otherwise, like
// every other integration gate in the repo.

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	migratorPassword = "policytest_migrator"
	blogRTPassword   = "policytest_blog_rt"

	// SQLSTATE 42501.
	insufficientPrivilege = "42501"
)

// The four schemas the blog role must never reach.
var moneySchemas = []string{"ledger", "identity", "recon", "webhooks"}

func TestBlogRoleCannotReachMoneySchemas(t *testing.T) {
	adminURL := os.Getenv("LEDGERCORE_TEST_ADMIN_URL")
	if adminURL == "" {
		t.Skip("LEDGERCORE_TEST_ADMIN_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)

	provision(ctx, t, admin)

	blogURL, err := withUser(adminURL, "ledgercore_blog_rt", blogRTPassword)
	if err != nil {
		t.Fatalf("build blog dsn: %v", err)
	}
	blog, err := pgx.Connect(ctx, blogURL)
	if err != nil {
		t.Fatalf("blog connect: %v", err)
	}
	defer blog.Close(ctx)

	// The role must not be able to sidestep RLS anywhere, ever.
	var privileged bool
	if err := blog.QueryRow(ctx,
		"SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user",
	).Scan(&privileged); err != nil {
		t.Fatalf("inspect role: %v", err)
	}
	if privileged {
		t.Fatal("ledgercore_blog_rt is SUPERUSER or BYPASSRLS; it must be neither")
	}

	t.Run("denied on money schemas", func(t *testing.T) {
		for _, schema := range moneySchemas {
			for _, stmt := range []string{
				fmt.Sprintf("SELECT * FROM %s.policy_probe", schema),
				fmt.Sprintf("INSERT INTO %s.policy_probe (note) VALUES ('x')", schema),
				fmt.Sprintf("DELETE FROM %s.policy_probe", schema),
			} {
				_, err := blog.Exec(ctx, stmt)
				assertDenied(t, err, stmt)
			}
		}
	})

	t.Run("no DDL even in its own schema", func(t *testing.T) {
		_, err := blog.Exec(ctx, "CREATE TABLE blog.escalation (id int)")
		assertDenied(t, err, "CREATE TABLE blog.escalation")
	})

	t.Run("its own schema still works", func(t *testing.T) {
		var id string
		if err := blog.QueryRow(ctx,
			`INSERT INTO blog.comments (slug, author_name, body, author_hash)
             VALUES ('policy-test', 'Policy Test', 'contract test', 'hash')
             RETURNING id`,
		).Scan(&id); err != nil {
			t.Fatalf("insert comment: %v", err)
		}
		if _, err := blog.Exec(ctx,
			"INSERT INTO blog.post_views (slug, visitor_hash, day) VALUES ('policy-test', 'h', current_date)",
		); err != nil {
			t.Fatalf("insert view: %v", err)
		}
		var n int
		if err := blog.QueryRow(ctx,
			"SELECT count(*) FROM blog.comments WHERE slug = 'policy-test'").Scan(&n); err != nil {
			t.Fatalf("select comments: %v", err)
		}
		if n != 1 {
			t.Fatalf("expected 1 comment, got %d", n)
		}
		// Moderation needs UPDATE and DELETE through the same role.
		if _, err := blog.Exec(ctx,
			"UPDATE blog.comments SET status = 'hidden' WHERE id = $1", id); err != nil {
			t.Fatalf("hide comment: %v", err)
		}
		if _, err := blog.Exec(ctx,
			"DELETE FROM blog.comments WHERE slug = 'policy-test'"); err != nil {
			t.Fatalf("delete comment: %v", err)
		}
		if _, err := blog.Exec(ctx,
			"DELETE FROM blog.post_views WHERE slug = 'policy-test'"); err != nil {
			t.Fatalf("delete view: %v", err)
		}
	})

	t.Run("replies stay one level deep", func(t *testing.T) {
		var root, reply string
		if err := blog.QueryRow(ctx,
			`INSERT INTO blog.comments (slug, author_name, body, author_hash)
             VALUES ('policy-depth', 'Root', 'root', 'h') RETURNING id`).Scan(&root); err != nil {
			t.Fatalf("insert root: %v", err)
		}
		defer blog.Exec(ctx, "DELETE FROM blog.comments WHERE slug = 'policy-depth'")

		if err := blog.QueryRow(ctx,
			`INSERT INTO blog.comments (slug, parent_id, author_name, body, author_hash)
             VALUES ('policy-depth', $1, 'Reply', 'reply', 'h') RETURNING id`,
			root).Scan(&reply); err != nil {
			t.Fatalf("insert reply: %v", err)
		}
		// A reply to a reply must be refused by the trigger, not by convention.
		if _, err := blog.Exec(ctx,
			`INSERT INTO blog.comments (slug, parent_id, author_name, body, author_hash)
             VALUES ('policy-depth', $1, 'Nested', 'nested', 'h')`, reply); err == nil {
			t.Fatal("expected the depth trigger to refuse a reply to a reply")
		}
	})
}

func assertDenied(t *testing.T, err error, stmt string) {
	t.Helper()
	if err == nil {
		t.Errorf("LEAK: ledgercore_blog_rt was allowed to run %q", stmt)
		return
	}
	var pgErr *pgconn.PgError
	if !asPgError(err, &pgErr) {
		t.Errorf("%q failed with a non-Postgres error: %v", stmt, err)
		return
	}
	if pgErr.Code != insufficientPrivilege {
		t.Errorf("%q was refused with SQLSTATE %s (%s); expected %s (insufficient_privilege)",
			stmt, pgErr.Code, pgErr.Message, insufficientPrivilege)
	}
}

func asPgError(err error, target **pgconn.PgError) bool {
	for err != nil {
		if pgErr, ok := err.(*pgconn.PgError); ok {
			*target = pgErr
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// provision mirrors infra/postgres/init/01-init.sql for the blog role, and
// stands up empty money schemas so the denial assertions are meaningful.
func provision(ctx context.Context, t *testing.T, admin *pgx.Conn) {
	t.Helper()

	roles := `
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_migrator') THEN
        CREATE ROLE ledgercore_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ledgercore_blog_rt') THEN
        CREATE ROLE ledgercore_blog_rt LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
    END IF;
END $$;`
	mustExec(ctx, t, admin, roles)

	mustExec(ctx, t, admin, fmt.Sprintf(
		"ALTER ROLE ledgercore_migrator LOGIN PASSWORD %s", quoteLiteral(migratorPassword)))
	mustExec(ctx, t, admin, fmt.Sprintf(
		"ALTER ROLE ledgercore_blog_rt LOGIN PASSWORD %s", quoteLiteral(blogRTPassword)))

	// Stand-ins for the money schemas. Created only if missing so a shared CI
	// database keeps whatever the service suites already built there.
	//
	// NOTHING here touches ledgercore_blog_rt's privileges on them. That is the
	// whole point: an earlier draft of this test REVOKEd them first and then
	// asserted the denial, which made it assert a posture it had just created —
	// it stayed green even after a deliberate GRANT. The denial must come from
	// the role model as provisioned elsewhere, never from this function.
	for _, schema := range moneySchemas {
		mustExec(ctx, t, admin, fmt.Sprintf(
			"CREATE SCHEMA IF NOT EXISTS %s AUTHORIZATION ledgercore_migrator", schema))
		mustExec(ctx, t, admin, fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s.policy_probe (id bigserial PRIMARY KEY, note text)", schema))
	}

	// Rebuild `blog` from scratch so the migration under test is the only
	// source of its shape.
	mustExec(ctx, t, admin, "DROP SCHEMA IF EXISTS blog CASCADE")
	mustExec(ctx, t, admin, "CREATE SCHEMA blog AUTHORIZATION ledgercore_migrator")
	mustExec(ctx, t, admin, "GRANT USAGE ON SCHEMA blog TO ledgercore_blog_rt")
	mustExec(ctx, t, admin,
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA blog GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_blog_rt")
	mustExec(ctx, t, admin,
		"ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA blog GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_blog_rt")
	mustExec(ctx, t, admin, "REVOKE ALL ON SCHEMA blog FROM PUBLIC")

	// Apply the console's real migration AS THE MIGRATOR, so ownership and the
	// default privileges above are what the runtime role actually inherits.
	migratorURL, err := withUser(os.Getenv("LEDGERCORE_TEST_ADMIN_URL"),
		"ledgercore_migrator", migratorPassword)
	if err != nil {
		t.Fatalf("build migrator dsn: %v", err)
	}
	mig, err := pgx.Connect(ctx, migratorURL)
	if err != nil {
		t.Fatalf("migrator connect: %v", err)
	}
	defer mig.Close(ctx)

	mustExec(ctx, t, mig, "SET search_path TO blog")
	for _, sql := range blogMigrations(t) {
		mustExec(ctx, t, mig, sql)
	}
}

// blogMigrations returns the Up section of every console migration, in order.
func blogMigrations(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "apps", "console", "migrations")
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no migrations found in %s — the blog schema would be empty", dir)
	}
	out := make([]string, 0, len(files))
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		out = append(out, gooseUp(string(raw)))
	}
	return out
}

// gooseUp extracts everything between "-- +goose Up" and "-- +goose Down" and
// drops the StatementBegin/End markers, which are directives for the goose CLI
// and not SQL. The remainder is valid to send as one simple-protocol batch.
func gooseUp(sql string) string {
	if i := strings.Index(sql, "-- +goose Up"); i >= 0 {
		sql = sql[i+len("-- +goose Up"):]
	}
	if i := strings.Index(sql, "-- +goose Down"); i >= 0 {
		sql = sql[:i]
	}
	sql = strings.ReplaceAll(sql, "-- +goose StatementBegin", "")
	sql = strings.ReplaceAll(sql, "-- +goose StatementEnd", "")
	return sql
}

func mustExec(ctx context.Context, t *testing.T, conn *pgx.Conn, sql string) {
	t.Helper()
	if _, err := conn.Exec(ctx, sql); err != nil {
		t.Fatalf("provision failed (%.60s…): %v", strings.TrimSpace(sql), err)
	}
}

func withUser(rawURL, user, password string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	u.User = url.UserPassword(user, password)
	return u.String(), nil
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
