#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# pre-flight-check.sh — Pre-deployment prerequisite checks
# Usage: sudo ./pre-flight-check.sh [--strict]
# Checks all prerequisites before starting bare metal deployment
# Must be run BEFORE nixos-rebuild switch

set -euo pipefail
IFS=$'\n\t'

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TIMESTAMP=$(date +%Y-%m-%d_%H:%M:%S)
MIN_KERNEL_MAJOR=6
MIN_KERNEL_MINOR=0
MIN_DISK_GB=40
MIN_RAM_GB=8
UNHEADED_SRC="${UNHEADED_SRC:-/opt/unheaded}"
# shellcheck disable=SC2034  # parsed and advertised, but nothing reads it yet.
# --strict is in the usage line at the top of this file and sets this in the
# arg-parsing case, so the flag is accepted and silently changes nothing.
# Same class as --verbose in tomb/provision.sh. The bug is the unimplemented
# behaviour, not the assignment, so this is flagged rather than deleted.
STRICT_MODE=false
HOSTNAME_DETECTED=$(hostname)

# Track results
declare -A check_results
required_failed=0
optional_failed=0

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --strict)
            shift
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Helper functions
log_pass() {
    local name=$1
    local value=${2:-}
    echo -e "${GREEN}[PASS]${NC} $name"
    if [[ -n "$value" ]]; then
        echo "        $value"
    fi
    check_results["$name"]="PASS"
}

log_fail() {
    local name=$1
    local value=${2:-}
    echo -e "${RED}[FAIL]${NC} $name"
    if [[ -n "$value" ]]; then
        echo "        $value"
    fi
    check_results["$name"]="FAIL"
}

log_warn() {
    local name=$1
    local value=${2:-}
    echo -e "${YELLOW}[WARN]${NC} $name"
    if [[ -n "$value" ]]; then
        echo "        $value"
    fi
    check_results["$name"]="WARN"
}

log_section() {
    echo ""
    echo -e "${BLUE}=== $1 ===${NC}"
}

# ===== REQUIRED CHECKS (Kernel & OS) =====
log_section "REQUIRED: Kernel & OS Compatibility"

# Kernel version check
kernel_version=$(uname -r)
kernel_major=$(echo "$kernel_version" | cut -d. -f1)
kernel_minor=$(echo "$kernel_version" | cut -d. -f2)

if [[ $kernel_major -gt $MIN_KERNEL_MAJOR ]] || [[ $kernel_major -eq $MIN_KERNEL_MAJOR && $kernel_minor -ge $MIN_KERNEL_MINOR ]]; then
    log_pass "Kernel Version >= ${MIN_KERNEL_MAJOR}.${MIN_KERNEL_MINOR}" "$kernel_version"
else
    log_fail "Kernel Version >= ${MIN_KERNEL_MAJOR}.${MIN_KERNEL_MINOR}" "Current: $kernel_version (required for eBPF features)"
    ((required_failed++))
fi

# NixOS check
if command -v nixos-version &> /dev/null; then
    nixos_ver=$(nixos-version 2>/dev/null | head -1)
    log_pass "NixOS OS" "$nixos_ver"
else
    log_fail "NixOS OS" "Not running NixOS"
    ((required_failed++))
fi

# ===== REQUIRED CHECKS (eBPF Capabilities) =====
log_section "REQUIRED: eBPF & Kernel Features"

# BTF check
if [[ -f /sys/kernel/btf/vmlinux ]]; then
    log_pass "BTF vmlinux (CO-RE)" "Available"
else
    log_fail "BTF vmlinux (CO-RE)" "Required for eBPF programs"
    ((required_failed++))
fi

# Cgroup v2 check
if grep -q cgroup2 /proc/filesystems 2>/dev/null; then
    log_pass "Cgroup v2 Unified Hierarchy" "Supported"
else
    log_fail "Cgroup v2 Unified Hierarchy" "Required for unified cgroup control"
    ((required_failed++))
fi

# Cgroup subtree_control check
if [[ -f /sys/fs/cgroup/cgroup.subtree_control ]]; then
    log_pass "Cgroup v2 Delegation" "Available"
else
    log_fail "Cgroup v2 Delegation" "systemd cgroup delegation required"
    ((required_failed++))
fi

# IPv6 check
if ip -6 addr show 2>/dev/null | grep -q "inet6"; then
    log_pass "IPv6 Support" "Enabled"
else
    log_fail "IPv6 Support" "IPv6 must be enabled"
    ((required_failed++))
fi

# IPv6 loopback connectivity
if ping6 -c 1 -W 2 ::1 &> /dev/null; then
    log_pass "IPv6 Loopback Connectivity" "OK"
else
    log_warn "IPv6 Loopback Connectivity" "Cannot reach ::1 (may need configuration)"
fi

# ===== REQUIRED CHECKS (System Resources) =====
log_section "REQUIRED: System Resources"

# Available disk space
root_available_kb=$(df / 2>/dev/null | tail -1 | awk '{print $4}')
root_available_gb=$((root_available_kb / 1024 / 1024))

if [[ $root_available_gb -ge $MIN_DISK_GB ]]; then
    log_pass "Disk Space (/ partition)" "${root_available_gb}GB available (minimum: ${MIN_DISK_GB}GB)"
else
    log_fail "Disk Space (/ partition)" "${root_available_gb}GB available (minimum: ${MIN_DISK_GB}GB required)"
    ((required_failed++))
fi

# Total RAM
mem_total_kb=$(grep "^MemTotal:" /proc/meminfo | awk '{print $2}')
mem_total_gb=$((mem_total_kb / 1024 / 1024))

if [[ $mem_total_gb -ge $MIN_RAM_GB ]]; then
    log_pass "Total RAM" "${mem_total_gb}GB (minimum: ${MIN_RAM_GB}GB)"
else
    log_fail "Total RAM" "${mem_total_gb}GB (minimum: ${MIN_RAM_GB}GB required)"
    ((required_failed++))
fi

# ===== REQUIRED CHECKS (Software & Git) =====
log_section "REQUIRED: Required Tools & Configuration"

# Git availability
if command -v git &> /dev/null; then
    git_ver=$(git --version)
    log_pass "Git" "$git_ver"
else
    log_fail "Git" "Git is not installed"
    ((required_failed++))
fi

# Unheaded repo check
if [[ -d "$UNHEADED_SRC" && -d "$UNHEADED_SRC/.git" ]]; then
    repo_branch=$(cd "$UNHEADED_SRC" && git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
    repo_hash=$(cd "$UNHEADED_SRC" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    log_pass "Unheaded Repository" "$UNHEADED_SRC (branch: $repo_branch, commit: $repo_hash)"
else
    log_fail "Unheaded Repository" "Not found at $UNHEADED_SRC"
    ((required_failed++))
fi

# SSH key for unheaded user
unheaded_user=$(id -u unheaded 2>/dev/null || echo "")
if [[ -n "$unheaded_user" ]]; then
    if [[ -f /home/unheaded/.ssh/id_ed25519 ]] || [[ -f /home/unheaded/.ssh/id_rsa ]]; then
        log_pass "Unheaded User SSH Key" "Found"
    else
        log_fail "Unheaded User SSH Key" "SSH key not found for unheaded user"
        ((required_failed++))
    fi
else
    log_fail "Unheaded User Account" "unheaded user does not exist"
    ((required_failed++))
fi

# ===== OPTIONAL CHECKS (GPU & Host-specific) =====
log_section "OPTIONAL: Host-Specific Capabilities"

# Auto-detect host role
if [[ "$HOSTNAME_DETECTED" =~ forge|host-a ]]; then
    host_role="forge"
elif [[ "$HOSTNAME_DETECTED" =~ outpost|host-b ]]; then
    host_role="outpost"
else
    host_role="unknown"
fi

echo "Detected host role: $host_role"

if [[ "$host_role" == "forge" ]]; then
    echo "Running Host-A (Forge) specific checks..."
    
    # AMD GPU detection
    if command -v lspci &> /dev/null; then
        if lspci | grep -qi "amd.*vga\|ati.*vga"; then
            gpu_info=$(lspci | grep -i "amd.*vga\|ati.*vga")
            log_pass "AMD GPU Detected" "$gpu_info"
        else
            log_warn "AMD GPU Detected" "No AMD GPU found (optional for forge)"
            ((optional_failed++))
        fi
    else
        log_warn "lspci Tool" "Not available for GPU detection"
    fi
    
    # AMDGPU kernel module
    if lsmod | grep -q amdgpu; then
        log_pass "AMDGPU Kernel Module" "Loaded"
    else
        log_warn "AMDGPU Kernel Module" "Not loaded (optional if GPU present)"
    fi
    
    # ROCm check
    if command -v rocm-smi &> /dev/null; then
        rocm_ver=$(rocm-smi --version 2>/dev/null || echo "unknown")
        log_pass "ROCm Installation" "$rocm_ver"
    else
        log_warn "ROCm Installation" "ROCm not found (optional for GPGPU compute)"
        ((optional_failed++))
    fi
else
    echo "Skipping Host-A specific checks (host is not forge)"
fi

# ===== OPTIONAL CHECKS (Networking) =====
log_section "OPTIONAL: Network Configuration"

# Network interface count
iface_count=$(ip -j link show 2>/dev/null | jq 'length' 2>/dev/null || echo "0")
log_pass "Network Interfaces" "Found $iface_count interface(s)"

# WireGuard check
if command -v wg &> /dev/null; then
    log_pass "WireGuard Tool" "Installed"
else
    log_warn "WireGuard Tool" "Not installed (will be configured by Nix)"
fi

# ===== Generate JSON Report =====
log_section "Generating JSON Report"

mkdir -p /var/lib/unheaded
json_report="/var/lib/unheaded/preflight-${TIMESTAMP}.json"

cat > "$json_report" << JSON_EOF
{
  "metadata": {
    "timestamp": "$TIMESTAMP",
    "capture_date_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "hostname": "$HOSTNAME_DETECTED",
    "host_role": "$host_role"
  },
  "system": {
    "kernel_version": "$kernel_version",
    "kernel_major": $kernel_major,
    "kernel_minor": $kernel_minor,
    "total_ram_gb": $mem_total_gb,
    "root_available_gb": $root_available_gb
  },
  "summary": {
    "required_checks_passed": $(($(echo "${#check_results[@]}") - required_failed)),
    "required_checks_failed": $required_failed,
    "optional_checks_failed": $optional_failed,
    "ready_for_deployment": $(if [[ $required_failed -eq 0 ]]; then echo "true"; else echo "false"; fi)
  }
}
JSON_EOF

log_pass "JSON Report" "$json_report"

# ===== Final Summary =====
log_section "Pre-flight Check Summary"

passed_checks=0
failed_checks=0
warn_checks=0

for result in "${check_results[@]}"; do
    case "$result" in
        PASS) ((passed_checks++)) ;;
        FAIL) ((failed_checks++)) ;;
        WARN) ((warn_checks++)) ;;
    esac
done

echo ""
echo "Results:"
echo "  Passed:  $passed_checks"
echo "  Failed:  $failed_checks (required)"
echo "  Warned:  $warn_checks (optional)"
echo ""

if [[ $required_failed -eq 0 ]]; then
    echo -e "${GREEN}Status: READY FOR DEPLOYMENT${NC}"
    echo "All required checks passed. You may proceed with nixos-rebuild switch."
    exit 0
else
    echo -e "${RED}Status: NOT READY FOR DEPLOYMENT${NC}"
    echo "Fix the $required_failed failed required check(s) before proceeding."
    exit 1
fi
