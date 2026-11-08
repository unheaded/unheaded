#!/bin/bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
# test-go-root.sh — the Go tests that skip unless they run as root.
#
# Every test that loads a real BPF object or pins a map skips with
# "needs root (CAP_BPF)" under a plain `go test ./...`. On 2026-11-08 that was
# six packages, and only pkg/cartridge's ran anywhere (upc-regression.sh): the
# 27 hop_xdp checks in tests/hopchecks, heimdall-daemon's object load,
# compliance-evidence's ring test, protocol-api's pinned map and pkg/ebpf's
# pinned-map tests had never run in CI. All pass as root on west.
#
# Packages are found by that skip message, so a new root test is picked up.
# Each package's test binary runs under sudo from its own directory (the
# tests find objects by relative path). A skip that names a missing object
# fails this script: under root, a skip means the test did not run.
#
# Needs the objects scripts/check-ebpf-loads.sh builds (run it first); builds
# crates/heimdall-bpf itself. Needs nightly Rust, bpf-linker and sudo.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

OBJ_DIR="${EBPF_LOAD_TARGET_DIR:-${REPO_ROOT}/ebpf/target/load-gate}/default/bpfel-unknown-none/release"
if [[ ! -f "${OBJ_DIR}/hop-ebpf" ]]; then
    echo "[FAIL] no eBPF objects under ${OBJ_DIR}; run scripts/check-ebpf-loads.sh first" >&2
    exit 1
fi

SUDO=()
if [[ "$(id -u)" -ne 0 ]]; then
    if ! sudo -n true 2>/dev/null; then
        echo "[FAIL] needs root or passwordless sudo" >&2
        exit 1
    fi
    SUDO=(sudo -n)
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo "== building crates/heimdall-bpf"
if ! (cd crates/heimdall-bpf && cargo build --release --locked) >"${WORK}/heimdall.log" 2>&1; then
    cat "${WORK}/heimdall.log" >&2
    echo "[FAIL] crates/heimdall-bpf did not build" >&2
    exit 1
fi

mapfile -t PKGS < <(git grep -l 'needs root (CAP_BPF)' -- '*_test.go' | xargs -r -n1 dirname | sort -u)
if [[ "${#PKGS[@]}" -eq 0 ]]; then
    echo "[FAIL] found no test that needs root; the search is broken" >&2
    exit 1
fi

fail=0
for pkg in "${PKGS[@]}"; do
    bin="${WORK}/$(tr / _ <<<"$pkg").test"
    if ! go test -c -o "$bin" "./${pkg}" >"${WORK}/build.log" 2>&1; then
        cat "${WORK}/build.log" >&2
        echo "[FAIL] ${pkg}: test build"
        fail=1
        continue
    fi
    log="${WORK}/run.log"
    rc=0
    (cd "$pkg" && "${SUDO[@]}" env \
        "CARTRIDGE_OBJ=${OBJ_DIR}/cartridge-ebpf" \
        "HOP_OBJ=${OBJ_DIR}/hop-ebpf" \
        "HEIMDALL_BPF_DIR=${REPO_ROOT}/crates/heimdall-bpf/target/bpfel-unknown-none/release" \
        "$bin" -test.count=1 -test.v -test.timeout 300s) >"$log" 2>&1 || rc=$?
    passed="$(grep -c -- '--- PASS' "$log" || true)"
    if [[ "$rc" -ne 0 ]] || grep -q -- '--- FAIL' "$log"; then
        grep -B2 -A8 -- '--- FAIL' "$log" | head -40 >&2 || tail -20 "$log" >&2
        echo "[FAIL] ${pkg} (exit ${rc})"
        fail=1
    elif grep -E -q 'needs root \(CAP_BPF\)|not built|not found|missing' <(grep -B1 -- '--- SKIP' "$log"); then
        grep -B1 -- '--- SKIP' "$log" >&2
        echo "[FAIL] ${pkg}: a root test skipped, so it did not run"
        fail=1
    else
        echo "[PASS] ${pkg} (${passed} passed)"
    fi
done
exit "$fail"
