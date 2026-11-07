#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# rolling-previous.sh: what runbooks/deploy/rolling-upgrade.yaml puts back.
#
#   rolling-previous.sh record KEEP APT_BASE PKG...   # before anything moves
#   rolling-previous.sh restore west|east KEEP        # the rollback
#
# record writes what each host holds of PKG to KEEP/<host>.versions and keeps the
# deb of every version in KEEP/debs, from the repo's pool or an earlier build in
# dist/debs (paths are from the checkout, which is the working directory). The
# repo holds one version of a package, so a publish drops the deb a rollback
# needs: record runs first. A version with no deb stops the run with nothing
# moved. A KEEP already there stays: on a retry a host may have moved.
#
# restore installs that host's debs, downgrades allowed, and fails unless the
# host then holds exactly what was recorded. It does not touch the repo, which
# still serves the failed build, nor the Well.
#
# Tested by tests/runbooks/test_rolling_rollback.py.
set -euo pipefail

EAST=govan@east

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
# version, going by its control file and not by its name.
find_deb() {
	local candidate
	while IFS= read -r candidate; do
		if [[ "$(dpkg-deb -f "$candidate" Package Version 2>/dev/null)" == "Package: $2"$'\n'"Version: $3" ]]; then
			echo "$candidate"
			return
		fi
	done < <(find "$1/pool" dist/debs -type f -name "${2}_${3#*:}_*.deb" 2>/dev/null || true)
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
	for host in west east; do
		held_on "$host" "$@" >"$part/$host.versions"
		while read -r pkg version; do
			deb="$(find_deb "$base" "$pkg" "$version")"
			if [[ -n "$deb" ]]; then
				cp -f -- "$deb" "$part/debs/"
			else
				missing+=("$host:$pkg=$version")
			fi
		done <"$part/$host.versions"
	done
	if [[ ${#missing[@]} -gt 0 ]]; then
		rm -rf "$part"
		die "no deb to put these back: ${missing[*]}; nothing moved"
	fi
	(cd "$part/debs" && find . -type f -name '*.deb' -printf '%P\0' | xargs -0 -r sha256sum --) >"$part/SHA256SUMS"
	mv "$part" "$keep"
	echo "rolling-previous: recorded west and east in $keep"
}

restore() {
	local host=$1 keep=$2 pkg version
	[[ "$host" == west || "$host" == east ]] || die "restore: west or east, not '$host'"
	[[ -f "$keep/$host.versions" ]] || die "no record for $host in $keep"
	local debs=() pkgs=() found
	while read -r pkg version; do
		found=("$PWD/$keep/debs/${pkg}_${version#*:}_"*.deb)
		[[ -e "${found[0]}" ]] || die "no kept deb for $pkg $version"
		debs+=("${found[0]}")
		pkgs+=("$pkg")
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

	local now
	now="$(held_on "$host" "${pkgs[@]}" | LC_ALL=C sort)"
	if [[ "$now" != "$(LC_ALL=C sort "$keep/$host.versions")" ]]; then
		die "$host did not get back to its record; it holds: ${now//$'\n'/, }"
	fi
	echo "rolling-previous: $host is back at what it held"
}

case "${1:-}" in
record)
	[[ $# -ge 4 ]] || die "usage: record KEEP APT_BASE PKG..."
	shift
	record "$@"
	;;
restore)
	[[ $# -eq 3 ]] || die "usage: restore west|east KEEP"
	shift
	restore "$@"
	;;
*) die "usage: record KEEP APT_BASE PKG... | restore west|east KEEP" ;;
esac
