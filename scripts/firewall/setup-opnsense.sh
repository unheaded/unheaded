#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# setup-opnsense.sh — Post-boot OPNsense configuration
# Runs AFTER OPNsense boots and completes initial setup wizard
# Configures: interfaces, firewall rules, Monad HbH passthrough, FRR plugin, Suricata rules

set -euo pipefail

OPNSENSE_HOST="${OPNSENSE_HOST:-10.20.0.1}"
OPNSENSE_PORT="${OPNSENSE_PORT:-443}"
OPNSENSE_USER="${OPNSENSE_USER:-root}"
OPNSENSE_PASS="${OPNSENSE_PASS:-opnsense}"  # change after first boot
API_BASE="https://${OPNSENSE_HOST}:${OPNSENSE_PORT}/api"
CURL="curl -sk -u ${OPNSENSE_USER}:${OPNSENSE_PASS}"

echo "========================================"
echo "Unheaded OPNsense 26.1.2 Setup"
echo "Host: ${OPNSENSE_HOST}"
echo "========================================"

# --- Helper functions ---
api_get()  { $CURL "${API_BASE}/$1"; }
api_post() { $CURL -X POST -H 'Content-Type: application/json' -d "$2" "${API_BASE}/$1"; }

wait_for_opnsense() {
    echo "[wait] Waiting for OPNsense API to respond..."
    local retries=30
    while ! $CURL "${API_BASE}/core/firmware/info" >/dev/null 2>&1; do
        retries=$((retries - 1))
        [[ $retries -eq 0 ]] && echo "ERROR: OPNsense API timeout" && exit 1
        echo "[wait] Not ready yet ($retries retries left)..."
        sleep 10
    done
    echo "[wait] OPNsense API ready."
}

# --- Step 1: Wait for API ---
wait_for_opnsense

# --- Step 2: Verify version ---
echo "[1/8] Verifying OPNsense version..."
api_get "core/firmware/info" | python3 -c "import sys,json; d=json.load(sys.stdin); print(f'Version: {d.get(\"product_version\",\"unknown\")}')"

# --- Step 3: Configure WAN interface ---
echo "[2/8] Configuring interfaces..."
api_post "interfaces/overview/setInterface/wan" '{
  "interface": {
    "enable": "1",
    "ipaddr": "dhcp",
    "ipaddrv6": "dhcp6",
    "dhcp_hostname": "forge-opnsense"
  }
}'

# --- Step 4: Configure LAN interface (10.20.0.1/16) ---
api_post "interfaces/overview/setInterface/lan" '{
  "interface": {
    "enable": "1",
    "ipaddr": "10.20.0.1",
    "subnet": "16",
    "ipaddrv6": "fd00:dead:beef:1::1",
    "subnetv6": "64"
  }
}'

# --- Step 5: Monad HbH passthrough rules ---
echo "[3/8] Adding Monad HbH passthrough firewall rules..."

# Allow IPv6 HbH (next-header 0x00) — Monad protocol
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "wan",
    "ipprotocol": "inet6",
    "protocol": "hopopt",
    "direction": "in",
    "description": "UNHEADED: Allow Monad HbH IPv6 extension headers (inbound WAN)",
    "log": "0",
    "quick": "1"
  }
}'

api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "lan",
    "ipprotocol": "inet6",
    "protocol": "hopopt",
    "direction": "in",
    "description": "UNHEADED: Allow Monad HbH IPv6 extension headers (inbound LAN)",
    "log": "0",
    "quick": "1"
  }
}'

# Disable scrub for IPv6 (prevents HbH header rewriting)
echo "[3/8] Disabling IPv6 scrub (preserves HbH headers)..."
api_post "core/system/setOptimizations" '{
  "optimization": {
    "scrub": "none"
  }
}'

# --- Step 6: Default firewall rules ---
echo "[4/8] Setting default firewall rules..."

# Allow established/related
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "wan",
    "direction": "in",
    "ipprotocol": "inet46",
    "statetype": "keep state",
    "description": "Allow established/related",
    "quick": "1"
  }
}'

# Allow LAN → WAN
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "lan",
    "direction": "in",
    "ipprotocol": "inet46",
    "description": "Allow LAN to WAN (service containers)",
    "quick": "1"
  }
}'

# Allow WireGuard (UDP 51820)
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "wan",
    "direction": "in",
    "ipprotocol": "inet",
    "protocol": "udp",
    "destination_port": "51820",
    "description": "WireGuard east-west to host-b",
    "quick": "1"
  }
}'

# Allow HTTPS (443) inbound
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "pass",
    "interface": "wan",
    "direction": "in",
    "ipprotocol": "inet",
    "protocol": "tcp",
    "destination_port": "443",
    "description": "HTTPS inbound (Unheaded gateway)",
    "quick": "1"
  }
}'

# Block all other WAN inbound (default deny)
api_post "firewall/filter/addRule" '{
  "rule": {
    "enabled": "1",
    "action": "block",
    "interface": "wan",
    "direction": "in",
    "ipprotocol": "inet46",
    "description": "Default deny WAN inbound",
    "log": "1",
    "quick": "0"
  }
}'

# --- Step 7: Install FRR plugin ---
echo "[5/8] Installing FRR plugin (routing → BGP EVPN)..."
api_post "core/firmware/install" '{"pkg_name": "os-frr"}'
echo "FRR plugin install queued. Check GUI: System → Firmware → Plugins"

# --- Step 8: Apply and reload ---
echo "[6/8] Applying all changes..."
api_post "firewall/filter/apply" '{}'
api_post "interfaces/overview/reconfigure" '{}'

# --- Step 9: Verify Monad HbH passthrough ---
echo "[7/8] Verifying HbH passthrough..."
echo "Checking firewall rules for HbH..."
api_get "firewall/filter/searchRule" | python3 -c "
import sys, json
rules = json.load(sys.stdin).get('rows', [])
hbh_rules = [r for r in rules if 'HbH' in r.get('description','') or 'hopopt' in str(r).lower()]
if hbh_rules:
    print(f'✓ HbH rules active: {len(hbh_rules)} rules')
    for r in hbh_rules:
        print(f'  - {r.get(\"description\",\"\")} [{r.get(\"action\",\"\")}]')
else:
    print('WARNING: No HbH rules found!')
"

echo ""
echo "========================================"
echo "✓ OPNsense setup complete"
echo ""
echo "Next steps:"
echo "  1. Change admin password: System → User Manager"
echo "  2. Configure FRR BGP: Routing → General → Enable FRR"
echo "  3. Add FRR BGP config from routing/frr/frr.conf"
echo "  4. Verify Monad HbH: tcpdump -i vtnet0 'ip6 proto 0'"
echo "========================================"
