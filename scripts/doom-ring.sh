#!/usr/bin/env bash
# doom-ring.sh — Set up the 6-namespace packet circulation ring for Doom-over-IPv6
#
# Creates a directed ring of 6 network namespaces connected by veth pairs.
# Each hop runs monad_cpu at XDP, executing 1 MBC instruction per packet.
# ns0 has Shield (entry/exit point) for packet injection and collection.
#
# Topology:
#
#   ns0 --veth01--> ns1 --veth12--> ns2
#    ^                                |
#    |                                v
#   ns5 <--veth54-- ns4 <--veth43-- ns3
#
# 6 hops per circuit = 6 instructions per round-trip
# Target: ~2-3 MHz effective clock (~300k-500k circuits/sec)
#
# Usage:
#   sudo ./scripts/doom-ring.sh setup     # Create ring + load BPF
#   sudo ./scripts/doom-ring.sh teardown  # Destroy ring
#   sudo ./scripts/doom-ring.sh status    # Show ring status
#   sudo ./scripts/doom-ring.sh inject    # Inject a single test packet
#
# Prerequisites:
#   - Linux kernel >= 5.15 with eBPF + XDP support
#   - monad-cpu-ebpf built: cd ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release -p monad-cpu-ebpf
#   - shield-ebpf built:    cd ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release -p shield-ebpf
#   - bpftool, ip (iproute2)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EBPF_BIN_DIR="${PROJECT_ROOT}/ebpf/target/bpfel-unknown-none/release"
BPFFS_ROOT="/sys/fs/bpf/unheaded/doom-ring"

# Ring configuration
NUM_HOPS=6
NS_PREFIX="monad"           # namespace names: monad0..monad5
IPV6_PREFIX="fd00:monad"    # fd00:monad::N/128 per namespace
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

# ============================================================================
# SETUP
# ============================================================================

do_setup() {
    log_step "Setting up Doom packet circulation ring (${NUM_HOPS} hops)..."

    # Preflight
    if [[ $EUID -ne 0 ]]; then
        log_error "Must be root (sudo)"
        exit 1
    fi

    # Check for required tools
    for tool in ip bpftool; do
        if ! command -v "$tool" &>/dev/null; then
            log_error "Required tool not found: $tool"
            exit 1
        fi
    done

    # Mount bpffs if needed
    if ! mount | grep -q "type bpf"; then
        mount -t bpf bpf /sys/fs/bpf
    fi
    mkdir -p "${BPFFS_ROOT}"

    # Create namespaces
    log_step "Creating ${NUM_HOPS} network namespaces..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list | grep -qw "$ns"; then
            log_warn "Namespace $ns already exists, skipping"
        else
            ip netns add "$ns"
            log_info "Created namespace: $ns"
        fi

        # Enable loopback in each namespace
        ip netns exec "$ns" ip link set lo up
    done

    # Create veth pairs forming a directed ring.
    # For each hop i, create vethXY connecting nsX → nsY
    # where Y = (X + 1) % NUM_HOPS
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

        # Create veth pair in the default namespace, then move ends
        ip link add "$veth_src" type veth peer name "$veth_dst"
        ip link set "$veth_src" netns "$ns_src"
        ip link set "$veth_dst" netns "$ns_dst"

        # Configure IPv6 addresses
        #   Source side: fd00:monad::i:1/64
        #   Dest side:   fd00:monad::i:2/64
        ip netns exec "$ns_src" ip -6 addr add "${IPV6_PREFIX}::${i}:1/64" dev "$veth_src"
        ip netns exec "$ns_dst" ip -6 addr add "${IPV6_PREFIX}::${i}:2/64" dev "$veth_dst"

        # Set MTU and bring up
        ip netns exec "$ns_src" ip link set "$veth_src" mtu "$MTU" up
        ip netns exec "$ns_dst" ip link set "$veth_dst" mtu "$MTU" up

        # Disable IPv6 duplicate address detection (speeds up startup)
        ip netns exec "$ns_src" sysctl -qw "net.ipv6.conf.${veth_src}.accept_dad=0"
        ip netns exec "$ns_dst" sysctl -qw "net.ipv6.conf.${veth_dst}.accept_dad=0"

        log_info "veth: ${ns_src}/${veth_src} <-> ${ns_dst}/${veth_dst}"
    done

    # Add static routes to create the directed ring.
    # Each namespace routes "next hop" traffic to the outgoing veth.
    log_step "Configuring static IPv6 routes for circulation..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        j=$(( (i + 1) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        veth_out="veth${i}${j}"

        # Default route in each namespace points to the next hop.
        # The peer address on the outgoing veth is fd00:monad::i:2
        ip netns exec "$ns" ip -6 route replace default \
            via "${IPV6_PREFIX}::${i}:2" dev "$veth_out" 2>/dev/null || \
        ip netns exec "$ns" ip -6 route add default \
            via "${IPV6_PREFIX}::${i}:2" dev "$veth_out" 2>/dev/null || true

        log_info "Route: ${ns} -> ${NS_PREFIX}${j} via ${veth_out}"
    done

    # Enable IPv6 forwarding in all namespaces
    log_step "Enabling IPv6 forwarding..."
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        ip netns exec "$ns" sysctl -qw net.ipv6.conf.all.forwarding=1
    done

    # Load BPF programs
    log_step "Loading BPF programs..."
    load_bpf_programs

    log_info "Doom ring setup complete!"
    echo ""
    do_status
}

# ============================================================================
# LOAD BPF PROGRAMS
# ============================================================================

load_bpf_programs() {
    local monad_cpu_bin="${EBPF_BIN_DIR}/monad-cpu-ebpf"
    local shield_bin="${EBPF_BIN_DIR}/shield-ebpf"

    # Check binaries exist
    if [[ ! -f "$monad_cpu_bin" ]]; then
        log_warn "monad-cpu-ebpf binary not found at ${monad_cpu_bin}"
        log_warn "Build with: cd ${PROJECT_ROOT}/ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release -p monad-cpu-ebpf"
        log_warn "Skipping BPF loading (ring topology is still configured)"
        return 0
    fi

    # Load monad_cpu on each hop's incoming veth interface
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        prev=$(( (i - 1 + NUM_HOPS) % NUM_HOPS ))
        ns="${NS_PREFIX}${i}"
        # Incoming interface is the peer end of the previous hop's veth
        veth_in="veth${prev}${i}p"

        mkdir -p "${BPFFS_ROOT}/hop${i}"

        if bpftool prog load "$monad_cpu_bin" \
            "${BPFFS_ROOT}/hop${i}/monad_cpu" \
            pinmaps "${BPFFS_ROOT}/hop${i}" 2>/dev/null; then

            # Attach to XDP on the incoming interface
            ip netns exec "$ns" bpftool net attach xdp \
                pinned "${BPFFS_ROOT}/hop${i}/monad_cpu" \
                dev "$veth_in" 2>/dev/null && \
                log_info "Hop ${i}: monad_cpu loaded on ${ns}/${veth_in}" || \
                log_warn "Hop ${i}: loaded but XDP attach failed on ${veth_in}"
        else
            log_warn "Hop ${i}: bpftool load failed for monad_cpu"
        fi
    done

    # Load Shield on ns0's outgoing interface (entry/exit point)
    if [[ -f "$shield_bin" ]]; then
        local ns0_veth_out="veth01"
        mkdir -p "${BPFFS_ROOT}/shield"

        if bpftool prog load "$shield_bin" \
            "${BPFFS_ROOT}/shield/prog" \
            pinmaps "${BPFFS_ROOT}/shield" 2>/dev/null; then

            ip netns exec "${NS_PREFIX}0" bpftool net attach xdp \
                pinned "${BPFFS_ROOT}/shield/prog" \
                dev "$ns0_veth_out" 2>/dev/null && \
                log_info "Shield loaded on ${NS_PREFIX}0/${ns0_veth_out}" || \
                log_warn "Shield loaded but XDP attach failed"
        else
            log_warn "Shield bpftool load failed"
        fi
    else
        log_warn "shield-ebpf binary not found, skipping Shield"
    fi
}

# ============================================================================
# TEARDOWN
# ============================================================================

do_teardown() {
    log_step "Tearing down Doom packet circulation ring..."

    if [[ $EUID -ne 0 ]]; then
        log_error "Must be root (sudo)"
        exit 1
    fi

    # Remove BPF pins
    if [[ -d "${BPFFS_ROOT}" ]]; then
        rm -rf "${BPFFS_ROOT}"
        log_info "Removed BPF pins: ${BPFFS_ROOT}"
    fi

    # Delete namespaces (this also removes all veth pairs)
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list | grep -qw "$ns"; then
            ip netns del "$ns"
            log_info "Deleted namespace: $ns"
        fi
    done

    log_info "Teardown complete"
}

# ============================================================================
# STATUS
# ============================================================================

do_status() {
    echo -e "${CYAN}=== Doom Ring Status ===${NC}"
    echo ""

    # Check namespaces
    echo -e "${BLUE}Namespaces:${NC}"
    local ns_count=0
    for i in $(seq 0 $((NUM_HOPS - 1))); do
        ns="${NS_PREFIX}${i}"
        if ip netns list 2>/dev/null | grep -qw "$ns"; then
            echo -e "  ${GREEN}[UP]${NC}   $ns"
            ((ns_count++))
        else
            echo -e "  ${RED}[DOWN]${NC} $ns"
        fi
    done
    echo ""

    if [[ $ns_count -eq 0 ]]; then
        echo "  Ring not configured. Run: sudo $0 setup"
        return
    fi

    # Check veth connectivity
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

    # Check BPF programs
    echo -e "${BLUE}BPF Programs:${NC}"
    if [[ -d "${BPFFS_ROOT}" ]]; then
        for i in $(seq 0 $((NUM_HOPS - 1))); do
            if [[ -f "${BPFFS_ROOT}/hop${i}/monad_cpu" ]]; then
                echo -e "  ${GREEN}[LOADED]${NC} hop${i}/monad_cpu"
            else
                echo -e "  ${YELLOW}[NONE]${NC}   hop${i}"
            fi
        done
        if [[ -f "${BPFFS_ROOT}/shield/prog" ]]; then
            echo -e "  ${GREEN}[LOADED]${NC} shield (ns0 entry)"
        else
            echo -e "  ${YELLOW}[NONE]${NC}   shield"
        fi
    else
        echo "  No BPF programs loaded"
    fi
    echo ""

    # Show ring topology diagram
    echo -e "${BLUE}Ring Topology:${NC}"
    echo ""
    echo "  ${NS_PREFIX}0 --veth01--> ${NS_PREFIX}1 --veth12--> ${NS_PREFIX}2"
    echo "   ^                                   |"
    echo "   |                                   v"
    echo "  ${NS_PREFIX}5 <--veth54-- ${NS_PREFIX}4 <--veth43-- ${NS_PREFIX}3"
    echo ""
    echo "  6 instructions per circuit (1 per hop)"
    echo ""
}

# ============================================================================
# INJECT TEST PACKET
# ============================================================================

do_inject() {
    log_step "Injecting test packet into the ring..."

    if [[ $EUID -ne 0 ]]; then
        log_error "Must be root (sudo)"
        exit 1
    fi

    # Verify ns0 exists
    if ! ip netns list | grep -qw "${NS_PREFIX}0"; then
        log_error "Ring not set up. Run: sudo $0 setup"
        exit 1
    fi

    # Send an IPv6 UDP packet from ns0 that will circulate the ring.
    # The destination is ns0's own address via the ring path.
    # Shield on ns0 adds the Monad Hop-by-Hop header.
    ip netns exec "${NS_PREFIX}0" \
        python3 -c "
import socket
import struct

# Create IPv6 UDP socket
sock = socket.socket(socket.AF_INET6, socket.SOCK_DGRAM)

# Send to next hop (ns1) — the ring will circulate it back
# The packet content is a simple 'TICK' marker
dst = '${IPV6_PREFIX}::0:2'
port = 31337  # Doom ring control port

# Build a minimal payload with circuit metadata
# [magic:4][tick_count:4][circuit_id:4]
payload = struct.pack('!III', 0x4D4F4E41, 0, 1)  # 'MONA', tick 0, circuit 1

sock.sendto(payload, (dst, port))
print(f'Sent MONAD tick packet to [{dst}]:{port}')
sock.close()
" 2>/dev/null && log_info "Test packet injected" || log_error "Injection failed"
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
        do_inject
        ;;
    help|--help|-h)
        echo "Usage: sudo $0 {setup|teardown|status|inject}"
        echo ""
        echo "Commands:"
        echo "  setup      Create 6-namespace ring and load BPF programs"
        echo "  teardown   Destroy ring and remove BPF pins"
        echo "  status     Show ring status"
        echo "  inject     Inject a test packet into the ring"
        ;;
    *)
        log_error "Unknown command: $1"
        echo "Usage: sudo $0 {setup|teardown|status|inject}"
        exit 1
        ;;
esac
