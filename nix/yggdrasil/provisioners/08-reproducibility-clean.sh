#!/bin/bash
# 08-reproducibility-clean.sh — Final cleanup for byte-identical image rebuilds.
# Free to use. Free to share.
set -euo pipefail

SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-1746921600}"

echo "=== Reproducibility cleanup pass ==="

# 1. Zero the machine-id so each booted instance generates its own
echo "[reproducibility] Zero machine-id"
: > /etc/machine-id
: > /var/lib/dbus/machine-id 2>/dev/null || true

# 2. Clean apt cache
echo "[reproducibility] Clean apt cache"
DEBIAN_FRONTEND=noninteractive apt-get clean
rm -rf /var/cache/apt/archives/* /var/lib/apt/lists/*

# 3. Remove the preseed password
echo "[reproducibility] Wipe yggdrasil preseed password"
passwd -l yggdrasil >/dev/null
# Force key-only access — sudoers.d/yggdrasil already configured by provisioner 01

# 4. Remove SSH host keys — first boot regenerates
echo "[reproducibility] Wipe ssh host keys (regen on first boot)"
rm -f /etc/ssh/ssh_host_*
cat > /etc/systemd/system/yggdrasil-firstboot-sshkeys.service <<'EOF'
[Unit]
Description=Yggdrasil first-boot SSH host key regen
ConditionPathExists=!/etc/ssh/ssh_host_ed25519_key
Before=ssh.service
After=local-fs.target

[Service]
Type=oneshot
ExecStart=/usr/bin/dpkg-reconfigure openssh-server
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF
systemctl enable yggdrasil-firstboot-sshkeys.service

# 5. Clear command-line history
echo "[reproducibility] Clear shell history"
: > /root/.bash_history 2>/dev/null || true
: > /home/yggdrasil/.bash_history 2>/dev/null || true

# 6. Clear logs (the image ships clean; auditd starts fresh on first boot)
echo "[reproducibility] Clear logs"
find /var/log -type f -exec truncate -s 0 {} \; 2>/dev/null || true

# 7. Touch all files to SOURCE_DATE_EPOCH for deterministic mtimes
echo "[reproducibility] Touch all files to SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH"
find / -path /proc -prune -o -path /sys -prune -o -path /dev -prune -o -path /run -prune -o -print 2>/dev/null \
    | xargs -r touch -h -d "@$SOURCE_DATE_EPOCH" 2>/dev/null || true

# 8. Sync + trim
echo "[reproducibility] Sync + fstrim"
sync
fstrim -av 2>/dev/null || true

echo "=== Step 08 complete ==="
