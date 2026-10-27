#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# scripts/lib/deb-maintscripts.sh: run the generated postinst and prerm the
# way dpkg calls them, against a fake systemctl/systemd-sysusers that only
# record their arguments. The contract: an upgrade never stops or disables
# the service (postinst restarts it on the new binary); removal stops and
# disables it.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/deb-maintscripts.sh
. "$REPO_ROOT/scripts/lib/deb-maintscripts.sh"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir "$T/bin"
for tool in systemctl systemd-sysusers unheaded-keygen; do
	printf '#!/bin/sh\necho "%s $*" >>"%s/calls"\n' "$tool" "$T" >"$T/bin/$tool"
	chmod +x "$T/bin/$tool"
done
mkdir "$T/keys"
# The generated postinst names its keygen and key directory on two lines of
# its own; point them at the fake tool and a temp dir.
fake_paths() {
	sed -e "s|^KEYGEN=.*|KEYGEN=$T/bin/unheaded-keygen|" -e "s|^KEYDIR=.*|KEYDIR=$T/keys|"
}
deb_postinst unheaded-wotan wotan unheaded-wotan | fake_paths >"$T/postinst"
deb_postinst unheaded-wotan | fake_paths >"$T/postinst-nokey"
deb_prerm unheaded-wotan >"$T/prerm"
deb_prerm unheaded-well-retention unheaded-well-retention.timer unheaded-well-retention.service >"$T/prerm-timer"
chmod +x "$T/postinst" "$T/postinst-nokey" "$T/prerm" "$T/prerm-timer"

fail=0
run() { # run SCRIPT ARGS...: the calls it made, one per line
	: >"$T/calls"
	PATH="$T/bin:$PATH" "$T/$1" "${@:2}"
	cat "$T/calls"
}
expect() { # expect DESC GOT WANT
	if [[ "$2" != "$3" ]]; then
		printf '[FAIL] %s\n  got:  %s\n  want: %s\n' "$1" "${2//$'\n'/ | }" "${3//$'\n'/ | }"
		fail=1
	else
		printf '[ok] %s\n' "$1"
	fi
}

expect "prerm upgrade touches nothing" "$(run prerm upgrade 1.0.1)" ""
expect "prerm failed-upgrade touches nothing" "$(run prerm failed-upgrade 1.0.0)" ""
expect "prerm remove stops and disables" "$(run prerm remove)" \
	"systemctl stop unheaded-wotan.service
systemctl disable unheaded-wotan.service"
expect "prerm of a timer package stops and disables the timer, then the service" "$(run prerm-timer remove)" \
	"systemctl stop unheaded-well-retention.timer
systemctl disable unheaded-well-retention.timer
systemctl stop unheaded-well-retention.service
systemctl disable unheaded-well-retention.service"
expect "prerm of a timer package: upgrade touches nothing" "$(run prerm-timer upgrade 1.0.1)" ""
expect "postinst configure: account, key (absent), reload, try-restart" "$(run postinst configure 1.0.0)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-wotan.conf
unheaded-keygen -owner unheaded-wotan wotan
systemctl daemon-reload
systemctl try-restart unheaded-wotan.service"
: >"$T/keys/wotan.key"
expect "postinst configure never touches an existing key" "$(run postinst configure 1.0.1)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-wotan.conf
systemctl daemon-reload
systemctl try-restart unheaded-wotan.service"
rm "$T/keys/wotan.key"
expect "postinst without an identity makes no key" "$(run postinst-nokey configure 1.0.0)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-wotan.conf
systemctl daemon-reload
systemctl try-restart unheaded-wotan.service"
expect "postinst abort-upgrade touches nothing" "$(run postinst abort-upgrade 1.0.1)" ""
expect "unpatched postinst runs the packaged keygen" \
	"$(deb_postinst unheaded-wotan wotan unheaded-wotan | grep -c '^KEYGEN=/opt/unheaded/bin/unheaded-keygen$')" "1"

# build-all-debs.sh passes each service's identity and account, and every
# service package depends on the one package that ships unheaded-keygen.
expect "build-all-debs passes identity and account to deb_postinst" \
	"$(grep -c 'deb_postinst "\$PKG_NAME" "\$svc" "\$SVC_USER"' "$REPO_ROOT/scripts/build-all-debs.sh")" "1"
expect "service packages depend on unheaded-keygen" \
	"$(grep -c '^Depends: libc6, unheaded-keygen (= \${VERSION})$' "$REPO_ROOT/scripts/build-all-debs.sh")" "1"

# debian/unheaded-wotan.postinst (the dh tree) is the generated script, not a
# second hand-written one: no shared "unheaded" account (deploy/sysusers.d
# gives every service its own), no unit written inline.
DH="$REPO_ROOT/debian/unheaded-wotan.postinst"
expect "debian postinst is the generated one" \
	"$(sed -n '1,/^esac$/p' "$DH")" "$(deb_postinst unheaded-wotan wotan unheaded-wotan)"
expect "debian postinst creates no shared account" \
	"$(grep -cE 'useradd|adduser|User=unheaded$|id -u unheaded( |$)' "$DH")" "0"

# The wotan package ships the topic config its unit reads, as a conffile.
mkdir -p "$T/pkg/DEBIAN" "$T/other/DEBIAN"
deb_conffiles wotan "$T/pkg" "$REPO_ROOT"
expect "wotan ships /etc/unheaded/wotan.yaml as a conffile" "$(cat "$T/pkg/DEBIAN/conffiles")" "/etc/unheaded/wotan.yaml"
expect "wotan.yaml is the repo's configs/wotan.yaml" "$(cmp -s "$REPO_ROOT/configs/wotan.yaml" "$T/pkg/etc/unheaded/wotan.yaml" && echo same)" "same"
expect "unit reads that path" "$(grep -c -- '--topic-config /etc/unheaded/wotan.yaml' "$REPO_ROOT/deploy/systemd/unheaded-wotan.service")" "1"
deb_conffiles timeguru "$T/other" "$REPO_ROOT"
expect "other services ship no conffiles" "$(ls "$T/other/DEBIAN")" ""

exit "$fail"
