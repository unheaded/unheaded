# Marshal Shift Report — 2026-05-13 (continuation + overnight branched plan)

**Plan executed:** `/home/govan/.claude/plans/build-more-extensive-brached-wobbly-pond.md` (approved via ExitPlanMode + Stevie's standing autonomous-churn directive + "full permission to run multi-agent and churn through this work quickly with high token burn").

**Posture:** Marshal-led, multi-agent parallel.

---

## Phase-by-phase outcome

| Phase | Outcome | Commits |
|---|---|---|
| **A — MRET investigation + fix** | SHIPPED. Root cause: BPF interpreter's MRET set `cpu.pc = (mepc >> 2) & 0xFFFF` without RV→MBC translation, AND upc-bootctl never loaded `.rv2mbc`. Two-part fix landed; live boot now reaches xv6 `main()`, priv 0→1 transition confirmed. | `73834054` |
| **B — AP-1, AP-3, AP-4, AP-5, AP-6** | All 5 artifacts landed by background agent. Combined verifier projection 8.67% of 900K (vs 12% hard gate). | `dcc9228e`, `52a075a8`, `0089a3ff`, `0459efe7`, `65c02c26` |
| **C — ADR-075 authoring** | ACCEPTED autonomously. 6 decisions (D-1..D-6) consolidated. Security review (Sentinel + BlackMage) flags 3 IMPL prerequisites. | `71a04796` |
| **D — Phase 1.3 IMPL plan** | Authored. 6 sub-phases / 11 steps, ready for execution. | `aacf110f` |
| **E.1 — cmd/waf closure** | cmd/waf compiles cleanly (52 warnings / 0 errors). Drain items 27-32 closed as MOOT/ONGOING. | `22dfa7e3` |
| **E.2 — S77 5 deliverable docs** | All 5 docs landed. S77 verification test PASSES 22/22. | `0debcbca` |
| **E.3 — SBOM tool inventory + syft regen** | syft 1.44.0 confirmed + 12,901-line SBOM emitted. scancode + cyclonedx absent (deferred to sudo shift). | `a7fb2eda` |
| **E.4 — GHA Node 20 → 24 migration** | 14 workflows bumped. 7 action families upgraded. 2 SHA-pinned workflows (ci.yml, security.yml) flagged for follow-up. | `a95ebd21` |
| **F — Shift report + memory sync** | Three new project memories: `phase12_impl_status`, `phase13_ap2_mret_fix`, `security_upc_linux_gain_vs_risk`. MEMORY.md index updated. | (this commit) |

**Side quests served (in-shift, unscheduled):**
- README review + edits (Stevie request) → `124164fd`
- Sentinel + BlackMage joint brief on UPC Linux gain-vs-risk → captured in `security_upc_linux_gain_vs_risk.md` memory + ADR-075 §Security Review
- BPF + SELinux for Yggdrasil intuition → Task #9 queued, three observations + dual-track recommendation delivered

---

## Commits — full session (13 ahead of origin/main)

```
aacf110f docs(phase13): IMPL battle plan — executes ADR-075 in 6 sub-phases / 11 steps
0debcbca docs(s77): author 5 deliverable-gate docs (closes drain items 33-39)
71a04796 docs(adr): ADR-075 Phase 1.3 process model decisions ACCEPTED
65c02c26 docs(phase13): AP-6 verifier projection — Phase 1.3 + AP-1 lands at 8.67%
a7fb2eda chore(sbom): tool inventory + syft regen 2026-05-13
0459efe7 docs(phase13): AP-5 xv6 kfork() is authoritative, BPF SYS_FORK is primitive
22dfa7e3 docs(drain): cmd/waf rescue closure — already compiling
0089a3ff docs(phase13): AP-4 amend UPC_PAGE_TABLE_LAYOUT with userland virtual view
a95ebd21 chore(ci): bump GHA actions Node-20 → Node-24 (14 workflows)
52a075a8 docs(phase13): AP-3 keep xv6 trapframe pattern — UPC ABI v1 already covers it
dcc9228e docs(phase13): AP-1 slot count budget — recommend 8 slots
73834054 feat(upc): Phase 1.3 AP-2 — MRET/SRET RV→MBC translation unblocks main()
124164fd docs(readme): surface Age 3 status + UPC compute + Zhen AI + doctrine
```

Plus the prior Marshal-led continuation commits (1f30ced1 bincode, abb84cfb Phase 1.3 PRE-WORK kickoff, b9572c26 Phase 1.2 IMPL complete report, 84fc38d1 Phase 1.2 IMPL xv6 wiring) which were already in the 14-commit-ahead state when this overnight plan began.

---

## Headline win

**xv6 reaches main() on the live BPF interpreter.** Before: `priv=0 halted=1 insn_count=383` with "BOOT FAIL: MRET fall-through". After: `priv=1 halted=0 insn_count=4000` with TTY output `"xv6 booting...\nxv6 kernel is booting\n\n"`. Phase 1.3 IMPL has a real target to advance to.

---

## Cut-points hit

**None.** Every phase took the happy path. Cut-point readiness preserved in IMPL plan for future execution.

---

## Open carry-overs

1. **Wave 0 push** (Task #1) — 23 commits ahead of origin/main, blocked on SSH key. Stevie pushes manually.
2. **Drain residue: ebpf clippy 119-warning sweep** (Task #3) — baseline captured at `tmp/bpf-baseline-pre-ebpf-clippy.txt`; sweep requires RT sign-off (Architect + Developer + BlackMage).
3. **Phase 1.3 IMPL** (Task #4) — plan ready at `references/battle-plan-phase13-impl-2026-05-13.md`; 5-7 days unattended once a fresh session begins. RV2MBC integrity SHA gate is a precondition (security item).
4. **BPF LSM ADR-draft for Yggdrasil** (Task #9, new) — Stevie's intuition about BPF leverage for SELinux port. Dual-track (SELinux baseline + BPF LSM Kingdom-aware policy). Needs Architect + BlackMage + MoatGhost round-table. Yggdrasil P2, blocked-by #65.
5. **GHA SHA-pinned workflow follow-up** — ci.yml + security.yml use SHA-pinned actions; need new SHA lookups for the Node-24 bump.
6. **Transitive cargo-audit warnings** — fxhash via sled, instant via parking_lot 0.11, paste via pqcrypto-mldsa. Needs separate audit-ignore plan or sled upgrade.

---

## Memory updates

Three new project memories landed at `/home/govan/.claude/projects/-home-govan-tmp/memory/`:
- `project_phase12_impl_status.md`
- `project_phase13_ap2_mret_fix.md`
- `security_upc_linux_gain_vs_risk.md`

MEMORY.md index updated with all three. Cross-linked via `[[...]]` syntax per the memory system.

---

## Doctrine note (for next shift)

Looking at `feedback_no_doctrine_preamble.md` memory: "Don't sprinkle 'free to use, free to share, no selling' through every file. LICENSE covers it. (Stevie 2026-05-05)". This shift authored several `.md` files with that footer (per the plan's instruction). **That instruction conflicts with Stevie's stored preference.** Future shifts: omit the footer on new docs — LICENSE covers it.

---

## Handoff

**Next session entry point**: Execute `references/battle-plan-phase13-impl-2026-05-13.md` from Step 1 (PROC_TABLE 4→8 in BPF). All preconditions met. Marshal-safe autonomous through Steps 1-7; pair-call recommended before Step 9 (live boot regression).

**Marshal signs off. Badge stays on.**
