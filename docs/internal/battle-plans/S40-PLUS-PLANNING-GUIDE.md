# S40+ Planning Guide — What Comes After Alpha

**Date**: 2026-02-24
**Context**: After S36-S39, we'll have v0.1.0-alpha. This document captures the roadmap beyond.

---

## The Battle Plan Formula That Works

S36 proved it: **detailed stepped plans with exact bash commands, verification gates, and commit checkpoints** let agents rip through 100+ steps in 15-20 minutes. The formula:

1. **Intelligence gathering first** — grep the codebase, read handoffs, understand current state
2. **Warmonger-format plans** — every step numbered, tagged [B][V][D][W][R][C], with time estimates
3. **Exit gates per phase** — hard stops that prevent cascading failures
4. **Auto-chain prompts** — S37→S38→S39 style, agent never stops
5. **`--dangerously-skip-permissions`** — fully autonomous execution
6. **Commit every 4-5 steps** — never lose more than 15 minutes of work
7. **Stuck protocol** — skip after 3x time, commit before skip, log everything

### Template for New Battle Plans (v2)

> **Template version**: v2 (2026-03-19). Canonical source: `references/WARMONGER-BATTLE-PLAN-TEMPLATE.md`
> v2 adds: VARIABLES block, PREFLIGHT HYPOTHESES, KNOWN FAILURES BASELINE, PARALLEL MATRIX, per-phase Definition of Done, SECURITY REVIEW GATE, COMPLIANCE GATE, POST-EXECUTION phase, resilient tool installation, `[DECIDE]`/`[ESCALATE]` tags (replacing `[BLOCKED]` for decisions).

```markdown
# S[N] [TITLE] BATTLE PLAN — [X] Phases, [Y]+ Steps

**Date**: YYYY-MM-DD
**Sprint**: S[N] — [description]
**Prerequisite**: S[N-1] complete. Build passes. Tests pass.
**Target**: [what "done" looks like]
**Commit Cadence**: Every [4-5] steps
**Stuck Protocol**: Skip after 3x time or 2 failed debug attempts

### Multi-Agent Time Estimates
| Mode | Agents | Duration | Critical Path |
|------|--------|----------|---------------|
| Solo | 1 | [X-Y hours] | [chain] |
| Pair | 2 | [X-Y hours] | [chain] |
| Swarm | 4 | [X-Y hours] | [chain] |

## VARIABLES
$PROJECT_ROOT, $SPRINT_ID, $SPEC_DIR, $AUDIT_DOC — no hardcoded paths.

## LEGEND
[B][V][D][W][R][S][P][P:N][SEQ][C][ENV][BUILD][TEST][CODE][DESIGN]
[STUCK][BLOCKED][DECIDE][ESCALATE][PREFLIGHT][REGEN][AUDIT-UPDATE]
[DOC-UPDATE][SECURITY][COMPLIANCE][VM-SCAN][BARE-METAL]

## PREFLIGHT HYPOTHESES
Verify every assumption before execution. Scientist lens.

## KNOWN FAILURES BASELINE
Record pre-existing test failures. NEW failures = regression.

## PARALLEL MATRIX
Dependency graph + phase assignment + critical path per agent mode.

## PHASE 0: INTELLIGENCE & PREFLIGHT (Steps 1-N)
...resolve $PROJECT_ROOT, verify hypotheses, establish baseline...

## PHASE 1-N: [WORK PHASES]
...every step numbered, every [B] has a [V], exit gates...
...every phase has Definition of Done (Micromanager gate)...
...[DECIDE] for autonomous decisions, [ESCALATE] for human-required...

## SECURITY REVIEW GATE
Trust boundary + listener + input + secret leak detection. BlackMage lens.

## COMPLIANCE GATE
New deps + license audit + SPDX headers + SBOM impact. Sentinel lens.

## POST-EXECUTION (mandatory final phase)
[REGEN] + [AUDIT-UPDATE] + [DOC-UPDATE] + baseline comparison + handoff.

## AUTO-CHAIN
When complete, read docs/battle-plans/S[N+1]-*.md and continue.
```

---

## S40+ Sprint Ideas (Priority Order)

### S40: Timeguru Expansion — Centralized Config Management
**Why now**: S36 exposed the hardcoded port problem. We solved it with `pkg/ports/` but need a broader solution.
**Scope**:
- Expand `configs/` into a full cascading config system
- Timeguru becomes the config server — ports, proxies, networking, all config variables
- YAML/JSON/TOML definitions organized by domain
- Our own take on existing IaC platforms (Ansible, Terraform, and others)
- Config rendering: generate docker-compose, NixOS configs, nginx configs from single source
**Estimated**: ~150 steps, 4-5 phases

### S41: Package Breakout — Monorepo → Multi-Repo
**Why now**: Cleaner CI, independent versioning, license boundary enforcement
**Scope**:
- Extract Wotan → `~/tmp/wotan/`
- Extract Sophia → `~/tmp/sophia/`
- Extract Kanban → `~/tmp/kanban/`
- Extract Dashboard → `~/tmp/dashboard/`
- Extract Doom → `~/tmp/doom/` (GPL boundary — critical)
- Update go.mod with replace directives for local dev
- CI per-repo
**Estimated**: ~100 steps, 3 phases

### S42: Cloudflare Research Sprint
**Why now**: They're FAANG-level eBPF and edge compute leaders
**Scope**:
- Audit all Cloudflare open source projects
- Assess: cf-terraforming, cloudflared, CFSSL, quiche, boring-crypto
- Architecture pattern analysis (how they use eBPF, Workers, etc.)
- Identify libraries/tools to adopt
- Write findings document
**Estimated**: ~60 steps, 2 phases (research + documentation)

### S43: Gorgonia LLM/RAG Integration
**Why now**: Muck's gaming PC can be repurposed as dev playground
**Scope**:
- Set up Gorgonia (https://github.com/gorgonia/gorgonia)
- Train on Unheaded wiki, docs, protocol specs
- Build RAG pipeline for infrastructure management assistance
- Deploy on gaming PC (dual-boot Linux)
- Demo: LLM-assisted infrastructure management
**Estimated**: ~200 steps, 6 phases (this is a full new subsystem)
**Hardware**: Modern gaming PC with dual-boot

### S44: Multi-Machine Kingdom Expansion
**Why now**: Needed for real-world deployment and demos
**Scope**:
- Multi-machine log aggregation (cross-machine log forwarding)
- Federated Wotan instances
- Machine identity in service discovery
- Cross-machine mTLS
- Demo: 2+ machines running Unheaded stacks
- Fireguard tunnel for public demo exposure
**Estimated**: ~250 steps, 8 phases

### S45: Compliance Dashboard
**Why now**: Enterprise readiness
**Scope**:
- Export to CSV
- Admin/group-based security feature enablement
- Full control matrix (NIST, GDPR, SOC2, HIPAA, PCI-DSS)
- Cross-framework overlap handling (enable MFA → auto-checks in all frameworks)
- Audit trail
**Estimated**: ~200 steps, 6 phases

### S46: Advanced Log Viewer
**Why now**: S36 built the foundation, now polish
**Scope**:
- Tail-f behavior (latest bottom, oldest top)
- Bottom 2 lines at 2x height with highlighting
- Filter by IP/port/response code/service/level
- Click to expand JSON fields
- Copy-to-clipboard
- Time range picker
- RDAP integration (whois replacement)
**Estimated**: ~80 steps, 3 phases

### S47: Demo Video + Public Launch Prep
**Why now**: v0.1.0-alpha is tagged, time to show the world
**Scope**:
- 5-minute Doom-over-IPv6 demo video
- eBPF packet tracing live demo
- Dashboard walkthrough
- README final polish for public
- `git push` to public repo
- Austin VC materials (Captain + Barrister session)
**Estimated**: ~60 steps, 3 phases

---

## Key Principles for S40+ Planning

1. **Each sprint gets its own battle plan** — never wing it past 10 steps
2. **Auto-chain where possible** — S37→S38→S39 proved this works overnight
3. **Intelligence gathering is not optional** — grep before you plan, read before you code
4. **Warmonger format is law** — numbered steps, tags, gates, commits, stuck protocol
5. **The battle plan IS the product** — if the plan is incomplete, execution will be incomplete
6. **Celebrate wins** — every sprint completion is a victory. Peace and love.

---

*The road to v1.0 is long. The road to v0.1.0-alpha ends tonight.*
*S40 and beyond: the Kingdom grows.*
