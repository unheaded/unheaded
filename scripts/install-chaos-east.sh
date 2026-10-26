#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# install-chaos-east.sh — install the Unheaded chaos-controller on EAST
# (S-REAL-STEEL Phase 12, E3). East runs the systemd injector only: no
# CAP_NET_ADMIN, no CAP_BPF; authorization for stop/kill/start of the
# allowlisted units comes from a polkit rule rendered from the allowlist.
#
# Dry run by default (prints every step, changes nothing). Pass --apply to
# make changes; --apply needs root. The chaos.env file holding secrets is
# NEVER written by this script: it prints the file for you to create by hand
# (credentials are not stored in the repo).
#
#   ./scripts/install-chaos-east.sh            # dry run
#   sudo ./scripts/install-chaos-east.sh --apply
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
POLKIT_DST=/etc/polkit-1/rules.d/60-unheaded-chaos.rules
SYSUSERS_DST=/etc/sysusers.d/unheaded.conf

ALLOWLIST_SRC="$REPO_ROOT/configs/chaos/allowlist.yaml"
POLICY_SRC="$REPO_ROOT/observability/alerting/policy.yaml"
UNIT_SRC="$REPO_ROOT/deploy/systemd/unheaded-chaos-controller.service"
DROPIN_SRC="$REPO_ROOT/deploy/systemd/unheaded-chaos-controller.service.d/east.conf"
SYSUSERS_SRC="$REPO_ROOT/deploy/sysusers.d/unheaded.conf"

say() { printf '  %s\n' "$*"; }

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

host="$(hostname)"
if [ "$host" != "east" ]; then
	say "WARNING: hostname is '$host', not 'east'. The systemd injector refuses to run"
	say "         anywhere but east, so this install would be inert here."
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

echo "== systemd unit and east drop-in (no capabilities) =="
run install -m 0644 "$UNIT_SRC" "$UNIT_DST"
run install -d -m 0755 "$DROPIN_DIR"
run install -m 0644 "$DROPIN_SRC" "$DROPIN_DIR/east.conf"

echo "== polkit rule (rendered from the allowlist) =="
if [ "$APPLY" -eq 1 ]; then
	install -d -m 0755 /etc/polkit-1/rules.d
	CHAOS_ALLOWLIST="$ETC_DIR/allowlist.yaml" "$BIN_DST" render-polkit >"$POLKIT_DST"
	chmod 0644 "$POLKIT_DST"
	echo "wrote $POLKIT_DST"
else
	printf 'DRY  %s render-polkit > %s\n' "$BIN_DST" "$POLKIT_DST"
	echo "--- rule that would be written ---"
	CHAOS_ALLOWLIST="$ALLOWLIST_SRC" go run "$REPO_ROOT/cmd/chaos-controller" render-polkit 2>/dev/null || true
	echo "--- end rule ---"
fi

cat <<EOF

NEXT (by hand — this script never writes secrets):
  Create ${ETC_DIR}/chaos.env (0640 root:unheaded-chaos) with:
    AUTH_ENABLED=true
    AUTH_API_KEYS=<comma-separated keys>
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
