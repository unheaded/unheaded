# Agent Operating Procedure (AOP)
## The Canonical Workflow for Every Unheaded Session

**Version**: 1.0
**Created**: 2026-02-20 (Round Table S26)
**Owner**: Micromanager
**Approved By**: Round Table (All 9 Seats)

---

## Purpose

This document defines the mandatory start and end procedures for every agent session working on the Unheaded Kingdom. It ensures continuity between sessions, prevents drift during gaps, and maintains the institutional memory that makes multi-agent development possible.

**The Sacred Rule**: Every session inherits from the last. Every session hands off to the next. The chain never breaks.

---

## Session Start Protocol

### Step 1: Read Previous Handoff (MANDATORY)

```
Agent reads: docs/sessions/S<N-1>-*-handoff.md
```

This is the single most important action. The handoff contains:
- What was accomplished in the previous session
- What is pending / blocked
- Current state of all Unheaded layers
- Recommended next actions
- Relevant commit SHAs

**If the handoff is missing**: Check `git log --oneline -20` for recent activity and reconstruct context from commit messages.

### Step 2: Load Kingdom Context

```
Load skills: captain, micromanager, architect, developer, timeguru,
             calendar, busboy, lore, kingdom, blackmage, moatghost, rfceditor
Check: timeline.md for current Age/Epoch/milestone
Check: docs/adr/ for active Architecture Decision Records
```

### Step 3: Confirm Environment Health

```bash
# Verify toolchains
go version              # Need 1.24.0+
rustup show             # Need nightly for fuzzing
cargo --version         # eBPF compilation

# Verify codebase
go build ./...          # Must exit 0
go test -race ./...     # Must pass (currently 134/134)

# Verify git state
git status              # Clean working tree?
git log --oneline -5    # Recent commits match handoff?

# Set PATH for installed tools
export PATH=$PATH:$(go env GOPATH)/bin
```

**If running in Cowork VM** (no toolchains): Skip environment health, focus on documentation, planning, and file edits. Note this limitation in the handoff.

### Step 4: Sprint Begins

```
Confirm with Muck: Session goals and priority order
Set mode: Aggressive, parallel, multi-agent where safe
Begin execution
```

---

## Session End Protocol

### Step 1: Write Handoff Document (MANDATORY)

```
Create: docs/sessions/S<N>-<description>-handoff.md
```

The handoff MUST include:

1. **Session Summary**: What was done (bullet points with specifics)
2. **Commits Made**: Full list with SHAs and descriptions
3. **Files Changed**: Count and categories
4. **What's NOT Done**: Outstanding items with context
5. **Blockers**: Anything preventing progress, with root cause
6. **Environment State**: Toolchain versions, PATH notes, installed tools
7. **Known Issues**: Non-blocking problems documented for awareness
8. **Quick Start for Next Agent**: Copy-paste commands to verify state
9. **Priority Order**: If next session is time-limited, what to do first
10. **Session Metrics**: Agents spawned, files changed, tests status

### Step 2: Update Timeline

```
Update: timeline.md
Add: Session wins, milestone progress, velocity metrics
Update: Current Age/Epoch progress percentage
```

### Step 3: Commit and Push

```bash
git add docs/sessions/S<N>-*-handoff.md
git add timeline.md
git commit -m "$(cat <<'EOF'
docs(session): add S<N> handoff — <brief description>

<summary of session accomplishments>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
git push origin main
```

### Step 4: Final Status Check

```bash
go build ./...       # Still green?
go test -race ./...  # Still passing?
git status           # Clean tree?
```

---

## Session Templates

### Session Start Checklist

```markdown
## S<N> Session Start Checklist
- [ ] Read S<N-1> handoff document
- [ ] Load all Unheaded skills
- [ ] Check timeguru for current Age/Epoch
- [ ] Verify environment health (or note Cowork VM limitations)
- [ ] Confirm session goals with Muck
- [ ] Set aggressive multi-agent mode
- [ ] Begin sprint
```

### Session End Checklist

```markdown
## S<N> Session End Checklist
- [ ] Write S<N> handoff document with all 10 required sections
- [ ] Update timeline.md with wins and progress
- [ ] Commit all changes with proper commit messages
- [ ] Push to remote
- [ ] Verify build still green
- [ ] Verify tests still pass
```

---

## Naming Convention for Session Documents

```
docs/sessions/S<number>-<description>-handoff.md
```

Examples:
- `S24-dev-machine-runbook.md`
- `S25-verification-sprint-handoff.md`
- `S26-battle-plan.md`
- `S26-verification-sprint-handoff.md`

Session numbers are sequential. Never reuse a session number.

---

## Multi-Agent Coordination Rules

1. **Parallel execution is encouraged** when tasks have no dependencies
2. **Sequential execution is required** when tasks have data dependencies
3. **Git conflicts**: If multiple agents modify the same file, the later commit rebases
4. **Skill routing**: Use the right skill for the right task. Don't ask Developer for strategy. Don't ask Captain for code.
5. **Round Table**: Convene for major decisions, age transitions, sprint planning. Weekly max.
6. **Handoff chain**: Every session reads the previous handoff. No exceptions.

---

## Commit Message Convention

```
<type>(<scope>): <description>

[body — what and why]

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
```

**Types**: `feat`, `fix`, `test`, `ci`, `docs`, `refactor`, `security`, `perf`

**Scopes**: `protocol`, `ebpf`, `cmd`, `lint`, `session`, `lore`, `workflow`

---

## Emergency Procedures

### If Build Breaks
1. Check `go build ./...` error output
2. Fix immediately — do not proceed with broken build
3. Commit fix: `fix(<scope>): resolve build failure from <cause>`
4. Document in handoff

### If Tests Fail
1. Check `go test -race ./...` output
2. Identify failing test and root cause
3. Fix or document as known issue with severity
4. Never mark a session handoff as "green" if tests are failing

### If Git State is Dirty
1. `git status` to assess
2. `git stash` if work-in-progress needs saving
3. Never force-push to main
4. Document any unusual git state in handoff

---

_The chain never breaks. Every session inherits. Every session hands off._
_Formalized at Round Table S26 — February 20, 2026_
