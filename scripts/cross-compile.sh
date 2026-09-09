#!/usr/bin/env bash
# cross-compile.sh -- C -> RV32I ELF -> MBC cross-compilation pipeline
#
# Compiles C source to a RISC-V 32-bit ELF, then translates the ELF to
# Monad Bytecode (MBC) using the rv32i-to-mbc translator.
#
# Usage:
#   ./scripts/cross-compile.sh <input.c> [output.mbc]
#   ./scripts/cross-compile.sh demos/mbc/sum99.c
#   ./scripts/cross-compile.sh demos/mbc/sum99.c demos/mbc/sum99.mbc
#
# If output is omitted, it is derived from the input filename.
#
# Requirements:
#   - riscv64-unknown-elf-gcc (apt: gcc-riscv64-unknown-elf)
#   - Rust toolchain (for building rv32i-to-mbc)
#
# The pipeline:
#   1. C --> RV32I ELF  (riscv64-unknown-elf-gcc -march=rv32i -mabi=ilp32)
#   2. RV32I ELF --> MBC  (rv32i-to-mbc translator)
#
# Compiler flags explained:
#   -march=rv32i         Target RV32I base integer ISA (no M/A/F/D extensions)
#   -mabi=ilp32          32-bit int/long/pointer ABI
#   -nostdlib            No standard library (bare metal)
#   -static              Static linking (no dynamic loader)
#   -ffreestanding       Freestanding environment (no hosted assumptions)
#   -fno-builtin         Don't use GCC builtins (memcpy, etc.)
#   -O2                  Optimize for speed (also reduces register pressure)
#   -ffixed-x16..x31    Don't use registers x16-x31 (MBC only has r0-r15)
#   -Wl,-Ttext=0x0      Place .text at address 0 (MBC ROM base)

set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ── Paths ─────────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
RV32I_TO_MBC="${PROJECT_ROOT}/target/release/rv32i-to-mbc"

# ── Usage ─────────────────────────────────────────────────────────────────────

usage() {
    echo "Usage: $0 <input.c> [output.mbc]"
    echo ""
    echo "Cross-compile C source to MBC bytecode via RV32I."
    echo ""
    echo "  input.c      C source file"
    echo "  output.mbc   MBC output file (default: derived from input)"
    echo ""
    echo "Options:"
    echo "  --disasm     Show MBC disassembly after translation"
    echo "  --stats      Show translation statistics"
    echo "  --keep-elf   Don't delete the intermediate ELF file"
    echo "  --objdump    Run riscv64-unknown-elf-objdump on the ELF"
    echo "  -O0|-O1|-O2|-O3|-Os  Optimization level (default: -O2)"
    echo "  -h, --help   Show this help"
    echo ""
    echo "Examples:"
    echo "  $0 demos/mbc/sum99.c"
    echo "  $0 demos/mbc/sum99.c demos/mbc/sum99.mbc --disasm --stats"
    echo "  $0 demos/mbc/add42.c --objdump --keep-elf"
    exit "${1:-0}"
}

# ── Parse arguments ───────────────────────────────────────────────────────────

INPUT=""
OUTPUT=""
SHOW_DISASM=false
SHOW_STATS=false
KEEP_ELF=false
SHOW_OBJDUMP=false
OPT_LEVEL="-O2"

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage 0
            ;;
        --disasm)
            SHOW_DISASM=true
            shift
            ;;
        --stats)
            SHOW_STATS=true
            shift
            ;;
        --keep-elf)
            KEEP_ELF=true
            shift
            ;;
        --objdump)
            SHOW_OBJDUMP=true
            KEEP_ELF=true  # need ELF for objdump
            shift
            ;;
        -O0|-O1|-O2|-O3|-Os)
            OPT_LEVEL="$1"
            shift
            ;;
        -*)
            echo -e "${RED}Error:${NC} Unknown option: $1" >&2
            usage 1
            ;;
        *)
            if [[ -z "$INPUT" ]]; then
                INPUT="$1"
            elif [[ -z "$OUTPUT" ]]; then
                OUTPUT="$1"
            else
                echo -e "${RED}Error:${NC} Too many arguments" >&2
                usage 1
            fi
            shift
            ;;
    esac
done

if [[ -z "$INPUT" ]]; then
    echo -e "${RED}Error:${NC} No input file specified" >&2
    usage 1
fi

if [[ ! -f "$INPUT" ]]; then
    echo -e "${RED}Error:${NC} Input file not found: $INPUT" >&2
    exit 1
fi

# Derive output paths
BASENAME="$(basename "$INPUT" .c)"
DIRNAME="$(dirname "$INPUT")"

if [[ -z "$OUTPUT" ]]; then
    OUTPUT="${DIRNAME}/${BASENAME}.mbc"
fi

ELF_FILE="${DIRNAME}/${BASENAME}.elf"

# ── Banner ────────────────────────────────────────────────────────────────────

echo ""
echo -e "${CYAN}${BOLD}Cross-Compile Pipeline: C -> RV32I -> MBC${NC}"
echo -e "${DIM}────────────────────────────────────────────${NC}"
echo -e "  Input:  ${INPUT}"
echo -e "  ELF:    ${ELF_FILE}"
echo -e "  Output: ${OUTPUT}"
echo -e "  Opt:    ${OPT_LEVEL}"
echo ""

# ── Step 0: Preflight checks ─────────────────────────────────────────────────

# Check for RISC-V cross-compiler
CC=""
if command -v riscv64-unknown-elf-gcc &>/dev/null; then
    CC="riscv64-unknown-elf-gcc"
elif command -v riscv32-unknown-elf-gcc &>/dev/null; then
    CC="riscv32-unknown-elf-gcc"
elif command -v riscv64-linux-gnu-gcc &>/dev/null; then
    CC="riscv64-linux-gnu-gcc"
else
    echo -e "${RED}Error:${NC} No RISC-V cross-compiler found." >&2
    echo "Install with:" >&2
    echo "  sudo apt-get update && sudo apt-get install -y gcc-riscv64-unknown-elf" >&2
    exit 1
fi
echo -e "${GREEN}[OK]${NC} Cross-compiler: ${CC}"

# Check for rv32i-to-mbc translator
if [[ ! -x "$RV32I_TO_MBC" ]]; then
    echo -e "${YELLOW}[BUILD]${NC} rv32i-to-mbc not found, building..."
    if ! cargo build --release -p monad-mbc --bin rv32i-to-mbc \
         --manifest-path "${PROJECT_ROOT}/crates/monad-mbc/Cargo.toml" 2>&1; then
        echo -e "${RED}Error:${NC} Failed to build rv32i-to-mbc" >&2
        exit 1
    fi
    if [[ ! -x "$RV32I_TO_MBC" ]]; then
        # Try alternate path (workspace target dir)
        ALT_BIN="${PROJECT_ROOT}/crates/monad-mbc/target/release/rv32i-to-mbc"
        if [[ -x "$ALT_BIN" ]]; then
            RV32I_TO_MBC="$ALT_BIN"
        else
            echo -e "${RED}Error:${NC} rv32i-to-mbc binary not found after build" >&2
            echo "Searched: ${RV32I_TO_MBC}" >&2
            echo "      and: ${ALT_BIN}" >&2
            exit 1
        fi
    fi
fi
echo -e "${GREEN}[OK]${NC} Translator:     ${RV32I_TO_MBC}"
echo ""

# ── Step 1: C -> RV32I ELF ───────────────────────────────────────────────────

echo -e "${BLUE}[1/2]${NC} Compiling C -> RV32I ELF..."

# Build the GCC command
# -ffixed-x16 through -ffixed-x31: restrict GCC to only use x0-x15
# (MBC only has 16 registers: r0-r15)
# shellcheck disable=SC2054  # the comma is inside -Wl,-Ttext=0x0, which is one
# GCC argument, not an array separator.
GCC_CMD=(
    "$CC"
    -march=rv32i
    -mabi=ilp32
    -nostdlib
    -static
    -ffreestanding
    -fno-builtin
    "$OPT_LEVEL"
    # Restrict to x0-x15 (MBC register file is 16 registers)
    -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19
    -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23
    -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27
    -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31
    # Place .text at address 0 (MBC ROM base address)
    -Wl,-Ttext=0x0
    -o "$ELF_FILE"
    "$INPUT"
)

echo -e "${DIM}  \$ ${GCC_CMD[*]}${NC}"

if ! "${GCC_CMD[@]}" 2>&1; then
    echo -e "${RED}Error:${NC} Cross-compilation failed" >&2
    exit 1
fi

ELF_SIZE=$(stat -c%s "$ELF_FILE" 2>/dev/null || stat -f%z "$ELF_FILE" 2>/dev/null)
echo -e "${GREEN}[OK]${NC} ELF: ${ELF_FILE} (${ELF_SIZE} bytes)"

# Optional: show objdump
if $SHOW_OBJDUMP; then
    echo ""
    echo -e "${CYAN}${BOLD}RV32I Disassembly (.text section):${NC}"
    OBJDUMP=""
    if command -v riscv64-unknown-elf-objdump &>/dev/null; then
        OBJDUMP="riscv64-unknown-elf-objdump"
    elif command -v riscv32-unknown-elf-objdump &>/dev/null; then
        OBJDUMP="riscv32-unknown-elf-objdump"
    elif command -v riscv64-linux-gnu-objdump &>/dev/null; then
        OBJDUMP="riscv64-linux-gnu-objdump"
    fi

    if [[ -n "$OBJDUMP" ]]; then
        "$OBJDUMP" -d -M no-aliases "$ELF_FILE" 2>/dev/null || \
            "$OBJDUMP" -d "$ELF_FILE" 2>/dev/null || \
            echo "  (objdump failed)"
    else
        echo "  (no objdump found)"
    fi
    echo ""
fi

# ── Step 2: RV32I ELF -> MBC ─────────────────────────────────────────────────

echo -e "${BLUE}[2/2]${NC} Translating RV32I ELF -> MBC..."

TRANS_ARGS=("$ELF_FILE" -o "$OUTPUT")

if $SHOW_STATS; then
    TRANS_ARGS+=(--stats)
fi

if $SHOW_DISASM; then
    TRANS_ARGS+=(--disasm)
fi

echo -e "${DIM}  \$ rv32i-to-mbc ${TRANS_ARGS[*]}${NC}"

if ! "$RV32I_TO_MBC" "${TRANS_ARGS[@]}" 2>&1; then
    echo ""
    echo -e "${RED}Error:${NC} RV32I -> MBC translation failed" >&2
    echo ""
    echo "Common causes:" >&2
    echo "  - Registers x16-x31 used (add more -ffixed-xN flags)" >&2
    echo "  - Unsupported instructions (register-based shifts, etc.)" >&2
    echo "  - General JALR (indirect jumps other than RET)" >&2
    echo "" >&2
    echo "Debug: run with --objdump to see the RV32I disassembly" >&2

    # Cleanup ELF on failure unless --keep-elf
    if ! $KEEP_ELF && [[ -f "$ELF_FILE" ]]; then
        rm -f "$ELF_FILE"
    fi
    exit 1
fi

MBC_SIZE=$(stat -c%s "$OUTPUT" 2>/dev/null || stat -f%z "$OUTPUT" 2>/dev/null)
echo -e "${GREEN}[OK]${NC} MBC: ${OUTPUT} (${MBC_SIZE} bytes)"

# ── Cleanup ───────────────────────────────────────────────────────────────────

if ! $KEEP_ELF && [[ -f "$ELF_FILE" ]]; then
    rm -f "$ELF_FILE"
    echo -e "${DIM}  Removed intermediate ELF${NC}"
fi

# ── Summary ───────────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}${BOLD}Pipeline complete!${NC}"
echo -e "  C source:  ${INPUT}"
if $KEEP_ELF; then
    echo -e "  RV32I ELF: ${ELF_FILE} (${ELF_SIZE} bytes)"
fi
echo -e "  MBC:       ${OUTPUT} (${MBC_SIZE} bytes)"
MBC_INSNS=$((MBC_SIZE / 4))
echo -e "  MBC insns: ${MBC_INSNS}"
echo ""
