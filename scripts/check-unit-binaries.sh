#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-unit-binaries.sh — every deploy/systemd unit runs a binary that
# scripts/build-binaries.sh builds (or a named third-party one). ADR-085.
exec "$(dirname "$0")/build-binaries.sh" --check-units
