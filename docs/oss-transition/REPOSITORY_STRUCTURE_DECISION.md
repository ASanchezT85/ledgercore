# REPOSITORY_STRUCTURE_DECISION

**Date:** 2026-08-31 · **Decision: keep the current split — a monorepo for the
core, and one repository per SDK.** No migration.

## The current shape

| Repository | Contents |
|---|---|
| `ledgercore` | Four Go services, `libs/go`, OpenAPI and event contracts, `infra/`, the Next.js console, docs |
| `ledgercore-sdk-typescript` | TypeScript SDK, published as `@ledgercore/sdk` |
| `ledgercore-sdk-php` | PHP SDK, published as `ledgercore/sdk` |

The SDK sources also exist under `sdks/` in the monorepo, mirrored out to their
own repositories for publication.

## Options considered

**A. Status quo** — monorepo for the core, separate repositories for the SDKs.
**B. Full monorepo** — pull the SDK repositories in; publish from subdirectories.
**C. Full polyrepo** — split each service into its own repository.

## Assessment

### Releases

The four services release together: they share a database, a role model and a
migration ordering, and a release is "this stack, at this commit". A monorepo
expresses that directly.

SDKs are the opposite. `@ledgercore/sdk@0.2.0` must be publishable when a client
bug is fixed, without implying anything shipped in the ledger. Their consumers
pin them independently. Publishing from a monorepo subdirectory is possible but
means tag namespacing, path-filtered release workflows and a permanent
explanation of which tags mean what.

### Coupling

The services are tightly coupled through the database and the contracts — a
migration and the handler that needs it must land together, or the deployment is
broken between two merges. That argues strongly for one repository, and against
option C.

The SDKs are coupled to the *contract*, not to the code. They talk HTTP. A
generated client does not need to sit next to the server that generated it.

### Contributors

One maintainer today. The realistic outside contribution to this project is an
SDK fix — someone integrating in PHP hits a bug and sends a patch. That person
should be able to clone a small repository, run `composer install && vendor/bin/phpunit`,
and be done. Making them clone a monorepo with a Go workspace, a Postgres
requirement and a Next.js app to fix a header is a real deterrent.

This is the strongest argument for the split, and it is about people rather than
tooling.

### CI

Monorepo CI is a module matrix with path filters. SDK CI is `install && test` in
a repository of a few hundred lines, in seconds. Merging them means every SDK
pull request either drags the whole matrix or needs path filtering to skip it.

### Maintenance

Option A's real cost: the SDK sources are **duplicated** between `sdks/` in the
monorepo and their published repositories, and duplication drifts. This is the
one genuine mark against the status quo, and it is mitigated rather than solved
— see below.

## Decision

**Option A.** Different release cadences, different coupling, and different
contributor profiles. Option B trades a real contributor-friction cost for a
duplication problem that a sync step handles. Option C is unjustifiable: these
four services are one deployable unit and splitting them would mean coordinating
migrations across repositories, which is exactly how a distributed monolith is
built.

## Handling the duplication

The monorepo's `sdks/` is the source of truth; the public repositories are
publication targets. Until a sync workflow exists, drift is prevented by a rule
rather than by tooling: **an SDK change lands in `sdks/` first, and the mirror is
updated in the same session.**

That is a weak control and is recorded as such. The intended fix, when SDK churn
justifies it, is a release workflow that pushes `sdks/typescript` and `sdks/php`
to their repositories on tag, making the mirrors read-only.

Contract drift is the more dangerous kind — an SDK that no longer matches the
API — and that one is not left to discipline: `contracts/openapi/` is the single
source of truth, and both SDKs are generated from it.

## Layout inside the monorepo

```
ledgercore/
├── services/      one Go module per service; independent build and image
├── libs/go/       shared: money, ident, events, pgxutil, httpx, obs
├── contracts/     OpenAPI 3.1 + event JSON Schemas — the source of truth
├── apps/console/  Next.js console
├── infra/         compose, PostgreSQL init and role model
├── sdks/          SDK sources, mirrored to their own repositories
├── docs/          architecture, invariants, concurrency, ADRs
└── scripts/       ci-local.sh, export-clean.sh
```

A Go module per service with relative `replace` directives and a root `go.work`
keeps the modules independently buildable while allowing a single pull request
to change a contract, a service and the console atomically. See
[ADR-004](../adr/ADR-004-monorepo-with-go-modules.md).

## What would change this decision

- **An outside maintainer for an SDK** — strengthens the split further.
- **Three or more SDKs** — the duplication rule stops scaling; automate the
  mirror before adding a third.
- **The SDKs becoming fully generated at build time** — then they are artefacts,
  not source, and option B becomes attractive.
