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
#
# 2026-08-03: extended to .ipynb after the exact same blind spot showed up one
# artifact class over. notebooks/02_hypothesis_matrix.ipynb had three code
# cells that could not parse — labels written as 'H1 RAM<newline>(MB)' where
# 'H1 RAM\n(MB)' was meant, so every one was an unterminated string literal.
# It was born that way in 3dbd7eee and had never been run. This check was
# GATING and green the whole time, because it only ever looked at *.py.
#
# That single file was 104 of ruff's 427 findings — the parser cascades — so
# it also made the ruff baseline read far worse than the tree deserved.
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

# Notebooks: compile each code cell independently. Cells are not a single
# module — a NameError across cells is normal and expected — so this compiles
# per cell and reports the cell index, which is what Jupyter shows you.
# stdlib only (json + compile); no nbformat dependency.
NB_COUNT=0
while IFS= read -r f; do
    [ -f "${f}" ] || continue
    NB_COUNT=$((NB_COUNT + 1))
    if ! err="$(python3 - "${f}" <<'PY' 2>&1
import json, sys
path = sys.argv[1]
try:
    nb = json.load(open(path, encoding="utf-8"))
except Exception as e:
    print(f"not valid notebook JSON: {e}")
    sys.exit(1)
bad = 0
for i, cell in enumerate(nb.get("cells", [])):
    if cell.get("cell_type") != "code":
        continue
    src = "".join(cell.get("source", []))
    try:
        compile(src, f"{path}:cell{i}", "exec")
    except SyntaxError as e:
        bad += 1
        print(f"cell {i}, line {e.lineno}: {e.msg}")
sys.exit(1 if bad else 0)
PY
    )"; then
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
done < <(git ls-files '*.ipynb')

# py_compile litters __pycache__ next to sources; don't leave the tree dirty.
find . -name '__pycache__' -type d -prune -exec rm -rf {} + 2>/dev/null || true

if [ "${FAILED}" -gt 0 ]; then
    echo "  ${FAILED} of $((COUNT + NB_COUNT)) tracked Python files/notebooks do not compile."
    echo "  A file that cannot parse cannot run, and no linter output about"
    echo "  it means anything until this passes."
    echo "============================================================"
    exit 1
fi

echo "PASS: all ${COUNT} tracked Python files and ${NB_COUNT} notebooks parse."
exit 0
