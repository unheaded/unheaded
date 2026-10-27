#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# test-check-perf-regression.sh: check-perf-regression.sh against fixture
# perf/v1 reports. Faster passes, p99 more than 20 % slower fails, a higher
# error ratio fails, skipped or missing scenarios do not fail, --update
# writes a baseline from measured results only.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CHECK="${SCRIPT_DIR}/check-perf-regression.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

PASS=0
FAIL=0

expect_rc() {
    local name="$1" want="$2"
    shift 2
    local out rc
    out="$("$@" 2>&1)"
    rc=$?
    if [ "$rc" = "$want" ]; then
        printf '  PASS  %s (exit %s)\n' "$name" "$rc"
        PASS=$((PASS + 1))
    else
        printf '  FAIL  %s: exit %s, want %s\n%s\n' "$name" "$rc" "$want" "$out"
        FAIL=$((FAIL + 1))
    fi
}

expect_eq() {
    local name="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then
        printf '  PASS  %s\n' "$name"
        PASS=$((PASS + 1))
    else
        printf '  FAIL  %s: got %s, want %s\n' "$name" "$got" "$want"
        FAIL=$((FAIL + 1))
    fi
}

# report NAME P99 ERROR_RATIO [SKIPPED] -> one result object
result() {
    local skipped="${4:-}"
    jq -nc --arg n "$1" --argjson p99 "$2" --argjson er "$3" --arg sk "$skipped" \
        '{name:$n, n:(if $sk=="" then 100 else 0 end), attempts:100, errors:0, p50:1, p95:1, p99:$p99, error_ratio:$er} + (if $sk=="" then {} else {skipped:$sk} end)'
}

report() { # FILE RESULT...
    local f="$1"
    shift
    printf '%s\n' "$@" | jq -s '{schema:"perf/v1", host:"west", commit:"abc", at:"2026-10-27T00:00:00Z", duration_unit:"ns", results:.}' > "$f"
}

B="$T/baseline.json"

echo "--- --update ---"
report "$T/base1.json" "$(result e2e-trace 10000000 0)" "$(result http3 0 0 'curl lacks HTTP3')"
report "$T/base2.json" "$(result http-load 2000000 0.001)"
expect_rc "update from two reports" 0 "$CHECK" --update --baseline "$B" "$T/base1.json" "$T/base2.json"
expect_eq "baseline schema" "$(jq -r .schema "$B")" "perf-baseline/v1"
expect_eq "baseline keeps measured scenarios" "$(jq -r '.scenarios|keys|join(",")' "$B")" "e2e-trace,http-load"
expect_eq "baseline e2e p99" "$(jq -r '.scenarios["e2e-trace"].p99' "$B")" "10000000"
expect_eq "baseline records host and commit" "$(jq -r '.scenarios["http-load"]|.host+"@"+.commit' "$B")" "west@abc"

echo "--- compare ---"
report "$T/faster.json" "$(result e2e-trace 5000000 0)" "$(result http-load 1000000 0)"
expect_rc "faster passes" 0 "$CHECK" --baseline "$B" "$T/faster.json"
report "$T/edge.json" "$(result e2e-trace 12000000 0)"
expect_rc "exactly +20 % p99 passes" 0 "$CHECK" --baseline "$B" "$T/edge.json"
report "$T/slow.json" "$(result e2e-trace 12000001 0)"
expect_rc "p99 over +20 % fails" 1 "$CHECK" --baseline "$B" "$T/slow.json"
report "$T/errs.json" "$(result http-load 2000000 0.002)"
expect_rc "higher error ratio fails" 1 "$CHECK" --baseline "$B" "$T/errs.json"
report "$T/skipped.json" "$(result e2e-trace 0 0 'stack down')" "$(result container-start 1 0)"
expect_rc "skipped and unbaselined scenarios do not fail" 0 "$CHECK" --baseline "$B" "$T/skipped.json"
expect_rc "a later passing report does not hide an earlier regression" 1 "$CHECK" --baseline "$B" "$T/slow.json" "$T/faster.json"

echo "--- refusals ---"
expect_rc "missing baseline" 2 "$CHECK" --baseline "$T/none.json" "$T/faster.json"
echo '{"schema":"perf/v0","results":[]}' > "$T/old.json"
expect_rc "wrong report schema" 2 "$CHECK" --baseline "$B" "$T/old.json"
expect_rc "no report" 2 "$CHECK" --baseline "$B"
report "$T/allskipped.json" "$(result http3 0 0 'curl lacks HTTP3')"
expect_rc "update with nothing measured" 2 "$CHECK" --update --baseline "$T/b2.json" "$T/allskipped.json"
expect_eq "nothing written when nothing measured" "$(test -e "$T/b2.json" && echo yes || echo no)" "no"

echo ""
echo "check-perf-regression: ${PASS} passed, ${FAIL} failed"
[ "$FAIL" -eq 0 ]
