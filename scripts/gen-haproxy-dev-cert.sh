#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Writes a one-day self-signed P-256 certificate + key for the haproxy edge's
# dev TLS bind to docker/haproxy/edge/certs/dev.pem (mode 0600, gitignored).
# Dev only: production certificates come from the gateway's ACME path.
#
# Usage: scripts/gen-haproxy-dev-cert.sh [--force]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${REPO_ROOT}/docker/haproxy/edge/certs/dev.pem"

if [[ -e "$OUT" && "${1:-}" != "--force" ]]; then
	echo "gen-haproxy-dev-cert: $OUT exists (pass --force to replace it)" >&2
	exit 1
fi
command -v openssl >/dev/null || { echo "gen-haproxy-dev-cert: openssl not found" >&2; exit 2; }

umask 077
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
	-subj /CN=haproxy-dev -keyout "${tmp}/key.pem" -out "${tmp}/cert.pem" 2>/dev/null
cat "${tmp}/cert.pem" "${tmp}/key.pem" >"${tmp}/dev.pem"
install -m 0600 "${tmp}/dev.pem" "$OUT"
echo "gen-haproxy-dev-cert: wrote $OUT (0600, expires in 1 day)"
echo "  the edge runs as uid 16747: sudo chown 16747:16747 $OUT before compose up"
