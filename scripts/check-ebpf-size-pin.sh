#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# check-ebpf-size-pin.sh: ADR-089's develop -> staging entry gate item 4 as a
# script. A change that claims to be inert to the eBPF layer must leave
# `monad-cpu-ebpf --features ascend-linux` at the pinned byte size: the
# cheapest proof the UPC substrate was not perturbed (S-IRON-GATE Step 316).
#
#   scripts/check-ebpf-size-pin.sh            # build, compare with docs/policy/ebpf-size-pins.txt
#   scripts/check-ebpf-size-pin.sh --update   # build, rewrite the pin (review the delta first)
#
# Byte size depends on the toolchain, so the pin records the rustc that made
# it. A different rustc is not compared: the gate fails and says so (re-pin
# deliberately with --update), rather than passing or failing on toolchain
# noise. That is also why it runs on the build host as a promotion gate and
# not in CI, whose nightly floats.
# Exit: 0 match (or updated), 1 size differs, 2 build/toolchain/usage problem.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PINS="${EBPF_SIZE_PINS:-$ROOT/docs/policy/ebpf-size-pins.txt}"
KEY="monad-cpu-ebpf+ascend-linux"
UPDATE=0
case "${1:-}" in
"") ;;
--update) UPDATE=1 ;;
*) echo "usage: $0 [--update]" >&2; exit 2 ;;
esac

cd "$ROOT/ebpf"
rustc_v="$(rustc -V)"
if [[ -z "${EBPF_SIZE_OBJ:-}" ]]; then
	cargo build --release --target=bpfel-unknown-none -Z build-std=core -p monad-cpu-ebpf --features ascend-linux >/dev/null 2>&1 ||
		{ echo "[FAIL] cargo build of monad-cpu-ebpf --features ascend-linux failed" >&2; exit 2; }
fi
OBJ="${EBPF_SIZE_OBJ:-$ROOT/ebpf/target/bpfel-unknown-none/release/monad-cpu-ebpf}"
[[ -f "$OBJ" ]] || { echo "[FAIL] no object at $OBJ" >&2; exit 2; }
size="$(stat -c %s "$OBJ")"

if [[ "$UPDATE" -eq 1 ]]; then
	old="$(awk -v k="$KEY" '$1 == k {print $2}' "$PINS" 2>/dev/null || true)"
	{
		echo "# ADR-089 entry gate item 4: byte size of the eBPF object, per toolchain."
		echo "# Written by scripts/check-ebpf-size-pin.sh --update; review the delta before committing."
		echo "# <object+features> <bytes> <rustc version...>"
		echo "$KEY $size $rustc_v"
	} >"$PINS"
	echo "[UPDATED] $KEY: ${old:-none} -> $size bytes ($rustc_v)"
	exit 0
fi

line="$(awk -v k="$KEY" '$1 == k' "$PINS" 2>/dev/null || true)"
[[ -n "$line" ]] || { echo "[FAIL] no pin for $KEY in $PINS (run --update)" >&2; exit 2; }
pin="$(awk '{print $2}' <<<"$line")"
pin_rustc="$(cut -d' ' -f3- <<<"$line")"
if [[ "$pin_rustc" != "$rustc_v" ]]; then
	echo "[FAIL] pin was taken with '$pin_rustc', this build uses '$rustc_v': not comparable; review and re-pin with --update" >&2
	exit 2
fi
if [[ "$size" != "$pin" ]]; then
	echo "[FAIL] $KEY is $size bytes, pinned $pin (delta $((size - pin))): the change is not inert to the eBPF layer, or re-pin with --update" >&2
	exit 1
fi
echo "[PASS] $KEY $size bytes, as pinned ($rustc_v)"
