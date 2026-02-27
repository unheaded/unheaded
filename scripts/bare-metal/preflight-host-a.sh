#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# preflight-host-a.sh — HOST-A (THE FORGE) PREFLIGHT CHECK
#
# Purpose: Verify hardware and software prerequisites before boot sequence
# Usage: ./preflight-host-a.sh
# Exit Code: 0 = PASS, 1 = FAIL

set -euo pipefail

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track results
checks_passed=0
checks_failed=0

# Helper functions
print_check() {
  local name="$1"
  printf "%-50s " "$name"
}

pass() {
  echo -e "${GREEN}PASS${NC}"
  ((checks_passed++))
}

fail() {
  echo -e "${RED}FAIL${NC}"
  ((checks_failed++))
}

warn() {
  echo -e "${YELLOW}WARN${NC}"
}

print_header() {
  echo ""
  echo "=========================================="
  echo "$1"
  echo "=========================================="
}

# Main preflight checks

print_header "HOST-A (THE FORGE) PREFLIGHT CHECK"

# ============ KERNEL CHECKS ============
print_header "1. KERNEL REQUIREMENTS"

print_check "Kernel version >= 5.17"
kernel_version=$(uname -r | cut -d. -f1,2)
if [[ $(echo "$kernel_version >= 5.17" | bc -l) -eq 1 ]] 2>/dev/null || [[ "$kernel_version" == "5.17" ]] || [[ "$kernel_version" == "5.18" ]] || [[ "$kernel_version" > "5.18" ]]; then
  pass
else
  fail
fi

print_check "BTF support available (/sys/kernel/btf/vmlinux exists)"
if [[ -f /sys/kernel/btf/vmlinux ]]; then
  pass
else
  fail
fi

print_check "eBPF kernel module available"
if modprobe -n bpf >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "BPF filesystem can be mounted"
if mountpoint -q /sys/fs/bpf 2>/dev/null || [[ -d /sys/fs/bpf ]]; then
  pass
else
  # Try to create and mount
  if mkdir -p /sys/fs/bpf/test && mount -t bpf bpf /sys/fs/bpf/test >/dev/null 2>&1; then
    umount /sys/fs/bpf/test 2>/dev/null || true
    rmdir /sys/fs/bpf/test 2>/dev/null || true
    pass
  else
    fail
  fi
fi

# ============ TOOLS CHECK ============
print_header "2. REQUIRED TOOLS"

print_check "bpftool available"
if command -v bpftool >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "Docker/containerd running"
if docker ps >/dev/null 2>&1 || (systemctl is-active -q docker 2>/dev/null || systemctl is-active -q containerd 2>/dev/null); then
  pass
else
  fail
fi

print_check "virt-install available (for OPNsense VM)"
if command -v virt-install >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "libvirtd running"
if systemctl is-active -q libvirtd 2>/dev/null || [[ -S /var/run/libvirt/libvirt-sock ]]; then
  pass
else
  fail
fi

print_check "jq available (for JSON parsing)"
if command -v jq >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "python3 available"
if command -v python3 >/dev/null 2>&1; then
  pass
else
  fail
fi

# ============ NETWORK CHECK ============
print_header "3. NETWORK REQUIREMENTS"

print_check "NIC count >= 2"
nic_count=$(ip link show | grep -E '^[0-9]+:' | grep -v 'lo:' | wc -l)
if [[ $nic_count -ge 2 ]]; then
  pass
else
  fail
fi

print_check "Primary NIC (eno1) exists"
if ip link show eno1 >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "Primary NIC has IP address"
if ip addr show eno1 | grep -qE 'inet|inet6'; then
  pass
else
  fail
fi

print_check "Loopback interface configured"
if ip addr show lo | grep -q '127.0.0.1'; then
  pass
else
  fail
fi

print_check "IPv6 supported"
if ip addr show lo | grep -q 'inet6'; then
  pass
else
  fail
fi

print_check "DNS resolution working"
if getent hosts google.com >/dev/null 2>&1 || nslookup google.com >/dev/null 2>&1; then
  pass
else
  fail
fi

# ============ HARDWARE CHECK ============
print_header "4. HARDWARE REQUIREMENTS"

print_check "RAM >= 16GB"
ram_gb=$(free -h | awk 'NR==2 {print $2}' | sed 's/Gi//')
if [[ -n "$ram_gb" ]] && [[ $(echo "$ram_gb >= 16" | bc -l) -eq 1 ]]; then
  pass
else
  warn  # Warning, not fail, as some test envs may be smaller
fi

print_check "Disk space >= 100GB free"
disk_gb=$(df / | awk 'NR==2 {print $4}' | xargs -I {} expr {} / 1024 / 1024)
if [[ -n "$disk_gb" ]] && [[ $disk_gb -ge 100 ]]; then
  pass
else
  fail
fi

print_check "CPU supports VMX or SVM (virtualization)"
if grep -qE 'vmx|svm' /proc/cpuinfo; then
  pass
else
  fail
fi

# ============ LANGUAGE RUNTIMES ============
print_header "5. BUILD ENVIRONMENT"

print_check "Go 1.24+ available"
if command -v go >/dev/null 2>&1; then
  go_version=$(go version | awk '{print $3}' | sed 's/go//')
  if [[ "$go_version" > "1.24" ]] || [[ "$go_version" == "1.24" ]]; then
    pass
  else
    fail
  fi
else
  fail
fi

print_check "Rust/cargo available"
if command -v cargo >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "Rust nightly toolchain available"
if rustup toolchain list 2>/dev/null | grep -q 'nightly'; then
  pass
else
  fail
fi

# ============ PROTOCOL SUPPORT ============
print_header "6. PROTOCOL SUPPORT"

print_check "FRR dependencies installable"
if dpkg-query -W -f='${Status}' frr 2>/dev/null | grep -q 'installed' || \
   command -v vtysh >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "VXLAN kernel module loadable"
if modprobe -n vxlan >/dev/null 2>&1 || modprobe -n vxlan 2>/dev/null; then
  pass
else
  fail
fi

print_check "WireGuard module available"
if modprobe -n wireguard >/dev/null 2>&1 || modprobe -n wireguard 2>/dev/null 2>&1 || \
   command -v wg >/dev/null 2>&1; then
  pass
else
  fail
fi

print_check "Bridge utilities available"
if command -v ip >/dev/null 2>&1 && ip link add type bridge help >/dev/null 2>&1; then
  pass
else
  fail
fi

# ============ PERMISSIONS ============
print_header "7. PERMISSIONS & SUDO"

print_check "Can load eBPF programs (root or CAP_BPF+CAP_PERFMON)"
if [[ "$EUID" -eq 0 ]]; then
  pass
else
  if sudo -n bpftool version >/dev/null 2>&1; then
    pass
  else
    warn
  fi
fi

print_check "Can mount BPF filesystem"
if [[ "$EUID" -eq 0 ]]; then
  pass
else
  if sudo -n mount -t bpf bpf /tmp/bpf-test >/dev/null 2>&1; then
    sudo umount /tmp/bpf-test 2>/dev/null || true
    pass
  else
    fail
  fi
fi

print_check "Can run docker compose"
if docker compose version >/dev/null 2>&1; then
  pass
else
  fail
fi

# ============ FILE STRUCTURE ============
print_header "8. REPOSITORY STRUCTURE"

print_check "nixos/ directory exists"
if [[ -d ./nixos ]]; then
  pass
else
  fail
fi

print_check "scripts/firewall/ directory exists"
if [[ -d ./scripts/firewall ]]; then
  pass
else
  fail
fi

print_check "routing/ directory exists"
if [[ -d ./routing ]]; then
  pass
else
  fail
fi

print_check "Makefile exists"
if [[ -f ./Makefile ]]; then
  pass
else
  fail
fi

print_check "docker-compose.yml exists"
if [[ -f ./docker-compose.yml ]]; then
  pass
else
  fail
fi

print_check "ebpf/ directory exists"
if [[ -d ./ebpf ]]; then
  pass
else
  fail
fi

# ============ SUMMARY ============
print_header "PREFLIGHT CHECK SUMMARY"

echo "Checks Passed: ${GREEN}${checks_passed}${NC}"
echo "Checks Failed: ${RED}${checks_failed}${NC}"

if [[ $checks_failed -eq 0 ]]; then
  echo ""
  echo -e "${GREEN}✓ ALL CHECKS PASSED — HOST-A IS READY FOR BOOT SEQUENCE${NC}"
  echo ""
  echo "Next Steps:"
  echo "  1. Review BOOT_SEQUENCE_HOST_A.md"
  echo "  2. Create NixOS ISO: nix run nixpkgs#nixos-generators -- --format iso --flake \".#host-a\" -o nixos-host-a-live.iso"
  echo "  3. Boot target hardware with ISO"
  echo "  4. Follow Phase 1 (NixOS Base Install)"
  echo ""
  exit 0
else
  echo ""
  echo -e "${RED}✗ PREFLIGHT CHECK FAILED${NC}"
  echo ""
  echo "Failed Checks:"
  echo "  Fix the above issues before proceeding to boot sequence."
  echo "  See BOOT_SEQUENCE_HOST_A.md > TROUBLESHOOTING for remediation."
  echo ""
  exit 1
fi
