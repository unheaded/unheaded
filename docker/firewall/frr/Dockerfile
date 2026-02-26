# SPDX-License-Identifier: MIT
# FRR Dockerfile — builds from ~/tmp/frr-master/ source
# Based on FRR's own docker/ directory patterns
# Multi-stage build for minimal final image

FROM ubuntu:24.04 AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git autoconf automake libtool make libreadline-dev texinfo \
    pkg-config libpam0g-dev libjson-c-dev bison flex \
    libc-ares-dev python3-dev python3-sphinx \
    install-info build-essential libsnmp-dev perl \
    libcap-dev libelf-dev libunwind-dev \
    cmake libprotobuf-c-dev protobuf-c-compiler \
    libgrpc-dev libgrpc++-dev protobuf-compiler-grpc \
    libsystemd-dev libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy FRR source (expected to be bind-mounted or COPYd from context)
COPY . /src/frr
WORKDIR /src/frr

RUN ./bootstrap.sh && \
    ./configure \
        --prefix=/usr \
        --sysconfdir=/etc/frr \
        --localstatedir=/var/run/frr \
        --sbindir=/usr/lib/frr \
        --enable-user=frr \
        --enable-group=frr \
        --enable-vty-group=frrvty \
        --enable-multipath=64 \
        --enable-vtysh \
        --enable-isisd \
        --enable-bfdd \
        --enable-bgpd \
        --enable-zebra \
        --enable-mgmtd \
        --enable-staticd \
        --disable-ospfd \
        --disable-ospf6d \
        --disable-ripd \
        --disable-ripngd \
        --disable-pimd \
        --disable-ldpd \
        --with-pkg-git-version \
        --with-pkg-extra-version=-unheaded && \
    make -j$(nproc) && \
    make install

FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    libreadline8 libjson-c5 libcap2 libelf1 libunwind8 \
    libprotobuf-c1 libsqlite3-0 iproute2 iptables \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -r -g 92 frr \
    && groupadd -r -g 85 frrvty \
    && useradd -r -u 92 -g frr -G frrvty -d /var/run/frr frr

COPY --from=builder /usr/lib/frr /usr/lib/frr
COPY --from=builder /usr/bin/vtysh /usr/bin/vtysh
COPY --from=builder /etc/frr /etc/frr
COPY frr.conf /etc/frr/frr.conf
COPY daemons /etc/frr/daemons
COPY vtysh.conf /etc/frr/vtysh.conf

RUN chown -R frr:frr /etc/frr /var/run/frr && \
    mkdir -p /var/run/frr && \
    chown -R frr:frr /var/run/frr

EXPOSE 2601 2602 2605 2609 2610

HEALTHCHECK --interval=30s --timeout=10s --retries=5 --start-period=60s \
    CMD /usr/bin/vtysh -c "show bgp summary" > /dev/null 2>&1 || exit 1

CMD ["/usr/lib/frr/watchfrr", "-d", "zebra", "bgpd", "isisd", "bfdd", "staticd", "mgmtd"]
