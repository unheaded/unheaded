# S[N] [TITLE] BATTLE PLAN — [X] Phases, [Y]+ Steps

**Date**: YYYY-MM-DD
**Sprint**: S[N] — [One-line description]
**Prerequisite**: [What must be true before execution starts]
**Target**: [What "done" looks like in one sentence]
**Estimated Duration**: [X-Y hours across Z sessions]
**Agent Strategy**: [Which phases sequential, which parallelizable]

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
```

**Exit Gate Convention**: Every phase ends with a gate. If the gate fails, DO NOT proceed. Debug in-phase.

---

## PHASE 0: [TITLE] (Steps 1-[N])

**Goal**: [One sentence]
**Prerequisite**: [What must be true]
**Time**: [Estimate]
**Agent**: [Solo | Coordinator | Agent | Agent [P]]

### [Section Title]

- [ ] **Step 1** [B]: [Description]
  ```bash
  [exact command]
  ```

- [ ] **Step 2** [V]: [Verification description]
  - If pass → proceed
  - If fail → Step 3

- [ ] **Step 3** [D]: [Debug description]
  ```bash
  [debug command]
  ```

- [ ] **Step [N]** [V]: **PHASE 0 EXIT GATE** — [Condition]

---

<!-- Repeat PHASE pattern for each phase -->

---

## APPENDIX A: EMERGENCY PROCEDURES

### [Symptom 1]
```bash
# Step-by-step recovery
```

### [Symptom 2]
```bash
# Step-by-step recovery
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Est. Time |
|-------|-----------|---------------|--------------|-----------|
| 0 | Solo | No | None | X min |
| 1 | Coordinator | No | Phase 0 | X min |

**Critical Path**: 0 → 1 → ... → N

---

## APPENDIX C: QUICK REFERENCE

### [Data Structure / CLI / Map Paths / Wire Format]
```
[Quick reference content]
```

---

*S[N] Battle Plan — Forged YYYY-MM-DD*
*[X] Phases. [Y] Steps. [Evocative closing line.]*
