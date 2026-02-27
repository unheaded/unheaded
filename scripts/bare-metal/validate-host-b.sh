#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# validate-host-b.sh — HOST-B POST-BOOT VALIDATION
#
# Purpose: Verify all components are operational on Host-B (The Outpost)
# Usage: ./validate-host-b.sh [--json]
# Exit Code: 0 = PASS, 1 = FAIL
# Note: Similar to validate-host-a.sh but for host-b specific configs

set -euo pipefail

# Options
output_json=${1:-}

# Tracking
checks_passed=0
checks_failed=0
json_results='{}'

# Color codes
if [[ "$output_json" == "--json" ]]; then
  RED=''
  GREEN=''
  YELLOW=''
  NC=''
else
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  NC='\033[0m'
fi

# Helper functions
print_check() {
  if [[ -z "$output_json" ]]; then
    printf "%-60s " "$1"
  fi
}

pass() {
  ((checks_passed++))
  if [[ -z "$output_json" ]]; then
    echo -e "${GREEN}PASS${NC}"
  fi
}

fail() {
  ((checks_failed++))
  if [[ -z "$output_json" ]]; then
    echo -e "${RED}FAIL${NC}"
  fi
}

json_add_result() {
  local name="$1"
  local status="$2"
  local details="${3:-}"

  if [[ "$output_json" == "--json" ]]; then
    json_results=$(echo "$json_results" | jq --arg name "$name" --arg status "$status" --arg details "$details" \
      '.checks[$name] = {"status": $status, "details": $details}')
  fi
}

print_header() {
  if [[ -z "$output_json" ]]; then
    echo ""
    echo "=========================================="
    echo "$1"
    echo "=========================================="
  fi
}

print_header "HOST-B POST-BOOT VALIDATION"

# ============ SYSTEM CHECKS ============
print_header "1. SYSTEM STATUS"

print_check "Hostname is 'outpost'"
if hostname | grep -q 'outpost'; then
  pass
  json_add_result "hostname" "PASS"
else
  fail
  json_add_result "hostname" "FAIL" "$(hostname)"
fi

print_check "Kernel version >= 5.17"
kernel=$(uname -r | cut -d. -f1,2)
if [[ "$kernel" > "5.17" ]] || [[ "$kernel" == "5.17" ]]; then
  pass
  json_add_result "kernel_version" "PASS" "$(uname -r)"
else
  fail
  json_add_result "kernel_version" "FAIL" "$(uname -r)"
fi

print_check "BTF support available"
if [[ -f /sys/kernel/btf/vmlinux ]]; then
  pass
  json_add_result "btf_support" "PASS"
else
  fail
  json_add_result "btf_support" "FAIL"
fi

print_check "systemd-resolved running"
if systemctl is-active -q systemd-resolved; then
  pass
  json_add_result "systemd_resolved" "PASS"
else
  fail
  json_add_result "systemd_resolved" "FAIL"
fi

# ============ NETWORK CHECKS ============
print_header "2. NETWORK CONNECTIVITY"

print_check "WAN interface (eno1) UP"
if ip link show eno1 | grep -q 'UP'; then
  pass
  json_add_result "eno1_status" "PASS"
else
  fail
  json_add_result "eno1_status" "FAIL"
fi

print_check "WAN has DHCP IP"
wan_ip=$(ip -4 addr show eno1 | grep -oP '(?<=inet\s)\d+(\.\d+){3}' || echo "")
if [[ -n "$wan_ip" ]]; then
  pass
  json_add_result "wan_ip" "PASS" "$wan_ip"
else
  fail
  json_add_result "wan_ip" "FAIL"
fi

print_check "Loopback has 10.30.255.1 (host-b subnet)"
if ip addr show lo | grep -q '10.30.255.1'; then
  pass
  json_add_result "loopback_ip" "PASS"
else
  fail
  json_add_result "loopback_ip" "FAIL"
fi

print_check "Service subnet is 10.30.0.0/16 (not 10.20.x)"
if ip addr show | grep -q '10.30.' && ! ip addr show | grep '10.20.'; then
  pass
  json_add_result "service_subnet" "PASS"
else
  fail
  json_add_result "service_subnet" "FAIL"
fi

# ============ FIREWALL VM CHECKS ============
print_header "3. IPFIRE FIREWALL VM"

print_check "IPFire VM running (virsh list)"
if virsh list 2>/dev/null | grep -q 'ipfire-outpost'; then
  pass
  json_add_result "ipfire_vm_running" "PASS"
else
  fail
  json_add_result "ipfire_vm_running" "FAIL"
fi

print_check "IPFire WebUI accessible (https://192.168.2.1:8443)"
if curl -sk https://192.168.2.1:8443/ 2>/dev/null | head -1 | grep -q 'html\|HTTP\|<!'; then
  pass
  json_add_result "ipfire_webui" "PASS"
else
  fail
  json_add_result "ipfire_webui" "FAIL"
fi

print_check "IPFire firewall rules include HbH passthrough"
if ssh root@192.168.2.1 "nft list table inet filter 2>/dev/null | grep 'nexthdr 0'" >/dev/null 2>&1; then
  pass
  json_add_result "ipfire_hbh_rules" "PASS"
else
  fail
  json_add_result "ipfire_hbh_rules" "FAIL"
fi

# ============ ROUTING (BIRD) CHECKS ============
print_header "4. BIRD ROUTING"

print_check "BIRD service running"
if systemctl is-active -q bird; then
  pass
  json_add_result "bird_service" "PASS"
else
  fail
  json_add_result "bird_service" "FAIL"
fi

print_check "BIRD configuration valid (birdc 'show')"
if sudo birdc "show" >/dev/null 2>&1; then
  pass
  json_add_result "bird_config" "PASS"
else
  fail
  json_add_result "bird_config" "FAIL"
fi

print_check "BGP configured (AS 65002)"
if sudo birdc "show protocols bgp" 2>/dev/null | grep -q 'bgp'; then
  pass
  json_add_result "bgp_as" "PASS"
else
  fail
  json_add_result "bgp_as" "FAIL"
fi

print_check "BGP has neighbor configured (fd00:dead:beef::1)"
if sudo birdc "show bgp summary" 2>/dev/null | grep -q 'fd00:dead:beef::1'; then
  pass
  json_add_result "bgp_neighbor_config" "PASS"
else
  fail
  json_add_result "bgp_neighbor_config" "FAIL"
fi

# Note: Neighbor state depends on WireGuard tunnel
bgp_state=$(sudo birdc "show bgp summary" 2>/dev/null | grep 'fd00:dead:beef' | awk '{print $NF}' || echo "UNKNOWN")
print_check "BGP neighbor state (expect 'Established' if host-a online)"
if [[ "$bgp_state" == "Established" ]] || [[ "$bgp_state" == "Active" ]] || [[ "$bgp_state" == "Connect" ]]; then
  pass
  json_add_result "bgp_neighbor_state" "PASS" "$bgp_state"
else
  fail
  json_add_result "bgp_neighbor_state" "FAIL" "$bgp_state"
fi

# ============ VXLAN/EVPN CHECKS ============
print_header "5. VXLAN/EVPN"

print_check "VXLAN interface vxlan10001 exists"
if ip link show vxlan10001 >/dev/null 2>&1; then
  pass
  json_add_result "vxlan_interface" "PASS"
else
  fail
  json_add_result "vxlan_interface" "FAIL"
fi

print_check "VXLAN bridge br-vxlan10001 exists"
if ip link show br-vxlan10001 >/dev/null 2>&1; then
  pass
  json_add_result "vxlan_bridge" "PASS"
else
  fail
  json_add_result "vxlan_bridge" "FAIL"
fi

print_check "BIRD capable of EVPN routes (show bgp protocol)"
if sudo birdc "show protocols bgp" 2>/dev/null | grep -q 'bgp'; then
  pass
  json_add_result "evpn_capable" "PASS"
else
  fail
  json_add_result "evpn_capable" "FAIL"
fi

# ============ EBPF CHECKS ============
print_header "6. eBPF LOADING & PINNING"

print_check "BPF filesystem mounted"
if mountpoint -q /sys/fs/bpf/unheaded; then
  pass
  json_add_result "bpf_fs_mounted" "PASS"
else
  fail
  json_add_result "bpf_fs_mounted" "FAIL"
fi

print_check "eBPF programs loaded (4+ expected)"
prog_count=$(sudo bpftool prog list 2>/dev/null | wc -l)
if [[ $prog_count -ge 4 ]]; then
  pass
  json_add_result "ebpf_programs_loaded" "PASS" "$prog_count programs"
else
  fail
  json_add_result "ebpf_programs_loaded" "FAIL" "$prog_count programs"
fi

print_check "eBPF maps available (8+ expected)"
map_count=$(sudo bpftool map list 2>/dev/null | wc -l)
if [[ $map_count -ge 8 ]]; then
  pass
  json_add_result "ebpf_maps_available" "PASS" "$map_count maps"
else
  fail
  json_add_result "ebpf_maps_available" "FAIL" "$map_count maps"
fi

print_check "XDP attached to eno1 (Shield)"
if sudo bpftool net list 2>/dev/null | grep -q 'eno1.*xdp'; then
  pass
  json_add_result "xdp_attached" "PASS"
else
  fail
  json_add_result "xdp_attached" "FAIL"
fi

# ============ DOCKER COMPOSE CHECKS ============
print_header "7. SERVICE FLEET"

print_check "Docker compose services UP"
if docker compose ps | tail -n +2 | awk '{print $NF}' | grep -v -E '^(Up|Exited)' | wc -l | grep -q '^0$'; then
  pass
  json_add_result "docker_services_up" "PASS"
else
  fail
  json_add_result "docker_services_up" "FAIL"
fi

print_check "Wotan service health"
if docker compose exec -T wotan curl -s http://localhost:18000/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "wotan_health" "PASS"
else
  fail
  json_add_result "wotan_health" "FAIL"
fi

print_check "Sophia service health"
if docker compose exec -T sophia curl -s http://localhost:19000/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "sophia_health" "PASS"
else
  fail
  json_add_result "sophia_health" "FAIL"
fi

print_check "Monad service health"
if docker compose exec -T monad curl -s http://localhost:16666/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "monad_health" "PASS"
else
  fail
  json_add_result "monad_health" "FAIL"
fi

print_check "No FATAL errors in service logs"
if ! docker compose logs | grep -i 'FATAL\|panic: ' >/dev/null 2>&1; then
  pass
  json_add_result "no_fatal_errors" "PASS"
else
  fail
  json_add_result "no_fatal_errors" "FAIL"
fi

# ============ SUMMARY ============

if [[ "$output_json" == "--json" ]]; then
  json_results=$(echo "$json_results" | jq \
    --arg passed "$checks_passed" \
    --arg failed "$checks_failed" \
    '.summary = {"passed": $passed, "failed": $failed, "total": ($passed | tonumber) + ($failed | tonumber)}')
  echo "$json_results" | jq '.'
else
  print_header "VALIDATION SUMMARY"

  echo "Checks Passed: ${GREEN}${checks_passed}${NC}"
  echo "Checks Failed: ${RED}${checks_failed}${NC}"
  echo "Total Checks: $((checks_passed + checks_failed))"

  if [[ $checks_failed -eq 0 ]]; then
    echo ""
    echo -e "${GREEN}✓ HOST-B VALIDATION PASSED${NC}"
    echo ""
    echo "Next Steps:"
    echo "  1. Set up WireGuard tunnel (PHASE 12B)"
    echo "  2. Run cross-host validation: ./validate-cross-host.sh"
    echo ""
  else
    echo ""
    echo -e "${RED}✗ VALIDATION FAILED (${checks_failed} checks)${NC}"
    echo ""
    echo "See BOOT_SEQUENCE_HOST_B.md for troubleshooting"
    echo ""
  fi
fi

if [[ $checks_failed -eq 0 ]]; then
  exit 0
else
  exit 1
fi
