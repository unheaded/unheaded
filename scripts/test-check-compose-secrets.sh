#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-compose-secrets.sh: the compose secrets gate refuses each form it
# names and passes the ones that take the value from outside the file. Each
# case is one compose file handed to the gate.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

# $1 label, $2 exit status wanted, stdin the compose file.
secrets_case() {
	local file rc=0
	file="$(mktemp "${WORK}/compose.XXXXXX.yml")"
	cat >"${file}"
	"${REPO_ROOT}/scripts/check-compose-secrets.sh" "${file}" >/dev/null 2>&1 || rc=$?
	if [ "${rc}" -eq "$2" ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s (exit %s, wanted %s)\n' "$1" "${rc}" "$2"
		FAILED=$((FAILED + 1))
	fi
}

# Both helpers put one passing secret-named key beside the case, so a case
# about another kind of key is still a verdict (the gate exits 2 on a file set
# with no secret-named key at all).
# $1 label, $2 exit status wanted, $3 key, $4 value: map form.
secrets_map() {
	secrets_case "$1" "$2" <<YAML
services:
  probe:
    environment:
      ANCHOR_TOKEN: \${ANCHOR_TOKEN:?set it}
      $3: $4
YAML
}

# $1 label, $2 exit status wanted, $3 the list entry.
secrets_list() {
	secrets_case "$1" "$2" <<YAML
services:
  probe:
    environment:
      - ANCHOR_TOKEN=\${ANCHOR_TOKEN:?set it}
      - $3
YAML
}

echo "--- these take the value from outside the file ---"
secrets_map "a required variable" 0 ADMIN_PASSWORD '${ADMIN_PASSWORD:?set ADMIN_PASSWORD in .env}'
secrets_map "a required variable, colon-less" 0 ADMIN_PASSWORD '${ADMIN_PASSWORD?set it}'
secrets_map "a plain variable" 0 SESSION_SECRET '${SESSION_SECRET}'
secrets_map "a bare variable" 0 SESSION_SECRET '$SESSION_SECRET'
secrets_map "a variable with an empty default" 0 APP_PASSWORD '"${APP_PASSWORD:-}"'
secrets_map "no value in map form" 0 API_TOKEN ''
secrets_map "an empty string" 0 API_TOKEN '""'
secrets_map "a file reference under /run/secrets" 0 SIGNING_KEY /run/secrets/signing_key
secrets_map "a file reference under /etc" 0 LXD_CLIENT_KEY /etc/unheaded/cuirass/client.key
secrets_map "a _FILE key is not a secret-named key" 0 WEBUI_SECRET_KEY_FILE /run/secrets/webui_secret_key
secrets_map "a key that only contains the word" 0 KEYCLOAK_REALM unheaded
secrets_list "list form, a required variable" 0 'ADMIN_PASSWORD=${ADMIN_PASSWORD:?set it}'
secrets_list "list form, passed through by name" 0 'ADMIN_PASSWORD'
secrets_map "a URL whose password is a variable" 0 DATABASE_URL '"postgres://app:${APP_PASSWORD:?set it}@postgres:5432/app"'
secrets_map "a URL with no credentials" 0 VM_URL 'http://victoria:8428'
secrets_case "a build argument from a variable" 0 <<'YAML'
services:
  probe:
    build:
      context: .
      args:
        NPM_TOKEN: ${NPM_TOKEN:?set it}
YAML
secrets_case "a top-level secret read from a file" 0 <<'YAML'
services:
  probe:
    environment:
      SIGNING_KEY: /run/secrets/signing_key
    secrets:
      - signing_key
secrets:
  signing_key:
    file: ${SIGNING_KEY_FILE:?set it}
YAML

echo "--- these put the value in the file ---"
secrets_map "a literal password" 1 GF_SECURITY_ADMIN_PASSWORD lab-default-value
secrets_list "list form, a literal password" 1 'GF_SECURITY_ADMIN_PASSWORD=lab-default-value'
secrets_map "a literal secret" 1 SESSION_SECRET '"0123456789abcdef"'
secrets_map "a literal token" 1 LOGSTORE_INGEST_TOKEN abcdef
secrets_map "a literal key" 1 OPENAI_API_KEY unused
secrets_map "a literal passphrase" 1 BACKUP_PASSPHRASE correct-horse
secrets_map "a lower-case key name" 1 admin_password lab-default-value
secrets_map "a literal default" 1 ADMIN_PASSWORD '${ADMIN_PASSWORD:-admin}'
secrets_map "a literal default, colon-less" 1 ADMIN_PASSWORD '${ADMIN_PASSWORD-admin}'
secrets_map "a literal replacement" 1 ADMIN_PASSWORD '${ADMIN_PASSWORD:+admin}'
secrets_map "a literal around a variable" 1 API_TOKEN '"Bearer-${API_TOKEN}-suffix"'
secrets_map "an escaped dollar, which is a literal" 1 ADMIN_PASSWORD '$$ADMIN_PASSWORD'
secrets_map "a relative path, which is not a file reference" 1 SIGNING_KEY ./signing.key
secrets_map "a number" 1 ADMIN_PASSWORD 12345678
secrets_map "a URL with a literal password, under any key" 1 DATABASE_URL 'postgres://app:lab-default@postgres:5432/app'
secrets_list "list form, a URL with a literal password" 1 'WELL_DSN=postgres://app:lab-default@postgres:5432/app'
secrets_map "a URL whose password has a literal default" 1 DATABASE_URL '"postgres://app:${APP_PASSWORD:-lab}@postgres:5432/app"'
secrets_case "a literal in an anchor merged into a service" 1 <<'YAML'
x-env: &env
  ADMIN_PASSWORD: lab-default-value
services:
  probe:
    environment:
      <<: *env
      OTHER: "1"
YAML
secrets_case "a literal build argument" 1 <<'YAML'
services:
  probe:
    build:
      context: .
      args:
        NPM_TOKEN: abcdef
YAML
secrets_case "a top-level secret with its value inline" 1 <<'YAML'
services:
  probe:
    image: x
    secrets:
      - signing_key
secrets:
  signing_key:
    content: lab-default-value
YAML

echo "--- these are not a verdict ---"
secrets_case "no secret-named key anywhere exits 2" 2 <<'YAML'
services:
  probe:
    environment:
      TZ: UTC
YAML
secrets_case "a file that is not YAML fails" 1 <<'YAML'
services: [unterminated
YAML
missing_rc=0
"${REPO_ROOT}/scripts/check-compose-secrets.sh" "${WORK}/no-such-file.yml" >/dev/null 2>&1 || missing_rc=$?
if [ "${missing_rc}" -eq 2 ]; then
	printf '  PASS  %s\n' "a file that does not exist exits 2"
	PASSED=$((PASSED + 1))
else
	printf '  FAIL  %s (exit %s, wanted 2)\n' "a file that does not exist exits 2" "${missing_rc}"
	FAILED=$((FAILED + 1))
fi

echo
echo "passed ${PASSED}, failed ${FAILED}"
[ "${FAILED}" -eq 0 ]
