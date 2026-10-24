#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "east-service-restart", "params": ["SERVICE"], "requires_sudo": true, "shell_has": ["systemctl\\s+restart\\s+unheaded-\\$\\{?SERVICE"], "verify_has": ["is-active"]}' runbook.yaml
