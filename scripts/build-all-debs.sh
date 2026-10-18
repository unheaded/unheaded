#!/bin/bash
# build-all-debs.sh — Build .deb packages for all Unheaded services
# ADR-026: .deb Packaging Pipeline
set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-1.0.0}"
OUTPUT_DIR="/tmp/unheaded-debs"
mkdir -p "$OUTPUT_DIR"

echo "=============================================="
echo "UNHEADED .DEB BUILD — v${VERSION}"
echo "=============================================="

# Build all Go binaries first
echo "Building Go binaries..."
cd "$PROJECT_ROOT"
go build -ldflags="-s -w" ./... 2>&1 | tail -3
echo "  Go build complete"


# Build binary for each service
echo ""
echo "Building service binaries..."
go build -ldflags="-s -w" -o /tmp/build-wotan ./services/wotan/cmd/wotan/ 2>/dev/null && echo "  wotan: OK" || echo "  wotan: SKIP"
go build -ldflags="-s -w" -o /tmp/build-unheaded-daemon ./cmd/unheaded-daemon/ 2>/dev/null && echo "  daemon: OK" || echo "  daemon: SKIP"
go build -ldflags="-s -w" -o /tmp/build-dashboard-backend ./cmd/dashboard-backend/ 2>/dev/null && echo "  dashboard: OK" || echo "  dashboard: SKIP"
go build -ldflags="-s -w" -o /tmp/build-kanban-app ./cmd/kanban-app/ 2>/dev/null && echo "  kanban: OK" || echo "  kanban: SKIP"
go build -ldflags="-s -w" -o /tmp/build-akira ./cmd/akira/ 2>/dev/null && echo "  akira: OK" || echo "  akira: SKIP"
go build -ldflags="-s -w" -o /tmp/build-timeguru ./services/timeguru/cmd/timeguru/ 2>/dev/null && echo "  timeguru: OK" || echo "  timeguru: SKIP"
go build -ldflags="-s -w" -o /tmp/build-captain ./services/captain/cmd/captain/ 2>/dev/null && echo "  captain: OK" || echo "  captain: SKIP"
go build -ldflags="-s -w" -o /tmp/build-architect ./services/architect/cmd/architect/ 2>/dev/null && echo "  architect: OK" || echo "  architect: SKIP"
go build -ldflags="-s -w" -o /tmp/build-micromanager ./services/micromanager/cmd/micromanager/ 2>/dev/null && echo "  micromanager: OK" || echo "  micromanager: SKIP"

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
for svc in wotan unheaded-daemon dashboard-backend kanban-app akira timeguru captain architect micromanager; do
    BIN="/tmp/build-${svc}"
    if [ ! -f "$BIN" ]; then
        echo "  SKIP: $svc (binary not found)"
        continue
    fi

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
ls -lh ${OUTPUT_DIR}/*.deb 2>/dev/null
echo ""
echo "To publish: reprepro -b /var/lib/apt-repo includedeb noble ${OUTPUT_DIR}/*.deb"
