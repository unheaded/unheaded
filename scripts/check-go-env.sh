#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-go-env.sh — the Go build environment is pinned where builds run
# (S-LOW-FRUIT Phase 10): GOFLAGS=-mod=readonly (a build can't rewrite
# go.mod/go.sum), GOSUMDB=sum.golang.org (hashes checked against the public
# log), GOTOOLCHAIN=local (no silent toolchain download), in both the
# Makefile and .github/workflows/ci.yml.
set -euo pipefail
cd "$(dirname "$0")/.."
fail=0
check() { # file regex what
	grep -qE "$2" "$1" || { echo "FAIL $1: missing $3"; fail=1; }
}
for want in 'GOFLAGS[ :=]+-mod=readonly' 'GOSUMDB[ :=]+sum\.golang\.org' 'GOTOOLCHAIN[ :=]+local'; do
	check Makefile "^export ${want}" "${want//\\/}"
	check .github/workflows/ci.yml "^  ${want}" "${want//\\/}"
done
grep -qE '^\s*(export )?GONOSUMDB|^\s*(export )?GOPRIVATE|GOFLAGS.*-mod=mod' Makefile .github/workflows/ci.yml && { echo "FAIL: GONOSUMDB/GOPRIVATE or -mod=mod set; module fetches would skip the checksum database"; fail=1; }
(( fail )) && exit 1
echo "OK: GOFLAGS=-mod=readonly, GOSUMDB=sum.golang.org, GOTOOLCHAIN=local in Makefile and ci.yml"
