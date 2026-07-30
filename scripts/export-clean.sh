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
# Requirements: git AND gitleaks. gitleaks is MANDATORY (R-001): the script
# aborts if it is not on PATH rather than shipping an unscanned archive. Set
# EXPORT_ALLOW_NO_GITLEAKS=1 only for an explicit, deliberate local dry-run.
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
HAVE_GITLEAKS=0
if command -v gitleaks >/dev/null 2>&1; then
    HAVE_GITLEAKS=1
    echo ">> Scanning git history for secrets with gitleaks (up to ${REF})"
    if ! gitleaks detect --source "$REPO_ROOT" --redact --no-banner \
        --log-opts="--all"; then
        echo "error: gitleaks detected potential secrets in history — refusing to export." >&2
        echo "       Review the findings above, purge the secret, and rotate it." >&2
        exit 2
    fi
    echo ">> gitleaks (history): clean"
elif [ "${EXPORT_ALLOW_NO_GITLEAKS:-0}" = "1" ]; then
    echo "WARNING: gitleaks not found but EXPORT_ALLOW_NO_GITLEAKS=1 — proceeding" >&2
    echo "         WITHOUT a secret scan. Do NOT ship this archive externally." >&2
else
    echo "error: gitleaks is not on PATH — refusing to build an unscanned export." >&2
    echo "       Install it (https://github.com/gitleaks/gitleaks) and re-run." >&2
    echo "       For a local dry-run only: EXPORT_ALLOW_NO_GITLEAKS=1 $0 ..." >&2
    exit 3
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

# ---------------------------------------------------------------------------
# 3. Belt-and-suspenders: scan the ACTUAL bytes we are about to ship. `git
#    archive` cannot include untracked files, but this proves it on the exact
#    artifact (catches a tracked secret, an export-ignore mistake, or a future
#    change to how the archive is built). Extract to a temp dir and run
#    gitleaks in no-git (filesystem) mode; any finding aborts and deletes the
#    archive so a leaky package can never be handed off.
# ---------------------------------------------------------------------------
if [ "$HAVE_GITLEAKS" = "1" ]; then
    echo ">> Scanning the built archive with gitleaks (filesystem mode)"
    SCAN_DIR="$(mktemp -d)"
    trap 'rm -rf "$SCAN_DIR"' EXIT
    tar xzf "$ARCHIVE" -C "$SCAN_DIR"
    if ! gitleaks detect --source "$SCAN_DIR" --no-git --redact --no-banner; then
        echo "error: gitleaks detected secrets INSIDE the built archive — deleting it." >&2
        rm -f "$ARCHIVE"
        exit 2
    fi
    echo ">> gitleaks (archive): clean"
fi

SIZE="$(du -h "$ARCHIVE" | cut -f1)"
echo ">> Done: $ARCHIVE ($SIZE)"
echo ">> Verify contents with:  tar tzf $ARCHIVE | head"
echo ">> Recipient reproduces with:  tar xzf <archive> && cd ledgercore-${SHORT_SHA}"
