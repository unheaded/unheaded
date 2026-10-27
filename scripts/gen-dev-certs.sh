#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# gen-dev-certs.sh: development TLS material for the compose stack
# (S-IRON-GATE Phase 8, ADR-105, FND-017). A dev CA and one certificate per
# first-party compose service (SANs: the compose service name, its auth
# identity where different, localhost, 127.0.0.1, ::1), plus unheaded-cli's
# client certificate, laid out one directory per service so each container
# mounts only its own key:
#
#   configs/certs/dev/ca.pem              the CA every service trusts (0644)
#   configs/certs/dev/ca-key.pem          the CA's key (0600; never mounted)
#   configs/certs/dev/<svc>/<svc>.pem     certificate (0644)
#   configs/certs/dev/<svc>/<svc>-key.pem private key (0600)
#   configs/certs/dev/<svc>/ca.pem        copy of the CA certificate
#
# Everything here is generated at run time and gitignored (*.pem); never
# commit it. Development only: hosts get certificates from the Kingdom CA
# runbook (runbooks/pki/generate-kingdom-certs.yaml), not from this script.
# A re-run keeps the existing CA and re-issues every certificate.
#
# Containers run as their own uid (pkg/uids) and read their key through a
# bind mount that keeps the host file's owner and mode. Run it once as
# yourself (builds bin/cert-gen), then with sudo so each service directory
# and key belong to its compose uid (root never runs the Go toolchain):
#
#   scripts/gen-dev-certs.sh && sudo scripts/gen-dev-certs.sh
#
# Usage: scripts/gen-dev-certs.sh [svc1,svc2,...]
# Environment: CERT_DIR (default configs/certs/dev), CERT_VALIDITY (8760h).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "$PROJECT_DIR" || exit 2
CERT_DIR="${CERT_DIR:-configs/certs/dev}"
SERVICES="${1:-}"
CERT_VALIDITY="${CERT_VALIDITY:-8760h}"
CERTGEN="${CERTGEN:-bin/cert-gen}"

IS_ROOT=0
[[ "$(id -u)" -eq 0 ]] && IS_ROOT=1

if [[ $IS_ROOT -eq 1 ]]; then
	[[ -x "$CERTGEN" ]] || { echo "gen-dev-certs: $CERTGEN missing; run scripts/gen-dev-certs.sh as yourself first (root does not run the Go toolchain)" >&2; exit 2; }
else
	go build -o "$CERTGEN" ./cmd/cert-gen
fi

mkdir -p "$CERT_DIR"
chmod 0750 "$CERT_DIR"
ARGS=(-out "$CERT_DIR" -ttl "$CERT_VALIDITY" -layout per-service -reuse-ca)
[[ -n "$SERVICES" ]] && ARGS+=(-services "$SERVICES")
"$CERTGEN" "${ARGS[@]}"

# As root: each service directory and key to its compose uid; the CLI's and
# the CA's to whoever ran sudo. Compose names map to uids through the
# `user:` lines of docker-compose.yml.
if [[ $IS_ROOT -eq 1 ]]; then
	owner_sudo="${SUDO_UID:?run with sudo, not as root directly (SUDO_UID owns the CA and CLI material)}:${SUDO_GID:?}"
	chown "$owner_sudo" "$CERT_DIR" "$CERT_DIR/ca.pem" "$CERT_DIR/ca-key.pem"
	python3 - docker-compose.yml >"$CERT_DIR/.uids" <<'PY'
import sys, yaml
doc = yaml.safe_load(open(sys.argv[1]))
for name, svc in sorted(doc["services"].items()):
    if "build" in svc and svc.get("user"):
        print(name, str(svc["user"]).split(":")[0])
PY
	for dir in "$CERT_DIR"/*/; do
		svc="$(basename "$dir")"
		uid="$(awk -v s="$svc" '$1 == s {print $2}' "$CERT_DIR/.uids")"
		if [[ -n "$uid" ]]; then
			owner="$uid:$uid"
		else
			owner="$owner_sudo"
		fi
		chown -R "$owner" "$dir"
		chmod 0750 "$dir"
		printf '  %-18s %s\n' "$svc" "$owner"
	done
	rm -f "$CERT_DIR/.uids"
fi

echo "CA: $CERT_DIR/ca.pem  (curl --cacert $CERT_DIR/ca.pem https://127.0.0.1:<port>/health)"
