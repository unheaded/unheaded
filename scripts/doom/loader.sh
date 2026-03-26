#!/usr/bin/env bash
# doom-loader.sh — Load Doom ROM, data sections, WAD, and CPU state into BPF maps
#
# Prerequisites:
#   - doom-ring.sh setup has been run (namespaces, BPF loaded, maps pinned)
#   - doom.mbc exists (translated MBC binary)
#   - doom.elf exists (for data section extraction)
#   - doom1.wad exists
#
# Usage:
#   sudo ./scripts/doom-loader.sh [--rom-only] [--rv2mbc-only] [--data-only] [--wad-only] [--cpu-only]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

# Paths
BPFFS_ROOT="/sys/fs/bpf/unheaded/doom-ring"
MAP_PIN_DIR="${BPFFS_ROOT}/maps"
DOOM_DIR="${PROJECT_ROOT}/doom/doomgeneric/doomgeneric"
DOOM_MBC="${DOOM_DIR}/doom.mbc"
DOOM_ELF="${DOOM_DIR}/doom.elf"
DOOM_WAD="${PROJECT_ROOT}/doom/doom1.wad"
DOOM_DATA="${DOOM_DIR}/doom_data.bin"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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
# ROM LOADER — Load doom.mbc into ROM_MAP (Array<u32>)
# ============================================================================

load_rom() {
    log_step "Loading ROM (doom.mbc → ROM_MAP)..."

    if [[ ! -f "$DOOM_MBC" ]]; then
        log_error "doom.mbc not found at ${DOOM_MBC}"
        exit 1
    fi

    local rom_pin="${MAP_PIN_DIR}/ROM_MAP"
    if [[ ! -f "$rom_pin" ]]; then
        log_error "ROM_MAP not pinned at ${rom_pin}. Run doom-ring.sh setup first."
        exit 1
    fi

    local size
    size=$(stat -c%s "$DOOM_MBC")
    local num_words=$((size / 4))
    log_info "ROM: ${num_words} words (${size} bytes)"

    # Use Python for bulk loading via bpftool batch
    python3 "${SCRIPT_DIR}/loader_core.py" rom "$rom_pin" "$DOOM_MBC"

    log_info "ROM loaded: ${num_words} instructions"
}

# ============================================================================
# DATA LOADER — Load .rodata/.data sections into RAM_MAP
# ============================================================================

load_data() {
    log_step "Loading data sections (doom.elf → RAM_MAP)..."

    local ram_pin="${MAP_PIN_DIR}/RAM_MAP"
    if [[ ! -f "$ram_pin" ]]; then
        log_error "RAM_MAP not pinned at ${ram_pin}. Run doom-ring.sh setup first."
        exit 1
    fi

    # Extract data sections if not already done
    if [[ ! -f "$DOOM_DATA" ]]; then
        log_info "Extracting data sections from doom.elf..."
        riscv64-unknown-elf-objcopy -O binary \
            -j .rodata -j '.srodata*' -j .data \
            "$DOOM_ELF" "$DOOM_DATA"
    fi

    local data_size
    data_size=$(stat -c%s "$DOOM_DATA")
    log_info "Data sections: ${data_size} bytes"

    # Data starts at .rodata VMA (first extracted section). Compute from ELF
    # to avoid stale hardcoded values when .text size changes across builds.
    local data_start_addr
    data_start_addr=$(riscv64-unknown-elf-objdump -h "$DOOM_ELF" \
        | awk '/\.rodata[[:space:]]/{print "0x"$4}')
    if [[ -z "$data_start_addr" ]]; then
        log_error "Could not determine .rodata VMA from ${DOOM_ELF}"
        exit 1
    fi
    log_info "Data start address: ${data_start_addr} (.rodata VMA)"

    python3 "${SCRIPT_DIR}/loader_core.py" ram "$ram_pin" "$DOOM_DATA" "$data_start_addr"

    log_info "Data sections loaded"
}

# ============================================================================
# WAD LOADER — Load doom1.wad into RAM_MAP at 0x800000 (matches linker.ld WAD_BASE)
# ============================================================================

load_wad() {
    log_step "Loading WAD (doom1.wad → RAM_MAP @ 0x800000)..."

    local ram_pin="${MAP_PIN_DIR}/RAM_MAP"
    if [[ ! -f "$ram_pin" ]]; then
        log_error "RAM_MAP not pinned at ${ram_pin}. Run doom-ring.sh setup first."
        exit 1
    fi

    if [[ ! -f "$DOOM_WAD" ]]; then
        log_error "doom1.wad not found at ${DOOM_WAD}"
        exit 1
    fi

    local wad_size
    wad_size=$(stat -c%s "$DOOM_WAD")
    log_info "WAD: ${wad_size} bytes"

    # WAD starts at byte address 0x800000 (from linker.ld WAD ORIGIN + libc_stubs.c WAD_BASE)
    local wad_start_addr=0x800000

    python3 "${SCRIPT_DIR}/loader_core.py" ram "$ram_pin" "$DOOM_WAD" "$wad_start_addr"

    log_info "WAD loaded"
}

# ============================================================================
# RV2MBC LOADER — Load RV32I→MBC address translation map
# ============================================================================

load_rv2mbc() {
    log_step "Loading RV2MBC map (doom.rv2mbc → RV2MBC_MAP)..."

    local rv2mbc_file="${DOOM_DIR}/doom.rv2mbc"
    if [[ ! -f "$rv2mbc_file" ]]; then
        log_error "doom.rv2mbc not found at ${rv2mbc_file}"
        log_error "Re-run rv32i-to-mbc with -o to generate it."
        exit 1
    fi

    local rv2mbc_pin="${MAP_PIN_DIR}/RV2MBC_MAP"
    if [[ ! -f "$rv2mbc_pin" ]]; then
        log_error "RV2MBC_MAP not pinned at ${rv2mbc_pin}. Run doom-ring.sh setup first."
        exit 1
    fi

    local size
    size=$(stat -c%s "$rv2mbc_file")
    local num_entries=$((size / 4))
    log_info "RV2MBC: ${num_entries} entries (${size} bytes)"

    python3 "${SCRIPT_DIR}/loader_core.py" rv2mbc "$rv2mbc_pin" "$rv2mbc_file"

    log_info "RV2MBC map loaded: ${num_entries} entries"
}

# ============================================================================
# CPU STATE — Initialize CPU_MAP with starting state
# ============================================================================

load_cpu() {
    log_step "Initializing CPU state (CPU_MAP)..."

    local cpu_pin="${MAP_PIN_DIR}/CPU_MAP"
    if [[ ! -f "$cpu_pin" ]]; then
        log_error "CPU_MAP not pinned at ${cpu_pin}. Run doom-ring.sh setup first."
        exit 1
    fi

    # Instance 0xDE matches the default flow_label in bulk_inject.py
    python3 "${SCRIPT_DIR}/loader_core.py" cpu "$cpu_pin" 0xDE

    log_info "CPU state initialized: instance=0xDE, PC=0, SP=0x3F00000"
}

# ============================================================================
# MAIN
# ============================================================================

require_root

if [[ ! -d "$MAP_PIN_DIR" ]]; then
    log_error "Map pin directory not found: ${MAP_PIN_DIR}"
    log_error "Run: sudo ./scripts/doom-ring.sh setup"
    exit 1
fi

case "${1:-all}" in
    all)
        load_rom
        load_rv2mbc
        load_data
        load_wad
        load_cpu
        log_info "All data loaded! Ready to inject tick packets."
        ;;
    rom|--rom-only)
        load_rom
        ;;
    rv2mbc|--rv2mbc-only)
        load_rv2mbc
        ;;
    data|--data-only)
        load_data
        ;;
    wad|--wad-only)
        load_wad
        ;;
    cpu|--cpu-only)
        load_cpu
        ;;
    *)
        echo "Usage: sudo $0 {all|rom|rv2mbc|data|wad|cpu}"
        exit 1
        ;;
esac
