#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-js-lint.sh — oxlint over every tracked .js file (ADR-006).
#
# Replaces eslint, the repo's only npm use: eslint runs its ~76 npm
# dependencies' code while it lints, and the Sept 2025 npm worm showed what
# that costs on a host. oxlint is one static binary from the oxc project,
# pinned by version and sha256 in lib/tool-pins.sh (checked on every run),
# like gitleaks, syft, grype and helm. No npm, no node_modules.
#
# Rules and globals are in .oxlintrc.json: eslint's core correctness rules,
# the browser globals, and, per file, the globals other <script> files on
# the same page define (oxlint does not read /* global */ comments).
# Parity with eslint 10.12.0 measured 2026-10-21: 0 findings on the same 41
# files; 57 of 59 rules (no-dupe-args is reported as no-redeclare; legacy
# octal literals like 071 are not caught).
#
# Kept current by scripts/check-tool-freshness.sh (security.yml, daily).
#
#   scripts/check-js-lint.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"
# shellcheck source=scripts/lib/tool-pins.sh
source scripts/lib/tool-pins.sh
# shellcheck source=scripts/lib/fetch-tool.sh
source scripts/lib/fetch-tool.sh
BIN="$(fetch_tool oxlint "$OXLINT_VERSION" \
	"https://github.com/oxc-project/oxc/releases/download/oxlint_v${OXLINT_VERSION}/oxlint-x86_64-unknown-linux-musl.tar.gz" \
	"$OXLINT_SHA256" oxlint-x86_64-unknown-linux-musl)"

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT
rc=0
# JSON, not the text report: oxlint drops its "on N files" summary when it
# sees an AI agent's environment (CLAUDECODE, AI_AGENT), and the vacuous-pass
# check below needs that number whoever runs this.
"$BIN" -c .oxlintrc.json --deny-warnings --format json . >"$OUT" || rc=$?
python3 -I - "$OUT" "$rc" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
rc = int(sys.argv[2])
for g in d["diagnostics"]:
    span = (g.get("labels") or [{}])[0].get("span", {})
    print(f'{g["filename"]}:{span.get("line", 0)}:{span.get("column", 0)}: {g["message"]} [{g["code"]}]')
n = d.get("number_of_files", 0)
# Refuse to pass vacuously: a config or ignore mistake that lints nothing
# would otherwise report success.
if n == 0:
    print("[FAIL] oxlint linted no files")
    sys.exit(2)
if rc or d["diagnostics"]:
    print(f'[FAIL] {len(d["diagnostics"])} finding(s) in {n} files (oxlint, .oxlintrc.json)')
    sys.exit(1)
print(f'[PASS] oxlint: {n} files, {d.get("number_of_rules")} rules, no findings')
PY
