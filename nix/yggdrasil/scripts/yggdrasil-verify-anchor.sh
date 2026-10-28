#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# yggdrasil-verify-anchor.sh — Verify anchor.nix points at a real Debian release.
# Runs on the BUILD HOST (shell-local provisioner) BEFORE any in-VM work.
# Free to use. Free to share.
set -euo pipefail

ANCHOR_FILE="$(dirname "$0")/../anchor.nix"
if [ ! -f "$ANCHOR_FILE" ]; then
    echo "FAIL: anchor.nix missing at $ANCHOR_FILE"
    exit 1
fi

# anchor.nix's keys are `codename` and `version` (not release_*: that
# spelling never matched, so this script failed on every run until the
# first PR touching nix/yggdrasil/ exercised it, 2026-10-28).
CODENAME=$(grep -E '^\s*codename\s*=' "$ANCHOR_FILE" | sed -E 's/.*"([^"]+)".*/\1/' | head -1)
VERSION_LINE=$(grep -E '^\s*version\s*=' "$ANCHOR_FILE" | head -1)
VERSION=$(echo "$VERSION_LINE" | sed -nE 's/.*"([^"]+)".*/\1/p')
YGG_VERSION=$(grep -E '^\s*yggdrasil_version\s*=' "$ANCHOR_FILE" | sed -E 's/.*"([^"]+)".*/\1/' | head -1)

if [ -z "$CODENAME" ]; then
    echo "FAIL: anchor.nix missing codename"
    exit 1
fi

case "$CODENAME" in
    bookworm|trixie|forky)
        echo "OK: anchor codename '$CODENAME' is a known Debian release"
        ;;
    *)
        echo "FAIL: anchor codename '$CODENAME' is not a recognized Debian release"
        echo "      Must be bookworm (12), trixie (13), or forky (14)"
        exit 1
        ;;
esac

# Before the first packer build the anchor is a scaffold: version is null
# and yggdrasil_version ends in -scaffold. Both together are allowed; a
# null version on a non-scaffold anchor is not.
if [[ "$VERSION_LINE" =~ =[[:space:]]*null ]] && [[ "$YGG_VERSION" == *-scaffold ]]; then
    echo "NOTE: anchor version unset (yggdrasil_version $YGG_VERSION): scaffold, no build yet"
    echo "OK: Yggdrasil anchor verified — $CODENAME (scaffold)"
    exit 0
fi

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
    echo "FAIL: anchor version '$VERSION' not in N.N or N.N.N form"
    exit 1
fi

echo "OK: Yggdrasil anchor verified — $CODENAME ($VERSION)"
