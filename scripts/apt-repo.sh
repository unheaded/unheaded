#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# apt-repo.sh — the signed apt repository on west (ADR-026, S-IRON-GATE
# Phase 14). reprepro keeps the repo; nginx serves dists/, pool/ and the
# public key on 18888 as the `nginx` account (deploy/nginx/unheaded-apt.conf).
# The repo's signed Release (InRelease / Release.gpg) is the trust anchor:
# clients pin the key with signed-by, never apt-key or [trusted=yes].
#
#   apt-repo.sh init --keyid <KEYID>     # conf/distributions, public key export
#   apt-repo.sh include <deb>...         # add packages (re-signs Release)
#   apt-repo.sh list                     # what the repo holds
#   apt-repo.sh check                    # reprepro check/checkpool + Release signature
#   apt-repo.sh serve                    # print the nginx server block
#
# Base directory: -b DIR or APT_REPO_BASE (default /srv/unheaded-apt).
# Signing uses the GPG key in $GNUPGHOME (E4: which key, held where, is
# Stevie's decision; nothing here creates one). Runbook:
# runbooks/deploy/apt-repo-publish.yaml. Test: tests/apt/apt_repo_test.sh.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASE="${APT_REPO_BASE:-/srv/unheaded-apt}"
CODENAME=unheaded
KEY_FILE=unheaded-apt.gpg

die() { echo "apt-repo: $*" >&2; exit 1; }
usage() { sed -n '4,22p' "$0" | sed 's/^# \{0,1\}//'; exit 2; }

[[ "${1:-}" == -b ]] && { BASE="${2:?-b needs a directory}"; shift 2; }
cmd="${1:-}"
[[ -n "$cmd" ]] || usage
shift

need_reprepro() { command -v reprepro >/dev/null || die "reprepro is not installed (apt-get install reprepro)"; }
need_init() { [[ -f "$BASE/conf/distributions" ]] || die "$BASE is not initialised (apt-repo.sh init --keyid <KEYID>)"; }
signwith() { sed -n 's/^SignWith: //p' "$BASE/conf/distributions"; }

case "$cmd" in
init)
	keyid=""
	while [[ $# -gt 0 ]]; do
		case "$1" in
		--keyid) keyid="${2:-}"; shift 2 ;;
		*) die "init: unknown argument $1" ;;
		esac
	done
	[[ -n "$keyid" ]] || die "init: --keyid is required (the repo is never unsigned)"
	[[ "$keyid" =~ ^[0-9A-Fa-f]{16,40}$ ]] || die "init: --keyid must be a key id or fingerprint (16-40 hex), got '$keyid'"
	gpg --batch --list-secret-keys "$keyid" >/dev/null 2>&1 || die "init: no secret key $keyid in ${GNUPGHOME:-~/.gnupg}"
	if [[ -f "$BASE/conf/distributions" ]]; then
		[[ "$(signwith)" == "$keyid" ]] || die "init: $BASE already signs with $(signwith); refusing to switch keys here (rotation is a runbook step)"
	else
		install -d -m 0755 "$BASE" "$BASE/conf"
		install -d -m 0750 "$BASE/incoming"
		cat >"$BASE/conf/distributions" <<EOF
Origin: Unheaded
Label: Unheaded
Codename: $CODENAME
Suite: stable
Architectures: amd64
Components: main
Description: Unheaded packages (scripts/build-all-debs.sh)
SignWith: $keyid
EOF
		cat >"$BASE/conf/options" <<EOF
basedir $BASE
EOF
	fi
	# Binary keyring: what apt's signed-by= takes as is.
	gpg --batch --yes --export "$keyid" >"$BASE/$KEY_FILE.tmp"
	[[ -s "$BASE/$KEY_FILE.tmp" ]] || die "init: exporting $keyid gave nothing"
	chmod 0644 "$BASE/$KEY_FILE.tmp"
	mv "$BASE/$KEY_FILE.tmp" "$BASE/$KEY_FILE"
	echo "apt-repo: $BASE initialised, Codename $CODENAME, signed with $keyid; public key $BASE/$KEY_FILE"
	;;
include)
	need_reprepro
	need_init
	[[ $# -gt 0 ]] || die "include: no .deb given"
	for deb in "$@"; do
		[[ -f "$deb" && "$deb" == *.deb ]] || die "include: $deb is not a .deb file"
		dpkg-deb --info "$deb" >/dev/null 2>&1 || die "include: $deb is not a valid package"
	done
	reprepro -b "$BASE" includedeb "$CODENAME" "$@"
	[[ -s "$BASE/dists/$CODENAME/InRelease" ]] || die "include: no InRelease written; is the SignWith key usable?"
	echo "apt-repo: included $# package(s); Release re-signed"
	;;
list)
	need_reprepro
	need_init
	reprepro -b "$BASE" list "$CODENAME"
	;;
check)
	need_reprepro
	need_init
	reprepro -b "$BASE" check "$CODENAME"
	reprepro -b "$BASE" checkpool fast
	gpg --batch --no-default-keyring --keyring "$BASE/$KEY_FILE" \
		--verify "$BASE/dists/$CODENAME/Release.gpg" "$BASE/dists/$CODENAME/Release" 2>/dev/null ||
		die "check: Release.gpg does not verify against $BASE/$KEY_FILE"
	gpg --batch --no-default-keyring --keyring "$BASE/$KEY_FILE" \
		--verify "$BASE/dists/$CODENAME/InRelease" 2>/dev/null ||
		die "check: InRelease does not verify against $BASE/$KEY_FILE"
	echo "apt-repo: $BASE consistent; Release and InRelease signed by the exported key"
	;;
serve)
	sed "s|/srv/unheaded-apt|$BASE|g" "$REPO_ROOT/deploy/nginx/unheaded-apt.conf"
	;;
-h | --help | help) usage ;;
*) die "unknown command '$cmd' (init, include, list, check, serve)" ;;
esac
