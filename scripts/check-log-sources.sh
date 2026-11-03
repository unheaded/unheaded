#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# S-LOG-HARVEST gate: every systemd unit file under deploy/systemd, every
# service in docker-compose.yml and every host in compliance/hosts has an
# entry in config/logging/sources.yaml, and the file validates (unique ids,
# known kinds, a retention class the policy names, a reason on every
# exclusion). The tests in pkg/logsources are the gate; this wrapper gives CI
# and the meta-gate (scripts/check-gates-can-fail.sh) one exit code.
#
#   ./scripts/check-log-sources.sh
#
# Env overrides (used by the meta-gate's provocations, not by CI):
#   LOGSOURCES_FILE, LOGSOURCES_UNITS_DIR, LOGSOURCES_COMPOSE
set -euo pipefail
cd "$(dirname "$0")/.."
if ! out="$(go test ./pkg/logsources/ -count=1 2>&1)"; then
    echo "check-log-sources: FAIL" >&2
    printf '%s\n' "${out}" | grep -vE '^(ok|FAIL\s+unheaded|---)' | sed 's/^/  /' >&2
    exit 1
fi
echo "check-log-sources: OK (config/logging/sources.yaml covers every unit, compose service and host)"
