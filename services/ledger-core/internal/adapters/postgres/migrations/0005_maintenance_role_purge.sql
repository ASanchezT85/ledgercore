-- +goose Up
-- LC-004 (HIGH) — Close the append-only bypass available to the runtime role.
--
-- BEFORE: the append-only guards on postings and transactions could be
-- disabled by any code holding the runtime role simply by running
--   SET ledger.allow_maintenance = '1'
-- inside its transaction. purge.go did exactly that with the application role.
-- That means the append-only guarantee was, in practice, opt-out by the very
-- role that runs all business writes — a privilege the auditor flagged.
--
-- AFTER: the guards bypass ONLY when BOTH hold:
--   1. current_user = 'ledgercore_maint'  (a dedicated, restricted role that
--      the application never logs in as), AND
--   2. the ledger.allow_maintenance flag is set for the transaction.
-- Because the runtime role can set the flag but can never *be* the maintenance
-- role, it can no longer edit or delete postings/transactions, nor open an
-- effective maintenance path on its own. The flag alone is now inert.
--
-- The only sanctioned maintenance path is the SECURITY DEFINER function
-- purge_expired_sandbox_tenant below: it is owned by ledgercore_maint, so it
-- executes with current_user = ledgercore_maint, sets the flag itself for the
-- duration of one transaction, validates that the tenant is sandbox-only, and
-- performs the child-first deletes. It is parameterised (a single tenant id),
-- auditable (a plain function, not a scattered GUC), and refuses to touch any
-- tenant that owns a 'live' ledger.
--
-- ============================================================================
-- INFRA COORDINATION (handled by the infra/postgres/init agent — do NOT edit
-- infra from this service). This migration runs as ledgercore_app and therefore
-- CANNOT set the function owner to another role. After the role exists, infra
-- must run (idempotently):
--
--   CREATE ROLE ledgercore_maint NOLOGIN NOBYPASSRLS;
--   GRANT USAGE ON SCHEMA ledger TO ledgercore_maint;
--   GRANT SELECT, DELETE ON ALL TABLES IN SCHEMA ledger TO ledgercore_maint;
--   ALTER FUNCTION ledger.purge_expired_sandbox_tenant(uuid) OWNER TO ledgercore_maint;
--   REVOKE EXECUTE ON FUNCTION ledger.purge_expired_sandbox_tenant(uuid) FROM PUBLIC;
--   GRANT  EXECUTE ON FUNCTION ledger.purge_expired_sandbox_tenant(uuid) TO ledgercore_app;
--
-- Until infra reassigns the owner, the function is owned by ledgercore_app and
-- (correctly) fails closed: executing it as the app role does NOT satisfy the
-- current_user = 'ledgercore_maint' condition, so the append-only guards block
-- the deletes and the purge raises rather than silently running with runtime
-- privileges. This is the intended fail-closed posture during rollout.
-- ============================================================================

-- Re-point the two append-only guards at the role-gated bypass. Bodies are
-- otherwise identical to 0001_init.sql; only the bypass condition changes.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION transactions_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_user = 'ledgercore_maint'
       AND current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'ledger transactions are append-only (DELETE forbidden)';
    END IF;

    IF NEW.id              IS DISTINCT FROM OLD.id
       OR NEW.tenant_id       IS DISTINCT FROM OLD.tenant_id
       OR NEW.ledger_id       IS DISTINCT FROM OLD.ledger_id
       OR NEW.reference       IS DISTINCT FROM OLD.reference
       OR NEW.description     IS DISTINCT FROM OLD.description
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.effective_at    IS DISTINCT FROM OLD.effective_at
       OR NEW.reverses_id     IS DISTINCT FROM OLD.reverses_id
       OR NEW.metadata        IS DISTINCT FROM OLD.metadata
       OR NEW.created_at      IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'ledger transactions: only status, posted_at and reversed_by_id may change';
    END IF;

    IF OLD.status = 'draft' AND NEW.status = 'posted'
       AND OLD.posted_at IS NULL AND NEW.posted_at IS NOT NULL
       AND NEW.reversed_by_id IS NOT DISTINCT FROM OLD.reversed_by_id THEN
        RETURN NEW;
    END IF;

    IF OLD.status = 'posted' AND NEW.status = 'reversed'
       AND OLD.reversed_by_id IS NULL AND NEW.reversed_by_id IS NOT NULL
       AND NEW.posted_at IS NOT DISTINCT FROM OLD.posted_at THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid transaction transition % -> % (allowed: draft->posted, posted->reversed)',
        OLD.status, NEW.status;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION postings_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_user = 'ledgercore_maint'
       AND current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'ledger postings are append-only';
END;
$$;
-- +goose StatementEnd

-- The sanctioned, auditable sandbox purge. SECURITY DEFINER: intended to be
-- owned by ledgercore_maint (see INFRA COORDINATION above). SET search_path is
-- mandatory hardening for SECURITY DEFINER functions.
-- +goose StatementBegin
CREATE FUNCTION purge_expired_sandbox_tenant(p_tenant uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = ledger AS $$
DECLARE
    v_live    integer;
    v_deleted bigint := 0;
    v_n       bigint;
BEGIN
    IF p_tenant IS NULL THEN
        RAISE EXCEPTION 'purge_expired_sandbox_tenant: tenant id is required';
    END IF;

    -- Environment guard: never purge a tenant that owns any live ledger. Only
    -- sandbox tenants (whose TTL expiry identity announces) are purgeable.
    SELECT count(*) INTO v_live FROM ledgers WHERE tenant_id = p_tenant AND environment = 'live';
    IF v_live > 0 THEN
        RAISE EXCEPTION 'refusing to purge tenant %: it owns % live ledger(s)', p_tenant, v_live;
    END IF;

    -- Opt in to the maintenance path for THIS transaction only. Combined with
    -- current_user = ledgercore_maint (true inside this SECURITY DEFINER
    -- function), this satisfies the append-only guards.
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

COMMENT ON FUNCTION purge_expired_sandbox_tenant(uuid) IS
    'LC-004: sanctioned sandbox-tenant purge. Owned by ledgercore_maint (SECURITY DEFINER); refuses tenants with live ledgers; the only path allowed to bypass the append-only guards.';

-- +goose Down
DROP FUNCTION IF EXISTS purge_expired_sandbox_tenant(uuid);

-- Restore the original GUC-only bypass (0001 behaviour) on rollback.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION transactions_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'ledger transactions are append-only (DELETE forbidden)';
    END IF;

    IF NEW.id              IS DISTINCT FROM OLD.id
       OR NEW.tenant_id       IS DISTINCT FROM OLD.tenant_id
       OR NEW.ledger_id       IS DISTINCT FROM OLD.ledger_id
       OR NEW.reference       IS DISTINCT FROM OLD.reference
       OR NEW.description     IS DISTINCT FROM OLD.description
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.effective_at    IS DISTINCT FROM OLD.effective_at
       OR NEW.reverses_id     IS DISTINCT FROM OLD.reverses_id
       OR NEW.metadata        IS DISTINCT FROM OLD.metadata
       OR NEW.created_at      IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'ledger transactions: only status, posted_at and reversed_by_id may change';
    END IF;

    IF OLD.status = 'draft' AND NEW.status = 'posted'
       AND OLD.posted_at IS NULL AND NEW.posted_at IS NOT NULL
       AND NEW.reversed_by_id IS NOT DISTINCT FROM OLD.reversed_by_id THEN
        RETURN NEW;
    END IF;

    IF OLD.status = 'posted' AND NEW.status = 'reversed'
       AND OLD.reversed_by_id IS NULL AND NEW.reversed_by_id IS NOT NULL
       AND NEW.posted_at IS NOT DISTINCT FROM OLD.posted_at THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid transaction transition % -> % (allowed: draft->posted, posted->reversed)',
        OLD.status, NEW.status;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION postings_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'ledger postings are append-only';
END;
$$;
-- +goose StatementEnd
