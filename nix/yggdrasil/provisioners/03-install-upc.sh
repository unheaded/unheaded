#!/bin/bash
# 03-install-upc.sh — Install Kingdom apt source + required UPC packages.
# Free to use. Free to share.
set -euo pipefail

KINGDOM_APT_URL="${KINGDOM_APT_URL:-https://apt.unheaded.dev/yggdrasil}"
APT_SOURCE_DIR="/etc/apt/sources.list.d"
KEYRING_DIR="/etc/apt/trusted.gpg.d"
DEBIAN_CODENAME="${DEBIAN_CODENAME:-bookworm}"

echo "=== Apply UPC overlay patches (apt source + required packages) ==="
# The overlay/upc/series ships the apt source list + required-packages list.
# Provisioner 02 already ran quilt push; the files are now in place. We just
# verify + apt-update + install.

if [ ! -f "$APT_SOURCE_DIR/unheaded-upc.list" ]; then
    echo "WARN: apt source not in place; installing from /tmp/yggdrasil-upc/"
    if [ -f /tmp/yggdrasil-upc/0001-add-upc-apt-source.patch ]; then
        # Parse the patch and install the source list directly
        cat <<EOF > "$APT_SOURCE_DIR/unheaded-upc.list"
# Yggdrasil — Unheaded Protocol Computer apt source
# Repo built by task #65 (Debian hardening pipeline).
# Free to use. Free to share.
deb [signed-by=$KEYRING_DIR/unheaded-upc.gpg] $KINGDOM_APT_URL $DEBIAN_CODENAME main
EOF
    fi
fi

echo "=== Install UPC repo signing key ==="
# In production the key comes from a HSM-backed location. For the scaffold
# we accept it via a build-host file mount. If missing, fall back to a
# placeholder that fails the package install — surfacing the missing key
# loudly rather than silently shipping an unsigned-source image.
if [ ! -f "$KEYRING_DIR/unheaded-upc.gpg" ]; then
    # The pipeline must inject this. Touch a placeholder so apt sees it
    # but flag it for the lynis gate to catch.
    touch "$KEYRING_DIR/unheaded-upc.gpg"
    echo "WARN: unheaded-upc.gpg placeholder created — production pipeline must replace before publishing"
fi

echo "=== apt-get update ==="
DEBIAN_FRONTEND=noninteractive apt-get update -q

echo "=== Install required UPC packages ==="
REQUIRED_PKGS_FILE="/etc/yggdrasil/required-packages.d/upc.list"
if [ -f "$REQUIRED_PKGS_FILE" ]; then
    # shellcheck disable=SC2046
    DEBIAN_FRONTEND=noninteractive apt-get install -y -q $(grep -v '^#' "$REQUIRED_PKGS_FILE" | tr '\n' ' ')
else
    echo "WARN: required-packages list missing at $REQUIRED_PKGS_FILE"
    echo "      Falling back to the canonical list from overlay/upc/"
    DEBIAN_FRONTEND=noninteractive apt-get install -y -q \
        upc-bootctl upc-tty-bridge monad-cpu-ebpf unheaded-shared unheaded-runner yggdrasil-evidence
fi

echo "=== Verify UPC binaries on PATH ==="
for BIN in upc-bootctl upc-tty-bridge yggdrasil-evidence; do
    if ! command -v "$BIN" >/dev/null 2>&1; then
        echo "FAIL: $BIN not on PATH after install"
        exit 1
    fi
    echo "OK: $(command -v "$BIN")"
done

echo "=== Verify monad-cpu-ebpf.o present ==="
test -f /opt/unheaded/share/monad-cpu-ebpf.o || {
    echo "FAIL: /opt/unheaded/share/monad-cpu-ebpf.o missing"
    exit 1
}
echo "OK: monad-cpu-ebpf.o present ($(stat -c%s /opt/unheaded/share/monad-cpu-ebpf.o) bytes)"

echo "=== Step 03 complete ==="
