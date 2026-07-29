#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# preflight-host-b.sh — HOST-B (THE OUTPOST) PREFLIGHT CHECK
#
# Purpose: Verify hardware and software prerequisites before boot sequence
# Similar to preflight-host-a.sh but with BIRD/IPFire specific checks
# Usage: ./preflight-host-b.sh
# Exit Code: 0 = PASS, 1 = FAIL

set -euo pipefail

# version_ge <have> <want> — true if $1 >= $2, compared as VERSIONS.
#
# The checks below previously used bash's [[ "$a" > "$b" ]], which is a
# LEXICOGRAPHIC string comparison, not a numeric one. That silently passed
# older kernels: [[ "5.9" > "5.18" ]] is TRUE because "9" sorts after "1".
# A host running 5.9 cleared a ">= 5.17" gate that exists precisely because
# the eBPF features this project needs landed in 5.17.
version_ge() {
    [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" = "$2" ]
}


# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Track results
checks_passed=0
checks_failed=0

# Helper functions (same as host-a)
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

print_header "HOST-B (THE OUTPOST) PREFLIGHT CHECK"

# ============ KERNEL CHECKS ============
print_header "1. KERNEL REQUIREMENTS"

print_check "Kernel version >= 5.17"
kernel_version=$(uname -r | cut -d. -f1,2)
if version_ge "$kernel_version" "5.17"; then
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

print_check "virt-install available (for IPFire VM)"
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
  warn
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
  if version_ge "$go_version" "1.24"; then
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
print_header "6. PROTOCOL SUPPORT (HOST-B SPECIFIC)"

print_check "BIRD daemon available (not FRR)"
if command -v bird >/dev/null 2>&1 || dpkg-query -W -f='${Status}' bird2 2>/dev/null | grep -q 'installed'; then
  pass
else
  fail
fi

print_check "BIRD has IPv6 support"
if command -v bird >/dev/null 2>&1 && bird -v 2>/dev/null | grep -q 'bird'; then
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
if modprobe -n wireguard >/dev/null 2>&1 || modprobe -n wireguard 2>/dev/null 2>&1 || command -v wg >/dev/null 2>&1; then
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

print_check "nftables (IPFire firewall backend)"
if command -v nft >/dev/null 2>&1 || modprobe -n nft_compat >/dev/null 2>&1; then
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

print_check "scripts/firewall/ directory exists (for IPFire scripts)"
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

# ============ HOST-B SPECIFIC ============
print_header "9. HOST-B SPECIFIC PREREQUISITES"

print_check "Will use AS 65002 for BGP (confirm routing config exists)"
if [[ -f ./routing/bird/bird.conf ]] || [[ -f ./nixos/hosts/host-b/configuration.nix ]]; then
  pass
else
  warn
fi

print_check "Service subnet will be 10.30.0.0/16 (not 10.20.x)"
if grep -r '10.30' ./nixos/ ./routing/ 2>/dev/null | head -1 | grep -q '10.30'; then
  pass
else
  warn
fi

# ============ SUMMARY ============
print_header "PREFLIGHT CHECK SUMMARY"

echo "Checks Passed: ${GREEN}${checks_passed}${NC}"
echo "Checks Failed: ${RED}${checks_failed}${NC}"

if [[ $checks_failed -eq 0 ]]; then
  echo ""
  echo -e "${GREEN}✓ ALL CHECKS PASSED — HOST-B IS READY FOR BOOT SEQUENCE${NC}"
  echo ""
  echo "Next Steps:"
  echo "  1. Ensure Host-A is online first (prerequisite)"
  echo "  2. Review BOOT_SEQUENCE_HOST_B.md"
  echo "  3. Create NixOS ISO: nix run nixpkgs#nixos-generators -- --format iso --flake \".#host-b\" -o nixos-host-b-live.iso"
  echo "  4. Boot target hardware with ISO"
  echo "  5. Follow Phase 1 (NixOS Base Install)"
  echo ""
  exit 0
else
  echo ""
  echo -e "${RED}✗ PREFLIGHT CHECK FAILED${NC}"
  echo ""
  echo "Failed Checks:"
  echo "  Fix the above issues before proceeding to boot sequence."
  echo "  Many checks are identical to Host-A; see BOOT_SEQUENCE_HOST_B.md for host-b specific differences."
  echo ""
  exit 1
fi
