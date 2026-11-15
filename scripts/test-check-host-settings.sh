#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-host-settings.sh: the host-settings gate (S-WEST-GROUND, ADR-147)
# against a fake root and a fake repository under mktemp -d. Ten cases:
#   1 identical files pass; 2 an exact file one byte off drifts; 3 a contains
#   row missing a line drifts; 4 a redact row that differs only in keys, a
#   password and comments passes; 5 a redact row with other AllowedIPs drifts;
#   6 no key or password ever reaches the output; 7 an absent row whose path
#   exists drifts; 8 an unreadable host file is SKIP (exit 3, or 0 with
#   --allow-skip); 9 a two-field manifest line is exit 2; 10 the repository
#   self-check refuses huge pages and a number in a slice.
# Runtime rows (sysctl, findmnt, /proc/swaps) are not exercised: --root turns
# them off, and they read the live host.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GATE="${REPO_ROOT}/scripts/check-host-settings.sh"
WORK="$(mktemp -d)"
trap 'chmod -R u+rwX "${WORK}" 2>/dev/null; rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0
FAKE_KEY='kTESTpRiVaTeKeYdOnOtPrInT0000000000000000000='
FAKE_PSK='pTESTpReShArEdKeYdOnOtPrInT00000000000000000='
FAKE_PW='TESTbgpPassw0rdDoNotPrint'

ok() { printf '  PASS  %s\n' "$1"; PASSED=$((PASSED + 1)); }
bad() { printf '  FAIL  %s\n' "$1"; FAILED=$((FAILED + 1)); }

# A fresh fake repository and root, with one row per mode.
setup() {
	rm -rf "${WORK}/repo" "${WORK}/root"
	mkdir -p "${WORK}/repo/deploy/host-settings" "${WORK}/repo/deploy/sysctl.d" \
		"${WORK}/repo/deploy/systemd" "${WORK}/repo/config/wireguard" \
		"${WORK}/root/etc/sysctl.d" "${WORK}/root/etc/systemd/system" \
		"${WORK}/root/etc/wireguard" "${WORK}/root/var"
	printf '# SPDX-License-Identifier: GPL-3.0-or-later\nvm.nr_hugepages = 0\nvm.swappiness = 10\n' \
		>"${WORK}/repo/deploy/sysctl.d/99-unheaded-west.conf"
	cp "${WORK}/repo/deploy/sysctl.d/99-unheaded-west.conf" "${WORK}/root/etc/sysctl.d/99-unheaded.conf"
	printf '# SPDX-License-Identifier: GPL-3.0-or-later\n# a comment the host need not hold\n/srv/a /srv/b none bind,nofail 0 0\n/dev/x /srv/x ext4 defaults 0 2\n' \
		>"${WORK}/repo/deploy/host-settings/west-fstab.fragment"
	printf 'UUID=1 / ext4 defaults 0 1\n/srv/a /srv/b none bind,nofail 0 0\n/dev/x /srv/x ext4 defaults 0 2\n' \
		>"${WORK}/root/etc/fstab"
	printf '# SPDX-License-Identifier: GPL-3.0-or-later\n[Slice]\nCPUAccounting=yes\n' \
		>"${WORK}/repo/deploy/systemd/unheaded.slice"
	cp "${WORK}/repo/deploy/systemd/unheaded.slice" "${WORK}/root/etc/systemd/system/unheaded.slice"
	# The repository copy keeps a header and placeholders; the host has keys.
	printf '# SPDX-License-Identifier: GPL-3.0-or-later\n# west\n[Interface]\nAddress = fd00:dead:beef::1/128\nPrivateKey = <WEST_PRIVATE_KEY>\n\n[Peer]\nPublicKey = <east>\nPresharedKey = <psk>\nAllowedIPs = fd00:dead:beef::2/128\n! frr style comment\nneighbor 192.0.2.1 password <BGP_PASSWORD>\n' \
		>"${WORK}/repo/config/wireguard/west.conf"
	printf '[Interface]\nAddress = fd00:dead:beef::1/128\nPrivateKey = %s\n[Peer]\nPublicKey = eastPUBLIC=\nPresharedKey = %s\nAllowedIPs = fd00:dead:beef::2/128\nneighbor 192.0.2.1 password %s\n' \
		"${FAKE_KEY}" "${FAKE_PSK}" "${FAKE_PW}" >"${WORK}/root/etc/wireguard/wg0.conf"
	printf '# SPDX-License-Identifier: GPL-3.0-or-later\n# host_path<TAB>repo_path<TAB>mode\n/etc/sysctl.d/99-unheaded.conf\tdeploy/sysctl.d/99-unheaded-west.conf\texact\n/etc/fstab\tdeploy/host-settings/west-fstab.fragment\tcontains\n/etc/systemd/system/unheaded.slice\tdeploy/systemd/unheaded.slice\texact\n/etc/wireguard/wg0.conf\tconfig/wireguard/west.conf\tredact\n/var/swapfile9\t-\tabsent\n' \
		>"${WORK}/repo/deploy/host-settings/west.tsv"
}

# Runs the gate against the fake tree; sets RC and OUT.
gate() {
	RC=0
	OUT="$("${GATE}" --host west --repo "${WORK}/repo" --root "${WORK}/root" "$@" 2>&1)" || RC=$?
}

# $1 label, $2 wanted exit, $3 a fixed string the output must hold ('' for none).
expect() {
	if [ "${RC}" -ne "$2" ]; then
		bad "$1 (exit ${RC}, wanted $2)"
		printf '%s\n' "${OUT}" | sed 's/^/        /'
		return
	fi
	if [ -n "$3" ] && ! printf '%s\n' "${OUT}" | grep -qF -- "$3"; then
		bad "$1 (no line holding '$3')"
		printf '%s\n' "${OUT}" | sed 's/^/        /'
		return
	fi
	ok "$1"
}

no_secret() {
	if printf '%s\n' "${OUT}" | grep -qF -e "${FAKE_KEY}" -e "${FAKE_PSK}" -e "${FAKE_PW}" -e 'eastPUBLIC'; then
		bad "$1: a key or password reached the output"
	else
		ok "$1: no key or password in the output"
	fi
}

if [ ! -x "${GATE}" ]; then
	echo "FAIL: ${GATE} missing or not executable"
	exit 1
fi

echo "--- 1: identical ---"
setup
gate
expect "1 identical files exit 0" 0 ''
if printf '%s\n' "${OUT}" | grep -v '^OK	' | grep -q .; then
	bad "1 every line starts OK"
	printf '%s\n' "${OUT}" | sed 's/^/        /'
else
	ok "1 every line starts OK"
fi
[ "$(printf '%s\n' "${OUT}" | grep -c '^OK	')" -eq 5 ] && ok "1 one line per row (5)" || bad "1 one line per row (5): $(printf '%s\n' "${OUT}" | grep -c .)"

echo "--- 2: exact, one byte changed ---"
setup
sed -i 's/vm.swappiness = 10/vm.swappiness = 11/' "${WORK}/root/etc/sysctl.d/99-unheaded.conf"
gate
expect "2 one byte drifts" 1 "DRIFT	exact	/etc/sysctl.d/99-unheaded.conf"

echo "--- 3: contains, a line missing ---"
setup
sed -i '/^\/dev\/x /d' "${WORK}/root/etc/fstab"
gate
expect "3 a missing fstab line drifts" 1 "DRIFT	contains	/etc/fstab"
setup
printf '/srv/a /srv/b none bind,nofail 0 0 extra\n' >"${WORK}/root/etc/fstab.part"
sed -i '/^\/srv\/a /d' "${WORK}/root/etc/fstab"
cat "${WORK}/root/etc/fstab.part" >>"${WORK}/root/etc/fstab"
gate
expect "3 a line that only starts the same drifts" 1 "DRIFT	contains	/etc/fstab"

echo "--- 4: redact, keys, password and comments differ ---"
setup
gate
expect "4 redact passes" 0 "OK	redact	/etc/wireguard/wg0.conf"
no_secret "4"

echo "--- 5: redact, AllowedIPs differ ---"
setup
sed -i 's|AllowedIPs = fd00:dead:beef::2/128|AllowedIPs = fd00:dead:beef::/48|' "${WORK}/root/etc/wireguard/wg0.conf"
gate
expect "5 AllowedIPs drift" 1 "DRIFT	redact	/etc/wireguard/wg0.conf"
no_secret "6 (case 5)"

echo "--- 6: nothing printed from a file, with --textfile ---"
setup
sed -i 's|Address = fd00:dead:beef::1/128|Address = fd00:dead:beef::1/64|' "${WORK}/root/etc/wireguard/wg0.conf"
mkdir -p "${WORK}/textfile"
gate --textfile "${WORK}/textfile"
expect "6 drift with a textfile" 1 "DRIFT	redact	/etc/wireguard/wg0.conf"
no_secret "6"
if printf '%s\n' "${OUT}" | grep -qF -e 'fd00:dead:beef::1/64' -e 'Address'; then
	bad "6 a line of the file reached the output"
else
	ok "6 no line of the file in the output"
fi
prom="${WORK}/textfile/unheaded_host_settings.prom"
if [ -f "${prom}" ] && grep -qx 'unheaded_host_settings_drift_files{host="west"} 1' "${prom}" &&
	grep -qx 'unheaded_host_settings_skipped_files{host="west"} 0' "${prom}" &&
	grep -qE '^unheaded_host_settings_checked_timestamp_seconds\{host="west"\} [0-9]+$' "${prom}" &&
	[ "$(stat -c %a "${prom}")" = "644" ] && ! grep -qF -e "${FAKE_KEY}" -e "${FAKE_PW}" "${prom}"; then
	ok "6 textfile: drift 1, skipped 0, a timestamp, mode 0644"
else
	bad "6 textfile"
	[ -f "${prom}" ] && sed 's/^/        /' "${prom}"
fi
left=0
for f in "${WORK}"/textfile/* "${WORK}"/textfile/.[!.]*; do
	[ -e "${f}" ] || continue
	[ "${f##*/}" = "unheaded_host_settings.prom" ] || left=$((left + 1))
done
if [ "${left}" -ne 0 ]; then
	bad "6 a temporary file was left in the textfile directory"
else
	ok "6 no temporary file left"
fi

echo "--- 7: absent ---"
setup
: >"${WORK}/root/var/swapfile9"
gate
expect "7 an absent path that exists drifts" 1 "DRIFT	absent	/var/swapfile9"

echo "--- 8: unreadable ---"
if [ "$(id -u)" -eq 0 ]; then
	echo "  SKIP  8 (running as root: chmod 000 does not stop root)"
else
	setup
	chmod 000 "${WORK}/root/etc/wireguard/wg0.conf"
	gate
	expect "8 unreadable is exit 3" 3 "SKIP	redact	/etc/wireguard/wg0.conf"
	gate --allow-skip
	expect "8 --allow-skip is exit 0" 0 "SKIP	redact	/etc/wireguard/wg0.conf"
	chmod 600 "${WORK}/root/etc/wireguard/wg0.conf"
fi

echo "--- 9: malformed manifest ---"
setup
printf '/etc/fstab\tdeploy/host-settings/west-fstab.fragment\n' >>"${WORK}/repo/deploy/host-settings/west.tsv"
gate
expect "9 two fields is exit 2" 2 ''
setup
printf '/etc/fstab\tdeploy/host-settings/west-fstab.fragment\tmaybe\n' >>"${WORK}/repo/deploy/host-settings/west.tsv"
gate
expect "9 an unknown mode is exit 2" 2 ''
setup
RC=0
OUT="$("${GATE}" --host 'west/../x' --repo "${WORK}/repo" --root "${WORK}/root" 2>&1)" || RC=$?
expect "9 a host name with a slash is exit 2" 2 ''
RC=0
OUT="$("${GATE}" --host nosuch --repo "${WORK}/repo" --root "${WORK}/root" 2>&1)" || RC=$?
expect "9 a host with no manifest is exit 2" 2 ''
RC=0
OUT="$("${GATE}" --bogus 2>&1)" || RC=$?
expect "9 an unknown option is exit 2" 2 ''

echo "--- 10: the repository self-check ---"
setup
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 a clean fake repository passes" 0 ''
sed -i 's/^vm.nr_hugepages = 0$/vm.nr_hugepages = 1024/' "${WORK}/repo/deploy/sysctl.d/99-unheaded-west.conf"
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 huge pages refused" 1 'nr_hugepages'
setup
printf 'MemoryLow=3G\n' >>"${WORK}/repo/deploy/systemd/unheaded.slice"
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 a number in a slice refused" 1 'decision 13'
setup
printf '/etc/x\tdeploy/no/such/file\texact\n' >>"${WORK}/repo/deploy/host-settings/west.tsv"
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 a repo_path that does not exist refused" 1 'deploy/no/such/file'
setup
printf '/etc/x\tdeploy/sysctl.d/99-unheaded-west.conf\n' >>"${WORK}/repo/deploy/host-settings/west.tsv"
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 a malformed manifest refused" 1 ''
setup
rm -rf "${WORK}/repo/deploy/host-settings"
RC=0
OUT="$("${GATE}" --repo "${WORK}/repo" 2>&1)" || RC=$?
expect "10 no manifest directory is exit 2, never a pass" 2 ''

echo ""
echo "test-check-host-settings: ${PASSED} passed, ${FAILED} failed"
[ "${FAILED}" -eq 0 ]
