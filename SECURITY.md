# Security Policy

## Before anything else

LedgerCore is an early-stage, pre-1.0 project. It has **not** been through an
independent security review, and it is not certified or compliant with any
standard. Treat it accordingly: read the code before you run it near money.

## Supported versions

Only the `main` branch and the most recent tagged release receive fixes. There
are no long-term support branches and no backports to earlier tags.

| Version | Supported |
|---|---|
| `main` | yes |
| latest tag | yes |
| anything older | no |

## Reporting a vulnerability

**Do not open a public issue for a security problem.**

Use GitHub's private reporting on this repository:
**Security → Report a vulnerability**
(<https://github.com/ASanchezT85/ledgercore/security/advisories/new>).

That channel is private to the maintainer and lets us discuss and fix the issue
before anything becomes public. If private reporting is unavailable to you, open
a public issue containing only the words "security report, please contact me"
and no technical detail, and you will be contacted.

Please include, as far as you can:

- what the flaw is, and which component it affects;
- how to reproduce it — a `curl` sequence or a failing test is ideal;
- what an attacker gets out of it;
- the commit or tag you tested.

### What to expect

This is a single-maintainer project, so response times are best-effort, not
contractual. The intent is an acknowledgement within a week, an assessment
within two, and a fix or a documented decision not to fix after that. If you
have heard nothing in two weeks, ping the report thread.

Please give a fix a reasonable window before disclosing publicly. There is no
bug bounty.

## What counts as a vulnerability

In scope — anything that breaks a guarantee the README makes:

- posting or persisting an unbalanced transaction;
- mutating or deleting posted accounting history;
- reading or writing another tenant's data (an RLS bypass);
- an idempotency key producing a duplicated accounting effect;
- forging or replaying a webhook signature;
- authentication or authorisation bypass, JWT forgery, scope escalation;
- extracting a signing key or a webhook secret from the database without the
  master key;
- server-side request forgery through webhook delivery;
- SQL injection, or privilege escalation through a database role.

Out of scope:

- anything that requires the attacker to already be a database superuser or to
  hold the master key;
- the documented development defaults — `LEDGERCORE_AUTH_DISABLED=true`, the
  `*_dev` role passwords, the sample master key in `docker-compose.yml`. These
  are deliberately public and the hardened overlay refuses to start with them.
  Deploying them to a public host is a misconfiguration, not a vulnerability;
- missing hardening that the README already declares missing under
  [Limitations](README.md#limitations) — no KMS, no rate limiting, no HA;
- denial of service by brute resource exhaustion against a self-hosted instance;
- findings from an automated scanner with no demonstrated impact.

## Handling secrets in this repository

Every credential-shaped string committed here is either an example or a test
fixture. The repository history has been scanned end to end with `gitleaks`
using the default ruleset and **without** the project allowlist; that scan
reports no findings. See
[`docs/oss-transition/SECRET_AUDIT.md`](docs/oss-transition/SECRET_AUDIT.md).

If you believe you have found a real credential in the code or in the history,
report it privately through the channel above rather than opening an issue.
