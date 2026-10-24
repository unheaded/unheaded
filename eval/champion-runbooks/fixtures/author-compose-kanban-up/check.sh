#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "compose-kanban-up", "order": [["up[^\\n]*postgres", "up[^\\n]*kanban-app"]], "rollback_has": ["docker\\s+compose[^\\n]*(stop|down)"], "verify_has": ["20001/health"]}' runbook.yaml
