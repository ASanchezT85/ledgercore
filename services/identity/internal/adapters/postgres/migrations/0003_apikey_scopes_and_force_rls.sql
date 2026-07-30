-- +goose Up
-- LC-012 + LC-002/LC-014 hardening for identity.api_keys.
--
-- 1. scopes: API keys can now be minted with a bounded scope set, so the
--    tokens they issue carry least-privilege scopes (enforced per-route by
--    libs/go/ident.RequireScope). Legacy rows default to '{}' and the service
--    falls back to DefaultScopes for them.
--
-- 2. FORCE ROW LEVEL SECURITY + WITH CHECK: the app role (ledgercore_app)
--    OWNS this schema (see infra/postgres/init/01-init.sql), and RLS that is
--    only ENABLEd does NOT apply to a table's owner. Without FORCE the
--    tenant_isolation policy was effectively bypassed for every identity
--    query. FORCE makes it apply to the owner too. The system_access policy
--    keeps the pre-tenant paths working (token issuance and admin bootstrap
--    run with app.tenant_id unset). WITH CHECK is added so the policies gate
--    INSERT/UPDATE, not just reads: a tenant context can neither read nor
--    write another tenant's keys.
--
-- The other identity tables (tenants, signing_keys, sandbox_signups, outbox)
-- are legitimately cross-tenant / system-level and stay exempt from RLS by
-- design (documented in 0001_init.sql and 0002_sandbox.sql).

ALTER TABLE identity.api_keys
    ADD COLUMN scopes TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE identity.api_keys FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON identity.api_keys;
CREATE POLICY tenant_isolation ON identity.api_keys
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

DROP POLICY IF EXISTS system_access ON identity.api_keys;
CREATE POLICY system_access ON identity.api_keys
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- +goose Down
DROP POLICY IF EXISTS system_access ON identity.api_keys;
CREATE POLICY system_access ON identity.api_keys
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

DROP POLICY IF EXISTS tenant_isolation ON identity.api_keys;
CREATE POLICY tenant_isolation ON identity.api_keys
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE identity.api_keys NO FORCE ROW LEVEL SECURITY;
ALTER TABLE identity.api_keys DROP COLUMN IF EXISTS scopes;
