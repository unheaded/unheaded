#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# lxd/deploy.sh
# Unheaded Kingdom LXD deployment helper

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILES_DIR="${SCRIPT_DIR}/profiles"
CLOUD_INIT_DIR="${SCRIPT_DIR}/cloud-init"
NETWORKS_DIR="${SCRIPT_DIR}/networks"
STORAGE_DIR="${SCRIPT_DIR}/storage"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Check LXD is installed and running
check_lxd() {
    if ! command -v lxc &> /dev/null; then
        log_error "LXD/LXC not found. Please install LXD."
        exit 1
    fi
    
    if ! lxc info &> /dev/null; then
        log_error "LXD daemon not responding. Is it running?"
        exit 1
    fi
    
    log_info "LXD daemon ready"
}

# Initialize storage pool
init_storage() {
    log_info "Initializing storage pool..."
    
    if lxc storage list | grep -q unheaded-ssd; then
        log_warn "Storage pool 'unheaded-ssd' already exists"
        return
    fi
    
    # Check for existing ZFS pool
    if zfs list unheaded &> /dev/null; then
        log_info "Using existing ZFS pool 'unheaded'"
        lxc storage create unheaded-ssd zfs source=unheaded || true
    else
        log_info "Creating new ZFS pool 'unheaded'"
        lxc storage create unheaded-ssd zfs || {
            log_warn "ZFS pool creation failed. Attempting with loop device..."
            lxc storage create unheaded-ssd dir path=/var/lib/lxd/storage-pools/unheaded-ssd || true
        }
    fi
}

# Initialize network bridge
init_network() {
    log_info "Initializing network bridge..."
    
    if lxc network list | grep -q unheaded; then
        log_warn "Network 'unheaded' already exists"
        return
    fi
    
    lxc network create unheaded \
        ipv4.address=10.20.0.1/16 \
        ipv4.dhcp=true \
        ipv4.dhcp.ranges=10.20.1.0-10.20.1.250 \
        ipv4.firewall=true \
        ipv4.nat=true \
        ipv6.address=fd00:dead:beef:1::1/64 \
        ipv6.dhcp=true \
        ipv6.dhcp.stateful=true \
        ipv6.firewall=true \
        ipv6.nat=false \
        dns.domain=unheaded.internal \
        dns.mode=managed
    
    log_info "Network 'unheaded' created"
}

# Create profiles
init_profiles() {
    log_info "Creating LXD profiles..."
    
    for profile_file in "${PROFILES_DIR}"/*.yaml; do
        profile_name=$(basename "$profile_file" .yaml)
        
        if lxc profile list | grep -q "^${profile_name}$"; then
            log_warn "Profile '${profile_name}' already exists"
            continue
        fi
        
        log_info "Creating profile: ${profile_name}"
        lxc profile create "${profile_name}"
        cat "${profile_file}" | lxc profile edit "${profile_name}"
    done
}

# List all created resources
list_resources() {
    log_info "=== Storage Pools ==="
    lxc storage list || true
    
    log_info "=== Networks ==="
    lxc network list || true
    
    log_info "=== Profiles ==="
    lxc profile list || true
}

# Usage
usage() {
    cat << 'USAGE'
Unheaded Kingdom LXD Deployment Helper

Usage: deploy.sh [COMMAND]

Commands:
  check         Check LXD is installed and running
  storage       Initialize storage pool (unheaded-ssd)
  network       Initialize network bridge (unheaded)
  profiles      Create all profiles (unheaded-base, -service, -ebpf, -gpu, -telemetry)
  init          Run: check, storage, network, profiles (full initialization)
  list          List all created resources (storage, networks, profiles)
  help          Show this help message

Examples:
  # Full initialization
  ./deploy.sh init
  
  # Just create profiles
  ./deploy.sh profiles
  
  # List what was created
  ./deploy.sh list
USAGE
}

main() {
    local command="${1:-help}"
    
    case "${command}" in
        check)
            check_lxd
            ;;
        storage)
            check_lxd
            init_storage
            ;;
        network)
            check_lxd
            init_network
            ;;
        profiles)
            check_lxd
            init_profiles
            ;;
        init)
            check_lxd
            init_storage
            init_network
            init_profiles
            log_info "Initialization complete!"
            list_resources
            ;;
        list)
            check_lxd
            list_resources
            ;;
        help|--help|-h)
            usage
            ;;
        *)
            log_error "Unknown command: ${command}"
            usage
            exit 1
            ;;
    esac
}

main "$@"
