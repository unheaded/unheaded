# ADR-080 — The UPC Programmatic API (multi-workload substrate)

**Status**: ACCEPTED (describes the realized architecture; formalizes the contract going forward)
**Date**: 2026-07-03
**Deciders**: Stevie (directive: "UPC runs both [Doom and Linux] as foundation for future
experimental programmatic expansion; highest priority is maintaining playable Doom and no
regressions to progress"); Fable 5 session
**Builds on**: ADR-067 (MBC ISA v2 + UPC ABI v1), ADR-072 (BOOT_MAGIC), ADR-074/075 (per-pid
memory + process model), ADR-077 (feature-gated `MbcCpuState` + per-process rv2mbc_base),
ADR-078 (Gate-D FS reader + argv ABI), ADR-079 (RET return-address tagging)

## 1. What this ADR captures

As of 2026-07-03 the UPC (Unheaded Protocol Computer — a virtual CPU executing MBC bytecode
inside an eBPF/XDP program on the Monad transport) runs **two independent guest workloads on one
shared interpreter core**:

- **Doom** — a full game (id DOOM over doomgeneric), driven by 4 syscalls
  (DRAW_FRAME/GET_KEY/GET_TICKS/SLEEP), framebuffer to RAM, playable in-browser via doom-runner.
- **xv6 / ASCEND-LINUX** — a real OS kernel: interactive shell, fork/exec/wait, an in-BPF
  filesystem reader, per-pid MMU isolation (Phase 1 complete; uClinux/Linux is Phase 2).

This ADR names that arrangement a **programmatic API**: the UPC is not a Doom appliance or an
xv6 appliance — it is a general substrate that runs *arbitrary MBC programs*, and Doom and xv6
are the first two "clients." It records the contract by which a program plugs in, and the
constraints that govern expansion. It is the architectural anchor for "future experimental
programmatic expansion."

## 2. The contract — how a program runs on the UPC

A UPC guest is defined by five things:

1. **An MBC image** (`ROM_MAP`) — instructions, one word per slot. Produced offline by
   `rv32i_to_mbc` (RISC-V → MBC) or emitted directly. Entry PC is the loader's convention.
2. **A branch-translation map** (`RV2MBC_MAP`) — RV word address → MBC PC, for indirect
   branches (JMPR/CALLR) and privileged returns (MRET/SRET). Per-program disjoint region via
   `rv2mbc_base` (ADR-077).
3. **A memory model** (`RAM_MAP`, 64 MiB) — flat physical space; optional per-pid VA slices +
   Sv32-style MMU for isolated processes (ADR-074).
4. **A syscall surface** — the set of `ecall`/INT numbers the program calls and their handlers.
   This is the program's *API into the host*. See §3.
5. **(Kernel-class guests) a boot protocol** — UPC Boot Protocol v2: a 256-byte `BootParams`
   at 0x100, an optional two-stage `upc-bootstub` that verifies params and MRETs to the kernel
   in S-mode (ADR-067, `crates/upc-bootstub`).

Return addresses are tagged (ADR-079) so the call/return ABI is image-size-independent — a
prerequisite for large (Linux-class) images.

## 3. Syscall surfaces are per-workload and DCE-partitioned (the load-bearing rule)

The single `monad-cpu-ebpf` object is compiled twice via the `ascend-linux` cargo feature:

- **Doom build** (`--no-default`): carries ONLY Doom's surface (DRAW_FRAME/GET_KEY/GET_TICKS/
  SLEEP) + the shared interpreter core. Every xv6 syscall arm is `cfg!(feature = "ascend-linux")`
  and dead-code-eliminates out.
- **ASCEND build** (`--features ascend-linux`): carries the xv6/Linux surface (fork/exec/wait/
  open/read/write/fstat/… + MMU/CSR ops); the Doom arms (`!cfg!(...)`) DCE out.

**This partitioning is mandatory, not stylistic.** The kernel BPF verifier caps a program at
**1,000,000 processed instructions**. Both surfaces in one build exceeds it. The 2026-07-03 Doom
outage was exactly this: xv6 syscall arms had accumulated in the *shared* dispatch and were NOT
gated out of the Doom build, pushing the Doom object 1 instruction over the ceiling
(commit `7eba83e0` fixed it by gating them). **Rule: a workload's build carries only its own
surface; cross-workload syscall code must be `cfg`-gated so it DCEs apart.**

Corollary discipline (learned the hard way): **every build variant must be LOAD-tested, not just
compile-tested.** The Doom object silently drifted over budget for the entire xv6 sprint because
doom-runner was never run; compilation succeeded but the verifier would have rejected it. CI must
gate on a real load of *each* variant, not just `cargo build`.

## 4. Consequences / direction for programmatic expansion

- **Adding a new experimental guest today** = compile it to MBC, give it a syscall surface
  (a new `cfg`-gated arm-set, or reuse an existing one), load via `upc-bootctl`/`doom-runner`.
  It must fit the 1M budget *for its own build* — which the DCE partitioning makes tractable
  because a guest never pays for another guest's surface.
- **Known scaling limits** (Phase 2 substrate, see
  `references/phase2-preflight-capacity-audit-2026-07-02.md`): `ROM_MAP` (262 K words) and
  `RV2MBC_MAP` (64 K) are sized for small images; a Linux-class kernel needs a code-store
  redesign (grow-maps / demand-translate / RAM-resident code — an open pair-call decision).
- **Future cleanup (not yet decided):** the per-workload `cfg` partition is a coarse plugin
  mechanism. A cleaner long-term model is a *registered dispatch* — a workload declares its
  surface as a table the core consults — so new experiments slot in without editing the central
  `if/else` chain. Deferred; the `cfg` partition is sufficient and proven for two workloads.
- **Non-negotiable per Stevie:** playable Doom and no regressions to shipped progress are the
  gating constraints on any expansion. Both Doom and xv6 are verified green as of `a5e538b0`.

## 5. Status of the two reference clients (2026-07-03)

| Client | State | Surface | Entry |
|--------|-------|---------|-------|
| Doom | Playable in-browser (bridge :16666) | DRAW_FRAME/GET_KEY/GET_TICKS/SLEEP | doom-runner, PC=0 |
| xv6 | Phase 1 complete (sh + 5 cmds + FS) | RV32I ecall (fork/exec/wait/open/read/write/fstat/…) | upc-bootctl, PC=0x4000 |
| uClinux/Linux | Phase 2, blocked on code-store decision | (will extend the ASCEND surface) | bootstub → S-mode MRET |
