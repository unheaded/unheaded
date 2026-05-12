#!/bin/bash
# publish.sh — Publish .deb artifacts to the Yggdrasil apt repo.
# Called by the Jenkins pipeline on tag builds.
#
# Repo layout (reprepro-managed):
#   apt.unheaded.dev/yggdrasil/
#     dists/bookworm/main/binary-amd64/Packages.gz
#     pool/main/u/upc-bootctl/upc-bootctl_<version>_amd64.deb
#     pool/main/u/upc-tty-bridge/upc-tty-bridge_<version>_amd64.deb
#     pool/main/m/monad-cpu-ebpf/monad-cpu-ebpf_<version>_all.deb
#     pool/main/u/unheaded-shared/unheaded-shared_<version>_all.deb
#     pool/main/u/unheaded-runner/unheaded-runner_<version>_amd64.deb
#     pool/main/y/yggdrasil-evidence/yggdrasil-evidence_<version>_amd64.deb
#
# Status: SCAFFOLD. Each package is built by its parent crate's standard
# `cargo deb` / `dpkg-buildpackage` recipe (TBD per crate); this script
# only handles the publish step.
#
# Free to use. Free to share.
set -euo pipefail

YGGDRASIL_VERSION="${1:-${YGGDRASIL_VERSION:-0.0.0-scaffold}}"
REPO_BASE="${REPO_BASE:-/srv/apt.unheaded.dev/yggdrasil}"
GPG_KEY_ID="${GPG_KEY_ID:-yggdrasil-build-key}"
DEBIAN_CODENAME="${DEBIAN_CODENAME:-bookworm}"

ARTIFACTS_DIR="${ARTIFACTS_DIR:-build/debs}"

echo "=== Yggdrasil apt repo publish — version ${YGGDRASIL_VERSION} ==="

if [ ! -d "$REPO_BASE/conf" ]; then
    echo "WARN: repo base $REPO_BASE not initialized. Running first-time init..."
    install -d -m 0755 "$REPO_BASE/conf" "$REPO_BASE/dists" "$REPO_BASE/pool/main"

    cat > "$REPO_BASE/conf/distributions" <<EOF
Origin: Unheaded Kingdom
Label: Yggdrasil
Codename: $DEBIAN_CODENAME
Architectures: amd64 source
Components: main
Description: Yggdrasil hardened-Debian apt repo for UPC tooling
SignWith: $GPG_KEY_ID
EOF
fi

if ! command -v reprepro >/dev/null 2>&1; then
    echo "FAIL: reprepro not installed on builder. Install via 'apt-get install reprepro'"
    exit 1
fi

if [ ! -d "$ARTIFACTS_DIR" ]; then
    echo "WARN: no $ARTIFACTS_DIR — nothing to publish"
    echo "      Real build pipeline should emit .deb artifacts here from each kingdom crate's"
    echo "      'cargo deb' or 'dpkg-buildpackage' step BEFORE invoking this publish.sh"
    exit 0
fi

for DEB in "$ARTIFACTS_DIR"/*.deb; do
    [ -f "$DEB" ] || continue
    echo "Publishing $(basename "$DEB")..."
    reprepro -b "$REPO_BASE" includedeb "$DEBIAN_CODENAME" "$DEB"
done

# Update + sign the index
reprepro -b "$REPO_BASE" export
reprepro -b "$REPO_BASE" check
reprepro -b "$REPO_BASE" verify

echo "=== Publish complete. Repo at $REPO_BASE/ ==="
echo "Clients install: apt-get update && apt-get install upc-bootctl upc-tty-bridge ..."
