#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Solved when usage.sh's help documents every option it accepts and the
# script still parses and behaves the same.
set -euo pipefail
cd "$(dirname "$0")"
bash -n usage.sh
help="$(bash usage.sh --help)"
for opt in --verbose --dry-run; do
	grep -q -- "$opt" <<<"$help" || { echo "usage text does not document $opt"; exit 1; }
done
[[ "$(bash usage.sh --dry-run x)" == "would greet x" ]] || { echo "--dry-run changed behaviour"; exit 1; }
[[ "$(bash usage.sh x)" == "hello x" ]] || { echo "greeting changed"; exit 1; }
echo ok
