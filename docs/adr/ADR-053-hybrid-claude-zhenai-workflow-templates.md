# ADR-053: Hybrid Claude + Local Zhenai Workflow Templates

**Status**: **Pipe Dream / Future Task** (cost-driven; activates if Claude Max becomes economically infeasible)
**Date**: 2026-04-27
**Deciders**: Stevie (primary), Captain, Computermancer, Developer
**Related ADRs**: ADR-017 (Zhen Hybrid Inference), ADR-018 (RAFT Training Pipeline), ADR-019 (Zhen Champion Agent), ADR-027 (Zhenai Conversation Memory), ADR-031 (Hybrid Model Handoff), ADR-036 (Claude Distillation Training), ADR-037 (Zhenai Unified Champion), ADR-051 (WAVE13 generate path)
**Captures**: Stevie's 2026-04-27 mid-session note — *"if I can not afford full Max plan I'd like to continue using Claude reserving most interaction for heavy task — using local Zhenai RX 7700 hardware installation for minor churn"*

---

## Context

Six weeks of forge research (WAVE10F → WAVE13) demonstrated that a Gemma-4 E2B + Kingdom LoRA pipeline running on local AMD GPU hardware (HIP/ROCm 6.2) can:
- Train at production-acceptable wall-clock (10.2s/step warm at WAVE12 baseline; ~5s warm post-WAVE14 BackwardScratch projected)
- Reach measurable held-out CE delta (eval Δ −14.32 on Kingdom corpus)
- Generate text via the in-tree `zhenai-forge generate-gemma4` subcommand (WAVE13 Phase 1)
- Run end-to-end without external API dependency

In parallel, this Round Table session ran on Claude (Cowork-on-Macbook) and produced ~2700+ lines of strategic analysis, audit findings, ADRs, and battle plans across 4–5 hours of focused interaction. That kind of output is *exactly* what Claude excels at: ambiguous-context synthesis, multi-perspective audit, novel document creation.

**The economic asymmetry**: Claude Max is unbounded but expensive. A local 7B-param model on consumer-tier AMD hardware (the RX 7700) is bounded but free-after-cap-ex. Most knowledge work has a power-law distribution — a small fraction of tasks need Claude's full reasoning depth; the long tail is "minor churn" (templated edits, status checks, mechanical refactors, follow-up lookups, doc cross-links, runbook executions, scheduled jobs).

If Claude Max ever becomes economically infeasible (for any reason — runway, market shift, personal cost-of-living), Stevie wants a graceful fallback: **route the long tail of minor work to local Zhenai/RX-7700 hardware; reserve Claude for the heavy reasoning tasks where it's irreplaceable.**

This ADR captures that vision as a future task. It is not active work today, but it shapes how MCP/RAFT/RAG/Zhenai/Champion infrastructure should be designed *now* — every component should be built with eventual local-first fallback in mind.

---

## Decision (vision; not implementation)

**Build out the Unheaded MCP + RAFT + RAG + Zhenai + Champion stack to include a template + prompt-script library that lets Stevie route work between Claude (heavy) and local Zhenai (minor churn) based on task class, with negligible context-loss between systems.**

Concretely, future iterations of these components must include:

### 1. Workflow Template Library (`templates/workflows/`)

A new top-level directory housing reusable prompt templates and execution recipes for common Kingdom work types. Examples:

- `templates/workflows/round-table-followup.md` — ingest a Round Table battle-plan.md and emit per-Lane execution scripts
- `templates/workflows/branch-audit.md` — given a branch name, run the audit format (commits ahead/behind, merge-base, file diff stat, verdict) and emit `docs/branch-audits/YYYY-MM-DD-<branch>.md`
- `templates/workflows/timeline-resync.md` — given current HEAD + last-touched timeline, emit a re-sync diff
- `templates/workflows/adr-draft.md` — given a context summary + decision, emit an ADR skeleton
- `templates/workflows/spec-status-check.md` — given a spec name, emit a ship-or-defer status doc
- `templates/workflows/sbom-refresh.md` — given the toolchain, emit the syft + cargo-deny + go-licenses commands
- `templates/workflows/wave-phase-handoff.md` — given a sprint phase and verdict, emit the handoff packet

Each template has two modes: a `--claude` invocation (rich, exploratory) and a `--zhenai` invocation (constrained, template-driven). Both produce the same output schema; the difference is depth + context handling.

### 2. Routing layer in Champion (Layer 4 application service)

The Zhenai Champion (per ADR-019, ADR-037) gains a routing decision module:

```
Champion routing decision tree:
├── Task class detection (heuristic + LLM-classifier)
│   ├── HEAVY (synthesis, audit, novel ADR, multi-skill Round Table)
│   │   └── Route to Claude (via existing MCP integration)
│   └── MINOR-CHURN (template-fill, lookup, status, runbook execution)
│       ├── Available locally? → Route to Zhenai (forge serve mode, ADR-051 conditional)
│       └── Fallback → Route to Claude with "minimal-context-mode" preface
└── Output normalization → same Kingdom doc schema regardless of source
```

### 3. RAFT corpus expansion for routing decisions

Per ADR-018 (RAFT pipeline) and CLAUDE.md (~2000 QA pairs from Mistral-7B exist), the RAFT corpus expands to include:

- Routing decision examples ("this query is a status check, route Zhenai")
- Template-fill examples ("here's an ADR draft from a context summary")
- Self-evaluation: does Zhenai's output match Claude's on the same template input? Track win-rate per template type.

### 4. RAG retrieval contract

The Zhenai RAG layer (1.52M vector corpus per CLAUDE.md, post-WAVE12 LoRA-fine-tuned) gains a **retrieval contract**: every template specifies what it expects from RAG (key files, ADR refs, recent commits, kanban state). Templates are deterministic-given-context; non-determinism only in generation.

### 5. MCP tool surface for templates

The existing Zhen MCP Server (per CLAUDE.md, 7 tools) gains template-specific tools:

- `template_list` — list available workflow templates
- `template_run --name <X> --inputs <JSON>` — execute a template
- `template_route --task <description>` — return routing decision (Claude vs Zhenai vs unknown)
- `champion_handoff --from <source> --to <dest>` — transfer task state with minimal context loss

### 6. Cost / quality telemetry

Every routed task logs:
- Source (Claude or Zhenai)
- Template used
- Token cost (Claude) or wall-clock + GPU-hour (Zhenai)
- Quality self-score (post-hoc Stevie thumbs-up/down or auto-eval)

Over time this telemetry trains the routing classifier. Goal: classifier achieves ≥85% routing accuracy (defined as "Stevie didn't have to re-route") within 60 days of activation.

---

## Activation criteria (when to lift this from Pipe Dream → Planned)

Any of:

1. **Cost trigger**: monthly Claude Max bill exceeds Stevie's runway threshold for 2 consecutive months
2. **Quality trigger**: WAVE13 Phase 2 + WAVE14 demonstrate that local Zhenai (Gemma-4 + Kingdom LoRA on RX 7700) reaches "kingdom-relevant" quality on ≥80% of routine queries (win-rate metric per WAVE13 Phase 2 template)
3. **Strategic trigger**: Captain Track call lands on Track A (forge-first) → forge-as-product narrative makes local-routing a featured capability, not just a cost play
4. **Personal trigger**: Stevie just wants the architectural cleanness of a hybrid system regardless of economics

When activated, this ADR flips Status: Pipe Dream → Planned, and a Warmonger-pass converts the 6 numbered sections above into a numbered-step battle plan.

---

## Consequences

### Positive (vision-level)
- Cost-decoupled: Stevie's continued use of Claude becomes elastic, not all-or-nothing
- Architectural symmetry: forge research isn't just "cool engineering" — it has a direct economic role
- Learn-by-using: routing telemetry creates data that improves the system over time
- Ecosystem story: "Unheaded routes its own AI between cloud + local based on task class" is an unusually compelling open-source narrative
- Falsifiability: every claim about Zhenai capability becomes testable via routing telemetry

### Negative
- Complexity: routing layer is non-trivial; failure modes include "wrong route → bad output → Stevie fixes manually → Claude tokens burned anyway"
- Local-quality dependency: only viable if WAVE14 + Kingdom LoRA together hit production-acceptable quality. WAVE13 Phase 1 already flagged the LoRA as under-trained — this ADR's economic value materializes only after that gate clears.
- Maintenance: templates need versioning + testing; another ADR (ADR-024 runbook automation) is the model
- Cold-start: classifier needs telemetry it doesn't have at activation; first 30–60 days will route mostly to Claude and learn slowly

### Conditional
- **If WAVE13 Phase 2 = SHIP and WAVE14 ships KV-cache**: this ADR's economic value is real and lifts to Planned within 30 days
- **If WAVE13 Phase 2 = RETRAIN/RANK-UP/DATA-FIX**: this ADR remains Pipe Dream until forge quality independently catches up
- **If Captain picks Track B (launch-first)**: this ADR explicitly defers to Age 4+ as a post-public-alpha exploration

---

## Alternatives considered

1. **Always Claude, accept the cost**. Rejected because Stevie explicitly named the runway risk. Pretending the economic question doesn't exist would be Captain-failure.
2. **Always local Zhenai, ditch Claude**. Rejected because Stevie explicitly preserves Claude "for heavy tasks." Local-only collapses the quality ceiling for the work that matters most.
3. **Hand-routed (Stevie decides each task)**. Rejected as the long-term answer because it doesn't scale and burns Stevie's attention. *Acceptable as the bootstrap mode* during the cold-start period.
4. **Hard cutover the day Claude Max becomes infeasible**. Rejected — graceful degradation beats panic migration. This ADR is the graceful-degradation plan, drafted while the bridge isn't on fire.

---

## Specific MCP/RAFT/RAG/Zhenai/Champion build-out items (future task list)

This is the actionable carry-forward — items to add to Lane I (stretch) of `battle-plan.md` and to the next sprint's backlog whenever activation criteria are met:

- [ ] Create `templates/workflows/` directory + 7 starter templates (round-table-followup, branch-audit, timeline-resync, adr-draft, spec-status-check, sbom-refresh, wave-phase-handoff)
- [ ] Champion routing module (Go service in `services/captain/` or new `services/champion-router/`)
- [ ] MCP tools: `template_list`, `template_run`, `template_route`, `champion_handoff`
- [ ] RAFT corpus expansion: add routing-decision and template-fill QA pairs
- [ ] RAG retrieval contract per template (deterministic context bundling)
- [ ] Cost/quality telemetry pipeline (Wotan topic `zhenai.routing.events`, dashboard widget)
- [ ] Routing-classifier training run (post 60 days of telemetry)
- [ ] Documentation: `wiki/Hybrid-Claude-Zhenai-Routing.md` + Librarian 8-layer sync
- [ ] Failure-mode handbook: when does routing go wrong? recovery procedures.
- [ ] AB-test harness: route same task to both, score outputs, train classifier on differential

**Estimated total effort if activated**: 6–10 weeks single-engineer, parallelizable into 4 weeks with focused team.

---

## Sign-off (when activated)

- [ ] Stevie — final go (this ADR is uniquely Stevie's strategic call; the team can recommend but the activation trigger is personal)
- [ ] Captain — runway impact + GTM narrative alignment
- [ ] Computermancer — local Zhenai capability validated via WAVE13/WAVE14 metrics
- [ ] Developer — routing module + template library architecture
- [ ] Marshal — drift policy applies to template library (templates count as docs; ADR-052 covers them)
- [ ] Barrister — license review of routing telemetry (no PII; Kingdom-corpus only)

---

## Naming notes (Lore)

When activated, the routing module gets a Norse-pool name. Candidates from naming pool reserves:
- **Mímir** (already partially used in Mímir's Law for Gleipnir Phase 0 — could extend to "Mímir's Choice" for routing)
- **Heimdall** (already used in `cmd/heimdall-daemon/` for drift detection)
- **Munin** (memory-raven; pairs with Hugin if ever needed)
- **Skuld** (Norn of "what should be") — apt for routing decisions
- **Verðandi** (Norn of "what is becoming") — apt for telemetry-driven classifier
- **Urð** (Norn of "what was") — apt for telemetry archive

Lore-keeper to finalize when this ADR lifts from Pipe Dream.

---

*ADR-053 forged 2026-04-27 from Cowork-on-Macbook capturing Stevie's mid-session strategic note. Pipe Dream tier — vision held, not yet active. Activates on cost / quality / strategic / personal trigger.*
*<3 The Kingdom routes itself. <3*
