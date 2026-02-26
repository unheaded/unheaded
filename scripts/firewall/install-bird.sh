#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# install-bird.sh — Build and install BIRD from ~/tmp/bird-master/

set -euo pipefail

BIRD_SRC="${BIRD_SRC:-${HOME}/tmp/bird-master}"
JOBS="${JOBS:-$(nproc)}"

echo "Building BIRD from: $BIRD_SRC"

if command -v apt-get &>/dev/null; then
    apt-get install -y make gcc flex bison libreadline-dev libssh-dev
fi

groupadd -g 153 bird 2>/dev/null || true
useradd -u 153 -g bird -d /var/run/bird bird 2>/dev/null || true

cd "$BIRD_SRC"
autoreconf -fi

./configure \
    --prefix=/usr \
    --sysconfdir=/etc/bird \
    --localstatedir=/var/run \
    --enable-ipv6 \
    --with-protocols=bgp,ospf,bfd,static,direct,kernel,device,radv,rpki,l3vpn

make -j"$JOBS"
make install

# Deploy config
mkdir -p /etc/bird
cp "$(dirname "$0")/../../routing/bird/bird.conf" /etc/bird/bird.conf
chown bird:bird /etc/bird/bird.conf

# Create systemd service
cat > /etc/systemd/system/bird.service << 'EOF'
[Unit]
Description=BIRD Internet Routing Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=forking
PIDFile=/var/run/bird.ctl
ExecStart=/usr/sbin/bird -f -c /etc/bird/bird.conf
ExecReload=/usr/sbin/birdc configure
Restart=on-failure
RestartSec=5
User=bird
Group=bird
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable bird

echo "✓ BIRD installed. Start with: systemctl start bird"
