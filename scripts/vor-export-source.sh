#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Export tracked files of HEAD for vor (DP-712). A hardened unit cannot read
# /home, and a working tree holds .env; git archive carries tracked files only.
#   scripts/vor-export-source.sh <destination directory>
set -euo pipefail
dest="${1:?usage: vor-export-source.sh <destination directory>}"
[[ "$dest" == /*/unheaded ]] || { echo "vor-export-source: destination must be an absolute path ending in /unheaded" >&2; exit 2; }
if [[ -e "$dest" && ! -e "$dest/.vor-export" ]]; then
	echo "vor-export-source: $dest is not an earlier export; refusing to replace it" >&2
	exit 2
fi
repo="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
tmp="$(mktemp -d "$(dirname "$dest")/.vor-export.XXXXXX")"
trap 'rm -rf -- "$tmp"' EXIT
git -C "$repo" archive --format=tar HEAD | tar -x -C "$tmp"
touch "$tmp/.vor-export"
chmod -R a+rX "$tmp"
rm -rf -- "$dest"
mv -- "$tmp" "$dest"
trap - EXIT
echo "exported $(git -C "$repo" rev-parse --short HEAD) to $dest"
