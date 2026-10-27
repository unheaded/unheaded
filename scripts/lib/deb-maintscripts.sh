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

# deb_prerm PKG: stop and disable only when the package is being removed.
# dpkg also runs prerm with "upgrade" on every upgrade; disabling there left
# the service disabled at the next boot after any apt upgrade.
deb_prerm() {
	cat <<SCRIPT
#!/bin/sh
set -e
case "\$1" in
remove|deconfigure)
	systemctl stop $1.service 2>/dev/null || true
	systemctl disable $1.service 2>/dev/null || true
	;;
esac
SCRIPT
}
