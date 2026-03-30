#!/usr/bin/env bash
# doom-ring.sh — 6-namespace packet circulation ring for Doom-over-IPv6
#
# Creates a directed ring of 6 network namespaces connected by veth pairs.
# Each hop runs monad_cpu at XDP, executing 1 MBC instruction per packet.
# ALL hops share the same BPF maps (ROM, CPU, RAM, etc.) via pinned maps.
#
# Topology:
#
#   ns0 --veth01--> ns1 --veth12--> ns2
#    ^                                |
#    |                                v
#   ns5 <--veth54-- ns4 <--veth43-- ns3
#
# 6 hops per circuit = 6 instructions per round-trip.
# The wire is the processor. Wotan is the RAM.
#
# Usage:
#   sudo ./scripts/doom-ring.sh setup     # Create ring + load BPF
#   sudo ./scripts/doom-ring.sh teardown  # Destroy ring
#   sudo ./scripts/doom-ring.sh status    # Show ring status
#   sudo ./scripts/doom-ring.sh inject [flow-label]  # Inject one packet
#
# Prerequisites:
#   - Linux kernel >= 5.15 with eBPF + XDP support
#   - monad-cpu-ebpf built (release):
#     cargo +nightly build -p monad-cpu-ebpf --target bpfel-unknown-none \
#       -Z build-std=core --release --manifest-path ebpf/Cargo.toml
#   - ebpf-loader built: cargo build --manifest-path cmd/ebpf-loader/Cargo.toml --release
#   - ip (iproute2), python3

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EBPF_BIN_DIR="${PROJECT_ROOT}/ebpf/target/bpfel-unknown-none/release"
LOADER_BIN="${PROJECT_ROOT}/cmd/ebpf-loader/target/release/ebpf-loader"
BPFFS_ROOT="/sys/fs/bpf/unheaded/doom-ring"
MAP_PIN_DIR="${BPFFS_ROOT}/maps"

# Ring configuration
NUM_HOPS=${DOOM_RING_HOPS:-2}
NS_PREFIX="monad"
IPV6_PREFIX="fd00:3f:75"   # Kingdom ULA prefix (per draft-bellis-02 §7)
MTU=1500

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }

require_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "Must be root (sudo)"
        exit 1
    fi
}

# ============================================================================
# SETUP
# ============================================================================

do_setup() {
    log_step "Setting up Doom packet circulation ring (${NUM_HOPS} hops)..."
    require_root

    for tool in ip python3 nsenter; do
        if ! command -v "$tool" &>/dev/null; then
            log_error "Required tool not found: $tool"
            exit 1
        fi
    done

    # Check ebpf-loader binary
    if [[ ! -f "$LOADER_BIN" ]]; then
        log_error "ebpf-loader not found at ${LOADER_BIN}"
        log_error "Build it first: cargo build --manifest-path cmd/ebpf-loader/Cargo.toml --release"
        exit 1
    fi

    # Mount bpffs if needed
    if ! mount | grep -q "type bpf"; then
        mount -t bpf bpf /sys/fs/bpf
        log_info "Mounted bpffs at /sys/fs/bpf"
    fi
    mkdir -p "${BPFFS_ROOT}" "${MAP_PIN_DIR}"

    # ── Create namespaces ─────────────────────────────────────────────────
    log_step "Creating ${NUM_HOPS} network namespaces..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list | grep -qw "$ns"; then
            log_warn "Namespace $ns already exists, skipping"
        else
            ip netns add "$ns"
            log_info "Created namespace: $ns"
        fi
        ip netns exec "$ns" ip link set lo up
    done

    # ── Create veth pairs ─────────────────────────────────────────────────
    log_step "Creating veth pairs (directed ring)..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        j=$(( (i + 1) % NUM_HOPS ))
        ns_src="${NS_PREFIX}${i}"
        ns_dst="${NS_PREFIX}${j}"
        veth_src="veth${i}${j}"
        veth_dst="veth${i}${j}p"

        if ip netns exec "$ns_src" ip link show "$veth_src" &>/dev/null 2>&1; then
            log_warn "veth pair ${veth_src} already exists, skipping"
            continue
        fi

        ip link add "$veth_src" type veth peer name "$veth_dst"
        ip link set "$veth_src" netns "$ns_src"
        ip link set "$veth_dst" netns "$ns_dst"

        # IPv6 addresses
        ip netns exec "$ns_src" ip -6 addr add "${IPV6_PREFIX}:${i}::1/64" dev "$veth_src"
        ip netns exec "$ns_dst" ip -6 addr add "${IPV6_PREFIX}:${i}::2/64" dev "$veth_dst"

        # MTU + up
        ip netns exec "$ns_src" ip link set "$veth_src" mtu "$MTU" up
        ip netns exec "$ns_dst" ip link set "$veth_dst" mtu "$MTU" up

        # Disable DAD (speeds up startup)
        ip netns exec "$ns_src" sysctl -qw "net.ipv6.conf.${veth_src}.accept_dad=0"
        ip netns exec "$ns_dst" sysctl -qw "net.ipv6.conf.${veth_dst}.accept_dad=0"

        log_info "veth: ${ns_src}/${veth_src} <-> ${ns_dst}/${veth_dst}"
    done

    # ── Static routes for circulation ─────────────────────────────────────
    log_step "Configuring static IPv6 routes..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        j=$(( (i + 1) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        veth_out="veth${i}${j}"

        ip netns exec "$ns" ip -6 route replace default \
            via "${IPV6_PREFIX}:${i}::2" dev "$veth_out" 2>/dev/null || \
        ip netns exec "$ns" ip -6 route add default \
            via "${IPV6_PREFIX}:${i}::2" dev "$veth_out" 2>/dev/null || true

        log_info "Route: ${ns} -> ${NS_PREFIX}${j} via ${veth_out}"
    done

    # ── Enable IPv6 forwarding ────────────────────────────────────────────
    log_step "Enabling IPv6 forwarding..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        ip netns exec "$ns" sysctl -qw net.ipv6.conf.all.forwarding=1
    done

    # ── Add static neighbor entries for reliable forwarding ───────────────
    log_step "Adding static neighbor entries..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        j=$(( (i + 1) % NUM_HOPS ))
        ns_src="${NS_PREFIX}${i}"
        ns_dst="${NS_PREFIX}${j}"
        veth_src="veth${i}${j}"
        veth_dst="veth${i}${j}p"

        # Get MAC address of the peer interface
        peer_mac=$(ip netns exec "$ns_dst" ip link show "$veth_dst" 2>/dev/null | \
            awk '/ether/ {print $2}')
        if [[ -n "$peer_mac" ]]; then
            ip netns exec "$ns_src" ip -6 neigh replace "${IPV6_PREFIX}:${i}::2" \
                lladdr "$peer_mac" dev "$veth_src" nud permanent 2>/dev/null || true
        fi
    done

    # ── Load BPF programs (shared maps) ───────────────────────────────────
    log_step "Loading BPF programs..."
    load_bpf_programs

    log_info "Doom ring setup complete!"
    echo ""
    do_status
}

# ============================================================================
# LOAD BPF PROGRAMS — shared maps between all hops
# ============================================================================

load_bpf_programs() {
    local monad_cpu_bin="${EBPF_BIN_DIR}/monad-cpu-ebpf"

    if [[ ! -f "$monad_cpu_bin" ]]; then
        log_warn "monad-cpu-ebpf binary not found at ${monad_cpu_bin}"
        log_warn "Build: cargo +nightly build -p monad-cpu-ebpf --target bpfel-unknown-none -Z build-std=core --release --manifest-path ebpf/Cargo.toml"
        log_warn "Skipping BPF loading"
        return 0
    fi

    if [[ ! -x "$LOADER_BIN" ]]; then
        log_error "ebpf-loader not found at ${LOADER_BIN}"
        log_error "Build: cargo build --manifest-path cmd/ebpf-loader/Cargo.toml --release"
        return 1
    fi

    if ! command -v bpftool &>/dev/null; then
        log_error "bpftool not found — required for shared-program XDP attachment"
        return 1
    fi

    mkdir -p "${MAP_PIN_DIR}" "/run/doom-ring"

    # ── Strategy: load ONCE, attach EVERYWHERE ───────────────────────────
    #
    # aya-ebpf 0.1.x emits legacy "maps" ELF section.  EbpfLoader::map_pin_path()
    # does NOT reuse pinned legacy maps — each load creates independent maps.
    # To guarantee all 6 hops share the SAME maps (ROM/CPU/RAM/SCREEN/etc.),
    # we load the program exactly once (hop 0), pin the maps, then attach the
    # SAME program instance to the other 5 interfaces via bpftool.
    #
    # Hop 0: aya loader → creates program + maps, pins maps, holds link FD
    #        (background process, link-based XDP ownership)
    #
    # Hops 1-5: bpftool net attach xdpgeneric id <prog_id> dev <iface>
    #           (legacy netlink attachment, persists without FD holder)
    #
    # Result: 1 program, 1 set of maps, 6 XDP attachment points.

    # ── Strategy: load ONCE on hop0, attach SAME program to remaining hops ──
    #
    # The aya loader creates the program + maps and holds a link-based XDP
    # attachment on hop0. It MUST stay alive as a daemon — killing it releases
    # the link and detaches XDP. Use nohup + file redirect (NOT pipe) to keep
    # it alive after the script exits.
    #
    # Hops 1+: bpftool attaches the SAME program by ID (netlink-based,
    # persists without the loader process).

    local ns0="${NS_PREFIX}0"
    local veth0="veth$((NUM_HOPS - 1))0p"
    local pid_file="/run/doom-ring/hop0.pid"

    log_info "Hop 0 (primary): loading monad_cpu on ${ns0}/${veth0}..."

    # Use setsid to fully daemonize. nohup alone doesn't survive nsenter exit.
    setsid nsenter --net="/var/run/netns/${ns0}" \
        "$LOADER_BIN" \
        --only monad-cpu \
        --obj-dir "${EBPF_BIN_DIR}" \
        --interface "$veth0" \
        --map-pin-path "${MAP_PIN_DIR}" \
        --xdp-skb-mode \
        --pid-file "$pid_file" \
        < /dev/null > /tmp/doom-ring-loader.log 2>&1 &

    # Wait for loader to start, verify, JIT, and attach XDP (up to 15s).
    local wait_total=0
    while [[ ! -f "$pid_file" ]] && [[ $wait_total -lt 15 ]]; do
        sleep 1
        wait_total=$((wait_total + 1))
        log_info "Waiting for BPF verifier... (${wait_total}s)"
    done

    if [[ -f "$pid_file" ]]; then
        local pid0
        pid0=$(cat "$pid_file")
        if kill -0 "$pid0" 2>/dev/null; then
            log_info "Hop 0: monad_cpu attached on ${ns0}/${veth0} (PID ${pid0})"
        else
            log_error "Hop 0: loader exited prematurely (verifier rejection?)"
            cat /tmp/doom-ring-loader.log 2>/dev/null | tail -5
            return 1
        fi
    else
        log_error "Hop 0: loader failed to start within 15s"
        cat /tmp/doom-ring-loader.log 2>/dev/null | tail -10
        return 1
    fi

    # Get the program ID — use tail -1 to get the NEWEST if multiples exist
    local prog_id
    prog_id=$(bpftool prog list 2>/dev/null | grep "name monad_cpu" | tail -1 | awk '{print $1}' | tr -d ':')
    if [[ -z "$prog_id" ]]; then
        log_error "Could not find monad_cpu program ID via bpftool"
        return 1
    fi
    log_info "monad_cpu program id=${prog_id} — attaching to hops 1-$((NUM_HOPS - 1))"

    # Attach remaining hops via bpftool (netlink, shares same program + maps)
    for i in $(seq 1 $((NUM_HOPS - 1))); do
        local prev=$(( (i - 1 + NUM_HOPS) % NUM_HOPS ))
        local ns="${NS_PREFIX}${i}"
        local veth_in="veth${prev}${i}p"

        log_info "Hop ${i}: attaching prog ${prog_id} to ${ns}/${veth_in}..."

        if ! nsenter --net="/var/run/netns/${ns}" \
            bpftool net attach xdpgeneric id "$prog_id" dev "$veth_in" 2>&1; then
            log_error "Hop ${i}: bpftool attach failed"
            return 1
        fi
        log_info "Hop ${i}: monad_cpu attached on ${ns}/${veth_in} (shared prog ${prog_id})"
    done

    log_info "All ${NUM_HOPS} hops loaded — stable netlink XDP, no background process needed"
}

# ============================================================================
# TEARDOWN
# ============================================================================

do_teardown() {
    log_step "Tearing down Doom packet circulation ring..."
    require_root

    # Kill hop0 loader (releases link-based XDP on hop0)
    if [[ -d "/run/doom-ring" ]]; then
        for pid_file in /run/doom-ring/hop*.pid; do
            if [[ -f "$pid_file" ]]; then
                local pid
                pid=$(cat "$pid_file")
                if kill -0 "$pid" 2>/dev/null; then
                    kill "$pid" 2>/dev/null || true
                    log_info "Killed loader PID $pid ($(basename "$pid_file" .pid))"
                fi
                rm -f "$pid_file"
            fi
        done
        sleep 0.5
    fi

    # Detach XDP from all hops (hops 1-5 use netlink attachment, must be explicit)
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        prev=$(( (i - 1 + NUM_HOPS) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        veth_in="veth${prev}${i}p"
        if [[ -f "/var/run/netns/${ns}" ]]; then
            nsenter --net="/var/run/netns/${ns}" \
                ip link set "$veth_in" xdp off 2>/dev/null || true
        fi
    done

    # Remove BPF pins
    if [[ -d "${BPFFS_ROOT}" ]]; then
        rm -rf "${BPFFS_ROOT}"
        log_info "Removed BPF pins: ${BPFFS_ROOT}"
    fi

    # Delete namespaces (also removes veth pairs)
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list | grep -qw "$ns"; then
            ip netns del "$ns"
            log_info "Deleted namespace: $ns"
        fi
    done

    # Clean up runtime dir
    rm -rf /run/doom-ring

    log_info "Teardown complete"
}

# ============================================================================
# STATUS
# ============================================================================

do_status() {
    echo -e "${CYAN}=== Doom Ring Status ===${NC}"
    echo ""

    echo -e "${BLUE}Namespaces:${NC}"
    local ns_count=0
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list 2>/dev/null | grep -qw "$ns"; then
            echo -e "  ${GREEN}[UP]${NC}   $ns"
            ((ns_count++)) || true
        else
            echo -e "  ${RED}[DOWN]${NC} $ns"
        fi
    done
    echo ""

    if [[ $ns_count -eq 0 ]]; then
        echo "  Ring not configured. Run: sudo $0 setup"
        return
    fi

    echo -e "${BLUE}Veth Links:${NC}"
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        j=$(( (i + 1) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        veth="veth${i}${j}"
        if ip netns exec "$ns" ip link show "$veth" &>/dev/null 2>&1; then
            state=$(ip netns exec "$ns" ip -br link show "$veth" 2>/dev/null | awk '{print $2}')
            echo -e "  ${GREEN}[${state}]${NC} ${ns}/${veth} -> ${NS_PREFIX}${j}"
        else
            echo -e "  ${RED}[MISSING]${NC} ${ns}/${veth}"
        fi
    done
    echo ""

    echo -e "${BLUE}BPF Programs (shared maps):${NC}"
    if [[ -d "${MAP_PIN_DIR}" ]]; then
        echo -e "  Map pin dir: ${MAP_PIN_DIR}"
        for mname in ROM_MAP CPU_MAP RAM_MAP SCREEN_MAP KBD_MAP STATS L1_CACHE COMPUTE_EVENTS; do
            if [[ -f "${MAP_PIN_DIR}/${mname}" ]]; then
                echo -e "  ${GREEN}[PINNED]${NC} ${mname}"
            else
                echo -e "  ${YELLOW}[NONE]${NC}   ${mname}"
            fi
        done
    else
        echo "  No BPF programs loaded"
    fi
    echo ""

    echo -e "${BLUE}XDP attachments:${NC}"
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        prev=$(( (i - 1 + NUM_HOPS) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        veth_in="veth${prev}${i}p"
        if [[ -f "/var/run/netns/${ns}" ]]; then
            xdp_info=$(nsenter --net="/var/run/netns/${ns}" \
                ip link show "$veth_in" 2>/dev/null | grep -o "xdp[^ ]*" || true)
            if [[ -n "$xdp_info" ]]; then
                echo -e "  ${GREEN}[XDP]${NC}  hop${i}: ${ns}/${veth_in}"
            else
                echo -e "  ${YELLOW}[NONE]${NC} hop${i}: ${ns}/${veth_in}"
            fi
        else
            echo -e "  ${RED}[NS?]${NC}  hop${i}: namespace ${ns} missing"
        fi
    done
    echo ""

    echo -e "${BLUE}Ring Topology:${NC}"
    echo ""
    echo "  ${NS_PREFIX}0 --veth01--> ${NS_PREFIX}1 --veth12--> ${NS_PREFIX}2"
    echo "   ^                                   |"
    echo "   |                                   v"
    echo "  ${NS_PREFIX}5 <--veth54-- ${NS_PREFIX}4 <--veth43-- ${NS_PREFIX}3"
    echo ""
    echo "  1 instruction per hop, 6 per circuit"
    echo "  Maps shared: all hops see same ROM/CPU/RAM state"
    echo ""
}

# ============================================================================
# INJECT — craft and inject IPv6 + HBH + Monad packet
# ============================================================================

do_inject() {
    local flow_label="${2:-0xDE}"
    log_step "Injecting Monad CPU tick packet (flow_label=${flow_label})..."
    require_root

    if ! ip netns list | grep -qw "${NS_PREFIX}0"; then
        log_error "Ring not set up. Run: sudo $0 setup"
        exit 1
    fi

    # Get the MAC address of veth01p (peer in ns1) and veth01 (in ns0)
    local src_mac
    src_mac=$(ip netns exec "${NS_PREFIX}0" ip link show veth01 2>/dev/null | \
        awk '/ether/ {print $2}')
    local dst_mac
    dst_mac=$(ip netns exec "${NS_PREFIX}1" ip link show veth01p 2>/dev/null | \
        awk '/ether/ {print $2}')

    if [[ -z "$src_mac" || -z "$dst_mac" ]]; then
        log_error "Could not determine MAC addresses for veth01/veth01p"
        exit 1
    fi

    # Inject raw IPv6 + HBH + Monad packet via AF_PACKET
    ip netns exec "${NS_PREFIX}0" python3 - "$flow_label" "$src_mac" "$dst_mac" <<'PYEOF'
import socket
import struct
import sys

flow_label = int(sys.argv[1], 0)
src_mac_str = sys.argv[2]
dst_mac_str = sys.argv[3]

def mac_bytes(mac_str):
    return bytes(int(b, 16) for b in mac_str.split(':'))

src_mac = mac_bytes(src_mac_str)
dst_mac = mac_bytes(dst_mac_str)

# ── Ethernet header (14 bytes) ───────────────────────────────────────────
eth = dst_mac + src_mac + struct.pack('!H', 0x86DD)

# ── IPv6 header (40 bytes) ───────────────────────────────────────────────
version_tc_fl = (6 << 28) | (0 << 20) | (flow_label & 0xFFFFF)
payload_len = 24  # HBH extension header = 24 bytes
next_header = 0   # Hop-by-Hop Options
hop_limit = 255

# Source: fd00:3f:75:0::1 (monad0 address)
src_addr = bytes([0xfd, 0x00, 0x00, 0x3f, 0x00, 0x75, 0x00, 0x00,
                  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])
# Destination: fd00:dead::1 (outside all connected /64 prefixes — forces default-route forwarding)
dst_addr = bytes([0xfd, 0x00, 0xde, 0xad, 0x00, 0x00, 0x00, 0x00,
                  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01])

ipv6 = struct.pack('!IHBB', version_tc_fl, payload_len, next_header, hop_limit)
ipv6 += src_addr + dst_addr

# ── Hop-by-Hop Options header (24 bytes) ─────────────────────────────────
# next_header=59 (No Next Header), hdr_ext_len=2 (=24 total bytes)
# opt_type=0x3E (MONAD_OPT_TYPE), opt_data_len=20
hbh_prefix = struct.pack('BBBB', 59, 2, 0x3E, 20)

# ── Monad (20 bytes) ─────────────────────────────────────────────────────
# version=0x01, src_svc=0, dst_svc=0, hop_count=0,
# qos=0, flow_action=0, circuit_state=0, flags=0x02 (CUSTOM),
# latency_hint=0, deploy_ring=0, mesh_flags=0, src_prefix=0, dst_prefix=0,
# scratch=[0,0,0,0], checksum=[0,0]
monad = struct.pack('BBBBBBBB', 0x01, 0, 0, 0, 0, 0, 0, 0x02)  # 8 bytes
monad += struct.pack('!H', 0)  # latency_hint (2 bytes)
monad += struct.pack('BBBB', 0, 0, 0, 0)  # deploy_ring, mesh_flags, src_prefix, dst_prefix
monad += struct.pack('BBBB', 0, 0, 0, 0)  # scratch[0..3]
monad += struct.pack('BB', 0, 0)           # checksum[0..1]

assert len(monad) == 20, f"Monad size mismatch: {len(monad)}"

packet = eth + ipv6 + hbh_prefix + monad

# Send via AF_PACKET raw socket on veth01
sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
sock.bind(('veth01', 0))
sock.send(packet)
sock.close()

print(f'Injected {len(packet)} byte Monad CPU tick packet (flow_label=0x{flow_label:05X})')
PYEOF

    log_info "Packet injected into ${NS_PREFIX}0/veth01"
}

# ============================================================================
# DUMP — dump BPF map state for debugging
# ============================================================================

do_dump() {
    require_root

    if [[ ! -d "${MAP_PIN_DIR}" ]]; then
        log_error "Maps not pinned. Run: sudo $0 setup"
        exit 1
    fi

    echo -e "${CYAN}=== BPF Map Dump ===${NC}"
    echo ""

    echo -e "${BLUE}CPU_MAP:${NC}"
    bpftool map dump pinned "${MAP_PIN_DIR}/CPU_MAP" 2>&1 || echo "  (empty or error)"
    echo ""

    echo -e "${BLUE}STATS:${NC}"
    bpftool map dump pinned "${MAP_PIN_DIR}/STATS" 2>&1 || echo "  (empty or error)"
    echo ""

    echo -e "${BLUE}ROM_MAP (first 10 entries):${NC}"
    bpftool map dump pinned "${MAP_PIN_DIR}/ROM_MAP" 2>&1 | head -60 || echo "  (empty or error)"
    echo ""
}

# ============================================================================
# MAIN
# ============================================================================

case "${1:-help}" in
    setup)
        do_setup
        ;;
    teardown)
        do_teardown
        ;;
    status)
        do_status
        ;;
    inject)
        do_inject "$@"
        ;;
    dump)
        do_dump
        ;;
    help|--help|-h)
        echo "Usage: sudo $0 {setup|teardown|status|inject|dump}"
        echo ""
        echo "Commands:"
        echo "  setup                Create 6-namespace ring + load BPF (shared maps)"
        echo "  teardown             Destroy ring and remove BPF pins"
        echo "  status               Show ring status"
        echo "  inject [flow-label]  Inject a Monad CPU tick packet (default: 0xDE)"
        echo "  dump                 Dump BPF map state"
        ;;
    *)
        log_error "Unknown command: $1"
        echo "Usage: sudo $0 {setup|teardown|status|inject|dump}"
        exit 1
        ;;
esac
