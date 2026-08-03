#!/bin/bash
# SPDX-License-Identifier: MIT
# check-python-syntax.sh — every tracked .py file must actually parse.
#
# GATING from day one, and the tree was NOT clean when this was written:
# scripts/vault-to-runway.py had a SyntaxError at HEAD ("name 'THRESHOLD_PCT'
# is used prior to global declaration"). The script could not be imported or
# run at all — `python3 scripts/vault-to-runway.py --list` died before
# printing anything. It had presumably been broken since the --threshold flag
# was added, because nothing in CI ever parsed the Python.
#
# ruff was in the tree but non-gating and does not flag this class; bandit
# skips files it cannot parse. So a file that cannot run read as "clean" from
# both. Same shape as the YAML manifests bug that check-manifest-yaml.sh was
# written for: a whole category of artifact nobody had ever loaded.
#
# This is deliberately the dumbest possible check — compile, don't lint. It
# cannot have opinions to argue with, and it cannot be satisfied by a file
# that does not run.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}" || exit 1

FAILED=0
COUNT=0

while IFS= read -r f; do
    [ -f "${f}" ] || continue
    COUNT=$((COUNT + 1))
    if ! err="$(python3 -m py_compile "${f}" 2>&1)"; then
        if [ "${FAILED}" -eq 0 ]; then
            echo "============================================================"
            echo "  FAIL: Python files that do not parse"
            echo "============================================================"
            echo
        fi
        echo "  ${f}"
        printf '%s\n' "${err}" | sed 's/^/      /'
        echo
        FAILED=$((FAILED + 1))
    fi
done < <(git ls-files '*.py')

# py_compile litters __pycache__ next to sources; don't leave the tree dirty.
find . -name '__pycache__' -type d -prune -exec rm -rf {} + 2>/dev/null || true

if [ "${FAILED}" -gt 0 ]; then
    echo "  ${FAILED} of ${COUNT} tracked Python files do not compile."
    echo "  A file that cannot parse cannot run, and no linter output about"
    echo "  it means anything until this passes."
    echo "============================================================"
    exit 1
fi

echo "PASS: all ${COUNT} tracked Python files parse."
exit 0
