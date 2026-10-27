#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# scripts/apt-repo.sh against a temp directory with a throwaway GPG key (a
# temp GNUPGHOME; never the user's keyring): init writes a signing
# distribution and exports the public key, include signs Release, check
# verifies it, apt itself accepts the repo with signed-by= and refuses it
# with any other key, and the refusals hold (no key id, no init, not a deb,
# a key switch, a tampered Release). Also the nginx block: only dists/,
# pool/ and the key are served. Needs reprepro, gpg, dpkg-deb; apt-get for
# the client half (skipped without it).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
APT_REPO="$REPO_ROOT/scripts/apt-repo.sh"
command -v reprepro >/dev/null || { echo "[FAIL] reprepro not installed"; exit 1; }

T="$(mktemp -d)"
export GNUPGHOME="$T/gnupg"
cleanup() { gpgconf --kill all 2>/dev/null || true; rm -rf "$T"; }
trap cleanup EXIT
install -d -m 0700 "$GNUPGHOME"
BASE="$T/repo"
export APT_REPO_BASE="$BASE"

fail=0
expect() { # expect DESC GOT WANT
	if [[ "$2" != "$3" ]]; then
		printf '[FAIL] %s\n  got:  %s\n  want: %s\n' "$1" "${2//$'\n'/ | }" "${3//$'\n'/ | }"
		fail=1
	else
		printf '[ok] %s\n' "$1"
	fi
}
rc() { local r=0; "$@" >"$T/out" 2>&1 || r=$?; echo "$r"; }

newkey() { # newkey UID: a throwaway ed25519 signing key, prints its fingerprint
	gpg --batch --passphrase '' --quick-gen-key "$1" ed25519 sign never >/dev/null 2>&1
	gpg --batch --with-colons --list-secret-keys "$1" 2>/dev/null | awk -F: '$1 == "fpr" { print $10; exit }'
}
KEY="$(newkey 'apt-repo-test <apt-repo-test@invalid>')"
OTHER="$(newkey 'apt-repo-other <apt-repo-other@invalid>')"
[[ -n "$KEY" && -n "$OTHER" ]] || { echo "[FAIL] could not create the throwaway keys"; exit 1; }

# A minimal package to publish.
mkdir -p "$T/pkg/DEBIAN" "$T/pkg/opt/unheaded/bin"
printf '#!/bin/sh\necho fixture\n' >"$T/pkg/opt/unheaded/bin/fixture"
chmod 0755 "$T/pkg/opt/unheaded/bin/fixture"
cat >"$T/pkg/DEBIAN/control" <<EOF
Package: unheaded-fixture
Version: 0.2.0~alpha+test
Architecture: amd64
Maintainer: Stevie Bellis <stevie@bellis.tech>
Section: admin
Priority: optional
Description: apt-repo.sh test fixture
 Not a real package.
EOF
dpkg-deb --root-owner-group --build "$T/pkg" "$T/unheaded-fixture_0.2.0~alpha+test_amd64.deb" >/dev/null
DEB="$T/unheaded-fixture_0.2.0~alpha+test_amd64.deb"
printf 'not a package\n' >"$T/bogus.deb"

# Refusals before there is a repo.
expect "init without a key id refuses (the repo is never unsigned)" "$(rc "$APT_REPO" init)" "1"
expect "init with a key that is not in the keyring refuses" "$(rc "$APT_REPO" init --keyid 0123456789ABCDEF)" "1"
expect "include before init refuses" "$(rc "$APT_REPO" include "$DEB")" "1"

expect "init" "$(rc "$APT_REPO" init --keyid "$KEY")" "0"
expect "distributions: Codename unheaded, signed with the key" \
	"$(grep -E '^(Codename|SignWith):' "$BASE/conf/distributions" | tr '\n' ' ')" "Codename: unheaded SignWith: $KEY "
expect "public key exported for signed-by=" \
	"$(gpg --batch --no-default-keyring --keyring "$BASE/unheaded-apt.gpg" --with-colons --list-keys 2>/dev/null | awk -F: '$1 == "fpr" { print $10; exit }')" "$KEY"
expect "init again with the same key is fine" "$(rc "$APT_REPO" init --keyid "$KEY")" "0"
expect "init with another key refuses to switch" "$(rc "$APT_REPO" init --keyid "$OTHER")" "1"
expect "include a file that is not a package refuses" "$(rc "$APT_REPO" include "$T/bogus.deb")" "1"

expect "include" "$(rc "$APT_REPO" include "$DEB")" "0"
expect "Release names the distribution" "$(grep -E '^(Codename|Suite):' "$BASE/dists/unheaded/Release" | tr '\n' ' ')" "Suite: stable Codename: unheaded "
expect "Release.gpg is a PGP signature" "$(head -n 1 "$BASE/dists/unheaded/Release.gpg")" "-----BEGIN PGP SIGNATURE-----"
expect "InRelease is clearsigned" "$(head -n 1 "$BASE/dists/unheaded/InRelease")" "-----BEGIN PGP SIGNED MESSAGE-----"
expect "list shows the package" "$("$APT_REPO" list 2>/dev/null | awk '{ print $2, $3 }')" "unheaded-fixture 0.2.0~alpha+test"
expect "check: consistent and signed" "$(rc "$APT_REPO" check)" "0"

# apt itself: signed-by= with the exported key accepts the repo; another key
# does not. A private apt state under $T, no root, no system sources.
if command -v apt-get >/dev/null; then
	apt_update() { # apt_update KEYRING: exit code of apt-get update against the repo
		local a="$T/apt-$1" r=0
		rm -rf "$a"
		mkdir -p "$a/etc/sources.list.d" "$a/etc/preferences.d" "$a/etc/apt.conf.d" "$a/state/lists/partial" "$a/cache/archives/partial"
		echo "deb [signed-by=$2] file://$BASE unheaded main" >"$a/etc/sources.list"
		APT_CONFIG=/dev/null apt-get -q -o Dir::Etc="$a/etc" -o Dir::Etc::sourcelist="$a/etc/sources.list" \
			-o Dir::Etc::sourceparts="$a/etc/sources.list.d" -o Dir::State="$a/state" -o Dir::Cache="$a/cache" \
			-o Debug::NoLocking=1 -o APT::Architecture=amd64 -o Acquire::Languages=none update >"$a/log" 2>&1 || r=$?
		grep -qE '^(W|E): .*(NO_PUBKEY|not signed|signature|Signed-By|is not signed)' "$a/log" && r=99
		echo "$r"
		[[ "$r" == 0 ]] || return 0
		APT_CONFIG=/dev/null apt-cache -o Dir::Etc="$a/etc" -o Dir::Etc::sourcelist="$a/etc/sources.list" \
			-o Dir::Etc::sourceparts="$a/etc/sources.list.d" -o Dir::State="$a/state" -o Dir::Cache="$a/cache" \
			policy unheaded-fixture >"$a/policy" 2>&1 || true
	}
	expect "apt update with signed-by=<exported key> succeeds" "$(apt_update good "$BASE/unheaded-apt.gpg")" "0"
	expect "apt sees the package as a candidate" \
		"$(awk '/Candidate:/ { print $2 }' "$T/apt-good/policy" 2>/dev/null)" "0.2.0~alpha+test"
	gpg --batch --yes --export "$OTHER" >"$T/other.gpg"
	expect "apt update with any other key is refused" "$(apt_update bad "$T/other.gpg" | grep -cx 0 || true)" "0"
else
	echo "[skip] apt-get not installed: client half not run"
fi

# A Release edited after signing does not verify.
sed -i 's/^Suite: stable$/Suite: tampered/' "$BASE/dists/unheaded/Release"
expect "check: a tampered Release fails" "$(rc "$APT_REPO" check)" "1"

# The nginx block serves dists/, pool/ and the key; everything else 403.
NG="$REPO_ROOT/deploy/nginx/unheaded-apt.conf"
expect "nginx: root /srv/unheaded-apt, autoindex off" \
	"$(grep -cE '^\s*(root /srv/unheaded-apt;|autoindex off;)$' "$NG")" "2"
expect "nginx: default location denies (conf/, db/, incoming/ are 403)" \
	"$(awk '/location \/ \{/ { getline; print; exit }' "$NG" | tr -d ' ')" "denyall;"
expect "nginx: only dists/, pool/ and the key are opened" \
	"$(grep -oE 'location (= )?/[a-z.-]*/?' "$NG" | tr '\n' ' ')" "location / location /dists/ location /pool/ location = /unheaded-apt.gpg "
expect "nginx: no wildcard listen" "$(grep -cE 'listen (18888|0\.0\.0\.0|\[::\])' "$NG" || true)" "0"
expect "serve prints the block for this base" "$("$APT_REPO" serve | grep -c "root $BASE;")" "1"

exit "$fail"
