#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Makes The Well's TLS material (ERDA 4.2): a private CA and a server
# certificate for the names clients use (postgres, unheaded-postgres,
# localhost, 127.0.0.1), ECDSA P-256. Written outside the repo; the keys
# never enter it.
#
#   sudo scripts/well/make-tls.sh /etc/unheaded/well-tls        # first time
#   sudo scripts/well/make-tls.sh /etc/unheaded/well-tls --renew  # new server cert, same CA
#
# Files: ca.crt (give to clients for sslmode=verify-full), ca.key (root
# 0600, only for --renew), server.crt, server.key (root:70 0640: the
# postgres user in the alpine image is uid/gid 70, and PostgreSQL accepts a
# root-owned key readable by its group). Refuses a directory inside the repo
# and will not overwrite an existing CA.
set -euo pipefail
DIR="${1:?usage: $0 DIR [--renew]}"
RENEW="${2:-}"
R="$(cd "$(dirname "$0")/../.." && pwd)"
PG_GID="${WELL_PG_GID:-70}"
# More DNS names for the certificate, comma-separated, so clients can use
# sslmode=verify-full where the Well has another name: on Kubernetes,
# WELL_TLS_EXTRA_DNS=the-well,the-well.unheaded.svc,the-well.unheaded.svc.cluster.local
SANS="DNS:postgres,DNS:unheaded-postgres,DNS:localhost,IP:127.0.0.1"
if [[ -n "${WELL_TLS_EXTRA_DNS:-}" ]]; then
	IFS=',' read -r -a extra <<<"$WELL_TLS_EXTRA_DNS"
	for n in "${extra[@]}"; do
		[[ "$n" =~ ^[A-Za-z0-9]([A-Za-z0-9-]{0,62})(\.[A-Za-z0-9]([A-Za-z0-9-]{0,62}))*$ ]] ||
			{ echo "[well-tls] not a DNS name: '$n'" >&2; exit 1; }
		SANS="$SANS,DNS:$n"
	done
fi
DAYS_CA=3650 DAYS_SERVER=825

mkdir -p -- "$DIR"
DIR="$(cd "$DIR" && pwd)"
case "$DIR/" in "$R"/*) echo "[well-tls] refusing $DIR: inside the repo" >&2; exit 1 ;; esac
umask 077
if [[ "$RENEW" != --renew ]]; then
	[[ ! -e "$DIR/ca.key" ]] || { echo "[well-tls] $DIR/ca.key exists; use --renew for a new server certificate" >&2; exit 1; }
	openssl req -new -x509 -nodes -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
		-days "$DAYS_CA" -subj "/CN=The Well private CA" \
		-addext "basicConstraints=critical,CA:TRUE,pathlen:0" -addext "keyUsage=critical,keyCertSign,cRLSign" \
		-keyout "$DIR/ca.key" -out "$DIR/ca.crt" 2>/dev/null
fi
[[ -s "$DIR/ca.key" ]] || { echo "[well-tls] no CA in $DIR" >&2; exit 1; }
T="$(mktemp -d)"; trap 'rm -rf -- "$T"' EXIT
openssl req -new -nodes -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
	-subj "/CN=unheaded-postgres" -keyout "$T/server.key" -out "$T/server.csr" 2>/dev/null
printf '%s\n' "subjectAltName=$SANS" \
	"basicConstraints=critical,CA:FALSE" "keyUsage=critical,digitalSignature" "extendedKeyUsage=serverAuth" >"$T/ext"
openssl x509 -req -in "$T/server.csr" -CA "$DIR/ca.crt" -CAkey "$DIR/ca.key" -CAcreateserial \
	-days "$DAYS_SERVER" -extfile "$T/ext" -out "$T/server.crt" 2>/dev/null
openssl verify -CAfile "$DIR/ca.crt" "$T/server.crt" >/dev/null
install -m 0644 "$T/server.crt" "$DIR/server.crt"
if ((EUID == 0)); then
	install -m 0640 "$T/server.key" "$DIR/server.key"
	chown "0:$PG_GID" "$DIR/server.key"
	chmod 0644 "$DIR/ca.crt"; chmod 0600 "$DIR/ca.key"; chmod 0755 "$DIR"
else
	install -m 0600 "$T/server.key" "$DIR/server.key"
	chmod 0644 "$DIR/ca.crt"; chmod 0755 "$DIR"
	echo "[well-tls] not root: server.key is yours, 0600; the container needs it root:$PG_GID 0640" >&2
fi
echo "[well-tls] $DIR: server.crt valid until $(openssl x509 -enddate -noout -in "$DIR/server.crt" | cut -d= -f2)"
