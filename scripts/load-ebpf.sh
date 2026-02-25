#!/usr/bin/env bash
# load-ebpf.sh — Load Unheaded eBPF programs into the kernel
#
# This script loads all 4 eBPF programs and pins their maps
# to /sys/fs/bpf/unheaded/ for the trace-collector to read.
#
# Usage: sudo ./scripts/load-ebpf.sh [--dry-run] [--verbose]
#
# Programs loaded:
#   packet-marker  — XDP trace ID injection
#   flow-tracker   — TC connection tracking
#   latency-probe  — kprobe/kretprobe RTT measurement
#   syscall-tracer — raw tracepoint syscall tracing
#
# Prerequisites:
#   - Linux kernel >= 5.15
#   - eBPF programs built: cd ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release
#   - bpftool installed (apt install linux-tools-common)

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
EBPF_BIN_DIR="${PROJECT_ROOT}/ebpf/target/bpfel-unknown-none/release"
BPFFS_ROOT="/sys/fs/bpf/unheaded"
INTERFACE="${UNHEADED_INTERFACE:-eth0}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Flags
DRY_RUN=false
VERBOSE=false

# Parse arguments
for arg in "$@"; do
    case "$arg" in
        --dry-run)  DRY_RUN=true ;;
        --verbose)  VERBOSE=true ;;
        --help|-h)
            echo "Usage: sudo $0 [--dry-run] [--verbose]"
            echo ""
            echo "Options:"
            echo "  --dry-run   Show what would be done without loading programs"
            echo "  --verbose   Enable verbose output"
            echo ""
            echo "Environment:"
            echo "  UNHEADED_INTERFACE  Network interface for XDP/TC (default: eth0)"
            exit 0
            ;;
        *)
            echo "Unknown argument: $arg"
            exit 1
            ;;
    esac
done

log_info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }
log_debug() { $VERBOSE && echo -e "[DEBUG] $*" || true; }

run_cmd() {
    if $DRY_RUN; then
        echo "  [dry-run] $*"
    else
        log_debug "Running: $*"
        "$@"
    fi
}

# ============================================================================
# PREFLIGHT CHECKS
# ============================================================================

log_step "Preflight checks..."

# Must be root
if [[ $EUID -ne 0 ]] && ! $DRY_RUN; then
    log_error "This script must be run as root (sudo)"
    exit 1
fi

# Check kernel version
KERNEL_VERSION=$(uname -r | cut -d. -f1-2)
KERNEL_MAJOR=$(echo "$KERNEL_VERSION" | cut -d. -f1)
KERNEL_MINOR=$(echo "$KERNEL_VERSION" | cut -d. -f2)
if [[ "$KERNEL_MAJOR" -lt 5 ]] || { [[ "$KERNEL_MAJOR" -eq 5 ]] && [[ "$KERNEL_MINOR" -lt 15 ]]; }; then
    log_error "Kernel >= 5.15 required (found: $(uname -r))"
    exit 1
fi
log_info "Kernel version: $(uname -r)"

# Check eBPF binaries exist
PROGRAMS=(packet-marker flow-tracker latency-probe syscall-tracer)
MISSING=()
for prog in "${PROGRAMS[@]}"; do
    if [[ ! -f "${EBPF_BIN_DIR}/${prog}" ]]; then
        MISSING+=("$prog")
    fi
done

if [[ ${#MISSING[@]} -gt 0 ]]; then
    log_error "Missing eBPF binaries: ${MISSING[*]}"
    log_error "Build first: cd ${PROJECT_ROOT}/ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release"
    exit 1
fi
log_info "All 4 eBPF binaries found in ${EBPF_BIN_DIR}"

# Check for bpftool
if ! command -v bpftool &>/dev/null; then
    log_warn "bpftool not found — will use bpf() syscall via programs directly"
    log_warn "Install with: apt install linux-tools-common linux-tools-$(uname -r)"
    HAS_BPFTOOL=false
else
    HAS_BPFTOOL=true
    log_info "bpftool found: $(which bpftool)"
fi

# Check network interface
if ! ip link show "$INTERFACE" &>/dev/null; then
    log_warn "Interface '$INTERFACE' not found. Available:"
    ip -o link show | awk -F': ' '{print "  " $2}'
    log_warn "Set UNHEADED_INTERFACE=<iface> to use a different interface"
    # Don't exit — some programs don't need an interface
fi

# ============================================================================
# SETUP BPFFS
# ============================================================================

log_step "Setting up BPF filesystem..."

# Mount bpffs if not already mounted
if ! mount | grep -q "type bpf"; then
    log_info "Mounting bpffs at /sys/fs/bpf"
    run_cmd mount -t bpf bpf /sys/fs/bpf
else
    log_info "bpffs already mounted"
fi

# Create pin directory
if [[ ! -d "$BPFFS_ROOT" ]]; then
    log_info "Creating pin directory: ${BPFFS_ROOT}"
    run_cmd mkdir -p "${BPFFS_ROOT}"
else
    log_info "Pin directory exists: ${BPFFS_ROOT}"
fi

# Create subdirectories for each program's maps
for prog in "${PROGRAMS[@]}"; do
    run_cmd mkdir -p "${BPFFS_ROOT}/${prog}"
done

# ============================================================================
# LOAD eBPF PROGRAMS
# ============================================================================

LOADED=0
FAILED=0

# --- 1. packet-marker (XDP) ---
log_step "Loading packet-marker (XDP on ${INTERFACE})..."

if $HAS_BPFTOOL; then
    if run_cmd bpftool prog load "${EBPF_BIN_DIR}/packet-marker" \
        "${BPFFS_ROOT}/packet-marker/prog" \
        pinmaps "${BPFFS_ROOT}/packet-marker" 2>/dev/null; then

        # Attach to XDP on the interface
        if ip link show "$INTERFACE" &>/dev/null; then
            run_cmd bpftool net attach xdp \
                pinned "${BPFFS_ROOT}/packet-marker/prog" \
                dev "$INTERFACE" 2>/dev/null || \
            run_cmd ip link set dev "$INTERFACE" xdp obj "${EBPF_BIN_DIR}/packet-marker" sec xdp 2>/dev/null || \
            log_warn "Could not attach packet-marker to ${INTERFACE} (XDP may not be supported)"
        fi

        log_info "packet-marker loaded and pinned"
        ((LOADED++))
    else
        log_warn "packet-marker: bpftool load failed, trying ip link..."
        if ip link show "$INTERFACE" &>/dev/null; then
            run_cmd ip link set dev "$INTERFACE" xdp obj "${EBPF_BIN_DIR}/packet-marker" sec xdp 2>/dev/null && \
                { log_info "packet-marker attached via ip link"; ((LOADED++)); } || \
                { log_error "packet-marker failed to load"; ((FAILED++)); }
        else
            log_error "packet-marker: no interface available"
            ((FAILED++))
        fi
    fi
else
    log_warn "packet-marker: skipped (no bpftool)"
    ((FAILED++))
fi

# --- 2. flow-tracker (TC) ---
log_step "Loading flow-tracker (TC on ${INTERFACE})..."

if $HAS_BPFTOOL && ip link show "$INTERFACE" &>/dev/null; then
    if run_cmd bpftool prog load "${EBPF_BIN_DIR}/flow-tracker" \
        "${BPFFS_ROOT}/flow-tracker/prog" \
        pinmaps "${BPFFS_ROOT}/flow-tracker" 2>/dev/null; then

        # Setup TC qdisc and attach
        run_cmd tc qdisc add dev "$INTERFACE" clsact 2>/dev/null || true
        PROG_ID=$(bpftool prog show pinned "${BPFFS_ROOT}/flow-tracker/prog" 2>/dev/null | head -1 | awk '{print $1}' | tr -d ':')
        if [[ -n "$PROG_ID" ]]; then
            run_cmd tc filter add dev "$INTERFACE" ingress bpf da fd "${BPFFS_ROOT}/flow-tracker/prog" 2>/dev/null || \
                log_warn "Could not attach flow-tracker to TC ingress"
        fi

        log_info "flow-tracker loaded and pinned"
        ((LOADED++))
    else
        log_error "flow-tracker failed to load"
        ((FAILED++))
    fi
else
    if ! $HAS_BPFTOOL; then
        log_warn "flow-tracker: skipped (no bpftool)"
    else
        log_warn "flow-tracker: skipped (no interface)"
    fi
    ((FAILED++))
fi

# --- 3. latency-probe (kprobe/kretprobe) ---
log_step "Loading latency-probe (kprobe)..."

if $HAS_BPFTOOL; then
    if run_cmd bpftool prog load "${EBPF_BIN_DIR}/latency-probe" \
        "${BPFFS_ROOT}/latency-probe/prog" \
        pinmaps "${BPFFS_ROOT}/latency-probe" 2>/dev/null; then

        log_info "latency-probe loaded and pinned"
        ((LOADED++))

        # Attach kprobes
        log_debug "Attaching kprobe to tcp_v4_connect..."
        run_cmd bpftool prog attach pinned "${BPFFS_ROOT}/latency-probe/prog" \
            kprobe tcp_v4_connect 2>/dev/null || \
            log_warn "Could not attach kprobe (may need manual perf_event setup)"
    else
        log_error "latency-probe failed to load"
        ((FAILED++))
    fi
else
    log_warn "latency-probe: skipped (no bpftool)"
    ((FAILED++))
fi

# --- 4. syscall-tracer (raw tracepoint) ---
log_step "Loading syscall-tracer (raw_tracepoint)..."

if $HAS_BPFTOOL; then
    if run_cmd bpftool prog load "${EBPF_BIN_DIR}/syscall-tracer" \
        "${BPFFS_ROOT}/syscall-tracer/prog" \
        pinmaps "${BPFFS_ROOT}/syscall-tracer" 2>/dev/null; then

        log_info "syscall-tracer loaded and pinned"
        ((LOADED++))

        # Attach to raw_tracepoint/sys_enter
        run_cmd bpftool prog attach pinned "${BPFFS_ROOT}/syscall-tracer/prog" \
            raw_tracepoint sys_enter 2>/dev/null || \
            log_warn "Could not attach raw_tracepoint (may need manual setup)"
    else
        log_error "syscall-tracer failed to load"
        ((FAILED++))
    fi
else
    log_warn "syscall-tracer: skipped (no bpftool)"
    ((FAILED++))
fi

# ============================================================================
# CREATE SHARED RING BUFFER PINS
# ============================================================================

log_step "Setting up shared ring buffer pins..."

# Create symlinks for trace-collector to find the ring buffers
# The trace-collector expects maps at /sys/fs/bpf/unheaded/trace_events
# and /sys/fs/bpf/unheaded/perf_events

# Find and link the ring buffer maps
for prog in "${PROGRAMS[@]}"; do
    MAP_DIR="${BPFFS_ROOT}/${prog}"
    if [[ -d "$MAP_DIR" ]]; then
        for map_file in "${MAP_DIR}"/*; do
            map_name=$(basename "$map_file")
            case "$map_name" in
                *event*|*ring*|*trace*)
                    if [[ ! -e "${BPFFS_ROOT}/trace_events" ]]; then
                        run_cmd ln -sf "$map_file" "${BPFFS_ROOT}/trace_events"
                        log_info "Linked trace_events → ${map_file}"
                    fi
                    ;;
                *perf*)
                    if [[ ! -e "${BPFFS_ROOT}/perf_events" ]]; then
                        run_cmd ln -sf "$map_file" "${BPFFS_ROOT}/perf_events"
                        log_info "Linked perf_events → ${map_file}"
                    fi
                    ;;
            esac
        done
    fi
done

# ============================================================================
# SUMMARY
# ============================================================================

echo ""
log_step "Summary"
echo "  Programs loaded: ${LOADED}/4"
echo "  Programs failed: ${FAILED}/4"
echo "  Pin directory:   ${BPFFS_ROOT}"
echo "  Interface:       ${INTERFACE}"
echo ""

if $HAS_BPFTOOL && ! $DRY_RUN; then
    log_step "Loaded programs:"
    bpftool prog show 2>/dev/null | head -20 || true
    echo ""
    log_step "Pinned maps:"
    ls -la "${BPFFS_ROOT}"/ 2>/dev/null || true
fi

if [[ $LOADED -gt 0 ]]; then
    log_info "Next steps:"
    echo "  1. Start trace-collector: cd ${PROJECT_ROOT}/cmd/trace-collector && cargo run --release -- run"
    echo "  2. Start Wotan:          cd ${PROJECT_ROOT} && go run ./cmd/wotan/"
    echo "  3. Verify events:         cargo run --release -- dump --max-events 10"
    echo ""
fi

if [[ $FAILED -gt 0 ]]; then
    log_warn "Some programs failed to load. This may be because:"
    echo "  - bpftool is not installed"
    echo "  - BTF (BPF Type Format) is not available on this kernel"
    echo "  - The kernel doesn't support the required BPF program types"
    echo ""
    echo "  Install bpftool:  apt install linux-tools-common linux-tools-\$(uname -r)"
    echo "  Check BTF:        ls /sys/kernel/btf/vmlinux"
fi

exit $FAILED
