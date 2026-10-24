#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "zhen-index-swap", "params": ["NEW_INDEX", "OLD_INDEX"], "preconditions_has": ["test\\s+-f"], "rollback_has": ["ln\\s+-sfn[^\\n]*OLD_INDEX"], "shell_has": ["ln\\s+-sfn"], "verify_has": ["readlink"]}' runbook.yaml
