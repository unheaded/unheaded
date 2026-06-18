# ADR-076 — ASCEND-LINUX Phase 1.7 Gate B: Feature-Gated `MbcCpuState` ABI Fork (per-process `rv2mbc_base`, Doom-insulated)

**Status**: ACCEPTED (Stevie's call, 2026-06-18)
**Date**: 2026-06-18
**Deciders**: Stevie + unheaded-developer + unheaded-scientist + unheaded-micromanager (consulted in-session)
**Aligns with**: ADR-067 (MBC ISA v2 + UPC ABI v1 — froze `MbcCpuState` at 136 bytes), ADR-074 (Phase 1.2 per-pid page-table model), ADR-075 (Phase 1.3 process model), ADR-052 (in-tree source-of-truth)
**Triggers**: Phase 1.7 Gate B (`exec` → live `sh` prompt). Design lives in `references/battle-plan-phase17-gateB-exec-design-2026-06-18.md`; predecessor `references/battle-plan-phase17-real-exec-to-shell-2026-06-01.md`.
**Implementation status at adoption**: NOT YET IMPLEMENTED. This ADR is the spec a future engineer implements against; it must pass the acceptance gates in §7 before it lands.

---

## 1. Context

### The collision crux (why a per-process base is needed)

Under Gate B approach **A** (loader-side pre-translation — Stevie's §Decision), the host loader
pre-translates each xv6 userland program (`sh`, `ls`, `cat`, `echo`, `wc`) to MBC and `exec(7)`
becomes table-lookup + slice-load + PC-set. The non-obvious blocker:

- `RV2MBC_MAP` is a **single shared** BPF map indexed by RV word address (`rv_addr >> 2`). It
  resolves every indirect branch — `JMPR`/`CALLR` (function pointers, returns-through-pointer)
  and the kernel→user trap-return (`MRET`/`SRET`, which looks up `RV2MBC_MAP[sepc >> 2]`).
- **Every xv6 user program is linked by `user.ld` at RV byte 0.** init's entry and `sh`'s entry
  both want `RV2MBC_MAP[0]`; their whole text ranges collide at RV-words `0..N`. They genuinely
  coexist: init is pid 0; it forks pid 1, whose image `exec` replaces with `sh` — so after exec,
  pid 0 must still resolve init's branches while pid 1 resolves `sh`'s. **One shared
  `RV2MBC_MAP[0]` cannot serve both pids.**

**Resolution:** give each resident program a disjoint `RV2MBC_MAP` region and select it per
process with a `rv2mbc_base`: every indirect-branch lookup becomes
`RV2MBC_MAP[rv2mbc_base + (rv_addr >> 2)]`. `rv2mbc_base` is a per-process property, restored on
context switch from `PROC_TABLE` (slot 21, already sized `[u32; 21]`).

### Where to store `rv2mbc_base` — and the Doom problem

Two storage placements were on the table:

- **In `MbcCpuState`** (the per-tick CPU state already loaded every fetch): the hot-loop read is a
  cheap field access — **verifier-safe**, which matters because Gate A's boot-verify (2026-06-18)
  blew the BPF verifier's 1,000,000-processed-insn complexity ceiling with `translate_address()`
  in a per-byte loop. Adding map lookups to the hottest paths is exactly the wrong direction.
- **In `PROC_TABLE` only**: no CPU-struct change, but every indirect branch
  (`JMPR`/`CALLR`/`MRET`/`SRET` — the hottest paths) gains a `PROC_TABLE` map lookup → real
  verifier-budget risk.

The `MbcCpuState` placement is verifier-safe but collides with a hard constraint: **`MbcCpuState`
is mirrored in multiple crates and consumed by the working Doom subsystem.**

- `ebpf/monad-common/src/lib.rs` — canonical, compiled into the BPF program. Enforces
  `size_of::<MbcCpuState>() % 8 == 0` (lib.rs `#[test]`). **136 is 8-aligned; 140 is NOT** — so the
  field add is `rv2mbc_base: u32` + 4 bytes padding → **144 bytes, not the 140 first sketched.**
- `cmd/upc-bootctl/src/runner.rs` — host mirror used by the ASCEND boot loader (writes/reads
  `CPU_MAP`); has its own `assert!(size_of == 136)`.
- `crates/doom-runner/src/main.rs` — **the Doom workload** keeps its own `MbcCpuState` mirror that
  must match the live BPF `CPU_MAP` layout it loads.
- `crates/monad-mbc/src/cpu.rs` — an **independent** pure-Rust emulator struct that does NOT touch
  the BPF `CPU_MAP`; unaffected by this change.

Stevie's directive: **add `rv2mbc_base` for ASCEND-LINUX without breaking working Doom.** Doom must
continue to build AND run.

---

## 2. Decision

Add `rv2mbc_base` to `MbcCpuState` **behind a new `ascend-linux` Cargo feature on
`monad-common`**, forwarded from `monad-cpu-ebpf`. This forks the ABI along the *already-existing*
`ascend-linux` boundary (the same flag that gates MRET/SRET/LR.W/SC.W per ADR-067):

- **Non-ascend build** (Doom, default): `MbcCpuState` stays **136 bytes**, byte-identical to
  ADR-067. Doom is untouched.
- **Ascend build** (`--features ascend-linux`): `MbcCpuState` is **144 bytes** with the new field.

### 2.1 Concrete changes (the only files this touches)

1. `ebpf/monad-common/Cargo.toml` — add a `[features]` section with `ascend-linux = []`.
2. `ebpf/monad-cpu-ebpf/Cargo.toml` — forward it: `ascend-linux = ["monad-common/ascend-linux"]`
   (the crate already declares `ascend-linux = []`; change to the forwarding form).
3. `ebpf/monad-common/src/lib.rs` — in `struct MbcCpuState`, **append after `reservation_address`**
   (the current last field, so `repr(C)` offsets of all existing fields are preserved):
   ```rust
   // ── Phase 1.7 Gate B (ADR-076): per-process RV2MBC base, ASCEND-only ──
   #[cfg(feature = "ascend-linux")]
   pub rv2mbc_base: u32,
   #[cfg(feature = "ascend-linux")]
   pub _pad7: [u8; 4],   // keep size_of % 8 == 0 (136 → 144)
   ```
   - Gate the field's `Default` initializer with the same `#[cfg]`.
   - Split the size const-assert:
     ```rust
     #[cfg(not(feature = "ascend-linux"))]
     const _: () = assert!(core::mem::size_of::<MbcCpuState>() == 136);
     #[cfg(feature = "ascend-linux")]
     const _: () = assert!(core::mem::size_of::<MbcCpuState>() == 144);
     ```
   - The existing `size_of % 8 == 0` test holds for both (136 and 144).
4. `cmd/upc-bootctl/src/runner.rs` — grow the host mirror to 144 (append the same field + pad),
   update its two `136` asserts to `144`. (upc-bootctl is the ASCEND-only loader; it always loads
   the ascend object, so it is unconditionally 144 — no `#[cfg]` needed here.)
5. `ebpf/monad-cpu-ebpf/src/main.rs` — the four indirect-branch lookups read `cpu.rv2mbc_base`
   (these compile only under `--features ascend-linux`, where the field exists).

### 2.2 Explicitly NOT touched

- `crates/doom-runner/src/main.rs` — stays at 136. Doom's mirror is hand-rolled (does not use
  `monad-common`'s type), so cargo feature-unification cannot grow it.
- `crates/monad-mbc/src/cpu.rs` — independent emulator struct, stays at 136.

---

## 3. Why this is safe for Doom (Scientist analysis)

1. **`repr(C)` append-only ⇒ offset-preserving.** Fields lay out in declaration order; appending
   after `reservation_address` does not move any existing field. Doom reads the same offsets it
   always did.
2. **Cross-load is safe even under the clobber footgun.** Doom and ASCEND load `monad-cpu-ebpf`
   from the *same* `target/` path, rebuilt with/without the feature (the documented footgun:
   `scripts/bpf-verifier-check.sh` / a plain `cargo build` rebuilds WITHOUT the feature). If Doom
   ever loaded the 144-byte object through its 136-byte mirror, a BPF `Array` value read returns
   144 bytes; reading the first 136 yields every Doom field correctly (append-only) and ignores
   the trailing 8. Doom never reads `rv2mbc_base`, and a 136-byte write into a 144-byte slot leaves
   the tail zero — harmless. So the ABI mismatch is non-fatal *by construction*, not by luck.
3. **Cargo feature-unification immunity.** doom-runner's struct is independent of
   `monad-common`'s, so unification of `monad-common/ascend-linux` (off by default) within some
   build graph cannot silently grow Doom's struct. The only residual concern is "which object got
   built before the Doom run," which §7 G2 (a Doom *run* proof) covers.

---

## 4. Alternatives considered

| Option | ABI change | Touches Doom | Verifier risk | Verdict |
|---|---|---|---|---|
| **B1 feature-gated (this ADR)** | 136↔144 along `ascend-linux` | No (insulated) | None (field read) | **CHOSEN** |
| B1 unconditional | 136→144 everywhere | Yes (Doom mirror must grow) | None | Rejected — breaks Stevie's "don't touch Doom" |
| B2 loader-shift each program to a distinct RV base | None | No | None | Rejected — per-exec physical staging + slice-packing complexity |
| `PROC_TABLE`-only base | None | No | **High** (map lookup in hottest paths) | Rejected — Gate A just showed hot-path map lookups blow the 1M budget |

---

## 5. Consequences

- **Two ABIs to keep in lockstep.** The non-ascend (136) and ascend (144) layouts diverge only by
  the trailing 8 bytes. The cfg-split const-assert in `monad-common` is the tripwire that fails the
  build if either drifts.
- **`exec(7)` core is kept FS-source-agnostic** (forward-compat with option C — see §6).
- **No frozen-ABI ceremony.** ADR-067 froze the v1 ABI at 136; this is a feature-gated *extension*
  visible only in the ascend configuration, so the Doom/Doom-mode v1 ABI is preserved bit-for-bit.

## 6. Forward-compatibility with option C (revive xv6 `fs.c`/`exec.c`)

Stevie's constraint: design A so it benefits the eventual option C. The seam is `exec(7)`'s core —
**build per-pid pgd → stage `.data` → set `rv2mbc_base` → set `sepc` → SRET** — kept independent of
*how* the program bytes were located. Under A, the Rust host loader pre-fills a `PROGRAM_TABLE`;
under C, xv6's `exec.c` walks `fs.img` inodes and translates on demand. Both feed the same exec
core, and `rv2mbc_base` generalizes to on-demand translation (N programs, arbitrary bases) for free.
So none of this is throwaway when C arrives.

## 7. Definition of Done — acceptance gates (Micromanager; all MUST pass, none skippable)

- **G1 — ASCEND boot proof.** `cargo build --release -p monad-cpu-ebpf --features ascend-linux`
  compiles with the 144 assert; init boot still shows the `init: starting sh` / `init: exec sh
  failed` loop; `gate2` still prints `P: ISOLATION-PASS`. (Once `exec` lands: child `exec("sh")`
  stops failing and `sh` emits its first byte — the Gate B headline.)
- **G2 — Doom regression proof.** Non-ascend build compiles with the 136 assert AND **doom-runner
  actually RUNS** (smoke: renders/advances frames as before). **Build-only is NOT sufficient** —
  this is the gate most tempting to skip; do not skip it.
- **G3 — Invariant.** `size_of::<MbcCpuState>() % 8 == 0` test green in BOTH feature configs.
- **Budget.** Re-measure verifier headroom after `exec` lands; hard gate < 12% of the static
  budget AND the object must LOAD (the 1M processed-insn complexity ceiling is the real limit —
  see Gate A, 2026-06-18).

## 8. Implementation checklist (for the future engineer)

1. Add/forward the `ascend-linux` feature (§2.1 items 1–2).
2. Append the cfg-gated field + pad + Default init + split const-assert in `monad-common` (item 3).
3. Grow the `upc-bootctl` host mirror to 144 + asserts (item 4).
4. Run **G3** (both configs compile) and **G2** (Doom build + run) BEFORE writing any `exec` code —
   prove the ABI fork is Doom-safe in isolation first.
5. Then build the `PROGRAM_TABLE` host builder + `exec(7)` handler + context-switch save/restore of
   `rv2mbc_base` + `fstat(8)` (the rest of Gate B), reading `cpu.rv2mbc_base` at the four
   indirect-branch sites. Run **G1**.
6. Update `references/battle-plan-phase17-gateB-exec-design-2026-06-18.md` status and this ADR's
   implementation-status line when it lands.
