# ADR-079 — RET return-address tagging (retire the MBC-vs-RV magnitude floor)

**Status**: ACCEPTED — **IMPLEMENTED + xv6 corpus regression-green 2026-07-02**
**Date**: 2026-07-02
**Deciders**: Fable 5 session under Marshal Phase 2.0 preflight; recommended by the
capacity audit (`references/phase2-preflight-capacity-audit-2026-07-02.md`)
**Aligns with**: ADR-067 (MBC ISA v2), ADR-077 (per-process rv2mbc_base), ADR-078 (Gate D)
**Supersedes**: the `RET_RV_FLOOR` value-disambiguation scheme (Gate D.1, commit `a769d5ee`)
**Implementation**: `ebpf/monad-cpu-ebpf/src/main.rs` (RET/CALL/CALLR), loader guard in
`cmd/upc-bootctl/src/main.rs`

## 1. Context — why the magnitude floor is terminal

The eBPF interpreter runs a hybrid model: xv6 is translated RV32→MBC offline, but
indirect branches and kernel→user returns are resolved at runtime through `RV2MBC_MAP`
(keyed by RV word address). So `r14` (the link register) can hold either:

- **(a)** an MBC PC saved by a prior `CALL`/`CALLR` (the compiled-RV path — save/restore
  `ra` through the stack, then `RET`), or
- **(b)** a raw RV byte address written by C (`p->context.ra = (uint64)&forkret`), which
  `RET` must translate via `RV2MBC_MAP`.

The original disambiguation compared `r14` against a fixed floor: below = MBC PC (jump
direct), at/above = RV address (translate). This is **fragile by construction** — it
assumes `max(MBC PC) < floor <= min(RV address)`. Gate D.1 already had to raise the floor
`0x10000 → 0x20000` when wc's ROM base (`0x12000`) crossed it, producing the `wc README`
runaway (wc's own return addresses misparsed as RV pointers → jumps into kexec/freewalk).
At Linux scale it is **unfixable**: a >512 KB MBC image pushes MBC PCs past any fixed
floor, and the kernel's RV `.text` base cannot stay pinned at `0x20000`.

## 2. Decision — tag MBC return addresses with bit 31

`CALL` and `CALLR` store `cpu.pc | 0x8000_0000` into `r14` (`RET_MBC_TAG`). `RET` checks
bit 31:

- **tag set** → an MBC PC: untag (`r14 & !RET_MBC_TAG`), jump direct.
- **tag clear** → a raw RV address (or a stale 0): `RV2MBC_MAP` lookup (unchanged path).

Bit 31 is free and unambiguous: MBC PCs are `ROM_MAP` word indices (max `0x40000`) and
every guest RV address is `< 0x8000_0000` (kernel `.text` ≈ `0x20000`, user VA < 8 MiB,
stack/RAM < 64 MiB). A tagged value round-trips through the guest stack (`SW ra`/`LW ra`)
transparently — it is a plain 32-bit word and `ra` is never arithmetic'd between save and
restore.

This is strictly **more** correct than the floor, not just more scalable: a normally
descheduled task has a TAGGED `context.ra` (saved by `swtch`'s own `CALL`), while a
freshly-forked task has an UNtagged `context.ra` (`= (uint64)forkret` set by C). The tag
distinguishes them by origin, not by a magnitude coincidence.

## 3. Scope

- **eBPF interpreter only.** The standalone host MBC emulator (`monad-mbc/src/execute.rs`)
  uses a separate stack-based CALL/RET with no link register and no rv2mbc — it runs pure
  MBC, never sees raw RV addresses, and needs no tag.
- The `upc-bootctl` loader's floor guard is replaced by a plain `ROM_MAP` capacity check
  (a program's ROM must fit 262,144 words) — the base-below-floor constraint is gone,
  which is what unblocks Linux-scale images.

## 4. Consequences

- **Phase 2 unblocked (prerequisite #1 of 2).** The capacity audit named RET tagging a
  Phase 2 substrate prerequisite; it is now done. The remaining prerequisite is the
  code-store strategy pair call (grow-maps / demand-translate / RAM-resident).
- **Doom (hypothesis, needs Marshal-supervised confirm).** Doom MBC PCs span
  `[0x2, 0x151BF]`; under the old floor every Doom return in `[0x10000, 0x151BF]`
  misparsed as an RV address. Tagging makes that misparse **impossible by construction**,
  so this is expected to fix (part of) the documented Doom PC-corruption blocker. The
  non-ascend config compiles; a doom-runner regression run under Marshal oversight
  (`feedback_marshal_oversight`) is required before the Doom win is claimed.
- **Cost**: verifier budget 847,943 → 848,188 (+245 insns, +0.03%) — effectively free.

## 5. Verification (2026-07-02, ascend-linux, kernel 6.17)

- xv6 corpus green: `ls` real root listing, `cat README` full fixture, `echo hello` →
  `hello`, `wc README` → `5 49 283 README`.
- Gate C echo baseline byte-identical (`forks=2 tty_r=5 waitpid=1 halt=0x0`).
- gate2 `ISOLATION-PASS`; gate_nway `NWAY-FAIL pid2` (pre-existing).
- Both eBPF configs compile; `UPC_VERIFIER_STATS=1` reports 848,188 insns.
