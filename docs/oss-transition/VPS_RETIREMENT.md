# VPS_RETIREMENT

**Date executed:** 2026-08-31 · **Status: complete on the host. One manual step
remains (DNS).**

LedgerCore ran a hosted sandbox on a shared 2 vCPU / 2 GB VPS that also hosts the
owner's personal portfolio. The commercial positioning is retired, so the
resources go back to the portfolio and to other work. The open-source project
does not depend on any of it: everything below can be reproduced with
`docker compose up`.

---

## What was there

| Item | Detail |
|---|---|
| Containers | 7 (`ledger-core`, `identity`, `reconciliation`, `webhooks`, `console`, `postgres`, `nats`) plus a `migrate` one-shot |
| Volumes | `ledgercore_postgres_data` (64 MB), `ledgercore_go_cache` (776 MB) |
| Code | `/opt/ledgercore` (14 MB), pulled with a read-only deploy key |
| Boot guard | `ledgercore-stack.service` (systemd, enabled) |
| Web | Two vhosts in the **portfolio's** Caddyfile, on the shared `web` Docker network |
| Images | 5, 434 MB total |
| DNS | `ledgercore` and `api.ledgercore` A records → `162.213.253.226` |
| Backups | None. `scripts/backup/install-cron.sh` existed but had never been installed. |

**Shared with the portfolio** — and therefore not to be touched: Caddy (TLS
termination on 80/443), the external `web` network, the host's Docker daemon and
its build-cache guard cron.

## What was checked before deleting anything

Deleting is easy; deleting something irreplaceable is the risk. In order:

1. **The repository holds everything needed to rebuild.** The full local stack
   is `infra/compose/docker-compose.yml`; nothing on the host was unique.
2. **A mirror backup of the repository** was taken outside it —
   `ledgercore-backup-20260831.git`, holding the complete pre-transition
   history. It stays private.
3. **Private documents were preserved outside the repository** before removal —
   runbooks, pitch material, policies, the blog posts.
4. **The database was inspected, not assumed:**

   ```sql
   SELECT schemaname, relname, n_live_tup FROM pg_stat_user_tables WHERE n_live_tup > 0;
    identity | outbox     | 1
    blog     | post_views | 1
   ```

   **No tenants. No API keys. No ledger data. No signups. No personal data.**
   The sandbox never accumulated real usage, so retirement is a deletion, not a
   migration, and there is no data-subject obligation to discharge.
5. **A final `pg_dumpall` was taken anyway** — 15 KB, at
   `/root/ledgercore-final-20260831.sql.gz` on the host. Kept because it costs
   nothing; it contains no personal data.

## Execution

Ordered so that the portfolio was never at risk, and verified at the step where
it could have been.

### 1 · Web routing removed

The LedgerCore vhosts lived inside the portfolio's Caddyfile, delimited by
`# === LedgerCore` / `# === end LedgerCore`. This was the one step that could
have taken the portfolio down.

```bash
cp Caddyfile Caddyfile.bak-20260831        # rollback in place first
# remove the delimited block, asserting alexander.sanchezavila.com survives
docker exec asanchest-portfolio-caddy-1 caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
#   → Valid configuration
docker exec asanchest-portfolio-caddy-1 caddy reload  --config /etc/caddy/Caddyfile --adapter caddyfile
```

Verified immediately, three times:

```
https://alexander.sanchezavila.com/   200, 200, 200
```

The edit script asserted both that no `ledgercore` string remained **and** that
`alexander.sanchezavila.com` was still present, so a bad regex would have failed
before writing rather than after reloading.

### 2 · Boot guard removed

```bash
systemctl disable --now ledgercore-stack.service
rm -f /etc/systemd/system/ledgercore-stack.service
systemctl daemon-reload
```

Done before the teardown, so nothing could bring the stack back on the next
reboot.

### 3 · Stack and data removed

```bash
cd /opt/ledgercore/infra/compose
docker compose -f docker-compose.yml -f docker-compose.sandbox.yml \
               -f docker-compose.vps-shared.yml --profile web down -v --remove-orphans
```

Compose is project-scoped, so `-v` and `--remove-orphans` acted only on the
`ledgercore` project. Both volumes and the `ledgercore_default` network were
removed. The portfolio's four volumes were untouched.

### 4 · Code, images and build cache

```bash
docker rmi ledgercore-{console,ledger-core,webhooks,identity,reconciliation}:latest
rm -rf /opt/ledgercore
docker image prune -f && docker builder prune -af
```

`docker builder prune` reclaimed 2.0 GB of build cache — the same cache that
once filled this disk and took PostgreSQL down with it, so clearing it was worth
doing on its own.

### 5 · Credentials revoked

```bash
gh api -X DELETE repos/ASanchezT85/ledgercore/keys/158528622   # vps-sandbox-readonly
rm -f ~/.ssh/ledgercore_deploy ~/.ssh/ledgercore_deploy.pub
```

The repository now has **zero deploy keys**. Every other secret that existed only
to serve this deployment — the master key, the per-role database passwords, the
admin token — died with the volume and is recorded as compromised-by-default in
[`SECRET_AUDIT.md`](SECRET_AUDIT.md#secrets-belonging-to-the-retired-deployment).
None may be reused.

## Result

```
Disk        52% → 45%          (≈3 GB returned)
Containers  10  → 3            (portfolio only)
Volumes     6   → 4            (portfolio only)
/opt        only portfolio artefacts remain
systemd     no ledgercore unit
cron        no ledgercore entry
```

Final probe:

```
https://alexander.sanchezavila.com/               200
https://ledgercore.sanchezavila.com/              no response
https://api.ledgercore.sanchezavila.com/healthz   no response
```

The portfolio's own deploy cron restarted its app container at 16:41 UTC,
independently of this work (`RestartCount: 0`, its log records `OK`). Noted so
the timing is not misread as collateral damage.

## MANUAL ACTION REQUIRED — DNS

Two A records still resolve to the VPS:

```
ledgercore.sanchezavila.com      → 162.213.253.226
api.ledgercore.sanchezavila.com  → 162.213.253.226
```

They now point at a host that serves nothing for them. This is harmless but
untidy, and it must be done from the Namecheap control panel, which is outside
this session's reach.

**In Namecheap → Domain List → `sanchezavila.com` → Advanced DNS, delete the two
A records `ledgercore` and `api.ledgercore`.** Leave every other record alone —
the bare domain and `alexander` serve the portfolio.

### Should a redirect be left instead?

Considered and **not** recommended. A redirect to the GitHub repository would
require keeping a Caddy vhost, a certificate renewal and a DNS record alive for
a hostname nothing links to any more — reintroducing the dependency this
transition removed. Nothing published points at these hostnames: they are gone
from the OpenAPI documents, the console, both SDK READMEs and the repository
description. There is no inbound link to preserve.

If a redirect is wanted anyway, it is three lines in the portfolio's Caddyfile:

```caddyfile
ledgercore.sanchezavila.com, api.ledgercore.sanchezavila.com {
	redir https://github.com/ASanchezT85/ledgercore{uri} permanent
}
```

Trading a permanent maintenance obligation for a courtesy nobody is asking for.

## Rebuilding, if it is ever needed

There is no recovery procedure to preserve, because there is nothing to recover:

```bash
git clone https://github.com/ASanchezT85/ledgercore.git
cd ledgercore
docker compose -f infra/compose/docker-compose.yml up -d --build
```

For a public deployment, add the hardened overlay
(`docker-compose.sandbox.yml`), which requires every secret through `${VAR:?}`
and refuses to boot with the repository's development defaults. Generate fresh
values — **none of the retired ones may be reused.**

## Not touched

The portfolio, in every respect: its containers, its volumes, its database, its
deploy and watchdog crons, its Caddy container, its `web` network, and the
`alexander.sanchezavila.com` vhost. The Docker build-cache guard cron
(`/etc/cron.d/docker-cache-guard`) is host-level and stays.
