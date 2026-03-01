#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
#
# setup-dark-mirror.sh — Install Prometheus + Grafana + Loki + Promtail inside the Tomb VM
#
# The Dark Mirror observes the Kingdom from the outside — an untrusted vantage point.
# All Docker images must be pre-pulled and loaded from tar files (air-gapped).
#
# Prerequisites:
#   - Docker installed in the Tomb VM
#   - Pre-saved Docker image tars in /tmp/dark-mirror-images/
#   - Network connectivity to Kingdom at 192.168.13.1
#   - Run as root or with sudo
#
# Usage:
#   sudo ./setup-dark-mirror.sh
#   sudo ./setup-dark-mirror.sh --load-images   # Only load Docker images
#   sudo ./setup-dark-mirror.sh --skip-images    # Skip image loading
#   sudo ./setup-dark-mirror.sh --teardown       # Stop and remove containers

set -euo pipefail

# --- Configuration ---
TOMB_ROOT="/opt/tomb"
DARK_MIRROR_ROOT="${TOMB_ROOT}/dark-mirror"
IMAGES_DIR="/tmp/dark-mirror-images"
SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"

SKIP_IMAGES=false
LOAD_ONLY=false
TEARDOWN=false

# --- Argument Parsing ---
for arg in "$@"; do
    case "$arg" in
        --skip-images)  SKIP_IMAGES=true ;;
        --load-images)  LOAD_ONLY=true ;;
        --teardown)     TEARDOWN=true ;;
        --help|-h)
            echo "Usage: sudo $0 [--skip-images|--load-images|--teardown]"
            exit 0
            ;;
        *) echo "[ERROR] Unknown argument: $arg"; exit 1 ;;
    esac
done

# --- Helpers ---
log_info()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [INFO]  $*"; }
log_warn()  { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [WARN]  $*"; }
log_error() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR] $*"; }
log_ok()    { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [OK]    $*"; }

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root (or with sudo)."
        exit 1
    fi
}

check_docker() {
    if ! command -v docker &>/dev/null; then
        log_error "Docker is not installed. Install Docker first."
        exit 1
    fi
    if ! docker info &>/dev/null; then
        log_error "Docker daemon is not running. Start Docker first."
        exit 1
    fi
    log_ok "Docker is available and running"
}

# --- Teardown ---
teardown() {
    log_info "Tearing down Dark Mirror stack..."
    cd "${SCRIPT_DIR}" 2>/dev/null || cd "${DARK_MIRROR_ROOT}" 2>/dev/null || true

    if [[ -f docker-compose.yml ]]; then
        docker compose down -v 2>/dev/null || docker-compose down -v 2>/dev/null || true
    fi

    log_ok "Dark Mirror stack stopped and removed"
    exit 0
}

# --- Phase 1: Directory Structure ---
setup_directories() {
    log_info "Creating Dark Mirror directory structure..."

    mkdir -p "${DARK_MIRROR_ROOT}"/{prometheus/data,grafana/dashboards,grafana/provisioning/datasources,grafana/provisioning/dashboards,loki/data,promtail,alerts}
    chmod -R 755 "${DARK_MIRROR_ROOT}"

    # Grafana needs write access
    chown -R 472:472 "${DARK_MIRROR_ROOT}/grafana" 2>/dev/null || true
    # Prometheus needs write access to data
    chown -R 65534:65534 "${DARK_MIRROR_ROOT}/prometheus/data" 2>/dev/null || true
    # Loki needs write access
    chown -R 10001:10001 "${DARK_MIRROR_ROOT}/loki/data" 2>/dev/null || true

    log_ok "Directory structure created at ${DARK_MIRROR_ROOT}"
}

# --- Phase 2: Load Docker Images ---
load_images() {
    if [[ "$SKIP_IMAGES" == "true" ]]; then
        log_warn "Skipping image loading (--skip-images)"
        return 0
    fi

    log_info "Loading pre-saved Docker images (air-gapped install)..."

    if [[ ! -d "$IMAGES_DIR" ]]; then
        log_error "Image directory not found: $IMAGES_DIR"
        log_error "Run load-images.sh on an internet-connected machine first."
        exit 1
    fi

    local loaded=0
    for tarfile in "${IMAGES_DIR}"/*.tar; do
        if [[ -f "$tarfile" ]]; then
            log_info "Loading: $(basename "$tarfile")..."
            docker load -i "$tarfile"
            ((loaded++))
        fi
    done

    if [[ $loaded -eq 0 ]]; then
        log_error "No .tar files found in $IMAGES_DIR"
        exit 1
    fi

    log_ok "Loaded $loaded Docker image(s)"
}

# --- Phase 3: Deploy Configuration Files ---
deploy_configs() {
    log_info "Deploying Dark Mirror configuration files..."

    # Copy configs from the script's directory to the deployment root
    local configs=(
        "prometheus.yml:${DARK_MIRROR_ROOT}/prometheus/prometheus.yml"
        "loki-config.yml:${DARK_MIRROR_ROOT}/loki/loki-config.yml"
        "promtail-config.yml:${DARK_MIRROR_ROOT}/promtail/promtail-config.yml"
        "alerts/kingdom-alerts.yml:${DARK_MIRROR_ROOT}/alerts/kingdom-alerts.yml"
        "grafana/datasources.yml:${DARK_MIRROR_ROOT}/grafana/provisioning/datasources/datasources.yml"
    )

    for mapping in "${configs[@]}"; do
        local src="${SCRIPT_DIR}/${mapping%%:*}"
        local dst="${mapping#*:}"

        if [[ -f "$src" ]]; then
            mkdir -p "$(dirname "$dst")"
            cp "$src" "$dst"
            log_ok "Deployed: $(basename "$src") -> $dst"
        else
            log_warn "Config not found: $src"
        fi
    done

    # Copy Grafana dashboards
    if [[ -d "${SCRIPT_DIR}/grafana/dashboards" ]]; then
        cp "${SCRIPT_DIR}"/grafana/dashboards/*.json "${DARK_MIRROR_ROOT}/grafana/dashboards/" 2>/dev/null || true
        log_ok "Deployed Grafana dashboards"
    fi

    # Create Grafana dashboard provisioning config
    cat > "${DARK_MIRROR_ROOT}/grafana/provisioning/dashboards/dashboards.yml" <<'EOF'
apiVersion: 1
providers:
  - name: 'Kingdom Dashboards'
    orgId: 1
    folder: 'Tomb of Knowledge'
    type: file
    disableDeletion: false
    editable: true
    updateIntervalSeconds: 30
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
      foldersFromFilesStructure: false
EOF

    log_ok "Grafana provisioning configured"

    # Copy alert rules to prometheus directory
    if [[ -f "${DARK_MIRROR_ROOT}/alerts/kingdom-alerts.yml" ]]; then
        cp "${DARK_MIRROR_ROOT}/alerts/kingdom-alerts.yml" "${DARK_MIRROR_ROOT}/prometheus/kingdom-alerts.yml"
        log_ok "Alert rules deployed to Prometheus directory"
    fi
}

# --- Phase 4: Deploy Docker Compose ---
deploy_compose() {
    log_info "Deploying Docker Compose configuration..."

    if [[ -f "${SCRIPT_DIR}/docker-compose.yml" ]]; then
        cp "${SCRIPT_DIR}/docker-compose.yml" "${DARK_MIRROR_ROOT}/docker-compose.yml"
    else
        log_error "docker-compose.yml not found in ${SCRIPT_DIR}"
        exit 1
    fi

    log_ok "Docker Compose deployed"
}

# --- Phase 5: Start Stack ---
start_stack() {
    log_info "Starting Dark Mirror stack..."

    cd "${DARK_MIRROR_ROOT}"

    docker compose up -d 2>/dev/null || docker-compose up -d

    # Wait for services to be healthy
    log_info "Waiting for services to start (30s)..."
    sleep 10

    local retries=6
    local ready=false
    while [[ $retries -gt 0 ]]; do
        # Check Prometheus
        if curl -sf http://127.0.0.1:9090/-/ready &>/dev/null; then
            log_ok "Prometheus is ready"
            ready=true
            break
        fi
        log_info "Waiting for Prometheus... ($retries attempts remaining)"
        sleep 5
        ((retries--))
    done

    if [[ "$ready" != "true" ]]; then
        log_warn "Prometheus may not be fully ready yet — check 'docker compose logs prometheus'"
    fi

    # Check Grafana
    if curl -sf http://127.0.0.1:3000/api/health &>/dev/null; then
        log_ok "Grafana is ready"
    else
        log_warn "Grafana not responding yet — it may need more time"
    fi

    # Check Loki
    if curl -sf http://127.0.0.1:3100/ready &>/dev/null; then
        log_ok "Loki is ready"
    else
        log_warn "Loki not responding yet"
    fi
}

# --- Phase 6: Verification ---
verify() {
    log_info "Running post-install verification..."

    echo ""
    echo "  Dark Mirror Stack Status:"
    echo "  ========================="
    docker compose ps 2>/dev/null || docker-compose ps 2>/dev/null || true
    echo ""

    echo "  Service Endpoints:"
    echo "  =================="
    echo "  Prometheus: http://127.0.0.1:9090"
    echo "  Grafana:    http://127.0.0.1:3000 (admin/tomb-of-knowledge)"
    echo "  Loki:       http://127.0.0.1:3100"
    echo ""

    # Test Kingdom connectivity
    if ping -c 1 -W 2 192.168.13.1 &>/dev/null; then
        log_ok "Kingdom (192.168.13.1) is reachable"
    else
        log_warn "Kingdom (192.168.13.1) is NOT reachable — metrics scraping will fail"
    fi

    echo ""
    log_ok "=== Dark Mirror setup complete ==="
}

# --- Main ---
main() {
    echo "============================================"
    echo "  Tomb of Knowledge — Dark Mirror Setup"
    echo "  Prometheus + Grafana + Loki + Promtail"
    echo "============================================"
    echo ""

    check_root

    if [[ "$TEARDOWN" == "true" ]]; then
        teardown
    fi

    check_docker

    if [[ "$LOAD_ONLY" == "true" ]]; then
        load_images
        log_ok "Image loading complete. Run without --load-images to deploy."
        exit 0
    fi

    setup_directories
    load_images
    deploy_configs
    deploy_compose
    start_stack
    verify

    echo ""
    log_info "Next steps:"
    log_info "  1. Open Grafana: http://127.0.0.1:3000 (admin / tomb-of-knowledge)"
    log_info "  2. Check Prometheus targets: http://127.0.0.1:9090/targets"
    log_info "  3. Generate attack report: ${SCRIPT_DIR}/attack-report.sh"
    log_info "  4. Monitor: docker compose -f ${DARK_MIRROR_ROOT}/docker-compose.yml logs -f"
}

main "$@"
