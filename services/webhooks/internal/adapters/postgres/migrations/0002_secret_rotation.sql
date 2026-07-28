-- +goose Up
-- Secret rotation with a grace period: after a rotation the previous secret
-- stays valid for verification until previous_secret_expires_at (24h). While
-- it is valid the dispatcher signs every delivery with BOTH secrets (two v1
-- entries in X-LedgerCore-Signature), so receivers can switch at their own
-- pace without losing events. Expired previous secrets are purged by the
-- service (background sweep) and never used again.
ALTER TABLE subscriptions
    ADD COLUMN previous_secret            TEXT,
    ADD COLUMN previous_secret_expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE subscriptions
    DROP COLUMN previous_secret,
    DROP COLUMN previous_secret_expires_at;
