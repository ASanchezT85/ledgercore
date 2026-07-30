-- +goose Up
-- R-005 (HIGH) — the sanctioned purge silently deleted NOTHING under FORCE RLS.
--
-- BEFORE: purge_expired_sandbox_tenant (migration 0005) runs as ledgercore_maint
-- (SECURITY DEFINER), which is NOBYPASSRLS. Every ledger table is FORCE ROW
-- LEVEL SECURITY with the tenant_isolation policy
--     USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
-- The function never set app.tenant_id itself. It relied entirely on the caller
-- (PurgeTenant -> WithTenantTx) having set it. That coupling is fragile and, for
-- a maintenance function whose entire job is to bypass the append-only guards
-- for ONE tenant, wrong: if the function is ever invoked without a matching
-- tenant GUC (a system transaction, a future caller, a direct SET ROLE repair),
-- the protective SELECT and every DELETE see ZERO rows and the purge is a silent
-- no-op that still returns success. Expired sandbox data is never actually
-- removed.
--
-- AFTER: the function fixes its OWN RLS context. After it has validated that the
-- tenant owns no live ledger, it pins app.tenant_id = p_tenant for the remainder
-- of the transaction (set_config(..., is_local => true) == SET LOCAL, parameter-
-- ised so no literal quoting is involved). From that point the tenant_isolation
-- policy matches exactly the rows of THIS tenant, so the deletes remove real
-- rows. The function is now correct regardless of the caller's tenant context.
--
-- The live-ledger guard is evaluated the same way: it must run with the tenant
-- context set so it can actually see that tenant's ledgers. We therefore set the
-- GUC FIRST (before the guard SELECT), then run the guard, then the deletes.
--
-- Hardening carried over / strengthened from 0005:
--   * SECURITY DEFINER + owned by ledgercore_maint (assigned by infra grants.sql).
--   * search_path pinned to `ledger, pg_temp` so unqualified names can never be
--     hijacked via a temp schema; every object reference below is ALSO fully
--     qualified (ledger.<table>) as belt-and-suspenders.
--   * Still refuses any tenant that owns a 'live' ledger.
--
-- This migration only REPLACES the function body; CREATE OR REPLACE preserves
-- ownership, so the ledgercore_maint ownership established by grants.sql is kept
-- (and the migrator, a member of ledgercore_maint, is allowed to replace it).

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_expired_sandbox_tenant(p_tenant uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ledger, pg_temp AS $$
DECLARE
    v_live    integer;
    v_deleted bigint := 0;
    v_n       bigint;
BEGIN
    IF p_tenant IS NULL THEN
        RAISE EXCEPTION 'purge_expired_sandbox_tenant: tenant id is required';
    END IF;

    -- Pin this transaction's RLS context to the target tenant. Without this the
    -- FORCE RLS tenant_isolation policy hides every row from ledgercore_maint
    -- (NOBYPASSRLS) and both the guard SELECT and the DELETEs match nothing.
    -- is_local => true dies with the transaction; parameterised, so safe.
    PERFORM set_config('app.tenant_id', p_tenant::text, true);

    -- Environment guard: never purge a tenant that owns any live ledger. Only
    -- sandbox tenants (whose TTL expiry identity announces) are purgeable. This
    -- now runs WITH the tenant context set, so it can actually see the rows.
    SELECT count(*) INTO v_live FROM ledger.ledgers WHERE tenant_id = p_tenant AND environment = 'live';
    IF v_live > 0 THEN
        RAISE EXCEPTION 'refusing to purge tenant %: it owns % live ledger(s)', p_tenant, v_live;
    END IF;

    -- Opt in to the append-only maintenance path for THIS transaction only.
    -- Combined with current_user = ledgercore_maint (true inside this SECURITY
    -- DEFINER function), this satisfies the append-only guards.
    PERFORM set_config('ledger.allow_maintenance', '1', true);

    DELETE FROM ledger.postings         WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.idempotency_keys WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.holds            WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.account_balances WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.transactions     WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.accounts         WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.ledgers          WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledger.outbox           WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;

    RETURN v_deleted;
END;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION purge_expired_sandbox_tenant(uuid) IS
    'R-005/LC-004: sanctioned sandbox-tenant purge. Owned by ledgercore_maint (SECURITY DEFINER); pins app.tenant_id so FORCE RLS lets it delete the target tenant''s rows; refuses tenants with live ledgers; the only path allowed to bypass the append-only guards.';

-- +goose Down
-- Restore the 0005 body: fully-qualified deletes but WITHOUT setting app.tenant_id
-- (the pre-R-005 behaviour, kept so the rollback is faithful).
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION purge_expired_sandbox_tenant(p_tenant uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ledger AS $$
DECLARE
    v_live    integer;
    v_deleted bigint := 0;
    v_n       bigint;
BEGIN
    IF p_tenant IS NULL THEN
        RAISE EXCEPTION 'purge_expired_sandbox_tenant: tenant id is required';
    END IF;

    SELECT count(*) INTO v_live FROM ledgers WHERE tenant_id = p_tenant AND environment = 'live';
    IF v_live > 0 THEN
        RAISE EXCEPTION 'refusing to purge tenant %: it owns % live ledger(s)', p_tenant, v_live;
    END IF;

    PERFORM set_config('ledger.allow_maintenance', '1', true);

    DELETE FROM postings         WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM idempotency_keys WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM holds            WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM account_balances WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM transactions     WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM accounts         WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM ledgers          WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;
    DELETE FROM outbox           WHERE tenant_id = p_tenant; GET DIAGNOSTICS v_n = ROW_COUNT; v_deleted := v_deleted + v_n;

    RETURN v_deleted;
END;
$$;
-- +goose StatementEnd
