# S-G9-FOLD BATTLE PLAN — 10 Phases, 78 Steps

**Date**: 2026-11-04
**Sprint**: S-G9-FOLD: DP-203. Each parked G9 control plane gets one parked ADR; compliance alone folds as code tonight.
**Series**: `docs/battle-plans/SPRINT-SERIES-2026-11-04.md`, lane D, after S-WIRE-HONEST. Its rules 1-17 bind this plan.
**Prerequisite**: Phases 0-4: none beyond preflight. Phase 5: S-WIRE-HONEST merged into develop, because the series runs this lane after it and both edit `pkg/protocol/bpfschema`; nothing in Phase 5 calls its code. West kernel `6.17.0-41-generic`.
**Target**: eight parked ADRs; `compliance-evidence wire` real, off unless `--serve`, honest when empty; no running service changes behaviour; seven folds wait on Stevie.
**Duration**: about 6 agent-hours, 3 hours wall.
**Branch**: `sprint/s-g9-fold`, worktree `/home/govan/tmp/wt/S-G9-FOLD`.
**Agent Strategy**: Phase 1 one Haiku agent. Phases 2-4 and 6 Sonnet in sequence; Phase 5 Opus. Every [S] step is Opus. See "Swarm table".
**Commit Cadence**: every 4-5 steps of change at each [C] (a run of verify-only steps has nothing to commit) through `commit.sh`: signed, named files only, `[PLAN S-G9-FOLD] Steps X-Y: ...` with the series trailers. Only commit after the last [V] passed. Never push. Never `git add -A`.
**Stuck Protocol**: skip after 3x the estimate or 2 failed debug attempts: mark `[STUCK]`, commit, move on. Questions go to "For Stevie" in the progress file. Haiku and Sonnet never improvise: any output the plan did not predict is `[STUCK]`.
**Log**: `docs/battle-plans/progress/S-G9-FOLD-progress.md`.
**Scope fence**: nothing deleted. No code in `cmd/unheaded-daemon`, `cmd/monad`, `cmd/unheaded-auth`, `cmd/unheaded-baseline`, or any `services/<plane>` but `services/compliance`. No unit, no compose change, no uid, no port constant, no eBPF program change, no catalog change.

Every command runs from `/home/govan/tmp/wt/S-G9-FOLD`. Shell state does not persist, so start each one with `cd /home/govan/tmp/wt/S-G9-FOLD && P=/var/tmp/S-G9-FOLD-payloads && `. Use `GOMAXPROCS=4`, `CARGO_BUILD_JOBS=4`; build output under `/var/tmp` or the worktree. All scratch files live in `$P`, this plan's private directory.

## WHAT THE TREE SAID (measured 2026-11-04 on develop `ba87679b`)

Common to all eight: `Start` binds an open listener on `:<port>`, every interface, no auth, no TLS. Only `services/loadbalancer` touches a BPF map; the others call themselves control planes for a program they never speak to. None of the eBPF halves is attached by anything. Their metrics have no input, so each reports a calm empty state.

| Plane | Asked | Judgement | Why | ADR |
|---|---|---|---|---|
| compliance | unheaded-baseline or compliance-evidence | **compliance-evidence** | baseline is `Type=oneshot` with `CAP_DAC_READ_SEARCH` (reads `/etc/shadow`): no listener there. compliance-evidence already dispatches subcommands | 127 |
| firewall | cuirass | **Right for intent only** | Cuirass has an empty `CapabilityBoundingSet` and runs once; XDP runs per node. A node writer is missing | 128 |
| qos | cuirass | **Right, without the 10 Hz publisher** | policy is desired state; queue stats are node facts | 129 |
| nfv | cuirass | **Does not fit alone: a question for Stevie** | the job is loading tail-called XDP, and Cuirass has no `CAP_BPF`; the reload API takes a `prog_fd` integer over HTTP and swaps nothing; `chain_id` overloads frozen `circuit_state`. `upc-cartridgectl` is signed but loads MBC into one interpreter, is ORPHAN, and cartridges miss two benchmark bars | 130 |
| failover | cuirass | **Right as actuator; the scoring stays out** | Cuirass probes and owns the proxy device; deciding is Akira's (ADR-029). `FailCount` is never incremented outside tests | 131 |
| loadbalancer | cuirass | **Right, the cleanest** | backends are what Cuirass knows; the map write goes to the node writer | 132 |
| version | monad | **Right by name, nothing to do yet** | one wire version exists; empty capability set; 243,780 verified instructions on a path that cannot fire | 133 |
| keymgmt | unheaded-auth | **Right for a registry, wrong for generation** | `GenerateKey` generates no key; custody (DP-06) keeps private keys off the issuer; no `CAP_BPF` | 134 |

Other facts that shape the plan:

| Fact | Effect |
|---|---|
| An empty `services/compliance` reports **100 %** and audit completeness 1.0 | Fixed first (Phase 2): `no_data`, 0, never 100 |
| The score reads `len(s.violations)`, and the list trims at 10,000: 300,000 blocked of 300,000 scored 96.67 | Phase 2 scores from `Blocked`; `auditTotal` counts audit records for the same reason |
| `compliance-evidence`'s dependency set | **Grows from 237 to 414 packages** (first-party 10 to 25, measured through Phase 4): the timer compiles grpc and `pkg/auth` it never runs. Step 78 proves the timer survives it |
| `findings --help` exits 1 | `wire` follows ADR-111 through `cmdline.HandleParseError`. The other subcommands keep the bug: For Stevie |
| `RecordAllowed`, `RecordViolation` have no production caller | `--audit-ring` is the first real input (Phases 4-5) |
| `compliance_xdp` is loaded by nothing | The ring is exercised by a `BPF_PROG_TEST_RUN` test, not on an interface |
| Routes: firewall and qos both `/api/v1/policy`, `/api/v1/stats`; all `/health`, `/metrics`; `cmd/monad` has `/api/v1/stats` | Later folds mount under a prefix (ADRs); compliance has its own server |
| `compliance-evidence` default path is a **user timer on `go run` from `~/tmp/unheaded`** | A merge to develop changes what it compiles at its next run. Default path must stay identical: Phase 6 proves it |
| 19110 is `ports.Firewall + 100`, unregistered | ADR-132 |
| TRUE-WIRE C1-C6 default DELETE | DP-203 overrides; S-WIRE-HONEST's Appendix C records it |
| Rehearsal | Phases 2, 3, 4 ran on a scratch snapshot; output below is measured |

## FOR STEVIE (nothing here stops the plan)

1. **NFV**: you said Cuirass; ADR-130 sets out three options and recommends none, because cartridges are a different mechanism. Your call before any code.
2. **The node writer (ADR-128)**: one answer gates four folds (firewall, qos, loadbalancer, failover), five if NFV stays in Cuirass. Recommendation: ADR-125's Go writer over `pkg/ebpf.PinnedMap`. `ebpf-loader --watch` would need new crates, a rule 4 confirm.
3. **keymgmt**: ADR-134 keeps generation out of `unheaded-auth`. It also says the fold can wait for ADR-119.
4. **version**: ADR-133 asks whether it is worth folding before a v0x02 draft exists.
5. **compliance**: `wire` results stay out of the evidence file until you say which controls they feed (ADR-127 Q2).
6. Once merged, the wire plane is reachable but has no data source on a host: nothing loads `compliance_xdp`. That is ADR-127's "not done here", along with east (no deb, so nothing runs it there) and who may open the `AUDIT_LOG` pin.
7. `--serve` under the operator's account has `sudo -n` behind it. A lab test only, until `wire` has its own nologin account (ADR-127).
8. `compliance-evidence`'s build grows from 237 to 414 packages. If that bothers you, `wire` can be its own build tag later.
9. `findings`, and the other subcommands parsed the same way, exit 1 on `--help` against ADR-111. Not fixed here.

## LEGEND

[B] bash · [V] verify, must pass · [D] debug, only on failure · [W] write or edit a file · [R] read · [S] sudo · [P] parallel with other [P] steps of the phase · [C] commit checkpoint · [STUCK] skipped under the Skip Protocol · [BLOCKED] waits on an upstream STUCK or on Stevie's reading · [DECIDE] decided here, proceed; override only on the stated condition

---

## PHASE 0: PREFLIGHT (Steps 1-7)

**Goal**: A worktree that builds, payloads out, baselines recorded.
**Prerequisite**: This plan on disk at `/home/govan/tmp/unheaded/docs/battle-plans/S-G9-FOLD-battle-plan.md`.
**Time**: 15m · **Agent**: Solo · **Model**: Sonnet

- [ ] **Step 1** [B] ~1m: 
  ```bash
  mkdir -p /home/govan/tmp/wt && ln -sfn /home/govan/tmp/projects /home/govan/tmp/wt/projects; git -C /home/govan/tmp/unheaded worktree add -b sprint/s-g9-fold /home/govan/tmp/wt/S-G9-FOLD develop
  ```
  Expected last line: `HEAD is now at`. Branch exists: [STUCK]. The `projects` link is what lets go.mod's relative doomgeneric replace resolve from a worktree.
- [ ] **Step 2** [B] ~1m: Extract the payloads.
  ```bash
  python3 - <<'PY'
  import re, os
  plan = open('/home/govan/tmp/unheaded/docs/battle-plans/S-G9-FOLD-battle-plan.md').read()
  d = '/var/tmp/S-G9-FOLD-payloads'; os.makedirs(d, exist_ok=True)
  for name, body in re.findall(r'<!-- PAYLOAD ([\w.\-]+) -->\n````text\n(.*?)\n````\n', plan, re.S):
      open(f'{d}/{name}', 'w').write(body + '\n')
  print(len(os.listdir(d)))
  PY
  chmod +x /var/tmp/S-G9-FOLD-payloads/commit.sh
  ```
  Expected: `20`. Any other count: [STUCK].
- [ ] **Step 3** [W] ~1m: 
  ```bash
  cat > docs/battle-plans/progress/S-G9-FOLD-progress.md <<'MD'
  <!-- SPDX-License-Identifier: GPL-3.0-or-later -->
  # S-G9-FOLD progress

  ## Log

  ## For Stevie
  MD
  ```
- [ ] **Step 4** [B] ~5m: `GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go test -race ./services/... ./cmd/compliance-evidence/ ./cmd/trace-collector-go/ ./pkg/protocol/... ./pkg/ebpf/ ./pkg/cmdline/ 2>&1 | grep -E '^FAIL[[:space:]]+unheaded/' | awk '{print $2}' | sort -u > /var/tmp/S-G9-FOLD-payloads/go-baseline.txt; wc -l < /var/tmp/S-G9-FOLD-payloads/go-baseline.txt; systemctl list-units 'unheaded*' --no-pager --state=running --no-legend | wc -l; ss -ltn | grep -c ':19015 '`
- [ ] **Step 5** [V] ~1m: Record the three numbers in the progress file as "baseline failures", "running units", "19015 listeners". The last must be `0`. `go build` failing on `cmd/wotan-ctl` means the `projects` link is missing.
- [ ] **Step 6** [V] ~1m: Numbers free: `ls docs/adr | grep -E '^ADR-(127|128|129|130|131|132|133|134)-'`. Expected no output. A taken number: mark that ADR's Phase 1 step [STUCK] and record it under "For Stevie" (S-STAGE-2B starts at 135 and S-CLEAN-CUT uses 121, so 127-134 should be free). If ADR-127 is the one taken, Phases 2-5 are [BLOCKED].
- [ ] **Step 7** [C] ~1m: **COMMIT** `/var/tmp/S-G9-FOLD-payloads/commit.sh S-G9-FOLD "Steps 1-7: preflight" docs/battle-plans/progress/S-G9-FOLD-progress.md`

**PHASE 0 EXIT GATE**: commit exists; 19015 listeners `0`.

## PHASE 1: ONE PARKED ADR PER FOLD (Steps 8-19)

**Goal**: Stevie's condition: an ADR before any code. Eight files; text final in the payloads.
**Prerequisite**: Phase 0. **Time**: 10m · **Agent**: Solo · **Model**: Haiku

- [ ] **Step 8** [W] ~1m: `cp $P/ADR-127-fold-compliance.md docs/adr/`
- [ ] **Step 9** [W] ~1m: `cp $P/ADR-128-fold-firewall.md docs/adr/`
- [ ] **Step 10** [W] ~1m: `cp $P/ADR-129-fold-qos.md docs/adr/`
- [ ] **Step 11** [W] ~1m: `cp $P/ADR-130-fold-nfv.md docs/adr/`
- [ ] **Step 12** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 8-12: ADR-127 to ADR-130" docs/adr/ADR-127-fold-compliance.md docs/adr/ADR-128-fold-firewall.md docs/adr/ADR-129-fold-qos.md docs/adr/ADR-130-fold-nfv.md`.
- [ ] **Step 13** [W] ~1m: `cp $P/ADR-131-fold-failover.md docs/adr/`
- [ ] **Step 14** [W] ~1m: `cp $P/ADR-132-fold-loadbalancer.md docs/adr/`
- [ ] **Step 15** [W] ~1m: `cp $P/ADR-133-fold-version.md docs/adr/`
- [ ] **Step 16** [W] ~1m: `cp $P/ADR-134-fold-keymgmt.md docs/adr/`
- [ ] **Step 17** [W] ~1m: `python3 $P/index_rows_g9.py`. Expected output starts `index rows added after:`. `ANCHOR MISSING`: [STUCK].
- [ ] **Step 18** [V] ~1m: `ls docs/adr/ADR-12[789]-fold-* docs/adr/ADR-13[0-4]-fold-* | wc -l; grep -c 'ADR-1[23][0-9]-fold-' docs/adr/ADR-INDEX.md; grep -L '^\*\*Status:\*\* Proposed (parked' docs/adr/ADR-12[789]-fold-* docs/adr/ADR-13[0-4]-fold-* | wc -l`. Expected `8`, `8`, `0`.
- [ ] **Step 19** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 13-19: ADR-131 to ADR-134 and the index: one parked ADR per fold" docs/adr/ADR-131-fold-failover.md docs/adr/ADR-132-fold-loadbalancer.md docs/adr/ADR-133-fold-version.md docs/adr/ADR-134-fold-keymgmt.md docs/adr/ADR-INDEX.md`

**PHASE 1 EXIT GATE**: eight ADRs committed **before** any code commit: `git log --reverse --format=%s develop..HEAD | head -3` shows preflight, then the two ADR commits. Stevie's condition is met for compliance. If not, Phases 2-5 do not start.

## PHASE 2: AN EMPTY PLANE PROVES NOTHING (Steps 20-25)

**Goal**: `no_data` replaces the false 100 %; the score survives the lists trimming; the service gains `Handler()` and `StartBackground()` so it can sit behind auth.
**Prerequisite**: Phase 1. **Time**: 20m · **Agent**: Agent · **Model**: Sonnet

Signatures: `func (s *WireComplianceService) Handler() *http.ServeMux`; `func (s *WireComplianceService) StartBackground(ctx context.Context) error`; `func (s *WireComplianceService) appendAudit(e AuditEntry)` (caller holds `s.mu`); new field `ComplianceScore.NoData bool` with JSON `no_data`; new `Config.ListenAddr string`, default `127.0.0.1:19015`. The score reads `s.score.Blocked` and `s.auditTotal`, never a list length. `TestScoreSurvivesTrimming`: 700 allowed, 300 blocked, lists capped at 100: 70.0 %, completeness 0.3, 100 kept.

- [ ] **Step 20** [W] ~1m: Tests first: `python3 $P/compliance_test_edit.py`. Expected: `compliance_test_edit.py: applied`.
- [ ] **Step 21** [V] ~2m: **RED**: `GOMAXPROCS=4 go test ./services/compliance/ 2>&1 | tail -4`. Expected: `svc.GetScore().NoData undefined` (or `score.NoData undefined`), then `[build failed]`. Anything else: [STUCK].
- [ ] **Step 22** [W] ~1m: `python3 $P/compliance_code.py`. Expected: `compliance_code.py: all edits applied`.
- [ ] **Step 23** [V] ~3m: **GREEN**: `gofmt -l services/compliance; GOMAXPROCS=4 go vet ./services/compliance/ && GOMAXPROCS=4 go test -race ./services/compliance/ 2>&1 | tail -1`. Expected only `ok  	unheaded/services/compliance`.
- [ ] **Step 24** [V] ~1m: `grep -c 'ports.DefaultAddr' services/compliance/wire_compliance.go`. Expected `0`: no more `:19015` on every interface.
- [ ] **Step 25** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 20-25: wire compliance reports no_data, not 100 percent; Handler and StartBackground" services/compliance/wire_compliance.go services/compliance/wire_compliance_test.go`

**PHASE 2 EXIT GATE**: green, `0`.

## PHASE 3: THE WIRE SUBCOMMAND (Steps 26-32)

**Goal**: `compliance-evidence wire`, off unless `--serve`, behind `pkg/auth` and `pkg/tls`, loopback unless authenticated.
**Prerequisite**: Phase 2. **Time**: 20m · **Agent**: Agent · **Model**: Sonnet

Signatures, in `cmd/compliance-evidence/wire.go`: `func runWire(args []string, stdout, stderr io.Writer) int` (0; 1 failed serve; 2 usage error; `--help` on stdout with 0, ADR-111); `func checkWireListen(addr string, authEnabled bool) error`; `func serveWire(ctx context.Context, o wireOptions, ready chan<- string) error`.

- [ ] **Step 26** [W] ~1m: Tests first: `cp $P/wire_test.go cmd/compliance-evidence/wire_test.go`
- [ ] **Step 27** [V] ~2m: **RED**: `GOMAXPROCS=4 go test ./cmd/compliance-evidence/ -run Wire 2>&1 | tail -5`. Expected: `undefined:` lines naming `checkWireListen`, `serveWire`, `wireOffMessage` or `wireOptions`, then `[build failed]`.
- [ ] **Step 28** [W] ~1m: `cp $P/wire.go $P/wire_audit.go cmd/compliance-evidence/ && python3 $P/wire_main.py`. Expected: `wire_main.py: all edits applied`.
- [ ] **Step 29** [V] ~4m: **GREEN**: `gofmt -l cmd/compliance-evidence; GOMAXPROCS=4 go vet ./cmd/compliance-evidence/ && GOMAXPROCS=4 go test -race ./cmd/compliance-evidence/ ./pkg/cmdline/ 2>&1 | tail -2`. Expected two `ok` lines.
- [ ] **Step 30** [V] ~2m: Behaviour: 
  ```bash
  go build -o $P/ce ./cmd/compliance-evidence; $P/ce wire; echo "rc=$?"; $P/ce wire --help >/dev/null; echo "help rc=$?"; $P/ce wire now 2>/dev/null; echo "operand rc=$?"; $P/ce wire --serve --listen 0.0.0.0:19015 2>&1 | head -1; $P/ce --help | grep -c 'off unless --serve'; rm -f $P/ce
  ```
  Expected: `wire compliance is off; pass --serve to run it (ADR-127)`, `rc=0`, `help rc=0`, `operand rc=2`, a line containing `is not loopback: set AUTH_ENABLED=true first`, `1`.
- [ ] **Step 31** [B] ~3m: The man page lists subcommands, so it moves: `scripts/gen-man.sh --write 2>&1 | tail -1; git status --short man | wc -l`. Expected a line `OK: ... 1 written ...`, then `1`. Any other count: [STUCK].
- [ ] **Step 32** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 26-32: compliance-evidence wire, off unless --serve" cmd/compliance-evidence/wire.go cmd/compliance-evidence/wire_audit.go cmd/compliance-evidence/wire_test.go cmd/compliance-evidence/main.go man/compliance-evidence.1`

**PHASE 3 EXIT GATE**: green; default message exact.

## PHASE 4: THE AUDIT RECORD (Steps 33-37)

**Goal**: The 64-byte record mirrored in bpfschema, decoded, refused when hostile.
**Prerequisite**: Phase 3. **Time**: 15m · **Agent**: Agent · **Model**: Sonnet

Signature: `func (s *WireComplianceService) IngestAuditRecord(b []byte) error`. Decision 1 counts allowed and adds an audit entry; 2 records a violation; anything else, or a size other than 64, is an error and counts nothing. The trace id is the flow label: the little-endian u32 at offset 16, in hex.

- [ ] **Step 33** [W] ~1m: Tests first: `cp $P/audit_record_test.go services/compliance/audit_record_test.go`
- [ ] **Step 34** [V] ~2m: **RED**: `GOMAXPROCS=4 go test ./services/compliance/ 2>&1 | tail -4`. Expected: `undefined: auditDecisionAllow` or `svc.IngestAuditRecord undefined`, then `[build failed]`.
- [ ] **Step 35** [W] ~1m: `python3 $P/audit_schema.py && cp $P/audit_record.go services/compliance/audit_record.go`. Expected: `audit_schema.py: all edits applied`.
- [ ] **Step 36** [V] ~3m: **GREEN**: `gofmt -l services/compliance pkg/protocol/bpfschema; GOMAXPROCS=4 go test -race ./services/compliance/ ./pkg/protocol/bpfschema/ 2>&1 | tail -2`. Expected two `ok`. `TestRustSourceStructParity/ComplianceAuditRecord` proves the Go matches the Rust.
- [ ] **Step 37** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 33-37: AuditRecord mirrored and ingested" services/compliance/audit_record.go services/compliance/audit_record_test.go pkg/protocol/bpfschema/core_maps.go pkg/protocol/bpfschema/rust_source_test.go`

**PHASE 4 EXIT GATE**: two `ok`.

## PHASE 5: THE RING CONSUMER (Steps 38-48)

**Goal**: `--audit-ring` reads a pinned `AUDIT_LOG`; proven end to end: a packet through `compliance_xdp` moves the score.
**Prerequisite**: Phase 4. **Time**: 90m · **Agent**: Coordinator · **Model**: Opus (concurrency, BPF, root)

`OpenPinnedAnamnesisReader` is not touched: its only caller is `cmd/trace-collector-go/main.go:974`, a supervised service, and that path has no test of its own.

- [ ] **Step 38** [V] ~1m: S-WIRE-HONEST is in: `git -C /home/govan/tmp/unheaded log develop --grep 'S-WIRE-HONEST' --oneline | wc -l`. Above `0` to go on. `0`: Phase 5 is [BLOCKED]; `--audit-ring` keeps its "not built yet" error; go to Phase 6.
- [ ] **Step 39** [B] ~3m: Bring develop in: `git merge --no-ff -S --no-edit develop`.
  - gpg failure: `git merge --abort; git merge --no-ff --no-gpg-sign --no-edit develop`, and record the unsigned merge in the progress file for the handoff.
  - Conflict: keep both sides' rows in `docs/adr/ADR-INDEX.md`, `pkg/protocol/bpfschema/core_maps.go` and `structParity` in `pkg/protocol/bpfschema/rust_source_test.go`, then `git commit --no-edit`. A conflict anywhere else: `git merge --abort`, [STUCK], Phase 5 [BLOCKED].
- [ ] **Step 40** [R] ~10m: Read `pkg/ebpf/anamnesis_reader_linux.go` and `pkg/ebpf/ringbuf_consume.go`.
- [ ] **Step 41** [W] ~25m: New file `pkg/ebpf/ring_consume_linux.go`: `func ConsumePinnedRing(ctx context.Context, path string, emit func([]byte)) error`, built on `bpfObjGet`, `getRingbufSize` and `consumeRingbuf`, with its own mmap and poll loop. Blocks until ctx ends, then unmaps and closes. Refuses a pin whose map type is not ringbuf. `pkg/ebpf/ring_consume_stub.go` (`//go:build !linux`) returns `ErrNotSupported`.
- [ ] **Step 42** [V] ~4m: `gofmt -l pkg/ebpf; go vet ./pkg/ebpf/ && GOOS=darwin go vet ./pkg/ebpf/ && GOMAXPROCS=4 go test -race ./pkg/ebpf/ ./cmd/trace-collector-go/ ./cmd/dashboard-backend/internal/ebpf/ 2>&1 | tail -3; git diff --name-only develop...HEAD -- pkg/ebpf/anamnesis_reader_linux.go | wc -l`. Three `ok`, then `0`.
- [ ] **Step 43** [W] ~10m: `cmd/compliance-evidence/wire_audit.go`: real `consumeAuditRing(ctx, path, svc)`: `ebpf.ConsumePinnedRing`, each record into `svc.IngestAuditRecord`; a refused record is counted and logged, never fatal. This is the only reader of `AUDIT_LOG`.
- [ ] **Step 44** [W] ~30m: Root test `cmd/compliance-evidence/wire_ring_linux_test.go`: load `ebpf/target/load-gate/default/bpfel-unknown-none/release/compliance-ebpf` with cilium/ebpf (test code only), write one `SERVICE_POLICIES` block entry, pin `AUDIT_LOG` at exactly `/sys/fs/bpf/g9-ring-test/AUDIT_LOG`, run `compliance_xdp` with `pkg/monadpkt.Build` frames through `Run`, once allowed and once blocked, consume, assert `Allowed` 1, `Blocked` 1, `NoData` false. `t.Cleanup` removes the pin and the directory. Skip unprivileged.
- [ ] **Step 45** [B] ~6m: Build as the normal user.
  ```bash
  gofmt -l cmd/compliance-evidence; go vet ./cmd/compliance-evidence/ && (cd ebpf && CARGO_BUILD_JOBS=4 CARGO_TARGET_DIR=target/load-gate/default cargo build --release -p compliance-ebpf 2>&1 | tail -1); go test -c -o $P/g9.test ./cmd/compliance-evidence
  ```
  No gofmt line, `Finished`.
- [ ] **Step 46** [V][S] ~3m: Only this step is root.
  ```bash
  (cd cmd/compliance-evidence && sudo $P/g9.test -test.run Ring -test.v 2>&1 | grep -E '^(--- |PASS|FAIL)'); sudo ls /sys/fs/bpf | grep -c g9-ring-test
  ```
  `--- PASS`, `PASS`, `0`. **This is the proof evidence can fail**: a blocked packet lowers the score.
  - [D] Record never arrives: the map key is `src | (dst << 16)` with u8 service ids; check the frame sets `src_service_id` and `dst_service_id`. Two attempts, then [STUCK].
- [ ] **Step 47** [B] ~1m: `rm -f $P/g9.test`
- [ ] **Step 48** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 38-48: --audit-ring consumes AUDIT_LOG; a blocked packet moves the score" pkg/ebpf/ring_consume_linux.go pkg/ebpf/ring_consume_stub.go cmd/compliance-evidence/wire_audit.go cmd/compliance-evidence/wire_ring_linux_test.go`
  - On the [STUCK] path: `git checkout -- cmd/compliance-evidence/wire_audit.go && rm -f cmd/compliance-evidence/wire_ring_linux_test.go`, then commit only `pkg/ebpf/ring_consume_linux.go pkg/ebpf/ring_consume_stub.go`. The stub keeps refusing `--audit-ring`.

**PHASE 5 EXIT GATE**: root test PASS, or [STUCK] or [BLOCKED] logged with the stub intact.

## PHASE 6: DEFAULT OFF, NOTHING RUNNING MOVES (Steps 49-60)

**Goal**: Stevie's check, by measurement.
**Prerequisite**: Phases 2-5. **Time**: 20m · **Agent**: Agent · **Model**: Sonnet

- [ ] **Step 49** [V] ~1m: **No caller turns it on**: `git grep -nE 'compliance-evidence.* wire|wire --serve' -- deploy docker docker-compose.yml nix scripts runbooks Makefile ':!scripts/check-*' | wc -l`. Expected `0`.
- [ ] **Step 50** [V] ~1m: **Unit and compose untouched**: `git diff --name-only develop...HEAD -- deploy docker docker-compose.yml nix | wc -l`. Expected `0`.
- [ ] **Step 51** [V] ~1m: **No other destination touched**: `git diff --name-only develop...HEAD -- cmd/unheaded-daemon cmd/monad cmd/unheaded-auth cmd/unheaded-baseline services/firewall services/qos services/nfv services/selfheal services/version services/keymgmt services/loadbalancer ebpf/ pkg/ports | wc -l`. Expected `0`.
- [ ] **Step 52** [V] ~1m: **Evidence model untouched**: `git diff --name-only develop...HEAD -- compliance/ pkg/compliance | wc -l`. Expected `0`.
- [ ] **Step 53** [W] ~1m: Log the four results above in the progress file under "Default off".
- [ ] **Step 54** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 49-54: nothing outside the fold moved" docs/battle-plans/progress/S-G9-FOLD-progress.md`
- [ ] **Step 55** [V] ~4m: **Default path identical.** The user timer runs `go run ./cmd/compliance-evidence` from `~/tmp/unheaded`.
  ```bash
  git -C /home/govan/tmp/unheaded rev-parse --abbrev-ref HEAD; (cd /home/govan/tmp/unheaded && go run ./cmd/compliance-evidence --help) > $P/help-old.txt; go run ./cmd/compliance-evidence --help > $P/help-new.txt; diff $P/help-old.txt $P/help-new.txt | grep -c '^[<>]'; diff $P/help-old.txt $P/help-new.txt | grep '^>' 
  ```
  Expected `develop`, `1`, and the one `wire` line. The `main()` dispatch runs `wire` only on `os.Args[1] == "wire"`.
- [ ] **Step 56** [V] ~1m: **Off means off**: `go run ./cmd/compliance-evidence wire; ss -ltn | grep -c ':19015 '`. Expected the off message, `0`.
- [ ] **Step 57** [V] ~1m: **Running units**: `systemctl list-units 'unheaded*' --no-pager --state=running --no-legend | wc -l` equals "running units" in the progress file.
- [ ] **Step 58** [B] ~5m: Gates: `for g in check-http-timeouts check-gosec-ratchet check-dependency-baseline check-sbom-trend gen-man; do ./scripts/$g.sh >/dev/null 2>&1 && echo "PASS $g" || echo "FAIL $g"; done`. Five `PASS` lines. Exit code is the verdict.
  - [D] `check-gosec-ratchet` flags `wire.go`: read the line. A real finding is Opus's; do not add a `#nosec` by rote. [STUCK].
- [ ] **Step 59** [W] ~1m: Add the remaining results to "Default off" in the progress file.
- [ ] **Step 60** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 55-60: default off proven; no running service moves" docs/battle-plans/progress/S-G9-FOLD-progress.md`

**PHASE 6 EXIT GATE**: every expected value above. A miss blocks the merge.

## PHASE 7: DOCS (Steps 61-64)

**Goal**: The wiki and the unreached list tell the truth.
**Prerequisite**: Phase 6. Docs only. **Time**: 10m · **Agent**: Agent · **Model**: Haiku

- [ ] **Step 61** [W] ~1m: `python3 $P/g9_docs.py`. Expected: `g9_docs.py: all edits applied`.
- [ ] **Step 62** [B] ~1m: `scripts/live-path-inventory.sh --report > docs/LIVE-PATHS.md && git diff --stat docs/LIVE-PATHS.md | tail -1`. Any result is fine; a script error is [STUCK].
- [ ] **Step 63** [V] ~2m: Reached by a main now:
  ```bash
  mains=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...); go list -deps $mains | grep -c '^unheaded/services/compliance$'; grep -c '`services/compliance`' docs/UNREACHED-GO-PACKAGES.md
  ```
  Expected `1`, then `1` (the new DP-203 paragraph; the list line is gone).
- [ ] **Step 64** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 61-64: docs: wire plane, unreached list, live paths" docs/UNREACHED-GO-PACKAGES.md wiki/Compliance-Paperwork.md docs/LIVE-PATHS.md`

**PHASE 7 EXIT GATE**: values above.

## PHASE 8: GATED FOLDS, DO NOT EXECUTE (Steps 65-71)

**Goal**: Name what opens each fold. No agent runs these tonight.
**Prerequisite**: Stevie has read the ADR and said go. An agent that reaches this phase stops. **Agent**: none · **Model**: none until opened; each becomes its own plan under ADR-112's rule.

Order when opened: ADR-128's node writer first: it gates four of them, five if NFV stays in Cuirass.

- [ ] **Step 65** [BLOCKED]: firewall: Stevie reads ADR-128 and answers the node writer. Then `services/firewall` gets `Handler()`; `CUIRASS_FEATURE_FIREWALL`, default off.
- [ ] **Step 66** [BLOCKED]: loadbalancer: ADR-132 and the node writer. Table in Cuirass, write in the node writer, 19110 retired.
- [ ] **Step 67** [BLOCKED]: qos: ADR-129. Policy routes only.
- [ ] **Step 68** [BLOCKED]: failover: ADR-131. Consume `system.health.remediate`; no scoring loop.
- [ ] **Step 69** [BLOCKED]: nfv: ADR-130, and his choice among its three options; RFC desk on `circuit_state` first.
- [ ] **Step 70** [BLOCKED]: keymgmt: ADR-134. Registry only; `GenerateKey` renamed.
- [ ] **Step 71** [BLOCKED]: version: ADR-133. Spec compliance gate before any rule is accepted.

**PHASE 8 EXIT GATE**: none tonight. `git log develop...HEAD -- services/firewall services/qos services/nfv services/selfheal services/version services/keymgmt services/loadbalancer | wc -l` is `0`.

## PHASE 9: SPRINT EXIT (Steps 72-78)

**Goal**: One merge, one handoff, and the evidence timer proven on the merged code.
**Prerequisite**: Phases 0-7. **Time**: 30m · **Agent**: Coordinator · **Model**: Opus

- [ ] **Step 72** [B] ~3m: Rule 15: bring develop in first: `git merge --no-ff -S --no-edit develop`. gpg failure: `git merge --abort; git merge --no-ff --no-gpg-sign --no-edit develop`, recorded in the progress file. A conflict is [STUCK]: `git merge --abort`, no merge to develop.
- [ ] **Step 73** [B] ~10m: Closing gate on that tree: `GOMAXPROCS=4 go build ./... && GOMAXPROCS=4 go test -race ./services/... ./cmd/compliance-evidence/ ./cmd/trace-collector-go/ ./pkg/protocol/... ./pkg/ebpf/ ./pkg/cmdline/ 2>&1 | grep -E '^FAIL[[:space:]]+unheaded/' | awk '{print $2}' | sort -u > $P/go-post.txt; comm -13 $P/go-baseline.txt $P/go-post.txt | wc -l`. Expected `0`: no package failing that was not failing before.
- [ ] **Step 74** [R] ~10m: Load `unheaded-moatghost`: does anything claim more than the plane does? `no_data`, the off default, the evidence file untouched. Findings to the progress file.
- [ ] **Step 75** [W] ~3m: Close the progress file: phases, For Stevie, unsigned commits if any.
- [ ] **Step 76** [C] ~1m: **COMMIT** `$P/commit.sh S-G9-FOLD "Steps 65-76: exit gates green, handoff" docs/battle-plans/progress/S-G9-FOLD-progress.md`
- [ ] **Step 77** [B] ~2m: The coordinator runs this on a clean develop; never push.
  ```bash
  M='Merge sprint/s-g9-fold (S-G9-FOLD: eight ADRs, compliance fold)'; git -C /home/govan/tmp/unheaded merge --no-ff -S sprint/s-g9-fold -m "$M" || { git -C /home/govan/tmp/unheaded merge --abort; git -C /home/govan/tmp/unheaded merge --no-ff --no-gpg-sign sprint/s-g9-fold -m "$M"; echo UNSIGNED; }
  ```
  `UNSIGNED` goes in the progress file and the handoff.
- [ ] **Step 78** [V] ~5m: The evidence timer compiles develop with `go run`, and the merge just grew that binary's dependency set **from 237 to 414 packages**. Prove it still works.
  ```bash
  T=$(date +%s); systemctl --user start unheaded-compliance-evidence.service; journalctl --user -u unheaded-compliance-evidence --since "@$T" --no-pager | grep -c 'Finished unheaded-compliance-evidence'
  ```
  Expected `1`. The unit is a user oneshot with `TimeoutStartSec=15min`, about 20 s before the merge, longer on the first compile. `0`: Appendix C, "The evidence timer fails".

**PHASE 9 EXIT GATE**: develop holds the merge; the timer finished; the coordinator folds the progress file into `~/tmp/next.md`.

---

## Appendix A: Swarm table

| Wave | Agents | Phases | Model | Files | Builder? |
|---|---|---|---|---|---|
| 0 | 1 | 0 | Sonnet | progress file | yes |
| 1 | 1 | 1 | Haiku | eight ADRs, `ADR-INDEX.md` | no |
| 2 | 1 | 2 | Sonnet | `services/compliance/wire_compliance*.go` | yes |
| 3 | 1 | 3 | Sonnet | `cmd/compliance-evidence/{wire*.go,main.go}` | yes |
| 4 | 1 | 4 | Sonnet | `services/compliance/audit_record*.go`, `pkg/protocol/bpfschema` | yes |
| 5 | 1 | 5 | Opus | `pkg/ebpf/ring_consume*.go`, `cmd/compliance-evidence/wire_audit.go`, `wire_ring_linux_test.go` | yes |
| 6 | 1 | 6 | Sonnet | progress file | yes, checks only |
| 7 | 1 | 7 | Haiku | docs | no |
| 8 | none | 8 | none | none | no |
| 9 | 1 | 9 | Opus | progress file | yes |

Phases 2, 3, 4 are serial on purpose: 3 needs 2's `Handler`, and each is 15-20 minutes. Phase 1 may run while S-WIRE-HONEST is still building: ADRs are not builders. Phase 7's two doc edits can run beside Phase 6.

## Appendix B: Agent assignment and critical path

| Phase | Model | Depends on | Time |
|---|---|---|---|
| 0 Preflight | Sonnet | none | 15m |
| 1 ADRs | Haiku | 0 | 10m |
| 2 Honest score | Sonnet | 1 | 20m |
| 3 Wire subcommand | Sonnet | 2 | 20m |
| 4 Audit record | Sonnet | 3 | 15m |
| 5 Ring consumer | Opus | 4, S-WIRE-HONEST | 90m |
| 6 Default off | Sonnet | 5 | 20m |
| 7 Docs | Haiku | 6 | 10m |
| 8 Gated | none | Stevie | n/a |
| 9 Exit | Opus | 7 | 30m |

Critical path: 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 9, about 3.5 hours.

Phase 1 is one agent: eight file copies do not repay a swarm, and they share `ADR-INDEX.md`.

## Appendix C: Emergency procedures

- **`go build ./...` fails on doomgeneric**: `~/tmp/wt/projects` is missing; Phase 0's first step makes it.
- **The evidence timer fails after the merge**: it compiles `cmd/compliance-evidence` from `~/tmp/unheaded` at each run. `journalctl --user -u unheaded-compliance-evidence -n 30`. A compile error on develop: `git revert -m 1` the merge locally and record it; never `--amend`.
- **Port 19015 is found open**: nothing should bind it. `ss -ltnp | grep ':19015 '` names the pid; `kill <pid>`. Never `pkill -f`. A test left it: fix the test.
- **Pin left behind** (Opus only, a named [S] step; a cheaper agent marks [STUCK]): `sudo rm -f /sys/fs/bpf/g9-ring-test/AUDIT_LOG && sudo rmdir /sys/fs/bpf/g9-ring-test`. No glob.
- **gpg-agent timed out**: `commit.sh` falls back to `--no-gpg-sign` and says so. Note it in the handoff.

## Appendix D: Quick reference

- `compliance-evidence wire` flags: `--serve` (default off), `--listen` (default `127.0.0.1:19015`), `--wotan`, `--audit-ring`.
- Audit record, little-endian: 0 timestamp u64; 8 src u16; 10 dst u16; 12 decision (1 allow, 2 block); 13 violation (1 policy, 2 encryption, 3 PII egress, 4 zone); 14 classification; 15 qos; 16 trace u32; 20 zone; 21 flags; 22 latency; 24 Monad (20); 44 reserved (20).
- Feature flags the ADRs propose, none built: `CUIRASS_FEATURE_{FIREWALL,QOS,NFV,FAILOVER,LOADBALANCER}`, `MONAD_FEATURE_VERSION`, `--feature-keyreg`.
- Plane ports today, all unused: firewall 19010, qos 19013, selfheal 19014, compliance 19015, nfv 19016, version 19017, keymgmt 19007, xdpsync 19110.

## Appendix E: Payloads

Extracted by Phase 0 to `/var/tmp/S-G9-FOLD-payloads/`. Phases 2-4 were rehearsed on a scratch snapshot of develop. Do not retype them.

<!-- PAYLOAD commit.sh -->
````text
#!/usr/bin/env bash
# usage: commit.sh PLAN "Steps X-Y: subject" [path ...]   (run from the worktree root)
# Stages only the named paths; `git rm` deletions are already staged.
set -euo pipefail
plan=$1; subject=$2; shift 2
if [ $# -gt 0 ]; then git add -- "$@"; fi
if git diff --cached --quiet; then echo "commit.sh: nothing staged"; exit 3; fi
msg="[PLAN ${plan}] ${subject}

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01TzL3Tqe5ebLDopsg5ZkZdp"
if ! git commit -q -S -m "$msg"; then
  git commit -q --no-gpg-sign -m "$msg"
  echo "commit.sh: gpg unavailable; committed UNSIGNED (record it in the handoff)"
fi
git log -1 --format='%h %G? %s'
````

<!-- PAYLOAD ADR-127-fold-compliance.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-127: Fold the wire compliance plane into compliance-evidence

**Status:** Proposed (parked; DP-203's condition: an ADR before any code). The code lands behind `--serve` while this stays Proposed, because Stevie asked for the compliance fold first. Stevie accepts it.
**Date:** 2026-11-04
**Related:** DP-203, ADR-097 (evidence must be able to fail), ADR-098, ADR-103, TRUE-WIRE C1 and E1, `docs/protocol/RFC-EDITOR-REQUESTS-WH.md` item 1.

## The pitch (decision pack, DP-203)

> Fold the eight G9 control planes into existing binaries as flag-gated features (compliance into unheaded-baseline/compliance-evidence ...), one parked ADR per fold before any code; compliance is critical; nothing deleted.

## What is there

`services/compliance`: a long-running HTTP service on 19015 with no auth, Wotan subscriptions, a 10-second loop, policies, zones, violations, an audit list and a score. It touches no BPF map. Nothing calls `RecordAllowed` or `RecordViolation` but its tests, so the score has no input, and an empty service reports **100 % and audit completeness 1.0**.

`ebpf/compliance-ebpf`: `compliance_xdp`, 29,222 verified instructions, loads in the gate, attached by nothing. It reads policy from `SERVICE_POLICIES` and writes 64-byte records to `AUDIT_LOG`. Nothing writes the first or reads the second.

## Which binary

The pack offered two. **compliance-evidence, not unheaded-baseline.**

- unheaded-baseline runs `Type=oneshot` on a 5-minute timer (`TimeoutStartSec=3min`, its own `--timeout` 2 minutes), on every host, with `CAP_DAC_READ_SEARCH`: it can read `/etc/shadow`. A listener in that process is a bad trade, and a daemon does not fit a oneshot. Its unit refuses mount namespacing on purpose, so nothing would contain the listener.
- compliance-evidence already dispatches subcommands, owns the evidence model, and its default path runs as a user timer. A `wire` subcommand leaves that path alone.

Neither is a daemon today. `wire --serve` runs in the foreground; a unit is a later decision, with its own uid.

## Decision

- `compliance-evidence wire`: without `--serve` it prints that it is off and exits 0. No unit, timer or compose service passes `--serve`.
- `--listen` defaults to `127.0.0.1:19015` and refuses anything but loopback unless `AUTH_ENABLED=true`: the plane takes policy writes.
- The service runs behind `pkg/auth` and `pkg/tls`, never its own open listener: new `Handler()` and `StartBackground()`.
- **Honest score.** No packets means `no_data: true`, `score_pct: 0`, `audit_completeness: 0`, never 100.
- `--audit-ring PATH` consumes a pinned `AUDIT_LOG`; `IngestAuditRecord` is the first real input. Record layout mirrored in `bpfschema`.
- The evidence catalog does not change. A new evidence source is a separate decision: it would move every framework's figures.

## Not done here

- Writing `SERVICE_POLICIES` (string service ids against a u8 pair; needs a mapping), attaching `compliance_xdp` anywhere, a unit, a uid, Wotan topics on `auto_approve`.
- **East.** compliance-evidence is a `go run` user timer with no deb, so nothing runs it on east.
- **The operator's account.** That timer runs as an account with `sudo -n`. `--serve` there is not much better than a listener in baseline, so the `/etc/shadow` argument only fully holds once `wire` has its own nologin account and unit. Do not run `--serve` from that account beyond a lab test.
- **Who may open the `AUDIT_LOG` pin.** Root only today. `pkg/cartridge/loader_linux.go`'s `EventsGID` is the precedent: the one ring pinned `root:<gid> 0660` for a reader with no right to the other maps.

## Open questions

1. compliance-ebpf reads classification from the low bits of scratch[0]. RFC item 1 may move it. Does classification belong on the wire at all?
2. Should wire results ever enter the evidence file, and under which controls?
3. Who loads `compliance_xdp`, on which interface? Same question as ADR-128's node writer.
````

<!-- PAYLOAD ADR-128-fold-firewall.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-128: Fold the firewall plane into Cuirass, as intent only

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-127, ADR-129 to ADR-132, TRUE-WIRE C2 and E2.

## The pitch (decision pack, DP-203)

> Firewall ... into cuirass.

## What is there

`services/firewall`: HTTP on 19010, no auth; rules, `/api/v1/connections`, `/api/v1/stats`, `/api/v1/policy`, a conntrack aggregator publishing `firewall.conntrack`. No BPF map is touched: the aggregator summarises an in-memory table nothing fills. `ebpf/firewall-ebpf`: `firewall_xdp`, 33,795 verified instructions, CONNTRACK 1,000,000 entries, POLICY_CACHE, attached by nothing.

## Verdict: right for intent, wrong for enforcement

Cuirass holds desired state, so policy intent belongs there: it has auth, TLS and `/api/v1/state/desired`. But four facts stop it being the whole answer:

1. **Cuirass cannot write maps.** `deploy/systemd/unheaded-daemon.service` has an empty `CapabilityBoundingSet` and `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`. Adding `CAP_BPF` to the process that holds the LXD client certificate widens the worst blast radius in the Kingdom.
2. **Cuirass runs once; XDP runs per node.** east needs rules too.
3. **Routes collide.** firewall and qos both register `/api/v1/policy` and `/api/v1/stats`; all five planes register `/health`, `/ready`, `/metrics`. Each plane must expose a handler and mount under a prefix, `/api/v1/firewall/...`.
4. **`Start` binds an open listener.** A fold must not call it.

## The node writer question (shared by ADR-128 to ADR-132)

Intent in Cuirass needs a per-node process with `CAP_BPF` to write the maps. Five planes have maps to write: firewall, qos, loadbalancer, failover and nfv. It gates four folds, or five if NFV stays in Cuirass (ADR-130), so decide it once. Candidates:

| Candidate | For | Against |
|---|---|---|
| ADR-125's Go writer (`pkg/hoprules`, `unheaded hop rules apply`) over `pkg/ebpf.PinnedMap`, generalised | No new module; loads nothing, only writes pins; a `Type=oneshot` unit on a timer with `CAP_BPF` alone | A unit and a uid per node; polls desired state rather than reacting; exists only after S-WIRE-HONEST |
| `cmd/ebpf-loader --watch` | It already pins maps, and it stays resident: it blocks on `rx.recv()` to hold its attachments | It is a TOOL: no unit runs it. Rust, with no Wotan or HTTP client: a watch mode needs new crates, a rule 4 confirm for Stevie |
| `cmd/trace-collector-go` | Runs live with `CAP_BPF` | A tracer should not enforce |

**Recommendation:** the Go writer: nothing new enters the tree. It is Stevie's call.

## Decision direction

Flag `CUIRASS_FEATURE_FIREWALL=true`, default off: mounts policy routes under `/api/v1/firewall/`, behind the existing auth. The conntrack aggregator does not run in Cuirass; conntrack is a node fact. When no data is in, the API says so (ADR-127's rule).

## Open questions

1. The node writer, above.
2. Does firewall-ebpf overlap the host nftables work (DP-22)? One default-deny source of truth?
````

<!-- PAYLOAD ADR-129-fold-qos.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-129: Fold the QoS plane into Cuirass, without its 10 Hz loop

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-128 (the node writer), ADR-108 (cartridges), TRUE-WIRE C4 and E4.

## The pitch (decision pack, DP-203)

> ... qos ... into cuirass.

## What is there

`services/qos`: HTTP on 19013, no auth; per-service policies with CoDel parameters, `/api/v1/congestion`, and a loop that publishes `qos.statistics` to Wotan **ten times a second** once it has stats. `UpdateStats` has no caller, so today it finds none and stays silent. No BPF map is touched. `ebpf/qos-ebpf`: `qos_xdp`, 55,684 verified instructions, attached by nothing; it is also the cartridge rate limiter's parity reference (`pkg/cartridge/ratelimit_linux_test.go`).

## Verdict: right, with one part removed

Per-service policy is desired state: Cuirass is the right home. The 10 Hz publisher is not: it reports node queues, which Cuirass cannot see, and fed for real it is ten Wotan messages a second from the control plane. It stays out until a node source exists.

## Decision direction

Flag `CUIRASS_FEATURE_QOS=true`, default off: policy routes under `/api/v1/qos/`. No aggregation loop. Map writes wait on ADR-128's node writer. `qos-ebpf` must keep its parity role untouched.

## Open questions

1. With cartridges able to rate-limit per flow (ADR-108), is `qos-ebpf` the enforcement path or only the reference?
````

<!-- PAYLOAD ADR-130-fold-nfv.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-130: Fold the NFV plane: where, is a question for Stevie

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-108 (MBC cartridges), ADR-128, `docs/protocol/RFC-EDITOR-REQUESTS-WH.md`, TRUE-WIRE C3 and E3.

## The pitch (decision pack, DP-203)

> ... nfv ... into cuirass.

## What is there

`services/nfv`: HTTP on 19016, no auth; up to 64 chains of 10 functions. `POST /api/v1/functions/reload` takes **`prog_fd` as a JSON integer**, stores it, sleeps 200 ms and answers "reloaded". A file descriptor number means nothing across processes, and no program is swapped. `ebpf/nfv-ebpf`: `nfv_xdp` and four tail-called XDP functions (firewall, nat, rate limiter, LB route), attached by nothing.

## Verdict: Cuirass alone does not fit; the alternative is not a clean fit either

1. **The job is loading programs into a tail-call array.** That needs `CAP_BPF`. Cuirass has none and should not get it (ADR-128).
2. **The reload API is unsafe as written.** It must not move anywhere unchanged. An fd from a client is not a capability.
3. **A wire conflict.** `nfv-ebpf` carries `chain_id` in `circuit_state` bits 5-0 and an auth level in bits 7-6. foundation-06 defines `circuit_state` as an exponent-encoded field, and hop-ebpf writes it (`cs::OPEN`). That is a frozen-field overload: it needs the RFC desk before any code.

`cmd/upc-cartridgectl` looks close and is not the same thing. It loads signed MBC programs into one eBPF interpreter (`cartridge-ebpf`); it does not load eBPF. NFV's functions are tail-called XDP. It is ORPHAN, with no unit, and interpreted cartridges miss two of ADR-108's benchmark bars (rate limiter +193 ns against 150, sampler +216 ns against 120). What it has that NFV lacks is a signature check and a registry.

## Options

- **A. Definitions only in Cuirass, as asked.** `CUIRASS_FEATURE_NFV=true`, `/api/v1/nfv/`, `prog_fd` deleted. Chains are data; loading is the node writer's or nobody's.
- **B. Rewrite the functions as cartridges.** One signed path, but a different mechanism and slower.
- **C. A signed loader for XDP objects**, borrowing cartridgectl's Gungnir check. New work.

Nothing moves until Stevie has chosen. In all three, `prog_fd` goes.

## Open questions

1. Is NFV a separate thing from cartridges at all?
2. Where does a chain id live on a frozen wire?
````

<!-- PAYLOAD ADR-131-fold-failover.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-131: Fold failover (services/selfheal) into Cuirass, deciding nothing Akira decides

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-029 (health consensus), ADR-128, TRUE-WIRE C5 and E5.

## The pitch (decision pack, DP-203)

> ... failover (services/selfheal) ... into cuirass.

## What is there

`services/selfheal`: HTTP on 19014, under the constant `ports.BackupMgr`; no auth. Endpoint health scores, a circuit-breaker state machine, backup rosters, manual and automatic failover. **No probe:** `evaluateEndpoint` scores fields nothing sets, so no real endpoint ever fails. Failover only writes events to a slice. `ebpf/failover-ebpf`: `failover_xdp`, 29,125 verified instructions, its own scoring in the kernel, attached by nothing.

## Verdict: right destination, half the code should not come

Cuirass already probes instance health and owns the proxy device that gives a stable endpoint, so it is the right actuator. But **deciding** something is unhealthy is Akira's job under ADR-029: percentage consensus, advisory `system.health.remediate`, three restarts then escalate. Two brains deciding one outage is how failovers fight. hop-ebpf's `CIRCUIT_ERRORS` is a third circuit-breaker view.

## Decision direction

Flag `CUIRASS_FEATURE_FAILOVER=true`, default off. Consume `system.health.remediate`, act through the proxy device; rosters under `/api/v1/failover/`. Do not bring `healthMonitorLoop` or the scoring: Akira and Cuirass's own probes already exist. Rename the port constant only if a listener survives; it should not.

## Open questions

1. Which circuit breaker is the truth: `failover-ebpf`'s, hop-ebpf's `CIRCUIT_ERRORS`, or Akira's severity band?
2. Does kernel-side failover (XDP_TX to a backup) ever run ahead of consensus?
````

<!-- PAYLOAD ADR-132-fold-loadbalancer.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-132: Fold the load balancer plane (xdpsync) into Cuirass

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, ADR-128, TRUE-WIRE C8.

## The pitch (decision pack, DP-203)

> ... loadbalancer into cuirass.

## What is there

`services/loadbalancer`: the one plane with real map code. `xdpsync.go` builds a Maglev table and `SyncToBPF` writes it through `ebpf.LoadPinnedMap` (cilium/ebpf), falling back quietly when the pin is absent. HTTP on 19110, reached by the arithmetic `ports.Firewall + 100`, with no constant of its own; no auth. `ebpf/maglev-ebpf`: `maglev_xdp`, 33,085 verified instructions, attached by nothing.

## Verdict: right, the cleanest of the eight

Backends are exactly what Cuirass knows: it runs the instances and owns the proxy device. The table computation is pure and belongs beside them.

## Decision direction

Flag `CUIRASS_FEATURE_LOADBALANCER=true`, default off: backends and distribution under `/api/v1/lb/`, and the table computed in Cuirass. The map write leaves Cuirass, which has no `CAP_BPF`: the table goes to ADR-128's node writer. If that writer is the Go one, `pkg/ebpf.PinnedMap` (DP-211) replaces cilium/ebpf here; a Rust loader would use aya instead. Either way 19110 stops existing.

## Open questions

1. The node writer, ADR-128.
2. Does Maglev serve anything before there is more than one backend per service?
````

<!-- PAYLOAD ADR-133-fold-version.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-133: Fold version translation into monad, and leave it thin

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, wire freeze v0x01, TRUE-WIRE C6 and E6.

## The pitch (decision pack, DP-203)

> ... version into monad ...

## What is there

`services/version`: HTTP on 19017, no auth; translation rules, per-service versions, rollout events, statistics nothing feeds. `ebpf/version-ebpf`: the heaviest of the G9 halves, `version_xdp_ingress` 243,780 and `version_tc_egress` 213,386 verified instructions, the latter an `#[xdp]` program whose name says TC. Attached by nothing.

## Verdict: right by name, with nothing yet to do

The Monad service is the natural home for Monad version rules. But **the wire is frozen at v0x01**: only one version exists, so `VERSION_MATRIX` can hold no real row, and any rule that moves a field touches the frozen format. The monad unit has an empty `CapabilityBoundingSet`, so it can never write those maps itself; `/api/v1/stats` already exists in `cmd/monad`, so routes need a prefix.

## Decision direction

Flag `MONAD_FEATURE_VERSION=true`, default off: a rules store under `/api/v1/version/`, no map writes, every rule refused until a second version exists. Spec compliance gate and RFC desk before a rule can be accepted. This fold is cheap and can come last.

## Open questions

1. Worth folding before v0x02 is even a draft?
2. 243,780 instructions for a path that cannot fire: keep it in the load gate?
````

<!-- PAYLOAD ADR-134-fold-keymgmt.md -->
````text
<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-134: Fold keymgmt into unheaded-auth as a registry, never a key generator

**Status:** Proposed (parked). No code until Stevie reads this.
**Date:** 2026-11-04
**Related:** DP-203, DP-204, DP-06, ADR-103, ADR-119 open question 6, TRUE-WIRE C7, L5.

## The pitch (decision pack, DP-203)

> ... keymgmt into unheaded-auth.

## What is there

`services/keymgmt` plus `handler`, `lifecycle`, `store`: HTTP on 19007, no auth. **`GenerateKey` generates no key.** It allocates a 16-bit KeyRef and stores a record whose `PublicKeyHex` stays empty, then logs "PQC key generated". `store/sophia_store.go` is an in-memory stand-in. The useful part is the lifecycle: 60-second grace, revocation cascading to SigRefs, KeyRef overflow handled.

## Verdict: right for the registry, wrong for generation

unheaded-auth already publishes public keys (JWKS) and has rotation semantics, so a registry of public keys and KeyRefs fits it. Generation does not:

1. **Custody.** DP-06: daemon keys live on hosts under dedicated accounts; `unheaded-keygen` makes them. Series rule 5 keeps key generation for custody attended. The login-facing issuer must never hold another service's private key.
2. **Blast radius.** The issuer is the most exposed identity service; a key-minting API on it is the wrong place.
3. **No BPF.** `deploy/systemd/unheaded-auth.service` has an empty `CapabilityBoundingSet`, so it can never write Sophia's PQC maps.

## Decision direction

Flag `--feature-keyreg`, default off: `/api/v1/keyreg/` behind admin RBAC, taking **public** keys from `unheaded-keygen` output and assigning KeyRefs. Rename `GenerateKey` to `RegisterKey` so the log stops lying. ADR-119 can then use it or stay on `trust.d`: it works either way.

## Open questions

1. Does anything need KeyRefs before ADR-119 is built? If not, this fold can wait with it.
2. Revocation list: Wotan signed topic, or a file in `trust.d`?
````

<!-- PAYLOAD index_rows_g9.py -->
````text
#!/usr/bin/env python3
"""Add ADR-127..134 to docs/adr/ADR-INDEX.md after the last numbered row."""
import re, sys
p = "docs/adr/ADR-INDEX.md"; s = open(p).read()
rows = """| [127](ADR-127-fold-compliance.md) | Fold wire compliance into compliance-evidence behind --serve; not unheaded-baseline (oneshot, reads /etc/shadow); no data is not 100 % | **Proposed** (parked) | 2026-11-04 |
| [128](ADR-128-fold-firewall.md) | Fold firewall into Cuirass as intent only; Cuirass has no CAP_BPF and runs once: the node writer question (Go writer recommended) | **Proposed** (parked) | 2026-11-04 |
| [129](ADR-129-fold-qos.md) | Fold QoS into Cuirass without its 10 Hz publisher | **Proposed** (parked) | 2026-11-04 |
| [130](ADR-130-fold-nfv.md) | Fold NFV: where is a question: Cuirass has no CAP_BPF, cartridges are a different mechanism; prog_fd over HTTP goes | **Proposed** (parked) | 2026-11-04 |
| [131](ADR-131-fold-failover.md) | Fold failover into Cuirass as the actuator; deciding stays with Akira | **Proposed** (parked) | 2026-11-04 |
| [132](ADR-132-fold-loadbalancer.md) | Fold the load balancer into Cuirass; the map write goes to the node writer | **Proposed** (parked) | 2026-11-04 |
| [133](ADR-133-fold-version.md) | Fold version translation into monad, thin: one wire version exists | **Proposed** (parked) | 2026-11-04 |
| [134](ADR-134-fold-keymgmt.md) | Fold keymgmt into unheaded-auth as a public-key registry; GenerateKey generates no key | **Proposed** (parked) | 2026-11-04 |
"""
m = list(re.finditer(r"^\| \[1\d\d\]\(ADR-1\d\d-[^\n]*\n", s, flags=re.M))
if not m:
    sys.exit("ANCHOR MISSING: no ADR-1xx rows")
i = m[-1].end()
open(p, "w").write(s[:i] + rows + s[i:])
print("index rows added after:", m[-1].group(0)[:30])
````

<!-- PAYLOAD compliance_test_edit.py -->
````text
#!/usr/bin/env python3
"""DP-203 compliance fold: the test edit, run first. Run from the worktree root."""
import sys
def rep(p, old, new):
    s = open(p).read()
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
    open(p, "w").write(s.replace(old, new, 1))
T = "services/compliance/wire_compliance_test.go"
rep(T, '''	t.Run("empty service has 100% score", func(t *testing.T) {
		svc := newTestService(t)
		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		if score.ScorePct != 100.0 {
			t.Errorf("ScorePct = %.2f, want 100.0 for empty service", score.ScorePct)
		}
	})
''', '''	t.Run("empty service reports no data, not 100%", func(t *testing.T) {
		svc := newTestService(t)
		if !svc.GetScore().NoData {
			t.Error("NoData = false before any aggregation, want true")
		}
		svc.aggregateViolations(context.Background())
		score := svc.GetScore()

		if !score.NoData {
			t.Error("NoData = false for an empty service, want true")
		}
		if score.ScorePct != 0 || score.AuditCompleteness != 0 {
			t.Errorf("ScorePct = %.2f, AuditCompleteness = %.2f, want 0 and 0", score.ScorePct, score.AuditCompleteness)
		}
	})

	t.Run("first evaluated packet clears no data", func(t *testing.T) {
		svc := newTestService(t)
		svc.RecordAllowed()
		svc.aggregateViolations(context.Background())
		if svc.GetScore().NoData {
			t.Error("NoData = true after a packet, want false")
		}
	})
''')
s = open(T).read().rstrip("\n") + '''

// The lists trim; the score must not. 300 blocked of 1,000 is 70 % whatever
// MaxViolations is, and audit completeness counts records, not list length.
func TestScoreSurvivesTrimming(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxViolations, cfg.MaxAuditEntries = 100, 100
	svc := NewService(logger.New(io.Discard), nil, cfg)
	for i := 0; i < 700; i++ {
		svc.RecordAllowed()
	}
	for i := 0; i < 300; i++ {
		svc.RecordViolation(Violation{Timestamp: time.Now(), ViolationType: ViolationPolicy, ActionTaken: ActionBlock})
	}
	svc.aggregateViolations(context.Background())
	score := svc.GetScore()
	if score.Blocked != 300 || score.Violations != 300 || score.ScorePct != 70.0 {
		t.Fatalf("score = %+v, want 300 blocked, 300 violations, 70%%", score)
	}
	if score.AuditCompleteness != 0.3 {
		t.Fatalf("AuditCompleteness = %.4f, want 0.3", score.AuditCompleteness)
	}
	if n := len(svc.ListViolations()); n != 100 {
		t.Fatalf("violations kept = %d, want 100", n)
	}
}
'''
open(T, "w").write(s)
print("compliance_test_edit.py: applied")
````

<!-- PAYLOAD compliance_code.py -->
````text
#!/usr/bin/env python3
"""DP-203 compliance fold, service half. Run from the worktree root."""
import sys
def rep(p, old, new):
    s = open(p).read()
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
    open(p, "w").write(s.replace(old, new, 1))
W = "services/compliance/wire_compliance.go"
rep(W, '''	"fmt"
	"net/http"
	"strings"
''', '''	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
''')
rep(W, '''	ViolationCircuit   ViolationType = "CIRCUIT"
)
''', '''	ViolationCircuit   ViolationType = "CIRCUIT"
	// ViolationEncryption and ViolationZone are the compliance-ebpf audit
	// record's VIOLATION_ENCRYPTION_REQUIRED and VIOLATION_ZONE_BREACH.
	ViolationEncryption ViolationType = "ENCRYPTION"
	ViolationZone       ViolationType = "ZONE"
)
''')
rep(W, '''	ScorePct          float64 `json:"score_pct"`          // overall percentage
}
''', '''	ScorePct          float64 `json:"score_pct"`          // overall percentage
	// NoData is true until a packet has been evaluated. ScorePct and
	// AuditCompleteness are then 0, not 100: an empty plane proves nothing,
	// and evidence must be able to fail (ADR-097).
	NoData bool `json:"no_data"`
}
''')
rep(W, '''type Config struct {
	AggregationWindow time.Duration `json:"aggregation_window"`
''', '''type Config struct {
	// ListenAddr is where Start binds; loopback by default.
	ListenAddr        string        `json:"listen_addr"`
	AggregationWindow time.Duration `json:"aggregation_window"`
''')
rep(W, '''	return &Config{
		AggregationWindow: 10 * time.Second,
''', '''	return &Config{
		ListenAddr:        net.JoinHostPort("127.0.0.1", strconv.Itoa(ports.Compliance)),
		AggregationWindow: 10 * time.Second,
''')
rep(W, '''	auditLog   []AuditEntry              // audit trail
''', '''	auditLog   []AuditEntry              // audit trail, newest MaxAuditEntries
	auditTotal int64                     // audit records ever seen; the list trims, this does not
''')
rep(W, '''		zones:      make(map[string]*GeoZone),
		alertsCh:   make(chan *wotanClient.Message, 100),
	}
}
''', '''		zones:      make(map[string]*GeoZone),
		score:      ComplianceScore{NoData: true},
		alertsCh:   make(chan *wotanClient.Message, 100),
	}
}
''')
rep(W, '''// Start initializes Wotan subscriptions, HTTP server, and violation aggregation.
func (s *WireComplianceService) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
''', '''// Start runs StartBackground and serves Handler on Config.ListenAddr with no
// authentication: for tests. A deployment mounts Handler behind pkg/auth
// (cmd/compliance-evidence wire --serve).
func (s *WireComplianceService) Start(ctx context.Context) error {
	if err := s.StartBackground(ctx); err != nil {
		return err
	}

	s.httpServer = &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Addr:              s.config.ListenAddr,
		Handler:           s.Handler(),
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	s.log.Info().Str("addr", s.config.ListenAddr).Msg("wire compliance service started")
	return nil
}

// Handler returns the service's routes on a new mux.
func (s *WireComplianceService) Handler() *http.ServeMux {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// StartBackground starts the Wotan subscriptions and the aggregation loop,
// without a listener.
func (s *WireComplianceService) StartBackground(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.started, 0, 1) {
''')
rep(W, '''	// Start HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Addr:              ports.DefaultAddr(ports.Compliance),
		Handler:           mux,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start violation aggregation loop
	go s.aggregationLoop(ctx)

	s.log.Info().Int("port", ports.Compliance).Msg("wire compliance service started")
	return nil
}
''', '''	// Start violation aggregation loop
	go s.aggregationLoop(ctx)
	return nil
}
''')
rep(W, '''	totalPackets := s.score.TotalPackets
	if totalPackets == 0 {
		s.score.ScorePct = 100.0
		s.score.AuditCompleteness = 1.0
		return
	}

	violations := int64(len(s.violations))
''', '''	totalPackets := s.score.TotalPackets
	if totalPackets == 0 {
		s.score.NoData = true
		s.score.ScorePct = 0
		s.score.AuditCompleteness = 0
		return
	}
	s.score.NoData = false

	violations := s.score.Blocked
''')
rep(W, '''	// Audit completeness: ratio of audit entries to total packets
	auditEntries := int64(len(s.auditLog))
	if totalPackets > 0 {
		s.score.AuditCompleteness = float64(auditEntries) / float64(totalPackets)
''', '''	// Audit completeness: audit records ever seen against total packets.
	// Counted, not read off the list: the list trims at MaxAuditEntries.
	if totalPackets > 0 {
		s.score.AuditCompleteness = float64(s.auditTotal) / float64(totalPackets)
''')
rep(W, '''	s.mu.Lock()
	s.violations = append(s.violations, v)
	s.score.Blocked++
	s.score.TotalPackets++

	// Also add to audit log
	s.auditLog = append(s.auditLog, AuditEntry{
''', '''	s.mu.Lock()
	s.violations = append(s.violations, v)
	if over := len(s.violations) - s.config.MaxViolations; over > 0 && s.config.MaxViolations > 0 {
		s.violations = s.violations[over:]
	}
	s.score.Blocked++
	s.score.TotalPackets++

	// Also add to audit log
	s.appendAudit(AuditEntry{
''')
rep(W, '''// RecordAllowed records a successful policy check (allowed traffic).
''', '''// appendAudit keeps the newest MaxAuditEntries entries and counts every one.
// The caller holds s.mu.
func (s *WireComplianceService) appendAudit(e AuditEntry) {
	s.auditLog = append(s.auditLog, e)
	s.auditTotal++
	if over := len(s.auditLog) - s.config.MaxAuditEntries; over > 0 && s.config.MaxAuditEntries > 0 {
		s.auditLog = s.auditLog[over:]
	}
}

// RecordAllowed records a successful policy check (allowed traffic).
''')
print("compliance_code.py: all edits applied")
````

<!-- PAYLOAD wire_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRunWire_OffByDefault(t *testing.T) {
	var out bytes.Buffer
	if code := runWire(nil, &out, io.Discard); code != 0 {
		t.Fatalf("runWire = %d, want 0", code)
	}
	if got := strings.TrimSpace(out.String()); got != wireOffMessage {
		t.Fatalf("output = %q, want %q", got, wireOffMessage)
	}
}

func TestRunWire_RefusesOperands(t *testing.T) {
	var errOut bytes.Buffer
	if code := runWire([]string{"now"}, io.Discard, &errOut); code != 2 {
		t.Fatalf("runWire(operand) = %d, want 2", code)
	}
	if !strings.Contains(errOut.String(), "--help") {
		t.Fatalf("stderr = %q, want the --help hint", errOut.String())
	}
}

// ADR-111: --help on stdout, exit 0.
func TestRunWire_Help(t *testing.T) {
	var out bytes.Buffer
	if code := runWire([]string{"--help"}, &out, io.Discard); code != 0 {
		t.Fatalf("runWire(--help) = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "--serve") {
		t.Fatalf("help output has no --serve: %q", out.String())
	}
}

func TestCheckWireListen(t *testing.T) {
	for _, c := range []struct {
		addr string
		auth bool
		ok   bool
	}{
		{"127.0.0.1:19015", false, true},
		{"[::1]:19015", false, true},
		{":19015", false, false},
		{"0.0.0.0:19015", false, false},
		{"192.168.13.2:19015", false, false},
		{"192.168.13.2:19015", true, true},
		{"nonsense", true, false},
	} {
		err := checkWireListen(c.addr, c.auth)
		if (err == nil) != c.ok {
			t.Errorf("checkWireListen(%q, %v) = %v, want ok=%v", c.addr, c.auth, err, c.ok)
		}
	}
}

func TestServeWire_NoDataUntilEvaluated(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "")
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serveWire(ctx, wireOptions{listen: "127.0.0.1:0"}, ready) }()
	var addr string
	select {
	case addr = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("listener not up after 5 s")
	}
	resp, err := http.Get("http://" + addr + "/api/v1/compliance/score")
	if err != nil {
		t.Fatalf("GET score: %v", err)
	}
	defer resp.Body.Close()
	var score struct {
		ScorePct float64 `json:"score_pct"`
		NoData   bool    `json:"no_data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&score); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !score.NoData || score.ScorePct != 0 {
		t.Fatalf("score = %+v, want no_data true and score_pct 0", score)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveWire: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serveWire did not stop")
	}
}
````

<!-- PAYLOAD wire.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
	"unheaded/pkg/logger"
	utls "unheaded/pkg/tls"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/services/compliance"
)

// `compliance-evidence wire`: the wire compliance plane (services/compliance),
// folded in behind --serve (ADR-127, DP-203). Without --serve it says it is
// off and does nothing: no listener, no Wotan, no eBPF. No unit, timer or
// compose service passes --serve. The evidence file is untouched either way.

const wireOffMessage = "wire compliance is off; pass --serve to run it (ADR-127)"

type wireOptions struct {
	serve     bool
	listen    string
	wotanAddr string
	auditRing string
}

// runWire returns the process exit status: 0, 1 for a failed serve, 2 for a
// usage error, and --help goes to stdout with 0 (ADR-111).
func runWire(args []string, stdout, stderr io.Writer) int {
	var o wireOptions
	spec := cmdline.Spec{Name: "compliance-evidence wire", Purpose: "Run the wire compliance plane; off unless --serve (ADR-127)."}
	fl := flag.NewFlagSet("wire", flag.ContinueOnError)
	fl.SetOutput(io.Discard)
	fl.BoolVar(&o.serve, "serve", false, "run the wire compliance plane (default off)")
	fl.StringVar(&o.listen, "listen", compliance.DefaultConfig().ListenAddr, "HTTP listen address; anything but loopback needs AUTH_ENABLED=true")
	fl.StringVar(&o.wotanAddr, "wotan", "", "Wotan HTTP address (default: none, violations stay local)")
	fl.StringVar(&o.auditRing, "audit-ring", "", "pinned compliance-ebpf AUDIT_LOG ring to consume (default: none)")
	if err := fl.Parse(args); err != nil {
		return cmdline.HandleParseError(err, fl, spec, args, stdout, stderr)
	}
	if fl.NArg() > 0 {
		return cmdline.ReportUsageError(stderr, spec, "unexpected argument %q", fl.Arg(0))
	}
	if !o.serve {
		fmt.Fprintln(stdout, wireOffMessage)
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serveWire(ctx, o, nil); err != nil {
		fmt.Fprintln(stderr, "compliance-evidence wire:", err)
		return 1
	}
	return 0
}

// checkWireListen refuses a non-loopback address without authentication: the
// plane accepts policy writes.
func checkWireListen(addr string, authEnabled bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("--listen %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if !authEnabled && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("--listen %q is not loopback: set AUTH_ENABLED=true first", addr)
	}
	return nil
}

// serveWire runs the plane until ctx ends. ready, when set, receives the bound
// address once the listener is up.
func serveWire(ctx context.Context, o wireOptions, ready chan<- string) error {
	authCfg := auth.LoadServiceAuthConfig("compliance-evidence")
	if err := checkWireListen(o.listen, authCfg.Enabled); err != nil {
		return err
	}
	var wc *wotanClient.Client
	if o.wotanAddr != "" {
		wotanClient.InitServiceIdentity("compliance-evidence")
		c, err := wotanClient.NewClient(o.wotanAddr)
		if err != nil {
			return fmt.Errorf("wotan: %w", err)
		}
		wc = c
	}
	svc := compliance.NewService(logger.New(os.Stderr), wc, nil)
	if err := svc.StartBackground(ctx); err != nil {
		return err
	}
	if o.auditRing != "" {
		go func() {
			if err := consumeAuditRing(ctx, o.auditRing, svc); err != nil {
				fmt.Fprintln(os.Stderr, "compliance-evidence wire: audit ring:", err)
			}
		}()
	}
	mux := svc.Handler()
	h, err := auth.NewServiceHandler(mux, authCfg, auth.ServiceRoutes{})
	if err != nil {
		return err
	}
	srv, _, err := utls.MaybeWrapServer(o.listen, httputil.NewServiceMetrics("wire-compliance").Instrument(mux, h))
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", o.listen)
	if err != nil {
		return err
	}
	if ready != nil {
		ready <- ln.Addr().String()
	}
	errc := make(chan error, 1)
	go func() {
		if srv.TLSConfig != nil {
			errc <- srv.ServeTLS(ln, "", "")
			return
		}
		errc <- srv.Serve(ln)
	}()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	}
}
````

<!-- PAYLOAD wire_audit.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"context"
	"errors"

	"unheaded/services/compliance"
)

// consumeAuditRing is replaced by the ring consumer in Phase 5.
func consumeAuditRing(_ context.Context, _ string, _ *compliance.WireComplianceService) error {
	return errors.New("--audit-ring is not built yet")
}
````

<!-- PAYLOAD wire_main.py -->
````text
#!/usr/bin/env python3
"""Wire the `wire` subcommand into cmd/compliance-evidence/main.go. Run from the worktree root."""
import sys
p = "cmd/compliance-evidence/main.go"; s = open(p).read()
for old, new in [
 ('''	if len(os.Args) > 1 && os.Args[1] == "findings" {
''', '''	if len(os.Args) > 1 && os.Args[1] == "wire" {
		os.Exit(runWire(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "findings" {
'''),
 ('''			{Name: "baseline", Purpose: "host baseline"},
''', '''			{Name: "baseline", Purpose: "host baseline"},
			{Name: "wire", Purpose: "wire compliance plane, off unless --serve (ADR-127)"},
'''),
 ('''//	go run ./cmd/compliance-evidence snapshot -host east -key SEED -out east.snapshot.json
''', '''//	go run ./cmd/compliance-evidence snapshot -host east -key SEED -out east.snapshot.json
//	go run ./cmd/compliance-evidence wire --serve     # wire compliance plane, off by default (ADR-127)
'''),
]:
    if old not in s:
        sys.exit(f"ANCHOR MISSING: {old[:60]!r}")
    s = s.replace(old, new, 1)
open(p, "w").write(s)
print("wire_main.py: all edits applied")
````

<!-- PAYLOAD audit_record_test.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"context"
	"encoding/binary"
	"testing"
)

func auditBytes(decision, violation, class byte) []byte {
	b := make([]byte, 64)
	binary.LittleEndian.PutUint16(b[8:10], 7)
	binary.LittleEndian.PutUint16(b[10:12], 9)
	b[12], b[13], b[14] = decision, violation, class
	binary.LittleEndian.PutUint32(b[16:20], 0xABCDE)
	b[20] = 3
	return b
}

func TestIngestAuditRecord_AllowAndBlock(t *testing.T) {
	svc := newTestService(t)
	if err := svc.IngestAuditRecord(auditBytes(auditDecisionAllow, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := svc.IngestAuditRecord(auditBytes(auditDecisionBlock, auditViolationPIIEgress, 3)); err != nil {
		t.Fatal(err)
	}
	svc.aggregateViolations(context.Background())
	score := svc.GetScore()
	if score.NoData || score.Allowed != 1 || score.Blocked != 1 || score.ScorePct != 50.0 {
		t.Fatalf("score = %+v, want one allowed, one blocked, 50%%", score)
	}
	if score.AuditCompleteness != 1.0 {
		t.Fatalf("AuditCompleteness = %.4f, want 1: every ring record is an audit record", score.AuditCompleteness)
	}
	v := svc.ListViolations()
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].SrcServiceID != "7" || v[0].DstServiceID != "9" || v[0].ViolationType != ViolationPIIEgress ||
		v[0].Classification != ClassSovereignPII || v[0].TraceID != "abcde" || v[0].ZoneID != "3" {
		t.Fatalf("violation = %+v", v[0])
	}
}

func TestIngestAuditRecord_HostileInputRefused(t *testing.T) {
	svc := newTestService(t)
	for name, b := range map[string][]byte{
		"short":           make([]byte, 63),
		"long":            make([]byte, 65),
		"nil":             nil,
		"zero decision":   auditBytes(0, 0, 0),
		"bad decision":    auditBytes(9, 0, 0),
		"bad violation":   auditBytes(auditDecisionBlock, 0x7f, 0),
		"none on a block": auditBytes(auditDecisionBlock, 0, 0),
	} {
		if err := svc.IngestAuditRecord(b); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if s := svc.GetScore(); s.TotalPackets != 0 {
		t.Fatalf("TotalPackets = %d after refused input, want 0", s.TotalPackets)
	}
}
````

<!-- PAYLOAD audit_record.go -->
````text
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package compliance

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"time"

	"unheaded/pkg/protocol/bpfschema"
)

// Decisions and violation types of the compliance-ebpf audit record.
const (
	auditDecisionAllow = 0x01
	auditDecisionBlock = 0x02

	auditViolationPolicy     = 0x01
	auditViolationEncryption = 0x02
	auditViolationPIIEgress  = 0x03
	auditViolationZone       = 0x04
)

// IngestAuditRecord feeds one 64-byte compliance-ebpf AUDIT_LOG record into the
// score: the only path by which the plane sees a real packet. Hostile input:
// a record of the wrong size or an unknown decision is refused.
func (s *WireComplianceService) IngestAuditRecord(b []byte) error {
	if len(b) != bpfschema.ComplianceAuditRecordSize {
		return fmt.Errorf("audit record is %d bytes, want %d", len(b), bpfschema.ComplianceAuditRecordSize)
	}
	src := strconv.Itoa(int(binary.LittleEndian.Uint16(b[8:10])))
	dst := strconv.Itoa(int(binary.LittleEndian.Uint16(b[10:12])))
	trace := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b[16:20])), 16) // the flow label, host order
	class := DataClassification(b[14] & 0x03)
	zone := strconv.Itoa(int(b[20]))
	switch b[12] {
	case auditDecisionAllow:
		s.mu.Lock()
		s.appendAudit(AuditEntry{Timestamp: time.Now(), TraceID: trace, SrcServiceID: src, DstServiceID: dst,
			Action: ActionAllow, Classification: class, ZoneID: zone})
		s.mu.Unlock()
		s.RecordAllowed()
		return nil
	case auditDecisionBlock:
	default:
		return fmt.Errorf("audit record decision %#x", b[12])
	}
	var vt ViolationType
	switch b[13] {
	case auditViolationPolicy:
		vt = ViolationPolicy
	case auditViolationEncryption:
		vt = ViolationEncryption
	case auditViolationPIIEgress:
		vt = ViolationPIIEgress
	case auditViolationZone:
		vt = ViolationZone
	default:
		return fmt.Errorf("audit record violation type %#x", b[13])
	}
	s.RecordViolation(Violation{
		Timestamp:      time.Now(),
		SrcServiceID:   src,
		DstServiceID:   dst,
		ViolationType:  vt,
		Classification: class,
		ActionTaken:    ActionBlock,
		TraceID:        trace,
		ZoneID:         zone,
	})
	return nil
}
````

<!-- PAYLOAD audit_schema.py -->
````text
#!/usr/bin/env python3
"""Mirror compliance-ebpf's AuditRecord in bpfschema. Run from the worktree root."""
import subprocess, sys
B = "pkg/protocol/bpfschema/core_maps.go"; s = open(B).read().rstrip("\n") + '''

// === compliance-ebpf (DP-203) ===

// ComplianceAuditRecord mirrors ebpf/compliance-ebpf/src/main.rs AuditRecord,
// the AUDIT_LOG ring record.
type ComplianceAuditRecord struct {
	TimestampNs    uint64   // bpf_ktime_get_ns.
	SrcServiceID   uint16   // Source service.
	DstServiceID   uint16   // Destination service.
	Decision       uint8    // 1 allow, 2 block.
	ViolationType  uint8    // 0 none, 1 policy, 2 encryption, 3 PII egress, 4 zone.
	Classification uint8    // Low two bits of scratch[0].
	QosClass       uint8    // From the Monad.
	TraceID        uint32   // Flow label.
	ZoneID         uint8    // Geographic zone.
	Flags          uint8    // Monad flags snapshot.
	LatencyHint    [2]byte  // From the Monad.
	MonadSnapshot  [20]byte // The full Monad.
	_              [20]byte // Reserved.
}

// ComplianceAuditRecordSize is the exact wire size.
const ComplianceAuditRecordSize = 64
'''
open(B, "w").write(s)
R = "pkg/protocol/bpfschema/rust_source_test.go"; s = open(R).read()
anchor = '''	{reflect.TypeOf(FlowCancelValue{}), "flow-tracker/src/main.rs", "CancelFlowValue"},
'''
if anchor not in s:
    sys.exit("ANCHOR MISSING in rust_source_test.go")
open(R, "w").write(s.replace(anchor, anchor + '''	{reflect.TypeOf(ComplianceAuditRecord{}), "compliance-ebpf/src/main.rs", "AuditRecord"},
''', 1))
subprocess.run(["gofmt", "-w", B], check=True)
print("audit_schema.py: all edits applied")
````

<!-- PAYLOAD g9_docs.py -->
````text
#!/usr/bin/env python3
"""Compliance fold doc ripple. Run from the worktree root."""
import sys
def edit(p, old, new):
    s = open(p).read()
    if old not in s:
        sys.exit(f"ANCHOR MISSING in {p}: {old[:70]!r}")
    open(p, "w").write(s.replace(old, new, 1))
edit("docs/UNREACHED-GO-PACKAGES.md", "- `services/compliance` (G9; unwired: control plane for ebpf/compliance-ebpf)\n", "")
edit("docs/UNREACHED-GO-PACKAGES.md", "## The list\n", """## DP-203 (2026-11-04, S-G9-FOLD)

The G9 planes are not deleted (TRUE-WIRE C1-C6 defaults do not apply). `services/compliance` is reached by `cmd/compliance-evidence wire`, off unless `--serve` (ADR-127). The other seven stay on this list until Stevie has read ADR-128 to ADR-134.

## The list
""")
s = open("wiki/Compliance-Paperwork.md").read().rstrip("\n")
open("wiki/Compliance-Paperwork.md", "w").write(s + """

## Wire compliance plane

`compliance-evidence wire` is off: without `--serve` it says so and exits. `--serve` runs the plane on `127.0.0.1:19015` (ADR-127); any other address needs `AUTH_ENABLED=true`. `--audit-ring PATH` consumes a pinned `compliance-ebpf` `AUDIT_LOG`. With no packets evaluated the API answers `no_data: true` and a score of 0, never 100. No unit runs it, nothing loads `compliance_xdp` yet, and the plane adds nothing to the evidence file.
""")
print("g9_docs.py: all edits applied")
````


---

*S-G9-FOLD Battle Plan — Forged 2026-11-04*
*10 Phases. 78 Steps. Eight ADRs say where each plane belongs, including where the pack was wrong; one plane folds tonight.*
*An empty plane proves nothing, and now it says so.*
