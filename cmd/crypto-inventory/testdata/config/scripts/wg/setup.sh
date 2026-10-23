#!/bin/bash
cat > /etc/wireguard/wg0.conf <<CONF
[Peer]
PublicKey = ${peer}
PresharedKey = ${psk}
CONF
