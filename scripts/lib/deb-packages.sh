#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# The .deb package table (ADR-026, S-IRON-GATE Phase 14): which binaries are
# packaged, under which package name, with which unit, account and key.
# scripts/build-all-debs.sh builds from it and `build-all-debs.sh --control`
# renders debian/control from it, so the two cannot drift. Tested by
# scripts/test-deb-maintscripts.sh.
#
# Rules:
#   - every binary a deploy/systemd unit runs from /usr/local/bin (what
#     scripts/build-binaries.sh --check-units maps) is in DEB_SERVICES;
#   - package name: the binary name when it already starts with "unheaded-"
#     (unheaded-daemon, not unheaded-unheaded-daemon), else unheaded-<binary>;
#   - the unit ships under the repo's own unit name (deploy/systemd), the name
#     Akira's remediation (`systemctl restart unheaded-<target>`), the
#     runbooks and the hand-installed hosts use;
#   - unheaded-keygen ships once, in its own package, and every package that
#     creates a key depends on it; unheaded-configs ships the runbook runner
#     and the runbooks (Akira's cert-expiry exec check).

# Binaries packaged with their unit, in build order.
DEB_SERVICES=(
	wotan unheaded-daemon dashboard-backend kanban-app akira
	timeguru captain architect micromanager monad sophia
	unheaded-auth heimdall-daemon sword chaos-controller anomaly kev-poller
	trace-collector-go unheaded-baseline huginn zhen-agentd well-retention
)

# Packages with no unit of their own (deb_control writes their stanzas).
# shellcheck disable=SC2034  # read by scripts/test-deb-maintscripts.sh
DEB_EXTRA_PACKAGES=(unheaded-keygen unheaded-configs)

# deb_pkg_name BINARY: the package name.
deb_pkg_name() {
	case "$1" in
	unheaded-*) echo "$1" ;;
	*) echo "unheaded-$1" ;;
	esac
}

# deb_unit_for BINARY: the deploy/systemd unit that runs it.
deb_unit_for() {
	case "$1" in
	dashboard-backend) echo unheaded-dashboard.service ;;
	kanban-app) echo unheaded-kanban.service ;;
	unheaded-daemon | unheaded-auth | unheaded-baseline) echo "$1.service" ;;
	heimdall-daemon) echo unheaded-heimdall.service ;;
	trace-collector-go) echo unheaded-trace-collector.service ;;
	huginn | zhen-agentd) echo "$1.service" ;;
	*) echo "unheaded-$1.service" ;;
	esac
}

# deb_extra_units BINARY: further units the package ships (timers, paths).
# Shipped, never enabled by the package.
deb_extra_units() {
	case "$1" in
	unheaded-baseline) echo unheaded-baseline.timer unheaded-baseline.path ;;
	well-retention) echo unheaded-well-retention.timer ;;
	esac
}

# deb_identity_for BINARY: the auth identity the binary runs as (the key its
# postinst creates: the name it passes to auth.LoadServiceAuthConfig or
# wotanClient.InitServiceIdentity). Empty: the binary holds no service key
# (huginn reports to Prometheus only; well-retention logs in to the Well
# with a password).
deb_identity_for() {
	case "$1" in
	heimdall-daemon) echo heimdall ;;
	trace-collector-go) echo trace-collector ;;
	huginn | well-retention) ;;
	*) echo "$1" ;;
	esac
}

# deb_former_units BINARY: unit names earlier debs installed for this binary
# (the unit was named after the package until 2026-10-27; east runs these).
# preinst stops and disables them while their unit file still exists and
# postinst carries their enabled/active state over to the new unit.
deb_former_units() {
	case "$1" in
	dashboard-backend) echo unheaded-dashboard-backend.service ;;
	kanban-app) echo unheaded-kanban-app.service ;;
	unheaded-daemon) echo unheaded-unheaded-daemon.service ;;
	esac
}

# deb_former_pkgs BINARY: package names this package replaces (Conflicts +
# Replaces, so apt swaps them and the shared files change owner cleanly).
deb_former_pkgs() {
	case "$1" in
	unheaded-daemon) echo unheaded-unheaded-daemon ;;
	esac
}

# deb_conffile_paths BINARY: files under /etc the package ships that dpkg
# must keep local edits of (deb_conffiles installs them).
deb_conffile_paths() {
	case "$1" in
	wotan) echo /etc/unheaded/wotan.yaml ;;
	well-retention) echo /etc/unheaded/well/retention-classes.yaml ;;
	esac
}

# deb_tls_files UNIT_FILE: the TLS files the unit needs at start (CA, cert,
# key from its UNHEADED_TLS_* environment), when it runs with TLS on. A
# service refuses to start without them (no plaintext fallback), so postinst
# does not (re)start one whose files are missing.
deb_tls_files() {
	grep -q '^Environment=UNHEADED_TLS_ENABLED=true$' "$1" || return 0
	sed -n 's/^Environment=UNHEADED_TLS_\(CA_CERT\|CERT\|KEY_FILE\)=\(\/.*\)$/\2/p' "$1"
}

# deb_description UNIT_FILE: the unit's Description=, the package summary.
deb_description() {
	sed -n 's/^Description=//p' "$1" | head -n 1
}

# deb_depends BINARY: the Depends: line's value.
deb_depends() {
	local d="libc6"
	[ -z "$(deb_identity_for "$1")" ] || d="$d, unheaded-keygen (= \${VERSION})"
	echo "$d"
}

# deb_control REPO_ROOT: debian/control, one stanza per package (the dh tree;
# debian/rules builds through scripts/build-all-debs.sh).
deb_control() {
	local root=$1 svc unit d former
	cat <<'EOF'
# Generated by `scripts/build-all-debs.sh --control > debian/control`
# from scripts/lib/deb-packages.sh. Do not edit by hand.
Source: unheaded
Section: admin
Priority: optional
Maintainer: Stevie Bellis <stevie@bellis.tech>
Build-Depends: golang-go (>= 2:1.21)
Standards-Version: 4.6.2
Homepage: https://github.com/unheaded/unheaded

Package: unheaded-keygen
Architecture: amd64
Depends: libc6
Description: Unheaded service key tool (ADR-103)
 Creates, rotates and retires each service's Ed25519 key pair.

Package: unheaded-configs
Architecture: all
Depends: python3, python3-yaml, openssl
Description: Unheaded runbooks and the runbook runner
 The runbooks under /opt/unheaded/runbooks and run-runbook.py, which
 Akira's cert-expiry exec check runs.
EOF
	for svc in "${DEB_SERVICES[@]}"; do
		unit="$root/deploy/systemd/$(deb_unit_for "$svc")"
		d="$(deb_depends "$svc")"
		echo
		echo "Package: $(deb_pkg_name "$svc")"
		echo "Architecture: amd64"
		echo "Depends: ${d//\$\{VERSION\}/\$\{binary:Version\}}"
		[ "$svc" != akira ] || echo "Recommends: unheaded-configs"
		former="$(deb_former_pkgs "$svc")"
		if [ -n "$former" ]; then
			echo "Conflicts: $former"
			echo "Replaces: $former"
		fi
		echo "Description: $(deb_description "$unit")"
		echo " Binary $svc, unit $(deb_unit_for "$svc")."
	done
}
