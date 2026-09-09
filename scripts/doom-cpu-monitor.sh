#!/usr/bin/env bash
# doom-cpu-monitor.sh — Continuous CPU state monitor for Doom-over-IPv6
#
# Polls CPU_MAP and RAM_MAP at regular intervals, catching known failure patterns:
#   - Stack drain (Bug 22): alert if r15 (SP) drops below threshold
#   - BSS corruption (Bug 23): alert if 'initialized' == 0 while CPU has executed
#   - Halt detection: alert if halted != 0 with PC not at known halt point
#   - Stall detection: alert if insn_count stalls for 2 consecutive polls
#
# Usage:
#   sudo ./scripts/doom-cpu-monitor.sh [--interval 5] [--duration 0] \
#        [--output FILE] [--threshold-sp 0x03E00000] [--quiet]
#
# Dependencies: bpftool, python3, riscv64-unknown-elf-nm (optional, for symbol lookup)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
INTERVAL=5
DURATION=0          # 0 = infinite
OUTPUT_FILE=""
THRESHOLD_SP=0x03E00000
QUIET=0

# BPF paths
MAP_PIN_DIR="/sys/fs/bpf/unheaded/doom-ring/maps"
CPU_MAP_PIN="${MAP_PIN_DIR}/CPU_MAP"
RAM_MAP_PIN="${MAP_PIN_DIR}/RAM_MAP"

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
NC='\033[0m'

# ── Logging ──────────────────────────────────────────────────────────────────

ts() { date '+%Y-%m-%d %H:%M:%S'; }

emit() {
    local level="$1"
    shift
    local msg
    msg="[$(ts)] ${level} $*"
    if [[ $QUIET -eq 0 ]]; then
        case "$level" in
            OK)   echo -e "${GREEN}${msg}${NC}" ;;
            WARN) echo -e "${YELLOW}${msg}${NC}" ;;
            FAIL) echo -e "${RED}${BOLD}${msg}${NC}" ;;
            INFO) echo -e "${BLUE}${msg}${NC}" ;;
            *)    echo "$msg" ;;
        esac
    fi
    if [[ -n "$OUTPUT_FILE" ]]; then
        echo "$msg" >> "$OUTPUT_FILE"
    fi
}

# ── Argument parsing ─────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case "$1" in
        --interval)
            INTERVAL="$2"
            shift 2
            ;;
        --duration)
            DURATION="$2"
            shift 2
            ;;
        --output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        --threshold-sp)
            THRESHOLD_SP="$2"
            shift 2
            ;;
        --quiet|-q)
            QUIET=1
            shift
            ;;
        -h|--help)
            echo "Usage: sudo $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --interval SECS      Poll interval in seconds (default: 5)"
            echo "  --duration SECS      Total duration, 0=infinite (default: 0)"
            echo "  --output FILE        Log output to file"
            echo "  --threshold-sp HEX   Minimum SP value (default: 0x03E00000)"
            echo "  --quiet, -q          Suppress terminal output (file only)"
            echo ""
            echo "Monitors:"
            echo "  Stack drain (Bug 22): SP < threshold"
            echo "  BSS corruption (Bug 23): initialized=0 while running"
            echo "  Halt detection: halted != 0"
            echo "  Stall detection: insn_count unchanged for 2 polls"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# Convert threshold to decimal for comparison
THRESHOLD_SP_DEC=$(printf '%d' "$THRESHOLD_SP" 2>/dev/null)

# ── Symbol Resolution ────────────────────────────────────────────────────────

INITIALIZED_ADDR=""
VIRTUAL_TICKS_ADDR=""

resolve_symbols() {
    local elf=""
    [[ -f "$DOOM_ELF" ]] && elf="$DOOM_ELF"
    [[ -z "$elf" && -f "$DOOM_ELF_ALT" ]] && elf="$DOOM_ELF_ALT"

    if [[ -z "$elf" ]]; then
        emit INFO "No doom.elf found, skipping symbol resolution"
        return
    fi

    local nm_tool="riscv64-unknown-elf-nm"
    if ! command -v "$nm_tool" &>/dev/null; then
        emit INFO "riscv64-unknown-elf-nm not found, skipping symbol resolution"
        return
    fi

    INITIALIZED_ADDR=$("$nm_tool" "$elf" 2>/dev/null | awk '/ initialized$/{print "0x"$1}' | head -1)
    VIRTUAL_TICKS_ADDR=$("$nm_tool" "$elf" 2>/dev/null | awk '/ virtual_ticks$/{print "0x"$1}' | head -1)

    if [[ -n "$INITIALIZED_ADDR" ]]; then
        emit INFO "Symbol: initialized @ ${INITIALIZED_ADDR}"
    fi
    if [[ -n "$VIRTUAL_TICKS_ADDR" ]]; then
        emit INFO "Symbol: virtual_ticks @ ${VIRTUAL_TICKS_ADDR}"
    fi
}

# ── CPU State Reader (embedded Python) ───────────────────────────────────────

read_cpu_state() {
    python3 -c "
import subprocess, struct, sys, json

result = subprocess.run(
    ['bpftool', 'map', 'lookup', 'pinned', '${CPU_MAP_PIN}',
     'key', 'hex', 'de', '00', '00', '00'],
    capture_output=True, text=True
)
if result.returncode != 0:
    print(json.dumps({'error': 'lookup_failed'}))
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
    print(json.dumps({'error': f'short_read_{len(hex_bytes)}'}))
    sys.exit(0)

bs = bytes(int(b, 16) for b in hex_bytes[:104])
regs = struct.unpack_from('<16I', bs, 0)
pc = struct.unpack_from('<I', bs, 64)[0]
flags, halted, stalled, pad = struct.unpack_from('<BBBB', bs, 68)
insn_count = struct.unpack_from('<Q', bs, 80)[0]

print(json.dumps({
    'pc': pc,
    'sp': regs[15],
    'halted': halted,
    'stalled': stalled,
    'insn_count': insn_count,
    'flags': flags,
    'r0': regs[0],
}))
"
}

# ── RAM Reader ───────────────────────────────────────────────────────────────

read_ram_word() {
    local byte_addr="$1"
    local word_addr=$((byte_addr / 4))

    # Encode key as 4-byte LE
    local b0 b1 b2 b3
    b0=$(printf '%02x' $((word_addr & 0xFF)))
    b1=$(printf '%02x' $(((word_addr >> 8) & 0xFF)))
    b2=$(printf '%02x' $(((word_addr >> 16) & 0xFF)))
    b3=$(printf '%02x' $(((word_addr >> 24) & 0xFF)))

    local result
    result=$(bpftool map lookup pinned "${RAM_MAP_PIN}" \
        key hex "$b0" "$b1" "$b2" "$b3" 2>/dev/null) || echo ""

    if [[ -z "$result" ]]; then
        echo "error"
        return
    fi

    # Extract value bytes
    local val_hex
    val_hex=$(echo "$result" | awk '/value:/{found=1; next} found{print}' | tr -d ' \n')
    if [[ -z "$val_hex" ]]; then
        val_hex=$(echo "$result" | sed -n 's/.*value:\s*//p' | tr -d ' \n')
    fi

    if [[ ${#val_hex} -ge 8 ]]; then
        local v0="${val_hex:0:2}"
        local v1="${val_hex:2:2}"
        local v2="${val_hex:4:2}"
        local v3="${val_hex:6:2}"
        printf '%d' "0x${v3}${v2}${v1}${v0}" 2>/dev/null || echo "error"
    else
        echo "error"
    fi
}

# ── Preflight ────────────────────────────────────────────────────────────────

if [[ $EUID -ne 0 ]]; then
    echo "ERROR: Must run as root (sudo)" >&2
    exit 1
fi

if [[ ! -e "$CPU_MAP_PIN" ]]; then
    echo "ERROR: CPU_MAP not pinned at ${CPU_MAP_PIN}" >&2
    echo "Run: sudo ./scripts/doom-ring.sh setup" >&2
    exit 1
fi

for tool in bpftool python3; do
    if ! command -v "$tool" &>/dev/null; then
        echo "ERROR: Required tool not found: $tool" >&2
        exit 1
    fi
done

# ── Banner ───────────────────────────────────────────────────────────────────

if [[ $QUIET -eq 0 ]]; then
    echo ""
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  Doom CPU Monitor — Continuous Health Polling${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "  Interval: ${INTERVAL}s  Duration: ${DURATION}s (0=infinite)"
    echo -e "  SP threshold: $(printf '0x%X' $THRESHOLD_SP_DEC)"
    [[ -n "$OUTPUT_FILE" ]] && echo -e "  Log file: ${OUTPUT_FILE}"
    echo ""
fi

# ── Symbol Resolution ────────────────────────────────────────────────────────

resolve_symbols

# ── Monitor Loop ─────────────────────────────────────────────────────────────

PREV_INSN_COUNT=-1
STALL_COUNT=0
START_TIME=$(date +%s)
POLL_NUM=0
WARN_COUNT=0
FAIL_COUNT=0

cleanup() {
    echo ""
    emit INFO "Monitor stopped after ${POLL_NUM} polls (${WARN_COUNT} warnings, ${FAIL_COUNT} failures)"
    exit 0
}

trap cleanup INT TERM

while true; do
    ((POLL_NUM++)) || true

    # Check duration
    if [[ $DURATION -gt 0 ]]; then
        elapsed=$(( $(date +%s) - START_TIME ))
        if [[ $elapsed -ge $DURATION ]]; then
            emit INFO "Duration reached (${DURATION}s)"
            break
        fi
    fi

    # Read CPU state
    cpu_json=$(read_cpu_state)

    error=$(echo "$cpu_json" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('error',''))" 2>/dev/null)
    if [[ -n "$error" ]]; then
        emit WARN "cpu_read_error=${error}"
        sleep "$INTERVAL"
        continue
    fi

    # Parse fields
    pc=$(echo "$cpu_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['pc'])")
    sp=$(echo "$cpu_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['sp'])")
    halted=$(echo "$cpu_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['halted'])")
    insn_count=$(echo "$cpu_json" | python3 -c "import sys,json; print(json.load(sys.stdin)['insn_count'])")

    # ── Check: Stack drain (Bug 22) ─────────────────────────────────────
    if [[ $sp -lt $THRESHOLD_SP_DEC && $insn_count -gt 0 ]]; then
        emit FAIL "stack_drain sp=0x$(printf '%X' $sp) threshold=0x$(printf '%X' $THRESHOLD_SP_DEC) insn_count=${insn_count}"
        ((FAIL_COUNT++)) || true
    else
        emit OK "sp=0x$(printf '%X' $sp) pc=0x$(printf '%X' $pc) insn=${insn_count} halted=${halted}"
    fi

    # ── Check: BSS corruption (Bug 23) ──────────────────────────────────
    if [[ -n "$INITIALIZED_ADDR" && -e "$RAM_MAP_PIN" ]]; then
        init_val=$(read_ram_word "$(printf '%d' "$INITIALIZED_ADDR")")
        if [[ "$init_val" != "error" && "$init_val" == "0" && $insn_count -gt 1000 ]]; then
            emit FAIL "bss_corruption initialized=0 at ${INITIALIZED_ADDR} (insn_count=${insn_count})"
            ((FAIL_COUNT++)) || true
        fi
    fi

    # ── Check: Halt detection ────────────────────────────────────────────
    if [[ $halted -ne 0 ]]; then
        emit WARN "halted=${halted} pc=0x$(printf '%X' $pc) insn=${insn_count}"
        ((WARN_COUNT++)) || true
    fi

    # ── Check: Stall detection ───────────────────────────────────────────
    if [[ $PREV_INSN_COUNT -ge 0 && $insn_count -eq $PREV_INSN_COUNT && $halted -eq 0 ]]; then
        ((STALL_COUNT++)) || true
        if [[ $STALL_COUNT -ge 2 ]]; then
            emit FAIL "stall_detected insn_count=${insn_count} unchanged for ${STALL_COUNT} polls"
            ((FAIL_COUNT++)) || true
        else
            emit WARN "possible_stall insn_count=${insn_count} unchanged"
            ((WARN_COUNT++)) || true
        fi
    else
        STALL_COUNT=0
    fi

    PREV_INSN_COUNT=$insn_count

    sleep "$INTERVAL"
done

emit INFO "Monitor complete: ${POLL_NUM} polls, ${WARN_COUNT} warnings, ${FAIL_COUNT} failures"

if [[ $FAIL_COUNT -gt 0 ]]; then
    exit 1
fi
exit 0
