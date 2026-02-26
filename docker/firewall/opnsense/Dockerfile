# SPDX-License-Identifier: MIT
# Copyright (c) 2024-2026 Steven Bellis. All rights reserved.
# OPNsense 26.1.2 QEMU-in-Docker — runs FreeBSD OPNsense in KVM inside a container
# Requires: --device /dev/kvm, --privileged (for KVM + macvlan), NET_ADMIN cap

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-system-x86 qemu-utils qemu-kvm ovmf \
    iproute2 bridge-utils iptables nftables socat \
    busybox-static pbzip2 curl wget \
    && rm -rf /var/lib/apt/lists/*

# Create runtime directories
RUN mkdir -p /images /run/opnsense /etc/opnsense

# Copy entrypoint
COPY entrypoint.sh /entrypoint.sh
COPY prepare-image.sh /prepare-image.sh
RUN chmod +x /entrypoint.sh /prepare-image.sh

# OPNsense serial console on /dev/ttyS0 — accessible via docker exec
EXPOSE 22 80 443 8080

VOLUME ["/images", "/var/lib/opnsense"]

ENV OPNSENSE_MEM=4096 \
    OPNSENSE_CPUS=4 \
    OPNSENSE_WAN_IFACE=eth0 \
    OPNSENSE_LAN_IFACE=eth1 \
    OPNSENSE_SERIAL=yes

ENTRYPOINT ["/entrypoint.sh"]
