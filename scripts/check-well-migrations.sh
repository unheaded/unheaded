#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-101: every numbered file in db/migrations is applied by the MANIFEST
# or explicitly skipped, every file the manifest names exists, and none uses
# a psql meta-command (well-migrate sends plain SQL). A migration nobody
# routed is how 013/014 reached a fresh Well but never the live one (W-09).
# Needs Go; no database.
set -euo pipefail
cd "$(dirname "$0")/.."
go run ./cmd/well-migrate -dir db/migrations check
