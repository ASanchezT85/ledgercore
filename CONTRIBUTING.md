# Contributing to LedgerCore

Thanks for looking. This is a small project with one maintainer, so the most
useful contributions are focused ones: a failing test, a precise bug report, a
narrow fix.

## The one rule

**A contribution may not weaken a financial invariant.**

The invariants are listed in [`docs/invariants.md`](docs/invariants.md), each
with the test that holds it up. If your change touches

- the ledger schema or any migration under `services/*/internal/adapters/postgres/migrations/`,
- `libs/go/money`,
- the guard triggers (`postings_append_only`, `transactions_guard`,
  `check_transaction_balance`),
- the idempotency fingerprint,

then it needs a test that **fails without your change and passes with it**. Say
so in the pull request, and say which invariant it defends. A change that makes
an invariant weaker needs to explain, in the pull request body, why the
invariant was wrong — not why the change is convenient.

## Local setup

```bash
git clone https://github.com/ASanchezT85/ledgercore.git
cd ledgercore
docker compose -f infra/compose/docker-compose.yml up -d --build
```

Ports are overridable if the defaults clash; see [`.env.example`](.env.example).
If the first `up` fails partway, run `docker compose -f infra/compose/docker-compose.yml down -v`
before retrying — PostgreSQL will otherwise skip its init script forever.

## Tests

```bash
make test-go     # vet + unit tests, no database
```

The integration, RLS-isolation and purge tests need PostgreSQL. Give them a
superuser DSN and each service provisions its own roles and schema, then runs
its tests as the real `NOBYPASSRLS` runtime role:

```bash
LEDGERCORE_TEST_ADMIN_URL='postgres://postgres:postgres@localhost:5432/ledgercore' make test-go
```

`scripts/ci-local.sh` runs the full gate against a clean `git archive` checkout.
Run it before opening a pull request that touches Go or SQL.

> **Note for Windows contributors.** Some Application Control policies block
> test binaries executing from `%TEMP%`. Set `GOTMPDIR` to a directory inside
> the repository (`.gotmp/` is git-ignored) if `go test` reports
> `An Application Control policy has blocked this file`.

## Coding rules

- **Go**: standard library first. `net/http` with `ServeMux` method patterns,
  `pgx/v5`, `log/slog`. A new third-party dependency needs a justification in
  the pull request — this is code that moves money, and the dependency surface
  is deliberately small.
- **Money never touches a float.** Anywhere. Use `libs/go/money`.
- **Every database access goes through `pgxutil.WithTenantTx`**, which sets
  `app.tenant_id` for RLS. A query that bypasses it will read nothing or, worse,
  the wrong thing.
- **No cross-schema queries.** A service reads only its own schema. If you need
  another service's data, call its API or consume its event.
- `gofmt -s` clean, `go vet` clean.
- Code and comments in English.

## Migrations

- One migration per change, numbered, never edited once merged.
- Both an `Up` and a `Down` section.
- If the migration protects an invariant, it needs a test that proves it
  protects it. Migration `0009` is the worked example: it pins `search_path` on
  the guard functions, and `TestBalanceGuardIsSearchPathIndependent` fails
  without it.
- Migrations run as the migrator role, never as a runtime role. Runtime roles
  have no DDL privilege by design — do not grant them any.

## API and contracts

The OpenAPI documents in `contracts/openapi/` are the source of truth and are
written before the handler. `v1` is additive-only: a breaking change is a new
version, not an edit. If you change a contract, update the copy the console
serves under `apps/console/public/openapi/` in the same pull request.

## Pull requests

- One concern per pull request.
- Describe what breaks without the change.
- Include the test output for whatever you touched.
- New behaviour that the README would have to describe should come with the
  README edit.

Please do not add a claim to the README that no test backs. The
["What LedgerCore guarantees"](README.md#what-ledgercore-guarantees) section is
a contract with readers, and every line in it is meant to be traceable to a
test.

## Security

Do not open a public issue for a vulnerability. Follow
[`SECURITY.md`](SECURITY.md).

## Code of conduct

Be decent. Discuss the code, not the person. The maintainer will remove comments
and contributors that make this an unpleasant place to work, and does not owe a
process for it.

## License

By contributing you agree that your contribution is licensed under
[Apache-2.0](LICENSE), the same terms as the project.
