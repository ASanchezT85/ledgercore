-- +goose Up
-- LC-002: harden tenant isolation on the webhooks tables.
--
-- 0001 only ran ENABLE ROW LEVEL SECURITY. The app role (ledgercore_app) OWNS
-- these tables, and a table owner bypasses RLS unless the table is also set to
-- FORCE ROW LEVEL SECURITY — so the tenant_isolation policy was effectively a
-- no-op and tenant scoping relied solely on the WHERE tenant_id clauses in Go.
-- We now FORCE RLS (policies apply to the owner too) and give every policy an
-- explicit WITH CHECK so writes cannot land rows in another tenant.
--
-- The dispatcher's ClaimDue scan and the previous-secret purge run in a system
-- transaction (no app.tenant_id set) and legitimately span tenants; a
-- permissive system_drain policy grants access exactly in that case, mirroring
-- the outbox drain in ledger-core. Tenant-scoped transactions keep seeing only
-- their own rows through tenant_isolation.

ALTER TABLE subscriptions FORCE ROW LEVEL SECURITY;
ALTER TABLE deliveries    FORCE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON subscriptions;
CREATE POLICY tenant_isolation ON subscriptions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY system_drain ON subscriptions
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

DROP POLICY IF EXISTS tenant_isolation ON deliveries;
CREATE POLICY tenant_isolation ON deliveries
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
CREATE POLICY system_drain ON deliveries
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- +goose Down
DROP POLICY IF EXISTS system_drain ON deliveries;
DROP POLICY IF EXISTS tenant_isolation ON deliveries;
CREATE POLICY tenant_isolation ON deliveries
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

DROP POLICY IF EXISTS system_drain ON subscriptions;
DROP POLICY IF EXISTS tenant_isolation ON subscriptions;
CREATE POLICY tenant_isolation ON subscriptions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

ALTER TABLE deliveries    NO FORCE ROW LEVEL SECURITY;
ALTER TABLE subscriptions NO FORCE ROW LEVEL SECURITY;
