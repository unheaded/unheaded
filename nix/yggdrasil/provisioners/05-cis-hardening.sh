#!/bin/bash
# 05-cis-hardening.sh — Apply CIS Debian Linux 12 Benchmark Level 1 hardening.
# Free to use. Free to share.
set -euo pipefail

echo "=== CIS Level 1 hardening pass ==="

# ── Section 1: Initial Setup ────────────────────────────────────────────
echo "[1.1.1] Disable unused filesystem modules"
for FS in cramfs freevxfs jffs2 hfs hfsplus squashfs udf usb-storage; do
    cat > /etc/modprobe.d/cis-${FS}.conf <<EOF
install $FS /bin/true
blacklist $FS
EOF
done

echo "[1.5] Address Space Layout Randomization"
echo "kernel.randomize_va_space = 2" > /etc/sysctl.d/cis-aslr.conf

echo "[1.7] Banner"
cat > /etc/issue <<EOF
Authorized uses only. All activity may be monitored and reported.
This system is part of the Unheaded Kingdom (Yggdrasil).
EOF
cat > /etc/issue.net <<EOF
Authorized uses only. All activity may be monitored and reported.
EOF

# ── Section 3: Network Configuration ────────────────────────────────────
echo "[3.1] Disable IPv4 forwarding (re-enable per service if needed)"
cat > /etc/sysctl.d/cis-network.conf <<'EOF'
# Yggdrasil CIS L1 network hardening
net.ipv4.ip_forward = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.default.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv4.conf.default.accept_source_route = 0
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv4.conf.all.secure_redirects = 0
net.ipv4.conf.default.secure_redirects = 0
net.ipv4.conf.all.log_martians = 1
net.ipv4.conf.default.log_martians = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.icmp_ignore_bogus_error_responses = 1
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.tcp_syncookies = 1
# IPv6 — Monad wire format requires IPv6; do NOT disable
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.default.accept_redirects = 0
net.ipv6.conf.all.accept_source_route = 0
net.ipv6.conf.default.accept_source_route = 0
EOF

# ── Section 4: Logging and Auditing ─────────────────────────────────────
echo "[4.1] Configure auditd"
cat > /etc/audit/rules.d/cis-yggdrasil.rules <<'EOF'
# Yggdrasil CIS L1 audit rules
-D
-b 8192
-f 1
# Time changes
-a always,exit -F arch=b64 -S adjtimex -S settimeofday -k time-change
-a always,exit -F arch=b64 -S clock_settime -k time-change
-w /etc/localtime -p wa -k time-change
# User/group modifications
-w /etc/group -p wa -k identity
-w /etc/passwd -p wa -k identity
-w /etc/gshadow -p wa -k identity
-w /etc/shadow -p wa -k identity
-w /etc/security/opasswd -p wa -k identity
# Network configuration
-w /etc/issue -p wa -k system-locale
-w /etc/issue.net -p wa -k system-locale
-w /etc/hosts -p wa -k system-locale
-w /etc/network -p wa -k system-locale
# Privileged commands
-a always,exit -F path=/usr/bin/sudo -F perm=x -F auid>=1000 -F auid!=4294967295 -k sudo-actions
-a always,exit -F path=/bin/su -F perm=x -F auid>=1000 -F auid!=4294967295 -k su-actions
# Mount events
-a always,exit -F arch=b64 -S mount -F auid>=1000 -F auid!=4294967295 -k mounts
# UPC events (Kingdom-specific)
-w /usr/bin/upc-bootctl -p x -k upc-boot
-w /opt/unheaded/share/monad-cpu-ebpf.o -p wa -k upc-bpf-mutation
-w /etc/systemd/system/upc-tty-bridge.service -p wa -k upc-unit-mutation
# Immutable rule (must be last)
-e 2
EOF

# ── Section 5: Access, Authentication and Authorization ─────────────────
echo "[5.2] SSH server hardening (additional to provisioner 01)"
cat >> /etc/ssh/sshd_config.d/cis-yggdrasil.conf <<'EOF'
Protocol 2
LogLevel INFO
MaxAuthTries 4
ClientAliveInterval 300
ClientAliveCountMax 0
LoginGraceTime 60
AllowUsers yggdrasil
Banner /etc/issue.net
Ciphers chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,aes128-gcm@openssh.com,aes256-ctr,aes192-ctr,aes128-ctr
MACs hmac-sha2-512-etm@openssh.com,hmac-sha2-256-etm@openssh.com,hmac-sha2-512,hmac-sha2-256
KexAlgorithms curve25519-sha256,curve25519-sha256@libssh.org,ecdh-sha2-nistp521,ecdh-sha2-nistp384,ecdh-sha2-nistp256,diffie-hellman-group-exchange-sha256
EOF

echo "[5.3] Password policy"
cat > /etc/pam.d/common-password-cis <<'EOF'
password requisite pam_pwquality.so retry=3 minlen=14 dcredit=-1 ucredit=-1 ocredit=-1 lcredit=-1
password [success=1 default=ignore] pam_unix.so obscure use_authtok try_first_pass yescrypt remember=5
EOF

# ── Section 6: System Maintenance ───────────────────────────────────────
echo "[6.1] System file permissions"
chmod 600 /etc/shadow /etc/gshadow /etc/security/opasswd 2>/dev/null || true
chmod 644 /etc/passwd /etc/group
chmod 700 /root

echo "=== Apply sysctl ==="
sysctl --system >/dev/null 2>&1 || echo "WARN: sysctl --system had warnings"

echo "=== Restart auditd ==="
systemctl restart auditd 2>/dev/null || echo "WARN: auditd restart deferred to next boot"

echo "=== Restart sshd to pick up new config ==="
systemctl restart ssh

echo "=== Step 05 complete (CIS L1 baseline applied) ==="
