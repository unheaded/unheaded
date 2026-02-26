# SPDX-License-Identifier: MIT
# IPFire 2.29 QEMU-in-Docker — runs IPFire Linux in KVM
# Same QEMU pattern as OPNsense for consistency

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-system-x86 qemu-utils qemu-kvm ovmf \
    iproute2 bridge-utils iptables nftables \
    busybox-static xz-utils curl wget \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /images /run/ipfire /etc/ipfire

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22 444 81

VOLUME ["/images", "/var/lib/ipfire"]

ENV IPFIRE_MEM=2048 \
    IPFIRE_CPUS=2 \
    IPFIRE_WAN_IFACE=eth0 \
    IPFIRE_LAN_IFACE=eth1

ENTRYPOINT ["/entrypoint.sh"]
