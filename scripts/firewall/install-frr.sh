#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# install-frr.sh — Build and install FRR from ~/tmp/frr-master/
# For: bare metal or NixOS non-declarative install

set -euo pipefail

FRR_SRC="${FRR_SRC:-${HOME}/tmp/frr-master}"
INSTALL_PREFIX="${INSTALL_PREFIX:-/usr}"
JOBS="${JOBS:-$(nproc)}"

echo "Building FRR from: $FRR_SRC"
echo "Jobs: $JOBS"

# Install build deps (Debian/Ubuntu)
if command -v apt-get &>/dev/null; then
    apt-get install -y \
        git autoconf automake libtool make libreadline-dev texinfo \
        pkg-config libpam0g-dev libjson-c-dev bison flex \
        libc-ares-dev python3-dev libelf-dev libcap-dev \
        cmake libprotobuf-c-dev protobuf-c-compiler \
        libsystemd-dev libsqlite3-dev libunwind-dev
fi

# Create FRR user/group
groupadd -r -g 92 frr 2>/dev/null || true
groupadd -r -g 85 frrvty 2>/dev/null || true
useradd -r -u 92 -g frr -G frrvty -d /var/run/frr frr 2>/dev/null || true

cd "$FRR_SRC"
./bootstrap.sh

./configure \
    --prefix="$INSTALL_PREFIX" \
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
    --disable-pimd \
    --disable-ldpd \
    --with-pkg-extra-version=-unheaded

make -j"$JOBS"
make install

# Install systemd service
cp /usr/lib/frr/frr.service /etc/systemd/system/ 2>/dev/null || true
systemctl daemon-reload
systemctl enable frr

# Deploy configs from repo
mkdir -p /etc/frr
cp "$(dirname "$0")/../../routing/frr/frr.conf" /etc/frr/frr.conf
cp "$(dirname "$0")/../../routing/frr/daemons" /etc/frr/daemons
cp "$(dirname "$0")/../../routing/frr/vtysh.conf" /etc/frr/vtysh.conf
chown -R frr:frrvty /etc/frr
chmod 640 /etc/frr/*.conf

echo "✓ FRR installed. Start with: systemctl start frr"
