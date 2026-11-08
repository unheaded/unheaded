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
# A test that skips with "needs EUID 0" runs alone, by name, as root.
# Each package's test binary runs under sudo from its own directory (the
# tests find objects by relative path). A skip that names a missing object
# fails this script: under root, a skip means the test did not run.
#
# Uses the objects scripts/check-ebpf-loads.sh builds, running it when they
# are missing; builds crates/heimdall-bpf and the cart CLI itself. Needs
# nightly Rust, bpf-linker and sudo.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

OBJ_DIR="${EBPF_LOAD_TARGET_DIR:-${REPO_ROOT}/ebpf/target/load-gate}/default/bpfel-unknown-none/release"
if [[ ! -f "${OBJ_DIR}/hop-ebpf" ]]; then
    echo "== no eBPF objects under ${OBJ_DIR}; building them with check-ebpf-loads.sh"
    if ! "${REPO_ROOT}/scripts/check-ebpf-loads.sh" >/dev/null 2>&1 || [[ ! -f "${OBJ_DIR}/hop-ebpf" ]]; then
        echo "[FAIL] check-ebpf-loads.sh did not leave objects under ${OBJ_DIR}" >&2
        exit 1
    fi
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

# pkg/cartridge's replay, sampler and scorer tests drive the cart CLI.
echo "== building crates/upc-cartridge (cart CLI)"
if ! cargo build --release --locked --manifest-path crates/upc-cartridge/Cargo.toml >"${WORK}/cart.log" 2>&1; then
    cat "${WORK}/cart.log" >&2
    echo "[FAIL] crates/upc-cartridge did not build" >&2
    exit 1
fi

mapfile -t PKGS < <(git grep -l 'needs root (CAP_BPF)' -- '*_test.go' | xargs -r -n1 dirname | sort -u)
if [[ "${#PKGS[@]}" -eq 0 ]]; then
    echo "[FAIL] found no test that needs root; the search is broken" >&2
    exit 1
fi

# Tests that skip with "needs EUID 0" run alone, by name: as root the rest
# of their package refuses (pkg/champion's model swap refuses root first).
mapfile -t EUID_TESTS < <(git grep -n 'needs EUID 0' -- '*_test.go' | while IFS=: read -r f l _; do
	name="$(awk -v L="$l" 'NR <= L && /^func Test/ { n = $2 } END { sub(/\(.*/, "", n); print n }' "$f")"
	[[ -n "$name" ]] && echo "$(dirname "$f") $name"
done)
if [[ "${#EUID_TESTS[@]}" -eq 0 ]]; then
	echo "[FAIL] found no test that needs EUID 0; the search is broken" >&2
	exit 1
fi

fail=0
for entry in "${EUID_TESTS[@]}"; do
	pkg="${entry% *}" name="${entry#* }"
	bin="${WORK}/$(tr / _ <<<"$pkg").euid.test"
	if ! go test -c -o "$bin" "./${pkg}" >"${WORK}/build.log" 2>&1; then
		cat "${WORK}/build.log" >&2
		echo "[FAIL] ${pkg}: test build"
		fail=1
		continue
	fi
	log="${WORK}/run.log"
	rc=0
	(cd "$pkg" && "${SUDO[@]}" "$bin" -test.count=1 -test.v -test.timeout 300s -test.run "^${name}\$") >"$log" 2>&1 || rc=$?
	if [[ "$rc" -eq 0 ]] && grep -q -- "--- PASS: ${name} " "$log"; then
		echo "[PASS] ${pkg} ${name} (as root)"
	else
		tail -20 "$log" >&2
		echo "[FAIL] ${pkg} ${name} as root (exit ${rc})"
		fail=1
	fi
done

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
        "CART_BIN=${REPO_ROOT}/crates/upc-cartridge/target/release/cart" \
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
