#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when the unit tests pass (stdlib unittest; no network, no packages).
set -euo pipefail
cd "$(dirname "$0")"
python3 -B -m unittest -q 2>&1
