#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
# Prepare OPNsense image helper script
# Can be used to resize or inspect the image before boot

set -euo pipefail

IMG="${1:-/images/opnsense.img}"

if [[ ! -f "$IMG" ]]; then
    echo "ERROR: Image not found at $IMG"
    exit 1
fi

# Resize if needed (e.g., qemu-img resize opnsense.img +10G)
# qemu-img info "$IMG"

echo "Image prepared: $IMG"
echo "Size: $(du -h "$IMG" | cut -f1)"
