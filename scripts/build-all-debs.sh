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
# shellcheck source=scripts/lib/deb-maintscripts.sh
. "$PROJECT_ROOT/scripts/lib/deb-maintscripts.sh"
# Default version: v0.1.0-alpha in Debian form (`~` sorts before the release,
# so 0.1.0~alpha < 0.1.0), plus the commit it was built from. The commit
# time keeps successive builds ordered for apt; the hash names the commit.
# A dirty tree has no commit to name, so it needs an explicit version.
if [[ -n "${1:-}" ]]; then
    VERSION="$1"
else
    [[ -z "$(git -C "$PROJECT_ROOT" status --porcelain)" ]] || {
        echo "FAIL: uncommitted changes; commit first or pass a version" >&2
        exit 1
    }
    VERSION="0.1.0~alpha+$(TZ=UTC git -C "$PROJECT_ROOT" show -s --format=%cd --date=format-local:%Y%m%d%H%M%S HEAD).g$(git -C "$PROJECT_ROOT" rev-parse --short=8 HEAD)"
fi
# Not a fixed /tmp path: another local user could pre-create it.
OUTPUT_DIR="${DEB_OUTPUT_DIR:-${PROJECT_ROOT}/dist/debs}"
mkdir -p "$OUTPUT_DIR"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
SERVICES=(wotan unheaded-daemon dashboard-backend kanban-app akira timeguru captain architect micromanager unheaded-auth)

echo "=============================================="
echo "UNHEADED .DEB BUILD — v${VERSION}"
echo "=============================================="

echo "Building service binaries..."
cd "$PROJECT_ROOT"
"$PROJECT_ROOT/scripts/build-binaries.sh" --out "$BUILD_DIR" "${SERVICES[@]}" unheaded-keygen

# unheaded-keygen ships once, in its own package, and every service package
# depends on it: the service postinst creates the service's key with it
# (ADR-103). One file in several packages would make dpkg refuse the second.
KG_DIR="${OUTPUT_DIR}/unheaded-keygen_${VERSION}"
rm -rf "$KG_DIR"
mkdir -p "$KG_DIR/DEBIAN" "$KG_DIR/opt/unheaded/bin"
install -m 0755 "$BUILD_DIR/unheaded-keygen" "$KG_DIR/opt/unheaded/bin/unheaded-keygen"
cat > "$KG_DIR/DEBIAN/control" <<EOF
Package: unheaded-keygen
Version: ${VERSION}
Architecture: amd64
Maintainer: Stevie Bellis <stevie@bellis.tech>
Description: Unheaded service key tool (ADR-103)
 Creates, rotates and retires each service's Ed25519 key pair.
Depends: libc6
Section: admin
Priority: optional
EOF
dpkg-deb --root-owner-group --build "$KG_DIR" 2>/dev/null
echo "  Built: unheaded-keygen_${VERSION}.deb"

# deploy/systemd unit for each packaged binary
unit_for() {
    case "$1" in
    dashboard-backend) echo unheaded-dashboard.service ;;
    kanban-app) echo unheaded-kanban.service ;;
    unheaded-daemon) echo unheaded-daemon.service ;;
    unheaded-auth) echo unheaded-auth.service ;;
    *) echo "unheaded-$1.service" ;;
    esac
}

# Build .deb for each service
BUILT=1   # unheaded-keygen
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
Depends: libc6, unheaded-keygen (= ${VERSION})
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

    # Maintainer scripts (scripts/lib/deb-maintscripts.sh): postinst creates
    # the account (systemd creates and owns the StateDirectory) and, on first
    # install, the service's key owned by that account (identity = the binary
    # name, what it passes to auth.LoadServiceAuthConfig); prerm stops and
    # disables only on removal, never on upgrade.
    deb_postinst "$PKG_NAME" "$svc" "$SVC_USER" >"$PKG_DIR/DEBIAN/postinst"
    deb_prerm "$PKG_NAME" >"$PKG_DIR/DEBIAN/prerm"
    chmod 755 "$PKG_DIR/DEBIAN/postinst" "$PKG_DIR/DEBIAN/prerm"
    deb_conffiles "$svc" "$PKG_DIR" "$PROJECT_ROOT"

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
