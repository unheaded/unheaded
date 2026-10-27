#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# runbook-manifest.sh: runbooks/MANIFEST.sha256, the sha256 of every runbook
# (S-CHAMPION-HARDENING Phase 4, RT-08). pkg/champion's RunbookExecute
# refuses a live run of a runbook whose bytes are not listed here, so a
# runbook edited on disk after review does not run until the edit is
# committed with a fresh manifest.
#
# Usage:
#   scripts/runbook-manifest.sh --write   # regenerate after editing a runbook
#   scripts/runbook-manifest.sh --check   # exit 1 when the manifest is stale (CI)
#
# Format: sha256sum output, paths relative to runbooks/, sorted by path
# (LC_ALL=C), so `cd runbooks && sha256sum -c MANIFEST.sha256` also works.
set -euo pipefail
cd "$(dirname "$0")/../runbooks"

MANIFEST=MANIFEST.sha256

render() {
    find . -type f -name '*.yaml' -printf '%P\n' | LC_ALL=C sort | while IFS= read -r f; do
        sha256sum -- "$f"
    done
}

case "${1:-}" in
    --write)
        render > "$MANIFEST.tmp"
        mv "$MANIFEST.tmp" "$MANIFEST"
        echo "runbook-manifest: wrote runbooks/$MANIFEST ($(wc -l < "$MANIFEST") runbooks)"
        ;;
    --check)
        if [[ ! -f "$MANIFEST" ]]; then
            echo "runbook-manifest: runbooks/$MANIFEST missing; run scripts/runbook-manifest.sh --write" >&2
            exit 1
        fi
        if ! diff -u "$MANIFEST" <(render) >&2; then
            echo "runbook-manifest: runbooks/$MANIFEST is stale (a runbook changed, was added or removed);" >&2
            echo "  review the change, then run scripts/runbook-manifest.sh --write and commit both" >&2
            exit 1
        fi
        echo "runbook-manifest: runbooks/$MANIFEST fresh ($(wc -l < "$MANIFEST") runbooks)"
        ;;
    *)
        echo "usage: $0 --write | --check" >&2
        exit 2
        ;;
esac
