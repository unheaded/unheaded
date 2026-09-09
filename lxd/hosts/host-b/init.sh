#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# lxd/hosts/host-b/init.sh
# LXD host-b (Outpost) bootstrap script - minimal configuration
# Usage: sudo ./init.sh
# Run once on a fresh Ubuntu 24.04 / Debian 12 consumer-grade host

set -euo pipefail

# ANSI color codes
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly NC='\033[0m' # No Color

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
readonly REPO_ROOT
readonly BIN_DIR="${REPO_ROOT}/bin"
readonly HOST_BIN_DIR="/opt/unheaded/bin"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*" >&2
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root"
    exit 1
fi

log_info "Starting LXD host-b (Outpost) bootstrap..."

# Install LXD via snap if not installed
if ! command -v lxd &> /dev/null; then
    log_info "Installing LXD 5.21 via snap..."
    snap install lxd --channel=5.21/stable
    log_success "LXD installed"
else
    log_info "LXD already installed: $(lxd --version)"
fi

# Add current user to lxd group (if not already)
if [[ -n "${SUDO_USER:-}" ]]; then
    log_info "Adding ${SUDO_USER} to lxd group..."
    usermod -aG lxd "${SUDO_USER}" || log_warn "Failed to add user to lxd group"
fi

# Initialize LXD with preseed
log_info "Initializing LXD with preseed configuration..."
if [[ -f "${SCRIPT_DIR}/preseed.yaml" ]]; then
    lxd init --preseed < "${SCRIPT_DIR}/preseed.yaml" || log_warn "LXD preseed failed (may already be initialized)"
    log_success "LXD initialization complete"
else
    log_error "preseed.yaml not found at ${SCRIPT_DIR}/preseed.yaml"
    exit 1
fi

# Verify network was created
log_info "Verifying unheaded-outpost network..."
if ! lxc network ls | grep -q "unheaded-outpost"; then
    log_warn "Network 'unheaded-outpost' not found in preseed output, checking LXD state..."
fi

# Verify storage pool was created
log_info "Verifying unheaded-minimal storage pool..."
if ! lxc storage ls | grep -q "unheaded-minimal"; then
    log_warn "Storage pool 'unheaded-minimal' not found in preseed output, checking LXD state..."
fi

# Create host binary directory structure
log_info "Creating host binary directory structure..."
mkdir -p "${HOST_BIN_DIR}"
chmod 755 "${HOST_BIN_DIR}"
log_success "Host binary directory created at ${HOST_BIN_DIR}"

# Copy binaries from repo if they exist
if [[ -d "${BIN_DIR}" ]] && [[ -n "$(ls -A "${BIN_DIR}" 2>/dev/null || true)" ]]; then
    log_info "Copying service binaries from ${BIN_DIR}..."
    cp -v "${BIN_DIR}"/* "${HOST_BIN_DIR}/" 2>/dev/null || log_warn "No binaries to copy"
    chmod +x "${HOST_BIN_DIR}"/*
    log_success "Binaries copied and made executable"
else
    log_warn "No binaries found at ${BIN_DIR} - skipping binary copy"
fi

# Create LXD profiles if they don't exist
log_info "Applying LXD profiles..."

# Check if profiles directory exists
PROFILES_DIR="${REPO_ROOT}/lxd/profiles"
if [[ -d "${PROFILES_DIR}" ]]; then
    for profile_file in "${PROFILES_DIR}"/*.yaml; do
        if [[ -f "${profile_file}" ]]; then
            profile_name=$(basename "${profile_file}" .yaml)
            log_info "Checking profile: ${profile_name}"
            if lxc profile show "${profile_name}" &>/dev/null; then
                log_info "Profile '${profile_name}' already exists"
            else
                log_info "Creating profile '${profile_name}' from ${profile_file}..."
                lxc profile create "${profile_name}"
                lxc profile edit "${profile_name}" < "${profile_file}"
                log_success "Profile '${profile_name}' created"
            fi
        fi
    done
else
    log_warn "Profiles directory not found at ${PROFILES_DIR}"
fi

# Setup WireGuard connection to host-a (if configuration exists)
WIREGUARD_CONFIG="${SCRIPT_DIR}/wireguard.conf"
if [[ -f "${WIREGUARD_CONFIG}" ]]; then
    log_info "Configuring WireGuard tunnel to host-a..."
    
    # Install wireguard tools if not present
    if ! command -v wg &> /dev/null; then
        log_info "Installing wireguard-tools..."
        apt-get update && apt-get install -y wireguard-tools 2>/dev/null || log_warn "Failed to install wireguard-tools"
    fi
    
    # Copy WireGuard config
    if [[ ! -d /etc/wireguard ]]; then
        mkdir -p /etc/wireguard
        chmod 700 /etc/wireguard
    fi
    
    cp "${WIREGUARD_CONFIG}" /etc/wireguard/wg0.conf
    chmod 600 /etc/wireguard/wg0.conf
    
    # Bring up WireGuard interface
    if ip link show wg0 &>/dev/null; then
        log_info "WireGuard interface wg0 already exists"
    else
        log_info "Bringing up WireGuard interface wg0..."
        wg-quick up wg0 || log_warn "Failed to bring up WireGuard interface"
    fi
    
    log_success "WireGuard configuration complete"
else
    log_warn "WireGuard configuration not found at ${WIREGUARD_CONFIG}"
fi

# Print success summary
echo ""
log_success "LXD host-b (Outpost) bootstrap complete!"
echo ""
echo "Next steps:"
echo "  1. Run 'lxc network ls' to verify the 'unheaded-outpost' network"
echo "  2. Run 'lxc storage ls' to verify the 'unheaded-minimal' storage pool"
echo "  3. Run 'lxc profile ls' to verify profiles are loaded"
echo "  4. Review host binary directory: ls -la ${HOST_BIN_DIR}/"
echo "  5. Verify WireGuard tunnel: wg show wg0"
echo "  6. Run './launch-minimal.sh' to launch core services"
echo ""
log_info "LXD API is listening on [::]:8443"

