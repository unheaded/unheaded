#!/bin/bash
# yggdrasil-verify-anchor.sh — Verify anchor.nix points at a real Debian release.
# Runs on the BUILD HOST (shell-local provisioner) BEFORE any in-VM work.
# Free to use. Free to share.
set -euo pipefail

ANCHOR_FILE="$(dirname "$0")/../anchor.nix"
if [ ! -f "$ANCHOR_FILE" ]; then
    echo "FAIL: anchor.nix missing at $ANCHOR_FILE"
    exit 1
fi

CODENAME=$(grep -E '^\s*release_codename\s*=' "$ANCHOR_FILE" | sed -E 's/.*"([^"]+)".*/\1/' | head -1)
VERSION=$(grep -E '^\s*release_version\s*=' "$ANCHOR_FILE" | sed -E 's/.*"([^"]+)".*/\1/' | head -1)

if [ -z "$CODENAME" ]; then
    echo "FAIL: anchor.nix missing release_codename"
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

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
    echo "FAIL: anchor version '$VERSION' not in N.N or N.N.N form"
    exit 1
fi

echo "OK: Yggdrasil anchor verified — $CODENAME ($VERSION)"
