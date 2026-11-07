#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# ci-install-tools.sh — install release-pinned tools into DIR for CI jobs.
#
# Replaces `curl .../main/install.sh | sh` (syft, grype: whatever was on each
# project's main branch, run on a runner holding a security-events: write
# token). Versions and sha256s are in
# lib/tool-pins.sh; lib/fetch-tool.sh checks the hash on every use;
# scripts/check-tool-freshness.sh keeps them current.
#
#   scripts/ci-install-tools.sh DIR syft|grype|cosign|buf...
set -euo pipefail

dir="${1:?usage: $0 DIR syft|grype|cosign|buf...}"
shift
cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib/tool-pins.sh
source scripts/lib/tool-pins.sh
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
	cosign) bin="$(fetch_tool cosign "$COSIGN_VERSION" \
		"https://github.com/sigstore/cosign/releases/download/v${COSIGN_VERSION}/cosign-linux-amd64" \
		"$COSIGN_SHA256" -)" ;;
	buf) bin="$(fetch_tool buf "$BUF_VERSION" \
		"https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64" \
		"$BUF_SHA256" -)" ;;
	*) echo "[FAIL] unknown tool: $tool" >&2; exit 2 ;;
	esac
	install -m 0755 "$bin" "$dir/$tool"
	echo "[ok] $tool -> $dir/$tool"
done
