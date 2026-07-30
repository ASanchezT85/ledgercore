-- Post-migration ownership + EXECUTE grants (run by the `migrate` one-shot as
-- the migrator, AFTER goose has applied every service's migrations).
--
-- Why this exists: service migrations create SECURITY DEFINER maintenance/purge
-- functions but deliberately leave ownership and EXECUTE grants to infra (see
-- the "INFRA COORDINATION" block in ledger-core's purge migration). The append-
-- only guards those functions bypass are keyed on `current_user =
-- 'ledgercore_maint'`, which only holds when the function is OWNED BY
-- ledgercore_maint and marked SECURITY DEFINER. So after every migration run we:
--   1. reassign every SECURITY DEFINER function in each service schema to
--      ledgercore_maint (idempotent — already-owned ones are a no-op);
--   2. grant EXECUTE on it to that schema's runtime role, so the service can
--      invoke the sanctioned maintenance path without ever authenticating as,
--      or being able to SET ROLE to, ledgercore_maint.
--
-- Generic on purpose: infra never hard-codes function names, so new SECURITY
-- DEFINER maintenance functions added by any service are picked up automatically.

DO $$
DECLARE
    sch  text;
    rt   text;
    fn   text;
    map  text[][] := ARRAY[
        ['ledger',   'ledgercore_ledger_rt'],
        ['identity', 'ledgercore_identity_rt'],
        ['recon',    'ledgercore_recon_rt'],
        ['webhooks', 'ledgercore_webhooks_rt']
    ];
    i int;
BEGIN
    FOR i IN 1 .. array_length(map, 1) LOOP
        sch := map[i][1];
        rt  := map[i][2];
        FOR fn IN
            SELECT p.oid::regprocedure::text
            FROM pg_proc p
            JOIN pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname = sch
              AND p.prosecdef            -- SECURITY DEFINER only
        LOOP
            EXECUTE format('ALTER FUNCTION %s OWNER TO ledgercore_maint', fn);
            EXECUTE format('GRANT EXECUTE ON FUNCTION %s TO %I', fn, rt);
            RAISE NOTICE 'maint-owned; EXECUTE granted to %: %', rt, fn;
        END LOOP;
    END LOOP;
END $$;

-- Webhooks maintenance functions (claim_due_deliveries, set_encrypted_secret)
-- run as ledgercore_maint and UPDATE their tables. The init grants maint only
-- SELECT+DELETE ("no INSERT/UPDATE"), so without this those SECURITY DEFINER
-- functions would fail. Granting UPDATE only on the webhooks tables keeps the
-- role narrow (still no INSERT anywhere; UPDATE only where a sanctioned maint
-- function needs it). maint is NOLOGIN and reachable only via those functions.
GRANT UPDATE ON webhooks.subscriptions, webhooks.deliveries TO ledgercore_maint;
