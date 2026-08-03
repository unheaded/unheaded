#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# validate-host-a.sh — HOST-A POST-BOOT VALIDATION
#
# Purpose: Verify all components are operational after boot sequence
# Usage: ./validate-host-a.sh [--json]
# Exit Code: 0 = PASS, 1 = FAIL

set -euo pipefail

# Firewall API credentials come from the environment — never the source tree.
#
# These were previously inline in the curl calls below, which put working
# OPNsense admin credentials in git history (gitleaks: curl-auth-user). They
# must be rotated regardless of this change; see
# docs/security/PRE-PUBLIC-BLOCKERS.md.
: "${OPNSENSE_API_KEY:?set OPNSENSE_API_KEY — firewall API credentials are no longer embedded in this script}"
: "${OPNSENSE_API_SECRET:?set OPNSENSE_API_SECRET — firewall API credentials are no longer embedded in this script}"


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


# Options
output_json=${1:-}

# Tracking
checks_passed=0
checks_failed=0
json_results='{}'

# Color codes (disabled if JSON output)
if [[ "$output_json" == "--json" ]]; then
  RED=''
  GREEN=''
  NC=''
else
  RED='\033[0;31m'
  GREEN='\033[0;32m'
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

# Main validation checks

print_header "HOST-A POST-BOOT VALIDATION"

# ============ SYSTEM CHECKS ============
print_header "1. SYSTEM STATUS"

print_check "Hostname is 'forge'"
if hostname | grep -q 'forge'; then
  pass
  json_add_result "hostname" "PASS"
else
  fail
  json_add_result "hostname" "FAIL" "$(hostname)"
fi

print_check "Kernel version >= 5.17"
kernel=$(uname -r | cut -d. -f1,2)
if version_ge "$kernel" "5.17"; then
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

print_check "Loopback has 10.20.255.1"
if ip addr show lo | grep -q '10.20.255.1'; then
  pass
  json_add_result "loopback_ip" "PASS"
else
  fail
  json_add_result "loopback_ip" "FAIL"
fi

print_check "DNS resolution working"
if getent hosts google.com >/dev/null 2>&1 || nslookup google.com >/dev/null 2>&1; then
  pass
  json_add_result "dns" "PASS"
else
  fail
  json_add_result "dns" "FAIL"
fi

# ============ FIREWALL VM CHECKS ============
print_header "3. OPNSENSE FIREWALL VM"

print_check "OPNsense VM running (virsh list)"
if virsh list 2>/dev/null | grep -q 'opnsense-forge'; then
  pass
  json_add_result "opnsense_vm_running" "PASS"
else
  fail
  json_add_result "opnsense_vm_running" "FAIL"
fi

print_check "OPNsense WebUI accessible (https://192.168.1.1)"
if curl -sk https://192.168.1.1:443/ 2>/dev/null | head -1 | grep -q 'html\|HTTP\|<!'; then
  pass
  json_add_result "opnsense_webui" "PASS"
else
  fail
  json_add_result "opnsense_webui" "FAIL"
fi

print_check "OPNsense firewall rules include HbH (Monad passthrough)"
if curl -sk -u "${OPNSENSE_API_KEY}:${OPNSENSE_API_SECRET}" https://192.168.1.1:443/api/firewall/filter/searchRule 2>/dev/null | \
   python3 -c "import sys, json; rules = json.load(sys.stdin).get('rows', []); hbh = [r for r in rules if 'HbH' in r.get('description','')]; sys.exit(0 if hbh else 1)"; then
  pass
  json_add_result "opnsense_hbh_rules" "PASS"
else
  fail
  json_add_result "opnsense_hbh_rules" "FAIL"
fi

# ============ ROUTING (FRR) CHECKS ============
print_header "4. FRR ROUTING"

print_check "FRR service running"
if systemctl is-active -q frr; then
  pass
  json_add_result "frr_service" "PASS"
else
  fail
  json_add_result "frr_service" "FAIL"
fi

print_check "IS-IS configured (show isis interface)"
if vtysh -c "show isis interface" 2>/dev/null | grep -q 'wg0'; then
  pass
  json_add_result "isis_interface" "PASS"
else
  fail
  json_add_result "isis_interface" "FAIL"
fi

print_check "BGP configured (router bgp 65001)"
if vtysh -c "show bgp summary" 2>/dev/null | grep -q '65001'; then
  pass
  json_add_result "bgp_as" "PASS"
else
  fail
  json_add_result "bgp_as" "FAIL"
fi

print_check "BGP has neighbor configured (fd00:dead:beef::2)"
if vtysh -c "show bgp neighbor" 2>/dev/null | grep -q 'fd00:dead:beef::2'; then
  pass
  json_add_result "bgp_neighbor_config" "PASS"
else
  fail
  json_add_result "bgp_neighbor_config" "FAIL"
fi

# Note: Neighbor state will be "Active" until host-b is online
bgp_state=$(vtysh -c "show bgp summary" 2>/dev/null | grep 'fd00:dead:beef' | awk '{print $NF}' || echo "UNKNOWN")
print_check "BGP neighbor state (expect 'Established' if host-b online, else 'Active')"
if [[ "$bgp_state" == "Established" ]] || [[ "$bgp_state" == "Active" ]]; then
  pass
  json_add_result "bgp_neighbor_state" "PASS" "$bgp_state"
else
  fail
  json_add_result "bgp_neighbor_state" "FAIL" "$bgp_state"
fi

print_check "BFD peer configured"
if vtysh -c "show bfd peer" 2>/dev/null | grep -q 'fd00:dead:beef::2'; then
  pass
  json_add_result "bfd_peer" "PASS"
else
  fail
  json_add_result "bfd_peer" "FAIL"
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

print_check "FRR advertising EVPN routes"
if vtysh -c "show bgp l2vpn evpn summary" 2>/dev/null | grep -q 'Received'; then
  pass
  json_add_result "evpn_advertising" "PASS"
else
  fail
  json_add_result "evpn_advertising" "FAIL"
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

print_check "Monad BPF map present (monad_flow_table)"
if sudo bpftool map list 2>/dev/null | grep -q 'monad'; then
  pass
  json_add_result "monad_map" "PASS"
else
  fail
  json_add_result "monad_map" "FAIL"
fi

# ============ DOCKER COMPOSE CHECKS ============
print_header "7. SERVICE FLEET"

print_check "Docker compose services UP (not 'starting')"
if docker compose ps | tail -n +2 | awk '{print $NF}' | grep -v -E '^(Up|Exited)' | wc -l | grep -q '^0$'; then
  pass
  json_add_result "docker_services_up" "PASS"
else
  fail
  json_add_result "docker_services_up" "FAIL"
fi

print_check "Wotan service health (curl localhost:18000/health)"
if docker compose exec -T wotan curl -s http://localhost:18000/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "wotan_health" "PASS"
else
  fail
  json_add_result "wotan_health" "FAIL"
fi

print_check "Sophia service health (curl localhost:19000/health)"
if docker compose exec -T sophia curl -s http://localhost:19000/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "sophia_health" "PASS"
else
  fail
  json_add_result "sophia_health" "FAIL"
fi

print_check "Monad service health (curl localhost:16666/health)"
if docker compose exec -T monad curl -s http://localhost:16666/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "monad_health" "PASS"
else
  fail
  json_add_result "monad_health" "FAIL"
fi

print_check "Shield service health (curl localhost:16667/health)"
if docker compose exec -T shield curl -s http://localhost:16667/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "shield_health" "PASS"
else
  fail
  json_add_result "shield_health" "FAIL"
fi

print_check "Gateway service health (curl localhost:21000/health)"
if docker compose exec -T gateway curl -s http://localhost:21000/health 2>/dev/null | jq -e '.status == "healthy"' >/dev/null 2>&1; then
  pass
  json_add_result "gateway_health" "PASS"
else
  fail
  json_add_result "gateway_health" "FAIL"
fi

print_check "No FATAL errors in service logs"
if ! docker compose logs | grep -i 'FATAL\|panic: ' >/dev/null 2>&1; then
  pass
  json_add_result "no_fatal_errors" "PASS"
else
  fail
  json_add_result "no_fatal_errors" "FAIL"
fi

# ============ PACKET FLOW CHECKS ============
print_header "8. END-TO-END VALIDATION"

print_check "Ping service subnet IPs (10.10.10.1-10)"
ping_success=0
for i in {1..10}; do
  if timeout 1 ping -c 1 10.10.10.$i >/dev/null 2>&1; then
    ((ping_success++))
  fi
done
if [[ $ping_success -ge 5 ]]; then
  pass
  json_add_result "service_ping" "PASS" "$ping_success/10 IPs reachable"
else
  fail
  json_add_result "service_ping" "FAIL" "$ping_success/10 IPs reachable"
fi

print_check "Monad HbH packet capture (expect ip6 proto 0 traffic)"
if timeout 5 sudo tcpdump -i eno1 'ip6 proto 0' -c 1 >/dev/null 2>&1; then
  pass
  json_add_result "monad_packets" "PASS"
else
  # Not critical if no traffic yet
  fail
  json_add_result "monad_packets" "FAIL" "No HbH packets captured (expected if idle)"
fi

print_check "eBPF programs receiving traffic (packets > 0)"
if sudo bpftool prog stat 2>/dev/null | grep -q 'runs > 0' || sudo bpftool map show 2>/dev/null | grep -q 'entries > 0'; then
  pass
  json_add_result "ebpf_traffic" "PASS"
else
  fail
  json_add_result "ebpf_traffic" "FAIL"
fi

# ============ OUTPUT RESULTS ============

if [[ "$output_json" == "--json" ]]; then
  # Output JSON results
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
    echo -e "${GREEN}✓ ALL VALIDATION CHECKS PASSED${NC}"
    echo ""
    echo "Host-A (The Forge) is fully operational!"
    echo ""
    echo "Next Steps:"
    echo "  1. If this is a multi-host setup, proceed to PHASE 12A: Host-B boot sequence"
    echo "  2. Configure WireGuard tunnel (PHASE 12B) to connect Host-A and Host-B"
    echo "  3. Run cross-host validation (./validate-cross-host.sh)"
    echo ""
  else
    echo ""
    echo -e "${RED}✗ VALIDATION FAILED (${checks_failed} checks)${NC}"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Check service logs: docker compose logs --tail=50"
    echo "  2. Review BOOT_SEQUENCE_HOST_A.md > TROUBLESHOOTING"
    echo "  3. Verify all PHASE 1-7 steps completed successfully"
    echo ""
  fi
fi

# Exit with proper code
if [[ $checks_failed -eq 0 ]]; then
  exit 0
else
  exit 1
fi
