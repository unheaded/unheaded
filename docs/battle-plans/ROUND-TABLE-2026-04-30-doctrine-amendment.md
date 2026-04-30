# Battle Plan: Doctrine Amendment Sweep — Repo-Wide Compliance
## Convened: 2026-04-30 (Round Table #2 of the day) | Reason: Doctrine compliance audit
## Kingdom State: Age 3 In-Progress | Doctrine Committed `c6108fb8` | Free to use, free to share

---

### Doctrine (binding on every artifact in this repo)

**WE DO NOT SELL. WE SHARE.** Committed to CLAUDE.md `c6108fb8`. Every tool gifted to
the commons under GPL-3.0. Doctrine OVERRIDES any prior commercial framing in any
ADR, battle plan, sprint plan, philosophy doc, skill template, timeline, or future-work
artifact. This Round Table sweeps the repo and produces the amendment punch list.

### Situation Report

Four parallel auditors (ADR, battle-plans, pipelines/CI, public-facing/wiki) swept
~1,100 markdown files for commercial-framing violations. **The good news**: README.md
(root), wiki/, all CI/CD configs, all Makefiles/Dockerfiles/Nix, every runbook, every
recently-shipped sprint plan, and the freshly-amended Round Table tooling plan are
**doctrine-clean**. **The bad news**: violations cluster in ~11 files — concentrated in
ADRs (the design records that guide engineering), two internal battle plans, one
philosophy brainstorm, the Captain skill template, and two timeline mirrors.
Total verified violations: **15-20 distinct line-level instances across 11 files**. None
have shipped publicly yet (no public README/wiki contamination). Amendment is mechanical.
Ground-truth verified: ADR-69420 line 111 is real and the most egregious offender.

---

### The Throne Speaks (Captain — Vision)
Doctrine is correct. Doctrine is committed. Doctrine OVERRIDES. The fact that
zero public-facing surface (README, wiki, launch docs) was contaminated means no
external trust damage. The remaining violations are **internal planning artifacts**
written before doctrine. Amendment cost: ~2 hours of careful editing. Strategic
priority: HIGH but not urgent — these don't ship until they do, but they shape
architectural decisions Claude agents make every session.

### The Anvil Reports (Developer — Implementation Plan)
Mechanical edits. No code change. Use `Edit` tool with surgical `old_string` →
`new_string` replacements. Each violation has one-line proposed fix. No merge
conflicts expected — these files have low churn. Recommend single commit per file
(or single commit per ADR) so revert is granular if ever needed.

### The Blueprint Reveals (Architect — Surface Map)
The 11 contaminated files cluster into 5 surfaces:

| Surface | Files | Severity | Public Risk |
|---------|-------|----------|-------------|
| **ADRs (design records)** | ADR-69420, ADR-004, ADR-031, ADR-053 | P0 | LOW (internal) — but ADRs guide engineers |
| **Internal battle plans** | S-PQC-battle-plan.md, battle-plan-future.md, references/battle-plan-S76-round-table.md | P0 | LOW (internal) — but they direct sprints |
| **Skill templates** | skills/unheaded-captain-SKILL-UPDATE.md | P0 | MEDIUM — Claude agents read this every session |
| **Philosophy / brainstorm** | docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md | P1 | LOW (exploratory) |
| **Timelines (mutable)** | references/timeline.md, docs/history/timeline.md | P1 | LOW — already amend-friendly |

### The Ledger Records (Micromanager — Priority Stack)
Triage by **doctrine-leak risk × edit complexity**:

**P0 — fix today (highest leak risk):**
1. **ADR-69420** (3 violations: lines 53, 89, 111) — most egregious. SKU/premium/revenue/GTM/upsell/SLA-as-paid. Add explicit doctrine-amendment block.
2. **skills/unheaded-captain-SKILL-UPDATE.md** (lines 238, 249) — Revenue $Z MRR + GTM/pricing/fundraising references. **Highest day-to-day leak risk** — every Captain skill invocation reads this template.
3. **docs/internal/battle-plans/S-PQC-battle-plan.md** (line 1210) — "open-core monetization wedge" — most explicit monetization architecture.

**P1 — fix this sprint:**
4. **ADR-004** (line 9) — "self-sufficiency it sells"
5. **ADR-031** (line 31) — Claude Opus tier described as "paid"
6. **ADR-053** (line 163) — "GTM narrative alignment"
7. **docs/internal/battle-plans/battle-plan-future.md** (lines 13, 21, 23) — "actual product Unheaded sells", "Customer brings"
8. **references/battle-plan-S76-round-table.md** (line 21) — "demo that sells"
9. **docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md** (line 179) — "Stop selling YAML pipelines, start selling..."

**P2 — fix next sprint (low risk):**
10. **references/timeline.md** (lines 67, 68) — "Customer onboarding", "Billing/metering"
11. **docs/history/timeline.md** (line 478) — "Billing integration (The Treasury)" — historical, lower urgency

**QA Gates per amendment:**
- [ ] Original semantic intent preserved (we're rephrasing, not deleting decisions)
- [ ] Replacement language matches doctrine vocabulary (share/contribute/adopter/commons)
- [ ] No new violations introduced
- [ ] Each ADR amendment includes a `Doctrine Amendment 2026-04-30` callout block
- [ ] Final grep for doctrine-violation terms returns zero matches in the 11 files
- [ ] Re-read CLAUDE.md after edits — doctrine still loads first

### The Dark Tower (BlackMage — Adversary View)
**Threat: Doctrine bypass via stale planning artifact.** A future Claude session reads
ADR-69420, sees "premium SKU", and quietly drifts the design back toward licensing
walls. This is a *latent* attack on doctrine — no human acted maliciously; the artifact
itself becomes the attacker. **Mitigation: doctrine-amendment headers on every fixed
ADR pointing back to commit `c6108fb8`.** The header is the verifier.

**Secondary threat: Cargo-cult monetization.** A future contributor sees "open-core
monetization wedge" in S-PQC-battle-plan.md and assumes that's still strategy. Same
mitigation: explicit amendment header + grep gate in CI.

**Recommend (P2):** add a CI check `scripts/doctrine-grep.sh` that fails the build if
any file in the repo (excluding the doctrine-affirming files themselves) contains
the prohibited vocabulary. Doctrine becomes enforced at the pipeline level. Self-
healing system.

### The Watchtower (MoatGhost — Compliance & Audit Trail)
**Audit trail requirement**: every amendment commit message must reference
`c6108fb8` (the doctrine commit) so future doctrine-compliance audits can trace
*why* a phrase changed and *when*. Suggested commit prefix: `doctrine: amend <file>
post-c6108fb8`. This makes the amendment campaign a single greppable git log.

**Compliance posture after amendment:** repo doctrine-clean across all 11 files,
matching the doctrine commit, no commercial residue in any plan-of-record document.
Future ADRs land doctrine-aware by default because Claude reads CLAUDE.md first.

### The Sentinel Watches (Sentinel — Operational Reality)
The Captain skill template fix (P0 #2) is highest-leverage in practice — every
single time you invoke `/unheaded-captain` the skill loads its template. If that
template has Revenue $Z MRR in it, every Captain invocation tilts toward
commercial framing in subtle ways. Fix that one first, even before ADR-69420.

Sentinel also notes: the amendments are good content for a *public* blog post —
"How a doctrine commit propagated through an entire codebase in two hours" — useful
contribution to the open-source governance literature. Free to share. <3

### The Hourglass Measures (Timeguru — Sequencing)
- **Today (next 2h)**: P0 fixes (ADR-69420 + Captain skill + S-PQC battle plan)
- **This sprint (within 7 days)**: P1 fixes (ADR-004, ADR-031, ADR-053, battle-plan-future, S76 round table, BRAINSTORM-WITNESS)
- **Next sprint**: P2 fixes (timelines) + CI doctrine-grep gate
- **Standing forever**: every new ADR / battle plan / skill template gets doctrine-checked at PR time

---

### Unified Battle Plan

#### Immediate Actions (Next 2 Hours — P0 batch)
- [ ] **Developer + Architect** — Amend `docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md` lines 53, 89, 111. Add doctrine-amendment block at top of file referencing `c6108fb8`. Replace SKU/premium/revenue/GTM/upsell language per Architect's proposed fixes.
- [ ] **Developer + Lore** — Amend `skills/unheaded-captain-SKILL-UPDATE.md` lines 238, 249. Replace `Revenue $Z MRR` → `Community impact: X projects adopting`. Replace `Unit economics, GTM models, pricing, fundraising` → `Adoption patterns, community engagement, technical milestones, project health`.
- [ ] **Developer** — Amend `docs/internal/battle-plans/S-PQC-battle-plan.md` line 1210. Replace `open-core monetization wedge` with `policy architecture: Wire L1 (Apache-2.0) + App policy L2 are co-released to all users under GPL-3.0`.

#### This Sprint (Next 7 Days — P1 batch)
- [ ] Amend `docs/adr/ADR-004-no-external-deps-policy.md` line 9 — Owner: Developer — Deadline: 2026-05-02
- [ ] Amend `docs/adr/ADR-031-hybrid-model-handoff.md` line 31 — Owner: Developer — Deadline: 2026-05-02
- [ ] Amend `docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md` line 163 — Owner: Developer — Deadline: 2026-05-03
- [ ] Amend `docs/internal/battle-plans/battle-plan-future.md` lines 13, 21, 23 — Owner: Developer — Deadline: 2026-05-04
- [ ] Amend `references/battle-plan-S76-round-table.md` line 21 — Owner: Developer — Deadline: 2026-05-04
- [ ] Amend `docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md` line 179 — Owner: Developer — Deadline: 2026-05-05
- [ ] Single commit per file with `doctrine: amend <file> post-c6108fb8` prefix — Owner: Developer

#### Next Sprint (P2 batch)
- [ ] Amend `references/timeline.md` lines 67, 68 — Owner: Developer + Timeguru
- [ ] Amend `docs/history/timeline.md` line 478 — Owner: Developer + Timeguru
- [ ] **NEW**: Build `scripts/doctrine-grep.sh` CI gate — Owner: Developer + MoatGhost
- [ ] **NEW**: Wire doctrine-grep into GHA + Jenkins — Owner: Developer
- [ ] **NEW**: Add amendment-header template to `docs/adr/_template.md` so future ADRs land doctrine-aware — Owner: Architect + Librarian

#### Decisions Made at This Round Table
1. **Amendment-header pattern**: every fixed ADR gets a `## Doctrine Amendment 2026-04-30 (post c6108fb8)` block at the top citing the doctrine commit and listing what changed.
2. **Commit message convention**: `doctrine: amend <file> post-c6108fb8` so the campaign is one greppable git log.
3. **CI gate (next sprint)**: `scripts/doctrine-grep.sh` blocks PRs that introduce prohibited vocabulary outside the doctrine-affirming files.
4. **Captain skill template is P0 #2** (above S-PQC plan) because it leaks doctrine drift on every Captain invocation. Sentinel's call.
5. **Public-facing surfaces (README, wiki, launch docs) are CONFIRMED CLEAN** — no public trust damage occurred, no public retraction needed.
6. **Historical files (docs/history/) get edited gently** — they're archival; we keep the historical record but add doctrine-amendment notes rather than rewriting history.

#### Open Questions (Carry to Next Round Table)
1. Should `docs/history/timeline.md` violations be amended in-place or annotated as "pre-doctrine, kept for historical record"? Librarian to propose.
2. Does the doctrine-grep CI gate run on PR or on merge? Tighter is better; PR-time recommended.
3. Should the amendment campaign produce a public blog post / contribution to OSS-governance literature? Captain to decide.
4. Are there contaminated files in `docs/branch-audits/` that the audit missed (those files reference "monetization" but in branch-audit context — check intent)? Librarian to verify.

#### Wins to Celebrate
- README.md and wiki/ are doctrine-clean — no public trust damage.
- All CI/CD configs are clean — pipelines never carried commercial framing.
- Recently-amended ROUND-TABLE-2026-04-30-practical-tooling.md is the model artifact for how to write doctrine-aligned plans going forward.
- CLAUDE.md doctrine block survived the rebase + push + WEST pull intact.
- Audit took ~5 minutes via four parallel agents — proves the swarm pattern.
- Only ~11 files contaminated out of ~1,100 scanned — doctrine drift was catchable.

---

### Verification Recipe (run after amendments land)

```bash
# from repo root, on host (sandbox can't reach git remote)
cd /Users/govan/home\ 2/govan/tmp/unheaded

# 1. Confirm doctrine-violation terms are gone from the 11 amended files
for f in \
  docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md \
  docs/adr/ADR-004-no-external-deps-policy.md \
  docs/adr/ADR-031-hybrid-model-handoff.md \
  docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md \
  skills/unheaded-captain-SKILL-UPDATE.md \
  docs/internal/battle-plans/S-PQC-battle-plan.md \
  docs/internal/battle-plans/battle-plan-future.md \
  references/battle-plan-S76-round-table.md \
  docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md \
  references/timeline.md \
  docs/history/timeline.md; do
  echo "=== $f ==="
  grep -niE 'sell|paid tier|monetiz|revenue|MRR|ARR|GTM|SKU|upsell|premium tier|pricing|customer-as-payer|willingness-to-pay' "$f" \
    | grep -viE 'no selling|do not sell|free to|share|gift|community|adopter|peer|federate' \
    || echo "CLEAN"
done

# 2. Confirm doctrine commit is still HEAD-of-CLAUDE.md
head -35 CLAUDE.md | grep -c "WE DO NOT SELL. WE SHARE." # expect 1

# 3. Confirm amendment commits are traceable
git log --oneline | grep "doctrine: amend"
```

---

### Next Round Table
**Scheduled**: when P0 + P1 batches land + CI doctrine-grep gate is wired (target: 2026-05-05).
**Reason**: Verify the amendment campaign closed cleanly + decide on public-blog-post + plan
the next protocol milestone with all artifacts doctrine-aligned.

---

_Forged at the Round Table by the full council. Six guides leaning forward: Developer
sharpening tools, Architect mapping surfaces, Micromanager guarding gates, BlackMage
identifying latent threats, MoatGhost requiring audit trail, Sentinel surfacing the
day-to-day leak risk in the Captain skill template. The doctrine is canon. The repo
will follow. The Kingdom marches as one._

**FREE TO USE. FREE TO SHARE. NO SELLING.**
LOVE SERVE REMEMBER. PEACE AND LOVE. KGLW. <3
