# ADR-080 — The UPC Programmatic API (multi-workload substrate)

**Status**: ACCEPTED (describes the realized architecture; formalizes the contract going forward)
**Date**: 2026-07-03
**Amended**: 2026-10-28, §6 I/O-time gates (S-ASCEND-2 Steps 95-106); §7 image map freeze (Steps 157-168). Status unchanged.
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

## 6. I/O-time gates (S-ASCEND-2, amended 2026-10-28)

`upc_api::validate` checks what the `Workload` descriptor alone can prove. Two checks need the
eBPF object the loader is about to load. They live in `upc_api::validate_against_object(&Workload,
&MapCapacities, &ObjectFacts)` (= `check_surface` + `check_mmu`), which `upc_api::load` runs after
`validate` and before the first map write. Both loaders also run them on the object file before
they open it (`preflight_object` in `cmd/upc-bootctl/src/main.rs` and `crates/doom-runner/src/main.rs`):
aya creates every map when it loads an object, so a refusal there leaves no map, program, netns or
ring behind. `upc-bootctl boot --dry-run` runs the same preflight, which needs no privileges.

### 6.1 What the object declares

`ebpf/monad-cpu-ebpf/src/upc_facts.rs` defines two `#[no_mangle] #[link_section = ".rodata.upc"]
pub static`s. No BPF code path reads them. The `.text` and `xdp` sections are byte-identical with
and without them, in both builds; aya turns `.rodata.upc` into one 52-byte read-only array map,
created and frozen at load like `.rodata.cst*`.

| Symbol | Type | Value |
|---|---|---|
| `UPC_SURFACE_ID` | `[u8; 32]` | `surface_id(numbers)` for the active feature set, emitted by `build.rs` |
| `UPC_MMU_GEOMETRY` | `[u32; 5]` | `[phase12::USER_CHILD_SLICE, phase12::SLICE_STRIDE, USER_VA_CEILING, phase12::RAM_ISOLATION_CEILING, intr::MAX_PROCESSES]` |

`upc_api::ObjectFacts::from_elf` reads both by symbol from `.rodata.upc` (ELF64 LE, every offset
bounds-checked). An object without `UPC_SURFACE_ID` (built before this amendment) is refused.

### 6.2 Gate A: surface vs loaded object

`surface_id(numbers) = SHA-256("UPC-SURFACE-ID-v1\0" || le32(n) || le32(num_0..num_{n-1}))` over the
sorted, deduplicated numbers (`crates/upc-api/src/surface_hash.rs`; `build.rs` `include!`s the same
file, so the encoding cannot diverge). `ebpf/monad-cpu-ebpf/surface.rs` lists the numbers with a
top-level arm in the `op::SYSCALL` dispatch of `src/main.rs`, per build:

| Build | Numbers |
|---|---|
| default (Doom) | 1 DRAW_FRAME, 2 GET_KEY, 3 GET_TICKS, 4 SLEEP, 16 write, 200 READ_BLOCK, 201 WRITE_BLOCK |
| `ascend-linux` (xv6) | 1 fork, 2 exit, 3 wait, 5 read, 7 exec, 8 fstat, 10 dup, 11 getpid, 13 pause, 15 open, 16 write, 17 mknod, 21 close, 200, 201 |

The declared surface must equal the object's: `surface_id(declared) != UPC_SURFACE_ID` is
`ValidationError::SurfaceObjectMismatch`, fail-closed. Equality, not subset: the descriptor is the
documented surface of one build, and a hash can only be compared for equality.
`cmd/upc-bootctl/tests/parity_surface.rs` parses the dispatch chain and fails when an arm is added or
removed without `surface.rs`. The INT 0x80 Linux-compatible path (`vector == intr::VECTOR_SYSCALL`,
default build only) is a second entry vector no shipped guest uses and is not part of either surface.

Corrections made while wiring this:

- §3 says the Doom build carries only DRAW_FRAME/GET_KEY/GET_TICKS/SLEEP. It also carries write
  (16), READ_BLOCK (200), WRITE_BLOCK (201) on the ecall path, and the INT 0x80 path.
- doom-runner declared Doom's numbers as 0-3, copied from `demos/doom/doomgeneric_unheaded.c`; the
  object dispatches 1-4 (`monad_common::mbc_syscalls`). The descriptor now uses the object's
  numbers. `demos/doom/doomgeneric_unheaded.c` still defines 0-3.
- upc-bootctl declared all 21 xv6 syscalls; the object has no arm for pipe 4, kill 6, chdir 9,
  sbrk 12, uptime 14, unlink 18, link 19, mkdir 20 (they fall through to "silently ignore"). The
  descriptor now lists the 15 the object handles.

### 6.3 Gate B: per-pid slice geometry

A `MemoryModel::PerPidMmu` workload carries `mmu: Some(MmuLayout { slices: SliceGeometry, reserved:
Vec<MemRegion> })`; a `Flat` one carries `None` (any other pairing is `MmuLayoutMismatch`).
`SliceGeometry { base, stride, slice_len, ceiling, max_pids }` describes child pids only: pid 0 is
identity-mapped over the kernel's low memory (ADR-074) and is not a slice. The slices checked are
the ones fork() can create: pid `p` in `1..max_pids` with `base + (p-1)*stride + stride <= ceiling`
(the capacity guard in the xv6 `SYS_fork` arm), each mapping `[base + (p-1)*stride, +slice_len)`.
Checks, in order, on the declared geometry: `slice_len > 0` (`EmptySlice`); `1 <= max_pids <= 256`
(`MaxPidsOutOfRange`; pids are `u8`); no slice arithmetic overflows `u32` (`SliceOutOfRange`; the core
computes it wrapping); `stride >= slice_len` (`SliceOverlap` with the next pid); each slice ends
inside `RAM_MAP` (`SliceOutOfRange`) and overlaps no reserved region (`SliceOverlap`). Then the
declared geometry must equal `UPC_MMU_GEOMETRY` (`SliceGeometryMismatch`; absent:
`ObjectGeometryMissing`), so what passed is what the interpreter ships.

The xv6 layout (`cmd/upc-bootctl/src/geometry.rs`, pinned against the sources by
`tests/parity_slice_constants.rs`):

| Value | Constant | Source |
|---|---|---|
| base 0x0100_0000 (16 MiB) | `USER_CHILD_SLICE` | `ebpf/monad-cpu-ebpf/src/phase12.rs` |
| stride 0x0080_0000 (8 MiB) | `SLICE_STRIDE` | `phase12.rs` |
| slice_len 0x0080_0000 | `USER_VA_CEILING` (two 4 MiB superpage leaves) | `ebpf/monad-cpu-ebpf/src/main.rs` |
| ceiling 0x0380_0000 (56 MiB) | `RAM_ISOLATION_CEILING` | `phase12.rs` |
| max_pids 8 | `mbc_interrupts::MAX_PROCESSES`; `PROC_TABLE` `max_entries(8)` | `monad-common`; `main.rs` |
| reserved [0, 0x0080_0000) | kernel, pid-0 identity slice, MMIO windows; `-DPHYSTOP=0x00800000UL` | `crates/xv6-mbc/adapters/Makefile.mbc` |
| reserved [0x0080_0000, +4 MiB) | ramdisk; `mbc_block::RAMDISK_BASE_WORD`, `RAMDISK_SIZE` | `monad-common`; `boot --ramdisk` |
| reserved [0x00C0_0000, +5 × 0x4_0000) | resident program `.data` staging | `cmd/upc-bootctl/src/geometry.rs` |
| reserved [0x00F0_0000, +8 × 4 KiB) | per-pid pgds; `MBC_PER_PID_PGD_BASE`, `PER_PID_PGD_BASE` | `adapters/uvm.c`; `phase12.rs` |
| reserved [0x0380_0000, 0x03F0_0000) | kernel boot stack (initial SP 0x03F0_0000) | `cmd/upc-bootctl/src/runner.rs` |

Admitted slices are pids 1-5 at [16, 56) MiB. The owned kernel (`adapters/*.c`) computes no child
slice; it only places page directories. Doom passes `mmu: None`.

`upc-bootctl --unsafe-geometry` reads `UPC_SLICE_STRIDE` and declares that stride instead, with a
warning; without the flag the variable is ignored with a warning. The object's stride is a
compile-time constant, so a stride below 8 MiB is refused with `SliceOverlap` and any other
non-shipped value with `SliceGeometryMismatch`: the hook can only make the loader refuse.

## 7. Image map freeze (S-ASCEND-2 Steps 157-168, amended 2026-10-28)

Load order: `.rv2mbc` SHA gate → object preflight (§6) → `validate` + `validate_against_object` →
`ROM_MAP` / `RV2MBC_MAP` writes → `ImageSink::finalize` (freeze) → XDP attach. `finalize` freezes both
maps with `bpf(BPF_MAP_FREEZE)` on the fd aya holds (aya 0.13 has no public freeze); afterwards every
userspace update of either map fails `EPERM`. The program side is unaffected: `monad-cpu-ebpf` only
reads the two maps. They are declared with flags 0, not `BPF_F_RDONLY_PROG`. `upc_api::load` is
gates, writes, finalize, for a loader with one image (doom-runner). `upc_api::write_image` is the same
without the freeze, for a loader that writes several: upc-bootctl writes the kernel (and bootstub) image
and the kernel `.rv2mbc` directly, init and the resident programs through `write_image`, then calls
`upc_api::finalize` once in `boot_prepare`, before `netns::prepare` and `attach_xdp`. A failed freeze
(`LoadError::Finalize`) aborts the boot before attach. `upc-bootctl boot --freeze-check` writes slot 0
of each map back with its own value after the freeze and fails the boot unless both writes return
`EPERM`. The Go `pkg/ebpf/mapfreeze.go`, which described this sequence for a Go loader, is deleted.
