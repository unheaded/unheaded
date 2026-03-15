#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# capture-baseline.sh — Capture system baseline before Unheaded deployment
# Usage: sudo ./capture-baseline.sh [--host-role forge|outpost] [--output-dir DIR]
# Produces: JSON + text snapshots of CPU, RAM, disk, net, kernel, eBPF capabilities

set -euo pipefail
IFS=$'\n\t'

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
HOST_ROLE=""
OUTPUT_DIR=""
TIMESTAMP=$(date +%Y-%m-%d_%H:%M:%S)
HOSTNAME_DETECTED=$(hostname)

# Parse command-line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --host-role)
            HOST_ROLE="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Auto-detect host role from hostname if not specified
if [[ -z "$HOST_ROLE" ]]; then
    if [[ "$HOSTNAME_DETECTED" =~ forge|host-a ]]; then
        HOST_ROLE="forge"
    elif [[ "$HOSTNAME_DETECTED" =~ outpost|host-b ]]; then
        HOST_ROLE="outpost"
    else
        HOST_ROLE="unknown"
    fi
fi

# Set output directory
if [[ -z "$OUTPUT_DIR" ]]; then
    OUTPUT_DIR="/var/lib/unheaded/baselines/${TIMESTAMP}"
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Initialize JSON object
declare -A json_data
json_data[timestamp]="$TIMESTAMP"
json_data[hostname]="$HOSTNAME_DETECTED"
json_data[host_role]="$HOST_ROLE"
json_data[capture_date]="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Helper functions
log_section() {
    echo -e "${BLUE}=== $1 ===${NC}" | tee -a "$OUTPUT_DIR/baseline.txt"
}

log_check() {
    local name=$1
    local status=$2
    local value=${3:-}
    
    if [[ "$status" == "PASS" ]]; then
        echo -e "${GREEN}[PASS]${NC} $name" | tee -a "$OUTPUT_DIR/baseline.txt"
    else
        echo -e "${RED}[FAIL]${NC} $name" | tee -a "$OUTPUT_DIR/baseline.txt"
    fi
    
    if [[ -n "$value" ]]; then
        echo "        $value" | tee -a "$OUTPUT_DIR/baseline.txt"
    fi
}

# ===== CPU Information =====
log_section "CPU Information"
cp /proc/cpuinfo "$OUTPUT_DIR/cpuinfo.txt"

cpu_count=$(grep -c "^processor" /proc/cpuinfo || echo "0")
cpu_model=$(grep "^model name" /proc/cpuinfo | head -1 | cut -d: -f2 | xargs)
cpu_freq=$(grep "cpu MHz" /proc/cpuinfo | head -1 | cut -d: -f2 | xargs)

log_check "CPU Count" "PASS" "Cores: $cpu_count"
log_check "CPU Model" "PASS" "$cpu_model"
log_check "CPU Frequency" "PASS" "$cpu_freq MHz"

# ===== Memory Information =====
log_section "Memory Information"
cp /proc/meminfo "$OUTPUT_DIR/meminfo.txt"

mem_total=$(grep "^MemTotal:" /proc/meminfo | awk '{print $2}')
mem_available=$(grep "^MemAvailable:" /proc/meminfo | awk '{print $2}')
mem_total_gb=$((mem_total / 1024 / 1024))
mem_available_gb=$((mem_available / 1024 / 1024))

log_check "Total Memory" "PASS" "${mem_total_gb}GB (${mem_total}kB)"
log_check "Available Memory" "PASS" "${mem_available_gb}GB (${mem_available}kB)"

# ===== Load Average =====
log_section "Load Average"
cp /proc/loadavg "$OUTPUT_DIR/loadavg.txt"
loadavg=$(cat /proc/loadavg | awk '{print $1, $2, $3}')
log_check "Load Average (1m, 5m, 15m)" "PASS" "$loadavg"

# ===== Kernel Information =====
log_section "Kernel Information"
cp /proc/version "$OUTPUT_DIR/version.txt"

kernel_version=$(uname -r)
kernel_major=$(echo "$kernel_version" | cut -d. -f1)
kernel_minor=$(echo "$kernel_version" | cut -d. -f2)

log_check "Kernel Version" "PASS" "$kernel_version"
if [[ $kernel_major -gt 6 ]] || [[ $kernel_major -eq 6 && $kernel_minor -ge 0 ]]; then
    log_check "Kernel >= 6.0" "PASS" "OK for eBPF features"
else
    log_check "Kernel >= 6.0" "FAIL" "Current: $kernel_major.$kernel_minor"
fi

# ===== Disk Information =====
log_section "Disk Information"
lsblk -J > "$OUTPUT_DIR/lsblk.json" 2>/dev/null || echo '{"blockdevices":[]}' > "$OUTPUT_DIR/lsblk.json"
df -h > "$OUTPUT_DIR/df.txt" 2>/dev/null || touch "$OUTPUT_DIR/df.txt"

root_usage=$(df / | tail -1)
log_check "Root Filesystem" "PASS" "$root_usage"

root_available=$(df / | tail -1 | awk '{print $4}')
log_check "Root Available Space" "PASS" "$root_available"

# ===== Network Interfaces =====
log_section "Network Interfaces"
ip -j link show > "$OUTPUT_DIR/ip_link.json" 2>/dev/null || echo '[]' > "$OUTPUT_DIR/ip_link.json"
ip -j addr show > "$OUTPUT_DIR/ip_addr.json" 2>/dev/null || echo '[]' > "$OUTPUT_DIR/ip_addr.json"

iface_count=$(ip -j link show | jq 'length' 2>/dev/null || echo "0")
log_check "Network Interfaces" "PASS" "Count: $iface_count"

# ===== IPv6 Check =====
ipv6_addrs=$(ip -6 addr show 2>/dev/null | grep -c "inet6" || echo "0")
if [[ $ipv6_addrs -gt 0 ]]; then
    log_check "IPv6 Support" "PASS" "Found $ipv6_addrs IPv6 addresses"
else
    log_check "IPv6 Support" "FAIL" "No IPv6 addresses configured"
fi

# ===== BTF (BPF Type Format) =====
log_section "eBPF Capabilities"
if [[ -f /sys/kernel/btf/vmlinux ]]; then
    log_check "BTF vmlinux" "PASS" "Available for CO-RE"
    touch "$OUTPUT_DIR/btf_available.flag"
else
    log_check "BTF vmlinux" "FAIL" "Not available"
fi

# ===== Cgroup v2 Check =====
if grep -q cgroup2 /proc/filesystems 2>/dev/null; then
    log_check "Cgroup v2 Support" "PASS" "Kernel supports unified hierarchy"
    touch "$OUTPUT_DIR/cgroupv2_available.flag"
else
    log_check "Cgroup v2 Support" "FAIL" "Not available"
fi

# Check cgroup delegation
if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
    cgroup_controllers=$(cat /sys/fs/cgroup/cgroup.controllers)
    log_check "Cgroup Controllers" "PASS" "$cgroup_controllers"
    echo "$cgroup_controllers" > "$OUTPUT_DIR/cgroup_controllers.txt"
else
    log_check "Cgroup Controllers" "FAIL" "Not accessible"
fi

# ===== PSI (Pressure Stall Information) =====
log_section "System Pressure Metrics"
if [[ -f /proc/pressure/cpu ]]; then
    log_check "PSI CPU" "PASS" "Available"
    cp /proc/pressure/cpu "$OUTPUT_DIR/psi_cpu.txt" 2>/dev/null || true
    cp /proc/pressure/memory "$OUTPUT_DIR/psi_memory.txt" 2>/dev/null || true
    cp /proc/pressure/io "$OUTPUT_DIR/psi_io.txt" 2>/dev/null || true
else
    log_check "PSI CPU" "FAIL" "Not available"
fi

# ===== eBPF Tools =====
if command -v bpftool &> /dev/null; then
    bpftool prog list > "$OUTPUT_DIR/bpftool_progs.txt" 2>/dev/null || echo "bpftool not accessible" > "$OUTPUT_DIR/bpftool_progs.txt"
    log_check "bpftool" "PASS" "Available"
else
    echo "bpftool not available" > "$OUTPUT_DIR/bpftool_progs.txt"
    log_check "bpftool" "FAIL" "Not installed"
fi

# ===== WireGuard Check =====
log_section "Network Tunneling"
if command -v wg &> /dev/null; then
    if wg show &>/dev/null; then
        wg show > "$OUTPUT_DIR/wg_status.txt" 2>/dev/null || echo "WireGuard not running" > "$OUTPUT_DIR/wg_status.txt"
        log_check "WireGuard" "PASS" "Running"
    else
        echo "WireGuard not running" > "$OUTPUT_DIR/wg_status.txt"
        log_check "WireGuard" "FAIL" "Tool present but no active interfaces"
    fi
else
    echo "WireGuard not available" > "$OUTPUT_DIR/wg_status.txt"
    log_check "WireGuard" "FAIL" "Not installed"
fi

# ===== Host-specific checks =====
if [[ "$HOST_ROLE" == "forge" ]] || [[ "$HOSTNAME_DETECTED" =~ host-a ]]; then
    log_section "Host-A (Forge) Specific Checks"
    
    # ROCm check
    if command -v rocm-smi &> /dev/null; then
        rocm-smi > "$OUTPUT_DIR/rocm_smi.txt" 2>/dev/null || echo "rocm-smi failed" > "$OUTPUT_DIR/rocm_smi.txt"
        log_check "ROCm SMI" "PASS" "Available"
    else
        echo "ROCm not available" > "$OUTPUT_DIR/rocm_smi.txt"
        log_check "ROCm SMI" "FAIL" "Not installed"
    fi
    
    # AMD GPU check
    if command -v lspci &> /dev/null; then
        if lspci | grep -qi "amd\|ati"; then
            lspci | grep -i "amd\|ati" > "$OUTPUT_DIR/amd_gpu.txt"
            log_check "AMD GPU" "PASS" "Detected"
        else
            echo "No AMD GPU detected" > "$OUTPUT_DIR/amd_gpu.txt"
            log_check "AMD GPU" "FAIL" "Not found"
        fi
    fi
    
    # AMD GPU kernel modules
    if lsmod | grep -q amdgpu; then
        log_check "AMDGPU Kernel Module" "PASS" "Loaded"
        lsmod | grep amdgpu > "$OUTPUT_DIR/amdgpu_module.txt"
    else
        log_check "AMDGPU Kernel Module" "FAIL" "Not loaded"
        echo "amdgpu not loaded" > "$OUTPUT_DIR/amdgpu_module.txt"
    fi
fi

# ===== Build JSON Summary =====
log_section "Generating JSON Summary"

# Build JSON manually
cat > "$OUTPUT_DIR/baseline.json" << JSON_EOF
{
  "metadata": {
    "timestamp": "$TIMESTAMP",
    "capture_date_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "hostname": "$HOSTNAME_DETECTED",
    "host_role": "$HOST_ROLE"
  },
  "cpu": {
    "count": $cpu_count,
    "model": "$cpu_model",
    "frequency_mhz": $(echo "$cpu_freq" | cut -d. -f1)
  },
  "memory": {
    "total_kb": $mem_total,
    "available_kb": $mem_available,
    "total_gb": $mem_total_gb,
    "available_gb": $mem_available_gb
  },
  "load_average": {
    "one_minute": $(cat /proc/loadavg | awk '{print $1}'),
    "five_minute": $(cat /proc/loadavg | awk '{print $2}'),
    "fifteen_minute": $(cat /proc/loadavg | awk '{print $3}')
  },
  "kernel": {
    "version": "$kernel_version",
    "major": $kernel_major,
    "minor": $kernel_minor,
    "supports_ebpf": $(if [[ $kernel_major -gt 6 ]] || [[ $kernel_major -eq 6 && $kernel_minor -ge 0 ]]; then echo "true"; else echo "false"; fi)
  },
  "storage": {
    "root_available": "$root_available"
  },
  "network": {
    "interface_count": $iface_count,
    "ipv6_enabled": $(if [[ $ipv6_addrs -gt 0 ]]; then echo "true"; else echo "false"; fi)
  },
  "capabilities": {
    "btf_available": $(if [[ -f /sys/kernel/btf/vmlinux ]]; then echo "true"; else echo "false"; fi),
    "cgroup_v2_supported": $(if grep -q cgroup2 /proc/filesystems 2>/dev/null; then echo "true"; else echo "false"; fi),
    "psi_available": $(if [[ -f /proc/pressure/cpu ]]; then echo "true"; else echo "false"; fi),
    "bpftool_available": $(if command -v bpftool &> /dev/null; then echo "true"; else echo "false"; fi),
    "wireguard_available": $(if command -v wg &> /dev/null; then echo "true"; else echo "false"; fi)
  }
}
JSON_EOF

log_check "JSON Summary" "PASS" "$OUTPUT_DIR/baseline.json"

# ===== Final Summary Table =====
log_section "Baseline Capture Summary"
echo ""
echo "Output Directory: $OUTPUT_DIR"
echo "Files Generated:"
ls -lh "$OUTPUT_DIR" | tail -n +2 | awk '{print "  " $9 " (" $5 ")"}'
echo ""

# Determine exit code based on critical checks
critical_pass=true
if [[ ! -f /sys/kernel/btf/vmlinux ]]; then
    critical_pass=false
fi
if ! grep -q cgroup2 /proc/filesystems 2>/dev/null; then
    critical_pass=false
fi
if [[ $kernel_major -lt 6 ]]; then
    critical_pass=false
fi

if [[ "$critical_pass" == "true" ]]; then
    echo -e "${GREEN}All critical checks PASSED${NC}"
    exit 0
else
    echo -e "${RED}Some critical checks FAILED${NC}"
    exit 1
fi
