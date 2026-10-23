#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# The Kanban Genesis cards live in db/seed/kanban-genesis.json (reviewable)
# and reach the Well through db/migrations/034_app_kanban_genesis.sql,
# which scripts/kanban/render-genesis-sql.py renders from it. This fails
# when the two disagree, so neither can drift from the other (S-TWO-BRAINS
# Step 421). Same pattern as check-k8s-well-initdb.sh.
set -euo pipefail
cd "$(dirname "$0")/.."
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
python3 scripts/kanban/render-genesis-sql.py --out "$tmp"
if ! cmp -s "$tmp" db/migrations/034_app_kanban_genesis.sql; then
	echo "FAIL: db/migrations/034_app_kanban_genesis.sql is not the render of db/seed/kanban-genesis.json"
	echo "  re-render: python3 scripts/kanban/render-genesis-sql.py (and never edit 034 once applied: add a migration)"
	diff -u db/migrations/034_app_kanban_genesis.sql "$tmp" | head -20 || true
	exit 1
fi
echo "db/migrations/034_app_kanban_genesis.sql matches db/seed/kanban-genesis.json ($(python3 -c 'import json,sys; print(len(json.load(open(sys.argv[1]))))' db/seed/kanban-genesis.json) cards)"
