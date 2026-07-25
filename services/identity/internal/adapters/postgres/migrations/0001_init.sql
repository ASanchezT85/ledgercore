-- +goose Up
-- Identity service schema. Owned exclusively by this service; no other
-- service may touch the "identity" schema.
-- Schema creation removed: the schema is provisioned by infra (01-init.sql)
-- or by Migrate() in Go; the app role lacks CREATE on the database.

-- ---------------------------------------------------------------------------
-- tenants: the system-level tenant registry.
--
-- WHY NO RLS HERE: this table is the ROOT of the multi-tenant model. Rows are
-- created and read by platform operators (admin bootstrap endpoints) and by
-- the token issuance flow BEFORE any tenant context exists — there is no
-- app.tenant_id to scope by at that point. Scoping the registry by tenant
-- would be circular: you need to read the tenant row to establish the tenant
-- context in the first place. Isolation for tenant-owned data lives in the
-- business schemas (ledger, recon, webhooks), all of which carry RLS.
-- ---------------------------------------------------------------------------
CREATE TABLE identity.tenants (
    id         UUID PRIMARY KEY,
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- api_keys: long-lived credentials. Only the SHA-256 hash of the secret and a
-- short lookup prefix are stored; the plaintext is returned once at creation.
--
-- RLS: enabled with two OR-ed policies. When a transaction runs with a tenant
-- context (pgxutil.WithTenantTx sets app.tenant_id), the standard
-- tenant_isolation policy applies. System paths (admin bootstrap, token
-- issuance — which must look keys up by prefix BEFORE knowing the tenant) run
-- without tenant context and are allowed by system_access. NULLIF guards
-- against the empty-string placeholder Postgres leaves after a SET LOCAL
-- transaction ends, which would otherwise fail the ::uuid cast.
-- ---------------------------------------------------------------------------
CREATE TABLE identity.api_keys (
    id          UUID PRIMARY KEY,
    tenant_id   UUID NOT NULL REFERENCES identity.tenants (id),
    environment TEXT NOT NULL CHECK (environment IN ('sandbox', 'live')),
    name        TEXT NOT NULL,
    key_prefix  VARCHAR(16) NOT NULL,
    secret_hash BYTEA NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX api_keys_key_prefix_idx ON identity.api_keys (key_prefix);
CREATE INDEX api_keys_tenant_id_idx ON identity.api_keys (tenant_id);

ALTER TABLE identity.api_keys ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON identity.api_keys
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY system_access ON identity.api_keys
    USING (NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- ---------------------------------------------------------------------------
-- signing_keys: Ed25519 key pairs used to sign the JWTs this service issues.
--
-- WHY NO RLS HERE: signing keys are platform-wide infrastructure (system
-- level); they do not belong to any tenant. The public halves are served to
-- the whole world via /.well-known/jwks.json.
--
-- TODO(kms): in production the private key must live in a KMS/HSM; persisting
-- PEM here is a development bootstrap convenience only.
-- ---------------------------------------------------------------------------
CREATE TABLE identity.signing_keys (
    kid             UUID PRIMARY KEY,
    private_key_pem TEXT NOT NULL,
    public_key_pem  TEXT NOT NULL,
    algorithm       TEXT NOT NULL DEFAULT 'EdDSA' CHECK (algorithm = 'EdDSA'),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    active          BOOLEAN NOT NULL DEFAULT true
);

-- +goose Down
DROP TABLE IF EXISTS identity.signing_keys;
DROP TABLE IF EXISTS identity.api_keys;
DROP TABLE IF EXISTS identity.tenants;
