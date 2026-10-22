#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-prometheus-rules.sh — every alert rule parses, and every rule test
# passes, in the Prometheus this repository deploys and in a current one.
#
# monitoring/prometheus/rules/*.yml are loaded by rule_files: 'rules/*.yml'.
# A rule that does not parse makes Prometheus refuse the whole file, so every
# alert in it goes silent; a threshold or guard edited wrong fires on nothing
# (or on everything). Both look like a quiet system. tests/*_test.yml pin each
# alert's firing AND quiet cases; this runs them.
#
# Images: the one monitoring/docker-compose.yml runs (read from it, so this
# tests what is deployed), and 3.10.0 pinned by digest (the two majors differ
# in template functions and rule syntax).
#
# Needs docker. Without it this FAILS (exit 2); it never passes by skipping.
#
# Usage: scripts/check-prometheus-rules.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RULES="${REPO_ROOT}/monitoring/prometheus/rules"
COMPOSE="${REPO_ROOT}/monitoring/docker-compose.yml"
# 3.10.0 resolved to this digest on 2026-10-13.
CURRENT_IMAGE="${CURRENT_PROM_IMAGE:-prom/prometheus:v3.15.0@sha256:efd719c99d83b060d9daefdcf00360461adf279f45ef5391f8d111892118753e}"

if ! docker info >/dev/null 2>&1; then
    echo "[FAIL] docker is required to run promtool; this gate does not pass by skipping." >&2
    exit 2
fi

DEPLOYED_IMAGE="$(awk '/image: *prom\/prometheus/ {print $2; exit}' "$COMPOSE")"
if [[ -z "$DEPLOYED_IMAGE" ]]; then
    echo "[FAIL] no prom/prometheus image found in ${COMPOSE#"$REPO_ROOT"/}" >&2
    exit 1
fi

cd "$RULES" || exit 2
mapfile -t rule_files < <(find . -maxdepth 1 -name '*.yml' -printf '%f\n' | sort)
mapfile -t tests < <(find tests -maxdepth 1 -name '*_test.yml' -printf '%f\n' | sort)
if [[ ${#rule_files[@]} -eq 0 || ${#tests[@]} -eq 0 ]]; then
    echo "[FAIL] no rule files or no rule tests under ${RULES#"$REPO_ROOT"/}" >&2
    exit 1
fi

# A rule file nobody tests is a rule file whose alerts nobody has seen fire.
#
# UNTESTED may only SHRINK. Both entries alert on series nothing scrapes
# (2026-10-13): monad.yml's unheaded_monad_hbh_drop_total, unheaded_bgp_session_up,
# unheaded_bfd_failover_total and unheaded_routing_health_up are emitted by no code in this repo, and
# loadbalancers.yml's haproxy_*/nginx_* have no scrape job in prometheus.yml.
# Testing them would pin thresholds on metrics that do not exist; deciding
# which metrics they should watch comes first.
UNTESTED=(loadbalancers.yml monad.yml)
rc=0
for f in "${rule_files[@]}"; do
    covered=false
    grep -qx "  - \.\./${f}" tests/*_test.yml && covered=true
    baselined=false
    for u in "${UNTESTED[@]}"; do [[ "$u" == "$f" ]] && baselined=true; done
    if [[ "$covered" == false && "$baselined" == false ]]; then
        echo "[FAIL] ${f} is loaded by Prometheus but no tests/*_test.yml covers it" >&2
        rc=1
    elif [[ "$covered" == true && "$baselined" == true ]]; then
        echo "[FAIL] ${f} is now tested: remove it from UNTESTED in $0" >&2
        rc=1
    fi
done
for u in "${UNTESTED[@]}"; do
    [[ -f "$u" ]] || { echo "[FAIL] UNTESTED names ${u}, which no longer exists: remove it" >&2; rc=1; }
done

promtool() {
    docker run --rm --network none --user "$(id -u):$(id -g)" \
        -v "$RULES:/rules:ro" -w "/rules${2:-}" --entrypoint promtool "$1" "${@:3}"
}

for image in "$DEPLOYED_IMAGE" "$CURRENT_IMAGE"; do
    echo "== ${image%%@*}"
    if ! out="$(promtool "$image" "" check rules "${rule_files[@]}" 2>&1)"; then
        printf '%s\n' "$out" >&2
        echo "[FAIL] rules do not parse in ${image%%@*}" >&2
        rc=1
        continue
    fi
    if ! out="$(promtool "$image" /tests test rules "${tests[@]}" 2>&1)"; then
        printf '%s\n' "$out" >&2
        echo "[FAIL] rule tests fail in ${image%%@*}" >&2
        rc=1
        continue
    fi
    echo "[PASS] ${#rule_files[@]} rule files parse, ${#tests[@]} test files pass"
done
exit "$rc"
