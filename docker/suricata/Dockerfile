# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# Multi-stage Suricata Docker build for Unheaded Kingdom
# Suricata compiled from ~/tmp/suricata/ with eBPF + AF_PACKET support

# === Stage 1: Builder ===
FROM ubuntu:24.04 AS builder

LABEL stage=builder
LABEL description="Build environment for Suricata with eBPF and AF_PACKET support"

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    autoconf \
    automake \
    libtool \
    libpcap-dev \
    libnet1-dev \
    libyaml-dev \
    zlib1g-dev \
    libcap-ng-dev \
    libgeoip-dev \
    liblua5.1-dev \
    libhiredis-dev \
    libevent-dev \
    python3-distutils \
    libjansson-dev \
    libnetfilter-queue-dev \
    libnetfilter-queue1 \
    libnetfilter-conntrack-dev \
    libnetfilter-conntrack3 \
    pkg-config \
    libpcre2-dev \
    libmagic-dev \
    libbpf-dev \
    clang \
    llvm \
    && rm -rf /var/lib/apt/lists/*

# NOTE: Suricata is built from ~/tmp/suricata/ at deployment time
# See scripts/suricata/build-suricata.sh for compilation with:
#   ./configure --enable-ebpf --enable-af-packet --prefix=/usr/local
#   make && make install

# Create placeholder script to verify binary features
RUN mkdir -p /tmp/check && cat > /tmp/check/verify-suricata.sh << 'EOFSCRIPT' \
#!/bin/bash
if ! command -v suricata &>/dev/null; then
    echo "ERROR: suricata binary not found"
    exit 1
fi
suricata --build-info | grep -q "eBPF support: yes" || (echo "ERROR: eBPF not enabled" && exit 1)
suricata --build-info | grep -q "AF_PACKET support: yes" || (echo "ERROR: AF_PACKET not enabled" && exit 1)
echo "Binary verification complete"
EOFSCRIPT

# === Stage 2: Runtime ===
FROM ubuntu:24.04 AS runtime

LABEL maintainer="Steven Bellis"
LABEL description="Suricata IDS/IPS with EVE JSON boundary for GPL isolation"
LABEL org.unheaded.service="suricata"

# Install runtime dependencies only
RUN apt-get update && apt-get install -y --no-install-recommends \
    libpcap0.8 \
    libyaml-0-2 \
    zlib1g \
    libcap-ng0 \
    libgeoip1 \
    libevent-2.1-7 \
    libjansson4 \
    libnetfilter-queue1 \
    libnetfilter-conntrack3 \
    libbpf1 \
    && rm -rf /var/lib/apt/lists/*

# Create suricata user (uid=994) and group (gid=994)
RUN groupadd -g 994 suricata && \
    useradd -r -u 994 -g suricata -d /var/lib/suricata -s /usr/sbin/nologin suricata

# Create log directory with proper ownership
RUN mkdir -p /var/log/suricata && \
    chown -R suricata:suricata /var/log/suricata && \
    chmod 755 /var/log/suricata

# Create runtime socket directory
RUN mkdir -p /run/suricata && \
    chown -R suricata:suricata /run/suricata && \
    chmod 700 /run/suricata

# Create config directory
RUN mkdir -p /etc/suricata/rules && \
    chmod 755 /etc/suricata

# COPY suricata.yaml from build context
COPY suricata.yaml /etc/suricata/suricata.yaml
RUN chmod 644 /etc/suricata/suricata.yaml

# COPY entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod 755 /entrypoint.sh

# NOTE: suricata binary must be installed from ~/tmp/suricata/ build
# The build process is managed by scripts/suricata/build-suricata.sh
# Binary location: /usr/local/bin/suricata or /usr/bin/suricata
# This allows GPL-2.0 isolation — Suricata runs on bare metal or in container
# Communication via: EVE JSON (/var/log/suricata/eve.json) + unix socket (/run/suricata/suricata.socket)

USER suricata:suricata
WORKDIR /var/log/suricata

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD test -f /var/log/suricata/eve.json || exit 1

ENTRYPOINT ["/entrypoint.sh"]
