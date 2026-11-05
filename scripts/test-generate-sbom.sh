#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# scripts/generate-sbom.sh writes sbom/SBOM.md, and the committed one says what
# the committed JSON says: the summary may not sit behind its detail files,
# as it did from 2026-03-15 to 2026-11-05.
set -uo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
cp "$REPO_ROOT"/sbom/go-dependencies.json "$REPO_ROOT"/sbom/rust-dependencies.json \
	"$REPO_ROOT"/sbom/python-pip-freeze.txt "$T/"
printf '**Generated:** 2026-03-15\n' >"$T/SBOM.md"

fail=0
ok() { # ok DESC COMMAND...
	local desc=$1
	shift
	if "$@"; then printf '[ok] %s\n' "$desc"; else printf '[FAIL] %s\n' "$desc"; fail=1; fi
}

SBOM_DIR="$T" "$REPO_ROOT/scripts/generate-sbom.sh" --report-only >"$T/out" 2>&1
ok "--report-only runs" [ $? -eq 0 ]
ok "a stale SBOM.md is replaced, with today's date" grep -qx "\*\*Generated:\*\* $(date -u +%Y-%m-%d)" "$T/SBOM.md"

go_total="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["summary"]["totalDependencies"])' "$T/go-dependencies.json")"
ok "its Go count is the JSON's" grep -q "^| Go | ${go_total} |" "$T/SBOM.md"

for path in $(grep -oE '`[A-Za-z_./-]+\.(md|json|txt|sh)`' "$T/SBOM.md" | tr -d '`' | sort -u); do
	ok "it names $path, which exists" [ -e "$REPO_ROOT/$path" ]
done

strip_date() { grep -v '^\*\*Generated:\*\*' "$1"; }
ok "the committed SBOM.md matches its detail files (scripts/generate-sbom.sh --report-only)" \
	cmp -s <(strip_date "$REPO_ROOT/sbom/SBOM.md") <(strip_date "$T/SBOM.md")

ok "the full run writes it too" grep -qE '^write_report$' "$REPO_ROOT/scripts/generate-sbom.sh"

[ "$fail" -eq 0 ] || exit 1
echo "[PASS] sbom/SBOM.md is generated and current"
