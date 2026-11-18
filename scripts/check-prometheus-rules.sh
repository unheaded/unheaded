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
# A module's rules (modules/<name>/prometheus/rules/*.yml, ADR-149) are
# checked here the same way, each directory with its own tests/; Prometheus
# loads them only while the module's switch is on (/run/unheaded/modules).
# A module rules directory with no tests/ fails.
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
# 3.15.1 resolved to this digest on 2026-11-04.
CURRENT_IMAGE="${CURRENT_PROM_IMAGE:-prom/prometheus:v3.15.1@sha256:fa3302279710be2f21a8301ec17a8b4f2301fd829bd113839b3b6b02b6f191a2}"

# ADR-147 decisions 9, 10 (D4 = A): Prometheus keeps only the series its
# rules read. A rule that reads a series rule-series.txt lacks, or a stale
# list in prometheus.yml, fails here, before promtool.
if ! python3 "${REPO_ROOT}/scripts/gen-prom-keep-list.py" --check; then
    exit 1
fi
if ! "${REPO_ROOT}/scripts/test-prom-keep-list.sh"; then
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    echo "[FAIL] docker is required to run promtool; this gate does not pass by skipping." >&2
    exit 2
fi

DEPLOYED_IMAGE="$(awk '/image: *prom\/prometheus/ {print $2; exit}' "$COMPOSE")"
if [[ -z "$DEPLOYED_IMAGE" ]]; then
    echo "[FAIL] no prom/prometheus image found in ${COMPOSE#"$REPO_ROOT"/}" >&2
    exit 1
fi

# check_rules DIR LABEL: coverage, then promtool check and test in both
# images, for one rules directory. Sets rc=1 on any failure.
rc=0
check_rules() {
    local dir="$1" label="$2"
    local -a rule_files tests
    mapfile -t rule_files < <(cd "$dir" && find . -maxdepth 1 -name '*.yml' -printf '%f\n' | sort)
    if [[ -d "$dir/tests" ]]; then
        mapfile -t tests < <(cd "$dir" && find tests -maxdepth 1 -name '*_test.yml' -printf '%f\n' | sort)
    else
        tests=()
    fi
    if [[ ${#rule_files[@]} -eq 0 || ${#tests[@]} -eq 0 ]]; then
        echo "[FAIL] ${label}: no rule files or no rule tests under ${dir#"$REPO_ROOT"/}" >&2
        rc=1
        return
    fi

    # A rule file nobody tests is a rule file whose alerts nobody has seen fire.
    local f u covered baselined
    for f in "${rule_files[@]}"; do
        covered=false
        grep -qx "  - \.\./${f}" "$dir"/tests/*_test.yml && covered=true
        baselined=false
        if [[ "$label" == platform ]]; then
            for u in "${UNTESTED[@]}"; do [[ "$u" == "$f" ]] && baselined=true; done
        fi
        if [[ "$covered" == false && "$baselined" == false ]]; then
            echo "[FAIL] ${label}: ${f} is loaded by Prometheus but no tests/*_test.yml covers it" >&2
            rc=1
        elif [[ "$covered" == true && "$baselined" == true ]]; then
            echo "[FAIL] ${f} is now tested: remove it from UNTESTED in $0" >&2
            rc=1
        fi
    done

    local image out
    for image in "$DEPLOYED_IMAGE" "$CURRENT_IMAGE"; do
        echo "== ${label}: ${image%%@*}"
        if ! out="$(promtool "$image" "$dir" "" check rules "${rule_files[@]}" 2>&1)"; then
            printf '%s\n' "$out" >&2
            echo "[FAIL] ${label}: rules do not parse in ${image%%@*}" >&2
            rc=1
            continue
        fi
        if ! out="$(promtool "$image" "$dir" /tests test rules "${tests[@]}" 2>&1)"; then
            printf '%s\n' "$out" >&2
            echo "[FAIL] ${label}: rule tests fail in ${image%%@*}" >&2
            rc=1
            continue
        fi
        if [[ "$label" == platform ]]; then
            echo "[PASS] ${#rule_files[@]} rule files parse, ${#tests[@]} test files pass"
        else
            echo "[PASS] ${label}: ${#rule_files[@]} rule files parse, ${#tests[@]} test files pass"
        fi
    done
}

# promtool IMAGE DIR WORKDIR ARGS...: promtool in the image, DIR at /rules.
promtool() {
    docker run --rm --network none --user "$(id -u):$(id -g)" \
        -v "$2:/rules:ro" -w "/rules${3:-}" --entrypoint promtool "$1" "${@:4}"
}

# A rule file nobody tests is a rule file whose alerts nobody has seen fire.
#
# UNTESTED may only SHRINK, and is empty. Its last two entries, monad.yml and
# loadbalancers.yml, alerted on series nothing emitted or scraped; they were
# deleted (S-WELL-SPRING Step 163, decision 25: a rule returns with its
# producer and a test), NodeExporterDown moving to host.yml.
UNTESTED=()
for u in "${UNTESTED[@]}"; do
    [[ -f "$RULES/$u" ]] || { echo "[FAIL] UNTESTED names ${u}, which no longer exists: remove it" >&2; rc=1; }
done

# ADR-100 section 4: every alert names a runbook that exists in this repo
# (annotations.runbook_url: a repo path, or a GitHub blob/wiki URL to one).
# Strict, no baseline; cmd/alert-policy's tests prove it refuses a missing
# annotation, a missing file and a dead URL. Appendix A6 of
# docs/battle-plans/S-WELL-SPRING-battle-plan.md says what to do when it fails.
if ! (cd "$REPO_ROOT" && go run ./cmd/alert-policy -runbooks); then
    rc=1
fi

check_rules "$RULES" platform
while IFS= read -r mdir; do
    [[ -n "$mdir" ]] || continue
    mname="${mdir#"$REPO_ROOT"/modules/}"
    mname="${mname%%/*}"
    check_rules "$mdir" "module ${mname}"
done < <(find "$REPO_ROOT/modules" -mindepth 3 -maxdepth 3 -type d -path '*/prometheus/rules' 2>/dev/null | sort)
exit "$rc"
