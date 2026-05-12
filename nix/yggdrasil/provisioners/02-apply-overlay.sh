#!/bin/bash
# 02-apply-overlay.sh — Apply quilt overlay patch series.
# Free to use. Free to share.
set -euo pipefail

OVERLAY_PATCHES_DIR="${OVERLAY_PATCHES_DIR:-/tmp/yggdrasil-overlay/patches}"

echo "=== Installing quilt ==="
DEBIAN_FRONTEND=noninteractive apt-get install -y -q quilt

echo "=== Applying Yggdrasil overlay patches ==="
if [ ! -f "$OVERLAY_PATCHES_DIR/series" ]; then
    echo "WARN: no series file at $OVERLAY_PATCHES_DIR/series — skipping overlay apply"
    exit 0
fi

cd /
export QUILT_PATCHES="$OVERLAY_PATCHES_DIR"
export QUILT_SERIES="$OVERLAY_PATCHES_DIR/series"

# Defense in depth — verify patch count + LOC delta budget per OS-FORK-DISCIPLINE.md §8
PATCH_COUNT=$(grep -cv '^#' "$QUILT_SERIES" 2>/dev/null || echo 0)
if [ "$PATCH_COUNT" -gt 50 ]; then
    echo "FAIL: patch count $PATCH_COUNT exceeds OS-FORK-DISCIPLINE.md §8 budget (50)"
    exit 1
fi

TOTAL_LOC=0
while IFS= read -r p; do
    [ -z "$p" ] && continue
    [[ "$p" =~ ^# ]] && continue
    LOC=$(diffstat -p1 -t "$OVERLAY_PATCHES_DIR/$p" 2>/dev/null | tail -n+2 | awk -F, '{sum += $2 + $3} END {print sum+0}')
    TOTAL_LOC=$((TOTAL_LOC + LOC))
done < "$QUILT_SERIES"

if [ "$TOTAL_LOC" -gt 5000 ]; then
    echo "FAIL: total LOC delta $TOTAL_LOC exceeds OS-FORK-DISCIPLINE.md §8 budget (5000)"
    exit 1
fi

echo "=== Discipline gates passed: $PATCH_COUNT patches, $TOTAL_LOC LOC delta ==="

# Apply via quilt
if ! quilt push -a; then
    echo "FAIL: quilt push -a failed; an overlay patch does not apply cleanly against the anchor"
    quilt top
    exit 1
fi

echo "=== Step 02 complete ($PATCH_COUNT patches applied) ==="
