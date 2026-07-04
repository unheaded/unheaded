# ADR-081 — Unheaded Linux: build our own minimal OS on the UPC (do not vendor uClinux)

**Status**: ACCEPTED (Stevie's call, 2026-07-03)
**Date**: 2026-07-03
**Deciders**: Stevie (directive); Fable 5 session
**Supersedes**: the ASCEND-LINUX battle-plan Phase 2 step "vendor Linux 6.x + busybox"
(`references/battle-plan-ascend-linux-2026-05-08.md` §5)
**Relates to**: ADR-067 (MBC ISA v2 / UPC ABI), ADR-074/075/077/078 (the xv6 substrate: MMU,
process model, exec, in-BPF FS reader), ADR-080 (UPC programmatic API — Unheaded Linux is a new
guest workload), ADR-69420 (Yggdrasil / Unheaded OS golden image)

## Decision

Do **not** vendor uClinux 6.x + busybox for ASCEND-LINUX Phase 2. Instead, build **Unheaded
Linux** — our own minimal, from-scratch OS kernel — starting on the UPC substrate, 100% in-house
("us + kernel team"). Scale it up over time toward the **Yggdrasil golden-image** vision, applying
the existing soft-fork *discipline* (upstream-tracking + overlay + chain-of-custody per
`docs/OS-FORK-DISCIPLINE.md`) to our **own** tree rather than to a vendored Debian/Linux base.

## Why

- **Stevie's directive (2026-07-03):** as light/small as possible; DIY / build our own; "fuck
  busybox." (See the `project-ascend-linux-light-diy` memory.)
- **Small is mandatory, not aesthetic.** The two binding walls on the UPC — the eBPF verifier's
  **1M-instruction budget** and the **code-store map capacity** (ROM_MAP / RV2MBC_MAP) — both
  punish size. A vendored multi-MB Linux kernel fights both; a purpose-built minimal kernel can
  fit the *existing* maps (possibly dodging the `large-image` feature entirely) and keep its
  syscall surface under budget.
- **100% ownership.** No GPL-vendoring entanglement, no busybox bloat, every byte ours — matching
  the DIY reality that made Phase 1 fit (we hand-wrote every xv6 adapter/shim).
- **Proven substrate.** Phase 1 (xv6) already validated the hard parts on the UPC: ISA v2
  (MRET/SRET/atomics), per-pid MMU (ADR-074), process model + fork/exec/wait (ADR-075/077),
  in-BPF FS reader + argv (ADR-078), Boot Protocol v2. Unheaded Linux reuses all of it — in
  ADR-080's terms it is simply a new **guest workload** on the same programmatic UPC substrate.

## The through-line (the "golden-image soft-fork" vision)

```
UPC PoC (Doom → xv6)  →  UNHEADED LINUX  →  YGGDRASIL golden image
  proves completeness     our kernel,        the production OS substrate
                          born on the UPC     (ADR-69420)
```

ADR-69420 today frames Yggdrasil as a soft-fork of **Debian 12**. This decision opens the path for
Unheaded Linux to become (or feed) that substrate with a **100%-owned base** instead of a Debian
anchor — same soft-fork *discipline* (`OS-FORK-DISCIPLINE.md`: single anchor, bounded divergence,
chain-of-custody for compliance), a different, owned upstream.

## Open questions (resolve as we go — none block the first step)

1. **Kernel language** — C via the proven RV32I→MBC pipeline (like the xv6 adapters), or Rust?
   *Default:* C — reuse the working toolchain; revisit Rust for safety-critical pieces.
2. **Greenfield vs. evolve-from-xv6** — clean-sheet, or start from xv6's shipped adapters and
   replace the MIT code piece by piece while it keeps booting? *Default:* evolve — xv6 boots
   today; swap in our own code incrementally, never breaking the boot (the Doom/xv6 no-regression
   discipline).
3. **Linux-ABI compatibility target** — how "Linux" is Unheaded Linux? The full Linux syscall ABI
   is a large surface that fights the verifier budget; a minimal own-ABI is leanest. *Likely:*
   minimal own-ABI first, a Linux-ABI subset later only where it earns its keep.
4. **Yggdrasil reconciliation** — does Unheaded-Linux-on-UPC eventually replace the Debian anchor,
   stay UPC-only, or become a parallel bare-metal track? *Defer* to Age 2 Yggdrasil first-artifacts.
5. **Naming** — "Unheaded Linux" is the working brand; since it is from-scratch (not the Linux
   kernel), Lore should bless the name and disambiguate from the GPL "Linux" mark.

## Consequences

- The 2026-05-08 ascend battle-plan **Phase 2 (vendor uClinux + busybox, tasks 54–79) is
  SUPERSEDED**; a new Unheaded-Linux battle plan is needed (Warmonger).
- The two Phase 2.1 pair calls (LTS version pick; `arch/mbc` cargo-cult base) are **moot** — there
  is no upstream to vendor.
- The code-store pair call (grow-maps / demand-translate / RAM-resident) is **de-prioritized**: a
  small own-kernel may fit today's maps. `large-image` (option A, committed 2026-07-03) stays
  available if needed.
- **First step:** scope the minimal Unheaded Linux kernel (the smallest thing past xv6 that is
  meaningfully *ours*) and forge a Warmonger battle plan.

## Progress

- **2026-07-04 — Track 2 Phase 2.1 (Own PID 1) SHIPPED.** `user/uinit.c` (UNHEADED-INIT), the
  first Unheaded-authored program to run as PID 1 on the xv6-mbc kernel. Talks to the kernel only
  through the syscall ABI; booted via `upc-bootctl --userland target/uinit.mbc` with the kernel and
  fs.img unchanged. Gate: banner token `0xP1D1-0UR5` prints, sh forks+execs, `$` prompt returns,
  `echo hello` completes, child reaped. Zero eBPF change (ascend verifier 900,031 unchanged; Doom
  cannot regress). Two permanent gates in `scripts/upc-regression.sh`. Q2's "evolve, never break
  the boot" default is now practice, and Q5's Lore blessing began: `uinit` is the working name in
  the Ascension tradition.
