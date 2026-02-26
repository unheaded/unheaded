#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# import-opnsense.sh — Import OPNsense 26.1.2 into LXD as VM image
#
# OPNsense serial-amd64 img is a raw FreeBSD disk image.
# LXD VM images require: raw disk image + metadata.yaml tarball
#
set -euo pipefail

OPNSENSE_BZ2="${HOME}/tmp/opnsense/OPNsense-26.1.2-serial-amd64.img.bz2"
OPNSENSE_IMG="${HOME}/tmp/opnsense/OPNsense-26.1.2-serial-amd64.img"
WORK_DIR="$(mktemp -d)"
ALIAS="opnsense-26.1.2"

echo "[1/5] Checking source image..."
if [[ ! -f "$OPNSENSE_BZ2" ]]; then
    echo "ERROR: OPNsense image not found at $OPNSENSE_BZ2"
    exit 1
fi

echo "[2/5] Decompressing OPNsense image (bz2 → raw)..."
if [[ ! -f "$OPNSENSE_IMG" ]]; then
    pbzip2 -dk "$OPNSENSE_BZ2"
    echo "Decompressed to: $OPNSENSE_IMG"
else
    echo "Already decompressed: $OPNSENSE_IMG"
fi

echo "[3/5] Creating LXD image metadata..."
cat > "${WORK_DIR}/metadata.yaml" << 'EOF'
architecture: x86_64
creation_date: 1740000000
properties:
  description: "OPNsense 26.1.2 (FreeBSD, serial console)"
  os: "opnsense"
  release: "26.1.2"
  variant: "serial-amd64"
  license: "BSD-2-Clause"
  architecture: "x86_64"
templates: {}
EOF

tar czf "${WORK_DIR}/metadata.tar.gz" -C "${WORK_DIR}" metadata.yaml

echo "[4/5] Importing into LXD (this may take a minute)..."
lxc image import \
    "${WORK_DIR}/metadata.tar.gz" \
    "${OPNSENSE_IMG}" \
    --alias "${ALIAS}" \
    --type vm

echo "[5/5] Verifying import..."
lxc image info "${ALIAS}"
echo ""
echo "✓ OPNsense imported as LXD VM image: ${ALIAS}"
echo "  Launch with: lxc launch ${ALIAS} unheaded-opnsense --vm --profile unheaded-firewall"
echo ""
rm -rf "${WORK_DIR}"
