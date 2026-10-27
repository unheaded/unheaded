<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-DOOM-LAST BATTLE PLAN: 9 Phases, 104 Steps

**Date**: 2026-10-27
**Sprint**: Sprint 7 of 7 (2026-10-27 ladder), DOOM LAST. The Doom docs tell the truth, the three Doom units stop running as root, the committed `doom.mbc` gets provenance, and the banding root cause gets one disciplined, time-boxed differential pass.
**Prerequisite**: Every other 2026-10-27 plan's exit gates passed (S-CLEAN-SLATE, S-IRON-GATE, S-WELL-SPRING, S-CHAMPION-HARDENING, S-TRUE-WIRE, S-ASCEND-2), or Stevie names an idle window (`DOOM_WINDOW=1` in the executing shell). Branch based on `a7fe3acd` (develop) or newer; `go build ./...` clean; `riscv64-unknown-elf-gcc` toolchain present on west; `~/tmp/projects/doom-related/DOOM.WAD` present (never in the repo).
**Target**: `docs/doom/STATUS.md` names banding as a diagnosed MBC-executor bug; `unheaded-doom-{ring,runner,injector}.service` carry `User=unheaded-doom` with pinned uid 16790 and the `User=` gate enforces for every unit; `doom/doom.mbc` is byte-identical to what the committed translator emits from `doom/doom.elf` and the regression harness proves it on every run; H1 (CPU_MAP/.bss readback) is closed with evidence either way; the two executor hypotheses (misaligned LD, word-memcpy) are each confirmed or eliminated by a reference-vs-eBPF differential on an identical probe, within a 3 h box.
**Estimated Duration**: 9-13 h across 2-3 sessions. One attended look from Stevie (E3) for anything visual; one west window (E1) to install the units.
**Agent Strategy**: Coordinator only. Doom work is sequential (every phase touches the same ring, maps and ROM) and historically drifted into NOP loops, so no parallel agents and every phase carries a hard time box (Marshal oversight, memory feedback_marshal_oversight). Hosts are read-only except Phase 2's install under E1 and the lab ring on west, which S5 and every Doom session before it already used.
**Commit Cadence**: every 5 steps (max(3, min(5, 104/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if `git commit-tree -S` succeeds in Step 12, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk).
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B). Phases 4 and 5 additionally have a phase-level box (60 min and 180 min): when the box expires the phase is closed INCOMPLETE with its findings written to `docs/doom/FINDINGS.md`, whatever the state of the hypothesis.

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| `docs/sweeps/OUTSTANDING-2026-10-27.md` E15 (M-240..M-246), decision 50 | The scope. Doom is tier (f), lowest priority of any work (Stevie 2026-10-27). |
| `docs/doom/HANDOFF.md:100-330` | PU_CACHE disproven; root cause `texturecolumnlump[]` garbage from Z_Malloc corruption on MBC; THE PARADOX table; hypotheses H1 (CPU_MAP/.bss readback), H2 (misaligned LD), H3 (word memcpy), H4 (fd drift); "bump-only zone allocator" as the next approach. |
| `docs/doom/FINDINGS.md:46-90` | PC corruption is RESOLVED: Doom ran with ADR-079 live on 2026-07-03, PC valid into the billions. Nobody re-diagnoses it. |
| `docs/doom/RUNBOOK.md:426-450` | WAD size limit RESOLVED (`WAD_MAX_SIZE` 16 MiB, size published at `WAD_SIZE_ADDR`); IWAD flavour detected from the lump directory (retail and shareware both run). Nobody re-diagnoses either. |
| `references/s5-upc-wire-log.md:26-29, 85-87` | Committed `doom/doom.mbc` (`a8f65173…`) is NOT the current translator's output from `doom/doom.elf` (`dcded746…`); `doom.rv2mbc` matches (`e7ab7b67…`). Doom verifier baseline 761,580 insns. |
| `compliance/findings/register.yaml` FND-010 | The Doom half: three units start as root with binaries under `/home/govan`; "installed on no host". The Well-jobs half is NOT this plan's. |
| `scripts/check-systemd-units.sh:37-66` | Today the gate only checks units that already have `User=`; a unit with no `User=` passes silently. That is the "verify-only" this plan flips. |
| `pkg/uids/uids.go`, `pkg/uids/uids_test.go:265-290`, `deploy/sysusers.d/unheaded.conf` | The uid registry and the test that pins a host-only service's sysusers line to its registry uid. 16790 is free (16789 and 16799 are the neighbours). |
| `tomb/lich/LICH-015-mbc-differential/README.md` | Arm A = `crates/monad-mbc` reference (`ExecCpu`), arm B = `monad_common::cart::step` (cartridge allowlist only: no RAM LD/ST). So LD/ST hypotheses need the reference interpreter vs the live `monad-cpu-ebpf`, not the in-process cartridge arm. |
| `demos/doom/Makefile:37, 90-92`, `demos/doom/libc_stubs.c:148-168` | `rv32i-to-mbc <elf> -o <mbc> --stats` is the translation; memcpy's word fast path (`*dw++ = *sw++`) is H3's exact code. |
| `scripts/upc-regression.sh:109-134` | Variant 2 loads `doom/doom.mbc` through doom-runner and passes on "pipeline complete"; insn count is logged under `UPC_VERIFIER_STATS=1`. |
| `scripts/ci-fetch-doomgeneric.sh` | The id Doom core lives in the sibling checkout `../projects/doomgeneric/unheaded` (pinned SHA), not in this repo; `doom/doomgeneric` is a gitlink. |
| Memory feedback_doom_screen_size | A smaller Doom screen is an authentic 486 experience, not a workaround. Never conflate with banding. |
| Memory feedback_marshal_oversight | Marshal oversees all Doom work: time boxes, no NOP loops, no hallucinated fixes. |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E1]     Runs only once E1 is answered; skipped (not STUCK) until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, plus `-race` on touched packages).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `scripts/check-*` green).
6. Attack surface unchanged, or the change is listed in Phase 7's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## SITUATION (verified at a7fe3acd, 2026-10-27)

- **Priority.** Stevie, 2026-10-27: "DOOM related stuff is lowest priority of any work." This plan runs after the other six or in a named idle window. Decision 50: Doom stays in the repo (the S35 "move out before going public" line in `CLAUDE.md:900` is superseded; the repo is already public).
- **Resolved, do not re-diagnose.** (1) WAD size: `RUNBOOK.md:437-440`, `WAD_MAX_SIZE` 16 MiB, both retail 12,408,292 B and shareware 4,196,020 B load. (2) IWAD identity: flavour detected from the lump directory, `RUNBOOK.md:426-428`. (3) PC corruption: `FINDINGS.md:46-52`, Doom ran 2026-07-03 with ADR-079 live, `regs[14]=0x800130fa`, PC valid into the billions. `CLAUDE.md:874` still says "Current blocker: PC corruption" and is wrong.
- **Banding is diagnosed, not fixed, and the docs lie.** `docs/doom/STATUS.md:61` and `docs/doom/README.md:105` call banding "authentic nearest-neighbor magnification / NOT A BUG". `HANDOFF.md:168-200` proves `texturecolumnlump[]` holds garbage lump numbers (BROWN96 col[0]=24 SSECTORS, col[1]=0 PLAYPAL, col[6]=18961 OOB) because Z_Malloc'd blocks are corrupted on MBC (`w_file_stdc.c:6` already documents the same class for WAD handles); the malloc-bypass workaround removed most banding but crashes in rooms 2-3 (THE PARADOX, `HANDOFF.md:300-318`); workarounds were reverted (`f21ec120`). PU_CACHE is disproven (`HANDOFF.md:108-120`). Open hypotheses: H1 CPU_MAP/.bss readback (`HANDOFF.md:124-136`), H2 misaligned LD (`ebpf/monad-cpu-ebpf/src/main.rs:983` loads `mem_read_word(word_addr)`, the `addr & 3` byte offset is only used by the byte paths at 1234/3772/3792), H3 word memcpy (`demos/doom/libc_stubs.c:155-160`), H4 fd drift.
- **E1M2.** `RUNBOOK.md:434-435`: "some textures render incorrectly on E1M2", open since 2026-03-30, never tied to the banding class.
- **Units run as root from /home.** `deploy/systemd/unheaded-doom-{ring,runner,injector}.service` have no `User=`; `WorkingDirectory=/home/govan/tmp/unheaded`; `ExecStart` paths under `/home/govan/...`; the runner points at `demos/doom/doom.mbc` (a `.gitignore`d build output) while the harness loads the committed `doom/doom.mbc`; the injector writes `/run/doom-injector.env`. `pkg/uids/uids.go` has no Doom entry; `deploy/sysusers.d/unheaded.conf` has no `unheaded-doom`. FND-010 (medium, 2026-10-18) records this half plus the Well jobs half (not in scope). `scripts/check-systemd-units.sh:47-49` skips units without `User=`.
- **Artifact provenance.** Committed `doom/doom.mbc` sha256 `a8f651736ffa8ece471a3a008e47de8569b616e03f33d66f79647299fc73e926` (2026-04-02); `doom/doom.elf` `3a04e4ddea4bbba42d871d8e711837e6c3f30e65381e76f21cabf7c3b0b3319a`. S5 (2026-10-23) measured the current `rv32i-to-mbc` emitting `dcded746…` from that ELF: pre-existing drift, so the harness today loads a ROM no committed translator produces. `demos/doom/doom.mbc` (`250ba58a…`) and `demos/doom/doom.elf` (`2bbf1cc9…`) are a third, untracked pair. `crates/monad-mbc/target/release/rv32i-to-mbc` is not built on west right now.
- **Harness.** `scripts/upc-regression.sh` Variant 2 loads `doom/doom.mbc` via doom-runner (`--hops 2`, WAD from `$DOOM_WAD` or `~/tmp/projects/doom-related/DOOM.WAD`) and passes on "pipeline complete"; `UPC_VERIFIER_STATS=1` gives the Doom insn count (S5 baseline 761,580). `scripts/doom-build-check.sh` enforces `doom.mbc < 262,144 words`, `doom.rv2mbc < 65,536 entries`, Bug 21-23 regressions. `scripts/doom-smoke.sh` is the full lifecycle smoke (needs sudo, asserts screen via `doom-screen-check.py`).
- **Differential tooling.** LICH-015's `diff_harness` compares the `crates/monad-mbc` reference with the in-process cartridge arm (ALU/branch/MOV allowlist). RAM `LD`/`ST`/`LBU`/`STB` are outside that arm, so H2/H3 need: the same probe ROM run on (A) the reference interpreter on the host and (B) the live `monad-cpu-ebpf` on west's ring, comparing a results region readable from both (`SCREEN_BASE` 0x70000 readback works, `HANDOFF.md:136-140`; `.bss` readback does not, hence H1).
- **Doom C core is out of repo.** `doom/doomgeneric` is a gitlink; the source is `../projects/doomgeneric/unheaded` (fetched at a pinned SHA by `scripts/ci-fetch-doomgeneric.sh`). `demos/doom/` holds only the MBC platform glue (`i_*_mbc.c`, `libc_stubs.c`, `w_file_stdc.c`, `crt0.S`, `linker.ld`). Any `z_zone.c`/`r_data.c` experiment edits the sibling checkout on a branch and is never pushed.
- **Ports/paths.** doom-bridge 16666, doom-injector 16667 (`CLAUDE.md:129`). Map pins `/sys/fs/bpf/unheaded/doom-ring/maps/{CPU_MAP,RAM_MAP,...}`. Ring namespaces `monad0`/`monad1`, veth `veth01`/`veth01p`, flow label 222 (0xDE).

**Escalations**

| ID | One line | Blocks |
|---|---|---|
| E1 | West window to install sysusers + the three Doom units and start `unheaded-doom.target` as `unheaded-doom` (host change). | Steps 48-51 |
| E2 | The Doom C core checkout `../projects/doomgeneric/unheaded` is absent or not writable, and fetching/branching it is Stevie's call. | Steps 91-95 |
| E3 | One attended look in a browser (no headless browser on west): banding after any executor change, E1M2 textures, fps. | Steps 96, 99, 101 |

---

## PHASE 0: PREFLIGHT (Steps 1-14)

**Goal**: Prove the gate that lets Doom run at all, re-check every SITUATION claim live, and record the byte-identity baseline nothing after Phase 3 may change (M-240..M-246 all depend on it).
**Prerequisite**: none.
**Time**: 45 min (Step 10 dominates).
**Agent**: Coordinator.

### The LAST gate

- [ ] **Step 1** [PREFLIGHT][B] ~1m: Doom runs only after the six other plans shipped, or in a named window
  ```bash
  cd /home/govan/tmp/unheaded && n=0; for p in S-CLEAN-SLATE S-IRON-GATE S-WELL-SPRING S-CHAMPION-HARDENING S-TRUE-WIRE S-ASCEND-2; do f=docs/battle-plans/$p-battle-plan.md; if grep -qE '^\> \*\*SHIPPED' "$f" 2>/dev/null; then n=$((n+1)); echo "shipped: $p"; else echo "NOT shipped: $p"; fi; done; echo "shipped=$n/6 DOOM_WINDOW=${DOOM_WINDOW:-unset}"
  ```
- [ ] **Step 2** [V] ~1m: **DOOM-LAST GATE**: `shipped=6/6`, or `DOOM_WINDOW=1` was set by Stevie in this shell.
  - If neither → STOP. Do not start Doom work. Write "gate not met" in the progress file and return to the first unshipped plan.

### Environment

- [ ] **Step 3** [PREFLIGHT][ENV][B] ~1m: Tools present
  ```bash
  for t in riscv64-unknown-elf-gcc riscv64-unknown-elf-objdump riscv64-unknown-elf-nm cargo go bpftool ip python3 sha256sum; do command -v $t >/dev/null && echo "$t: OK" || echo "$t: MISSING"; done; ls -la ~/tmp/projects/doom-related/DOOM.WAD 2>/dev/null || echo "WAD MISSING"
  ```
- [ ] **Step 4** [V] ~1m: No MISSING line; WAD present. If the riscv toolchain is missing → [D] Step 5. If the WAD is missing → STOP (never fetch a WAD into the repo; Stevie supplies it).
- [ ] **Step 5** [D][S] ~3m: Install the toolchain the way DOOM-BUILD-GUIDE.md names it
  ```bash
  grep -nE 'apt|toolchain|riscv64' /home/govan/tmp/unheaded/docs/doom/DOOM-BUILD-GUIDE.md | head -8
  ```
  Follow the guide's apt line. If the guide has none → STOP, escalate (no new toolchain source without Stevie).
- [ ] **Step 6** [PREFLIGHT][BUILD][B] ~3m: Go compiles; known-failures baseline recorded
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... && echo BUILD_OK; go test ./pkg/uids/... ./pkg/ports/... -count=1 2>&1 | tail -4
  ```
- [ ] **Step 7** [V] ~1m: `BUILD_OK`; `pkg/uids` and `pkg/ports` tests `ok`. Record the two lines as KNOWN FAILURES BASELINE in `docs/battle-plans/progress/S-DOOM-LAST-progress.md` (create the file with a `## Baseline` heading).

### SITUATION re-checks

- [ ] **Step 8** [PREFLIGHT][B] ~1m: The three doc lies and the three root units are still as described
  ```bash
  cd /home/govan/tmp/unheaded && grep -n 'NOT A BUG' docs/doom/STATUS.md; grep -n 'authentic' docs/doom/README.md; grep -n 'Current blocker' CLAUDE.md; grep -n 'Move out of repo' CLAUDE.md; for u in deploy/systemd/unheaded-doom-*.service; do printf '%s User=%s\n' "$u" "$(sed -n 's/^User=//p' $u | tail -1)"; done; grep -c -i doom pkg/uids/uids.go deploy/sysusers.d/unheaded.conf
  ```
- [ ] **Step 9** [V] ~1m: STATUS.md has `NOT A BUG`; README.md has `authentic`; CLAUDE.md has both lines; every unit prints an empty `User=`; both grep counts are `0`. Any mismatch → the matching phase below is already partly done; read the diff (`git log -3 --oneline -- <file>`) and tick those steps as done-before-plan rather than redoing them.
- [ ] **Step 10** [PREFLIGHT][B][S][BARE-METAL] ~15m: Byte-identity and verifier baseline through the real harness (loads BPF on west's lab ring; the standing Doom practice since S31, cleans up after itself)
  ```bash
  cd /home/govan/tmp/unheaded && sha256sum doom/doom.elf doom/doom.mbc doom/doom.rv2mbc | tee /tmp/doom-last-baseline.sha && sudo UPC_VERIFIER_STATS=1 scripts/upc-regression.sh 2>&1 | tee /tmp/doom-last-regression.log | grep -E 'Doom|ascend|PASS|FAIL' | head -12
  ```
- [ ] **Step 11** [V] ~1m: `doom/doom.mbc` = `a8f651736ffa8ece471a3a008e47de8569b616e03f33d66f79647299fc73e926`, `doom/doom.elf` = `3a04e4ddea4bbba42d871d8e711837e6c3f30e65381e76f21cabf7c3b0b3319a`; harness prints `Doom object loads` PASS and `Doom verifier: 761580` (or a number you now record as THE baseline if the toolchain drifted, as S5 did from 737,087). Copy the three shas and the insn count into the progress file under `## Byte-identity baseline`. If the harness fails to load Doom → [D] Step 11a.
- [ ] **Step 11a** [D][S] ~5m: Stale ring or runner holding the port
  ```bash
  pgrep -af "doom-runner run" | head -3; sudo ss -lntp | grep -E '16666|16667' || echo "ports free"; sudo /home/govan/tmp/unheaded/crates/doom-runner/target/release/doom-runner ring teardown --hops 2 2>&1 | tail -2
  ```
  Then rerun Step 10 once. Second failure → STOP: the harness itself regressed, which belongs to S-ASCEND-2, not here.

### Commit plumbing

- [ ] **Step 12** [ENV][B] ~1m: Signing probe and branch
  ```bash
  cd /home/govan/tmp/unheaded && git rev-parse --abbrev-ref HEAD && git status --short | head -5; tree=$(git rev-parse HEAD^{tree}); if timeout 20 git commit-tree -S -m probe "$tree" >/dev/null 2>&1; then echo SIGN=yes; else echo SIGN=no; fi
  ```
- [ ] **Step 13** [V] ~1m: On `develop`, tree clean (the untracked `cmd/host-agent/` and `host-agent` are pre-existing and left alone). Record `SIGN=yes|no`; every [C] below uses `--no-gpg-sign` when `no`.
- [ ] **Step 14** [V] ~1m: **PHASE 0 EXIT GATE**: Steps 2, 4, 7, 9, 11, 13 passed; progress file has Baseline and Byte-identity sections.
  - If pass → Phase 1. If fail → fix within Phase 0; nothing below is safe without the baseline.

---

## PHASE 1: THE DOCS TELL THE TRUTH (Steps 15-26)

**Goal**: M-240 and decision 50: STATUS.md, README.md, NEXT-STEPS.md, RUNBOOK.md and CLAUDE.md say what HANDOFF.md and FINDINGS.md proved; the screen-size doctrine is preserved verbatim.
**Prerequisite**: Phase 0.
**Time**: 40 min.
**Agent**: Coordinator.

- [ ] **Step 15** [R] ~3m: Read the exact lines you will change
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '58,64p' docs/doom/STATUS.md; sed -n '100,108p' docs/doom/README.md; sed -n '1,6p' docs/doom/NEXT-STEPS.md; sed -n '430,436p' docs/doom/RUNBOOK.md; sed -n '872,876p;898,901p' CLAUDE.md
  ```
- [ ] **Step 16** [W][DOC-UPDATE] ~5m: `docs/doom/STATUS.md:61` row becomes: `| Texture banding | Horizontal color bands on walls close up | MBC executor bug: Z_Malloc'd blocks corrupt on MBC, so texturecolumnlump[] holds garbage lump numbers (HANDOFF.md "ROOT CAUSE FOUND", THE PARADOX). Not nearest-neighbour magnification. | OPEN, diagnosed |`. Add directly under the table: `Screen size is a separate matter: a smaller Doom screen is the authentic 486 experience, not a workaround for banding.`
- [ ] **Step 17** [W][DOC-UPDATE] ~3m: `docs/doom/README.md:105` becomes `- Banding on some textures is an open MBC executor bug (Z_Malloc corruption → garbage texturecolumnlump[]; see STATUS.md, HANDOFF.md). Not authentic magnification.`
- [ ] **Step 18** [W][DOC-UPDATE] ~4m: `docs/doom/NEXT-STEPS.md`: change `**Last updated:** 2026-03-30` to `2026-10-27`, add a line `**Priority (Stevie 2026-10-27):** Doom is the lowest priority of any work; this list runs only after every other 2026-10-27 plan, per docs/battle-plans/S-DOOM-LAST-battle-plan.md.` Under P0 item 1 add `  - 2026-07: bridge.rs streams 64 KB frames at target_fps 60 over WS; visible fps never re-measured since (S-DOOM-LAST Step 101).` Replace item 4 (NixOS service module) with `4. ~~NixOS service module~~ shipped as systemd units deploy/systemd/unheaded-doom-*.service (own user from S-DOOM-LAST Phase 2).`
- [ ] **Step 19** [W][DOC-UPDATE] ~2m: `docs/doom/RUNBOOK.md:434-435` known issue 2 becomes `2. **E1M2 texture corruption** — some textures render incorrectly on E1M2 (open since 2026-03-30). Working assumption: the same Z_Malloc/texturecolumnlump class as E1M1 banding (HANDOFF.md); confirmed or refuted by S-DOOM-LAST Step 99.`
- [ ] **Step 20** [W][DOC-UPDATE] ~3m: `CLAUDE.md:874` becomes `**Current blocker:** texture banding (MBC executor: Z_Malloc corruption → garbage texturecolumnlump[], docs/doom/HANDOFF.md). PC corruption was resolved 2026-07-03 (ADR-079 live, FINDINGS.md). Priority: lowest of any work (Stevie 2026-10-27).` `CLAUDE.md:900` drop the sentence `Move out of repo before going public.` and append `Stays in the repo (decision 50, 2026-10-27).`
- [ ] **Step 21** [B] ~1m: Nothing else in the tree still carries the two false claims
  ```bash
  cd /home/govan/tmp/unheaded && grep -rn 'NOT A BUG' docs/doom wiki CLAUDE.md | grep -i band; grep -rn 'authentic Doom 320x200' docs wiki CLAUDE.md; grep -rn 'Move out of repo before' CLAUDE.md docs wiki; echo "grep done"
  ```
- [ ] **Step 22** [V] ~1m: Only `grep done` prints (no remaining hits). Any hit → fix it in the same style and rerun.
- [ ] **Step 23** [B] ~1m: Wiki mirror check
  ```bash
  cd /home/govan/tmp/unheaded && grep -rln -i 'banding' wiki/*.md | head; grep -n -i 'banding' wiki/Doom*.md wiki/UPC*.md 2>/dev/null | head -5
  ```
- [ ] **Step 24** [W][DOC-UPDATE] ~3m: For every wiki page Step 23 lists that says or implies "not a bug", change it to one sentence pointing at `docs/doom/STATUS.md` (wiki pages summarise, the docs/doom file is the source; `CLAUDE.md:861-872`).
- [ ] **Step 25** [C] ~2m: **COMMIT CHECKPOINT**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/doom/STATUS.md docs/doom/README.md docs/doom/NEXT-STEPS.md docs/doom/RUNBOOK.md CLAUDE.md wiki && git commit ${SIGN_FLAG} -m "[PLAN S-DOOM-LAST] Steps 15-24: Doom docs state the diagnosed banding bug, drop 'not a bug' and 'move out of repo'

Phase 1: The docs tell the truth
Steps completed: 15-24
Verification gates passed: no-false-claims grep (22)
Next: Step 26 (Phase 1 exit gate)

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3TuPhKAVVHnD9jzuabE"
  ```
  (`SIGN_FLAG` is empty when Step 13 said `SIGN=yes`, else `--no-gpg-sign`.)
- [ ] **Step 26** [V] ~1m: **PHASE 1 EXIT GATE**: Step 22 passed; `git status --short` empty; `grep -c 'OPEN, diagnosed' docs/doom/STATUS.md` = 1.
  - If pass → Phase 2. If fail → fix within Phase 1.

---

## PHASE 2: ONE OWNER FOR THE DOOM UNITS (Steps 27-52)

**Goal**: M-245 and the Doom half of FND-010: uid 16790 `unheaded-doom` in the registry and sysusers; the three units run as that user from `/var/lib/unheaded/doom` with the committed `doom/` artifacts and only the capabilities each needs; the `User=` gate enforces for every unit; installed live on west under E1.
**Prerequisite**: Phase 1 (doc truth first; Marshal order).
**Time**: 2.5 h (plus the E1 wait).
**Agent**: Coordinator.

### Registry and sysusers (red → green)

- [ ] **Step 27** [R] ~2m: The registry's host-only block and the pinning test
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '120,150p' pkg/uids/uids.go; sed -n '262,292p' pkg/uids/uids_test.go; grep -n 'unheaded-chaos\|unheaded-anomaly' deploy/sysusers.d/unheaded.conf
  ```
- [ ] **Step 28** [CODE][W] ~4m: Test first. In `pkg/uids/uids_test.go` extend the host-only pinning table (the one `TestHostOnlySysusersPinned` reads, Step 27 shows its shape) with `{"doom", "unheaded-doom", Doom}` using exactly the field names the existing rows use.
- [ ] **Step 29** [TEST][B] ~1m: Expect compile failure (undefined `Doom`)
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/uids/... -count=1 2>&1 | tail -3
  ```
- [ ] **Step 30** [V] ~1m: Output names `undefined: Doom` (RED). If it passes, the table shape was wrong; re-read Step 27 and fix the row.
- [ ] **Step 31** [CODE][W] ~2m: `pkg/uids/uids.go`: in the host-only const block add `Doom = 16790 // unheaded-doom: doom-ring, doom-runner, doom-injector (FND-010)` next to `Yaldabaoth`, keeping the block's ordering comment honest.
- [ ] **Step 32** [W] ~2m: `deploy/sysusers.d/unheaded.conf`: append `u     unheaded-doom          16790 "Unheaded Doom on the UPC (ring, runner, injector)" / /usr/sbin/nologin` (same column layout as the `unheaded-chaos` line).
- [ ] **Step 33** [TEST][B] ~1m: GREEN, with race
  ```bash
  cd /home/govan/tmp/unheaded && go test ./pkg/uids/... -race -count=1 2>&1 | tail -3 && grep -c 'unheaded-doom' deploy/sysusers.d/unheaded.conf
  ```
- [ ] **Step 34** [V] ~1m: `ok` and count `1`. If the parity test fails on another row, you edited the wrong table; revert Step 28 and redo.
- [ ] **Step 35** [C] ~2m: **COMMIT CHECKPOINT** `[PLAN S-DOOM-LAST] Steps 27-34: uid 16790 unheaded-doom pinned in pkg/uids and sysusers` (same trailer format as Step 25).

### The units

- [ ] **Step 36** [DECIDE][DESIGN] ~3m: **Where the Doom binaries and WAD live when not under /home.** RECOMMENDATION: binaries in `/usr/local/libexec/unheaded/doom/` (`doom-runner`, `doom-go-injector`, `doom-ring.sh`, `doom-xdp-attach.sh`), ROM/ELF/RV2MBC copied from the committed `doom/` set to `/var/lib/unheaded/doom/rom/`, the WAD copied to `/var/lib/unheaded/doom/wad/DOOM.WAD` mode 0640 owned `root:unheaded-doom`, state via `StateDirectory=unheaded/doom`, runtime env via `RuntimeDirectory=unheaded-doom` at `/run/unheaded-doom/injector.env`. Rationale: nologin users cannot read `/home/govan`; the harness and the units then load the same committed artifacts (today the units load the ignored `demos/doom` copy); `/var/lib/unheaded/<svc>` is the pattern every other unit uses. Override ONLY if Stevie wants the WAD somewhere else (it is his file, never in the repo).
- [ ] **Step 37** [DECIDE][DESIGN] ~3m: **Capabilities per unit, minimal first.** RECOMMENDATION: ring (oneshot, creates netns + veth, attaches nothing): `AmbientCapabilities=CAP_NET_ADMIN CAP_SYS_ADMIN`, `CapabilityBoundingSet=` the same; runner (loads BPF, pins under `/sys/fs/bpf/unheaded`, attaches XDP in the netns via the attach script): `CAP_BPF CAP_NET_ADMIN CAP_PERFMON CAP_SYS_ADMIN`; injector (`ip netns exec` + raw sockets): `CAP_SYS_ADMIN CAP_NET_RAW CAP_NET_ADMIN`. Plus `NoNewPrivileges=yes`, `ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`, `ReadWritePaths=/sys/fs/bpf/unheaded`. Rationale: `CAP_SYS_ADMIN` is unavoidable for `ip netns` (setns) and for the bpffs pin dir on 6.17 without a `bpf` LSM policy; every other cap is dropped. Override ONLY if the E1 live run shows an `EPERM` the [D] steps cannot attribute (then widen one cap at a time and record why in the unit comment).
- [ ] **Step 38** [W] ~8m: Rewrite `deploy/systemd/unheaded-doom-ring.service` `[Service]`: keep `Type=oneshot`, `RemainAfterExit=yes`; set `User=unheaded-doom`, `Group=unheaded-doom`, `WorkingDirectory=/var/lib/unheaded/doom`, `StateDirectory=unheaded/doom`, `ExecStartPre=-/usr/local/libexec/unheaded/doom/doom-ring.sh teardown`, `ExecStart=/usr/local/libexec/unheaded/doom/doom-ring.sh setup`, `ExecStop=/usr/local/libexec/unheaded/doom/doom-ring.sh teardown`, the Step 37 caps and hardening lines. Keep `Documentation=` but point it at `docs/doom/ARCHITECTURE.md` by repo path, not `file:///home/...`.
- [ ] **Step 39** [W] ~8m: Rewrite `deploy/systemd/unheaded-doom-runner.service` `[Service]` likewise: `ExecStart=/usr/local/libexec/unheaded/doom/doom-runner run --doom-mbc /var/lib/unheaded/doom/rom/doom.mbc --doom-elf /var/lib/unheaded/doom/rom/doom.elf --rv2mbc /var/lib/unheaded/doom/rom/doom.rv2mbc --wad /var/lib/unheaded/doom/wad/DOOM.WAD --hops 2`, `ExecStartPost=/usr/local/libexec/unheaded/doom/doom-xdp-attach.sh 20`, runner caps, `ReadWritePaths=/sys/fs/bpf/unheaded`. Keep `Restart=on-failure`, timeouts, `PartOf=unheaded-doom.target`.
- [ ] **Step 40** [W] ~6m: Rewrite `deploy/systemd/unheaded-doom-injector.service` `[Service]`: `RuntimeDirectory=unheaded-doom`, `ExecStartPre` writes `/run/unheaded-doom/injector.env`, `EnvironmentFile=/run/unheaded-doom/injector.env`, `ExecStart` uses `/usr/local/libexec/unheaded/doom/doom-go-injector`, injector caps and hardening. Everything else unchanged.
- [ ] **Step 41** [W] ~8m: `scripts/doom-install-units.sh` (new, `set -euo pipefail`, GPL SPDX header): with `sudo`, `install -d -m 0755 /usr/local/libexec/unheaded/doom /var/lib/unheaded/doom/rom`; `install -m 0755 crates/doom-runner/target/release/doom-runner cmd/doom-go-injector/doom-go-injector scripts/doom-ring.sh scripts/doom-xdp-attach.sh /usr/local/libexec/unheaded/doom/`; `install -m 0644 doom/doom.mbc doom/doom.elf doom/doom.rv2mbc /var/lib/unheaded/doom/rom/`; `install -d -m 0750 -o root -g unheaded-doom /var/lib/unheaded/doom/wad`; `install -m 0640 -o root -g unheaded-doom "${DOOM_WAD:?set DOOM_WAD to the WAD path}" /var/lib/unheaded/doom/wad/DOOM.WAD`; `install -m 0644 deploy/sysusers.d/unheaded.conf /etc/sysusers.d/unheaded.conf && systemd-sysusers`; `install -m 0644 deploy/systemd/unheaded-doom-*.service deploy/systemd/unheaded-doom.target /etc/systemd/system/ && systemctl daemon-reload`; `--check` mode prints what it would do and exits 0. Nothing in it reads or writes the repo's `.gitignore`d `demos/doom` outputs.
- [ ] **Step 42** [B] ~1m: Unit files verify offline
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/doom-install-units.sh && for u in deploy/systemd/unheaded-doom-*.service; do systemd-analyze verify "$u" 2>&1 | grep -v -e 'is not executable' -e 'No such file' || true; done; echo verify-done
  ```
- [ ] **Step 43** [V] ~1m: Only `verify-done` prints (no parse errors; missing-binary warnings are filtered because the binaries are installed in Step 49, not present at repo paths).

### The gate flips to enforce

- [ ] **Step 44** [CODE][W] ~6m: `scripts/check-systemd-units.sh`: after the existing per-unit loop, add a second check: every `*.service` in `deploy/systemd` MUST have `User=` or `DynamicUser=yes`, except names listed in a new `declare -A ROOT_ALLOWLIST` with a reason, seeded ONLY with `unheaded-well-backup.service`, `unheaded-well-drill.service`, `unheaded-well-offsite.service`, `unheaded-well-offsite-check.service`, `kingdom-startup.service`, `kingdom-shutdown.service`, `node-exporter.service` each with reason `FND-010 Well-jobs half (docker exec); not S-DOOM-LAST scope` / `host lifecycle oneshot` / `upstream unit, own account via its package`; print `[FAIL] <unit>: runs as root and is not in ROOT_ALLOWLIST` and set `fail=1`. Update the script header comment: the gate now enforces, no longer verify-only.
- [ ] **Step 45** [TEST][B] ~1m: The gate passes on the new units and would have failed on the old ones
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -4; git stash -q && (bash scripts/check-systemd-units.sh 2>&1 | grep -c 'runs as root' || true); git stash pop -q
  ```
- [ ] **Step 46** [V] ~1m: First run ends `[PASS]`; the stashed run counted `3` (the three Doom units). If the stashed count is 0 the new check is not wired; if the first run fails, a unit outside Doom lacks `User=` and is not allowlisted: add it to `ROOT_ALLOWLIST` with its reason and a register pointer, never silently.
- [ ] **Step 47** [C] ~2m: **COMMIT CHECKPOINT** `[PLAN S-DOOM-LAST] Steps 36-46: Doom units run as unheaded-doom from /var/lib/unheaded/doom; User= gate enforces` (trailer as Step 25).

### Live on west (E1)

- [ ] **Step 48** [ESCALATE] ~0m: **E1 — install the Doom units on west.** Context: FND-010 says the Doom units are installed on no host; installing sysusers + three units + copying the WAD to `/var/lib/unheaded/doom/wad` is a host change. Questions: (1) Window now? (2) WAD path to copy from (default `~/tmp/projects/doom-related/DOOM.WAD`)? Why no recommendation: host changes are Stevie's by standing rule; the rest of the plan does not need it, so this chain stops here while Phases 3-5 continue.
- [ ] **Step 49** [AFTER-E1][LIVE-RW][S][BARE-METAL][B] ~5m: Install
  ```bash
  cd /home/govan/tmp/unheaded && make build-upc-bootctl >/dev/null 2>&1 || true; (cd crates/doom-runner && cargo build --release 2>&1 | tail -1); (cd cmd/doom-go-injector && go build -o doom-go-injector . ) && sudo DOOM_WAD="${DOOM_WAD:-$HOME/tmp/projects/doom-related/DOOM.WAD}" bash scripts/doom-install-units.sh && id unheaded-doom && sudo systemctl start unheaded-doom.target && sleep 20 && systemctl --no-pager status unheaded-doom-ring unheaded-doom-runner unheaded-doom-injector 2>&1 | grep -E 'Active|Main PID|User' 
  ```
- [ ] **Step 50** [AFTER-E1][LIVE-RO][V] ~3m: All three `Active: active`; `ps -o user= -p $(systemctl show -p MainPID --value unheaded-doom-runner)` prints `unheaded-doom`; frames are live: `curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:16666/` is `200` and `sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>/dev/null | head -3` is non-empty. If a unit fails → [D] Step 50a.
- [ ] **Step 50a** [AFTER-E1][D][S] ~5m: Attribute the EPERM
  ```bash
  journalctl -u unheaded-doom-ring -u unheaded-doom-runner -u unheaded-doom-injector --since '-5 min' --no-pager | grep -iE 'permission|EPERM|denied|operation not permitted|No such file' | tail -8
  ```
  A bpffs `EPERM` → add `CAP_SYS_ADMIN` was missing or `ReadWritePaths` lacks `/sys/fs/bpf/unheaded`; a netns `EPERM` → `CAP_SYS_ADMIN` on that unit; `No such file` → the install copied the wrong path. Fix the unit, `sudo install` it again, `daemon-reload`, restart the target. Two attempts, then Skip Protocol: leave `unheaded-doom.target` stopped, record the exact denial in FINDINGS.md.
- [ ] **Step 51** [AFTER-E1][LIVE-RW][S][B] ~2m: Window close: stop the target, leave the units installed
  ```bash
  sudo systemctl stop unheaded-doom.target && sudo systemctl is-active unheaded-doom-runner || echo stopped; sudo bpftool prog show 2>/dev/null | grep -c monad_cpu || true; ip netns list | grep -c monad || echo "no netns"
  ```
  Expect `stopped`, `0`, `no netns` (the ring unit's `ExecStop` tears down).
- [ ] **Step 52** [V] ~1m: **PHASE 2 EXIT GATE**: Steps 34, 43, 46 passed and committed; if E1 answered, Step 50 passed and Step 51 left west clean; `scripts/check-systemd-units.sh` `[PASS]`.
  - If pass → Phase 3 (or Phase 3 already ran in parallel while E1 waited). If Steps 49-51 are pending E1, the phase is `[AFTER-E1]`-incomplete, not STUCK.

---

## PHASE 3: ARTIFACT PROVENANCE (Steps 53-66)

**Goal**: M-244: `doom/doom.mbc` is what the committed translator emits from `doom/doom.elf`, proven on every harness run; the units and the harness load the same files.
**Prerequisite**: Phase 0 baseline (Step 11).
**Time**: 1.5 h.
**Agent**: Coordinator.

- [ ] **Step 53** [BUILD][B] ~5m: Build the translator the Makefile names
  ```bash
  cd /home/govan/tmp/unheaded/crates/monad-mbc && cargo build --release --bin rv32i-to-mbc 2>&1 | tail -2 && ls -la target/release/rv32i-to-mbc
  ```
- [ ] **Step 54** [B] ~2m: Translate the committed ELF to a scratch file and compare
  ```bash
  cd /home/govan/tmp/unheaded && crates/monad-mbc/target/release/rv32i-to-mbc doom/doom.elf -o /tmp/doom-last.mbc --stats 2>&1 | tail -4; sha256sum /tmp/doom-last.mbc doom/doom.mbc; ls -la /tmp/doom-last.mbc doom/doom.mbc | awk '{print $5, $9}'
  ```
- [ ] **Step 55** [V] ~1m: `/tmp/doom-last.mbc` sha starts `dcded746` (S5's measurement) and differs from `a8f65173`. If they are EQUAL, S5's drift was already healed by a later translator commit: record that, skip to Step 61 with "no regeneration needed". If `/tmp/doom-last.mbc` is neither → the translator changed since S5; record its sha and continue (it is still the only reproducible output).
- [ ] **Step 56** [B] ~1m: rv2mbc is produced by the same tool? Find how
  ```bash
  cd /home/govan/tmp/unheaded && grep -rnE 'rv2mbc' demos/doom/Makefile crates/monad-mbc/src/bin/*.rs 2>/dev/null | head -6
  ```
- [ ] **Step 57** [DECIDE][DESIGN] ~3m: **Regenerate the committed ROM from the committed ELF.** RECOMMENDATION: replace `doom/doom.mbc` (and `doom/doom.rv2mbc` if Step 56 shows the translator emits it and it differs) with the Step 54 output; keep `doom/doom.elf` as is. Rationale: S5 proved the translator's Doom output stable before/after its CSR fix (`s5-upc-wire-log.md:85`), so `dcded746…` is the reproducible artefact; the committed `a8f6…` came from a translator no commit can rebuild. Override ONLY if Step 60's load fails or the insn count rises above the 950,000 getpid-flip ceiling.
- [ ] **Step 58** [REGEN][B] ~1m: Regenerate in place
  ```bash
  cd /home/govan/tmp/unheaded && cp /tmp/doom-last.mbc doom/doom.mbc && sha256sum doom/doom.mbc doom/doom.rv2mbc doom/doom.elf | tee doom/SHA256SUMS
  ```
- [ ] **Step 59** [B] ~2m: Size limits and known-regression checks still hold
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/doom-build-check.sh --build-dir /home/govan/tmp/unheaded/doom 2>&1 | tail -6
  ```
- [ ] **Step 60** [B][S][BARE-METAL] ~15m: The regenerated ROM loads and runs through the harness; insn count recorded
  ```bash
  cd /home/govan/tmp/unheaded && sudo UPC_VERIFIER_STATS=1 scripts/upc-regression.sh 2>&1 | grep -E 'Doom|PASS|FAIL' | head -8
  ```
- [ ] **Step 61** [V] ~2m: `Doom object loads` PASS; `Doom verifier: N` recorded next to the baseline in the progress file. If N differs from 761,580, write one line explaining it (different ROM bytes → different translated instruction stream; the eBPF object is untouched) and update the baseline comment in `scripts/upc-regression.sh:109-113`. If the load FAILS → revert Step 58 (`git checkout doom/doom.mbc`), mark M-244 STUCK with the harness log path, continue.
- [ ] **Step 62** [CODE][W] ~8m: Self-consistency check in the harness. In `scripts/upc-regression.sh`, before Variant 2, add Variant 2a: if `crates/monad-mbc/target/release/rv32i-to-mbc` exists, translate `doom/doom.elf` to `$TMP/doom.mbc` and compare `sha256sum` against `doom/doom.mbc`; `pass "Doom ROM matches translator output"` or `fail "doom/doom.mbc is not the committed translator's output from doom/doom.elf (regenerate: make -C demos/doom translator && rv32i-to-mbc doom/doom.elf -o doom/doom.mbc)"`; if the binary is absent, `note` and skip (no silent pass). Also assert `doom/SHA256SUMS` with `sha256sum -c --quiet doom/SHA256SUMS`.
- [ ] **Step 63** [TEST][B] ~15m: Harness green with the new variant
  ```bash
  cd /home/govan/tmp/unheaded && bash -n scripts/upc-regression.sh && sudo scripts/upc-regression.sh 2>&1 | grep -E 'ROM matches|SHA256SUMS|PASS|FAIL' | head -8
  ```
- [ ] **Step 64** [V] ~1m: `Doom ROM matches translator output` PASS, no FAIL lines, `ALL GREEN` (or the harness's equivalent closing line).
- [ ] **Step 65** [C][DOC-UPDATE] ~4m: Add a `## Provenance` section to `docs/doom/DOOM-BUILD-GUIDE.md`: the committed triple, the exact translate command, `doom/SHA256SUMS`, the harness Variant 2a, and the 2026-04-02 → 2026-10-27 drift note (old sha `a8f65173…`). Then **COMMIT CHECKPOINT** `[PLAN S-DOOM-LAST] Steps 53-65: doom/doom.mbc regenerated from doom/doom.elf; harness checks ROM provenance` (trailer as Step 25).
- [ ] **Step 66** [V] ~1m: **PHASE 3 EXIT GATE**: Steps 59, 61, 64 passed; `sha256sum -c doom/SHA256SUMS` clean; tree clean.
  - If pass → Phase 4. If fail → fix within Phase 3; never leave a ROM in `doom/` that the harness cannot reproduce.

---

## PHASE 4: H1 — CPU_MAP AND .BSS READBACK, ONE SENTINEL SHIFT (Steps 67-78)

**Goal**: M-243: close H1 (`HANDOFF.md:124-136`) with evidence either way: do RAM_MAP reads of `.bss` through bpftool see what the running game writes, and does CPU_MAP key 0xDE describe the live instance? Box: 60 min, then the phase closes with its findings written, whatever the answer.
**Prerequisite**: Phase 3 (the ROM in `doom/` is now reproducible); a west ring (Step 10 pattern); `DOOM_WINDOW` or the Step 2 gate.
**Time**: 60 min hard box (Marshal).
**Agent**: Coordinator.

- [ ] **Step 67** [R] ~3m: Map types and key layout the diagnosis depends on
  ```bash
  cd /home/govan/tmp/unheaded && grep -nE 'RAM_MAP|CPU_MAP|SCREEN|BSS|RAM_BASE|SCREEN_BASE' crates/doom-runner/src/memory.rs | head -20; grep -nE '#\[map\]|BPF_MAP_TYPE|PerCpu|Array<|HashMap<' ebpf/monad-cpu-ebpf/src/main.rs | grep -iE 'ram|cpu' | head -8
  ```
- [ ] **Step 68** [V] ~1m: You can state: RAM_MAP type (Array vs PerCpuArray), word size per entry, how a byte address maps to a key (`addr >> 2` per `mem_read_word`), and CPU_MAP's key for instance 0xDE. If RAM_MAP is **PerCpu**, H1 is explained already (bpftool shows every CPU's copy; the game's writes land on the CPU that ran the XDP program): write that down and go to Step 75 with "H1 = per-CPU view, not corruption".
- [ ] **Step 69** [B] ~2m: Pick the sentinel: `gametic` (a `.bss` int that increments 35x per second while the game runs)
  ```bash
  cd /home/govan/tmp/unheaded && riscv64-unknown-elf-nm doom/doom.elf | grep -E ' [bBdD] (gametic|leveltime|diag_buf)$'
  ```
- [ ] **Step 70** [V] ~1m: `gametic` has an address in the `.bss` range memory.rs names. Record `GT=<hex addr>` and `KEY=$((GT>>2))` in the progress file.
- [ ] **Step 71** [B][S][BARE-METAL] ~10m: Start the game through the harness path and sample the sentinel three times, 5 s apart
  ```bash
  cd /home/govan/tmp/unheaded && sudo setsid crates/doom-runner/target/release/doom-runner run --doom-mbc doom/doom.mbc --doom-elf doom/doom.elf --rv2mbc doom/doom.rv2mbc --wad "${DOOM_WAD:-$HOME/tmp/projects/doom-related/DOOM.WAD}" --hops 2 >/tmp/doom-last-h1.log 2>&1 & sleep 25; for i in 1 2 3; do sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key $(printf '%d' $KEY) 2>&1 | tail -1; sleep 5; done; sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/CPU_MAP 2>&1 | head -12
  ```
  (Attach XDP first if the log says it is deferred: `sudo scripts/doom-xdp-attach.sh 20`.)
- [ ] **Step 72** [V] ~2m: Three different `gametic` values, increasing → `.bss` readback WORKS and H1's ".bss values are zero" was a key/layout mistake in March: record which (wrong `>> 2`, wrong map, wrong base). Three identical zeros while `STAT_FRAME_READY` advances in the log → readback really misses `.bss` writes: go to Step 73. CPU_MAP: PC inside ROM range and `halted` ∈ {0,1} → the March "PC=51M, halted=48" was a struct-layout misread; record the layout from `MbcCpuState` (136 bytes, `CLAUDE.md` ASCEND Phase 0).
- [ ] **Step 73** [D][S] ~8m: One sentinel shift: write a marker into the sentinel's word and see whether the game overwrites it
  ```bash
  sudo bpftool map update pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key $(printf '%d' $KEY) value hex aa bb cc dd 2>&1 | tail -1; sleep 3; sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key $(printf '%d' $KEY) | tail -1
  ```
  Marker gone → the game writes there and reads lag or the key was wrong (recheck `KEY` against memory.rs: `.bss` may sit above a RAM window offset). Marker stays → two maps: the executor writes a different map or a per-instance slice (`HANDOFF.md:126`: "CPU_MAP key 0xDE doesn't match the active instance"); dump `bpftool map show` and list every RAM-like map; one more lookup on the other candidate, then stop.
- [ ] **Step 74** [B][S] ~2m: Tear down, leave west clean
  ```bash
  sudo pkill -9 -f "doom-runner run" ; sleep 1; sudo /home/govan/tmp/unheaded/crates/doom-runner/target/release/doom-runner ring teardown --hops 2 2>&1 | tail -1; sudo bpftool prog show 2>/dev/null | grep -c monad_cpu || true; ip netns list | grep -c monad || echo "no netns"
  ```
- [ ] **Step 75** [W][DOC-UPDATE] ~8m: `docs/doom/FINDINGS.md`: append `## 2026-10-27 — H1 closed: CPU_MAP / .bss readback` with the exact commands, the three samples, the CPU_MAP dump interpretation, the verdict (one of: per-CPU view / key arithmetic / layout misread / genuine two-map split) and what it means for H2/H3 (if `.bss` readback works, Phase 5 may read results from `.bss` as well as `SCREEN_BASE`). `docs/doom/HANDOFF.md` H1 section gets one line at the top: `**CLOSED 2026-10-27 — see FINDINGS.md.**`
- [ ] **Step 76** [V] ~1m: FINDINGS.md has the new section with a verdict line; `HANDOFF.md` H1 carries the CLOSED line; the box (60 min from Step 67) was respected or the overrun is written down.
- [ ] **Step 77** [C] ~2m: **COMMIT CHECKPOINT** `[PLAN S-DOOM-LAST] Steps 67-76: H1 (CPU_MAP/.bss readback) closed with evidence` (trailer as Step 25).
- [ ] **Step 78** [V] ~1m: **PHASE 4 EXIT GATE**: Steps 72 or 73 produced a verdict; Step 74 left west clean; Step 76 passed.
  - If pass → Phase 5. If the box expired with no verdict → mark the phase INCOMPLETE with the samples recorded, commit, continue to Phase 5 (it does not depend on H1's answer; it only gains a second readback path).

---

## PHASE 5: BANDING — ONE DISCIPLINED DIFFERENTIAL PASS (Steps 79-100)

**Goal**: M-241 and M-242: confirm or eliminate H2 (misaligned `LD`) and H3 (word `memcpy`) by running one probe ROM on the reference interpreter and on the live eBPF engine and comparing a results region; then, and only if a divergence is found, fix the executor; only if none is found, run the bump-only zone allocator experiment. Box: 180 min total.
**Prerequisite**: Phases 3 and 4; the riscv toolchain (Step 3); `crates/monad-mbc` reference built (Step 53).
**Time**: 180 min hard box (Marshal). Two sub-boxes: probe differential 90 min; follow-on 90 min.
**Agent**: Coordinator.

### 5a. The probe (both hypotheses in one ROM)

- [ ] **Step 79** [R] ~5m: The exact code under test and the reference's entry points
  ```bash
  cd /home/govan/tmp/unheaded && sed -n '975,990p' ebpf/monad-cpu-ebpf/src/main.rs; sed -n '3686,3700p' ebpf/monad-cpu-ebpf/src/main.rs; grep -nE 'pub struct ExecCpu|pub fn (new|step|run|load_rom|load_ram|ram)\b' crates/monad-mbc/src/execute.rs | head -10; ls crates/monad-mbc/src/bin/
  ```
- [ ] **Step 80** [V] ~2m: You can name: (a) how `LD` computes `word_addr` from the RV address (whether `addr & 3` is dropped), (b) the reference's API to load a ROM + RAM image, run N steps and read RAM back. If (b) has no RAM-readback API → the [D] in Step 85 writes a 40-line test instead of a bin.
- [ ] **Step 81** [W][CODE] ~15m: `demos/doom/probe/probe.c` (new dir, built with the Doom Makefile's `CC`, `CFLAGS`, `crt0.S`, `linker.ld`, `libc_stubs.c`): a `main` that (1) fills a 64-byte `.data` pattern `0x00..0x3F`; (2) H2: for `off` in 0..3 does `uint32_t v; memcpy(&v, src+off, 4)` AND a direct `*(volatile uint32_t*)(src+off)` load (GCC may emit a misaligned `lw` for the latter on RV32 with `-mno-strict-align`; record which flags the Makefile uses), storing both with byte stores (`STB`, known-good) at `SCREEN_BASE + 0x100 + off*8`; (3) H3: `memcpy(dst, src+1, 37)` (misaligned src, aligned dst) and `memcpy(dst2, src, 40)` (both aligned → word path) then byte-stores both results at `SCREEN_BASE + 0x200`; (4) writes `0xC0FFEE42` as four byte-stores at `SCREEN_BASE + 0x300` as a done marker; then loops forever. Add `demos/doom/probe/Makefile` reusing the parent's variables (`include ../Makefile` is wrong, copy the five variable lines) producing `probe.elf`, `probe.mbc`, `probe.rv2mbc`, `probe.data` via the same `rv32i-to-mbc` invocation as `demos/doom/Makefile:90-92`.
- [ ] **Step 82** [BUILD][B] ~3m: Build the probe
  ```bash
  cd /home/govan/tmp/unheaded/demos/doom/probe && make 2>&1 | tail -4 && riscv64-unknown-elf-objdump -d probe.elf | grep -cE '^\s+[0-9a-f]+:\s+[0-9a-f]+\s+lw\b' && ls -la probe.mbc probe.rv2mbc
  ```
- [ ] **Step 83** [V] ~1m: `probe.mbc` exists; `lw` count > 0. If the translator rejects the probe → it is the same toolchain Doom uses; read its error, fix the probe, max 2 tries.
- [ ] **Step 84** [W][CODE] ~15m: Host arm. `crates/monad-mbc/tests/probe_differential.rs` (or `src/bin/probe_run.rs` if the API from Step 80 favours it): load `probe.mbc` + `probe.data` into `ExecCpu` with the Doom memory layout (`SCREEN_BASE` 0x70000, RAM base from `crates/doom-runner/src/memory.rs`), run up to 5,000,000 steps or until the word at `SCREEN_BASE+0x300` reads `0xC0FFEE42`, then dump `SCREEN_BASE+0x100..0x340` as hex to `/tmp/probe-host.hex`. Expected values are computable in the test: assert the host arm itself is self-consistent (misaligned copy == pattern slice) so a reference bug is caught before blaming eBPF.
- [ ] **Step 85** [TEST][B] ~3m: Host arm runs and is self-consistent
  ```bash
  cd /home/govan/tmp/unheaded/crates/monad-mbc && PROBE_DIR=/home/govan/tmp/unheaded/demos/doom/probe cargo test --release --test probe_differential -- --nocapture 2>&1 | tail -6; head -c 200 /tmp/probe-host.hex; echo
  ```
  - If the reference itself fails self-consistency → STOP this chain: the reference interpreter has the bug, record it in FINDINGS.md, open a row for S-TRUE-WIRE/E12 (reference vs spec), skip to Step 97.
- [ ] **Step 86** [B][S][BARE-METAL] ~10m: Live arm. Run the probe on the real engine through doom-runner (it is a ROM + ELF + rv2mbc like Doom; the WAD is still mapped but unused) and read the same region
  ```bash
  cd /home/govan/tmp/unheaded && sudo setsid crates/doom-runner/target/release/doom-runner run --doom-mbc demos/doom/probe/probe.mbc --doom-elf demos/doom/probe/probe.elf --rv2mbc demos/doom/probe/probe.rv2mbc --wad "${DOOM_WAD:-$HOME/tmp/projects/doom-related/DOOM.WAD}" --hops 2 >/tmp/doom-last-probe.log 2>&1 & sleep 15; sudo scripts/doom-xdp-attach.sh 20 >/dev/null 2>&1; (cd /home/govan/tmp/unheaded/cmd/doom-go-injector && sudo timeout 20 ip netns exec monad0 ./doom-go-injector -iface veth01 -src-mac "$(sudo ip netns exec monad0 cat /sys/class/net/veth01/address)" -dst-mac "$(sudo ip netns exec monad1 cat /sys/class/net/veth01p/address)" -mode sendmmsg -batch 64 -flow-label 222 -count 200000 >/dev/null 2>&1 || true); for k in $(seq $((0x70300>>2)) $((0x70300>>2))); do sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP key $k | tail -1; done; python3 - <<'PY' > /tmp/probe-live.hex
import subprocess
out=[]
for k in range(0x70100>>2, 0x70340>>2):
    r=subprocess.run(["sudo","bpftool","map","lookup","pinned","/sys/fs/bpf/unheaded/doom-ring/maps/RAM_MAP","key",str(k)],capture_output=True,text=True)
    out.append(r.stdout.strip().split("value:")[-1].strip())
print("\n".join(out))
PY
  head -c 200 /tmp/probe-live.hex; echo
  ```
  - If the done marker (`0xC0FFEE42` little-endian bytes `42 ee ff c0`) is not present after 200k packets, inject 200k more once; if still absent, the probe never reached `main` on the engine: that is itself a finding (loader/entry), record, Skip Protocol.
- [ ] **Step 87** [B] ~1m: Compare
  ```bash
  diff <(tr -d ' \n' </tmp/probe-host.hex | fold -w8) <(tr -d ' \n' </tmp/probe-live.hex | fold -w8) | head -20 && echo "IDENTICAL" || echo "DIVERGENT"
  ```
- [ ] **Step 88** [V] ~3m: `IDENTICAL` → H2 and H3 are ELIMINATED for aligned and misaligned word loads and for both memcpy paths; go to Step 91. `DIVERGENT` → map each differing word back to its probe case (offset 0x100+off*8 → H2 at misalignment `off`; 0x200.. → H3 misaligned-src copy or word-path copy) and record which. Go to Step 89.
- [ ] **Step 89** [CODE][W] ~20m: Fix the executor for the divergent case only. H2: `ebpf/monad-cpu-ebpf/src/main.rs` `LD` path (around line 983): when `addr & 3 != 0`, assemble the word from two `mem_read_word` results shifted by `(addr & 3) * 8` (and the mirror for `ST`), guarded so the aligned path is untouched (verifier budget: measure). H3 on an aligned copy with no H2 divergence would point at `ST` word stores or at the translator's handling of the `*dw++ = *sw++` loop: inspect `rv32i-to-mbc` output for that loop before touching the engine. Write the regression as a second probe case first (red), then fix (green).
- [ ] **Step 90** [TEST][B][S] ~15m: Rebuild both Doom objects, re-run Steps 86-87 (expect IDENTICAL), then the full harness
  ```bash
  cd /home/govan/tmp/unheaded && make ebpf-monad-cpu-doom 2>&1 | tail -2 && make ebpf-monad-cpu-ascend 2>&1 | tail -2 && sudo UPC_VERIFIER_STATS=1 scripts/upc-regression.sh 2>&1 | grep -E 'verifier|PASS|FAIL' | head -10
  ```
  - [V]: probe IDENTICAL; harness ALL GREEN; both verifier counts recorded (they will move: say by how much; the ascend ceiling 950,000 must hold); `doom/doom.mbc` sha UNCHANGED (the translator did not change). Then Step 96.

### 5b. No divergence: the allocator experiment (only reached from Step 88 IDENTICAL)

- [ ] **Step 91** [B] ~1m: Is the Doom C core here and writable?
  ```bash
  cd /home/govan/tmp/unheaded && git ls-files -s doom/doomgeneric | cut -c1-6; ls -d ../projects/doomgeneric/unheaded 2>/dev/null && (cd ../projects/doomgeneric && git status --short | head -3; git rev-parse --short HEAD) || echo "ABSENT"
  ```
- [ ] **Step 92** [ESCALATE] ~0m: **E2 — the Doom C core checkout.** Context: the id Doom core (`z_zone.c`, `r_data.c`) lives in `../projects/doomgeneric/unheaded` (pinned by `scripts/ci-fetch-doomgeneric.sh`), outside this repo. If Step 91 says `ABSENT`, fetching it and creating an experiment branch there is Stevie's call (it is his fork and its history is not this repo's). Questions: fetch with `scripts/ci-fetch-doomgeneric.sh`? branch name `exp/zone-bump-only`? Why no recommendation: touching a second repository is outside this plan's write scope. If Step 91 shows the checkout present and clean, E2 is answered by default (branch locally, never push) and this step is skipped.
- [ ] **Step 93** [AFTER-E2][W][CODE] ~25m: In the sibling checkout, branch `exp/zone-bump-only`; in `z_zone.c` make `Z_Malloc` bump-only inside the existing 12 MB zone (sequential allocation, `Z_Free`/`Z_ChangeTag`/purge become no-ops, headers still written so `Z_CheckHeap` passes); keep `r_data.c` stock (no malloc workaround, per THE PARADOX row 1 vs 3). Rebuild through `demos/doom/Makefile` (it compiles the core from the sibling path; confirm with `grep -n 'doomgeneric\|projects' demos/doom/Makefile`).
- [ ] **Step 94** [AFTER-E2][B][S] ~15m: Run the experimental build through doom-runner (NOT the committed `doom/` set; use `demos/doom/doom.mbc` outputs) for 3 minutes of injected packets; capture the screen via the smoke checker
  ```bash
  cd /home/govan/tmp/unheaded && sudo scripts/doom-smoke.sh --no-teardown --timeout 180 2>&1 | tail -12
  ```
- [ ] **Step 95** [AFTER-E2][V] ~3m: Smoke passes (game loop runs, screen non-zero). Banding itself is visual: capture `SCREEN_BASE` to a PPM for Stevie (`python3 scripts/doom-screen-check.py --dump /tmp/doom-last-zone.ppm` if the checker supports it; else dump 64,000 bytes of RAM_MAP from 0x70000 with the Step 86 Python pattern and `convert`-free PPM header). Stability: no `I_Error`/HALT in 3 min. Record both in FINDINGS.md as the experiment's outcome. Teardown as Step 74.
- [ ] **Step 96** [ESCALATE] ~0m: **E3 — one attended look.** Context: west has no headless browser. Questions: (1) open `http://west:16666/` during a 5-minute window with the current build and say whether first-room pillars band; (2) `-warp`/E1M2: walk into E1M2 and say whether textures are wrong the same way (M-242); (3) read `/tmp/doom-last-zone.ppm` if Step 95 produced it. Why no recommendation: only eyes settle "banding gone".
- [ ] **Step 97** [W][DOC-UPDATE] ~10m: `docs/doom/FINDINGS.md`: `## 2026-10-27 — Banding differential pass` with: probe design, host/live hex paths, IDENTICAL/DIVERGENT verdict per hypothesis, the fix (if any) with verifier deltas, the allocator experiment outcome (if run), and the exact remaining hypothesis list (H4 fd drift is untouched by design). `docs/doom/HANDOFF.md` top: `**2026-10-27: H2/H3 verdicts in FINDINGS.md.**` `docs/doom/STATUS.md` banding row status → `FIXED (pending E3 look)` or `OPEN, diagnosed (H2/H3 eliminated)`.
- [ ] **Step 98** [C] ~2m: **COMMIT CHECKPOINT** `[PLAN S-DOOM-LAST] Steps 79-97: banding differential pass; H2/H3 <verdict>` (trailer as Step 25). The probe source and its test are committed; `demos/doom/probe/*.mbc|elf|rv2mbc|data` are build outputs: add them to `demos/doom/.gitignore`.
- [ ] **Step 99** [AFTER-E3][AUDIT-UPDATE] ~5m: Record Stevie's answers in FINDINGS.md and the STATUS.md rows (banding, E1M2). If E1M2 is NOT the same class, open it as its own row in `docs/sweeps/OUTSTANDING-2026-10-27.md` E15 with the observation; never diagnose it as WAD size or IWAD identity (both resolved).
- [ ] **Step 100** [V] ~1m: **PHASE 5 EXIT GATE**: Step 88 produced a verdict for both hypotheses; if DIVERGENT, Step 90 is green and the harness is ALL GREEN; FINDINGS.md updated; the 180-min box respected or the overrun recorded; west clean (`bpftool prog show | grep -c monad_cpu` = 0, no `monad*` netns).
  - If pass → Phase 6. If the box expired mid-5b → close INCOMPLETE with the probe verdicts committed (they are the valuable part), continue.

---

## PHASE 6: STRETCH, DECIDED NOT DONE (Steps 101-102)

**Goal**: M-246: the four stretch items get a recorded decision, not silent neglect.
**Prerequisite**: Phase 5.
**Time**: 15 min.
**Agent**: Coordinator.

- [ ] **Step 101** [DECIDE] ~5m: **Weapon-sprite flash, visible fps, demo video, sound/multiplayer.** RECOMMENDATION: (a) fps: ask Stevie to read the browser's frame counter during E3 (bridge.rs streams at `target_fps` 60; `crates/doom-runner/src/bridge.rs:49-57`), record the number in NEXT-STEPS.md, no code; (b) weapon flash: record as open, likely the same column-draw timing as before the back buffer, no work; (c) demo video: Stevie's, not an agent's; (d) sound/multiplayer: SKIP, each is an ISA/spec gate (`i_sound_mbc.c` is a no-op by design) and Doom is lowest priority. Rationale: none of these moves the platform; all four cost attended time. Override ONLY if Stevie names Doom UX as a goal.
- [ ] **Step 102** [W][DOC-UPDATE][V] ~5m: `docs/doom/NEXT-STEPS.md`: rewrite P0-P3 to reflect Step 101 (P0 becomes "banding follow-up per FINDINGS 2026-10-27"; fps/video/flash under "attended only"; sound/multiplayer under "not planned (ISA gate)"). [V]: `grep -c 'ISA gate' docs/doom/NEXT-STEPS.md` ≥ 1. **PHASE 6 EXIT GATE** is this [V].

---

## PHASE 7: SECURITY AND COMPLIANCE REVIEW (Step 103)

**Goal**: What this sprint changed on the attack surface is listed and gated.
**Prerequisite**: Phases 2-5.
**Time**: 25 min.
**Agent**: Coordinator.

- [ ] **Step 103** [SECURITY][COMPLIANCE][B] ~15m: Gates and the surface table
  ```bash
  cd /home/govan/tmp/unheaded && bash scripts/check-systemd-units.sh 2>&1 | tail -2; bash scripts/check-gitleaks.sh 2>&1 | tail -1; bash scripts/check-dependency-baseline.sh 2>&1 | tail -1; for s in scripts/check-spdx*.sh; do bash "$s" 2>&1 | tail -1; done; go mod tidy -diff | head -3; go test ./pkg/uids/... -race -count=1 2>&1 | tail -1; cargo test --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | grep -E '^test result' | head -2
  ```
  **PHASE 7 EXIT GATE** [V]: every gate line above prints PASS/ok (or `tail` shows the known baseline), `go mod tidy -diff` empty, no new test failure versus Phase 0. Then write the review table into the progress file: | change | surface | mitigation | — rows for: Doom units now run as `unheaded-doom` with `CAP_SYS_ADMIN` (narrower than root; `NoNewPrivileges`, `ProtectSystem=strict`); WAD copied to `/var/lib/unheaded/doom/wad` 0640 root:unheaded-doom (no new network listener; bridge 16666 unchanged); `doom/SHA256SUMS` + harness Variant 2a (new integrity check, no new code path at runtime); executor `LD` change if Step 89 ran (eBPF verified; verifier insn deltas recorded; fuzz: `cd crates/monad-mbc/fuzz && cargo fuzz run mbc_roundtrip -- -max_total_time=120` if the fuzz target exists, else note); probe sources under `demos/doom/probe` (build-only). Everything gated must print PASS/ok.

---

## PHASE 8: DOCS, REGISTERS, HANDOFF (Step 104)

**Goal**: Every register, index and handoff reflects the sprint; the master list rows this sprint closed are ticked.
**Prerequisite**: Phase 7.
**Time**: 40 min.
**Agent**: Coordinator.

- [ ] **Step 104** [DOC-UPDATE][AUDIT-UPDATE][REGEN][C][V] ~40m: In this order, then commit and gate:
  1. `compliance/findings/register.yaml` FND-010: add a `progress:` line "Doom half resolved 2026-10-27 (S-DOOM-LAST Phase 2: unheaded-doom uid 16790, units installed on west Step 49 or pending E1); Well-jobs half open"; if E1 is answered and Step 50 passed and the Well half is already closed elsewhere, set the status the register's schema uses for closed.
  2. `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md` (regenerates the register section; expect no new UNTRIAGED rows).
  3. `docs/sweeps/OUTSTANDING-2026-10-27.md`: E15 rows M-240..M-246 get a trailing `| CLOSED <commit>` or `| PARTIAL: <what remains>` note; decision 50 marked recorded.
  4. `references/timeline.md`: one Age 3 bullet `- **S-DOOM-LAST (2026-10-27): Doom docs tell the truth about banding; Doom units run as unheaded-doom (FND-010 half); doom/doom.mbc reproducible from doom/doom.elf; H1 closed, H2/H3 <verdict>.**` then regenerate mirrors the way `f83c9cde` did (timeguru syncer: `grep -n 'Auto-sync' services/timeguru/cmd/timeguru/main.go` names the path; if it needs the service, run `go run ./services/timeguru/cmd/timeguru -sync-once` or the flag the help lists) and fix the stale header lines (`LAST UPDATED`, `HEAD`, `Synced`) if S-CLEAN-SLATE has not already.
  5. `wiki/`: Doom pages from Step 23 already updated; add the install recipe (`scripts/doom-install-units.sh`, paths under `/var/lib/unheaded/doom`, the three caps) to the wiki page that documents Doom ops (ops facts live in wiki, memory feedback_ops_facts_in_wiki); update `wiki/Port-Registry.md` only if a port changed (none expected).
  6. `~/tmp/next.md`: a `## S-DOOM-LAST` section: what shipped, the three escalation states (E1/E2/E3), the verifier numbers, the exact west clean-up state.
  7. Add the `> **SHIPPED 2026-..**` banner at the top of this file in the S-REAL-STEEL style, listing per phase what ran and every [ESCALATE] still open.
  8. `git add -A docs compliance references wiki scripts deploy pkg demos/doom/probe demos/doom/.gitignore doom crates/monad-mbc/tests && git commit ${SIGN_FLAG} -m "[PLAN S-DOOM-LAST] Steps 101-104: stretch decided, security review, registers, timeline, handoff; SHIPPED banner"` with the Step 25 trailer.
  **PHASE 8 EXIT GATE** [V]: `git status --short` empty; `sha256sum -c doom/SHA256SUMS` clean; `scripts/check-systemd-units.sh` PASS; `scripts/check-timeline-freshness.sh` FRESH; this file's SHIPPED banner names every open escalation; west: `bpftool prog show | grep -c monad_cpu` = 0 and `unheaded-doom.target` inactive unless Stevie asked for it running.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. doom-runner will not start: `Address already in use` on 16666.** 1. `pgrep -af "doom-runner run"`. 2. `pgrep -f "doom-runner run" | xargs -r sudo kill -9`. 3. `sudo ss -lntp | grep 16666 || echo free`. 4. Retry the step once. (RUNBOOK.md Troubleshooting.)

**A2. Frames black / CPU_MAP PC stays 0.** XDP was never attached (`FINDINGS.md` 2026-07-24). 1. `sudo scripts/doom-xdp-attach.sh 20`. 2. Confirm the log line `server ready` and `detected IWAD flavour`. 3. If still black, the ring is stale: `doom-runner ring teardown --hops 2` then redo the step.

**A3. Harness says `Doom object OVER 1M ceiling` after Step 89.** The executor change blew the verifier budget. 1. Read `UPC_VERIFIER_STATS` numbers in the harness log. 2. `git stash` the main.rs change, rebuild, confirm the baseline count returns. 3. Re-implement the misaligned path with a single extra `mem_read_word` and a branchless shift; if still over, STUCK: the fix needs the ascend `bpf_loop` restructuring that S-ASCEND-2 owns.

**A4. `systemd-sysusers` created `unheaded-doom` with a different uid.** 16790 was taken on that host. 1. `getent passwd 16790`. 2. Pick the next free uid in the 167xx host-only block, update `pkg/uids/uids.go`, the sysusers line and `pkg/uids` tests together (the parity test fails otherwise). 3. `sudo userdel unheaded-doom && sudo systemd-sysusers`. Never leave the registry and the host disagreeing.

**A5. Unit EPERM under the new user.** See Step 50a. Order of suspicion: bpffs pin dir (`ReadWritePaths`), `CAP_SYS_ADMIN` for `setns`, `CAP_NET_RAW` for the injector's raw socket, `ProtectHome=yes` blocking a stray `/home` path still in a script (`grep -n /home/govan scripts/doom-ring.sh scripts/doom-xdp-attach.sh`).

**A6. Probe never reaches `main` on the engine (no `0xC0FFEE42`).** 1. Check `doom-runner` log for `rv2mbc` or ROM size rejections. 2. Compare `probe.rv2mbc` entry count against the loader's `--rv2mbc` expectations (`doom-build-check.sh` limits). 3. The probe's `_start` must be RV byte 0 (`crt0.S`, the Gate C lesson: `-e main` omitted → wrong entry). 4. Two attempts, then record and Skip Protocol.

**A7. Doom drift: a debug session is running past its box.** This is the failure mode the Marshal exists for. 1. Stop. 2. Write what you know into FINDINGS.md with timestamps. 3. Commit. 4. Move to the next phase. Doom is the lowest priority of any work; a box overrun is never justified by "almost there".

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel with | Depends on | Time |
|---|---|---|---|---|
| 0 Preflight | Coordinator | – | gate (6 plans SHIPPED or `DOOM_WINDOW=1`) | 45 m |
| 1 Docs truth | Coordinator | – | 0 | 40 m |
| 2 Own user | Coordinator | 3 (while E1 waits) | 1; Steps 49-51 on E1 | 2.5 h |
| 3 Provenance | Coordinator | 2 | 0 (Step 11 baseline) | 1.5 h |
| 4 H1 | Coordinator | – | 3 | 60 m box |
| 5 Banding | Coordinator | – | 3, 4; 5b on E2; look on E3 | 180 m box |
| 6 Stretch | Coordinator | – | 5 | 15 m |
| 7 Security | Coordinator | – | 2-5 | 25 m |
| 8 Docs/handoff | Coordinator | – | 7 | 40 m |

**Critical path**: 0 → 1 → 3 → 4 → 5 → 6 → 7 → 8 (Phase 2's code half runs beside 3; its live half waits for E1 and is never on the path).

**Dependency map (escalations)**: E1 blocks Steps 49-51 only (Phase 2 live install; FND-010 closure line in Step 104.1 says "pending E1" otherwise). E2 blocks Steps 93-95 only (allocator experiment; the probe verdicts in 5a never need it). E3 blocks Steps 96, 99 and the fps number in 101(a); every other step and the SHIPPED banner proceed, with the banner listing E3 as open.

## APPENDIX C: QUICK REFERENCE

| Item | Value |
|---|---|
| Committed ROM triple | `doom/doom.elf` `3a04e4dd…`, `doom/doom.mbc` `a8f65173…` (pre-Phase 3) → translator output (`dcded746…` at S5), `doom/doom.rv2mbc` `e7ab7b67…`; `doom/SHA256SUMS` after Step 58 |
| Translate | `crates/monad-mbc/target/release/rv32i-to-mbc <elf> -o <mbc> --stats` (`demos/doom/Makefile:37,90-92`); build with `cd crates/monad-mbc && cargo build --release --bin rv32i-to-mbc` |
| Harness | `sudo UPC_VERIFIER_STATS=1 scripts/upc-regression.sh`; Variant 2 = Doom load; Variant 2a (new) = ROM provenance; baselines ascend 926,542 / Doom 761,580 (S5) |
| Smoke / build checks | `sudo scripts/doom-smoke.sh [--rebuild] [--timeout N]`; `scripts/doom-build-check.sh --build-dir DIR` (mbc < 262,144 words, rv2mbc < 65,536) |
| eBPF builds | `make ebpf-monad-cpu-doom`, `make ebpf-monad-cpu-ascend` (ascend needs `--features ascend-linux`) |
| Map pins | `/sys/fs/bpf/unheaded/doom-ring/maps/{CPU_MAP,RAM_MAP,...}`; RAM word key = byte addr `>> 2`; `SCREEN_BASE` 0x70000 (64,000 B) |
| Ring | netns `monad0`/`monad1`, veth `veth01`/`veth01p`, flow label 222 (0xDE), `doom-runner ring teardown --hops 2` |
| Ports | doom-bridge 16666, doom-go-injector 16667 (`pkg/ports/ports.go`) |
| Units (after Phase 2) | `User=unheaded-doom` uid 16790; binaries `/usr/local/libexec/unheaded/doom/`; ROM `/var/lib/unheaded/doom/rom/`; WAD `/var/lib/unheaded/doom/wad/DOOM.WAD` 0640; env `/run/unheaded-doom/injector.env`; `scripts/doom-install-units.sh [--check]` |
| WAD | `~/tmp/projects/doom-related/DOOM.WAD` (retail 12,408,292 B) or `doom1.wad` (shareware); never in the repo |
| Doom C core | `../projects/doomgeneric/unheaded` (pinned by `scripts/ci-fetch-doomgeneric.sh`); `doom/doomgeneric` is a gitlink; MBC glue in `demos/doom/` |
| Reference interpreter | `crates/monad-mbc` (`execute.rs`, `ExecCpu`); LICH-015 harness `tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness` (cartridge allowlist arm only) |
| Executor hot spots | `ebpf/monad-cpu-ebpf/src/main.rs:983` (`LD` → `mem_read_word(word_addr)`), `:3686` (`mem_read_word`), `:3772/3792` (byte paths), `:1234` |
| Resolved, never re-open | WAD size (`RUNBOOK.md:437`), IWAD identity (`:426`), PC corruption (`FINDINGS.md:46`, ADR-079, 2026-07-03), PU_CACHE (`HANDOFF.md:108`) |

## APPENDIX D: COVERAGE (master list → steps)

| M-id | Steps | Status at forge |
|---|---|---|
| M-240 docs say "NOT A BUG" | 15-26 | planned |
| M-245 Doom units root → `unheaded-doom`, gate enforces | 27-52 (49-51 on E1) | planned |
| M-244 `doom.mbc` provenance + harness check | 53-66 | planned |
| M-243 CPU_MAP/.bss sentinel, close H1 | 67-78 | planned |
| M-241 banding H2/H3 differential, fix or allocator experiment | 79-98 (93-95 on E2) | planned |
| M-242 E1M2 same class or not | 19, 96, 99 (on E3) | planned |
| M-246 stretch (flash, fps, video, sound) | 101-102 | decided-not-done |
| Decision 50 (Doom stays in repo) | 20 | recorded |
| FND-010 Well-jobs half | – | NOT this plan (S-WELL-SPRING scope; `ROOT_ALLOWLIST` cites it) |
| H4 fd drift | – | untouched by design; listed in Step 97 as remaining |

---

*S-DOOM-LAST Battle Plan — Forged 2026-10-27*
*9 Phases. 104 Steps. The Doom docs stop lying, the units stop running as root, the ROM becomes reproducible, and two executor hypotheses get one honest test inside a hard box.*
