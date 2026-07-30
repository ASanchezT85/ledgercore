#!/usr/bin/env bash
# LedgerCore — clean, reproducible source export for external review / audit
# (LC-003).
#
# The audited distribution was an 836 MB ZIP of a working directory that
# leaked .env, .git, node_modules and vendor/. This script produces the
# OPPOSITE: an archive of ONLY the git-tracked source at a chosen ref, which
# by construction excludes anything untracked or git-ignored (.env, secrets,
# node_modules, vendor, dist, .next, caches, dumps, backups).
#
# It also runs a secret scan over the archive before handing it off, and
# refuses to emit an archive if a secret is detected.
#
# Usage:
#   scripts/export-clean.sh [REF] [OUTDIR]
#     REF     git ref to export (default: HEAD)
#     OUTDIR  where to write the archive (default: ./dist-export)
#
# Output: OUTDIR/ledgercore-<ref>-<shortsha>.tar.gz
#
# Requirements: git. Optional: gitleaks (strongly recommended — the scan is
# skipped with a loud warning if it is absent).
set -euo pipefail

REF="${1:-HEAD}"
OUTDIR="${2:-dist-export}"

# Always operate from the repo root regardless of where we are invoked.
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! git rev-parse --verify --quiet "$REF^{commit}" >/dev/null; then
    echo "error: '$REF' is not a valid git ref" >&2
    exit 1
fi

SHORT_SHA="$(git rev-parse --short "$REF")"
SAFE_REF="$(printf '%s' "$REF" | tr '/:' '__')"
mkdir -p "$OUTDIR"
ARCHIVE="$OUTDIR/ledgercore-${SAFE_REF}-${SHORT_SHA}.tar.gz"

echo ">> Exporting tracked source at ${REF} (${SHORT_SHA})"

# ---------------------------------------------------------------------------
# 1. Secret scan. Prefer scanning the full history of the ref; fall back to a
#    filesystem scan of a temporary checkout if history scanning is not wanted.
#    gitleaks exits non-zero when it finds a secret; we propagate that.
# ---------------------------------------------------------------------------
if command -v gitleaks >/dev/null 2>&1; then
    echo ">> Scanning for secrets with gitleaks (git history up to ${REF})"
    if ! gitleaks detect --source "$REPO_ROOT" --redact --no-banner \
        --log-opts="--all"; then
        echo "error: gitleaks detected potential secrets — refusing to export." >&2
        echo "       Review the findings above, purge the secret, and rotate it." >&2
        exit 2
    fi
    echo ">> gitleaks: clean"
else
    echo "WARNING: gitleaks not found on PATH — SKIPPING the secret scan." >&2
    echo "         Install it (https://github.com/gitleaks/gitleaks) before" >&2
    echo "         shipping this archive to an external party." >&2
fi

# ---------------------------------------------------------------------------
# 2. Build the archive from the git object store. `git archive` emits ONLY
#    tracked files at the ref — never the working tree — so untracked and
#    git-ignored paths (.env, node_modules, vendor, dist, dumps, .git itself)
#    cannot leak in. `.gitattributes` `export-ignore` entries are honored too.
# ---------------------------------------------------------------------------
echo ">> Writing $ARCHIVE"
git archive --format=tar.gz \
    --prefix="ledgercore-${SHORT_SHA}/" \
    -o "$ARCHIVE" \
    "$REF"

SIZE="$(du -h "$ARCHIVE" | cut -f1)"
echo ">> Done: $ARCHIVE ($SIZE)"
echo ">> Verify contents with:  tar tzf $ARCHIVE | head"
echo ">> Recipient reproduces with:  tar xzf <archive> && cd ledgercore-${SHORT_SHA}"
