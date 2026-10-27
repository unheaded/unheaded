#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# build-all-debs.sh — Build the .deb packages (ADR-026, S-IRON-GATE Phase 14)
#
#   scripts/build-all-debs.sh [VERSION]       # every package into dist/debs
#   DEB_OUTPUT_DIR=DIR scripts/build-all-debs.sh 0.2.0~alpha+test
#   DEB_SIGN_KEY=<keyid> scripts/build-all-debs.sh   # + signed SHA256SUMS
#   scripts/build-all-debs.sh --control       # debian/control, from the same table
#
# What is packaged, under which name, with which unit, account and key comes
# from scripts/lib/deb-packages.sh; the maintainer scripts from
# scripts/lib/deb-maintscripts.sh. Binaries come from
# scripts/build-binaries.sh (ADR-085); a binary that fails to build fails
# this script instead of being skipped. Only .deb files (and SHA256SUMS)
# land in the output directory; package trees are staged in a temp dir.
set -euo pipefail
# Package modes must not depend on the builder's umask (0002 made unit
# files 0664 and directories 0775 inside the .deb).
umask 022

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib/deb-maintscripts.sh
. "$PROJECT_ROOT/scripts/lib/deb-maintscripts.sh"
# shellcheck source=scripts/lib/deb-packages.sh
. "$PROJECT_ROOT/scripts/lib/deb-packages.sh"

if [[ "${1:-}" == --control ]]; then
    deb_control "$PROJECT_ROOT"
    exit 0
fi

# Default version: v0.2.0-alpha in Debian form (`~` sorts before the release,
# so 0.2.0~alpha < 0.2.0), plus the commit it was built from. The commit
# time keeps successive builds ordered for apt; the hash names the commit.
# A dirty tree has no commit to name, so it needs an explicit version.
if [[ -n "${1:-}" ]]; then
    VERSION="$1"
else
    [[ -z "$(git -C "$PROJECT_ROOT" status --porcelain)" ]] || {
        echo "FAIL: uncommitted changes; commit first or pass a version" >&2
        exit 1
    }
    VERSION="0.2.0~alpha+$(TZ=UTC git -C "$PROJECT_ROOT" show -s --format=%cd --date=format-local:%Y%m%d%H%M%S HEAD).g$(git -C "$PROJECT_ROOT" rev-parse --short=8 HEAD)"
fi
[[ "$VERSION" =~ ^[0-9][A-Za-z0-9.+~-]*$ ]] || { echo "FAIL: '$VERSION' is not a Debian version" >&2; exit 1; }

# Not a fixed /tmp path: another local user could pre-create it.
OUTPUT_DIR="${DEB_OUTPUT_DIR:-${PROJECT_ROOT}/dist/debs}"
mkdir -p "$OUTPUT_DIR"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
BIN_DIR="$BUILD_DIR/bin"
STAGE="$BUILD_DIR/pkg"
mkdir -p "$STAGE"
BUILT=()

echo "=============================================="
echo "UNHEADED .DEB BUILD — v${VERSION}"
echo "=============================================="

echo "Building binaries..."
cd "$PROJECT_ROOT"
"$PROJECT_ROOT/scripts/build-binaries.sh" --out "$BIN_DIR" "${DEB_SERVICES[@]}" unheaded-keygen

# control PKG ARCH DEPENDS DESCRIPTION BODY [EXTRA_FIELDS]
control() {
    cat <<EOF
Package: $1
Version: ${VERSION}
Architecture: $2
Maintainer: Stevie Bellis <stevie@bellis.tech>
Depends: $3
${6:+$6
}Section: admin
Priority: optional
Description: $4
 $5
EOF
}

# build_deb PKG ARCH: dpkg-deb the staged tree into the output directory.
build_deb() {
    local deb="${OUTPUT_DIR}/${1}_${VERSION}_${2}.deb"
    dpkg-deb --root-owner-group --build "$STAGE/$1" "$deb" >/dev/null
    BUILT+=("$deb")
    echo "  Built: $(basename "$deb") ($(du -h "$deb" | awk '{print $1}'))"
}

# unheaded-keygen ships once, in its own package, and every package that
# creates a key depends on it: its postinst creates the service's key with
# it (ADR-103). One file in several packages would make dpkg refuse the
# second.
P="$STAGE/unheaded-keygen"
mkdir -p "$P/DEBIAN"
install -D -m 0755 "$BIN_DIR/unheaded-keygen" "$P/opt/unheaded/bin/unheaded-keygen"
control unheaded-keygen amd64 libc6 "Unheaded service key tool (ADR-103)" \
    "Creates, rotates and retires each service's Ed25519 key pair." >"$P/DEBIAN/control"
build_deb unheaded-keygen amd64

# unheaded-configs: the runbooks and the runbook runner (Akira's cert-expiry
# exec check runs /opt/unheaded/scripts/run-runbook.py on
# /opt/unheaded/runbooks/pki/akira-cert-expiry-check.yaml, configs/akira-*.yaml).
P="$STAGE/unheaded-configs"
mkdir -p "$P/DEBIAN" "$P/opt/unheaded/runbooks"
cp -R "$PROJECT_ROOT/runbooks/." "$P/opt/unheaded/runbooks/"
find "$P/opt/unheaded/runbooks" -name __pycache__ -prune -exec rm -rf {} +
find "$P/opt/unheaded/runbooks" -type d -exec chmod 0755 {} + -o -type f -exec chmod 0644 {} +
install -D -m 0755 "$PROJECT_ROOT/scripts/run-runbook.py" "$P/opt/unheaded/scripts/run-runbook.py"
install -D -m 0644 "$PROJECT_ROOT/scripts/svc_token.py" "$P/opt/unheaded/scripts/svc_token.py"
control unheaded-configs all "python3, python3-yaml, openssl" "Unheaded runbooks and the runbook runner" \
    "The runbooks under /opt/unheaded/runbooks and run-runbook.py, which Akira's cert-expiry exec check runs." >"$P/DEBIAN/control"
build_deb unheaded-configs all

for svc in "${DEB_SERVICES[@]}"; do
    PKG_NAME="$(deb_pkg_name "$svc")"
    UNIT="$(deb_unit_for "$svc")"
    UNIT_SRC="$PROJECT_ROOT/deploy/systemd/$UNIT"
    [[ -f "$UNIT_SRC" ]] || { echo "  FAIL: $svc: no unit deploy/systemd/$UNIT" >&2; exit 1; }
    P="$STAGE/$PKG_NAME"
    mkdir -p "$P/DEBIAN" "$P/etc/systemd/system" "$P/usr/lib/sysusers.d"

    install -D -m 0755 "$BIN_DIR/$svc" "$P/opt/unheaded/bin/$svc"

    # The repo's units (own account, sandbox) under their repo names, binary
    # path moved to where the package puts it.
    EXTRA=()
    read -r -a EXTRA <<<"$(deb_extra_units "$svc")"
    for u in "$UNIT" "${EXTRA[@]}"; do
        sed 's|/usr/local/bin/|/opt/unheaded/bin/|g' "$PROJECT_ROOT/deploy/systemd/$u" >"$P/etc/systemd/system/$u"
    done

    # The service's own nologin account (one line of deploy/sysusers.d/unheaded.conf).
    SVC_USER="$(sed -n 's/^User=//p' "$UNIT_SRC")"
    awk -v u="$SVC_USER" '$1 == "u" && $2 == u' "$PROJECT_ROOT/deploy/sysusers.d/unheaded.conf" \
        >"$P/usr/lib/sysusers.d/${PKG_NAME}.conf"
    [[ -s "$P/usr/lib/sysusers.d/${PKG_NAME}.conf" ]] || { echo "  FAIL: $svc: User=$SVC_USER not in deploy/sysusers.d/unheaded.conf" >&2; exit 1; }

    EXTRA_FIELDS=""
    [[ "$svc" != akira ]] || EXTRA_FIELDS="Recommends: unheaded-configs"
    FORMER_PKG="$(deb_former_pkgs "$svc")"
    if [[ -n "$FORMER_PKG" ]]; then
        EXTRA_FIELDS="${EXTRA_FIELDS:+$EXTRA_FIELDS
}Conflicts: $FORMER_PKG
Replaces: $FORMER_PKG"
    fi
    DEPENDS="$(deb_depends "$svc")"
    control "$PKG_NAME" amd64 "${DEPENDS//\$\{VERSION\}/$VERSION}" "$(deb_description "$UNIT_SRC")" \
        "Binary $svc, unit $UNIT." "$EXTRA_FIELDS" >"$P/DEBIAN/control"

    # Maintainer scripts (scripts/lib/deb-maintscripts.sh): preinst is the
    # ADR-098 enforcing gate and records a former unit's state; postinst
    # creates the account and, on first install, the service's key owned by
    # it, carries a former unit's state over, and restarts a running service
    # only when its TLS material is in place; prerm stops and disables only
    # on removal, never on upgrade.
    ARGS=(--pkg "$PKG_NAME" --unit "$UNIT")
    ID="$(deb_identity_for "$svc")"
    [[ -z "$ID" ]] || ARGS+=(--identity "$ID" --owner "$SVC_USER")
    while read -r f; do
        [[ -z "$f" ]] || ARGS+=(--require "$f")
    done < <(deb_tls_files "$UNIT_SRC")
    PRE=(--pkg "$PKG_NAME")
    FORMER=()
    read -r -a FORMER <<<"$(deb_former_units "$svc")"
    for f in "${FORMER[@]}"; do
        ARGS+=(--former "$f")
        PRE+=(--former "$f")
    done
    deb_postinst "${ARGS[@]}" >"$P/DEBIAN/postinst"
    deb_preinst "${PRE[@]}" >"$P/DEBIAN/preinst"
    # Timers and paths stop first, so they cannot start the service again.
    deb_prerm "$PKG_NAME" "${EXTRA[@]}" "$UNIT" >"$P/DEBIAN/prerm"
    chmod 755 "$P/DEBIAN/postinst" "$P/DEBIAN/prerm" "$P/DEBIAN/preinst"
    if [[ ${#FORMER[@]} -gt 0 ]]; then
        deb_postrm "${PRE[@]}" >"$P/DEBIAN/postrm"
        chmod 755 "$P/DEBIAN/postrm"
    fi
    deb_conffiles "$svc" "$P" "$PROJECT_ROOT"

    build_deb "$PKG_NAME" amd64
done

# Optional per-build signature (belt and braces; the apt repo's signed
# Release file is the trust anchor, scripts/apt-repo.sh): SHA256SUMS over
# this build's debs and a detached GPG signature of it. Verify with
#   gpg --verify SHA256SUMS.gpg SHA256SUMS && sha256sum -c SHA256SUMS
if [[ -n "${DEB_SIGN_KEY:-}" ]]; then
    (cd "$OUTPUT_DIR" && for d in "${BUILT[@]}"; do sha256sum "$(basename "$d")"; done) >"$OUTPUT_DIR/SHA256SUMS"
    rm -f "$OUTPUT_DIR/SHA256SUMS.gpg"
    gpg --batch --yes --local-user "$DEB_SIGN_KEY" --detach-sign --output "$OUTPUT_DIR/SHA256SUMS.gpg" "$OUTPUT_DIR/SHA256SUMS"
    echo "  Signed: SHA256SUMS ($DEB_SIGN_KEY)"
fi

echo ""
echo "=============================================="
echo "BUILD COMPLETE: ${#BUILT[@]} packages"
echo "=============================================="
echo ""
echo "To publish: scripts/apt-repo.sh include ${OUTPUT_DIR}/*_${VERSION}_*.deb (runbooks/deploy/apt-repo-publish.yaml)"
