#!/bin/bash
# SPDX-License-Identifier: MIT
# check-ruff.sh — gate the Python surface, ratcheted by rule ID.
#
# Baseline history:
#   427  2026-08-03, start of the Staging Ladder
#   323  after repairing notebooks/02_hypothesis_matrix.ipynb — three code
#        cells that had never parsed, generating 104 cascading findings from
#        one unterminated string literal
#   137  after Phases 3-13
#     0  with the two exclusions below
#
# RATCHET POLICY (ADR-073, docs/security/findings-remediation-2026-07-29.md):
# exclude by RULE ID with a stated reason, never by severity, and never by
# raising a threshold count. Every exclusion below has to name what would remove
# it. If a rule is excluded and later fixed, delete the exclusion — the baseline
# may only ever shrink.
#
# Two exclusions:
#
#   --exclude crates/xv6-mbc/upstream
#       Vendored xv6-riscv (28d5ac98). Not ours to lint. It carries 3 findings
#       including a genuine F821 NameError in an upstream error path, which is
#       upstream's bug to fix. ADR-090 excludes vendored trees from the source
#       sweep for the same reason. Removed if the vendored tree ever goes away —
#       Phase 2.2-2.4 has been progressively replacing it with our own code.
#
#   --ignore BLE001
#       134 blind `except Exception` handlers. NOT a suppression-and-forget:
#       they are measured, split and costed in
#       docs/security/exception-handling-triage-2026-08-03.md — 71 of the 130
#       non-notebook sites are silent (catch everything, neither log nor
#       re-raise), 59 already log, and 62 of the 134 are in raft/zhen_app.py
#       alone. Narrowing a handler is the one change that can turn a working
#       path into a crash, so it is attended work, planned as the first real
#       exercise of the ADR-090 sweep. Removed when that lands.
#
# Deliberately NOT done: `--select` narrowing. Restricting the rule set changes
# which findings count AND makes unrelated `noqa` directives report as unused
# (RUF100 "non-enabled"), so the number stops meaning what it says. This script
# runs ruff with its default rule set, exactly as the workflow does.
#
# Usage:
#   ./scripts/check-ruff.sh        # gate: exit 1 on any finding
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}" || exit 1

RUFF="${RUFF:-ruff}"
if ! command -v "${RUFF}" >/dev/null 2>&1; then
    echo "check-ruff: ${RUFF} not found on PATH." >&2
    echo "  CI pins ruff==0.16.0; an unpinned ruff moves the baseline between" >&2
    echo "  minor releases because its default rule set changes." >&2
    exit 127
fi

EXCLUDE_PATHS="crates/xv6-mbc/upstream"
IGNORE_RULES="BLE001"

echo "============================================================"
echo "  ruff — $("${RUFF}" --version)"
echo "  excluded paths : ${EXCLUDE_PATHS}"
echo "  ignored rules  : ${IGNORE_RULES}"
echo "============================================================"

OUTPUT="$("${RUFF}" check . \
    --exclude "${EXCLUDE_PATHS}" \
    --ignore "${IGNORE_RULES}" \
    --output-format concise 2>&1)"
STATUS=$?

if [ "${STATUS}" -ne 0 ]; then
    printf '%s\n' "${OUTPUT}"
    echo
    echo "============================================================"
    echo "  FAIL: ruff is ratcheted to zero."
    echo
    echo "  Fix the finding, or — if it genuinely should not apply —"
    echo "  add a rule-ID exclusion to this script WITH a reason and"
    echo "  what would remove it. Do not raise a threshold, do not"
    echo "  filter by severity, and do not silence it with a bare"
    echo "  noqa: a comment whose first word is 'noqa' suppresses"
    echo "  every rule on that line, not the one you meant."
    echo "============================================================"
    exit 1
fi

echo "PASS: ruff 0 findings (with the exclusions above)."
exit 0
