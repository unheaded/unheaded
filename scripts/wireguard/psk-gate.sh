#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# psk-gate.sh: forced command for the WireGuard rotation key on east (DP-05).
# authorized_keys: command="/usr/local/lib/unheaded/wireguard/psk-gate.sh",
# no-port-forwarding,no-agent-forwarding,no-X11-forwarding,no-pty <key>
# rotate-psk.sh feeds its side script on stdin to `sudo -n sh -s`; that is a
# root shell, so its hash is pinned here and anything else is refused.
#
# The pinned hash is the sha256 of the side script exactly as rotate-psk.sh
# sends it (the lines between "cat <<'SIDE'" and "SIDE", newline at the end).
# A new rotate-psk.sh needs a new side.sha256 on east, or every rotation is
# refused here. scripts/wireguard/test-rotate-psk.sh runs this file.
set -euo pipefail
SIDE_SHA=/usr/local/lib/unheaded/wireguard/side.sha256
# The test points this at a temporary file. sshd sets SSH_CONNECTION on every
# session, so a caller who got a variable through AcceptEnv cannot move the pin.
if [ -z "${SSH_CONNECTION:-}" ] && [ -n "${PSK_GATE_SIDE_SHA:-}" ]; then
	SIDE_SHA="$PSK_GATE_SIDE_SHA"
fi
c="${SSH_ORIGINAL_COMMAND:-}"
P='[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]='
logger -t unheaded-psk-gate -- "request: ${c:0:120}" || true
case "$c" in
true) exit 0 ;;
'sudo -n wg show wg0 public-key') exec sudo -n wg show wg0 public-key ;;
"sudo -n sh -c 'umask 077; cat >/etc/wireguard/wg0.psk.new'") exec sudo -n sh -c 'umask 077; cat >/etc/wireguard/wg0.psk.new' ;;
"sudo -n sh -c 'sha256sum </etc/wireguard/wg0.psk.new'") exec sudo -n sh -c 'sha256sum </etc/wireguard/wg0.psk.new' ;;
esac
if [[ "$c" =~ ^sudo\ -n\ sh\ -s\ --\ (check|stage-old|apply|restore|commit|cleanup)\ wg0\ /etc/wireguard\ ($P)$ ]]; then
	body="$(
		cat
		echo x
	)"
	body="${body%x}"
	[ "$(printf '%s' "$body" | sha256sum | cut -d' ' -f1)" = "$(cat "$SIDE_SHA")" ] || {
		echo "psk-gate: side script hash mismatch" >&2
		exit 3
	}
	rc=0
	printf '%s' "$body" | sudo -n sh -s -- "${BASH_REMATCH[1]}" wg0 /etc/wireguard "${BASH_REMATCH[2]}" || rc=$?
	exit "$rc"
fi
echo "psk-gate: refused" >&2
exit 3
