#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# lxd/LAUNCH_EXAMPLES.sh
# Example launch commands for all 25 Unheaded Kingdom microservices

# NOTE: These are EXAMPLES. Adjust profiles, resources, and options per service.
# Use: bash LAUNCH_EXAMPLES.sh [service-name] to launch individual services
# Or source this file to define functions for each service.

set -euo pipefail

BASE_IMAGE="images:ubuntu/24.04/cloud"
CLOUD_INIT_BASE="cloud-init/base.yaml"
CLOUD_INIT_EBPF="cloud-init/ebpf.yaml"
CLOUD_INIT_GPU="cloud-init/gpu.yaml"

# ============================================================================
# CATEGORY 1: API & WEB SERVICES (10 containers)
# ============================================================================

launch_api_gateway() {
    lxc launch "$BASE_IMAGE" api-gateway \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_auth_service() {
    lxc launch "$BASE_IMAGE" auth-service \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_user_service() {
    lxc launch "$BASE_IMAGE" user-service \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_catalog_service() {
    lxc launch "$BASE_IMAGE" catalog-service \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_order_service() {
    lxc launch "$BASE_IMAGE" order-service \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_payment_processor() {
    lxc launch "$BASE_IMAGE" payment-processor \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_notification_service() {
    lxc launch "$BASE_IMAGE" notification-service \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_search_indexer() {
    lxc launch "$BASE_IMAGE" search-indexer \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_recommendation_engine() {
    lxc launch "$BASE_IMAGE" recommendation-engine \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_analytics_pipeline() {
    lxc launch "$BASE_IMAGE" analytics-pipeline \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

# ============================================================================
# CATEGORY 2: eBPF & NETWORK OBSERVABILITY (5 containers)
# ============================================================================

launch_shield() {
    lxc launch "$BASE_IMAGE" shield \
        --profile unheaded-base \
        --profile unheaded-ebpf \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_EBPF")"
}

launch_unheaded_daemon() {
    lxc launch "$BASE_IMAGE" unheaded-daemon \
        --profile unheaded-base \
        --profile unheaded-ebpf \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_EBPF")"
}

launch_ebpf_exporter() {
    lxc launch "$BASE_IMAGE" ebpf-exporter \
        --profile unheaded-base \
        --profile unheaded-ebpf \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_EBPF")"
}

launch_network_tracer() {
    lxc launch "$BASE_IMAGE" network-tracer \
        --profile unheaded-base \
        --profile unheaded-ebpf \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_EBPF")"
}

launch_syscall_monitor() {
    lxc launch "$BASE_IMAGE" syscall-monitor \
        --profile unheaded-base \
        --profile unheaded-ebpf \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_EBPF")"
}

# ============================================================================
# CATEGORY 3: DATA & PERSISTENCE (6 containers)
# ============================================================================

launch_postgres() {
    lxc launch "$BASE_IMAGE" postgres \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_redis_primary() {
    lxc launch "$BASE_IMAGE" redis-primary \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_redis_replica() {
    lxc launch "$BASE_IMAGE" redis-replica \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_elasticsearch() {
    lxc launch "$BASE_IMAGE" elasticsearch \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_minio() {
    lxc launch "$BASE_IMAGE" minio \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_mongodb() {
    lxc launch "$BASE_IMAGE" mongodb \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

# ============================================================================
# CATEGORY 4: AI/ML & LLM INFERENCE (2 containers) — HOST-A ONLY
# ============================================================================

launch_vllm_deepseek() {
    # GPU passthrough only available on host-a with RX 7700 XT
    lxc launch "$BASE_IMAGE" vllm-deepseek \
        --profile unheaded-base \
        --profile unheaded-gpu \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_GPU")"
}

launch_embeddings_service() {
    # GPU passthrough only available on host-a with RX 7700 XT
    lxc launch "$BASE_IMAGE" embeddings-service \
        --profile unheaded-base \
        --profile unheaded-gpu \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE" && cat "$CLOUD_INIT_GPU")"
}

# ============================================================================
# CATEGORY 5: OBSERVABILITY & TELEMETRY (2 containers)
# ============================================================================

launch_prometheus() {
    lxc launch "$BASE_IMAGE" prometheus \
        --profile unheaded-base \
        --profile unheaded-telemetry \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_grafana() {
    lxc launch "$BASE_IMAGE" grafana \
        --profile unheaded-base \
        --profile unheaded-service \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_loki() {
    lxc launch "$BASE_IMAGE" loki \
        --profile unheaded-base \
        --profile unheaded-telemetry \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

launch_victoria_metrics() {
    lxc launch "$BASE_IMAGE" victoria-metrics \
        --profile unheaded-base \
        --profile unheaded-telemetry \
        --config user.user-data="$(cat "$CLOUD_INIT_BASE")"
}

# ============================================================================
# BATCH LAUNCH FUNCTIONS
# ============================================================================

launch_all_api_services() {
    echo "Launching API & Web Services..."
    launch_api_gateway
    launch_auth_service
    launch_user_service
    launch_catalog_service
    launch_order_service
    launch_payment_processor
    launch_notification_service
    launch_search_indexer
    launch_recommendation_engine
    launch_analytics_pipeline
}

launch_all_ebpf_services() {
    echo "Launching eBPF & Network Observability Services..."
    launch_shield
    launch_unheaded_daemon
    launch_ebpf_exporter
    launch_network_tracer
    launch_syscall_monitor
}

launch_all_data_services() {
    echo "Launching Data & Persistence Services..."
    launch_postgres
    launch_redis_primary
    launch_redis_replica
    launch_elasticsearch
    launch_minio
    launch_mongodb
}

launch_all_gpu_services() {
    echo "Launching AI/ML & GPU Services (host-a only)..."
    launch_vllm_deepseek
    launch_embeddings_service
}

launch_all_telemetry_services() {
    echo "Launching Observability & Telemetry Services..."
    launch_prometheus
    launch_grafana
    launch_loki
    launch_victoria_metrics
}

launch_all_services() {
    echo "=== Unheaded Kingdom Full Deployment ==="
    echo "Launching all 25 microservices..."
    
    launch_all_data_services && sleep 15
    launch_all_telemetry_services && sleep 15
    launch_all_ebpf_services && sleep 15
    launch_all_api_services && sleep 15
    launch_all_gpu_services
    
    echo "=== Deployment Complete ==="
    echo "Run: lxc list"
}

# ============================================================================
# UTILITY FUNCTIONS
# ============================================================================

verify_infrastructure() {
    echo "Verifying LXD infrastructure..."
    echo "Storage pools:"
    lxc storage list
    echo ""
    echo "Networks:"
    lxc network list
    echo ""
    echo "Profiles:"
    lxc profile list
}

cleanup_all() {
    echo "WARNING: Deleting all Unheaded containers..."
    read -p "Are you sure? (yes/no): " confirm
    if [ "$confirm" = "yes" ]; then
        lxc list --format=csv | awk -F',' '{print $1}' | xargs -I {} lxc delete -f {}
        echo "All containers deleted"
    fi
}

show_status() {
    echo "=== Unheaded Kingdom Container Status ==="
    lxc list
}

# ============================================================================
# MAIN - Allow sourcing or direct execution
# ============================================================================

if [ "${1:-}" == "" ]; then
    echo "Unheaded Kingdom LXD Launch Helper"
    echo ""
    echo "Usage: bash LAUNCH_EXAMPLES.sh [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  api-gateway                    Launch api-gateway service"
    echo "  shield                         Launch shield eBPF service"
    echo "  postgres                       Launch postgres database"
    echo "  vllm-deepseek                  Launch vLLM GPU service (host-a only)"
    echo "  prometheus                     Launch prometheus telemetry"
    echo ""
    echo "  all-api                        Launch all 10 API services"
    echo "  all-ebpf                       Launch all 5 eBPF services"
    echo "  all-data                       Launch all 6 data services"
    echo "  all-gpu                        Launch all 2 GPU services (host-a only)"
    echo "  all-telemetry                  Launch all 4 telemetry services"
    echo "  all                            Launch all 25 services"
    echo ""
    echo "  verify                         Verify infrastructure"
    echo "  status                         Show container status"
    echo "  cleanup                        Delete all containers (destructive)"
    echo ""
    exit 0
fi

case "$1" in
    # API Services
    api-gateway)           launch_api_gateway ;;
    auth-service)          launch_auth_service ;;
    user-service)          launch_user_service ;;
    catalog-service)       launch_catalog_service ;;
    order-service)         launch_order_service ;;
    payment-processor)     launch_payment_processor ;;
    notification-service)  launch_notification_service ;;
    search-indexer)        launch_search_indexer ;;
    recommendation-engine) launch_recommendation_engine ;;
    analytics-pipeline)    launch_analytics_pipeline ;;
    
    # eBPF Services
    shield)                launch_shield ;;
    unheaded-daemon)       launch_unheaded_daemon ;;
    ebpf-exporter)         launch_ebpf_exporter ;;
    network-tracer)        launch_network_tracer ;;
    syscall-monitor)       launch_syscall_monitor ;;
    
    # Data Services
    postgres)              launch_postgres ;;
    redis-primary)         launch_redis_primary ;;
    redis-replica)         launch_redis_replica ;;
    elasticsearch)         launch_elasticsearch ;;
    minio)                 launch_minio ;;
    mongodb)               launch_mongodb ;;
    
    # GPU Services
    vllm-deepseek)         launch_vllm_deepseek ;;
    embeddings-service)    launch_embeddings_service ;;
    
    # Telemetry Services
    prometheus)            launch_prometheus ;;
    grafana)               launch_grafana ;;
    loki)                  launch_loki ;;
    victoria-metrics)      launch_victoria_metrics ;;
    
    # Batch commands
    all-api)               launch_all_api_services ;;
    all-ebpf)              launch_all_ebpf_services ;;
    all-data)              launch_all_data_services ;;
    all-gpu)               launch_all_gpu_services ;;
    all-telemetry)         launch_all_telemetry_services ;;
    all)                   launch_all_services ;;
    
    # Utilities
    verify)                verify_infrastructure ;;
    status)                show_status ;;
    cleanup)               cleanup_all ;;
    
    *)
        echo "Unknown command: $1"
        echo "Run: bash LAUNCH_EXAMPLES.sh (no args) for help"
        exit 1
        ;;
esac
