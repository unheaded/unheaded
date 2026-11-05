#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-compose-publish-bind.sh: the publish gate refuses each form it
# names and passes the internal ones. Each case is one compose file, handed to
# the gate with an empty allowlist.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
: >"${WORK}/allowlist.txt"
PASSED=0
FAILED=0

# $1 label, $2 exit status wanted, stdin the compose file.
publish_case() {
	local file rc=0
	file="$(mktemp "${WORK}/compose.XXXXXX.yml")"
	cat >"${file}"
	COMPOSE_PUBLISH_ALLOWLIST="${WORK}/allowlist.txt" \
		"${REPO_ROOT}/scripts/check-compose-publish-bind.sh" "${file}" >/dev/null 2>&1 || rc=$?
	if [ "${rc}" -eq "$2" ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s (exit %s, wanted %s)\n' "$1" "${rc}" "$2"
		FAILED=$((FAILED + 1))
	fi
}

# $1 label, $2 exit status wanted, $3 the one publish, short form.
publish_short() {
	publish_case "$1" "$2" <<YAML
services:
  probe:
    ports:
      - "$3"
YAML
}

echo "--- these bind where the file says ---"
publish_short "loopback" 0 '127.0.0.1:8428:8428'
publish_short "docker0's gateway" 0 '172.17.0.1:9598:9598'
publish_short "the point-to-point link" 0 '192.168.13.2:8428:8428'
publish_short "the overlay" 0 '[fd00:dead:beef::1]:8428:8428'
publish_short "a variable with an internal default" 0 '${BIND:-127.0.0.1}:8428:8428'
publish_short "udp on loopback" 0 '127.0.0.1:5353:1053/udp'

echo "--- these do not ---"
publish_short "no address" 1 '8428:8428'
publish_short "a bare port" 1 '8428'
publish_short "every IPv4 interface" 1 '0.0.0.0:8428:8428'
publish_short "every IPv6 interface" 1 '[::]:8428:8428'
publish_short "a LAN address" 1 '192.168.69.184:8428:8428'
publish_short "a variable with no default" 1 '${BIND}:8428:8428'
publish_short "a variable that must be set" 1 '${BIND:?set it}:8428:8428'
publish_short "a default an empty value overrides" 1 '${BIND-127.0.0.1}:8428:8428'
publish_short "a default of every interface" 1 '${BIND:-0.0.0.0}:8428:8428'
publish_case "long form with no host_ip" 1 <<'YAML'
services:
  probe:
    ports:
      - target: 8428
        published: 8428
YAML
publish_case "network_mode host with ports" 1 <<'YAML'
services:
  probe:
    network_mode: host
    ports:
      - "127.0.0.1:8428:8428"
YAML
publish_case "extends from another file" 1 <<'YAML'
services:
  anchor:
    ports:
      - "127.0.0.1:8428:8428"
  probe:
    extends:
      file: other.yml
      service: base
YAML
publish_case "include of another file" 1 <<'YAML'
include:
  - other.yml
services:
  probe:
    ports:
      - "127.0.0.1:8428:8428"
YAML

printf '\n%s passed, %s failed\n' "${PASSED}" "${FAILED}"
[ "${FAILED}" -eq 0 ]
