#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Two-brains live fixture (S-TWO-BRAINS Step 216): a docs card. The
# script accepts --dry-run, but its usage text does not say so.
set -euo pipefail

usage() {
	cat <<'EOF'
usage: usage.sh [--verbose] NAME

Greets NAME.
  --verbose   say more
EOF
}

DRY=0 VERBOSE=0
while (($#)); do
	case "$1" in
	--verbose) VERBOSE=1; shift ;;
	--dry-run) DRY=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*) break ;;
	esac
done
(($# == 1)) || { usage >&2; exit 2; }
if ((DRY)); then echo "would greet $1"; exit 0; fi
if ((VERBOSE)); then echo "greeting $1 now"; fi
echo "hello $1"
