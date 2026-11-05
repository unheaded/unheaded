#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# generate-sbom.sh -- Generate Software Bill of Materials for Unheaded
#
# Lightweight SBOM generation using built-in tooling (go list, Cargo.lock parsing).
# No external tools required (ScanCode, FOSSology, Syft are optional). Licenses
# are read from what is on this host (the Go module cache, the cargo registry,
# the Zhen venv, any venv in PYTHON_EXTRA_VENVS) by scripts/lib/sbom_licenses.py;
# it fetches nothing, and what it cannot read the report names.
#
# Usage: ./scripts/generate-sbom.sh                # everything below
#        ./scripts/generate-sbom.sh --report-only  # only SBOM.md, from the files
#                                                  # already in sbom/
#
# Outputs:
#   sbom/go-dependencies.json     -- Go module SBOM (SPDX-2.3 compatible), every
#       tracked go.mod outside testdata/
#   sbom/rust-dependencies.json   -- Rust crate SBOM (SPDX-2.3 compatible)
#   sbom/python-pip-freeze.txt  -- Python package list (pip freeze of the
#       whole Zhen venv). Not named *requirements*.txt on purpose: GitHub's
#       dependency graph read it as a manifest and raised Dependabot alerts
#       for packages nothing installs from here (2026-10-28). The manifest
#       is raft/requirements.txt.
#       Kept as it was when this host has no such venv.
#   sbom/python-dependencies.json -- that list and every tracked
#       requirements file, with each package's license
#   sbom/javascript-dependencies.json -- CDN-loaded files the HTML pages name
#   sbom/SBOM.md                  -- Human-readable summary of the above,
#       written last, so its numbers are theirs (scripts/test-generate-sbom.sh)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SBOM_DIR="${SBOM_DIR:-$PROJECT_ROOT/sbom}"
TODAY="$(date -u +%Y-%m-%d)"

mkdir -p "$SBOM_DIR"

# write_report: sbom/SBOM.md from the JSON. Everything but the notes at the
# end is read from those files.
write_report() {
    python3 "$SCRIPT_DIR/lib/sbom_licenses.py" report "$SBOM_DIR" "$TODAY"
}

if [ "${1:-}" = "--report-only" ]; then
    write_report
    echo "Wrote $SBOM_DIR/SBOM.md"
    exit 0
fi

echo "=== Unheaded SBOM Generator ==="
echo "Date: $TODAY"
echo "Project root: $PROJECT_ROOT"
echo ""

WORK="$(mktemp -d "${TMPDIR:-/var/tmp}/generate-sbom.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# --------------------------------------------------------------------------
# 1. Go Dependencies
# --------------------------------------------------------------------------
echo "[1/5] Generating Go dependency SBOM..."

cd "$PROJECT_ROOT"
# A failure here stops the run: an empty list is not a clean SBOM.
go list -m -json all > "$WORK/modules.json"
go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./... > "$WORK/compiled.raw"
# Any other module (tools/gosec) is read from its go.mod alone, which lists every
# module it needs and asks nothing of the network; go list -m all would.
GO_MOD_EDITS=()
while read -r gomod; do
    grep -q '^require' "$PROJECT_ROOT/$gomod" || continue
    edit="$WORK/gomod.${#GO_MOD_EDITS[@]}.json"
    (cd "$PROJECT_ROOT/$(dirname "$gomod")" && go mod edit -json) > "$edit"
    (cd "$PROJECT_ROOT/$(dirname "$gomod")" && go list -deps -f '{{with .Module}}{{.Path}}{{end}}' ./... tool) \
        >> "$WORK/compiled.raw"
    GO_MOD_EDITS+=("$edit")
done < <(git -C "$PROJECT_ROOT" ls-files -- '*/go.mod' ':(exclude,glob)**/testdata/**')
sort -u "$WORK/compiled.raw" > "$WORK/compiled.txt"
python3 "$SCRIPT_DIR/lib/sbom_licenses.py" go "$TODAY" "$WORK/modules.json" \
    "$(go env GOMODCACHE)" "$WORK/compiled.txt" "${GO_MOD_EDITS[@]}" > "$SBOM_DIR/go-dependencies.json"

# --------------------------------------------------------------------------
# 2. Rust Dependencies
# --------------------------------------------------------------------------
echo "[2/5] Generating Rust dependency SBOM..."

python3 "$SCRIPT_DIR/lib/sbom_licenses.py" rust "$TODAY" "$PROJECT_ROOT" \
    "${CARGO_HOME:-$HOME/.cargo}" > "$SBOM_DIR/rust-dependencies.json"

# --------------------------------------------------------------------------
# 3. Python Dependencies
# --------------------------------------------------------------------------
echo "[3/5] Generating Python dependency list..."

PYTHON_VENV="${PYTHON_VENV:-$HOME/.venv/zhen}"
SITES=()
if [ -f "$PYTHON_VENV/bin/activate" ]; then
    # shellcheck disable=SC1091
    source "$PYTHON_VENV/bin/activate"
    pip freeze > "$SBOM_DIR/python-pip-freeze.txt" 2>/dev/null
    PY_COUNT=$(wc -l < "$SBOM_DIR/python-pip-freeze.txt")
    echo "  Python: $PY_COUNT packages from $PYTHON_VENV"
    SITES+=("$(python3 -c 'import sysconfig; print(sysconfig.get_path("purelib"))')")
    deactivate 2>/dev/null || true
else
    # The last freeze stays: its packages go in the report as not read, by name.
    # Writing over it would drop them, and the report would never say so.
    echo "  Python: No virtualenv found at $PYTHON_VENV (its licenses stay not read)"
    [ -s "$SBOM_DIR/python-pip-freeze.txt" ] ||
        echo "# No Python virtualenv found at generation time" > "$SBOM_DIR/python-pip-freeze.txt"
fi
# The CI tools' venv and the like, colon-separated. Only a package at the very
# version a manifest pins is read from one.
IFS=: read -ra EXTRA_VENVS <<< "${PYTHON_EXTRA_VENVS:-}"
for venv in ${EXTRA_VENVS[@]+"${EXTRA_VENVS[@]}"}; do
    if [ -x "$venv/bin/python3" ]; then
        SITES+=("$("$venv/bin/python3" -c 'import sysconfig; print(sysconfig.get_path("purelib"))')")
    else
        echo "  Python: No virtualenv at $venv (skipped)"
    fi
done
git -C "$PROJECT_ROOT" ls-files -- '*requirements*.txt' ':(exclude,glob)**/testdata/**' > "$WORK/manifests.txt"
python3 "$SCRIPT_DIR/lib/sbom_licenses.py" python "$TODAY" "$SBOM_DIR/python-pip-freeze.txt" \
    "$PROJECT_ROOT" "$WORK/manifests.txt" ${SITES[@]+"${SITES[@]}"} > "$SBOM_DIR/python-dependencies.json"

# --------------------------------------------------------------------------
# 4. JavaScript loaded from CDNs
# --------------------------------------------------------------------------
echo "[4/5] Listing CDN-loaded JavaScript..."

git -C "$PROJECT_ROOT" ls-files '*.html' > "$WORK/pages.txt"
python3 "$SCRIPT_DIR/lib/sbom_licenses.py" javascript "$TODAY" "$PROJECT_ROOT" "$WORK/pages.txt" \
    > "$SBOM_DIR/javascript-dependencies.json"

# --------------------------------------------------------------------------
# 5. Summary and GPL boundary verdict
# --------------------------------------------------------------------------
echo "[5/5] Writing the report..."

write_report

echo ""
echo "Output files:"
echo "  $SBOM_DIR/go-dependencies.json"
echo "  $SBOM_DIR/rust-dependencies.json"
echo "  $SBOM_DIR/python-pip-freeze.txt"
echo "  $SBOM_DIR/python-dependencies.json"
echo "  $SBOM_DIR/javascript-dependencies.json"
echo "  $SBOM_DIR/SBOM.md"
echo ""
echo "Done. Review sbom/SBOM.md for the summary."
