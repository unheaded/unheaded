#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
# Suricata entrypoint — IDS mode on ${SURICATA_INTERFACE:-eth1}
# EVE JSON tailed to stdout for Docker log aggregation + written to file for anamnesis

set -euo pipefail

IFACE="${SURICATA_INTERFACE:-eth1}"
LOG_DIR="${SURICATA_LOG_DIR:-/var/log/suricata}"
CONFIG="${SURICATA_CONFIG:-/etc/suricata/suricata.yaml}"

# Verify suricata binary exists
if ! command -v suricata &>/dev/null; then
    echo "ERROR: suricata binary not found. Build from ~/tmp/suricata/ and install first." >&2
    echo "Run: scripts/suricata/build-suricata.sh" >&2
    exit 1
fi

# Verify configuration file exists
if [ ! -f "${CONFIG}" ]; then
    echo "ERROR: Configuration file not found: ${CONFIG}" >&2
    exit 1
fi

# Verify log directory exists
if [ ! -d "${LOG_DIR}" ]; then
    echo "ERROR: Log directory not found: ${LOG_DIR}" >&2
    exit 1
fi

echo "Starting Suricata IDS on interface: ${IFACE}"
echo "Configuration: ${CONFIG}"
echo "Log directory: ${LOG_DIR}"

# Start suricata in background, AF_PACKET mode, autofp load balancing
suricata \
    -c "${CONFIG}" \
    --af-packet="${IFACE}" \
    --runmode=autofp \
    --unix-socket=/run/suricata/suricata.socket &
SURICATA_PID=$!

echo "Suricata started (PID: ${SURICATA_PID})"

# Wait for Suricata to initialize and open EVE JSON log
sleep 3

# Tail EVE JSON to stdout for Docker log capture + anamnesis bridge
if [ -f "${LOG_DIR}/eve.json" ]; then
    echo "Tailing EVE JSON to stdout..."
    tail -f "${LOG_DIR}/eve.json" &
else
    echo "WARNING: EVE JSON not yet created, waiting..."
    for i in {1..30}; do
        if [ -f "${LOG_DIR}/eve.json" ]; then
            echo "EVE JSON ready, tailing to stdout..."
            tail -f "${LOG_DIR}/eve.json" &
            break
        fi
        sleep 1
    done
fi

# Wait for suricata process (if it dies, container dies)
wait ${SURICATA_PID}
