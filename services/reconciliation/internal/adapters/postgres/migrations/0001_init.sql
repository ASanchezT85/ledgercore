-- +goose Up
-- Schema "recon" is created by the migrator before goose runs (it also hosts
-- the goose version table). Everything here is schema-qualified anyway.

CREATE TABLE recon.sources (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    name        VARCHAR(255) NOT NULL,
    kind        VARCHAR(16) NOT NULL CHECK (kind IN ('bank', 'psp', 'provider', 'manual')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);
ALTER TABLE recon.sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.sources FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.sources
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);

CREATE TABLE recon.imports (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    source_id   UUID NOT NULL REFERENCES recon.sources (id),
    filename    VARCHAR(512) NOT NULL,
    status      VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'processed', 'failed')),
    row_count   INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE recon.imports ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.imports FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.imports
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE INDEX imports_tenant_source_idx ON recon.imports (tenant_id, source_id);

CREATE TABLE recon.external_transactions (
    id               UUID PRIMARY KEY,
    tenant_id        UUID NOT NULL,
    import_id        UUID NOT NULL REFERENCES recon.imports (id),
    source_id        UUID NOT NULL REFERENCES recon.sources (id),
    external_ref     VARCHAR(255) NOT NULL,
    amount           BIGINT NOT NULL,
    asset            VARCHAR(12) NOT NULL,
    occurred_at      TIMESTAMPTZ NOT NULL,
    raw              JSONB NOT NULL DEFAULT '{}'::jsonb,
    match_status     VARCHAR(16) NOT NULL DEFAULT 'unmatched'
                     CHECK (match_status IN ('unmatched', 'matched', 'disputed')),
    matched_entry_id UUID NULL
);
ALTER TABLE recon.external_transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.external_transactions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.external_transactions
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE INDEX external_tx_matching_idx
    ON recon.external_transactions (tenant_id, source_id, match_status);
CREATE INDEX external_tx_ref_idx
    ON recon.external_transactions (tenant_id, external_ref);

-- Local mirror of ledger postings, fed only by ledger.transaction.posted
-- events (event-carried state transfer). Never populated by querying the
-- ledger schema. reference carries the transaction idempotency_key.
CREATE TABLE recon.ledger_entries_mirror (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL,
    transaction_id UUID NOT NULL,
    account_id     UUID NOT NULL,
    direction      VARCHAR(6) NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    amount         BIGINT NOT NULL,
    asset          VARCHAR(12) NOT NULL,
    reference      VARCHAR(255) NOT NULL,
    posted_at      TIMESTAMPTZ NOT NULL
);
ALTER TABLE recon.ledger_entries_mirror ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.ledger_entries_mirror FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.ledger_entries_mirror
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE INDEX mirror_reference_idx ON recon.ledger_entries_mirror (tenant_id, reference);

CREATE TABLE recon.reconciliation_runs (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    source_id       UUID NOT NULL REFERENCES recon.sources (id),
    status          VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    matched_count   INTEGER NOT NULL DEFAULT 0,
    unmatched_count INTEGER NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ NULL
);
ALTER TABLE recon.reconciliation_runs ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.reconciliation_runs FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.reconciliation_runs
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE INDEX runs_tenant_source_idx ON recon.reconciliation_runs (tenant_id, source_id);

CREATE TABLE recon.discrepancies (
    id         UUID PRIMARY KEY,
    tenant_id  UUID NOT NULL,
    run_id     UUID NOT NULL REFERENCES recon.reconciliation_runs (id),
    kind       VARCHAR(32) NOT NULL
               CHECK (kind IN ('missing_internal', 'missing_external', 'amount_mismatch', 'duplicate')),
    details    JSONB NOT NULL DEFAULT '{}'::jsonb,
    status     VARCHAR(16) NOT NULL DEFAULT 'open'
               CHECK (status IN ('open', 'investigating', 'resolved')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE recon.discrepancies ENABLE ROW LEVEL SECURITY;
ALTER TABLE recon.discrepancies FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recon.discrepancies
    USING (tenant_id = current_setting('app.tenant_id', true)::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true)::uuid);
CREATE INDEX discrepancies_status_idx ON recon.discrepancies (tenant_id, status);
CREATE INDEX discrepancies_run_idx ON recon.discrepancies (tenant_id, run_id);

-- Transactional outbox. Carries tenant_id but has NO RLS on purpose: the
-- poller drains it across tenants inside a system transaction (see
-- pgxutil.WithSystemTx). It is infrastructure, not a business table.
CREATE TABLE recon.outbox (
    id           UUID PRIMARY KEY, -- the envelope id
    tenant_id    UUID NOT NULL,
    topic        VARCHAR(64) NOT NULL,
    envelope     JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ NULL
);
CREATE INDEX outbox_unpublished_idx ON recon.outbox (created_at) WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS recon.outbox;
DROP TABLE IF EXISTS recon.discrepancies;
DROP TABLE IF EXISTS recon.reconciliation_runs;
DROP TABLE IF EXISTS recon.ledger_entries_mirror;
DROP TABLE IF EXISTS recon.external_transactions;
DROP TABLE IF EXISTS recon.imports;
DROP TABLE IF EXISTS recon.sources;
