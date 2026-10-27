#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ci-status-textfile.sh: the ci-develop SLI (ADR-100 Phase 4, S-WELL-SPRING
# Step 179). Reads the latest GitHub Actions runs on develop from the public
# API (no token: the repository is public, 60 requests an hour is plenty at
# one call every 10 minutes) and writes, for huginn's textfile collector:
#
#   unheaded_ci_run_success{workflow="..."}        1 if that workflow's latest
#                                                  decided run succeeded, else 0
#   unheaded_ci_run_checked_timestamp_seconds      when this script last got an answer
#
# "Decided" skips runs still in progress and runs that were cancelled or
# skipped (a newer push cancels the older run; that is not a red build).
# A workflow with no decided run in the last 30 runs is left out.
#
# When the API cannot be read the file is left as it was, so the checked
# timestamp ages and the SLO counts the data as stale (bad), never as green.
#
# Usage: ci-status-textfile.sh <dir>      writes <dir>/ci_status.prom
#   CI_STATUS_REPO    owner/repo (default unheaded/unheaded)
#   CI_STATUS_BRANCH  branch (default develop)
#   CI_STATUS_API     API base (default https://api.github.com; tests point it elsewhere)
set -euo pipefail

dir="${1:?usage: ci-status-textfile.sh <textfile dir>}"
repo="${CI_STATUS_REPO:-unheaded/unheaded}"
branch="${CI_STATUS_BRANCH:-develop}"
api="${CI_STATUS_API:-https://api.github.com}"
command -v jq >/dev/null || { echo "ci-status: jq is required" >&2; exit 2; }
[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { echo "ci-status: CI_STATUS_REPO $repo is not owner/repo" >&2; exit 2; }
[[ "$branch" =~ ^[A-Za-z0-9_./-]+$ ]] || { echo "ci-status: CI_STATUS_BRANCH $branch is not a branch name" >&2; exit 2; }
mkdir -p "$dir"

body="$(curl -fsS -m 20 -H 'Accept: application/vnd.github+json' \
    "${api}/repos/${repo}/actions/runs?branch=${branch}&per_page=30")" || {
    echo "ci-status: GitHub API unreachable; leaving ${dir}/ci_status.prom as it was" >&2
    exit 1
}

# One line per workflow: its latest decided run, as a Prometheus sample.
# Label values are escaped (backslash, quote, newline) so a workflow name
# cannot break the line.
samples="$(printf '%s' "$body" | jq -r '
  [.workflow_runs[]? | select(.status == "completed" and (.conclusion | IN("cancelled", "skipped") | not))]
  | group_by(.name)[]
  | max_by(.created_at)
  | "unheaded_ci_run_success{workflow=\"\(.name | gsub("\\\\"; "\\\\") | gsub("\""; "\\\"") | gsub("\n"; " "))\"} \(if .conclusion == "success" then 1 else 0 end)"
')" || { echo "ci-status: unexpected API answer; leaving the file as it was" >&2; exit 1; }
[[ -n "$samples" ]] || { echo "ci-status: no decided runs on ${branch}; leaving the file as it was" >&2; exit 1; }

tmp="$(mktemp "${dir}/.ci_status.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
{
    echo "# HELP unheaded_ci_run_success 1 if the workflow's latest decided run on ${branch} succeeded"
    echo "# TYPE unheaded_ci_run_success gauge"
    printf '%s\n' "$samples"
    echo "# HELP unheaded_ci_run_checked_timestamp_seconds When the CI status was last read"
    echo "# TYPE unheaded_ci_run_checked_timestamp_seconds gauge"
    echo "unheaded_ci_run_checked_timestamp_seconds $(date +%s)"
} > "$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "${dir}/ci_status.prom"
trap - EXIT
