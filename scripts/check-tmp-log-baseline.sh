#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-tmp-log-baseline.sh — the set of /tmp log paths may only SHRINK.
#
# ADR-092 moves every service log to /var/log/unheaded/<service>/. 31 executable
# call sites still write to /tmp, which is world-writable (any local user can
# pre-create or truncate a log a privileged process later opens), cleared on
# reboot (post-mortem evidence destroyed exactly when it is wanted), tmpfs on
# some hosts (logs consume RAM), and covered by no rotation.
#
# Migrating all 31 at once is not the plan — ADR-092 orders it per service,
# after each service's /var/log directory exists. This holds the line in the
# meantime: existing paths may be removed, none may be added.
#
# COMPARES THE PATH SET, NOT A COUNT.
#
# scripts/check-secrets-baseline.sh learned this the hard way and says so:
# comparing totals lets someone delete one entry and add another with the
# total unchanged, so the gate stays green while a new one lands. Membership
# is the property that matters. Removals always pass and lower the ceiling.
#
# Docs are deliberately out of scope. Battle plans and session notes record
# what commands were actually run; rewriting them would be falsifying a
# record, not migrating a call site. Only executable and config surfaces count.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MANIFEST="${REPO_ROOT}/docs/policy/tmp-log-baseline.txt"

cd "$REPO_ROOT" || exit 2

# Executable and config surfaces only — the things that actually run.
current="$(git ls-files '*.sh' '*.py' '*.go' '*.yaml' '*.yml' '*.service' '*.nix' \
    | xargs grep -oh '/tmp/[A-Za-z0-9_*.-]*\.log' 2>/dev/null \
    | sort -u)"

if [[ "${1:-}" == "--update" ]]; then
    mkdir -p "$(dirname "$MANIFEST")"
    {
        echo "# /tmp log paths remaining, per ADR-092. This set may only SHRINK."
        echo "# Regenerate after migrating call sites: scripts/check-tmp-log-baseline.sh --update"
        echo "# Adding a path here is not a fix — it is how the ratchet stops working."
        printf '%s\n' "$current"
    } >"$MANIFEST"
    echo "[UPDATED] $MANIFEST ($(printf '%s\n' "$current" | grep -c . ) paths)"
    exit 0
fi

if [[ ! -f "$MANIFEST" ]]; then
    echo "[FAIL] $MANIFEST is missing; create it with: $0 --update" >&2
    exit 1
fi

baseline="$(grep -v '^#' "$MANIFEST" | grep -v '^[[:space:]]*$' | sort -u)"

# Anything present now that is not in the baseline is a NEW /tmp log path.
added="$(comm -23 <(printf '%s\n' "$current") <(printf '%s\n' "$baseline"))"
removed="$(comm -13 <(printf '%s\n' "$current") <(printf '%s\n' "$baseline"))"

if [[ -n "$added" ]]; then
    echo "[FAIL] new /tmp log path(s) added (ADR-092):" >&2
    printf '%s\n' "$added" | sed 's/^/         /' >&2
    echo >&2
    echo "       /tmp is world-writable, cleared on reboot, and unrotated." >&2
    echo "       Write to /var/log/unheaded/<service>/ instead." >&2
    echo "       Do NOT add the path to $MANIFEST — that defeats the ratchet." >&2
    exit 1
fi

n_current="$(printf '%s\n' "$current" | grep -c .)"
n_baseline="$(printf '%s\n' "$baseline" | grep -c .)"

if [[ -n "$removed" ]]; then
    echo "[PASS] ${n_current} /tmp log paths remain, down from ${n_baseline}. Migrated:"
    printf '%s\n' "$removed" | sed 's/^/         /'
    echo
    echo "       Lower the ceiling: $0 --update"
    exit 0
fi

echo "[PASS] ${n_current} /tmp log paths, unchanged from the baseline (ADR-092 migration pending)"
