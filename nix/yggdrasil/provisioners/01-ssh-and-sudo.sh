#!/bin/bash
# 01-ssh-and-sudo.sh — Replace preseed password with SSH key + sudo lockdown.
# Runs as root via packer shell provisioner.
# Free to use. Free to share.
set -euo pipefail

YGGDRASIL_AUTHORIZED_KEYS_URL="${YGGDRASIL_AUTHORIZED_KEYS_URL:-https://apt.unheaded.dev/yggdrasil/authorized_keys}"

echo "=== Installing yggdrasil user SSH authorized_keys ==="
install -d -m 0700 -o yggdrasil -g yggdrasil /home/yggdrasil/.ssh

# Authorized keys are baked into the image at build time. In production the
# keys come from the kingdom secret store; for the scaffold we accept a URL
# overridable via env var.
curl -fsSL --retry 3 --max-time 30 "$YGGDRASIL_AUTHORIZED_KEYS_URL" \
    -o /home/yggdrasil/.ssh/authorized_keys || {
        echo "WARN: authorized_keys fetch failed; falling back to empty (SSH will be unusable until populated)"
        : > /home/yggdrasil/.ssh/authorized_keys
    }
chmod 0600 /home/yggdrasil/.ssh/authorized_keys
chown yggdrasil:yggdrasil /home/yggdrasil/.ssh/authorized_keys

echo "=== Disabling SSH password authentication ==="
sed -i \
    -e 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' \
    -e 's/^#*PermitRootLogin.*/PermitRootLogin no/' \
    -e 's/^#*PubkeyAuthentication.*/PubkeyAuthentication yes/' \
    -e 's/^#*ChallengeResponseAuthentication.*/ChallengeResponseAuthentication no/' \
    -e 's/^#*X11Forwarding.*/X11Forwarding no/' \
    -e 's/^#*AllowTcpForwarding.*/AllowTcpForwarding no/' \
    /etc/ssh/sshd_config

echo "=== Locking the preseed password (sudo via key only) ==="
# Keep sudo working for the build (provisioners need it) but DROP the
# password-based unlock. The packer template sets ssh_password=yggdrasil
# during build; provisioner 08 wipes the password hash entirely.
echo 'yggdrasil ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/yggdrasil
chmod 0440 /etc/sudoers.d/yggdrasil

echo "=== Restarting sshd ==="
systemctl restart ssh

echo "=== Step 01 complete ==="
