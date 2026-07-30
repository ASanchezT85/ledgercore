-- +goose Up
-- R-004 (HIGH) — Remove the permissive cross-tenant `system_drain` policy that
-- was reachable by the API runtime role, and replace the cross-tenant access
-- the worker/dispatcher legitimately needs with narrow, auditable SECURITY
-- DEFINER functions owned by ledgercore_maint.
--
-- BEFORE (migration 0003): both webhooks tables carried a `system_drain`
-- policy that granted GLOBAL, cross-tenant access whenever `app.tenant_id` was
-- absent:
--     USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
-- The dispatcher's ClaimDue scan and the previous-secret purge run without a
-- tenant context (WithSystemTx), so they relied on that policy. But the
-- dispatcher shares its role AND its process with the HTTP API, so the very
-- same runtime role could read/write EVERY tenant's rows simply by NOT setting
-- the tenant context — a plain bug that omits the tenant scope, or a process
-- compromise, silently escalates to full cross-tenant access.
--
-- AFTER: `system_drain` is dropped. Only `tenant_isolation` remains for the
-- runtime role, so a query without a tenant context now sees ZERO rows
-- (deny-by-default) instead of every tenant. The sanctioned cross-tenant paths
-- move into SECURITY DEFINER functions owned by ledgercore_maint. A single
-- maintenance policy grants access ONLY when current_user = 'ledgercore_maint'
-- — which is true exclusively INSIDE those functions (they are owned by, and
-- therefore execute as, ledgercore_maint). The API runtime role
-- (ledgercore_webhooks_rt) can never *be* ledgercore_maint (it is NOLOGIN and
-- no runtime role is granted membership / SET ROLE to it), so it can only ever
-- reach its own tenant through tenant_isolation. This mirrors the append-only
-- maintenance model ledger-core uses in 0005 (current_user = 'ledgercore_maint'
-- is the real security boundary, not the mere absence of a GUC).
--
-- ============================================================================
-- INFRA COORDINATION (handled by infra/postgres/migrate/grants.sql — do NOT
-- edit infra from this service). This migration runs as the migrator and
-- CANNOT set a function's owner to another role, nor widen ledgercore_maint's
-- table privileges. After the roles exist, infra must ensure (idempotently):
--
--   -- (a) already generic in grants.sql: every SECURITY DEFINER function in
--   --     the webhooks schema is reassigned to ledgercore_maint and EXECUTE is
--   --     granted to the schema's runtime role:
--   ALTER FUNCTION webhooks.<fn> OWNER TO ledgercore_maint;
--   GRANT  EXECUTE ON FUNCTION webhooks.<fn> TO ledgercore_webhooks_rt;
--
--   -- (b) NEW for webhooks: init/01-init.sql grants ledgercore_maint only
--   --     SELECT + DELETE on every schema (ledger-core purge only deletes).
--   --     The webhooks maintenance functions also UPDATE (lease a delivery,
--   --     null-out an expired previous secret, seal a plaintext secret), so
--   --     maint additionally needs UPDATE on the two webhooks tables:
--   GRANT UPDATE ON webhooks.subscriptions, webhooks.deliveries TO ledgercore_maint;
--
-- Until infra reassigns the owner, each function stays owned by the migrator
-- and (correctly) fails closed: executing it does NOT make current_user =
-- 'ledgercore_maint', so the maintenance policy denies every row and the
-- function returns nothing rather than running with the migrator's privileges.
-- This is the intended fail-closed posture during rollout.
-- ============================================================================

-- 1. Drop the permissive cross-tenant policy on both tables.
DROP POLICY IF EXISTS system_drain ON subscriptions;
DROP POLICY IF EXISTS system_drain ON deliveries;

-- 2. Role-gated maintenance policy. PERMISSIVE + FOR ALL, but the USING /
--    WITH CHECK predicate only holds for code executing AS ledgercore_maint,
--    i.e. inside the SECURITY DEFINER functions below. current_user is a
--    string comparison, so this DDL does not require the role to exist at
--    migrate time (portable to dev clusters without the full role model).
CREATE POLICY maintenance_access ON subscriptions
    USING (current_user = 'ledgercore_maint')
    WITH CHECK (current_user = 'ledgercore_maint');
CREATE POLICY maintenance_access ON deliveries
    USING (current_user = 'ledgercore_maint')
    WITH CHECK (current_user = 'ledgercore_maint');

-- 3. Sanctioned cross-tenant maintenance functions. All are SECURITY DEFINER
--    (intended owner ledgercore_maint) with a hardened, fully-qualified
--    search_path. Each is parameterised and auditable — a plain function, not
--    a scattered policy the runtime role can trip by omission.

-- claim_due_deliveries leases up to p_limit due deliveries across all tenants:
-- it locks the rows (FOR UPDATE SKIP LOCKED), pushes their next_attempt_at
-- `p_lease_seconds` into the future so a crashed worker's claims resurface,
-- and returns them joined with their subscription's endpoint data. Replaces
-- the dispatcher's direct cross-tenant SELECT + UPDATE.
-- +goose StatementBegin
CREATE FUNCTION claim_due_deliveries(p_limit int, p_lease_seconds double precision)
RETURNS TABLE (
    id                         uuid,
    tenant_id                  uuid,
    subscription_id            uuid,
    event_id                   uuid,
    event_type                 text,
    payload                    jsonb,
    attempts                   int,
    url                        text,
    secret                     text,
    previous_secret            text,
    previous_secret_expires_at timestamptz
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = webhooks AS $$
-- The RETURNS TABLE column names collide with table column names; prefer the
-- column meaning inside the query body so references resolve unambiguously.
#variable_conflict use_column
BEGIN
    RETURN QUERY
    WITH due AS (
        SELECT d.id AS d_id
        FROM deliveries d
        JOIN subscriptions s ON s.id = d.subscription_id
        WHERE d.status IN ('pending', 'failed')
          AND d.next_attempt_at <= now()
          AND s.active
        ORDER BY d.next_attempt_at
        LIMIT p_limit
        FOR UPDATE OF d SKIP LOCKED
    ),
    leased AS (
        UPDATE deliveries d
        SET next_attempt_at = now() + make_interval(secs => p_lease_seconds)
        FROM due
        WHERE d.id = due.d_id
        RETURNING d.id AS leased_id
    )
    SELECT d.id, d.tenant_id, d.subscription_id, d.event_id, d.event_type, d.payload, d.attempts,
           s.url, s.secret, s.previous_secret, s.previous_secret_expires_at
    FROM deliveries d
    JOIN subscriptions s ON s.id = d.subscription_id
    WHERE d.id IN (SELECT leased.leased_id FROM leased)
    ORDER BY d.created_at;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION claim_due_deliveries(int, double precision) IS
    'R-004: sanctioned cross-tenant dispatcher claim. Owned by ledgercore_maint (SECURITY DEFINER); leases due deliveries and returns them with endpoint data.';

-- purge_expired_previous_secrets clears the pre-rotation secret of every
-- subscription whose grace window has passed, across all tenants. Replaces the
-- worker's direct cross-tenant UPDATE. Returns the number of rows cleaned.
-- +goose StatementBegin
CREATE FUNCTION purge_expired_previous_secrets() RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = webhooks AS $$
DECLARE
    v_n bigint;
BEGIN
    UPDATE subscriptions
    SET previous_secret = NULL, previous_secret_expires_at = NULL
    WHERE previous_secret IS NOT NULL AND previous_secret_expires_at <= now();
    GET DIAGNOSTICS v_n = ROW_COUNT;
    RETURN v_n;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION purge_expired_previous_secrets() IS
    'R-004: sanctioned cross-tenant sweep of expired previous webhook secrets. Owned by ledgercore_maint (SECURITY DEFINER).';

-- list_plaintext_secrets / set_encrypted_secret support the LC-008 at-rest
-- re-encryption startup migration, which must read plaintext secrets across
-- tenants and write back the sealed values. The encryption itself happens in
-- Go (per-subscription AAD), so the cross-tenant read and the per-row write are
-- each exposed as their own sanctioned, parameterised function rather than a
-- blanket cross-tenant grant.
-- +goose StatementBegin
CREATE FUNCTION list_plaintext_secrets()
RETURNS TABLE (id uuid, secret text, previous_secret text)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = webhooks AS $$
BEGIN
    RETURN QUERY
    SELECT s.id, s.secret, s.previous_secret
    FROM subscriptions s
    WHERE s.secret NOT LIKE 'enc:v1:%'
       OR (s.previous_secret IS NOT NULL AND s.previous_secret NOT LIKE 'enc:v1:%');
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION list_plaintext_secrets() IS
    'R-004/LC-008: sanctioned cross-tenant read of subscriptions whose secrets are still plaintext. Owned by ledgercore_maint (SECURITY DEFINER).';

-- +goose StatementBegin
CREATE FUNCTION set_encrypted_secret(p_id uuid, p_secret text, p_previous_secret text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = webhooks AS $$
BEGIN
    UPDATE subscriptions
    SET secret = p_secret, previous_secret = p_previous_secret
    WHERE id = p_id;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION set_encrypted_secret(uuid, text, text) IS
    'R-004/LC-008: sanctioned per-row write-back of a sealed webhook secret. Owned by ledgercore_maint (SECURITY DEFINER).';

-- +goose Down
DROP FUNCTION IF EXISTS set_encrypted_secret(uuid, text, text);
DROP FUNCTION IF EXISTS list_plaintext_secrets();
DROP FUNCTION IF EXISTS purge_expired_previous_secrets();
DROP FUNCTION IF EXISTS claim_due_deliveries(int, double precision);

DROP POLICY IF EXISTS maintenance_access ON deliveries;
DROP POLICY IF EXISTS maintenance_access ON subscriptions;

-- Restore the 0003 permissive cross-tenant policy on rollback.
CREATE POLICY system_drain ON subscriptions
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);
CREATE POLICY system_drain ON deliveries
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);
