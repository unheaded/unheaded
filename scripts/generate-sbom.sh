#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# generate-sbom.sh -- Generate Software Bill of Materials for Unheaded
#
# Lightweight SBOM generation using built-in tooling (go list, Cargo.lock parsing).
# No external tools required (ScanCode, FOSSology, Syft are optional). Licenses
# are read from what is on this host (the Go module cache, the cargo registry,
# the Zhen venv) by scripts/lib/sbom_licenses.py; it fetches nothing, and what
# it cannot read the report names.
#
# Usage: ./scripts/generate-sbom.sh                # everything below
#        ./scripts/generate-sbom.sh --report-only  # only SBOM.md, from the files
#                                                  # already in sbom/
#
# Outputs:
#   sbom/go-dependencies.json     -- Go module SBOM (SPDX-2.3 compatible)
#   sbom/rust-dependencies.json   -- Rust crate SBOM (SPDX-2.3 compatible)
#   sbom/python-pip-freeze.txt  -- Python package list (pip freeze of the
#       whole Zhen venv). Not named *requirements*.txt on purpose: GitHub's
#       dependency graph read it as a manifest and raised Dependabot alerts
#       for packages nothing installs from here (2026-10-28). The manifest
#       is raft/requirements.txt.
#   sbom/python-dependencies.json -- that list with each package's license
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
go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./... | sort -u > "$WORK/compiled.txt"
python3 "$SCRIPT_DIR/lib/sbom_licenses.py" go "$TODAY" "$WORK/modules.json" \
    "$(go env GOMODCACHE)" "$WORK/compiled.txt" > "$SBOM_DIR/go-dependencies.json"

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
SITE_PACKAGES="$WORK/no-venv"
if [ -f "$PYTHON_VENV/bin/activate" ]; then
    # shellcheck disable=SC1091
    source "$PYTHON_VENV/bin/activate"
    pip freeze > "$SBOM_DIR/python-pip-freeze.txt" 2>/dev/null
    PY_COUNT=$(wc -l < "$SBOM_DIR/python-pip-freeze.txt")
    echo "  Python: $PY_COUNT packages from $PYTHON_VENV"
    SITE_PACKAGES="$(python3 -c 'import sysconfig; print(sysconfig.get_path("purelib"))')"
    deactivate 2>/dev/null || true
else
    echo "  Python: No virtualenv found at $PYTHON_VENV (skipped)"
    echo "# No Python virtualenv found at generation time" > "$SBOM_DIR/python-pip-freeze.txt"
fi
python3 "$SCRIPT_DIR/lib/sbom_licenses.py" python "$TODAY" \
    "$SBOM_DIR/python-pip-freeze.txt" "$SITE_PACKAGES" > "$SBOM_DIR/python-dependencies.json"

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
