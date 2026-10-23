#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when the script parses and shellcheck reports nothing at info level.
set -euo pipefail
cd "$(dirname "$0")"
bash -n deploy.sh
shellcheck -S info deploy.sh
