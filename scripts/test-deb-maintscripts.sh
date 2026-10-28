#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# scripts/lib/deb-maintscripts.sh: run the generated maintainer scripts the
# way dpkg calls them, against a fake systemctl/systemd-sysusers that only
# record their arguments (and answer is-enabled/is-active from a list). The
# contract: an upgrade never stops or disables the service (postinst
# restarts it on the new binary, and only when its TLS material is there);
# removal stops and disables it; a renamed unit keeps its enabled and
# running state. scripts/lib/deb-packages.sh: every unit's binary is
# packaged, under the right name, and debian/control is generated from it.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/deb-maintscripts.sh
. "$REPO_ROOT/scripts/lib/deb-maintscripts.sh"
# shellcheck source=scripts/lib/deb-packages.sh
. "$REPO_ROOT/scripts/lib/deb-packages.sh"

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir "$T/bin" "$T/keys" "$T/tls" "$T/migrate"
for tool in systemd-sysusers unheaded-keygen; do
	printf '#!/bin/sh\necho "%s $*" >>"%s/calls"\n' "$tool" "$T" >"$T/bin/$tool"
	chmod +x "$T/bin/$tool"
done
# systemctl: records every call; is-enabled/is-active answer 0 for the units
# listed in $T/on (one per line).
cat >"$T/bin/systemctl" <<EOF
#!/bin/sh
echo "systemctl \$*" >>"$T/calls"
case "\$1" in
is-enabled|is-active) grep -qxF "\$3" "$T/on" 2>/dev/null ;;
esac
EOF
chmod +x "$T/bin/systemctl"
: >"$T/on"
export UNHEADED_DEB_MIGRATE_DIR="$T/migrate"
# No baseline report unless a test writes one (the host's own must not count).
export UNHEADED_BASELINE_REPORT="$T/no-report.json"
# The generated postinst names its keygen and key directory on two lines of
# its own; point them at the fake tool and a temp dir.
fake_paths() {
	sed -e "s|^KEYGEN=.*|KEYGEN=$T/bin/unheaded-keygen|" -e "s|^KEYDIR=.*|KEYDIR=$T/keys|"
}
gen() { # gen NAME FUNC ARGS...: write the generated script to $T/NAME
	"${@:2}" | fake_paths >"$T/$1"
	chmod +x "$T/$1"
}
gen postinst deb_postinst --pkg unheaded-wotan --unit unheaded-wotan.service --identity wotan --owner unheaded-wotan
gen postinst-nokey deb_postinst --pkg unheaded-huginn --unit huginn.service
gen postinst-tls deb_postinst --pkg unheaded-wotan --unit unheaded-wotan.service \
	--require "$T/tls/ca.pem" --require "$T/tls/wotan.pem"
gen prerm deb_prerm unheaded-wotan
gen prerm-timer deb_prerm unheaded-well-retention unheaded-well-retention.timer unheaded-well-retention.service
gen preinst-mv deb_preinst --pkg unheaded-daemon --former unheaded-unheaded-daemon.service
gen postinst-mv deb_postinst --pkg unheaded-daemon --unit unheaded-daemon.service --former unheaded-unheaded-daemon.service
gen postrm-mv deb_postrm --pkg unheaded-daemon --former unheaded-unheaded-daemon.service

fail=0
run() { # run SCRIPT ARGS...: the calls it made, one per line
	: >"$T/calls"
	PATH="$T/bin:$PATH" "$T/$1" "${@:2}" 2>"$T/stderr"
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
expect "postinst without an identity makes no key; restarts the unit, not the package name" \
	"$(run postinst-nokey configure 1.0.0)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-huginn.conf
systemctl daemon-reload
systemctl try-restart huginn.service"
expect "postinst abort-upgrade touches nothing" "$(run postinst abort-upgrade 1.0.1)" ""
expect "unpatched postinst runs the packaged keygen" \
	"$(deb_postinst --pkg unheaded-wotan --unit unheaded-wotan.service --identity wotan --owner unheaded-wotan | grep -c '^KEYGEN=/opt/unheaded/bin/unheaded-keygen$')" "1"
expect "deb_postinst refuses an identity without an owner" \
	"$(deb_postinst --pkg p --unit p.service --identity p >/dev/null 2>&1 && echo ok || echo refused)" "refused"

# TLS material: the units run TLS-only, so restarting one whose files are
# missing would take a running service down.
expect "postinst: TLS material missing, no restart" "$(run postinst-tls configure 1.0.1)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-wotan.conf
systemctl daemon-reload"
expect "postinst: and says which file" "$(grep -c "missing: $T/tls/ca.pem $T/tls/wotan.pem" "$T/stderr")" "1"
: >"$T/tls/ca.pem"
expect "postinst: one file still missing, still no restart" "$(run postinst-tls configure 1.0.1 | grep -c try-restart || true)" "0"
: >"$T/tls/wotan.pem"
expect "postinst: TLS material in place, restart" "$(run postinst-tls configure 1.0.1 | tail -n 1)" \
	"systemctl try-restart unheaded-wotan.service"

# Unit rename (east's debs named the unit after the package): the former
# unit's state moves to the new one.
printf 'unheaded-unheaded-daemon.service\n' >"$T/on"
expect "preinst install: former unit recorded, stopped and disabled" "$(run preinst-mv install)" \
	"systemctl is-enabled --quiet unheaded-unheaded-daemon.service
systemctl is-active --quiet unheaded-unheaded-daemon.service
systemctl disable --now unheaded-unheaded-daemon.service"
expect "preinst: recorded enabled and active" "$(ls "$T/migrate/unheaded-daemon" | tr '\n' ' ')" \
	"unheaded-unheaded-daemon.service.active unheaded-unheaded-daemon.service.enabled "
: >"$T/on"
expect "postinst configure: new unit enabled and started" "$(run postinst-mv configure)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-daemon.conf
systemctl daemon-reload
systemctl enable unheaded-daemon.service
systemctl start unheaded-daemon.service"
expect "postinst: state cleared" "$(ls "$T/migrate")" ""
expect "postinst configure again (plain upgrade): try-restart only" "$(run postinst-mv configure 1.0.1)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-daemon.conf
systemctl daemon-reload
systemctl try-restart unheaded-daemon.service"
printf 'unheaded-unheaded-daemon.service\n' >"$T/on"
run preinst-mv upgrade 1.0.0 >/dev/null
expect "postrm abort-upgrade: the former unit gets its state back" "$(run postrm-mv abort-upgrade 1.0.0)" \
	"systemctl enable unheaded-unheaded-daemon.service
systemctl start unheaded-unheaded-daemon.service"
expect "postrm: state cleared" "$(ls "$T/migrate")" ""
: >"$T/on"
run preinst-mv install >/dev/null
expect "preinst: a former unit that was off records nothing" "$(ls "$T/migrate/unheaded-daemon")" ""
expect "postinst then neither enables nor starts" "$(run postinst-mv configure | grep -cE '^systemctl (enable|start) ' || true)" "0"

# debian/ (the dh tree) has no hand-written maintainer scripts: debian/rules
# builds through build-all-debs.sh, so there is one copy (the old
# debian/unheaded-wotan.postinst made a shared "unheaded" account).
expect "debian/ holds no maintainer scripts of its own" \
	"$(find "$REPO_ROOT/debian" -maxdepth 1 \( -name '*.postinst' -o -name '*.preinst' -o -name '*.prerm' -o -name '*.postrm' \) | wc -l)" "0"

# The wotan package ships the topic config its unit reads, as a conffile.
mkdir -p "$T/pkg/DEBIAN" "$T/other/DEBIAN" "$T/wr/DEBIAN"
deb_conffiles wotan "$T/pkg" "$REPO_ROOT"
expect "wotan ships /etc/unheaded/wotan.yaml as a conffile" "$(cat "$T/pkg/DEBIAN/conffiles")" "/etc/unheaded/wotan.yaml"
expect "wotan.yaml is the repo's configs/wotan.yaml" "$(cmp -s "$REPO_ROOT/configs/wotan.yaml" "$T/pkg/etc/unheaded/wotan.yaml" && echo same)" "same"
expect "unit reads that path" "$(grep -c -- '--topic-config /etc/unheaded/wotan.yaml' "$REPO_ROOT/deploy/systemd/unheaded-wotan.service")" "1"
deb_conffiles well-retention "$T/wr" "$REPO_ROOT"
expect "well-retention ships its classes as a conffile" "$(cat "$T/wr/DEBIAN/conffiles")" "/etc/unheaded/well/retention-classes.yaml"
deb_conffiles timeguru "$T/other" "$REPO_ROOT"
expect "other services ship no conffiles" "$(ls "$T/other/DEBIAN")" ""

# The ADR-098 enforcing gate (S-WELL-SPRING Step 276): preinst refuses an
# install or upgrade only when the baseline agent's report says enforcing is
# in effect with critical deviations left. The report is the agent's own
# MarshalIndent output (two-space keys).
deb_preinst unheaded-wotan >"$T/preinst"
chmod +x "$T/preinst"
report() { # report MODE CRIT
	printf '{\n  "version": 1,\n  "host": "east",\n  "mode_config": "%s",\n  "mode_effective": "%s",\n  "critical_deviations": %s,\n  "checks": []\n}\n' "$1" "$1" "$2" >"$T/report.json"
}
gate() { # gate ARGS...: exit code of preinst against $T/report.json
	local rc=0
	UNHEADED_BASELINE_REPORT="$T/report.json" "$T/preinst" "$@" 2>/dev/null || rc=$?
	echo "$rc"
}
rm -f "$T/report.json"
expect "preinst: no baseline report, install goes ahead" "$(gate install)" "0"
report audit 0
expect "preinst: audit, install goes ahead" "$(gate install)" "0"
report audit 4
expect "preinst: audit with deviations, install goes ahead (audit never blocks)" "$(gate upgrade 1.0.0)" "0"
report enforcing 0
expect "preinst: enforcing with nothing left, install goes ahead" "$(gate install)" "0"
report enforcing 2
expect "preinst: enforcing with critical deviations refuses install" "$(gate install)" "1"
expect "preinst: and refuses upgrade" "$(gate upgrade 1.0.0)" "1"
expect "preinst: abort-upgrade is never refused" "$(gate abort-upgrade 1.0.1)" "0"
expect "preinst: the override lets it through" "$(UNHEADED_BASELINE_GATE=off gate install)" "0"
printf '{\n  "mode_effective": "enforcing",\n  "critical_deviations": "x"\n}\n' >"$T/report.json"
expect "preinst: enforcing with an unreadable count refuses (fail closed)" "$(gate install)" "1"
gen preinst-mv-gate deb_preinst --pkg unheaded-daemon --former unheaded-unheaded-daemon.service
printf 'unheaded-unheaded-daemon.service\n' >"$T/on"
report enforcing 2
expect "preinst: a refused install stops nothing" \
	"$(UNHEADED_BASELINE_REPORT="$T/report.json" run preinst-mv-gate install || true)" ""
: >"$T/on"
rm -rf "${T:?}/migrate/"*

# The package table (scripts/lib/deb-packages.sh).
unit_bins() { # every /usr/local/bin binary a unit runs, minus third-party ones
	sed -n 's|^ExecStart=-\{0,1\}/usr/local/bin/\([A-Za-z0-9._-]*\).*|\1|p' \
		"$REPO_ROOT"/deploy/systemd/*.service | grep -vxF node_exporter | sort -u
}
expect "every unit's binary is packaged, and nothing else" \
	"$(printf '%s\n' "${DEB_SERVICES[@]}" | sort -u)" "$(unit_bins)"
expect "unheaded-daemon packages as unheaded-daemon" "$(deb_pkg_name unheaded-daemon)" "unheaded-daemon"
expect "unheaded-auth packages as unheaded-auth" "$(deb_pkg_name unheaded-auth)" "unheaded-auth"
expect "wotan packages as unheaded-wotan" "$(deb_pkg_name wotan)" "unheaded-wotan"
bad=""
for svc in "${DEB_SERVICES[@]}"; do
	pkg="$(deb_pkg_name "$svc")"
	unit="$REPO_ROOT/deploy/systemd/$(deb_unit_for "$svc")"
	[[ "$pkg" != unheaded-unheaded-* ]] || bad="$bad $svc:double-prefix"
	[[ -f "$unit" ]] || { bad="$bad $svc:no-unit"; continue; }
	grep -q "^ExecStart=-\{0,1\}/usr/local/bin/$svc\( \|$\)" "$unit" || bad="$bad $svc:unit-runs-another-binary"
	for u in $(deb_extra_units "$svc"); do [[ -f "$REPO_ROOT/deploy/systemd/$u" ]] || bad="$bad $svc:no-$u"; done
	user="$(sed -n 's/^User=//p' "$unit")"
	awk -v u="$user" '$1 == "u" && $2 == u { f = 1 } END { exit !f }' "$REPO_ROOT/deploy/sysusers.d/unheaded.conf" || bad="$bad $svc:no-sysusers-line"
	[[ -n "$(deb_description "$unit")" ]] || bad="$bad $svc:no-description"
	# The key postinst creates is the one the unit reads.
	keyf="$(sed -n 's|^Environment=AUTH_KEY_FILE=/etc/unheaded/keys/\(.*\)\.key$|\1|p' "$unit")"
	[[ -z "$keyf" || "$keyf" == "$(deb_identity_for "$svc")" ]] || bad="$bad $svc:identity-$(deb_identity_for "$svc")-vs-unit-$keyf"
done
expect "each package: one name, its unit, its account, its key" "$bad" ""
expect "a TLS unit's postinst requires its CA, cert and key" \
	"$(deb_tls_files "$REPO_ROOT/deploy/systemd/unheaded-wotan.service" | tr '\n' ' ')" \
	"/etc/unheaded/tls/wotan/ca.pem /etc/unheaded/tls/wotan/wotan.pem /etc/unheaded/tls/wotan/wotan-key.pem "
expect "a CA-only unit requires the CA" \
	"$(deb_tls_files "$REPO_ROOT/deploy/systemd/unheaded-kev-poller.service")" "/etc/unheaded/tls/ca.pem"

# debian/control is generated from the same table and committed as is.
expect "debian/control is the generated one (scripts/build-all-debs.sh --control > debian/control)" \
	"$(cat "$REPO_ROOT/debian/control")" "$(deb_control "$REPO_ROOT")"
expect "debian/control: one stanza per package" \
	"$(grep -c '^Package:' "$REPO_ROOT/debian/control")" "$((${#DEB_SERVICES[@]} + ${#DEB_EXTRA_PACKAGES[@]}))"
expect "debian/control: no double prefix" "$(grep -c '^Package: unheaded-unheaded-' "$REPO_ROOT/debian/control" || true)" "0"

# build-all-debs.sh wires the table into every package.
expect "build-all-debs ships the gate in every service package" \
	"$(grep -c 'deb_preinst "\${PRE\[@\]}" >"\$P/DEBIAN/preinst"' "$REPO_ROOT/scripts/build-all-debs.sh")" "1"
expect "build-all-debs: key-holding packages depend on unheaded-keygen" \
	"$(deb_depends wotan)" 'libc6, unheaded-keygen (= ${VERSION})'
expect "build-all-debs: keyless packages do not" "$(deb_depends huginn)" "libc6"
expect "the Jenkinsfile packages through build-all-debs.sh (one copy)" \
	"$(grep -c '^ *DEB_OUTPUT_DIR=build/deb \./scripts/build-all-debs\.sh ' "$REPO_ROOT/Jenkinsfile")" "1"
expect "the Jenkinsfile writes no maintainer scripts of its own" \
	"$(grep -cE 'deb_(postinst|prerm|preinst)' "$REPO_ROOT/Jenkinsfile" || true)" "0"

exit "$fail"
