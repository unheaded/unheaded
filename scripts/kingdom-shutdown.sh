#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
#
# Kingdom Shutdown — kill/stop everything Unheaded-related
# Usage: kingdom-shutdown.sh [--force] [--keep-containers] [--keep-bpf] [--east] [--dry-run]

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

# Flags
FORCE=false
KEEP_CONTAINERS=false
KEEP_BPF=false
RESET_EAST=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --force) FORCE=true; shift ;;
        --keep-containers) KEEP_CONTAINERS=true; shift ;;
        --keep-bpf) KEEP_BPF=true; shift ;;
        --east) RESET_EAST=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        -h|--help)
            echo "Usage: kingdom-shutdown.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --force            Skip confirmation prompt"
            echo "  --keep-containers  Skip Docker and LXD container teardown"
            echo "  --keep-bpf         Skip eBPF state cleanup"
            echo "  --east             Also shut down EAST server services"
            echo "  --dry-run          Show what would be done without doing it"
            echo "  -h, --help         Show this help"
            exit 0
            ;;
        *) log_error "Unknown option: $1"; exit 1 ;;
    esac
done

# Dry-run wrapper — runs command or prints it
run() {
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] $*"
    else
        "$@"
    fi
}

# Confirmation prompt (unless --force or --dry-run)
if [[ "$FORCE" != "true" && "$DRY_RUN" != "true" ]]; then
    echo -e "${RED}WARNING: This will stop ALL Unheaded processes, containers, and clean state.${NC}"
    read -r -p "Continue? [y/N] " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        log_info "Aborted."
        exit 0
    fi
fi

echo ""
log_info "=== Kingdom Shutdown ==="
echo ""

# Known Unheaded binaries (reverse dependency order — wotan last)
BINARIES=(
    trace-collector-go
    ebpf-collector
    ebpf-exporter
    ebpf-loader
    kanban-app
    wiki-server
    dashboard-backend
    protocol-api
    chaos-controller
    pqc-verifier
    lich-security
    routing-health
    waf
    doom-bridge
    doom-go-injector
    doom-loader
    timeguru
    architect
    captain
    micromanager
    monad
    sophia
    cuirass
    cert-gen
    wotan-ctl
    unheaded-daemon
    unheaded-cli
    wotan
)

# ── Phase 1: Graceful SIGTERM to Go services ──────────────────────────
log_info "Phase 1: Sending SIGTERM to Unheaded services..."
killed=0
for bin in "${BINARIES[@]}"; do
    if pgrep -f "$bin" > /dev/null 2>&1; then
        log_info "  Stopping $bin..."
        run pkill -f "$bin" 2>/dev/null || true
        ((killed++)) || true
    fi
done

if [[ $killed -eq 0 ]]; then
    log_info "  No running services found"
else
    log_info "  Sent SIGTERM to $killed service(s), waiting up to 10s..."
    if [[ "$DRY_RUN" != "true" ]]; then
        elapsed=0
        while [[ $elapsed -lt 10 ]]; do
            still_running=false
            for bin in "${BINARIES[@]}"; do
                if pgrep -f "$bin" > /dev/null 2>&1; then
                    still_running=true
                    break
                fi
            done
            if [[ "$still_running" != "true" ]]; then
                break
            fi
            sleep 1
            ((elapsed++)) || true
        done

        # SIGKILL survivors
        for bin in "${BINARIES[@]}"; do
            if pgrep -f "$bin" > /dev/null 2>&1; then
                log_warn "  Force-killing $bin..."
                pkill -9 -f "$bin" 2>/dev/null || true
            fi
        done
    fi
fi
log_success "Phase 1 complete"
echo ""

# ── Phase 2: Stop Docker containers ──────────────────────────────────
if [[ "$KEEP_CONTAINERS" != "true" ]]; then
    log_info "Phase 2: Stopping Docker containers..."

    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
    COMPOSE_FILE="$PROJECT_ROOT/docker-compose.yml"

    if [[ -f "$COMPOSE_FILE" ]]; then
        log_info "  docker compose down..."
        run docker compose -f "$COMPOSE_FILE" down 2>/dev/null || log_warn "  docker compose down failed (ignored)"
    fi

    # Stop any straggler unheaded containers
    if command -v docker &> /dev/null; then
        stragglers=$(docker ps --filter name=unheaded -q 2>/dev/null || true)
        if [[ -n "$stragglers" ]]; then
            log_info "  Stopping straggler containers..."
            run docker stop $stragglers 2>/dev/null || true
        fi
    fi

    log_success "Phase 2 complete"
    echo ""

    # ── Phase 3: Stop/delete LXD containers ───────────────────────────
    log_info "Phase 3: Stopping LXD containers..."
    if command -v lxc &> /dev/null; then
        lxd_containers=$(lxc list -cn --format csv 2>/dev/null | grep "^unheaded-" || true)
        if [[ -n "$lxd_containers" ]]; then
            while IFS= read -r ctr; do
                log_info "  Stopping LXD container: $ctr"
                run lxc stop --force "$ctr" 2>/dev/null || true
                run lxc delete --force "$ctr" 2>/dev/null || true
            done <<< "$lxd_containers"
        else
            log_info "  No unheaded LXD containers found"
        fi
    else
        log_info "  lxc not installed (skip)"
    fi
    log_success "Phase 3 complete"
    echo ""
else
    log_info "Phase 2: Skipped (--keep-containers)"
    log_info "Phase 3: Skipped (--keep-containers)"
    echo ""
fi

# ── Phase 4: Clean eBPF state ─────────────────────────────────────────
if [[ "$KEEP_BPF" != "true" ]]; then
    log_info "Phase 4: Cleaning eBPF state..."
    if [[ -d /sys/fs/bpf/unheaded ]]; then
        run sudo rm -rf /sys/fs/bpf/unheaded
        log_success "  Pinned BPF maps removed"
    else
        log_info "  No pinned BPF maps found (skip)"
    fi
    log_success "Phase 4 complete"
    echo ""
else
    log_info "Phase 4: Skipped (--keep-bpf)"
    echo ""
fi

# ── Phase 5: Port verification ────────────────────────────────────────
log_info "Phase 5: Scanning Doom Range (16666-26666) for remaining listeners..."
orphans=$(ss -tlnp 2>/dev/null | awk '{print $4}' | grep -oP ':\K[0-9]+$' | sort -un | while read -r port; do
    if [[ "$port" -ge 16666 && "$port" -le 26666 ]]; then
        echo "$port"
    fi
done || true)

if [[ -n "$orphans" ]]; then
    log_warn "  Orphan listeners detected on ports:"
    echo "$orphans" | while read -r port; do
        proc=$(ss -tlnp "sport = :$port" 2>/dev/null | tail -1 || true)
        log_warn "    :$port  $proc"
    done
else
    log_success "  Doom Range clear — no orphan listeners"
fi
log_success "Phase 5 complete"
echo ""

# ── EAST server shutdown ──────────────────────────────────────────────
if [[ "$RESET_EAST" == "true" ]]; then
    log_info "Shutting down EAST server services..."
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] ssh govan@east 'sudo systemctl stop wotan unheaded-daemon kingdom-shutdown'"
    else
        ssh govan@east "sudo systemctl stop wotan unheaded-daemon kingdom-shutdown" 2>/dev/null \
            && log_success "  EAST services stopped" \
            || log_warn "  EAST not reachable or services not running"
    fi
    echo ""
fi

log_success "=== Kingdom Shutdown Complete ==="
