#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# rolling-previous.sh: what runbooks/deploy/rolling-upgrade.yaml puts back.
#
#   rolling-previous.sh record KEEP APT_BASE PKG...   # before anything moves
#   rolling-previous.sh health west|east KEEP         # after a host moves
#   rolling-previous.sh restore west|east KEEP        # the rollback
#
# record writes what each host holds of PKG to KEEP/<host>.versions and keeps the
# deb of every version in KEEP/debs, from the repo's pool or an earlier build in
# dist/debs (paths are from the checkout, which is the working directory). The
# repo holds one version of a package, so a publish drops the deb a rollback
# needs: record runs first. A version with no deb stops the run with nothing
# moved. record also writes what a health sweep finds up on each host to
# KEEP/<host>.healthy, and keeps west's sweep tool, built here so that no later
# step waits on a compile. A host where the sweep finds nothing up stops the run
# with nothing moved: nothing could then be told from a good build. A KEEP already
# there stays: on a retry a host may have moved.
#
# health fails unless everything in KEEP/<host>.healthy is up again within
# ROLLING_SETTLE seconds (60), as restarted services take a moment. It reads what
# the sweep prints: akira --once exits 0 however many services failed, and a host
# may be restored to a build older than any change to that. The record is the
# bar, not every target: a target already down is not this build's doing, and
# west's built-in target list names services it does not run.
#
# restore installs that host's debs, downgrades allowed, removes any of PKG it
# did not hold, and fails unless the host then holds exactly what was recorded
# and passes health. It does not touch the repo, which still serves the failed
# build, nor the Well.
#
# Tested by tests/runbooks/test_rolling_rollback.py.
set -euo pipefail

EAST=govan@east
EAST_SWEEP='/opt/unheaded/bin/akira --once --config /opt/unheaded/etc/akira.yaml'
WEST_CONFIG=configs/akira-west.yaml
PAUSE=5

die() { echo "rolling-previous: $*" >&2; exit 1; }

# held_on HOST PKG...: "<package> <version>" for each it holds. A host that
# cannot be read is a failure, not an empty list.
held_on() {
	local host=$1
	shift
	if [[ "$host" == west ]]; then
		dpkg --version >/dev/null
		{ dpkg -l "$@" 2>/dev/null || true; }
	else
		# shellcheck disable=SC2029 # package names from the runbook, meant for that shell
		ssh "$EAST" "dpkg --version >/dev/null && { dpkg -l $* 2>/dev/null || true; }"
	fi | awk '$1 ~ /^[uihrp][iUFHWt]/ { sub(/:.*/, "", $2); print $2, $3 }'
}

# find_deb APT_BASE PKG VERSION: a deb that really is that package at that
# version, going by its control file and not by its name, which in an earlier
# build under dist/debs may lack the architecture.
find_deb() {
	local candidate
	while IFS= read -r candidate; do
		if [[ "$(dpkg-deb -f "$candidate" Package Version 2>/dev/null)" == "Package: $2"$'\n'"Version: $3" ]]; then
			echo "$candidate"
			return
		fi
	done < <(find "$1/pool" dist/debs -type f -name "${2}_${3#*:}*.deb" 2>/dev/null || true)
}

# sweep HOST DIR: the services a finished sweep found up, sorted. DIR holds west's
# sweep tool. A sweep that did not run or did not finish is a failure, not an
# empty list.
sweep() {
	local host=$1 dir=$2 out rc=0
	if [[ "$host" == west ]]; then
		out="$("$dir/akira" --once --config "$WEST_CONFIG" 2>&1)" || rc=$?
	else
		# shellcheck disable=SC2029 # the sweep is meant for that shell
		out="$(ssh "$EAST" "$EAST_SWEEP 2>&1")" || rc=$?
	fi
	if ((rc != 0)) || ! grep -Eq '^[0-9]+/[0-9]+ healthy$' <<<"$out"; then
		tail -5 <<<"$out" >&2
		echo "rolling-previous: the sweep on $host did not finish (exit $rc)" >&2
		return 1
	fi
	awk '$1 == "[OK]" { print $2 }' <<<"$out" | LC_ALL=C sort
}

health() {
	local host=$1 keep=$2 now lost give_up=$((SECONDS + ${ROLLING_SETTLE:-60}))
	[[ "$host" == west || "$host" == east ]] || die "health: west or east, not '$host'"
	[[ -f "$keep/$host.healthy" ]] || die "no health record for $host in $keep"
	while :; do
		if now="$(sweep "$host" "$keep")"; then
			lost="$(grep -vxF -f <(printf '%s\n' "$now") "$keep/$host.healthy" || true)"
			if [[ -z "$lost" ]]; then
				echo "rolling-previous: $host has everything it had up: ${now//$'\n'/, }"
				return
			fi
		else
			lost="(no finished sweep)"
		fi
		((SECONDS < give_up)) || break
		sleep "$PAUSE"
	done
	die "$host lost what the record had up: ${lost//$'\n'/, }"
}

record() {
	local keep=$1 base=$2 host pkg version deb
	shift 2
	if [[ -d "$keep" ]]; then
		echo "rolling-previous: keeping the record in $keep"
		return
	fi
	local part="$keep.part" missing=()
	rm -rf "$part"
	mkdir -p "$part/debs"
	printf '%s\n' "$@" >"$part/packages"
	for host in west east; do
		held_on "$host" "$@" >"$part/$host.versions"
		while read -r pkg version; do
			deb="$(find_deb "$base" "$pkg" "$version")"
			if [[ -n "$deb" ]]; then
				cp -f -- "$deb" "$part/debs/${pkg}_${version#*:}_$(dpkg-deb -f "$deb" Architecture).deb"
			else
				missing+=("$host:$pkg=$version")
			fi
		done <"$part/$host.versions"
	done
	if [[ ${#missing[@]} -gt 0 ]]; then
		rm -rf "$part"
		die "no deb to put these back: ${missing[*]}; nothing moved"
	fi
	go build -o "$part/akira" ./cmd/akira || { rm -rf "$part"; die "the sweep tool did not build; nothing moved"; }
	for host in west east; do
		sweep "$host" "$part" >"$part/$host.healthy" || { rm -rf "$part"; die "no sweep of $host; nothing moved"; }
		[[ -s "$part/$host.healthy" ]] || { rm -rf "$part"; die "the sweep finds nothing up on $host; nothing moved"; }
	done
	(cd "$part/debs" && find . -type f -name '*.deb' -printf '%P\0' | xargs -0 -r sha256sum --) >"$part/SHA256SUMS"
	mv "$part" "$keep"
	echo "rolling-previous: recorded west and east in $keep"
}

restore() {
	local host=$1 keep=$2 pkg version
	[[ "$host" == west || "$host" == east ]] || die "restore: west or east, not '$host'"
	[[ -f "$keep/$host.versions" ]] || die "no record for $host in $keep"
	[[ -f "$keep/packages" ]] || die "no package list in $keep"
	local debs=() pkgs=() found
	mapfile -t pkgs <"$keep/packages"
	while read -r pkg version; do
		found=("$PWD/$keep/debs/${pkg}_${version#*:}_"*.deb)
		[[ -e "${found[0]}" ]] || die "no kept deb for $pkg $version"
		debs+=("${found[0]}")
	done <"$keep/$host.versions"
	if [[ ${#debs[@]} -eq 0 ]]; then
		echo "rolling-previous: $host held none of these; nothing to put back"
		return
	fi
	(cd "$keep/debs" && sha256sum -c --quiet ../SHA256SUMS) || die "a kept deb in $keep changed: nothing installed"

	if [[ "$host" == west ]]; then
		sudo apt-get install -y --allow-downgrades "${debs[@]}" 2>&1 | tail -10
	else
		local dir rc=0
		dir="$(ssh "$EAST" mktemp -d /var/tmp/unheaded-rollback.XXXXXXXX)"
		# shellcheck disable=SC2029 # $dir is this host's answer, meant for that shell
		{ scp -q "${debs[@]}" "$EAST:$dir" &&
			ssh "$EAST" "set -o pipefail; sudo apt-get install -y --allow-downgrades $dir/*.deb 2>&1 | tail -10"; } || rc=$?
		# shellcheck disable=SC2029
		ssh "$EAST" "rm -rf $dir"
		((rc == 0)) || die "apt-get on east exited $rc"
	fi

	# What the upgrade brought in that the host did not hold, unheaded-keygen
	# above all. dpkg, as apt-get remove would also take whatever depends on it.
	local added=()
	mapfile -t added < <(held_on "$host" "${pkgs[@]}" | awk '{print $1}' |
		grep -vxF -f <(awk '{print $1}' "$keep/$host.versions") || true)
	if [[ ${#added[@]} -gt 0 ]]; then
		if [[ "$host" == west ]]; then
			sudo dpkg --remove "${added[@]}" 2>&1 | tail -10 || die "could not remove ${added[*]}"
		else
			# shellcheck disable=SC2029 # package names from the record, meant for that shell
			ssh "$EAST" "set -o pipefail; sudo dpkg --remove ${added[*]} 2>&1 | tail -10" ||
				die "could not remove ${added[*]}"
		fi
	fi

	local now
	now="$(held_on "$host" "${pkgs[@]}" | LC_ALL=C sort)"
	if [[ "$now" != "$(LC_ALL=C sort "$keep/$host.versions")" ]]; then
		die "$host did not get back to its record; it holds: ${now//$'\n'/, }"
	fi
	echo "rolling-previous: $host is back at what it held"
	health "$host" "$keep"
}

case "${1:-}" in
record)
	[[ $# -ge 4 ]] || die "usage: record KEEP APT_BASE PKG..."
	shift
	record "$@"
	;;
health)
	[[ $# -eq 3 ]] || die "usage: health west|east KEEP"
	shift
	health "$@"
	;;
restore)
	[[ $# -eq 3 ]] || die "usage: restore west|east KEEP"
	shift
	restore "$@"
	;;
*) die "usage: record KEEP APT_BASE PKG... | health west|east KEEP | restore west|east KEEP" ;;
esac
