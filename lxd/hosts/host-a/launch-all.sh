#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# lxd/hosts/host-a/launch-all.sh
# Launch all 25 Unheaded service containers + telemetry on host-a
# Respects service dependencies and resource allocation

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
    local ebpf_profile=${4:-false}
    local gpu_profile=${5:-false}
    
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
    
    if [[ "${ebpf_profile}" == "true" ]]; then
        cmd="${cmd} --profile unheaded-ebpf"
    fi
    
    if [[ "${gpu_profile}" == "true" ]]; then
        cmd="${cmd} --profile unheaded-gpu"
    fi
    
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
log_info "Starting Unheaded Kingdom service deployment on host-a..."
echo ""

# ============================================================
# PHASE 0: INGRESS/EGRESS FIREWALL + ROUTING (must be first)
# ============================================================
log_info "Phase 0: Importing firewall VM images..."
bash "$(dirname "$0")/../../firewall/import-opnsense.sh" 2>/dev/null || log_warn "OPNsense image import failed or already imported"

log_info "Phase 0: Launching OPNsense VM (ingress/egress firewall)..."
if lxc list "unheaded-opnsense" 2>/dev/null | grep -q "unheaded-opnsense"; then
    log_warn "unheaded-opnsense already exists, skipping launch"
    LAUNCHED_OK+=("opnsense")
else
    if lxc launch opnsense-26.1.2 unheaded-opnsense \
        --vm \
        --profile unheaded-firewall \
        --config limits.cpu=4 \
        --config limits.memory=4GB \
        --config boot.autostart.priority=200 2>/dev/null; then
        log_success "OPNsense VM launched"
        LAUNCHED_OK+=("opnsense")
    else
        log_error "OPNsense VM launch failed"
        LAUNCHED_FAIL+=("opnsense")
    fi
fi

log_info "Phase 0: Waiting for OPNsense to initialize (90s)..."
sleep 90
if lxc info unheaded-opnsense 2>/dev/null | grep -E "Status|Name"; then
    log_success "OPNsense is running"
else
    log_warn "OPNsense status check failed (may still be initializing)"
fi

log_info "Phase 0: Launching FRR routing container..."
launch_container "frr" 2 1024 true false
log_success "Ingress/egress tier ready. Proceeding to service containers..."
echo ""

# Phase 1: Message bus
log_info "Phase 1: Launching message bus (wotan)..."
launch_container "wotan" 4 1024
push_and_start_service "wotan"
sleep 2

# Phase 2: Protocol layer
log_info "Phase 2: Launching protocol layer (monad, sophia, anamnesis)..."
launch_container "monad" 2 512
launch_container "sophia" 2 512
launch_container "anamnesis" 2 512
push_and_start_service "monad"
push_and_start_service "sophia"
push_and_start_service "anamnesis"
sleep 2

# Phase 3: eBPF control layer
log_info "Phase 3: Launching eBPF control (shield, unheaded-daemon)..."
launch_container "shield" 4 2048 true false
launch_container "unheaded-daemon" 4 1024 true false
push_and_start_service "shield"
push_and_start_service "unheaded-daemon"
sleep 2

# Phase 4: Routing layer
log_info "Phase 4: Launching routing (gateway, service-discovery)..."
launch_container "gateway" 2 512
launch_container "service-discovery" 1 256
push_and_start_service "gateway"
push_and_start_service "service-discovery"
sleep 2

# Phase 5: Presentation layer
log_info "Phase 5: Launching presentation (dashboard-backend, dashboard-frontend, protocol-api)..."
launch_container "dashboard-backend" 2 512
launch_container "dashboard-frontend" 1 256
launch_container "protocol-api" 2 512
push_and_start_service "dashboard-backend"
push_and_start_service "dashboard-frontend"
push_and_start_service "protocol-api"
sleep 2

# Phase 6: Observability and compute
log_info "Phase 6: Launching observability and compute (trace-collector, doom)..."
launch_container "trace-collector" 2 512
launch_container "doom" 4 1024
push_and_start_service "trace-collector"
push_and_start_service "doom"
sleep 2

# Phase 7: Service containers (batch 1)
log_info "Phase 7: Launching service batch 1 (kanban, captain, micromanager, timeguru, architect, developer)..."
launch_container "kanban" 2 512
launch_container "captain" 1 256
launch_container "micromanager" 1 256
launch_container "timeguru" 1 256
launch_container "architect" 1 256
launch_container "developer" 1 256

for svc in kanban captain micromanager timeguru architect developer; do
    push_and_start_service "${svc}"
done
sleep 2

# Phase 8: Service containers (batch 2)
log_info "Phase 8: Launching service batch 2 (lore, busboy, kingdom, blackmage, moatghost)..."
launch_container "lore" 1 256
launch_container "busboy" 1 256
launch_container "kingdom" 1 256
launch_container "blackmage" 1 256
launch_container "moatghost" 1 256

for svc in lore busboy kingdom blackmage moatghost; do
    push_and_start_service "${svc}"
done
sleep 2

# Phase 9: Adversary (last)
log_info "Phase 9: Launching adversary (lich - depends on all services)..."
launch_container "lich" 4 2048
push_and_start_service "lich"
sleep 2

# Phase 10: Telemetry stack
log_info "Phase 10: Launching telemetry (prometheus, victoriametrics, loki, grafana, ebpf-exporter)..."
launch_container "prometheus" 2 512
launch_container "victoriametrics" 2 512
launch_container "loki" 2 512
launch_container "grafana" 1 256
launch_container "ebpf-exporter" 1 256

for svc in prometheus victoriametrics loki grafana ebpf-exporter; do
    push_and_start_service "${svc}"
done
sleep 2

# Print final status
print_status_table

# Exit status
if [[ ${#LAUNCHED_FAIL[@]} -eq 0 ]]; then
    echo ""
    log_success "All services launched successfully!"
    exit 0
else
    echo ""
    log_warn "Some services failed to launch. Review errors above."
    exit 1
fi
