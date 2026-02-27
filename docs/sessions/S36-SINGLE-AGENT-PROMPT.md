# S36 Single Agent Claude Code CLI Prompt

## Launch Command

```bash
cd ~/tmp/unheaded && claude --dangerously-skip-permissions
```

The `--dangerously-skip-permissions` flag enables auto-accept for all tool calls (file writes, bash commands, git commits). No human confirmation prompts — the agent runs autonomously.

Then paste the prompt below:

```
You are executing Sprint S36 of the Unheaded project — a configuration management automation platform (~260K production LOC, ~464K with tests). Go 1.24 + Rust + eBPF. You are picking up where S35 left off. Working tree is CLEAN. Build and tests PASS.

## MANDATORY: Read These Files First (In Order)

1. `CLAUDE.md` — Agent guide. Architecture, 6-layer stack, Sacred Laws. THIS IS YOUR BIBLE.
2. `docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md` — THE BATTLE PLAN. 310 numbered steps across 7 phases. This is your execution script. Read the ENTIRE file before writing any code.
3. `battle-plan.md` — Living battle plan with S34 Round Table decisions and S35 strategic review.
4. `references/timeline.md` — Living roadmap.

## YOUR MISSION: Execute ALL 7 Phases of the S36 Battle Plan

The battle plan at `docs/battle-plans/S36-FOUR-PILLARS-BATTLE-PLAN.md` contains 310 numbered steps organized into 7 phases. Execute them IN ORDER:

**Phase 0** (Steps 1-20): Foundation — verify environment, commit outstanding files, port audit baseline
**Phase 1** (Steps 21-105): Port Authority — migrate ALL services to Doom Range (16666-26666), create pkg/ports/
**Phase 2** (Steps 106-175): gRPC-First Transport — create pkg/transport/, flip all services to gRPC default, dual health checks
**Phase 3** (Steps 176-228): Log Aggregation — create pkg/logagg/, dashboard log viewer, wire all services
**Phase 4** (Steps 229-272): Service Discovery — extend pkg/discovery/, three-layer resolution, kill ALL hardcoded 10.10.10 IPs
**Phase 5** (Steps 273-290): Documentation — update all 8 doc layers, create 4 wiki pages
**Phase 6** (Steps 291-310): Integration Verification — full build, full test, port audit, IP audit, final commit

Every step in the battle plan has a type tag:
- [B] = Bash command (run it)
- [V] = Verification (MUST pass before next step)
- [D] = Debug (only if prior [V] fails)
- [W] = Write file
- [R] = Read file
- [C] = Commit checkpoint (git commit)
- [S] = Sudo required

## EXECUTION RULES

- YOU ARE RUNNING IN AUTONOMOUS MODE. No human will confirm your actions. Proceed through every step without pausing for approval. Do NOT ask "should I continue?" — just continue.
- Auto-commit: When you hit a [C] checkpoint, commit immediately. Do not ask permission.
- Auto-proceed: After every verification [V] passes, immediately proceed to the next step. No pauses.
- If a [V] fails, go to the [D] debug step immediately. Fix it, re-verify, move on.
- FOLLOW THE BATTLE PLAN STEP BY STEP. Every step is numbered. Execute in order.
- TDD: Write tests FIRST, then implementation (red-green-refactor)
- Race detection: ALL Go tests with `-race`
- Security: ALL inputs hostile. Validate everything.
- Commit every 4 steps. Conventional commits: `type(scope): description`
- NEVER skip a [V] verification step. If it fails, execute the [D] debug step.
- Stuck protocol: If stuck on a step for >3x the estimated time or after 2 debug attempts, SKIP it:
  1. Log what failed and why
  2. Mark step as [STUCK] in your notes
  3. Commit current state
  4. Skip to next non-dependent step
  5. When all non-blocked work is done, report a STUCK REPORT
- Every Phase has an EXIT GATE. Do NOT start the next phase until the EXIT GATE passes.

## WHAT NOT TO DO

- DO NOT modify protocol specs (docs/protocol/)
- DO NOT touch doom/ directory
- DO NOT change licensing files
- DO NOT push to remote (18+ commits ahead, will push manually)
- DO NOT gold-plate. Minimum viable implementations that unblock the next phase.
- DO NOT use proprietary lock-in for any backend choice
- DO NOT skip EXIT GATES

## START

Read the 4 files listed above. Then execute Phase 0, Step 1. Follow every step in the battle plan sequentially through Step 310.

When Phase 6 EXIT GATE passes, report: "S36 COMPLETE — THE FOUR PILLARS STAND" with total commits, files changed, and any stuck steps.

Go.
```
