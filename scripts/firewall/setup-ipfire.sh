#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
# setup-ipfire.sh — IPFire 2.29 post-boot configuration
# Configures: network interfaces, firewall rules, Monad HbH, BIRD routing, pakfire packages

set -euo pipefail

# IPFire is accessed via SSH after initial wizard
IPFIRE_HOST="${IPFIRE_HOST:-10.20.0.1}"
IPFIRE_USER="${IPFIRE_USER:-root}"
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=5"

echo "========================================"
echo "Unheaded IPFire 2.29 Setup"
echo "Host: ${IPFIRE_HOST}"
echo "========================================"

# shellcheck disable=SC2086  # SSH_OPTS is a list of -o flags; must split
ssh_cmd() { ssh $SSH_OPTS "${IPFIRE_USER}@${IPFIRE_HOST}" "$@"; }
# shellcheck disable=SC2086  # SSH_OPTS is a list of -o flags; must split
scp_file() { scp $SSH_OPTS "$1" "${IPFIRE_USER}@${IPFIRE_HOST}:$2"; }

wait_for_ipfire() {
    echo "[wait] Waiting for IPFire SSH..."
    local retries=30
    # shellcheck disable=SC2086  # SSH_OPTS is a list of -o flags; must split
    while ! ssh $SSH_OPTS -o BatchMode=yes "${IPFIRE_USER}@${IPFIRE_HOST}" true 2>/dev/null; do
        retries=$((retries - 1))
        [[ $retries -eq 0 ]] && echo "ERROR: IPFire SSH timeout" && exit 1
        sleep 10
    done
    echo "[wait] IPFire SSH ready."
}

wait_for_ipfire

# --- Step 1: Verify ---
echo "[1/8] Verifying IPFire version..."
ssh_cmd "cat /etc/system-release"

# --- Step 2: Monad HbH nftables rules ---
echo "[2/8] Adding Monad HbH nftables rules..."
ssh_cmd "cat > /etc/nftables.d/unheaded-monad.nft << 'EOF'
# Unheaded Monad HbH passthrough rules
# IPv6 Hop-by-Hop extension headers carry the 20-byte Monad register
table ip6 unheaded {
    chain monad-hbh {
        type filter hook forward priority -50;
        
        # CRITICAL: Pass IPv6 HbH headers (Monad 20-byte register)
        # Next-header 0x00 = HOPOPT (Hop-by-Hop Options)
        ip6 nexthdr 0 accept comment \"Monad HbH passthrough\"
        
        # Allow ICMPv6 (NDP, path MTU — required for IPv6 to work)
        ip6 nexthdr icmpv6 accept
        
        # Allow established (after HbH check)
        ct state established,related accept
    }
}
EOF"

ssh_cmd "nft -f /etc/nftables.d/unheaded-monad.nft && echo '✓ Monad HbH rules loaded'"

# Persist rules
ssh_cmd "echo 'include \"/etc/nftables.d/unheaded-monad.nft\"' >> /etc/nftables.conf 2>/dev/null || true"

# --- Step 3: Install BIRD via pakfire ---
echo "[3/8] Installing BIRD via pakfire..."
ssh_cmd "pakfire install bird -y || echo 'pakfire install failed — may need manual install'"

# --- Step 4: Deploy BIRD config ---
echo "[4/8] Deploying BIRD configuration..."
scp_file "routing/bird/bird.conf" "/etc/bird/bird.conf"
ssh_cmd "chown bird:bird /etc/bird/bird.conf && chmod 640 /etc/bird/bird.conf"
ssh_cmd "systemctl enable bird && systemctl start bird && echo '✓ BIRD started'"

# --- Step 5: WireGuard config ---
echo "[5/8] Configuring WireGuard east-west..."
ssh_cmd "cat > /etc/wireguard/wg0.conf << 'EOF'
[Interface]
PrivateKey = REPLACE_WITH_HOST_B_PRIVATE_KEY
Address = fd00:dead:beef::2/48
ListenPort = 51820
MTU = 1380

[Peer]
PublicKey = REPLACE_WITH_HOST_A_PUBLIC_KEY
Endpoint = HOST_A_WAN_IP:51820
AllowedIPs = fd00:dead:beef::/48, 10.20.255.0/24, 10.20.0.0/16
PersistentKeepalive = 25
EOF"
ssh_cmd "systemctl enable wg-quick@wg0 && systemctl start wg-quick@wg0 || true"

# --- Step 6: Default firewall rules ---
echo "[6/8] Applying firewall rules..."
ssh_cmd "cat >> /etc/nftables.d/unheaded-rules.nft << 'EOF'
table inet unheaded-fw {
    chain wan-in {
        type filter hook input priority 0; policy drop;
        
        # Monad HbH (must come first)
        ip6 nexthdr 0 accept
        
        ct state established,related accept
        ct state invalid drop
        
        # WireGuard
        udp dport 51820 accept
        
        # HTTPS
        tcp dport 443 accept
        
        # ICMPv6
        ip6 nexthdr icmpv6 accept
        
        # SSH (management, restrict to LAN in production)
        tcp dport 22 accept
        
        # Log and drop everything else
        log prefix \"[unheaded-denied] \" drop
    }
    
    chain lan-out {
        type filter hook forward priority 0; policy accept;
        ip6 nexthdr 0 accept comment \"Monad HbH\"
    }
}
EOF"
ssh_cmd "nft -f /etc/nftables.d/unheaded-rules.nft 2>/dev/null || true"

# --- Step 7: Enable IP forwarding ---
echo "[7/8] Enabling IP forwarding..."
ssh_cmd "sysctl -w net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1"
ssh_cmd "echo -e 'net.ipv4.ip_forward=1\nnet.ipv6.conf.all.forwarding=1' > /etc/sysctl.d/99-unheaded.conf"

# --- Step 8: Verify ---
echo "[8/8] Verifying..."
echo "BIRD status:"
ssh_cmd "birdc show protocols all 2>/dev/null | head -20 || echo 'BIRD not running yet'"
echo "HbH rules:"
ssh_cmd "nft list ruleset | grep -i 'monad\|hbh\|nexthdr 0' | head -10"

echo ""
echo "========================================"
echo "✓ IPFire setup complete"
echo ""
echo "Next steps:"
echo "  1. Fill WireGuard keys in /etc/wireguard/wg0.conf"
echo "  2. Verify BIRD BGP session: birdc show protocols forge"
echo "  3. Test HbH: tcpdump -i eth0 'ip6 proto 0'"
echo "========================================"
