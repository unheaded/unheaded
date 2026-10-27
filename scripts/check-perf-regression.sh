#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-perf-regression.sh: compare perf/v1 reports from
# `go run ./cmd/perf-benchmark -scenario ...` with the committed baseline
# (S-WELL-SPRING Phase 10, M-089).
#
#   check-perf-regression.sh [--baseline FILE] REPORT...           compare
#   check-perf-regression.sh --update [--baseline FILE] REPORT...  write baseline
#
# Per scenario in the baseline: p99 may grow at most 20 %, error_ratio may
# not grow at all. Faster is allowed, so this is not a shrink-only gate:
# a faster run passes and --update records it when someone chooses to. A
# scenario that was skipped or not run is reported, not failed (the stack
# decides what can be measured); a scenario with no baseline is new.
# --update keeps baseline entries the reports did not measure and writes
# nothing when no report measured anything.
#
# Exit: 0 pass, 1 regression, 2 usage / missing baseline / wrong schema.
# Needs the stack, so it runs from scripts/ci-local.sh (CI_LOCAL_PERF=1)
# on west, not on GitHub.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/docs/policy/perf-baseline.json"
UPDATE=0
P99_GROWTH="1.20"

usage() {
    echo "usage: $0 [--update] [--baseline FILE] REPORT..." >&2
    exit 2
}

REPORTS=()
while [ $# -gt 0 ]; do
    case "$1" in
        --update) UPDATE=1 ;;
        --baseline) [ $# -ge 2 ] || usage; BASELINE="$2"; shift ;;
        -h|--help) usage ;;
        -*) echo "unknown flag $1" >&2; usage ;;
        *) REPORTS+=("$1") ;;
    esac
    shift
done
[ ${#REPORTS[@]} -gt 0 ] || usage
command -v jq >/dev/null || { echo "jq is required" >&2; exit 2; }

for r in "${REPORTS[@]}"; do
    if [ ! -r "$r" ]; then
        echo "cannot read report $r" >&2
        exit 2
    fi
    schema="$(jq -r '.schema // empty' "$r" 2>/dev/null || true)"
    if [ "$schema" != "perf/v1" ]; then
        echo "$r: schema '${schema}', want perf/v1" >&2
        exit 2
    fi
done

# measured: every result that was run (not skipped) and has samples, tagged
# with its report's host/commit/at; a later report wins for one scenario.
measured="$(jq -s '[ .[] as $rep | $rep.results[]
    | select((.skipped // "") == "" and (.n // 0) > 0)
    | {key: .name, value: {p99, error_ratio, n, host: $rep.host, commit: $rep.commit, at: $rep.at}} ]
    | from_entries' "${REPORTS[@]}")"

if [ "$UPDATE" = 1 ]; then
    if [ "$(jq 'length' <<<"$measured")" = 0 ]; then
        echo "no report measured anything (all skipped or empty); baseline not written" >&2
        exit 2
    fi
    old='{}'
    [ -r "$BASELINE" ] && old="$(jq '.scenarios // {}' "$BASELINE")"
    tmp="$(mktemp "${BASELINE}.XXXXXX")"
    jq -n --argjson old "$old" --argjson new "$measured" --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
        '{schema: "perf-baseline/v1",
          rule: "p99 may grow at most 20 % per scenario; error_ratio may not grow (scripts/check-perf-regression.sh)",
          updated_at: $now,
          scenarios: ($old + $new)}' > "$tmp"
    mv "$tmp" "$BASELINE"
    echo "baseline written: $BASELINE"
    jq -r '.scenarios | to_entries[] | "  \(.key): p99=\(.value.p99) ns error_ratio=\(.value.error_ratio) n=\(.value.n) (\(.value.host) \(.value.commit[0:12]) \(.value.at))"' "$BASELINE"
    exit 0
fi

if [ ! -r "$BASELINE" ]; then
    echo "no baseline at $BASELINE; record one with: $0 --update REPORT..." >&2
    exit 2
fi
if [ "$(jq -r '.schema // empty' "$BASELINE")" != "perf-baseline/v1" ]; then
    echo "$BASELINE: not a perf-baseline/v1 file" >&2
    exit 2
fi

# One line per (report, result) and per baseline scenario nothing measured.
lines="$(jq -rs --slurpfile b "$BASELINE" --argjson g "$P99_GROWTH" '
    ($b[0].scenarios) as $base
    | [ .[] | .results[] ] as $all
    | ( $all[]
        | . as $r
        | $base[$r.name] as $bl
        | if (($r.skipped // "") != "") then "SKIP \($r.name): \($r.skipped)"
          elif $bl == null then "NEW  \($r.name): p99=\($r.p99) ns error_ratio=\($r.error_ratio) (no baseline)"
          elif ($r.error_ratio > $bl.error_ratio) then "FAIL \($r.name): error_ratio \($r.error_ratio) > baseline \($bl.error_ratio)"
          elif (($r.n // 0) > 0 and $r.p99 > ($bl.p99 * $g)) then "FAIL \($r.name): p99 \($r.p99) ns > baseline \($bl.p99) ns x \($g)"
          else "OK   \($r.name): p99 \($r.p99) ns (baseline \($bl.p99)), error_ratio \($r.error_ratio) (baseline \($bl.error_ratio))"
          end ),
      ( $base | keys[] | select(. as $k | [$all[] | select(.name == $k and (.skipped // "") == "")] | length == 0)
        | "MISS \(.): in the baseline, not measured by these reports" )
' "${REPORTS[@]}")"

echo "$lines"
if grep -q '^FAIL' <<<"$lines"; then
    echo "perf regression against $BASELINE" >&2
    exit 1
fi
echo "no perf regression against $BASELINE"
