#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# install-chaos-west.sh — install the Unheaded chaos-controller on WEST
# (S-REAL-STEEL Phase 11, E2.5, Step 303). West runs the netem and
# container-kill injectors: it needs CAP_NET_ADMIN (tc) and, for the Phase 13
# packet lab, CAP_BPF, granted by the west-packet-lab drop-in. The unit's
# ExecStopPost runs `chaos-controller revert-all`, so no fault outlives the
# controller (D-19), a crash or SIGKILL included.
#
# Dry run by default (prints every step, changes nothing). Pass --apply to
# make changes; --apply needs root. The chaos.env file holding secrets is
# NEVER written by this script: it prints the file for you to create by hand.
#
#   ./scripts/install-chaos-west.sh            # dry run
#   sudo ./scripts/install-chaos-west.sh --apply
#
# After it finishes: create /etc/unheaded/chaos/chaos.env, then
#   sudo systemctl daemon-reload
#   sudo systemctl enable --now unheaded-chaos-controller
set -euo pipefail

APPLY=0
for arg in "$@"; do
	case "$arg" in
	--apply) APPLY=1 ;;
	-h | --help)
		sed -n '2,24p' "$0"
		exit 0
		;;
	*)
		echo "unknown argument: $arg" >&2
		exit 2
		;;
	esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BIN_DST=/usr/local/bin/chaos-controller
ETC_DIR=/etc/unheaded/chaos
UNIT_DST=/etc/systemd/system/unheaded-chaos-controller.service
DROPIN_DIR=/etc/systemd/system/unheaded-chaos-controller.service.d
SYSUSERS_DST=/etc/sysusers.d/unheaded.conf

ALLOWLIST_SRC="$REPO_ROOT/configs/chaos/allowlist.yaml"
POLICY_SRC="$REPO_ROOT/observability/alerting/policy.yaml"
UNIT_SRC="$REPO_ROOT/deploy/systemd/unheaded-chaos-controller.service"
DROPIN_SRC="$REPO_ROOT/deploy/systemd/unheaded-chaos-controller.service.d/west-packet-lab.conf"
SYSUSERS_SRC="$REPO_ROOT/deploy/sysusers.d/unheaded.conf"

run() {
	if [ "$APPLY" -eq 1 ]; then
		"$@"
	else
		printf 'DRY  %s\n' "$*"
	fi
}

if [ "$APPLY" -eq 1 ] && [ "$(id -u)" -ne 0 ]; then
	echo "--apply must run as root (try sudo)" >&2
	exit 1
fi

for f in "$ALLOWLIST_SRC" "$POLICY_SRC" "$UNIT_SRC" "$DROPIN_SRC" "$SYSUSERS_SRC"; do
	if [ ! -f "$f" ]; then
		echo "missing source file: $f" >&2
		exit 1
	fi
done

echo "== build chaos-controller =="
BIN_BUILT="$REPO_ROOT/chaos-controller"
run env -C "$REPO_ROOT" go build -o "$BIN_BUILT" ./cmd/chaos-controller

echo "== system user (unheaded-chaos, uid 16763) =="
run install -m 0644 "$SYSUSERS_SRC" "$SYSUSERS_DST"
run systemd-sysusers

echo "== binary =="
run install -m 0755 -o root -g root "$BIN_BUILT" "$BIN_DST"

echo "== config dir and files =="
run install -d -m 0750 -o root -g unheaded-chaos "$ETC_DIR"
run install -m 0640 -o root -g unheaded-chaos "$ALLOWLIST_SRC" "$ETC_DIR/allowlist.yaml"
run install -m 0640 -o root -g unheaded-chaos "$POLICY_SRC" "$ETC_DIR/policy.yaml"

echo "== systemd unit and west packet-lab drop-in (CAP_NET_ADMIN + CAP_BPF) =="
run install -m 0644 "$UNIT_SRC" "$UNIT_DST"
run install -d -m 0755 "$DROPIN_DIR"
run install -m 0644 "$DROPIN_SRC" "$DROPIN_DIR/west-packet-lab.conf"

cat <<EOF

NEXT (by hand — this script never writes secrets):
  Create ${ETC_DIR}/chaos.env (0640 root:unheaded-chaos) with:
    AUTH_ENABLED=true
    AUTH_API_KEYS=<comma-separated keys>
    CUIRASS_URL=http://127.0.0.1:19006
    CUIRASS_API_KEY=<key the daemon accepts, for container_kill>
    VM_URL=http://127.0.0.1:8428
    WOTAN_ADDR=<wotan http addr>        # optional: events + chaos.abort
    CHAOS_DRY_RUN=false                 # ONLY during a game-day window
  Then:
    sudo chmod 0640 ${ETC_DIR}/chaos.env
    sudo chown root:unheaded-chaos ${ETC_DIR}/chaos.env
    sudo systemctl daemon-reload
    sudo systemctl enable --now unheaded-chaos-controller
EOF

if [ "$APPLY" -eq 0 ]; then
	echo
	echo "dry run only; re-run with --apply (as root) to make changes."
fi
