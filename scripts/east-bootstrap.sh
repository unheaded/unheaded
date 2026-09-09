#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# scripts/east-bootstrap.sh — EAST bare metal bootstrap (4-core/8GB DDR3)
#
# REFERENCE DOCS (all in ~/tmp/unheaded/):
#   references/EAST-BOOTSTRAP-CHECKLIST.md    — 6-phase checklist
#   nix/east-flake.nix                        — NixOS config for EAST
#   lxd/hosts/host-b/wireguard-ipv6.conf.example — WireGuard template
#   lxd/hosts/host-b/launch-minimal.sh        — LXD container launcher (9 svc)
#
# USAGE:
#   sudo bash scripts/east-bootstrap.sh [phase]
#   Phases: preflight | wireguard | bgp | services | observability | verify | all
#
# ASSUMPTIONS:
#   - NixOS already installed and booted on EAST hardware
#   - SSH access working (root@fd00:dead:beef::2)
#   - WEST is online at fd00:dead:beef::1
#   - Repo cloned/copied to ~/tmp/unheaded/ on EAST
set -euo pipefail

# ============================================================================
# CONFIG
# ============================================================================
REPO_DIR="${REPO_DIR:-$HOME/tmp/unheaded}"
WEST_IP="fd00:dead:beef::1"
EAST_IP="fd00:dead:beef::2"
EAST_WG_IP="fd00:dead:beef:wg::2"
WEST_WG_IP="fd00:dead:beef:wg::1"
EAST_SUBNET="fd00:dead:beef:2::/64"
EAST_AS=65002
WG_PORT=51820
WG_MTU=1380
WG_DIR="/etc/unheaded/wg"
EAST_SVC_DIR="/opt/east"

# 9 EAST services + memory budgets (total ~4.5GB of 8GB)
declare -A SERVICES=(
  [wotan]=800        # message bus
  [monad]=600        # state
  [sophia]=600       # knowledge
  [anamnesis]=400    # memory
  [gateway]=400      # public access
  [dashboard-backend]=400  # UI backend
)
# These 3 are NixOS-native (configured in east-flake.nix):
#   prometheus-agent (300MB) — port 9090
#   promtail (200MB)         — log shipper to WEST Loki
#   node-exporter (100MB)    — port 9100

# ANSI
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()      { echo -e "${GREEN}[  OK]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()    { echo -e "${RED}[FAIL]${NC} $*"; }
header()  { echo -e "\n${CYAN}═══════════════════════════════════════════${NC}"; echo -e "${CYAN}  $*${NC}"; echo -e "${CYAN}═══════════════════════════════════════════${NC}\n"; }

die() { fail "$*"; exit 1; }

# ============================================================================
# PHASE 0: PRE-FLIGHT
# ============================================================================
phase_preflight() {
  header "PHASE 0: PRE-FLIGHT CHECKS"

  # Hardware
  info "Checking hardware..."
  local cores mem_kb mem_gb
  cores=$(nproc 2>/dev/null || echo "?")
  mem_kb=$(grep MemTotal /proc/meminfo | awk '{print $2}')
  mem_gb=$(( mem_kb / 1024 / 1024 ))
  ok "CPU: ${cores} cores"
  ok "RAM: ${mem_gb}GB (${mem_kb}kB)"
  [[ ${mem_gb} -lt 7 ]] && warn "RAM < 8GB — adjust service memory limits in east-flake.nix"

  # Network
  info "Checking network interface..."
  ip -6 addr show scope global | head -5 || warn "No global IPv6 address yet"

  # Repo
  info "Checking repo at ${REPO_DIR}..."
  [[ -d "${REPO_DIR}" ]] && ok "Repo found" || die "Repo not found at ${REPO_DIR}"
  [[ -f "${REPO_DIR}/nix/east-flake.nix" ]] && ok "east-flake.nix present" || warn "east-flake.nix missing"
  [[ -f "${REPO_DIR}/lxd/hosts/host-b/wireguard-ipv6.conf.example" ]] && ok "WireGuard template present" || warn "WireGuard template missing"
  [[ -f "${REPO_DIR}/lxd/hosts/host-b/launch-minimal.sh" ]] && ok "launch-minimal.sh present" || warn "launch-minimal.sh missing"

  # Disk
  info "Checking disk space..."
  local avail
  avail=$(df -BG / | awk 'NR==2 {print $4}' | tr -d 'G')
  ok "Root filesystem: ${avail}GB available"
  [[ ${avail} -lt 10 ]] && warn "Less than 10GB free — consider cleanup"

  ok "Pre-flight complete"
}

# ============================================================================
# PHASE 1: WIREGUARD TUNNEL
# ============================================================================
phase_wireguard() {
  header "PHASE 1: WIREGUARD TUNNEL TO WEST"

  # Generate keys if not present
  mkdir -p "${WG_DIR}"
  chmod 700 "${WG_DIR}"

  if [[ ! -f "${WG_DIR}/private.key" ]]; then
    info "Generating WireGuard keypair..."
    umask 077
    wg genkey > "${WG_DIR}/private.key"
    wg pubkey < "${WG_DIR}/private.key" > "${WG_DIR}/public.key"
    ok "Keypair generated"
  else
    ok "Keypair already exists"
  fi

  local east_pub
  east_pub=$(cat "${WG_DIR}/public.key")
  echo ""
  echo -e "${YELLOW}══════════════════════════════════════════════════════════${NC}"
  echo -e "${YELLOW}  EAST PUBLIC KEY (give this to WEST):${NC}"
  echo -e "${CYAN}  ${east_pub}${NC}"
  echo -e "${YELLOW}══════════════════════════════════════════════════════════${NC}"
  echo ""

  # Check if wg0 already up
  if ip link show wg0 &>/dev/null; then
    ok "wg0 interface already exists"
    wg show wg0
  else
    info "WireGuard interface not up yet."
    info "Ensure east-flake.nix has WEST public key + endpoint, then:"
    echo "  nixos-rebuild switch"
    echo ""
    info "Or bring up manually:"
    echo "  ip link add wg0 type wireguard"
    echo "  wg set wg0 private-key ${WG_DIR}/private.key listen-port ${WG_PORT}"
    echo "  ip addr add ${EAST_WG_IP}/64 dev wg0"
    echo "  ip link set wg0 up mtu ${WG_MTU}"
    echo "  wg set wg0 peer <WEST_PUBLIC_KEY> endpoint <WEST_ENDPOINT>:${WG_PORT} allowed-ips ${WEST_WG_IP}/128,fd00:dead:beef::/32 persistent-keepalive 25"
  fi

  # Test tunnel
  echo ""
  info "Testing tunnel to WEST (${WEST_WG_IP})..."
  if ping6 -c 3 -W 3 "${WEST_WG_IP}" &>/dev/null; then
    ok "WireGuard tunnel ACTIVE — WEST reachable"
    ping6 -c 3 "${WEST_WG_IP}" 2>&1 | tail -1
  else
    warn "WEST not reachable yet — configure keys and rebuild NixOS"
    echo ""
    echo "  On WEST, add EAST peer:"
    echo "    wg set wg0 peer ${east_pub} allowed-ips ${EAST_WG_IP}/128,${EAST_SUBNET}"
    echo ""
    echo "  On EAST, set WEST key in east-flake.nix:"
    echo "    services.unheaded.wireguard.serverPublicKey = \"<WEST_PUBLIC_KEY>\";"
    echo "    services.unheaded.wireguard.serverEndpoint = \"<WEST_PUBLIC_IP>:${WG_PORT}\";"
    echo "  Then: nixos-rebuild switch"
  fi
}

# ============================================================================
# PHASE 2: BGP ROUTING (BIRD)
# ============================================================================
phase_bgp() {
  header "PHASE 2: BGP ROUTING (BIRD AS ${EAST_AS})"

  # Check BIRD running
  if systemctl is-active bird &>/dev/null; then
    ok "BIRD daemon running"
  else
    info "Starting BIRD..."
    systemctl enable bird 2>/dev/null || true
    systemctl start bird 2>/dev/null || warn "BIRD failed to start — check config"
  fi

  # Show protocols
  info "BGP protocol status:"
  if command -v birdc &>/dev/null; then
    birdc show protocols 2>/dev/null || warn "birdc show protocols failed"
    echo ""
    info "Routes received:"
    birdc show route 2>/dev/null | head -10 || warn "No routes yet"
  else
    warn "birdc not found — BIRD may not be installed"
  fi

  # Verify WEST routes
  echo ""
  info "Testing route to WEST subnet..."
  if ping6 -c 2 -W 3 "${WEST_IP}" &>/dev/null; then
    ok "WEST reachable via BGP routes"
  else
    warn "WEST subnet not reachable — BGP may not be established yet"
    echo "  Check: birdc show protocol all forge_ebgp"
  fi
}

# ============================================================================
# PHASE 3: SERVICE DEPLOYMENT
# ============================================================================
phase_services() {
  header "PHASE 3: SERVICE DEPLOYMENT (6 app services)"

  # Create directories
  mkdir -p "${EAST_SVC_DIR}"/{bin,data,logs,config}
  chown -R unheaded:unheaded "${EAST_SVC_DIR}" 2>/dev/null || true

  info "Service directory: ${EAST_SVC_DIR}"
  info "Expected binaries in ${EAST_SVC_DIR}/bin/:"

  local all_found=true
  for svc in "${!SERVICES[@]}"; do
    local mem=${SERVICES[$svc]}
    if [[ -f "${EAST_SVC_DIR}/bin/${svc}" ]]; then
      ok "  ${svc} (${mem}MB) — binary found"
    else
      warn "  ${svc} (${mem}MB) — binary MISSING"
      all_found=false
    fi
  done

  if [[ "${all_found}" == "false" ]]; then
    echo ""
    info "Copy binaries from WEST:"
    echo "  # On WEST:"
    echo "  tar czf /tmp/east-svc.tar.gz -C /opt/unheaded/bin wotan monad sophia anamnesis gateway dashboard-backend"
    echo "  scp /tmp/east-svc.tar.gz root@${EAST_IP}:${EAST_SVC_DIR}/"
    echo ""
    echo "  # On EAST:"
    echo "  tar xzf ${EAST_SVC_DIR}/east-svc.tar.gz -C ${EAST_SVC_DIR}/bin/"
    echo "  chmod +x ${EAST_SVC_DIR}/bin/*"
    return
  fi

  # Create systemd units
  info "Creating systemd service units..."
  local port=18000
  for svc in wotan monad sophia anamnesis gateway dashboard-backend; do
    local mem=${SERVICES[$svc]}
    cat > "/etc/systemd/system/east-${svc}.service" << UNIT
[Unit]
Description=EAST - ${svc}
After=network-online.target wireguard-wg0.service
Wants=network-online.target

[Service]
Type=simple
User=unheaded
ExecStart=${EAST_SVC_DIR}/bin/${svc} --listen [::1]:${port}
Restart=on-failure
RestartSec=5s
MemoryMax=${mem}M
Environment=LOG_LEVEL=info
Environment=SERVICE_NAME=${svc}
Environment=WOTAN_ADDR=[::1]:18000

[Install]
WantedBy=multi-user.target
UNIT
    ok "  east-${svc}.service (port ${port}, ${mem}MB)"
    port=$(( port + 1 ))
  done

  systemctl daemon-reload

  # Start services in order
  info "Starting services (wotan first, then rest)..."
  systemctl enable --now east-wotan 2>/dev/null && ok "wotan started" || warn "wotan failed"
  sleep 2

  for svc in monad sophia anamnesis gateway dashboard-backend; do
    systemctl enable --now "east-${svc}" 2>/dev/null && ok "${svc} started" || warn "${svc} failed"
  done

  # Health check
  echo ""
  info "Service status:"
  for svc in wotan monad sophia anamnesis gateway dashboard-backend; do
    if systemctl is-active "east-${svc}" &>/dev/null; then
      ok "  east-${svc}: active"
    else
      fail "  east-${svc}: $(systemctl is-active east-${svc} 2>/dev/null || echo 'unknown')"
    fi
  done
}

# ============================================================================
# PHASE 4: OBSERVABILITY
# ============================================================================
phase_observability() {
  header "PHASE 4: OBSERVABILITY (metrics + logs → WEST)"

  # Node exporter
  info "Checking node-exporter..."
  if systemctl is-active prometheus-node-exporter &>/dev/null; then
    ok "node-exporter running on :9100"
    curl -s "http://[::1]:9100/metrics" | head -3 && echo "  ..."
  else
    warn "node-exporter not running"
    echo "  systemctl enable --now prometheus-node-exporter"
  fi

  # Prometheus agent
  echo ""
  info "Checking prometheus..."
  if systemctl is-active prometheus &>/dev/null; then
    ok "prometheus running on :9090"
    local targets
    targets=$(curl -s "http://[::1]:9090/api/v1/targets" 2>/dev/null | jq -r '.data.activeTargets | length' 2>/dev/null || echo "?")
    ok "  Active scrape targets: ${targets}"
  else
    warn "prometheus not running"
    echo "  systemctl enable --now prometheus"
  fi

  # Promtail
  echo ""
  info "Checking promtail..."
  if systemctl is-active promtail &>/dev/null; then
    ok "promtail running — shipping logs to WEST Loki"
  else
    warn "promtail not running"
    echo "  systemctl enable --now promtail"
  fi

  # Test WEST connectivity for metrics
  echo ""
  info "Testing WEST telemetry endpoints..."
  if curl -s -o /dev/null -w "%{http_code}" "http://[${WEST_IP}]:9090/-/healthy" 2>/dev/null | grep -q 200; then
    ok "WEST Prometheus reachable"
  else
    warn "WEST Prometheus not reachable at ${WEST_IP}:9090"
  fi

  if curl -s -o /dev/null -w "%{http_code}" "http://[${WEST_IP}]:3100/ready" 2>/dev/null | grep -q 200; then
    ok "WEST Loki reachable"
  else
    warn "WEST Loki not reachable at ${WEST_IP}:3100"
  fi
}

# ============================================================================
# PHASE 5: INTEGRATION VERIFICATION
# ============================================================================
phase_verify() {
  header "PHASE 5: INTEGRATION VERIFICATION"

  local pass=0 total=0

  # WireGuard
  total=$((total+1))
  info "[1/7] WireGuard tunnel..."
  if ping6 -c 3 -W 3 "${WEST_WG_IP}" &>/dev/null; then
    ok "Tunnel active, 0% loss"; pass=$((pass+1))
  else
    fail "Tunnel DOWN"
  fi

  # BGP
  total=$((total+1))
  info "[2/7] BGP session..."
  if birdc show protocols 2>/dev/null | grep -qi "established\|up"; then
    ok "BGP ESTABLISHED"; pass=$((pass+1))
  else
    fail "BGP not established"
  fi

  # 6 app services
  total=$((total+1))
  info "[3/7] Application services..."
  local svc_up=0
  for svc in wotan monad sophia anamnesis gateway dashboard-backend; do
    systemctl is-active "east-${svc}" &>/dev/null && svc_up=$((svc_up+1))
  done
  if [[ ${svc_up} -eq 6 ]]; then
    ok "All 6 app services running"; pass=$((pass+1))
  else
    fail "${svc_up}/6 app services running"
  fi

  # Node exporter
  total=$((total+1))
  info "[4/7] Node exporter..."
  if curl -sf "http://[::1]:9100/metrics" &>/dev/null; then
    ok "Metrics endpoint responding"; pass=$((pass+1))
  else
    fail "Node exporter not responding"
  fi

  # Prometheus
  total=$((total+1))
  info "[5/7] Prometheus agent..."
  if curl -sf "http://[::1]:9090/-/healthy" &>/dev/null; then
    ok "Prometheus healthy"; pass=$((pass+1))
  else
    fail "Prometheus not healthy"
  fi

  # Promtail
  total=$((total+1))
  info "[6/7] Promtail log shipper..."
  if systemctl is-active promtail &>/dev/null; then
    ok "Promtail active"; pass=$((pass+1))
  else
    fail "Promtail not active"
  fi

  # Latency
  total=$((total+1))
  info "[7/7] End-to-end latency..."
  local avg_ms
  avg_ms=$(ping6 -c 5 -W 3 "${WEST_IP}" 2>/dev/null | grep "avg" | awk -F'/' '{printf "%.1f", $5}')
  if [[ -n "${avg_ms}" ]]; then
    ok "Avg latency: ${avg_ms}ms"; pass=$((pass+1))
  else
    fail "Could not measure latency"
  fi

  # Summary
  echo ""
  echo -e "${CYAN}═══════════════════════════════════════════${NC}"
  if [[ ${pass} -eq ${total} ]]; then
    echo -e "${GREEN}  EAST BOOTSTRAP: ${pass}/${total} CHECKS PASSED ✓${NC}"
    echo -e "${GREEN}  EAST IS PRODUCTION READY${NC}"
  else
    echo -e "${YELLOW}  EAST BOOTSTRAP: ${pass}/${total} CHECKS PASSED${NC}"
    echo -e "${YELLOW}  Review failures above${NC}"
  fi
  echo -e "${CYAN}═══════════════════════════════════════════${NC}"

  # Snapshot
  echo ""
  info "Saving bootstrap snapshot..."
  mkdir -p "${EAST_SVC_DIR}/bootstrap-snapshot"
  date > "${EAST_SVC_DIR}/bootstrap-snapshot/timestamp.txt"
  birdc show protocols > "${EAST_SVC_DIR}/bootstrap-snapshot/bgp-status.txt" 2>/dev/null || true
  ip -6 addr > "${EAST_SVC_DIR}/bootstrap-snapshot/network.txt"
  wg show wg0 > "${EAST_SVC_DIR}/bootstrap-snapshot/wireguard.txt" 2>/dev/null || true
  systemctl status east-* > "${EAST_SVC_DIR}/bootstrap-snapshot/services.txt" 2>/dev/null || true
  ok "Snapshot saved to ${EAST_SVC_DIR}/bootstrap-snapshot/"
}

# ============================================================================
# MAIN
# ============================================================================
main() {
  local phase="${1:-all}"

  echo -e "${CYAN}"
  echo "  ╔═══════════════════════════════════════════════╗"
  echo "  ║   EAST BARE METAL BOOTSTRAP                  ║"
  echo "  ║   4-core / 8GB DDR3 / NixOS                  ║"
  echo "  ║   ${EAST_IP} (AS ${EAST_AS})             ║"
  echo "  ║   ──────────────────────────────              ║"
  echo "  ║   Reference: ~/tmp/unheaded/                  ║"
  echo "  ║     references/EAST-BOOTSTRAP-CHECKLIST.md    ║"
  echo "  ║     nix/east-flake.nix                        ║"
  echo "  ║     lxd/hosts/host-b/wireguard-ipv6.conf.example  ║"
  echo "  ║     lxd/hosts/host-b/launch-minimal.sh        ║"
  echo "  ╚═══════════════════════════════════════════════╝"
  echo -e "${NC}"

  case "${phase}" in
    preflight)     phase_preflight ;;
    wireguard|wg)  phase_wireguard ;;
    bgp)           phase_bgp ;;
    services|svc)  phase_services ;;
    observability|obs) phase_observability ;;
    verify|check)  phase_verify ;;
    all)
      phase_preflight
      phase_wireguard
      phase_bgp
      phase_services
      phase_observability
      phase_verify
      ;;
    *)
      echo "Usage: $0 [preflight|wireguard|bgp|services|observability|verify|all]"
      exit 1
      ;;
  esac
}

main "$@"
