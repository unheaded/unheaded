# S36 Claude Code CLI Prompts — Multi-Agent Execution

## How to Use

You need **up to 6 terminals** running `claude` simultaneously. Each gets a different prompt below. The dependency order is:

```
Coordinator (Phase 0) → Agent A (Phase 1) → Agent B (Phase 2) + Agent D (Phase 4) + Agent E (Phase 5)
                                              Agent B done → Agent C (Phase 3)
                                              ALL done → Coordinator (Phase 6)
```

**Minimum viable**: 2 terminals (Coordinator does 0+6, one agent does 1→2→3→4→5 sequentially).
**Recommended**: 4 terminals (Coordinator, Agent A→B→C sequential, Agent D parallel, Agent E parallel).
**Maximum parallelism**: 6 terminals as shown above.

---

## Terminal 1 — Coordinator (Phase 0: Foundation)

Copy and paste:

```
You are the Coordinator executing Phase 0 of the S36 Four Pillars Battle Plan for the Unheaded project (~260K production LOC, Go + Rust + eBPF).

Read these files FIRST, in order:
1. CLAUDE.md — your bible
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md — the FULL battle plan (read ALL of it to understand the full picture, but execute ONLY Phase 0)
3. battle-plan.md — strategic context

Execute ONLY Phase 0 (Steps 1-20): Foundation — verify environment, commit outstanding files, establish clean baseline, run port audit.

Rules:
- Commit every 4 steps. Conventional commits: type(scope): description
- If build or tests fail, fix before proceeding
- Record port audit baseline for Phase 6 verification
- When Phase 0 EXIT GATE passes, report "PHASE 0 COMPLETE — Agents A and E may begin"

Go.
```

---

## Terminal 2 — Agent A (Phase 1: Port Authority)

Wait until Coordinator reports Phase 0 complete, then paste:

```
You are Agent A executing Phase 1 of the S36 Four Pillars Battle Plan for the Unheaded project (~260K production LOC, Go + Rust + eBPF).

Read these files FIRST, in order:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (read Phase 1 carefully, skim rest for context)
3. pkg/config/config.go (understand existing config patterns)

Execute ONLY Phase 1 (Steps 21-105): Port Authority — migrate ALL services from legacy ports (8000-9100) to Doom Range (16666-26666).

Key deliverables:
- pkg/ports/ports.go — single source of truth for all port constants
- configs/port-registry.yaml — YAML for container configs
- Every service in cmd/ and services/ updated to new ports
- docker-compose.yml, NixOS configs, container configs updated
- ALL tests updated to expect new ports
- ZERO old port references in production code

Rules:
- TDD: tests first. Race detection: -race on all Go tests. All inputs hostile.
- Commit every 4 steps. Conventional commits: feat(ports): description
- Stuck protocol: skip after 3x time or 2 failed attempts. Commit before skip.
- DO NOT touch doom/, ebpf/, crates/, docs/protocol/, licensing files
- DO NOT push to remote

When Phase 1 EXIT GATE passes (Step 105), report "PHASE 1 COMPLETE — Agents B, D, and E may begin"

Go.
```

After Agent A completes, reuse this terminal for Agent B:

---

## Terminal 2 (continued) — Agent B (Phase 2: gRPC-First Transport)

```
You are Agent B executing Phase 2 of the S36 Four Pillars Battle Plan for the Unheaded project.

Read these files FIRST:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (Phase 2 in detail)
3. pkg/ports/ports.go (just created in Phase 1)
4. services/wotan/cmd/wotan/main.go (the nerve center)

Execute ONLY Phase 2 (Steps 106-175): gRPC-First Transport.

Key deliverables:
- pkg/transport/ — transport.go, grpc.go, http.go, cascade.go, health.go, flags.go
- Every service wired to use transport.Connect() for Wotan communication
- Every service has dual health checks (gRPC + HTTP)
- DEGRADED state when gRPC down but HTTP up
- Env var + flag + config file override for transport selection

Rules:
- TDD: write transport tests BEFORE implementations
- Commit every 4 steps. Conventional commits: feat(transport): description
- Stuck protocol: skip after 3x time or 2 failed attempts.
- DO NOT modify port assignments (Phase 1 already set those)

When Phase 2 EXIT GATE passes (Step 175), report "PHASE 2 COMPLETE — Agent C may begin"

Go.
```

After Agent B completes, reuse for Agent C:

---

## Terminal 2 (continued) — Agent C (Phase 3: Log Aggregation)

```
You are Agent C executing Phase 3 of the S36 Four Pillars Battle Plan for the Unheaded project.

Read these files FIRST:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (Phase 3 in detail)
3. pkg/transport/transport.go (created in Phase 2 — you'll use this)
4. cmd/dashboard-backend/main.go (you'll add log endpoints here)

Execute ONLY Phase 3 (Steps 176-228): Log Aggregation.

Key deliverables:
- pkg/logagg/ — publisher.go, ringbuffer.go, query.go, subscriber.go, setup.go
- Dashboard endpoints: GET /api/v1/logs + WebSocket /ws/logs
- dashboard/logs.html + dashboard/js/log-viewer.js (UI)
- Every service wired with logagg.SetupServiceLogger() hook
- Ring buffer retains 10K entries per service

Rules:
- TDD: write logagg tests BEFORE implementations
- Commit every 4 steps. Conventional commits: feat(logs): description
- Zerolog hook MUST NOT block service operation — async publish

When Phase 3 EXIT GATE passes (Step 228), report "PHASE 3 COMPLETE"

Go.
```

---

## Terminal 3 — Agent D (Phase 4: Service Discovery) — PARALLEL with Terminal 2

Start this AFTER Phase 1 completes. Runs parallel with Phases 2-3.

```
You are Agent D executing Phase 4 of the S36 Four Pillars Battle Plan for the Unheaded project.

Read these files FIRST:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (Phase 4 in detail)
3. pkg/discovery/ (existing code — understand before modifying)
4. pkg/ports/ports.go (created in Phase 1)

Execute ONLY Phase 4 (Steps 229-272): Service Discovery.

Key deliverables:
- pkg/discovery/ extended: scanner.go, registrar.go, resolver.go, static.go, setup.go
- configs/services.yaml — static fallback
- Every service wired with discovery.SetupServiceDiscovery()
- ZERO hardcoded 10.10.10 IPs in production code
- CLI uses resolver instead of hardcoded endpoints

IMPORTANT: You are running PARALLEL with Agents B and C. Do NOT modify:
- pkg/transport/ (Agent B owns this)
- pkg/logagg/ (Agent C owns this)
- dashboard/ (Agent C owns this)
If you need transport.Connection, import it but don't modify the package.

Rules:
- TDD: tests first. Commit every 4 steps: feat(discovery): description
- Registration MUST be fire-and-forget — services start even if registration fails
- Kill ALL hardcoded 10.10.10 IPs in production code

When Phase 4 EXIT GATE passes (Step 272), report "PHASE 4 COMPLETE"

Go.
```

---

## Terminal 4 — Agent E (Phase 5: Documentation) — PARALLEL with all

Start this AFTER Phase 1 completes. Runs parallel with everyone.

```
You are Agent E executing Phase 5 of the S36 Four Pillars Battle Plan for the Unheaded project.

Read these files FIRST:
1. CLAUDE.md
2. docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (Phase 5 in detail)
3. battle-plan.md, README.md, QUICKSTART.md, references/timeline.md (files you'll update)
4. wiki/ directory listing

Execute ONLY Phase 5 (Steps 273-290): Documentation — update all 8 documentation layers.

Key deliverables:
- CLAUDE.md updated with all four pillars
- README.md and QUICKSTART.md updated with Doom Range ports
- 4 new wiki pages: Port-Registry.md, Transport-Cascade.md, Log-Aggregation.md, Service-Discovery.md
- wiki Home, Sidebar, Architecture updated
- battle-plan.md and timeline.md updated
- Zero stale port references in any documentation

IMPORTANT: You are running PARALLEL with code agents. Do NOT modify:
- Any .go files
- docker-compose.yml, Makefile, container configs
Only modify .md files, wiki pages, and documentation.

Commit every 4 steps: docs(s36): description

When Phase 5 EXIT GATE passes (Step 290), report "PHASE 5 COMPLETE"

Go.
```

---

## Terminal 1 (again) — Coordinator (Phase 6: Integration Verification)

Wait until ALL agents report complete, then paste:

```
You are the Coordinator executing Phase 6 (final) of the S36 Four Pillars Battle Plan for the Unheaded project.

ALL previous phases are complete. Your job is final integration verification.

Read: docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md (Phase 6 + Appendix B Quick Reference)

Execute Phase 6 (Steps 291-310): full build, full test suite, static analysis, port audit, IP audit, package inventory verification.

Key checks:
1. go build ./... — MUST pass
2. go test -race -count=1 ./... — MUST pass
3. go vet ./... — MUST pass
4. Zero old port references in production .go files
5. Zero hardcoded 10.10.10 IPs in production .go files
6. pkg/ports/, pkg/transport/, pkg/logagg/, pkg/discovery/ all exist with tests
7. dashboard/logs.html exists
8. configs/ has port-registry.yaml and services.yaml

When Phase 6 EXIT GATE passes (Step 310), report:
"S36 COMPLETE — THE FOUR PILLARS STAND"
Include: total commits, files changed, test count, any stuck steps.

Go.
```
