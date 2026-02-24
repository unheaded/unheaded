#!/usr/bin/env bash
# ═══════════════════════════════════════════════════════════════════════════════
# clean-artifacts.sh — Nuke non-source, uncommitted files based on .gitignore
#
# This script removes:
#   1. All files matched by .gitignore (git clean -fdX)
#   2. Known heavy artifact directories (Rust target/, Go bin/, node_modules/)
#   3. Stale root-level Go binaries (architect, captain, sophia, etc.)
#   4. Junk files (.DS_Store, coverage.*, swap files, backups)
#   5. Python __pycache__ directories
#   6. Doom C build artifacts (build_monad/)
#   7. Fuzz corpus/artifacts (crates/monad-mbc/fuzz/)
#
# Usage:
#   ./scripts/clean-artifacts.sh          # Dry run (shows what would be removed)
#   ./scripts/clean-artifacts.sh --nuke   # Actually delete everything
#   ./scripts/clean-artifacts.sh --size   # Show sizes of artifact dirs only
#
# Safe by design:
#   - Default mode is DRY RUN — shows but does not delete
#   - Only removes gitignored files (tracked files are never touched)
#   - Prints total space reclaimed
# ═══════════════════════════════════════════════════════════════════════════════
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ─── Colors ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# ─── Known heavy artifact directories ───────────────────────────────────────
# Sorted by typical size (largest first). All are gitignored and regenerate.
ARTIFACT_DIRS=(
    # Rust/Cargo build caches (biggest offenders: ~2.5GB combined)
    "cmd/ebpf-collector/target"
    "cmd/ebpf-collector/ebpf-programs/target"
    "cmd/ebpf-loader/target"
    "crates/monad-mbc/target"
    "crates/monad-mbc/fuzz/target"
    "ebpf/target"

    # Go build outputs
    "bin"
    ".build"
    "dist"

    # Doom build artifacts (C object files)
    "doom/doomgeneric/doomgeneric/build_monad"

    # Runtime data (SQLite DBs, WAL files)
    "data"

    # Fuzz corpus & artifacts (regenerate on next fuzz run)
    "crates/monad-mbc/fuzz/corpus"
    "crates/monad-mbc/fuzz/artifacts"

    # Dependencies & caches
    "node_modules"
    "vendor"
    ".direnv"
    ".docker"
    ".coverage"
    ".claude-cache"
    "result"

    # Python caches
    "scripts/__pycache__"
    "scripts/doom/__pycache__"

    # Misc temp
    "tmp"
    "temp"
)

# ─── Known stale root-level binaries ──────────────────────────────────────
# These are Go binaries accidentally built into repo root instead of bin/.
# Safe to nuke — they're ELF executables, not source code.
STALE_ROOT_BINARIES=(
    "architect"
    "captain"
    "cert-gen"
    "doom-bridge"
    "doom-go-injector"
    "doom-loader"
    "dashboard-backend"
    "kanban-app"
    "micromanager"
    "sophia"
    "timeguru"
    "trace-collector-go"
    "unheaded-daemon"
)

# ─── Known stale binaries in subdirectories ─────────────────────────────
# Demo/test ELF binaries that get rebuilt from source.
STALE_SUBDIR_BINARIES=(
    "demos/c/test_add.elf"
    "demos/doom/unheaded-doom.elf"
    "demos/doom/unheaded-doom.mbc"
    "demos/doom/unheaded-doom.rv2mbc"
)

# ─── Junk file patterns ──────────────────────────────────────────────────
# Individual files scattered across the repo.
JUNK_PATTERNS=(
    ".DS_Store"
    "coverage.out"
    "coverage.html"
    "*.swp"
    "*.bak"
    "*~"
    "*.db-shm"
    "*.db-wal"
)

# ─── Functions ───────────────────────────────────────────────────────────────

show_sizes() {
    echo -e "${CYAN}${BOLD}═══ Artifact Directory Sizes ═══${NC}"
    echo ""
    local total=0

    # Artifact directories
    echo -e "${YELLOW}Artifact directories:${NC}"
    for dir in "${ARTIFACT_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            size=$(du -sm "$dir" 2>/dev/null | cut -f1)
            total=$((total + size))
            printf "  ${YELLOW}%6dM${NC}  %s/\n" "$size" "$dir"
        fi
    done
    echo ""

    # Stale binaries (root + subdirectory)
    local stale_total=0
    local stale_found=0
    for bin in "${STALE_ROOT_BINARIES[@]}"; do
        if [ -f "$bin" ] && file "$bin" 2>/dev/null | grep -q "ELF"; then
            size=$(du -sm "$bin" 2>/dev/null | cut -f1)
            stale_total=$((stale_total + size))
            stale_found=$((stale_found + 1))
            printf "  ${RED}%6dM${NC}  %s ${RED}(stale binary)${NC}\n" "$size" "$bin"
        fi
    done
    for bin in "${STALE_SUBDIR_BINARIES[@]}"; do
        if [ -f "$bin" ]; then
            size=$(du -sm "$bin" 2>/dev/null | cut -f1)
            stale_total=$((stale_total + size))
            stale_found=$((stale_found + 1))
            printf "  ${RED}%6dM${NC}  %s ${RED}(stale binary)${NC}\n" "$size" "$bin"
        fi
    done
    if [ "$stale_found" -gt 0 ]; then
        total=$((total + stale_total))
        echo -e "  ${YELLOW}${stale_found} stale binaries: ${stale_total}M${NC}"
        echo ""
    fi

    # Junk files
    local junk_count=0
    for pattern in "${JUNK_PATTERNS[@]}"; do
        count=$(find . -name "$pattern" -not -path "*/target/*" 2>/dev/null | wc -l)
        junk_count=$((junk_count + count))
    done
    if [ "$junk_count" -gt 0 ]; then
        echo -e "  ${YELLOW}${junk_count}${NC} junk files (.DS_Store, coverage.*, etc.)"
        echo ""
    fi

    # Doom WADs (special — warn but don't auto-count)
    local wad_total=0
    for wad in doom/doom.wad doom/doom2.wad; do
        if [ -f "$wad" ]; then
            size=$(du -sm "$wad" 2>/dev/null | cut -f1)
            wad_total=$((wad_total + size))
        fi
    done
    if [ "$wad_total" -gt 0 ]; then
        echo -e "  ${CYAN}${wad_total}M${NC}  Doom WADs (gitignored, nuked by --nuke, re-download if needed)"
        echo ""
    fi

    # Also show gitignored files not in those dirs
    ignored_count=$(git clean -fdXn 2>/dev/null | wc -l)
    echo -e "  ${YELLOW}${ignored_count}${NC} total gitignored files/dirs"
    echo ""
    echo -e "  ${BOLD}Estimated recoverable: ${GREEN}${total}M+${NC}"
    echo ""

    # Current repo size
    repo_size=$(du -sm . 2>/dev/null | cut -f1)
    echo -e "  ${BOLD}Current repo size: ${RED}${repo_size}M${NC}"
}

dry_run() {
    echo -e "${CYAN}${BOLD}═══ DRY RUN — Files that would be removed ═══${NC}"
    echo ""

    # Show artifact dir sizes
    local total_mb=0
    echo -e "${YELLOW}Artifact directories:${NC}"
    for dir in "${ARTIFACT_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            size=$(du -sm "$dir" 2>/dev/null | cut -f1)
            total_mb=$((total_mb + size))
            printf "  ${RED}[DELETE]${NC} %s/ ${YELLOW}(%dM)${NC}\n" "$dir" "$size"
        fi
    done
    echo ""

    # Stale binaries (root + subdirectory)
    local stale_found=0
    for bin in "${STALE_ROOT_BINARIES[@]}"; do
        if [ -f "$bin" ] && file "$bin" 2>/dev/null | grep -q "ELF"; then
            size=$(du -sm "$bin" 2>/dev/null | cut -f1)
            total_mb=$((total_mb + size))
            stale_found=$((stale_found + 1))
            printf "  ${RED}[DELETE]${NC} %s ${YELLOW}(%dM, stale ELF binary)${NC}\n" "$bin" "$size"
        fi
    done
    for bin in "${STALE_SUBDIR_BINARIES[@]}"; do
        if [ -f "$bin" ]; then
            stale_found=$((stale_found + 1))
            printf "  ${RED}[DELETE]${NC} %s ${YELLOW}(stale build output)${NC}\n" "$bin"
        fi
    done
    if [ "$stale_found" -gt 0 ]; then
        echo ""
    fi

    # Junk files
    local junk_count=0
    echo -e "${YELLOW}Junk files:${NC}"
    for pattern in "${JUNK_PATTERNS[@]}"; do
        while IFS= read -r junk; do
            junk_count=$((junk_count + 1))
            if [ "$junk_count" -le 20 ]; then
                echo -e "  ${RED}[DELETE]${NC} $junk"
            fi
        done < <(find . -name "$pattern" -not -path "*/target/*" 2>/dev/null)
    done
    if [ "$junk_count" -gt 20 ]; then
        echo -e "  ${YELLOW}... and $((junk_count - 20)) more junk files${NC}"
    fi
    echo ""

    # Show all gitignored files (git clean -fdXn = dry run, ignored only)
    echo -e "${YELLOW}All gitignored files (git clean -fdXn):${NC}"
    local count=0
    while IFS= read -r line; do
        # git clean -n output: "Would remove foo/bar"
        file="${line#Would remove }"
        count=$((count + 1))
        if [ $count -le 50 ]; then
            echo -e "  ${RED}[DELETE]${NC} $file"
        fi
    done < <(git clean -fdXn 2>/dev/null)

    if [ $count -gt 50 ]; then
        echo -e "  ${YELLOW}... and $((count - 50)) more files${NC}"
    fi

    echo ""
    echo -e "${BOLD}Total: ${count} gitignored entries, ${stale_found} stale binaries, ${junk_count} junk files${NC}"
    echo -e "${BOLD}Estimated recoverable: ~${total_mb}M+${NC}"
    echo ""
    echo -e "${GREEN}Run with --nuke to actually delete.${NC}"
}

nuke() {
    echo -e "${RED}${BOLD}═══ NUKING ARTIFACTS ═══${NC}"
    echo ""

    # Safety check: make sure we're in a git repo
    if ! git rev-parse --is-inside-work-tree &>/dev/null; then
        echo -e "${RED}ERROR: Not inside a git repository!${NC}"
        exit 1
    fi

    # Safety check: no uncommitted changes to tracked files
    if ! git diff --quiet HEAD 2>/dev/null; then
        echo -e "${YELLOW}WARNING: You have uncommitted changes to tracked files.${NC}"
        echo -e "${YELLOW}Tracked files will NOT be touched, but double-check git status.${NC}"
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Aborted."
            exit 1
        fi
    fi

    # Get before size
    before=$(du -sm . 2>/dev/null | cut -f1)

    # Remove gitignored files (this is the nuclear option for .gitignore matches)
    # -f = force, -d = directories, -X = ONLY gitignored files
    echo -e "${YELLOW}Running: git clean -fdX${NC}"
    git clean -fdX

    # Double-check known artifact dirs (git clean should have gotten them,
    # but some might be in nested repos or have permission issues)
    for dir in "${ARTIFACT_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            echo -e "${YELLOW}Force-removing leftover dir: ${dir}/${NC}"
            rm -rf "$dir"
        fi
    done

    # Remove stale root-level binaries (Go builds accidentally output to repo root)
    for bin in "${STALE_ROOT_BINARIES[@]}"; do
        if [ -f "$bin" ] && file "$bin" 2>/dev/null | grep -q "ELF"; then
            echo -e "${YELLOW}Removing stale binary: ${bin}${NC}"
            rm -f "$bin"
        fi
    done

    # Remove stale binaries in subdirectories (demo/test build outputs)
    for bin in "${STALE_SUBDIR_BINARIES[@]}"; do
        if [ -f "$bin" ]; then
            echo -e "${YELLOW}Removing stale binary: ${bin}${NC}"
            rm -f "$bin"
        fi
    done

    # Remove junk files (.DS_Store, coverage.*, swap files, etc.)
    for pattern in "${JUNK_PATTERNS[@]}"; do
        find . -name "$pattern" -not -path "*/target/*" -delete 2>/dev/null
    done

    # Get after size
    after=$(du -sm . 2>/dev/null | cut -f1)
    saved=$((before - after))

    echo ""
    echo -e "${GREEN}${BOLD}═══ DONE ═══${NC}"
    echo -e "  Before: ${RED}${before}M${NC}"
    echo -e "  After:  ${GREEN}${after}M${NC}"
    echo -e "  Saved:  ${BOLD}${GREEN}${saved}M${NC}"
    echo ""

    # Verify no tracked files were harmed
    lost=$(git diff --name-only --diff-filter=D 2>/dev/null | wc -l)
    if [ "$lost" -gt 0 ]; then
        echo -e "${RED}WARNING: ${lost} tracked files appear deleted!${NC}"
        echo -e "${RED}Run: git checkout -- .${NC}"
    else
        echo -e "${GREEN}All tracked files intact.${NC}"
    fi
}

# ─── Main ────────────────────────────────────────────────────────────────────

case "${1:-}" in
    --nuke)
        nuke
        ;;
    --size)
        show_sizes
        ;;
    --help|-h)
        echo "Usage: $0 [--nuke|--size|--help]"
        echo ""
        echo "  (default)  Dry run — show what would be removed"
        echo "  --nuke     Actually delete all gitignored artifacts"
        echo "  --size     Show artifact directory sizes only"
        echo "  --help     This message"
        ;;
    *)
        dry_run
        ;;
esac
