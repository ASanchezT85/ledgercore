-- +goose Up
-- LC-001 (CRITICAL) — Double-entry balance enforced by PostgreSQL itself.
--
-- Domain code (domain.ValidateBalanced) already guarantees the invariant on
-- the normal write path, but that is the ONLY thing standing between a bug (or
-- a direct SQL write bypassing the Go layer) and a permanently corrupt ledger.
-- This migration makes PostgreSQL refuse to COMMIT any *firm* transaction that
-- is not perfectly balanced, so the guarantee no longer depends on the
-- application being correct.
--
-- Mechanism: a DEFERRABLE INITIALLY DEFERRED CONSTRAINT TRIGGER. Deferring the
-- check to COMMIT means every posting of the transaction is already present
-- when the invariant runs, so multi-row inserts (the normal case) and
-- out-of-order direct SQL inserts are both handled. The invariant is:
--   (a) for every (transaction_id, asset): SUM(debits) = SUM(credits)
--   (b) the transaction has at least 2 postings
-- applied to transactions that are *firm* (status 'posted' or 'reversed').
-- Drafts are intentionally exempt while they are being assembled; the check
-- runs when they transition to 'posted' (the transactions trigger below fires
-- on that UPDATE).
--
-- Defense in depth: domain.ValidateBalanced stays exactly where it is — it
-- gives callers a fast, descriptive 422 before the DB round-trip. This trigger
-- is the backstop that also catches writes that never went through the domain.

-- +goose StatementBegin
CREATE FUNCTION check_transaction_balance(p_txid uuid) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
    v_status text;
    v_total  bigint;
    v_unbal  bigint;
BEGIN
    -- Enforce only on firm transactions. A transaction that was removed within
    -- the same transaction (should never happen: append-only) leaves nothing
    -- to check.
    SELECT status INTO v_status FROM transactions WHERE id = p_txid;
    IF v_status IS NULL OR v_status NOT IN ('posted', 'reversed') THEN
        RETURN;
    END IF;

    SELECT count(*) INTO v_total FROM postings WHERE transaction_id = p_txid;
    IF v_total < 2 THEN
        RAISE EXCEPTION 'double-entry violation: transaction % has % posting(s), at least 2 are required', p_txid, v_total
            USING ERRCODE = 'check_violation', CONSTRAINT = 'transaction_min_postings';
    END IF;

    SELECT count(*) INTO v_unbal
    FROM (
        SELECT asset
        FROM postings
        WHERE transaction_id = p_txid
        GROUP BY asset
        HAVING COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'), 0)
            <> COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0)
    ) unbalanced_assets;
    IF v_unbal > 0 THEN
        RAISE EXCEPTION 'double-entry violation: transaction % is unbalanced in % asset(s) (debits <> credits)', p_txid, v_unbal
            USING ERRCODE = 'check_violation', CONSTRAINT = 'transaction_balanced';
    END IF;
END;
$$;
-- +goose StatementEnd

-- Trigger wrapper fired by posting writes: validate the owning transaction.
-- +goose StatementBegin
CREATE FUNCTION postings_balance_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM check_transaction_balance(NEW.transaction_id);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- Trigger wrapper fired by transaction writes: validate the row itself. This
-- covers the draft->posted transition, where the postings do not change but
-- the transaction becomes firm, and any direct SQL that sets a transaction to
-- 'posted'/'reversed' after inserting unbalanced postings.
-- +goose StatementBegin
CREATE FUNCTION transactions_balance_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    PERFORM check_transaction_balance(NEW.id);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER postings_balance_guard
    AFTER INSERT OR UPDATE ON postings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION postings_balance_guard();

CREATE CONSTRAINT TRIGGER transactions_balance_guard
    AFTER INSERT OR UPDATE ON transactions
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION transactions_balance_guard();

-- FORCE ROW LEVEL SECURITY audit (part of the P0 audit checklist). Every
-- tenant-scoped table already declares FORCE ROW LEVEL SECURITY and a
-- tenant_isolation policy with an explicit WITH CHECK in 0001_init.sql. We
-- re-assert FORCE here idempotently as a belt-and-suspenders guarantee that
-- the table owner (ledgercore_app) can never bypass RLS. WITH CHECK is already
-- present on every policy and cannot be re-asserted without dropping the
-- policy, so it is verified by inspection, not re-created here.
ALTER TABLE ledgers          FORCE ROW LEVEL SECURITY;
ALTER TABLE accounts         FORCE ROW LEVEL SECURITY;
ALTER TABLE transactions     FORCE ROW LEVEL SECURITY;
ALTER TABLE postings         FORCE ROW LEVEL SECURITY;
ALTER TABLE account_balances FORCE ROW LEVEL SECURITY;
ALTER TABLE holds            FORCE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;
ALTER TABLE outbox           FORCE ROW LEVEL SECURITY;

-- +goose Down
DROP TRIGGER IF EXISTS transactions_balance_guard ON transactions;
DROP TRIGGER IF EXISTS postings_balance_guard ON postings;
DROP FUNCTION IF EXISTS transactions_balance_guard();
DROP FUNCTION IF EXISTS postings_balance_guard();
DROP FUNCTION IF EXISTS check_transaction_balance(uuid);
