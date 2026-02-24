# S37→S38→S39 Overnight Sprint — Claude Code CLI Prompt

## Launch Command

```bash
cd ~/tmp/unheaded && claude --dangerously-skip-permissions
```

Then paste everything below:

```
You are executing an OVERNIGHT SPRINT for the Unheaded project — starting with S37, then auto-chaining to S38 and S39. (~260K production LOC, Go 1.24 + Rust + eBPF). S36 Four Pillars are COMPLETE. Working tree CLEAN. Build and tests PASS.

## YOUR MISSION: Execute S37, then S38, then S39 — in sequence, without stopping.

This is an autonomous overnight run. You will execute three battle plans back-to-back:

1. **S37**: LICENSE + SBOM + Documentation Cleanup
2. **S38**: eBPF Production (if bare metal available, else SKIP)
3. **S39**: Full Industrialization → v0.1.0-alpha

## STEP 1: Execute S37

Read these files FIRST:
1. `CLAUDE.md` — Agent guide. Sacred Laws. YOUR BIBLE.
2. `docs/battle-plans/S37-LICENSE-SBOM-BATTLE-PLAN.md` — THE BATTLE PLAN. Read ALL of it including the ADDENDUM at the bottom.
3. `LICENSES/` directory — current license state
4. `references/timeline.md` — where we are

Execute ALL phases of S37:
- Phase 0: Foundation
- Phase 1: BSL 1.1 License Drafting
- Phase 2: SBOM Scanning (ScanCode + FOSSology + ORT)
- Phase 3: Doom Fork (id-Software/DOOM with sound)
- Phase 4: Pre-Public Audit (secrets scan, comment audit)
- Phase 5: Final Verification

**THEN execute the ADDENDUM** (documentation cleanup):
- A1: Rename "product" → "application" in all docs
- A2: Rename "customer" → "user" in all docs
- A3: Remove all champagne/dogfooding references
- A4: Add foundational RFC references (791, 792, 793, 768) to protocol specs
- A5: Document Linux ephemeral port range tuning (27000-65000)
- A6: Commit addendum changes

When S37 is complete, report "S37 COMPLETE" and immediately proceed to Step 2.

## STEP 2: Attempt S38

Read `docs/battle-plans/S38-EBPF-PRODUCTION-BATTLE-PLAN.md`.

**First, check if bare metal BPF is available:**
```bash
uname -r && cat /boot/config-$(uname -r) 2>/dev/null | grep -E "CONFIG_BPF=|CONFIG_XDP" | head -5
```

- If BPF/XDP support is confirmed → Execute ALL phases of S38
- If NOT available (VM, container, or missing kernel config) → Report "S38 BLOCKED — requires bare metal Linux with BPF/XDP. Skipping to S39." and proceed to Step 3

## STEP 3: Execute S39

Read `docs/battle-plans/S39-INDUSTRIALIZATION-BATTLE-PLAN.md`.

Execute ALL phases of S39:
- Phase 0: Environment verification
- Phase 1: Auth hardening (JWT + API key on ALL services)
- Phase 2: mTLS Service Mesh
- Phase 3: Lich Campaigns D1-D6 (offensive security)
- Phase 4: Lich Remediation
- Phase 5: Wotan Hardening
- Phase 6: Container Security
- Phase 7: E2E Integration Test Suite
- Phase 8: Deployment Pipeline
- Phase 9: Documentation Final Pass
- Phase 10: Alpha Ship Gate → v0.1.0-alpha tag

## EXECUTION RULES (ALL SPRINTS)

- YOU ARE RUNNING IN AUTONOMOUS MODE. No human will confirm your actions. Proceed without pausing for approval. Do NOT ask "should I continue?" — just continue.
- Auto-commit at every [C] checkpoint. Do not ask permission.
- Auto-proceed: After every verification [V] passes, immediately proceed to the next step.
- Follow each battle plan STEP BY STEP. Every step is numbered. Execute in order.
- TDD: Write tests FIRST, then implementation (red-green-refactor)
- Race detection: ALL Go tests with `-race`
- Security: ALL inputs hostile. Validate everything.
- Commit every 4-5 steps. Conventional commits: `type(scope): description`
- Stuck protocol: If stuck >3x estimated time or after 2 debug attempts, SKIP it:
  1. Log what failed and why
  2. Commit current state
  3. Skip to next non-dependent step
  4. Continue forward

## WHAT NOT TO DO (ALL SPRINTS)

- DO NOT push to remote (will push manually)
- DO NOT modify protocol specs unless battle plan explicitly says to
- DO NOT gold-plate — minimum viable implementations
- DO NOT skip EXIT GATES
- DO NOT stop between sprints — chain S37 → S38 → S39 automatically

## FINAL REPORT

When S39 Phase 10 EXIT GATE passes (or when you've exhausted all non-blocked forward progress across all three sprints), produce a FINAL OVERNIGHT REPORT:

OVERNIGHT SPRINT REPORT
=======================
S37: [COMPLETE/PARTIAL] — [deliverables]
S38: [COMPLETE/BLOCKED/PARTIAL] — [reason if blocked]
S39: [COMPLETE/PARTIAL] — [deliverables]
Total commits: [N]
Total files changed: [N]
Stuck steps: [list or "none"]
Tests passing: [yes/no]
Build passing: [yes/no]

Go. Start with S37. Do not stop until S39 is complete or all forward progress is exhausted.
```
