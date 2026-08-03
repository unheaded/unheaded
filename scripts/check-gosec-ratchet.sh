#!/bin/bash
# SPDX-License-Identifier: MIT
# check-gosec-ratchet.sh — enforce that the gosec exclusion list only SHRINKS.
#
# The gate in .github/workflows/security.yml suppresses gosec rules by name
# while their findings are worked through
# (docs/security/findings-remediation-2026-07-29.md). That list is a security
# liability: nothing in YAML stops someone appending a rule to make a red
# build go green, and the diff looks like a one-word config tweak.
#
# This session found THREE gates that were green because they could not fail
# (cargo-audit `|| true`, gosec `//nolint` annotations it ignores, trivy
# scanners never enabled). A suppression list guarded only by good intentions
# is the same bug waiting to happen a fourth time.
#
# Contract:
#   workflow exclusions MUST be a subset of the baseline.
#   Removing a rule (remediated)  -> PASS, and the baseline should be updated.
#   Adding a rule (new suppression) -> FAIL. Fix the finding instead.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOW="${REPO_ROOT}/.github/workflows/security.yml"
BASELINE="${REPO_ROOT}/docs/security/gosec-ratchet-baseline.txt"

[ -f "${WORKFLOW}" ] || { echo "FAIL: missing ${WORKFLOW}"; exit 1; }
[ -f "${BASELINE}" ] || { echo "FAIL: missing ${BASELINE}"; exit 1; }

CURRENT="$(grep -oP '(?<=-exclude=)[A-Z0-9,]+' "${WORKFLOW}" | tr ',' '\n' | grep -E '^G[0-9]+$' | sort -u)"
ALLOWED="$(grep -E '^G[0-9]+$' "${BASELINE}" | sort -u)"

if [ -z "${CURRENT}" ]; then
    echo "PASS: gosec runs with no rule exclusions — ratchet fully closed."
    exit 0
fi

# Any rule excluded in the workflow but absent from the baseline is a NEW
# suppression. That is the thing this script exists to stop.
ADDED="$(comm -23 <(printf '%s\n' "${CURRENT}") <(printf '%s\n' "${ALLOWED}"))"
REMOVED="$(comm -13 <(printf '%s\n' "${CURRENT}") <(printf '%s\n' "${ALLOWED}"))"

if [ -n "${ADDED}" ]; then
    echo "============================================================"
    echo "  FAIL: gosec ratchet went BACKWARDS"
    echo "============================================================"
    echo
    echo "  New rule suppressions not present in the baseline:"
    # shellcheck disable=SC2086  # deliberate split: one rule ID per line
    printf '    %s\n' ${ADDED}
    echo
    echo "  The exclusion list may only shrink. Suppressing a new rule"
    echo "  hides findings rather than fixing them."
    echo
    echo "  If a finding is a genuine false positive, annotate the SITE"
    echo "  with '// #nosec Gxxx -- <reason>' (note: gosec ignores"
    echo "  //nolint:gosec) rather than disabling the rule tree-wide."
    echo
    echo "  Baseline: docs/security/gosec-ratchet-baseline.txt"
    echo "  Plan:     docs/security/findings-remediation-2026-07-29.md"
    echo "============================================================"
    exit 1
fi

# ── Guard 3: no #nosec swallowed by a string literal ─────────────────────────
#
# Appending "// #nosec ..." to a line ending in an opening backtick puts the
# annotation INSIDE the raw string, not in a comment. Go still compiles: the
# text simply becomes string content. That is how a bulk annotation pass turned
#     query := fmt.Sprintf(`
# into a SQL statement whose first line was "// #nosec G201 -- ...".
#
# The build cannot catch this and neither can gosec. Check it directly.
if command -v python3 >/dev/null 2>&1; then
    SWALLOWED="$(cd "${REPO_ROOT}" && git ls-files '*.go' | python3 -c '
import sys
bad=[]
for f in sys.stdin.read().split():
    try: src=open(f).read()
    except OSError: continue
    if "#nosec" not in src: continue
    raw=False
    for n,l in enumerate(src.split("\n"),1):
        if "#nosec" in l and (raw or l.split("#nosec")[0].count("`")%2==1):
            bad.append(f"{f}:{n}")
        raw ^= (l.count("`")%2==1)
print("\n".join(bad))
')"
    if [ -n "${SWALLOWED}" ]; then
        echo "============================================================"
        echo "  FAIL: #nosec swallowed by a string literal"
        echo "============================================================"
        echo
        # shellcheck disable=SC2086  # deliberate split: one rule ID per line
    printf '    %s\n' ${SWALLOWED}
        echo
        echo "  The annotation is string CONTENT, not a comment — it suppresses"
        echo "  nothing and corrupts the literal. Put it on the line ABOVE."
        echo "============================================================"
        exit 1
    fi
fi

# ── Guard 2: #nosec must LEAD its comment ────────────────────────────────────
#
# gosec only honours a suppression when the comment text begins with "#nosec".
# ANY text in front of it makes the annotation inert — not just //nolint:
#
#     foo() //nolint:gosec // reason // #nosec G306 -- ...   <- inert
#     emit(...)            // 63: JMP // #nosec G115 -- ...  <- also inert
#
# The second form is easy to create by appending to a line that already had a
# trailing comment, and it reads as annotated. This repo shipped ~20 of the
# first kind; a bulk pass in this branch created 13 of the second. Both are
# caught here by requiring #nosec to be the first thing in its comment.
#
# gosec only honours a suppression comment that LEADS with "#nosec". A line
# like:
#     foo() //nolint:gosec // public cert // #nosec G306 -- reason
# looks annotated in review and is completely inert — gosec never sees it.
#
# This repo shipped ~20 such annotations. They read as dispositions and
# suppressed nothing, which is how the gosec job came to report findings
# everyone believed were already handled. Catch it mechanically instead of
# hoping the next reviewer spots it.
SHADOWED="$(cd "${REPO_ROOT}" && git ls-files '*.go' | python3 -c '
import sys
bad=[]
for f in sys.stdin.read().split():
    try: src=open(f).read()
    except OSError: continue
    if "#nosec" not in src: continue
    for n,l in enumerate(src.split("\n"),1):
        if "#nosec" not in l: continue
        i=l.find("//")
        if i>=0 and not l[i+2:].strip().startswith("#nosec"):
            bad.append(f"{f}:{n}")
print("\n".join(bad))
' 2>/dev/null || true)"
if [ -n "${SHADOWED}" ]; then
    echo "============================================================"
    echo "  FAIL: #nosec does not lead its comment (annotation is inert)"
    echo "============================================================"
    echo
    printf '%s\n' "${SHADOWED}"
    echo
    echo "  gosec only honours '#nosec' when it LEADS the comment."
    echo "  Rewrite as:  // #nosec Gxxx -- reason"
    echo "============================================================"
    exit 1
fi

echo "PASS: gosec ratchet intact — $(printf '%s\n' "${CURRENT}" | wc -l)/$(printf '%s\n' "${ALLOWED}" | wc -l) baseline rules still excluded."
if [ -n "${REMOVED}" ]; then
    echo
    echo "  Progress — these rules are now ENFORCED and can never be re-added:"
    # shellcheck disable=SC2086  # deliberate split: one rule ID per line
    printf '    %s\n' ${REMOVED}
    echo
    echo "  Update the baseline to lock the win in:"
    echo "    ./scripts/check-gosec-ratchet.sh --update"
fi

if [ "${1:-}" = "--update" ]; then
    printf '%s\n' "${CURRENT}" > "${BASELINE}"
    echo "  Baseline updated -> $(printf '%s\n' "${CURRENT}" | wc -l) rules."
fi
exit 0
