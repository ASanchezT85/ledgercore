-- +goose Up
-- Schema is also ensured by the service before goose runs (the goose version
-- table itself lives in the webhooks schema via search_path).
-- Schema creation removed: the schema is provisioned by infra (01-init.sql)
-- or by Migrate() in Go; the app role lacks CREATE on the database.

CREATE TABLE subscriptions (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    url         TEXT NOT NULL,
    -- TODO(KMS): encrypt at rest with a per-tenant data key; plaintext is a
    -- deliberate v1 shortcut, the column stays TEXT so the swap is cheap.
    secret      TEXT NOT NULL,
    event_types TEXT[] NOT NULL DEFAULT '{}',
    active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE subscriptions ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscriptions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE INDEX subscriptions_tenant_idx ON subscriptions (tenant_id, created_at);

CREATE TABLE deliveries (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL,
    subscription_id  UUID NOT NULL REFERENCES subscriptions (id) ON DELETE CASCADE,
    event_id         UUID NOT NULL,
    event_type       TEXT NOT NULL,
    payload          JSONB NOT NULL,
    status           TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivered', 'failed', 'dead')),
    attempts         INT NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_status_code INT,
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at     TIMESTAMPTZ,
    -- Idempotent fan-out: one delivery per (subscription, event).
    CONSTRAINT deliveries_subscription_event_unique UNIQUE (subscription_id, event_id)
);

ALTER TABLE deliveries ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON deliveries
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid);

-- Dispatcher scan: due deliveries ordered by next_attempt_at.
CREATE INDEX deliveries_status_next_attempt_idx ON deliveries (status, next_attempt_at);
-- API listing: keyset pagination per tenant.
CREATE INDEX deliveries_tenant_created_idx ON deliveries (tenant_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS deliveries;
DROP TABLE IF EXISTS subscriptions;
