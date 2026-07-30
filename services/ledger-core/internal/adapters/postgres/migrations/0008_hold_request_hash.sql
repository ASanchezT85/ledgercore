-- +goose Up
-- R-008 — persist the hold idempotency fingerprint, mirroring idempotency_keys.
--
-- BEFORE: CreateHold recomputed the fingerprint from the STORED hold row on
-- replay (holdFingerprint(existing)). That row does not carry whether the
-- client originally supplied expires_at, so once expires_at became part of the
-- fingerprint the recomputed hash could never reflect the original request's
-- "provided/omitted" bit — a legitimate replay of a hold created WITH an
-- explicit expires_at would be misread as a conflict.
--
-- AFTER: we store the request fingerprint at creation, exactly like
-- idempotency_keys.request_hash for transactions, and compare the STORED hash
-- against the incoming request's fingerprint on replay. Nullable so holds
-- written before this migration replay as before (hash NULL => skip the check).
ALTER TABLE holds ADD COLUMN request_hash BYTEA;

COMMENT ON COLUMN holds.request_hash IS
    'R-008: SHA-256 of the canonical hold request (see postgres.holdFingerprint). Reusing an idempotency key with a different request is a 409 conflict; NULL for rows created before this column existed.';

-- +goose Down
ALTER TABLE holds DROP COLUMN request_hash;
