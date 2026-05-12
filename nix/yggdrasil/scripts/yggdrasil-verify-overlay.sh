#!/bin/bash
# yggdrasil-verify-overlay.sh — Pre-build sanity check on the overlay tree.
# Free to use. Free to share.
set -euo pipefail

OVERLAY_DIR="$(dirname "$0")/../overlay"
PATCHES_DIR="$OVERLAY_DIR/patches"
UPC_DIR="$OVERLAY_DIR/upc"

FAILURES=0

check() {
    local label="$1"; shift
    if "$@" >/dev/null 2>&1; then
        echo "OK: $label"
    else
        echo "FAIL: $label"
        FAILURES=$((FAILURES + 1))
    fi
}

echo "=== Yggdrasil overlay pre-build verification ==="
check "overlay/patches/ exists" test -d "$PATCHES_DIR"
check "overlay/patches/series exists" test -f "$PATCHES_DIR/series"
check "overlay/upc/ exists" test -d "$UPC_DIR"
check "overlay/upc/series exists" test -f "$UPC_DIR/series"
check "overlay/upc/0001-add-upc-apt-source.patch exists" test -f "$UPC_DIR/0001-add-upc-apt-source.patch"
check "overlay/upc/0002-preinstall-upc-tools.patch exists" test -f "$UPC_DIR/0002-preinstall-upc-tools.patch"
check "overlay/systemd/upc-tty-bridge.service exists" test -f "$OVERLAY_DIR/systemd/upc-tty-bridge.service"
check "bin/yggdrasil-doctor-upc exists" test -x "$(dirname "$0")/../bin/yggdrasil-doctor-upc"
check "evidence-pack/schema/manifest-v1.yaml exists" test -f "$(dirname "$0")/../evidence-pack/schema/manifest-v1.yaml"

# Patch budget per OS-FORK-DISCIPLINE.md §8
if [ -f "$PATCHES_DIR/series" ]; then
    PATCH_COUNT=$(grep -cv '^#' "$PATCHES_DIR/series" 2>/dev/null || echo 0)
    if [ "$PATCH_COUNT" -le 50 ]; then
        echo "OK: patch count $PATCH_COUNT ≤ 50"
    else
        echo "FAIL: patch count $PATCH_COUNT exceeds budget 50"
        FAILURES=$((FAILURES + 1))
    fi
fi

# Every patch has SPDX header
for P in "$PATCHES_DIR"/*.patch "$UPC_DIR"/*.patch; do
    [ -f "$P" ] || continue
    if grep -q "From\|Subject" "$P" 2>/dev/null; then
        echo "OK: $(basename "$P") has patch header"
    else
        echo "WARN: $(basename "$P") missing standard patch header"
    fi
done

if [ "$FAILURES" -gt 0 ]; then
    echo "FAIL: $FAILURES overlay verification(s) failed"
    exit 1
fi
echo "=== Overlay verification PASSED ==="
