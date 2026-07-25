-- +goose Up
-- Sandbox self-service signup + tenant TTL support (Fase 1a).

-- Tenants gain a TTL (sandbox only) and two lifecycle statuses used by the
-- sweeper: purging (expiry announced, grace period running) and purged
-- (grace elapsed, credentials deleted, downstream schemas cleaned).
ALTER TABLE identity.tenants
    DROP CONSTRAINT tenants_status_check;
ALTER TABLE identity.tenants
    ADD CONSTRAINT tenants_status_check
    CHECK (status IN ('active', 'suspended', 'purging', 'purged'));
ALTER TABLE identity.tenants
    ADD COLUMN expires_at TIMESTAMPTZ;

COMMENT ON COLUMN identity.tenants.expires_at IS
    'Sandbox tenant TTL. NULL = never expires. Sweeper announces expiry and purges after a grace period.';

CREATE INDEX tenants_expiring_idx ON identity.tenants (expires_at)
    WHERE expires_at IS NOT NULL AND status <> 'purged';

-- One self-service signup per email, forever, plus the global per-day cap
-- (counted over created_at). System-level table: no RLS (same rationale as
-- tenants — rows exist before any tenant context does).
CREATE TABLE identity.sandbox_signups (
    email      TEXT PRIMARY KEY,
    tenant_id  UUID NOT NULL REFERENCES identity.tenants (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sandbox_signups_created_idx ON identity.sandbox_signups (created_at);

-- Transactional outbox, same pattern as ledger-core. Identity is a
-- system-level service; the sweeper writes envelopes here inside the same
-- transaction that flips tenant status, and a poller drains them to NATS.
-- No RLS: rows are written and drained by system paths only.
CREATE TABLE identity.outbox (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    UUID NOT NULL,
    topic        TEXT NOT NULL,
    envelope     JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX identity_outbox_unpublished_idx ON identity.outbox (id)
    WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS identity.outbox;
DROP TABLE IF EXISTS identity.sandbox_signups;
DROP INDEX IF EXISTS identity.tenants_expiring_idx;
ALTER TABLE identity.tenants DROP COLUMN IF EXISTS expires_at;
ALTER TABLE identity.tenants DROP CONSTRAINT tenants_status_check;
ALTER TABLE identity.tenants
    ADD CONSTRAINT tenants_status_check CHECK (status IN ('active', 'suspended'));
