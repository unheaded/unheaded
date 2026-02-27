#!/usr/bin/env bash
# wait-for-healthy.sh -- Poll /health on all Unheaded services
#
# Retries up to MAX_RETRIES times with RETRY_INTERVAL between attempts.
# Exits 0 if all services become healthy, exits 1 if any remain unhealthy.
#
# Usage: ./scripts/wait-for-healthy.sh [--timeout SECONDS]

set -euo pipefail

# ============================================================================
# CONFIGURATION
# ============================================================================

MAX_RETRIES="${UNHEADED_HEALTH_RETRIES:-30}"
RETRY_INTERVAL="${UNHEADED_HEALTH_INTERVAL:-2}"
CURL_TIMEOUT=3

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Parse optional --timeout flag (overrides MAX_RETRIES * RETRY_INTERVAL)
for arg in "$@"; do
    case "$arg" in
        --timeout)
            shift
            TIMEOUT_SEC="${1:-60}"
            MAX_RETRIES=$(( TIMEOUT_SEC / RETRY_INTERVAL ))
            shift
            ;;
    esac
done

# ============================================================================
# SERVICE DEFINITIONS
# ============================================================================
# Format: "name|host|port|path"
# host is localhost when accessed from the Docker host via published ports.
# Adjust if services do not publish ports to the host.

SERVICES=(
    # Infrastructure services
    "traefik|localhost|80|ping"       # requires --ping=true in compose
    "victoria|localhost|8428|-/healthy"
    "clickhouse|localhost|8123|ping"
    "grafana|localhost|3001|api/health"
    # Application services (Kingdom Core) -- Doom Range ports
    "wotan|localhost|18000|health"
    "timeguru|localhost|19000|health"
    "captain|localhost|19002|health"
    "architect|localhost|19001|health"
    "micromanager|localhost|19003|health"
    "monad|localhost|19004|health"
    "sophia|localhost|19005|health"
    "cuirass|localhost|19006|health"
)

# ============================================================================
# HEALTH CHECK LOGIC
# ============================================================================

check_service() {
    local name="$1"
    local host="$2"
    local port="$3"
    local path="$4"

    local url="http://${host}:${port}/${path}"
    local status
    status=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout "$CURL_TIMEOUT" "$url" 2>/dev/null || echo "000")

    if [[ "$status" == "200" || "$status" == "204" ]]; then
        return 0
    else
        return 1
    fi
}

# ============================================================================
# MAIN LOOP
# ============================================================================

echo ""
echo -e "${BOLD}Unheaded Health Check${NC}"
echo -e "Polling ${#SERVICES[@]} services (max ${MAX_RETRIES} retries, ${RETRY_INTERVAL}s interval)"
echo ""

attempt=0
while (( attempt < MAX_RETRIES )); do
    ((attempt++))
    all_healthy=true
    healthy_count=0
    total_count=${#SERVICES[@]}

    for entry in "${SERVICES[@]}"; do
        IFS='|' read -r name host port path <<< "$entry"
        if check_service "$name" "$host" "$port" "$path"; then
            ((healthy_count++))
        else
            all_healthy=false
        fi
    done

    if $all_healthy; then
        break
    fi

    # Print progress on every 5th attempt or the first attempt
    if (( attempt == 1 || attempt % 5 == 0 )); then
        echo -e "  [${attempt}/${MAX_RETRIES}] ${healthy_count}/${total_count} services healthy..."
    fi

    sleep "$RETRY_INTERVAL"
done

# ============================================================================
# FINAL STATUS TABLE
# ============================================================================

echo ""
printf "${BOLD}%-20s %-10s %-30s${NC}\n" "SERVICE" "STATUS" "ENDPOINT"
printf "%-20s %-10s %-30s\n" "--------" "------" "--------"

final_healthy=0
final_total=${#SERVICES[@]}

for entry in "${SERVICES[@]}"; do
    IFS='|' read -r name host port path <<< "$entry"
    if check_service "$name" "$host" "$port" "$path"; then
        printf "%-20s ${GREEN}%-10s${NC} %-30s\n" "$name" "HEALTHY" "http://${host}:${port}/${path}"
        ((final_healthy++))
    else
        printf "%-20s ${RED}%-10s${NC} %-30s\n" "$name" "UNHEALTHY" "http://${host}:${port}/${path}"
    fi
done

echo ""
echo -e "${BOLD}Result: ${final_healthy}/${final_total} services healthy${NC}"

if (( final_healthy == final_total )); then
    echo -e "${GREEN}All services operational.${NC}"
    echo ""
    exit 0
else
    echo -e "${RED}Some services failed to become healthy after $((MAX_RETRIES * RETRY_INTERVAL))s.${NC}"
    echo ""
    echo "Troubleshooting:"
    echo "  docker compose ps                   # Check container status"
    echo "  docker compose logs <service>        # Check service logs"
    echo "  docker compose restart <service>     # Restart a service"
    echo ""
    exit 1
fi
