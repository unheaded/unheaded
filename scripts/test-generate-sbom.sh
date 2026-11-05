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

# The verdict: a license nobody read clears nothing.
verdict_for() { # verdict_for GO_LICENSE RUST_LICENSE: the report's GPL boundary line
	local d="$T/verdict"
	mkdir -p "$d"
	: >"$d/python-pip-freeze.txt"
	printf '{"summary": {"totalDependencies": 1, "directDependencies": 1}, "packages": [{"module": "example.invalid/m", "version": "v1", "indirect": false, "license": "%s"}]}\n' "$1" >"$d/go-dependencies.json"
	printf '{"summary": {"totalExternalCrates": 1, "workspaceCrates": ["w"], "lockFiles": ["Cargo.lock"]}, "packages": [{"crate": "c", "version": "1", "license": "%s"}]}\n' "$2" >"$d/rust-dependencies.json"
	SBOM_DIR="$d" "$REPO_ROOT/scripts/generate-sbom.sh" --report-only >/dev/null 2>&1
	sed -n '/^## GPL boundary$/{n;n;p}' "$d/SBOM.md"
}
ok "every license read, none GPL: PASS" grep -q '^PASS: every license was read' <(verdict_for MIT Apache-2.0)
ok "a GPL Go module: REVIEW, by name" grep -q '^REVIEW: .*example.invalid/m' <(verdict_for GPL-3.0-only MIT)
ok "a GPL Rust crate: REVIEW, by name" grep -q '^REVIEW: .*: c\.' <(verdict_for MIT GPL-2.0-only)
ok "an Unknown Go module: not PASS, and counted" grep -q '^INCOMPLETE: .*1 of 1 Go modules are Unknown and 0 of 1 Rust' <(verdict_for Unknown MIT)
ok "an assumed Rust crate: not PASS, and counted" grep -q '^INCOMPLETE: .*0 of 1 Go modules are Unknown and 1 of 1 Rust' <(verdict_for MIT 'MIT OR Apache-2.0 (assumed)')
ok "the committed report counts what it did not read" grep -qE '^INCOMPLETE: .*[1-9][0-9]* of [0-9]+ Go modules are Unknown' "$T/SBOM.md"
ok "it lists the workspace crates and lock files the JSON holds" grep -q '^Rust lock files read: `' "$T/SBOM.md"

strip_date() { grep -v '^\*\*Generated:\*\*' "$1"; }
ok "the committed SBOM.md matches its detail files (scripts/generate-sbom.sh --report-only)" \
	cmp -s <(strip_date "$REPO_ROOT/sbom/SBOM.md") <(strip_date "$T/SBOM.md")

ok "the full run writes it too" grep -qE '^write_report$' "$REPO_ROOT/scripts/generate-sbom.sh"

[ "$fail" -eq 0 ] || exit 1
echo "[PASS] sbom/SBOM.md is generated and current"
