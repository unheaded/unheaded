#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# build-binaries.sh — build the service binaries the deploy/systemd units run
# (ADR-085 Phase 1), from one map.
#
#   scripts/build-binaries.sh --out bin/release            # build every one
#   scripts/build-binaries.sh --out bin/release akira wotan
#   scripts/build-binaries.sh --list                       # name and package
#   scripts/build-binaries.sh --check-units                # gate, builds nothing
#
# Any build failure is an error (the .deb builder used to print SKIP and
# package what was left). --out must be a directory the caller owns; nothing
# is written to a fixed /tmp path. Installing is a separate, root-only copy:
#   sudo install -m 0755 bin/release/* /usr/local/bin/    (make install)
#
# --check-units fails when a unit runs /usr/local/bin/<x> that is neither in
# the map nor a named third-party binary, so a unit cannot point at a binary
# nothing builds.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UNITS_DIR="${UNITS_DIR:-${REPO_ROOT}/deploy/systemd}"
cd "$REPO_ROOT" || exit 2

# binary            Go package
MAP="
wotan               ./services/wotan/cmd/wotan
unheaded-daemon     ./cmd/unheaded-daemon
dashboard-backend   ./cmd/dashboard-backend
kanban-app          ./cmd/kanban-app
akira               ./cmd/akira
timeguru            ./services/timeguru/cmd/timeguru
captain             ./services/captain/cmd/captain
architect           ./services/architect/cmd/architect
micromanager        ./services/micromanager/cmd/micromanager
monad               ./cmd/monad
sophia              ./cmd/sophia
huginn              ./cmd/huginn
trace-collector-go  ./cmd/trace-collector-go
kev-poller          ./cmd/kev-poller
chaos-controller    ./cmd/chaos-controller
sword               ./cmd/sword
unheaded-baseline   ./cmd/unheaded-baseline
zhen-agentd         ./cmd/zhen-agentd
anomaly             ./cmd/anomaly
unheaded-keygen     ./cmd/unheaded-keygen
unheaded-auth       ./cmd/unheaded-auth
"
# Run by a unit but not built here: an upstream release binary.
THIRD_PARTY="node_exporter"

pkg_of() { awk -v n="$1" 'NF == 2 && $1 == n { print $2 }' <<<"$MAP"; }
names() { awk 'NF == 2 { print $1 }' <<<"$MAP"; }

check_units() {
	local unit bin rc=0 n=0 name pkg dir
	# A binary is named after the directory it is built from (ADR-084): the
	# live-path inventory (scripts/live-path-inventory.sh) and every reader
	# find a unit's source by that name. huginn built from cmd/host-agent (until 2026-10-25) made
	# a supervised service on both hosts read as an unsupervised TOOL.
	while read -r name pkg; do
		[[ -n "$name" ]] || continue
		dir="$(basename "$pkg")"
		[[ "$dir" == cmd ]] && dir="$(basename "$(dirname "$pkg")")"
		if [[ "$dir" != "$name" ]]; then
			echo "[FAIL] binary $name is built from $pkg; name the directory after the binary" >&2
			rc=1
		fi
	done <<<"$MAP"
	shopt -s nullglob
	for unit in "$UNITS_DIR"/*.service "$UNITS_DIR"/user/*.service; do
		while read -r bin; do
			n=$((n + 1))
			[[ -n "$(pkg_of "$bin")" ]] && continue
			[[ " $THIRD_PARTY " == *" $bin "* ]] && continue
			echo "[FAIL] ${unit#"$REPO_ROOT"/} runs /usr/local/bin/$bin, which scripts/build-binaries.sh does not build" >&2
			rc=1
		done < <(sed -n 's|^ExecStart=-\{0,1\}/usr/local/bin/\([A-Za-z0-9._-]*\).*|\1|p' "$unit")
	done
	if [[ $n -eq 0 ]]; then
		echo "[FAIL] no ExecStart=/usr/local/bin/... found under ${UNITS_DIR#"$REPO_ROOT"/}; refusing to pass vacuously" >&2
		return 1
	fi
	[[ $rc -eq 0 ]] && echo "[PASS] ${n} unit ExecStart binaries, each built here or named third-party"
	return "$rc"
}

out="" ; want=()
while [[ $# -gt 0 ]]; do
	case "$1" in
	--list) while read -r n; do printf '%-20s %s\n' "$n" "$(pkg_of "$n")"; done < <(names); exit 0 ;;
	--check-units) check_units; exit $? ;;
	--out) out="${2:-}"; shift 2 ;;
	-h|--help) sed -n '4,20p' "$0"; exit 0 ;;
	-*) echo "unknown flag $1" >&2; exit 2 ;;
	*) want+=("$1"); shift ;;
	esac
done
[[ -n "$out" ]] || { echo "usage: $0 --out DIR [binary...] | --list | --check-units" >&2; exit 2; }
[[ ${#want[@]} -gt 0 ]] || mapfile -t want < <(names)

mkdir -p "$out" || exit 1
if [[ -L "$out" || ! -O "$out" ]]; then
	echo "[FAIL] $out is a symlink or not owned by $(id -un)" >&2
	exit 1
fi

failed=0
for n in "${want[@]}"; do
	pkg="$(pkg_of "$n")"
	[[ -n "$pkg" ]] || { echo "[FAIL] no package for '$n' (see --list)" >&2; failed=1; continue; }
	if CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$out/$n" "$pkg"; then
		echo "  built $n"
	else
		echo "[FAIL] go build $pkg" >&2
		failed=1
	fi
done
[[ $failed -eq 0 ]] || exit 1
echo "[OK] ${#want[@]} binaries in $out"
