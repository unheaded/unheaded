#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# drift-detect.sh — compare each deployed OpenTofu layer with its live state.
#
# A layer counts as deployed when its state file exists (the backend is
# local: deploy/tofu/.state/<env>/<layer>/terraform.tfstate). For each one,
# `terragrunt plan -detailed-exitcode`: 0 clean, 2 drift, anything else an
# error. Layers with no state are reported as not deployed, never as clean.
#
# Exit: 0 no drift (or nothing deployed), 2 drift, 1 a plan failed.
# With GITHUB_OUTPUT set, writes result=clean|drift|error|none and a job
# summary, so the workflow opens an issue only for real drift.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE_ROOT="${STATE_ROOT:-$REPO_ROOT/deploy/tofu/.state}"
TERRAGRUNT="${TERRAGRUNT:-terragrunt}"

echo "=== Unheaded Drift Detection ($(date -u +%Y-%m-%dT%H:%M:%SZ)) ==="

drift=() errors=() clean=() undeployed=()
for env in dev staging prod; do
    for layer in networking compute identity observability ebpf; do
        dir="$REPO_ROOT/deploy/tofu/environments/$env/$layer"
        [[ -d "$dir" ]] || continue
        if [[ ! -f "$STATE_ROOT/$env/$layer/terraform.tfstate" ]]; then
            undeployed+=("$env/$layer")
            continue
        fi
        echo "--- $env/$layer"
        (cd "$dir" && "$TERRAGRUNT" plan -detailed-exitcode -input=false -lock=false)
        case $? in
            0) clean+=("$env/$layer") ;;
            2) drift+=("$env/$layer"); echo "  DRIFT in $env/$layer" ;;
            *) errors+=("$env/$layer"); echo "  ERROR: plan failed for $env/$layer" >&2 ;;
        esac
    done
done

if ((${#errors[@]})); then
    result=error
elif ((${#drift[@]})); then
    result=drift
elif ((${#clean[@]})); then
    result=clean
else
    result=none
fi

summary() {
    echo "### Drift detection: $result"
    echo
    echo "- drift: ${drift[*]:-none}"
    echo "- plan errors: ${errors[*]:-none}"
    echo "- clean: ${clean[*]:-none}"
    echo "- not deployed (no state): ${undeployed[*]:-none}"
    if [[ "$result" == none ]]; then
        echo
        echo "No layer has state here, so there is nothing to drift. The backend is local"
        echo "(deploy/tofu/.state); drift is checked where the state lives."
    fi
}
summary
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "result=$result" >>"$GITHUB_OUTPUT"
    echo "layers=${drift[*]:-}" >>"$GITHUB_OUTPUT"
fi
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    summary >>"$GITHUB_STEP_SUMMARY"
fi

case $result in
    error) exit 1 ;;
    drift) exit 2 ;;
esac
exit 0
