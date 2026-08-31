# SECRET_AUDIT

**Date:** 2026-08-31 · **Scope:** `ASanchezT85/ledgercore`, complete history ·
**Tool:** `gitleaks v8` (default ruleset)

## Result

**Clean.** No credential has ever been committed to this repository.

| Scan | Coverage | Findings |
|---|---|---|
| `gitleaks git` **without the project allowlist** | all 63 commits, 6.15 MB | **0** |
| `gitleaks git -c .gitleaks.toml` | all commits | 0 |
| `gitleaks dir` (working tree, includes ignored files) | 110 MB | 82, all in git-ignored build artefacts — see below |
| Targeted history grep (private keys, cloud tokens, password assignments) | all commits | 0 real |

The first row is the one that matters. The repository ships a
`.gitleaks.toml` with an allowlist for documented example credentials and test
fixtures, and an allowlist can hide a real leak. So the authoritative scan was
run **with the allowlist disabled**, using only gitleaks' default rules. It
reports nothing across the entire history.

## Method

This was not a scan of the current checkout. Deleting a file does not remove it
from the history, so the whole commit graph was scanned:

1. **`gitleaks git`, no config** — every commit, every blob, default rules.
2. **Targeted regex sweep over `git log --all -p`** for shapes gitleaks weights
   loosely: `-----BEGIN … PRIVATE KEY`, `AKIA…`, `ghp_`, `github_pat_`, `xox[bapr]-`,
   `sk-…`, and `password|secret|token|api_key` assignments with a value of 12+
   characters.
3. **Manual review of every hex string of 32 or 64 characters** in the history —
   the shape of this project's master key.
4. **File-add history** (`git log --diff-filter=A --name-only`) filtered for
   `.env`, `*.pem`, `*.key`, `id_rsa`, `*secret*`.

## Every candidate, and why it is not a secret

| Value / pattern | Where | Verdict |
|---|---|---|
| `-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIA==\n-----END…` | `keycrypt` unit test | **Safe.** A 16-byte base64 stub, not a usable Ed25519 key. Exists to exercise the parse path. |
| `b988dac0…`, `8d6e0eb1…`, `42f72bb4…` (64 hex) | `services/webhooks/internal/signature/signature_test.go` | **Safe.** Known-answer HMAC-SHA256 *output* vectors. Publishing a signature reveals nothing about the key. |
| `5257a869…` (64 hex) | webhooks documentation page | **Safe.** An illustrative `X-LedgerCore-Signature` header in a docs example. |
| `9f1b2c3d4e5f60718293a4b5c6d7e8f9` (×14) | tests and docs | **Safe.** A hand-written pattern used as a placeholder identifier. |
| `6465762d…646576` | `docker-compose.yml` | **Safe.** Hex encoding of the ASCII string `dev-only-master-key-32-bytes-dev`. Deliberately public, and the hardened overlay refuses to start with it. |
| `a75493003c43342d…` (32 hex) | `sdks/php/composer.lock` | **Safe.** Composer's content hash. |
| `ledgercore_*_dev` passwords | `docker-compose.yml`, tests | **Safe.** Documented development defaults. The `docker-compose.sandbox.yml` overlay declares every one of them as `${VAR:?}`, so a non-development deployment cannot boot with them. |
| `whsec_…0000…`, `lk_sandbox_9f8e7d6c…` | tests, OpenAPI, SDK docs | **Safe.** Example credential shapes; never issued. |
| `LEDGERCORE_ADMIN_TOKEN=dev-admin-token` | dev default | **Safe.** `requireHardenedSandbox()` refuses to start when `LEDGERCORE_ENV=sandbox-public` and this value is present. |

### The 82 working-tree findings

All in files that are git-ignored and cannot reach a publish:

- **81 in `apps/console/.next/`** — Next.js build output. Regenerated on every
  build, ignored by `.gitignore`, never committed (verified: no `.next` path
  appears anywhere in the history).
- **1 in `infra/compose/.env`** — a local development file holding
  `LEDGERCORE_MASTER_KEY`. Ignored by `.gitignore:34` (confirmed with
  `git check-ignore -v`), and never committed at any point in the history.

No `.env` file other than `.env.example` has ever existed in a commit.

## Secrets belonging to the retired deployment

The hosted sandbox at `ledgercore.sanchezavila.com` is being decommissioned (see
[`VPS_RETIREMENT.md`](VPS_RETIREMENT.md)). Every secret that existed only to
serve it is treated as **dead, and as compromised**:

| Secret | Disposition |
|---|---|
| `LEDGERCORE_MASTER_KEY` (production value) | Retired with the host. It encrypted signing keys and webhook secrets in a database holding no tenant data. **Must never be reused** in any future deployment. |
| Per-role PostgreSQL passwords | Retired with the database. |
| `LEDGERCORE_ADMIN_TOKEN` | Retired with the service. |
| Blog hash salt and moderation token | The blog subsystem was removed entirely. |
| Read-only deploy key on the host | To be revoked from the repository's deploy keys — **manual action, see `VPS_RETIREMENT.md`**. |

**Prior incident, recorded for completeness.** During an external audit in July
2026, `infra/compose/.env` was twice included in a package produced by
compressing the working directory. It never entered git, but the master key was
disclosed to a third party and was rotated on each occasion. The lesson was
encoded as `scripts/export-clean.sh`, which builds packages from
`git archive` — tracked files only — and runs gitleaks twice over the result.
That script is why no equivalent leak has happened since.

## Standing controls

- `.gitignore` excludes `.env`, `.env.*` (keeping `.env.example`), `*.pem`,
  `*.key`, `*.dump`, `*.sql.gz`, `backups/`.
- `.gitleaks.toml` is used by CI and by `scripts/export-clean.sh`. Its allowlist
  covers documented examples and test fixtures **only**; real service source,
  `infra/`, and any `.env` are explicitly not allowlisted.
- `scripts/export-clean.sh` is the only sanctioned way to produce an archive of
  this repository.

## Recommendations

1. **Enable GitHub secret scanning and push protection** when the repository
   becomes public. It is free for public repositories and catches the next
   mistake at push time rather than at audit time.
2. **Revoke the deploy key** used by the retired host.
3. Keep running `gitleaks` **without** the project allowlist in any future
   audit. An allowlist is a convenience for CI, not evidence.

## Verdict

**No blocker.** Nothing in this repository's history prevents publication on
secret-handling grounds. No history rewrite is required to remove a credential.
