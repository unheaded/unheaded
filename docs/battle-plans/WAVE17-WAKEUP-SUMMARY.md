# WAVE17 — Wake-Up Summary (overnight 2026-05-05)

**TL;DR:** stack is exactly as you left it (qwen-7b serving, 9/9 bare-metal
services green). Wotan main.go review committed & tested. Active/active
clustering speced (ADR-064, *not* implemented). Doctrine cleanup landed
across 9 files + 2 pre-doctrine decision records flagged. cmd/tools/
scaffolded for the round-table P0 wedges.
**K8s substrate proven on kind: 9/9 services Running, two real bugs found
+ fixed mid-run.** Kind cluster torn down at end. No live infra disturbed.

**Phase 7 follow-on** (Stevie's "no idle if pending todos" directive):
runbook stubs for the two referenced-but-missing files (ADR-064 cutover,
Zhen On-Prem air-gap-egress), timeline Age 3 entry for WAVE16 + WAVE17,
ADR-INDEX freshness sweep (count 56→65, recent additions), and a
historical-note amendment on the two pre-doctrine track-call decision
records. **18 commits total this session.**

---

## What you can do now that you couldn't last night

1. **Bring up a 3-node local Unheaded cluster in one command.**
   `./deploy/k8s/kind/bring-up.sh` → ~90s later all 9 services are Running
   in kind. Idempotent. Tear-down: `kind delete cluster --name unheaded`.
   This is the *substrate* ADR-064 was waiting on — active/active migration
   can now iterate from a working baseline whenever you say go.

2. **Run wotan tests in isolation.** 13 new unit tests cover `parseFlags`,
   `monitorPendingMembers` (renamed from `startAdminCLI`), context-aware
   ticker shutdown, and the cluster fail-fast guard:
   `go test ./services/wotan/cmd/wotan/...` should be clean.

3. **Read ADR-064.** `docs/adr/ADR-064-wotan-active-active-cluster-k8s-native.md`
   — full spec for the 3-node anycast/broadcast Wotan cluster (Raft
   membership + topic-leader election, quorum-acked publish, K8s-native
   StatefulSet). This one supersedes ADR-035 once the implementation phases
   land. Per your guidance, it stays parked until you say go.

---

## Commits this overnight session (18 in WAVE17)

```
a4f9d443 doctrine: amend pre-doctrine track-call decision records with historical note
971fc0ef docs: ADR-INDEX freshness sweep — totals + recent additions
47258fb1 WAVE17 Phase 7: stub the two referenced-but-missing runbooks + timeline Age 3 entry
c679cb2a docs: WAVE17 wake-up summary — Stevie's morning briefing
bba0acb1 WAVE17: K8s substrate proven on kind (9/9 services running)
6e6dfd07 cmd/tools/: scaffold the 3 P0 tools (Mímir / Anamnesis Lite / Zhen On-Prem)
3edac195 doctrine: amend docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md post-c6108fb8
3b07db2f doctrine: amend references/battle-plan-S76-round-table.md post-c6108fb8
5dd53254 doctrine: amend docs/internal/battle-plans/battle-plan-future.md post-c6108fb8
dc6324f9 doctrine: amend docs/adr/ADR-053-hybrid-claude-zhenai-workflow-templates.md post-c6108fb8
cb11f521 doctrine: amend docs/adr/ADR-031-hybrid-model-handoff.md post-c6108fb8
0ba796b7 doctrine: amend docs/adr/ADR-004-no-external-deps-policy.md post-c6108fb8
20101173 doctrine: amend docs/internal/battle-plans/S-PQC-battle-plan.md post-c6108fb8
26f2acd2 doctrine: amend skills/unheaded-captain-SKILL-UPDATE.md (continued) post-c6108fb8
a3c0e329 doctrine: amend skills/unheaded-captain-SKILL-UPDATE.md post-c6108fb8
01981fa8 doctrine: amend docs/adr/ADR-69420-kingdom-bgp-and-unheaded-os.md post-c6108fb8
fc58dfe8 docs: ADR-064 — Wotan Active/Active Cluster (3-node K8s, supersedes ADR-035)
b1e91c74 services/wotan/cmd/wotan: unit tests for main.go helpers (13 tests)
66b96252 services/wotan/cmd/wotan: fix 12 issues from grounded code review
```

---

## What landed, by lane

### Lane 1 — Wotan main.go: ground-up review + tests (2 commits)

Replacing qwen-coder-14b's generic review with a proper grounded walkthrough
turned up 12 real findings in `services/wotan/cmd/wotan/main.go`. All fixed
in `66b96252`:

- **Real bug**: cluster mode was silently discarded (`_ = clusterCfg`) — an
  operator with `--cluster-mode=cluster` saw "cluster mode enabled" logs but
  ran standalone. Now `log.Fatal()` at boot.
- **5 goroutine leaks** past graceful shutdown (system metrics, HTTP, gRPC,
  pending-members monitor, cleanup tickers) — all now context-aware via a
  `runCtx` derived in main(), so SIGTERM stops them within ~1s instead of
  hitting the 15s shutdown deadline.
- HTTP server hardening: `ReadHeaderTimeout: 5s` (CLAUDE.md baseline).
- gRPC hardening: `MaxRecvMsgSize(4 << 20)` + keepalive params/policy.
- Dropped deprecated `PreferServerCipherSuites` (TLS 1.3 ignores it).
- Renamed `startAdminCLI` → `monitorPendingMembers` (it's a 10s-tick logger,
  not a CLI).
- Promoted hardcoded timeouts to flags (`--read-timeout`,
  `--write-timeout`, `--idle-timeout`, `--shutdown-timeout`).
- gofmt cleanup, doc comments on the nil-msgStore fallback.

`b1e91c74` adds 13 unit tests covering the boot-path helpers — closes the
parked WAVE16 item that wanted main.go test coverage.

### Lane 2 — Active/active spec (1 commit)

`fc58dfe8` is **ADR-064-wotan-active-active-cluster-k8s-native.md**. Your
words from this session: *"active/active with anycast/broadcast - events,
alerting, upc, service to service communication, etc all work via wotan -
it must always be up and redundant ( min 3 node cluser - hop on k8
bandwagon )"* + *"active passive works but doesn't scale"* + *"roundrobin"*.

Spec covers:
- 3-node minimum cluster, hashicorp/raft for membership + topic-leader
  election (CP per CAP — quorum-acked publish, broadcast read).
- Round-robin at every dispatch layer: kube-proxy IPVS-rr, headless DNS
  rotation, topic-leader RR assignment.
- StatefulSet, headless service, PodDisruptionBudget(2/3), readiness gating
  on Raft followership, init-container pre-flight.
- LICH-014 split-brain campaign (red-team net-partition + leader churn).
- Akira CTF integration tag (per ADR-063).
- Migration plan: 4 phases, supersedes ADR-035 only after Phase 4 cuts over.
- Explicitly NOT implemented tonight per your "active/active on hold as
  ADR for now" instruction.

### Lane 3 — Doctrine sweep (10 commits)

The community-first doctrine landed at `c6108fb8` on 2026-04-30. Round-table
P0 wave caught 9 files still using sell/runway/CFO/premium framing.
Separate commit per file (audit trail, easy revert if any single change is
wrong):

- ADR-69420 (Kingdom BGP+OS) — stripped SKU/premium/GTM/Enterprise tier
  framing.
- skills/unheaded-captain-SKILL-UPDATE.md — round-table P0 first pass, then
  a second pass after your *"no runway no revenue oss - free to use"*
  feedback explicitly removed the runway/CFO archetype + revenue
  vocabulary.
- docs/internal/battle-plans/S-PQC-battle-plan.md
- ADR-004 (no-external-deps), ADR-031 (hybrid-model-handoff), ADR-053
  (Claude-Zhenai workflow templates)
- docs/internal/battle-plans/battle-plan-future.md
- references/battle-plan-S76-round-table.md
- docs/philosophy/BRAINSTORM-WITNESS-FABRIC.md

Per your *"remove those cheesy doctrine lines.... license covers that"*
note: the `cmd/tools/` README scaffolding (next lane) ships *without*
preachy doctrine preambles — license header is enough. Saved as feedback
memory `feedback_no_doctrine_preamble.md` so future sessions don't put
the cheese back.

### Lane 4 — cmd/tools/ scaffold (1 commit)

`6e6dfd07` creates `cmd/tools/` for the 3 round-table P0 wedges
(Mímir, Anamnesis Lite, Zhen On-Prem). Each gets a README + BUILD doc.
Mímir + Anamnesis Lite also get COMPONENTS.md inventories. The pattern
plus invariants are codified in `cmd/tools/README.md`. Curation only —
no Go code shipped tonight; the actual tool binaries will be carved out
of the existing services later.

### Lane 6 — Phase 7 follow-on (3 commits)

After the wake-up doc landed, the WAVE17 plan's Phase 7+ items got picked
up in three small batches per your *"keep churning beyond phase 6 if there
is any work pending - do not sit idle if there are any pending todo task"*
direction:

- `47258fb1` — two stub runbooks created (`runbooks/wotan/active-active-cutover.yaml`
  for ADR-064 Phase 4 cutover; `runbooks/network/air-gap-egress-validation.yaml`
  for the Zhen On-Prem 24h egress proof). Both have full step scaffolding
  (commands marked TODO) so they can be filled in incrementally.
  ADR-064 had referenced `runbooks/cluster/wotan-active-active-cutover.yaml`,
  but the actual `runbooks/cluster/` directory doesn't exist —
  cluster-failover.yaml + cluster-setup.yaml live under `runbooks/wotan/`,
  so the new cutover runbook joined them and ADR-064's path references
  were updated to match. `references/timeline.md` Age 3 also gets WAVE16
  + WAVE17 entries appended; mirrors (json/yaml/toml) are pre-existing
  stale (Timeguru drift since 2026-03-25, leave alone).
- `971fc0ef` — `docs/adr/ADR-INDEX.md` freshness sweep. Header claimed
  56 ADRs; actual table count is 65. Status table also stale (28 Accepted
  → 33; 6 Planned → 9; 4 Deferred → 5; 2 Pipe Dream → 4; missing rows for
  Acknowledged / PoC-Research / Phase 1 Shipped / Superseded). Two
  recent-additions callouts added (2026-05-04 batch ADR-060/061/062/063
  + 2026-05-05 ADR-064). All individual table rows for ADR-060 through
  ADR-064 were already there — this was metadata refresh, not new content.
- `a4f9d443` — extended doctrine sweep beyond the round-table's original
  9 files. Grepped for sell/monetize/premium/runway/ACV/GTM across docs/
  + skills/ + references/ + README.md. Most hits were doctrine
  *enforcement* (battle plans declaring "no selling"), not violations.
  Two real findings: `docs/decisions/2026-04-29-track-call.md` and
  `docs/decisions/2026-04-29-track-call-options.md` were committed
  2026-04-27 (`fb002223`), three days BEFORE the doctrine commit
  `c6108fb8` on 2026-04-30. Track A in those docs is the "VC pitch +
  Zhen-as-product GTM hook" option that the doctrine has since
  obsoleted. Decision records are historical artifacts; surgical
  removal would be revisionist. Both files now lead with a
  one-paragraph historical-note flagging Track A's framing as obsoleted
  and pointing readers at CLAUDE.md's Community-First Doctrine for the
  current charter; full text preserved verbatim.

### Lane 5 — K8s substrate (1 commit, 9/9 verified)

`bba0acb1` is the WAVE17 substrate. Plan at
`docs/battle-plans/WAVE17-OVERNIGHT-K8S-SUBSTRATE.md`, full results at
`eval/k8s-bringup/2026-05-05/RESULTS.md`. Started at Phase 0 with the helm
chart never having been deployed locally; ended with all 9 services
Running on a 3-node kind cluster.

Two real bugs surfaced and got fixed in the same run:

**Bug 1 — Monad K8s service-link collision** (filed as kanban
`k8s-monad-svc-env-bug-mn05`, closed same run). K8s auto-injects
`<SVC>_PORT=tcp://<clusterip>:<port>` env vars for every service in the
namespace. Monad reads `MONAD_PORT` as a port-number; K8s injected
`MONAD_PORT="tcp://10.96.x.x:19004"` → listen address became
`":tcp://10.96.x.x:19004"` → "too many colons" parse error → CrashLoop.
Fix: `enableServiceLinks: false` on every pod template (helm chart edit).
Services discover peers via stable DNS instead, which is what the chart
already wanted.

**Bug 2 — Helm chart had no volumes surface** (filed as kanban
`k8s-chart-volume-support-mn05`, closed same run).
`readOnlyRootFilesystem: true` (correct CLAUDE.md hardening) blocked
captain + timeguru from creating their state directories. Chart had no
values-keys for `volumes`/`volumeMounts`. Fix: extended
`helm/unheaded/templates/services.yaml` with `{{- with $svc.volumes }}` +
`{{- with $svc.volumeMounts }}` blocks; `values-local.yaml` adds emptyDir
mounts at the right paths.

Final state, captured live mid-run:
```
$ kubectl get pods -n unheaded
NAME                                 READY  STATUS   RESTARTS  AGE
architect-...                        1/1    Running  0         54s
captain-...                          1/1    Running  0         42s
dashboard-backend-...                1/1    Running  0         54s
kanban-app-...                       1/1    Running  0         54s
micromanager-...                     1/1    Running  0         54s
monad-...                            1/1    Running  0         44s
sophia-...                           1/1    Running  0         54s
timeguru-...-x2                      2/2    Running  0         42s  (replicas: 2)
wotan-...                            1/1    Running  0         54s
```

Wotan health probe in-cluster:
```json
{"rooms":0,"service":"wotan","status":"healthy","total_members":0,
 "version":"0.1.0","timestamp":"2026-05-05T06:28:01..."}
```

Cluster torn down at end of run per Marshal restore-state checklist.
Bare-metal stack on host stayed 9/9 green throughout — kind runs in
parallel inside docker without touching host port 5432/8081/9876/18000/
19009/20000/20001/20002/20103/20105.

---

## Backlog moves

Closed:
- `wotan-main-12-issues` (Lane 1)
- `wotan-main-tests` (Lane 1, parked from WAVE16)
- `active-active-spec` (Lane 2 — spec only; impl deferred per Stevie)
- 9× `doctrine-violation-*` items (Lane 3)
- `tools-curation-scaffold` (Lane 4)
- `k8s-substrate-proven-mn05` (Lane 5)
- `k8s-monad-svc-env-bug-mn05` (Lane 5, found + fixed mid-run)
- `k8s-chart-volume-support-mn05` (Lane 5, found + fixed mid-run)

Untouched (deliberately):
- ADR-064 implementation phases (0/4) — gated on your green light per
  *"active/active on hold as ADR for now"*.
- Anything in the cmd/tools/ tool binaries themselves — scaffold only.

---

## What got deliberately skipped

WAVE17 closed all five Phase 7+ items from the battle plan (runbooks,
timeline, ADR-INDEX, extended doctrine sweep). What's left is genuinely
upstream of "can run autonomously":

- Cmd-tools binary carve-outs (Mímir, Anamnesis Lite, Zhen On-Prem actual
  Go entry points) — scaffold and patterns are now in place; the actual
  `cmd/<tool>/main.go` entry points need design decisions on what to
  bundle vs link, so they wait for your input.
- ADR-064 implementation phases (0/4) — gated on your green light per
  *"active/active on hold as ADR for now"*.
- The two new stub runbooks (cutover + air-gap-egress) — every step is
  marked TODO; commands need to be written + tested. Stub form was the
  right exit since they're now reachable from the docs that reference
  them, and filling them in needs the actual cluster + appliance to
  exist.
- WAVE14 retrain + Captain Track A/B/C decision — both gated on your
  bandwidth + sign-off (timeline.md Age 3 lists them).
- Timeline JSON/YAML/TOML mirrors at `references/timeline.{json,yaml,toml}` —
  they've been stale since 2026-03-25 (Timeguru service drift,
  pre-existing; not a WAVE17 regression). Hand-editing them by string
  munging would risk introducing more drift; better to let the service
  regenerate them.

---

## Stack state at signoff

- **bare-metal**: postgres :5432, llama-server :8081, vor :9876, wotan
  :18000, shield :19009, dashboard-backend :20000, kanban-app :20001,
  wiki-server :20002, zhen_app :20103, zhen-agentd :20105 — all listening,
  wotan returns healthy.
- **llama-server**: still serving the canonical default (`qwen2.5-coder-7b-instruct`).
- **kind**: torn down. No leftover state.
- **git**: clean working tree, 18 commits ahead of where you went to bed.

LOVE SERVE REMEMBER. <3
