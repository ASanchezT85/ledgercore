-- +goose Up
-- LC (verifier) — Integrity check: postings (source of truth) vs
-- account_balances (derived running totals).
--
-- account_balances is maintained incrementally alongside postings, and the
-- trial balance without as_of reads it directly. A bug in balance application
-- (a missed upsert, a double-apply, a manual UPDATE) would drift the derived
-- table away from the postings that back it, and the plain trial balance would
-- not notice. This function recomputes posted/pending debit & credit sums from
-- the postings and returns only the (account, asset) rows where the derived
-- account_balances disagree. An empty result means the derived table is
-- perfectly consistent with its source.
--
-- Firmness mapping mirrors the write path:
--   * posted/reversed postings -> posted_debits/posted_credits
--   * draft postings           -> pending_debits/pending_credits
--
-- SECURITY INVOKER (default): runs under the caller's role and RLS, so it only
-- ever sees the caller's tenant (app.tenant_id must be set by the caller).

-- +goose StatementBegin
CREATE FUNCTION verify_account_balances(p_ledger uuid)
RETURNS TABLE (
    account_id       uuid,
    asset            varchar(12),
    computed_posted_debits   bigint,
    stored_posted_debits     bigint,
    computed_posted_credits  bigint,
    stored_posted_credits    bigint,
    computed_pending_debits  bigint,
    stored_pending_debits    bigint,
    computed_pending_credits bigint,
    stored_pending_credits   bigint
)
LANGUAGE sql STABLE AS $$
    WITH recomputed AS (
        SELECT p.account_id, p.asset,
            COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'  AND t.status IN ('posted', 'reversed')), 0) AS posted_debits,
            COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT' AND t.status IN ('posted', 'reversed')), 0) AS posted_credits,
            COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'DEBIT'  AND t.status = 'draft'), 0) AS pending_debits,
            COALESCE(SUM(p.amount) FILTER (WHERE p.direction = 'CREDIT' AND t.status = 'draft'), 0) AS pending_credits
        FROM postings p
        JOIN transactions t ON t.id = p.transaction_id
        JOIN accounts a ON a.id = p.account_id
        WHERE a.ledger_id = p_ledger
        GROUP BY p.account_id, p.asset
    ),
    stored AS (
        SELECT b.account_id, b.asset,
               b.posted_debits, b.posted_credits, b.pending_debits, b.pending_credits
        FROM account_balances b
        JOIN accounts a ON a.id = b.account_id
        WHERE a.ledger_id = p_ledger
    )
    SELECT
        COALESCE(r.account_id, s.account_id) AS account_id,
        COALESCE(r.asset, s.asset)           AS asset,
        COALESCE(r.posted_debits, 0),   COALESCE(s.posted_debits, 0),
        COALESCE(r.posted_credits, 0),  COALESCE(s.posted_credits, 0),
        COALESCE(r.pending_debits, 0),  COALESCE(s.pending_debits, 0),
        COALESCE(r.pending_credits, 0), COALESCE(s.pending_credits, 0)
    FROM recomputed r
    FULL OUTER JOIN stored s ON s.account_id = r.account_id AND s.asset = r.asset
    WHERE COALESCE(r.posted_debits, 0)   IS DISTINCT FROM COALESCE(s.posted_debits, 0)
       OR COALESCE(r.posted_credits, 0)  IS DISTINCT FROM COALESCE(s.posted_credits, 0)
       OR COALESCE(r.pending_debits, 0)  IS DISTINCT FROM COALESCE(s.pending_debits, 0)
       OR COALESCE(r.pending_credits, 0) IS DISTINCT FROM COALESCE(s.pending_credits, 0)
    ORDER BY account_id, asset;
$$;
-- +goose StatementEnd

COMMENT ON FUNCTION verify_account_balances(uuid) IS
    'Integrity verifier: recomputes balances from postings and returns rows where account_balances (derived) drifts from the source. Empty result = consistent.';

-- +goose Down
DROP FUNCTION IF EXISTS verify_account_balances(uuid);
