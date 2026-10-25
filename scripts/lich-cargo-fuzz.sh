#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# lich-cargo-fuzz.sh — build and run every Rust libFuzzer target in the tree
# (ADR-062 CI integration; run by .github/workflows/lich.yml).
#
# The Go fuzz targets run in security.yml's lich-fuzz job. The Rust ones —
# crates/monad-mbc/fuzz (the real MBC decoder, executor and cartridge step,
# LICH-001/007) and crates/lich-fuzz — ran nowhere: no workflow, no Jenkins
# stage, no Makefile target.
#
# A fuzz crate is any tracked Cargo.toml that depends on libfuzzer-sys; its
# targets are its [[bin]] names, so a new target is fuzzed without editing
# this script. Builds with the same instrumentation cargo-fuzz uses
# (sanitizer coverage + AddressSanitizer, nightly) but needs only cargo.
#
# Usage:
#   scripts/lich-cargo-fuzz.sh --list
#   scripts/lich-cargo-fuzz.sh [--time SECONDS] [--out DIR] [TARGET...]
#
# --time   seconds per target (default $LICH_FUZZ_SECONDS, else 60)
# --out    work directory: build tree, corpora, crash artifacts, logs.
#          Default: a mktemp directory, deleted on success; on failure only
#          its artifacts/ and logs/ are kept (the ASan build is ~650 MB and
#          /tmp is often tmpfs).
#
# Exit 1 when any target crashes, leaks, times out or executes nothing; when
# no target is found; or when a named target does not exist. The failing
# input is kept under <out>/artifacts/<target>/.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TIME="${LICH_FUZZ_SECONDS:-60}"
OUT=""
LIST=false
want=()
while [[ $# -gt 0 ]]; do
	case "$1" in
	--list) LIST=true; shift ;;
	--time) TIME="${2:-}"; shift 2 ;;
	--out) OUT="${2:-}"; shift 2 ;;
	-h|--help) sed -n '4,30p' "$0"; exit 0 ;;
	-*) echo "unknown flag $1" >&2; exit 2 ;;
	*) want+=("$1"); shift ;;
	esac
done
[[ "$TIME" =~ ^[1-9][0-9]*$ ]] || { echo "--time takes whole seconds" >&2; exit 2; }

# targets: "<manifest> <bin>" for every [[bin]] of every libFuzzer crate.
targets() {
	local m
	while read -r m; do
		grep -qE '^libfuzzer-sys[[:space:]]*=' "$REPO_ROOT/$m" || continue
		awk -v m="$m" '
			/^\[\[bin\]\]/ { inbin = 1; next }
			/^\[/          { inbin = 0 }
			inbin && /^name[[:space:]]*=/ {
				n = $0; sub(/^[^"]*"/, "", n); sub(/".*/, "", n); print m, n
			}' "$REPO_ROOT/$m"
	done < <(git -C "$REPO_ROOT" ls-files '*Cargo.toml' | sort)
}

mapfile -t all < <(targets)
if [[ ${#all[@]} -eq 0 ]]; then
	echo "[FAIL] no libFuzzer targets found; refusing to pass vacuously" >&2
	exit 1
fi
if $LIST; then
	printf '%s\n' "${all[@]}"
	exit 0
fi

run=()
if [[ ${#want[@]} -eq 0 ]]; then
	run=("${all[@]}")
else
	for w in "${want[@]}"; do
		hit=""
		for t in "${all[@]}"; do [[ "${t#* }" == "$w" ]] && hit="$t"; done
		[[ -n "$hit" ]] || { echo "[FAIL] no fuzz target named $w (see --list)" >&2; exit 1; }
		run+=("$hit")
	done
fi

if [[ -z "$OUT" ]]; then
	OUT="$(mktemp -d)"
	trap 'rc=$?; if [[ $rc -eq 0 ]]; then rm -rf "$OUT"; else rm -rf "$OUT/target" "$OUT/corpus"; fi' EXIT
else
	mkdir -p "$OUT" || exit 1
fi
if [[ -L "$OUT" || ! -O "$OUT" ]]; then
	echo "[FAIL] $OUT is a symlink or not owned by $(id -un)" >&2
	exit 1
fi

HOST="$(rustc +nightly -vV | sed -n 's/^host: //p')"
[[ -n "$HOST" ]] || { echo "[FAIL] no nightly rustc (rustup toolchain install nightly)" >&2; exit 1; }
# cargo-fuzz's default instrumentation. --target keeps RUSTFLAGS off build
# scripts and proc macros, which must not be sanitized.
export RUSTFLAGS="-Cpasses=sancov-module -Cllvm-args=-sanitizer-coverage-level=4 -Cllvm-args=-sanitizer-coverage-inline-8bit-counters -Cllvm-args=-sanitizer-coverage-pc-table -Cllvm-args=-sanitizer-coverage-trace-compares --cfg fuzzing -Zsanitizer=address -Cdebug-assertions -Coverflow-checks"
export CARGO_TARGET_DIR="$OUT/target"

failed=0
declare -A built=()
for t in "${run[@]}"; do
	manifest="${t% *}"
	bin="${t#* }"
	if [[ -z "${built[$manifest]:-}" ]]; then
		echo "== build ${manifest}"
		if ! cargo +nightly build --release --locked --bins --target "$HOST" \
			--manifest-path "$REPO_ROOT/$manifest"; then
			echo "[FAIL] build ${manifest}" >&2
			exit 1
		fi
		built[$manifest]=1
	fi
	exe="$CARGO_TARGET_DIR/$HOST/release/$bin"
	corpus="$OUT/corpus/$bin"
	art="$OUT/artifacts/$bin"
	log="$OUT/logs/$bin.log"
	mkdir -p "$corpus" "$art" "$(dirname "$log")"
	# A committed seed corpus, when there is one, is read but never written.
	seeds=()
	seed_dir="$REPO_ROOT/$(dirname "$manifest")/corpus/$bin"
	[[ -d "$seed_dir" ]] && seeds=("$seed_dir")
	echo "== fuzz ${bin} (${TIME}s)"
	"$exe" "$corpus" "${seeds[@]}" -max_total_time="$TIME" -timeout=10 \
		-rss_limit_mb=2048 -artifact_prefix="$art/" -print_final_stats=1 >"$log" 2>&1
	rc=$?
	execs="$(sed -n 's/^stat::number_of_executed_units: *//p' "$log" | tail -1)"
	if [[ $rc -ne 0 ]]; then
		echo "[FAIL] ${bin}: libFuzzer exit ${rc}; input in ${art}/, log ${log}" >&2
		tail -30 "$log" >&2
		failed=1
	elif [[ -z "$execs" || "$execs" -eq 0 ]]; then
		echo "[FAIL] ${bin}: executed nothing (log ${log})" >&2
		failed=1
	else
		echo "[ok] ${bin}: ${execs} execs"
	fi
done

echo "work directory: $OUT"
if [[ $failed -ne 0 ]]; then
	exit 1
fi
echo "[PASS] ${#run[@]} libFuzzer target(s), ${TIME}s each, no crash"
