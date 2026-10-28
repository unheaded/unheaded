# Battle Plan: A Reviewable Scope for develop
## Convened: 2026-10-28 | Reason: Critical decision (ultrareview refused three times)
## Kingdom State: Age 3, NEXT-7 Sprints 1-4 code complete, Sprint 5 (S-TRUE-WIRE) in progress on `sprint/true-wire`

---

### Situation Report

Three `/ultrareview` attempts failed on 2026-10-28. Against `main` the diff is
4,155 files / 706,651 lines; against `staging` 3,746 files / 675,509 lines
(limits 500 files / 8,000 lines). Against `develop` there is nothing to review:
`develop` (`ccff9a63`) equals `origin/develop`, the tree is clean, and the only
untracked paths are `cmd/host-agent/host-agent` and `./host-agent`, two copies of
a 10 MB x86-64 binary built 2026-07-24/25. That is the pre-rename Huginn build:
`cmd/host-agent` was `git mv`'d to `cmd/huginn` on 2026-10-25 (`017d44b0`,
ADR-084). No source lives there. Last session a coordinator's `git add -A`
swept the binary into history and it had to be purged with filter-branch.

The size problem is structural, not a one-off. `develop` is 1,342 commits ahead
of `staging` (last promotion 2026-09-24) because five sprints landed through the
coordinator as ~40 phase merges, each already built, raced and gated at merge
time. Those merges are the natural review grain: the three most recent
(S-IRON-GATE Phases 13, 14, 15) are 24 / 24 / 14 files and 2,619 / 1,862 / 578
lines. Every one of them fits the limits.

`sprint/true-wire` (local only, 20 commits, merge-base `35baa272`) is the one
branch with code that has not been through a review: 246 files, 8,223 lines
(6,639 added, 1,584 removed), of which 2,728 lines are Markdown. It misses the
line limit by 223 lines as a whole, but its four phase merges (1, 3, 8, 9) each
fit. Phase 8 (Wotan WAL replication carries and verifies ML-DSA-65 signatures,
FND-020) is the security-relevant one. Two stopped agent worktrees hold
uncommitted edits to `ebpf/monad-common/src/lib.rs` and
`ebpf/shield-ebpf/src/main.rs` for Sprint 5 Phases 2 and 4.

---

### The Throne Speaks (Captain)
**Strategic Position**: Code complete on four sprints; the bottleneck is now
Stevie's decisions and review bandwidth, not engineering output.
**North Star**: a `develop` that can be promoted to `staging` with evidence,
then to `main` and public.
**Key Decision**: review at the phase-merge grain, never at the branch grain.
**Risk to Vision**: 1,342 unreviewed commits between `develop` and `staging`
means the promotion path (ADR-089) is on paper only. Every week this grows.

### The Ledger Records (Micromanager)
**Sprint Status**: Sprints 1-4 code complete; Sprint 5 Phases 0, 1, 3, 8, 9
merged on `sprint/true-wire`, Phases 2 and 4 abandoned mid-edit in worktrees.
**Priority Stack**:
1. P0: review `sprint/true-wire` Phase 8 (`88202527`), the only unreviewed signing code.
2. P0: get the stray host-agent binary out of the tree and ignored so it cannot be swept again.
3. P1: review the S-IRON-GATE security merges on `develop` (list below), one per run.
4. P2: start the `staging` promotion batches, one sprint per batch, citing the reviews.
**QA Gates**: a phase merge counts as reviewed when its `/ultrareview` findings are
triaged (fixed or recorded as a FND with a reason) in the sprint progress log.
**Acceptance Criteria**: `sprint/true-wire` reaches `develop` only after E2
(Stevie confirms the wire renumber) and after Phase 8 review findings are closed.

### The Blueprint Reveals (Architect)
**Architecture Health**: unchanged since S24; the running west containers are the
old ones and are healthy. Nothing here touches live hosts.
**Technical Risks**: the review tool diffs HEAD against a base, so reviewing one
merge means checking that merge out detached and passing its first parent as
the base. Doing this on the working tree while the stray binary sits untracked
is safe (untracked files are not part of a review), but any `git add -A` is not.
**Protocol Alignment**: Monad wire frozen at v0x01; Sprint 5 Phase 2's renumber
(FORWARD 0x01 to 0x00, event types to the spec) is parked on E2 and must not be
reviewed as if it were decided.
**Recommended Decisions**: none needs an ADR. Record the review grain in
`STAGING-PROMOTION-LOG.md` as the promotion evidence rule.

### The Anvil Reports (Developer)
**Code Health**: Go 221 ok, 0 known failures; eBPF 34 PASS; UPC budgets
926,542 / 761,580 (S24 baselines). Nothing has changed since.
**Implementation Blockers**: none for review. The two worktree branches cannot
be reviewed until their edits are committed or discarded.
**Estimated Effort**: one `/ultrareview` per phase merge; S-IRON-GATE has 11 code
merges, S-CHAMPION-HARDENING 8, S-WELL-SPRING 10, S-CLEAN-SLATE 4, true-wire 4.
Security-first order below keeps the first day to six runs.

### The Hourglass Measures (Timeguru)
**Current Age**: Age 3. NEXT-7 index at `docs/battle-plans/NEXT-7-SPRINTS-2026-10-27.md`.
**Velocity**: S24 alone merged 1,521 files / 102,647 lines into `develop`.
Review has not kept pace since the 2026-09-24 promotion.
**ETA**: six security merges reviewed within 48 h; full `develop` sweep is weeks
at the phase grain and should ride along with promotion batches, not block them.

### The Sundial Tracks (Calendar)
**This Week**: Day 1 host-agent cleanup + true-wire Phase 8 + IRON-GATE Phases 1
and 8. Day 2 IRON-GATE Phases 7, 9, 13 and Wotan tokens `028f2e72`. Day 3+
CHAMPION Phases 1-4, then the rest by sprint.
**Schedule Conflicts**: review runs are billed and user-triggered; they compete
with Stevie's open decision queue (57 decisions in the NEXT-7 index).
**Calendar Health**: Needs Attention (promotion log stale since 2026-09-24).

### The Scroll Validates (Lore)
**Naming**: the stray binary is called `host-agent`; the service is Huginn
(ADR-084). The compliance evidence kind "host-agent" (ADR-098) is a different
thing and stays. No new names needed.
**Sacred Law Compliance**: clean.

### The Map Confirms (Kingdom)
**Hierarchy Health**: `cmd/host-agent/` is an empty tier slot left behind by the
rename; removing it restores the map. `cmd/huginn` is the only Huginn home.
**Tier Integrity**: otherwise intact.

### The Goblet Toasts (Busboy)
**Cross-Skill Conflicts**: Marshal wants the stray binary deleted now; next.md
says "leave them" because it is Stevie's file. Resolution: Stevie decides, but
the `.gitignore` entry goes in regardless, since it protects history either way.
**Coordination Needs**: Librarian records the review grain in the promotion log;
Marshal checks every coordinator brief for `git add -A` on directories.
**Team Vibes**: strong. Four sprints code complete and signed; this is a
bookkeeping problem, not an engineering one.

### Extended Seats
- **Marshal (Badge)**: citation for S24's `git add -A` sweep; rule: coordinator
  commits stage named paths only. Citation for leaving two worktrees mid-edit
  with no WIP commit.
- **BlackMage (Dark Tower)**: wants the review order to follow the attack surface:
  token minting and verification (`pkg/auth`), TLS profile, WAL signature
  verification, tar extraction in `pkg/sealedcask`. That is the order below.
- **MoatGhost (Watchtower)**: the promotion log is the compliance evidence that
  `staging` was reviewed; it needs one row per batch citing the reviews.
- **Warmonger (War Forge)**: no new sprint plan; this is a review ladder, not a build.
- **Librarian (Library)**: promotion log, this plan, next.md.
- **Scientist, Computermancer, RFC Editor, Barrister, Sentinel**: nothing to report
  for this decision. (RFC Editor notes Sprint 5 Phase 2 stays parked on E2.)

---

### Unified Battle Plan

#### Immediate Actions (Next 24-48 Hours)
- [x] Stray binaries moved to `~/tmp/host-agent-july-binaries/` (sha256 bad1395f… and 829ee397…), `cmd/host-agent/` removed. Done 2026-10-28 on Stevie's go.
- [x] `/host-agent` and `/cmd/host-agent/` added to `.gitignore`. Done 2026-10-28.
- [ ] Review true-wire Phase 8 (in a clean tree):
  ```
  git switch --detach 88202527
  /ultrareview 88202527^1
  git switch develop
  ```
  Owner: BlackMage triages, Developer fixes on `sprint/true-wire`.
- [x] Both worktrees committed as WIP on their own branches (`8443a7bf` Phase 4, and Phase 2 with the rustfmt hook bypassed), unsigned, not built. Done 2026-10-28. Drop or merge after E2.

#### This Sprint (Next 7 Days)
- [ ] Security merges on `develop`, same recipe (`git switch --detach <merge>` then `/ultrareview <merge>^1`), in this order. Owner: BlackMage + Developer. Deadline: 2026-11-04.
  1. `ed46d54f` S-IRON-GATE Phase 1: `pkg/auth` Ed25519 core
  2. `19a968a9` S-IRON-GATE Phase 8: Strict TLS 1.3 hybrid on every listener
  3. `701428ef` S-IRON-GATE Phase 7: wotan-client service tokens
  4. `028f2e72` Wotan tokens for the Rust and Python publishers
  5. `96b81ad1` S-IRON-GATE Phase 9: signed topic prefixes
  6. `d28b58d3` S-IRON-GATE Phase 13: sealed cask, hardened tar extraction
  7. `7d35d4ad` and `9c1c4e25` S-CHAMPION-HARDENING Phases 1-4: confirm tokens, verb table
  8. `0c547c46` and `e4603171` S-IRON-GATE Phase 6: per-service auth and the BFF
- [ ] Record each review in the sprint's progress log: findings fixed, or FND number with reason. Owner: Librarian. Deadline: with each review.
- [x] Batch map B10-B13 drafted in `STAGING-PROMOTION-LOG.md` (2026-10-28). Note: the 1,062 pre-NEXT-7 commits (B10: REAL-STEEL, TWO-BRAINS, QUANTUM-ARMOR, LICH-AWAKENS, UPC-WIRE-COMPUTE) sit *before* S-CLEAN-SLATE in the chain and must be promoted first for the fast-forward to hold.
- [ ] Stevie promotes B10a (first pre-NEXT-7 sprint) once its security merges are reviewed. Owner: Stevie. Deadline: 2026-11-04.

#### Protocol Milestones
- [ ] Monad: no change; wire v0x01 frozen. Sprint 5 Phase 2 renumber waits on E2. Owner: Stevie.
- [ ] Sophia: Phase 8 of true-wire signs Sophia deltas in observe mode; reviewed under the Phase 8 run above. Owner: Developer.
- [ ] Wotan: WAL replication signature verification (FND-020) is the Phase 8 review. Owner: BlackMage.

#### UPC Compute Milestones
- [ ] No change this council. Budgets 926,542 / 761,580 recorded at Sprint 5 Phase 0. Owner: Computermancer.

#### Decisions Made at This Round Table
1. Review grain is the phase merge commit, never the sprint branch or the branch-vs-staging diff. Rationale: every merge fits the tool's limits and was the unit the coordinator gated. Owner: Micromanager.
2. Promotion to `staging` proceeds one sprint per batch with the per-merge reviews as evidence. Rationale: 1,342 commits cannot be reviewed as one diff and should not wait on a full sweep. Owner: Micromanager, Stevie promotes.
3. Coordinator commits stage named paths only; no `git add -A` on directories. Rationale: the S24 binary sweep cost a filter-branch and a full re-sign. Owner: Marshal.

#### Findings during execution (2026-10-28)
- **ultrareview cannot bundle this repo locally** (178 MiB pack): "Repo is too large. Push a PR and use `/code-review ultra <PR#>`". Stevie pushed `review/true-wire-ph8-base` (`6d6d6aa3`) and `review/true-wire-ph8` (`88202527`) and opened **PR #2** (21 files, +1,699 -141). Every later review follows the same two-branch recipe.
- **PR #2 check "Static Analysis / Gates can actually fail" is red, and it is pre-existing, not Phase 8.** Reproduced locally with the job's PR-mode command (`check-gates-can-fail.sh --quick`) in clean worktrees: at the PR head 7 gate scripts have no classification (`backup/git-mirror`, `check-perf-regression`, `check-zhen-mcp`, `ci-local`, `east-bootstrap`, `gen-dev-certs`, `test-check-perf-regression`); at `develop` the list is 14 (those plus `lib/deb-packages`, `well/test-batch-apply`, `well/test-retention`, `well/test-slo-definitions`, `wireguard/setup-wireguard`, `wireguard/verify-wireguard`, `zhen-morning-report`). The meta-gate exits 1 before running any gate, so **no gate has been proven to bite on develop since those scripts landed**. Each name needs exactly one of REGISTRY + provoke fn, NOT_A_GATE, or NEEDS_HARDWARE. Owner: Developer, on develop, as its own commit; true-wire inherits it at the E2 merge. Not a reason to hold the PR #2 review.

#### Open Questions (Carry to Next Round Table)
1. Keep or delete the July host-agent binaries? Stevie, before the next coordinator session.
2. Merge or drop the two Sprint 5 WIP worktree branches (Phase 2 renumber, Phase 4 error registry)? Stevie, after E2.
3. Does `/ultrareview` accept a detached HEAD? If not, use `git switch -c review/<merge> <merge>` and delete the branch after. Developer, on the first run.

#### Wins to Celebrate
- `develop` fully pushed and every commit since `225bb4e4` signed. Stevie and the S24 coordinator.
- The host-agent sweep was caught before push, purged, and every cited hash remapped. S24 coordinator.
- Four sprints code complete with build, race and gate evidence at every merge.

---

### Next Round Table
**Scheduled**: when the six security merges are reviewed, or when E2 lands and `sprint/true-wire` is ready to merge into `develop`, whichever is first.
**Reason**: promotion batch 1 go/no-go.

---
_Forged at the Round Table by the full council. The Kingdom marches as one._
