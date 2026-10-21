#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# ci-install-sbom-tools.sh — install pinned syft and/or grype into DIR.
#
# The weekly SBOM audit used to run `curl .../main/install.sh | sh` for both:
# whatever was on each project's main branch at that moment ran on a runner
# holding a security-events: write token. Now each is a release tarball
# pinned by version and sha256 (scripts/lib/fetch-tool.sh), kept current by
# scripts/check-tool-freshness.sh.
#
#   scripts/ci-install-sbom-tools.sh DIR syft grype
set -euo pipefail

SYFT_VERSION="1.54.1"
SYFT_SHA256="c069905b391cc4c20a5ba65ad5c10be2a7ba074f8ea6ad203e24d14e303dad47" # syft_<v>_linux_amd64.tar.gz
GRYPE_VERSION="0.120.1"
GRYPE_SHA256="0a9ee97ef5ae2ee953b0a80098105052e846cdbe319a57d808b519c33cd1343d" # grype_<v>_linux_amd64.tar.gz

dir="${1:?usage: $0 DIR syft|grype...}"
shift
cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib/fetch-tool.sh
source scripts/lib/fetch-tool.sh
mkdir -p "$dir"
for tool in "$@"; do
	case "$tool" in
	syft) bin="$(fetch_tool syft "$SYFT_VERSION" \
		"https://github.com/anchore/syft/releases/download/v${SYFT_VERSION}/syft_${SYFT_VERSION}_linux_amd64.tar.gz" \
		"$SYFT_SHA256" syft)" ;;
	grype) bin="$(fetch_tool grype "$GRYPE_VERSION" \
		"https://github.com/anchore/grype/releases/download/v${GRYPE_VERSION}/grype_${GRYPE_VERSION}_linux_amd64.tar.gz" \
		"$GRYPE_SHA256" grype)" ;;
	*) echo "[FAIL] unknown tool: $tool" >&2; exit 2 ;;
	esac
	install -m 0755 "$bin" "$dir/$tool"
	"$dir/$tool" version | sed -n '2p;/^Version/p' | head -1
done
