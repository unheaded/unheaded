#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Maintainer scripts for the per-service .deb packages (ADR-026, ADR-085
# Phase 2), shared by scripts/build-all-debs.sh and the Jenkinsfile so the
# two cannot drift. Source it, then:
#
#   deb_postinst unheaded-wotan >"$PKG_DIR/DEBIAN/postinst"
#   deb_prerm    unheaded-wotan >"$PKG_DIR/DEBIAN/prerm"
#   deb_conffiles wotan "$PKG_DIR" "$REPO_ROOT"
#
# Tested by scripts/test-deb-maintscripts.sh.

# deb_postinst PKG [IDENTITY OWNER]: on configure, create the service's
# account from its sysusers.d line; with IDENTITY, create the service's
# Ed25519 key pair if it has none (ADR-103: `unheaded-keygen -owner OWNER
# IDENTITY`, from the unheaded-keygen package every service package depends
# on; an existing key is never touched, rotation is an operator step,
# deploy/systemd/README.md#keys-adr-103); reload units, and restart the
# service if it was running (an upgrade then runs the new binary; a stopped
# service stays stopped).
deb_postinst() {
	local keygen=""
	if [ -n "${2:-}" ]; then
		keygen="	if [ ! -e \"\$KEYDIR/$2.key\" ]; then
		\"\$KEYGEN\" -owner $3 $2
	fi
"
	fi
	cat <<SCRIPT
#!/bin/sh
set -e
KEYGEN=/opt/unheaded/bin/unheaded-keygen
KEYDIR=/etc/unheaded/keys
case "\$1" in
configure)
	systemd-sysusers /usr/lib/sysusers.d/$1.conf
${keygen}	systemctl daemon-reload
	systemctl try-restart $1.service || true
	;;
esac
SCRIPT
}

# deb_conffiles SVC PKG_DIR REPO_ROOT: install the config files the service's
# unit reads into the package tree and list them in DEBIAN/conffiles, so dpkg
# keeps local edits on upgrade. Wotan's unit reads --topic-config
# /etc/unheaded/wotan.yaml; the package did not ship it, so on east no
# publisher was ever auto-approved (found 2026-10-20).
deb_conffiles() {
	case "$1" in
	wotan)
		install -D -m 0644 "$3/configs/wotan.yaml" "$2/etc/unheaded/wotan.yaml"
		echo /etc/unheaded/wotan.yaml >"$2/DEBIAN/conffiles"
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

# deb_preinst PKG: the ADR-098 enforcing gate (S-WELL-SPRING Step 276). On
# install or upgrade, refuse when the host's baseline agent says enforcing is
# in effect and left critical deviations (report.json: "mode_effective":
# "enforcing" and "critical_deviations" above 0): installing more on a host
# that is out of its baseline is what enforcing exists to stop. Installs
# only; refusing to START services on such a host is E-E Q5. Audit mode, no
# agent, or no report: the install goes ahead. A report claiming enforcing
# whose count cannot be read blocks (fail closed); the agent flags a report
# changed behind its back as a critical event on its next run. Recovery
# override, which the refusal names: UNHEADED_BASELINE_GATE=off apt install ...
deb_preinst() {
	cat <<SCRIPT
#!/bin/sh
set -e
REPORT=\${UNHEADED_BASELINE_REPORT:-/var/lib/unheaded/baseline/report.json}
case "\$1" in
install|upgrade)
	[ -r "\$REPORT" ] || exit 0
	grep -Eq '"mode_effective"[[:space:]]*:[[:space:]]*"enforcing"' "\$REPORT" || exit 0
	crit=\$(grep -Eo '"critical_deviations"[[:space:]]*:[[:space:]]*[0-9]+' "\$REPORT" | grep -Eo '[0-9]+\$' | head -n 1)
	if [ "\${crit:-1}" != 0 ]; then
		if [ "\${UNHEADED_BASELINE_GATE:-}" = off ]; then
			echo "$1: baseline gate overridden (UNHEADED_BASELINE_GATE=off); host is enforcing with \${crit:-unknown} critical deviation(s)" >&2
			exit 0
		fi
		echo "$1: refusing to install: this host's baseline is enforcing and \${crit:-unknown} critical deviation(s) remain (\$REPORT, ADR-098). Fix or undo them, or set UNHEADED_BASELINE_GATE=off." >&2
		exit 1
	fi
	;;
esac
exit 0
SCRIPT
}
