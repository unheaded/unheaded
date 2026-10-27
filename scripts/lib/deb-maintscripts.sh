#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Maintainer scripts for the per-service .deb packages (ADR-026, ADR-085
# Phase 2), used by scripts/build-all-debs.sh (which the Jenkinsfile and
# release.yml call) so there is one copy. Source it, then:
#
#   deb_postinst --pkg unheaded-wotan --unit unheaded-wotan.service \
#       --identity wotan --owner unheaded-wotan \
#       --require /etc/unheaded/tls/wotan/ca.pem >"$PKG_DIR/DEBIAN/postinst"
#   deb_prerm    unheaded-wotan unheaded-wotan.service >"$PKG_DIR/DEBIAN/prerm"
#   deb_preinst  --pkg unheaded-wotan >"$PKG_DIR/DEBIAN/preinst"
#   deb_conffiles wotan "$PKG_DIR" "$REPO_ROOT"
#
# Tested by scripts/test-deb-maintscripts.sh.

# Where preinst records the state of a former unit (one the package used to
# install under another name) for postinst to carry over.
DEB_MIGRATE_DIR=/var/lib/unheaded/deb-migrate

# deb_postinst --pkg PKG --unit UNIT [--identity ID --owner USER]
#              [--require FILE]... [--former UNIT]...
# On configure: create the service's account from its sysusers.d line; with
# --identity, create the service's Ed25519 key pair if it has none (ADR-103:
# `unheaded-keygen -owner USER ID`, from the unheaded-keygen package every
# such package depends on; an existing key is never touched, rotation is an
# operator step, deploy/systemd/README.md#keys-adr-103); reload units; carry
# a former unit's enabled/active state over to UNIT (preinst recorded it);
# then restart UNIT if it was running, so an upgrade runs the new binary
# (a stopped service stays stopped, nothing is ever enabled on a fresh
# install). Each --require file (the unit's TLS CA, cert and key) must exist
# first: the service refuses to start without them (no plaintext fallback),
# and restarting it into that refusal would take a running service down. So
# with one missing, postinst says which and leaves the unit as it is.
deb_postinst() {
	local pkg="" unit="" identity="" owner="" require="" former=""
	while [ $# -gt 0 ]; do
		case "$1" in
		--pkg) pkg=$2 ;;
		--unit) unit=$2 ;;
		--identity) identity=$2 ;;
		--owner) owner=$2 ;;
		--require) require="$require $2" ;;
		--former) former="$former $2" ;;
		*) echo "deb_postinst: unknown argument $1" >&2; return 2 ;;
		esac
		shift 2
	done
	[ -n "$pkg" ] && [ -n "$unit" ] || { echo "deb_postinst: --pkg and --unit are required" >&2; return 2; }
	[ -z "$identity" ] || [ -n "$owner" ] || { echo "deb_postinst: --identity needs --owner" >&2; return 2; }
	local keygen="" tls="" carry="" restart
	if [ -n "$identity" ]; then
		keygen="	if [ ! -e \"\$KEYDIR/$identity.key\" ]; then
		\"\$KEYGEN\" -owner $owner $identity
	fi
"
	fi
	if [ -n "$require" ]; then
		tls="# The files $unit needs before it can start (its TLS material).
ready() {
	missing=\"\"
	for f in${require}; do
		[ -e \"\$f\" ] || missing=\"\$missing \$f\"
	done
	[ -z \"\$missing\" ] && return 0
	echo \"$pkg: $unit cannot start until its TLS material is installed (deploy/systemd/README.md#tls); missing:\$missing\" >&2
	return 1
}
"
	else
		tls="ready() { return 0; }
"
	fi
	restart="	if ready; then
		systemctl try-restart $unit || true
	fi
"
	if [ -n "$former" ]; then
		carry="	# A former unit name (preinst stopped and disabled it while its file
	# still existed): $unit takes over its enabled and running state.
	if ls \"\$MIGRATE\"/*.enabled >/dev/null 2>&1; then
		systemctl enable $unit
	fi
	if ls \"\$MIGRATE\"/*.active >/dev/null 2>&1; then
		if ready; then
			systemctl start $unit
		fi
	elif ready; then
		systemctl try-restart $unit || true
	fi
	rm -rf \"\$MIGRATE\"
"
		restart=""
	fi
	cat <<SCRIPT
#!/bin/sh
set -e
KEYGEN=/opt/unheaded/bin/unheaded-keygen
KEYDIR=/etc/unheaded/keys
MIGRATE=\${UNHEADED_DEB_MIGRATE_DIR:-$DEB_MIGRATE_DIR}/$pkg
${tls}case "\$1" in
configure)
	systemd-sysusers /usr/lib/sysusers.d/$pkg.conf
${keygen}	systemctl daemon-reload
${carry}${restart}	;;
esac
SCRIPT
}

# deb_conffiles SVC PKG_DIR REPO_ROOT: install the config files the service's
# unit reads into the package tree and list them in DEBIAN/conffiles, so dpkg
# keeps local edits on upgrade. Wotan's unit reads --topic-config
# /etc/unheaded/wotan.yaml; the package did not ship it, so on east no
# publisher was ever auto-approved (found 2026-10-20). The retention classes
# are edited locally (ratified: true, a legal hold) and must survive upgrades.
deb_conffiles() {
	case "$1" in
	wotan)
		install -D -m 0644 "$3/configs/wotan.yaml" "$2/etc/unheaded/wotan.yaml"
		echo /etc/unheaded/wotan.yaml >"$2/DEBIAN/conffiles"
		;;
	well-retention)
		install -D -m 0644 "$3/compliance/retention/classes.yaml" "$2/etc/unheaded/well/retention-classes.yaml"
		echo /etc/unheaded/well/retention-classes.yaml >"$2/DEBIAN/conffiles"
		;;
	esac
}

# deb_prerm PKG [UNIT...]: stop and disable only when the package is being
# removed. dpkg also runs prerm with "upgrade" on every upgrade; disabling
# there left the service disabled at the next boot after any apt upgrade.
# UNITs default to PKG.service; a timer package names its timer first.
deb_prerm() {
	local pkg=$1 u lines=""
	shift
	[ $# -gt 0 ] || set -- "$pkg.service"
	for u in "$@"; do
		lines="$lines	systemctl stop $u 2>/dev/null || true
	systemctl disable $u 2>/dev/null || true
"
	done
	cat <<SCRIPT
#!/bin/sh
set -e
case "\$1" in
remove|deconfigure)
${lines}	;;
esac
SCRIPT
}

# deb_preinst --pkg PKG [--former UNIT]... (or deb_preinst PKG)
#
# The ADR-098 enforcing gate (S-WELL-SPRING Step 276). On install or
# upgrade, refuse when the host's baseline agent says enforcing is in effect
# and left critical deviations (report.json: "mode_effective": "enforcing"
# and "critical_deviations" above 0): installing more on a host that is out
# of its baseline is what enforcing exists to stop. Installs only; refusing
# to START services on such a host is E-E Q5. Audit mode, no agent, or no
# report: the install goes ahead. A report claiming enforcing whose count
# cannot be read blocks (fail closed); the agent flags a report changed
# behind its back as a critical event on its next run. Recovery override,
# which the refusal names: UNHEADED_BASELINE_GATE=off apt install ...
#
# Then, with --former, the unit rename: each former unit (the name an
# earlier deb installed the service under) is recorded as enabled and/or
# running, then stopped and disabled while its unit file still exists (dpkg
# deletes it on unpack). postinst configure gives the new unit that state;
# postrm abort-* gives it back to the former unit if the unpack fails.
deb_preinst() {
	local pkg="" former=""
	if [ $# -gt 0 ] && [ "${1#--}" = "$1" ]; then
		pkg=$1
		shift
	fi
	while [ $# -gt 0 ]; do
		case "$1" in
		--pkg) pkg=$2 ;;
		--former) former="$former $2" ;;
		*) echo "deb_preinst: unknown argument $1" >&2; return 2 ;;
		esac
		shift 2
	done
	[ -n "$pkg" ] || { echo "deb_preinst: --pkg is required" >&2; return 2; }
	local migrate=""
	if [ -n "$former" ]; then
		migrate="	mkdir -p \"\$MIGRATE\"
	for u in${former}; do
		if systemctl is-enabled --quiet \"\$u\" 2>/dev/null; then : >\"\$MIGRATE/\$u.enabled\"; fi
		if systemctl is-active --quiet \"\$u\" 2>/dev/null; then : >\"\$MIGRATE/\$u.active\"; fi
		systemctl disable --now \"\$u\" 2>/dev/null || true
	done
"
	fi
	cat <<SCRIPT
#!/bin/sh
set -e
REPORT=\${UNHEADED_BASELINE_REPORT:-/var/lib/unheaded/baseline/report.json}
MIGRATE=\${UNHEADED_DEB_MIGRATE_DIR:-$DEB_MIGRATE_DIR}/$pkg
baseline_gate() {
	[ -r "\$REPORT" ] || return 0
	grep -Eq '"mode_effective"[[:space:]]*:[[:space:]]*"enforcing"' "\$REPORT" || return 0
	crit=\$(grep -Eo '"critical_deviations"[[:space:]]*:[[:space:]]*[0-9]+' "\$REPORT" | grep -Eo '[0-9]+\$' | head -n 1)
	[ "\${crit:-1}" != 0 ] || return 0
	if [ "\${UNHEADED_BASELINE_GATE:-}" = off ]; then
		echo "$pkg: baseline gate overridden (UNHEADED_BASELINE_GATE=off); host is enforcing with \${crit:-unknown} critical deviation(s)" >&2
		return 0
	fi
	echo "$pkg: refusing to install: this host's baseline is enforcing and \${crit:-unknown} critical deviation(s) remain (\$REPORT, ADR-098). Fix or undo them, or set UNHEADED_BASELINE_GATE=off." >&2
	return 1
}
case "\$1" in
install|upgrade)
	baseline_gate || exit 1
${migrate}	;;
esac
exit 0
SCRIPT
}

# deb_postrm --pkg PKG --former UNIT...: when the unpack that preinst
# prepared fails (dpkg runs the new postrm with abort-install/abort-upgrade
# and puts the old files back), the former units get back the state preinst
# took from them. Only packages that renamed their unit have a postrm.
deb_postrm() {
	local pkg="" former=""
	while [ $# -gt 0 ]; do
		case "$1" in
		--pkg) pkg=$2 ;;
		--former) former="$former $2" ;;
		*) echo "deb_postrm: unknown argument $1" >&2; return 2 ;;
		esac
		shift 2
	done
	[ -n "$pkg" ] && [ -n "$former" ] || { echo "deb_postrm: --pkg and --former are required" >&2; return 2; }
	cat <<SCRIPT
#!/bin/sh
set -e
MIGRATE=\${UNHEADED_DEB_MIGRATE_DIR:-$DEB_MIGRATE_DIR}/$pkg
case "\$1" in
abort-install|abort-upgrade)
	for u in${former}; do
		if [ -e "\$MIGRATE/\$u.enabled" ]; then systemctl enable "\$u" 2>/dev/null || true; fi
		if [ -e "\$MIGRATE/\$u.active" ]; then systemctl start "\$u" 2>/dev/null || true; fi
	done
	rm -rf "\$MIGRATE"
	;;
esac
exit 0
SCRIPT
}
