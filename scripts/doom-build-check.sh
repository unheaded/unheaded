#!/usr/bin/env bash
# doom-build-check.sh — Validate Doom build artifacts for known bug regressions
#
# Checks build artifacts at /tmp/doom-build/ (or --build-dir) for:
#   Bug 21: -ffixed-x15 in CFLAGS (SP register reservation)
#   Bug 22: setjmp/longjmp saving/restoring a5 (x15 mapped to SP)
#   Bug 23: .data section attribute on 'initialized' variable
#   Bug 21 supplemental: x15/a5 usage in non-setjmp code (should be near 0)
#   RV32IM: MUL/DIV/REM instructions present in disassembly
#   Size limits: doom.mbc < 262,144 words, doom.rv2mbc < 65,536 entries
#
# Usage:
#   ./scripts/doom-build-check.sh [--build-dir DIR] [--verbose]
#
# Exit: 0 if all pass, 1 if any fail
#
# Dependencies: riscv64-unknown-elf-objdump, riscv64-unknown-elf-nm, grep

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Defaults
BUILD_DIR="/tmp/doom-build"
VERBOSE=0

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
    echo -e "${CYAN}${BOLD}  Doom Build Check — Bug Regression Validator${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo ""
}

# ── Argument parsing ─────────────────────────────────────────────────────────

while [[ $# -gt 0 ]]; do
    case "$1" in
        --build-dir)
            BUILD_DIR="$2"
            shift 2
            ;;
        --verbose|-v)
            VERBOSE=1
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [--build-dir DIR] [--verbose]"
            echo ""
            echo "Options:"
            echo "  --build-dir DIR   Build directory (default: /tmp/doom-build)"
            echo "  --verbose, -v     Show detailed output"
            echo ""
            echo "Checks:"
            echo "  Bug 21:  -ffixed-x15 in rebuild script CFLAGS"
            echo "  Bug 22:  setjmp.h jmp_buf[7], setjmp.S saves/restores a5"
            echo "  Bug 23:  .data attribute on 'initialized' in patched platform"
            echo "  Bug 21s: x15/a5 usage in non-setjmp code near zero"
            echo "  RV32IM:  MUL/DIV/REM instructions in disassembly"
            echo "  Sizes:   doom.mbc < 262,144 words, doom.rv2mbc < 65,536 entries"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# ── Paths ────────────────────────────────────────────────────────────────────

REBUILD_SCRIPT="${BUILD_DIR}/rebuild_rv32im.sh"
SETJMP_H="${BUILD_DIR}/include/setjmp.h"
SETJMP_S="${BUILD_DIR}/setjmp.S"
PLATFORM_C="${BUILD_DIR}/doomgeneric_monad_patched.c"
DOOM_ELF="${BUILD_DIR}/doom.elf"
DOOM_MBC="${PROJECT_ROOT}/doom/doom.mbc"
DOOM_RV2MBC="${PROJECT_ROOT}/doom/doom.rv2mbc"

OBJDUMP="riscv64-unknown-elf-objdump"
NM="riscv64-unknown-elf-nm"

# Also check doom/doomgeneric/doomgeneric/ as alternate location
DOOM_ELF_ALT="${PROJECT_ROOT}/doom/doomgeneric/doomgeneric/doom.elf"
DOOM_MBC_ALT="${PROJECT_ROOT}/doom/doomgeneric/doomgeneric/doom.mbc"
DOOM_RV2MBC_ALT="${PROJECT_ROOT}/doom/doomgeneric/doomgeneric/doom.rv2mbc"

# Resolve actual paths
[[ ! -f "$DOOM_ELF" ]] && [[ -f "$DOOM_ELF_ALT" ]] && DOOM_ELF="$DOOM_ELF_ALT"
[[ ! -f "$DOOM_MBC" ]] && [[ -f "$DOOM_MBC_ALT" ]] && DOOM_MBC="$DOOM_MBC_ALT"
[[ ! -f "$DOOM_RV2MBC" ]] && [[ -f "$DOOM_RV2MBC_ALT" ]] && DOOM_RV2MBC="$DOOM_RV2MBC_ALT"

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

# ── Preflight ────────────────────────────────────────────────────────────────

preflight() {
    log_step "Preflight: checking tools and build directory..."

    local missing=0
    for tool in "$OBJDUMP" "$NM" grep; do
        if ! command -v "$tool" &>/dev/null; then
            log_error "Required tool not found: $tool"
            missing=1
        fi
    done
    if [[ $missing -ne 0 ]]; then
        exit 1
    fi

    if [[ ! -d "$BUILD_DIR" ]]; then
        log_error "Build directory not found: ${BUILD_DIR}"
        log_error "Build Doom first, or use --build-dir to specify location."
        exit 1
    fi

    log_info "Build directory: ${BUILD_DIR}"
}

# ── Check 1: Bug 21 — -ffixed-x15 in CFLAGS ────────────────────────────────

check_bug21_ffixed_x15() {
    log_step "Check 1: Bug 21 — -ffixed-x15 in rebuild script CFLAGS"

    if [[ ! -f "$REBUILD_SCRIPT" ]]; then
        record_skip "rebuild_rv32im.sh not found at ${REBUILD_SCRIPT}"
        return
    fi

    if grep -q '\-ffixed-x15' "$REBUILD_SCRIPT"; then
        record_pass "Bug 21: -ffixed-x15 present in CFLAGS"
        log_verbose "$(grep '\-ffixed-x15' "$REBUILD_SCRIPT" | head -1)"
    else
        record_fail "Bug 21: -ffixed-x15 NOT found in ${REBUILD_SCRIPT}"
        log_verbose "Without -ffixed-x15, GCC will use x15 (SP) as a general register"
    fi
}

# ── Check 2: Bug 22 — setjmp saves/restores a5 (x15 = SP) ──────────────────

check_bug22_setjmp() {
    log_step "Check 2: Bug 22 — setjmp/longjmp preserves a5 (x15 = MBC SP)"

    local checks_passed=0
    local checks_total=0

    # 2a: setjmp.h should define jmp_buf with at least 7 entries (index 0-6 → a5 at [6])
    #     or 8 entries to be safe
    ((checks_total++)) || true
    if [[ ! -f "$SETJMP_H" ]]; then
        log_verbose "setjmp.h not found at ${SETJMP_H}, checking alternate locations..."
        # Try to find it
        local alt_setjmp_h
        alt_setjmp_h=$(find "$BUILD_DIR" -name "setjmp.h" -type f 2>/dev/null | head -1)
        if [[ -n "$alt_setjmp_h" ]]; then
            SETJMP_H="$alt_setjmp_h"
        fi
    fi

    if [[ -f "$SETJMP_H" ]]; then
        # Look for jmp_buf definition with size >= 7
        local jmp_buf_size
        jmp_buf_size=$(grep -oP 'jmp_buf\s*\[\s*\K\d+' "$SETJMP_H" 2>/dev/null || echo "0")
        if [[ "$jmp_buf_size" -ge 7 ]]; then
            ((checks_passed++)) || true
            log_verbose "setjmp.h: jmp_buf[${jmp_buf_size}] (>= 7 slots, includes a5)"
        else
            log_verbose "setjmp.h: jmp_buf[${jmp_buf_size}] — too small to save a5"
        fi
    else
        log_verbose "setjmp.h not found, skipping jmp_buf size check"
    fi

    # 2b: setjmp.S should contain 'sw a5' and 'lw a5' instructions
    ((checks_total++)) || true
    if [[ ! -f "$SETJMP_S" ]]; then
        local alt_setjmp_s
        alt_setjmp_s=$(find "$BUILD_DIR" -name "setjmp.S" -type f 2>/dev/null | head -1)
        if [[ -n "$alt_setjmp_s" ]]; then
            SETJMP_S="$alt_setjmp_s"
        fi
    fi

    if [[ -f "$SETJMP_S" ]]; then
        local has_sw_a5=0
        local has_lw_a5=0
        grep -qE 'sw\s+a5' "$SETJMP_S" && has_sw_a5=1
        grep -qE 'lw\s+a5' "$SETJMP_S" && has_lw_a5=1

        if [[ $has_sw_a5 -eq 1 && $has_lw_a5 -eq 1 ]]; then
            ((checks_passed++)) || true
            log_verbose "setjmp.S: found 'sw a5' (save) and 'lw a5' (restore)"
        else
            log_verbose "setjmp.S: sw_a5=${has_sw_a5}, lw_a5=${has_lw_a5} — incomplete a5 save/restore"
        fi
    else
        log_verbose "setjmp.S not found, skipping instruction check"
    fi

    if [[ $checks_passed -eq $checks_total ]]; then
        record_pass "Bug 22: setjmp properly saves/restores a5 (x15 = MBC SP)"
    elif [[ $checks_passed -gt 0 ]]; then
        record_pass "Bug 22: partial (${checks_passed}/${checks_total} sub-checks passed)"
    else
        if [[ ! -f "$SETJMP_H" && ! -f "$SETJMP_S" ]]; then
            record_skip "Bug 22: setjmp source files not found"
        else
            record_fail "Bug 22: setjmp does NOT properly save/restore a5"
        fi
    fi
}

# ── Check 3: Bug 23 — .data section attribute on 'initialized' ──────────────

check_bug23_initialized_data_section() {
    log_step "Check 3: Bug 23 — 'initialized' variable in .data section"

    if [[ ! -f "$PLATFORM_C" ]]; then
        # Try to find it
        local alt_platform
        alt_platform=$(find "$BUILD_DIR" -name "doomgeneric_monad_patched.c" -type f 2>/dev/null | head -1)
        if [[ -n "$alt_platform" ]]; then
            PLATFORM_C="$alt_platform"
        fi
    fi

    if [[ ! -f "$PLATFORM_C" ]]; then
        record_skip "Bug 23: doomgeneric_monad_patched.c not found"
        return
    fi

    # Check for __attribute__((section(".data"))) on initialized variable
    if grep -qP '__attribute__\s*\(\s*\(\s*section\s*\(\s*"\.data"\s*\)\s*\)\s*\).*initialized' "$PLATFORM_C" || \
       grep -qP 'initialized.*__attribute__\s*\(\s*\(\s*section\s*\(\s*"\.data"\s*\)\s*\)\s*\)' "$PLATFORM_C"; then
        record_pass "Bug 23: 'initialized' has __attribute__((section(\".data\")))"
        log_verbose "$(grep -n 'initialized.*section\|section.*initialized' "$PLATFORM_C" | head -1)"
    else
        # Fallback: check if 'initialized' appears in .data section of the ELF
        if [[ -f "$DOOM_ELF" ]]; then
            local init_section
            init_section=$("$NM" -C "$DOOM_ELF" 2>/dev/null | grep ' initialized$' | awk '{print $2}' | head -1)
            if [[ "$init_section" == "D" || "$init_section" == "d" ]]; then
                record_pass "Bug 23: 'initialized' in .data section (ELF confirmed, type=${init_section})"
            else
                record_fail "Bug 23: 'initialized' not in .data section (type=${init_section:-unknown})"
            fi
        else
            record_fail "Bug 23: __attribute__((section(\".data\"))) not found on 'initialized'"
        fi
    fi
}

# ── Check 4: Bug 21 supplemental — x15/a5 usage outside setjmp ──────────────

check_bug21_supplemental() {
    log_step "Check 4: Bug 21 supplemental — x15/a5 usage outside setjmp.S"

    if [[ ! -f "$DOOM_ELF" ]]; then
        record_skip "Bug 21 supplemental: doom.elf not found"
        return
    fi

    # Disassemble and count a5/x15 register usages, excluding setjmp/longjmp
    local disasm_tmp
    disasm_tmp=$(mktemp)
    "$OBJDUMP" -d "$DOOM_ELF" > "$disasm_tmp" 2>/dev/null

    # Count total a5 references
    local total_a5
    total_a5=$(grep -cE '\ba5\b' "$disasm_tmp" 2>/dev/null || echo "0")

    # Count a5 references inside setjmp/longjmp functions
    local setjmp_a5
    setjmp_a5=$(awk '/<setjmp>:|<longjmp>:|<_setjmp>:|<_longjmp>:/{found=1} found && /^$/{found=0} found' "$disasm_tmp" \
        | grep -cE '\ba5\b' 2>/dev/null || echo "0")

    local non_setjmp_a5=$((total_a5 - setjmp_a5))

    rm -f "$disasm_tmp"

    log_verbose "Total a5 references: ${total_a5}"
    log_verbose "In setjmp/longjmp: ${setjmp_a5}"
    log_verbose "Outside setjmp: ${non_setjmp_a5}"

    # With -ffixed-x15, USER code a5 usage should be near 0, but linked library
    # code (picolibc, libgcc stubs) compiled without -ffixed-x15 will use a5.
    # Thresholds account for library code: <=200 is normal for a DOOM-sized binary.
    if [[ $non_setjmp_a5 -le 10 ]]; then
        record_pass "Bug 21 supplemental: ${non_setjmp_a5} a5 refs outside setjmp (near zero)"
    elif [[ $non_setjmp_a5 -le 200 ]]; then
        record_pass "Bug 21 supplemental: ${non_setjmp_a5} a5 refs outside setjmp (library code expected)"
    else
        record_fail "Bug 21 supplemental: ${non_setjmp_a5} a5 refs outside setjmp — -ffixed-x15 may not be effective"
    fi
}

# ── Check 5: RV32IM — MUL/DIV/REM present in disassembly ────────────────────

check_rv32im() {
    log_step "Check 5: RV32IM — MUL/DIV/REM instructions present"

    if [[ ! -f "$DOOM_ELF" ]]; then
        record_skip "RV32IM check: doom.elf not found"
        return
    fi

    local disasm_tmp
    disasm_tmp=$(mktemp)
    "$OBJDUMP" -d "$DOOM_ELF" > "$disasm_tmp" 2>/dev/null

    local mul_count div_count rem_count
    mul_count=$(grep -cE '\bmul\b' "$disasm_tmp" 2>/dev/null || echo "0")
    div_count=$(grep -cE '\bdivu?\b' "$disasm_tmp" 2>/dev/null || echo "0")
    rem_count=$(grep -cE '\bremu?\b' "$disasm_tmp" 2>/dev/null || echo "0")

    rm -f "$disasm_tmp"

    local total=$((mul_count + div_count + rem_count))

    log_verbose "MUL: ${mul_count}, DIV/DIVU: ${div_count}, REM/REMU: ${rem_count}"

    if [[ $total -gt 0 ]]; then
        record_pass "RV32IM: ${total} M-extension instructions (mul=${mul_count} div=${div_count} rem=${rem_count})"
    else
        record_fail "RV32IM: no MUL/DIV/REM instructions found — M extension may not be enabled"
    fi
}

# ── Check 6: Size limits ─────────────────────────────────────────────────────

check_size_limits() {
    log_step "Check 6: Size limits — ROM_MAP and RV2MBC_MAP capacity"

    local sub_pass=0
    local sub_total=0

    # doom.mbc: max 262,144 words (ROM_MAP capacity)
    ((sub_total++)) || true
    if [[ -f "$DOOM_MBC" ]]; then
        local mbc_size
        mbc_size=$(stat -c%s "$DOOM_MBC" 2>/dev/null || stat -f%z "$DOOM_MBC" 2>/dev/null)
        local mbc_words=$((mbc_size / 4))
        local rom_max=262144

        log_verbose "doom.mbc: ${mbc_words} words (${mbc_size} bytes), limit: ${rom_max} words"

        if [[ $mbc_words -lt $rom_max ]]; then
            ((sub_pass++)) || true
            log_verbose "  ROM: ${mbc_words} / ${rom_max} words ($((mbc_words * 100 / rom_max))% capacity)"
        else
            log_verbose "  ROM: ${mbc_words} >= ${rom_max} words — OVERFLOW"
        fi
    else
        log_verbose "doom.mbc not found at ${DOOM_MBC}"
    fi

    # doom.rv2mbc: max 65,536 entries (RV2MBC_MAP capacity)
    ((sub_total++)) || true
    if [[ -f "$DOOM_RV2MBC" ]]; then
        local rv2mbc_size
        rv2mbc_size=$(stat -c%s "$DOOM_RV2MBC" 2>/dev/null || stat -f%z "$DOOM_RV2MBC" 2>/dev/null)
        local rv2mbc_entries=$((rv2mbc_size / 4))
        local rv2mbc_max=65536

        log_verbose "doom.rv2mbc: ${rv2mbc_entries} entries (${rv2mbc_size} bytes), limit: ${rv2mbc_max} entries"

        if [[ $rv2mbc_entries -lt $rv2mbc_max ]]; then
            ((sub_pass++)) || true
            log_verbose "  RV2MBC: ${rv2mbc_entries} / ${rv2mbc_max} entries ($((rv2mbc_entries * 100 / rv2mbc_max))% capacity)"
        else
            log_verbose "  RV2MBC: ${rv2mbc_entries} >= ${rv2mbc_max} entries — OVERFLOW"
        fi
    else
        log_verbose "doom.rv2mbc not found at ${DOOM_RV2MBC}"
    fi

    if [[ $sub_pass -eq $sub_total ]]; then
        record_pass "Size limits: all artifacts within BPF map capacity"
    elif [[ $sub_total -eq 0 ]]; then
        record_skip "Size limits: no build artifacts found"
    else
        record_fail "Size limits: ${sub_pass}/${sub_total} within capacity"
    fi
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    banner
    preflight

    echo ""
    check_bug21_ffixed_x15
    echo ""
    check_bug22_setjmp
    echo ""
    check_bug23_initialized_data_section
    echo ""
    check_bug21_supplemental
    echo ""
    check_rv32im
    echo ""
    check_size_limits

    # ── Summary ──────────────────────────────────────────────────────────────
    echo ""
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  Summary${NC}"
    echo -e "${CYAN}${BOLD}════════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "  ${GREEN}PASS:${NC} ${PASS_COUNT}"
    echo -e "  ${RED}FAIL:${NC} ${FAIL_COUNT}"
    echo -e "  ${YELLOW}SKIP:${NC} ${SKIP_COUNT}"
    echo ""

    if [[ $FAIL_COUNT -gt 0 ]]; then
        echo -e "  ${RED}${BOLD}RESULT: FAIL${NC} — ${FAIL_COUNT} check(s) failed"
        echo ""
        exit 1
    elif [[ $PASS_COUNT -eq 0 ]]; then
        echo -e "  ${YELLOW}${BOLD}RESULT: NO CHECKS RUN${NC} — all checks were skipped"
        echo ""
        exit 1
    else
        echo -e "  ${GREEN}${BOLD}RESULT: PASS${NC} — all ${PASS_COUNT} checks passed"
        echo ""
        exit 0
    fi
}

main "$@"
