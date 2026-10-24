#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "gpu-lease-report", "shell_has": ["gpu\\.lock", "rocm-smi"], "shell_lacks": ["\\bkill\\b", "\\brm\\s"]}' runbook.yaml
