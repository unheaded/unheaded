#!/usr/bin/env bash
# deploy-alpha.sh — Deploy Unheaded Alpha to LXD containers
#
# Usage: sudo ./scripts/deploy-alpha.sh [--teardown] [--skip-build]
#
# Deploys all 8 services into individual LXD system containers
# on a dedicated bridge network (10.10.10.0/24).
#
# Prerequisites:
#   - LXD initialized (lxd init)
#   - Go 1.24+ installed
#   - Root privileges (sudo)

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

NETWORK_NAME="unheaded-br0"
SUBNET="10.10.10.0/24"
GATEWAY="10.10.10.1"
IMAGE="ubuntu:24.04"
PROFILE="unheaded"

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

# Service definitions: name:binary_path:port:static_ip
declare -A SERVICES=(
    [wotan]="services/wotan/cmd/wotan:8081:10.10.10.10"
    [timeguru]="services/timeguru/cmd/timeguru:8082:10.10.10.20"
    [captain]="services/captain/cmd/captain:8083:10.10.10.21"
    [architect]="services/architect/cmd:8084:10.10.10.22"
    [micromanager]="services/micromanager/cmd:8085:10.10.10.23"
    [monad]="cmd/monad:8086:10.10.10.24"
    [sophia]="cmd/sophia:8087:10.10.10.25"
    [cuirass]="cmd/unheaded-daemon:8080:10.10.10.100"
)

# Deploy order (wotan first as hub)
DEPLOY_ORDER=(wotan timeguru captain architect micromanager monad sophia cuirass)

# Flags
TEARDOWN=false
SKIP_BUILD=false

for arg in "$@"; do
    case "$arg" in
        --teardown)   TEARDOWN=true ;;
        --skip-build) SKIP_BUILD=true ;;
        --help|-h)
            echo "Usage: sudo $0 [--teardown] [--skip-build]"
            echo ""
            echo "Options:"
            echo "  --teardown    Remove all containers and network, then exit"
            echo "  --skip-build  Skip Go build step (use existing binaries)"
            exit 0
            ;;
    esac
done

# ============================================================================
# PREFLIGHT
# ============================================================================

echo ""
echo "========================================"
echo "  Unheaded Alpha LXD Deployment"
echo "========================================"
echo ""

# Check root
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root (sudo)"
    exit 1
fi

# Check tools
for tool in lxc go; do
    if ! command -v "$tool" &>/dev/null; then
        # Try PATH expansion for go
        if [[ "$tool" == "go" ]] && [[ -x "/usr/local/go/bin/go" ]]; then
            export PATH="/usr/local/go/bin:$PATH"
        else
            log_error "Required tool '$tool' not found"
            exit 1
        fi
    fi
done

log_info "Go: $(go version)"
log_info "LXD: $(lxc --version)"

# ============================================================================
# TEARDOWN (if requested)
# ============================================================================

teardown() {
    log_step "Tearing down Unheaded LXD deployment..."

    for svc in "${DEPLOY_ORDER[@]}"; do
        cname="unheaded-${svc}"
        if lxc info "$cname" &>/dev/null; then
            log_info "Stopping and deleting $cname"
            lxc stop "$cname" --force 2>/dev/null || true
            lxc delete "$cname" --force 2>/dev/null || true
        fi
    done

    if lxc profile show "$PROFILE" &>/dev/null; then
        log_info "Deleting profile: $PROFILE"
        lxc profile delete "$PROFILE" 2>/dev/null || true
    fi

    if lxc network show "$NETWORK_NAME" &>/dev/null; then
        log_info "Deleting network: $NETWORK_NAME"
        lxc network delete "$NETWORK_NAME" 2>/dev/null || true
    fi

    log_info "Teardown complete"
}

if $TEARDOWN; then
    teardown
    exit 0
fi

# ============================================================================
# PHASE 1: BUILD GO BINARIES
# ============================================================================

BIN_DIR="${PROJECT_ROOT}/.build/lxd-bins"
mkdir -p "$BIN_DIR"

if ! $SKIP_BUILD; then
    log_step "Building Go binaries..."

    cd "$PROJECT_ROOT"
    for svc in "${DEPLOY_ORDER[@]}"; do
        IFS=':' read -r src_path port ip <<< "${SERVICES[$svc]}"
        log_info "Building $svc from ./$src_path"
        CGO_ENABLED=0 GOOS=linux go build -o "${BIN_DIR}/${svc}" "./${src_path}"
    done

    log_info "All binaries built in ${BIN_DIR}"
    ls -lh "$BIN_DIR"/
else
    log_warn "Skipping build (--skip-build)"
fi

# ============================================================================
# PHASE 2: SETUP NETWORK
# ============================================================================

log_step "Setting up LXD network..."

if lxc network show "$NETWORK_NAME" &>/dev/null; then
    log_info "Network $NETWORK_NAME already exists"
else
    log_info "Creating network: $NETWORK_NAME (${SUBNET})"
    lxc network create "$NETWORK_NAME" \
        ipv4.address="${GATEWAY}/24" \
        ipv4.nat=true \
        ipv6.address=none \
        dns.domain=unheaded.local
fi

# ============================================================================
# PHASE 3: CREATE PROFILE
# ============================================================================

log_step "Setting up LXD profile..."

if lxc profile show "$PROFILE" &>/dev/null; then
    log_info "Profile $PROFILE already exists, updating..."
else
    log_info "Creating profile: $PROFILE"
    lxc profile create "$PROFILE"
fi

lxc profile set "$PROFILE" limits.cpu=1
lxc profile set "$PROFILE" limits.memory=256MiB

# Attach network to profile
lxc profile device remove "$PROFILE" eth0 2>/dev/null || true
lxc profile device add "$PROFILE" eth0 nic \
    network="$NETWORK_NAME" \
    name=eth0

# Add root disk
lxc profile device remove "$PROFILE" root 2>/dev/null || true
lxc profile device add "$PROFILE" root disk \
    path=/ \
    pool=default \
    size=1GiB

# ============================================================================
# PHASE 4: DEPLOY CONTAINERS
# ============================================================================

log_step "Deploying containers..."

for svc in "${DEPLOY_ORDER[@]}"; do
    IFS=':' read -r src_path port ip <<< "${SERVICES[$svc]}"
    cname="unheaded-${svc}"

    echo ""
    log_step "Deploying $cname ($ip:$port)..."

    # Create container if it doesn't exist
    if lxc info "$cname" &>/dev/null; then
        log_warn "$cname already exists, recreating..."
        lxc stop "$cname" --force 2>/dev/null || true
        lxc delete "$cname" --force 2>/dev/null || true
    fi

    log_info "Launching $cname from $IMAGE"
    lxc launch "$IMAGE" "$cname" --profile default --profile "$PROFILE"

    # Set static IP
    lxc config device override "$cname" eth0 \
        ipv4.address="$ip"

    # Wait for container to be running
    for _ in $(seq 1 30); do
        state=$(lxc info "$cname" 2>/dev/null | grep "^Status:" | awk '{print $2}')
        if [[ "$state" == "RUNNING" ]]; then
            break
        fi
        sleep 1
    done

    # Wait for networking
    sleep 3

    # Create app directory and unheaded user
    lxc exec "$cname" -- mkdir -p /app /data /var/lib/unheaded
    lxc exec "$cname" -- adduser --system --group --home /app unheaded 2>/dev/null || true

    # Push binary
    log_info "Pushing $svc binary..."
    lxc file push "${BIN_DIR}/${svc}" "${cname}/app/${svc}" --mode=0755

    # Create systemd service unit
    log_info "Creating systemd service..."

    # Build environment and command based on service
    local_env=""
    local_cmd="/app/${svc}"

    case "$svc" in
        wotan)
            local_cmd="/app/wotan --http-port ${port} --grpc-port 5555"
            ;;
        timeguru)
            local_env="PORT=${port}\nWOTAN_ADDR=${GATEWAY}:8081\nDB_PATH=/data/timeguru.db\nTIMELINE_PATH=/data/timeline.md"
            ;;
        captain)
            local_env="HTTP_ADDR=0.0.0.0:${port}\nDATA_PATH=/data\nWOTAN_ADDR=${GATEWAY}:8081"
            ;;
        architect)
            local_cmd="/app/architect -addr :${port}"
            ;;
        micromanager)
            local_cmd="/app/micromanager -port ${port}"
            ;;
        monad)
            local_env="MONAD_PORT=${port}\nWOTAN_ADDR=${GATEWAY}:8081"
            ;;
        sophia)
            local_env="SOPHIA_LISTEN_ADDR=:${port}\nWOTAN_ADDR=${GATEWAY}:8081"
            ;;
        cuirass)
            local_env="HTTP_ADDR=:${port}\nWOTAN_ADDR=${GATEWAY}:8081"
            ;;
    esac

    # Write environment file
    if [[ -n "$local_env" ]]; then
        # %b, not %s: local_env deliberately embeds \n for printf to expand, but it
        # is DATA (env-file contents), so it must not be the format string — a
        # value containing % would be read as a directive.
        printf '%b\n' "$local_env" | lxc exec "$cname" -- tee "/app/${svc}.env" > /dev/null
    else
        lxc exec "$cname" -- touch "/app/${svc}.env"
    fi

    # Write systemd unit
    cat <<UNIT | lxc exec "$cname" -- tee "/etc/systemd/system/${svc}.service" > /dev/null
[Unit]
Description=Unheaded ${svc}
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=unheaded
Group=unheaded
WorkingDirectory=/app
ExecStart=${local_cmd}
EnvironmentFile=/app/${svc}.env
Restart=always
RestartSec=5s

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/data /var/lib/unheaded

[Install]
WantedBy=multi-user.target
UNIT

    # Fix permissions
    lxc exec "$cname" -- chown -R unheaded:unheaded /app /data /var/lib/unheaded

    # Enable and start service
    lxc exec "$cname" -- systemctl daemon-reload
    lxc exec "$cname" -- systemctl enable "${svc}.service"
    lxc exec "$cname" -- systemctl start "${svc}.service"

    log_info "$cname deployed and started"
done

# ============================================================================
# PHASE 5: HEALTH CHECKS
# ============================================================================

echo ""
log_step "Running health checks (waiting 10s for services to start)..."
sleep 10

HEALTHY=0
TOTAL=0

for svc in "${DEPLOY_ORDER[@]}"; do
    IFS=':' read -r src_path port ip <<< "${SERVICES[$svc]}"
    ((TOTAL++))

    status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 5 "http://${ip}:${port}/health" 2>/dev/null || echo "FAIL")
    if [[ "$status" == "200" ]]; then
        log_info "$svc ($ip:$port) — ${GREEN}HEALTHY${NC}"
        ((HEALTHY++))
    else
        log_error "$svc ($ip:$port) — UNHEALTHY (HTTP $status)"
        # Show last few log lines for debugging
        lxc exec "unheaded-${svc}" -- journalctl -u "${svc}.service" -n 5 --no-pager 2>/dev/null || true
    fi
done

# ============================================================================
# SUMMARY
# ============================================================================

echo ""
echo "========================================"
log_step "Deployment Summary"
echo "========================================"
echo ""
echo "  Network:    $NETWORK_NAME ($SUBNET)"
echo "  Profile:    $PROFILE"
echo "  Containers: ${#DEPLOY_ORDER[@]}"
echo "  Healthy:    ${HEALTHY}/${TOTAL}"
echo ""

lxc list | grep unheaded || true

echo ""
if [[ $HEALTHY -eq $TOTAL ]]; then
    log_info "All services healthy! Alpha deployment complete."
else
    log_warn "${HEALTHY}/${TOTAL} services healthy. Check logs for failed services."
    echo ""
    echo "  Debug: lxc exec unheaded-<service> -- journalctl -u <service>.service -f"
    echo "  Teardown: sudo $0 --teardown"
fi

echo ""
echo "  Teardown: sudo $0 --teardown"
echo ""
