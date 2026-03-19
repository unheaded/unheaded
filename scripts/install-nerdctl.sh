#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/install-nerdctl.sh — Install nerdctl + CNI plugins for containerd
#
# Installs:
#   - nerdctl (containerd CLI) to /usr/local/bin/nerdctl
#   - CNI plugins to /opt/cni/bin/
#
# Prerequisites: containerd must already be installed and running.

set -euo pipefail

NERDCTL_VERSION="${NERDCTL_VERSION:-2.0.3}"
CNI_VERSION="${CNI_VERSION:-1.6.1}"
ARCH="$(uname -m)"
INSTALL_DIR="/usr/local/bin"
CNI_DIR="/opt/cni/bin"

log()  { printf '[%s] %s\n' "$(date '+%H:%M:%S')" "$*"; }
die()  { log "FATAL: $*"; exit 1; }

# Normalize architecture
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    *)       die "Unsupported architecture: ${ARCH}" ;;
esac

# Check prerequisites
command -v containerd >/dev/null 2>&1 || die "containerd not found — install it first"
command -v curl >/dev/null 2>&1 || die "curl not found"
command -v tar >/dev/null 2>&1 || die "tar not found"

# Check if already installed
if command -v nerdctl >/dev/null 2>&1; then
    current=$(nerdctl --version 2>/dev/null | grep -oP '[\d.]+' | head -1 || echo "unknown")
    log "nerdctl already installed (version ${current})"
    read -r -p "Reinstall? [y/N] " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        log "Keeping existing installation"
        exit 0
    fi
fi

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Install nerdctl
log "Downloading nerdctl v${NERDCTL_VERSION} (${ARCH})..."
curl -fsSL "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-${ARCH}.tar.gz" \
    -o "${TMP_DIR}/nerdctl.tar.gz"

log "Installing nerdctl to ${INSTALL_DIR}..."
sudo tar -xzf "${TMP_DIR}/nerdctl.tar.gz" -C "${INSTALL_DIR}" nerdctl
sudo chmod 755 "${INSTALL_DIR}/nerdctl"
log "nerdctl installed: $(nerdctl --version 2>/dev/null)"

# Install CNI plugins
if [[ -d "$CNI_DIR" ]] && ls "${CNI_DIR}"/*.bridge 2>/dev/null || ls "${CNI_DIR}/bridge" 2>/dev/null; then
    log "CNI plugins appear to already be installed at ${CNI_DIR}"
else
    log "Downloading CNI plugins v${CNI_VERSION} (${ARCH})..."
    curl -fsSL "https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-${ARCH}-v${CNI_VERSION}.tgz" \
        -o "${TMP_DIR}/cni-plugins.tgz"

    log "Installing CNI plugins to ${CNI_DIR}..."
    sudo mkdir -p "${CNI_DIR}"
    sudo tar -xzf "${TMP_DIR}/cni-plugins.tgz" -C "${CNI_DIR}"
    log "CNI plugins installed"
fi

# Verify
log "=== Installation Summary ==="
log "  nerdctl: $(which nerdctl) ($(nerdctl --version 2>/dev/null))"
log "  CNI:     ${CNI_DIR}/ ($(ls "${CNI_DIR}" 2>/dev/null | wc -l) plugins)"
log "  containerd: $(containerd --version 2>/dev/null)"
log ""
log "Installation complete. containerd + nerdctl ready for Operation Three Crowns."
