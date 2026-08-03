#!/usr/bin/env bash
# doom-smoke.sh — Full automated lifecycle smoke test for Doom-over-IPv6
#
# Orchestrates the complete DOOM testing pipeline:
#   1. Preflight checks (tools, binaries)
#   2. Teardown stale ring
#   3. Optionally rebuild eBPF
#   4. Setup ring
#   5. Load DOOM data (ROM, RAM/WAD, RV2MBC, CPU)
#   6. Start injector
#   7. Start bridge
#   8. Wait for game loop (initialized=1, virtual_ticks > threshold)
#   9. Assert CPU health (SP, halted)
#  10. Assert screen (doom-screen-check.py)
#  11. Report and exit
#
# Usage:
#   sudo ./scripts/doom-smoke.sh [--rebuild] [--no-teardown] [--timeout 120] [--verbose]
#
# Exit: 0 if all assertions pass, 1 if any fail

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
DO_REBUILD=0
DO_TEARDOWN=1
TIMEOUT=120
VERBOSE=0

# BPF paths
MAP_PIN_DIR="/sys/fs/bpf/unheaded/doom-ring/maps"
CPU_MAP_PIN="${MAP_PIN_DIR}/CPU_MAP"

# Binary paths
DOOM_BRIDGE="${PROJECT_ROOT}/cmd/doom-bridge/doom-bridge"
DOOM_LOADER="${PROJECT_ROOT}/cmd/doom-loader/doom-loader"

# Process tracking
INJECTOR_PID=""
BRIDGE_PID=""

# ELF for symbol resolution
DOOM_ELF="${PROJECT_ROOT}/doom/doomgeneric/doomgeneric/doom.elf"
DOOM_ELF_ALT="/tmp/doom-build/doom.elf"

# ── Colors ───────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ── Logging ──────────────────────────────────────────────────────────────────

log_info()    { echo -e "${GREEN}[INFO]${NC}    $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}    $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC}   $*"; }
log_step()    { echo -e "${BLUE}[STEP]${NC}    $*"; }
log_pass()    { echo -e "${GREEN}${BOLD}[PASS]${NC}    $*"; }
log_fail()    { echo -e "${RED}${BOLD}[FAIL]${NC}    $*"; }
log_verbose() { [[ $VERBOSE -eq 1 ]] && echo -e "${DIM}          $*${NC}" || true; }

banner() {
    echo ""
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  Doom-over-IPv6 — Full Lifecycle Smoke Test${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ── Counters ─────────────────────────────────────────────────────────────────

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

record_pass() {
    ((PASS_COUNT++)) || true
    log_pass "$1"
}

record_fail() {
    ((FAIL_COUNT++)) || true
    log_fail "$1"
}

record_skip() {
    ((SKIP_COUNT++)) || true
    log_warn "SKIP: $1"
}

# ── Argument parsing ─────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case "$1" in
        --rebuild)
            DO_REBUILD=1
            shift
            ;;
        --no-teardown)
            DO_TEARDOWN=0
            shift
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=1
            shift
            ;;
        -h|--help)
            echo "Usage: sudo $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --rebuild        Rebuild eBPF before test"
            echo "  --no-teardown    Don't teardown ring on exit"
            echo "  --timeout SECS   Game loop wait timeout (default: 120)"
            echo "  --verbose, -v    Show detailed output"
            echo ""
            echo "Pipeline:"
            echo "  1. Preflight checks"
            echo "  2. Teardown stale ring"
            echo "  3. Rebuild eBPF (if --rebuild)"
            echo "  4. Setup ring"
            echo "  5. Load DOOM data"
            echo "  6. Start injector"
            echo "  7. Start bridge"
            echo "  8. Wait for game loop"
            echo "  9. Assert CPU health"
            echo " 10. Assert screen"
            echo " 11. Report"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ── Cleanup trap ─────────────────────────────────────────────────────────────

cleanup() {
    local exit_code=$?
    echo ""
    log_step "Cleaning up..."

    # Kill injector
    if [[ -n "$INJECTOR_PID" ]] && kill -0 "$INJECTOR_PID" 2>/dev/null; then
        kill "$INJECTOR_PID" 2>/dev/null || true
        wait "$INJECTOR_PID" 2>/dev/null || true
        log_info "Stopped injector (PID ${INJECTOR_PID})"
    fi

    # Kill bridge
    if [[ -n "$BRIDGE_PID" ]] && kill -0 "$BRIDGE_PID" 2>/dev/null; then
        kill "$BRIDGE_PID" 2>/dev/null || true
        wait "$BRIDGE_PID" 2>/dev/null || true
        log_info "Stopped bridge (PID ${BRIDGE_PID})"
    fi

    # Teardown ring
    if [[ $DO_TEARDOWN -eq 1 ]]; then
        if [[ -x "${SCRIPT_DIR}/doom-ring.sh" ]]; then
            "${SCRIPT_DIR}/doom-ring.sh" teardown 2>/dev/null || true
            log_info "Ring torn down"
        fi
    else
        log_info "Skipping teardown (--no-teardown)"
    fi

    exit "$exit_code"
}

trap cleanup EXIT INT TERM

# ── Step 1: Preflight ────────────────────────────────────────────────────────

preflight() {
    log_step "Step 1: Preflight checks..."

    if [[ $EUID -ne 0 ]]; then
        log_error "Must run as root (sudo)"
        exit 1
    fi

    local missing=0
    for tool in bpftool ip python3 nsenter; do
        if ! command -v "$tool" &>/dev/null; then
            log_error "Required tool not found: $tool"
            missing=1
        fi
    done

    # Check doom-loader
    if [[ ! -x "$DOOM_LOADER" ]]; then
        log_info "Building doom-loader..."
        (cd "$PROJECT_ROOT" && go build -o cmd/doom-loader/doom-loader ./cmd/doom-loader/) || {
            log_error "Failed to build doom-loader"
            missing=1
        }
    fi

    # Check doom-bridge
    if [[ ! -x "$DOOM_BRIDGE" ]]; then
        log_info "Building doom-bridge..."
        (cd "$PROJECT_ROOT" && go build -o cmd/doom-bridge/doom-bridge ./cmd/doom-bridge/) || {
            log_error "Failed to build doom-bridge"
            missing=1
        }
    fi

    # Check doom-ring.sh
    if [[ ! -x "${SCRIPT_DIR}/doom-ring.sh" ]]; then
        log_error "doom-ring.sh not found at ${SCRIPT_DIR}/doom-ring.sh"
        missing=1
    fi

    # Check doom-loader.sh
    if [[ ! -x "${SCRIPT_DIR}/doom-loader.sh" ]]; then
        log_error "doom-loader.sh not found at ${SCRIPT_DIR}/doom-loader.sh"
        missing=1
    fi

    # Check injector binary
    local injector=""
    if [[ -x "${PROJECT_ROOT}/cmd/doom-injector-c/doom-injector" ]]; then
        injector="${PROJECT_ROOT}/cmd/doom-injector-c/doom-injector"
    elif [[ -x "${PROJECT_ROOT}/cmd/doom-go-injector/doom-go-injector" ]]; then
        injector="${PROJECT_ROOT}/cmd/doom-go-injector/doom-go-injector"
    fi

    if [[ -z "$injector" ]]; then
        log_warn "No injector binary found (doom-injector-c or doom-go-injector)"
        log_warn "Will attempt to build doom-go-injector..."
        if [[ -d "${PROJECT_ROOT}/cmd/doom-go-injector" ]]; then
            (cd "$PROJECT_ROOT" && go build -o cmd/doom-go-injector/doom-go-injector ./cmd/doom-go-injector/) 2>/dev/null || {
                log_error "Failed to build doom-go-injector"
                missing=1
            }
        else
            log_error "No injector source found"
            missing=1
        fi
    fi

    if [[ $missing -ne 0 ]]; then
        exit 1
    fi

    log_info "All preflight checks passed"
}

# ── Step 2: Teardown stale ring ──────────────────────────────────────────────

teardown_stale() {
    log_step "Step 2: Teardown stale ring..."
    "${SCRIPT_DIR}/doom-ring.sh" teardown 2>/dev/null || true
    log_info "Stale ring cleared"
}

# ── Step 3: Rebuild eBPF (optional) ──────────────────────────────────────────

rebuild_ebpf() {
    if [[ $DO_REBUILD -eq 0 ]]; then
        log_step "Step 3: Rebuild eBPF (skipped, use --rebuild to enable)"
        return
    fi

    log_step "Step 3: Rebuilding eBPF monad-cpu..."
    (cd "$PROJECT_ROOT" && make ebpf-monad-cpu) || {
        log_error "eBPF rebuild failed"
        exit 1
    }
    log_info "eBPF rebuild complete"
}

# ── Step 4: Setup ring ───────────────────────────────────────────────────────

setup_ring() {
    log_step "Step 4: Setting up doom ring..."
    "${SCRIPT_DIR}/doom-ring.sh" setup || {
        log_error "Ring setup failed"
        exit 1
    }
    log_info "Ring setup complete"
}

# ── Step 5: Load DOOM data ───────────────────────────────────────────────────

load_doom_data() {
    log_step "Step 5: Loading DOOM data (ROM, RAM/WAD, RV2MBC, CPU)..."
    "${SCRIPT_DIR}/doom-loader.sh" all || {
        log_error "DOOM data loading failed"
        exit 1
    }
    log_info "All DOOM data loaded"
}

# ── Step 6: Start injector ───────────────────────────────────────────────────

start_injector() {
    log_step "Step 6: Starting tick injector..."

    # Get MAC addresses for veth01 and veth01p
    local src_mac dst_mac
    src_mac=$(ip netns exec monad0 ip link show veth01 2>/dev/null | awk '/ether/ {print $2}')
    dst_mac=$(ip netns exec monad1 ip link show veth01p 2>/dev/null | awk '/ether/ {print $2}')

    if [[ -z "$src_mac" || -z "$dst_mac" ]]; then
        log_error "Could not determine MAC addresses for veth01/veth01p"
        exit 1
    fi

    log_verbose "src_mac=${src_mac} dst_mac=${dst_mac}"

    # Find injector binary
    local injector=""
    if [[ -x "${PROJECT_ROOT}/cmd/doom-injector-c/doom-injector" ]]; then
        injector="${PROJECT_ROOT}/cmd/doom-injector-c/doom-injector"
    elif [[ -x "${PROJECT_ROOT}/cmd/doom-go-injector/doom-go-injector" ]]; then
        injector="${PROJECT_ROOT}/cmd/doom-go-injector/doom-go-injector"
    fi

    if [[ -z "$injector" ]]; then
        log_error "No injector binary found"
        exit 1
    fi

    # Start injector in monad0 namespace
    nsenter --net=/var/run/netns/monad0 \
        "$injector" \
        --src-mac "$src_mac" \
        --dst-mac "$dst_mac" \
        --interface veth01 \
        > /tmp/doom-smoke-injector.log 2>&1 &
    INJECTOR_PID=$!

    sleep 0.5
    if ! kill -0 "$INJECTOR_PID" 2>/dev/null; then
        log_error "Injector died immediately. Log:"
        cat /tmp/doom-smoke-injector.log 2>/dev/null || true
        exit 1
    fi

    log_info "Injector started (PID ${INJECTOR_PID})"
}

# ── Step 7: Start bridge ─────────────────────────────────────────────────────

start_bridge() {
    log_step "Step 7: Starting doom-bridge on port 16666..."

    "$DOOM_BRIDGE" \
        --port 16666 \
        --map-dir "${MAP_PIN_DIR}" \
        > /tmp/doom-smoke-bridge.log 2>&1 &
    BRIDGE_PID=$!

    sleep 1
    if ! kill -0 "$BRIDGE_PID" 2>/dev/null; then
        log_error "Bridge died immediately. Log:"
        cat /tmp/doom-smoke-bridge.log 2>/dev/null || true
        exit 1
    fi

    log_info "Bridge started (PID ${BRIDGE_PID}, port 16666)"
}

# ── Step 8: Wait for game loop ───────────────────────────────────────────────

read_cpu_field() {
    # Read a specific field from CPU_MAP via embedded Python
    local field="$1"
    python3 -c "
import subprocess, struct, sys, json

result = subprocess.run(
    ['bpftool', 'map', 'lookup', 'pinned', '${CPU_MAP_PIN}',
     'key', 'hex', 'de', '00', '00', '00'],
    capture_output=True, text=True
)
if result.returncode != 0:
    print('error')
    sys.exit(0)

lines = result.stdout.strip().split('\n')
hex_bytes = []
in_value = False
for line in lines:
    if 'value:' in line:
        in_value = True
        parts = line.split('value:')[1].strip().split()
        hex_bytes.extend(parts)
    elif in_value and line.strip() and not line.startswith('key:') and not line.startswith('Found'):
        hex_bytes.extend(line.strip().split())

if len(hex_bytes) < 104:
    print('error')
    sys.exit(0)

bs = bytes(int(b, 16) for b in hex_bytes[:104])
regs = struct.unpack_from('<16I', bs, 0)
pc = struct.unpack_from('<I', bs, 64)[0]
halted = bs[69]
insn_count = struct.unpack_from('<Q', bs, 80)[0]

fields = {
    'pc': pc,
    'sp': regs[15],
    'halted': halted,
    'insn_count': insn_count,
}
print(fields.get('${field}', 'unknown'))
" 2>/dev/null
}

read_ram_word_at() {
    # Read a word from RAM_MAP at the given byte address
    local byte_addr="$1"
    python3 -c "
import subprocess, struct, sys

word_addr = ${byte_addr} // 4
key = struct.pack('<I', word_addr)
key_hex = ' '.join(f'{b:02x}' for b in key)

result = subprocess.run(
    ['bpftool', 'map', 'lookup', 'pinned', '${MAP_PIN_DIR}/RAM_MAP',
     'key', 'hex'] + key_hex.split(),
    capture_output=True, text=True
)
if result.returncode != 0:
    print('error')
    sys.exit(0)

lines = result.stdout.strip().split('\n')
hex_bytes = []
in_value = False
for line in lines:
    if 'value:' in line:
        in_value = True
        parts = line.split('value:')[1].strip().split()
        hex_bytes.extend(parts)
    elif in_value and line.strip():
        hex_bytes.extend(line.strip().split())

if len(hex_bytes) < 4:
    print('error')
    sys.exit(0)

val = struct.unpack('<I', bytes(int(b, 16) for b in hex_bytes[:4]))[0]
print(val)
" 2>/dev/null
}

wait_for_game_loop() {
    log_step "Step 8: Waiting for game loop (timeout: ${TIMEOUT}s)..."

    # Resolve 'initialized' address from ELF
    local init_addr=""
    local elf=""
    [[ -f "$DOOM_ELF" ]] && elf="$DOOM_ELF"
    [[ -z "$elf" && -f "$DOOM_ELF_ALT" ]] && elf="$DOOM_ELF_ALT"

    if [[ -n "$elf" ]] && command -v riscv64-unknown-elf-nm &>/dev/null; then
        init_addr=$(riscv64-unknown-elf-nm "$elf" 2>/dev/null | awk '/ initialized$/{print "0x"$1}' | head -1)
        [[ -n "$init_addr" ]] && log_verbose "Symbol: initialized @ ${init_addr}"
    fi

    local elapsed=0
    local poll_interval=5

    while [[ $elapsed -lt $TIMEOUT ]]; do
        local insn_count
        insn_count=$(read_cpu_field "insn_count")

        if [[ "$insn_count" == "error" ]]; then
            log_verbose "Poll ${elapsed}s: CPU read error"
            sleep "$poll_interval"
            elapsed=$((elapsed + poll_interval))
            continue
        fi

        local halted
        halted=$(read_cpu_field "halted")

        log_verbose "Poll ${elapsed}s: insn_count=${insn_count} halted=${halted}"

        # Check if game loop has started: insn_count > 10000 indicates real progress
        if [[ "$insn_count" -gt 10000 && "$halted" == "0" ]]; then
            # If we have the initialized symbol, check it
            if [[ -n "$init_addr" ]]; then
                local init_val
                init_val=$(read_ram_word_at "$(printf '%d' "$init_addr")")
                if [[ "$init_val" == "1" ]]; then
                    log_info "Game loop confirmed: initialized=1, insn_count=${insn_count}"
                    return 0
                fi
                log_verbose "initialized=${init_val} (waiting for 1)"
            else
                # No symbol — just use insn_count threshold
                log_info "Game loop confirmed: insn_count=${insn_count} (no symbol check)"
                return 0
            fi
        fi

        # Check for early halt (crash)
        if [[ "$halted" != "0" && "$insn_count" -gt 0 ]]; then
            log_warn "CPU halted at insn_count=${insn_count} (may be normal HALT)"
        fi

        sleep "$poll_interval"
        elapsed=$((elapsed + poll_interval))
    done

    log_error "Timeout (${TIMEOUT}s) waiting for game loop"
    local final_insn
    final_insn=$(read_cpu_field "insn_count")
    log_error "Final insn_count=${final_insn}"
    return 1
}

# ── Step 9: Assert CPU health ────────────────────────────────────────────────

assert_cpu_health() {
    log_step "Step 9: Asserting CPU health..."

    local sp halted insn_count pc
    sp=$(read_cpu_field "sp")
    halted=$(read_cpu_field "halted")
    insn_count=$(read_cpu_field "insn_count")
    pc=$(read_cpu_field "pc")

    log_verbose "CPU: sp=0x$(printf '%X' "$sp") pc=0x$(printf '%X' "$pc") insn=${insn_count} halted=${halted}"

    # SP check (Bug 22: stack drain)
    local sp_threshold=0x03E00000
    local sp_threshold_dec
    sp_threshold_dec=$(printf '%d' "$sp_threshold")

    if [[ "$sp" != "error" && "$sp" -gt "$sp_threshold_dec" ]]; then
        record_pass "SP = 0x$(printf '%X' "$sp") > threshold 0x$(printf '%X' "$sp_threshold_dec")"
    elif [[ "$sp" == "error" ]]; then
        record_skip "SP check: could not read CPU state"
    else
        record_fail "SP = 0x$(printf '%X' "$sp") <= threshold 0x$(printf '%X' "$sp_threshold_dec") (Bug 22: stack drain)"
    fi

    # Halted check
    if [[ "$halted" != "error" && "$halted" == "0" ]]; then
        record_pass "halted = 0 (CPU running)"
    elif [[ "$halted" == "error" ]]; then
        record_skip "halted check: could not read CPU state"
    else
        record_fail "halted = ${halted} (CPU stopped unexpectedly)"
    fi
}

# ── Step 10: Assert screen ───────────────────────────────────────────────────

assert_screen() {
    log_step "Step 10: Asserting screen (doom-screen-check.py)..."

    local screen_check="${SCRIPT_DIR}/doom-screen-check.py"
    if [[ ! -f "$screen_check" ]]; then
        record_skip "Screen check: doom-screen-check.py not found"
        return
    fi

    local screen_json
    screen_json=$(python3 "$screen_check" --json 2>/dev/null) || true

    if [[ -z "$screen_json" ]]; then
        record_skip "Screen check: could not read SCREEN_MAP"
        return
    fi

    local valid nonzero_pct ascii_runs
    valid=$(echo "$screen_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['valid'])" 2>/dev/null)
    nonzero_pct=$(echo "$screen_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['nonzero_pct'])" 2>/dev/null)
    ascii_runs=$(echo "$screen_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['ascii_runs'])" 2>/dev/null)

    log_verbose "Screen: valid=${valid} nonzero=${nonzero_pct}% ascii_runs=${ascii_runs}"

    # Non-zero pixel check
    if [[ -n "$nonzero_pct" ]]; then
        local nonzero_int
        nonzero_int=$(python3 -c "print(int(float('${nonzero_pct}')))" 2>/dev/null || echo "0")
        if [[ "$nonzero_int" -gt 50 ]]; then
            record_pass "Screen: ${nonzero_pct}% non-zero pixels (threshold: >50%)"
        else
            record_fail "Screen: ${nonzero_pct}% non-zero pixels (threshold: >50%)"
        fi
    else
        record_skip "Screen: could not parse nonzero_pct"
    fi

    # WAD pollution check (Bug 24)
    if [[ -n "$ascii_runs" && "$ascii_runs" == "0" ]]; then
        record_pass "Screen: no WAD pollution detected (Bug 24 clean)"
    elif [[ -n "$ascii_runs" ]]; then
        record_fail "Screen: ${ascii_runs} WAD ASCII runs detected (Bug 24)"
    else
        record_skip "Screen: could not parse ascii_runs"
    fi
}

# ── Step 11: Report ──────────────────────────────────────────────────────────

report_and_exit() {
    echo ""
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  Smoke Test Summary${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "  ${GREEN}PASS:${NC} ${PASS_COUNT}"
    echo -e "  ${RED}FAIL:${NC} ${FAIL_COUNT}"
    echo -e "  ${YELLOW}SKIP:${NC} ${SKIP_COUNT}"
    echo ""

    if [[ $FAIL_COUNT -gt 0 ]]; then
        echo -e "  ${RED}${BOLD}RESULT: FAIL${NC} — ${FAIL_COUNT} assertion(s) failed"
        echo ""
        exit 1
    elif [[ $PASS_COUNT -eq 0 ]]; then
        echo -e "  ${YELLOW}${BOLD}RESULT: NO ASSERTIONS RUN${NC}"
        echo ""
        exit 1
    else
        echo -e "  ${GREEN}${BOLD}RESULT: PASS${NC} — all ${PASS_COUNT} assertions passed"
        echo ""
        exit 0
    fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    banner

    preflight
    echo ""

    teardown_stale
    echo ""

    rebuild_ebpf
    echo ""

    setup_ring
    echo ""

    load_doom_data
    echo ""

    start_injector
    echo ""

    start_bridge
    echo ""

    if ! wait_for_game_loop; then
        record_fail "Game loop did not start within ${TIMEOUT}s"
    else
        record_pass "Game loop started"
    fi
    echo ""

    assert_cpu_health
    echo ""

    assert_screen
    echo ""

    report_and_exit
}

main "$@"
