#!/usr/bin/env bash
# bring-up.sh — local kind cluster + helm install of the Unheaded stack.
#
# Idempotent — safe to re-run; will tear down a stale "unheaded" kind
# cluster before creating a new one. Designed for the dev box, not
# production.
#
# Usage:
#   ./deploy/k8s/kind/bring-up.sh           # full bring-up
#   ./deploy/k8s/kind/bring-up.sh --skip-helm  # cluster only, no chart
#
# Pre-requisites checked at runtime: kind, kubectl, helm, docker.

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-unheaded}"
CHART_PATH="${CHART_PATH:-helm/unheaded/}"
VALUES_FILE="${VALUES_FILE:-deploy/k8s/kind/values-local.yaml}"
KIND_CONFIG="${KIND_CONFIG:-deploy/k8s/kind/cluster-config.yaml}"
NAMESPACE="${NAMESPACE:-unheaded}"

SKIP_HELM=false
for arg in "$@"; do
    case "$arg" in
        --skip-helm) SKIP_HELM=true ;;
        --help|-h)
            sed -n '2,15p' "$0" | sed 's/^# \?//'
            exit 0
            ;;
    esac
done

# ───────────────────── pre-flight ─────────────────────

for tool in kind kubectl helm docker; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "✗ missing tool: $tool" >&2
        exit 1
    fi
done

if ! docker info >/dev/null 2>&1; then
    echo "✗ docker daemon not reachable; start docker first" >&2
    exit 1
fi

# ───────────────────── recreate cluster ─────────────────────

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER_NAME"; then
    echo "  cluster '$CLUSTER_NAME' exists — tearing down for clean bring-up"
    kind delete cluster --name "$CLUSTER_NAME"
fi

echo "  creating kind cluster '$CLUSTER_NAME' from $KIND_CONFIG..."
kind create cluster --config "$KIND_CONFIG"

echo "  cluster ready:"
kubectl get nodes

# ───────────────────── load local images into kind ─────────────────────
#
# kind nodes can't see the host docker daemon — images must be loaded
# explicitly. We load the unheaded-dev-* images that the values-local.yaml
# references.

IMAGES=(
    unheaded-dev-wotan:latest
    unheaded-dev-timeguru:latest
    unheaded-dev-architect:latest
    unheaded-dev-captain:latest
    unheaded-dev-micromanager:latest
    unheaded-dev-monad:latest
    unheaded-dev-sophia:latest
    unheaded-dev-dashboard-backend:latest
    unheaded-dev-kanban-app:latest
)

echo ""
echo "  loading ${#IMAGES[@]} images into kind cluster..."
MISSING=()
for img in "${IMAGES[@]}"; do
    if ! docker image inspect "$img" >/dev/null 2>&1; then
        MISSING+=("$img")
        continue
    fi
    kind load docker-image --name "$CLUSTER_NAME" "$img" 2>&1 | tail -1
done

if [ ${#MISSING[@]} -gt 0 ]; then
    echo ""
    echo "  ⚠ ${#MISSING[@]} image(s) not in local docker daemon:"
    printf "    - %s\n" "${MISSING[@]}"
    echo "  build them first: docker compose build  (or your equivalent)"
    echo "  continuing anyway — those services' pods will be stuck in ImagePullBackOff"
fi

# ───────────────────── helm install ─────────────────────

if $SKIP_HELM; then
    echo ""
    echo "  --skip-helm — chart install skipped; cluster is up + idle"
    exit 0
fi

echo ""
echo "  helm install/upgrade unheaded chart..."
helm upgrade --install unheaded "$CHART_PATH" \
    --namespace "$NAMESPACE" \
    --create-namespace \
    --values "$VALUES_FILE" \
    --wait --timeout 120s || {
        echo ""
        echo "  helm install timed out — pod state:"
        kubectl get pods -n "$NAMESPACE" -o wide
        echo ""
        echo "  events for failing pods:"
        kubectl get events -n "$NAMESPACE" --sort-by='.lastTimestamp' | tail -20
        exit 1
    }

echo ""
echo "  ✓ chart installed. Pods:"
kubectl get pods -n "$NAMESPACE" -o wide
echo ""
echo "  Services:"
kubectl get svc -n "$NAMESPACE"
echo ""
echo "  Smoke: hit wotan /health via kind node-port mapping"
echo "    curl -s http://127.0.0.1:30080/health"
