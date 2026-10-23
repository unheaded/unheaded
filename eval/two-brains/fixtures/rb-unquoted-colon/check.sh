#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when the runbook parses and passes the runbook/v1 shape check.
set -euo pipefail
cd "$(dirname "$0")"
python3 -B validate_runbook.py runbook.yaml
