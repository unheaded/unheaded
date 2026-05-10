# Marshal Shift Report — Phase 1.1 SHIP Banner Shipped — 2026-05-10

**Plan**: `references/battle-plan-phase11-ship-2026-05-10.md` + `references/battle-plan-NORTH-STAR-2026-05-05.md`
**Owner**: Marshal (continuation from `marshal-shift-2026-05-10-phase11-stuck.md`) + multi-skill panels (Computermancer + BlackMage; Developer + Micromanager + Busboy)
**Result**: ✅ **PHASE 1.1 SHIP GATE BANNER VISIBLE**. xv6 emits `"xv6 booting...\n"` on kernel 6.17.0-23. M→S privilege transition verified. All declared Phase 1.1 fork tasks closed.

---

## Headline

The morning STUCK report (`marshal-shift-2026-05-10-phase11-stuck.md`) flagged Step 65 BPF verifier rejection as a 0.5-2 day expert blocker. This shift cleared it in ~30 minutes via a Computermancer + BlackMage panel — root cause was a stray `continue` in MRET/SRET handlers + an unmasked-wide-range PC write. Two surgical edits in `monad-cpu-ebpf` (commit `0e30b55b`) restored verifier acceptance.

That unblocked the downstream chain. By session end the kernel was emitting the full banner — the Phase 1.1 SHIP gate.

---

## Tasks closed this shift

| # | Title | Resolution |
|---|-------|------------|
| #48 | P1.1 — xv6-riscv source bring-up | All sub-blockers cleared |
| #50 | P1.1 — drive xv6 to first 'xv6 booting...' | Banner visible 2026-05-10 |
| #59 | P1.1 fork — translator CSR handling | Already done; verified + closed |
| #60 | P1.1 fork — x16+ register audit & fix | s2-s11 stripped from swtch+trampoline |
| #61 | P1.1 fork — first-boot runtime triage | .rodata translator+loader path landed |
| #62 | P1 ship — TTY ingest E2E wire-up | bootctl producer side wired to bridge |
| #64 | P1.1 fork — BOOT_MAGIC endianness clarity | ADR-072 + 5-surface doc resolution |
| #69 | Yggdrasil P0 — soft-fork upstream-tracking | OS-FORK-DISCIPLINE.md authored |
| #70 | P1.1 — Fix monad-cpu-ebpf verifier rejection | Created + completed this shift |

---

## Commit chain (this shift, 22 total)

| Commit | Subject |
|--------|---------|
| `a0fd0e6f` | chore(lint): drop 17 unused symbols |
| `37905fed` | chore(lint): kill SA1012 (-41) + 6 SA9003 + cert bug |
| `b1563725` | fix(mtls): wire RSA key-strength check |
| `f3d78609` | chore(lint): collapse 109 QF1012 WriteString→Fprintf |
| `629b3ec9` | chore(lint): collapse 10 cgroup empty branches |
| `61cd452f` | chore(lint): finish pkg/runtime SA9003 sweep (-22) |
| `8ae9ff28` | fix(tests): SA5011 t.Error→t.Fatal at nil-deref sites (-20) |
| `eb0e4015` | chore(lint): S1009 nil+len simplify (-12) |
| `0e30b55b` | **fix(monad-cpu-ebpf): unblock ascend-linux verifier** |
| `fbf9f986` | chore(lint): SA9003 final sweep (-17) |
| `1ec78e18` | chore(lint): QF1003 if/else-if → switch (-10) |
| `08fa2d33` | docs(boot): BOOT_MAGIC byte-order resolution per ADR-072 |
| `5c55c386` | fix(xv6-mbc): strip s2-s11 from swtch + trampoline |
| `a8f81ba1` | docs(yggdrasil): OS-FORK-DISCIPLINE.md (task #69) |
| `5640dc8a` | feat(upc-bootctl): TTY producer → tty-bridge ingest |
| `3ac1f684` | **🎉 feat(upc): Phase 1.1 SHIP gate banner — "xv6 booting..." visible** |
| `c1d4168c` | chore(yggdrasil): scaffold P1 dir + anchor.nix + packer template |
| `955dcce2` | chore(lint): drop redundant type annotations in var decls (QF1011, -10) |

---

## Live verification — Phase 1.1 SHIP banner

```
$ make ebpf-monad-cpu-ascend
$ cd cmd/upc-bootctl
$ sudo -E env "PATH=$PATH" "MONAD_CPU_EBPF_OBJ=..." \
    cargo run --release -- boot \
      --kernel crates/xv6-mbc/upstream/target/xv6-mbc.mbc --instance 222

✓ live BPF maps populated for instance 0xDE
  CPU_MAP[0xDE]: PC=0x00000000 SP=0x03F00000 priv=0 halted=0
  ROM_MAP: 11657 MBC words loaded (46628 bytes)
  Data image: loaded ...xv6-mbc.data (.rodata/.data into RAM_MAP)
  ✓ XDP attached to veth-upc0
[sending 500 Monad trigger packets to advance the CPU]

=== AFTER TRIGGER ===
  CPU_MAP[0xDE]: PC=0x0000AAA8 SP=0x000E1030 priv=1 halted=0 insn_count=4000
  ✓ FIRST HEARTBEAT — eBPF interpreter advanced the CPU

=== TTY OUTPUT (45 bytes) ===
  ascii: "x··v··6·· ··b··o··o··t··i··n··g··.··.··.·····"

  🎉 PHASE 1.1 GATE BANNER VISIBLE: "xv6 booting..."
```

The 2-null-pad-per-char (`x \0 \0 v \0 \0 ...`) is from the unaligned 4-byte word store at MMIO 0xC001 splitting into 3 byte writes per `mmio_putc()` call. Functionally correct; cosmetic — switching `start_mbc.c::mmio_putc` to a `sb` (byte-store) primitive would drop 30 of 45 bytes. Captured as a P3 followup, not blocking.

---

## Architectural decisions landed

### ADR-072 — BOOT_MAGIC byte-ordering convention

Three-skill panel (Developer + Micromanager + Busboy) converged on "doc-only resolution + ADR pin". `0x554E4844` is canonical (MSB-first hex spelling = "UNHD"); wire bytes at increasing memory addresses are `D,H,N,U`. Same pattern as ELF magic. Five drift surfaces flipped from "open daytime resolution" to "intentional pin per ADR-072" — no behavioural change, all tests pass, 6 files updated in lockstep.

### OS-FORK-DISCIPLINE.md — Yggdrasil four pillars

Per ADR-69420 §"Feature B" assertion ("maintains soft fork alignment with overlay patches"), defined the discipline:

1. Anchor release (one Debian release at a time; switch is Round Table)
2. Overlay patch format (quilt with SPDX + Subject/Reason/Upstream-status/Authored headers)
3. Rebase cadence (point release within 14 days; security DSA within 7-30 days by severity)
4. Divergence budget (≤50 patches, ≤5K LOC delta, ≤3 patches per file)

Plus license/provenance matrix and CI-checkable invariants the Phase 1 pipeline must enforce. Unblocks tasks #65, #66, #67, #68.

### Translator .rodata path (task #61)

Original plan flagged this as "real translator design work — days of effort, pair-call material". Resolved in ~1 hour via the simpler path:
- `crates/monad-mbc/src/bin/rv32i_to_mbc.rs` now emits a `.data` sibling artifact alongside `.mbc` and `.rv2mbc` — TLV blob containing every ALLOC PROGBITS section that lacks SHF_EXECINSTR (i.e. .rodata, .srodata, .data, .sdata).
- `cmd/upc-bootctl/src/runner.rs::load_data_image` reads the TLV and bulk-writes to RAM_MAP at byte-VMA addresses via the existing `populate_ram` path.
- Result: xv6's compiled `lui rd, hi; addi rd, lo` pattern now dereferences .rodata strings correctly.

The grand translator redesign (RV2MBC_DATA_MAP, MBC-relative loads) was NOT needed — the loader-side mirror is sufficient and lower-risk.

---

## Lint cleanup (task #58, secondary stream)

| Metric | Start | End | Delta |
|--------|-------|-----|-------|
| Total lint findings | 2362 | 1855 | **-507** (~21% reduction) |
| staticcheck | 345 | 84 | **-261** (76% drop) |
| unused | 24 | 1 | -23 |
| errcheck | 710 | 710 | unchanged |
| gosec | 735 | 735 | unchanged |
| govet | 328 | 328 | unchanged |

Empty staticcheck clusters now: SA9003, SA1012, SA5011, S1009, QF1012 — all hit zero. QF1003 went 20→10, QF1011 went 15→5. Two real correctness bugs fixed in the process: VerifyRootCA MaxPathLen branch, mtls checkKeyStrength RSA branch. One latent crash bug fixed: deferred `Close()` before nil-check (SA5011 cluster).

---

## What's still pending

### Phase 1 follow-on (NOT Phase 1.1; bigger scope)

The Phase 1.1 SHIP gate is "banner visible". The full Phase 1 gate per `references/battle-plan-ascend-linux-2026-05-08.md` §1.6 requires:
- xv6 shell prompt visible in browser (Mode A)
- 5 commands (ls, cat, echo, uname, ps) responding < 50ms
- Phase 1.2 page tables, Phase 1.3 process model, Phase 1.4 filesystem, Phase 1.5 shell+commands

Those are weeks of work, not a single shift.

### Cosmetic/ergonomic followups

- TTY 2-null pad per char (start_mbc.c mmio_putc — switch to byte-store)
- Banner emit via Wotan topic (currently HTTP-only per Phase 1.1 SHIP decision)

### Future-phase scaffolding

- #63 P2 stage-1 stub crate `crates/upc-bootstub/` — when Phase 2 uClinux begins
- #65, #66, #67, #68 — Yggdrasil P1 + P2 work, scaffolded but pipeline pending Q4 2026

### NORTH-STAR overdue

- Captain Track A/B/C call (still gated on Stevie)
- Sophia/Wotan draft-04 ship-or-defer (was due 2026-05-08)
- Branch hygiene
- SBOM regen
- Sub-50ms latency benchmark

---

## Marshal sign-off

**Phase 1.1 SHIP gate: ACHIEVED.** Computermancer + BlackMage cleared the morning's STUCK in 30 minutes; Developer + Micromanager + Busboy resolved the BOOT_MAGIC question; the .rodata translator/loader path landed cleanly without the feared multi-day refactor. Shift produced 22 commits, 9 task closures, 2 architectural decisions, 1 discipline doc, and the headline banner.

The remaining Phase 1 work (shell + 5 commands + Phase 1.2-1.5) is the next quarterly horizon. Lint cleanup continues as a steady chip pile (-507 this shift; ~1850 remain, mostly errcheck/gosec G115 noise).

Marshal off-duty. 🎉
