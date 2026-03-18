# S39 Claude Code CLI Prompt — Full Industrialization → v0.1.0-alpha

## Launch Command

```bash
cd ~/tmp/unheaded && claude --dangerously-skip-permissions
```

Then paste:

```
You are executing Sprint S39 of the Unheaded project — THE FULL INDUSTRIALIZATION. This is the final sprint to ship v0.1.0-alpha. (~260K production LOC, Go 1.24 + Rust + eBPF, 20+ service binaries). S36 (Four Pillars), S37 (LICENSE+SBOM), and S38 (eBPF Production) are ALL complete.

## MANDATORY: Read These Files First

1. `CLAUDE.md` — Agent guide. Sacred Laws. Architecture. YOUR BIBLE.
2. `docs/battle-plans/S39-INDUSTRIALIZATION-BATTLE-PLAN.md` — THE BATTLE PLAN. 320+ steps, 10 phases. Read the ENTIRE file.
3. `battle-plan.md` — Living battle plan with all strategic decisions.
4. `references/timeline.md` — Living roadmap.
5. `pkg/auth/` — existing auth skeleton (JWT + API key, needs wiring)
6. `pkg/transport/` — gRPC-first transport (from S36)
7. `pkg/discovery/` — three-layer service discovery (from S36)

## YOUR MISSION

Execute ALL 10 phases of the S39 Industrialization Battle Plan:

**Phase 0**: Environment verification + validate S36/S37/S38 state
**Phase 1**: Auth hardening — wire JWT + API key to ALL 20+ services, RBAC, deny by default
**Phase 2**: mTLS Service Mesh — cert generation, rotation, zero plaintext between services
**Phase 3**: Lich Campaigns D1-D6 — offensive security tests against the full stack
**Phase 4**: Lich Remediation — fix everything the Lich found
**Phase 5**: Wotan Hardening — ack/nack, retry, dead letter queue, message ordering
**Phase 6**: Container Security — image scanning, seccomp, AppArmor, read-only rootfs
**Phase 7**: E2E Integration Test Suite — browser → gateway → services → eBPF → Wotan → dashboard
**Phase 8**: Deployment Pipeline — `make deploy` actually works, health verification, rollback
**Phase 9**: Documentation Final Pass — all docs updated, wiki complete, API reference
**Phase 10**: Alpha Ship Gate — v0.1.0-alpha tag, compliance snapshot, SBOM refresh, release notes

## EXECUTION RULES

- YOU ARE RUNNING IN AUTONOMOUS MODE. Proceed without pausing. This is the big one.
- Auto-commit every 5 steps (epic scale). Conventional commits.
- Follow the battle plan STEP BY STEP.
- Sacred Laws are NON-NEGOTIABLE:
  1. ZERO customer data access — architectural isolation at every layer
  2. Security first — all inputs hostile, defensive coding
  3. TDD — tests before implementation, red-green-refactor
  4. Race detection — `go test -race` on EVERYTHING
  5. Interchangeable backends — no proprietary lock-in
- Stuck protocol: skip after 3x time or 2 failed attempts. Commit before skip.
- Security findings from Lich campaigns (Phase 3) MUST be remediated in Phase 4 before proceeding to Phase 5+.

## MULTI-AGENT NOTE

If running with multiple agents, the dependency graph is:
- Phases 0-2: sequential (auth → mTLS builds on auth)
- Phases 3-4: sequential (Lich → remediation)
- Phase 5: parallel with 3-4 (Wotan hardening is independent)
- Phase 6: parallel with 3-5 (container security is independent)
- Phase 7: requires 1-6 complete (E2E tests need everything)
- Phases 8-10: sequential after 7

## WHAT NOT TO DO

- DO NOT modify protocol specs (docs/protocol/)
- DO NOT change port assignments (S36)
- DO NOT change licensing (S37)
- DO NOT modify eBPF programs (S38)
- DO NOT push to remote until Phase 10 explicitly says to
- DO NOT ship with ANY Lich finding unresolved
- DO NOT tag v0.1.0-alpha until ALL exit gates pass

When Phase 10 EXIT GATE passes, report: "S39 COMPLETE — v0.1.0-alpha SHIPPED" with: tag name, commit count, test count, security findings resolved, SBOM status.

Go.
```
