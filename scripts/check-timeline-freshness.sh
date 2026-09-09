#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# scripts/check-timeline-freshness.sh
#
# Enforces ADR-052 Rule 1: references/timeline.md must be touched within
# MAX_AGE_DAYS of HEAD whenever HEAD has commits since timeline's last touch.
#
# Modes:
#   --check    CI mode. Exits 1 on drift, 0 if fresh. Prints citation message.
#   --report   Info mode. Always exits 0. Prints current age + status.
#   (default = --report when no arg given)
#
# Configuration:
#   MAX_AGE_DAYS  : staleness threshold in days (default 7, per ADR-052)
#   TIMELINE_PATH : path to timeline file relative to repo root
#                   (default references/timeline.md)
#
# Exit codes:
#   0  fresh (or --report mode)
#   1  drift detected (--check mode only)
#   2  invocation error (bad args, not in git repo, file missing)
#
# Usage examples:
#   ./scripts/check-timeline-freshness.sh --check
#   ./scripts/check-timeline-freshness.sh --report
#   MAX_AGE_DAYS=14 ./scripts/check-timeline-freshness.sh --check
#   TIMELINE_PATH=references/timeline.md ./scripts/check-timeline-freshness.sh
#
# Marshal-citable per ADR-052. CI-blocking on push to main and pull_request.

set -euo pipefail

# ----- Defaults -----
MAX_AGE_DAYS="${MAX_AGE_DAYS:-7}"
TIMELINE_PATH="${TIMELINE_PATH:-references/timeline.md}"
MODE="${1:---report}"

# ----- Helpers -----
err() { printf 'error: %s\n' "$*" >&2; }
note() { printf '%s\n' "$*" >&2; }

case "$MODE" in
  --check|--report) ;;
  -h|--help)
    sed -n '2,33p' "$0"
    exit 0
    ;;
  *)
    err "unknown mode: $MODE (try --check or --report)"
    exit 2
    ;;
esac

# ----- Find repo root -----
if ! REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  err "not inside a git repo"
  exit 2
fi
cd "$REPO_ROOT"

if [[ ! -f "$TIMELINE_PATH" ]]; then
  err "timeline file not found: $TIMELINE_PATH"
  exit 2
fi

# ----- Get last-modified timestamps from git -----
HEAD_TS="$(git log -1 --format=%ct HEAD 2>/dev/null || echo 0)"
TIMELINE_TS="$(git log -1 --format=%ct -- "$TIMELINE_PATH" 2>/dev/null || echo 0)"

if [[ "$HEAD_TS" == "0" ]]; then
  err "could not get HEAD timestamp"
  exit 2
fi
if [[ "$TIMELINE_TS" == "0" ]]; then
  err "$TIMELINE_PATH has no git history (never committed?)"
  exit 2
fi

# ----- Find latest commit AFTER timeline was last touched (excluding the timeline file itself) -----
# A commit that ONLY touches timeline.md doesn't count as "HEAD has new commits since timeline".
LATEST_NON_TIMELINE_COMMIT_TS="$(
  git log --format=%ct --since="@${TIMELINE_TS}" -- ':!'"$TIMELINE_PATH" 2>/dev/null \
    | head -1 \
    || echo 0
)"

DELTA_SECONDS=$(( HEAD_TS - TIMELINE_TS ))
DELTA_DAYS=$(( DELTA_SECONDS / 86400 ))
THRESHOLD_SECONDS=$(( MAX_AGE_DAYS * 86400 ))

# ----- Render report -----
TIMELINE_DATE="$(date -u -d @"$TIMELINE_TS" '+%Y-%m-%d %H:%M:%S UTC' 2>/dev/null || date -u -r "$TIMELINE_TS" '+%Y-%m-%d %H:%M:%S UTC')"
HEAD_DATE="$(date -u -d @"$HEAD_TS" '+%Y-%m-%d %H:%M:%S UTC' 2>/dev/null || date -u -r "$HEAD_TS" '+%Y-%m-%d %H:%M:%S UTC')"

note "==================== Timeline Freshness Check (ADR-052) ===================="
note "  Timeline path        : $TIMELINE_PATH"
note "  Timeline last touched: $TIMELINE_DATE  (ts=$TIMELINE_TS)"
note "  HEAD last commit     : $HEAD_DATE  (ts=$HEAD_TS)"
note "  Delta                : ${DELTA_DAYS} days (${DELTA_SECONDS}s)"
note "  Max age allowed      : ${MAX_AGE_DAYS} days"

# ----- Decide drift status -----
DRIFT=0
DRIFT_REASON=""

if [[ "$LATEST_NON_TIMELINE_COMMIT_TS" == "0" ]]; then
  note "  Status               : FRESH — no commits since timeline last touched"
elif (( DELTA_SECONDS > THRESHOLD_SECONDS )); then
  DRIFT=1
  DRIFT_REASON="HEAD has commits and delta ${DELTA_DAYS}d > ${MAX_AGE_DAYS}d threshold"
  note "  Status               : DRIFT — $DRIFT_REASON"
else
  note "  Status               : FRESH — within ${MAX_AGE_DAYS}-day threshold"
fi

note "============================================================================"

# ----- Mode-specific exit -----
if [[ "$MODE" == "--report" ]]; then
  exit 0
fi

# --check mode
if (( DRIFT == 1 )); then
  cat >&2 <<EOF

  CITATION (ADR-052 Rule 1):
    references/timeline.md is ${DELTA_DAYS} days behind HEAD.
    Rule: timeline.md MUST be ≤ ${MAX_AGE_DAYS} days from HEAD when
          HEAD has commits since timeline was last touched.
    Fix:  edit references/timeline.md to reflect current project state,
          then commit it in this PR before merging.
    Override: only via explicit commit message containing
              [TIMELINE-DRIFT-OVERRIDE: <rationale>] — Marshal-reviewable.

EOF
  exit 1
fi

exit 0
