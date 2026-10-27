#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Both haproxy configs must parse: `haproxy -c` under the host binary (when one
# is installed) and under the image the load-balancer compose file pins
# (docker/docker-compose.loadbalancers.yml). A config that does not parse is a
# load balancer that does not start; until S-CLEAN-SLATE (M-035) neither did.
#
# The edge binds TLS with `crt /etc/haproxy/certs/`, which compose mounts from
# docker/haproxy/edge/certs/ (gitignored pems). This check never reads that
# directory: it generates a throwaway P-256 self-signed cert into a mktemp dir,
# mounts that at the same path for the image run, and points a temp copy of the
# config at it for the host run.
#
# Env:
#   HAPROXY_IMAGE                 image to check under (default: the compose pin)
#   HAPROXY_CHECK_SKIP_IMAGE=1    skip the image run (only where docker is absent;
#                                 prints SKIP, never PASS, for that leg)
#
# Exit 0 when every leg passes, 1 when any config fails, 2 on a setup error.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE="${REPO_ROOT}/docker/docker-compose.loadbalancers.yml"
CONFIGS=(docker/haproxy/internal/haproxy.cfg docker/haproxy/edge/haproxy.cfg)
CERT_PATH_IN_CFG="/etc/haproxy/certs"
# A config that resolves server names through libc at check time can hang on
# DNS (no network in the container); bound every leg.
CHECK_TIMEOUT=60

pinned_image() {
	# First haproxy image the LB compose file pins.
	sed -nE 's/^[[:space:]]*image:[[:space:]]*(haproxy:[^[:space:]]+).*/\1/p' "$COMPOSE" | head -1
}

IMAGE="${HAPROXY_IMAGE:-$(pinned_image)}"
[[ -n "$IMAGE" ]] || { echo "check-haproxy-cfg: no haproxy image pinned in $COMPOSE" >&2; exit 2; }
command -v openssl >/dev/null || { echo "check-haproxy-cfg: openssl not found" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
CERTDIR="${WORK}/certs"
mkdir -m 0700 "$CERTDIR"
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
	-subj /CN=haproxy-dev -keyout "${WORK}/key.pem" -out "${WORK}/cert.pem" 2>/dev/null
cat "${WORK}/cert.pem" "${WORK}/key.pem" >"${CERTDIR}/dev.pem"
chmod 0600 "${CERTDIR}/dev.pem"

docker_ok=0
if [[ "${HAPROXY_CHECK_SKIP_IMAGE:-0}" != "1" ]]; then
	if command -v docker >/dev/null && docker info >/dev/null 2>&1; then
		docker_ok=1
	else
		echo "check-haproxy-cfg: docker unavailable; set HAPROXY_CHECK_SKIP_IMAGE=1 to skip the ${IMAGE} leg" >&2
		exit 2
	fi
fi

fail=0
report() { # leg cfg rc output
	local alerts
	alerts="$(grep -c '\[ALERT\]' <<<"$4" || true)"
	if [[ "$3" -eq 0 && "$alerts" -eq 0 ]]; then
		echo "PASS  $2  ($1)"
	else
		echo "FAIL  $2  ($1)"
		grep -E '\[(ALERT|WARNING)\]' <<<"$4" | sed 's/^/      /' || true
		fail=1
	fi
}

for cfg in "${CONFIGS[@]}"; do
	src="${REPO_ROOT}/${cfg}"
	[[ -f "$src" ]] || { echo "FAIL  ${cfg}  (missing)"; fail=1; continue; }

	if command -v haproxy >/dev/null; then
		host_cfg="${WORK}/${cfg//\//_}"
		sed "s#crt ${CERT_PATH_IN_CFG}/#crt ${CERTDIR}/#g" "$src" >"$host_cfg"
		rc=0
		out="$(timeout "$CHECK_TIMEOUT" haproxy -c -f "$host_cfg" 2>&1)" || rc=$?
		report "host haproxy $(haproxy -v 2>/dev/null | awk 'NR==1{print $3}')" "$cfg" "$rc" "$out"
	else
		echo "SKIP  ${cfg}  (host: no haproxy binary)"
	fi

	if [[ "$docker_ok" -eq 1 ]]; then
		rc=0
		out="$(docker run --rm --network none --user "$(id -u):$(id -g)" \
			-v "${src}:/usr/local/etc/haproxy/haproxy.cfg:ro" \
			-v "${CERTDIR}:${CERT_PATH_IN_CFG}:ro" \
			"$IMAGE" timeout "$CHECK_TIMEOUT" haproxy -c -f /usr/local/etc/haproxy/haproxy.cfg 2>&1)" || rc=$?
		report "image ${IMAGE}" "$cfg" "$rc" "$out"
	else
		echo "SKIP  ${cfg}  (image ${IMAGE}: HAPROXY_CHECK_SKIP_IMAGE=1)"
	fi
done

if [[ "$fail" -ne 0 ]]; then
	echo "check-haproxy-cfg: FAIL"
	exit 1
fi
echo "check-haproxy-cfg: PASS"
