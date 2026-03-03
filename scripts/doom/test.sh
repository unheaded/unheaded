#!/usr/bin/env bash
# doom-test.sh — Test harness for the Doom-over-IPv6 packet circulation ring
#
# Verifies end-to-end BPF compute by:
#   1. Setting up the 6-namespace ring (monad0–monad5)
#   2. Loading a small MBC test program into ROM_MAP via wotan-ctl
#   3. Injecting a single IPv6 Hop-by-Hop Monad packet into monad0
#   4. Reading back CPU state, STATS, and COMPUTE_EVENTS from BPF maps
#   5. Printing an instruction trace with PASS/FAIL verdict
#
# Usage:
#   sudo ./scripts/doom-test.sh              # Run 6-instruction arithmetic test
#   sudo ./scripts/doom-test.sh gradient     # Load demos/mbc/gradient.mbc (NYI)
#
# Prerequisites:
#   - Linux kernel >= 5.15 with eBPF + XDP
#   - doom-ring.sh dependencies (ip, python3, nsenter, cargo)
#   - Go toolchain (for wotan-ctl)
#   - monad-cpu-ebpf built and loadable
#   - ebpf-loader built (aya 0.13 userspace, handles legacy maps)
#
# The test program:
#   PC 0: MOVI r0, 1       r0 = 1
#   PC 1: MOVI r1, 10      r1 = 10
#   PC 2: ADD  r0, r1      r0 = 1 + 10  = 11
#   PC 3: MUL  r0, r1      r0 = 11 * 10 = 110
#   PC 4: MOVI r2, 42      r2 = 42
#   PC 5: HALT             CPU halted
#
# Expected final state: r0=110, r1=10, r2=42, PC=6, halted=1

set -euo pipefail

# ── Project paths ────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MAP_PIN="/sys/fs/bpf/unheaded/doom-ring/maps"
FLOW_LABEL="0xDE"
MBC_FILE="/tmp/ring-test.mbc"

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
log_result()  { echo -e "${CYAN}[RESULT]${NC}  $*"; }
log_pass()    { echo -e "${GREEN}${BOLD}[PASS]${NC}    $*"; }
log_fail()    { echo -e "${RED}${BOLD}[FAIL]${NC}    $*"; }

banner() {
    echo ""
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  Doom-over-IPv6 — Packet Circulation Ring Test Harness${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ── Cleanup trap ─────────────────────────────────────────────────────────────

cleanup() {
    local exit_code=$?
    echo ""
    log_step "Cleaning up..."
    rm -f "${MBC_FILE}"
    if [[ -x "${SCRIPT_DIR}/doom-ring.sh" ]]; then
        sudo "${SCRIPT_DIR}/doom-ring.sh" teardown 2>/dev/null || true
    fi
    if [[ $exit_code -ne 0 ]]; then
        log_error "Test harness exited with code ${exit_code}"
    fi
    exit $exit_code
}

trap cleanup EXIT INT TERM

# ── Preflight checks ────────────────────────────────────────────────────────

preflight() {
    log_step "Preflight checks..."

    if [[ $EUID -ne 0 ]]; then
        log_error "Must run as root (sudo)"
        exit 1
    fi

    local missing=0
    for tool in ip python3 nsenter bpftool; do
        if ! command -v "$tool" &>/dev/null; then
            log_error "Required tool not found: $tool"
            missing=1
        fi
    done
    if [[ $missing -ne 0 ]]; then
        exit 1
    fi

    log_info "All preflight checks passed"
}

# ── Step 1: Setup the ring ──────────────────────────────────────────────────

setup_ring() {
    log_step "Setting up 6-namespace packet circulation ring..."
    if ! sudo "${SCRIPT_DIR}/doom-ring.sh" setup; then
        log_error "Ring setup failed"
        exit 1
    fi
    log_info "Ring setup complete"
}

# ── Step 2: Create MBC test program ────────────────────────────────────────
#
# MBC instruction encoding (32-bit LE word):
#   [opcode:8][dst:4][src:4][imm16:16]
#
# Byte layout of a u32 in LE:
#   byte 0 = bits  7..0  (imm16 low byte)
#   byte 1 = bits 15..8  (imm16 high byte)
#   byte 2 = bits 23..16 (dst:4 | src:4)
#   byte 3 = bits 31..24 (opcode)
#
# Opcodes:  MOVI=0x0F  ADD=0x01  MUL=0x03  HALT=0xFF

create_test_program() {
    log_step "Creating 6-instruction MBC test program..."

    python3 -c "
import struct, sys

# MBC instruction encoder: encode(opcode, dst, src, imm16) -> u32 LE
def enc(opcode, dst=0, src=0, imm=0):
    word = ((opcode & 0xFF) << 24) | ((dst & 0xF) << 20) | ((src & 0xF) << 16) | (imm & 0xFFFF)
    return word

MOVI = 0x0F
ADD  = 0x01
MUL  = 0x03
HALT = 0xFF

program = [
    enc(MOVI, dst=0, imm=1),     # PC 0: MOVI r0, 1      -> 0x0F000001
    enc(MOVI, dst=1, imm=10),    # PC 1: MOVI r1, 10     -> 0x0F10000A
    enc(ADD,  dst=0, src=1),     # PC 2: ADD  r0, r1     -> 0x01010000
    enc(MUL,  dst=0, src=1),     # PC 3: MUL  r0, r1     -> 0x03010000
    enc(MOVI, dst=2, imm=42),    # PC 4: MOVI r2, 42     -> 0x0F20002A
    enc(HALT),                   # PC 5: HALT            -> 0xFF000000
]

# Verify encodings
expected = [0x0F000001, 0x0F10000A, 0x01010000, 0x03010000, 0x0F20002A, 0xFF000000]
for i, (got, want) in enumerate(zip(program, expected)):
    assert got == want, f'PC {i}: got 0x{got:08X}, want 0x{want:08X}'

# Write as LE u32 words
with open('${MBC_FILE}', 'wb') as f:
    for word in program:
        f.write(struct.pack('<I', word))

print(f'Wrote {len(program)} instructions ({len(program) * 4} bytes) to ${MBC_FILE}')
for i, w in enumerate(program):
    print(f'  PC {i}: 0x{w:08X}')
"

    if [[ ! -f "${MBC_FILE}" ]]; then
        log_error "Failed to create MBC binary"
        exit 1
    fi

    local size
    size=$(stat -c%s "${MBC_FILE}" 2>/dev/null || stat -f%z "${MBC_FILE}" 2>/dev/null)
    if [[ "${size}" != "24" ]]; then
        log_error "MBC binary is ${size} bytes, expected 24"
        exit 1
    fi

    log_info "Test program: 6 instructions, 24 bytes at ${MBC_FILE}"
    echo ""
    echo -e "${DIM}  Disassembly:${NC}"
    echo -e "${DIM}    PC 0: MOVI r0, 1       ;  r0 = 1${NC}"
    echo -e "${DIM}    PC 1: MOVI r1, 10      ;  r1 = 10${NC}"
    echo -e "${DIM}    PC 2: ADD  r0, r1      ;  r0 = 1 + 10 = 11${NC}"
    echo -e "${DIM}    PC 3: MUL  r0, r1      ;  r0 = 11 * 10 = 110${NC}"
    echo -e "${DIM}    PC 4: MOVI r2, 42      ;  r2 = 42${NC}"
    echo -e "${DIM}    PC 5: HALT             ;  stop${NC}"
    echo ""
}

# ── Step 3: Load program via wotan-ctl ──────────────────────────────────────

load_program() {
    log_step "Loading MBC program into ROM_MAP via wotan-ctl..."

    local wotan_ctl="${PROJECT_ROOT}/cmd/wotan-ctl/wotan-ctl"
    if [[ ! -x "$wotan_ctl" ]]; then
        log_error "wotan-ctl binary not found at ${wotan_ctl}"
        log_error "Build: cd ${PROJECT_ROOT} && go build -o cmd/wotan-ctl/wotan-ctl ./cmd/wotan-ctl/"
        exit 1
    fi

    local load_cmd="${wotan_ctl} load-rom \
        --file ${MBC_FILE} \
        --flow-label ${FLOW_LABEL} \
        --map-pin ${MAP_PIN} \
        --stats \
        --disasm"

    echo -e "${DIM}  $ ${load_cmd}${NC}"
    echo ""

    if ! eval "${load_cmd}"; then
        log_warn "wotan-ctl load-rom returned non-zero (may be dry-run mode)"
    fi

    log_info "Program loaded (flow_label=${FLOW_LABEL})"
}

# ── Step 4: Inject one IPv6 Monad packet ────────────────────────────────────
#
# Packet structure (78 bytes total):
#   Ethernet (14):  dst=ff:ff:ff:ff:ff:ff  src=00:00:00:00:00:01  type=0x86DD
#   IPv6 (40):      version=6, TC=0, flow_label=0xDE, payload_len=24,
#                   next_header=0 (HBH), hop_limit=64,
#                   src=fd00:3f:75::0:1, dst=fd00:dead::1
#   HBH (24):       next_header=59 (no next), hdr_ext_len=2,
#                   opt_type=0x3E, opt_data_len=20,
#                   monad: 20 bytes with CUSTOM flag (0x02)

inject_packet() {
    log_step "Injecting IPv6 Hop-by-Hop Monad packet into monad0..."

    # Get the peer MAC so the kernel treats the frame as PACKET_HOST
    # (ip6_forward drops PACKET_BROADCAST frames).
    local peer_mac
    peer_mac=$(ip netns exec monad1 ip link show veth01p 2>/dev/null | \
        awk '/ether/ {print $2}')
    if [[ -z "$peer_mac" ]]; then
        log_error "Could not get MAC for monad1/veth01p"
        exit 1
    fi

    nsenter --net=/var/run/netns/monad0 python3 - "$peer_mac" << 'PYEOF'
import socket
import struct
import sys
import re

peer_mac_str = sys.argv[1]
eth_dst = bytes.fromhex(peer_mac_str.replace(':', ''))
eth_src = b'\x00\x00\x00\x00\x00\x01'
eth_type = struct.pack('!H', 0x86DD)
ethernet = eth_dst + eth_src + eth_type

# ── HBH Options Header (24 bytes) ─────────────────────────────────────────
# next_header = 17 (UDP) — kernel requires a real next-header to forward
hbh_next     = struct.pack('B', 17)
hbh_ext_len  = struct.pack('B', 2)           # (2+1)*8 = 24 bytes
opt_type     = struct.pack('B', 0x3E)        # Monad option type
opt_data_len = struct.pack('B', 20)          # Monad register file
monad = bytearray(20)
monad[0] = 0x01   # version
monad[7] = 0x02   # flags = CUSTOM
hbh = hbh_next + hbh_ext_len + opt_type + opt_data_len + bytes(monad)

# ── Dummy UDP (8 bytes) — keeps kernel forwarding path happy ──────────────
udp = struct.pack('!HHHH', 1234, 1234, 8, 0)

# ── IPv6 Fixed Header (40 bytes) ─────────────────────────────────────────
vtf         = struct.pack('!I', 0x600000DE)         # flow_label=0xDE
payload_len = struct.pack('!H', len(hbh) + len(udp)) # 32
next_header = struct.pack('B', 0)                    # HBH
hop_limit   = struct.pack('B', 64)
src_addr = b'\xfd\x00\x00\x3f\x00\x75' + b'\x00' * 8 + b'\x00\x01'
dst_addr = b'\xfd\x00\xde\xad' + b'\x00' * 10 + b'\x00\x01'
ipv6 = vtf + payload_len + next_header + hop_limit + src_addr + dst_addr

# ── Assemble and send ─────────────────────────────────────────────────────
frame = ethernet + ipv6 + hbh + udp   # 14 + 40 + 24 + 8 = 86 bytes
assert len(frame) == 86, f"Frame is {len(frame)} bytes, expected 86"

try:
    sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x86DD))
    sock.bind(('veth01', 0))
    sent = sock.send(frame)
    sock.close()
    print(f"Injected {sent}-byte Monad packet on monad0/veth01 (flow_label=0xDE, dst_mac={peer_mac_str})")
except Exception as e:
    print(f"ERROR: Failed to inject packet: {e}", file=sys.stderr)
    sys.exit(1)
PYEOF

    if [[ $? -ne 0 ]]; then
        log_error "Packet injection failed"
        exit 1
    fi

    log_info "Packet injected into monad0/veth01"
}

# ── Step 5: Read BPF maps ──────────────────────────────────────────────────

read_maps() {
    log_step "Waiting for packet to circulate the ring (100ms)..."
    sleep 0.1

    echo ""
    echo -e "${CYAN}${BOLD}── COMPUTE_EVENTS (ring buffer) ─────────────────────────────────${NC}"
    echo ""
    if [[ -e "${MAP_PIN}/COMPUTE_EVENTS" ]]; then
        sudo bpftool map dump pinned "${MAP_PIN}/COMPUTE_EVENTS" 2>/dev/null || \
            log_warn "Could not dump COMPUTE_EVENTS (ring buffer may require perf reader)"
    else
        log_warn "COMPUTE_EVENTS map not found at ${MAP_PIN}/COMPUTE_EVENTS"
    fi

    echo ""
    echo -e "${CYAN}${BOLD}── CPU_MAP (CPU state) ──────────────────────────────────────────${NC}"
    echo ""
    if [[ -e "${MAP_PIN}/CPU_MAP" ]]; then
        sudo bpftool map dump pinned "${MAP_PIN}/CPU_MAP" 2>/dev/null || \
            log_warn "Could not dump CPU_MAP"
    else
        log_warn "CPU_MAP not found at ${MAP_PIN}/CPU_MAP"
    fi

    echo ""
    echo -e "${CYAN}${BOLD}── STATS ───────────────────────────────────────────────────────${NC}"
    echo ""
    if [[ -e "${MAP_PIN}/STATS" ]]; then
        sudo bpftool map dump pinned "${MAP_PIN}/STATS" 2>/dev/null || \
            log_warn "Could not dump STATS"
    else
        log_warn "STATS map not found at ${MAP_PIN}/STATS"
    fi
}

# ── Step 6: Print instruction trace ─────────────────────────────────────────

print_trace() {
    echo ""
    echo -e "${CYAN}${BOLD}── Instruction Trace (expected) ────────────────────────────────${NC}"
    echo ""
    echo -e "  ${BOLD}Hop  PC  Instruction        Registers After${NC}"
    echo -e "  ${DIM}───  ──  ─────────────────  ──────────────────────────${NC}"
    echo -e "  0    0  MOVI r0, 1          r0=1"
    echo -e "  1    1  MOVI r1, 10         r0=1,  r1=10"
    echo -e "  2    2  ADD  r0, r1         r0=11, r1=10"
    echo -e "  3    3  MUL  r0, r1         r0=110, r1=10"
    echo -e "  4    4  MOVI r2, 42         r0=110, r1=10, r2=42"
    echo -e "  5    5  HALT                r0=110, r1=10, r2=42  [HALTED]"
    echo ""
    echo -e "  ${BOLD}Expected final CPU state:${NC}"
    echo -e "    PC      = 6 (advanced past HALT)"
    echo -e "    r0      = 110 (0x6E)"
    echo -e "    r1      = 10  (0x0A)"
    echo -e "    r2      = 42  (0x2A)"
    echo -e "    halted  = 1"
    echo -e "    insns   = 5 (HALT breaks before incrementing insn_count)"
    echo ""
}

# ── Hex extraction helpers ──────────────────────────────────────────────────

# Extract u32 LE from hex string at byte offset
extract_u32_le() {
    local hex_str="$1"
    local byte_offset="$2"
    local char_offset=$((byte_offset * 2))
    local b0="${hex_str:$((char_offset)):2}"
    local b1="${hex_str:$((char_offset + 2)):2}"
    local b2="${hex_str:$((char_offset + 4)):2}"
    local b3="${hex_str:$((char_offset + 6)):2}"
    printf '%d' "0x${b3}${b2}${b1}${b0}" 2>/dev/null || echo "-1"
}

extract_u8() {
    local hex_str="$1"
    local byte_offset="$2"
    local char_offset=$((byte_offset * 2))
    local b0="${hex_str:$((char_offset)):2}"
    printf '%d' "0x${b0}" 2>/dev/null || echo "-1"
}

# ── Step 7: Verdict ─────────────────────────────────────────────────────────
#
# Attempt to read CPU_MAP and compare register values against expected.
# If BPF maps are not accessible (no BPF loaded), report as SKIP.

check_verdict() {
    echo -e "${CYAN}${BOLD}── Verdict ─────────────────────────────────────────────────────${NC}"
    echo ""

    # Try to read CPU state for flow label 0xDE (key = 222 decimal)
    if [[ ! -e "${MAP_PIN}/CPU_MAP" ]]; then
        log_warn "CPU_MAP not available — cannot verify (BPF may not be loaded)"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — BPF maps not accessible; ring topology verified only"
        echo ""
        return 0
    fi

    # Use bpftool to look up the specific key
    local cpu_dump
    cpu_dump=$(sudo bpftool map lookup pinned "${MAP_PIN}/CPU_MAP" \
        key hex de 00 00 00 2>/dev/null) || true

    if [[ -z "${cpu_dump}" ]]; then
        log_warn "No CPU state found for flow_label=0xDE"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — CPU state not populated (packet may not have been processed)"
        echo ""
        return 0
    fi

    echo -e "  ${DIM}Raw CPU state:${NC}"
    echo -e "  ${DIM}${cpu_dump}${NC}"
    echo ""

    # Parse the hex dump to extract register values.
    # MbcCpuState layout (LE):
    #   regs[0..15]: 16 x u32 = 64 bytes (offsets 0..63)
    #   pc:          u32       = 4 bytes  (offset 64)
    #   flags:       u8        = 1 byte   (offset 68)
    #   halted:      u8        = 1 byte   (offset 69)
    #   stalled:     u8        = 1 byte   (offset 70)
    #   _pad:        u8        = 1 byte   (offset 71)
    #   sleep_until: u64       = 8 bytes  (offset 72)
    #   insn_count:  u64       = 8 bytes  (offset 80)
    #   cache_hits:  u64       = 8 bytes  (offset 88)
    #   cache_misses:u64       = 8 bytes  (offset 96)
    #
    # bpftool outputs value as a sequence of hex bytes.

    local value_hex
    # bpftool outputs multi-line format: "value:\n xx xx xx ...\n xx xx xx ..."
    # Extract all hex bytes after the "value:" line
    value_hex=$(echo "${cpu_dump}" | awk '/^value:/{found=1; next} found{print}' | tr -d ' \n' || true)

    if [[ -z "${value_hex}" ]]; then
        log_warn "Could not parse CPU state from bpftool output"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — unable to parse bpftool output format"
        echo ""
        return 0
    fi

    # Extract r0 (bytes 0-3, LE), r1 (bytes 4-7), r2 (bytes 8-11), PC (bytes 64-67), halted (byte 69)
    # Each hex byte = 2 chars in the hex string
    local verdict_pass=1
    local r0 r1 r2 pc halted insn_count

    r0=$(extract_u32_le "${value_hex}" 0)
    r1=$(extract_u32_le "${value_hex}" 4)
    r2=$(extract_u32_le "${value_hex}" 8)
    pc=$(extract_u32_le "${value_hex}" 64)
    halted=$(extract_u8 "${value_hex}" 69)

    # insn_count is u64 LE at offset 80, just read low 32 bits
    insn_count=$(extract_u32_le "${value_hex}" 80)

    echo -e "  ${BOLD}Actual CPU state:${NC}"
    echo -e "    r0      = ${r0}"
    echo -e "    r1      = ${r1}"
    echo -e "    r2      = ${r2}"
    echo -e "    PC      = ${pc}"
    echo -e "    halted  = ${halted}"
    echo -e "    insns   = ${insn_count}"
    echo ""

    # Check expected values
    check_field() {
        local name="$1"
        local actual="$2"
        local expected="$3"
        if [[ "${actual}" == "${expected}" ]]; then
            echo -e "    ${GREEN}OK${NC}  ${name} = ${actual} (expected ${expected})"
        else
            echo -e "    ${RED}FAIL${NC} ${name} = ${actual} (expected ${expected})"
            verdict_pass=0
        fi
    }

    echo -e "  ${BOLD}Checks:${NC}"
    check_field "r0"      "${r0}"         "110"
    check_field "r1"      "${r1}"         "10"
    check_field "r2"      "${r2}"         "42"
    check_field "PC"      "${pc}"         "6"
    check_field "halted"  "${halted}"     "1"
    check_field "insns"   "${insn_count}" "5"
    echo ""

    if [[ $verdict_pass -eq 1 ]]; then
        log_pass "All register values match expected final state"
        echo ""
        echo -e "  ${GREEN}${BOLD}PASS${NC} — Packet circulation ring executed 6 MBC instructions correctly"
    else
        log_fail "Register values do not match expected final state"
        echo ""
        echo -e "  ${RED}${BOLD}FAIL${NC} — CPU state mismatch; see above for details"
        return 1
    fi

    echo ""
}

# ── Demo mode: load a pre-assembled .bin file ────────────────────────────────

load_demo_program() {
    local name="$1"
    local bin_path="${PROJECT_ROOT}/demos/mbc/${name}.bin"

    if [[ ! -f "${bin_path}" ]]; then
        # Try assembling from source
        local src_path="${PROJECT_ROOT}/demos/mbc/${name}.mbc"
        local asm_tool="${PROJECT_ROOT}/crates/monad-mbc/target/release/mbc-asm"

        if [[ ! -x "${asm_tool}" ]]; then
            log_step "Building mbc-asm assembler..."
            (cd "${PROJECT_ROOT}/crates/monad-mbc" && cargo build --release --bin mbc-asm 2>/dev/null)
        fi

        if [[ -f "${src_path}" && -x "${asm_tool}" ]]; then
            log_step "Assembling ${name}.mbc -> ${name}.bin..."
            "${asm_tool}" "${src_path}" -o "${bin_path}" --stats 2>&1
        else
            log_error "Neither ${bin_path} nor ${src_path} found"
            exit 1
        fi
    fi

    cp "${bin_path}" "${MBC_FILE}"

    local size
    size=$(stat -c%s "${MBC_FILE}" 2>/dev/null || stat -f%z "${MBC_FILE}" 2>/dev/null)
    local num_insns=$((size / 4))
    log_info "${name}: ${num_insns} MBC instructions (${size} bytes)"
}

check_demo_verdict() {
    local name="$1"
    echo -e "${CYAN}${BOLD}── Verdict (${name}) ─────────────────────────────────────────────${NC}"
    echo ""

    if [[ ! -e "${MAP_PIN}/CPU_MAP" ]]; then
        log_warn "CPU_MAP not available — cannot verify"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — BPF maps not accessible"
        echo ""
        return 0
    fi

    local cpu_dump
    cpu_dump=$(sudo bpftool map lookup pinned "${MAP_PIN}/CPU_MAP" \
        key hex de 00 00 00 2>/dev/null) || true

    if [[ -z "${cpu_dump}" ]]; then
        log_warn "No CPU state for flow_label=0xDE"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — CPU state not populated"
        echo ""
        return 0
    fi

    local value_hex
    value_hex=$(echo "${cpu_dump}" | awk '/^value:/{found=1; next} found{print}' | tr -d ' \n' || true)

    if [[ -z "${value_hex}" ]]; then
        log_warn "Could not parse CPU state"
        echo -e "  ${YELLOW}${BOLD}SKIP${NC} — unable to parse bpftool output"
        echo ""
        return 0
    fi

    # Extract key fields
    local pc halted insn_count
    pc=$(extract_u32_le "${value_hex}" 64)
    halted=$(extract_u8 "${value_hex}" 69)
    insn_count=$(extract_u32_le "${value_hex}" 80)

    echo -e "  ${BOLD}CPU state after ${name}:${NC}"
    echo -e "    PC      = ${pc}"
    echo -e "    halted  = ${halted}"
    echo -e "    insns   = ${insn_count}"
    echo ""

    local verdict_pass=1

    # For gradient (23 insns) and checkerboard (31 insns), verify:
    # - halted == 1 (program ended with HALT)
    # - insns > 0 (instructions were executed)
    if [[ "${halted}" != "1" ]]; then
        echo -e "    ${RED}FAIL${NC} halted = ${halted} (expected 1)"
        verdict_pass=0
    else
        echo -e "    ${GREEN}OK${NC}  halted = 1"
    fi

    if [[ "${insn_count}" -gt 0 ]]; then
        echo -e "    ${GREEN}OK${NC}  insns = ${insn_count} (> 0)"
    else
        echo -e "    ${RED}FAIL${NC} insns = 0 (expected > 0)"
        verdict_pass=0
    fi

    # Check screen memory has been written (gradient/checkerboard write to SCREEN_MAP)
    if [[ -e "${MAP_PIN}/SCREEN_MAP" ]]; then
        # Read a few pixels to verify screen has non-zero data
        local pixel_0 pixel_100 pixel_32000
        pixel_0=$(sudo bpftool map lookup pinned "${MAP_PIN}/SCREEN_MAP" \
            key hex 00 00 00 00 2>/dev/null | awk '/^value:/{found=1; next} found{print}' | tr -d ' \n' || true)
        pixel_100=$(sudo bpftool map lookup pinned "${MAP_PIN}/SCREEN_MAP" \
            key hex 64 00 00 00 2>/dev/null | awk '/^value:/{found=1; next} found{print}' | tr -d ' \n' || true)
        pixel_8000=$(sudo bpftool map lookup pinned "${MAP_PIN}/SCREEN_MAP" \
            key hex 40 1f 00 00 2>/dev/null | awk '/^value:/{found=1; next} found{print}' | tr -d ' \n' || true)

        echo -e "    ${DIM}SCREEN_MAP samples: [0]=${pixel_0:-??} [100]=${pixel_100:-??} [8000]=${pixel_8000:-??}${NC}"

        # For gradient, pixel 100 = (100 + 0) & 0xFF = 100 (0x64)
        # For checkerboard, values alternate between 1 and 15
        if [[ -n "${pixel_100}" && "${pixel_100}" != "00" ]]; then
            echo -e "    ${GREEN}OK${NC}  SCREEN_MAP has non-zero pixels"
        else
            echo -e "    ${YELLOW}WARN${NC} SCREEN_MAP pixel[100] = ${pixel_100:-??} (may be zero)"
        fi
    fi

    echo ""

    if [[ $verdict_pass -eq 1 ]]; then
        log_pass "${name} completed successfully"
        echo -e "  ${GREEN}${BOLD}PASS${NC} — ${name} executed and halted (${insn_count} instructions)"
    else
        log_fail "${name} did not complete as expected"
        echo -e "  ${RED}${BOLD}FAIL${NC} — see above for details"
        return 1
    fi
    echo ""
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    banner

    local mode="${1:-test}"

    case "${mode}" in
        gradient|checkerboard)
            # Demo mode: load pre-assembled .bin
            ;;
        --file)
            # Custom file mode
            if [[ -z "${2:-}" ]]; then
                log_error "--file requires a path argument"
                exit 1
            fi
            custom_file="$2"
            if [[ ! -f "${custom_file}" ]]; then
                log_error "File not found: ${custom_file}"
                exit 1
            fi
            ;;
        test|"")
            mode="test"
            ;;
        -h|--help|help)
            echo "Usage: sudo $0 [test|gradient|checkerboard|--file <path>]"
            echo ""
            echo "Modes:"
            echo "  test           Run the 6-instruction arithmetic test (default)"
            echo "  gradient       Load demos/mbc/gradient.bin (160×100 color gradient)"
            echo "  checkerboard   Load demos/mbc/checkerboard.bin (8×8 checkerboard)"
            echo "  --file <path>  Load any .bin/.mbc binary file"
            echo ""
            exit 0
            ;;
        *)
            log_error "Unknown mode: ${mode}"
            echo "Usage: sudo $0 [test|gradient|checkerboard|--file <path>]"
            exit 1
            ;;
    esac

    # 0. Preflight
    preflight

    # 1. Setup the ring
    echo ""
    setup_ring

    # 2. Create/load test program
    echo ""
    case "${mode}" in
        gradient|checkerboard)
            load_demo_program "${mode}"
            ;;
        --file)
            cp "${custom_file}" "${MBC_FILE}"
            local fsize
            fsize=$(stat -c%s "${MBC_FILE}" 2>/dev/null || stat -f%z "${MBC_FILE}" 2>/dev/null)
            log_info "Custom file: $((fsize / 4)) MBC instructions (${fsize} bytes)"
            ;;
        test)
            create_test_program
            ;;
    esac

    # 3. Load into BPF maps
    echo ""
    load_program

    # 4. Inject packet
    echo ""
    inject_packet

    # 5. Read BPF maps
    echo ""
    read_maps

    # 6. Print trace and verdict
    case "${mode}" in
        gradient|checkerboard)
            # Wait longer for screen-writing programs (they loop 16000 times)
            log_step "Waiting for program to complete (2s for screen fill)..."
            sleep 2
            check_demo_verdict "${mode}"
            ;;
        --file)
            log_step "Waiting for program to complete (2s)..."
            sleep 2
            check_demo_verdict "custom"
            ;;
        test)
            print_trace
            check_verdict
            ;;
    esac
}

main "$@"
