#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "well-dead-tuples", "risk": ["low"], "shell_has": ["\\bpsql\\b", "n_dead_tup"], "shell_lacks": ["vacuum\\s+full"]}' runbook.yaml
