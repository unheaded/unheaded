#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when the script parses and shellcheck reports nothing at info level.
set -euo pipefail
cd "$(dirname "$0")"
bash -n sizes.sh
shellcheck -S info sizes.sh
