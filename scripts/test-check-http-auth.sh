#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-check-http-auth.sh — check-http-auth.sh sees every listener form it
# names, and only those. Each case is a throwaway repository with a copy of
# the gate, an empty baseline and one Go package.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
PASSED=0
FAILED=0

# $1 label, $2 exit status wanted, stdin the file under test.
http_auth_case() {
	local dir rc=0
	dir="$(mktemp -d "${WORK}/case.XXXXXX")"
	mkdir -p "${dir}/scripts/lib" "${dir}/docs/policy" "${dir}/pkg/probe" "${dir}/pkg/anchor"
	cp "${REPO_ROOT}/scripts/check-http-auth.sh" "${dir}/scripts/"
	cp "${REPO_ROOT}/scripts/lib/sets.sh" "${dir}/scripts/lib/"
	: >"${dir}/docs/policy/http-auth-baseline.txt"
	# One wired listener, so a case with none does not read as no Go at all.
	printf 'package anchor\n\nimport (\n\t"net/http"\n\n\t"unheaded/pkg/auth"\n)\n\nvar _ = auth.Middleware\nvar S = http.Server{}\n' >"${dir}/pkg/anchor/a.go"
	cat >"${dir}/pkg/probe/serve.go"
	command git -C "${dir}" init -q
	"${dir}/scripts/check-http-auth.sh" >/dev/null 2>&1 || rc=$?
	if [ "${rc}" -eq "$2" ]; then
		printf '  PASS  %s\n' "$1"
		PASSED=$((PASSED + 1))
	else
		printf '  FAIL  %s (exit %s, wanted %s)\n' "$1" "${rc}" "$2"
		FAILED=$((FAILED + 1))
	fi
}

echo "--- these are listeners ---"
http_auth_case "http.ListenAndServe" 1 <<'GO'
package probe
import "net/http"
func Run() error { return http.ListenAndServe(":0", nil) }
GO
http_auth_case "http.Server literal" 1 <<'GO'
package probe
import "net/http"
var S = &http.Server{Addr: ":0"}
GO
http_auth_case "http.Serve on a listener" 1 <<'GO'
package probe
import ("net"; "net/http")
func Run(l net.Listener) error { return http.Serve(l, nil) }
GO
http_auth_case "net/http under another name" 1 <<'GO'
package probe
import (
	"net"
	nethttp "net/http"
)
func Run(l net.Listener) error { return nethttp.Serve(l, nil) }
GO
http_auth_case "pkg/tls's ListenAndServe" 1 <<'GO'
package probe
import utls "unheaded/pkg/tls"
func Run() error { return utls.ListenAndServe(nil, nil) }
GO
http_auth_case "pkg/tls's MaybeWrapServer" 1 <<'GO'
package probe
import utls "unheaded/pkg/tls"
func Run() { utls.MaybeWrapServer(nil, nil) }
GO
http_auth_case "ListenAndServe on a server built elsewhere" 1 <<'GO'
package probe
func Run(s interface{ ListenAndServe() error }) error { return s.ListenAndServe() }
GO

echo "--- these are not ---"
http_auth_case "a client" 0 <<'GO'
package probe
import "net/http"
func Run() (*http.Response, error) { return http.Get("http://127.0.0.1/") }
GO
http_auth_case "gRPC's Serve" 0 <<'GO'
package probe
func Run(s interface{ Serve(any) error }) error { return s.Serve(nil) }
GO
http_auth_case "a listener in a package with pkg/auth" 0 <<'GO'
package probe
import (
	"net/http"

	"unheaded/pkg/auth"
)
var _ = auth.Middleware
func Run() error { return http.ListenAndServe(":0", nil) }
GO

echo
echo "passed: ${PASSED}  failed: ${FAILED}"
[ "${FAILED}" -eq 0 ] || exit 1
echo "RESULT: PASS"
