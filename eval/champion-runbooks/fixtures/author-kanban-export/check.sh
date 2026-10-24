#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "kanban-export", "shell_has": ["20001/api/v1/tasks"], "verify_has": ["json\\.tool|\\bjq\\b"]}' runbook.yaml
