<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-CLEAN-SLATE progress log

Plan: `docs/battle-plans/S-CLEAN-SLATE-battle-plan.md`. This file is the session log
the plan refers to, and where `[STUCK]` / `[ESCALATE]` markers live. Step ticks live in
the plan; the escalation answers live in its ESCALATIONS table.

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed
debug attempts, mark it [STUCK] with what I tried, commit known-good state first, continue
with the next step that does not depend on it, and finish with a STUCK/ESCALATE report
ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 (Stevie present at the start, then unattended)

Coordinator on develop; four worktree agents per Appendix B: Phase 2, Phase 3, Phase 5
(code half) and Phase 6 (docs). The coordinator owns Phases 0, 1, 4, 7, 8, every live
host step and every merge. Agent operating contract: one brief (never push, no host
changes, no shared-ledger edits, full test suite serialised behind one lock).

### Plan amendments (Marshal Pattern 6)

- Worktrees were created from `aea808cb` (old `main`), not develop. Each agent noticed and
  reset its own branch to develop `a76307ab` before editing; develop was never touched by
  an agent. Future briefs say so up front.
- Step 43: `scripts/check-gates-can-fail.sh` refuses a dirty tree (the untracked
  `cmd/host-agent/` binaries, left alone by Stevie's word) and plants violations in tracked
  files, so it is timed in a clean clone on disk (`~/tmp/meta-gate-clone-20261027`, removed
  afterwards), never in the working checkout.
- Step 148: monitoring starts in a new Phase 0a, before the warm-up silence, not after the
  main stack (alert mail sends now; started late, warm-up would page). Phase 1b waits for
  `slo:error_ratio:5m`.
- Step 198: not needed; alert-mailer's file-based secret path (wiki/Alerting.md) works.
- Phase 0's `go test ./...` baseline: taken per agent at its own base (the lock-serialised
  full suite). Inherited FAIL lines at `a76307ab`: `cmd/alert-policy TestRefusals` (caused by
  the mode flip `265badbb`, fixed `a2b4e070`) and `scripts/lib
  TestSwitchModel_UsageAndStopUnderTheLease` (a real bug: `switch-model.sh` matched
  LLAMA_PORT as an unanchored regex, so `--stop` could kill a process on a longer port;
  handed to the Phase 3 agent, which owns the script).

### Done (commit, steps)

| Commit | Steps | What |
|---|---|---|
| `dba5e58e` | 38-42 (E1) | runbook pytest CI failure: the runner has no Python with `mcp`; step split, server test gated, CI summary self-reports |
| `265badbb` | 197, 199-201 (E5) | alert-mailer `mode: send`; first page delivered to stevie@bellis.tech |
| `225bb4e4` | 13, 25-34, 47-50 | dependency baseline 742; syft 1.55.0, grype 0.121.0, oxlint 1.89.0; drift-detect deleted |
| `a76307ab` | 51 | modernc.org/sqlite 1.61.0 (Stevie's word, a day inside the cooldown) |
| `e7123644` | 209-213 (E8, E9 east) | WireGuard PSK rotated live; verify-wireguard fixed; east sshd hybrid-only |
| `ecf1e555` | 207-208 (E7) | Well: nine role passwords rotated; argv leak in the rotation script fixed; FND-014 closed |
| `a2b4e070` | — | alert-policy TestRefusals anchors on the mode line |
| `d2608a11` | 177-193, 203, 211, 216, 219 (merge) | Phase 5 code half |
| `e8516f66` | 194-196 (E4) | FND-011 ebtables stopgap applied on east and west |
| `d88c8e16` | — | found: Prometheus and Loki were published on every interface with no auth; now loopback |
| `d9c1980a` | 147-151 | kingdom-startup Phase 0a/1b (FND-024 code half) |
| `7bbdd996` | 159-163 | discover meter ejects a dead replica (FND-023 code half) |
| `cf26212f` | 53-101 (merge) | Phase 2 |

### Open escalations (live halves)

| E | State | Blocks |
|---|---|---|
| E2 | granted; live halves pending until agents finish (memory): kingdom-startup live, haproxy pair live, FND-023 Step 310 re-run | 153-155, 155b, 164-166 |
| E3 | granted; waits for the Phase 3 merge so the debs carry everything | 173-174 |
| E6 | Stevie rotates the baselined credentials | 205-206 |
| E8 timer | rotation done; enabling the weekly timer needs a root-on-west SSH key authorized on govan@east (root on east): Stevie | FND-016 close |
| E9 west | east done; west took logins from 192.168.69.132/.135/.137 in 30 days whose KEX support is unknown: Stevie names the clients | 213 (west) |
| E10 | Stevie flips the GitHub settings | 215 |
| E11 | notice drafted (`docs/compliance/export-control-notice.md`); Stevie sends; the draft disputes the plan's legal citations (Barrister) | 218 |
| E12 | promotion after green CI, on Stevie's word | 288 |
| E13 | granted; coordinator runs the relicense after every branch merges | 253-258 |

### Found on the way (not in the plan)

- `scripts/well/set-role-passwords.sh` put each password in docker's argv (fixed, with a
  PATH-shim test).
- `scripts/wireguard/verify-wireguard.sh` reported a healthy east tunnel as missing (SSH by
  raw IP; fixed).
- Prometheus (9090) and Loki (3100) LAN-wide without auth (fixed, loopback).
- `cmd/monad` never subscribed to `alerts.critical`; `cmd/anomaly` exited on a down Wotan
  (Phase 2 agent, fixed).
- zhen-agentd ran `write_file` on a direct `/api/v1/tool/exec` with no justification
  (Phase 5 agent, fixed); its rate limiter still trusts `X-Forwarded-For` (open).
- A plaintext copy of `~/.bashrc` appeared as an untracked file named `!` in the repo root
  during the mailer pass; deleted, never staged. gitleaks scans tracked files only.
- The host has no `psql`; Well login checks run a `postgres:16.15-alpine` client container on
  the host network (the path pg_hba sees host clients on).
