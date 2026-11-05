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
# doom-runner and doom.elf live in unheaded-doom since 2026-11-04 (DP-305).
DR="${DOOM_RUNNER:-$HOME/tmp/projects/unheaded-doom/crates/doom-runner/target/release/doom-runner}"
DOOM_ELF="${DOOM_ELF:-$HOME/tmp/projects/unheaded-doom/doom/doom.elf}"
K="$ROOT/crates/xv6-mbc/upstream/target"
WAD="${DOOM_WAD:-$HOME/tmp/projects/doom-related/DOOM.WAD}"
LOG="$(mktemp)"
FAIL=0

pass() { printf '  \033[32mPASS\033[0m  %s\n' "$1"; }
fail() { printf '  \033[31mFAIL\033[0m  %s\n' "$1"; FAIL=1; }
note() { printf '        %s\n' "$1"; }

# A build verdict: cargo's own exit status, and a line that starts with "error".
# Never a pipe into grep here. Under pipefail a failed cargo makes the whole
# pipeline non-zero, so `cargo | grep -q error && fail` and `if cargo | grep -q`
# skip the failure branch exactly when the build failed; and an unanchored
# "error" matches `Compiling thiserror` on a cold target.
cargo_build_ok() {
  local out rc=0
  out="$("$@" 2>&1)" || rc=$?
  [ "$rc" -eq 0 ] && ! grep -qE '^error' <<<"$out" && return 0
  printf '%s\n' "$out" | tail -5 | sed 's/^/        /'
  return 1
}
ebpf_cargo_build() { ( cd "$EBPF" && cargo build --release "$@" ); }

# Stop doom-runner by its recorded pid (series rule 14). The launcher prints the
# pid into LOG: root cannot open a user-owned file in sticky /tmp
# (fs.protected_regular=2), but it inherits this already-open stdout. Truncating
# LOG afterwards means a second call can never kill a reused pid.
doom_stop() { local p; p="$(grep -aoE -m1 '^doom-runner-pid [0-9]+$' "$LOG" | cut -d' ' -f2)"; [ -n "$p" ] && sudo kill -9 "$p"; : > "$LOG"; }

cleanup() {
  doom_stop >/dev/null 2>&1
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
cargo_build_ok ebpf_cargo_build -p monad-cpu-ebpf --features ascend-linux || fail "ascend build"

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

# EVOLUTION-1 Stage 2 rung 0 (ADR-109): slot reuse (getpidloop) and three live
# slots (getpidmulti). Counts, not order: scheduling order may change once traps
# reach the kernel. With GETPID_INJECT on these are the rung 0 gate.
rung0_lines() { (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
  --ramdisk "$K/fs.img" --userland "$K/$1.mbc" --triggers 1500000 --instance 111 2>&1) | grep -a 'ascii:' \
  | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE '(GL|GM): [a-z0-9-]+(=[0-9a-z]+)?'; }
want_counts() { # $1 = lines, then "token count" pairs; every token must appear exactly count times
  local l="$1" tok n; shift
  while [ $# -gt 1 ]; do tok="$1" n="$2"; shift 2; [ "$(grep -cxF "$tok" <<<"$l")" = "$n" ] || return 1; done; }
GL="$(rung0_lines getpidloop)"
want_counts "$GL" "GL: child-pid=2" 3 "GL: fork-ret=2" 3 "GL: wait-ret=2" 3 "GL: done" 1 \
  && [ "$(wc -l <<<"$GL")" = 10 ] \
  && pass "getpidloop: slot 1 reused three times, child 2 each time" || fail "getpidloop (slot reuse)"
GM="$(rung0_lines getpidmulti)"
want_counts "$GM" "GM: c1-pid=2" 1 "GM: c2-pid=3" 1 "GM: c3-pid=4" 1 "GM: c1-again=2" 1 "GM: c2-again=3" 1 \
  "GM: c3-again=4" 1 "GM: fork-ret=2" 1 "GM: fork-ret=3" 1 "GM: fork-ret=4" 1 "GM: wait-ret=2" 1 "GM: wait-ret=3" 1 \
  "GM: wait-ret=4" 1 "GM: done" 1 && [ "$(wc -l <<<"$GM")" = 13 ] \
  && pass "getpidmulti: three live children 2, 3, 4, stable across pause" || fail "getpidmulti (live slots)"

# EVOLUTION-1 Stage 2 rung 2 (ADR-110): with UPC_RUNG2 on, uptime is answered by
# the owned kernel from BPF's tick count; it must rise in init and in a child,
# and a child must not read below its parent. Off: uptime has no BPF arm.
if grep -q 'const UPC_RUNG2: bool = true;' "$EBPF/monad-cpu-ebpf/src/main.rs"; then
  UPL="$( (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
    --ramdisk "$K/fs.img" --userland "$K/uptimeprobe.mbc" --triggers 2000000 --instance 111 2>&1) | grep -a 'ascii:' \
    | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE 'UP: [a-z-]+=[a-z]+')"
  want_counts "$UPL" "UP: init-moved=yes" 1 "UP: child-moved=yes" 1 "UP: child-mono=yes" 1 "UP: init-before=large" 1 \
    && pass "uptime: kernel clock rises in init and child, monotonic across them (ADR-110)" || fail "uptime (ADR-110)"
else
  note "uptime gate skipped: UPC_RUNG2 is off (rollback build)"
fi

# S-STAGE-2B rungs: each gate runs when its const is on; off, it is a note.
rung_on() { grep -q "^const $1: bool = true;" "$EBPF/monad-cpu-ebpf/src/main.rs"; }
probe_lines() { # $1 program, $2 triggers, $3 token prefix
  (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
    --ramdisk "$K/fs.img" --userland "$K/$1.mbc" --triggers "$2" --instance 111 2>&1) | grep -a 'ascii:' \
    | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE "$3: [a-z0-9-]+(=[a-z0-9]+)?"; }
rung_gate() { # $1 const, $2 program, $3 triggers, $4 prefix, $5 how many =yes lines
  if ! rung_on "$1"; then note "$2 gate skipped: $1 is off"; return; fi
  local l; l="$(probe_lines "$2" "$3" "$4")"
  if [ "$(grep -c '=yes$' <<<"$l")" = "$5" ] && ! grep -q '=no$' <<<"$l" && grep -qx "$4: done" <<<"$l"; then
    pass "$2: $5 checks ($1)"; else fail "$2 ($1)"; note "$(tr '\n' ' ' <<<"$l")"; fi; }
rung_gate UPC_RUNG_SBRK sbrkprobe 4000000 SB 13
rung_gate UPC_RUNG_KILL killprobe 3000000 KL 12
rung_gate UPC_RUNG_MKDIR mkprobe 4000000 MK 7
rung_gate UPC_RUNG_UNLINK rmprobe 4000000 RM 8
rung_gate UPC_RUNG_LINK lnprobe 4000000 LN 9
rung_gate UPC_RUNG_FD fdprobe 5000000 FD 49
rung_gate UPC_RUNG_CHDIR chdirprobe 5000000 CD 24
rung_gate UPC_RUNG_PIPE pipeprobe 8000000 PI 20
rung_gate UPC_RUNG_MKDIR hostile_ptr 3000000 HU 5
if rung_on UPC_RUNG_SLEEP; then
  SLL="$(probe_lines sleepprobe 3000000 SL)"
  if [ "$(grep -c '=yes$' <<<"$SLL")" = 4 ] && ! grep -q '=no$' <<<"$SLL" \
    && [ "$(grep -n 'parent-ran' <<<"$SLL" | cut -d: -f1)" -lt "$(grep -n 'child-woke' <<<"$SLL" | cut -d: -f1)" ]; then
    pass "sleepprobe: pause(n) waits, a sibling runs meanwhile (UPC_RUNG_SLEEP)"; else fail "sleepprobe (UPC_RUNG_SLEEP)"; fi
else
  note "sleepprobe gate skipped: UPC_RUNG_SLEEP is off"
fi

# ADR-109 D6 (M-247): a child stores to slot 0's pgd (15 MiB up, a page-walk
# miss) and into the slot tables. With UPC_D6_MISS_FAULT on, neither word may
# change. Off, the pgd store lands (the open hole); if it stops landing, the
# probe no longer reaches the hole and proves nothing when D6 is turned on.
HPL="$( (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
  --ramdisk "$K/fs.img" --userland "$K/hostile_pgd.mbc" --triggers 1500000 --instance 111 2>&1) | grep -a 'ascii:' \
  | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE 'HP: [a-z-]+=?[a-z]*')"
if grep -q 'const UPC_D6_MISS_FAULT: bool = true;' "$EBPF/monad-cpu-ebpf/src/main.rs"; then
  want_counts "$HPL" "HP: child=stored" 1 "HP: pgd=same" 1 "HP: tables=same" 1 "HP: done" 1 \
    && pass "hostile_pgd: a child's stores outside its slice are dropped (D6)" || fail "hostile_pgd (D6 on)"
elif want_counts "$HPL" "HP: child=stored" 1 "HP: pgd=changed" 1 "HP: done" 1; then
  note "hostile_pgd: D6 is off, a child wrote slot 0's pgd (M-247 open)"
else
  fail "hostile_pgd (D6 off: the probe no longer reaches the hole)"
fi

# Stage 2 Steps 134-135: a child forges cur_slot/trap_a7 at the table address it
# sees, or scribbles over slot 2's trapframe there, then a getpid (its own, or a
# grandchild's in slot 2) must still be right. Both hold with D6 off or on.
hs_lines() { (cd "$EBPF" && sudo ip netns exec upc0 env UPC_DEMO_MODE=1 "$BC" boot --kernel "$K/xv6-mbc.mbc" \
  --ramdisk "$K/fs.img" --userland "$K/$1.mbc" --triggers 2000000 --instance 111 2>&1) | grep -a 'ascii:' \
  | sed -E 's/([0-9A-F]{4} )+//g' | sed 's/\xc2\xb7/\n/g' | grep -aoE 'H[ST]: [a-z-]+=?[a-z0-9]*'; }
want_counts "$(hs_lines hostile_slot)" "HS: child-pid=2" 1 "HS: init-pid=1" 1 "HS: done" 1 \
  && pass "hostile_slot: forged cur_slot/trap_a7 leave getpid right" || fail "hostile_slot (forged handoff words)"
want_counts "$(hs_lines hostile_tf)" "HT: child-pid=2" 1 "HT: gc-pid=3" 1 "HT: gc-canary=ok" 1 "HT: done" 1 \
  && pass "hostile_tf: scribbled slot 2 trapframe, grandchild pid and s1 intact" || fail "hostile_tf (sibling trapframe)"

# S-STAGE-2B Step 258, ADR-135 R-F9: a child forges the reply word through a PDE it
# adds. In every build a word stored ahead of a trap must not take effect. With
# UPC_D6_MISS_FAULT on the PDE store is dropped and the route ends at closed. Off,
# the sret route and a forged fork note land (the open hole, as hostile_pgd); if they
# stop landing, the probe no longer reaches the hole and proves nothing about a fix.
if rung_on UPC_REPLY; then
  HRL="$(probe_lines hostile_reply 3000000 HR)"
  if rung_on UPC_D6_MISS_FAULT; then
    want_counts "$HRL" "HR: own-slice-intact=yes" 1 "HR: table-reached=no" 1 "HR: route=closed" 1 \
      "HR: heap-intact=yes" 1 "HR: done" 1 \
      && pass "hostile_reply: D6 closes the route to the reply word" || fail "hostile_reply (D6 on)"
  elif ! want_counts "$HRL" "HR: own-slice-intact=yes" 1 "HR: table-reached=yes" 1 "HR: heap-intact=yes" 1 \
    "HR: note-refused=yes" 1 "HR: kill-init-refused=yes" 1 "HR: done" 1; then
    fail "hostile_reply (ecall route, or init reached)"; note "$(tr '\n' ' ' <<<"$HRL")"
  elif want_counts "$HRL" "HR: note-forged=yes" 1 "HR: sret-forged=yes" 1; then
    pass "hostile_reply: a word stored ahead of a trap is wiped, init stays out of reach"
    note "hostile_reply: D6 is off, a child forged the reply by sret and a fork note (ADR-135 open)"
  else
    fail "hostile_reply (D6 off: the probe no longer reaches the hole)"
  fi
else
  note "hostile_reply gate skipped: UPC_REPLY is off"
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
cargo_build_ok ebpf_cargo_build -p monad-cpu-ebpf || fail "non-ascend build"

# The fixture stays in this repository: intact and structurally valid, always.
if ( cd "$ROOT/doom" && [ "$(sha256sum doom.rv2mbc | cut -d' ' -f1)" = "$(cat doom.rv2mbc.sha256)" ] ); then
  pass "Doom fixture: doom.rv2mbc matches its sha256"
else
  fail "Doom fixture: doom.rv2mbc does not match its sha256"
fi
if "$BC" validate --kernel "$ROOT/doom/doom.mbc" >/dev/null 2>&1; then
  pass "Doom fixture: doom.mbc is a valid image"
else
  fail "Doom fixture: upc-bootctl refuses doom.mbc"
fi

if [ -f "$WAD" ] && [ -x "$DR" ] && [ -f "$DOOM_ELF" ]; then
  { doom_stop; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
  # setsid detaches doom-runner into its own session so SIGKILL'ing it later
  # produces no job-control noise in this shell.
  # shellcheck disable=SC2024  # the redirect is deliberately unprivileged: LOG is a
  # user-owned mktemp file, and keeping it user-owned is what we want.
  ( cd "$ROOT" && sudo UPC_VERIFIER_STATS=1 setsid sh -c 'echo "doom-runner-pid $$"; exec "$@"' sh "$DR" run --doom-mbc doom/doom.mbc \
    --doom-elf "$DOOM_ELF" --rv2mbc doom/doom.rv2mbc --wad "$WAD" --hops 2 >"$LOG" 2>&1 & ) 2>/dev/null
  for _ in $(seq 1 20); do
    grep -aqE "pipeline complete|too large|Argument list too long" "$LOG" && break
    sleep 1
  done
  DBUD="$(grep -aoE 'verified [0-9]+ insns \([0-9.]+% of 1M ceiling\)' "$LOG" | head -1)"
  [ -n "$DBUD" ] && note "Doom verifier:   $DBUD"
  if grep -aq "pipeline complete" "$LOG"; then pass "Doom object loads (< 1M verifier ceiling)"
  elif grep -aqE "too large|Argument list too long" "$LOG"; then fail "Doom object OVER 1M ceiling"
  else fail "Doom load inconclusive (see harness log)"; fi
  { doom_stop; sudo "$DR" ring teardown --hops 2; } >/dev/null 2>&1
  wait 2>/dev/null
else
  if [ "${UPC_REQUIRE_DOOM:-0}" = 1 ]; then
    fail "Doom load: SKIPPED — needs $WAD, $DR and $DOOM_ELF (DOOM_WAD, DOOM_RUNNER, DOOM_ELF)"
  else
    note "Doom load: SKIPPED — needs $WAD, $DR and $DOOM_ELF (DOOM_WAD, DOOM_RUNNER, DOOM_ELF)"
  fi
fi

# ---- Variant 3: cartridge-ebpf (ADR-108) — LOAD test + budget gate ---------
# Its own object and verifier budget (never loaded by Doom or xv6). The
# root-gated pkg/cartridge tests load it through cilium/ebpf (bpftool cannot
# open Aya's legacy maps section), print processed-insn counts and fail any
# program over 500,000; the rest of the package's root tests run cartridges
# through BPF_PROG_TEST_RUN (no attach).
echo "[3/3] cartridge-ebpf (load + verifier gate + cartridge tests)"
if ! cargo_build_ok ebpf_cargo_build -p cartridge-ebpf; then
  fail "cartridge-ebpf build"
elif ! cargo_build_ok cargo build --release --manifest-path "$ROOT/crates/upc-cartridge/Cargo.toml"; then
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
