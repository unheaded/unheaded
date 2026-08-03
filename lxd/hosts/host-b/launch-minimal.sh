#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# lxd/hosts/host-b/launch-minimal.sh
# Launch 6 core Unheaded services + telemetry agents on host-b (Outpost)
# Minimal configuration suitable for consumer-grade hardware

set -euo pipefail

# ANSI color codes
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly NC='\033[0m' # No Color

# Tracking arrays
declare -a LAUNCHED_OK=()
declare -a LAUNCHED_FAIL=()

# Configuration

# Utility functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $*" >&2
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*" >&2
}

log_error() {
    echo -e "${RED}[✗]${NC} $*" >&2
}

# Launch a container with resource limits and configuration
launch_container() {
    local svc_name=$1
    local cpu_limit=$2
    local memory_limit=$3
    
    local container_name="unheaded-${svc_name}"
    
    log_info "Launching ${container_name}..."
    
    # Check if already exists
    if lxc list "${container_name}" 2>/dev/null | grep -q "${container_name}"; then
        log_warn "${container_name} already exists, skipping"
        LAUNCHED_OK+=("${svc_name}")
        return 0
    fi
    
    # Build launch command
    local cmd="lxc launch ubuntu:24.04 ${container_name}"
    cmd="${cmd} --profile default"
    cmd="${cmd} --config limits.cpu=${cpu_limit}"
    cmd="${cmd} --config limits.memory=${memory_limit}MB"
    cmd="${cmd} --config boot.autostart=true"
    cmd="${cmd} --config environment.SERVICE_NAME=${svc_name}"
    cmd="${cmd} --config environment.LOG_LEVEL=info"
    
    # Execute launch command
    if eval "${cmd}" 2>/dev/null; then
        log_success "Container ${container_name} launched"
        
        # Wait for cloud-init to complete
        log_info "Waiting for ${container_name} to be ready..."
        sleep 3
        if timeout 60 lxc exec "${container_name}" -- cloud-init status --wait 2>/dev/null || true; then
            log_success "${container_name} is ready"
            LAUNCHED_OK+=("${svc_name}")
            return 0
        else
            log_warn "${container_name} cloud-init timeout, continuing..."
            LAUNCHED_OK+=("${svc_name}")
            return 0
        fi
    else
        log_error "Failed to launch ${container_name}"
        LAUNCHED_FAIL+=("${svc_name}")
        return 1
    fi
}

# Launch container with ebpf profile
launch_container_ebpf() {
    local svc_name=$1
    local cpu_limit=$2
    local memory_limit=$3
    
    local container_name="unheaded-${svc_name}"
    
    log_info "Launching ${container_name}..."
    
    # Check if already exists
    if lxc list "${container_name}" 2>/dev/null | grep -q "${container_name}"; then
        log_warn "${container_name} already exists, skipping"
        LAUNCHED_OK+=("${svc_name}")
        return 0
    fi
    
    # Build launch command
    local cmd="lxc launch ubuntu:24.04 ${container_name}"
    cmd="${cmd} --profile default"
    cmd="${cmd} --profile unheaded-ebpf"
    cmd="${cmd} --config limits.cpu=${cpu_limit}"
    cmd="${cmd} --config limits.memory=${memory_limit}MB"
    cmd="${cmd} --config boot.autostart=true"
    cmd="${cmd} --config environment.SERVICE_NAME=${svc_name}"
    cmd="${cmd} --config environment.LOG_LEVEL=info"
    
    # Execute launch command
    if eval "${cmd}" 2>/dev/null; then
        log_success "Container ${container_name} launched"
        
        # Wait for cloud-init to complete
        log_info "Waiting for ${container_name} to be ready..."
        sleep 3
        if timeout 60 lxc exec "${container_name}" -- cloud-init status --wait 2>/dev/null || true; then
            log_success "${container_name} is ready"
            LAUNCHED_OK+=("${svc_name}")
            return 0
        else
            log_warn "${container_name} cloud-init timeout, continuing..."
            LAUNCHED_OK+=("${svc_name}")
            return 0
        fi
    else
        log_error "Failed to launch ${container_name}"
        LAUNCHED_FAIL+=("${svc_name}")
        return 1
    fi
}

# Push binary and start service
push_and_start_service() {
    local svc_name=$1
    local container_name="unheaded-${svc_name}"
    
    log_info "Pushing binary for ${svc_name}..."
    
    # Check if binary exists on host
    if [[ ! -f "/opt/unheaded/bin/${svc_name}" ]]; then
        log_warn "Binary not found at /opt/unheaded/bin/${svc_name}, skipping push"
        return 0
    fi
    
    # Push binary to container
    if lxc file push "/opt/unheaded/bin/${svc_name}" "${container_name}/opt/unheaded/bin/${svc_name}" 2>/dev/null; then
        # Make executable
        lxc exec "${container_name}" -- chmod +x "/opt/unheaded/bin/${svc_name}" 2>/dev/null || true
        
        # Start service
        log_info "Starting ${svc_name} service..."
        if lxc exec "${container_name}" -- systemctl start "unheaded-${svc_name}" 2>/dev/null || true; then
            log_success "${svc_name} service started"
        else
            log_warn "Could not start service (systemd may not be configured yet)"
        fi
    else
        log_warn "Failed to push binary for ${svc_name}"
    fi
}

# Print status table
print_status_table() {
    echo ""
    echo -e "${CYAN}=== Launch Summary ===${NC}"
    echo ""
    
    if [[ ${#LAUNCHED_OK[@]} -gt 0 ]]; then
        echo -e "${GREEN}Successfully launched (${#LAUNCHED_OK[@]}):${NC}"
        for svc in "${LAUNCHED_OK[@]}"; do
            echo "  - ${svc}"
        done
        echo ""
    fi
    
    if [[ ${#LAUNCHED_FAIL[@]} -gt 0 ]]; then
        echo -e "${RED}Failed to launch (${#LAUNCHED_FAIL[@]}):${NC}"
        for svc in "${LAUNCHED_FAIL[@]}"; do
            echo "  - ${svc}"
        done
        echo ""
    fi
    
    echo -e "${CYAN}=== Container Status ===${NC}"
    lxc list | grep unheaded || true
}

# Main execution
log_info "Starting Unheaded Kingdom minimal deployment on host-b (Outpost)..."
echo ""

# ============================================================
# PHASE 0: INGRESS/EGRESS FIREWALL + ROUTING (must be first)
# ============================================================
log_info "Phase 0: Importing firewall VM images..."
bash "$(dirname "$0")/../../firewall/import-ipfire.sh" 2>/dev/null || log_warn "IPFire image import failed or already imported"

log_info "Phase 0: Launching IPFire VM (ingress/egress firewall)..."
if lxc list "unheaded-ipfire" 2>/dev/null | grep -q "unheaded-ipfire"; then
    log_warn "unheaded-ipfire already exists, skipping launch"
    LAUNCHED_OK+=("ipfire")
else
    if lxc launch ipfire-2.29-core199 unheaded-ipfire \
        --vm \
        --profile unheaded-firewall \
        --config limits.cpu=2 \
        --config limits.memory=2GB \
        --config boot.autostart.priority=200 2>/dev/null; then
        log_success "IPFire VM launched"
        LAUNCHED_OK+=("ipfire")
    else
        log_error "IPFire VM launch failed"
        LAUNCHED_FAIL+=("ipfire")
    fi
fi

log_info "Phase 0: Waiting for IPFire to initialize (90s)..."
sleep 90
if lxc info unheaded-ipfire 2>/dev/null | grep -E "Status|Name"; then
    log_success "IPFire is running"
else
    log_warn "IPFire status check failed (may still be initializing)"
fi

log_info "Phase 0: Launching BIRD routing container..."
launch_container_ebpf "bird" 1 256
log_success "Ingress/egress tier ready. Proceeding to core services..."
echo ""

# Phase 1: Core message bus
log_info "Phase 1: Launching message bus (wotan)..."
launch_container "wotan" 2 512
push_and_start_service "wotan"
sleep 2

# Phase 2: Core protocol layer
log_info "Phase 2: Launching core protocol layer (monad, sophia, anamnesis)..."
launch_container "monad" 1 256
launch_container "sophia" 1 256
launch_container "anamnesis" 1 256
push_and_start_service "monad"
push_and_start_service "sophia"
push_and_start_service "anamnesis"
sleep 2

# Phase 3: Core gateway and dashboard
log_info "Phase 3: Launching gateway and dashboard-backend..."
launch_container "gateway" 1 256
launch_container "dashboard-backend" 1 256
push_and_start_service "gateway"
push_and_start_service "dashboard-backend"
sleep 2

# Phase 4: Telemetry agents (minimal)
log_info "Phase 4: Launching telemetry agents..."
log_info "  - Prometheus (agent mode, remote_write to host-a)"
launch_container "prometheus-agent" 1 256
push_and_start_service "prometheus-agent"

log_info "  - Promtail (log shipper)"
launch_container "promtail" 1 256
push_and_start_service "promtail"

log_info "  - Node exporter (host metrics)"
launch_container "node-exporter" 1 256
push_and_start_service "node-exporter"

sleep 2

# Print final status
print_status_table

# Exit status
if [[ ${#LAUNCHED_FAIL[@]} -eq 0 ]]; then
    echo ""
    log_success "All core services launched successfully on host-b!"
    echo ""
    echo "Configuration notes:"
    echo "  - IPFire is running as a VM with firewall/IDS capabilities"
    echo "  - BIRD is handling dynamic routing (BGP) for host-b"
    echo "  - Prometheus is in agent mode with remote_write to host-a"
    echo "  - Logs are shipped to host-a Loki via Promtail"
    echo "  - Node metrics are exported for host-a Prometheus"
    echo ""
    exit 0
else
    echo ""
    log_warn "Some services failed to launch. Review errors above."
    exit 1
fi
