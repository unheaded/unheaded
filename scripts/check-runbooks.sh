#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-runbooks.sh: the runbook/v1 gate (ADR-024, S-TWO-BRAINS Phase 8).
# Every runbook under runbooks/ against scripts/check-runbooks.py, ratcheted
# by runbooks/.check-baseline (may only shrink). Exit 1 on any finding not in
# the baseline, or on a baseline entry that no longer fails.
# Registered in scripts/check-gates-can-fail.sh, which proves it can fail.
set -euo pipefail
cd "$(dirname "$0")/.."
exec python3 -B scripts/check-runbooks.py runbooks/
