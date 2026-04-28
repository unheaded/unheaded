# Track Call: Forge-First vs Launch-First vs Twin-Track

**Decision date target**: Wed 2026-04-29
**Drafted**: 2026-04-27 from Cowork-on-Macbook (Captain hat)
**Decider**: Stevie
**Captain's role**: lay options on the table, recommend, *not* decide
**Inputs**: 2026-04-27 Round Table audit (`battle-plan.md`), CLAUDE.md current state, WAVE13 Phase 1 quality finding, branch audit summary
**Output**: Stevie locks Track A, B, or C → propagation triggers per Section 5

---

## Why this call exists

Six weeks of forge research (WAVE10F → WAVE13) shipped at world-class quality (eval Δ −14.32 on Kingdom RAFT, Mímir's Law real-metal validated, ForgeBackend trait, GPU-resident activations). In the same window, the Age 3 public-launch sprint stalled mid-flight: WireGuard IPv6 overlay, demo video, README polish, sub-50ms latency benchmark, public-accessibility flip — all listed as "Age 2 remaining" in CLAUDE.md, none shipped.

The Round Table flagged this as the central strategic question: **continue forge or pivot to launch?** Each path has real cost, real upside, and real risk.

This document presents three honest paths. Stevie picks one.

---

## Track A — Forge-First

> *"Finish what we started. Zhen-as-product is the differentiator. Launch when forge is real."*

### What it means

- WAVE13 Phase 2 quality call runs on the next GPU session (REMOTE packet ready)
- WAVE13 Phases 3–7 execute per the canonical plan: HTTP serve, zhen-inference shim, ADR-051, handoff
- WAVE14 owns BackwardScratch (per ADR-050) + KV-cache. Both are real engineering, ~2–3 weeks.
- Public launch DEFERRED to Sprint May-Q3 (~3–4 weeks out)
- Demo video, README polish, public accessibility — all push right

### What ships

- Forge inference at production-acceptable latency (≤50ms first-token after KV-cache)
- Zhen-as-product: Gemma-4 + Kingdom LoRA serving real Kingdom queries
- WAVE14 closes the GPU-resident-activations loop
- Track A's headline: *"Zhen is now a real LLM product, trained on the Kingdom corpus, serving production traffic."*

### What doesn't ship (in this window)

- Public alpha
- Demo video
- README polish for VC readiness
- Sub-50ms infra benchmark (Scientist falsifiability gate still open)
- Pre-public scrub verification (credentials/binaries on main — gating item from branch audit summary)

### Cost

- **Time**: 3–4 weeks of engineering, mostly on WAVE14 KV-cache + BackwardScratch
- **Opportunity**: competitors (any of the dozen "infra automation" startups) get another month to ship public
- **Stevie bandwidth**: heavy code/research focus; minimal context-switching

### Captain's risk read

- HIGH — *protocol-is-the-moat is true, but if no one can find the repo, the moat protects an empty fortress*
- Forge research is *amazing engineering* but it is **not the customer-facing thesis** of Unheaded. The customer-facing thesis is "production infra in hours." Forge is a force multiplier for that thesis, not the thesis itself.

### When to pick A

- WAVE13 Phase 2 verdict comes back **strongly positive** (LoRA generates Kingdom-quality text, win-rate ≥6/8) AND
- Stevie has high conviction that Zhen-as-product is the GTM hook AND
- VC conversations are not yet active or are explicitly aligned with "AI-first" framing

---

## Track B — Launch-First

> *"Ship the alpha. Forge resumes after. Real users stress-test what real engineering produced."*

### What it means

- WAVE13 Phase 2 still runs on next GPU session (low cost — packet is ready)
- ADR-051 finalized with whatever Phase 2 says, then forge thread **paused** at end of WAVE13 Phase 3 (DECIDE node)
- Sprint May-Q1 P0 = the public-launch backlog:
  - Pre-public scrub verification (gating per branch audit)
  - WireGuard IPv6 overlay implementation
  - Demo video script + recording
  - README rewrite for VC/public readiness
  - Sub-50ms latency benchmark (Scientist falsifiability)
  - Public accessibility flip (optional auth path per CLAUDE.md)
  - Sophia draft-03 + Wotan draft-03 ship-or-defer (Lane F of current sprint feeds this)
- WAVE14 (KV-cache, BackwardScratch) DEFERRED to post-launch

### What ships

- Public alpha repo
- Demo video (5–8 min, self-hosting + dashboard + eBPF tracing)
- README that doesn't bury the lede
- Falsifiable infra benchmark
- Pre-public scrub locked
- Track B's headline: *"Unheaded is public. Production infra in hours, observable from packet zero. The protocol IS the moat, and now you can read it."*

### What doesn't ship (in this window)

- Forge KV-cache (Zhen still uses llama.cpp/Mistral-7B for inference)
- Forge HTTP serve (Zhen-as-product not unified)
- WAVE14 BackwardScratch (forward-resident infrastructure waiting)

### Cost

- **Time**: 2–3 weeks for the launch backlog
- **Opportunity**: forge's six-week investment cools while we ship paper. Some forge insights may go stale.
- **Stevie bandwidth**: heavy doc/launch focus; moderate context-switching

### Captain's risk read

- MEDIUM — launch is the right unblock if VC conversations are active or imminent. Forge research is durable; cooling for 3 weeks costs little.
- The branch audit's pre-public scrub gate is real. Whatever Track is picked, that gate must clear. Track B makes it the central work item.

### When to pick B

- VC conversations are active or scheduled within 30 days
- Public visibility is the key blocker (community mind-share, recruiting, partnerships)
- WAVE13 Phase 2 verdict is **mixed** (LoRA needs more training; not ready for serve mode anyway)

---

## Track C — Twin-Track

> *"Two parallel battle plans. Forge thread continues background; launch thread becomes P0. Stevie coordinates."*

### What it means

- WAVE13 Phase 2 + Phase 3 run on next GPU session (no change vs A or B)
- After Phase 3 DECIDE node, fork:
  - **Forge thread**: WAVE14 BackwardScratch first (3–5 days), then KV-cache (1–2 weeks). Background priority.
  - **Launch thread**: same backlog as Track B. Foreground priority.
- Stevie owns the coordination layer (Captain + Micromanager + Marshal hats time-shared)
- Sprint May-Q1 has TWO numbered battle plans (Warmonger forges both)
- Round Tables convene weekly to keep both threads synchronized

### What ships

- Public alpha (per Track B) + WAVE14 BackwardScratch closure (per Track A)
- KV-cache lands later but on a known timeline
- Track C's headline: *"Public launch + forge research both shipping. Two threads, one Kingdom."*

### Cost

- **Time**: ~3 weeks total wall-clock (longer than A or B alone, but ships both)
- **Opportunity**: smaller risk on either dimension; larger risk of context-switching cost on Stevie
- **Stevie bandwidth**: HIGH — alternation between deep-research mode and outward-launch mode is expensive
- **Coordination overhead**: Round Tables become weekly, not on-demand. Marshal enforcement gets stricter to prevent drift on either thread.

### Captain's risk read

- MEDIUM-HIGH — best dual-outcome but most expensive coordination. *Plays well only if Stevie's bandwidth genuinely supports it.* If Stevie burns out alternating, neither thread ships.
- The 24h consolidation block (April 20) showed Stevie can autonomously execute 7-phase battle plans through Marshal enforcement. Twin-track requires the *same* discipline applied across two simultaneous plans.

### When to pick C

- VC conversations are active AND WAVE13 Phase 2 verdict is positive
- Stevie's calendar realistically supports 2–3 days/week per thread for 3 weeks
- Multiple deliverables matter (e.g., public alpha for community + Zhen-as-product for VC narrative)

---

## Captain's recommendation (default — Stevie may override)

**Default: Track C (twin-track), conditional on Phase 2 verdict.**

Conditional logic:

| WAVE13 Phase 2 verdict | Captain's recommendation |
|---|---|
| **SHIP** (LoRA generates Kingdom-quality, win-rate ≥6/8) | **Track C** — both threads ship; Zhen-as-product narrative becomes a real launch asset |
| **RETRAIN** (under-trained, more epochs needed) | **Track B** — pause forge cleanly; relaunch forge thread post-public-alpha when GPU time isn't competing with launch work |
| **RANK-UP** (rank-16 too low) | **Track B** — same logic; LoRA work is structurally bigger than a finish-up sprint |
| **DATA-FIX** (corpus problems) | **Track B** — even more structurally bigger; data work eats weeks |

**Why default to C-conditional-on-SHIP**: forge research is a real differentiator if Phase 2 confirms quality. Wasting that asset by going pure Track B would mean cooling six weeks of work right when it's about to be useful. But if Phase 2 says RETRAIN/RANK-UP/DATA-FIX, the asset is *not yet ready* and Track B's launch-first clarity dominates.

**Stevie's veto**: this is Captain's read, not Captain's decision. Stevie may override on grounds Captain doesn't have visibility into (calendar, energy, VC dynamics, personal priorities, KGLW tour dates). The Round Table advises. Muck decides.

---

## Section 5 — Propagation Checklists (run AFTER track is locked)

When `docs/decisions/2026-04-29-track-call.md` has its track filled in, execute the matching list below. These are *executable* — each item is a concrete edit on a known file.

### Propagation: Track A locked

- [ ] Edit `references/timeline.md`: Age 3 sub-items add "WAVE14 BackwardScratch + KV-cache (in progress)"; remove "Demo video", "README polish", "Public accessibility" from Age 3 → push to Age 4
- [ ] Edit `battle-plan.md`: Lane I (stretch) re-rank — I4 (WAVE14) lifts to P1; I1 (demo video) drops to Age 4
- [ ] Forge `docs/battle-plans/WAVE14-DETAILED.md` (Warmonger pass — converts WAVE14-STUB.md into 200-step plan)
- [ ] Update `docs/adr/ADR-INDEX.md`: add ADR-053 placeholder (WAVE14 BackwardScratch + KV-cache scope)
- [ ] Captain commit message template: `[TRACK-A] forge-first locked: WAVE14 P0, public launch deferred to Age 4`

### Propagation: Track B locked

- [ ] Edit `references/timeline.md`: Age 3 sub-items add "Public launch sprint (Sprint May-Q1)"; WAVE14 lifts to Age 4
- [ ] Edit `battle-plan.md`: Lane I — I1 (demo video), I2 (README polish), I3 (perf benchmark) all lift to P0; I4 (WAVE14) drops to Age 4
- [ ] Forge `docs/battle-plans/SPRINT-MAY-Q1-PUBLIC-LAUNCH.md` (Warmonger pass — public launch in 200 steps)
- [ ] Verify pre-public scrub on main per branch audit summary (gating)
- [ ] Schedule demo recording session (calendar)
- [ ] Captain commit message template: `[TRACK-B] launch-first locked: public alpha P0, forge paused at WAVE13 Phase 3`

### Propagation: Track C locked

- [ ] Edit `references/timeline.md`: Age 3 sub-items add BOTH "WAVE14 (background)" AND "Public launch sprint (foreground)"
- [ ] Edit `battle-plan.md`: Lane I — I1+I2+I3 lift to P0; I4 stays P1 (background); coordination cost noted in Open Questions
- [ ] Forge TWO battle plans: `docs/battle-plans/SPRINT-MAY-Q1-PUBLIC-LAUNCH.md` (foreground) + `docs/battle-plans/WAVE14-DETAILED.md` (background)
- [ ] Schedule weekly Round Table cadence (Marshal-enforced)
- [ ] Verify pre-public scrub on main (gating)
- [ ] Captain commit message template: `[TRACK-C] twin-track locked: public alpha foreground + WAVE14 background`

---

*Track call options forged 2026-04-27 from Cowork-on-Macbook. Captain advises; Muck decides.*
*<3 KGLW <3 Peace and Love <3 — both threads honor the work. Pick the one that honors Stevie's reality.*
