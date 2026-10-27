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

### Phase 0 (Steps 1-24): done in the S24 grounding, before and while forging

Branch develop at `a7fe3acd`+; tools present (go, jq, curl, python3, haproxy 3.0.12, docker,
shellcheck, gpg; Step 5 not needed); GPG probe SIGN=yes (key cached by Stevie) so every [C] is
signed (Step 7); `go build ./...` and `go mod tidy -diff` clean (Steps 8-9); the four CI reds
reproduced locally and the live CI state read from the public API (Steps 12-15); code-site,
haproxy and doc claims checked with the commands in the session (Steps 16-21; Step 17's ticker
grep must target `pkg/wotan-client/reprobe.go` after Phase 2). Steps 22-23: parallel agents in
Claude worktrees, not `../unheaded-wt-*` (see amendments). Step 10's baseline: per agent, above.

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
| `325cce68` | — | found: gRPC PublishTopic never reached the store (signed drift/config events lost on restart); M-038 closed |
| `57a3f3f0` | 253-255 | relicense script (header/footer only, content listed for review); MIT-KEEP empty on evidence; apply proven in a throwaway worktree (272 files, 0 non-licence lines) |
| `b1e3a132` | 102-122, 124-146 (merge) | Phase 3 (Step 123 coding-gate re-run moved to 155b) |
| `0c29126f` | — | found by Phase 3: Zhen UI `CORS(app)` let any origin read conversation history; removed with flask-cors; QUICKSTART placeholders |
| `801befcc` | — | Phase 2's admin-key test fixture tripped gitleaks; built at run time |

### Step 43: the meta-gate, measured

Full `scripts/check-gates-can-fail.sh` in a clean clone on disk at `18a4b861`: WALL 1904.76 s (31.75 min),
max RSS 0.98 GB, run while two worktree agents worked (an upper bound for west). Every registered gate bit
when provoked except `check-gitleaks`, INCONCLUSIVE because that tree already failed it (Phase 2's admin-key
fixture, fixed in `801befcc`). Re-proved with `--only check-gitleaks` at `a3f2eb70`: bites. The new
`check-haproxy-cfg` (merged after that clone): `--only` bites. Timeout 25 -> 44 min (Step 44's formula).

### Phase 7 exit (Steps 262-272)

Eighteen gates exit 0 (list in `3bb9d44b`); FuzzClientIP 4.8 M execs and the two wotan-client fuzzers
30 s each, no crasher; full `go test -count=1 ./...` at `3bb9d44b`: 221 ok, 0 FAIL (the base carried 2).

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

### Attack-surface table (Phase 7, Step 261)

| Change | Before | After | Proven by |
|---|---|---|---|
| Wotan admin endpoints | `-admin` on by default, key unsettable, every call refused | off by default; on only with `WOTAN_ADMIN_API_KEY` (fail-loud if missing); 401 without the header | `services/wotan/cmd/wotan/admin_key_test.go` |
| wotan-client gRPC re-probe | none | outbound health probe every 30 s while degraded; no listener | `pkg/wotan-client` reprobe tests |
| `SubscribeWithRetry` in micromanager, monad, sophia, gateway, architect, anomaly, xdpsync | one-shot join; monad never subscribed `alerts.critical`; anomaly exited on a down Wotan | retry until joined; no new listener | `TestLateWotan_*` (fake Wotan started after the service) |
| Wotan rate limiter | keyed by IP:port (a new source port is a new bucket) | keyed by IP | middleware tests |
| Wotan CORS under `*` | origin echoed with `Allow-Credentials: true` | `*` without credentials | middleware tests |
| Wotan gRPC `PublishTopic` | ring buffer only (signed drift/config lost on restart) | also the persistent store, async; replayed signed messages no longer verify (store keeps no signature, FND-020): fail closed | `topic_service_store_test.go` |
| Prometheus 9090, Loki 3100 | every interface, IPv4 and IPv6, no auth | 127.0.0.1 only | live: loopback 200, from east refused (curl 000) |
| haproxy edge/internal | configs refused to parse (could not start) | parse under 2.9.15 and 3.0; backends resolve via Docker DNS at runtime; no cert committed | `scripts/check-haproxy-cfg.sh` (new gate) |
| Zhen web UI | chat straight to llama-server (Champion gate bypassed, T6a); `CORS(app)` let any origin read conversation history; reflected XSS in `/view/source`; hard-coded Flask secret default | chat through zhen-agentd only (503 without it); CSRF token on state-changing browser requests; no CORS; XSS fixed; no secret default | `raft/tests/test_zhen_app_gate.py` (CSRF, CORS) |
| zhen-agentd | started without auth; rate limit 0; direct `/api/v1/tool/exec` ran `write_file` with a synthesized justification | refuses to start unauthenticated (unless `-no-auth` on loopback); 5 rps per IP, burst 10; justification required | `authstart_test.go`, `toolexec_justification_test.go`. Open: limiter trusts `X-Forwarded-For` |
| unheaded-baseline | audit only | also publishes `compliance.deviation.<host>` (metadata only) when `-wotan-addr` is set | `cmd/unheaded-baseline/wotan_test.go` |
| `/.well-known/security.txt` | absent | static public GET on dashboard-backend, exempt from auth | dashboard-backend tests |
| kingdom-startup | monitoring started by nobody | Phase 0a starts it before the warm-up silence; no new ports (all documented, now loopback where unauthenticated) | dry run; live run pending (Step 153) |
| alert-mailer `mode: send` | dry-run | outbound SMTP to smtp.gmail.com:587 with STARTTLS; app password 0400 uid 16710 under `/etc/unheaded/alert-mailer` | live: page delivered 2026-10-27 |
| Probe subnets (Cuirass, Yaldabaoth) | any instance-reported address | only inside the bridge subnet (10.10.20.0/24 on west, configured) | `pkg/probenet` tests |
| `setup-host.sh` LXD preseed | `LXD_TRUST_PASSWORD:-changeme` | no trust password | script review |
| `switch-model.sh --stop` | unanchored port match could kill a process on a longer port | exact port | `scripts/lib` gpulease tests (red: killed the test binary) |
| Well role passwords | set via `docker exec -e PW=<value>` (argv, /proc-readable) | value passed by name only; nine roles rotated | `test-backup-restore.sh` argv shim |
| WireGuard PSK, east sshd | static PSK since 2026-03-25; classical KEX accepted | rotated; east hybrid-only (ML-KEM / sntrup) | live checks in `e7123644` |
| ebtables modules | loadable | `install /bin/false` on both hosts (FND-011 stopgap) | live `modprobe` refusal |
| Relicense | — | no runtime effect | licence-only diff check |

### Found on the way (not in the plan)
- Go known failures are ZERO on develop after the Phase 3 merge (both inherited FAIL lines fixed).
- Live prerequisites from Phase 3, apply with the next deploy (not before: old binaries may reject the new keys):
  `probe_subnets: [10.10.20.0/24]` in `/etc/unheaded/chaos/allowlist.yaml` on west and east (west's bridge is
  10.10.20.0/24, not the 10.10.10.0/24 CLAUDE.md says); Cuirass redeploy with `CUIRASS_PROBE_SUBNETS=10.10.20.0/24`
  (`docker/compose.cuirass-lxd.yml`); `ZHEN_AGENTD_API_KEY` (or `ZHEN_AGENTD_NO_AUTH=1`) in the Zhen web UI env, else
  chat answers 503.
- Risk to measure (Step 155b): routing Zhen chat through zhen-agentd scored 7/14 vs 12/14 on the coding gate on
  2026-05-02; not re-measured since.
- Coordinator process lesson: never judge a gate through `| tail -1` (it returns tail's exit code); 0c29126f went in
  with gitleaks red because of it, fixed in 801befcc.

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
