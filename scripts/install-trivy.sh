#!/bin/sh
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# install-trivy.sh — put the pinned trivy in a directory, or fail.
#
# For the Jenkins agents; GitHub Actions runs trivy through trivy-action. The
# release tarball is checked against TRIVY_SHA256 in lib/tool-pins.sh before
# anything in it is unpacked. POSIX sh, not lib/fetch-tool.sh, because the
# alpine agents have no bash, and they have wget where others have curl.
#
#   scripts/install-trivy.sh DIR
set -eu

DEST="${1:?usage: install-trivy.sh DIR}"
SCRIPTS="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/lib/tool-pins.sh
. "${SCRIPTS}/lib/tool-pins.sh"

[ "$(uname -sm)" = "Linux x86_64" ] || {
	echo "[FAIL] trivy is pinned for Linux x86_64 only, not $(uname -sm)" >&2
	exit 2
}
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
URL="https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz"
if command -v curl >/dev/null 2>&1; then
	curl -fsSL --retry 3 -o "$WORK/trivy.tar.gz" "$URL"
else
	wget -q -O "$WORK/trivy.tar.gz" "$URL"
fi
echo "${TRIVY_SHA256}  ${WORK}/trivy.tar.gz" | sha256sum -c - >/dev/null
tar -xzf "$WORK/trivy.tar.gz" -C "$WORK" trivy
mkdir -p "$DEST"
cp "$WORK/trivy" "$DEST/trivy"
chmod 0755 "$DEST/trivy"
echo "[PASS] trivy ${TRIVY_VERSION} in ${DEST}"
