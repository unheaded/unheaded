#!/bin/bash
# SPDX-License-Identifier: MIT
# Setup telemetry stack on host-a (Forge)
# Creates host directories, writes configs, launches containers

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Unheaded Kingdom — Telemetry Stack Setup"
echo "========================================"
echo ""

# Create host directories for bind mounts
echo "Creating host directories for bind-mounted configs..."
sudo mkdir -p /etc/unheaded/prometheus
sudo mkdir -p /etc/unheaded/grafana/provisioning/{dashboards,datasources}
sudo chmod 755 /etc/unheaded/prometheus
sudo chmod 755 /etc/unheaded/grafana/provisioning/{dashboards,datasources}

# Write prometheus.yml to host
echo "Writing Prometheus configuration..."
sudo tee /etc/unheaded/prometheus/prometheus.yml > /dev/null << 'PROM'
global:
  scrape_interval: 5s
  evaluation_interval: 5s
  external_labels:
    cluster: 'unheaded-kingdom'

alerting:
  alertmanagers:
    - static_configs:
        - targets: []

scrape_configs:
  # Host node exporter
  - job_name: 'node-exporter-host'
    static_configs:
      - targets: ['10.20.0.1:9100']

  # Service containers (25 total)
  - job_name: 'unheaded-services'
    static_configs:
      - targets:
          - '10.20.1.10:9100'  # wotan
          - '10.20.1.11:9100'  # ares
          - '10.20.1.12:9100'  # hermes
          - '10.20.1.13:9100'  # athena
          - '10.20.1.14:9100'  # hephaestus
          - '10.20.1.15:9100'  # demeter
          - '10.20.1.16:9100'  # poseidon
          - '10.20.1.17:9100'  # hades
          - '10.20.1.18:9100'  # apollo
          - '10.20.1.19:9100'  # artemis
          - '10.20.1.20:9100'  # aphrodite
          - '10.20.1.21:9100'  # ares2
          - '10.20.1.22:9100'  # athena2
          - '10.20.1.23:9100'  # hermes2
          - '10.20.1.24:9100'  # monad
          - '10.20.1.25:9100'  # dyad
          - '10.20.1.26:9100'  # triad
          - '10.20.1.27:9100'  # tetrad
          - '10.20.1.28:9100'  # pentad
          - '10.20.1.29:9100'  # hexad
          - '10.20.1.30:9100'  # heptad
          - '10.20.1.31:9100'  # ogdoad
          - '10.20.1.32:9100'  # ennead
          - '10.20.1.33:9100'  # dekad
          - '10.20.1.34:9100'  # hendecad

remote_write:
  - url: http://10.20.1.51:8428/api/v1/write
    queue_config:
      capacity: 10000
      max_shards: 200
      min_shards: 1
      max_samples_per_send: 5000
      batch_send_wait: 5s
      min_backoff: 30ms
      max_backoff: 100ms
PROM

# Write Grafana datasources provisioning
echo "Writing Grafana datasources..."
sudo tee /etc/unheaded/grafana/provisioning/datasources/prometheus.yml > /dev/null << 'DATASRC'
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://10.20.1.50:9090
    isDefault: true
    editable: true

  - name: Loki
    type: loki
    access: proxy
    url: http://10.20.1.52:3100
    isDefault: false
    editable: true

  - name: VictoriaMetrics
    type: prometheus
    access: proxy
    url: http://10.20.1.51:8428
    isDefault: false
    editable: true
DATASRC

sudo chmod 644 /etc/unheaded/prometheus/prometheus.yml
sudo chmod 644 /etc/unheaded/grafana/provisioning/datasources/prometheus.yml

# Launch telemetry containers
echo ""
echo "Launching telemetry containers..."
echo ""

# Prometheus
echo "Launching Prometheus (10.20.1.50:9090)..."
lxc launch -f "$SCRIPT_DIR/prometheus-lxd.yaml" ubuntu:24.04 unheaded-prometheus

# VictoriaMetrics
echo "Launching VictoriaMetrics (10.20.1.51:8428)..."
lxc launch -f "$SCRIPT_DIR/victoriametrics-lxd.yaml" ubuntu:24.04 unheaded-victoriametrics

# Loki
echo "Launching Loki (10.20.1.52:3100)..."
lxc launch -f "$SCRIPT_DIR/loki-lxd.yaml" ubuntu:24.04 unheaded-loki

# Grafana
echo "Launching Grafana (10.20.1.53:3000)..."
lxc launch -f "$SCRIPT_DIR/grafana-lxd.yaml" ubuntu:24.04 unheaded-grafana

echo ""
echo "Telemetry stack launched!"
echo ""
echo "Access points:"
echo "  Prometheus:      http://10.20.1.50:9090"
echo "  VictoriaMetrics: http://10.20.1.51:8428"
echo "  Loki:            http://10.20.1.52:3100"
echo "  Grafana:         http://10.20.1.53:3000"
echo ""
echo "Waiting for containers to boot and initialize services..."
sleep 30

echo "Checking container status..."
lxc list unheaded-{prometheus,victoriametrics,loki,grafana}

echo ""
echo "Done! Telemetry stack is ready."
