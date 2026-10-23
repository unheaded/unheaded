#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-evidence-pack.sh: Yggdrasil evidence packs verify, and tampered ones
# do not (ADR-105 decisions 5-7; S-QUANTUM-ARMOR Phase 11; evidence source for
# UH-EVID-01).
#
# 1. builds cmd/yggdrasil-evidence;
# 2. the built binary accepts every fixture under testdata/manifests/valid/
#    and rejects every one under invalid/;
# 3. runs the E2E test, which signs a fixture pack with a throwaway ML-DSA-65
#    key (t.TempDir(), 0600, never printed), verifies it, and asserts that
#    every tamper case (manifest byte, swapped seal, ISO line, CVE scan, SBOM,
#    patch, reordered custody, missing seal, unsigned pack, wrong ISO,
#    path-traversal tarball, wrong key) exits 1.
#
# Step 3 checks that the E2E test actually RAN and passed: `go test -run E2E`
# on a renamed test matches nothing and exits 0, a gate that cannot fail.
set -euo pipefail

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT
bin="${tmp}/yggdrasil-evidence"
fixtures="cmd/yggdrasil-evidence/testdata/manifests"

if ! go build -o "${bin}" ./cmd/yggdrasil-evidence; then
    echo "check-evidence-pack: FAIL — cmd/yggdrasil-evidence does not build" >&2
    exit 1
fi

fail=0
for f in "${fixtures}"/valid/*.yaml; do
    if ! "${bin}" validate "${f}" >/dev/null 2>"${tmp}/err"; then
        echo "check-evidence-pack: FAIL — valid fixture rejected: ${f}" >&2
        sed 's/^/  /' "${tmp}/err" >&2
        fail=1
    fi
done
for f in "${fixtures}"/invalid/*.yaml; do
    if "${bin}" validate "${f}" >/dev/null 2>&1; then
        echo "check-evidence-pack: FAIL — invalid fixture accepted: ${f}" >&2
        fail=1
    fi
done

rc=0
go test -count=1 -v -run 'E2E' ./cmd/yggdrasil-evidence/ >"${tmp}/e2e.log" 2>&1 || rc=$?
if [ "${rc}" -ne 0 ] || ! grep -q -- '--- PASS: TestE2E_' "${tmp}/e2e.log" || grep -q -- '--- FAIL' "${tmp}/e2e.log"; then
    echo "check-evidence-pack: FAIL — E2E sign/verify/tamper test (rc=${rc}):" >&2
    grep -E -- '--- (FAIL|PASS)|_test.go:|^(ok|FAIL)|no tests to run' "${tmp}/e2e.log" | head -40 | sed 's/^/  /' >&2
    fail=1
fi

if [ "${fail}" -ne 0 ]; then
    exit 1
fi
echo "check-evidence-pack: PASS (fixtures agree with the built binary; signed pack verifies; every tamper exits 1)"
