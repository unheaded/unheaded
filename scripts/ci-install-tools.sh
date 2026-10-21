#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# ci-install-tools.sh — install release-pinned tools into DIR for CI jobs.
#
# Replaces `curl .../main/install.sh | sh` (syft, grype: whatever was on each
# project's main branch, run on a runner holding a security-events: write
# token), azure/setup-helm at Helm 3.14.0, and kubeconform from
# releases/latest with no checksum. Versions and sha256s are in
# lib/tool-pins.sh; lib/fetch-tool.sh checks the hash on every use;
# scripts/check-tool-freshness.sh keeps them current.
#
#   scripts/ci-install-tools.sh DIR syft|grype|helm|kubeconform...
set -euo pipefail

dir="${1:?usage: $0 DIR syft|grype|helm|kubeconform...}"
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
	helm) bin="$(fetch_tool helm "$HELM_VERSION" \
		"https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" \
		"$HELM_SHA256" linux-amd64/helm)" ;;
	kubeconform) bin="$(fetch_tool kubeconform "$KUBECONFORM_VERSION" \
		"https://github.com/yannh/kubeconform/releases/download/v${KUBECONFORM_VERSION}/kubeconform-linux-amd64.tar.gz" \
		"$KUBECONFORM_SHA256" kubeconform)" ;;
	*) echo "[FAIL] unknown tool: $tool" >&2; exit 2 ;;
	esac
	install -m 0755 "$bin" "$dir/$tool"
	echo "[ok] $tool -> $dir/$tool"
done
