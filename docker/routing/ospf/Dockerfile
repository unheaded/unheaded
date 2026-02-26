# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# FRR OSPFv3 Docker image — Alternate Routing Option A
# Build: docker build -t unheaded/frr-ospf:latest docker/routing/ospf/

FROM ubuntu:24.04 AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential autoconf automake libtool git \
    libreadline-dev libcap-dev libpcre2-dev libjson-c-dev \
    libelf-dev bison flex python3 python3-setuptools \
    pkg-config libprotobuf-c-dev protobuf-c-compiler \
    libzmq5 libzmq3-dev cmake && \
    rm -rf /var/lib/apt/lists/*

FROM ubuntu:24.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    libreadline8 libcap2 libjson-c5 libelf1 libpcre2-8-0 \
    libprotobuf-c1 libzmq5 iproute2 tcpdump && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd -g 92 frr && useradd -u 92 -g 92 -s /usr/sbin/nologin frr && \
    groupadd -g 85 frrvty && usermod -aG frrvty frr && \
    mkdir -p /etc/frr /var/log/frr /run/frr && \
    chown -R frr:frr /etc/frr /var/log/frr /run/frr

COPY entrypoint.sh /usr/local/bin/
COPY frr-ospf.conf /etc/frr/frr.conf

RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 2601 2617

HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["/usr/local/bin/entrypoint.sh"]
