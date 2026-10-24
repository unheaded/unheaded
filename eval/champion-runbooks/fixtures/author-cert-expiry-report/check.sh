#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when runbook.yaml passes the strict check and this task's requirements.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py --require '{"name": "cert-expiry-report", "shell_has": ["openssl\\s+x509[^\\n]*-enddate", "-checkend\\s+2592000"]}' runbook.yaml
