-- LedgerCore dev bootstrap — PostgreSQL role model (LC-014 / LC-002).
--
-- Runs exactly once, on the first initialization of the Postgres data
-- volume (mounted at /docker-entrypoint-initdb.d), connected as the image
-- superuser to the database created by the image itself via
-- POSTGRES_DB=ledgercore. This script therefore does NOT create the
-- database — only the roles and one schema per service.
--
-- IMPORTANT: no business tables live here. Each service owns its schema's
-- DDL through goose migrations (services/*/internal/adapters/postgres/
-- migrations). Those migrations run as the MIGRATOR role, never at runtime.
--
-- ============================================================================
-- Role model (three tiers — see docs/postgres-role-model.md for the matrix)
-- ============================================================================
--
--   ledgercore_migrator   OWNER of every schema and every table. Runs DDL
--                         (goose migrations) only. NEVER used at runtime.
--
--   ledgercore_<svc>_rt   One RUNTIME role per service. NOSUPERUSER,
--                         NOBYPASSRLS, DML-only on its OWN schema, no
--                         privilege at all on sibling schemas. This is the
--                         only role a service process ever authenticates as.
--
--   ledgercore_maint      Owner of the SECURITY DEFINER maintenance / purge
--                         functions created by the ledger-core migrations.
--                         Its credentials are NOT handed to any service at
--                         runtime; it is used only for controlled operations.
--
-- Because the runtime roles are NOT the table owner and carry NOBYPASSRLS,
-- the per-tenant RLS policies apply to every runtime query even without
-- FORCE ROW LEVEL SECURITY. The service migrations additionally set FORCE
-- RLS (belt-and-suspenders, and so the owner/maint roles are covered too).
-- This is what closes LC-002: RLS is no longer skippable by the app role,
-- and LC-014: a compromised service cannot read or write another schema.
--
-- Dev-only credentials below — NEVER reuse outside the local compose stack.
-- The public sandbox overlay injects strong secrets via environment.

-- ---------------------------------------------------------------------------
-- 1. Migrator / owner role (DDL only, never runtime).
-- ---------------------------------------------------------------------------
CREATE ROLE ledgercore_migrator
    LOGIN
    PASSWORD 'ledgercore_migrator_dev'
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOBYPASSRLS;

-- ---------------------------------------------------------------------------
-- 2. Maintenance role (owner of SECURITY DEFINER purge/maintenance functions).
--    Kept out of every service DSN. The migrator is granted membership so the
--    migrations (which run AS the migrator) can create these functions with
--    OWNER = ledgercore_maint and hand them SECURITY DEFINER rights.
-- ---------------------------------------------------------------------------
-- NOLOGIN on purpose: ledgercore_maint has NO password and cannot be used as a
-- connection identity by any service. It is reachable only two ways: (a) as the
-- definer of the SECURITY DEFINER purge functions it owns (current_user becomes
-- ledgercore_maint inside them), and (b) via SET ROLE by a member (the migrator,
-- or a human superuser) for controlled repair. This is what keeps maintenance
-- credentials out of runtime entirely.
CREATE ROLE ledgercore_maint
    NOLOGIN
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOBYPASSRLS;

GRANT ledgercore_maint TO ledgercore_migrator;

-- ---------------------------------------------------------------------------
-- 3. Per-service runtime roles (DML only, own schema only).
-- ---------------------------------------------------------------------------
CREATE ROLE ledgercore_ledger_rt
    LOGIN PASSWORD 'ledgercore_ledger_rt_dev'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;

CREATE ROLE ledgercore_identity_rt
    LOGIN PASSWORD 'ledgercore_identity_rt_dev'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;

CREATE ROLE ledgercore_recon_rt
    LOGIN PASSWORD 'ledgercore_recon_rt_dev'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;

CREATE ROLE ledgercore_webhooks_rt
    LOGIN PASSWORD 'ledgercore_webhooks_rt_dev'
    NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;

-- ---------------------------------------------------------------------------
-- 4. One schema per service, all OWNED BY THE MIGRATOR. Services never own a
--    schema, so a runtime role cannot run DDL even against its own schema.
-- ---------------------------------------------------------------------------
CREATE SCHEMA ledger   AUTHORIZATION ledgercore_migrator;
CREATE SCHEMA identity AUTHORIZATION ledgercore_migrator;
CREATE SCHEMA recon    AUTHORIZATION ledgercore_migrator;
CREATE SCHEMA webhooks AUTHORIZATION ledgercore_migrator;

-- ---------------------------------------------------------------------------
-- 5. Runtime grants. Each runtime role gets:
--      - USAGE on its OWN schema (resolve objects; NOT CREATE, so no DDL).
--      - DML on all CURRENT tables of its schema (there are none yet, but this
--        makes re-runs / manual bootstraps idempotent).
--      - DML on future tables + USAGE/SELECT on future sequences via
--        ALTER DEFAULT PRIVILEGES, so tables the migrator creates in later
--        migrations automatically grant DML to the right runtime role.
--    No GRANT is issued to a runtime role on any sibling schema; PostgreSQL
--    grants nothing cross-schema by default, and step 6 makes that explicit.
-- ---------------------------------------------------------------------------

-- ledger -> ledgercore_ledger_rt
GRANT USAGE ON SCHEMA ledger TO ledgercore_ledger_rt;
GRANT SELECT, INSERT, UPDATE, DELETE
    ON ALL TABLES IN SCHEMA ledger TO ledgercore_ledger_rt;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ledger TO ledgercore_ledger_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA ledger
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_ledger_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA ledger
    GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_ledger_rt;

-- identity -> ledgercore_identity_rt
GRANT USAGE ON SCHEMA identity TO ledgercore_identity_rt;
GRANT SELECT, INSERT, UPDATE, DELETE
    ON ALL TABLES IN SCHEMA identity TO ledgercore_identity_rt;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA identity TO ledgercore_identity_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA identity
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_identity_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA identity
    GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_identity_rt;

-- recon -> ledgercore_recon_rt
GRANT USAGE ON SCHEMA recon TO ledgercore_recon_rt;
GRANT SELECT, INSERT, UPDATE, DELETE
    ON ALL TABLES IN SCHEMA recon TO ledgercore_recon_rt;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA recon TO ledgercore_recon_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA recon
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_recon_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA recon
    GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_recon_rt;

-- webhooks -> ledgercore_webhooks_rt
GRANT USAGE ON SCHEMA webhooks TO ledgercore_webhooks_rt;
GRANT SELECT, INSERT, UPDATE, DELETE
    ON ALL TABLES IN SCHEMA webhooks TO ledgercore_webhooks_rt;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA webhooks TO ledgercore_webhooks_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA webhooks
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO ledgercore_webhooks_rt;
ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA webhooks
    GRANT USAGE, SELECT ON SEQUENCES TO ledgercore_webhooks_rt;

-- ---------------------------------------------------------------------------
-- 6. Explicit cross-schema REVOKE. Defence in depth: even if a future migration
--    or a superuser accidentally widens PUBLIC, no runtime role can touch a
--    schema that is not its own. (PUBLIC never had CREATE on these schemas
--    because they are owned by the migrator, but we revoke to be explicit.)
-- ---------------------------------------------------------------------------
REVOKE ALL ON SCHEMA ledger, identity, recon, webhooks FROM PUBLIC;

REVOKE ALL ON SCHEMA identity, recon, webhooks FROM ledgercore_ledger_rt;
REVOKE ALL ON SCHEMA ledger,   recon, webhooks FROM ledgercore_identity_rt;
REVOKE ALL ON SCHEMA ledger, identity, webhooks FROM ledgercore_recon_rt;
REVOKE ALL ON SCHEMA ledger, identity, recon    FROM ledgercore_webhooks_rt;

-- ---------------------------------------------------------------------------
-- 7. Maintenance role grants. It must be able to operate on every schema's
--    data (purge/repair) but is never used by a running service. The
--    migrations create SECURITY DEFINER functions owned by this role; callers
--    (services, at runtime) are granted EXECUTE by the migrations, never the
--    ability to authenticate as ledgercore_maint.
--
--    CREATE (not just USAGE) is required so a function can be OWNED BY
--    ledgercore_maint: PostgreSQL demands the new owner hold CREATE on the
--    function's schema. This is a privileged, non-runtime role, so schema-level
--    CREATE here is acceptable. Runtime roles never receive CREATE.
-- ---------------------------------------------------------------------------
GRANT USAGE, CREATE ON SCHEMA ledger, identity, recon, webhooks TO ledgercore_maint;

-- Maint operates on data for purge/repair: SELECT + DELETE on every current and
-- future migrator-created table (the ledger-core purge migration expects this).
-- Kept narrow: no INSERT/UPDATE, since maintenance only reads and removes rows.
DO $$
DECLARE s text;
BEGIN
    FOREACH s IN ARRAY ARRAY['ledger','identity','recon','webhooks'] LOOP
        EXECUTE format('GRANT SELECT, DELETE ON ALL TABLES IN SCHEMA %I TO ledgercore_maint', s);
        EXECUTE format('ALTER DEFAULT PRIVILEGES FOR ROLE ledgercore_migrator IN SCHEMA %I GRANT SELECT, DELETE ON TABLES TO ledgercore_maint', s);
    END LOOP;
END $$;
