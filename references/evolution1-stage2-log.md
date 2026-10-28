# EVOLUTION-1 Stage 2 log

Started 2026-10-28 on 2e060148 (branch sprint/evolution1-stage2, cut from sprint/ascend-2).
Plan: docs/battle-plans/EVOLUTION-1-STAGE-2-battle-plan.md (draft, Phases 0-2).

## Phase 0 (Steps 1-22)

- Baselines: harness ALL GREEN (ascend 924,950, Doom 758,199, cart 42,743 / 146,897), sweep 8/8 SAME, `go test ./...`
  0 failures. Known failures: none.
- `getpidfork`: flag OFF `parent-pid=1 fork-ret=2 child-pid=2 wait-ret=2 parent-after-wait`; flag ON the same except
  `child-pid=other`. As the plan assumes.
- Hypotheses: H1 `usertrapret` writes `sscratch` from `p->trapframe` (utrap.c:148): match. H2 `uservec` reads
  `sscratch` and saves 17 registers plus a0 (via mscratch) into it: match. H3 BPF never writes the `sscratch` word:
  match. H4 `scheduler_context_switch` writes nothing in the CSR window: match. H5 S-mode is flat (main.rs:3630):
  match. H6 BPF `wait` writes 0 to `*status` (no per-process exit status yet): amended, rung 7 must add it. H7 no
  timer interrupt reaches the ascend kernel (the timer path is `cfg!(not(feature = "ascend-linux"))`): match.

## Phase 1: paused

Not completed this session; the tree is back to the Phase 0 state (probe reverted, `GETPID_INJECT` false, kernel
`4e39c82f…`). Resume at Step 25.
