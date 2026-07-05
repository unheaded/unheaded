#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# upc-regression.sh — the UPC regression harness.
# Track 1 Epic 1.1 of docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md.
#
# Enforces the two standing UPC constraints in one command:
#   1. LOAD-TEST EACH BUILD VARIANT (ADR-080) — not just compile. The Doom
#      outage of 2026-07-03 was an over-budget non-ascend object that only ever
#      compile-checked; doom-runner would have caught it. So this harness LOADS
#      both variants through their real loaders.
#   2. NO REGRESSION — Doom playable (loads), xv6 corpus green (ls/cat/echo/wc),
#      gate2 ISOLATION-PASS.
#
# Prints both verifier budgets. Exit 0 iff everything green (CI-gateable).
# Requires passwordless sudo (netns + XDP). Rebuilds the ascend object at the
# end so the on-disk object matches the xv6/Linux dev default.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
EBPF="$ROOT/ebpf"
BC="$ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
DR="$ROOT/crates/doom-runner/target/release/doom-runner"
K="$ROOT/crates/xv6-mbc/upstream/target"
WAD="${DOOM_WAD:-$HOME/tmp/projects/doom-related/DOOM.WAD}"
LOG="$(mktemp)"
FAIL=0

pass() { printf '  \033[32mPASS\033[0m  %s\n' "$1"; }
fail() { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; FAIL=1; }
note() { printf '        %s\n' "$1"; }

cleanup() {
  { sudo pkill -9 -f "doom-runner run"; sudo pkill -9 -f doom-go-injector; } >/dev/null 2>&1
  sudo "$DR" ring teardown --hops 2 >/dev/null 2>&1
  rm -f "$LOG"
  # Restore the ascend object as the on-disk default (xv6/Linux dev path).
  ( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux >/dev/null 2>&1 )
  wait 2>/dev/null
}
trap cleanup EXIT

# upc-bootctl resolves the eBPF object at a CWD-relative path
# (target/bpfel-unknown-none/release/monad-cpu-ebpf), so it MUST run from ebpf/.
# Run an xv6 command and assert the boot output contains a token.
xv6_expect() { # $1=input line  $2=grep token  $3=label
  local out
  out="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/init.mbc" --triggers 4000000 --instance 111 \
    --input "$1"$'\n' 2>&1)"
  if grep -aq "$2" <<<"$out"; then pass "xv6: $3"; else fail "xv6: $3 (missing '$2')"; fi
}

echo "== UPC regression harness =="

# ---- Variant 1: ascend (xv6 / Linux) --------------------------------------
echo "[1/2] ascend build (xv6 corpus)"
( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 ) \
  | grep -qE "error" && { fail "ascend build"; }

VBUD="$(cd "$EBPF" && sudo UPC_VERIFIER_STATS=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/init.mbc" --triggers 3000 --instance 111 --input $'echo\n' 2>&1 \
  | grep -aoE 'verified [0-9]+ insns \([0-9.]+% of 1M ceiling\)')"
[ -n "$VBUD" ] && note "ascend verifier: $VBUD" || note "ascend verifier: (unreported)"

xv6_expect 'ls'           'README'                 'ls lists root dir'
xv6_expect 'cat README'   '0xGA7ED-D15C-READER'    'cat README (direct blocks)'
xv6_expect 'cat BIGFILE'  'INDIRECT-READ-OK'       'cat BIGFILE (>12 KiB, indirect block)'
xv6_expect 'cat sub/NOTE' 'SUBDIR-WALK-OK'         'cat sub/NOTE (multi-component path walk)'
xv6_expect 'ls sub'       'NOTE           2'       'ls sub (open dir + stat through subdir)'
xv6_expect 'echo hello'   'hello'                  'echo hello'
# Phase 2.2 (ADR-081): echo is OUR uecho.c build; multi-arg locks the
# one-space-join + trailing-newline contract the single-arg gate misses.
xv6_expect 'echo peace and love' 'peace and love'  'echo multi-arg (our uecho)'
xv6_expect 'wc README'    '5 49 283 README'        'wc README'

# Phase 2.2 finale (ADR-081): sh behavior lock — these gates landed GREEN
# against STOCK sh BEFORE the ush swap, so they gate the shell's observable
# behavior (exec-failure text, blank-line skip, prompt-loop survival across
# commands), not any particular implementation.
xv6_expect 'nosuch'       'exec nosuch failed'     'sh reports exec failure (unknown command)'
xv6_expect $'\necho unbroken' 'unbroken'           'sh skips blank line, loop survives'
MC="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/init.mbc" --triggers 4000000 --instance 111 \
  --input $'echo alpha\necho omega\n' 2>&1)"
grep -aq 'alpha' <<<"$MC" && pass "sh multi-command: first command runs" \
  || fail "sh multi-command: first command runs (missing 'alpha')"
grep -aq 'omega' <<<"$MC" && pass "sh multi-command: prompt loop survives to second" \
  || fail "sh multi-command: prompt loop survives to second (missing 'omega')"

G2="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/gate2.mbc" --triggers 800000 --instance 111 2>&1)"
grep -aq 'ISOLATION-PASS' <<<"$G2" && pass "gate2 ISOLATION-PASS" || fail "gate2 ISOLATION-PASS"

# Track 2 Phase 2.1 (ADR-081): OUR init as PID 1. Same kernel + fs.img, only
# the resident PID 1 program swapped to uinit.mbc. The token proves uinit ran;
# 'hello' proves it forked+exec'd sh and a command completed through it.
UI="$(cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/uinit.mbc" --triggers 4000000 --instance 111 \
  --input $'echo hello\n' 2>&1)"
grep -aq '0xP1D1-0UR5' <<<"$UI" && pass "uinit banner (own PID 1)" \
  || fail "uinit banner (own PID 1) (missing '0xP1D1-0UR5')"
grep -aq 'hello' <<<"$UI" && pass "uinit forks+execs sh, echo runs" \
  || fail "uinit forks+execs sh, echo runs (missing 'hello')"

# ---- Variant 2: non-ascend (Doom) — LOAD test -----------------------------
# The regression that actually happens is "Doom object over the 1M ceiling →
# won't load." doom-runner loads it through the real pipeline; "pipeline
# complete" == loaded == under budget. (Exact Doom insn count would need the
# UPC_VERIFIER_STATS knob mirrored into doom-runner — future enhancement.)
echo "[2/2] non-ascend build (Doom load)"
( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf 2>&1 ) \
  | grep -qE "error" && { fail "non-ascend build"; }

if [ -f "$WAD" ]; then
  { sudo pkill -9 -f "doom-runner run"; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
  # setsid detaches doom-runner into its own session so SIGKILL'ing it later
  # produces no job-control noise in this shell.
  ( cd "$ROOT" && sudo UPC_VERIFIER_STATS=1 setsid "$DR" run --doom-mbc doom/doom.mbc \
    --doom-elf doom/doom.elf --rv2mbc doom/doom.rv2mbc --wad "$WAD" --hops 2 >"$LOG" 2>&1 & ) 2>/dev/null
  for _ in $(seq 1 20); do
    grep -aqE "pipeline complete|too large|Argument list too long" "$LOG" && break
    sleep 1
  done
  DBUD="$(grep -aoE 'verified [0-9]+ insns \([0-9.]+% of 1M ceiling\)' "$LOG" | head -1)"
  [ -n "$DBUD" ] && note "Doom verifier:   $DBUD"
  if grep -aq "pipeline complete" "$LOG"; then pass "Doom object loads (< 1M verifier ceiling)"
  elif grep -aqE "too large|Argument list too long" "$LOG"; then fail "Doom object OVER 1M ceiling"
  else fail "Doom load inconclusive (see harness log)"; fi
  { sudo pkill -9 -f "doom-runner run"; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
  wait 2>/dev/null
else
  note "Doom: SKIPPED — WAD not found at $WAD (set DOOM_WAD=...)"
fi

echo "== $( [ $FAIL -eq 0 ] && echo 'ALL GREEN' || echo 'REGRESSION DETECTED' ) =="
exit $FAIL
