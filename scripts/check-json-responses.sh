#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-json-responses.sh — no handler encodes JSON straight to its
# ResponseWriter. Such an encoder writes nothing when a value will not marshal,
# so the client gets an empty 200 or the handler's status with no body;
# httputil.WriteJSON answers 500. The check parses every non-test Go file
# (pkg/httputil/policy_test.go); pkg/httputil itself is the one place allowed.
#
#   scripts/check-json-responses.sh
#
# Exits 1 naming each offender as FILE:LINE:COL, or if the check did not run.
set -euo pipefail
cd "$(dirname "$0")/.."

name=TestNoHandlerEncodesToTheResponseWriter
if ! out="$(go test ./pkg/httputil -run "^${name}\$" -count=1 -v 2>&1)"; then
	echo "${out}"
	echo "FAIL: a handler encodes to its ResponseWriter; use httputil.WriteJSON"
	exit 1
fi
if ! grep -q -- "--- PASS: ${name}" <<<"${out}"; then
	echo "${out}"
	echo "FAIL: ${name} did not run"
	exit 1
fi
echo "OK: no handler encodes JSON to its ResponseWriter"
