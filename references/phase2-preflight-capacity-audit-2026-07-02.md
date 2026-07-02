# Phase 2.0 Preflight — Capacity & Addressing Audit (Linux-scale)

**Date:** 2026-07-02 (Fable 5, Marshal-dispatched Phase 2.0 preflight GO-2)
**Question:** Does the substrate that runs xv6 (Gate D) scale to a uClinux 6.x nommu
image, or does Phase 2 need a substrate redesign first?
**Verdict:** **The offline-pretranslate-to-ROM_MAP model does NOT scale to Linux.** Three
independent hard walls, each exceeded ~2–8× by even a modest 2 MB kernel `.text`, plus a
verifier budget already at 84.8%. Phase 2 as written in the 2026-05-08 battle plan
(§5 "vendor Linux, port arch/mbc, cross-compile busybox") assumes a code-store model the
current substrate can't provide. **Substrate work is a Phase 2 prerequisite, not a step
inside it.**

## Measured baselines (all on kernel 6.17, ascend-linux object)

| Resource | Capacity | xv6 usage (kernel + 5 programs) | 2 MB Linux `.text` @ ~1:1 RV→MBC | Verdict |
|---|---|---|---|---|
| **Verifier** (interpreter complexity) | 1,000,000 processed insns (hard) | **847,943 (84.8%)** | interpreter unchanged, but any growth competes for the **15.2% (152 K) free** | ⚠️ near-full |
| **ROM_MAP** (MBC instruction store) | 262,144 words | ~0x12714 = 75,540 words | ~524,288 words | ❌ 2× over |
| **RV2MBC_MAP** (branch-target xlate) | 65,536 entries | kernel abs RV-word ≤ 0x9D80 = 40,320 | ~524,288 RV words | ❌ 8× over |
| **RAM_MAP** (guest phys memory) | 16,777,216 words = 64 MiB | PHYSTOP-capped at 8 MiB | nommu needs 16–32 MiB | ✅ headroom |
| **RET disambiguation floor** | `RET_RV_FLOOR = 0x20000` | MBC word idx ≤ 0x12714 < 0x20000; RV byte ≥ 0x20000 | MBC image > 512 KB pushes word idx ≥ 0x20000 | ❌ breaks |

How the numbers were taken:
- Verifier: new `UPC_VERIFIER_STATS=1` env knob on `upc-bootctl` prints
  `prog.info().verified_instruction_count()` after XDP load (`runner.rs::attach_xdp`).
  This is the ONLY reliable read for an Aya object — its legacy `maps` ELF section is
  rejected by libbpf 1.0+, so `veristat`/`bpftool prog loadall` both fail with "failed
  to open object file". bpftool `prog show` also does not surface `verified_insns` on
  this build.
- ROM/RV2MBC usage: `.mbc`/`.rv2mbc` file sizes + nonzero-index spans (see the inline
  python in the session commands). Resident ROM layout: `cmd/upc-bootctl/src/main.rs`
  — init@0x4000, then sh@0x6000, echo@0x9000, ls@0xC000, cat@0xF000, wc@0x12000
  (0x3000-word stride).

## The four walls, in detail

### 1. Verifier budget — 84.8% used (the tightest wall for *interpreter* growth)
Phase 2's plan gate "instruction count delta vs Phase 1 ≤ 30%" (task 78) is
**physically impossible**: 847,943 × 1.30 = 1.10 M > the 1 M ceiling. The real budget
for any new interpreter code (a demand-translator, an ELF segment loader, MMU-off
paging) is **152 K processed insns, ~18% relative growth**, before XDP load fails.
Note this bounds *interpreter* complexity, not guest code size — but every scaling
strategy below (esp. demand-translation) adds interpreter complexity.

### 2. ROM_MAP — 2× too small at 2 MB
The offline model pretranslates the *entire* kernel `.text` RV→MBC and resides it in
ROM_MAP (1 word/instruction). xv6's whole world is 75 K words; a 2 MB Linux `.text`
(~512 K RV instructions) needs ~512 K MBC words — 2× ROM_MAP, and Linux kernels run
larger. The plan's own risk table already flagged "Real Linux ELF image > ROM_MAP
capacity (1 MB) → kernel goes to RAM_MAP, multi-segment loader, design in Phase 2.1."
This audit quantifies it: **the resident-ROM model is out; Phase 2.1 must introduce
either a RAM_MAP-resident code path or on-the-fly translation.**

### 3. RV2MBC_MAP — 8× too small
Every indirect branch (JMPR/CALLR/MRET/SRET/RET-of-fnptr) looks up
`RV2MBC_MAP[rv_word]`. It is sized 65,536 = enough for xv6's 40 K-word RV `.text`. A
2 MB kernel's RV `.text` spans ~512 K words — 8× over. Growing the map is cheap
(it's a BPF Array), but a 512 K-entry Array is 2 MiB of locked memory per instance and
still assumes whole-kernel pretranslation (wall #2).

### 4. RET_RV_FLOOR value-disambiguation — unsound at scale (already a live footgun)
The RET handler splits `r14` by value: `< 0x20000` = MBC word-index return PC,
`>= 0x20000` = RV byte address needing an rv2mbc lookup. This works ONLY while
`max(MBC word index) < 0x20000 <= min(RV byte address)`. Gate D.1 (`a769d5ee`) already
had to raise the floor 0x10000→0x20000 when wc's ROM base (0x12000) crossed it, and
added a loader guard. At Linux scale an MBC image > 512 KB pushes word indices past any
fixed floor, and the kernel's RV `.text` base can't stay at 0x20000. **The value trick
is terminal.** The sound fix — deferred from Gate D.1 — is to **tag MBC return
addresses at CALL time** so RET never has to guess. This is now a **Phase 2 design
prerequisite**, not a nice-to-have (ADR candidate; ABI implication → Stevie call).

## Recommendation to Warmonger / Stevie

Phase 2.1 in the 2026-05-08 plan (tasks 54–60: vendor Linux, port arch/mbc) presupposes
the code-store problem is solved. It is not. **Insert a Phase 2.0 substrate epic before
2.1:**

1. ~~**RET address tagging** (ADR + impl) — removes the value-disambiguation ceiling.~~
   **✅ DONE 2026-07-02 (ADR-079).** CALL/CALLR tag MBC PCs with bit 31; RET tests the
   tag, not magnitude. Floor + loader guard retired. xv6 corpus green, verifier +0.03%.
   Also expected to fix the Doom PC-corruption misparse (Marshal-supervised confirm
   pending). Wall #4 (RET_RV_FLOOR) is now CLOSED.
2. **Code-store strategy decision** (pair call — this is the fork in the road):
   - **(A) Bigger resident maps** — grow ROM_MAP + RV2MBC_MAP to Linux scale (megabytes
     of locked memory/instance), keep whole-kernel offline pretranslation. Simplest;
     memory-heavy; still needs multi-segment loading.
   - **(B) Demand translation** — translate RV→MBC lazily in the interpreter on first
     execution of a page. Scales to any image; but adds interpreter complexity against a
     15% verifier budget — **high risk of blowing the ceiling** (measure a spike first).
   - **(C) RAM_MAP-resident code + interpreter fetch from RAM** — kernel image lives in
     the 64 MiB RAM_MAP (wall #4 has room), interpreter fetches/decodes from there
     instead of ROM_MAP. Middle path; the plan's own risk-table hint.
3. **Only then** 2.1 kernel bring-up.

Until the code-store decision is made, vendoring Linux is premature — we'd have a kernel
we cannot load. **Hold tasks 54–60; unblock task 53 (RET tagging) + a substrate pair
call first.**

## Cross-refs
- Verifier knob: `cmd/upc-bootctl/src/runner.rs` (`UPC_VERIFIER_STATS`).
- RET floor + Gate D.1: `references/phase17-gateD-shipped-2026-07-02.md`, commit `a769d5ee`.
- Phase 2 plan: `references/battle-plan-ascend-linux-2026-05-08.md` §5, risk table.
- Map sizes: `ebpf/monad-cpu-ebpf/src/main.rs` (ROM_MAP L74, RAM_MAP L84, RV2MBC_MAP L134).
