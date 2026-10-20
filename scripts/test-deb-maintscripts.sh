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
for tool in systemctl systemd-sysusers; do
	printf '#!/bin/sh\necho "%s $*" >>"%s/calls"\n' "$tool" "$T" >"$T/bin/$tool"
	chmod +x "$T/bin/$tool"
done
deb_postinst unheaded-wotan >"$T/postinst"
deb_prerm unheaded-wotan >"$T/prerm"
chmod +x "$T/postinst" "$T/prerm"

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
expect "postinst configure: account, reload, try-restart" "$(run postinst configure 1.0.0)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-wotan.conf
systemctl daemon-reload
systemctl try-restart unheaded-wotan.service"
expect "postinst abort-upgrade touches nothing" "$(run postinst abort-upgrade 1.0.1)" ""

# The wotan package ships the topic config its unit reads, as a conffile.
mkdir -p "$T/pkg/DEBIAN" "$T/other/DEBIAN"
deb_conffiles wotan "$T/pkg" "$REPO_ROOT"
expect "wotan ships /etc/unheaded/wotan.yaml as a conffile" "$(cat "$T/pkg/DEBIAN/conffiles")" "/etc/unheaded/wotan.yaml"
expect "wotan.yaml is the repo's configs/wotan.yaml" "$(cmp -s "$REPO_ROOT/configs/wotan.yaml" "$T/pkg/etc/unheaded/wotan.yaml" && echo same)" "same"
expect "unit reads that path" "$(grep -c -- '--topic-config /etc/unheaded/wotan.yaml' "$REPO_ROOT/deploy/systemd/unheaded-wotan.service")" "1"
deb_conffiles timeguru "$T/other" "$REPO_ROOT"
expect "other services ship no conffiles" "$(ls "$T/other/DEBIAN")" ""

exit "$fail"
