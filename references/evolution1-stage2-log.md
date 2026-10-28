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

## Side findings (2026-10-28)

- **Half the trigger frames become CPU ticks: localized.** One boot (200,000 triggers + a 2 x 20,000 hijack probe):
  `veth-upc0p` sent 240,000 frames and `veth-upc0` received 240,000, so nothing is lost on the link; the probe counted
  10,000 ticks per 20,000 frames. The loss is inside `monad_cpu` between receive and `STAT_CPU_TICKS`. Leads: the
  entry point's `XDP_TX` tail-call chain (`TAIL_ROUND`, main.rs ~480) and the fact that `veth-upc0`'s lifetime RX
  "otherhost" counter is exactly half its RX packets (90,673,000 of 181,346,055). Not debugged further.
- **Bootstub retirement: not done, needs Stevie.** `crates/upc-bootstub` is stage 1 of UPC Boot Protocol v2 (frozen,
  ADR-067), described in `docs/doom/UPC_BOOT_PROTOCOL_V2.md`, ADR-080's `BootProtocol` stub variant and bootctl's
  `--bootstub`. Removing it changes a frozen spec's two-stage boot, so it is a decision, not cleanup (S-ASCEND-2
  Steps 281-282 stay open).
