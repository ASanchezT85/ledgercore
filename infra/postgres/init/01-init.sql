-- LedgerCore dev bootstrap.
--
-- Runs exactly once, on the first initialization of the Postgres data
-- volume (mounted at /docker-entrypoint-initdb.d), connected as the image
-- superuser to the database created by the image itself via
-- POSTGRES_DB=ledgercore. This script therefore does NOT create the
-- database — only the application role and one schema per service.
--
-- IMPORTANT: no business tables live here. Each service owns its schema's
-- DDL through goose migrations embedded in the service binary (embed.FS),
-- which run at startup when LEDGERCORE_AUTO_MIGRATE=true. Services set
-- search_path to their own schema and must never touch a sibling schema.

-- Shared application role. Dev-only credentials (never reuse outside the
-- local compose stack). NOBYPASSRLS is deliberate: the tenant_isolation
-- row-level-security policies must apply to every application query.
CREATE ROLE ledgercore_app
    LOGIN
    PASSWORD 'ledgercore_dev'
    NOSUPERUSER
    NOCREATEDB
    NOCREATEROLE
    NOBYPASSRLS;

-- One schema per service, owned by the application role so each service
-- can run its own migrations without superuser privileges.
CREATE SCHEMA ledger   AUTHORIZATION ledgercore_app;
CREATE SCHEMA identity AUTHORIZATION ledgercore_app;
CREATE SCHEMA recon    AUTHORIZATION ledgercore_app;
CREATE SCHEMA webhooks AUTHORIZATION ledgercore_app;
