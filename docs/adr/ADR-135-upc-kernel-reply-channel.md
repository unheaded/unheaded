<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-135: The owned kernel's reply channel to BPF

**Status**: Proposed (2026-11-04; implements DP-213, DP-501, DP-502)
**Context sprint**: S-STAGE-2B (`docs/battle-plans/S-STAGE-2B-battle-plan.md`), Phases 10-13 and 17
**Relates to**: ADR-109 (kernel rows per BPF slot), ADR-110 (kernel time from BPF ticks), ADR-078 §5 (syscall number
registry), ADR-074 (per-pid page directories), ADR-075 (process model)

## Context

ADR-109 gave the kernel a row per BPF slot and a one-way path: BPF injects a trap and the kernel answers in a0. That
covers getpid and uptime. Every rung past them needs more:

- `sbrk` must move a break that BPF carries across fork and context switch.
- `kill` must end a process, and BPF owns process lifetime (I4).
- A call that cannot finish must block, and `sleep` and `sched` panic on a slot row (I4, I7).
- `mkdir`, `unlink`, `link` and later calls take user pointers, and `copyin`, `copyout` and `copyinstr` walk a
  pagetable that is BPF's pgd, opaque to the kernel (ADR-109 D5).
- FS calls need a cwd, and a refreshed row has `cwd = 0`.

ADR-109 D5 says the first rung that needs more than rows writes its own ADR. This is that ADR. It covers what the rungs
share; each rung still lands behind its own const with a measured verifier delta.

## Decision

### D1. Reply channel

Three new words in `struct upc_slot_tables`, after `ticks`:

| Offset | Word | Writer |
|---|---|---|
| +0x810 | `brk_in` | BPF, at injection |
| +0x814 | `reply` | kernel |
| +0x818 | `reply_arg` | kernel |
| +0x81C | pad | |

`UPC_ST_SIZE` becomes 0x820. The kernel writes `UPC_TAG_REPLY (0x5250_0000) | flags`. BPF reads `reply` in the SRET
arm when `spp == 0`, clears it, and acts. Flags: bit 0 SET_BRK, bit 1 YIELD, bit 2 KILL. One reply per trap. I7 holds:
the same process, with U-mode restored first.

### D2. Heap window

VA [0x10000, 0x40000) in every child, 192 KiB: above the CSR window (the last word is at 0xFFFC) and inside fork's low
copy band (`FORK_COPY_LOW_BYTES`, `phase12.rs`), so fork needs no change. A bigger heap needs a third copy band and its
verifier cost.

### D3. Break authority

BPF keeps `cpu.program_break`, which fork and the context switch already carry. The kernel reads `brk_in` at each trap
and normalizes anything outside the window to its base. That covers exec's `DEFAULT_PROGRAM_BREAK` (0x400000) with no
change to the exec arm. Growth goes back through SET_BRK.

### D4. Init has no heap

Slot 0 is identity-mapped, and physical 0x10000-0x40000 holds kernel `.rodata`. `sbrk` in slot 0 returns -1.

### D5. Zero on growth

The slice holds whatever fork copied, kernel `.rodata` included, or a dead process's bytes. The kernel zeroes the grown
range through the flat physical address. Lazy sbrk is treated as eager: no fault path exists.

### D6. User pointers

ADR-109 D5 leaves this to the first rung that dereferences one. `walkaddr` and copyout's `walk` get a slot-row branch:
`pa = slice_base(s) + va` when `va < 0x800000`, else 0. The BPF pgd stays unwalked. The branch is valid only inside a
tagged trap (`upc_slot_active()`), so boot-time xv6 paths keep today's behaviour.

### D7. Retry hand-off

The rule ADR-109 I7 sketches. A syscall that cannot finish leaves `epc` on the ecall, keeps a0, and replies YIELD. BPF
yields at SRET, and the ecall runs again when the slot is next scheduled. This is polling, about 1,350 MBC instructions
per retry. No `sleep()` on a slot row.

### D8. Kill

The kernel validates the pid range (2..8; init refused) and replies KILL with the slot. BPF checks liveness and writes
a0 = -1 if the target is dead. Otherwise it does what exit does: reparent, mark halted, and, if the target is the
caller, exit. A target dies at once, not at its next kernel entry; with cooperative scheduling that is the stricter
answer. Status stays 0 until rung 7.

### D9. Row cwd

A refreshed row gets `namei("/")` once and keeps it. `cwd` is not inherited until the fd rung.

## Guards

One const per step, off by default: `UPC_REPLY` for the channel, `UPC_RUNG_SBRK`, `UPC_RUNG_KILL`, `UPC_RUNG_SLEEP` for
the rungs. With a const off the object equals the previous object (I6 style). Each alias row carries
`cfg!(feature = "ascend-linux")`, because `kernel_routed_alias` runs in the Doom build too.

## Consequences

- `phase12.rs`, `upc_slots.h` and `parity_slot_tables.rs` move together. The table stays kernel `.bss`, published
  through BootParams `reserved[0]`; no BootParams field changes.
- Every rung past uptime uses one read in the SRET arm. Its verifier cost is measured on its own (plan Step 129). If it
  is over +3,000 inside the execute loop, the read moves beside the `DEFERRED_SWITCH` block.
- Rollback: the rung const off. With `UPC_REPLY` off nothing reads `reply`, and no kernel path reaches a reply unless a
  rung routes it.

## Alternatives considered

- **BPF serves sbrk itself.** One arm and no reply word, but kill and blocking still need a way back, so the channel
  arrives anyway one rung later, and the arm costs more than an alias row.
- **Kernel `sleep()` with a real context switch.** `sched()` from S-mode breaks I7 and moves scheduling authority to
  the kernel. That is rung 7's direction, not this ADR's.
- **A heap above the low band.** It needs a third fork copy band, and fork's copy loop is verifier-sensitive.
- **Walking BPF's pgd in the kernel.** Two formats for one table; each child's map is two superpages, so an offset
  gives the same answer.
