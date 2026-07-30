-- +goose Up
-- LC-006 (HIGH) — Idempotency key must be bound to the request payload.
--
-- Before this change idempotency_keys stored only the response snapshot. A
-- client that reused a key with a DIFFERENT payload silently received the
-- original result, masking a real bug (or an attack) instead of surfacing it.
--
-- We add request_hash: the SHA-256 of a canonical, field-ordered JSON of the
-- semantically relevant request fields (see postgres.transactionFingerprint /
-- holdFingerprint). On replay:
--   * hash matches   -> return the stored response (X-Idempotent-Replay: true)
--   * hash differs   -> 409 idempotency_conflict
-- The column is nullable so rows written before this migration keep working
-- (a NULL stored hash is treated as "no fingerprint on file" and replays as
-- before, never as a conflict).

ALTER TABLE idempotency_keys ADD COLUMN request_hash BYTEA;

COMMENT ON COLUMN idempotency_keys.request_hash IS
    'SHA-256 of the canonical JSON of the request payload. Replays whose payload hashes differently are rejected with 409 idempotency_conflict. NULL only for rows written before migration 0004.';

-- +goose Down
ALTER TABLE idempotency_keys DROP COLUMN request_hash;
