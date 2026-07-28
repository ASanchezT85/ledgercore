-- +goose Up
-- Reporting indexes (fase 1.5): statements and trial balance as-of aggregate
-- postings joined to transactions filtered by effective_at.

-- Historical trial balance: postings of a tenant's transactions up to as_of.
CREATE INDEX transactions_tenant_effective_idx
    ON transactions (tenant_id, effective_at);

-- Statements: all postings of one account (the join then filters by the
-- transaction's effective_at). Complements postings_account_created_idx,
-- which orders by created_at, not by accounting time.
CREATE INDEX postings_account_txn_idx
    ON postings (account_id, transaction_id);

-- +goose Down
DROP INDEX IF EXISTS postings_account_txn_idx;
DROP INDEX IF EXISTS transactions_tenant_effective_idx;
