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
mkdir -p "$T/pkg/DEBIAN" "$T/other/DEBIAN" "$T/wr/DEBIAN" "$T/sr/DEBIAN"
deb_conffiles wotan "$T/pkg" "$REPO_ROOT"
expect "wotan ships /etc/unheaded/wotan.yaml as a conffile" "$(cat "$T/pkg/DEBIAN/conffiles")" "/etc/unheaded/wotan.yaml"
expect "wotan.yaml is the repo's configs/wotan.yaml" "$(cmp -s "$REPO_ROOT/configs/wotan.yaml" "$T/pkg/etc/unheaded/wotan.yaml" && echo same)" "same"
expect "unit reads that path" "$(grep -c -- '--topic-config /etc/unheaded/wotan.yaml' "$REPO_ROOT/deploy/systemd/unheaded-wotan.service")" "1"
deb_conffiles well-retention "$T/wr" "$REPO_ROOT"
expect "well-retention ships its classes as a conffile" "$(cat "$T/wr/DEBIAN/conffiles")" "/etc/unheaded/well/retention-classes.yaml"
deb_conffiles syslog-relay "$T/sr" "$REPO_ROOT"
expect "syslog-relay ships its sources as a conffile" "$(cat "$T/sr/DEBIAN/conffiles")" "/etc/unheaded/log-sources.yaml"
expect "log-sources.yaml is the repo's config/logging/sources.yaml" "$(cmp -s "$REPO_ROOT/config/logging/sources.yaml" "$T/sr/etc/unheaded/log-sources.yaml" && echo same)" "same"
expect "syslog-relay's unit reads that path" "$(grep -c -- '--sources /etc/unheaded/log-sources.yaml' "$REPO_ROOT/deploy/systemd/unheaded-syslog-relay.service")" "1"
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
unit_bins() { # every /usr/local/bin binary a unit runs, minus third-party and cargo-built ones
	# Cargo-built binaries (build-binaries.sh CARGO, e.g. upc-bootctl) are not
	# packaged as .debs; their unit names the install command. Third-party ones
	# are named here, not read from build-binaries.sh's THIRD_PARTY: vor lives
	# in its own repository, and a binary built here must not drop out of this
	# check by joining that list.
	local cargo
	cargo="$(awk '/^CARGO="/{f=1;next} f&&/^"/{f=0} f&&NF==2{print $1}' "$REPO_ROOT/scripts/build-binaries.sh")"
	sed -n 's|^ExecStart=-\{0,1\}/usr/local/bin/\([A-Za-z0-9._-]*\).*|\1|p' \
		"$REPO_ROOT"/deploy/systemd/*.service | grep -vxF -e node_exporter -e vor | grep -vxF -e "${cargo:-/}" | sort -u
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
expect "syslog-relay holds no service key" "$(deb_identity_for syslog-relay)" ""
# Socket activation (ADR-124, answer #18 of 2026-11-09): the package ships the
# .socket that binds 514, and removal stops it before the service, so a sender
# cannot start the relay again on the way out. An upgrade touches neither.
expect "syslog-relay ships its socket unit" "$(deb_extra_units syslog-relay)" "unheaded-syslog-relay.socket"
read -r -a SR_EXTRA <<<"$(deb_extra_units syslog-relay)"
gen prerm-socket deb_prerm unheaded-syslog-relay "${SR_EXTRA[@]}" "$(deb_unit_for syslog-relay)"
expect "syslog-relay prerm remove: the socket stops and is disabled first" "$(run prerm-socket remove)" \
	"systemctl stop unheaded-syslog-relay.socket
systemctl disable unheaded-syslog-relay.socket
systemctl stop unheaded-syslog-relay.service
systemctl disable unheaded-syslog-relay.service"
expect "syslog-relay prerm upgrade touches nothing" "$(run prerm-socket upgrade 1.0.1)" ""
gen postinst-relay deb_postinst --pkg unheaded-syslog-relay --unit "$(deb_unit_for syslog-relay)"
expect "syslog-relay postinst: account, reload, try-restart; no key, nothing enabled" "$(run postinst-relay configure 1.0.0)" \
	"systemd-sysusers /usr/lib/sysusers.d/unheaded-syslog-relay.conf
systemctl daemon-reload
systemctl try-restart unheaded-syslog-relay.service"
expect "the relay's unit holds no capability: the socket binds 514" \
	"$(grep -cE '^(AmbientCapabilities=|CapabilityBoundingSet=.+)' "$REPO_ROOT/deploy/systemd/unheaded-syslog-relay.service" || true)" "0"
# The Zhen UI front (ADR-138): a certificate and key, no CA, no service key.
expect "zhen-ui-front holds no service key" "$(deb_identity_for zhen-ui-front)" ""
expect "zhen-ui-front's postinst requires its cert and key, and no CA" \
	"$(deb_tls_files "$REPO_ROOT/deploy/systemd/unheaded-zhen-ui-front.service" | tr '\n' ' ')" \
	"/etc/unheaded/tls/zhen-ui-front/zhen-ui-front.pem /etc/unheaded/tls/zhen-ui-front/zhen-ui-front-key.pem "
expect "the front's unit holds no capability and names no CA or profile" \
	"$(grep -cE '^(AmbientCapabilities=|CapabilityBoundingSet=.+|Environment=UNHEADED_TLS_(PROFILE|CA_CERT|MTLS_ENABLED)=)' "$REPO_ROOT/deploy/systemd/unheaded-zhen-ui-front.service" || true)" "0"
expect "the Jenkinsfile packages through build-all-debs.sh (one copy)" \
	"$(grep -c '^ *DEB_OUTPUT_DIR=build/deb \./scripts/build-all-debs\.sh ' "$REPO_ROOT/Jenkinsfile")" "1"
expect "the Jenkinsfile writes no maintainer scripts of its own" \
	"$(grep -cE 'deb_(postinst|prerm|preinst)' "$REPO_ROOT/Jenkinsfile" || true)" "0"

# East's 1.0.0 packages: runbooks/deploy/apt-repo-publish.yaml's east-side
# steps, run against stand-ins, with /etc/unheaded, /etc/apt and /var/backups
# moved under $T. awk, not a YAML parser: this job has no Python set up.
runbook_command() { # the command block of the step with this name, unindented
	awk -v want="  - name: $1" '
		$0 == want { found = 1; next }
		found && /^    command: \|$/ { body = 1; next }
		body && /^      / { print substr($0, 7); next }
		body && /^$/ { print; next }
		body { exit }' "$REPO_ROOT/runbooks/deploy/apt-repo-publish.yaml"
}
mkdir -p "$T/etc/unheaded" "$T/east" "$T/etc/apt/sources.list.d" "$T/var/backups"
for step in install-east east-source east-previous-version; do
	runbook_command "$step" | sed -e "s|/etc/unheaded|$T/etc/unheaded|g" -e "s|/etc/apt|$T/etc/apt|g" \
		-e "s|/var/backups|$T/var/backups|g" -e 's|\${TAG}|v0.2.0-alpha|' -e 's|\${KEYID}|REPOKEY|' \
		-e 's|\${PREVIOUS}|0.1.0|g' >"$T/$step"
done
printf '#!/bin/sh\nexec "$@"\n' >"$T/east/sudo"
for tool in apt-get systemctl; do
	printf '#!/bin/sh\necho "%s $*" >>"%s/calls"\n' "$tool" "$T" >"$T/east/$tool"
done
# dpkg-query: unheaded-wotan's conffiles, which $T/conffiles holds, or none.
printf '#!/bin/sh\n[ "$*" = "--control-show unheaded-wotan conffiles" ] && [ -s "%s/conffiles" ]\n' "$T" >"$T/east/dpkg-query"
# dpkg -l and apt-cache madison: what $T/dpkg-l and $T/madison hold, in their
# own layouts.
printf '#!/bin/sh\ncat "%s/dpkg-l"\n' "$T" >"$T/east/dpkg"
printf '#!/bin/sh\nawk -v p="$2" %s$1 == p%s "%s/madison"\n' "'" "'" "$T" >"$T/east/apt-cache"
printf '#!/bin/sh\necho public-key\n' >"$T/east/curl"
printf '#!/bin/sh\ncat "%s/fingerprint"\n' "$T" >"$T/east/gpg"
chmod +x "$T"/east/*
east_tls() { # the three files postinst requires, for each identity
	for id in "$@"; do
		mkdir -p "$T/etc/unheaded/tls/$id"
		: >"$T/etc/unheaded/tls/$id/ca.pem"
		: >"$T/etc/unheaded/tls/$id/$id.pem"
		: >"$T/etc/unheaded/tls/$id/$id-key.pem"
	done
}
east_run() { # east_run STEP: the calls the step made, then its exit status
	local rc=0
	: >"$T/calls"
	PATH="$T/east:$PATH" bash "$T/$1" 2>"$T/stderr" || rc=$?
	cat "$T/calls"
	echo "rc=$rc"
}
install_east() { east_run install-east; }
: >"$T/conffiles"
echo "1.0.0's file" >"$T/etc/unheaded/wotan.yaml"
east_tls wotan timeguru akira captain architect micromanager unheaded-daemon dashboard-backend
expect "install-east: one service's TLS material missing, nothing installed" "$(install_east | tail -n 2)" "rc=1"
expect "install-east: and says whose" "$(grep -c 'no .*/tls/kanban-app/ca.pem' "$T/stderr")" "1"
expect "install-east: and leaves wotan.yaml alone" "$(cat "$T/etc/unheaded/wotan.yaml")" "1.0.0's file"
east_tls kanban-app
bad=""
for unit in wotan timeguru akira captain architect micromanager daemon dashboard kanban; do
	while read -r f; do
		mv "$T$f" "$T$f.away"
		[ "$(install_east)" = "rc=1" ] && grep -q "no $T$f " "$T/stderr" || bad="$bad $f"
		mv "$T$f.away" "$T$f"
	done < <(deb_tls_files "$REPO_ROOT/deploy/systemd/unheaded-$unit.service")
done
expect "install-east stops on every file postinst requires, not the CA alone" "$bad" ""
got="$(install_east)"
expect "install-east finishes" "$(tail -n 1 <<<"$got")" "rc=0"
expect "install-east names the version and allows the move from 1.0.0" \
	"$(grep -c '^apt-get install -y --allow-downgrades .*unheaded-wotan=0\.2\.0~alpha .*unheaded-timeguru=0\.2\.0~alpha ' <<<"$got")" "1"
expect "install-east starts wotan and timeguru after the packages are in" \
	"$(grep -n . <<<"$got" | sed -n '2s/^2:systemctl enable --now unheaded-wotan unheaded-timeguru .*/yes/p')" "yes"
expect "install-east moves 1.0.0's plain wotan.yaml aside" \
	"$(cat "$T/etc/unheaded/wotan.yaml.pre-conffile"; [ ! -e "$T/etc/unheaded/wotan.yaml" ] && echo gone)" "1.0.0's file
gone"
echo "an edited conffile" >"$T/etc/unheaded/wotan.yaml"
rm "$T/etc/unheaded/wotan.yaml.pre-conffile"
printf ' /etc/unheaded/wotan.yaml 0123\n' >"$T/conffiles"
install_east >/dev/null
expect "install-east leaves a real conffile for dpkg" "$(cat "$T/etc/unheaded/wotan.yaml")" "an edited conffile"

# east-source: the unsigned source goes, or apt can take east back to 1.0.0.
OLD_SOURCE="$T/etc/apt/sources.list.d/unheaded.sources"
SIGNED_LIST="$T/etc/apt/sources.list.d/unheaded.list"
echo "Trusted: yes" >"$OLD_SOURCE"
echo "other key" >"$T/fingerprint"
expect "east-source: the wrong key stops it" "$(east_run east-source | tail -n 1 | grep -vc '^rc=0$')" "1"
expect "east-source: and the old source stays" "$(cat "$OLD_SOURCE"; [ ! -e "$SIGNED_LIST" ] && echo unlisted)" "Trusted: yes
unlisted"
echo "fpr REPOKEY" >"$T/fingerprint"
expect "east-source updates once it has moved the sources" "$(east_run east-source)" "apt-get update
rc=0"
expect "east-source retires the unsigned source" \
	"$([ ! -e "$OLD_SOURCE" ] && cat "$T/var/backups/unheaded.sources.bak-e1")" "Trusted: yes"
expect "east-source lists the signed repo" "$(grep -c 'signed-by=.*unheaded main' "$SIGNED_LIST")" "1"
expect "east-source runs again with the old source gone" "$(east_run east-source | tail -n 1)" "rc=0"

# Rollback: all of it in one apt run, or none of it.
printf 'ii  unheaded-wotan 0.2.0~alpha amd64 x\nii  unheaded-auth 0.2.0~alpha amd64 x\nrc  unheaded-unheaded-daemon 1.0.0 amd64 x\n' >"$T/dpkg-l"
printf ' unheaded-wotan | 0.2.0~alpha | http://r\n unheaded-wotan | 0.1.0 | http://r\n unheaded-auth | 0.2.0~alpha | http://r\n' >"$T/madison"
expect "rollback: one package with no such version, nothing changes" "$(east_run east-previous-version)" "rc=1"
expect "rollback: and it says which, and only that one" "$(grep -c 'no 0\.1\.0 build of: unheaded-auth:' "$T/stderr")" "1"
echo ' unheaded-auth | 0.1.0 | http://r' >>"$T/madison"
expect "rollback: every package in one apt run, a removed one left out" "$(east_run east-previous-version)" \
	"apt-get install -y --allow-downgrades unheaded-wotan=0.1.0 unheaded-auth=0.1.0
rc=0"

# The reasons, from dpkg itself, on packages shaped like 1.0.0's and this
# release's, in a private root.
expect "this release's version sorts below 1.0.0, so apt needs it named" \
	"$(dpkg --compare-versions '0.2.0~alpha' lt 1.0.0 && echo lower)" "lower"
R="$T/dpkg-root"
mkdir -p "$R/var/lib/dpkg/info" "$R/var/lib/dpkg/updates" "$R/var/lib/dpkg/triggers" \
	"$T/old/DEBIAN" "$T/old/etc/unheaded" "$T/new/DEBIAN" "$T/new/etc/unheaded"
: >"$R/var/lib/dpkg/status"
fixture_control() { printf 'Package: fixture\nVersion: %s\nArchitecture: all\nMaintainer: t <t@invalid>\nDescription: t\n' "$1"; }
fixture_control 1.0.0 >"$T/old/DEBIAN/control"
echo old >"$T/old/etc/unheaded/wotan.yaml"
fixture_control '0.2.0~alpha' >"$T/new/DEBIAN/control"
echo new >"$T/new/etc/unheaded/wotan.yaml"
echo /etc/unheaded/wotan.yaml >"$T/new/DEBIAN/conffiles"
printf '#!/bin/sh\necho "old prerm $1" >>"%s/order"\n' "$T" >"$T/old/DEBIAN/prerm"
printf '#!/bin/sh\necho "new preinst $1" >>"%s/order"\n' "$T" >"$T/new/DEBIAN/preinst"
chmod 755 "$T/old/DEBIAN/prerm" "$T/new/DEBIAN/preinst"
dpkg-deb --root-owner-group --build "$T/old" "$T/old.deb" >/dev/null
dpkg-deb --root-owner-group --build "$T/new" "$T/new.deb" >/dev/null
sandbox_dpkg() {
	dpkg --root="$R" --force-not-root --force-script-chrootless --no-triggers --log=/dev/null "$@" </dev/null >"$T/dpkg.out" 2>&1
}
sandbox_dpkg -i "$T/old.deb"
expect "dpkg -l's first column tells the installed from the rest" \
	"$(dpkg --root="$R" -l 'fixture' | awk '$1 == "ii" {print $2}')" "fixture"
expect "a package with no conffiles fails --control-show, so wotan.yaml moves" \
	"$(dpkg-query --root="$R" --control-show fixture conffiles 2>/dev/null || echo none)" "none"
expect "a plain file turned conffile fails with no terminal" \
	"$(sandbox_dpkg -i "$T/new.deb" && echo installed || grep -c 'end of file on stdin at conffile prompt' "$T/dpkg.out")" "1"
expect "the old prerm runs before any new script, so no package can read the state it destroys" \
	"$(cat "$T/order")" "old prerm upgrade
new preinst upgrade"
rm "$R/etc/unheaded/wotan.yaml"
expect "with the plain file moved aside the install completes" \
	"$(sandbox_dpkg --configure fixture && cat "$R/etc/unheaded/wotan.yaml")" "new"
expect "a package with conffiles passes --control-show, so wotan.yaml stays" \
	"$(dpkg-query --root="$R" --control-show fixture conffiles)" "/etc/unheaded/wotan.yaml"

exit "$fail"
