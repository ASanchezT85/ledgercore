-- +goose Up
-- Schema "ledger" is created by the migration runner before goose runs and
-- every connection pins search_path=ledger, so all objects land there.
--
-- Multi-tenancy: every business table carries tenant_id and RLS with the
-- tenant_isolation policy (app.tenant_id is set per transaction via
-- SET LOCAL by pgxutil.WithTenantTx). FORCE ROW LEVEL SECURITY makes the
-- policies apply to the table owner too (the dev role owns these tables).
--
-- Maintenance hatch: the append-only triggers on postings and transactions
-- can be bypassed by running SET ledger.allow_maintenance = '1' inside a
-- transaction. It exists ONLY for controlled data-repair operations run by a
-- human; application code must never set it.

-- ---- ledgers ----------------------------------------------------------------

CREATE TABLE ledgers (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'live')),
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledgers_tenant_name_key UNIQUE (tenant_id, name)
);

COMMENT ON TABLE ledgers IS 'An isolated chart of accounts within a tenant.';
COMMENT ON COLUMN ledgers.environment IS 'sandbox or live; taken from the token that created the ledger.';

ALTER TABLE ledgers ENABLE ROW LEVEL SECURITY;
ALTER TABLE ledgers FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON ledgers
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---- accounts ---------------------------------------------------------------

CREATE TABLE accounts (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL,
    ledger_id      UUID NOT NULL REFERENCES ledgers (id),
    name           TEXT NOT NULL,
    path           TEXT NOT NULL,
    type           TEXT NOT NULL CHECK (type IN ('asset', 'liability', 'equity', 'revenue', 'expense')),
    normal_balance TEXT NOT NULL CHECK (normal_balance IN ('debit', 'credit')),
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'frozen', 'closed')),
    metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT accounts_tenant_ledger_path_key UNIQUE (tenant_id, ledger_id, path)
);

COMMENT ON TABLE accounts IS 'Accounts hold balances per asset and receive postings.';
COMMENT ON COLUMN accounts.path IS 'Colon-namespaced identifier, unique within (tenant, ledger).';
COMMENT ON COLUMN accounts.normal_balance IS 'Side on which the account grows: debit or credit. Available balance is signed by it.';

CREATE INDEX accounts_ledger_idx ON accounts (ledger_id);

ALTER TABLE accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON accounts
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---- transactions -----------------------------------------------------------

CREATE TABLE transactions (
    id              UUID PRIMARY KEY,
    tenant_id       UUID NOT NULL,
    ledger_id       UUID NOT NULL REFERENCES ledgers (id),
    reference       TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('draft', 'posted', 'reversed')),
    effective_at    TIMESTAMPTZ NOT NULL,
    posted_at       TIMESTAMPTZ,
    reversed_by_id  UUID REFERENCES transactions (id),
    reverses_id     UUID REFERENCES transactions (id),
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT transactions_tenant_idempotency_key UNIQUE (tenant_id, idempotency_key)
);

COMMENT ON TABLE transactions IS 'Balanced sets of postings. Lifecycle: draft -> posted -> reversed, enforced by trigger.';
COMMENT ON COLUMN transactions.idempotency_key IS 'Client-supplied key, unique per tenant. Retries return the stored original response.';
COMMENT ON COLUMN transactions.reversed_by_id IS 'Set on the original transaction when it is reversed; points at the compensating transaction.';
COMMENT ON COLUMN transactions.reverses_id IS 'Set on a reversal transaction; points back at the original. Immutable after insert.';
COMMENT ON COLUMN transactions.effective_at IS 'Accounting effective instant; defaults to creation time.';

CREATE INDEX transactions_ledger_created_idx ON transactions (ledger_id, created_at DESC, id DESC);

ALTER TABLE transactions ENABLE ROW LEVEL SECURITY;
ALTER TABLE transactions FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON transactions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- Immutability guard: only the lifecycle transitions draft->posted (setting
-- posted_at) and posted->reversed (setting reversed_by_id) may UPDATE a row;
-- DELETE is forbidden. Escape hatch: SET ledger.allow_maintenance = '1'
-- (documented at the top of this file) bypasses the guard for manual repair.
-- +goose StatementBegin
CREATE FUNCTION transactions_guard() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'ledger transactions are append-only (DELETE forbidden)';
    END IF;

    IF NEW.id              IS DISTINCT FROM OLD.id
       OR NEW.tenant_id       IS DISTINCT FROM OLD.tenant_id
       OR NEW.ledger_id       IS DISTINCT FROM OLD.ledger_id
       OR NEW.reference       IS DISTINCT FROM OLD.reference
       OR NEW.description     IS DISTINCT FROM OLD.description
       OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key
       OR NEW.effective_at    IS DISTINCT FROM OLD.effective_at
       OR NEW.reverses_id     IS DISTINCT FROM OLD.reverses_id
       OR NEW.metadata        IS DISTINCT FROM OLD.metadata
       OR NEW.created_at      IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'ledger transactions: only status, posted_at and reversed_by_id may change';
    END IF;

    -- draft -> posted: posted_at must be set now, reversed_by_id untouched.
    IF OLD.status = 'draft' AND NEW.status = 'posted'
       AND OLD.posted_at IS NULL AND NEW.posted_at IS NOT NULL
       AND NEW.reversed_by_id IS NOT DISTINCT FROM OLD.reversed_by_id THEN
        RETURN NEW;
    END IF;

    -- posted -> reversed: reversed_by_id must be set now, posted_at untouched.
    IF OLD.status = 'posted' AND NEW.status = 'reversed'
       AND OLD.reversed_by_id IS NULL AND NEW.reversed_by_id IS NOT NULL
       AND NEW.posted_at IS NOT DISTINCT FROM OLD.posted_at THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid transaction transition % -> % (allowed: draft->posted, posted->reversed)',
        OLD.status, NEW.status;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER transactions_guard
    BEFORE UPDATE OR DELETE ON transactions
    FOR EACH ROW EXECUTE FUNCTION transactions_guard();

-- ---- postings ---------------------------------------------------------------

CREATE TABLE postings (
    id             UUID PRIMARY KEY,
    tenant_id      UUID NOT NULL,
    transaction_id UUID NOT NULL REFERENCES transactions (id),
    account_id     UUID NOT NULL REFERENCES accounts (id),
    direction      TEXT NOT NULL CHECK (direction IN ('DEBIT', 'CREDIT')),
    asset          VARCHAR(12) NOT NULL,
    amount         BIGINT NOT NULL CHECK (amount > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE postings IS 'Individual transaction legs. Append-only: never updated or deleted.';
COMMENT ON COLUMN postings.amount IS 'Strictly positive int64 in minor units of the asset; the sign lives in direction.';
COMMENT ON COLUMN postings.asset IS 'Asset code (A-Z0-9, max 12). The display exponent lives in the service asset registry.';

CREATE INDEX postings_transaction_id_idx ON postings (transaction_id);
CREATE INDEX postings_account_created_idx ON postings (account_id, created_at);

ALTER TABLE postings ENABLE ROW LEVEL SECURITY;
ALTER TABLE postings FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON postings
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- Postings are strictly append-only. Same maintenance hatch as transactions.
-- +goose StatementBegin
CREATE FUNCTION postings_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('ledger.allow_maintenance', true) = '1' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'ledger postings are append-only';
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER postings_append_only
    BEFORE UPDATE OR DELETE ON postings
    FOR EACH ROW EXECUTE FUNCTION postings_append_only();

-- ---- account_balances ---------------------------------------------------------

CREATE TABLE account_balances (
    account_id      UUID NOT NULL REFERENCES accounts (id),
    tenant_id       UUID NOT NULL,
    asset           VARCHAR(12) NOT NULL,
    posted_debits   BIGINT NOT NULL DEFAULT 0,
    posted_credits  BIGINT NOT NULL DEFAULT 0,
    pending_debits  BIGINT NOT NULL DEFAULT 0,
    pending_credits BIGINT NOT NULL DEFAULT 0,
    held            BIGINT NOT NULL DEFAULT 0,
    version         BIGINT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, asset)
);

COMMENT ON TABLE account_balances IS 'Running per-asset balances, maintained in the same transaction as postings.';
COMMENT ON COLUMN account_balances.posted_debits IS 'Sum of posted DEBIT postings in minor units.';
COMMENT ON COLUMN account_balances.posted_credits IS 'Sum of posted CREDIT postings in minor units.';
COMMENT ON COLUMN account_balances.pending_debits IS 'Sum of DEBIT postings of draft transactions.';
COMMENT ON COLUMN account_balances.pending_credits IS 'Sum of CREDIT postings of draft transactions.';
COMMENT ON COLUMN account_balances.held IS 'Total of active holds; subtracted from available.';
COMMENT ON COLUMN account_balances.version IS 'Optimistic concurrency counter, bumped on every balance change.';

ALTER TABLE account_balances ENABLE ROW LEVEL SECURITY;
ALTER TABLE account_balances FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON account_balances
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---- holds --------------------------------------------------------------------

CREATE TABLE holds (
    id                      UUID PRIMARY KEY,
    tenant_id               UUID NOT NULL,
    ledger_id               UUID NOT NULL REFERENCES ledgers (id),
    account_id              UUID NOT NULL REFERENCES accounts (id),
    idempotency_key         TEXT NOT NULL,
    asset                   VARCHAR(12) NOT NULL,
    amount                  BIGINT NOT NULL CHECK (amount > 0),
    status                  TEXT NOT NULL CHECK (status IN ('active', 'captured', 'released', 'expired')),
    expires_at              TIMESTAMPTZ NOT NULL,
    captured_transaction_id UUID REFERENCES transactions (id),
    metadata                JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT holds_tenant_idempotency_key UNIQUE (tenant_id, idempotency_key)
);

COMMENT ON TABLE holds IS 'Two-phase authorizations reserving funds until captured or released.';
COMMENT ON COLUMN holds.captured_transaction_id IS 'Posted transaction linked at capture time, when provided.';

CREATE INDEX holds_account_status_idx ON holds (account_id, status);

ALTER TABLE holds ENABLE ROW LEVEL SECURITY;
ALTER TABLE holds FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON holds
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---- idempotency_keys -----------------------------------------------------------

CREATE TABLE idempotency_keys (
    tenant_id       UUID NOT NULL,
    idempotency_key TEXT NOT NULL,
    status_code     INT NOT NULL,
    response        JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, idempotency_key)
);

COMMENT ON TABLE idempotency_keys IS 'Stored responses per idempotency key: replays return the original response byte-for-byte.';
COMMENT ON COLUMN idempotency_keys.response IS 'Serialized transaction snapshot captured when the key was first used.';

ALTER TABLE idempotency_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE idempotency_keys FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON idempotency_keys
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- ---- outbox --------------------------------------------------------------------

CREATE TABLE outbox (
    id           BIGSERIAL PRIMARY KEY,
    tenant_id    UUID NOT NULL,
    topic        TEXT NOT NULL,
    envelope     JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

COMMENT ON TABLE outbox IS 'Transactional outbox: events are inserted in the business transaction and published to NATS by a poller.';
COMMENT ON COLUMN outbox.envelope IS 'Full events.Envelope JSON, ready to publish to the topic.';
COMMENT ON COLUMN outbox.published_at IS 'NULL until the poller publishes the event; then the publish instant.';

CREATE INDEX outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;

ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;
ALTER TABLE outbox FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON outbox
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
-- The outbox poller drains events across tenants in a system transaction
-- (no app.tenant_id set). This additional permissive policy grants access
-- exactly in that case; tenant-scoped transactions keep seeing only their
-- own rows through tenant_isolation.
CREATE POLICY system_drain ON outbox
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- +goose Down
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS holds;
DROP TABLE IF EXISTS account_balances;
DROP TRIGGER IF EXISTS postings_append_only ON postings;
DROP TABLE IF EXISTS postings;
DROP FUNCTION IF EXISTS postings_append_only();
DROP TRIGGER IF EXISTS transactions_guard ON transactions;
DROP TABLE IF EXISTS transactions;
DROP FUNCTION IF EXISTS transactions_guard();
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS ledgers;
