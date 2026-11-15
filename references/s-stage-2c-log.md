# S-STAGE-2C log

Started 2026-11-15 on 18ec23a8 (branch sprint/s-stage-2c).
Plan: docs/battle-plans/S-STAGE-2C-battle-plan.md

## Ledger (verified insns, ascend)

| Change | OFF | ON | delta | net vs 921,036 | gate |
|---|---|---|---|---|---|

## Before (Phase 0, 2026-11-15)

- `gate p0: same=12 diff=0 rc=0 doom=1 ascend=889941 head=18ec23a8`
- `check-ebpf-insns.sh`: PASS, 41 programs within 10 % of the baseline; 0 allowlisted over 900,000. `pgrep -c doom-runner` after the gate: 0.
- `p3` (lines with `priv_level != 3` in `main.rs`): 2. `adr_last`: 144.
- Harness lines for the probes this plan extends:
  - PASS  rmprobe: 8 checks (UPC_RUNG_UNLINK)
  - PASS  lnprobe: 9 checks (UPC_RUNG_LINK)
  - PASS  fdprobe: 49 checks (UPC_RUNG_FD)
  - PASS  chdirprobe: 24 checks (UPC_RUNG_CHDIR)
  - PASS  pipeprobe: 20 checks (UPC_RUNG_PIPE)
  - PASS  hostile_ptr: 5 checks (UPC_RUNG_MKDIR)
  - PASS  sleepprobe: pause(n) waits, a sibling runs meanwhile (UPC_RUNG_SLEEP)
  - hostile_pgd: D6 is off, a child wrote slot 0's pgd (M-247 open)
  - PASS  hostile_reply: a word stored ahead of a trap is wiped, init stays out of reach
  - hostile_reply: D6 is off, a child forged the reply by sret and a fork note (ADR-135 open)

## Run cost

- One boot with `UPC_VERIFIER_STATS=1`: `verified 889941 insns` (the baseline row).
- eBPF ascend build: Maximum resident set size (kbytes): 975276.
- Gate p0: sweep12 about 2 min, harness about 7 min (other lanes' Go tests were running).
