#!/bin/bash
set -euo pipefail
echo "=== Observability Stack Verification ==="
echo ""

# `kubectl get pods -l ...` exits 0 with "No resources found" when nothing
# matches, so `&& echo UP` reported UP for components that were not deployed.
# Count the pods instead, and exit non-zero if any component is down.
down=0
pods_up() {
    local name="$1"
    if [ -n "$(kubectl -n unheaded-system get pods -l "app.kubernetes.io/name=${name}" -o name 2>/dev/null)" ]; then
        kubectl -n unheaded-system get pods -l "app.kubernetes.io/name=${name}"
        echo "  UP"
    else
        echo "  DOWN (no pods labelled app.kubernetes.io/name=${name})"
        down=$((down + 1))
    fi
}

echo "1. Prometheus..."
pods_up prometheus
echo ""
echo "2. Grafana..."
pods_up grafana
echo ""
echo "3. Alertmanager..."
pods_up alertmanager
echo ""
echo "4. OTel Collector..."
pods_up opentelemetry-collector
echo ""
echo "5. ServiceMonitors..."
kubectl get servicemonitors -A 2>/dev/null | grep unheaded || echo "  None found"
echo ""
echo "6. PrometheusRules..."
kubectl get prometheusrules -A 2>/dev/null | grep unheaded || echo "  None found"
echo ""
if [ "$down" -gt 0 ]; then
    echo "=== Verification FAILED: ${down} component(s) down ==="
    exit 1
fi
echo "=== Verification complete ==="
