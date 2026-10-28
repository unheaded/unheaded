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
#      gate2 ISOLATION-PASS, gate_nway 3x NWAY-ok, forkloop 3 children.
#
# Prints all three verifier budgets. Exit 0 iff everything green (CI-gateable).
#   3. (S5, ADR-108) the cartridge object cartridge-ebpf loads with each of
#      cart_xdp / cart_tick <= 500,000 processed insns, and its root-gated
#      pkg/cartridge tests pass.
# Requires passwordless sudo (netns + XDP). Every live boot passes
# UPC_DEMO_MODE=1 inside sudo (sudo resets the environment; upc-bootctl refuses
# a live boot without it, S-ASCEND-2 Item 4). Rebuilds the ascend object at the
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
  # The namespace this run brought up (scripts/upc-netns.sh); a host that runs
  # unheaded-upc-netns.service keeps it, since that unit owns it there.
  [ "$NETNS_OURS" = 1 ] && sudo bash "$ROOT/scripts/upc-netns.sh" down >/dev/null 2>&1
  # Restore the ascend object as the on-disk default (xv6/Linux dev path).
  ( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux >/dev/null 2>&1 )
  wait 2>/dev/null
}
trap cleanup EXIT

# Every xv6 boot runs inside netns upc0 (S-ASCEND-2 Phase 5): both veth ends
# live there and bootctl sends its own triggers, as unheaded-upc-bootctl@ does.
NETNS_OURS=0
if ! sudo ip netns list | awk '{print $1}' | grep -qx upc0; then NETNS_OURS=1; fi
sudo bash "$ROOT/scripts/upc-netns.sh" up >/dev/null || { echo "upc-netns.sh up failed" >&2; exit 1; }

# upc-bootctl resolves the eBPF object at a CWD-relative path
# (target/bpfel-unknown-none/release/monad-cpu-ebpf), so it MUST run from ebpf/.
# Run an xv6 command and assert the boot output contains a token.
xv6_expect() { # $1=input line  $2=grep token  $3=label
  local out
  out="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/init.mbc" --triggers 4000000 --instance 111 \
    --input "$1"$'\n' 2>&1)"
  if grep -aq "$2" <<<"$out"; then pass "xv6: $3"; else fail "xv6: $3 (missing '$2')"; fi
}

echo "== UPC regression harness =="

# ---- Variant 1: ascend (xv6 / Linux) --------------------------------------
echo "[1/3] ascend build (xv6 corpus)"
( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf --features ascend-linux 2>&1 ) \
  | grep -qE "error" && { fail "ascend build"; }

VBUD="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 UPC_VERIFIER_STATS=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
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
MC="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/init.mbc" --triggers 4000000 --instance 111 \
  --input $'echo alpha\necho omega\n' 2>&1)"
grep -aq 'alpha' <<<"$MC" && pass "sh multi-command: first command runs" \
  || fail "sh multi-command: first command runs (missing 'alpha')"
grep -aq 'omega' <<<"$MC" && pass "sh multi-command: prompt loop survives to second" \
  || fail "sh multi-command: prompt loop survives to second (missing 'omega')"

G2="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/gate2.mbc" --triggers 800000 --instance 111 2>&1)"
grep -aq 'ISOLATION-PASS' <<<"$G2" && pass "gate2 ISOLATION-PASS" || fail "gate2 ISOLATION-PASS"

# S-ASCEND-2 Phase 3: N-way CONCURRENT isolation (ADR-074 Gate 4) and the
# looped-fork gate. Both were "known failures" until the scheduler stopped
# resuming a zombie when nothing was runnable; build them with
# `make -f ../adapters/Makefile.mbc-userland gates` (a missing binary fails).
NW="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/gate_nway.mbc" --triggers 1500000 --instance 111 2>&1)"
# Count tokens, not lines: the TTY dump is one "ascii:" line.
[ "$(grep -aoE 'NWAY-ok pid[123]' <<<"$NW" | sort -u | grep -c .)" -eq 3 ] && ! grep -aq 'NWAY-FAIL' <<<"$NW" \
  && pass "gate_nway 3x NWAY-ok, 0 FAIL" || fail "gate_nway (concurrent isolation)"
FL="$(cd "$EBPF" && timeout 600 sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/forkloop.mbc" --triggers 1500000 --instance 111 2>&1)"
[ "$(grep -aoE 'FL: child' <<<"$FL" | grep -c .)" -eq 3 ] && grep -aq 'FL: parent-done' <<<"$FL" \
  && pass "forkloop: 3 children, parent done" || fail "forkloop (looped fork)"

# S-ASCEND-2 Phase 7: open("console") by name and write through the returned fd
# (and a dup of it). The Phase 1.4 PC tracer prints 4-hex-digit PCs between the
# bytes of a write that follows an SRET; strip them before matching.
DV="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/devprobe.mbc" --triggers 1500000 --instance 111 2>&1 | grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g')"
grep -aq 'DEV: console-open-ok' <<<"$DV" && grep -aq 'DEV: console-dup-ok' <<<"$DV" \
  && pass "devprobe: console opens by name, write + dup" || fail "devprobe (console by name)"

# S-ASCEND-2 E1 (option K, 2026-10-28): one pid numbering, init = 1. getpid in
# the parent and the child, fork()'s return and wait()'s return must agree.
PF="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/getpidfork.mbc" --triggers 800000 --instance 111 2>&1 | grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g')"
if grep -aq 'F: parent-pid=1' <<<"$PF" && grep -aq 'GF: child-pid=2' <<<"$PF" && grep -aq 'GF: fork-ret=2' <<<"$PF" \
  && grep -aq 'GF: wait-ret=2' <<<"$PF" && grep -aq 'GF: parent-after-wait' <<<"$PF"; then
  pass "getpidfork: init 1, child 2, fork and wait agree"
else
  fail "getpidfork (pid numbering, E1 option K)"
fi

# S26: xv6 usertests bodies as probes (user/ut_*.c) against BPF fork/exit/wait.
# forktest and reparent2 need wait() to return -1 once every child is reaped.
for ut in exitwait:1500000:ok twochildren:1500000:ok forktest:1500000:ok reparent2:3000000:reaped-all; do
  n="${ut%%:*}" rest="${ut#*:}" t="${rest%%:*}" want="${rest#*:}"
  UTO="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
    --userland "$K/ut_$n.mbc" --triggers "$t" --instance 111 2>&1 | grep -a 'ascii:' | sed -E 's/([0-9A-F]{4} )+//g')"
  if grep -aq "UT-$n: $want" <<<"$UTO" && ! grep -aq "UT-$n: fail" <<<"$UTO"; then
    pass "usertests $n"
  else
    fail "usertests $n"
  fi
done

# Track 2 Phase 2.1 (ADR-081): OUR init as PID 1. Same kernel + fs.img, only
# the resident PID 1 program swapped to uinit.mbc. The token proves uinit ran;
# 'hello' proves it forked+exec'd sh and a command completed through it.
UI="$(cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/uinit.mbc" --triggers 4000000 --instance 111 \
  --input $'echo hello\n' 2>&1)"
grep -aq '0xP1D1-0UR5' <<<"$UI" && pass "uinit banner (own PID 1)" \
  || fail "uinit banner (own PID 1) (missing '0xP1D1-0UR5')"
grep -aq 'hello' <<<"$UI" && pass "uinit forks+execs sh, echo runs" \
  || fail "uinit forks+execs sh, echo runs (missing 'hello')"

# ---- Variant 2: non-ascend (Doom) — LOAD test -----------------------------
# The regression that actually happens is "Doom object over the 1M ceiling →
# won't load." doom-runner loads it through the real pipeline; "pipeline
# complete" == loaded == under budget. doom-runner honours UPC_VERIFIER_STATS,
# so the exact Doom insn count is captured below as DBUD.
echo "[2/3] non-ascend build (Doom load)"
( cd "$EBPF" && cargo build --release -p monad-cpu-ebpf 2>&1 ) \
  | grep -qE "error" && { fail "non-ascend build"; }

if [ -f "$WAD" ]; then
  { sudo pkill -9 -f "doom-runner run"; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
  # setsid detaches doom-runner into its own session so SIGKILL'ing it later
  # produces no job-control noise in this shell.
  # shellcheck disable=SC2024  # the redirect is deliberately unprivileged: LOG is a
  # user-owned mktemp file, and keeping it user-owned is what we want.
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

# ---- Variant 3: cartridge-ebpf (ADR-108) — LOAD test + budget gate ---------
# Its own object and verifier budget (never loaded by Doom or xv6). The
# root-gated pkg/cartridge tests load it through cilium/ebpf (bpftool cannot
# open Aya's legacy maps section), print processed-insn counts and fail any
# program over 500,000; the rest of the package's root tests run cartridges
# through BPF_PROG_TEST_RUN (no attach).
echo "[3/3] cartridge-ebpf (load + verifier gate + cartridge tests)"
if ( cd "$EBPF" && cargo build --release -p cartridge-ebpf 2>&1 ) | grep -qE "^error"; then
  fail "cartridge-ebpf build"
elif ( cargo build --release --manifest-path "$ROOT/crates/upc-cartridge/Cargo.toml" 2>&1 ) | grep -qE "^error"; then
  fail "upc-cartridge (cart CLI) build"
else
  CT="$(mktemp)"
  if ( cd "$ROOT" && go test -c -o "$CT" ./pkg/cartridge/ ) >/dev/null 2>&1; then
    COUT="$(cd "$ROOT/pkg/cartridge" && sudo "$CT" -test.v 2>&1)"
    for prog in cart_xdp cart_tick; do
      n="$(grep -aoE "VERIFIER $prog processed [0-9]+" <<<"$COUT" | grep -oE '[0-9]+$')"
      if [ -z "$n" ]; then fail "cartridge $prog: no verifier count"
      elif [ "$n" -le 500000 ]; then pass "cartridge $prog verifier $n insns (<= 500,000)"
      else fail "cartridge $prog verifier $n insns (> 500,000)"; fi
    done
    if grep -aq '^PASS$' <<<"$COUT" && ! grep -aq -- '--- FAIL' <<<"$COUT"; then
      pass "pkg/cartridge root tests ($(grep -ac -- '--- PASS' <<<"$COUT") passed)"
    else
      fail "pkg/cartridge root tests"; grep -a -- '--- FAIL' <<<"$COUT" | head -5
    fi
  else
    fail "pkg/cartridge test build"
  fi
  rm -f "$CT"
fi

echo "== $( [ $FAIL -eq 0 ] && echo 'ALL GREEN' || echo 'REGRESSION DETECTED' ) =="
exit $FAIL
