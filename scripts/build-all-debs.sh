#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# build-all-debs.sh — Build .deb packages for all Unheaded services
# ADR-026: .deb Packaging Pipeline. Binaries come from
# scripts/build-binaries.sh (ADR-085); a binary that fails to build fails
# this script instead of being skipped.
set -euo pipefail
# Package modes must not depend on the builder's umask (0002 made unit
# files 0664 and directories 0775 inside the .deb).
umask 022

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-1.0.0}"
# Not a fixed /tmp path: another local user could pre-create it.
OUTPUT_DIR="${DEB_OUTPUT_DIR:-${PROJECT_ROOT}/dist/debs}"
mkdir -p "$OUTPUT_DIR"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
SERVICES=(wotan unheaded-daemon dashboard-backend kanban-app akira timeguru captain architect micromanager)

echo "=============================================="
echo "UNHEADED .DEB BUILD — v${VERSION}"
echo "=============================================="

echo "Building service binaries..."
cd "$PROJECT_ROOT"
"$PROJECT_ROOT/scripts/build-binaries.sh" --out "$BUILD_DIR" "${SERVICES[@]}"

# deploy/systemd unit for each packaged binary
unit_for() {
    case "$1" in
    dashboard-backend) echo unheaded-dashboard.service ;;
    kanban-app) echo unheaded-kanban.service ;;
    unheaded-daemon) echo unheaded-daemon.service ;;
    *) echo "unheaded-$1.service" ;;
    esac
}

# Build .deb for each service
BUILT=0
for svc in "${SERVICES[@]}"; do
    BIN="${BUILD_DIR}/${svc}"

    PKG_NAME="unheaded-${svc}"
    PKG_DIR="${OUTPUT_DIR}/${PKG_NAME}_${VERSION}"
    rm -rf "$PKG_DIR"
    mkdir -p "$PKG_DIR/DEBIAN"
    mkdir -p "$PKG_DIR/opt/unheaded/bin"
    mkdir -p "$PKG_DIR/etc/systemd/system"

    # Binary
    cp "$BIN" "$PKG_DIR/opt/unheaded/bin/${svc}"
    chmod 755 "$PKG_DIR/opt/unheaded/bin/${svc}"

    # Control
    cat > "$PKG_DIR/DEBIAN/control" <<EOF
Package: ${PKG_NAME}
Version: ${VERSION}
Architecture: amd64
Maintainer: Stevie Bellis <stevie@bellis.tech>
Description: Unheaded ${svc} service
 Part of the Unheaded Kingdom infrastructure platform.
Depends: libc6
Section: admin
Priority: optional
EOF

    # Systemd unit: the repo's unit (own user, sandbox), binary path moved to
    # where the package puts it. Unit name stays the package name.
    UNIT_SRC="$PROJECT_ROOT/deploy/systemd/$(unit_for "$svc")"
    sed 's|/usr/local/bin/|/opt/unheaded/bin/|g' "$UNIT_SRC" >"$PKG_DIR/etc/systemd/system/${PKG_NAME}.service"

    # The service's own nologin account (one line of deploy/sysusers.d/unheaded.conf).
    SVC_USER="$(sed -n 's/^User=//p' "$UNIT_SRC")"
    mkdir -p "$PKG_DIR/usr/lib/sysusers.d"
    awk -v u="$SVC_USER" '$1 == "u" && $2 == u' "$PROJECT_ROOT/deploy/sysusers.d/unheaded.conf" \
        >"$PKG_DIR/usr/lib/sysusers.d/${PKG_NAME}.conf"
    [ -s "$PKG_DIR/usr/lib/sysusers.d/${PKG_NAME}.conf" ] || { echo "  FAIL: $SVC_USER not in deploy/sysusers.d/unheaded.conf" >&2; exit 1; }

    # Postinst: create the account; systemd creates and owns the StateDirectory.
    cat > "$PKG_DIR/DEBIAN/postinst" <<POSTINST
#!/bin/sh
set -e
systemd-sysusers /usr/lib/sysusers.d/${PKG_NAME}.conf
systemctl daemon-reload
systemctl try-restart ${PKG_NAME}.service || true
POSTINST
    chmod 755 "$PKG_DIR/DEBIAN/postinst"

    # Prerm
    PKG_SVC="${PKG_NAME}"
    cat > "$PKG_DIR/DEBIAN/prerm" <<PRERM
#!/bin/sh
set -e
systemctl stop ${PKG_SVC} 2>/dev/null || true
systemctl disable ${PKG_SVC} 2>/dev/null || true
PRERM
    chmod 755 "$PKG_DIR/DEBIAN/prerm"

    # Build .deb
    dpkg-deb --root-owner-group --build "$PKG_DIR" 2>/dev/null
    SIZE=$(du -h "${PKG_DIR}.deb" | awk '{print $1}')
    echo "  Built: ${PKG_NAME}_${VERSION}.deb (${SIZE})"
    BUILT=$((BUILT + 1))
done

echo ""
echo "=============================================="
echo "BUILD COMPLETE: ${BUILT} packages"
echo "=============================================="
ls -lh "${OUTPUT_DIR}"/*.deb 2>/dev/null
echo ""
echo "To publish: reprepro -b /var/lib/apt-repo includedeb noble ${OUTPUT_DIR}/*.deb"
