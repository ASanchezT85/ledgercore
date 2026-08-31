-- +goose Up
-- Pin search_path on the ledger's guard functions.
--
-- Every SECURITY DEFINER function in this database already pins its
-- search_path. The SECURITY INVOKER trigger functions did not, and three of
-- them resolve table names at call time:
--
--   check_transaction_balance   -> FROM transactions / FROM postings
--   transactions_balance_guard  -> PERFORM check_transaction_balance(...)
--   postings_balance_guard      -> PERFORM check_transaction_balance(...)
--
-- Name resolution therefore depended on the *caller's* search_path. Two
-- consequences, both observed:
--
--   1. A direct psql session (search_path = "$user", public) hit
--      `function check_transaction_balance(uuid) does not exist` at COMMIT
--      instead of the balance check. It failed closed, but with a misleading
--      error, and it also blocked legitimate balanced writes from such a
--      session.
--   2. A caller able to create objects and put its own schema ahead of
--      `ledger` could make the guard validate a different `transactions` /
--      `postings` pair — i.e. satisfy the double-entry check against the wrong
--      data.
--
-- Direct SQL writes bypassing the Go layer are precisely what migration 0003
-- exists to catch, so the guard must not depend on how the caller is
-- configured. Pinning search_path on the function makes resolution a property
-- of the function itself. `pg_temp` is placed last so a temporary table can
-- never shadow a real one.
ALTER FUNCTION ledger.check_transaction_balance(uuid)  SET search_path = ledger, pg_temp;
ALTER FUNCTION ledger.transactions_balance_guard()     SET search_path = ledger, pg_temp;
ALTER FUNCTION ledger.postings_balance_guard()         SET search_path = ledger, pg_temp;
ALTER FUNCTION ledger.transactions_guard()             SET search_path = ledger, pg_temp;
ALTER FUNCTION ledger.postings_append_only()           SET search_path = ledger, pg_temp;
ALTER FUNCTION ledger.verify_account_balances(uuid) SET search_path = ledger, pg_temp;

-- Schema-qualify the call so the trigger body no longer relies on resolution
-- order at all.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION ledger.transactions_balance_guard() RETURNS trigger
LANGUAGE plpgsql SET search_path = ledger, pg_temp AS $$
BEGIN
    PERFORM ledger.check_transaction_balance(NEW.id);
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- +goose Down
ALTER FUNCTION ledger.check_transaction_balance(uuid)  RESET search_path;
ALTER FUNCTION ledger.transactions_balance_guard()     RESET search_path;
ALTER FUNCTION ledger.postings_balance_guard()         RESET search_path;
ALTER FUNCTION ledger.transactions_guard()             RESET search_path;
ALTER FUNCTION ledger.postings_append_only()           RESET search_path;
ALTER FUNCTION ledger.verify_account_balances(uuid) RESET search_path;
