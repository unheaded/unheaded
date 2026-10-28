<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-110: The owned kernel's clock comes from BPF's tick count

**Status**: Proposed (2026-10-28)
**Context sprint**: EVOLUTION-1 Stage 2 rung 2 (`docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md`)
**Relates to**: ADR-109 (kernel rows per BPF slot), ADR-067 (MBC ISA v2, UPC ABI v1), EVOLUTION-1
(`docs/battle-plans/EVOLUTION-1-TRAP-FLIP.md`)

## Context

Rung 2 of Stage 2 is `uptime` (14) and `pause` (13). The research is in `references/evolution1-stage2-log.md`,
§"Rung 2 research":
- `uptime` has no BPF arm. It falls into "Unknown syscall: silently ignore" and returns whatever a0 held.
- The owned kernel's `ticks` is always 0. It advances only in `clockintr`, on a timer trap, and the BPF timer is
  compiled out under `ascend-linux` (a timer trap with xv6's own vector unprogrammed reset the CPU). The skeleton's
  rung 2 prerequisite, "the timer interrupt reaches the kernel", does not hold.
- BPF services `pause` as a cooperative yield and ignores its argument. Its arm is now cheap, since the context
  switch is deferred to one site (`DEFERRED_SWITCH`, −45,118 verified insns).
- The kernel's `sys_pause` sleeps on `ticks`, and ADR-109 forbids `sleep` on a slot row (I4/I7).

## Decision

### T1. BPF publishes its tick count; the kernel's `ticks` follows it

BPF already keeps a monotonic per-instance count, `cpu.tick_counter`: one per XDP invocation, never reset under
`ascend-linux`. The ADR-109 slot table's spare word at +0x80C becomes `ticks`. At every trap injection,
`inject_strap()` writes `tick_counter` there. On a tagged trap, `upc_slot_enter()` raises the kernel's `ticks` to that
value under `tickslock`, never lowering it.

The unit is one UPC tick, which is one invocation of this CPU instance, not xv6's 100 ms. Programs that use `uptime`
and `pause` relatively (all of xv6's userland, `usertests` included) are unaffected by the unit.

### T2. `uptime` arrives in the kernel

A new ascend arm `syscall_nr == 14` injects the trap the same way getpid's arm does; the kernel's `sys_uptime` answers
from `ticks`. 14 joins `SURFACE_ASCEND` (`ebpf/monad-cpu-ebpf/surface.rs`), which changes the object's surface id.
The injection is shared, so the arm does not carry a second inlined copy of it.

### T3. `pause` stays a BPF yield in this rung

Moving `pause` into the kernel needs a hand-off ADR-109 I7 only sketches: the kernel decides that the sleep has not
ended, and BPF yields and re-injects. That is its own design, it will be measured, and it has a natural home in the
first rung that blocks in the kernel. `pause(n)` keeps ignoring `n`.

### Guard

One const, `UPC_RUNG2`, off by default. It guards the `ticks` write and the `uptime` arm. With it off, the object is
the post-`DEFERRED_SWITCH` object (I6-style equality), and the kernel's `ticks` stays 0, as today.

## Rejected

- **Injecting a real supervisor timer trap** (scause `0x8000_0005`) every N ticks. That would exercise `devintr` and
  `clockintr` as upstream wrote them, but it interrupts user code at arbitrary instructions and costs a full trap
  round trip each time, about 1,350 MBC instructions. It also needs `yield()` suppressed in `usertrap` (BPF
  schedules), and a timer that lands while the CPU is in S-mode needs its own rule. It belongs to whichever rung
  brings preemption.
- **`sys_uptime` reading the table directly.** Under T1, all kernel code (`sys_pause`, later `sleep`) sees the same
  clock, which a direct read would not give.

## Consequences

- Gate: `uptimeprobe` shows `uptime` increasing across busy work and `pause`, in init and in a child, and a child's
  value is not below the parent's earlier one. The 12 goldens stay SAME, the harness stays green, and gate_nway's
  co-scheduling is unchanged.
- Cost: a measured verified-insn delta with `UPC_RUNG2` on. `uptime` is an arrival (BPF had no arm), so the rung
  frees nothing by itself.
- **E2's stop rule** says to stop at the first rung whose freed instructions are below zero. Read per rung, an
  arrival can never pass it. Read cumulatively, the ladder has freed 52,746 so far (921,036 → 868,290). The flag
  stays off until Stevie says which reading holds.

## Security notes

- `ticks` is a BPF-written word the kernel now reads. Any value is harmless: it can only raise `ticks`, and `ticks`
  only feeds `sys_uptime` and the `sys_pause` comparison. A hostile writer can make `uptime` jump forward, never back.
- The hole ADR-109 D6 describes still applies while D6 is off: a child can reach the table.
