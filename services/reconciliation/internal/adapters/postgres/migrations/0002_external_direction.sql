-- +goose Up
-- LC-011: the matcher can now take posting direction into account. Statements
-- that carry a direction column populate this; older direction-less imports
-- leave it NULL (the matcher treats NULL as "unspecified" and matches either
-- side, preserving v1 behavior). The table already has FORCE ROW LEVEL
-- SECURITY + tenant_isolation (USING + WITH CHECK) from 0001_init.sql.
ALTER TABLE recon.external_transactions
    ADD COLUMN direction VARCHAR(6) NULL CHECK (direction IN ('DEBIT', 'CREDIT'));

-- +goose Down
ALTER TABLE recon.external_transactions DROP COLUMN IF EXISTS direction;
