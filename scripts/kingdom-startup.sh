#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
#
# Kingdom Startup — start everything Unheaded-related
# Usage: kingdom-startup.sh [--force] [--docker-only] [--native-only] [--east] [--dry-run]

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
DOCKER_ONLY=false
NATIVE_ONLY=false
START_EAST=false
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --force) FORCE=true; shift ;;
        --docker-only) DOCKER_ONLY=true; shift ;;
        --native-only) NATIVE_ONLY=true; shift ;;
        --east) START_EAST=true; shift ;;
        --dry-run) DRY_RUN=true; shift ;;
        -h|--help)
            echo "Usage: kingdom-startup.sh [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --force          Skip confirmation prompt"
            echo "  --docker-only    Only start Docker containers (skip native services and LXD)"
            echo "  --native-only    Only start native Go services (skip Docker and LXD)"
            echo "  --east           Also start EAST server services"
            echo "  --dry-run        Show what would be done without doing it"
            echo "  -h, --help       Show this help"
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
    echo -e "${BLUE}This will start all Unheaded services, containers, and infrastructure.${NC}"
    read -r -p "Continue? [y/N] " confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
        log_info "Aborted."
        exit 0
    fi
fi

echo ""
log_info "=== Kingdom Startup ==="
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_FILE="$PROJECT_ROOT/docker-compose.yml"
BIN_DIR="/opt/unheaded/bin"

# Known services in dependency order (wotan first)
NATIVE_SERVICES=(
    wotan
    unheaded-daemon
    trace-collector-go
    timeguru
    architect
    captain
    micromanager
    monad
    sophia
    cuirass
    dashboard-backend
    kanban-app
    protocol-api
)

# Health check endpoints (service:port)
HEALTH_ENDPOINTS=(
    "wotan:18000"
    "unheaded-daemon:17000"
    "timeguru:19000"
    "architect:19001"
    "captain:19002"
    "micromanager:19003"
    "monad:19004"
    "sophia:19005"
    "dashboard-backend:20000"
    "kanban-app:20001"
    "protocol-api:16666"
)

# ── Phase 1: Start Docker containers ─────────────────────────────────
if [[ "$NATIVE_ONLY" != "true" ]]; then
    log_info "Phase 1: Starting Docker containers..."

    if [[ -f "$COMPOSE_FILE" ]]; then
        if command -v docker &> /dev/null; then
            log_info "  docker compose up -d..."
            run docker compose -f "$COMPOSE_FILE" up -d 2>&1 | grep -E "Started|Running|Created" | while read -r line; do
                log_info "  $line"
            done || true
            log_success "Phase 1 complete"
        else
            log_warn "  docker not installed (skip)"
        fi
    else
        log_info "  No docker-compose.yml found (skip)"
    fi
    echo ""
else
    log_info "Phase 1: Skipped (--native-only)"
    echo ""
fi

# ── Phase 2: Start native Go services ────────────────────────────────
if [[ "$DOCKER_ONLY" != "true" ]]; then
    log_info "Phase 2: Starting native Go services..."
    started=0

    for svc in "${NATIVE_SERVICES[@]}"; do
        if pgrep -f "$svc" > /dev/null 2>&1; then
            log_info "  $svc already running"
            continue
        fi

        # Find binary: /opt/unheaded/bin first, then project cmd/ dir
        svc_bin=""
        if [[ -x "$BIN_DIR/$svc" ]]; then
            svc_bin="$BIN_DIR/$svc"
        elif [[ -d "$PROJECT_ROOT/cmd/$svc" ]]; then
            svc_bin="go-run:$PROJECT_ROOT/cmd/$svc"
        fi

        if [[ -z "$svc_bin" ]]; then
            log_warn "  $svc binary not found (skip)"
            continue
        fi

        log_info "  Starting $svc..."
        if [[ "$DRY_RUN" != "true" ]]; then
            if [[ "$svc_bin" == go-run:* ]]; then
                (cd "$PROJECT_ROOT" && go run "./${svc_bin#go-run:$PROJECT_ROOT/}" &) 2>/dev/null
            else
                "$svc_bin" &
            fi
            disown 2>/dev/null || true
        else
            echo "  [dry-run] start $svc_bin"
        fi
        ((started++)) || true

        # After wotan starts, wait for health before continuing
        if [[ "$svc" == "wotan" && "$DRY_RUN" != "true" ]]; then
            log_info "  Waiting for Wotan health..."
            for _ in $(seq 1 15); do
                if curl -sf http://localhost:18000/health > /dev/null 2>&1; then
                    log_success "  Wotan healthy"
                    break
                fi
                sleep 1
            done
        fi
    done

    if [[ $started -eq 0 ]]; then
        log_info "  No services needed starting"
    else
        log_info "  Started $started service(s)"
    fi
    log_success "Phase 2 complete"
    echo ""

    # ── Phase 3: Start LXD containers ────────────────────────────────
    log_info "Phase 3: Starting LXD containers..."
    if command -v lxc &> /dev/null; then
        stopped_containers=$(lxc list -cn,s --format csv 2>/dev/null | grep "^unheaded-" | grep ",STOPPED$" | cut -d, -f1 || true)
        if [[ -n "$stopped_containers" ]]; then
            while IFS= read -r ctr; do
                log_info "  Starting LXD container: $ctr"
                run lxc start "$ctr" 2>/dev/null || log_warn "  Failed to start $ctr"
            done <<< "$stopped_containers"
        else
            log_info "  No stopped unheaded LXD containers found"
        fi
    else
        log_info "  lxc not installed (skip)"
    fi
    log_success "Phase 3 complete"
    echo ""
else
    log_info "Phase 2: Skipped (--docker-only)"
    log_info "Phase 3: Skipped (--docker-only)"
    echo ""
fi

# ── Phase 4: Health verification ──────────────────────────────────────
log_info "Phase 4: Health verification..."
if [[ "$DRY_RUN" != "true" ]]; then
    # Give services a moment to finish starting
    sleep 2
fi

healthy=0
unhealthy=0
for entry in "${HEALTH_ENDPOINTS[@]}"; do
    svc="${entry%%:*}"
    port="${entry##*:}"
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] curl -sf http://localhost:$port/health"
        continue
    fi
    if curl -sf "http://localhost:$port/health" > /dev/null 2>&1; then
        log_success "  [OK] $svc (:$port)"
        ((healthy++)) || true
    else
        log_warn "  [--] $svc (:$port) — not responding"
        ((unhealthy++)) || true
    fi
done
if [[ "$DRY_RUN" != "true" ]]; then
    log_info "  $healthy healthy, $unhealthy not responding"
fi
log_success "Phase 4 complete"
echo ""

# ── Phase 5: Port summary ────────────────────────────────────────────
log_info "Phase 5: Doom Range (16666-26666) active listeners..."
listeners=$(ss -tlnp 2>/dev/null | awk '{print $4}' | grep -oP ':\K[0-9]+$' | sort -un | while read -r port; do
    if [[ "$port" -ge 16666 && "$port" -le 26666 ]]; then
        echo "$port"
    fi
done || true)

if [[ -n "$listeners" ]]; then
    count=$(echo "$listeners" | wc -l)
    log_success "  $count active listener(s) in Doom Range:"
    echo "$listeners" | while read -r port; do
        proc=$(ss -tlnp "sport = :$port" 2>/dev/null | grep -oP 'users:\(\("\K[^"]+' || echo "unknown")
        echo -e "    :$port  ${BLUE}$proc${NC}"
    done
else
    log_warn "  No listeners in Doom Range"
fi
log_success "Phase 5 complete"
echo ""

# ── EAST server startup ──────────────────────────────────────────────
if [[ "$START_EAST" == "true" ]]; then
    log_info "Starting EAST server services..."
    if [[ "$DRY_RUN" == "true" ]]; then
        echo "  [dry-run] ssh govan@east 'sudo systemctl start kingdom-startup wotan unheaded-daemon'"
    else
        ssh govan@east "sudo systemctl start kingdom-startup wotan unheaded-daemon" 2>/dev/null \
            && log_success "  EAST services started" \
            || log_warn "  EAST not reachable or services failed to start"
    fi
    echo ""
fi

log_success "=== Kingdom Startup Complete ==="
