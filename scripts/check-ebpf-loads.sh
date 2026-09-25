#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-ebpf-loads.sh — every eBPF program in ebpf/ must pass the kernel
# verifier, except those listed in docs/policy/ebpf-load-known-failing.txt,
# and that list may only SHRINK.
#
# Why this exists. The verifier runs at LOAD time, so building proves nothing
# and bpf-verifier-check.sh (static analysis) proves little. Until 2026-09-25
# nothing in the tree loaded these programs, and 16 of 31 had never passed the
# verifier: the firewall, qos, hop, nfv and others all built cleanly and could
# not run anywhere. This loads each one (never attaches) and fails on any
# rejection that is not already on the list.
#
# The list is a ratchet in both directions:
#   - a program that fails and is NOT listed          -> FAIL (new breakage)
#   - a listed program that now LOADS                 -> FAIL (take it off)
#   - a listed program that no longer exists          -> FAIL (stale entry)
# Membership is compared, not counts (the lesson in check-secrets-baseline.sh).
#
# Needs root, for BPF_PROG_LOAD. Without root or passwordless sudo it FAILS
# (exit 2). It never passes by skipping: a gate that goes green whenever it
# cannot run is the defect this repository keeps finding.
#
# Builds into its own target dir (default ebpf/target/load-gate). A plain
# `cargo build --release` in ebpf/ replaces the ascend-linux monad-cpu-ebpf
# ELF that xv6 boots from with the default-features build; this must not.
#
# Verdicts are kernel-specific. The list was measured on 6.17; a different
# kernel may accept or reject differently, and this gate will say so.
#
# Usage: scripts/check-ebpf-loads.sh
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EBPF="${REPO_ROOT}/ebpf"
TARGET="${EBPF_LOAD_TARGET_DIR:-${EBPF}/target/load-gate}"
KNOWN="${REPO_ROOT}/docs/policy/ebpf-load-known-failing.txt"
LOADER_DIR="${REPO_ROOT}/cmd/ebpf-load-check"

# Feature variants loaded in addition to each package's default build.
# monad-cpu-ebpf ascend-linux is the variant xv6 boots; ADR-080 requires each
# variant be load-tested, because they are different programs to the verifier.
VARIANTS="monad-cpu-ebpf:ascend-linux"

if [[ "${EUID}" -eq 0 ]]; then
    SUDO=()
elif sudo -n true 2>/dev/null; then
    SUDO=(sudo -n)
else
    echo "[FAIL] loading BPF programs needs root or passwordless sudo." >&2
    echo "       This gate does not pass by skipping. Run it where it can load." >&2
    exit 2
fi

if [[ ! -f "$KNOWN" ]]; then
    echo "[FAIL] $KNOWN is missing" >&2
    exit 2
fi

LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

echo "== building loader"
if ! cargo build --release --locked --manifest-path "${LOADER_DIR}/Cargo.toml" >"$LOG" 2>&1; then
    cat "$LOG" >&2
    echo "[FAIL] could not build cmd/ebpf-load-check" >&2
    exit 1
fi
LOADER="${LOADER_DIR}/target/release/ebpf-load-check"

echo "== building eBPF programs into ${TARGET}"
if ! (cd "$EBPF" && CARGO_TARGET_DIR="${TARGET}/default" cargo build --release --workspace) >"$LOG" 2>&1; then
    cat "$LOG" >&2
    echo "[FAIL] eBPF workspace build failed" >&2
    exit 1
fi
for v in $VARIANTS; do
    pkg="${v%%:*}" feat="${v#*:}"
    if ! (cd "$EBPF" && CARGO_TARGET_DIR="${TARGET}/${feat}" cargo build --release -p "$pkg" --features "$feat") >"$LOG" 2>&1; then
        cat "$LOG" >&2
        echo "[FAIL] build of ${pkg} --features ${feat} failed" >&2
        exit 1
    fi
done

# Expected ELFs come from cargo metadata, not from whatever the build left on
# disk: a binary that silently stopped being produced must fail, not vanish.
bins="$(cd "$EBPF" && cargo metadata --no-deps --format-version 1 | python3 -c '
import json, sys
for p in json.load(sys.stdin)["packages"]:
    for t in p["targets"]:
        if "bin" in t["kind"]:
            print(t["name"])
' | sort -u)"
if [[ -z "$bins" ]]; then
    echo "[FAIL] cargo metadata listed no eBPF binaries; refusing to pass an empty set" >&2
    exit 1
fi

# load <key> <elf>: print "<key>/<prog> OK|FAIL <reason>" for every program.
load() {
    local key="$1" elf="$2" out
    if [[ ! -f "$elf" ]]; then
        echo "${key}/* FAIL ELF not produced by the build: ${elf}"
        return
    fi
    if ! out="$("${SUDO[@]}" "$LOADER" "$elf" 2>&1)"; then
        echo "${key}/* FAIL ${out}"
        return
    fi
    printf '%s\n' "$out" | sed "s|^|${key}/|"
}

echo "== loading (kernel $(uname -r))"
results="$(
    for b in $bins; do
        load "$b" "${TARGET}/default/bpfel-unknown-none/release/${b}"
    done
    for v in $VARIANTS; do
        pkg="${v%%:*}" feat="${v#*:}"
        load "${pkg}[${feat}]" "${TARGET}/${feat}/bpfel-unknown-none/release/${pkg}"
    done
)"

failing="$(printf '%s\n' "$results" | awk '$2=="FAIL" {print $1}' | sort -u)"
loading="$(printf '%s\n' "$results" | awk '$2=="OK" {print $1}' | sort -u)"
known="$(sed 's/#.*//' "$KNOWN" | awk 'NF {print $1}' | sort -u)"

unexpected="$(comm -23 <(printf '%s\n' "$failing" | grep .) <(printf '%s\n' "$known" | grep .))"
now_loads="$(comm -12 <(printf '%s\n' "$known" | grep .) <(printf '%s\n' "$loading" | grep .))"
stale="$(comm -23 <(printf '%s\n' "$known" | grep .) <(printf '%s\n' "$failing" "$loading" | grep . | sort -u))"

rc=0
if [[ -n "$unexpected" ]]; then
    echo "[FAIL] the verifier rejects program(s) not on the known-failing list:" >&2
    while read -r k; do
        printf '%s\n' "$results" | awk -v k="$k" '$1==k {$2=""; print "         " $0}' >&2
    done <<<"$unexpected"
    echo "       Fix the program. Adding it to $(basename "$KNOWN") is not a fix." >&2
    rc=1
fi
if [[ -n "$now_loads" ]]; then
    echo "[FAIL] listed as known-failing but now LOADS; remove from $(basename "$KNOWN"):" >&2
    printf '%s\n' "$now_loads" | sed 's/^/         /' >&2
    rc=1
fi
if [[ -n "$stale" ]]; then
    echo "[FAIL] known-failing entries name programs that no longer exist:" >&2
    printf '%s\n' "$stale" | sed 's/^/         /' >&2
    rc=1
fi
if [[ $rc -ne 0 ]]; then
    exit $rc
fi

n_ok="$(printf '%s\n' "$loading" | grep -c .)"
n_known="$(printf '%s\n' "$known" | grep -c .)"
if [[ "$n_known" -eq 0 ]]; then
    echo "[PASS] ${n_ok} eBPF programs pass the verifier; no known failures"
else
    echo "[PASS] ${n_ok} eBPF programs pass the verifier; ${n_known} known failure(s):"
    printf '%s\n' "$known" | grep . | sed 's/^/         /'
fi
exit 0
