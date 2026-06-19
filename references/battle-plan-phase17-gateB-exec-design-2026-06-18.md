# Phase 1.7 Gate B design — `exec` → live `sh` prompt (the RV2MBC-collision crux)

**Date:** 2026-06-18
**Owner:** Computermancer (+ Architect on the ABI call; Developer on the loader)
**Status:** **GATE B SHIPPED 2026-06-19** — `exec("sh")` succeeds and sh emits its `$` prompt
(ADR-077 §Implementation status; commits `2766027b`…`cb94c406`). The §Decision resolved to A+B1
(loader-side pre-translation + per-process `rv2mbc_base` in a feature-gated `MbcCpuState`). Gate A
landed prior. Remaining: Gate C (interactive line — `read()` console-in drain + sh's post-`$`
halt). Successor to `battle-plan-phase17-real-exec-to-shell-2026-06-01.md`.

## What landed this session (Gate A — console fd substrate)

Decision-independent; every FS/exec approach (A/B/C) needs it, so it was safe to build ahead of
the §Decision.

- **`fdtable.rs`** — per-process fd table model: `NOFILE=16`, `FD_TABLE_LEN=128` (8 pids ×
  NOFILE), kinds `FD_FREE`/`FD_CONSOLE`, `fd_slot(pid,fd)`, `lowest_free(row)` (xv6
  lowest-numbered-fd policy). 6 unit tests, all green (run standalone — the crate is
  `#![no_std]`/`#![no_main]` so its `#[cfg(test)]` mods are pure-logic specs, same as
  `phase12.rs`).
- **`FD_TABLE` BPF map** (`Array<u32>`, 128 entries) + `fd_kind`/`fd_set_kind`/`fd_alloc`
  helpers (all bound-check `fd < NOFILE` so a garbage fd can't index a neighbour's row).
- **xv6 ecall handlers** in the `op::SYSCALL` branch (xv6 numbers, no collision with the
  INT-0x80 `lsys` numbers which live under `op::INT`): `open`(15)→lowest-free CONSOLE fd,
  `mknod`(17)→success-noop, `dup`(10)→dup kind onto lowest-free, `close`(21)→free,
  `read`(5)→console-fd **DEFERRED to Gate C** (see boot-verify note below), all fd-table-checked.
- **fork fd-inheritance** — the child copies the parent's `FD_TABLE` row, so init's console
  fds 0/1/2 survive `fork` → (future) `exec sh`.

Behaviour is **additive**: init previously "succeeded" at open/dup by the unknown-syscall
noop returning a ≥0 garbage value; now those return real fds 0/1/2.

### Boot-verify (2026-06-18) — Gate A PASS, after a verifier fix

The runtime regression check was **run** (not deferred): `--features ascend-linux` rebuild +
`upc-bootctl boot` of init / gate2 / gate_nway.

- **First boot FAILED to load**: `BPF program is too large. Processed 1000001 insn (limit
  1000000)` — the verifier's *complexity* ceiling, not the static program size. Root cause:
  the `read`(5) handler called `translate_address()` (a full Sv32 page-table walk) **once per
  byte inside an 8-iteration loop**. That 8× path explosion, stacked on the pre-existing
  INT-0x80 `lsys::SYS_READ` drain loop in the same monolithic `try_monad_cpu`, crossed 1M.
- **Fix**: `read`(5)'s drain loop was **deferred to Gate C** (init never `read`s before
  exec'ing sh, so it is not on the Gate A regression path). The handler keeps the cheap
  `fd_kind` console check and returns 0 bytes (poll-again). Gate C must restore the drain with
  a verifier-friendly shape: **translate the buffer base ONCE outside the loop** (the ≤8
  console bytes are consecutive and overwhelmingly intra-page), not per byte.
- **After the fix, all three boots PASS:**
  - **init** — boot output unchanged: clean `init: starting sh` / `init: exec sh failed`
    loop (open/dup/close + fork fd-inheritance exercised; CPU advances to priv=3 user mode).
  - **gate2** — `P: ISOLATION-PASS` / `C: inherited-ok` / `C: clobbered-own-slice` loop;
    per-pid isolation intact across the new fork fd-inheritance path.
  - **gate_nway** — `NWAY-ok pid1` then deterministic halt at `insn_count=5354501`. Verified
    **byte-identical to the committed baseline** (stash-revert `main.rs` → rebuild → same halt
    point + same single line) ⇒ Gate A is behavior-neutral; the halt-after-pid1 is a
    pre-existing looped-fork substrate condition (the Phase-3 caveat in `gate_nway.c`), not a
    Gate A regression.

`fstat`(8) and `exec`(7) intentionally NOT added yet: `fstat` needs the xv6 `struct stat` RV32
layout (lands with the FS reader); `exec` is Gate B (below).

## The §Decision (still open — Stevie's pick per the predecessor plan)

FS/exec placement: **(A) loader-side pre-translation** [recommended], (B) BPF-side FS walk +
live RV→MBC translate, (C) revive xv6 `fs.c`/`exec.c`. (A) is consistent with the Phase 1.2
loader-side large-pages call and is the shortest path to a prompt. The rest of this doc assumes
(A).

## The crux nobody had written down: RV2MBC collisions across resident programs

Under (A) the loader pre-translates each userland program (`sh ls cat echo wc`, already built
by `Makefile.mbc-userland`) and `exec` becomes a table lookup + slice load + PC set. The
non-obvious blocker:

- **`cpu.pc` is an absolute MBC index** into the shared `ROM_MAP`. Fine — give each program a
  disjoint ROM region (init already loads at `USER_ROM_BASE=0x4000`; sh at `0x8000`, etc.) and
  loader-patch its `CALL` immediates by that base (the existing init path already does this).
- **`RV2MBC_MAP` is shared and indexed by RV word address** (`rv_addr>>2`). It resolves every
  indirect branch: `JMPR`/`CALLR` (function pointers, returns-through-pointer), and critically
  **the kernel→user entry** — `exec` returns to the new image via the trap-return `SRET`, which
  looks up `RV2MBC_MAP[sepc>>2]`.
- **Every xv6 user program is linked by `user.ld` at RV byte 0.** So init's entry and sh's
  entry both want `RV2MBC_MAP[0]`, and their whole text ranges collide at RV-words `0..N`.
  A single shared map cannot hold both. And they genuinely coexist: init is pid 0 (identity
  slice); it forks pid 1, whose image `exec` then *replaces* with sh — so after exec, pid 0
  must still resolve init's branches while pid 1 resolves sh's. **One shared `RV2MBC_MAP[0]`
  can't serve both pids.** This is THE thing to solve for Gate B.

### Two ways to disambiguate (pick one — engineering call, recommendation given)

> **Scouting addendum (2026-06-18) — the B1 ABI blast radius is bigger than first written:**
> - **Size is 136→144, NOT 140.** `monad-common` enforces `size_of::<MbcCpuState>() % 8 == 0`
>   (lib.rs:1636); 140 fails it. The add is `rv2mbc_base: u32` + 4 bytes pad → 144.
> - **Three required mirrors, one of them is Doom.** `monad_common` (BPF), `cmd/upc-bootctl/
>   src/runner.rs` (host writer/reader), **and `crates/doom-runner/src/main.rs:55`** — doom-runner
>   keeps its own `MbcCpuState` that must match the live BPF `CPU_MAP` layout, so the struct grow
>   forces a mechanical, behavior-neutral edit to the Doom subsystem (append field + pad, bump its
>   `assert(==136)`→144). `crates/monad-mbc/cpu.rs` has an *independent* emulator struct that does
>   NOT touch the BPF map → leave it at 136.
> - **Alternative that avoids the Doom touch: keep `rv2mbc_base` in `PROC_TABLE` only** (slot 21,
>   already sized) and read it per indirect-branch instead of from `cpu`. Cost: a `PROC_TABLE`
>   map-lookup in the hot `JMPR`/`CALLR`/`MRET`/`SRET` paths — and Gate A *just* showed map-lookups
>   in hot loops blow the 1M verifier-complexity budget, so this trades the Doom touch for real
>   verifier-budget risk. Net: MbcCpuState-field is verifier-safe but touches Doom; PROC_TABLE-only
>   leaves Doom alone but must be budget-checked. **Open: Stevie's call before the 3-crate edit.**

> **RESOLVED 2026-06-18 — Stevie's call: B1 (MbcCpuState field), but Doom MUST keep building +
> running. Vetted by Developer + Scientist + Micromanager. Canonical decision record:
> `docs/adr/ADR-077-phase17-rv2mbc-base-feature-gated-abi-fork.md`.**
>
> **Insulation design (feature-gated ABI fork):** put `rv2mbc_base: u32` + 4-byte pad behind a NEW
> `ascend-linux` feature on `monad-common`, forwarded from `monad-cpu-ebpf`
> (`ascend-linux = ["monad-common/ascend-linux"]`). Result: non-ascend `MbcCpuState` = **136**
> (Doom), ascend = **144** (Linux). The field is appended AFTER `reservation_address`, so under
> `repr(C)` every existing field offset is preserved.
> - **cfg/asserts:** field + its Default init are `#[cfg(feature = "ascend-linux")]`; the size
>   const-assert splits `#[cfg(not(...))] ==136` / `#[cfg(...)] ==144`; the `%8==0` runtime test
>   (lib.rs:1636) holds for both. Host mirror `cmd/upc-bootctl/src/runner.rs` → 144 (ASCEND-only
>   loader). `crates/doom-runner` + `crates/monad-mbc/cpu.rs` keep their OWN 136-byte structs —
>   UNTOUCHED.
> - **Why this is safe for Doom (Scientist):** Doom & ASCEND load `monad-cpu-ebpf` from the same
>   `target/` path rebuilt with/without the feature (the known clobber footgun). Append-only +
>   Doom-never-reads-`rv2mbc_base` means even a cross-load is safe: a 136-byte mirror over a
>   144-byte map value reads all Doom fields (unmoved) correctly and ignores the trailing 8 bytes.
>   `doom-runner`'s hand-rolled struct is immune to cargo feature-unification (it doesn't use
>   `monad-common`'s type). The residual risk is purely "did the right object get built before the
>   run," which the DoD's Doom *run* proof covers.
> - **Definition of Done (Micromanager — all MUST pass before this lands; none skippable):**
>   - **G1 ASCEND boot proof:** `--features ascend-linux` build, 144 assert compiles; init boot
>     still shows the `init: starting sh` / `exec sh failed` loop; `gate2` still `ISOLATION-PASS`.
>   - **G2 Doom regression proof:** non-ascend build, 136 assert compiles; **doom-runner actually
>     RUNS** (smoke: renders/advances frames as before) — build-only is NOT sufficient.
>   - **G3 invariant:** `size_of % 8 == 0` test green in both feature configs.
>   - Tempting-to-skip gate flagged: **G2's run** (not just compile). Do not skip it.
> - **Scope:** in-scope for Gate B (B1 is its foundation); the feature-gating is justified Doom
>   insulation, not creep. The `exec(7)` handler is the remaining/separate Gate B increment.

**(B1) Per-process `rv2mbc_base` in CPU state [RECOMMENDED].**
Add a `u32 rv2mbc_base` to `MbcCpuState` (136→144 bytes; see scouting addendum). Load each program's `.rv2mbc` at a
distinct map base; `exec`/context-switch set `cpu.rv2mbc_base` for the running image. The four
indirect-branch lookups (`JMPR`, `CALLR`, and the `MRET`/`SRET` user-entry paths) become
`RV2MBC_MAP[rv2mbc_base + (rv_addr>>2)]`. Persist `rv2mbc_base` in `PROC_TABLE` (it has room —
currently `[u32;21]`, slot 21 is the natural home) so a context switch restores it.
- **Cost:** a real but *mechanical* ABI change — `MbcCpuState` in `monad_common` + the host
  mirror in `cmd/upc-bootctl/src/runner.rs` (+ its `const _: () = assert!(size==140)`), and
  ~4 lookup sites in the hot fetch loop. Pre-alpha; not a frozen-ABI milestone like the
  `priv_level` add was, so the ceremony is lighter.
- **Why preferred:** programs keep linking at RV 0 (no `user.ld` per-program surgery); `.data`
  VAs stay small and identical per program so they land cleanly in each pid's slice; the model
  matches a real per-process address space and generalizes to N programs for free.

**(B2) Loader-shift each program to a distinct RV base [no ABI change, but heavier].**
Assign init=RV0, sh=RV0x00100000, ls=0x00200000, … and loader-shift every program's `.rv2mbc`
indices, `.data` byte addrs, and the `sepc` exec sets — so the shared map never collides and no
per-process base is needed.
- **Cost:** each program's `.data`/stack now lives at a distinct VA that must be loaded into the
  *running pid's physical slice* at exec time (the per-pid pgd maps `VA[0,8MiB)`→slice, so the
  shifted VAs must stay < 8 MiB and be physically staged per-pid by `exec`). More exec-time
  copying + a packing constraint; avoids touching the CPU struct.

**Recommendation: B1.** Cleaner, generalizes, and the ABI delta is small/mechanical. B2 trades a
4-byte struct field for per-exec physical staging complexity and a slice-packing constraint.

## Gate B work items (assuming A + B1)

1. **Host image builder (`upc-bootctl`/Makefile).** Translate `sh ls cat echo wc` (reuse
   `monad_mbc::Translator::translate_program_with_map` as a lib, or the existing per-program
   `.mbc/.rv2mbc/.data` artifacts). Lay each into a disjoint ROM region + a disjoint
   `RV2MBC_MAP` base; emit a **program table** — a small BPF map `PROGRAM_TABLE` keyed by a
   name hash (8-byte FNV of the basename) → `{rom_base, rv2mbc_base, entry_rv, data_staged_off,
   data_len}`. The CALL-immediate `+rom_base` patch already exists for init; generalize it.
2. **`exec`(7) handler** (`op::SYSCALL` branch). `exec(path=a0, argv=a1)`:
   - Resolve `path` basename → `PROGRAM_TABLE` (string compare/hash over the user VA bytes;
     bounded loop, verifier-cheap).
   - Build/clear the current pid's address space (reuse the Gate-1 pgd/leaf build), stage the
     program's `.data` into the pid's slice, zero `.bss`.
   - Lay `argv`/`argc` on a fresh user stack; set `cpu.rv2mbc_base` + persist to `PROC_TABLE`;
     set the trap-return `sepc = entry_rv`; return to user via the existing `SRET` path.
   - On miss → return -1 (so a bad exec still prints `exec … failed`, not a crash).
3. **Context-switch** save/restore of `rv2mbc_base` (next to the existing `page_dir_base` slot).
4. **`fstat`(8)** minimal console stat (lands with #1's FS reader for `cat`).

## Verification gates (unchanged from predecessor)

- **Gate A** (console stdio) — ✅ **DONE + boot-verified 2026-06-18.** Boot still shows
  `init: starting sh` (no regression); gate2 + gate_nway regressions pass. Required a verifier
  fix (deferred `read`(5) drain to Gate C — see Boot-verify note above). Uncommitted (Stevie
  owns commits).
- **Gate B** (exec launches sh) — ✅ **DONE 2026-06-19.** child `exec("sh")` stops printing
  `exec sh failed`; `sh` runs in its pid-1 slice and emits its first byte (`$`). Headline.
  Verifier budget was freed by gating the dead INT-0x80 Linux/FUZIX dispatch (~560 lines) to
  non-ascend (`!cfg!(feature = "ascend-linux")` → rustc dead-eliminates it). The exec data-copy
  reads the path by direct slice arithmetic (`pid_phys_offset + va`), NOT `translate_address`,
  and stages `.data` re-entrant 16-words/tick — both to stay under the 1M complexity ceiling.
- **Gate C** (interactive line) — feed bytes via tty-bridge; `sh` `read`s a command, fork+execs
  `ls`/`echo`, output appears. (`read`(5) console-in already wired this session.)
- **Budget:** measure verifier % after Gate B (Gate A added ~5 small arms + 3 helpers + a
  NOFILE-bounded fork loop — modest). Hard gate < 12%. `scripts/bpf-verifier-check.sh` clobbers
  the ascend object — rebuild `--features ascend-linux` after running it.

## Build/boot recipe (unchanged — see predecessor plan + CLAUDE.md ASCEND-LINUX block)

`--features ascend-linux` is MANDATORY; run `upc-bootctl` from `ebpf/`; never trust a boot whose
rebuild didn't print `Compiling monad-cpu-ebpf`.
