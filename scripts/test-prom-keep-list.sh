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

got="$(python3 "$gen" --rules "$data/rules" --runbooks "$data/none" --print 2>&1)"
want="$(printf '%s\n' bar_seconds_count baz foo_total)"
if [[ "$got" != "$want" ]]; then
    echo "FAIL: names from the fixture:" >&2
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2
    fail=1
fi

# REV-6: a runbook that queries Prometheus' API (127.0.0.1:9090) is a
# reader: its series are kept. VictoriaMetrics' (8428) are not.
got="$(python3 "$gen" --rules "$data/rules" --runbooks "$data/runbooks" --print 2>&1)"
want="$(printf '%s\n' bar_seconds_count baz foo_total other_gauge runbook_only_total)"
if [[ "$got" != "$want" ]]; then
    echo "FAIL: names from the rules and the runbooks:" >&2
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2
    fail=1
fi

out="$(python3 "$gen" --rules "$data/byname" --print 2>&1)"
rc=$?
if [[ $rc -ne 1 || "$out" != *"b.yml"* ]]; then
    echo "FAIL: a __name__ regex must exit 1 naming the file (rc=$rc): $out" >&2
    fail=1
fi

# Phase 17 (S-SEE-AND-SURVIVE): a series every rule pins to jobs is kept in
# those jobs only, in a keep of their own; a bare use anywhere keeps it in all.
got="$(python3 - "$gen" <<'PY' 2>&1
import importlib.util, sys
spec = importlib.util.spec_from_file_location("g", sys.argv[1])
g = importlib.util.module_from_spec(spec); spec.loader.exec_module(g)
e = 'sum(rate(a_total{job="w",x="1"}[5m])) / sum(b_count{job=~"w|d"}) + c + c{job="w"} + d{status=~"5.."} + label_replace(up, "x", "a_total", "", "")'
s = g.scoped_in(e, g.names_in(e, "t"))
print(sorted(s["a_total"]), sorted(s["b_count"]), s["c"], s["d"])
text = "  - job_name: one\n    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)\n    # END rule-series\n  - job_name: w\n    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)\n    # END rule-series\n  - job_name: two\n    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)\n    # END rule-series"
print(g.render_prometheus(text, {"c"}, {"a_total": {"w"}}))
PY
)"
want="['w'] ['d', 'w'] None None
  - job_name: one
    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)
    metric_relabel_configs: &rule_series
      - source_labels: [__name__]
        regex: 'c'
        action: keep
    # END rule-series
  - job_name: w
    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)
    metric_relabel_configs:
      - source_labels: [__name__]
        regex: 'a_total|c'
        action: keep
    # END rule-series
  - job_name: two
    # BEGIN rule-series (scripts/gen-prom-keep-list.py; do not edit)
    metric_relabel_configs: *rule_series
    # END rule-series"
if [[ "$got" != "$want" ]]; then
    echo "FAIL: series pinned to jobs:" >&2
    diff <(printf '%s\n' "$want") <(printf '%s\n' "$got") >&2
    fail=1
fi

if [[ $fail -eq 0 ]]; then
    echo "test-prom-keep-list: OK"
fi
exit "$fail"
