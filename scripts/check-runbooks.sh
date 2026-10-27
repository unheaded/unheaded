#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-runbooks.sh: the runbook/v1 gate (ADR-024, S-TWO-BRAINS Phase 8).
# Every runbook under runbooks/ against scripts/check-runbooks.py, ratcheted
# by runbooks/.check-baseline (may only shrink). Exit 1 on any finding not in
# the baseline, or on a baseline entry that no longer fails.
# Then runbooks/MANIFEST.sha256 must match the runbooks' bytes
# (scripts/runbook-manifest.sh --check, RT-08): pkg/champion refuses a live
# run of a runbook that is not in it.
# Registered in scripts/check-gates-can-fail.sh, which proves it can fail.
set -euo pipefail
cd "$(dirname "$0")/.."
rc=0
python3 -B scripts/check-runbooks.py runbooks/ || rc=$?
scripts/runbook-manifest.sh --check || rc=1
exit "$rc"
