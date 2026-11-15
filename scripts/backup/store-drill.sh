#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# store-drill.sh: the weekly restore drill of both stores (ADR-147 decision
# 6, unheaded-store-drill.service). Runs clickhouse-drill.sh and then
# victoria-drill.sh, both always, and exits 1 if either failed, so one
# broken store never hides the other's result.
set -uo pipefail

D="$(dirname "$0")"
rc=0
"$D/clickhouse-drill.sh" || { echo "[store-drill] ClickHouse drill FAILED" >&2; rc=1; }
"$D/victoria-drill.sh" || { echo "[store-drill] VictoriaMetrics drill FAILED" >&2; rc=1; }
((rc == 0)) && echo "[store-drill] both drills passed"
exit "$rc"
