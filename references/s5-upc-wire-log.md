<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S5 UPC wire compute — execution log

Plan: `docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`. This session: Phases 0–4
(branch `sprint/upc-wire-compute`, worktree off `develop` @ `f7d730e7`). Sprint scratch
(`$S5`) lives outside the repo; evidence quoted here.

## Phase 0 — preflight + baseline (2026-10-23)

base: `f7d730e7` (develop tip)

| Hypothesis | Result |
|---|---|
| H0 | TRUE by content. `443de3f82` itself is not an ancestor: history was rewritten (re-sign); its equivalent `0a605108` ("runbook_list description no longer hard-codes 7 categories") is in `f7d730e7`'s history. Tree clean, branch `sprint/upc-wire-compute`. |
| H1 | TRUE. `scripts/upc-regression.sh` → `== ALL GREEN ==` (16 PASS + Doom load). |
| H2 | TRUE. 8/8 corpus `SAME` + 4/4 special (`uinit_echo`, `multi`, `nosuch`, `blank`) `SAME` against `~/tmp/golden`. No recapture. |

**Budgets (sprint baseline):** ascend verifier **926,542 insns (92.7%)**, Doom **761,580 (76.2%)**.
Both moved from the plan's expected 900,031 / 737,087 (July). No eBPF source change touches
`monad-cpu-ebpf` logic since `8de08368` (Stage 1, flag-off, measured 900,031 in July) except
`233b6451` (clippy; annotated the `read_monad_xdp` loop, not rewritten). Host kernel is now
6.17.0-41 and the nightly/bpf-linker toolchain has moved; the drift is attributed to
toolchain/kernel verifier changes, not a source regression. Recorded as the sprint baseline;
the getpid-flip budget gate (≤ 950,000) is measured against it.

**Translator / image baselines:** kernel 7552 RV32I → 11769 MBC, `xv6-mbc.mbc` sha256
`80ce532e…cb15`. Committed `doom/doom.mbc` (`a8f65173…`) is NOT what the current translator
emits from `doom/doom.elf` (`dcded746…`; `doom.rv2mbc` matches, `e7ab7b67…`): pre-existing
drift, so H-S5 compares translator output before/after the fix, not against the committed file.

**CSR instruction counts (Step 21):** kernel.elf 51; every userland ELF 0 (cat, echo, init, ls,
sh, uinit, wc); doom.elf 0.

**Known failures baseline (Step 12):** KF1 `gate_nway` → `NWAY-FAIL` (as listed); `go build ./...`
clean; `heimdall-bpf` builds `heimdall-vfs-write` (1 warning). Matches the KF table.

**Dormant S1.0/S1.1 present (Step 16):** `GETPID_INJECT: bool = false` (main.rs:413);
uservec `csrw mscratch,a0` + `csrr a0,sscratch`; userret `mv a0, a1`; `prepare_return` calls
`w_sscratch`; `user/getpidprobe.c` exists.

**prepare_return (Step 18):** `lw a5,48(a0)` → `csrr a2,satp` → `sw a2,0(a5)` … `csrw sscratch,a5`,
no other write to a5 in between. Translator CSR arm (Step 20): `csr_ptr = 12u8`, `tmp = 13u8`,
`mask = 14u8`, `14 => Ok(12)`, `15 => Ok(13)` all present.

**Other:** next free ADR number is **ADR-106** (ADR-105 is taken; `ADR_CART=ADR-106` for
Phase 9). Lich plan present: `docs/battle-plans/S-LICH-AWAKENS-battle-plan.md`. West
`enp16s0` = `192.168.13.2/30` + `fe80::da43:aeff:fe5f:a1c1/64`. East not contacted (this
session's scope is Phases 0–4; east legs belong to Phases 6–7). Mímir drift (Step 23):
ADR-043 + `wiki/Mimirs-Law.md` say all 13 phases shipped, CLAUDE.md says 9/11/13 pending —
left for Phase 8.

## Phase 1 — S1.2 root cause (2026-10-23)

**H-S1 CONFIRMED** (host + live). See `references/evolution1-stage1-2026-07-19.md`
§"S1.2 ROOT CAUSE (S5)". Live: `Z:SS satp=000007ff ss=000007ff tf=007be000`.
**H-S2 CONFIRMED**: `csrw sstatus,a4` stores 0xF400; any `csrr` overwrites a4 with the
CSR address. Red tests in `crates/monad-mbc/tests/csr_alias.rs` (5, all `#[ignore]`d).

| ID | Result |
|---|---|
| H-S3 | TRUE. Only two forms exist in any built image: `csrr` (CSRRS rd, x0) ×33 and `csrw` (CSRRW x0, rs) ×18, all in kernel.elf. No CSRRS/CSRRC with rs1≠x0, no immediate forms, no CSRRW with rd≠x0. Userland and Doom: zero CSR instructions. |
| H-S4 | **FALSE**. `start_mbc` reads `mhartid` (0xF14) → byte 0x12C50 > 0xFFFF. Keep the full-range mapping; ADR-067 amended instead of tightened. Indices used: sstatus, stvec, scause, sepc, stval, satp, sscratch, stimecmp, mscratch, mhartid. |
| H-S5 | TRUE. doom.elf has 0 CSR instructions. |

Kernel `.mbc` byte-identical to Phase 0 after the probe revert.
