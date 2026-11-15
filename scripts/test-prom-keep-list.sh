#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-prom-keep-list.sh — scripts/gen-prom-keep-list.py takes the metric
# names out of PromQL and nothing else, and refuses a rule it cannot list
# (ADR-147, D4 = A). No container; run by scripts/check-prometheus-rules.sh.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

gen=scripts/gen-prom-keep-list.py
data=scripts/testdata/prom-keep
fail=0

got="$(python3 "$gen" --rules "$data/rules" --print 2>&1)"
want="$(printf '%s\n' bar_seconds_count baz foo_total)"
if [[ "$got" != "$want" ]]; then
    echo "FAIL: names from the fixture:" >&2
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2
    fail=1
fi

out="$(python3 "$gen" --rules "$data/byname" --print 2>&1)"
rc=$?
if [[ $rc -ne 1 || "$out" != *"b.yml"* ]]; then
    echo "FAIL: a __name__ regex must exit 1 naming the file (rc=$rc): $out" >&2
    fail=1
fi

if [[ $fail -eq 0 ]]; then
    echo "test-prom-keep-list: OK"
fi
exit "$fail"
