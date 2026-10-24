#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "wotan-health-restart", "requires_sudo": true, "shell_has": ["curl[^\\n]*18000/health", "systemctl\\s+restart\\s+unheaded-wotan"], "verify_has": ["18000/health"]}' runbook.yaml
