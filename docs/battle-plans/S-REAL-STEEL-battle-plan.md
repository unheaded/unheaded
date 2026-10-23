<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-REAL-STEEL BATTLE PLAN: 17 Phases, 404 Steps

**Date**: 2026-10-23
**Sprint**: Sprint 3, REAL STEEL. Stubs become real, then chaos proves them.
**Prerequisite**: branch based on `443de3f82` (develop, S17 late) or newer; S17's Cuirass commits present (`b49e3f16` LXD REST client, `04c34ed0` safe mode, `5b490383` desired store, `22e9aa54` trust script); `go test ./...` green at start (S17: 267 ok / 0 fail).
**Target**: on west, Sword deploys a real workload into LXD through Cuirass (rolling, blue-green, canary), Yaldabaoth breaks it with real faults (netem, container kill, east-only systemd stop/kill, CHAOS-flag packet faults in a lab netns) inside an allowlisted blast radius with an abort switch and auto-revert, and the first game day shows deploy, inject, automatic rollback and steady state restored, with every "real" claim checked live.
**Estimated Duration**: 34-44 h of execution across 7-9 sessions. Three waits on Stevie (E1 sweep table, E2 west live window, E3 east install) plus one RFC Editor ruling (E4).
**Agent Strategy**: Coordinator owns Phases 0, 5, 9, 12, 14 (host-touching or iterative). Phases 2+3, 4+6, and 12+13 can run in parallel (Appendix B). Each parallel agent works in its own git worktree off develop. Hosts are read-only for every agent unless the step is covered by an answered [ESCALATE].
**Commit Cadence**: every 5 steps (max(3, min(5, 404/20)) = 5), at every exit gate, before every STUCK skip, after every red-to-green test. Never commit red. Never push (Stevie pushes). Signed if the key is cached, otherwise `--no-gpg-sign` (memory: feedback_unsigned_commits_when_afk).
**Stuck Protocol**: skip after 3x the step's `~time` or 2 failed debug attempts; write the STUCK marker into this file, commit, continue with the next step that does not depend on it (Appendix B dependency map).

**Builds on (cited, not re-derived):**

| Source | What this plan takes from it |
|---|---|
| ADR-090 (Proposed) | Rubric, "NOT slop" list, behavioural-equivalence bar, "burden of proof is on the deletion". Phase 0 is its first concrete tranche, scoped to the 148 unreached packages, not the 1,836-file campaign. |
| `docs/UNREACHED-GO-PACKAGES.md` (416eeef7) | The 148-package list and its regeneration command; the three "fake success" callouts. |
| ADR-088 (Planned) | Substrate ladder: LXD is a substrate, Sword is the deploy layer on top. Nothing here replaces compose or the debs. |
| ADR-099 (Proposed) | The Well is the source of truth long term; this sprint keeps deploy/chaos state in `pkg/statefile` files (the S17 pattern, 4affa68e) and records the Well migration as follow-on, not scope. |
| ADR-100 (Accepted) | SLO recording rules (`slo:good`, `slo:error_ratio:5m`, `slo:error_budget_remaining`) are the steady-state hypothesis. Burn tiers: fast 14.4x, medium 6x. Alerts stay quiet: game days run under `scripts/monitoring/maintenance.sh`. |
| ADR-064 (Proposed) | Only the replication snapshot TODO is touched, and only to stop it lying (Phase 3). No cluster work. |
| `~/tmp/next.md` S17 | Cuirass safe mode rules; "Needs Stevie's yes (Cuirass)"; the "Found, not fixed" list (pkg/lxd, pkg/certs/acme, 23 dashboard/js orphans, daemon internal/*); the Wotan client audit's open items (fallback queue, ProbeGRPC, stream health, timeguru/architect startup, transport.Connect, xdpsync). |
| Code read 2026-10-23 | Every "simulate" site cited in SITUATION below was read at `443de3f82`. |

---

## LEGEND

```
[B]            Bash command (run directly)
[V]            Verification step (MUST pass before proceeding)
[D]            Debug step (only if the prior step fails)
[W]            Write/create file
[R]            Read/inspect file
[S]            Sudo/elevated privileges required
[P]            Parallelizable with other [P] steps in the same phase
[P:N]          Parallelizable with Phase N
[SEQ]          MUST be sequential
[C]            Commit checkpoint
[ENV]          Environment setup/check
[BUILD]        Build/compilation step
[TEST]         Test execution
[CODE]         Code implementation (test first: red, then green)
[DESIGN]       Design decision recorded in-repo
[STUCK]        Step skipped via Skip Protocol (needs human)
[BLOCKED]      Blocked by an upstream [STUCK] (never used for decisions)
[DECIDE]       Decision with a pre-seeded RECOMMENDATION; proceed without asking
[ESCALATE]     Needs Stevie. STOP this chain; continue independent chains
[AFTER-E1]     Runs only once E1 is answered; skipped (not STUCK) until then
[PREFLIGHT]    Hypothesis verification (Phase 0 only)
[REGEN]        Regenerate derived artifacts
[AUDIT-UPDATE] Mark a finding resolved in its tracking doc
[DOC-UPDATE]   Update downstream docs (wiki, CLAUDE.md, next.md)
[SECURITY]     Security review step
[COMPLIANCE]   License/SBOM/dependency check
[BARE-METAL]   Requires the real hosts (west/east), real kernel, real LXD
[LIVE-RO]      Live read-only verification on west/east (GET, list, show)
[LIVE-RW]      Live host-changing step; only under an answered [ESCALATE]
```

**[DECIDE] format**: RECOMMENDATION, Rationale, "Override ONLY if". The agent proceeds with the recommendation.
**[ESCALATE] format**: Context, the exact question(s), "Why no recommendation" (or why the call is Stevie's even with one). The agent stops that chain only.
**Time tags**: `~Nm` is expected wall-clock. 3x over = Skip Protocol.

**DoD-STD (every phase's Definition of Done includes these; Micromanager gate):**
1. Every step executed or STUCK-logged in this file.
2. Every [V] in the phase passed.
3. Zero NEW test failures versus the KNOWN FAILURES BASELINE (`go test ./...`, plus `-race` on touched packages).
4. Commit checkpoint made; tree clean (`git status --short` empty).
5. No new dependency without a Barrister/ADR-004 check (`go mod tidy -diff` empty, `scripts/check-*` green).
6. Attack surface unchanged, or the change is listed in Phase 15's review table.
7. Every claim of "real" in a commit message has a [LIVE-RO] or [LIVE-RW] step that proved it, cited by step number.
8. Zero user data access: no step reads packet payloads, log contents of other services, or user data; probes read metadata only.

---

## VARIABLES

Set in Step 1; every later step assumes them.

```bash
export PROJECT_ROOT="$(git rev-parse --show-toplevel)"
export PLAN="$PROJECT_ROOT/docs/battle-plans/S-REAL-STEEL-battle-plan.md"
export SWEEP_DIR="$PROJECT_ROOT/docs/sweeps"
export VERDICTS="$SWEEP_DIR/REAL-STEEL-UNREACHED.tsv"     # Phase 0 ledger (package level)
export SCRATCH="${SCRATCH:-$(mktemp -d -t real-steel.XXXXXX)}"   # /tmp is a 7.2G tmpfs on west: keep it small
export VM="http://127.0.0.1:8428"                          # VictoriaMetrics on west (compose)
export CUIRASS="http://127.0.0.1:19006"                    # compose Cuirass on west (debs use 17000)
export SWORD="http://127.0.0.1:19011"                      # Sword (Phase 8, takes Canary's port: D-08)
export CHAOS="http://127.0.0.1:19012"                      # chaos-controller (ports.Chaos)
export EAST="govan@east"                                   # P2P link 192.168.13.1
export EAST_CHAOS="http://192.168.13.1:19012"              # chaos-controller on east (Phase 12)
export LXD_PROJECT="unheaded"
export GAMEDAY_SVC="steel-echo"
export TRAILER="${TRAILER:?set TRAILER to the attribution lines your session was given}"
# API keys live in host env files outside the repo (feedback_no_creds_in_repo); export them only
# in the shell that runs live steps. Fail loud, never a default:
#   : "${CUIRASS_API_KEY:?}" "${SWORD_API_KEY:?}" "${CHAOS_API_KEY:?}"

# Signing: probe the agent the cheap way (reference_resign_unpushed).
if git -C "$PROJECT_ROOT" commit-tree -S -m probe "HEAD^{tree}" >/dev/null 2>&1; then SIGN="-S"; else SIGN="--no-gpg-sign"; fi
export SIGN

# commit "<conventional subject>" "<step range>" <paths...>   (explicit existing paths, never -A;
#   deletions made with `git rm` are already staged and need not be listed)
commit() {
  local subj="$1" steps="$2"; shift 2
  git -C "$PROJECT_ROOT" add -- "$@" &&
  git -C "$PROJECT_ROOT" commit "$SIGN" -m "$subj" -m "Plan: S-REAL-STEEL steps $steps" -m "$TRAILER"
}

# vmq '<promql>'  -> "label value" lines; empty output means no series (counts as bad, ADR-100)
vmq() { curl -fsS --max-time 5 "$VM/api/v1/query" --data-urlencode "query=$1" \
        | jq -r '.data.result[] | "\(.metric.slo // .metric.job // .metric.instance // "-") \(.value[1])"'; }

# gotest <pkgs...>  -> race + count=1, tail only
gotest() { (cd "$PROJECT_ROOT" && go test -race -count=1 "$@" 2>&1 | tail -25); }
```

---

## SITUATION

**What is simulated today (read at `443de3f82`):**

| Where | What it does instead of the real thing |
|---|---|
| `pkg/deploy/deploy.go:955` `deployRolling` | `InstancesUpdated++; time.Sleep(100ms)` per replica. `NewEngine` takes a `container.Runtime` whose only implementation is `pkg/container/mock_runtime.go`. |
| `pkg/deploy/pipeline/stage.go:200-322` | build, deploy, verify, promote and cleanup stages are each "Simulate ...". |
| `pkg/deploy/strategy/{rolling,bluegreen,canary}.go` | Real control flow, but when `Callbacks.OnInstanceCreate` etc. are nil it invents an `Instance` and marks it running ("Simulate instance creation"). **This is the seam**: `strategy.Callbacks` (OnInstanceCreate/Delete/Update/HealthCheck/TrafficShift/Analysis) is exactly the driver interface a real backend needs. |
| `pkg/deploy/rollback` | `Manager` with `DeploymentLookup`, `InstanceUpdater`, `TrafficShifter` interfaces; no implementation anywhere. |
| `services/sword/sword.go:267` `Rollback` | `time.Sleep(100ms)` then `DeployRolledBack`. No binary imports `services/sword`. |
| `services/canary/canary.go:591` `evaluateCanary` | Hardcoded canary metrics (error 100 ppm, p99 45 ms, 250 rps) and stable metrics; the SLO check can never fail. No binary imports it. |
| `services/yaldabaoth/yaldabaoth.go:611-660` `registerBuiltinInjectors` | All four injectors log and return nil. `runProbe` returns `Success: true` always; `shouldAbort` returns false always. |
| `cmd/chaos-controller` | Uses only `yaldabaoth.GameDayScheduler` + `AuditTrail`; never builds `yaldabaoth.Service`. `InjectRule` stores a rule in a map and publishes an event: nothing reaches the kernel. Deployed nowhere (no compose service, no unit). |
| `services/yaldabaoth/gameday.go:218` | "In a real implementation, we would undo the injected rules here." |
| `ebpf/yaldabaoth-ebpf` | Real TC egress classifier: enrolled flow labels (`CHAOS_TARGETS`) get the Monad CHAOS flag (0x80) and a mode action. No userspace loader writes `CHAOS_TARGETS`; DELAY only writes `scratch_r0` ("netem qdisc ... feeds into" is not wired). |
| `services/wotan/internal/cluster/replication_server.go:105` | `TransferSnapshot` sends one empty chunk with `Final: true` and `LastSeq = current`: a caller trusting it would skip every message. No caller exists today. |
| `pkg/wotan-client/client.go:303` `ProbeGRPC` | Creates/reuses the client and declares health without an RPC (`_ = grpcCl`). |
| `pkg/wotan-client/client.go:605` `FlushFallbackQueue` | Exists, called by nothing: the queue fills to 1000 and drops oldest forever. |
| `pkg/transport/cascade.go:21` `Connect` | Called by nothing (cmd/sophia only mentions it in a comment). |
| Cuirass (`cmd/unheaded-daemon`) | Real LXD REST client (S17), safe mode, observe by default. Gaps for deploy: no health probe (LXD health is always "unknown"), no device (proxy) management, no out-of-band stop for chaos, `LXD_BACKEND` defaults to mock and nothing is deployed with https. |

**Spec facts that constrain packet-level chaos (RFC Editor territory, wire format v0x01 FROZEN):**

- Monad flags octet 0x07, bit 7 = CHAOS (foundation-06 §Flags Bitfield; `monad_common::flags::CHAOS = 0x80`). It already exists; this sprint adds no flag and no field.
- `hop-ebpf` skips CHAOS packets (STAT_CHAOS_SKIPPED); `anomaly-ebpf` scores them suspicious; `shield-ebpf:836` **reuses** CHAOS to tag blocked-source Anomaly events.
- **Mismatch found while grounding**: foundation-06 Anamnesis Event Type registry says `EVENT_CHAOS 0x04`, `EVENT_ANOMALY 0x08`; code (`monad_common::EventType`, `pkg/protocol/bpfschema` `EventChaos = 0x05`) says Chaos 0x05, Anomaly 0x04. Phase 13 raises it as E4; this sprint codes against the code constants and changes neither side.

**Hosts**: west = dev host (compose stack, LXD 5.21.8 with `pihole` unmanaged in the default project, stack DOWN since 10-19 by Stevie's "manual at boot"); east = staging (debs, older binaries, lldpd, reachable as `govan@east`). Read-only GETs against east are cheap and find real bugs (memory: reference_east_live_validation).

**Out of scope (parked, cited so the Marshal can wave them off):** Sabatons IPMI (`pkg/baremetal/ipmi`, `services/sabatons`) stays parked; ADR-064 clustering; the Well as deploy/chaos store (ADR-099 follow-on); server-side weighted traffic split (D-04); Wotan rate-limiter/admin-key/CORS items (S17 Agent 1 "Open (Stevie)"); the full 1,836-file ADR-090 ledger.

---

## PREFLIGHT HYPOTHESES

Verified in Phase 0 Steps 4-20. A FALSE hypothesis is not a failure: it amends the plan (note it here, then follow the "If false" column).

| # | Hypothesis | Verify via (step) | If false | Result (Phase 0, 2026-10-23) |
|---|---|---|---|---|
| H1 | HEAD contains `443de3f82` and the S17 Cuirass commits | Step 4 | Rebase/branch from develop; STOP if those commits are gone | **TRUE (amendment: the cited hashes were rewritten when unpushed commits were re-signed; the same commits are present by subject: `37f1c6c0` LXD REST client, `b04336e3` safe mode, `1883f910` desired store, `2a2f4e62` trust script, `c8d42b7f`/`e3769107` cli via Cuirass, `a7eeacf8` UNREACHED list; executed on `f7d730e7`)** |
| H2 | 148 of 300 packages are still unreached | Step 5 | Use the new count everywhere; the table is generated, not typed | **TRUE: 148 unreached of 307 packages (7 new packages since the doc, all reached); the list is identical to the doc's** |
| H3 | Every deploy path simulates when its driver is nil (sites in SITUATION) | Step 6 | Drop the sites that are already real from Phase 7 | **TRUE (rolling.go:209, bluegreen.go:139, canary.go invents instances when `OnInstanceCreate` is nil at :157/:297 without a 'simulate' comment; stage.go x5; deploy.go:955; sword.go:267; canary service :591)** |
| H4 | Yaldabaoth injectors only log; `runProbe` is always true; `shouldAbort` always false | Step 7 | Keep whatever is real; Phase 10/11 shrink | **TRUE** |
| H5 | `chaos-controller` never constructs `yaldabaoth.Service` | Step 7 | Phase 10 wires the existing one instead of adding it | **TRUE (only `NewAuditTrail`, `NewGameDayScheduler`)** |
| H6 | No userspace writes `CHAOS_TARGETS` | Step 8 | Reuse that loader in Phase 13 | **TRUE (zero hits outside the eBPF program, not even bpfschema)** |
| H7 | West LXD has an HTTPS listener and no `unheaded` project yet (trust script not applied) | Step 9 [LIVE-RO] | If the project exists, E2 shrinks to "confirm it is ours" | **TRUE (`[::]:8443`, projects: default only, `pihole,RUNNING`)** |
| H8 | West main stack (wotan, victoria, cuirass) is down | Step 10 [LIVE-RO] | If up, E2 drops the compose-up ask | **TRUE (VM, Cuirass, Wotan all refuse connections)** |
| H9 | East answers read-only; unheaded-* units active list known | Step 11 [LIVE-RO] | Phase 12 waits for east; nothing else does | **TRUE (12 units + 2 timers + 1 path active, incl. wotan, timeguru, architect, unheaded-daemon; list in `$SCRATCH/east-units.txt`)** |
| H10 | `sch_netem` and `tc` exist on west and east kernels | Step 12 | Phase 11 netem STUCK on that host; container kill still runs | **TRUE (both hosts: `sch_netem.ko.zst` on 6.17.0-41, iproute2-6.16.0)** |
| H11 | Wotan registers `grpc.health.v1` (services/wotan/cmd/wotan/main.go:343) | Step 13 | ProbeGRPC uses a cheap Wotan RPC instead (D-11) | **TRUE (line 352 now, not 343)** |
| H12 | `TransferSnapshot` has no client caller | Step 13 | Phase 3 must fix the caller too | **TRUE (zero callers)** |
| H13 | Spec EVENT_CHAOS 0x04 vs code 0x05 mismatch is real | Step 14 | Drop E4's first question | **TRUE (spec 0x04/0x08; code Anomaly 0x04, Chaos 0x05 in monad-common and bpfschema)** |
| H14 | `services/loadbalancer` (xdpsync) is unreached, so its Wotan bug is in dead code | Step 5 | Phase 2 fixes xdpsync's join instead of routing it to E1 | **TRUE** |
| H15 | `go test ./...` is green at HEAD (S17: 267 ok / 0 fail) | Step 16 | Record failures in KNOWN FAILURES BASELINE; they are not this sprint's | **TRUE: 277 ok / 0 FAIL (`go test -count=1 ./...` at f7d730e7)** |
| H16 | A restricted LXD client in a project with `features.images=false` cannot import images | Step 145 (after E2) | If it can, the image build runs as Cuirass's identity, not govan | open (after E2) |
| H17 | A compose container can reach lxdbr0 instance IPs (Cuirass's and Sword's probes) | Step 174 | D-09 fallback: Sword runs as a host systemd unit; Cuirass probe moves to the host side (STUCK until decided) | open (Step 174) |

## KNOWN FAILURES BASELINE

Pre-existing, not regressions (from next.md S17). Step 16 re-records the live list into `$SCRATCH/baseline-tests.txt`; only failures outside that file count against a phase.

| Failure | Source |
|---|---|
| `tests/e2e` timing tests flake under `-race` | S17 Wotan client audit |
| `pkg/secrets/store` `TestFileStore_WatchDelete` data race | S17 Agent 12 |
| `trace-collector-go` darwin vet broken | S17 Agent 3 |
| GitHub `ci.yml`, `docker.yml`, `ebpf.yml`, `release.yml` are `disabled_manually` | S17 (Stevie's setting; do not touch) |
| alert-mailer in `mode: dry-run`; no Watchdog | ADR-100 Implementation |
| East runs pre-S17 binaries (logs-oldest, gateway, uptime, blank-trace bugs visible there) | S17 deploy note |
| `go mod tidy -diff` not empty (drops indirect `gopkg.in/check.v1` and stale `kr/*` sums); go.mod/go.sum unchanged by this sprint | Step 78 (2026-10-23) |
| `cargo test -p monad-common` inside `ebpf/` fails to compile (the workspace config forces bpfel + build-std); use `make test-rust-ebpf-common` | Step 17 (2026-10-23) |

Step 16 at `f7d730e7` (2026-10-23): `go build ./...` clean, `go test -count=1 ./...` 277 ok / 0 FAIL.

---

## PHASE 0: PREFLIGHT AND THE ADR-090 UNREACHED SWEEP (Steps 1-44)

**Goal**: verify every hypothesis, record the test baseline, and classify all 148 unreached packages plus the known dead code into one evidence-backed delete/wire/keep/park table that Stevie answers in one pass (E1).
**Prerequisite**: a worktree on a branch based on `443de3f82` or newer.
**Time**: 2.5-3.5 h
**Agent**: Coordinator (Steps 1-20), Agent [P] (Steps 21-36), Coordinator (37-44)

### Environment and branch

- [x] **Step 1** [ENV] ~2m: Set VARIABLES (block above) in the shell; confirm.
  ```bash
  echo "$PROJECT_ROOT $SIGN"; test -f "$PLAN" && echo plan-ok
  ```
- [x] **Step 2** [B] ~1m: Confirm the branch is not develop/staging/main.
  ```bash
  git -C "$PROJECT_ROOT" branch --show-current
  ```
- [x] **Step 3** [V] ~30s: Branch name is a sprint/worktree branch. If it is `develop`, `staging` or `main` → `git -C "$PROJECT_ROOT" switch -c real-steel` and repeat Step 2.
- [x] **Step 4** [PREFLIGHT] ~1m: **H1**.
  ```bash
  for c in 443de3f82 b49e3f16 04c34ed0 5b490383 22e9aa54 f95b0764 416eeef7; do
    git -C "$PROJECT_ROOT" merge-base --is-ancestor "$c" HEAD && echo "ok $c" || echo "MISSING $c"; done
  ```
  - All `ok` → Step 5. Any MISSING → rebase onto develop; still missing → STOP (wrong base).

### Hypotheses

- [x] **Step 5** [PREFLIGHT] ~3m: **H2, H14**: regenerate the unreached list exactly as the doc says.
  - Done 2026-10-23: 148 / 307; services/loadbalancer unreached (1).
  ```bash
  cd "$PROJECT_ROOT"
  mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)
  comm -23 <(go list ./... | sort -u) <(go list -deps $mains | grep '^unheaded/' | sort -u) > "$SCRATCH/unreached.txt"
  wc -l < "$SCRATCH/unreached.txt"; go list ./... | wc -l
  grep -c '^unheaded/services/loadbalancer$' "$SCRATCH/unreached.txt"
  ```
  - Note: uutils `comm` fails on two pipes (memory: reference_west_tool_gotchas). Process substitution of one side each is fine; if comm errors → `go list ./... | sort -u > $SCRATCH/all.txt` and `... > $SCRATCH/reached.txt`, then `comm -23 $SCRATCH/all.txt $SCRATCH/reached.txt`.
  - Record the count in PREFLIGHT H2's row (edit this file).
- [x] **Step 6** [PREFLIGHT] ~2m: **H3**.
  ```bash
  grep -n -i 'simulat' "$PROJECT_ROOT"/pkg/deploy/{deploy.go,pipeline/stage.go,strategy/*.go} "$PROJECT_ROOT"/services/{sword/sword.go,canary/canary.go} | grep -v _test
  ```
- [x] **Step 7** [PREFLIGHT] ~2m: **H4, H5**.
  ```bash
  sed -n '/func (s \*Service) runProbe/,/^}/p;/func (s \*Service) shouldAbort/,/^}/p' "$PROJECT_ROOT/services/yaldabaoth/yaldabaoth.go"
  grep -n 'yaldabaoth\.New' "$PROJECT_ROOT/cmd/chaos-controller/main.go"
  ```
  - Expect `NewAuditTrail` and `NewGameDayScheduler` only, no `NewService`.
- [x] **Step 8** [PREFLIGHT] ~2m: **H6**.
  ```bash
  grep -rn 'CHAOS_TARGETS' "$PROJECT_ROOT" --include=*.go --include=*.rs | grep -v 'ebpf/yaldabaoth-ebpf/src/main.rs' | grep -v /target/
  ```
  - Only `pkg/protocol/bpfschema` doc/struct hits expected (no `ebpf.LoadPinnedMap` of it).
- [x] **Step 9** [PREFLIGHT][LIVE-RO][BARE-METAL] ~2m: **H7** on west.
  ```bash
  lxc version; lxc config get core.https_address; lxc project list --format csv | cut -d, -f1
  lxc list --project default --format csv -c ns
  ```
  - Expect a listener (non-empty), no `unheaded` project, `pihole,RUNNING`.
- [x] **Step 10** [PREFLIGHT][LIVE-RO] ~1m: **H8**.
  ```bash
  curl -fsS --max-time 3 "$VM/health" && echo VM-UP || echo VM-DOWN
  curl -fsS --max-time 3 "$CUIRASS/health" && echo CUIRASS-UP || echo CUIRASS-DOWN
  curl -fsS --max-time 3 http://127.0.0.1:18000/health && echo WOTAN-UP || echo WOTAN-DOWN
  ```
- [x] **Step 11** [PREFLIGHT][LIVE-RO] ~2m: **H9**.
  ```bash
  ssh -o BatchMode=yes -o ConnectTimeout=5 "$EAST" 'systemctl list-units "unheaded-*" --no-legend --plain | awk "{print \$1, \$3, \$4}"'
  ```
  - Record the active list in `$SCRATCH/east-units.txt` (Phase 12 picks its allowlist from it).
- [x] **Step 12** [PREFLIGHT][LIVE-RO] ~1m: **H10**.
  ```bash
  modinfo -F filename sch_netem; tc -V
  ssh -o BatchMode=yes "$EAST" 'modinfo -F filename sch_netem; tc -V'
  ```
- [x] **Step 13** [PREFLIGHT] ~2m: **H11, H12**.
  ```bash
  grep -n 'RegisterHealthServer' "$PROJECT_ROOT/services/wotan/cmd/wotan/main.go"
  grep -rn 'TransferSnapshot(' "$PROJECT_ROOT" --include=*.go | grep -v -E '_test|\.pb\.go|replication_server.go'
  ```
  - Expect one hit for health, zero for snapshot callers.
- [x] **Step 14** [PREFLIGHT] ~2m: **H13**.
  ```bash
  grep -n 'EVENT_CHAOS\|EVENT_ANOMALY' "$PROJECT_ROOT/docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md"
  grep -n 'Chaos = \|Anomaly = ' "$PROJECT_ROOT/ebpf/monad-common/src/lib.rs"
  grep -n 'EventChaos\|EventAnomaly' "$PROJECT_ROOT/pkg/protocol/bpfschema/core_maps.go"
  ```
- [x] **Step 15** [V] ~2m: **PREFLIGHT TABLE UPDATED**: every H1-H15 row in this file carries `TRUE`/`FALSE (amendment: ...)`. H16/H17 stay open until Steps 145/151.

### Test baseline

- [x] **Step 16** [TEST] ~15m: **H15**, full baseline (not -race: the race tier runs per touched package).
  ```bash
  cd "$PROJECT_ROOT" && go build ./... && go test -count=1 ./... 2>&1 | grep -E '^(ok|FAIL|---)' > "$SCRATCH/baseline-tests.txt"; \
  grep -c '^ok' "$SCRATCH/baseline-tests.txt"; grep '^FAIL' "$SCRATCH/baseline-tests.txt" || echo "no FAIL"
  ```
- [x] **Step 17** [B] ~3m: Rust/eBPF baseline for the two crates this sprint touches.
  - Done 2026-10-23: the Step 17 cargo line fails as written (ebpf/.cargo/config.toml forces bpfel + build-std: E0463/E0152). Used the Makefile recipe `test-rust-ebpf-common` (CARGO_TARGET_DIR outside ebpf/, --manifest-path ebpf/monad-common/Cargo.toml): 91 + 1 passed. `make ebpf-yaldabaoth` built; yaldabaoth-ebpf = 18080 bytes.
  ```bash
  cd "$PROJECT_ROOT/ebpf" && cargo test -p monad-common 2>&1 | tail -3
  cd "$PROJECT_ROOT" && make ebpf-yaldabaoth 2>&1 | tail -3 && ls -l ebpf/target/bpfel-unknown-none/release/yaldabaoth-ebpf
  ```
  - Record the object's byte size in `$SCRATCH/yald-size.txt` (Phase 13 compares).
- [x] **Step 18** [D] ~5m: Only if Step 17 build fails: `rustup show` in `ebpf/` (rust-toolchain.toml pins nightly + `-Z build-std`); `cargo +nightly --version`. 2 attempts, then STUCK Step 17 (blocks Phase 13 only).
  - Done 2026-10-23: attempt 1 (host target) still hit build-std; attempt 2 = the Makefile recipe, green. Environment: go.mod's replace needs `../projects/doomgeneric/unheaded`; in a worktree that is `.claude/worktrees/projects`, so a symlink to ~/tmp/projects was added there (outside the repo).
- [x] **Step 19** [V] ~1m: **BASELINE RECORDED**: `$SCRATCH/baseline-tests.txt` exists, FAIL lines (if any) copied into KNOWN FAILURES BASELINE in this file.
- [x] **Step 20** [C] ~2m: **COMMIT CHECKPOINT** (preflight results in the plan).
  ```bash
  commit "docs(battle-plan): S-REAL-STEEL preflight results" "1-20" "$PLAN"
  ```

### Evidence for the sweep table

`[DECIDE] D-01 (ledger shape)`: RECOMMENDATION: a **package-level** ledger `docs/sweeps/REAL-STEEL-UNREACHED.tsv` (`package <TAB> group <TAB> evidence <TAB> recommendation <TAB> verdict <TAB> commit <TAB> notes`), not ADR-090's 1,836-row file ledger. Rationale: ADR-090 is still Proposed and its ledger is per file; this sprint's question is per package (is anything going to call it). The TSV's columns map onto ADR-090's (`verdict` uses DELETED / FLAGGED / CLEAN, plus WIRE and PARK), so the file ledger can import it later. Override ONLY if Stevie has accepted ADR-090 by execution time: then also create the file ledger and mark these packages' files.

- [x] **Step 21** [W][P] ~10m: Write `scripts/sweep/unreached-evidence.sh` (new; dry, read-only): for each package in `$SCRATCH/unreached.txt` print one TSV row with
  - `test_importers`: count of `_test.go` files outside the package that import it (`grep -rl --include=*_test.go "\"$pkg\""`);
  - `prod_importers`: non-test importers (should be other unreached packages only);
  - `fake_markers`: count of `simulat|placeholder|in (a )?real implementation|would (be|actually)|not yet implemented` in its non-test files;
  - `exported_refl`: hits for `reflect\.|//go:linkname|//export |cgo` (ADR-090: invisible to the compiler);
  - `last_touched`: `git log -1 --format=%cs -- <dir>`;
  - `lines`: non-test line count (ADR-090 permits "lines removed" for a deletion campaign; it is reported only in the sweep ledger, never as delivered value).
  ```bash
  bash "$PROJECT_ROOT/scripts/sweep/unreached-evidence.sh" "$SCRATCH/unreached.txt" > "$SCRATCH/evidence.tsv"; wc -l < "$SCRATCH/evidence.tsv"
  ```
- [x] **Step 22** [V] ~1m: Row count equals Step 5's count. Mismatch → Step 23.
- [x] **Step 23** [D] ~5m: Diff `cut -f1 evidence.tsv` against `unreached.txt`; usual cause is a package dir with only `_test.go` or a build-tagged file (`//go:build`); give those rows `fake_markers=n/a` rather than skipping. 2 attempts → STUCK (blocks Steps 30-38).
  - Done 2026-10-23: not needed (148 rows = Step 5); test-only/build-tagged dirs carry fake_markers=n/a.
- [x] **Step 24** [B][P] ~3m: Non-Go dead code evidence: the 23 orphan dashboard scripts.
  - Done 2026-10-23: 23 orphans incl. monad-decode.js (Math.random registers, fake CRC, random 5% checksum_valid=false).
  ```bash
  cd "$PROJECT_ROOT/dashboard" && for f in js/*.js; do b=$(basename "$f"); \
    grep -rqs --include=*.html --include=*.js -F "$b" . --exclude="$b" || echo "$b"; done > "$SCRATCH/js-orphans.txt"; wc -l < "$SCRATCH/js-orphans.txt"
  grep -n -i -E 'Math\.random|crc|checksum' js/monad-decode.js | head
  ```
  - Expect 23, including monad-decode.js (invents register values, CRC and a random 5% "checksum invalid").
- [x] **Step 25** [B][P] ~2m: Parked `.go.dev` files (never compiled, so invisible to the list).
  - Done 2026-10-23: 5 files: pkg/lxd/real_client.go.dev, pkg/container/{lxd/lxd,image/image,network/network,storage/storage}.go.dev.
  ```bash
  cd "$PROJECT_ROOT" && git ls-files '*.go.dev' | xargs -r wc -l
  ```
  - Expect at least `pkg/lxd/real_client.go.dev` (1648 lines, client_golang + zerolog) and `pkg/container/lxd/lxd.go.dev`.
- [x] **Step 26** [B][P] ~2m: Dead-in-a-live-package evidence (unreached by symbol, not by package): `transport.Connect`, `yaldabaoth.Service` (wired by this sprint, so WIRE), `FlushFallbackQueue` (wired by Phase 2), `pkg/health/aggregator.go` (S17 Agent 2: used by no binary).
  - Done 2026-10-23: transport.Connect has zero callers (two comments only). pkg/health/aggregator.go has no binary caller, but response.go (also uncalled) uses its HealthStatus constants.
  ```bash
  cd "$PROJECT_ROOT" && grep -rn 'transport\.Connect(' --include=*.go . | grep -v _test
  grep -rln 'NewAggregator\|aggregator\.' --include=*.go cmd services | head
  ```
- [x] **Step 27** [R][P] ~10m: For each of the three S17 "fake success" items, read the whole file (ADR-090 per-file procedure step 1, not a grep) and note one line of evidence each in `$SCRATCH/fakes.md`: `pkg/lxd/client.go` RealClient, `pkg/certs/acme/acme.go`, `cmd/unheaded-daemon/internal/lxd/client.go`.
- [x] **Step 28** [B][P] ~3m: Which unreached packages only exist to serve another unreached package (delete together): build the reverse-import edges among the unreached set.
  - Done 2026-10-23: 149 prod edges among the unreached set. Also: 16 tests of REACHED packages import unreached ones (pkg/audit, pkg/secrets, pkg/storage tests import their sub-packages; kanban-app/monad/sophia/captain/metrics tests import mock/metricstest; pkg/crypto/pqc tests import pkg/monad).
  ```bash
  cd "$PROJECT_ROOT" && while read -r p; do go list -f '{{.ImportPath}} {{join .Imports " "}}' "./${p#unheaded/}" 2>/dev/null; done < "$SCRATCH/unreached.txt" \
    | awk '{for(i=2;i<=NF;i++) if ($i ~ /^unheaded\//) print $1, "->", $i}' > "$SCRATCH/unreached-edges.txt"; wc -l < "$SCRATCH/unreached-edges.txt"
  ```
- [x] **Step 29** [C] ~2m: **COMMIT CHECKPOINT**.
  ```bash
  commit "chore(sweep): unreached-package evidence script" "21-29" scripts/sweep/unreached-evidence.sh
  ```

### Classification

`[DECIDE] D-02 (grouping rule)`: RECOMMENDATION: classify every row by these rules, in order, first match wins:

| Rule | Group | Pre-seeded recommendation |
|---|---|---|
| R1 fakes success (S17 callouts, `fake_markers > 0` on a code path a caller would trust) and superseded by a live path | G1 fake-and-superseded | DELETE |
| R2 a test helper / harness / fixture package by nature (`*test*`, `tests/*`, `tomb/lich/harnesses`, `nix/tests`, `scripts/*`, `pkg/metrics/metricstest`, `pkg/wotan-client/mock`, `cmd/dashboard-backend/jstest`) with `test_importers > 0` | G6 test-only | KEEP (not slop by this metric) |
| R3 this sprint wires it (`pkg/deploy/strategy`, `pkg/deploy/rollback`, `services/sword`, `services/yaldabaoth` Service) | G4 wire-now | WIRE |
| R4 folded into a wired package by this sprint (`services/canary` into Sword) | G3 merge | MERGE, then DELETE |
| R5 parallel deploy abstractions the sprint does not use (`pkg/deploy` Engine, `pkg/deploy/pipeline`, `pkg/deploy/artifact`, `pkg/container` if only the Engine imports it) | G3 superseded | DELETE |
| R6 Sabatons / bare metal (`pkg/baremetal/ipmi`, `services/sabatons`, `pkg/baremetal/*`) | G7 parked | PARK (Stevie's standing "parked") |
| R7 protocol reference code (`pkg/protocol/*`) | G8 spec-adjacent | PARK pending RFC Editor review (deleting reference code for frozen specs is a spec decision) |
| R8 Armory/Gnostic service with a live eBPF counterpart in `ebpf/` (firewall, qos, nfv, version, anomaly, compliance, failover) | G9 control plane in waiting | PARK with a one-line "unwired: control plane for ebpf/<x>" note in UNREACHED-GO-PACKAGES.md |
| R9 everything else (library or service nobody imports, no live counterpart) | G10 unused | DELETE |

Rationale: ADR-090's bar is "burden of proof is on the deletion"; R2/R6/R7/R8 are the cases where the proof is weakest or the call is a standing Stevie/RFC decision, so they default to keep. Override ONLY if a row's evidence contradicts its rule (e.g. R9 package with `exported_refl > 0` → FLAGGED, ask in the table).

- [x] **Step 30** [W] ~20m: Write `$VERDICTS` (`docs/sweeps/REAL-STEEL-UNREACHED.tsv`): one row per package from `$SCRATCH/evidence.tsv`, plus non-Go rows (`dashboard/js/<file>` x23, `*.go.dev` files, `transport.Connect`, `pkg/health/aggregator.go`), each with group, evidence summary and recommendation from D-02. `verdict`, `commit` empty.
  - Done 2026-10-23: 178 rows (148 packages, 22+1 dashboard scripts, 5 .go.dev, transport.Connect, pkg/health/aggregator.go). D-02 applied with two readings recorded in the decision doc: R2 covers packages with test files only (they cannot have importers), and G10 is split into eight families (G10a-h) so Stevie can keep a family without row-by-row answers. Rule R8 beat the template's G5 for services/loadbalancer (maglev-ebpf), so it is FLAGGED (F6).
- [x] **Step 31** [V] ~2m: Every row has a group and a recommendation; no G10 row has `exported_refl > 0` without FLAGGED.
  - Done 2026-10-23: no row without group/recommendation; the three reflect rows (pkg/config, pkg/testing, services/kenoma) are FLAGGED (F4).
  ```bash
  awk -F'\t' 'NR>1 && ($2=="" || $4=="")' "$VERDICTS" | head; awk -F'\t' 'NR>1{print $2}' "$VERDICTS" | sort | uniq -c
  ```
- [x] **Step 32** [D] ~5m: Rows missing a group → re-read that package's doc comment and apply D-02 by hand. 2 attempts → mark the row FLAGGED (Stevie answers it in the table).
  - Done 2026-10-23: not needed.
- [x] **Step 33** [R] ~10m: Cross-edge check: a DELETE row must not be imported by a KEEP/WIRE/PARK row (`$SCRATCH/unreached-edges.txt`) or by any test of a kept package. Any conflict → demote the DELETE to FLAGGED with "imported by <x>".
  - Done 2026-10-23: 16 DELETE rows demoted to FLAGGED: imported by tests of live pkg/audit, pkg/secrets, pkg/storage (F1), by tests/e2e (F2), by tests/pqc (F3). Sub-packages imported only by a flagged parent go with its answer (G10f).
- [x] **Step 34** [B] ~2m: Non-compiler references for every DELETE row (ADR-090 "build passes is not sufficient"): BPF map names, `go:embed`, scripts, Makefile, Dockerfile, compose, systemd, Nix, Helm, docs that name the path.
  - Done 2026-10-23: 93 of 100 DELETE/MERGE rows have non-Go references; listed per row in the TSV notes.
  ```bash
  cd "$PROJECT_ROOT" && awk -F'\t' '$4=="DELETE"{print $1}' "$VERDICTS" | while read -r p; do d=${p#unheaded/}; \
    n=$(git grep -n -F "$d" -- ':!*.go' ':!docs/sweeps/*' ':!docs/UNREACHED-GO-PACKAGES.md' | wc -l); echo "$n $d"; done | sort -rn | head -30
  ```
  - Non-zero rows: add "references: <files>" to notes; the Phase 1 deletion of that row must edit those files in the same commit.
- [x] **Step 35** [W] ~15m: Write `docs/sweeps/REAL-STEEL-DECISIONS.md`: the one-pass decision table for Stevie (template below). One row per GROUP with the package list collapsed, counts, the recommendation, and an "answer" column; FLAGGED rows listed individually underneath. Link it from `docs/UNREACHED-GO-PACKAGES.md` and ADR-090's "Input gathered since" line.
  - Done 2026-10-23: docs/sweeps/REAL-STEEL-DECISIONS.md: 16 group rows (G10 split a-h) + 26 FLAGGED rows, Default column on every row, ADR-090 status question; linked from UNREACHED-GO-PACKAGES.md and ADR-090.

  Template:
  ```markdown
  | # | Group | Packages / files (count) | Evidence | Recommendation | Answer (Y = take rec / N = keep / P = park / W = wire) |
  |---|---|---|---|---|---|
  | G1 | Fake success, superseded | pkg/lxd (+real_client.go.dev), pkg/container/lxd/lxd.go.dev, pkg/nix (only caller of pkg/lxd), pkg/certs/acme, cmd/unheaded-daemon/internal/{lxd,state,config,ebpf} | S17 + Step 27 | DELETE | |
  | G2 | Orphan dashboard scripts | 23 dashboard/js files incl. monad-decode.js (invents register values/CRC) | Step 24 | DELETE (chaos/canary panels get rebuilt on real APIs later, not revived) | |
  | G3 | Deploy abstractions not used | pkg/deploy (Engine), pkg/deploy/pipeline, pkg/deploy/artifact, pkg/container (mock runtime), services/canary (after merge) | SITUATION | DELETE | |
  | G4 | Wired by this sprint | pkg/deploy/strategy, pkg/deploy/rollback, services/sword, services/yaldabaoth Service | Phases 7-11 | WIRE | |
  | G5 | Wotan dead paths | pkg/transport Connect/cascade.go, services/wotan/internal/circuitbreaker (if duplicate of client breaker), services/loadbalancer (xdpsync) | Steps 26, 13 | DELETE | |
  | G6 | Test-only | ... | Step 21 | KEEP | |
  | G7 | Bare metal / Sabatons | pkg/baremetal/*, services/sabatons | parked | PARK | |
  | G8 | Protocol reference | pkg/protocol/* | spec-adjacent | PARK (RFC Editor) | |
  | G9 | Control planes in waiting | services/{firewall,qos,nfv,version,anomaly,compliance,...} | ebpf/ counterparts | PARK + note | |
  | G10 | Unused libraries/services | (generated) | Step 21 | DELETE | |
  | F* | FLAGGED rows | one per row | | ask | |
  ```
- [x] **Step 36** [V] ~2m: Every TSV row appears in exactly one table group or FLAGGED line.
  - Done 2026-10-23: 178 unique rows, each in exactly one group or F line.
  ```bash
  awk -F'\t' 'NR>1{print $1}' "$VERDICTS" | sort -u | wc -l
  ```
- [x] **Step 37** [C] ~2m: **COMMIT CHECKPOINT**.
  ```bash
  commit "docs(sweep): REAL-STEEL unreached ledger and one-pass decision table" "30-37" \
    docs/sweeps/REAL-STEEL-UNREACHED.tsv docs/sweeps/REAL-STEEL-DECISIONS.md docs/UNREACHED-GO-PACKAGES.md docs/adr/ADR-090-total-source-sweep-slop-removal.md
  ```

### E1

- [ ] **Step 38** [ESCALATE] ~0m agent / one pass for Stevie: **E1, ADR-090 delete decisions.**
  - Done 2026-10-23: E1 posted, waiting on Stevie (docs/sweeps/REAL-STEEL-DECISIONS.md). Phase 1, Steps 91-92 and 233 wait; everything else continues.
  - **Context**: `docs/sweeps/REAL-STEEL-DECISIONS.md`, one table, about 12 group rows plus FLAGGED rows. Each group has a recommendation; "Y" on a row takes it.
  - **Questions**: (1) Answer column per group (Y/N/P/W). (2) FLAGGED rows individually. (3) ADR-090 status: accept it now (then D-01's override applies), or keep Proposed?
  - **Why Stevie**: deletions are his call (standing rule, and ADR-090 "re-decide documented retentions with Stevie"). The recommendations are pre-seeded, so this is one pass, not 148 questions.
  - **What stops**: Phase 1 only. Phases 2-16 do not depend on E1, except: Phase 2 Step 91 (transport.Connect) and Step 92 (xdpsync) read E1's G5 answer, and Phase 8 Step 233 reads G3 (canary delete after merge). Until E1 lands, those steps are [AFTER-E1] and skipped.
- [x] **Step 39** [B] ~2m: Post E1 to the handoff so it survives a session end.
  ```bash
  printf '\n## S-REAL-STEEL E1 (waiting on Stevie)\nAnswer the table in docs/sweeps/REAL-STEEL-DECISIONS.md (one pass).\n' >> ~/tmp/next.md
  ```
- [x] **Step 40** [V] ~1m: `grep -n 'S-REAL-STEEL E1' ~/tmp/next.md` returns one line.

### Exit

- [x] **Step 41** [B] ~2m: `go build ./... && go vet ./scripts/... 2>&1 | tail -3` (nothing deleted yet, so this must equal baseline).
- [x] **Step 42** [V] ~1m: Tree clean except this plan's STUCK/H-row edits.
  ```bash
  git -C "$PROJECT_ROOT" status --short
  ```
- [x] **Step 43** [C] ~1m: **COMMIT CHECKPOINT** (any remaining plan edits).
  ```bash
  git -C "$PROJECT_ROOT" diff --quiet -- "$PLAN" || commit "docs(battle-plan): S-REAL-STEEL phase 0 notes" "38-43" "$PLAN"
  ```
- [x] **Step 44** [V] ~2m: **PHASE 0 EXIT GATE**: H1-H15 resolved in the table; `$VERDICTS` and the decision table committed; E1 posted; baseline recorded.
  - Done 2026-10-23: PASS. H1-H15 resolved (all TRUE, H1 with the re-sign hash amendment); ledger + decision table committed (d1bd3dc7); E1 posted to ~/tmp/next.md; baseline 277 ok / 0 FAIL. Next: Phases 2, 3, 4.
  - Pass → Phase 1 when E1 is answered; meanwhile Phase 2 (and Phase 4 in parallel).
  - Fail → do not proceed; finish the missing artifact.

**DoD (Phase 0)**: DoD-STD + the decision table answerable in one pass (no row asks an open question without a recommendation, except FLAGGED rows, which each state what is unknown).

---

## PHASE 1: APPLY STEVIE'S SWEEP VERDICTS (Steps 45-66)

**Goal**: carry out E1 exactly: delete what Stevie said to delete, one coherent unit per commit, with behavioural equivalence shown; record verdicts in the ledger.
**Prerequisite**: E1 answered (Step 38). Phase 0 exit gate passed.
**Time**: 2-3 h
**Agent**: Agent (one worktree; deletions conflict with nothing else this sprint except Phase 7/8, which only touch G4 packages)

- [ ] **Step 45** [R] ~5m: Copy Stevie's answers into `$VERDICTS` (`verdict` column: DELETED-pending / KEEP / PARK / WIRE / FLAGGED-answered). Any answer that is not Y/N/P/W → [ESCALATE] back with the specific row (do not guess).
- [ ] **Step 46** [B] ~1m: Order the DELETE work: leaves first (no other DELETE row imports them), using `$SCRATCH/unreached-edges.txt`.
  ```bash
  awk -F'\t' '$5 ~ /^DELETED/{print $1}' "$VERDICTS" > "$SCRATCH/delete.txt"; wc -l < "$SCRATCH/delete.txt"
  ```
- [ ] **Step 47** [CODE] ~10m: **G1 unit**: `git rm -r` pkg/lxd (incl. real_client.go.dev), pkg/container/lxd/lxd.go.dev, pkg/nix (if G1 Y), pkg/certs/acme, cmd/unheaded-daemon/internal/{lxd,state,config,ebpf}. Edit every non-Go reference from Step 34 in the same change (wiki/docs lines that describe them as present).
- [ ] **Step 48** [BUILD][TEST] ~6m: Build and test the neighbours.
  ```bash
  cd "$PROJECT_ROOT" && go build ./... && go vet ./cmd/unheaded-daemon/... ./pkg/... 2>&1 | tail -5 && gotest ./cmd/unheaded-daemon/...
  ```
- [ ] **Step 49** [V] ~1m: Build green; Cuirass tests pass (its live LXD path is `lxd_rest.go`, untouched). Fail → Step 50.
- [ ] **Step 50** [D] ~5m: A reference survived: `go build ./... 2>&1 | head` names the importer. If the importer is a KEEP row, the deletion is wrong: restore with `git checkout HEAD -- <path>`, set the row FLAGGED ("imported by X"), continue with the next unit. 2 attempts → STUCK this unit.
- [ ] **Step 51** [C] ~2m: **COMMIT CHECKPOINT**.
  ```bash
  commit "chore(sweep): delete fake-success LXD/ACME clients and daemon internal scaffolding (E1 G1)" "47-51" "$PLAN" $(git -C "$PROJECT_ROOT" diff --name-only)
  ```
  (`git rm` already staged the deletions; the helper only adds the edited reference files and the plan.)
- [ ] **Step 52** [CODE] ~5m: **G2 unit**: `git rm` the 23 files in `$SCRATCH/js-orphans.txt` (only those Stevie answered Y).
- [ ] **Step 53** [TEST] ~4m: Dashboard JS tests + page load smoke (no page references them by construction, Step 24).
  ```bash
  cd "$PROJECT_ROOT" && gotest ./cmd/dashboard-backend/... && grep -rn -F -f <(sed 's/^/js\//' "$SCRATCH/js-orphans.txt") dashboard --include=*.html || echo "no references"
  ```
- [ ] **Step 54** [V] ~1m: Tests green; "no references".
- [ ] **Step 55** [C] ~2m: **COMMIT CHECKPOINT**: `chore(sweep): delete 23 unloaded dashboard scripts incl. monad-decode's invented values (E1 G2)`.
- [ ] **Step 56** [CODE] ~10m: **G5 unit** (if Y): delete `pkg/transport/cascade.go` `Connect` and anything only it uses; fix the stale comment in `cmd/sophia/main.go:847` and the "Every service uses transport.Connect()" doc line in `pkg/transport/transport.go:9`; delete `services/loadbalancer` if G5 includes it.
- [ ] **Step 57** [TEST] ~5m: `gotest ./pkg/transport/... ./pkg/wotan-client/... ./cmd/sophia/...`
- [ ] **Step 58** [V] ~1m: Green. CLAUDE.md's "pkg/transport/ implements the gRPC-first cascade" line is now false → note it for Phase 16 Step 398 (do not edit CLAUDE.md piecemeal).
- [ ] **Step 59** [C] ~2m: **COMMIT CHECKPOINT**: `chore(sweep): delete uncalled transport.Connect cascade (E1 G5)`.
- [ ] **Step 60** [CODE] ~20m: **G10 units**: delete per package family (one commit per family, e.g. `pkg/mesh/*`, `pkg/waf/*`, `pkg/storage/*`, `pkg/alerting/*`), leaves first. For each family: `git rm -r`, edit Step 34 references, `go build ./... && go vet ./...`.
- [ ] **Step 61** [TEST] ~10m: After each family: `go test -count=1 ./... 2>&1 | grep -E '^(FAIL|---)' | grep -v -F -f "$SCRATCH/baseline-tests.txt" || echo "no new failures"`.
- [ ] **Step 62** [V] ~1m: "no new failures" after every family. A new failure → restore that family (`git checkout HEAD -- <dirs>`), mark its rows FLAGGED with the failing test, continue.
- [ ] **Step 63** [C] ~2m: **COMMIT CHECKPOINT** per family: `chore(sweep): delete unused <family> (E1 G10)`.
- [ ] **Step 64** [REGEN] ~5m: Regenerate `docs/UNREACHED-GO-PACKAGES.md` with the Step 5 command (new count, list), keep its header, add a "Decided 2026-MM-DD (E1)" section listing PARK/KEEP groups with their one-line reasons; fill `commit` in `$VERDICTS`.
- [ ] **Step 65** [V] ~2m: **PHASE 1 EXIT GATE**: every Y row in E1 is either DELETED with a commit or FLAGGED with a reason; `go build ./...` green; no new test failures versus baseline; `$VERDICTS` has no empty `verdict`.
- [ ] **Step 66** [C] ~2m: **COMMIT CHECKPOINT**: `docs(sweep): record E1 verdicts; regenerate unreached list`.

**DoD (Phase 1)**: DoD-STD + zero behavioural regressions; every deletion commit names its E1 group; no deleted package is still described as present in wiki/ or docs/ (Step 34 references handled).

---

## PHASE 2: WOTAN CLIENT LEFTOVERS (Steps 67-98)

**Goal**: close the S17 Wotan client audit's open items so the services Sword and Yaldabaoth depend on report Wotan truthfully: the fallback queue drains or refuses, ProbeGRPC really probes, a dead stream is visible, and timeguru/architect recover when Wotan starts after them.
**Prerequisite**: Phase 0 exit gate. Independent of E1 except Steps 91-92.
**Time**: 3-4 h
**Agent**: Agent [P:3] [P:4]

### Fallback queue

- [x] **Step 67** [R] ~10m: Read `pkg/wotan-client/client.go` Publish (around line 487), `enqueueFallback` (578), `FlushFallbackQueue` (605) and the S17 Wotan audit commits for context (`git log --oneline -20 -- pkg/wotan-client`).
- [x] **Step 68** [DECIDE] ~0m: **D-10 fallback queue: retry, bounded in age, never for control topics.** RECOMMENDATION: keep buffering for telemetry/event topics, flush automatically, drop anything older than 2 min, and refuse to buffer control topics (`chaos.`, `config.`, `deploy.`, `cuirass.` prefixes): `Publish` returns the error for those. Rationale: a late `chaos.abort` or `deploy.*` replayed minutes after the fact is worse than a lost one, while a few minutes of metrics/events surviving a Wotan restart is what the queue was for; today it only fills and drops. Override ONLY if Stevie prefers "stop buffering" outright (then delete the queue and `FlushFallbackQueue`, make Publish return the network error; same tests minus the flush cases).
  - Done 2026-10-23: recommendation taken (no override: Stevie has not asked for 'stop buffering').
- [x] **Step 69** [CODE][TEST] ~25m: Tests first in `pkg/wotan-client/fallback_test.go` (httptest Wotan that returns 503/connection-refused, then 201):
  - Done 2026-10-23: fallback_test.go: the four named tests plus TestFallback_FlushRejoinsAfterWotanRestart (the flush rejoins when Wotan forgot the membership, which is exactly when the queue fills).
  - `TestFallback_FlushesInOrderAfterRecovery`: 3 publishes while down, recovery, all 3 arrive in order without another Publish call (background flush).
  - `TestFallback_ExpiresOlderThanMaxAge`: injected clock; an entry older than 2 min is dropped and counted as expired, not sent.
  - `TestFallback_ControlTopicsAreNotBuffered`: `chaos.abort` publish while down returns an error and the queue stays empty.
  - `TestFallback_CloseStopsFlusher`: no goroutine leak (`goleak` is not a dependency: count with `runtime.NumGoroutine` before/after with a settle loop).
- [x] **Step 70** [TEST] ~2m: Red.
  ```bash
  gotest ./pkg/wotan-client/ -run 'TestFallback_'
  ```
- [x] **Step 71** [CODE] ~30m: Implement: `pendingMessage.enqueuedAt`; `maxFallbackAge = 2*time.Minute`; `noBufferPrefixes`; a lazily started flusher goroutine (every 15 s while the queue is non-empty, plus an immediate kick after any successful publish or ProbeGRPC success) that stops on `Close`; counters `fallback{Queued,Flushed,Expired,Dropped}` exposed through the client's existing stats accessor and as `unheaded_wotan_client_fallback_total{result}` where the service registers client metrics.
  - Done 2026-10-23: fallback.go; 502/503/504 now count as 'Wotan unreachable' (buffered/retried) like a refused connection; the flush stops at the first network error so order holds; counters via FallbackStats() and unheaded_wotan_client_fallback_total{result=queued|flushed|expired|dropped|refused}. Bite: disabling the flusher, the control-topic guard, the age check or the rejoin each turns its test red.
- [x] **Step 72** [TEST][V] ~3m: Green, race-clean, whole package.
  - Done 2026-10-23: green, race-clean (53 s package); golangci-lint 0 issues; gosec clean.
  ```bash
  gotest ./pkg/wotan-client/...
  ```
- [x] **Step 73** [C] ~2m: **COMMIT CHECKPOINT**: `fix(wotan-client): flush the fallback queue, expire stale entries, never buffer control topics`.

### ProbeGRPC

- [x] **Step 74** [DECIDE] ~0m: **D-11 probe = grpc.health.v1 Check.** RECOMMENDATION: `ProbeGRPC` calls `healthpb.NewHealthClient(conn).Check` with a 2 s deadline and treats anything but SERVING as unhealthy. Rationale: Wotan already registers the health server (H11), the module is already in go.mod, and it is the standard probe. Override ONLY if H11 was FALSE: then call the cheapest existing Wotan RPC with a 2 s deadline.
  - Done 2026-10-23: recommendation taken (H11 TRUE). Asks the whole-server status (""), which grpc health.NewServer() sets SERVING.
- [x] **Step 75** [CODE][TEST] ~15m: Tests: `TestProbeGRPC_ServingIsHealthy`, `TestProbeGRPC_NotServingIsUnhealthy` (bufconn or 127.0.0.1:0 grpc server with `health.NewServer()` set to NOT_SERVING), `TestProbeGRPC_NothingListeningFailsWithinDeadline` (< 3 s).
  - Done 2026-10-23: probe_test.go: the three named tests plus TestProbeGRPC_SilentServerTimesOut (accepts, never answers: cut at 2 s).
- [x] **Step 76** [TEST] ~2m: Red (`-run TestProbeGRPC_`).
- [x] **Step 77** [CODE] ~15m: Implement; on failure set `grpcHealthy=false`, `transport=HTTP` (existing fields).
  - Done 2026-10-23: checkGRPCHealth; a successful probe also kicks the fallback flusher. The TestClient_GRPCStreamFallsBackToHTTP fixture now registers grpc.health.v1 like Wotan does. Bite: removing the Check turns three probe tests red.
- [x] **Step 78** [TEST][V] ~3m: Green; `go mod tidy -diff` prints nothing (no new module).
  - Done 2026-10-23: green, race-clean. `go mod tidy -diff` is NOT empty, but pre-existing: go.mod/go.sum are unchanged since f7d730e7 and the diff only drops gopkg.in/check.v1 (indirect) and stale kr/* sums; recorded in KNOWN FAILURES, not touched here (other worktrees edit go.mod).
  ```bash
  gotest ./pkg/wotan-client/... && (cd "$PROJECT_ROOT" && go mod tidy -diff && echo tidy-ok)
  ```
- [x] **Step 79** [C] ~2m: **COMMIT CHECKPOINT**: `fix(wotan-client): ProbeGRPC asks grpc.health.v1 instead of assuming`.

### Runtime stream health

- [x] **Step 80** [DECIDE] ~0m: **D-12 stream health = connection state plus stream liveness, not message recency.** RECOMMENDATION: each streaming subscriber records `alive` (its receive goroutine is running and the last Recv did not error) and the client reports `Health() {Transport, GRPCHealthy, Streams map[topic]bool, LastMessageAge map[topic]duration}`; gauge `unheaded_wotan_client_stream_up{topic}`. A service's `/health` includes it as detail and stays 200 (degraded is not dead); `/ready` is unchanged. Rationale: an idle topic is not a dead stream, so message age alone would cry wolf (ADR-100: no noise); a closed stream is the real failure the audit found invisible. Override ONLY if Wotan already sends stream keepalives (check `services/wotan` for a heartbeat on StreamMessages): then a keepalive age over 3x its period also marks the stream down.
  - Done 2026-10-23: recommendation taken: Wotan sends no keepalive on StreamTopics (only unary Ping/TopicPing), so the override does not apply.
- [x] **Step 81** [CODE][TEST] ~20m: Tests: `TestStreamHealth_ClosedStreamReportsDown` (server ends the stream; within 1 s `Health().Streams[topic]==false`), `TestStreamHealth_IdleStreamStaysUp`, `TestStreamHealth_FallbackToPollingReportsTransportHTTP`.
  - Done 2026-10-23: stream_health_test.go: the three named tests plus TestStreamHealth_HTTPPollingFollowsWotan (the HTTP-only client timeguru/architect use: down on a failing poll, up again, forgotten when the caller ends the stream).
- [x] **Step 82** [CODE] ~25m: Implement in the stream goroutine (set/clear an atomic flag around Recv; the HTTP-polling fallback path from S17 sets Transport=HTTP and Streams=up while polling succeeds).
  - Done 2026-10-23: stream_health.go + hooks: gRPC attempt is up on its first message or after 2 s open without an error, down on a failed open/Recv (never on the caller's own cancel); polling is up while polls answer. Client.Health() {transport, grpc_healthy, streams, last_message_age_seconds}; gauge unheaded_wotan_client_stream_up{topic}. Bite: never reporting down turns two tests red.
- [x] **Step 83** [TEST][V] ~3m: Green and race-clean: `gotest ./pkg/wotan-client/...`.
  - Done 2026-10-23: green, race-clean; lint 0; gosec clean.
- [x] **Step 84** [C] ~2m: **COMMIT CHECKPOINT**: `feat(wotan-client): report stream and transport health at runtime`.

### timeguru and architect stop giving up

- [x] **Step 85** [R] ~10m: Read `services/timeguru/main.go:64-75` and `services/architect/core.go:240-310`: list each one-shot Subscribe/Join that logs "disabled (degraded mode)" and never retries.
  - Done 2026-10-23: the deployed timeguru is services/timeguru/cmd/timeguru (Dockerfile, debs); its initWotan subscribed once with a 10 s ctx and on failure returned nil, so timeguru ran without Wotan for good. services/timeguru/main.go (the Nix build) did the same for timeline.updates. architect's Start subscribed once and returned the error; and cmd/architect called Start with a 10 s ctx it cancelled right after, which also killed the alerts.critical listener Start had just launched (a second bug, found here).
- [x] **Step 86** [CODE][TEST] ~20m: Tests: `TestTimeguru_SubscribesWhenWotanStartsLater` and `TestArchitect_SubscribesWhenWotanStartsLater`: start the service's subscription routine against a closed port, then bring up an httptest Wotan; assert a Join/Subscribe arrives within 5 s.
  - Done 2026-10-23: services/timeguru/cmd/timeguru/wotan_test.go, services/architect/wotan_retry_test.go (+ TestArchitect_HealthIncludesWotanDetail, TestHandleHealth_IncludesWotanDetail for the D-12 /health detail). Red first (compile), then bite: a one-shot SubscribeWithRetry turns both red.
- [x] **Step 87** [CODE] ~25m: Implement one helper in `pkg/wotan-client` (`SubscribeWithRetry(ctx, topic, name, onMsg)`: 1 s → 30 s exponential backoff with jitter, stops on ctx) and use it at every site from Step 85. Keep the degraded-mode log, once, at the first failure.
  - Done 2026-10-23: pkg/wotan-client/subscribe_retry.go (onMsg nil = join only); timeguru (both mains) and architect use it; architect main passes its long-lived ctx; /health of both carries the client's Health() as 'wotan' and stays 200. TestArchitectService_Start_SubscribeError now asserts the new contract (nil, retrying).
- [x] **Step 88** [TEST][V] ~3m: Green.
  - Done 2026-10-23: green; the two remaining 'disabled (degraded mode)' lines in architect fire only when there is no Wotan client at all (mock mode), not on a failed join; lint 0, gosec clean.
  ```bash
  gotest ./services/timeguru/... ./services/architect/... ./pkg/wotan-client/...
  grep -n 'disabled (degraded mode)' "$PROJECT_ROOT"/services/{timeguru,architect}/*.go
  ```
  - The grep may still print the log line, but every hit must sit in a retry loop (read them).
- [x] **Step 89** [C] ~2m: **COMMIT CHECKPOINT**: `fix(timeguru,architect): keep retrying Wotan subscriptions instead of giving up at start`.
- [x] **Step 90** [B] ~3m: Same pattern anywhere else? `grep -rn 'wotan unavailable' --include=*.go "$PROJECT_ROOT"/services "$PROJECT_ROOT"/cmd | grep -v _test`. Record the remaining sites in next.md as a follow-on list (Marshal: do not widen this phase to all services).
  - Done 2026-10-23: remaining live sites (micromanager, monad, sophia, gateway; yaldabaoth for Phase 10) listed in ~/tmp/next.md; not widened here.

### E1-dependent items

- [ ] **Step 91** [AFTER-E1] ~5m: `transport.Connect`: if E1 G5 = Y it was deleted in Phase 1 (nothing to do). If N (keep), write one line in next.md ("transport.Connect kept by Stevie; unused; finish when a caller appears") and move on: no phase needs it.
  - Done 2026-10-23: [AFTER-E1] skipped: waits on E1 G5.
- [ ] **Step 92** [AFTER-E1] ~20m: xdpsync (`services/loadbalancer/xdpsync.go:102`): if deleted in Phase 1, nothing to do. If kept/parked: test `TestXDPSync_JoinsBeforePublish` (publishes `lb.rebalance` only after Join as `loadbalancer`), fix the name (`xdpsync-service` → `loadbalancer`), add `loadbalancer` to `configs/wotan.yaml` `auto_approve`, `gotest ./services/loadbalancer/...`.
  - Done 2026-10-23: [AFTER-E1] skipped: waits on E1 (services/loadbalancer is F6, suggested P).

### Live proof on a scratch Wotan (no host change)

- [x] **Step 93** [BUILD] ~3m: Build a scratch Wotan and timeguru.
  - Done 2026-10-23: built ./services/timeguru/cmd/timeguru (the binary the Dockerfile and debs ship), not ./services/timeguru (the Nix-only main), plus the scratch Wotan.
  ```bash
  cd "$PROJECT_ROOT" && go build -o "$SCRATCH/wotan" ./services/wotan/cmd/wotan && go build -o "$SCRATCH/timeguru" ./services/timeguru
  ```
- [x] **Step 94** [LIVE-RO] ~10m: Start timeguru first, pointed at a Wotan that is not running yet (ports 28000/28001, user space, high ports, no root), wait 10 s, then start the scratch Wotan with a temp config whose `auto_approve` holds `timeguru`. Within 40 s Wotan's membership/stats endpoint lists timeguru's subscription.
  - Done 2026-10-23: env is WOTAN_ADDR/WOTAN_GRPC_ADDR/PORT/DB_PATH (scratch DB), Wotan flags --http-port/--grpc-port/--topic-config, memory store (no disk). Wotan has no stats/members endpoint; proof = Wotan's topic_subscriber_created log + timeguru's new /health detail.
  ```bash
  cp "$PROJECT_ROOT/configs/wotan.yaml" "$SCRATCH/wotan.yaml"
  WOTAN_ADDR=127.0.0.1:28000 WOTAN_GRPC_ADDR=127.0.0.1:28001 PORT=29000 "$SCRATCH/timeguru" >"$SCRATCH/tg.log" 2>&1 & echo $! > "$SCRATCH/tg.pid"
  sleep 10; "$SCRATCH/wotan" --config "$SCRATCH/wotan.yaml" --http-addr 127.0.0.1:28000 --grpc-addr 127.0.0.1:28001 >"$SCRATCH/w.log" 2>&1 & echo $! > "$SCRATCH/w.pid"
  sleep 40; curl -fsS 127.0.0.1:28000/api/v1/stats | jq '.'
  ```
  - Flag names are from the binaries' `--help`; check them first (`"$SCRATCH/wotan" --help | head -30`), they are not guessed here.
- [x] **Step 95** [V] ~1m: timeguru appears as a member/subscriber; its log shows one degraded line and then a success line. Fail → Step 96.
  - Done 2026-10-23: PASS. Wotan down: timeguru /health 200 with streams {}; one WARN per topic. Wotan up 10 s later: within 5 s Wotan logged topic_subscriber_created for timeline.updates and alerts.critical as timeguru (approved), /health streams {alerts.critical:true, timeline.updates:true}, and the startup timeline_loaded announcement reached Wotan (message_count 1). Found: the recovery INFO line only printed when a stream ended; fixed in the same commit (logged on join).
- [ ] **Step 96** [D] ~5m: Check timeguru's env names (`grep -n Getenv services/timeguru/main.go`) and Wotan's allowlist rejection in `w.log`. 2 attempts → STUCK (unit tests from Step 86 still stand; the live proof moves to Phase 5 Step 151).
  - Done 2026-10-23: not needed.
- [x] **Step 97** [B][V] ~2m: **PHASE 2 EXIT GATE**: stop the scratch processes; all four sub-sections green; no new test failures.
  - Done 2026-10-23: PASS: scratch processes stopped; all four sub-sections green and race-clean; no new failures.
  ```bash
  kill "$(cat "$SCRATCH/tg.pid")" "$(cat "$SCRATCH/w.pid")" 2>/dev/null; gotest ./pkg/wotan-client/... ./services/timeguru/... ./services/architect/...
  ```
- [x] **Step 98** [C] ~2m: **COMMIT CHECKPOINT** (plan ticks + next.md follow-on list).

**DoD (Phase 2)**: DoD-STD + each fix has a test that fails when the fix is reverted (bite-test one of them: revert the ProbeGRPC change locally, see red, restore).

---

## PHASE 3: WOTAN REPLICATION SNAPSHOT, ONLY WHAT IS NEEDED (Steps 99-108)

**Goal**: stop `TransferSnapshot` claiming a complete empty snapshot; record the real work against ADR-064 without starting it.
**Prerequisite**: Phase 0 exit gate (H12 known).
**Time**: 45 min
**Agent**: Agent [P:2]

- [x] **Step 99** [R] ~10m: Read `services/wotan/internal/cluster/replication_server.go` and `replication_client.go`: confirm the standby catches up with `CatchUp` from a sequence and never asks for a snapshot (H12).
  - Done 2026-10-23: H12 holds: replication_client.go only calls CatchUp(since lastApplied) and Ping; nothing calls TransferSnapshot.
- [x] **Step 100** [DECIDE] ~0m: **D-13 snapshot = honest Unimplemented.** RECOMMENDATION: return `status.Error(codes.Unimplemented, "snapshot transfer not implemented; catch up from seq 0 (ADR-064)")`. Rationale: ADR-064 is Proposed and nobody calls the RPC; the only harm today is that the stub answers `Final: true, TotalSize: 0, LastSeq: <current>`, which a future caller would read as "you are up to date". Implementing transfer belongs to ADR-064 Phase 1 once Stevie starts it. Override ONLY if H12 was FALSE (a caller exists): then implement chunked transfer of the WAL from its first retained sequence, with a test that a fresh standby reaches the primary's `lastSeq` and identical message bodies.
  - Done 2026-10-23: recommendation taken (H12 TRUE).
- [x] **Step 101** [CODE][TEST] ~10m: `TestTransferSnapshot_IsUnimplemented` (status code Unimplemented, no chunk sent) and keep/extend the existing catch-up test so it starts from seq 0.
  - Done 2026-10-23: replication_server_test.go (bufconn gRPC over a real WAL in a temp dir): TestTransferSnapshot_IsUnimplemented and TestCatchUp_FromZeroDeliversEverything (there was no catch-up test to extend; the package had none for the server).
- [x] **Step 102** [TEST] ~2m: Red: `gotest ./services/wotan/internal/cluster/ -run TestTransferSnapshot`.
  - Done 2026-10-23: red: the stub sent final:true last_seq:3.
- [x] **Step 103** [CODE] ~5m: Implement (remove the TODO; the log line stays at Info with the peer).
- [x] **Step 104** [TEST][V] ~3m: Green: `gotest ./services/wotan/internal/cluster/...`.
  - Done 2026-10-23: green, race-clean; lint 0; gosec clean.
- [x] **Step 105** [W] ~5m: ADR-064: under its phase list, one line: "Snapshot transfer: RPC answers Unimplemented since S-REAL-STEEL (was an empty Final chunk); implementing it is a Phase 1 prerequisite."
- [x] **Step 106** [C] ~2m: **COMMIT CHECKPOINT**: `fix(wotan): TransferSnapshot answers Unimplemented instead of an empty final snapshot`.
- [ ] **Step 107** [V] ~1m: **PHASE 3 EXIT GATE**: test green; ADR-064 line present (`grep -n 'Unimplemented since S-REAL-STEEL' docs/adr/ADR-064-*.md`).
- [ ] **Step 108** [AUDIT-UPDATE] ~2m: next.md Wotan open list: mark the snapshot TODO as "answered honestly, implementation waits on ADR-064".

**DoD (Phase 3)**: DoD-STD.

---

## PHASE 4: CUIRASS EXTENSIONS FOR DEPLOY AND CHAOS (Steps 109-136)

**Goal**: give Cuirass the three things deploy and chaos need, all inside its existing safe mode: a real HTTP health probe, a managed proxy device (the stable endpoint blue-green switches), and an allowlisted out-of-band stop/kill for chaos. Unit-tested against a fake LXD; nothing live yet.
**Prerequisite**: Phase 0 exit gate. (E2 not needed: no host contact.)
**Time**: 4-5 h
**Agent**: Agent [P:2] [P:6]

- [ ] **Step 109** [R] ~15m: Read `cmd/unheaded-daemon/lxd_rest.go` (client, `requireManaged`, `CreateContainer`), `lxd_rest_test.go` (fake LXD server pattern), `main.go` `planReconcile`/`applyAction`/`handleContainer`, `desired_store.go`, and wiki/Containers.md "Cuirass LXD backend".
- [ ] **Step 110** [DECIDE] ~0m: **D-03 immutable, versioned instances.** RECOMMENDATION: a deployed instance is named `<svc>-<ver>-<n>` (e.g. `steel-echo-v2-0`, versions sanitized to `[a-z0-9-]`, validated by `validLXDName`) and never updated in place. Rolling = create new, wait healthy, delete old, one at a time; rollback = the same in reverse. `OnInstanceUpdate` is implemented as create+delete. Rationale: Cuirass already knows create/start/delete safely; in-place image swaps need an "image drift" action Cuirass does not have and LXD rebuilds that lose the old instance (no cheap rollback). Override ONLY if LXD storage on west is too small for two copies of the workload (check `lxc storage info default` in Step 140).
- [ ] **Step 111** [DECIDE] ~0m: **D-04 traffic switch = a Cuirass-managed LXD proxy device; canary split is client-side.** RECOMMENDATION: `ContainerSpec.Proxy {Listen, Connect}` becomes device `unheaded-proxy` on exactly one instance per service (the "active" one). Blue-green cutover = move the device from blue to green (remove, then add; the gap is measured in Phase 9). Canary = stable and canary instances both run; the game-day load generator splits requests by weight and canary analysis reads each instance's own `/metrics`. Rationale: lxdbr0 is a plain bridge, so LXD network load-balancers (OVN only) are unavailable; HAProxy/pkg/loadbalancer would be a second sprint. Override ONLY if Stevie asks for server-side weighted split: then add a follow-on (HAProxy runtime API weights) to next.md; nothing in this phase changes.
- [ ] **Step 112** [CODE][TEST] ~20m: Health spec tests (`cmd/unheaded-daemon/health_probe_test.go`): `ContainerSpec.Health {Port int, Path string}` validation refuses port 0/65536, a path not starting with `/`, containing `..`, `?`-less absolute URLs (`http://`), or longer than 256; probe uses only the instance IP Cuirass read from LXD (never a caller-supplied host), 2 s timeout, no redirects followed, body discarded after 4 KiB; `TestReconcile_ThreeFailedProbesMarkUnhealthyAndPlanRestart`; `TestReconcile_NoHealthSpecStaysUnknown`.
- [ ] **Step 113** [TEST] ~2m: Red: `gotest ./cmd/unheaded-daemon/ -run 'Health|Probe'`.
- [ ] **Step 114** [CODE] ~40m: Implement: a prober (injectable for tests) run per sync for running managed instances with a Health spec; 3 consecutive failures → `Health="unhealthy"` (the existing plan then restarts it, enforce mode only); 1 success → `healthy`. Metric `cuirass_container_health{id}` (1/0/-1 unknown).
- [ ] **Step 115** [TEST][V] ~3m: Green, race-clean: `gotest ./cmd/unheaded-daemon/...`.
- [ ] **Step 116** [C] ~2m: **COMMIT CHECKPOINT**: `feat(cuirass): HTTP health probe for managed instances (instance IP only, no redirects)`.
- [ ] **Step 117** [CODE][TEST] ~25m: Proxy device tests (`proxy_device_test.go`): `Listen` must be `tcp:127.0.0.1:<port>` or `tcp:<west lab addr>:<port>` with port in the user-apps band 26000-26666 (`pkg/ports`), `Connect` must be `tcp:127.0.0.1:<1-65535>`; two desired specs with the same Listen are refused at PUT (409); `TestReconcile_ProxyDriftPlansUpdate` (actual device differs → action `update-proxy`); `TestLXDREST_UpdateProxyTouchesOnlyOurDevice` (fake LXD instance with an unrelated `eth0` device and a user device keeps both; read-modify-write with the instance ETag in `If-Match`; 412 → retry once, then error); refused on an unmanaged instance.
- [ ] **Step 118** [TEST] ~2m: Red: `gotest ./cmd/unheaded-daemon/ -run 'Proxy'`.
- [ ] **Step 119** [CODE] ~45m: Implement: parse `devices` in `lxdInstance` (only `unheaded-proxy` is surfaced into `ContainerState.Proxy`), `UpdateProxy(ctx, name, *ProxySpec)` via GET (+ETag) then PUT, new plan action `update-proxy` applied only in enforce mode, observe mode logs `planned: true` like the others.
- [ ] **Step 120** [TEST][V] ~3m: Green: `gotest ./cmd/unheaded-daemon/...`.
- [ ] **Step 121** [C] ~2m: **COMMIT CHECKPOINT**: `feat(cuirass): managed proxy device as the service's stable endpoint`.
- [ ] **Step 122** [CODE][TEST] ~25m: Chaos action tests (`chaos_action_test.go`), endpoint `POST /api/v1/containers/{id}/chaos` body `{"action":"stop"|"kill","reason":"..."}`:
  - 403 unless `AUTH_ENABLED=true` (same rule as PUT/DELETE, `04c34ed0`);
  - 403 unless `CUIRASS_CHAOS=true` (off by default; switched on only for game-day windows);
  - 409 on an unmanaged instance, and 409 unless the instance carries label `chaos=allowed` (`user.unheaded.label.chaos`, set through the desired spec's Labels);
  - `stop` = state stop with `force:false`, `kill` = `force:true`;
  - every call (allowed or refused) publishes `cuirass.chaos` and logs one line with id, action, reason, caller subject;
  - the reconciler is not told: in enforce mode it sees `status != running` on the next sync and starts the instance again (that is the auto-revert, proven live in Phase 11).
- [ ] **Step 123** [TEST] ~2m: Red: `gotest ./cmd/unheaded-daemon/ -run 'Chaos'`.
- [ ] **Step 124** [CODE] ~30m: Implement the handler + `lxdREST.ChaosStop(ctx, name, force)` (calls `requireManaged` and checks the label itself: two independent checks, the S17 pattern).
- [ ] **Step 125** [TEST][V] ~3m: Green: `gotest ./cmd/unheaded-daemon/...`.
- [ ] **Step 126** [C] ~2m: **COMMIT CHECKPOINT**: `feat(cuirass): allowlisted chaos stop/kill, off unless CUIRASS_CHAOS=true`.
- [ ] **Step 127** [CODE][TEST] ~15m: Metric `cuirass_reconcile_actions_total{action,result}` (result = done|failed|planned|refused) with a test reading `/metrics`; `/api/v1/info` gains `lxd_backend`, `lxd_project`, `enforce`, `prune`, `chaos` (today it reports none of them, so nobody can tell mock from real without reading the env). Chaos injectors (Phase 11) refuse to start unless `enforce` is true.
- [ ] **Step 128** [TEST][V] ~2m: Green.
- [ ] **Step 129** [SECURITY] ~20m: BlackMage pass on the three additions; write findings into the phase notes (this file):
  - SSRF: can any input steer the probe off the instance IP? (desired spec has no host field; IP comes from LXD state only.)
  - Device injection: can a desired spec add any device other than `unheaded-proxy`, or a proxy listening on 0.0.0.0 / a low port / outside the band? Can `Connect` point outside the container?
  - Chaos endpoint: can it reach pihole (unmanaged, other project), or any instance without the label? Is the refusal logged?
  - Body limits: `pkg/auth` DecodeJSONBody 413 path is used for the new handler.
- [ ] **Step 130** [B] ~5m: gosec on the package (the house ratchet: rule exclusions, never severity filters).
  ```bash
  cd "$PROJECT_ROOT" && gosec -quiet ./cmd/unheaded-daemon/... 2>&1 | tail -15
  ```
- [ ] **Step 131** [V] ~1m: No new gosec findings versus `git stash`-free comparison: run the same command at `HEAD~6` in a temporary worktree (`git worktree add "$SCRATCH/wt-base" HEAD~6`), diff the rule IDs, remove the worktree.
- [ ] **Step 132** [D] ~10m: New finding → fix it (preferred) or justify with a leading `#nosec G<id> -- reason` (memory: reference_gosec_nosec_directive: `#nosec` must lead the comment). 2 attempts → STUCK.
- [ ] **Step 133** [W] ~10m: wiki/Containers.md: Health, Proxy, Chaos fields; env `CUIRASS_CHAOS`; the two independent managed checks; how a game-day window turns chaos on and off.
- [ ] **Step 134** [C] ~2m: **COMMIT CHECKPOINT**: `docs(wiki): Cuirass health, proxy and chaos actions`.
- [ ] **Step 135** [V] ~3m: **PHASE 4 EXIT GATE**: `gotest ./cmd/unheaded-daemon/...` green; the safe-mode tests from `04c34ed0` still green (run the names `grep -ln -i pihole cmd/unheaded-daemon/*_test.go` points at, with `-run` on their `func Test...` names); default `LXD_BACKEND` still `mock`, `CUIRASS_CHAOS` default off (`grep -n 'CUIRASS_CHAOS\|"LXD_BACKEND", "mock"' cmd/unheaded-daemon/main.go`).
- [ ] **Step 136** [C] ~1m: **COMMIT CHECKPOINT** (plan ticks + Step 129 notes).

**DoD (Phase 4)**: DoD-STD + the three new safety refusals each have a bite test (remove the guard, test goes red, restore).

---

## PHASE 5: CUIRASS AGAINST REAL LXD ON WEST (Steps 137-160)

**Goal**: the trust script applied, Cuirass on the LXD HTTPS API in project `unheaded`, observe first, then enforce, with pihole provably untouched. Also the live half of Phase 2.
**Prerequisite**: Phase 4 exit gate; **E2 answered**.
**Time**: 2-3 h (plus Stevie's turnaround)
**Agent**: Coordinator [SEQ] [BARE-METAL]

- [ ] **Step 137** [ESCALATE] ~0m agent / one pass for Stevie: **E2, west live window.**
  - **Context**: next.md S17: "Needs Stevie's yes (Cuirass): `scripts/cuirass-lxd-trust.sh --apply` on west, then compose env (LXD_BACKEND=https, host.docker.internal) observe-only." The stack is down by his choice ("main stack stays manual at boot").
  - **Questions (answer each yes/no, one pass):**
    1. Bring the main compose stack up on west for live windows of this sprint, and stop it at the end of each window? (Recommended: yes, window model.)
    2. Run `scripts/cuirass-lxd-trust.sh --apply` (project `unheaded` restricted, P-384 client cert for uid 16740, restricted trust, server-cert pin)?
    3. Cuirass env on west: `LXD_BACKEND=https`, `LXD_PROJECT=unheaded`, observe first; then `CUIRASS_ENFORCE=true` + `CUIRASS_PRUNE=true` (prune can only see project `unheaded`, where every instance is ours); `AUTH_ENABLED=true` with an API key from a host env file outside the repo; `CUIRASS_CHAOS=true` only during Phase 11/14 windows?
    4. Import the `steel-echo` images (v1, v2, v3 and a deliberately bad v4) into LXD (Phase 6; built from an Alpine base, so west fetches `images:alpine/3.22` once)?
    5. Sword as a compose service (Phase 8); chaos-controller as a host systemd unit under a new sysusers account `unheaded-chaos` with `CAP_NET_ADMIN`, plus `CAP_BPF` on west only for the Phase 13 lab (Phases 10-13)?
    6. sudo on west for a lab netns pair `steel0/steel1` and attaching `yaldabaoth-ebpf` TC egress to the lab veth only, never a physical NIC (Phases 11, 13)?
    7. Run the Phase 9 deploy proof and the Phase 14 game day on west (LXD project `unheaded` only; pihole lives in `default` and is invisible to Cuirass)?
  - **Why Stevie**: every item changes west; items 1-3 are his standing gate. The recommendation is yes to all with the window model; he can trim.
  - **What stops**: Phases 5, 6 (live import only), 9, 11, 13 (live parts), 14 (west half). Phases 7, 8, 10 and the code halves of 6/11/13 continue.
- [ ] **Step 138** [LIVE-RO][BARE-METAL] ~3m: Dry run, read the output end to end.
  ```bash
  cd "$PROJECT_ROOT" && scripts/cuirass-lxd-trust.sh 2>&1 | tee "$SCRATCH/trust-dry.txt"
  ```
- [ ] **Step 139** [LIVE-RW][S] ~5m: Apply (E2.2).
  ```bash
  cd "$PROJECT_ROOT" && sudo scripts/cuirass-lxd-trust.sh --apply 2>&1 | tee "$SCRATCH/trust-apply.txt"
  ```
- [ ] **Step 140** [V][LIVE-RO] ~3m: Project and trust are restricted; storage has room for two copies (D-03 override check).
  ```bash
  lxc project show unheaded | grep -E 'restricted|features'
  lxc config trust list --format csv | grep -i unheaded
  lxc storage info default | grep -E 'space used|total space'
  ```
  - Fail → Step 141.
- [ ] **Step 141** [D] ~5m: Re-run the dry run; each step skips itself when done, so the diff shows what did not apply. Compare with `scripts/cuirass-lxd-trust.sh --help`. 2 attempts → STUCK (blocks this phase and 9/11/14 west halves).
- [ ] **Step 142** [W] ~10m: `docker/compose.cuirass-lxd.yml` (repo, no secrets): cuirass env `LXD_BACKEND=https`, `LXD_URL=https://host.docker.internal:<port from core.https_address>`, `LXD_PROJECT=unheaded`, `LXD_CLIENT_CERT/KEY/SERVER_CERT` from `/etc/unheaded/cuirass` mounted read-only, `extra_hosts: host.docker.internal:host-gateway`, `env_file: /etc/unheaded/cuirass/cuirass.env` (holds `AUTH_API_KEYS`, created by hand on west, 0600 root:16740). Enforce/prune/chaos stay unset here (observe).
- [ ] **Step 143** [LIVE-RW] ~5m: Window open (E2.1): bring the stack up with the override.
  ```bash
  cd "$PROJECT_ROOT" && sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d 2>&1 | tail -5
  ```
- [ ] **Step 144** [V][LIVE-RO] ~2m: Cuirass healthy and connected.
  ```bash
  curl -fsS "$CUIRASS/health"; curl -fsS "$CUIRASS/metrics" | grep -E '^cuirass_lxd_up'
  ```
  - `cuirass_lxd_up 1` → Step 145. `0` → `sudo docker logs unheaded-cuirass 2>&1 | tail -20`: pin mismatch, cert path, or host.docker.internal resolution; 2 attempts → STUCK.
- [ ] **Step 145** [PREFLIGHT][LIVE-RO] ~3m: **H16** (and the core safety proof): Cuirass sees an empty project; pihole is invisible; can the restricted identity list images?
  ```bash
  curl -fsS "$CUIRASS/api/v1/state/actual" | jq '.'
  lxc list --project default --format csv -c ns | grep pihole
  ```
  - `actual` must be `{}` / empty list (no pihole). Then record H16 by reading `/1.0/images?project=unheaded` with the Cuirass cert (`curl --cert ... --key ... --cacert <pinned> https://127.0.0.1:<port>/1.0/images?project=unheaded`): the answer decides who imports images in Phase 6 (D-15).
- [ ] **Step 146** [LIVE-RW] ~3m: Observe mode: desire a throwaway instance.
  ```bash
  curl -fsS -X PUT -H "Authorization: Bearer $CUIRASS_API_KEY" -H 'Content-Type: application/json' \
    "$CUIRASS/api/v1/containers/steel-smoke" -d '{"id":"steel-smoke","name":"steel-smoke","image":"images:alpine/3.22","labels":{"sprint":"real-steel"}}'
  sleep 35; curl -fsS "$CUIRASS/api/v1/state" | jq '.. | objects | select(.container_id? == "steel-smoke")'
  ```
- [ ] **Step 147** [V][LIVE-RO] ~1m: A `create` action with `planned: true`; `lxc list --project unheaded` is empty.
- [ ] **Step 148** [LIVE-RW] ~5m: Enforce on (E2.3): add `CUIRASS_ENFORCE: "true"` and `CUIRASS_PRUNE: "true"` to the override, `docker compose ... up -d cuirass`, wait one sync (30 s).
- [ ] **Step 149** [V][LIVE-RO] ~2m: Real instance, ours, and pihole untouched.
  ```bash
  lxc list --project unheaded --format csv -c ns; lxc config get --project unheaded steel-smoke user.unheaded.managed
  lxc list --project default --format csv -c ns | grep pihole
  ```
  - `steel-smoke,RUNNING`, `true`, `pihole,RUNNING`.
- [ ] **Step 150** [LIVE-RW] ~3m: Remove it the managed way: `curl -X DELETE ... "$CUIRASS/api/v1/containers/steel-smoke"`, wait one sync; `lxc list --project unheaded` empty.
- [ ] **Step 151** [LIVE-RO] ~5m: Phase 2 on the real stack: `sudo docker restart unheaded-wotan`; within 60 s timeguru's and architect's subscriptions are back (Wotan members/stats endpoint) and their `/health` detail shows `streams` up again.
- [ ] **Step 152** [V] ~1m: Both re-subscribed without a restart of their own. Fail → back to Phase 2 Step 87 with the live log (S2 Marshal: this is a regression of a passed gate).
- [ ] **Step 153** [LIVE-RO] ~2m: East is unchanged by design (truth check: nothing in this phase touched east). East runs the pre-S17 binary, so its `/api/v1/info` has no `lxd_backend` field; read the unit's environment instead.
  ```bash
  ssh "$EAST" 'systemctl show -p Environment -p EnvironmentFiles unheaded-unheaded-daemon; curl -fsS 127.0.0.1:17000/health'
  curl -fsS "$CUIRASS/api/v1/info" | jq '{lxd_backend, lxd_project, enforce, prune, chaos}'
  ```
  - East: no `LXD_BACKEND=https`; west: `https`, `unheaded`, `true`, `true`, `false`.
- [ ] **Step 154** [C] ~2m: **COMMIT CHECKPOINT**: `feat(deploy): compose override for Cuirass on LXD HTTPS (project unheaded)`; body cites Steps 145 and 149 as the live proof.
- [ ] **Step 155** [W] ~10m: wiki/Containers.md "Live on west since <date>": project, enforce/prune, the empty-project proof, how to switch back to mock.
- [ ] **Step 156** [V][LIVE-RO] ~2m: No LXD operation touched the default project during the window.
  ```bash
  lxc operation list --project default --format csv 2>/dev/null | head; lxc info --project default pihole | grep -E 'Status|Started'
  ```
  - pihole's `Started` time is older than Step 139.
- [ ] **Step 157** [LIVE-RW] ~3m: Window close (E2.1 window model): `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml stop`. Skip if Stevie said to leave it up.
- [ ] **Step 158** [V] ~1m: **PHASE 5 EXIT GATE**: Steps 144, 145, 149, 152, 156 all passed and are cited in the commit.
- [ ] **Step 159** [C] ~1m: **COMMIT CHECKPOINT** (wiki + plan).
- [ ] **Step 160** [AUDIT-UPDATE] ~3m: next.md: "Cuirass reconciles for real on west (project unheaded), enforce+prune, <date>; east still mock". Remove the "Needs Stevie's yes (Cuirass)" line.

**DoD (Phase 5)**: DoD-STD + pihole untouched (Steps 149, 156) + rollback path documented (switch the override off → mock).

---

## PHASE 6: THE GAME-DAY WORKLOAD, steel-echo (Steps 161-178)

**Goal**: a tiny, honest workload to deploy and break: four LXD images (v1, v2, v3, a deliberately bad v4) of one static Go binary that serves health, version and Prometheus metrics, plus a load generator that measures availability from the client side.
**Prerequisite**: Phase 0 exit gate (code); Phase 5 exit gate + E2.4 (live import).
**Time**: 3 h
**Agent**: Agent [P:4] (code), Coordinator (Steps 170-176)

- [ ] **Step 161** [DECIDE] ~0m: **D-14 image base = Alpine 3.22 + OpenRC + the static binary.** RECOMMENDATION: build each image from `images:alpine/3.22`: push the binary, an OpenRC service file and `/etc/conf.d/steel-echo` (version-specific fault knobs), `rc-update add`, then `lxc publish` to an alias. Rationale: Alpine brings the DHCP client and init the container needs on lxdbr0, and it stays small; a bare rootfs with the binary as `/sbin/init` would need its own network setup. Override ONLY if west cannot reach the simplestreams server: then build a rootfs tarball (binary as init, IPv6 SLAAC only) and `lxc image import` it, and record H17 against IPv6.
- [ ] **Step 162** [CODE][TEST] ~40m: `cmd/steel-echo` tests first (`main_test.go`):
  - `serve` (default): `GET /health` 200 `ok`; `GET /version` the ldflags `-X main.version`; `GET /` returns hostname and version only (never echoes request data: zero user data access); `GET /metrics` exposes `unheaded_http_requests_total` / `unheaded_http_request_duration_seconds` through `pkg/httputil` middleware (the house metrics, Session 4); fault knobs `STEEL_ERROR_RATE` (0-1, fraction of `/` answered 500) and `STEEL_LATENCY_MS`; SIGTERM and SIGPWR shut down cleanly (LXD stop sends SIGPWR to init's children via OpenRC).
  - `load`: `--target URL` (fixed) or `--discover $CUIRASS --svc NAME` (every 2 s, the healthy managed instances whose label `svc=NAME`, round-robin, so traffic share = replica share); `--rps`, `--duration`, `--out CSV` with one line per second: `ts,sent,ok,err5xx,errconn,p50_ms,p99_ms,versions_seen`. Discover mode reads only `/api/v1/state/actual` (GET, no key needed).
  - Tests: knob parsing bounds, error-rate within ±5% over 10k requests (fixed seed), discover refresh drops a removed instance within one period, CSV columns.
- [ ] **Step 163** [TEST] ~2m: Red: `gotest ./cmd/steel-echo/...`.
- [ ] **Step 164** [CODE] ~45m: Implement (stdlib + existing `pkg/httputil`/`pkg/metrics` only; no new module).
- [ ] **Step 165** [TEST][V] ~3m: Green; static build works.
  ```bash
  gotest ./cmd/steel-echo/... && (cd "$PROJECT_ROOT" && CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=v1" -o "$SCRATCH/steel-echo" ./cmd/steel-echo && file "$SCRATCH/steel-echo")
  ```
  - `statically linked`.
- [ ] **Step 166** [C] ~2m: **COMMIT CHECKPOINT**: `feat(steel-echo): game-day workload with fault knobs and a client-side availability meter`.
- [ ] **Step 167** [W] ~30m: `scripts/steel-echo-image.sh` (dry run unless `--apply`, the house pattern): for `v1 v2 v3` (error rate 0, latency 0) and `v4` (`STEEL_ERROR_RATE=0.3`): build the binary with its version, `lxc launch images:alpine/3.22 steel-build-<ver>` (project from D-15), `lxc file push`, write the OpenRC service and conf.d, `rc-update add steel-echo default`, `lxc stop`, `lxc publish steel-build-<ver> --alias steel-echo-<ver>`, `lxc delete steel-build-<ver>`. Refuses to run if any alias already exists unless `--replace`. Never touches an instance not named `steel-build-*`.
- [ ] **Step 168** [B][V] ~3m: shellcheck clean; dry run prints the exact command list.
  ```bash
  shellcheck "$PROJECT_ROOT/scripts/steel-echo-image.sh" && "$PROJECT_ROOT/scripts/steel-echo-image.sh" | head -40
  ```
- [ ] **Step 169** [C] ~2m: **COMMIT CHECKPOINT**: `feat(scripts): steel-echo LXD image builder (dry run by default)`.
- [ ] **Step 170** [DECIDE] ~0m: **D-15 who imports images** (reads H16, Step 145). RECOMMENDATION: if the restricted Cuirass identity cannot import, govan builds in the `default` project (the `unheaded` project shares the default project's images: trust script step 1), with builder names prefixed `steel-build-` so pihole is never in scope. Rationale: keeps Cuirass's identity read/run-only for images. Override ONLY if H16 showed the project has its own image store (`features.images=true`): then build with `--project unheaded`.
- [ ] **Step 171** [LIVE-RW][BARE-METAL] ~15m: (E2.4) `scripts/steel-echo-image.sh --apply 2>&1 | tee "$SCRATCH/images.txt"`.
- [ ] **Step 172** [V][LIVE-RO] ~2m: Four aliases, no builders left, pihole untouched.
  ```bash
  lxc image list --format csv -c lf | grep steel-echo; lxc list --all-projects --format csv -c n | grep steel-build || echo "no builders"
  lxc list --project default --format csv -c ns | grep pihole
  ```
- [ ] **Step 173** [LIVE-RW] ~5m: Window open (stack + override, Phase 5 Step 143 command). Desire one instance through Cuirass with health, proxy and the chaos label.
  ```bash
  curl -fsS -X PUT -H "Authorization: Bearer $CUIRASS_API_KEY" -H 'Content-Type: application/json' "$CUIRASS/api/v1/containers/steel-echo-v1-0" -d '{
    "id":"steel-echo-v1-0","name":"steel-echo-v1-0","image":"steel-echo-v1",
    "labels":{"svc":"steel-echo","ver":"v1","chaos":"allowed"},
    "health":{"port":8080,"path":"/health"},
    "proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"}}'
  sleep 70; curl -fsS "$CUIRASS/api/v1/state/actual" | jq '."steel-echo-v1-0"'; curl -fsS 127.0.0.1:26601/version
  ```
- [ ] **Step 174** [PREFLIGHT][V][LIVE-RO] ~2m: **H17**: `health: "healthy"` in Cuirass's actual state proves a compose container (Cuirass) reached the instance IP on lxdbr0; `/version` through the proxy says `v1`. Fail on health with the proxy working → H17 FALSE → Step 175.
- [ ] **Step 175** [D] ~10m: From the host `curl http://<ip>:8080/health` works but Cuirass's probe does not: docker's FORWARD chain or the compose network cannot route to lxdbr0. Inspect (`sudo iptables -S DOCKER-USER`, `ip route` inside the container via `docker exec unheaded-cuirass ip route` if present). Do not edit host firewall rules (not in E2). Record H17 FALSE, apply D-09's fallback for Sword, and STUCK the Cuirass probe item for Stevie (blocks Phase 9 health gating). 2 attempts max.
- [ ] **Step 176** [V][LIVE-RO] ~2m: Instance metrics are real: `curl -fsS http://<ip>:8080/metrics | grep unheaded_http_requests_total | head -3` after a few `/` requests.
- [ ] **Step 177** [V] ~1m: **PHASE 6 EXIT GATE**: four images; one healthy managed instance serving through its proxy; load generator CSV works against it for 30 s (`steel-echo load --target http://127.0.0.1:26601/ --rps 10 --duration 30s --out "$SCRATCH/smoke.csv"`, 300 ok).
- [ ] **Step 178** [C] ~2m: **COMMIT CHECKPOINT** (plan ticks; H17 row). Leave `steel-echo-v1-0` desired for Phase 9 or close the window per E2.1.

**DoD (Phase 6)**: DoD-STD + the workload never echoes request content (test in Step 162).

---

## PHASE 7: DEPLOY CORE, NO MORE SIMULATION (Steps 179-208)

**Goal**: `pkg/deploy/strategy` and `pkg/deploy/rollback` drive real instances through a Cuirass driver; a strategy without a driver fails instead of inventing instances.
**Prerequisite**: Phase 4 exit gate (Cuirass API shape). Live Cuirass not required (fake Cuirass in tests).
**Time**: 5-6 h
**Agent**: Agent

- [ ] **Step 179** [R] ~20m: Read `pkg/deploy/strategy/{strategy,rolling,bluegreen,canary}.go` (Callbacks at strategy.go:198) and `pkg/deploy/rollback/rollback.go` (interfaces at :154-186).
- [ ] **Step 180** [DECIDE] ~0m: **D-05 one deploy core.** RECOMMENDATION: `pkg/deploy/strategy` (+ `pkg/deploy/rollback`) is the core; `strategy.Callbacks` is the driver contract; `pkg/deploy` Engine, `pkg/deploy/pipeline` and `pkg/deploy/artifact` are not used (they are E1 group G3). Rationale: the strategy package already has the right seam and real control flow; the other two are parallel abstractions over a mock runtime, and building a third would be the "second system" the Marshal cites. Override ONLY if E1 answered G3 = W (wire): then still build on `strategy`, and adapt `pipeline.InstanceManager` to the same driver in a follow-on.
- [ ] **Step 181** [CODE][TEST] ~20m: `TestStrategies_NilDriverFails` (table over rolling, blue-green, canary, recreate: nil `OnInstanceCreate`/`OnInstanceDelete`/`OnHealthCheck` → `ErrNoDriver`, no `Instance` invented, metrics show 0 created); `TestBlueGreen_RequiresTrafficShift`; `TestCanary_RequiresAnalysis`.
- [ ] **Step 182** [TEST] ~2m: Red: `gotest ./pkg/deploy/strategy/ -run 'NilDriver|Requires'`.
- [ ] **Step 183** [CODE] ~40m: `Validate`/`Execute` require the callbacks each strategy needs; delete every "Simulate instance creation" / invented-instance branch.
- [ ] **Step 184** [TEST][V] ~3m: Green, and no simulation left.
  ```bash
  gotest ./pkg/deploy/strategy/... ./pkg/deploy/rollback/...
  grep -n -i 'simulat' "$PROJECT_ROOT"/pkg/deploy/strategy/*.go "$PROJECT_ROOT"/pkg/deploy/rollback/*.go | grep -v _test || echo "none"
  ```
- [ ] **Step 185** [D] ~10m: Existing tests relied on the simulation: rewrite them to pass a test driver (in `_test.go`, a fake is fine; the product path is what must not fake). 2 attempts → STUCK with the failing test names.
- [ ] **Step 186** [C] ~2m: **COMMIT CHECKPOINT**: `fix(deploy): strategies fail without a driver instead of inventing instances`.
- [ ] **Step 187** [CODE][TEST] ~30m: `pkg/deploy/cuirassdriver` tests first, against an httptest fake Cuirass that keeps desired/actual maps and flips instances to running/healthy after N polls:
  - `TestDriver_CreatePutsVersionedInstanceAndWaitsHealthy` (name `<svc>-<ver>-<n>`, image `<svc>-<ver>`, labels `svc`, `ver`, `deployment`, plus the caller's; Health spec passed through; returns IP).
  - `TestDriver_CreateTimesOutWhenNeverHealthy` (ctx deadline, error names the instance).
  - `TestDriver_DeleteRemovesDesired`.
  - `TestDriver_UpdateIsCreateThenDelete` (D-03).
  - `TestDriver_TrafficShiftMovesProxyOnce` (old spec PUT without `proxy`, then new spec PUT with it, then one reconcile-now call).
  - `TestDriver_RefusesWithoutAPIKey` (env `CUIRASS_API_KEY` unset → constructor error: credentials fail loud, never a default).
  - `TestDriver_HealthIsCuirassHealth` (the driver does not probe on its own: one source of truth).
- [ ] **Step 188** [TEST] ~2m: Red: `gotest ./pkg/deploy/cuirassdriver/...`.
- [ ] **Step 189** [CODE] ~60m: Implement `Driver` returning a `strategy.Callbacks`; HTTP client with timeouts, `pkg/auth`-compatible bearer key, JSON bodies capped, IDs validated with the same rule as Cuirass (`[a-z0-9-]`, LXD name length).
- [ ] **Step 190** [TEST][V] ~3m: Green, race-clean.
- [ ] **Step 191** [C] ~2m: **COMMIT CHECKPOINT**: `feat(deploy): Cuirass driver for deploy strategies`.
- [ ] **Step 192** [CODE][TEST] ~20m: Cuirass `POST /api/v1/reconcile` (auth required like PUT; at most one run per second, extra calls get 429; runs one sync+plan+apply cycle and returns its actions): tests in `cmd/unheaded-daemon/reconcile_now_test.go`. Without it a blue-green cutover waits up to one 30 s sync.
- [ ] **Step 193** [TEST][V] ~3m: `gotest ./cmd/unheaded-daemon/...` green.
- [ ] **Step 194** [C] ~2m: **COMMIT CHECKPOINT**: `feat(cuirass): reconcile-now endpoint for deploy cutovers`.
- [ ] **Step 195** [CODE][TEST] ~25m: Rollback wiring tests: `rollback.Manager` with a `DeploymentLookup` backed by a history store (interface only here; Sword provides the file-backed one in Phase 8), `InstanceUpdater` and `TrafficShifter` adapters over the driver. `TestRollback_RestoresPreviousVersionThroughDriver` (fake Cuirass ends with only the previous version's instances desired and the proxy on them), `TestRollback_NoPreviousVersionIsAnError`, `TestAutoRollbackMonitor_TriggersOnFailedHealth`.
- [ ] **Step 196** [TEST] ~2m: Red.
- [ ] **Step 197** [CODE] ~40m: Implement the adapters (`pkg/deploy/cuirassdriver/rollback.go`).
- [ ] **Step 198** [TEST][V] ~3m: Green.
- [ ] **Step 199** [C] ~2m: **COMMIT CHECKPOINT**: `feat(deploy): rollback manager drives the Cuirass driver`.
- [ ] **Step 200** [TEST][CODE] ~30m: End-to-end strategy tests against the fake Cuirass (`pkg/deploy/cuirassdriver/e2e_test.go`):
  - rolling 3 replicas v1→v2 with `max_unavailable=0`: at every fake tick, healthy count ≥ 3;
  - blue-green v2→v3: exactly one proxy move, old instances deleted only after the move;
  - canary v3→v4 with analysis returning fail: ends with only v3 desired, canary deleted, result `rolled_back`.
- [ ] **Step 201** [TEST] ~3m: Run: `gotest ./pkg/deploy/...`.
- [ ] **Step 202** [V] ~1m: All green; no `time.Sleep` in the product code paths of strategy/driver except bounded poll intervals (`grep -n 'time.Sleep' pkg/deploy/strategy/*.go pkg/deploy/cuirassdriver/*.go | grep -v _test`).
- [ ] **Step 203** [C] ~2m: **COMMIT CHECKPOINT**: `test(deploy): end-to-end strategy runs against a fake Cuirass`.
- [ ] **Step 204** [B] ~2m: Untouched leftovers are what E1 says they are: if G3 was answered, `pkg/deploy` Engine/pipeline/artifact are gone (Phase 1); if not answered yet, they stay unreached and unmodified (no half-edits).
- [ ] **Step 205** [B] ~3m: gosec on the new/changed packages: `gosec -quiet ./pkg/deploy/strategy/... ./pkg/deploy/rollback/... ./pkg/deploy/cuirassdriver/... | tail`.
- [ ] **Step 206** [V] ~2m: **PHASE 7 EXIT GATE**: Steps 184, 190, 198, 202 green; zero gosec findings in new code.
- [ ] **Step 207** [C] ~1m: **COMMIT CHECKPOINT** (plan ticks).
- [ ] **Step 208** [AUDIT-UPDATE] ~2m: `docs/UNREACHED-GO-PACKAGES.md`: `pkg/deploy/strategy`, `pkg/deploy/rollback` marked "wired in Phase 8 (Sword)" (they become reached when cmd/sword lands).

**DoD (Phase 7)**: DoD-STD + the "no simulate" grep is a test (`TestNoSimulationInDeployCore` reading the package sources) so it cannot regress.

---

## PHASE 8: SWORD, THE REAL DEPLOY SERVICE (Steps 209-236)

**Goal**: a `cmd/sword` binary that runs deployments and rollbacks through the Phase 7 core, persists its history, analyses canaries from real instance metrics with an ADR-100 Kingdom-burn guard, and folds the old Canary service in.
**Prerequisite**: Phase 7 exit gate.
**Time**: 5-6 h
**Agent**: Agent

- [ ] **Step 209** [R] ~15m: Read `services/sword/sword.go` (types, routes, `Rollback` at :250) and `services/canary/canary.go` (SLOThreshold, routes, `evaluateCanary` at :583, `checkSLO` at :686).
- [ ] **Step 210** [DECIDE] ~0m: **D-08 Sword is the binary; Canary folds in; Sword takes port 19011.** RECOMMENDATION: new `cmd/sword` hosting `services/sword`; canary analysis becomes a Sword strategy input; `pkg/ports.Canary` (19011) is renamed `Sword` (no duplicate ports, `TestNoDuplicatePorts`); uid `pkg/uids.Sword` (16746) already exists. Rationale: one deploy service, one port, no third copy of canary logic. Override ONLY if Stevie wants Canary as a separate service: then Sword takes the next free port in 19020-19099 after `grep -n '= 190[2-9][0-9]' pkg/ports/ports.go`.
- [ ] **Step 211** [DECIDE] ~0m: **D-06 canary analysis reads the instances, guarded by the Kingdom SLOs.** RECOMMENDATION: every analysis interval Sword scrapes each canary and stable instance's `/metrics` (IP from Cuirass actual state, port from the Health spec; same SSRF rules as Cuirass) and computes error ratio and p99 from counter/histogram deltas; fewer than `min_samples` requests = inconclusive (hold, never promote; three inconclusive intervals = rollback); separately, if any `page: true` SLO's `slo:error_ratio:5m` exceeds `14.4 × (1 − objective)` (ADR-100 fast tier) or its series is missing, analysis fails. Rationale: Victoria's scrape config does not know dynamic LXD instances, so the instance is the source of truth for the canary; the SLO guard keeps a deploy from burning real budget elsewhere. Override ONLY if Victoria gains LXD service discovery in this sprint (it should not: scope).
- [ ] **Step 212** [DECIDE] ~0m: **D-07 Sword state = `pkg/statefile` under `SWORD_DATA_DIR`.** RECOMMENDATION: the S17 pattern (4affa68e): no data dir → mutating routes answer 503; damaged file → fatal at start; failed save → 503 and the in-memory change undone. Rationale: rollback needs the previous version after a restart; ADR-099's Well migration is a follow-on. Override ONLY if the Well is up and Stevie asks for it there now.
- [ ] **Step 213** [DECIDE] ~0m: **D-09 Sword runs as a compose service on west.** RECOMMENDATION: compose service `sword` (uid 16746, `read_only`, `cap_drop: ALL`, `no-new-privileges`, named volume `sword-data`, `env_file` on the host for `CUIRASS_API_KEY`, networks `control` + `data`). Rationale: it talks to Cuirass and Victoria (compose) and instance IPs (H17, Step 174). Override ONLY if H17 was FALSE: then a host unit `deploy/systemd/unheaded-sword.service` with its own sysusers account `unheaded-sword`, same sandbox block as the other units.
- [ ] **Step 214** [CODE][TEST] ~30m: `services/sword` tests: `TestSword_DeployRunsStrategyThroughDriver` (no sleep path), `TestSword_RollbackRestoresPreviousVersion` (through `rollback.Manager`), `TestSword_FailedDeployRollsBackAutomatically`, `TestSword_HistorySurvivesRestart` (statefile), `TestSword_NoDataDirIsReadOnly` (503), `TestSword_PublishesDeployEvents` (topics `deploy.started|progress|succeeded|failed|rolled_back`, joined as `sword` before publishing: the S17 "published without joining" bug).
- [ ] **Step 215** [TEST] ~2m: Red: `gotest ./services/sword/...`.
- [ ] **Step 216** [CODE] ~60m: Rewrite `services/sword` around `strategy.Registry` + driver + `rollback.Manager` + statefile; delete the simulated `Rollback`; routes `POST /api/v1/deployments`, `GET /api/v1/deployments[/{id}]`, `POST /api/v1/deployments/{id}/rollback`, `/health`, `/ready` (ready = data dir writable + Cuirass reachable), `/metrics`.
- [ ] **Step 217** [TEST][V] ~3m: Green, race-clean.
- [ ] **Step 218** [C] ~2m: **COMMIT CHECKPOINT**: `feat(sword): deployments and rollbacks through the real deploy core, persisted`.
- [ ] **Step 219** [CODE][TEST] ~25m: `services/sword/analysis_test.go`: `TestAnalysis_ErrorRatioFromCounterDeltas` (two fake scrapes), `TestAnalysis_P99FromHistogramDeltas`, `TestAnalysis_TooFewSamplesIsInconclusive`, `TestAnalysis_ThreeInconclusiveRollsBack`, `TestAnalysis_KingdomFastBurnFails` (fake Victoria above 14.4× budget rate), `TestAnalysis_MissingSLOSeriesFails`.
- [ ] **Step 220** [TEST] ~2m: Red.
- [ ] **Step 221** [CODE] ~45m: Implement `services/sword/analysis.go` (port `SLOThreshold` from canary; drop its hardcoded metrics) and wire it as the canary strategy's `OnAnalysis`.
- [ ] **Step 222** [TEST][V] ~3m: Green.
- [ ] **Step 223** [C] ~2m: **COMMIT CHECKPOINT**: `feat(sword): canary analysis from real instance metrics with an SLO burn guard`.
- [ ] **Step 224** [CODE] ~30m: `cmd/sword/main.go`: env `HTTP_ADDR` (default `:19011` from `pkg/ports`), `CUIRASS_URL`, `CUIRASS_API_KEY` (fail loud), `VM_URL`, `SWORD_DATA_DIR`, `WOTAN_ADDR`/`WOTAN_GRPC_ADDR`; `pkg/auth` middleware (mutating routes 403 unless `AUTH_ENABLED`, the Cuirass rule); `discovery.Start(..., "sword", ...)`; `unheaded_http_*` + `sword_deployments_total{strategy,result}`, `sword_rollbacks_total{reason}`; graceful shutdown.
- [ ] **Step 225** [BUILD][TEST] ~5m: `cd "$PROJECT_ROOT" && go build ./cmd/sword && gotest ./cmd/sword/... ./pkg/ports/...`.
- [ ] **Step 226** [C] ~2m: **COMMIT CHECKPOINT**: `feat(sword): cmd/sword on 19011 (was Canary's port)`.
- [ ] **Step 227** [W] ~30m: Packaging: Dockerfile target `sword` (numeric USER 16746, the house pattern); compose service per D-09; `configs/wotan.yaml` `auto_approve` += `sword`; Victoria `scrape.yml` job `sword`; `deploy/systemd/unheaded-sword.service` + sysusers line (for the deb/host path, same sandbox block as the others, not installed anywhere in this sprint).
- [ ] **Step 228** [V] ~5m: Packaging checks.
  ```bash
  cd "$PROJECT_ROOT" && sudo docker compose -f docker-compose.yml config -q && echo compose-ok
  systemd-analyze verify deploy/systemd/unheaded-sword.service 2>&1 | tail -3
  ls scripts/check-*.sh | head -20   # run the compose/units/uids checks that exist, e.g.:
  bash scripts/check-systemd-units.sh 2>&1 | tail -3
  ```
- [ ] **Step 229** [C] ~2m: **COMMIT CHECKPOINT**: `build(sword): image, compose service, unit, Wotan allowlist, scrape job`.
- [ ] **Step 230** [W] ~15m: `wiki/Service-Sword.md` (routes, env, state, strategies, how rollback picks the previous version, the SLO guard); link from `_Sidebar` and the services index.
- [ ] **Step 231** [C] ~2m: **COMMIT CHECKPOINT**: `docs(wiki): Service-Sword`.
- [ ] **Step 232** [B] ~2m: `services/sword` and `pkg/deploy/{strategy,rollback,cuirassdriver}` are now reached: rerun Step 5's command; they must be absent from the unreached list.
- [ ] **Step 233** [AFTER-E1] ~10m: If E1 G3 = Y: `git rm -r services/canary`, fix references (wiki, ports comments), `go build ./...`, commit `chore(sweep): delete services/canary, folded into Sword (E1 G3)`. If N: leave it, add a deprecation line in its package doc pointing to Sword.
- [ ] **Step 234** [V] ~2m: **PHASE 8 EXIT GATE**: Steps 217, 222, 225, 228 green; Step 232 shows Sword reached.
- [ ] **Step 235** [C] ~1m: **COMMIT CHECKPOINT** (plan ticks).
- [ ] **Step 236** [AUDIT-UPDATE] ~2m: ADR-088: one line under the substrate ladder: "LXD substrate: Sword deploys into LXD through Cuirass (S-REAL-STEEL)".

**DoD (Phase 8)**: DoD-STD + Sword never reports success without a Cuirass-confirmed healthy instance (test asserts on the fake's state, not on Sword's own bookkeeping).

---

## PHASE 9: DEPLOY, LIVE ON WEST (Steps 237-258)

**Goal**: prove rolling, blue-green, automatic canary rollback and manual rollback on real LXD instances, measured from the client side.
**Prerequisite**: Phases 5, 6, 8 exit gates; E2.7.
**Time**: 2-3 h
**Agent**: Coordinator [SEQ] [BARE-METAL]

- [ ] **Step 237** [LIVE-RW] ~5m: Window open: stack + Cuirass override (enforce, prune, chaos off) + Sword: `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d --build sword cuirass`.
- [ ] **Step 238** [V][LIVE-RO] ~2m: Sword `/health` and `/ready` 200; Cuirass `/api/v1/info` shows `https`, `unheaded`, enforce true.
- [ ] **Step 239** [LIVE-RW] ~5m: Baseline: deploy `steel-echo` v1 with 3 replicas (`recreate`; removes any leftover from Phase 6 the managed way).
  ```bash
  deploy() { curl -fsS -X POST -H "Authorization: Bearer $SWORD_API_KEY" -H 'Content-Type: application/json' "$SWORD/api/v1/deployments" -d "$1" | jq '{id,status}'; }
  deploy '{"service":"steel-echo","version":"v1","replicas":3,"strategy":{"type":"recreate"},
           "health":{"port":8080,"path":"/health"},"proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},
           "labels":{"chaos":"allowed"}}'
  ```
- [ ] **Step 240** [V][LIVE-RO] ~3m: `lxc list --project unheaded -c ns --format csv` shows `steel-echo-v1-{0,1,2}` RUNNING; Cuirass health all `healthy`; `curl 127.0.0.1:26601/version` = v1.
- [ ] **Step 241** [B] ~1m: Start the meter for the whole phase.
  ```bash
  "$SCRATCH/steel-echo" load --discover "$CUIRASS" --svc steel-echo --rps 20 --duration 60m --out "$SCRATCH/phase9.csv" & echo $! > "$SCRATCH/load.pid"
  ```
- [ ] **Step 242** [LIVE-RW] ~10m: Rolling v1→v2 (`max_surge 1, max_unavailable 0`); note start/end timestamps from Sword's deployment record.
- [ ] **Step 243** [V][LIVE-RO] ~3m: Rolling result: Sword `succeeded`; only `steel-echo-v2-*` desired and running; across the rollout window the CSV shows availability ≥ 99.0% (`ok / sent`) and `versions_seen` only v1/v2.
  ```bash
  awk -F, -v s="$T0" -v e="$T1" 'NR>1 && $1>=s && $1<=e {sent+=$2; ok+=$3} END {printf "availability %.4f (%d/%d)\n", ok/sent, ok, sent}' "$SCRATCH/phase9.csv"
  ```
  - Below 99% → Step 244.
- [ ] **Step 244** [D] ~10m: Errors cluster at instance deletions (the 2 s discover refresh): confirm with `errconn` column; if so, add drain-before-delete (driver removes the `svc` label, waits one discover period, then deletes) as a Phase 7 amendment and re-run 242. 2 attempts → STUCK with the CSV attached.
- [ ] **Step 245** [LIVE-RW] ~8m: Blue-green v2→v3 with a second meter on the stable endpoint (proxy).
  ```bash
  "$SCRATCH/steel-echo" load --target http://127.0.0.1:26601/ --rps 20 --duration 5m --out "$SCRATCH/bg.csv" &
  deploy '{"service":"steel-echo","version":"v3","replicas":3,"strategy":{"type":"blue_green"},"health":{"port":8080,"path":"/health"},"proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},"labels":{"chaos":"allowed"}}'
  ```
- [ ] **Step 246** [V][LIVE-RO] ~3m: In `bg.csv` the version flips v2→v3 exactly once; the cutover gap (seconds with `errconn > 0` around the flip) is ≤ 2 s; blue instances deleted after the flip. Record the gap.
- [ ] **Step 247** [C] ~2m: **COMMIT CHECKPOINT**: plan ticks with the measured numbers (availability, gap).
- [ ] **Step 248** [LIVE-RW] ~10m: Canary v3→v4 (the bad build, 30% errors): `{"type":"canary","steps":[{"weight":33,"pause":"60s","analysis":true},{"weight":100}],"analysis":{"error_rate_threshold":0.01,"min_sample_size":200,"interval":"30s"}}`.
- [ ] **Step 249** [V][LIVE-RO] ~3m: Sword status `rolled_back`, reason `analysis_failed` with the measured error ratio (≈0.30) in the record; canary instance gone; only v3 desired; `sword_rollbacks_total{reason="analysis_failed"}` = 1; time from canary healthy to rollback complete recorded.
- [ ] **Step 250** [LIVE-RO] ~2m: Kingdom guard stayed quiet: `vmq 'slo:error_ratio:5m'` for wotan/dashboard/well equals the pre-window values (no new burn from the deploy).
- [ ] **Step 251** [LIVE-RW] ~5m: Manual rollback of the blue-green deployment (Step 245's id): `POST /api/v1/deployments/<id>/rollback`.
- [ ] **Step 252** [V][LIVE-RO] ~2m: Back on v2 (instances, proxy `/version`, Sword record `rolled_back`, reason `manual`).
- [ ] **Step 253** [LIVE-RO] ~3m: Sword restart keeps history: `sudo docker restart unheaded-sword`, then `GET /api/v1/deployments` lists all five records.
- [ ] **Step 254** [B] ~2m: Stop the meters; copy the CSVs to `docs/gamedays/data/phase9-<date>/` (numbers only: timestamps, counts, latencies; no payloads).
- [ ] **Step 255** [V][LIVE-RO] ~2m: pihole untouched (`lxc info --project default pihole | grep Started` older than the window).
- [ ] **Step 256** [LIVE-RW] ~2m: Window close per E2.1 (leave `steel-echo` v2 desired; LXD keeps it running with Cuirass stopped).
- [ ] **Step 257** [V] ~1m: **PHASE 9 EXIT GATE**: Steps 243, 246, 249, 252, 253, 255 passed with numbers recorded.
- [ ] **Step 258** [C] ~2m: **COMMIT CHECKPOINT**: `test(deploy): live rolling, blue-green, canary auto-rollback and manual rollback on west` (body: the measured numbers and step citations).

**DoD (Phase 9)**: DoD-STD + every number in the commit comes from the CSVs or Sword's records.

---

## PHASE 10: YALDABAOTH SAFETY ENVELOPE (Steps 259-288)

**Goal**: before any real fault exists, the engine that runs faults is safe: allowlisted blast radius, three abort paths, a journal that reverts every injection even after a crash, and a steady-state hypothesis built on ADR-100 SLOs and real probes. `chaos-controller` runs `yaldabaoth.Service`.
**Prerequisite**: Phase 0 exit gate (code only; no host needed). Phase 4 (Cuirass chaos API shape).
**Time**: 6-7 h
**Agent**: Agent (Steps 259-282), Coordinator (283-288)

- [ ] **Step 259** [R] ~20m: Read `services/yaldabaoth/yaldabaoth.go` (Experiment, Target, SteadyState, Probe, SafetyCheck, Rollback; executeExperiment at :420), `gameday.go` (:165-300), `audit.go`, and `cmd/chaos-controller/main.go`.
- [ ] **Step 260** [DECIDE] ~0m: **D-16 one engine.** RECOMMENDATION: `chaos-controller` constructs `yaldabaoth.Service` and every fault, including the existing `ChaosFaultRule` API (DROP/DELAY/CORRUPT/DUPLICATE) and GameDay stages, runs as an `Experiment` through it; the controller's own rule map and the scheduler's "would undo here" path go away. Rationale: one safety envelope; today there are two engines and neither touches anything. Override ONLY if a GameDay stage needs an experiment type the Service cannot express: add the type, do not bypass the Service.
- [ ] **Step 261** [DECIDE] ~0m: **D-17 blast-radius allowlist.** RECOMMENDATION: `/etc/unheaded/chaos/allowlist.yaml` (repo seed `configs/chaos/allowlist.yaml`), checked before injection and again by the injector itself:
  - `lxd`: instances in project `unheaded` carrying label `chaos=allowed` (Cuirass checks this too);
  - `veth`: only the host-side veth of an allowlisted LXD instance, resolved from Cuirass's actual state (`host_veth`, Phase 11), never a name taken from the request;
  - `netns`: names matching `^steel[0-9]+$` only (the Phase 11/13 lab);
  - `systemd`: an explicit unit list, and only when `os.Hostname()` is `east`;
  - `packet`: flow labels on lab interfaces `^steel[0-9]+-h$` only;
  - limits: 1 concurrent experiment, 10 min max duration, 50% max loss, 2 s max added delay.
  Refusals are audited (`AuditTrail`) and published on `chaos.refused`. Rationale: a chaos engine is a weapon (BlackMage); the default answer is no. Override ONLY by editing the allowlist file on the host (never by request fields).
- [ ] **Step 262** [DECIDE] ~0m: **D-18 abort switch, three independent paths.** RECOMMENDATION: (1) `POST /api/v1/chaos/abort` (auth), (2) Wotan `chaos.abort` from an allowlisted publisher, (3) the file `/run/unheaded/chaos/ABORT` (works with Wotan and HTTP auth both down: `touch` as root or `unheaded-chaos`). Any path cancels every running experiment and reverts. The file also refuses new experiments until removed. Rationale: the abort must survive the failure it is aborting. Override ONLY if a path cannot be made reliable: then document it as best-effort, never remove the file path.
- [ ] **Step 263** [DECIDE] ~0m: **D-19 auto-revert journal.** RECOMMENDATION: before acting, each injector writes a journal entry (`pkg/statefile`, `CHAOS_STATE_DIR`, 0600) with its revert recipe; revert runs at experiment end, on abort, on a per-injection TTL timer, at process start (replay leftovers), and from the unit's `ExecStopPost=/usr/local/bin/chaos-controller revert-all` (runs after crashes and SIGKILL). An entry is removed only after the revert is verified (e.g. `tc -j qdisc show` no longer lists handle `c4a0:`). A revert that cannot be verified raises `chaos.revert_failed` and keeps the entry. Rationale: a fault that outlives its controller is the worst outcome of chaos engineering. Override ONLY never.
- [ ] **Step 264** [DECIDE] ~0m: **D-20 steady state from ADR-100.** RECOMMENDATION: the hypothesis has two parts:
  - **Target probes** (real HTTP GETs to allowlist-resolved addresses only, expected status, timeout): success ratio ≥ the experiment's threshold before injection; during the experiment they measure impact rather than trigger abort, unless the experiment sets `abort_below`.
  - **Kingdom SLO guard**: for every `page: true` SLO in `observability/alerting/policy.yaml`, `slo:error_ratio:5m` below `14.4 × (1 − objective)`. Exceeding it aborts. A missing series before start refuses to start (ADR-100: missing counts as bad).

  Checked before injection (must hold), every `SafetyCheckInterval` during (abort on the guard), and after revert (the verdict). `runProbe`'s hardcoded `Success: true` and `shouldAbort`'s `return false` are deleted. Rationale: "we broke the target on purpose" is fine; "we started burning a real SLO" is the line. Override ONLY if Victoria is down: then no experiment starts (not "skip the guard").
- [ ] **Step 265** [CODE][TEST] ~20m: Probe tests: `TestRunProbe_RealHTTPStatus` (httptest 200 vs 500 vs timeout), `TestRunProbe_RefusesNonAllowlistedTarget`, `TestRunProbe_NoRedirects`.
- [ ] **Step 266** [CODE][TEST] ~20m: Guard tests (fake Victoria): `TestShouldAbort_KingdomFastBurn`, `TestShouldAbort_MissingSeriesRefusesStart`, `TestShouldAbort_AbortFile`, `TestShouldAbort_HealthyGuardContinues`.
- [ ] **Step 267** [CODE][TEST] ~25m: Allowlist tests: refuses `pihole` by name, an unmanaged instance, another project, a unit not listed, a systemd target on a host that is not east, a veth name from the request, a netns not `steel*`, duration/loss/delay over the limits, a second concurrent experiment; every refusal audited.
- [ ] **Step 268** [CODE][TEST] ~25m: Journal tests: `TestJournal_WrittenBeforeInject` (injector fails after the write → entry reverted and removed), `TestJournal_RevertOnStart`, `TestJournal_TTLReverts`, `TestJournal_UnverifiedRevertKeepsEntryAndAlerts`, `TestAbort_AllThreePathsRevert`.
- [ ] **Step 269** [TEST] ~2m: Red: `gotest ./services/yaldabaoth/...`.
- [ ] **Step 270** [CODE] ~40m: `services/yaldabaoth/allowlist.go` (YAML via the yaml module already in go.mod), wired into `StartExperiment` and passed to injectors.
- [ ] **Step 271** [CODE] ~40m: `steadystate.go`: Victoria client (`VM_URL`, query API, 5 s timeout), policy reader for `page: true` SLOs, real `runProbe`, real `shouldAbort`.
- [ ] **Step 272** [CODE] ~45m: `journal.go` (statefile; entry = id, injector, target, revert recipe, created, ttl) + TTL timers + `RevertAll(ctx)`.
- [ ] **Step 273** [CODE] ~30m: `abort.go`: HTTP hook, Wotan subscription via `SubscribeWithRetry` (Phase 2), file watch (poll 1 s; no inotify dependency).
- [ ] **Step 274** [TEST][V] ~5m: Green, race-clean; `DryRunMode` default stays true (a window turns it off by env).
  ```bash
  gotest ./services/yaldabaoth/... && grep -n 'DryRunMode' "$PROJECT_ROOT/services/yaldabaoth/yaldabaoth.go" | head -5
  ```
- [ ] **Step 275** [C] ~2m: **COMMIT CHECKPOINT**: `feat(yaldabaoth): allowlist, abort paths, revert journal and an SLO-based steady state`.
- [ ] **Step 276** [CODE][TEST] ~45m: `cmd/chaos-controller`: build `yaldabaoth.Service`; route `InjectRule` and GameDay stages through experiments (D-16); `revert-all` subcommand (exit 0 only when the journal is empty afterwards); routes `POST /api/v1/chaos/experiments`, `GET .../experiments[/{id}]`, `POST /api/v1/chaos/abort`; mutating routes 403 unless `AUTH_ENABLED`; metrics `chaos_experiments_total{type,result}`, `chaos_active_injections`, `chaos_aborts_total{path}`, `chaos_refusals_total{reason}`. Tests for each route and for `revert-all` on a seeded journal.
- [ ] **Step 277** [TEST][V] ~3m: Green: `gotest ./cmd/chaos-controller/... ./services/yaldabaoth/...`.
- [ ] **Step 278** [C] ~2m: **COMMIT CHECKPOINT**: `feat(chaos-controller): run every fault through the Yaldabaoth safety envelope`.
- [ ] **Step 279** [W] ~25m: Host packaging (not installed yet): `pkg/uids` `Yaldabaoth` = next free in the Gnostic tier (`grep -n '= 1676[0-9]' pkg/uids/uids.go`; expect 16763), sysusers line `unheaded-chaos`; `deploy/systemd/unheaded-chaos-controller.service`: `User=unheaded-chaos`, `AmbientCapabilities=CAP_NET_ADMIN` (west variant adds `CAP_BPF` for Phase 13 via a drop-in, `deploy/systemd/unheaded-chaos-controller.service.d/west-packet-lab.conf`), matching `CapabilityBoundingSet`, `NoNewPrivileges=yes`, `ProtectSystem=strict`, `StateDirectory=unheaded-chaos` (0700), `RuntimeDirectory=unheaded/chaos`, `EnvironmentFile=/etc/unheaded/chaos/chaos.env`, `ExecStopPost=/usr/local/bin/chaos-controller revert-all`, `Restart=on-failure`; the full sandbox block used by the other units.
- [ ] **Step 280** [V] ~3m: `systemd-analyze verify deploy/systemd/unheaded-chaos-controller.service` clean; `bash scripts/check-systemd-units.sh` green; `gotest ./pkg/uids/...`.
- [ ] **Step 281** [C] ~2m: **COMMIT CHECKPOINT**: `build(chaos): unit, sysusers account and uid for chaos-controller`.
- [ ] **Step 282** [W] ~30m: ADR `docs/adr/ADR-<N>-chaos-safety-envelope.md` (Proposed), N = next free number at execution time (`ls docs/adr | grep -o 'ADR-[0-9]*' | sort -t- -k2 -n | tail -1`; sibling sprints may have taken 105). Content: D-16..D-20, the D-17 allowlist schema, the revert guarantees, and what Stevie accepts by accepting it. Add it to `docs/adr/ADR-INDEX.md`.
- [ ] **Step 283** [C] ~2m: **COMMIT CHECKPOINT**: `docs(adr): ADR-<N> chaos safety envelope (Proposed)`.
- [ ] **Step 284** [SECURITY] ~25m: BlackMage on the controller: can any request field widen the allowlist; is the journal dir writable by anyone but `unheaded-chaos`; does abort work with Wotan down and auth misconfigured (file path); can a replayed `chaos.trigger` start an experiment (control topics are never buffered: Phase 2 D-10); are refusals rate-limited so they cannot flood Wotan. Findings fixed now or registered (ADR-098).
- [ ] **Step 285** [TEST] ~5m: Bite test: comment out the allowlist check in `StartExperiment`, `gotest ./services/yaldabaoth/ -run Allowlist` goes red, restore.
- [ ] **Step 286** [V] ~2m: **PHASE 10 EXIT GATE**: Steps 274, 277, 280 green; Step 285 bit; ADR committed; no injector does anything yet beyond the D-19 journal (the four built-in injectors still log only: Phase 11 replaces them).
- [ ] **Step 287** [C] ~1m: **COMMIT CHECKPOINT** (plan ticks + Step 284 notes).
- [ ] **Step 288** [AUDIT-UPDATE] ~2m: next.md: ADR-<N> waiting for Stevie's accept (not blocking).

**DoD (Phase 10)**: DoD-STD + no code path can mark a probe successful without performing it (test), and `revert-all` is idempotent (run twice, second is a no-op).

---

## PHASE 11: REAL INJECTORS I: NETEM AND CONTAINER KILL (Steps 289-312)

**Goal**: replace the simulated injectors with real ones: `tc netem` on the host-side veth of an allowlisted LXD instance (and in the `steel*` lab netns), and container stop/kill through Cuirass with Cuirass's enforce loop as the self-heal. Prove both live, including a crash of the controller mid-fault.
**Prerequisite**: Phase 10 exit gate; Phase 5 (Cuirass live); E2.5-E2.6 for the live steps.
**Time**: 4-5 h
**Agent**: Agent (code, Steps 289-302), Coordinator (303-312) [BARE-METAL]

- [ ] **Step 289** [DECIDE] ~0m: **D-21 netem targets are resolved, never named.** RECOMMENDATION: Cuirass's actual state gains `host_veth` (LXD `state.network.eth0.host_name`); the netem injector takes an LXD instance id, asks Cuirass for its `host_veth`, checks D-17, and shapes that interface's egress (traffic toward the container) with a root qdisc carrying our handle `c4a0:`. Lab netns targets use the fixed `steel<N>-h` names. Revert deletes the root qdisc only if its handle is `c4a0:`. Rationale: a request that names an interface could name `enp16s0`; a request that names an allowlisted instance cannot. Override ONLY if LXD reports no `host_name` (non-bridged NIC): then the instance is not chaos-eligible (refuse).
- [ ] **Step 290** [CODE][TEST] ~20m: Cuirass: `ContainerState.HostVeth` from LXD state; test with the fake LXD (`TestLXDREST_ReportsHostVeth`).
- [ ] **Step 291** [TEST][V] ~3m: Green: `gotest ./cmd/unheaded-daemon/...`.
- [ ] **Step 292** [C] ~2m: **COMMIT CHECKPOINT**: `feat(cuirass): report each instance's host-side veth`.
- [ ] **Step 293** [CODE][TEST] ~25m: Netem injector tests (`services/yaldabaoth/inject_netem_test.go`, with an injectable command runner): exact argv for delay (`tc qdisc replace dev <if> root handle c4a0: netem delay 200ms 20ms`), loss (`loss 10%`), partition (`loss 100%`), duplicate, corrupt; never a shell; refuses interfaces failing D-17; revert argv `tc qdisc del dev <if> root handle c4a0:`; revert verification parses `tc -j qdisc show dev <if>`; an already-gone interface (instance deleted) counts as reverted.
- [ ] **Step 294** [TEST] ~2m: Red.
- [ ] **Step 295** [CODE] ~40m: Implement `inject_netem.go`; register for `network_delay`, `network_loss`, `network_partition`, `latency_spike` (delay alias); delete the four simulated injectors those replace.
- [ ] **Step 296** [TEST][V] ~3m: Green.
- [ ] **Step 297** [CODE][TEST] ~30m: Container-kill injector (`inject_container.go`): precondition `GET $CUIRASS/api/v1/info` has `enforce: true` and `chaos: true` (else refuse: no self-heal means no revert); calls `POST /api/v1/containers/{id}/chaos` with `stop` or `kill`; revert = wait until Cuirass reports `running` + `healthy` within the experiment's MTTR budget (default 90 s); if not, the experiment records a High finding and the journal entry stays with `chaos.revert_failed` (there is deliberately no start backdoor around Cuirass). Tests against a fake Cuirass: refused when enforce is off, MTTR measured, timeout path.
- [ ] **Step 298** [TEST][V] ~3m: Green: `gotest ./services/yaldabaoth/...`; `grep -n 'Simulating\|would be restarted' services/yaldabaoth/*.go | grep -v _test` prints nothing.
- [ ] **Step 299** [C] ~2m: **COMMIT CHECKPOINT**: `feat(yaldabaoth): real netem and container-kill injectors; simulated ones removed`.
- [ ] **Step 300** [DECIDE] ~0m: **D-22 HTTP-error injector.** RECOMMENDATION: `http_error` is served by the workload's own knob (steel-echo `STEEL_ERROR_RATE` via a new image version), so Yaldabaoth does not get an L7 proxy this sprint; the experiment type stays registered but refuses with "not supported: deploy a faulty build instead" until a real L7 injector exists. Rationale: an honest refusal beats a fake success; the canary game day already covers bad builds. Override ONLY if Stevie wants L7 fault injection now (a separate design: Envoy/HAProxy fault filters).
- [ ] **Step 301** [CODE][TEST] ~10m: Implement D-22's refusal + test; `cpu_stress`, `memory_stress`, `disk_stress`, `dns_failure`, `time_skew`, `node_drain`, `resource_exhaustion`, `custom` likewise refuse with "not implemented" (they had no injector at all; now they say so).
- [ ] **Step 302** [C] ~2m: **COMMIT CHECKPOINT**: `fix(yaldabaoth): unimplemented experiment types refuse instead of succeeding`.
- [ ] **Step 303** [LIVE-RW][S][BARE-METAL] ~10m: (E2.5) Install chaos-controller on west: binary to `/usr/local/bin/chaos-controller` (root-owned 0755), sysusers, unit + west drop-in, `/etc/unheaded/chaos/{allowlist.yaml,chaos.env}` (env holds `AUTH_API_KEYS`, `CUIRASS_API_KEY`, `VM_URL`; 0640 root:unheaded-chaos), `systemctl daemon-reload && systemctl enable --now unheaded-chaos-controller`.
- [ ] **Step 304** [V][LIVE-RO] ~2m: Running as its own user with only the granted capabilities; journal empty.
  ```bash
  pid=$(systemctl show -p MainPID --value unheaded-chaos-controller); ps -o user= -p "$pid"
  grep CapEff /proc/"$pid"/status | awk '{print $2}' | xargs capsh --decode
  curl -fsS "$CHAOS/health"; sudo ls -la /var/lib/unheaded-chaos/
  ```
  - `unheaded-chaos`; `cap_net_admin,cap_bpf` (west drop-in) and nothing else.
- [ ] **Step 305** [LIVE-RW] ~10m: Window open (stack, Cuirass enforce, `CUIRASS_CHAOS=true`, Sword, meter on discover). Netem experiment: `network_delay` 200 ms ± 20 ms on `steel-echo-v2-0` for 120 s, probes on its `/health`.
  ```bash
  curl -fsS -X POST -H "Authorization: Bearer $CHAOS_API_KEY" -H 'Content-Type: application/json' "$CHAOS/api/v1/chaos/experiments" \
    -d '{"name":"rs-netem-1","type":"network_delay","target":{"kind":"lxd","id":"steel-echo-v2-0"},"parameters":{"delay_ms":200,"jitter_ms":20},"schedule":{"duration":"120s"},"rollback":{"automatic":true}}' | jq '{id,status}'
  ```
- [ ] **Step 306** [V][LIVE-RO] ~3m: During: `tc -j qdisc show dev <host_veth>` lists `c4a0:` netem; requests the meter sent to v2-0 show p50 ≥ 180 ms while the other two stay < 20 ms (per-instance CSV columns). After: qdisc gone, journal empty, experiment `completed`, hypothesis verdict recorded.
- [ ] **Step 307** [LIVE-RW] ~5m: Crash revert: start the same experiment with `duration: 10m`, wait for the qdisc, then `sudo systemctl kill -s SIGKILL unheaded-chaos-controller`.
- [ ] **Step 308** [V][LIVE-RO] ~2m: Within 5 s of the kill, `ExecStopPost` removed the qdisc (`tc qdisc show dev <host_veth>` has no `c4a0:`); the unit restarted; journal empty; `journalctl -u unheaded-chaos-controller -n 20` shows `revert-all` verified one entry. Fail → Step 309.
- [ ] **Step 309** [D] ~10m: `ExecStopPost` did not run or failed: `systemctl show -p ExecStopPost unheaded-chaos-controller`; check the binary path and that `revert-all` can read the journal (StateDirectory perms). Clean the qdisc by hand (`sudo tc qdisc del dev <if> root handle c4a0:`), fix, retry once. Second failure → STUCK; **Phase 14 may not start** (D-19 is a hard requirement).
- [ ] **Step 310** [LIVE-RW] ~8m: Container kill: `container_kill` on `steel-echo-v2-1` (`kill`, force). Negative tests in the same window: the same request for `pihole` and for an instance without `chaos=allowed`.
- [ ] **Step 311** [V][LIVE-RO] ~3m: v2-1 back `running` + `healthy` within 90 s (MTTR from the `cuirass.chaos` event to healthy, recorded); meter availability over the window ≥ 99% (two replicas kept serving); both negative requests refused twice (controller allowlist 4xx with `chaos_refusals_total` +1, and Cuirass 409 if it ever got that far: it must not have); pihole `Started` time unchanged.
- [ ] **Step 312** [C] ~2m: **PHASE 11 EXIT GATE + COMMIT**: Steps 306, 308, 311 passed; window closed (`CUIRASS_CHAOS` back off). Commit `test(chaos): live netem, crash revert and container kill on west` with the numbers.

**DoD (Phase 11)**: DoD-STD + D-19 proven live (Step 308) + negative live tests (Step 311).

---

## PHASE 12: REAL INJECTORS II: SYSTEMD STOP/KILL, EAST ONLY (Steps 313-330)

**Goal**: stop or kill an allowlisted systemd unit on east, with least privilege (a polkit rule scoped to named units for `unheaded-chaos`, no root, no sudo), revert by start, and monitoring that sees it.
**Prerequisite**: Phase 10 exit gate; E3 answered for the live steps.
**Time**: 3-4 h
**Agent**: Agent (code), Coordinator (live) [P:13]

- [ ] **Step 313** [ESCALATE] ~0m agent / one pass for Stevie: **E3, east chaos agent.**
  - **Context**: Phase 11 proved the envelope on west. East runs the debs; Step 11 recorded its active `unheaded-*` units.
  - **Questions:** (1) Install chaos-controller on east as `unheaded-chaos` (sysusers), root-owned binary in `/opt/unheaded/bin`, unit without `CAP_NET_ADMIN`/`CAP_BPF` (systemd injector only)? (2) Install `/etc/polkit-1/rules.d/60-unheaded-chaos.rules` (generated from the allowlist) letting `unheaded-chaos` start/stop/kill only the listed units? (3) Which units may be broken? Recommended: `unheaded-timeguru.service` (read-only service, regenerates from Git) only; add more later. (4) Run the Phase 12 tests and the Phase 14 east scenario on east during windows?
  - **Why Stevie**: host change on east (staging); stopping a service there is visible on his dashboards.
  - **What stops**: Steps 322-330 and Phase 14 Scenario C. Code Steps 314-321 continue.
- [ ] **Step 314** [DECIDE] ~0m: **D-23 systemd injector = polkit-scoped systemctl, generated from one allowlist.** RECOMMENDATION: the injector runs fixed argv `systemctl --no-ask-password {stop|kill --signal=SIGKILL|start} <unit>` as `unheaded-chaos`; authorization comes from a polkit rule that allows `org.freedesktop.systemd1.manage-units` only when `subject.user == "unheaded-chaos"` and `action.lookup("unit")` is in the list and `action.lookup("verb")` is one of `start`, `stop`, `kill`; the rule file is rendered by `chaos-controller render-polkit` from the same allowlist YAML (one source; a test fails if they diverge). Revert = `start`, verified with `systemctl is-active`. The injector refuses unless `os.Hostname()=="east"`. Rationale: no sudo, no root helper, no wildcard; the kernel-side check is systemd's own. Override ONLY if east's systemd/polkit does not pass `unit`/`verb` details (check `pkaction --verbose --action-id org.freedesktop.systemd1.manage-units` in Step 322): then a dedicated root-owned oneshot per unit (`unheaded-chaos-stop@<unit>.service`) that the user may start, still allowlisted.
- [ ] **Step 315** [CODE][TEST] ~25m: Tests: refuses on a host not named east (injectable hostname), refuses unlisted units and any `*.target`/`ssh*`/`systemd-*`/`dbus*` even if listed (hard deny list), exact argv, revert verifies `is-active == active`, `kill` relies on `Restart=` first and only starts if still inactive after 30 s.
- [ ] **Step 316** [TEST] ~2m: Red.
- [ ] **Step 317** [CODE] ~35m: Implement `inject_systemd.go` for `process_kill` (kill) and a new `service_stop` type; `render-polkit` subcommand; `deploy/polkit/60-unheaded-chaos.rules.tmpl`.
- [ ] **Step 318** [TEST][V] ~3m: Green, plus the divergence test (`TestPolkitRuleMatchesAllowlist`).
- [ ] **Step 319** [C] ~2m: **COMMIT CHECKPOINT**: `feat(yaldabaoth): systemd stop/kill injector for east, polkit-scoped`.
- [ ] **Step 320** [W] ~15m: East unit variant `deploy/systemd/unheaded-chaos-controller.service.d/east.conf` (no ambient capabilities, `CapabilityBoundingSet=` empty) and `scripts/install-chaos-east.sh` (dry run default; copies binary, sysusers, unit, drop-in, allowlist, rendered polkit rule; prints the env file to create by hand).
- [ ] **Step 321** [C] ~2m: **COMMIT CHECKPOINT**: `build(chaos): east install script and unit drop-in`.
- [ ] **Step 322** [LIVE-RO] ~3m: East polkit supports the details D-23 needs.
  ```bash
  ssh "$EAST" 'pkaction --verbose --action-id org.freedesktop.systemd1.manage-units | head -20; systemctl --version | head -1'
  ```
- [ ] **Step 323** [LIVE-RW][S] ~10m: (E3.1-2) Run the install script on east with `--apply`; create `/etc/unheaded/chaos/chaos.env` by hand (0640 root:unheaded-chaos); `systemctl enable --now unheaded-chaos-controller`.
- [ ] **Step 324** [V][LIVE-RO] ~3m: Least privilege holds: the account can stop the listed unit and nothing else.
  ```bash
  ssh "$EAST" 'sudo -u unheaded-chaos systemctl --no-ask-password stop ssh.service 2>&1 | tail -1'                 # must be "Access denied"
  ssh "$EAST" 'sudo -u unheaded-chaos systemctl --no-ask-password stop unheaded-kev-poller.service 2>&1 | tail -1' # must be "Access denied" (not listed)
  ssh "$EAST" 'systemctl is-active ssh unheaded-kev-poller'
  ```
- [ ] **Step 325** [LIVE-RW] ~5m: (E3.4) Experiment `service_stop` on `unheaded-timeguru.service` for 60 s via `$EAST_CHAOS`, with an HTTP probe on east's timeguru `/health`.
- [ ] **Step 326** [V][LIVE-RO] ~3m: During: inactive, probe failing (expected impact); east's Akira and dashboard report timeguru unhealthy (read-only GETs on east: monitoring sees chaos). After: active, `/health` 200, journal empty, verdict recorded.
- [ ] **Step 327** [LIVE-RW] ~3m: `process_kill` (SIGKILL) on the same unit.
- [ ] **Step 328** [V][LIVE-RO] ~2m: systemd's `Restart=` brought it back (NRestarts +1 in `systemctl show -p NRestarts`), or the injector's start did after 30 s; recorded which.
- [ ] **Step 329** [V][LIVE-RO] ~2m: East's active unit list equals Step 11's (`systemctl list-units "unheaded-*"` diff); **PHASE 12 EXIT GATE**.
- [ ] **Step 330** [C] ~2m: **COMMIT CHECKPOINT**: `test(chaos): live systemd stop/kill on east under a polkit allowlist`.

**DoD (Phase 12)**: DoD-STD + the negative test (Step 324) is part of the record; nothing on east runs as root that did not before.

---

## PHASE 13: PACKET-LEVEL CHAOS VIA THE MONAD CHAOS FLAG (Steps 331-356)

**Goal**: real packet faults that act only on flagged flows: `yaldabaoth-ebpf` flags enrolled flow labels (CHAOS bit, existing) and now also marks those packets for a netem band, so loss/delay/duplication hit flagged packets and nothing else, in a lab netns only. Wire format untouched.
**Prerequisite**: Phase 10 exit gate; Step 17 baseline; E2.6 for live steps; E4 does not block (see Step 332).
**Time**: 5-6 h
**Agent**: Agent (code), Coordinator (live) [P:12] [BARE-METAL]

- [ ] **Step 331** [V] ~5m: **SPEC COMPLIANCE GATE (before any code)**
  - Wire format version: v0x01 (FROZEN).
  - Change type: none to the wire. The program already sets CHAOS (bit 7, octet 0x07, foundation-06 §Flags Bitfield) on enrolled flows; this phase adds an skb mark, which never leaves the host.
  - New flags, fields, TLVs, registry entries: none.
  - RFC Editor reviewed: required Y before Step 334 (Step 332 records it).
  - Backward compatible: yes (non-enrolled packets byte-identical; checked live in Step 351).
  - IANA impact: none.
- [ ] **Step 332** [ESCALATE] ~0m agent / one pass for Stevie + RFC Editor: **E4, spec-vs-code findings.**
  - **Context**: grounding found (H13): foundation-06 Anamnesis Event Types say `EVENT_CHAOS 0x04`, `EVENT_ANOMALY 0x08`; code (`monad_common::EventType`, `bpfschema`) uses Chaos 0x05, Anomaly 0x04. And `shield-ebpf:836` sets the CHAOS flag on blocked-source Anomaly events (a second meaning for the bit inside Anamnesis).
  - **Questions:** (1) Which side moves on the event codes: amend the draft (draft-07) to the code's values, or change all eBPF producers and Go/JS consumers to the draft's? (2) Should shield stop reusing CHAOS for blocked sources (code-only change: the Monad there is an event-local copy, not a wire packet)? (3) RFC Editor sign-off on Step 331's "no wire change" reading.
  - **Recommendation offered**: (1) amend the draft to match the deployed code (no BPF churn; the registry is "Specification Required", still a draft); (2) yes, stop the reuse in a separate small change; (3) yes. **Why still escalated**: a spec registry change is RFC Editor + Stevie by the frozen-format rule; the Marshal halts (S4) any agent that edits a spec draft unilaterally.
  - **What stops**: only spec/draft edits and the shield change. This phase codes against the code constants (0x05) and changes neither.
- [ ] **Step 333** [DECIDE] ~0m: **D-24 mark, then shape.** RECOMMENDATION: after `apply_chaos`, `yaldabaoth_tc` sets `skb->mark = 0xC4A05` (constant local to yaldabaoth-ebpf; `monad-common` untouched, so no other program's artifact changes). Userspace builds on the lab egress interface: `tc qdisc replace dev <if> root handle c4a0: prio bands 3`, `tc filter add dev <if> parent c4a0: protocol ipv6 handle 0xC4A05 fw flowid c4a0:3`, `tc qdisc add dev <if> parent c4a0:3 handle c4a3: netem <loss|delay|duplicate>`; the classifier attaches at clsact egress via TCX (runs before the root qdisc enqueue). CORRUPT stays the program's existing BitFlip mode (stale CRC, detected at the next hop). DROP = netem loss on band 3; DELAY = netem delay; the program's `scratch_r0` delay hint stays as it is. Rationale: netem can only act on packets the program flagged, which is exactly "acts only on flagged flows", and the eBPF change is one helper call. Override ONLY if the verifier rejects `set_mark` in this context (it should not: TC programs may write `skb->mark`): then classify with a `u32` filter on the CHAOS bit offset instead (no eBPF change at all).
- [ ] **Step 334** [CODE] ~20m: `ebpf/yaldabaoth-ebpf/src/main.rs`: `const CHAOS_SKB_MARK: u32 = 0xC4A05;` and `ctx.set_mark(CHAOS_SKB_MARK)` after `apply_chaos` on every enrolled packet; header comment updated (the "netem qdisc ... feeds into" claim becomes true).
- [ ] **Step 335** [BUILD] ~5m: Build and size check.
  ```bash
  cd "$PROJECT_ROOT" && make ebpf-yaldabaoth 2>&1 | tail -3 && stat -c %s ebpf/target/bpfel-unknown-none/release/yaldabaoth-ebpf; cat "$SCRATCH/yald-size.txt"
  ```
- [ ] **Step 336** [V] ~2m: Builds; size grows by tens of bytes, not kilobytes; `git diff --stat -- ebpf/` touches only `ebpf/yaldabaoth-ebpf/`. (No `monad-common` change, so ADR-090's `monad-cpu-ebpf --features ascend-linux` 901,888-byte check is not triggered; confirm with `git diff --name-only -- ebpf/monad-common` empty.)
- [ ] **Step 337** [C] ~2m: **COMMIT CHECKPOINT**: `feat(yaldabaoth-ebpf): mark flagged packets for the chaos netem band`.
- [ ] **Step 338** [CODE][TEST] ~30m: Go loader `services/yaldabaoth/packet/` (cilium/ebpf, already a dependency) tests first: map value encoding equals `bpfschema.ChaosTargetValue` (mode low 32 bits, param high 32; key = flow label `& 0xFFFFF`), refuses any interface not matching `^steel[0-9]+-h$` or not present, refuses flow label 0, builds the exact `tc` argv of D-24, revert order (detach link → delete map entries → delete root qdisc `c4a0:` → unpin maps), all through the D-19 journal.
- [ ] **Step 339** [TEST] ~2m: Red.
- [ ] **Step 340** [CODE] ~60m: Implement: load the object (path from env `YALD_OBJ`, default the installed copy), `link.AttachTCX` egress on the lab interface, pin maps under `/sys/fs/bpf/unheaded/chaos/<exp-id>/`, write `CHAOS_TARGETS`/`CONFIG`, read `STATS` for the experiment record; experiment type `packet_chaos` (params: iface, flow_labels[], mode, netem) and the old `ChaosFaultRule` API mapped onto it (D-16).
- [ ] **Step 341** [TEST][V] ~3m: Green (unit tests; the BPF load itself is proven live in Step 345).
- [ ] **Step 342** [C] ~2m: **COMMIT CHECKPOINT**: `feat(yaldabaoth): packet_chaos experiments load and drive yaldabaoth-ebpf in the lab`.
- [ ] **Step 343** [R][DECIDE] ~15m: **D-25 lab traffic source.** Find the Go code that builds an IPv6 packet with a Monad Hop-by-Hop option and a valid CRC (`grep -rn -i 'hop-by-hop\|HopByHop\|IPV6_HOPOPTS' --include=*.go cmd pkg | grep -v _test`; `cmd/doom-go-injector` is known to). RECOMMENDATION: add `steel-echo monad-send --iface steel0-h --dst <ula> --flow-label <n> --count N` reusing that encoder (move it into a small package only if it lives in `package main`), and `steel-echo monad-count` in the netns that counts received packets per flow label and whether the CHAOS bit is set (header bytes only; no payload is read). Override ONLY if no Go encoder exists: then use the Rust test vectors in `monad-common` to build the 20-byte option in Go with a test pinning bytes to those vectors.
- [ ] **Step 344** [LIVE-RW][S] ~10m: (E2.6) Lab netns on west.
  ```bash
  sudo ip netns add steel0
  sudo ip link add steel0-h type veth peer name steel0-n && sudo ip link set steel0-n netns steel0
  sudo ip link set steel0-h mtu 1700 up && sudo ip -n steel0 link set steel0-n mtu 1700 up && sudo ip -n steel0 link set lo up
  sudo ip addr add fd00:5ee1::1/64 dev steel0-h nodad && sudo ip -n steel0 addr add fd00:5ee1::2/64 dev steel0-n nodad
  ```
  (MTU 1700 per foundation-06's guidance for chaos deployments.)
- [ ] **Step 345** [LIVE-RW] ~10m: Experiment `packet_chaos` on `steel0-h`, flow label `0x5EE1`, mode `marker`, netem `loss 50%`, 5 min. Start the counter in the netns, then send 1000 packets on `0x5EE1` (enrolled) and 1000 on `0x5EE2` (not enrolled).
  ```bash
  sudo ip netns exec steel0 "$SCRATCH/steel-echo" monad-count --iface steel0-n --seconds 30 > "$SCRATCH/pkt.json" &
  sudo "$SCRATCH/steel-echo" monad-send --iface steel0-h --dst fd00:5ee1::2 --flow-label 0x5EE1 --count 1000
  sudo "$SCRATCH/steel-echo" monad-send --iface steel0-h --dst fd00:5ee1::2 --flow-label 0x5EE2 --count 1000
  sleep 30; jq '.' "$SCRATCH/pkt.json"
  ```
- [ ] **Step 346** [V][LIVE-RO] ~3m: **Acts only on flagged flows**: 0x5EE1 received 400-600 with CHAOS bit set on every one; 0x5EE2 received 1000/1000 with the CHAOS bit clear. `bpftool map dump pinned /sys/fs/bpf/unheaded/chaos/<id>/STATS` shows the enrolled count; `bpftool net show dev steel0-h` lists the TCX program. Fail → Step 347.
- [ ] **Step 347** [D] ~10m: Nothing flagged → the sender's packets lack the HbH option type the program expects (`opt_type` check at main.rs:159) or the flow label is not where the program reads it: capture 3 packets with `sudo tcpdump -i steel0-h -c 3 -x ip6` (headers only) and compare with the program's offsets. Flagged but no loss → the fw filter did not match: `tc -s filter show dev steel0-h` hit counters. 2 attempts → STUCK (Phase 14 Scenario E skipped; the rest of the game day proceeds).
- [ ] **Step 348** [LIVE-RW] ~3m: Abort mid-experiment by the file path: `sudo touch /run/unheaded/chaos/ABORT`.
- [ ] **Step 349** [V][LIVE-RO] ~2m: Within 5 s: no program on `steel0-h` (`bpftool net show dev steel0-h`), no `c4a0:` qdisc, pinned dir gone, journal empty; re-send 1000 on `0x5EE1` → 1000/1000, CHAOS bit clear. Remove the ABORT file.
- [ ] **Step 350** [B] ~3m: Event cross-check: CHAOS Anamnesis events carry event type 0x05 (code constant), consistent with `bpfschema.EventChaos`; note the count beside E4.
- [ ] **Step 351** [V][LIVE-RO] ~5m: **SPEC COMPLIANCE GATE (after)**: a non-enrolled packet captured before and during the experiment is byte-identical in its 20-byte Monad (`tcpdump -x` hex of the HbH option); an enrolled packet in `marker` mode differs only in flag bit 7 and the recomputed CRC (bytes 18-19).
- [ ] **Step 352** [LIVE-RW][S] ~2m: Teardown: `sudo ip netns del steel0; sudo ip link del steel0-h 2>/dev/null || true`.
- [ ] **Step 353** [V] ~1m: `ip netns list | grep steel || echo clean`; `ip link show steel0-h 2>&1 | grep -c 'does not exist'`.
- [ ] **Step 354** [W] ~10m: `docs/protocol/notes/yaldabaoth-skb-mark.md`: what the program does now, the compliance gate results (Steps 331, 351), E4's status. Link from `docs/protocol/SPEC-VS-CODE-AUDIT.md`.
- [ ] **Step 355** [V] ~1m: **PHASE 13 EXIT GATE**: Steps 336, 346, 349, 351 passed; E4 posted.
- [ ] **Step 356** [C] ~2m: **COMMIT CHECKPOINT**: `test(chaos): CHAOS-flagged packet faults in a lab netns, wire format unchanged`.

**DoD (Phase 13)**: DoD-STD + the spec compliance gate passed twice (before and after) + no physical NIC ever carried the program (Step 346's `bpftool net show` only on `steel0-h`; `bpftool net show` on `enp16s0` unchanged).

---

## PHASE 14: FIRST GAME DAY: DEPLOY, INJECT, ROLLBACK PROVEN (Steps 357-380)

**Goal**: one planned, written-down game day that chains the sprint: a canary deploy is hurt by a real injected fault, Sword rolls it back on its own, the envelope reverts the fault, and steady state is shown restored, with numbers.
**Prerequisite**: Phases 9, 11 exit gates (Phase 12 for Scenario C; Phase 13 for Scenario E); E2.7 (and E3.4 for east).
**Time**: 3 h (90 min live window)
**Agent**: Coordinator [SEQ] [BARE-METAL]

- [ ] **Step 357** [W] ~20m: `docs/gamedays/2026-MM-DD-real-steel-1.md` from a new template `docs/gamedays/TEMPLATE.md`: hypothesis per scenario, blast radius (exact instance/unit/interface list), abort criteria (D-18/D-20), success criteria, who watches what, timeline table to fill in.
- [ ] **Step 358** [C] ~2m: **COMMIT CHECKPOINT**: `docs(gameday): plan for REAL STEEL game day 1`.
- [ ] **Step 359** [B] ~2m: Quiet the pager for the window (ADR-100: no self-made noise).
  ```bash
  cd "$PROJECT_ROOT" && scripts/monitoring/maintenance.sh 90m "S-REAL-STEEL game day 1"
  ```
- [ ] **Step 360** [V] ~1m: The silence is active (Alertmanager `/api/v2/silences` lists it, state `active`).
- [ ] **Step 361** [LIVE-RW] ~5m: Window open: stack + Cuirass (enforce, prune, `CUIRASS_CHAOS=true`) + Sword + chaos-controller (west; east if E3); steel-echo baseline v2 x3 (`recreate` if anything drifted).
- [ ] **Step 362** [V][LIVE-RO] ~3m: Pre-flight: every `page: true` SLO has an `slo:error_ratio:5m` series (`vmq 'slo:error_ratio:5m'`); steel-echo 3/3 healthy; journal empty on both hosts; pihole `Started` time noted; east unit list = Step 11.
- [ ] **Step 363** [B] ~1m: Meter on for the whole window (discover mode, 20 rps, `$SCRATCH/gameday.csv`).
- [ ] **Step 364** [LIVE-RW] ~10m: **Scenario A (the headline):** Sword canary v2→v3 (healthy build) with latency analysis (`latency_p99_threshold: 100ms`, interval 30 s, 33% then 100%). As soon as the canary instance is healthy, chaos `network_delay` 300 ms on the canary instance (allowlisted via its `chaos=allowed` label), duration 5 min.
- [ ] **Step 365** [V][LIVE-RO] ~3m: Sword rolled back on its own, reason `analysis_failed` (p99 above threshold, the measured value in the record); canary instance deleted; the chaos experiment's injection reverted when its target disappeared (journal entry closed as "target gone"); v2 x3 serving. Record: fault start → rollback decision → rollback complete timestamps.
- [ ] **Step 366** [V][LIVE-RO] ~3m: Steady state restored: probes 100% for 2 min after rollback; meter availability over Scenario A ≥ 99%; no Kingdom SLO fast-burn (`vmq 'slo:error_ratio:5m'` at pre-flight levels).
- [ ] **Step 367** [LIVE-RW] ~10m: **Scenario B:** Sword rolling v2→v3; when the first v3 instance is healthy, chaos `container_kill` on one remaining v2 instance.
- [ ] **Step 368** [V][LIVE-RO] ~3m: Cuirass restarted the killed instance (MTTR recorded) and the rollout finished (or rolled back cleanly); `lxc list --project unheaded` equals Sword's desired set exactly (no orphans, no duplicates); availability ≥ 99%.
- [ ] **Step 369** [LIVE-RW] ~5m: **Scenario C (east, if E3):** `service_stop` on east `unheaded-timeguru.service` for 60 s; [V] east's Akira/dashboard saw it (read-only GETs), it came back, journal empty.
- [ ] **Step 370** [LIVE-RW] ~5m: **Scenario D (abort drill):** start `network_delay` 10 min on a v3 instance, then abort through the HTTP path; then start another and abort through the file path; [V] each reverted in ≤ 5 s, `chaos_aborts_total{path}` counts 1 for `http` and 1 for `file`.
- [ ] **Step 371** [LIVE-RW] ~10m: **Scenario E (if Phase 13 passed):** repeat Steps 344-349 in a fresh `steel0` lab with `delay 100ms` instead of loss; [V] enrolled flow RTT +100 ms, other flow unchanged; teardown clean.
- [ ] **Step 372** [B] ~3m: Meter off; collect numbers: per-scenario availability, MTTR (B, C), rollback latency (A), abort revert times (D), Kingdom SLO values before/after.
- [ ] **Step 373** [V] ~2m: Every cell in the game-day results table has a number from a CSV, Sword/chaos records or Victoria; none is estimated.
- [ ] **Step 374** [LIVE-RW] ~5m: Teardown: `CUIRASS_CHAOS` off; steel-echo left at v3 x3 or removed (Sword delete → Cuirass prune) per the game-day doc; window closed per E2.1.
- [ ] **Step 375** [V][LIVE-RO] ~3m: Post-state: `lxc list --project unheaded` equals the doc's end state; pihole `Started` unchanged; no `c4a0:` qdisc on any host veth (`tc qdisc show | grep -c c4a0` = 0); journals empty on west and east; east unit list = Step 11.
- [ ] **Step 376** [B] ~1m: Expire the silence early if the window ended early (`maintenance.sh` expire option or Alertmanager API).
- [ ] **Step 377** [W] ~25m: Fill the game-day doc: timeline, results table, what went well, what did not, action items. Each action item becomes a register finding (ADR-098) or a Kanban card in The Well (ADR-100 §5), never a loose TODO.
- [ ] **Step 378** [C] ~2m: **COMMIT CHECKPOINT**: `docs(gameday): REAL STEEL game day 1 results` + CSV numbers under `docs/gamedays/data/`.
- [ ] **Step 379** [V] ~1m: **PHASE 14 EXIT GATE**: Scenario A passed (Steps 365, 366); B passed; D passed; C and E passed or STUCK-logged with reasons; post-state clean (Step 375).
- [ ] **Step 380** [AUDIT-UPDATE] ~5m: Action items filed (register IDs / card IDs listed in the doc).

**DoD (Phase 14)**: DoD-STD + deploy → inject → automatic rollback shown end to end with timestamps (Scenario A) + zero residue (Step 375).

---

## PHASE 15: SECURITY AND COMPLIANCE REVIEW (Steps 381-392)

**Goal**: review the attack surface the sprint added, run the house gates, and register anything not fixed.
**Prerequisite**: Phases 4, 8, 10-13 code merged on the branch.
**Time**: 2 h
**Agent**: Agent (BlackMage + MoatGhost lenses)

- [ ] **Step 381** [SECURITY] ~30m: Attack-surface table in this file:

  | Added | Exposure | Control | Reviewed |
  |---|---|---|---|
  | Sword :19011 | compose network + host port | auth on mutating routes; Cuirass key in host env file | |
  | chaos-controller :19012 (west, east) | host | auth; allowlist; abort; journal | |
  | Cuirass `/chaos`, `/reconcile`, proxy device, health probe | compose | auth; `CUIRASS_CHAOS`; managed + label checks; IP-only probe | |
  | LXD client cert (restricted, project `unheaded`) | `/etc/unheaded/cuirass` | 0600, pinned server cert | |
  | `CAP_NET_ADMIN` (+`CAP_BPF` west) for `unheaded-chaos` | host | own account, sandbox, allowlisted targets | |
  | polkit rule (east) | east | unit allowlist, verbs start/stop/kill | |
  | Wotan topics `deploy.*`, `chaos.*`, `cuirass.chaos` | Wotan | allowlisted publishers; control topics never buffered | |
- [ ] **Step 382** [B] ~10m: gosec ratchet and govulncheck.
  ```bash
  cd "$PROJECT_ROOT" && gosec -quiet ./... 2>&1 | tail -20; govulncheck ./... 2>&1 | tail -10
  ```
- [ ] **Step 383** [V] ~2m: No new gosec rule IDs versus Step 131's method at the sprint's first commit; govulncheck: 0 reachable.
- [ ] **Step 384** [COMPLIANCE] ~5m: No new dependencies.
  ```bash
  cd "$PROJECT_ROOT" && go mod tidy -diff && git diff --stat "$(git log --reverse --format=%H --grep 'S-REAL-STEEL' | head -1)" -- go.mod go.sum ebpf/Cargo.lock
  (cd ebpf && cargo audit 2>&1 | tail -3)
  ```
  - go.mod/go.sum/Cargo.lock unchanged. Any change → Barrister/ADR-004 review before commit (stop and ask).
- [ ] **Step 385** [B] ~3m: Secrets: nothing committed.
  ```bash
  cd "$PROJECT_ROOT" && gitleaks detect --no-banner --redact -s . 2>&1 | tail -3 && bash scripts/check-secrets-baseline.sh
  ```
- [ ] **Step 386** [V] ~1m: gitleaks clean; baseline did not grow.
- [ ] **Step 387** [TEST] ~10m: Fuzz the new parsers for 60 s each: allowlist YAML, Cuirass `ContainerSpec` (health/proxy), Sword deployment request.
  ```bash
  cd "$PROJECT_ROOT" && go test ./services/yaldabaoth/ -run '^$' -fuzz FuzzAllowlist -fuzztime 60s
  go test ./cmd/unheaded-daemon/ -run '^$' -fuzz FuzzContainerSpec -fuzztime 60s
  go test ./services/sword/ -run '^$' -fuzz FuzzDeployRequest -fuzztime 60s
  ```
- [ ] **Step 388** [V] ~1m: No crashers (any crasher → fix + keep the corpus file as a regression test).
- [ ] **Step 389** [TEST] ~5m: Gates can fail (ADR-093 principle): bite-test the Phase 7 "no simulation" test and the polkit divergence test (break each, see red, restore).
- [ ] **Step 390** [AUDIT-UPDATE] ~10m: Register (ADR-098) a finding for each item not fixed in this sprint; regenerate the ADR-098 section only after re-signing (S17 note).
- [ ] **Step 391** [V] ~1m: **PHASE 15 EXIT GATE**: table complete, Steps 383, 386, 388 passed.
- [ ] **Step 392** [C] ~2m: **COMMIT CHECKPOINT**: `docs(security): S-REAL-STEEL attack-surface review`.

**DoD (Phase 15)**: DoD-STD.

---

## PHASE 16: DOCS, REGISTERS, HANDOFF (Steps 393-404)

**Goal**: every document that describes deploy, chaos, Cuirass or Wotan tells the truth after this sprint; the next session can start cold.
**Prerequisite**: Phases 1-15 exit gates passed or STUCK-logged.
**Time**: 1.5-2 h
**Agent**: Librarian lens, Solo

- [ ] **Step 393** [DOC-UPDATE] ~30m: wiki: `Service-Sword.md` (Phase 8), new `Chaos-Engineering.md` (Yaldabaoth for real: allowlist, abort paths, journal, experiment types and which refuse), `Containers.md` (Phases 4-5), new `Game-Days.md` (links the doc), `Steel-Echo.md`; `_Sidebar` + Home links (wiki rule).
- [ ] **Step 394** [REGEN] ~5m: `docs/LIVE-PATHS.md` (binaries) and `docs/UNREACHED-GO-PACKAGES.md` (Step 5 command) regenerated.
- [ ] **Step 395** [DOC-UPDATE] ~10m: Port and uid tables (wiki + CLAUDE.md port registry): Sword 19011 replaces Canary; `unheaded-chaos` uid.
- [ ] **Step 396** [DOC-UPDATE] ~5m: ADR index statuses: ADR-090 per E1; the chaos ADR Proposed; ADR-088 and ADR-064 one-line notes (Steps 236, 105).
- [ ] **Step 397** [W] ~10m: `docs/battle-plans/S-REAL-STEEL-battle-plan.md`: a SHIPPED banner at the top (commit range, the live proofs by step number, what stayed STUCK), in the PHASE24 style.
- [ ] **Step 398** [DOC-UPDATE] ~10m: CLAUDE.md, one edit (Librarian ripple): the `pkg/transport` Connect line (if deleted), "Interchangeable/real deploy via Sword + Cuirass on LXD", Yaldabaoth line. No release/roadmap framing (memory: reference_readme_doc_style).
- [ ] **Step 399** [B] ~3m: Wiki mirror check and leak scan.
  ```bash
  cd "$PROJECT_ROOT" && scripts/sync-wiki.sh --dry-run 2>&1 | tail -5 && gitleaks detect --no-banner --redact -s wiki 2>&1 | tail -2
  ```
- [ ] **Step 400** [V] ~1m: Dry run clean; no mirror-only pages; gitleaks clean.
- [ ] **Step 401** [W] ~15m: `~/tmp/next.md`: a new top section "S-REAL-STEEL" (overwrite stale sprint status, keep the standing notes): commits (unsigned or signed), what is live on west/east and how to turn it off, E1-E4 status, STUCK list with the Stuck Report (format in the Warmonger skill), follow-ons (remaining "wotan unavailable" give-up sites from Step 90, server-side weighted split, the Well as deploy/chaos store, ADR-064 snapshot).
- [ ] **Step 402** [TEST] ~15m: Final full test run versus baseline.
  ```bash
  cd "$PROJECT_ROOT" && go build ./... && go test -count=1 ./... 2>&1 | grep -E '^(FAIL|---)' | grep -v -F -f "$SCRATCH/baseline-tests.txt" || echo "no new failures"
  ```
- [ ] **Step 403** [V] ~2m: **FINAL EXIT GATE (sprint DoD)**:
  - deploy is real: Phase 9 Steps 243, 246, 249, 252 passed;
  - chaos is real: Phase 11 Steps 306, 308, 311; Phase 12 Step 326 (or STUCK on E3); Phase 13 Step 346 (or STUCK);
  - game day: Phase 14 Steps 365-366 passed;
  - Wotan leftovers closed (Phases 2-3);
  - E1 applied (Phase 1) or still waiting with the table posted;
  - "no new failures" at Step 402; tree clean; nothing pushed.
- [ ] **Step 404** [C] ~2m: **FINAL COMMIT**: `docs: S-REAL-STEEL shipped banner, wiki and handoff`. Do not push (Stevie pushes; staging only on his word).

**DoD (Phase 16)**: DoD-STD + a cold session can find every live switch (how to turn Cuirass back to mock, how to stop chaos-controller, how to abort) from the wiki alone.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1. A fault is still active and the controller is gone or wedged.**
1. `sudo touch /run/unheaded/chaos/ABORT` (west; on east via ssh).
2. `sudo systemctl stop unheaded-chaos-controller` (ExecStopPost runs `revert-all`).
3. Check: `tc qdisc show | grep c4a0` (west veths), `bpftool net show | grep -i yald`, `ls /sys/fs/bpf/unheaded/chaos/`.
4. By hand if anything remains: `sudo tc qdisc del dev <if> root handle c4a0:`; `sudo rm -rf /sys/fs/bpf/unheaded/chaos/<id>` after `bpftool net detach tcx_egress dev <if>`; on east `sudo systemctl start <unit>`.
5. Record it in the game-day doc as an action item: D-19 failed, Phase 14 stops.

**A2. Cuirass touched (or wants to touch) something that is not ours.**
1. Stop it: `sudo docker stop unheaded-cuirass`.
2. `lxc list --all-projects -c nsP` and `lxc info --project default pihole`.
3. If pihole was stopped: `lxc start --project default pihole` (Stevie's service; tell him).
4. Switch back to mock: remove the override from the compose command (`docker compose -f docker-compose.yml up -d cuirass`), confirm `/api/v1/info` says `mock`.
5. S4 HALT: no further live steps until the cause is understood (the managed checks are independent twice; both failing is a design bug).

**A3. Sword left orphans or duplicates.**
1. `curl $SWORD/api/v1/deployments | jq` for the last record; `curl $CUIRASS/api/v1/state/desired | jq 'keys'`.
2. Remove extra desired entries with `DELETE $CUIRASS/api/v1/containers/<id>` (prune deletes the instances).
3. Re-run Phase 9 Step 239 baseline. File the orphan path as a Phase 7 bug with a fake-Cuirass reproducer.

**A4. Wotan clients are stuck on HTTP polling or a stream is down.**
1. `/health` detail of the service (D-12) and `unheaded_wotan_client_stream_up`.
2. `grpc_health_probe` is not installed; use the client's `ProbeGRPC` via the service restart or `curl :18000/health`.
3. If Wotan is up and streams stay down: restart the client service; file the case against Phase 2 with logs.

**A5. Victoria or the SLO series are missing.**
No experiment may start (D-20) and canary analysis fails closed (D-06). Bring Victoria up (`docker compose up -d victoria`), wait 10 min for `slo:error_ratio:5m` to populate, check with `vmq 'slo:error_ratio:5m'`. Never bypass the guard.

**A6. yaldabaoth-ebpf fails the verifier.**
1. `sudo bpftool prog load ebpf/target/bpfel-unknown-none/release/yaldabaoth-ebpf /sys/fs/bpf/yald-test type classifier 2>&1 | tail -40`.
2. Revert Step 334 (`git revert`) and take D-24's override (u32 filter on the CHAOS bit, no eBPF change).

**A7. East unreachable mid-window.**
East's own unit has `ExecStopPost`, TTL timers and systemd `Restart=`; nothing on east depends on west to revert. Wait for the link; then Step 329's unit diff. Do not power-cycle east without Stevie.

**A8. Disk pressure on west (/tmp is a 7.2G tmpfs; LXD images).**
`lxc image list` and `lxc storage info default`; delete `steel-build-*` leftovers; keep `$SCRATCH` small (CSVs only).

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX AND DEPENDENCIES

| Phase | Steps | Agent | Parallel with | Depends on | Waits on | Time |
|---|---|---|---|---|---|---|
| 0 Preflight + sweep table | 1-44 | Coordinator / Agent [P] | none | none | none | 2.5-3.5 h |
| 1 Apply E1 | 45-66 | Agent | 2-13 | 0 | **E1** | 2-3 h |
| 2 Wotan client | 67-98 | Agent | 3, 4, 6, 7 | 0 | E1 (Steps 91-92 only) | 3-4 h |
| 3 Wotan snapshot | 99-108 | Agent | 2, 4 | 0 | none | 0.75 h |
| 4 Cuirass extensions | 109-136 | Agent | 2, 3, 6 | 0 | none | 4-5 h |
| 5 Cuirass live | 137-160 | Coordinator | 7, 8, 10 (code) | 4 | **E2** | 2-3 h |
| 6 steel-echo | 161-178 | Agent, then Coordinator | 2-4, 7 | 0 (code), 5 (live) | E2.4 | 3 h |
| 7 Deploy core | 179-208 | Agent | 2, 3, 5, 6, 10 | 4 | none | 5-6 h |
| 8 Sword | 209-236 | Agent | 5, 10 | 7 | E1 (Step 233 only) | 5-6 h |
| 9 Deploy live | 237-258 | Coordinator | 10 (code) | 5, 6, 8 | E2.7 | 2-3 h |
| 10 Chaos envelope | 259-288 | Agent | 5-9 | 0, 4 | none | 6-7 h |
| 11 Injectors I | 289-312 | Agent, then Coordinator | 12, 13 (code) | 10, 5 | E2.5-6 | 4-5 h |
| 12 Injectors II (east) | 313-330 | Agent, then Coordinator | 11, 13 | 10 | **E3** | 3-4 h |
| 13 Packet chaos | 331-356 | Agent, then Coordinator | 11, 12 | 10 | E2.6 (+ **E4** for spec edits only) | 5-6 h |
| 14 Game day | 357-380 | Coordinator | none | 9, 11 (+12, 13 for C, E) | E2.7, E3.4 | 3 h |
| 15 Security/compliance | 381-392 | Agent | 16 (draft) | 4, 8, 10-13 | none | 2 h |
| 16 Docs/handoff | 393-404 | Solo | none | all | none | 1.5-2 h |

**Critical path** (longest chain, about 31 h plus Stevie's E2 turnaround):

```
P0 (3h) → P4 (4.5h) → [E2] → P5 (2.5h) → P6 live (1h) → P9 (2.5h) → P14 (3h) → P15 (2h) → P16 (2h)
                     ↘ P7 (5.5h) → P8 (5.5h) ↗ (P7+P8 run while E2 is pending; they gate P9)
P10 (6.5h) → P11 (4.5h) ↗ (gates P14; runs in parallel with P7/P8)
```

The real critical path is P0 → P4 → P7 → P8 → P9 → P14 → P15 → P16 (about 30 h) if E2 lands before P8 finishes; otherwise E2's turnaround is on it.

**Hello-world moments**: Step 149 (Cuirass creates a real instance, pihole untouched); Step 174 (first steel-echo healthy behind its proxy); Step 249 (first automatic rollback on real instances); Step 308 (a crashed controller still reverts its fault); Step 346 (packet faults hit only flagged flows); Step 365 (the headline).

**Escalations (four, each one pass):**

| ID | Step | Blocks | Asks |
|---|---|---|---|
| E1 | 38 | Phase 1; Steps 91-92, 233 | Answer the grouped delete/keep/park table (ADR-090) |
| E2 | 137 | Phases 5, 6 (live), 9, 11 (live), 13 (live), 14 (west) | West live window: stack up, trust `--apply`, Cuirass https/enforce/prune/chaos, image import, Sword + chaos units, lab netns sudo, game day |
| E3 | 313 | Phase 12 live, Phase 14 Scenario C | East chaos agent + polkit rule + unit allowlist |
| E4 | 332 | Spec/draft edits and the shield flag change only | Event-code mismatch (0x04 vs 0x05) and shield's reuse of CHAOS |

---

## APPENDIX C: QUICK REFERENCE

**Decisions (pre-seeded, agent proceeds):**

| ID | Step | Decision |
|---|---|---|
| D-01 | before 21 | Package-level ledger TSV, importable into ADR-090's file ledger |
| D-02 | before 30 | Grouping rules R1-R9 → G1-G10 |
| D-03 | 110 | Immutable versioned instances `<svc>-<ver>-<n>`; update = create + delete |
| D-04 | 111 | Proxy device is the stable endpoint; canary split is client-side by replica share |
| D-05 | 180 | One deploy core: `pkg/deploy/strategy` + `rollback`; Callbacks = driver contract |
| D-06 | 211 | Canary analysis from instance `/metrics` + Kingdom SLO fast-burn guard |
| D-07 | 212 | Sword state in `pkg/statefile` (`SWORD_DATA_DIR`) |
| D-08 | 210 | `cmd/sword` on 19011 (Canary's port); Canary folds in |
| D-09 | 213 | Sword as compose service; host unit if H17 false |
| D-10 | 68 | Fallback queue: flush, 2 min max age, never for control topics |
| D-11 | 74 | ProbeGRPC = grpc.health.v1 Check, 2 s |
| D-12 | 80 | Stream health = liveness + transport, not message age |
| D-13 | 100 | TransferSnapshot answers Unimplemented |
| D-14 | 161 | steel-echo images on Alpine 3.22 + OpenRC |
| D-15 | 170 | Images imported by govan into the shared default image store |
| D-16 | 260 | One chaos engine: `yaldabaoth.Service` inside chaos-controller |
| D-17 | 261 | Blast-radius allowlist file; targets resolved, never named |
| D-18 | 262 | Abort: HTTP, Wotan, file |
| D-19 | 263 | Revert journal + TTL + start replay + ExecStopPost |
| D-20 | 264 | Steady state: real probes + ADR-100 fast-burn guard; missing series = no start |
| D-21 | 289 | Netem on the instance's host veth (from Cuirass), handle `c4a0:` |
| D-22 | 300 | Unimplemented experiment types refuse |
| D-23 | 314 | East systemd via polkit-scoped `systemctl`, rule rendered from the allowlist |
| D-24 | 333 | yaldabaoth-ebpf sets skb mark 0xC4A05; netem on the fw-matched band only |
| D-25 | 343 | Lab Monad sender/counter reuse the existing HbH encoder |

**Ports and identities**

| What | Port | Account / uid |
|---|---|---|
| Cuirass (compose / deb) | 19006 / 17000 | 16740 |
| Sword | 19011 (was Canary) | 16746 |
| chaos-controller | 19012 | `unheaded-chaos` (uid from Step 279, expect 16763) |
| steel-echo in-container | 8080 | n/a (LXD container) |
| steel-echo stable endpoint (proxy) | 127.0.0.1:26601 (user-apps band) | n/a |
| VictoriaMetrics | 8428 | 16703 |
| Wotan HTTP / gRPC | 18000 / 18001 | |

**API cheat sheet**

```bash
# Cuirass (auth on writes)
curl -fsS $CUIRASS/api/v1/info | jq '{lxd_backend,lxd_project,enforce,prune,chaos}'
curl -fsS $CUIRASS/api/v1/state/actual | jq
curl -fsS -X PUT    -H "Authorization: Bearer $CUIRASS_API_KEY" $CUIRASS/api/v1/containers/<id> -d '<ContainerSpec>'
curl -fsS -X DELETE -H "Authorization: Bearer $CUIRASS_API_KEY" $CUIRASS/api/v1/containers/<id>
curl -fsS -X POST   -H "Authorization: Bearer $CUIRASS_API_KEY" $CUIRASS/api/v1/containers/<id>/chaos -d '{"action":"kill","reason":"gameday"}'
curl -fsS -X POST   -H "Authorization: Bearer $CUIRASS_API_KEY" $CUIRASS/api/v1/reconcile
# Sword
curl -fsS $SWORD/api/v1/deployments | jq
curl -fsS -X POST -H "Authorization: Bearer $SWORD_API_KEY" $SWORD/api/v1/deployments/<id>/rollback
# Chaos
curl -fsS -X POST -H "Authorization: Bearer $CHAOS_API_KEY" $CHAOS/api/v1/chaos/abort
sudo touch /run/unheaded/chaos/ABORT        # the abort that needs nothing else
```

**ContainerSpec additions (Phase 4)**: `health {port, path}`, `proxy {listen "tcp:127.0.0.1:266xx", connect "tcp:127.0.0.1:<port>"}`, label `chaos=allowed`.

**SLO queries (ADR-100)**: `slo:error_ratio:5m{slo="wotan"}`; fast-burn threshold = `14.4 * (1 - objective/100)`; objectives: wotan, dashboard, well = 99.5; others 99 (`observability/alerting/policy.yaml`).

**Monad bits used**: flags octet 0x07, CHAOS = 0x80 (exists, frozen); `CHAOS_TARGETS` key = flow label & 0xFFFFF, value = mode | param << 32; modes 1 BitFlip, 2 Delay, 3 Duplicate, 4 Truncate, 5 Marker; skb mark 0xC4A05 (host-local, not wire).

**Commit trailer**: `$TRAILER` (the executing session's attribution lines). Signing: `$SIGN`. Never push.

---

*S-REAL-STEEL Battle Plan, Forged 2026-10-23*
*17 Phases. 404 Steps. The stubs stop pretending: Sword cuts real instances, and Yaldabaoth breaks them on purpose, inside the fence.*
*Every fault is journaled before it lands, and every "real" has a live step that proved it.*
