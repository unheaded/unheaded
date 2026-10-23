<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-LICH-AWAKENS BATTLE PLAN — 12 Phases, 268 Steps

**Date**: 2026-10-23
**Sprint**: Sprint 4 — LICH AWAKENS (red team via MCP: local model + Claude, through the Champion gate, confined to LXD sandboxes)
**Prerequisite**: Sprint 2 "Two Brains, One Gate" (Champion confirmation for writes, spend caps, a `local_llm` MCP tool, `-route auto`). Phase 0 **verifies** it; if any leg is absent it is a `[ESCALATE]`/gate, not a silent build.
**Target**: An MCP surface (the Lich tools) that lets a local model and Claude *drive* the existing Lich (`tomb/lich/`) — list/show campaigns, run a harness, read crashes, triage, file a finding to the ADR-098 register — with **every target confined to an LXD sandbox or a local fuzz harness, scope enforced server-side (allowlist + network namespace with no egress + resource limits), never by prompt**, every tool call through the Champion gate, a hard daily spend cap, a kill switch, and findings that are never auto-published. CTF mode (ADR-063) is **planned and gated**, not activated.
**Authorization**: Stevie's own systems only (the solo GPL-3.0 Kingdom on WEST/EAST). No third-party targets, ever. This is defensive red-teaming of code and services the operator owns; scope is enforced in the server, so a jailbroken driver cannot widen it.
**Estimated Duration**: 10–16 hours across 3–4 sessions (Large).
**Agent Strategy**: Phase 0 is Solo/Coordinator (verification + decisions). Phases 1–2 serialize (sandbox before anything runs in it). Phases 3–4 serialize behind the server. Phases 5–6 (drivers) parallelize once the gate is wired. Phase 7 (differential) parallels Phase 8 (lifecycle). Phase 9 (meta red-team) gates Phase 11. Phase 10 (CTF) is design-only.
**Commit Cadence**: Every 4 steps (calculated: `max(3, min(5, 268/20)) = 5`, tightened to 4 for a security build). Always commit after a phase exit gate, before any STUCK skip, after a new file, and on red→green.
**Stuck Protocol**: Skip after 3× the step's `~time` estimate or 2 failed debug attempts. Log a `[STUCK]` marker, commit, scan forward for the next non-blocked step, produce a STUCK REPORT when forward progress is exhausted.
**Commits**: `git commit --no-gpg-sign` (develop is being re-signed in the main checkout; this worktree touches only its own branch, never pushes). Re-sign with `scripts/resign-unpushed.py` is Stevie's job after merge.

**Grounded on (swept before drafting, per the battle-plan-grounding rule):**
- **ADR-062** (Lich framework: campaign anatomy `tomb/lich/LICH-NNN-<name>/`, three-tier taxonomy, activation rules, exit-criteria templates, CI tiers, inventory LICH-001…013).
- **ADR-063** (CTF mode / Pipe Dream: Akira summons the Lich against a live eligible service; activation rule = LICH-001, LICH-002, and one of LICH-012/013 clean 7 consecutive days; Marshal HARD HALT; `configs/lich-ctf-eligible.yaml`; findings via Champion).
- **ADR-098** (rolling findings register `compliance/findings/register.yaml`, generator `cmd/compliance-evidence findings`, FND-NNN IDs, "a finding closes when its evidence passes", audit/enforcing modes, LICH-012 red-team gate).
- **ADR-104** (`pkg/claudebridge` + `cmd/zhen-claude`: Messages API via official Go SDK, `claude-opus-5-5`, ChampionGate, daily cap, audit, `Router` interface **unimplemented**; built on develop, **not run live — no creds on west**).
- **ADR-042** (CS→BlackMage/Lich integration), **ADR-053** (HEAVY→Claude / MINOR→local), **ADR-019/037** (Champion), **ADR-067** (MBC ISA v2 + UPC ABI v1), **ADR-080** (UPC programmatic API — the differential-fuzz substrate), **ADR-029** (Akira), **ADR-096/004** (dependency admission register).
- **Code**: `tomb/lich/{lich-runner.sh,crash-triage.sh,harnesses/,seeds/,results/,config/lich.yaml,LICH-012-config-convergence/}`; `cmd/lich-security/{main.go,campaigns/d1..d6,runner.go}`; `pkg/claudebridge/{bridge,gate,spend,audit,route,tools,filter,credentials}.go`; `pkg/champion/{champion,confirm,dispatch,modelswap,toolcall,tools,write}.go`; `pkg/mcpclient/`, `pkg/zhentools/`; `raft/zhen_mcp_server.py`; `crates/monad-mbc/` (reference interpreter), `ebpf/` monad-cpu-ebpf (eBPF path); Cuirass LXD backend (`LXD_BACKEND=mock|unix|https`, S17 `b49e3f16`, live west LXD 5.21.8).
- **next.md S17** (origin/develop=31e0457c; unsigned commits; stack DOWN; hosts read-only; never push).
- **CLAUDE.md** (community-first GPL doctrine; zero user data access; gRPC-first; Doom Range ports 16666–26666; secrets never in repo).

---

## PHASE 0 FINDINGS (2026-10-23, recorded at Step 23)

**Execution base**: this sprint runs on worktree base `f1d1f412` (sprint lead's explicit
override of the plan's `443de3f82`). All code prerequisites listed in "Grounded on" are
present at `f1d1f412`. Branch `sprint/lich-awakens` tracks HEAD and is force-updated after
every commit. Never pushes; never touches develop/staging/main.

**Preflight hypothesis results:**

| # | Result | Evidence |
|---|--------|----------|
| H1 | **TRUE** | All four Sprint-2 legs present at base: `local_llm` (`raft/zhen_mcp_server.py`), `-route auto` (`cmd/zhen-claude/route.go` `runAuto`/`routeAuto`), `IssuePendingConfirmation` (`pkg/champion/confirm.go`), spend `Ledger`/`Reserve` (`pkg/claudebridge/spend.go`). **E0 does NOT fire.** |
| H2 | **TRUE** | `go build ./pkg/claudebridge/... ./pkg/champion/...` OK; gate tests green. `AcceptToolCall` refuses mutating + untrusted-justification pending-confirmation (Rule 2). |
| H3 | **NO_KEY** (expected) | `ANTHROPIC_API_KEY` unset. No live API in P0–P3. Claude driver (Phase 6, out of this chunk) defaults `-dry-run`. |
| H4 | **TRUE** | LXD client+server 5.21.8 reachable. |
| H5 | **TRUE** | Lich harness 001 clean: ~28K execs/sec, new-interesting 0, **0 crashes**. |
| H6 | **TRUE** | `monad-mbc` 67 tests pass + 3 fuzz targets (decode/execute/roundtrip); eBPF `monad-cpu-ebpf` path exists → differential oracle feasible (Phase 7, later). |
| H7 | **TRUE** | `cmd/compliance-evidence findings` → "14 open (2 high, 4 medium, 8 low)". |
| H8 | **PASS (HARD gate)** | No-egress **proven unprivileged** via `unshare -rn`: inside the netns LOOPBACK_OK, egress route+connect BLOCKED, no default route; host itself reaches 1.1.1.1 (proves the block is the namespace, not a dead host). **No E1.** |

**Step 8 scope note**: NOT taken — H1 is fully TRUE (both `local_llm` and `-route auto`
already shipped on this base). The Sprint-2 prerequisite is met; Phase 4 wiring (out of
this chunk's P0–P3 scope) is the lead's.

**Step 22 topology DECISION (recommendation taken, no Stevie override present)**: build a
**separate `cmd/lich-mcp` server** exposing the Lich tools over **stdio only (no TCP
port)**, with the tool logic in a reusable **`pkg/lichtools`** implementing
`zhentools.Provider`. Not folded into `raft/zhen_mcp_server.py` (blast-radius isolation;
Go reuse of `pkg/champion`/`pkg/claudebridge`; independent start/stop = part of the kill
switch).

**Step 24 SCOPE LOCK (enforced server-side, never by prompt)**: a valid target is ONLY
(a) an LXD container whose name starts with `lich-sbx-`, OR (b) a local fuzz harness under
`tomb/lich/harnesses/`. Nothing else: no network/host target, no path outside
`$PROJECT_ROOT` or the sandbox. Allowlist lives in `configs/lich-mcp.yaml`, read
server-side; a driver's prompt can never widen it.

**Confinement substrate decision (H4 TRUE + H8 proven)**: LXD is available, but creating
LXD containers / nftables rules / persistent host state is an **[ESCALATE]** per the
sprint lead's SAFETY brief. This build therefore uses the **unprivileged fallback**
(`unshare` user+net namespace with no egress + rlimits; `bwrap`/`systemd-run --user` where
useful) as the default `netnsSandbox`, with the `lxdSandbox` path present but gated behind
operator opt-in. No-egress is proven by a test that tries to reach the network and must
fail (H8).

**Sprint-2 confirmation flow status**: the out-of-band confirm *redemption* UX is Sprint 2's
(`sprint/two-brains`, in progress). For P0–P3 the Champion gate's *refusal*
(`PendingConfirmation=true`) is what we build and test against; wiring the human-approval
round trip is Phase 4 (NOT started here). Recorded as **pending, not a blocker**.

---

## LEGEND

```
[B]            = Bash command (run directly)
[V]            = Verification step (MUST pass before proceeding)
[D]            = Debug step (only if prior [V] fails; max 2 attempts → Skip Protocol)
[W]            = Write/create file
[R]            = Read/inspect file
[S]            = Sudo/elevated privileges required
[P]            = Parallelizable with other [P] steps in same phase
[P:N]          = Parallelizable with Phase N
[SEQ]          = MUST be sequential
[C]            = Commit checkpoint (git commit --no-gpg-sign)
[ENV]          = Environment setup/check
[BUILD]        = Build/compilation step
[TEST]         = Test execution
[CODE]         = Code implementation
[DESIGN]       = Design/planning decision (recorded in the plan/ADR)
[STUCK]        = Step skipped via Skip Protocol (needs human)
[BLOCKED]      = Blocked by an upstream [STUCK] (NOT a decision)
[DECIDE]       = Decision point WITH pre-seeded recommendation — agent proceeds autonomously
[ESCALATE]     = Requires human input — no recommendation possible. STOP.
[PREFLIGHT]    = Hypothesis verification (Phase 0 only)
[SECURITY]     = Security review step (BlackMage lane)
[AUDIT-UPDATE] = Mark a finding resolved / add a finding in the ADR-098 register
[DOC-UPDATE]   = Update downstream docs (ADR, CLAUDE.md, wiki) via Librarian
[COMPLIANCE]   = License/SBOM/dependency check (Barrister / ADR-096 lane)
[VM-SCAN]      = Must run inside the LXD sandbox / sacrificial host (east), never a dev host
[BARE-METAL]   = Needs a real kernel / BPF / netns
```

`[DECIDE]` steps carry RECOMMENDATION + Rationale + "Override ONLY if". The agent proceeds with the recommendation — no stop. `[ESCALATE]` steps carry Context + "Why no recommendation" and the agent STOPS. Used sparingly (3 in this plan).

---

## VARIABLES

Resolve once in Step 1; every later step references these, never a hardcoded path.

```
$PROJECT_ROOT    = git rev-parse --show-toplevel          # the checkout executing this plan
$LICH            = $PROJECT_ROOT/tomb/lich                 # existing Lich framework
$BRIDGE          = $PROJECT_ROOT/pkg/claudebridge          # Claude bridge (ADR-104)
$CHAMPION        = $PROJECT_ROOT/pkg/champion              # Champion gate (ADR-019/037)
$MCP             = $PROJECT_ROOT/raft/zhen_mcp_server.py   # existing Zhen MCP server
$REGISTER        = $PROJECT_ROOT/compliance/findings/register.yaml   # ADR-098 register
$EVID            = $PROJECT_ROOT/cmd/compliance-evidence   # register generator
$SANDBOX_PREFIX  = lich-sbx-                               # LXD container name prefix (scope allowlist)
$NETNS           = lich-noegress                           # network namespace: loopback only, no egress
$OPUS            = claude-opus-5-5                          # triage/synthesis model ($4/$20 per MTok)
$HAIKU           = claude-haiku-5-5                         # volume model ($0.10/$0.50 per MTok)
$DAILY_CAP_USD   = 5                                        # ADR-104 hard daily cap default; Phase 3 confirms
$LICH_SRV        = $PROJECT_ROOT/cmd/lich-mcp              # NEW: the Lich MCP server (per Phase 1 [DECIDE])
$LICH_PKG        = $PROJECT_ROOT/pkg/lichtools             # NEW: the Lich tool Provider (zhentools.Provider impl)
```

> **Model-id discipline:** `$OPUS`/`$HAIKU` ids were confirmed against the `claude-api` skill's model table (cached 2026-10-06): `claude-opus-5-5` = $4 in / $20 out per MTok, cache read $0.20; `claude-haiku-5-5` = $0.10 / $0.50 per MTok. Never append a date suffix. Pricing lives in `$BRIDGE/prices.json`; add `$HAIKU` there in Phase 6.

---

## PREFLIGHT HYPOTHESES

Each is verified in Phase 0. A **FALSE** result converts the dependent build steps to `[BLOCKED]` or triggers the named `[ESCALATE]`/gate. Never build on an unverified assumption.

| # | Hypothesis | How verified | If FALSE |
|---|---|---|---|
| H1 | Sprint 2 shipped Champion-confirmation-for-writes, spend caps, a `local_llm` MCP tool, and `-route auto` | Step 5–9: grep/build for each leg; `cmd/zhen-claude -h` shows `-route`; MCP server lists `local_llm` | `[ESCALATE] E0` — Sprint 4's prerequisite is unmet; do not fake the drivers |
| H2 | `pkg/claudebridge` builds and its ChampionGate refuses mutating tools pending-confirmation (ADR-104) | `go build ./pkg/claudebridge/...`; `go test ./pkg/claudebridge/... -run Gate` | Fix build first; if gate logic absent, `[ESCALATE] E0` |
| H3 | `pkg/claudebridge` has **no** live-API credentials on this host (ADR-104: "not yet run against the live API") | `printenv ANTHROPIC_API_KEY`; `ant auth status` | Expected FALSE-ish; Claude-driver steps run `-dry-run` only. If a key IS present, still default to `-dry-run` until Step 6 of Phase 6 |
| H4 | LXD is reachable and Cuirass's LXD backend works (`LXD_BACKEND`, west LXD 5.21.8) | Step 12: `lxc version`; `LXD_BACKEND=unix` read-only list | Sandbox substrate falls back to `nsjail`/`bwrap` + netns; mark H4-dependent steps per Phase 1 [DECIDE] |
| H5 | The existing Lich runner + crash-triage + harnesses run clean at HEAD | `$LICH/lich-runner.sh --harness 001 --time 30s`; `crash-triage.sh` on the result | If a harness is red at HEAD, record in KNOWN FAILURES BASELINE; do not attribute to this sprint |
| H6 | The MBC reference interpreter (`crates/monad-mbc`) and the eBPF path (monad-cpu-ebpf) both decode/execute a shared program format — a differential oracle is possible | Step 18–19: `cargo test -p monad-mbc`; confirm a Go/Rust harness can feed both | If no shared feed exists, Phase 7 narrows to reference-interpreter-only fuzzing + a `[DECIDE]` on the oracle |
| H7 | `compliance/findings/register.yaml` + `cmd/compliance-evidence findings` are the canonical finding sink (ADR-098) | Step 20: `go run ./cmd/compliance-evidence findings` prints the table | If the generator is broken at HEAD, record baseline; file-finding writes YAML + flags regen for later |
| H8 | A network namespace with no default route actually blocks egress on this host | Step 14 `[BARE-METAL]`: `ip netns exec $NETNS ping -c1 -W2 1.1.1.1` must FAIL; loopback must pass | If egress leaks, HARD STOP the sandbox phase — `[ESCALATE] E1` (no fuzzing without confinement) |

---

## KNOWN FAILURES BASELINE

Record BEFORE execution so the agent never mistakes a pre-existing failure for a regression it caused. Captured in Phase 0 Step 3–4.

```
Pre-existing (from next.md S17 + repo state at 443de3f82):
- origin/develop = 31e0457c; this branch's commits are UNSIGNED (key not cached). Expected. Never push.
- The stack is DOWN on west; Wotan/trace-collector not running. Live-service steps are read-only or sandbox-only.
- ADR-104 bridge: NOT run against live API (no creds). Claude-driver = -dry-run by default.
- claudebridge.Router: interface only, no implementation (route.go). `-route auto` is a Sprint 2 deliverable (H1).
- ADR-062 inventory audit: harnesses 004/007 are SIMULATIONS redundant with Rust cargo-fuzz; 009 low-value; 005 kernel half needs root + a sacrificial host (east). Do not "fix" these as regressions.
- ADR-098 register at 443de3f82: run `cmd/compliance-evidence findings` in Step 20 and paste the exact "N open" line here as the baseline count. New findings this sprint files are ADDITIONS, not regressions.
- LICH-012 is OPEN (config convergence); CTF activation rule is therefore NOT met — Phase 10 stays design-only.

Filled at Step 3–4 (2026-10-23, worktree base f1d1f412 — this execution's base per sprint lead's override of the plan's 443de3f82 reference; all prerequisites present at f1d1f412):
- go build ./...: exit 0 (clean).
- go test ./... baseline: 207 ok / 0 fail (19 "no test files"). No failing packages.
- $LICH/lich-runner.sh --harness 001 --time 30s: ran FuzzMonadWireParse + FuzzMonadWireRoundtrip at ~28K execs/sec, "new interesting: 0", crashes: 0 (crashes/ empty).
- cmd/compliance-evidence findings: "14 open (2 high, 4 medium, 8 low)", 0 accepted, 0 overdue. New findings this sprint files are ADDITIONS to this count, not regressions.
```

---

## PHASE 0: PREFLIGHT & PREREQUISITE VERIFICATION (Steps 1–26)

**Goal**: Prove the ground is solid — branch base, Sprint 2 prerequisite (H1), the gate (H2), credentials posture (H3), LXD (H4), the existing Lich (H5), the differential oracle (H6), the register (H7) — and record the KNOWN FAILURES BASELINE. Decide the server topology.
**Prerequisite**: You are in the worktree; branch is based on `443de3f82`.
**Time**: 60–75 min
**Agent**: Solo / Coordinator

### Ground truth

- [ ] **Step 1** [B] ~1m: Resolve `$PROJECT_ROOT` and confirm base.
  ```bash
  cd "$(git rev-parse --show-toplevel)"; export PROJECT_ROOT="$(pwd)"
  git merge-base --is-ancestor 443de3f82 HEAD && echo "BASE_OK" || echo "BASE_MISSING"
  git branch --show-current
  ```
- [ ] **Step 2** [V]: **GATE** — `BASE_OK` printed and branch is NOT `develop`/`staging`/`main`.
  - If `BASE_MISSING` → `git reset --hard 443de3f82` (worktree only), re-run Step 1.
  - If on a protected branch → STOP; you are in the wrong checkout.
- [ ] **Step 3** [B][TEST] ~5m: Baseline the Go build/test. Record verbatim.
  ```bash
  go build ./... 2>&1 | tail -20
  go test ./... 2>&1 | tail -30 | tee /tmp/claude-lich-baseline-gotest.txt
  ```
- [ ] **Step 4** [W]: Paste the exact ok/fail counts and any failing packages into KNOWN FAILURES BASELINE above. Commit nothing yet.

### H1 — Sprint 2 "Two Brains, One Gate" prerequisite

- [ ] **Step 5** [B] ~2m: Look for the four Sprint 2 legs.
  ```bash
  grep -rniE 'local_llm' "$MCP" raft/ pkg/ cmd/ 2>/dev/null | head
  grep -rniE '"route"|flag\.String\("route|RouteAuto|-route' cmd/zhen-claude/ pkg/claudebridge/ 2>/dev/null | head
  grep -rniE 'IssuePendingConfirmation|AcceptToolCall|pending.?confirmation' pkg/champion/ pkg/claudebridge/ 2>/dev/null | head
  grep -rniE 'daily.?cap|DailyCap|spend\.json|reserve' pkg/claudebridge/spend.go 2>/dev/null | head
  ```
- [ ] **Step 6** [B] ~1m: Prove the CLI surface.
  ```bash
  go run ./cmd/zhen-claude -h 2>&1 | grep -iE 'route|daily-cap|dry-run|tools|effort' || echo "flags-missing"
  ```
- [ ] **Step 7** [V]: **H1 GATE (Sprint 2 prerequisite)** — Champion-confirmation-for-writes (`pkg/champion/confirm.go` present AND reachable from the bridge gate), a spend cap (`spend.go` reserve/commit), a `local_llm` MCP tool, and `-route auto` all exist.
  - If ALL present → H1 TRUE; proceed Step 10.
  - If confirmation + spend cap present but `local_llm` and/or `-route auto` are MISSING → **Step 8**.
- [ ] **Step 8** [DECIDE] ~5m: Missing `local_llm` / `-route auto`.
  - **RECOMMENDATION**: Treat the Sprint 4 scope's drivers as still buildable: the *gate* and *spend cap* are the load-bearing Sprint 2 legs and they exist (`pkg/champion/confirm.go`, `pkg/claudebridge/{gate,spend}.go`). `local_llm` (local-model MCP tool) and `-route auto` become **in-scope deliverables of Phase 5/Phase 3** of THIS plan, built on `pkg/claudebridge/route.go`'s existing `Router` interface. Record this as a scope note in the sprint doc and proceed.
  - **Rationale**: ADR-104 ships `Router` as interface-only by design; wiring an implementation is small and squarely the "drivers" scope. Blocking the whole sprint on a label would waste the session.
  - **Override ONLY if**: the Champion gate or the spend cap is itself absent/non-functional → then **Step 9** (`[ESCALATE]`).
- [ ] **Step 9** [ESCALATE]: Gate or spend cap absent.
  - **Context**: Sprint 4's entire safety model ("every tool call through the Champion gate", "hard daily spend cap") rests on Sprint 2 having delivered the gate and the cap. If `pkg/champion` has no `AcceptToolCall`/`Dispatch` path or `pkg/claudebridge/spend.go` does not reserve-before-send, the prerequisite is genuinely unmet.
  - **Why no recommendation**: Building red-team automation that can invoke tools without a working gate is exactly the thing this sprint must not do. Stevie decides whether to land Sprint 2's missing leg first or re-scope.

### H2–H3 — the gate and credentials

- [ ] **Step 10** [B][TEST] ~3m: Build + gate test.
  ```bash
  go build ./pkg/claudebridge/... ./pkg/champion/... && echo BRIDGE_OK
  go test ./pkg/claudebridge/... -run 'Gate|Spend' 2>&1 | tail -15
  go test ./pkg/champion/... -run 'Confirm|Dispatch|ToolCall' 2>&1 | tail -15
  ```
- [ ] **Step 11** [V]: **H2 GATE** — bridge builds; the gate test shows a mutating tool refused pending-confirmation. If red → `[D]` Step 11a (read `gate.go`/`confirm.go`), max 2 attempts, else Skip Protocol → STUCK REPORT (this blocks Phases 3,4,6).
- [ ] **Step 11a** [D] ~5m: `sed -n '1,120p' pkg/claudebridge/gate.go; sed -n '1,120p' pkg/champion/confirm.go` — confirm `ChampionGate.Execute` routes every `tool_use` through `AcceptToolCall`/`Dispatch`.
- [ ] **Step 12** [B] ~1m: Credentials posture (H3).
  ```bash
  printenv ANTHROPIC_API_KEY >/dev/null && echo "KEY_SET" || echo "NO_KEY"
  ant auth status 2>/dev/null | head -3 || echo "no ant cli"
  ```
- [ ] **Step 13** [V]: **H3 RESULT** — record whether a key/profile exists. **Regardless of result, the Claude driver defaults to `-dry-run`** until Phase 6 Step explicitly flips it with Stevie's word. A present key does NOT authorize live spend here.

### H4/H8 — LXD + no-egress (the confinement the whole sprint rests on)

- [ ] **Step 14** [B][BARE-METAL] ~2m: LXD + namespace reachability.
  ```bash
  lxc version 2>/dev/null | head -2 || echo "no lxc"
  LXD_BACKEND=unix go run ./cmd/cuirass -lxd-list 2>/dev/null | head || echo "cuirass-lxd check: see Phase 1"
  ```
- [ ] **Step 15** [S][B][BARE-METAL] ~2m: Prove a netns blocks egress (H8). This is the single most important preflight.
  ```bash
  sudo ip netns add "$NETNS" 2>/dev/null || true
  sudo ip netns exec "$NETNS" ip link set lo up
  sudo ip netns exec "$NETNS" ping -c1 -W2 127.0.0.1 && echo "LOOPBACK_OK"
  sudo ip netns exec "$NETNS" ping -c1 -W2 1.1.1.1 && echo "EGRESS_LEAK" || echo "EGRESS_BLOCKED"
  ```
- [ ] **Step 16** [V]: **H8 GATE (HARD)** — `LOOPBACK_OK` AND `EGRESS_BLOCKED`.
  - If `EGRESS_LEAK` → **`[ESCALATE] E1`**: the host cannot confine a fuzz target. No fuzzing proceeds without proven no-egress. STOP.
- [ ] **Step 17** [S][B] ~1m: Tear down the probe namespace (Phase 2 recreates it under config).
  ```bash
  sudo ip netns del "$NETNS" 2>/dev/null || true
  ```

### H5–H7 — existing Lich, differential oracle, register

- [ ] **Step 18** [B][TEST] ~3m: Existing Lich smoke (H5).
  ```bash
  "$LICH/lich-runner.sh" --harness 001 --time 30s 2>&1 | tail -15
  LATEST="$(ls -1dt "$LICH"/results/* | head -1)"; "$LICH/crash-triage.sh" "$LATEST" 2>&1 | tail -15
  ```
- [ ] **Step 19** [B][TEST] ~4m: Differential oracle feasibility (H6).
  ```bash
  cargo test -p monad-mbc 2>&1 | tail -15
  ls crates/monad-mbc/fuzz/fuzz_targets/ 2>/dev/null
  ls ebpf/ 2>/dev/null | grep -i monad-cpu || echo "find eBPF CPU path in Phase 7"
  ```
- [ ] **Step 20** [B] ~2m: Register generator (H7). Record the "N open" line in KNOWN FAILURES BASELINE.
  ```bash
  go run ./cmd/compliance-evidence findings 2>&1 | grep -iE 'open|claimable' | head -3
  ```
- [ ] **Step 21** [V]: **H5/H6/H7 GATE** — Lich smoke ran (crashes recorded as baseline, not regressions); `monad-mbc` tests pass; register generator prints a count. Any red → record in KNOWN FAILURES BASELINE; convert the specific dependent steps to `[BLOCKED]`, do not block the whole sprint.

### Topology decision + scope lock

- [ ] **Step 22** [DECIDE] ~5m: **Lich MCP server vs. Lich tools on the Zhen MCP server.**
  - **RECOMMENDATION**: Build a **separate `cmd/lich-mcp` server** (`$LICH_SRV`) that exposes the Lich tools, with the tool logic in a reusable `pkg/lichtools` (`$LICH_PKG`) implementing `zhentools.Provider`. Do **not** fold these into `raft/zhen_mcp_server.py`.
  - **Rationale**: (1) Blast-radius isolation — the Lich surface runs fuzzers and reads crash artifacts; keeping it out of the always-on read-only Zhen MCP server means a bug in Lich tooling cannot widen that server's surface. (2) Language — the gate, bridge, Lich runner and register generator are all Go; a Go `pkg/lichtools` reuses `pkg/champion`/`pkg/claudebridge` directly and is consumed both by the MCP server and by `cmd/lich-security`. (3) `pkg/claudebridge` already consumes `zhentools.Provider` via `pkg/mcpclient`, so a Go provider plugs into the Claude driver with no new seam. (4) It can be started/stopped independently — part of the kill switch.
  - **Override ONLY if**: Stevie has stated the MCP surface must be single-server. Then expose the tools as additional, default-OFF tools on `raft/zhen_mcp_server.py` behind an env opt-in (`ZHEN_MCP_ALLOW_LICH=1`), still routing every call through the Champion gate.
- [ ] **Step 23** [DESIGN][W] ~5m: Record the Step 22 decision, the Step 8 scope note (if taken), and the H1–H8 results as a "Phase 0 findings" block at the top of THIS file (edit in place).
- [ ] **Step 24** [SECURITY] ~3m: **Scope lock (write it down, enforce it in code later).** The server-side allowlist is: targets whose LXD name starts with `$SANDBOX_PREFIX`, OR a local fuzz harness under `$LICH/harnesses/`. Nothing else is a valid target. No network target. No host path outside `$PROJECT_ROOT` and the sandbox. This list lives in `configs/lich-mcp.yaml` (Phase 2) and is read server-side; a driver's prompt can never add to it.
- [ ] **Step 25** [C] ~1m: **COMMIT CHECKPOINT.**
  ```bash
  git add -A && git commit --no-gpg-sign -m "docs(battle-plan): S-LICH-AWAKENS Phase 0 preflight findings + scope lock"
  ```
- [ ] **Step 26** [V]: **PHASE 0 EXIT GATE** — H1,H2,H8 TRUE (or their `[ESCALATE]` resolved by Stevie); KNOWN FAILURES BASELINE filled; topology decided; scope locked; baseline committed.
  - **DoD**: all steps executed or STUCK-logged; H8 no-egress proven; zero NEW go-test failures vs Step 3; no new deps; commit made.
  - If pass → Phase 1. If H8 failed → do NOT proceed (E1).

---

## PHASE 1: LICH MCP SERVER SKELETON + SCOPE ENFORCEMENT (Steps 27–48)

**Goal**: A `cmd/lich-mcp` server (per Step 22) exposing *stubbed* Lich tools whose **scope is enforced server-side** — every tool validates its target against the allowlist before doing anything, and refuses outside it with a typed error. No real fuzzing yet; prove the shape, the gate seam, and the refusal.
**Prerequisite**: Phase 0 exit gate passed.
**Time**: 90–120 min
**Agent**: Coordinator (Developer lane)

- [ ] **Step 27** [W][CODE] ~8m: Create `$LICH_PKG/scope.go` — `type Target struct{Kind TargetKind; Name string}` (`KindSandbox`/`KindHarness`), and `func (s *Scope) Validate(t Target) error` that reads the allowlist from `configs/lich-mcp.yaml` and refuses anything else with `ErrOutOfScope`. **Scope is a struct field, never a function argument the model controls.**
- [ ] **Step 28** [W][TEST] ~6m: `$LICH_PKG/scope_test.go` — table test: a `$SANDBOX_PREFIX`-prefixed name passes; a bare hostname, an IP, a `..` path, an unlisted container all return `ErrOutOfScope`. Red first.
- [ ] **Step 29** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Scope` — expect RED (no impl). Then implement until GREEN.
- [ ] **Step 30** [V]: Scope test GREEN; every non-allowlisted input refused. If red after 2 `[D]` attempts → Skip Protocol.
- [ ] **Step 31** [C] ~1m: **COMMIT** `feat(lichtools): server-side scope allowlist (sandbox prefix + harness dir only)`.
- [ ] **Step 32** [W][CODE] ~10m: `$LICH_PKG/provider.go` — implement `zhentools.Provider`: `ListTools()` returns six tools (`lich_list_campaigns`, `lich_show_campaign`, `lich_run_harness`, `lich_read_crash`, `lich_triage`, `lich_file_finding`); `CallTool(name, input)` dispatches to stubs that first call `Scope.Validate`. Inputs are typed structs with JSON schema; `additionalProperties:false`.
- [ ] **Step 33** [W][CODE] ~6m: Classify each tool read-only vs mutating for the Champion gate: `list/show/read/triage` = read-only; `run_harness` = mutating-but-sandboxed (its own gate note); `file_finding` = mutating (writes the register) → always pending-confirmation under ADR-104 Rule 2.
- [ ] **Step 34** [W][CODE] ~10m: `$LICH_SRV/main.go` — an MCP server over stdio (mirror `pkg/mcpclient` protocol) that wraps the `pkg/lichtools` Provider **behind the Champion gate** (`champion.AcceptToolCall`/`Dispatch`), reads `configs/lich-mcp.yaml`, and logs every call via the ADR-104 `Auditor` shape. Default bind: stdio only (no TCP port) to keep it off the network.
- [ ] **Step 35** [W] ~5m: `configs/lich-mcp.yaml` — `scope.sandbox_prefix: lich-sbx-`, `scope.harness_dir: tomb/lich/harnesses`, `limits.{cpu,mem,timeout,pids}`, `netns: lich-noegress`, `kill_switch_file: /run/lich-mcp.halt`, `daily_cap_usd: 5`, `findings.auto_publish: false`. No secrets (fail-loud if a key is referenced).
- [ ] **Step 36** [C] ~1m: **COMMIT** `feat(lich-mcp): server skeleton, six stubbed tools behind Champion gate`.
- [ ] **Step 37** [W][TEST] ~8m: `$LICH_SRV/main_test.go` + `$LICH_PKG/provider_test.go` — a stub call to `lich_run_harness` with target `evil-host` is refused by scope BEFORE any gate; `lich_file_finding` is refused pending-confirmation by the gate; `lich_list_campaigns` (read-only) passes the gate.
- [ ] **Step 38** [B][TEST] ~2m: `go test ./pkg/lichtools/... ./cmd/lich-mcp/...` — GREEN.
- [ ] **Step 39** [V]: **Gate seam proven** — out-of-scope refused before the gate; mutating refused pending-confirmation; read-only allowed. If red → `[D]` Step 39a (max 2), else Skip.
- [ ] **Step 40** [C] ~1m: **COMMIT** `test(lich-mcp): scope-before-gate + pending-confirmation on file_finding`.
- [ ] **Step 41** [COMPLIANCE] ~3m: Confirm no new third-party dependency entered `go.mod` (reuse SDK/stdlib). `go mod tidy -diff` clean. If a new dep is unavoidable → ADR-096/ADR-004 register row + Barrister note; do NOT commit the dep without it.
- [ ] **Step 42** [B] ~2m: `go vet ./pkg/lichtools/... ./cmd/lich-mcp/...`; `gofmt -l` clean.
- [ ] **Step 43** [SECURITY] ~5m: **BlackMage self-review of the scope layer** — can a crafted tool input escape the allowlist? Check: symlink in `harness_dir`, `name` with embedded newline, Unicode homoglyph of the prefix, absolute path, empty target. Add a test for any gap found.
- [ ] **Step 44** [B][TEST] ~2m: Re-run scope tests after Step 43 additions — GREEN.
- [ ] **Step 45** [C] ~1m: **COMMIT** `test(lichtools): scope hardening against symlink/homoglyph/path escapes`.
- [ ] **Step 46** [W][DESIGN] ~4m: Write `tomb/lich/LICH-MCP-README.md` stub documenting the server, its tools, the scope model, and the kill switch (filled out in Phase 11).
- [ ] **Step 47** [C] ~1m: **COMMIT** `docs(lich): LICH-MCP server README stub`.
- [ ] **Step 48** [V]: **PHASE 1 EXIT GATE** — server builds; six tools listed; scope enforced server-side and refuses everything off-allowlist; gate seam proven; no new deps; tests GREEN.
  - **DoD**: all steps done/STUCK; scope refusal tested incl. adversarial inputs; zero NEW go-test failures; commit made; attack surface = stdio only, no port.
  - If pass → Phase 2.

---

## PHASE 2: THE SANDBOX SUBSTRATE — CONFINEMENT + KILL SWITCH (Steps 49–76)

**Goal**: Stand up the confined execution environment the Lich runs targets in: LXD sandbox containers named `$SANDBOX_PREFIX*` (or `nsjail`/`bwrap` fallback per H4), each in the no-egress `$NETNS`, with CPU/mem/pids/time limits, and a working kill switch. Nothing a driver asks for can run outside this.
**Prerequisite**: Phase 1 exit gate; H8 (no-egress) proven TRUE in Phase 0.
**Time**: 120–150 min
**Agent**: Coordinator (Architect + Developer lanes) — `[BARE-METAL]`/`[VM-SCAN]` heavy

- [ ] **Step 49** [DECIDE] ~3m: **Container runtime for sandboxes.**
  - **RECOMMENDATION**: Use **LXD** if H4 was TRUE (Cuirass already drives west LXD 5.21.8 via `LXD_BACKEND=unix`), creating ephemeral unprivileged containers named `$SANDBOX_PREFIX<campaign>-<nonce>` with `security.nesting=false`, `security.privileged=false`, no profile that grants a NIC with egress. If H4 was FALSE, fall back to `bwrap`/`nsjail` + the `$NETNS` namespace for harness processes.
  - **Rationale**: LXD gives per-container cgroup limits and a clean teardown; Cuirass's managed-marker convention (`user.unheaded.managed`) lets the kill switch find and stop them. The namespace fallback still satisfies no-egress + rlimits for local harnesses (which are the bulk of the work).
  - **Override ONLY if**: Stevie wants containerd/Docker parity — then mirror the limits in a compose profile with `network_mode: none`.
- [ ] **Step 50** [S][B][BARE-METAL] ~2m: Recreate `$NETNS` under config (loopback up, no default route, no veth to the host bridge).
  ```bash
  sudo ip netns add "$NETNS"; sudo ip netns exec "$NETNS" ip link set lo up
  sudo ip netns exec "$NETNS" ip route 2>/dev/null | grep -q default && echo "HAS_DEFAULT_ROUTE_BAD" || echo "NO_DEFAULT_ROUTE_GOOD"
  ```
- [ ] **Step 51** [V][BARE-METAL]: `NO_DEFAULT_ROUTE_GOOD` and egress still blocked (re-run the Step 15 egress probe). If a default route appears → `[D]` Step 51a, else `[ESCALATE] E1`.
- [ ] **Step 52** [W][CODE] ~10m: `$LICH_PKG/sandbox.go` — `type Sandbox interface { Spawn(Target, Limits) (Handle, error); Exec(Handle, cmd) (Result, error); Destroy(Handle) error }` with an `lxdSandbox` and a `netnsSandbox` impl. `Spawn` always: applies `$NETNS`, sets rlimits from config, names per `$SANDBOX_PREFIX`, refuses a Target that fails `Scope.Validate`.
- [ ] **Step 53** [W][CODE] ~6m: `Limits{CPUQuota, MemBytes, PidsMax, WallTimeout}` sourced from `configs/lich-mcp.yaml`; a target with no limit set gets the conservative default (1 CPU, 512 MiB, 256 pids, 60 s). **Never count swap** (per the no-OOM-from-UI-actions rule); preflight a memory fit check before Spawn.
- [ ] **Step 53a** [W][CODE] ~4m: Add the shared-chokepoint preflight: before any `Spawn`, check free RAM ≥ `MemBytes × safety` on the host; refuse with `ErrWontFit` rather than risk OOM/livelock.
- [ ] **Step 54** [W][TEST] ~8m: `$LICH_PKG/sandbox_test.go` — netns impl: a process spawned in the sandbox cannot reach `1.1.1.1` (skipped unless `[BARE-METAL]` env set); exceeds `WallTimeout` → killed; exceeds `PidsMax` → fork-bomb contained; `Destroy` leaves no stray process.
- [ ] **Step 55** [B][TEST][BARE-METAL] ~4m: `go test ./pkg/lichtools/ -run Sandbox` (with the bare-metal tag). GREEN.
- [ ] **Step 56** [V]: Sandbox confinement proven: no egress, time/pids/mem bounded, clean teardown. If red → `[D]` (max 2) → Skip.
- [ ] **Step 57** [C] ~1m: **COMMIT** `feat(lichtools): confined sandbox (netns no-egress + rlimits + fit preflight + teardown)`.
- [ ] **Step 58** [W][CODE] ~8m: **Kill switch.** `$LICH_PKG/killswitch.go` — a watcher on `kill_switch_file` (`/run/lich-mcp.halt`) and on SIGTERM/SIGINT; on trip: stop accepting tool calls, SIGTERM every live sandbox handle (10 s grace, then SIGKILL), destroy all `$SANDBOX_PREFIX*` containers, write a `HALTED` audit line. Mirror ADR-063's Marshal HARD-HALT discipline.
- [ ] **Step 59** [W][TEST] ~5m: `killswitch_test.go` — touching the halt file stops new `Spawn`s and tears down live handles; a second call is idempotent.
- [ ] **Step 60** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Kill` — GREEN.
- [ ] **Step 61** [V]: Kill switch halts spawns and reaps sandboxes within the grace window. If red → `[D]` (max 2) → Skip (this is safety-critical; a STUCK here blocks Phase 3 run-harness).
- [ ] **Step 62** [C] ~1m: **COMMIT** `feat(lichtools): kill switch — halt file + signals reap all sandboxes`.
- [ ] **Step 63** [W][CODE] ~6m: `$LICH_PKG/resource.go` — a global concurrency semaphore (max N live sandboxes from config) so a driver that asks for 100 harness runs cannot fork-bomb the host; excess calls queue or refuse with `ErrAtCapacity`.
- [ ] **Step 64** [W][TEST] ~4m: `resource_test.go` — N+1 concurrent `Spawn`s: the N+1th refuses/queues, never over-commits.
- [ ] **Step 65** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Resource` — GREEN.
- [ ] **Step 66** [C] ~1m: **COMMIT** `feat(lichtools): host-resource cap (max live sandboxes)`.
- [ ] **Step 67** [SECURITY][VM-SCAN] ~8m: **Confinement red-team (east, the sacrificial host — read-only on west).** From inside a sandbox, attempt: DNS lookup, raw socket, write outside the mount, nsenter to host pid, read `/proc/1`. Each must fail. Record results in `tomb/lich/LICH-MCP-README.md`.
- [ ] **Step 68** [V][VM-SCAN]: Every escape attempt in Step 67 failed. If ANY succeeded → `[ESCALATE] E1` (confinement is not real; stop).
- [ ] **Step 69** [C] ~1m: **COMMIT** `docs(lich): sandbox escape red-team results (east)`.
- [ ] **Step 70** [W][CODE] ~6m: Wire `lich_run_harness` (still stubbed on the fuzz side) to actually `Spawn`→`Exec`→`Destroy` a trivial no-op command in the sandbox, proving the MCP tool → sandbox path end-to-end.
- [ ] **Step 71** [B][TEST] ~3m: `go test ./cmd/lich-mcp/... -run RunHarnessSandbox` — the tool runs a command inside a confined sandbox and returns its bounded output.
- [ ] **Step 72** [V]: MCP tool → confined sandbox path works; scope still refuses off-allowlist targets through this path.
- [ ] **Step 73** [C] ~1m: **COMMIT** `feat(lich-mcp): run_harness executes inside a confined sandbox`.
- [ ] **Step 74** [S][B][BARE-METAL] ~1m: Confirm no stray sandboxes/namespaces leaked during the phase.
  ```bash
  lxc list 2>/dev/null | grep "$SANDBOX_PREFIX" || echo "no stray containers"
  ip netns list | grep "$NETNS" && echo "netns present (expected, config-owned)"
  ```
- [ ] **Step 75** [C] ~1m: **COMMIT** `chore(lich): phase-2 cleanup verification`.
- [ ] **Step 76** [V]: **PHASE 2 EXIT GATE** — targets run only inside confined, no-egress, resource-capped sandboxes; kill switch reaps everything; escape red-team clean; MCP→sandbox path proven.
  - **DoD**: no-egress + escape red-team PASS on east; kill switch tested; host-resource cap tested; no stray containers; zero NEW go-test failures; commits made.
  - If pass → Phase 3. If any escape succeeded → E1, STOP.

---

## PHASE 3: LICH TOOLS — REAL BEHAVIOUR THROUGH THE GATE (Steps 77–102)

**Goal**: Fill the six tools with real behaviour over the existing Lich framework: list/show campaigns from `tomb/lich/`, run a real harness in the sandbox, read crash artifacts, triage via `crash-triage.sh`, and file a finding (pending-confirmation) toward the ADR-098 register. Every call audited; spend-cap hook present for the driver phases.
**Prerequisite**: Phase 2 exit gate (confinement real).
**Time**: 120–150 min
**Agent**: Coordinator (Developer + BlackMage lanes)

- [ ] **Step 77** [W][CODE] ~8m: `lich_list_campaigns` — read `tomb/lich/LICH-*/README.md` + the ADR-062 inventory; return `{id, target, status, verdict, reference}`. Read-only; passes the gate. Cite ADR-062 as the authoritative inventory.
- [ ] **Step 78** [W][CODE] ~8m: `lich_show_campaign` — given a `LICH-NNN`, return its README, `exit-criteria.md`, `status.md`, and a results summary. Read-only.
- [ ] **Step 79** [B][TEST] ~3m: `go test ./pkg/lichtools/ -run 'List|Show'` — returns the real campaigns incl. LICH-001/002/012; an unknown id errors cleanly. GREEN.
- [ ] **Step 80** [C] ~1m: **COMMIT** `feat(lichtools): list/show campaigns from tomb/lich (ADR-062 inventory)`.
- [ ] **Step 81** [W][CODE] ~10m: `lich_run_harness` — validate the harness is under `harness_dir` (scope), `Spawn` a sandbox, run `lich-runner.sh --harness <id> --time <bounded>` inside it (time capped by config, not by the model), stream bounded output, `Destroy`. The model supplies only `{campaign_id, harness_id, duration}`; duration is clamped server-side to `[min, max]`.
- [ ] **Step 82** [W][CODE] ~6m: Enforce that `duration` from the driver is clamped (`min(request, config.max_fuzz_seconds)`) and the total across a day is bounded by a run budget — a cost control parallel to the dollar cap, for compute.
- [ ] **Step 83** [W][TEST] ~6m: `run_harness_test.go` — a bounded run of harness 001 inside the sandbox returns output; an over-long duration is clamped; an off-allowlist harness refused.
- [ ] **Step 84** [B][TEST][BARE-METAL] ~4m: `go test ./pkg/lichtools/ -run RunHarness` — GREEN.
- [ ] **Step 85** [V]: Real harness runs confined and bounded; duration clamp works. If red → `[D]` (max 2) → Skip.
- [ ] **Step 86** [C] ~1m: **COMMIT** `feat(lichtools): run_harness — bounded, clamped, confined`.
- [ ] **Step 87** [W][CODE] ~8m: `lich_read_crash` — given a results dir + crash id, return the crashing input (hex + size + SHA-256), the stack/first failing assertion, and the harness. **Read-only; never executes the crashing input.** Path-validated against the results tree.
- [ ] **Step 88** [W][CODE] ~8m: `lich_triage` — wrap `crash-triage.sh` over a results dir (run in-sandbox for safety), returning the severity categorisation (CRITICAL/HIGH/MEDIUM/LOW/INFO) and a dedup key (SHA-256 of normalised stack). This is the dedup the ADR-062 lifecycle requires.
- [ ] **Step 89** [B][TEST] ~4m: `go test ./pkg/lichtools/ -run 'ReadCrash|Triage'` — read returns bytes + hash without executing; triage returns a severity + dedup key; dedup collapses two identical crashes. GREEN.
- [ ] **Step 90** [C] ~1m: **COMMIT** `feat(lichtools): read_crash (no-exec) + triage (crash-triage.sh + dedup key)`.
- [ ] **Step 91** [W][CODE] ~10m: `lich_file_finding` — compose an ADR-098 register entry: a hand-listed `FND-NNN` (next free id), severity from triage, class, host, `opened` date, lockout risk, remediation step, plan, the dedup key, and the repro (crashing input SHA-256 + harness). **Writes a staged entry to `compliance/findings/register.yaml` ONLY after the Champion gate returns confirmed** (Rule 2: mutating + no retrieval justification → pending-confirmation). **Never auto-publishes.**
- [ ] **Step 92** [W][CODE] ~5m: After writing, call `go run ./cmd/compliance-evidence findings -adr …` is NOT auto-run (ADR-098: regen adds an untriaged git-signatures row on unsigned commits); instead, set a `needs_regen` marker the operator clears post-signing. Log the new FND-NNN.
- [ ] **Step 93** [W][TEST] ~6m: `file_finding_test.go` — without confirmation, the call returns pending-confirmation and writes nothing; with a confirmation token, it appends exactly one well-formed `FND-NNN` entry; a duplicate dedup key updates rather than duplicates.
- [ ] **Step 94** [B][TEST] ~3m: `go test ./pkg/lichtools/ -run FileFinding` — GREEN; `register.yaml` still parses (`go run ./cmd/compliance-evidence findings` does not error).
- [ ] **Step 95** [V]: Finding filing is gated, well-formed, non-duplicating, and never auto-published. If red → `[D]` (max 2) → Skip (blocks the finding path only).
- [ ] **Step 96** [C] ~1m: **COMMIT** `feat(lichtools): file_finding → ADR-098 register, pending-confirmation, no auto-publish`.
- [ ] **Step 97** [W][CODE] ~6m: Wire the ADR-104 `Auditor` into every tool call: tool, target, gate decision + reason, pending flag, duration, input size + SHA-256 (not the input), output size, truncation. JSON lines on stderr; `Auditor` interface lets a later service route to Wotan/ActionStore.
- [ ] **Step 98** [W][CODE] ~5m: Add the **daily spend-cap hook** surface (`SpendReserver` from `pkg/claudebridge/spend.go`) to the server so the Claude driver (Phase 6) reserves before each model call; local-model calls (Phase 5) are free and skip it but still count a compute-budget tick.
- [ ] **Step 99** [B][TEST] ~3m: `go test ./pkg/lichtools/... ./cmd/lich-mcp/...` — full suite GREEN; audit lines present.
- [ ] **Step 100** [SECURITY] ~6m: **BlackMage review of the tool surface** — can `file_finding` write outside the register? can `read_crash` read outside the results tree? can `triage` be pointed at an arbitrary dir? Add tests for any gap.
- [ ] **Step 101** [C] ~1m: **COMMIT** `feat(lich-mcp): per-call audit + spend-reserve hook; tool-surface hardening`.
- [ ] **Step 102** [V]: **PHASE 3 EXIT GATE** — all six tools real, confined, audited, gated; file_finding writes a valid register entry only on confirmation and never auto-publishes; spend hook present.
  - **DoD**: every tool tested incl. adversarial path inputs; register still parses; no auto-publish; no new deps; zero NEW go-test failures; commits made.
  - If pass → Phases 5 & 6 (drivers) may begin; Phase 4 (gate/confirm/route wiring) first.

---

## PHASE 4: GATE, CONFIRMATION FLOW & ROUTING WIRING (Steps 103–122)

**Goal**: Close the Sprint 2 gaps this plan owns (per Step 8): wire the out-of-band confirmation flow (`champion.IssuePendingConfirmation`) to the Lich MCP server so a human can approve a mutating call, and implement `-route auto` on `claudebridge.Router` so MINOR churn (seeds/mutations) goes local and HEAVY work (triage/finding synthesis) goes to Claude.
**Prerequisite**: Phase 3 exit gate.
**Time**: 90–120 min
**Agent**: Coordinator (Developer lane)

- [ ] **Step 103** [R] ~4m: Read `pkg/champion/confirm.go` + ADR-104 §3/open-q-2 — understand `IssuePendingConfirmation` and why the bridge left it unwired.
- [ ] **Step 104** [W][CODE] ~10m: Implement the confirmation sink for the Lich server: a pending-confirmation produces a ticket (id, tool, target, input SHA-256, requested-at) written to `var/lich/pending/<id>.json`; an operator approves via a CLI subcommand `cmd/lich-mcp confirm <id>` (local, authenticated by filesystem ownership — no network). Approval unblocks exactly that one call.
- [ ] **Step 105** [W][TEST] ~6m: `confirm_test.go` — a mutating call parks a ticket and returns pending; `confirm <id>` lets a re-issue of that exact call (same input hash) proceed; a different input hash does NOT inherit the approval; an unknown id errors.
- [ ] **Step 106** [B][TEST] ~2m: `go test ./cmd/lich-mcp/ -run Confirm` — GREEN.
- [ ] **Step 107** [V]: Confirmation flow parks and releases exactly one call, bound to its input hash. If red → `[D]` (max 2) → Skip.
- [ ] **Step 108** [C] ~1m: **COMMIT** `feat(lich-mcp): out-of-band confirmation flow (IssuePendingConfirmation wired)`.
- [ ] **Step 109** [W][CODE] ~10m: Implement `claudebridge.Router` as `autoRouter` in `$BRIDGE/route.go`: classify by `Hints` first (a Kanban/workflow label), else by prompt heuristics — seed/mutation/harness-variant generation and status/lookup → `WeightMinor` (local); crash triage, finding synthesis, novel-harness authoring, multi-crash correlation → `WeightHeavy` (Claude). Default to `WeightMinor` when uncertain (cheaper, local).
- [ ] **Step 110** [W][CODE] ~6m: Add `-route auto|heavy|minor` to `cmd/zhen-claude` (default `auto`), honouring the Router. **A `WeightHeavy` route must apply the ADR-104 filter**: refuse to send to Claude any task holding data not cleared for an external API (zero-user-data-access on prompt text) — reuse `$BRIDGE/filter.go`.
- [ ] **Step 111** [W][TEST] ~6m: `route_test.go` — "generate 50 mutated MBC seeds" → minor; "triage this crash and draft a repro" → heavy; a prompt containing an uncleared secret-shaped string → refused before send.
- [ ] **Step 112** [B][TEST] ~2m: `go test ./pkg/claudebridge/ -run 'Route|Filter'` — GREEN.
- [ ] **Step 113** [V]: Routing classifies correctly and the external-data filter blocks uncleared prompts. If red → `[D]` (max 2) → Skip.
- [ ] **Step 114** [C] ~1m: **COMMIT** `feat(claudebridge): autoRouter + -route auto; external-data filter on heavy route`.
- [ ] **Step 115** [W][CODE] ~5m: Teach the Lich MCP server to offer a `local_llm`-style local-model tool handle (the Sprint 2 `local_llm` leg this plan owns) that the driver uses for `WeightMinor` work, pointing at the local inference endpoint (`zhen-inference` 20100) — behind the same gate, free of dollar cost, counted against the compute budget.
- [ ] **Step 116** [W][TEST] ~4m: `local_llm_test.go` — the tool is listed, routes minor work to the local endpoint (mocked), never incurs a dollar reserve, and is gated like any other tool.
- [ ] **Step 117** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run LocalLLM` — GREEN.
- [ ] **Step 118** [C] ~1m: **COMMIT** `feat(lich-mcp): local_llm tool for minor-weight work (free, gated, compute-budgeted)`.
- [ ] **Step 119** [SECURITY] ~5m: **Review the routing boundary** — can a driver force a HEAVY task onto the free local path to exfiltrate via a tool, or force MINOR onto Claude to burn spend? Confirm the cap + filter + gate hold regardless of weight. Add tests.
- [ ] **Step 120** [B][TEST] ~2m: Re-run `go test ./pkg/claudebridge/... ./pkg/lichtools/...` — GREEN.
- [ ] **Step 121** [C] ~1m: **COMMIT** `test: routing boundary cannot bypass cap/filter/gate`.
- [ ] **Step 122** [V]: **PHASE 4 EXIT GATE** — confirmation flow wired; `-route auto` live; external-data filter enforced on the heavy route; `local_llm` tool present and gated; Sprint 2 gaps this plan owns are closed.
  - **DoD**: confirmation bound to input hash; routing + filter tested; local path free + capped by compute budget; no new deps; zero NEW go-test failures; commits.
  - If pass → Phases 5 & 6 (drivers) in parallel.

---

## PHASE 5: THE LOCAL-MODEL DRIVER — SEEDS & MUTATIONS AT VOLUME (Steps 123–144)

**Goal**: Use the free local model (via `local_llm`) to generate seed corpora, mutations, and harness-input variants at volume for the active campaigns — all written into `tomb/lich/seeds/` and fed to confined harnesses. This is the "generates at volume (free)" leg; it never calls Claude and never leaves the host.
**Prerequisite**: Phase 4 exit gate.
**Time**: 90–120 min
**Agent**: Agent `[P:6]` (parallel with Phase 6 once the gate is wired) — BlackMage + Developer lanes

- [ ] **Step 123** [W][CODE] ~8m: `$LICH_PKG/seedgen.go` — a driver loop that, for a given campaign, asks `local_llm` for N candidate inputs in the campaign's wire format (e.g. Monad 20-byte registers for LICH-001, MBC programs for the differential campaign), validates each is well-formed-enough to be a seed, and writes de-duplicated seeds to `tomb/lich/seeds/<campaign>/`.
- [ ] **Step 124** [W][CODE] ~6m: Mutation strategies from the BlackMage grimoire as prompt scaffolds (boundary values, truncation, type confusion, CRC-valid-but-corrupt, null bytes in every position). The local model proposes; the harness is the oracle.
- [ ] **Step 125** [W][TEST] ~6m: `seedgen_test.go` (local_llm mocked) — N requests yield ≤N de-duplicated seeds; malformed candidates are dropped; seeds land under the campaign's seed dir only (scope).
- [ ] **Step 126** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Seed` — GREEN.
- [ ] **Step 127** [V]: Seed generation writes valid, de-duplicated seeds in-scope. If red → `[D]` (max 2) → Skip.
- [ ] **Step 128** [C] ~1m: **COMMIT** `feat(lichtools): local-model seed/mutation generator (free, in-scope)`.
- [ ] **Step 129** [W][CODE] ~6m: A `generate→run→collect` orchestration for one campaign: generate a seed batch, run the harness in a sandbox over the new corpus (bounded), collect any crashes. No model call on the run side (the fuzzer mutates).
- [ ] **Step 130** [B][TEST][BARE-METAL] ~6m: Dry end-to-end on LICH-001 (Monad wire) with a tiny batch + 30 s run — produces seeds, runs confined, collects results. Record counts.
- [ ] **Step 131** [V]: End-to-end local loop runs confined and bounded; seeds persisted. If red → `[D]` (max 2) → Skip.
- [ ] **Step 132** [C] ~1m: **COMMIT** `feat(lichtools): local generate→run→collect loop (campaign-scoped, bounded)`.
- [ ] **Step 133** [W][CODE] ~5m: Compute-budget accounting — each local generation/run tick decrements a daily compute budget (sandbox-seconds); exhaustion pauses the loop and logs, never silently spins.
- [ ] **Step 134** [W][TEST] ~4m: `budget_test.go` — exhausting the compute budget pauses new runs; it resets per UTC day like the dollar cap.
- [ ] **Step 135** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Budget` — GREEN.
- [ ] **Step 136** [C] ~1m: **COMMIT** `feat(lichtools): daily compute budget on the local loop`.
- [ ] **Step 137** [SECURITY] ~5m: Can the local model smuggle an out-of-scope action through a seed (e.g. a seed that is a shell command the harness execs)? Confirm seeds are data only — the harness treats them as input bytes, never code. Add a test with an adversarial "seed".
- [ ] **Step 138** [B][TEST] ~2m: Re-run seed tests after Step 137 — GREEN.
- [ ] **Step 139** [C] ~1m: **COMMIT** `test(lichtools): seeds are inert data, never executed as code`.
- [ ] **Step 140** [W][DESIGN] ~4m: Record the local driver's role, limits, and the generate→run→collect contract in `tomb/lich/LICH-MCP-README.md`.
- [ ] **Step 141** [B][TEST] ~3m: Full `go test ./pkg/lichtools/...` — GREEN.
- [ ] **Step 142** [C] ~1m: **COMMIT** `docs(lich): local-driver contract`.
- [ ] **Step 143** [AUDIT-UPDATE] ~3m: If the local loop surfaced any real crash during Step 130, triage it and (pending-confirmation) stage an `FND-NNN` — do not count a baseline crash (H5) as new.
- [ ] **Step 144** [V]: **PHASE 5 EXIT GATE** — the free local driver generates in-scope seeds/mutations, runs them confined and bounded, accounts compute budget, and cannot execute arbitrary code via a seed.
  - **DoD**: seeds inert + in-scope; compute budget enforced; e2e loop confined; no new deps; zero NEW go-test failures; commits; any real crash staged (not auto-filed).
  - If pass → (parallel) Phase 6 and onward.

---

## PHASE 6: THE CLAUDE DRIVER — HAIKU VOLUME, OPUS TRIAGE (Steps 145–170)

**Goal**: Wire the Claude side via `zhen-claude`/`pkg/claudebridge`: Haiku (`$HAIKU`) for high-volume generation where a model helps (harness-variant drafting), Opus (`$OPUS`) for triage/finding synthesis. A hard daily dollar cap; every call through the Champion gate; **`-dry-run` by default** until Stevie authorizes live spend.
**Prerequisite**: Phase 4 exit gate (gate/route/filter); H3 (credentials posture) recorded.
**Time**: 120–150 min
**Agent**: Agent `[P:5]` — BlackMage + Developer lanes

- [ ] **Step 145** [R] ~5m: Re-read ADR-104 §1,§2,§5 (manual tool loop, `stop_reason` branching incl. `refusal`, worst-case reserve-before-send, price table). Confirm the model ids/pricing against the `claude-api` skill table: `$OPUS` $4/$20, `$HAIKU` $0.10/$0.50 per MTok.
- [ ] **Step 146** [W] ~3m: Add `$HAIKU` to `$BRIDGE/prices.json` (in $0.10 / out $0.50; a model absent from the table is priced at the table max, so this prevents over-reservation). Keep `$OPUS` row as shipped.
- [ ] **Step 147** [W][CODE] ~8m: A `claudebridge.Bridge` configured with the Lich tool Provider (`pkg/lichtools` via `pkg/mcpclient`), so Claude's `tool_use` blocks are the six Lich tools — each still through the Champion gate (mutating → pending-confirmation; `file_finding` therefore parks a ticket Phase 4's flow releases).
- [ ] **Step 148** [W][CODE] ~6m: Model selection: a `-model` override, but the default is driven by `-route auto` → Haiku for `WeightMinor`-ish bulk drafting, Opus for `WeightHeavy` triage/synthesis. Effort `medium` on Opus (ADR-104 default), `low` on Haiku bulk.
- [ ] **Step 148a** [W][CODE] ~4m: Streaming + `stop_reason` handling per ADR-104: `refusal` → typed error (fallbacks `"default"` on by default, `-no-fallback` to disable); `max_tokens`/`model_context_window_exceeded` → error, no tools run; `tool_use` → all blocks concurrently through the gate, results in one user message.
- [ ] **Step 149** [W][TEST] ~8m: `bridge_lich_test.go` against the `fake_api_test.go` harness in `$BRIDGE` — a scripted "triage this crash" turn: Claude calls `lich_read_crash` (allowed) then `lich_file_finding` (parks pending-confirmation); the loop bounds iterations; spend reserved per iteration.
- [ ] **Step 150** [B][TEST] ~3m: `go test ./pkg/claudebridge/... ./cmd/zhen-claude/...` — GREEN (all against the fake API; no live calls).
- [ ] **Step 151** [V]: The Claude driver runs the tool loop through the gate against the fake API; cap reserves before send; refusal handled. If red → `[D]` (max 2) → Skip.
- [ ] **Step 152** [C] ~1m: **COMMIT** `feat(zhen-claude): Lich tool loop — Haiku volume / Opus triage, gated, capped (fake-API tested)`.
- [ ] **Step 153** [W][CODE] ~6m: Daily dollar cap wiring — `-daily-cap-usd` default `$DAILY_CAP_USD` (5); reserve worst-case before each request (ADR-104 §5), commit real cost from `usage`; a request that would exceed the day's cap is refused before send. Confirm the `spend.json` flock + "unparseable → refuse all" behaviour.
- [ ] **Step 154** [W][TEST] ~5m: `cap_test.go` — a day already near the cap refuses the next request before any send; the ledger survives across runs; a corrupt ledger refuses everything.
- [ ] **Step 155** [B][TEST] ~2m: `go test ./pkg/claudebridge/ -run 'Cap|Spend'` — GREEN.
- [ ] **Step 156** [V]: Cap reserves/commits correctly and fails safe. If red → `[D]` (max 2) → Skip (blocks live spend).
- [ ] **Step 157** [C] ~1m: **COMMIT** `feat(zhen-claude): hard daily dollar cap (reserve-before-send, flock, fail-closed)`.
- [ ] **Step 158** [DECIDE] ~3m: **Live-API smoke — run or stay dry?**
  - **RECOMMENDATION**: Stay **`-dry-run`** for this sprint's automated execution. All logic is proven against `$BRIDGE/fake_api_test.go`. Do the one live smoke (ADR-104's `-tools none -effort low -max-tokens 1024 -daily-cap-usd 0.50 -p "Reply with the single word OK."`) ONLY if a key is present (H3) AND Stevie has said go; otherwise record it as a deferred manual step.
  - **Rationale**: next.md S17 says the stack is down and there are no creds on west; ADR-104 is explicitly not-yet-run-live. Burning spend speculatively violates the resource-awareness rule; the fake-API suite is the correct gate for autonomous work.
  - **Override ONLY if**: Stevie has authorized a live smoke this session and a credential resolves (`ant auth status`).
- [ ] **Step 159** [B] ~2m (conditional on Step 158 override): The ADR-104 live smoke with `-daily-cap-usd 0.50`. Record message id, reserved vs actual cost, the day's total. If no override → mark `[STUCK]`-deferred (needs Stevie), continue.
- [ ] **Step 160** [V]: Either dry-run proven or (if authorized) live smoke returned "OK" under $0.06 reserved. Record in README.
- [ ] **Step 161** [C] ~1m: **COMMIT** `test(zhen-claude): live-smoke deferred to dry-run (no creds / resource-awareness)`.
- [ ] **Step 162** [W][CODE] ~6m: Triage-quality harness — give Opus a crash + the code path and have it draft a finding (severity, repro, remediation) that `lich_file_finding` parks; assert the draft contains the dedup key and the repro SHA-256 (so a human reviews a complete artifact).
- [ ] **Step 163** [B][TEST] ~3m: `go test ./pkg/claudebridge/ -run Triage` (fake API) — GREEN; the drafted finding is well-formed and parks pending-confirmation.
- [ ] **Step 164** [C] ~1m: **COMMIT** `feat(zhen-claude): Opus triage drafts a complete, gated finding`.
- [ ] **Step 165** [SECURITY] ~6m: **Adversarial review of the Claude driver** — the system prompt must tell Claude not to rephrase around a gate refusal (ADR-104); confirm a refused mutating call cannot be laundered into an allowed one; confirm no prompt/response text is logged (only sizes + SHA-256). Add a test that a refusal stays refused across a re-ask.
- [ ] **Step 166** [B][TEST] ~2m: Re-run driver tests after Step 165 — GREEN.
- [ ] **Step 167** [C] ~1m: **COMMIT** `test(zhen-claude): gate refusals are terminal; no prompt text logged`.
- [ ] **Step 168** [W][DESIGN] ~4m: Document the two-driver model (free local volume / Haiku bulk / Opus triage), the cap, and the dry-run default in `tomb/lich/LICH-MCP-README.md`.
- [ ] **Step 169** [C] ~1m: **COMMIT** `docs(lich): Claude-driver contract (models, cap, dry-run default)`.
- [ ] **Step 170** [V]: **PHASE 6 EXIT GATE** — Claude driver runs the Lich tool loop through the gate (fake-API), Haiku/Opus split by route, hard dollar cap fail-closed, dry-run default, no prompt text logged, refusals terminal.
  - **DoD**: fake-API suite GREEN; cap tested fail-closed; models/pricing verified; live smoke only on authorization; no new deps beyond ADR-104's admitted SDK; zero NEW go-test failures; commits.
  - If pass → Phase 7.

---

## PHASE 7: DIFFERENTIAL MBC/UPC FUZZING — LICH-015 (Steps 171–198)

**Goal**: Open a new numbered campaign, **LICH-015**, that drives Claude/local-generated MBC programs through the reference interpreter (`crates/monad-mbc`) and the eBPF path (monad-cpu-ebpf) and turns any divergence into a finding. Coordinated with the UPC work (ADR-067 ISA v2, ADR-080 programmatic API).
**Prerequisite**: Phase 6 exit gate; H6 (differential oracle feasible) TRUE, else the Phase 0 narrowing applies.
**Time**: 120–150 min
**Agent**: Agent `[P:8]` (parallel with Phase 8) — BlackMage + Computermancer coordination

- [ ] **Step 171** [W] ~5m: Scaffold the campaign per ADR-062: `tomb/lich/LICH-015-mbc-differential/{README.md,harnesses/,seeds/,results/,exit-criteria.md,status.md}`. README states the target (MBC ISA v2 reference-vs-eBPF), the oracle (state divergence), and the success criteria.
- [ ] **Step 172** [R] ~6m: Read ADR-067 (the 5 new opcodes: FENCE/MRET/SRET/LR.W/SC.W, `MbcCpuState` 136 B, priv levels) + ADR-080 (the programmatic-API contract both paths must honour) + `crates/monad-mbc/fuzz/fuzz_targets/` to see how the reference path is already fuzzed.
- [ ] **Step 173** [DECIDE] ~5m: **The differential oracle.**
  - **RECOMMENDATION**: Run a bounded MBC program on both paths from the same initial `MbcCpuState` and compare the final state (registers, priv_level, reservation_address, a hash of touched RAM) after a fixed instruction budget. A mismatch (excluding documented intentional differences) is a divergence → finding. Use the existing `cargo-fuzz` reference harness as one arm and a Go/Rust shim that drives the eBPF path as the other.
  - **Rationale**: State-equivalence after a bounded run is the standard differential-fuzzing oracle; it needs no ground-truth, just two implementations. ADR-080's programmatic API gives a stable feed into the eBPF path.
  - **Override ONLY if**: H6 was FALSE (no shared feed) — then narrow LICH-015 to reference-interpreter coverage fuzzing and file a `[DESIGN]` note that the eBPF arm needs a feed built first (hand to the UPC sprint).
- [ ] **Step 174** [W][CODE] ~10m: `tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness` — takes a program + initial state, runs the reference interpreter (in-process), runs the eBPF path (in a sandbox, `[BARE-METAL]`), normalises both final states, and reports equal/divergent with the diff.
- [ ] **Step 175** [W][CODE] ~6m: A corpus of valid MBC programs as seeds (reuse `crates/monad-mbc/fuzz` corpus + `tomb/lich/seeds/lich_004_mbc_parser` + `lich_007_mbc_ebpf`); the local driver (Phase 5) generates mutations; Claude (Opus) proposes targeted programs exercising the ISA-v2 opcodes.
- [ ] **Step 176** [W][TEST] ~8m: `diff_harness_test` — a known-equivalent program reports equal; a deliberately divergent stub (inject a 1-bit difference in one path via a test hook) reports divergent with a correct diff. Red first.
- [ ] **Step 177** [B][TEST][BARE-METAL] ~6m: Build + run the harness on a tiny corpus. Record equal/divergent counts. (eBPF arm needs `--features ascend-linux` per CLAUDE.md's build gotcha.)
- [ ] **Step 178** [V]: The oracle correctly flags an injected divergence and passes equivalent programs. If red → `[D]` (max 2) → Skip (LICH-015 blocked; rest of sprint proceeds).
- [ ] **Step 179** [C] ~1m: **COMMIT** `feat(lich): LICH-015 MBC reference-vs-eBPF differential harness`.
- [ ] **Step 180** [B][TEST][BARE-METAL][VM-SCAN] ~8m: A bounded differential campaign (e.g. 5 min) over the mutated corpus inside the sandbox. Collect divergences + crashes to `results/`.
- [ ] **Step 181** [V]: Campaign ran confined and bounded; results captured. Record divergence count (baseline-aware: a pre-existing known divergence is not a regression).
- [ ] **Step 182** [B] ~3m: `crash-triage.sh` + the divergence classifier over the results; dedup by normalised state-diff.
- [ ] **Step 183** [DECIDE] ~3m: **Disposition of any divergence found.**
  - **RECOMMENDATION**: For each unique divergence, stage an `FND-NNN` (pending-confirmation) with the program, both final states, and the ISA/opcode implicated; tag it for the UPC sprint (ADR-067/080) and CC Computermancer. Do NOT auto-file; do NOT attempt a fix here (out of scope — this sprint finds, it does not fix the ISA).
  - **Rationale**: A reference-vs-eBPF divergence is a protocol/ISA bug; fixing it touches FROZEN-adjacent UPC internals that are Computermancer's lane and the UPC sprint's scope. Marshal: staying in lane.
  - **Override ONLY if**: a divergence is a trivial harness bug (fix the harness, not the ISA).
- [ ] **Step 184** [W] ~5m: Write the divergence seeds as regression seeds into `tomb/lich/seeds/lich_015_mbc_differential/` so the campaign re-arms on any future ISA change (ADR-062 re-run trigger).
- [ ] **Step 185** [C] ~1m: **COMMIT** `feat(lich): LICH-015 regression seeds + divergence triage`.
- [ ] **Step 186** [W] ~6m: `tomb/lich/LICH-015-mbc-differential/exit-criteria.md` per ADR-062: coverage threshold (80% of the ISA-v2 opcode decode paths), crash/divergence budget (24h continuous with zero net-new), severity gate (any divergence HIGH+ blocks a UPC release until triaged), re-run trigger (any monad-mbc or monad-cpu-ebpf change).
- [ ] **Step 187** [W] ~4m: Update the ADR-062 inventory table with LICH-015 (a new signed-commit row — note it's unsigned this session, flag for re-sign).
- [ ] **Step 188** [C] ~1m: **COMMIT** `docs(adr): ADR-062 inventory += LICH-015 (MBC differential)`.
- [ ] **Step 189** [B][TEST] ~3m: `cargo test -p monad-mbc` + `go test ./...` — confirm the new harness didn't regress the reference path or the Go build.
- [ ] **Step 190** [V]: No regression to monad-mbc or the Go build from LICH-015. If red → `[D]` (max 2) → Skip.
- [ ] **Step 191** [C] ~1m: **COMMIT** `test(lich): LICH-015 no-regression confirmation`.
- [ ] **Step 192** [SECURITY][VM-SCAN] ~6m: Confirm the eBPF arm runs confined (it attaches BPF — ensure it's on east/sandbox, never a dev host's live kernel unguarded) and that a crashing MBC program cannot escape the sandbox or wedge the host kernel. Per ADR-062 LICH-005's lesson (kernel-half fuzzing belongs on the sacrificial host).
- [ ] **Step 193** [V][VM-SCAN]: eBPF arm confined; no host-kernel wedge. If unsafe → mark the eBPF arm `[STUCK]` (needs east attended), keep the reference arm.
- [ ] **Step 194** [C] ~1m: **COMMIT** `docs(lich): LICH-015 eBPF-arm confinement notes (east only)`.
- [ ] **Step 195** [AUDIT-UPDATE] ~3m: Stage any confirmed divergence findings (pending-confirmation) and log the FND-NNN ids; CC the UPC sprint.
- [ ] **Step 196** [W][DOC-UPDATE] ~3m: Note LICH-015 in `docs/doom/` or the UPC docs per the coordination requirement (Librarian: which doc layer). Keep Doom knowledge in `docs/doom/`, not CLAUDE.md.
- [ ] **Step 197** [C] ~1m: **COMMIT** `docs: cross-link LICH-015 to UPC/Doom docs`.
- [ ] **Step 198** [V]: **PHASE 7 EXIT GATE** — LICH-015 scaffolded per ADR-062, differential oracle proven on an injected divergence, campaign ran confined, divergences triaged + staged (not auto-filed), regression seeds checked in, exit-criteria written, inventory updated, no regression.
  - **DoD**: oracle validated; campaign confined (east for eBPF); divergences staged not fixed (lane); regression seeds in; exit-criteria locked; zero NEW test failures; commits.
  - If pass → (join Phase 8) Phase 9.

---

## PHASE 8: CAMPAIGN LIFECYCLE, DEDUP & THE ADR-098 REGISTER (Steps 199–218)

**Goal**: Make the campaign machinery complete per ADR-062: numbered LICH-NNN results docs, crash dedup/triage via `crash-triage.sh`, regression seeds checked in, and a clean path from a triaged crash to a well-formed ADR-098 register entry that closes only when its evidence passes.
**Prerequisite**: Phase 3 exit gate (tools) + Phase 7 scaffold pattern.
**Time**: 90–120 min
**Agent**: Agent `[P:7]` (parallel with Phase 7) — Micromanager (lifecycle/QA) + Developer lanes

- [ ] **Step 199** [W][CODE] ~8m: `$LICH_PKG/lifecycle.go` — a campaign state machine (`opened→running→triaging→cleared|blocked`) matching ADR-062's "opened when code lands / closed when exit-criteria met". `lich_show_campaign` surfaces the current state.
- [ ] **Step 200** [W][CODE] ~6m: Standardise results docs: each run writes `results/<ts>/{summary.md, crashes/, coverage.txt}`; `summary.md` carries campaign id, HEAD sha, duration, exec count, crash/divergence counts, verdict.
- [ ] **Step 201** [W][TEST] ~5m: `lifecycle_test.go` — a campaign transitions correctly; a run with zero crashes for the configured budget flips `running→cleared`; a HIGH+ finding flips to `blocked`.
- [ ] **Step 202** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Lifecycle` — GREEN.
- [ ] **Step 203** [C] ~1m: **COMMIT** `feat(lichtools): campaign lifecycle state machine + standardised results docs`.
- [ ] **Step 204** [W][CODE] ~6m: Harden dedup: the triage dedup key (normalised stack + crashing-input class) collapses duplicate crashes across runs so a noisy campaign produces one finding per unique defect, not thousands (ADR-063 "findings volume could swamp triage" mitigation).
- [ ] **Step 205** [W][TEST] ~5m: `dedup_test.go` — 100 crashes with 3 unique stacks → 3 findings; dedup is stable across two separate runs' result dirs.
- [ ] **Step 206** [B][TEST] ~2m: `go test ./pkg/lichtools/ -run Dedup` — GREEN.
- [ ] **Step 207** [C] ~1m: **COMMIT** `feat(lichtools): stable crash dedup across runs`.
- [ ] **Step 208** [W][CODE] ~8m: The register bridge: `lich_file_finding` produces an entry that matches `compliance/findings/register.yaml`'s schema exactly (severity, class, host, opened, lockout, step, due-from-severity, plan, source-key for the dedup). ADR-098: an untriaged failing source counts; a finding closes only when its evidence passes — so the entry names a verifiable source (the regression seed re-running clean).
- [ ] **Step 209** [W][TEST] ~6m: `register_bridge_test.go` — a staged finding parses under `cmd/compliance-evidence findings`; the due date derives from severity (critical 7d / high 30d / medium 90d / low 180d); a second identical finding updates, not duplicates.
- [ ] **Step 210** [B][TEST] ~3m: `go test ./pkg/lichtools/ -run Register` + `go run ./cmd/compliance-evidence findings` (must not error, must show the staged count delta). GREEN.
- [ ] **Step 211** [V]: Findings are schema-valid, due-dated, de-duplicated, and close only on passing evidence. If red → `[D]` (max 2) → Skip.
- [ ] **Step 212** [C] ~1m: **COMMIT** `feat(lichtools): ADR-098 register bridge (schema-exact, severity-due, evidence-closes)`.
- [ ] **Step 213** [W][CODE] ~5m: Regression-seed discipline: on filing a finding, the crashing input is written as a checked-in regression seed under the campaign's `seeds/`, so the weekly ADR-062 replay re-arms it until the evidence passes.
- [ ] **Step 214** [B][TEST] ~3m: `go test ./pkg/lichtools/ -run Regression` — a filed finding leaves a checked-in seed; replaying it reproduces until a fix lands. GREEN.
- [ ] **Step 215** [C] ~1m: **COMMIT** `feat(lichtools): filing a finding checks in its regression seed`.
- [ ] **Step 216** [W][DESIGN] ~4m: Document the full lifecycle (open→run→triage→dedup→file→regress→close-on-evidence) in `tomb/lich/LICH-MCP-README.md` and cross-ref ADR-062/098.
- [ ] **Step 217** [C] ~1m: **COMMIT** `docs(lich): campaign lifecycle + register integration`.
- [ ] **Step 218** [V]: **PHASE 8 EXIT GATE** — lifecycle state machine + standardised results docs; stable cross-run dedup; schema-exact, severity-due, evidence-closing register entries; regression seeds checked in on filing.
  - **DoD**: register still generates; dedup collapses duplicates; findings close only on evidence; seeds checked in; zero NEW test failures; commits.
  - If pass → Phase 9.

---

## PHASE 9: SAFETY HARDENING & META RED-TEAM OF THE LICH ITSELF (Steps 219–240)

**Goal**: Make the safety guarantees total and prove them adversarially: kill switch, budget caps (dollar + compute), no destructive action outside sandboxes, findings never auto-published — then turn BlackMage on the Lich MCP server itself (the adversary auditing the adversary) and open LICH-016 against this new mutation surface (ADR-062 activation rule #2: a new mutation-power endpoint landed).
**Prerequisite**: Phases 3–8 exit gates.
**Time**: 90–120 min
**Agent**: Coordinator — BlackMage lead, Developer support, Marshal enforcement

- [ ] **Step 219** [SECURITY] ~6m: **Enumerate the Lich MCP server's own attack surface** (ADR-062 activation: a new HTTP/stdio endpoint with mutation power + a new auth surface landed → a campaign MUST open). List: scope bypass, gate bypass, confirmation-token forgery, cap bypass, kill-switch defeat, audit evasion, seed-as-code, finding forgery, sandbox escape, path traversal.
- [ ] **Step 220** [W] ~5m: Scaffold `tomb/lich/LICH-016-lich-mcp-self/{README.md,harnesses/,seeds/,results/,exit-criteria.md,status.md}` — the campaign that fuzzes the Lich server's own tool inputs + gate path.
- [ ] **Step 221** [W][CODE] ~8m: `LICH-016` harness — fuzz every tool's JSON input (malformed, oversized, unicode, nested, path-escape, newline-injection) and assert: scope refuses or the gate refuses, never a side effect outside the sandbox/register, never an uncaught panic.
- [ ] **Step 222** [B][TEST] ~5m: Run LICH-016 for a bounded window; triage. Any panic/side-effect is a CRITICAL self-finding.
- [ ] **Step 223** [V]: Zero scope/gate/cap/kill-switch bypasses; zero panics; zero side effects outside sandbox/register. If ANY → fix before proceeding (this is the sprint's core guarantee) or `[ESCALATE] E2`.
- [ ] **Step 224** [C] ~1m: **COMMIT** `feat(lich): LICH-016 — fuzz the Lich MCP server's own surface`.
- [ ] **Step 225** [SECURITY] ~5m: **Confirmation-token forgery test** — can a driver forge/replay a confirmation ticket, or inherit one across a changed input hash? Must fail (Phase 4 bound it to input SHA-256). Add a test.
- [ ] **Step 226** [SECURITY] ~5m: **Kill-switch defeat test** — can a live sandbox survive a halt? can a driver suppress the halt file watcher? Must not. Add a test; confirm reap within grace.
- [ ] **Step 227** [SECURITY] ~5m: **Cap bypass test** — can parallel calls race the dollar reserve past the cap, or the compute budget? The flock + reserve-before-send must hold; two in-flight reservations are conservative (ADR-104 limit noted). Add a test.
- [ ] **Step 228** [SECURITY] ~5m: **Auto-publish test** — assert there is NO code path from a finding to a public surface (no Wotan publish of finding bodies, no git push, no external post); findings land only in the gated register. Grep + a test.
- [ ] **Step 229** [B][TEST] ~3m: Run the full safety suite (225–228). GREEN.
- [ ] **Step 230** [V]: All four safety-invariant tests pass. If any red → fix or `[ESCALATE] E2` (a safety invariant is the non-negotiable product of this sprint).
- [ ] **Step 231** [C] ~1m: **COMMIT** `test(lich): safety invariants — no token forgery / kill-switch defeat / cap race / auto-publish`.
- [ ] **Step 232** [W] ~5m: `tomb/lich/LICH-016-lich-mcp-self/exit-criteria.md` — coverage of the gate + scope + cap paths; 24h zero net-new; any bypass is CRITICAL and blocks the server from any non-dry-run live use.
- [ ] **Step 233** [W] ~4m: Add LICH-016 to the ADR-062 inventory (unsigned-this-session, flag for re-sign).
- [ ] **Step 234** [C] ~1m: **COMMIT** `docs(adr): ADR-062 inventory += LICH-016 (self red-team)`.
- [ ] **Step 235** [AUDIT-UPDATE] ~4m: File (pending-confirmation) any self-finding from LICH-016 as `FND-NNN`; a CRITICAL self-finding keeps the server dry-run-only until fixed.
- [ ] **Step 236** [SECURITY][COMPLIANCE] ~4m: Confirm the server honours zero-user-data-access: nothing it sends to Claude, logs, or files contains user data; prompt text is filtered (Phase 4) and only sizes/SHA-256 are logged (Phase 3). MoatGhost-lane check.
- [ ] **Step 237** [B][TEST] ~3m: Full `go test ./pkg/lichtools/... ./cmd/lich-mcp/... ./pkg/claudebridge/...` + `cargo test -p monad-mbc` — GREEN.
- [ ] **Step 238** [V]: Full suite GREEN; safety invariants proven; LICH-016 open with exit-criteria. If red → `[D]` (max 2) → Skip.
- [ ] **Step 239** [C] ~1m: **COMMIT** `test(lich): full-suite green after safety hardening`.
- [ ] **Step 240** [V]: **PHASE 9 EXIT GATE** — kill switch, dollar + compute caps, no-destructive-outside-sandbox, no-auto-publish all proven adversarially; LICH-016 (self red-team) open; zero bypasses; zero-user-data-access confirmed.
  - **DoD**: all four safety invariants tested; LICH-016 scaffolded + exit-criteria; any self-finding staged; MoatGhost check; zero NEW test failures; commits.
  - If pass → Phase 10.

---

## PHASE 10: CTF MODE (ADR-063) — DESIGNED, GATED, NOT ACTIVATED (Steps 241–255)

**Goal**: Plan ADR-063's "Akira randomly summons the Lich for CTF mode" to the point it can be switched on in one sentence later — but leave it **gated** behind ADR-062's activation rule (LICH-001, LICH-002, and one of LICH-012/013 clean for 7 consecutive days). Build nothing that runs against a live service this sprint.
**Prerequisite**: Phase 9 exit gate.
**Time**: 60–75 min
**Agent**: Coordinator — BlackMage + Micromanager, Marshal enforcement (this phase is design-only by law)

- [ ] **Step 241** [R] ~5m: Re-read ADR-063 (round mechanics, eligibility list, frequency, Marshal HARD-HALT triggers, findings disposition through Champion) and ADR-029 (Akira's existing summon/tick scope).
- [ ] **Step 242** [V]: **ACTIVATION-RULE GATE (ADR-063 §"Why this is parked").** Check: are LICH-001, LICH-002, and one of LICH-012/013 running clean with zero net-new findings for 7 consecutive days?
  ```bash
  go run ./cmd/compliance-evidence findings 2>&1 | grep -iE 'LICH|open' | head
  ls -1dt tomb/lich/results/* | head
  ```
  - Per KNOWN FAILURES BASELINE, **LICH-012 is OPEN** → the rule is NOT met → CTF stays design-only. Record this verdict.
- [ ] **Step 243** [DECIDE] ~3m: **How far to build CTF now.**
  - **RECOMMENDATION**: Build the **config and the eligibility list only** (`configs/lich-ctf-eligible.yaml`, defaulting every service to `eligible: false`) plus a `cmd/akira` summon-codepath **stub guarded by a hard `if !activated { return ErrCTFNotActivated }`** where `activated` reads the 7-day-clean condition, never a flag. Write the round-journal layout. Do NOT wire Akira's tick to actually summon.
  - **Rationale**: ADR-063 says activation is "one sentence in a future commit"; having the config + guarded stub + journal ready makes that sentence a real one-liner while the guard makes accidental activation impossible. Marshal: this phase is design-only; the guard is what keeps it in lane.
  - **Override ONLY if**: Stevie has typed the activation sentence AND the 7-day rule is met (it is not, per Step 242) — then it's a separate sprint, not this one.
- [ ] **Step 244** [W] ~6m: `configs/lich-ctf-eligible.yaml` — the ADR-063 initial list (zhen-agentd `/api/v1/tool/exec` YES, shield YES, wotan YES, kanban YES; Sophia BPF / postgres / llama-server NO), every entry gated `eligible: false` until the service proves robust AND the rule is met.
- [ ] **Step 245** [W][CODE] ~8m: `cmd/akira/lich_summon.go` (stub): the round mechanics (random eligible target, random duration {15,30,60}min, opt-out tripwire topic, Marshal HARD-HALT conditions) behind `ErrCTFNotActivated`. The summon POSTs to the **Lich MCP server's confined surface**, never a raw fuzzer against a live service without the gate.
- [ ] **Step 246** [W][CODE] ~5m: The HARD-HALT wiring (reuse Phase 2 kill switch): `/health` >60 s non-200, lateral-movement finding (a non-eligible service implicated), `lich.ctf.abort` topic, Akira heartbeat loss → SIGTERM the round, journal `ABORTED`.
- [ ] **Step 247** [W][TEST] ~5m: `lich_summon_test.go` — with `activated=false` (the only state this sprint), every summon returns `ErrCTFNotActivated` and does nothing; the HARD-HALT conditions are unit-tested against a fake round.
- [ ] **Step 248** [B][TEST] ~2m: `go test ./cmd/akira/ -run Summon` — GREEN; no live-service interaction possible.
- [ ] **Step 249** [V]: CTF summon is inert (gated off) and HARD-HALT logic is correct. If red → `[D]` (max 2) → Skip.
- [ ] **Step 250** [C] ~1m: **COMMIT** `feat(akira): CTF summon stub — hard-gated off (ADR-063 activation rule unmet)`.
- [ ] **Step 251** [W] ~5m: `tomb/lich/ctf-rounds/README.md` — the journal layout (`<date>-<service>/{round.json, findings/}`), the Champion-gated kanban-autofile path (ADR-063: lich-ctf-finding justification → Rule-2 pending-confirmation → Stevie approves), and the new Wotan topics (`lich.ctf.{findings,aborted,summoned}`) that need ML-DSA-65 signing before activation.
- [ ] **Step 252** [W][DOC-UPDATE] ~4m: Append an "ADR-063 readiness" note to ADR-063 (or a sibling doc): what is built (config + guarded stub + journal), what remains (the 7-day-clean condition + the one-sentence activation + the topic-signing entries), and the exact activation checklist.
- [ ] **Step 253** [C] ~1m: **COMMIT** `docs(adr): ADR-063 readiness note — built vs. remaining, activation checklist`.
- [ ] **Step 254** [V]: Nothing in this phase can run against a live service; the activation gate reads the 7-day rule, not a flag. Re-confirm with a grep that no code path reaches a live target without the gate.
- [ ] **Step 255** [V]: **PHASE 10 EXIT GATE** — CTF mode designed to one-sentence-activation readiness, hard-gated off behind the ADR-062 activation rule, eligibility list defaults all-false, HARD-HALT logic unit-tested, nothing runs live.
  - **DoD**: activation rule verified unmet (LICH-012 open); summon inert; HARD-HALT tested; readiness note written; zero live interaction; commits.
  - If pass → Phase 11.

---

## PHASE 11: DOCS, LIBRARIAN RIPPLE, DoD & SPRINT CLOSE (Steps 256–268)

**Goal**: Ripple the docs (Librarian), finalise the Lich MCP README and the ADR updates, confirm the whole sprint's Definition of Done, and close with a clean verification sweep. No push.
**Prerequisite**: Phases 0–10 exit gates.
**Time**: 60–75 min
**Agent**: Coordinator — Librarian + Micromanager lanes

- [ ] **Step 256** [R] ~4m: Collect every doc touched (`tomb/lich/LICH-MCP-README.md`, ADR-062 inventory, ADR-063 readiness, ADR-104 open-q-2 now partly closed, `configs/lich-mcp.yaml`, `configs/lich-ctf-eligible.yaml`, LICH-015/016 campaign docs).
- [ ] **Step 257** [W][DOC-UPDATE] ~6m: **Librarian ripple** — ensure the document web is consistent: CLAUDE.md (a one-line pointer to the Lich MCP surface under the security section, if warranted), the wiki security pages, `docs/security/lich-*.md`, THIRD_PARTY/dep notes if any dep landed. Keep Doom/UPC specifics in `docs/doom/`.
- [ ] **Step 258** [W][DOC-UPDATE] ~5m: Finalise `tomb/lich/LICH-MCP-README.md`: topology (Step 22), the six tools + classification, scope model, sandbox + no-egress + kill switch, the two drivers, the caps (dollar + compute), the register path, the CTF-readiness gate, and the operator runbook (start/stop/kill/confirm).
- [ ] **Step 259** [W] ~4m: A runbook under `runbooks/security/` for operating the Lich MCP server (start, run a campaign, triage, confirm a finding, hit the kill switch) — per the Zhenai-authors-runbooks direction.
- [ ] **Step 260** [C] ~1m: **COMMIT** `docs(lich): final README + runbook + document-web ripple`.
- [ ] **Step 261** [B][TEST] ~5m: **Full regression sweep.**
  ```bash
  go build ./... 2>&1 | tail -10
  go test ./... 2>&1 | tail -20
  cargo test -p monad-mbc 2>&1 | tail -10
  go vet ./pkg/lichtools/... ./cmd/lich-mcp/... ; gofmt -l pkg/lichtools cmd/lich-mcp
  ```
- [ ] **Step 262** [V]: **NO NEW FAILURES vs the KNOWN FAILURES BASELINE** (Step 3). Any new red is this sprint's and must be fixed or `[STUCK]`-logged. `go mod tidy -diff` clean (no stray dep).
- [ ] **Step 263** [COMPLIANCE] ~3m: SPDX headers on every new `.go`/`.yaml`/`.md`; `gitleaks`/secret-scan clean (no key in repo); if the ADR-104 SDK was the only external dep, confirm its ADR-096/004 register row covers this use.
- [ ] **Step 264** [AUDIT-UPDATE] ~3m: Confirm the ADR-098 register still generates and the delta is exactly the findings this sprint staged (pending-confirmation), none auto-published, `needs_regen` marker set for post-signing regeneration.
- [ ] **Step 265** [SECURITY] ~4m: **Final BlackMage/Marshal sign-off checklist** — scope enforced server-side (not prompt); confinement + no-egress proven; kill switch reaps; dollar + compute caps fail-closed; findings gated + never auto-published; CTF hard-gated off; LICH-015 + LICH-016 open with exit-criteria; no third-party target reachable.
- [ ] **Step 266** [W] ~4m: Write/refresh `~/tmp/next.md` S-LICH-AWAKENS handoff: what shipped, the Phase 0 decisions, the dry-run default (live smoke deferred), the staged findings awaiting confirmation, LICH-015/016 status, CTF gated, and "re-sign unsigned commits before push; never push".
- [ ] **Step 267** [C] ~1m: **COMMIT** `chore(lich): sprint close — handoff, DoD sweep, register delta verified`.
- [ ] **Step 268** [V]: **PHASE 11 EXIT GATE + SPRINT DoD** —
  - [ ] All six Lich tools work, confined, gated, audited.
  - [ ] Scope enforced server-side (allowlist + netns no-egress + rlimits), proven against adversarial input + escape attempts.
  - [ ] Two drivers: free local volume + Haiku bulk / Opus triage; dry-run default; hard dollar cap + compute budget fail-closed.
  - [ ] LICH-015 (MBC reference-vs-eBPF differential) + LICH-016 (self red-team) open with exit-criteria + regression seeds; divergences staged, not fixed (lane).
  - [ ] Findings flow to the ADR-098 register pending-confirmation, dedup'd, severity-due, evidence-closing, never auto-published.
  - [ ] Kill switch + confirmation flow + `-route auto` + `local_llm` closed the Sprint 2 gaps this plan owned.
  - [ ] CTF mode designed to one-sentence readiness, hard-gated off (ADR-062 rule unmet: LICH-012 open).
  - [ ] Docs rippled (Librarian); runbook written; `next.md` handoff written.
  - [ ] Zero NEW test failures vs baseline; zero new deps without register; SPDX + secret-scan clean; all commits `--no-gpg-sign`; **nothing pushed**.
  - If all checked → sprint complete. Else → STUCK REPORT prioritised by downstream impact.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1 — Egress leaks from a sandbox (H8/E1).** STOP all fuzzing immediately. `touch /run/lich-mcp.halt` (kill switch). Verify with `ip netns exec $NETNS ping -c1 -W2 1.1.1.1` (must fail). Do not resume until `EGRESS_BLOCKED`. If LXD, confirm the container has no NIC profile granting egress (`lxc config show <c>`). This is a HARD STOP — confinement is the sprint's precondition.

**A2 — A sandbox won't die / stray containers.** `cmd/lich-mcp` kill switch first. Then manually: `for c in $(lxc list -c n --format csv | grep ^$SANDBOX_PREFIX); do lxc delete -f "$c"; done`. Netns: `sudo ip netns pids $NETNS | xargs -r sudo kill -9`. Confirm `lxc list | grep $SANDBOX_PREFIX` empty.

**A3 — Spend cap behaving wrong / runaway Claude spend.** The cap reserves before send; if spend climbs unexpectedly, check `$XDG_STATE_HOME/unheaded/zhen-claude/spend.json` (flock'd). A corrupt ledger refuses every request by design (ADR-104). Immediate stop: run the driver with `-dry-run` or `-daily-cap-usd 0`. Never bypass the cap.

**A4 — The gate is letting a mutating call through.** HARD STOP (`[ESCALATE] E2`). A mutating tool with no retrieval justification MUST be refused pending-confirmation (ADR-104 Rule 2). Re-run `go test ./pkg/claudebridge/ -run Gate` and `./pkg/champion/ -run Confirm`. Do not run any driver until green — this is the core safety invariant.

**A5 — A finding leaked to a public surface.** HARD STOP. Grep for any path from a finding to Wotan-publish/git-push/external-post (`grep -rn 'Publish\|push\|http.Post' pkg/lichtools`). Findings land ONLY in the gated register. Revert the offending code; add a Phase-9 Step-228 regression test.

**A6 — eBPF differential arm wedges the kernel (LICH-015).** Only ever run the eBPF arm on east (sacrificial host), confined, per ADR-062 LICH-005's lesson. If a dev host's kernel is affected, reboot; move the arm to east; mark the arm `[STUCK]`-attended.

**A7 — Live API smoke refused / classifier decline.** ADR-104 fallbacks `"default"` re-run server-side; `-no-fallback` disables. A persistent refusal on a benign security-tooling prompt is expected occasionally — stay `-dry-run`, the fake-API suite is the gate for autonomous work. Never rephrase around a refusal to force a tool (the system prompt forbids it).

**A8 — Register won't generate after file_finding.** `go run ./cmd/compliance-evidence findings` must parse `register.yaml`. If it errors, the staged entry is malformed — fix the YAML to match the schema (ADR-098 §1); do not hand-edit the generated section. The `needs_regen` marker defers `-adr` regeneration until commits are signed.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Steps | Agent | Parallelizable | Depends on | ~Time |
|---|---|---|---|---|---|
| 0 Preflight | 1–26 | Solo/Coordinator | no | base 443de3f82 | 60–75m |
| 1 MCP skeleton + scope | 27–48 | Coordinator (Dev) | no | P0 | 90–120m |
| 2 Sandbox + kill switch | 49–76 | Coordinator (Arch+Dev) | no | P1, H8 | 120–150m |
| 3 Lich tools real | 77–102 | Coordinator (Dev+BM) | no | P2 | 120–150m |
| 4 Gate/confirm/route | 103–122 | Coordinator (Dev) | no | P3 | 90–120m |
| 5 Local driver | 123–144 | Agent `[P:6]` | with P6 | P4 | 90–120m |
| 6 Claude driver | 145–170 | Agent `[P:5]` | with P5 | P4, H3 | 120–150m |
| 7 LICH-015 differential | 171–198 | Agent `[P:8]` | with P8 | P6, H6 | 120–150m |
| 8 Lifecycle + register | 199–218 | Agent `[P:7]` | with P7 | P3, P7 scaffold | 90–120m |
| 9 Safety + self red-team | 219–240 | Coordinator (BM) | no | P3–P8 | 90–120m |
| 10 CTF design (gated) | 241–255 | Coordinator (BM+MM) | no | P9 | 60–75m |
| 11 Docs/DoD/close | 256–268 | Coordinator (Lib+MM) | no | P0–P10 | 60–75m |

**Critical path** (longest sequential chain): P0 → P1 → P2 → P3 → P4 → P6 → P7 → P9 → P10 → P11. Phases 5 (with 6) and 8 (with 7) ride parallel and are NOT on the critical path. Minimum wall-clock ≈ the sum of the critical-path phases ≈ 10–13 h; with the parallel pair folded in, a solo coordinator runs ~13–16 h.

**Serialization rule (Marshal):** P1→P2→P3→P4 MUST be sequential — you cannot run tools before the sandbox, or drive before the gate. P9's safety red-team MUST pass before P10, and P11 before any thought of non-dry-run live use.

---

## APPENDIX C: QUICK REFERENCE

**Model ids / pricing (claude-api skill, cached 2026-10-06 — never append a date suffix):**
- `claude-opus-5-5` — triage/synthesis — $4 in / $20 out per MTok, cache read $0.20, effort default `medium`.
- `claude-haiku-5-5` — volume/bulk drafting — $0.10 in / $0.50 out per MTok (≤100K prompt).
- A model absent from `$BRIDGE/prices.json` is priced at the table max (over-reserves) — so add `$HAIKU` (Step 146).

**The six Lich tools (all through the Champion gate):**
| Tool | R/W | Gate verdict |
|---|---|---|
| `lich_list_campaigns` | read | allowed |
| `lich_show_campaign` | read | allowed |
| `lich_run_harness` | mutating (sandboxed) | allowed-with-clamp; runs confined only |
| `lich_read_crash` | read (never executes input) | allowed |
| `lich_triage` | read (runs crash-triage.sh in sandbox) | allowed |
| `lich_file_finding` | mutating (writes register) | pending-confirmation (ADR-104 Rule 2); never auto-publish |

**Scope allowlist (server-side, config `configs/lich-mcp.yaml`, never prompt):**
- Target = LXD container named `lich-sbx-*` OR a harness under `tomb/lich/harnesses/`. Nothing else. No network target. No path outside `$PROJECT_ROOT`/sandbox.

**Safety invariants (Phase 9 proves each):** no-egress confinement; kill switch (`/run/lich-mcp.halt` + signals) reaps all sandboxes; hard daily dollar cap (reserve-before-send, flock, fail-closed) + daily compute budget; confirmation token bound to input SHA-256; no auto-publish of findings; no prompt/response text logged (sizes + SHA-256 only).

**Campaigns opened this sprint (ADR-062 inventory):**
- **LICH-015** — MBC ISA-v2 reference (`crates/monad-mbc`) vs eBPF (`monad-cpu-ebpf`) differential; oracle = final-state equivalence after a bounded run; divergences → findings (staged, handed to the UPC sprint; ADR-067/080).
- **LICH-016** — the Lich MCP server's own surface (scope/gate/cap/kill-switch/auto-publish); any bypass is CRITICAL and keeps the server dry-run-only.

**CTF activation (ADR-063) — NOT met this sprint:** requires LICH-001, LICH-002, and one of LICH-012/013 clean 7 consecutive days. LICH-012 is OPEN → CTF stays design-only, hard-gated off.

**Build gotchas:** eBPF differential arm needs `cargo build --release -p monad-cpu-ebpf --features ascend-linux` (CLAUDE.md). Commits `--no-gpg-sign` (key not cached); re-sign before push; **never push**; worktree touches only its own branch.

**`[ESCALATE]` items:** E0 (Step 9 — Sprint 2 gate/cap genuinely absent), E1 (Step 16/51/68 — confinement/no-egress not real), E2 (Step 223/230/A4/A5 — a safety invariant bypassed).

---

*S-LICH-AWAKENS Battle Plan — Forged 2026-10-23*
*12 Phases. 268 Steps. The adversary that never sleeps, held on a server-side leash: confined, gated, capped, and auditable.*
*Break only what we own. Confine everything. Publish nothing without a human. Peace and love.*

