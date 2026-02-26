#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# import-ipfire.sh — Import IPFire 2.29 core199 into LXD as VM image
#
set -euo pipefail

IPFIRE_XZ="${HOME}/tmp/ipfire/ipfire-2.29-core199-x86_64.img.xz"
IPFIRE_IMG="${HOME}/tmp/ipfire/ipfire-2.29-core199-x86_64.img"
WORK_DIR="$(mktemp -d)"
ALIAS="ipfire-2.29-core199"

echo "[1/5] Checking source image..."
[[ ! -f "$IPFIRE_XZ" ]] && echo "ERROR: not found at $IPFIRE_XZ" && exit 1

echo "[2/5] Decompressing IPFire image (xz → raw)..."
if [[ ! -f "$IPFIRE_IMG" ]]; then
    xz -dk "$IPFIRE_XZ"
    echo "Decompressed to: $IPFIRE_IMG"
else
    echo "Already decompressed: $IPFIRE_IMG"
fi

echo "[3/5] Creating LXD image metadata..."
cat > "${WORK_DIR}/metadata.yaml" << 'EOF'
architecture: x86_64
creation_date: 1740000000
properties:
  description: "IPFire 2.29 core199 (Linux)"
  os: "ipfire"
  release: "2.29-core199"
  license: "GPL-2.0"
  architecture: "x86_64"
templates: {}
EOF

tar czf "${WORK_DIR}/metadata.tar.gz" -C "${WORK_DIR}" metadata.yaml

echo "[4/5] Importing into LXD..."
lxc image import \
    "${WORK_DIR}/metadata.tar.gz" \
    "${IPFIRE_IMG}" \
    --alias "${ALIAS}" \
    --type vm

echo "[5/5] Verifying..."
lxc image info "${ALIAS}"
echo "✓ IPFire imported as LXD VM image: ${ALIAS}"
rm -rf "${WORK_DIR}"
