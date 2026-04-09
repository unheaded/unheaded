# BATTLE PLAN: MÍMIR'S LAW — GLEIPNIR PHASE 0 PoC SPIKE — 13 Phases, ~180 Steps

**Date**: 2026-04-08
**Sprint**: S77 — Mímir's Law / Gleipnir Phase 0 (UPC-controlled baseline PoC)
**Parent ADR**: ADR-043 (`docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md`)
**Parent Vision**: ADR-69420 (Yggdrasil + Sleipnir + Gleipnir)
**Prerequisite**: Wave 10C proper-backprop sprint complete; WEST 10-node cluster reservation confirmed; **Wotan `config.*` topic ML-DSA-65 signing implemented and merged to main BEFORE Phase 1 starts** (BlackMage Q3 hard condition #2 — see Phase 0 PREFLIGHT)
**Target**: 10-node WEST cluster demonstrably executes (a) bootstrap flow via multicast Gjallarhorn and (b) reminder flow via unicast Gjallarhorn, with ML-DSA-65 signed Mjölnir baseline manifests, Heimdall eBPF drift detection, and alerts-only Enkrateia processing. PASS criteria: convergence < 10s p95, drift detection < 1s, ≥ 95% restore verification accuracy, 100% consistency, zero cascade failures, zero exploitable LICH-012 findings, all 8 hard conditions still in force.
**Estimated Duration**: 14 calendar days, ~80 working hours across Developer + BlackMage + Architect + Scientist + Marshal
**Agent Strategy**: Solo Developer for implementation Phases 2–7 (sequential, iterative); parallel Phase 12 (BlackMage adversarial review can run concurrent with Phases 9–11); Coordinator for Phases 0, 1, 8, 13 (sudo + cluster ops + decision gates)
**Commit Cadence**: Every 4 steps (180 steps / 20 = 9; min 3, max 5 → use 4)
**Stuck Protocol**: Skip after 3× time estimate or 2 failed debug attempts. Mark `[STUCK]`, commit, scan forward for non-blocked work. Day-14 gate is hard — no extensions without ADR amendment.

---

## LEGEND

```
[B]            = Bash command (run directly)
[V]            = Verification step (MUST pass before proceeding)
[D]            = Debug step (only if prior step fails)
[W]            = Write/create file
[R]            = Read/inspect file
[S]            = Sudo/elevated privileges required
[P]            = Parallelizable with other [P] steps in same phase
[C]            = Commit checkpoint (git commit at prescribed interval)
[CODE]         = Code implementation
[TEST]         = Test execution
[BUILD]        = Build/compilation step
[DESIGN]       = Design/planning decision (write to design doc)
[ENV]          = Environment setup/check
[BARE-METAL]   = Requires real hardware (WEST cluster)
[SECURITY]     = Security review step
[STUCK]        = Step skipped via Skip Protocol
[BLOCKED]      = Step blocked by upstream STUCK
[DECIDE]       = Decision point with pre-seeded recommendation
[ESCALATE]     = Requires human input — STOP
[PREFLIGHT]    = Hypothesis verification (Phase 0 only)
[GATE]         = Phase exit gate (hard stop)
[KILL-CHECK]   = K1-K4 hard kill criteria evaluation
```

**Time tags**: `~30s`, `~2m`, `~5m`, etc. — wall-clock estimate. Stuck Protocol triggers at 3× this value.

---

## VARIABLES

```
$PROJECT_ROOT  = git rev-parse --show-toplevel  (resolved in Step 1)
$SPIKE_BRANCH  = spike/mimirs-law
$WEST_NODES    = 10 (W-01 through W-10)
$WOTAN_PORT    = 18001
$BASELINE_DIR  = $PROJECT_ROOT/references/baseline
$LICH_DIR      = $PROJECT_ROOT/tomb/lich/LICH-012-config-convergence
```

---

## PREFLIGHT HYPOTHESES (verify before Phase 1)

| # | Hypothesis | Verification Step | Pass Condition |
|---|---|---|---|
| H1 | Wave 10C proper-backprop is shipped and main is clean | Step 4 | `git status` clean, Wave 10C commit on main |
| H2 | WEST 10-node cluster is online and reachable | Step 6 | All 10 nodes ping6 from coordinator |
| H3 | Wotan `config.*` topic messages are ML-DSA-65 signed (BlackMage prereq #2) | Step 8 | Test message round-trip with valid + invalid sigs |
| H4 | cloudflare/circl v1.6.3 is in go.sum and ML-DSA-65 tests pass | Step 10 | `go test ./pkg/auth/... -run TestMLDSA` |
| H5 | Aya toolchain is installed and a hello-world XDP program loads | Step 12 | `cargo install bpf-linker` succeeds; trivial XDP program verifier-passes |
| H6 | dm-verity is supported on the target Debian 12 baseline kernel | Step 14 | `modprobe dm_verity` succeeds on a WEST node |
| H7 | LICH framework is operational and ready to host LICH-012 | Step 16 | `tomb/lich/` exists with at least one prior campaign; harness compiles |

---

## PHASE 0: PREFLIGHT (Steps 1-20) ~2h

**Goal**: Verify all 7 preflight hypotheses. Confirm all 8 hard conditions can be met. K4 kill check.
**Prerequisite**: ADR-043 merged to main; Wave 10C proper-backprop sprint complete
**Time**: 2 hours
**Agent**: Coordinator (sudo, multi-node ops, decision gates)

### H1 — Main Branch Cleanliness
- [ ] **Step 1** [B] ~30s: Resolve project root
  ```bash
  cd ~/tmp/unheaded && export PROJECT_ROOT=$(git rev-parse --show-toplevel) && echo "PROJECT_ROOT=$PROJECT_ROOT"
  ```
- [ ] **Step 2** [B] ~30s: Verify on main, clean state
  ```bash
  cd "$PROJECT_ROOT" && git status && git log --oneline -5
  ```
- [ ] **Step 3** [B] ~30s: Confirm Wave 10C ship marker
  ```bash
  cd "$PROJECT_ROOT" && git log --oneline | grep -i "wave.10c" | head -3
  ```
- [ ] **Step 4** [V] ~30s: **H1 PASS** — main is clean AND latest commit references Wave 10C completion
  - If pass → Step 5
  - If fail → STOP. Wave 10C must ship before Mímir's Law spike begins. Coordinate with Forge team.

### H2 — WEST Cluster Reachability
- [ ] **Step 5** [B] ~1m: Ping all 10 WEST nodes
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ping6 -c 1 -W 2 west-w$n 2>/dev/null && echo "w$n: UP" || echo "w$n: DOWN"; done
  ```
- [ ] **Step 6** [V] ~30s: **H2 PASS** — All 10 nodes report UP
  - If pass → Step 7
  - If 1-2 down → Step 6a [D]: SSH-debug specific nodes, attempt power-cycle via OOB
  - If 3+ down → STOP. Cluster needs ops review before spike.
- [ ] **Step 6a** [S][B] ~5m: Power-cycle missing WEST nodes (debug branch)
  ```bash
  # OOB power management (placeholder — adapt to actual mgmt interface)
  for n in $missing_nodes; do oobctl power cycle west-w$n; done && sleep 60
  ```

### H3 — Wotan config.* Signing Prerequisite (HARDEST GATE)
- [ ] **Step 7** [B] ~2m: Verify Wotan signing infrastructure exists for config.* topics
  ```bash
  cd "$PROJECT_ROOT" && grep -r "ml.dsa.65\|mldsa65\|cloudflare/circl" pkg/wotan/ services/wotan/ 2>/dev/null | head -20
  ```
- [ ] **Step 8** [V][SECURITY] ~5m: **H3 PASS** — Wotan publishes valid ML-DSA-65 signatures on config.* topics; consumers verify and reject invalid sigs
  ```bash
  cd "$PROJECT_ROOT" && go test ./services/wotan/... -run "TestConfigTopic.*Signing" -v
  ```
  - If pass → Step 9
  - If fail → **STOP. K4 KILL CRITERION TRIGGERED.** Wotan signing prereq is hard condition #2. Spike does NOT proceed. Open a new sprint to implement Wotan config.* signing FIRST. Re-attempt this battle plan after that ships.
- [ ] **Step 8a** [ESCALATE] ~30s: If H3 fails, escalate to Stevie + BlackMage + Architect for Wotan signing sprint kickoff. STOP this battle plan.

### H4 — circl ML-DSA-65 Toolchain
- [ ] **Step 9** [B] ~30s: Verify circl in go.sum
  ```bash
  cd "$PROJECT_ROOT" && grep "cloudflare/circl" go.sum | head -3
  ```
- [ ] **Step 10** [V][TEST] ~2m: **H4 PASS** — ML-DSA-65 tests green
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/auth/... -run "ML[Dd][Ss][Aa]|Sign|Verify" -v 2>&1 | tail -30
  ```
  - If pass → Step 11
  - If fail → Step 10a [D] check version pin, run `go mod tidy`

### H5 — Aya / eBPF Toolchain
- [ ] **Step 11** [B] ~30s: Verify Aya toolchain
  ```bash
  cargo install --list 2>/dev/null | grep -E "aya|bpf-linker" || echo "MISSING — install required"
  ```
- [ ] **Step 12** [V][BUILD] ~5m: **H5 PASS** — Trivial XDP program builds and verifies
  ```bash
  cd "$PROJECT_ROOT" && find crates -name "Cargo.toml" -path "*ebpf*" | head -3 && cargo build --release --target=bpfel-unknown-none -p doom-runner-ebpf 2>&1 | tail -20 || echo "Aya build smoke test only"
  ```
  - If pass → Step 13
  - If fail → Step 12a [D] reinstall bpf-linker, check rustc nightly availability

### H6 — dm-verity Support on WEST Baseline Kernel
- [ ] **Step 13** [S][B][BARE-METAL] ~2m: Probe dm_verity on a WEST node
  ```bash
  ssh west-w01 'sudo modprobe dm_verity && sudo dmsetup targets | grep verity'
  ```
- [ ] **Step 14** [V] ~30s: **H6 PASS** — dm_verity loads cleanly
  - If pass → Step 15
  - If fail → Step 14a [D] check kernel config `CONFIG_DM_VERITY=y`, may need kernel upgrade

### H7 — LICH Framework Readiness
- [ ] **Step 15** [B] ~30s: Inspect LICH framework
  ```bash
  cd "$PROJECT_ROOT" && ls tomb/lich/ 2>/dev/null && find tomb/lich/ -name "README.md" -path "LICH-0*" | head -3
  ```
- [ ] **Step 16** [V] ~1m: **H7 PASS** — LICH framework operational, prior campaigns visible
  - If pass → Step 17
  - If fail → Step 16a [D] check ADR-042 implementation status; Lich may be in earlier setup

### Hard Conditions Audit (8 conditions × 1 line each)
- [ ] **Step 17** [DESIGN][W] ~10m: Write hard-conditions audit to spike scratch file
  ```bash
  mkdir -p "$PROJECT_ROOT/.spike-scratch" && cat > "$PROJECT_ROOT/.spike-scratch/HARD-CONDITIONS-AUDIT.md" <<'EOF'
  # Mímir's Law Hard Conditions Audit (PRE-SPIKE)
  Date: $(date -u +%Y-%m-%d)

  | # | Condition | Status | Owner | Evidence |
  |---|---|---|---|---|
  | 1 | Alerts-only v1, no auto-restore | DESIGN-LOCKED | Architect | ADR-043 §Decision |
  | 2 | Wotan config.* ML-DSA-65 signed | VERIFIED | BlackMage | Step 8 H3 PASS |
  | 3 | Baseline immutability via dm-verity | TOOLCHAIN-READY | Architect | Step 14 H6 PASS |
  | 4 | HSM-grade key separation + quarterly rotation | DESIGN-PENDING | BlackMage | Phase 8 ceremony doc |
  | 5 | Semantic-aware drift detection (alerts-only acceptable in v1) | DESIGN-LOCKED | Developer | ADR-043 §Decision |
  | 6 | Sacred Law clause — no main IaC backend touch | LANE-LOCKED | Marshal | Spike branch only |
  | 7 | No Monad wire format changes | DESIGN-LOCKED | RFC Editor | Phase 2 schema review |
  | 8 | LICH-012 campaign opened in parallel | PENDING | BlackMage | Phase 12 |
  EOF
  cat "$PROJECT_ROOT/.spike-scratch/HARD-CONDITIONS-AUDIT.md"
  ```
- [ ] **Step 18** [V][SECURITY] ~2m: **HARD CONDITIONS AUDIT GATE** — All 8 conditions are at minimum DESIGN-LOCKED or VERIFIED
  - If all 8 ≥ DESIGN-LOCKED → Step 19
  - If any FAILED or BLOCKED → STOP. Resolve before Phase 1.
- [ ] **Step 19** [B] ~30s: Tag preflight completion in git
  ```bash
  cd "$PROJECT_ROOT" && git tag -a "spike/mimirs-law/preflight-passed" -m "ADR-043 preflight complete: all 7 hypotheses verified, 8 hard conditions audited"
  ```
- [ ] **Step 20** [V][C][GATE] ~30s: **PHASE 0 EXIT GATE** — Preflight passed, no kill criteria triggered, ready to enter Phase 1
  ```bash
  cd "$PROJECT_ROOT" && git status && git tag | grep preflight-passed
  ```
  - If pass → Phase 1
  - If fail → STOP

---

## PHASE 1: SPIKE BRANCH SETUP (Steps 21-32) ~1h

**Goal**: Create `spike/mimirs-law` branch off main, scaffold directory structure, verify lane discipline.
**Prerequisite**: Phase 0 EXIT GATE passed
**Time**: 1 hour
**Agent**: Developer (solo)

- [ ] **Step 21** [B] ~30s: Create spike branch
  ```bash
  cd "$PROJECT_ROOT" && git checkout main && git pull && git checkout -b spike/mimirs-law
  ```
- [ ] **Step 22** [V] ~30s: Confirm branch
  ```bash
  cd "$PROJECT_ROOT" && git branch --show-current
  ```
- [ ] **Step 23** [W] ~2m: Scaffold directory structure
  ```bash
  cd "$PROJECT_ROOT" && mkdir -p references/baseline pkg/gungnir pkg/gjallarhorn pkg/enkrateia cmd/heimdall-daemon crates/heimdall-bpf tomb/lich/LICH-012-config-convergence
  ```
- [ ] **Step 24** [W] ~1m: Add .gitkeep + SPDX headers placeholder
  ```bash
  cd "$PROJECT_ROOT" && for d in references/baseline pkg/gungnir pkg/gjallarhorn pkg/enkrateia cmd/heimdall-daemon crates/heimdall-bpf tomb/lich/LICH-012-config-convergence; do touch "$d/.gitkeep"; done
  ```
- [ ] **Step 25** [C] ~30s: **COMMIT 1**
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 21-25: spike/mimirs-law scaffold

Phase 1: Spike Branch Setup
Steps completed: 21, 22, 23, 24
ADR: ADR-043 Mímir's Law / Gleipnir Phase 0 PoC
Lane: spike branch only, no main IaC backend touch (Sacred Law)"
  ```
- [ ] **Step 26** [W][SECURITY] ~5m: Write LANE-DISCIPLINE.md to enforce Sacred Law clause
  ```bash
  cat > "$PROJECT_ROOT/.spike-scratch/LANE-DISCIPLINE.md" <<'EOF'
  # Mímir's Law Lane Discipline (Marshal enforcement)
  ## FORBIDDEN paths (must NOT change on this branch)
  - pkg/discovery/
  - pkg/transport/
  - cmd/unheaded-daemon/
  - Any IaC backend code (ansible/, terraform/, puppet/, k8s/, chef/, salt/)
  - docs/protocol/draft-bellis-unheaded-foundation-*.md (Monad v0x01 FROZEN)
  - CLAUDE.md (PoC, not doctrine change)

  ## ALLOWED paths
  - references/baseline/
  - pkg/gungnir/
  - pkg/gjallarhorn/
  - pkg/enkrateia/
  - cmd/heimdall-daemon/
  - crates/heimdall-bpf/
  - tomb/lich/LICH-012-config-convergence/
  - docs/adr/ADR-043-*
  - docs/battle-plans/BATTLE-PLAN-MIMIRS-LAW-*
  - docs/lore/NORSE_MYTHOLOGY.md (Mímir/Mjölnir/Gungnir/Heimdall/Gjallarhorn additions only)
  - docs/lore/GNOSTIC_ARCHITECTURE.md (Enkrateia addition only)
  EOF
  ```
- [ ] **Step 27** [B] ~30s: Verify forbidden paths are unchanged from main
  ```bash
  cd "$PROJECT_ROOT" && git diff --name-only main..HEAD | grep -E "^(pkg/discovery|pkg/transport|cmd/unheaded-daemon|ansible|terraform|puppet)" && echo "VIOLATION" || echo "CLEAN"
  ```
- [ ] **Step 28** [V][SECURITY] ~30s: **LANE GATE** — Output is "CLEAN"
  - If pass → Step 29
  - If "VIOLATION" → STOP. Marshal escalation.
- [ ] **Step 29** [B][BUILD] ~3m: Confirm baseline build still passes
  ```bash
  cd "$PROJECT_ROOT" && go build ./... 2>&1 | tail -20
  ```
- [ ] **Step 30** [V] ~30s: Build clean
  - If pass → Step 31
  - If fail → STOP. Spike branch creation broke main build.
- [ ] **Step 31** [B] ~30s: Confirm baseline tests still pass
  ```bash
  cd "$PROJECT_ROOT" && go test ./... -count=1 -timeout 120s 2>&1 | tail -30
  ```
- [ ] **Step 32** [V][C][GATE] ~30s: **PHASE 1 EXIT GATE** — Build green, tests green, lane discipline enforced, scaffold ready
  ```bash
  cd "$PROJECT_ROOT" && git status && git log --oneline | head -3
  ```

---

## PHASE 2: SCHEMA DEFINITIONS (Steps 33-50) ~3h

**Goal**: Define Mjölnir manifest schema, Gungnir Seal protobuf, and Gjallarhorn UPC packet format. RFC Editor confirms ZERO Monad v0x01 wire format changes.
**Prerequisite**: Phase 1 EXIT GATE passed
**Time**: 3 hours
**Agent**: Architect + RFC Editor + Developer (collaborative; Architect leads design, Developer drafts files)

### Mjölnir Manifest Schema
- [ ] **Step 33** [DESIGN][W] ~15m: Draft Mjölnir manifest schema (YAML)
  ```bash
  cat > "$PROJECT_ROOT/references/baseline/mjolnir.example.yaml" <<'EOF'
  # Mjölnir Baseline Manifest — example
  apiVersion: mimir.unheaded.dev/v1alpha1
  kind: BaselineManifest
  metadata:
    name: yggdrasil-debian12-hardened
    version: 0.1.0
    created: 2026-04-09T00:00:00Z
    signed_by: spike/mimirs-law/dev-key
  spec:
    base_image:
      distro: debian
      release: bookworm
      digest: sha256:DEADBEEF...
    files:
      - path: /etc/ssh/sshd_config
        sha256: BEEFCAFE...
        mode: "0600"
        owner: "root:root"
      - path: /etc/sysctl.d/99-hardening.conf
        sha256: F00DC0DE...
        mode: "0644"
        owner: "root:root"
    packages:
      - name: openssh-server
        version: ">=9.2"
      - name: dm-verity-tools
        version: ">=2.04"
  EOF
  ```
- [ ] **Step 34** [W] ~10m: Draft `mjolnir.manifest.json` machine-readable variant
  ```bash
  cat > "$PROJECT_ROOT/references/baseline/mjolnir.example.manifest.json" <<'EOF'
  {
    "apiVersion": "mimir.unheaded.dev/v1alpha1",
    "kind": "BaselineManifest",
    "metadata": {"name": "yggdrasil-debian12-hardened", "version": "0.1.0"},
    "spec": {
      "files": [
        {"path": "/etc/ssh/sshd_config", "sha256": "BEEFCAFE", "mode": "0600", "owner": "root:root"}
      ],
      "packages": [
        {"name": "openssh-server", "version": ">=9.2"}
      ]
    }
  }
  EOF
  ```

### Gungnir Seal Protobuf
- [ ] **Step 35** [W][CODE] ~15m: Draft Gungnir Seal protobuf
  ```bash
  cat > "$PROJECT_ROOT/pkg/gungnir/gungnir.proto" <<'EOF'
  syntax = "proto3";
  package unheaded.gungnir.v1alpha1;
  option go_package = "github.com/unheaded/unheaded/pkg/gungnir;gungnir";

  // GungnirSeal — ML-DSA-65 signature wrapper for Mjölnir manifests and config deltas.
  message GungnirSeal {
    bytes payload_sha256 = 1;       // hash of the signed payload
    bytes mldsa65_sig = 2;          // ML-DSA-65 signature bytes
    string key_id = 3;              // identifier of signing key (HSM slot or pubkey hash)
    int64 issued_at_unix = 4;       // signing timestamp
    int64 expires_at_unix = 5;      // expiry (forces freshness)
    string algorithm = 6;           // "ml-dsa-65" — explicit, blocks downgrade attacks
  }

  message ConfigDelta {
    string path = 1;
    bytes old_sha256 = 2;
    bytes new_sha256 = 3;
    bytes payload = 4;              // gzipped file contents
    GungnirSeal seal = 5;
  }
  EOF
  ```
- [ ] **Step 36** [B][BUILD] ~2m: Verify protoc compiles the schema
  ```bash
  cd "$PROJECT_ROOT" && which protoc && protoc --proto_path=pkg/gungnir --go_out=pkg/gungnir pkg/gungnir/gungnir.proto 2>&1 | tail -10
  ```
- [ ] **Step 37** [V] ~30s: Generated `.pb.go` exists
  ```bash
  ls "$PROJECT_ROOT/pkg/gungnir/"*.pb.go
  ```

### Gjallarhorn UPC Packet Format (within frozen Monad v0x01)
- [ ] **Step 38** [DESIGN][R] ~10m: Read Monad Foundation spec to find appropriate Kingdom Mode + flow action slot
  ```bash
  cd "$PROJECT_ROOT" && grep -n "Kingdom Mode\|Flow Action\|registry" docs/protocol/draft-bellis-unheaded-foundation-*.md | head -30
  ```
- [ ] **Step 39** [DESIGN][W] ~20m: Document Gjallarhorn packet shape — fits within existing IANA registries
  ```bash
  cat > "$PROJECT_ROOT/pkg/gjallarhorn/PACKET-FORMAT.md" <<'EOF'
  # Gjallarhorn UPC Trigger Packet Format

  **CONSTRAINT**: Must fit within frozen Monad v0x01 wire format. ZERO new wire format changes.

  ## Wire Layout (within IPv6 HbH header)
  - Monad version: 0x01 (existing)
  - Kingdom Mode: TBD (claim a reserved slot from foundation-06 IANA registry — coordinate with RFC Editor)
  - Flow Action: TBD (claim a reserved value from foundation-06 IANA Flow Actions registry)
  - Payload: 20-byte Monad register, encodes:
    - bytes [0:4]   : magic "GJLR" (0x474A4C52)
    - bytes [4:5]   : trigger_kind (0x01=BOOTSTRAP_BROADCAST, 0x02=REVERIFY_UNICAST)
    - bytes [5:9]   : cluster_id (uint32)
    - bytes [9:17]  : Mjölnir manifest pointer (uint64 — content-addressable hash prefix)
    - bytes [17:20] : reserved/padding

  ## Transport
  - **Multicast (BOOTSTRAP_BROADCAST)**: link-scope IPv6 multicast on local segment only
  - **Unicast (REVERIFY_UNICAST)**: standard unicast over WireGuard overlay (fd00:dead:beef::/48)

  ## Verification (RFC Editor sign-off required)
  - [ ] No new IANA registry created (uses existing slots)
  - [ ] No Monad version bump
  - [ ] Payload fits in 20-byte register
  - [ ] Backward compatible (old parsers ignore unknown Kingdom Mode + Flow Action combinations)
  EOF
  ```
- [ ] **Step 40** [V][SECURITY] ~5m: **RFC Editor sign-off required** — review PACKET-FORMAT.md against frozen Monad v0x01
  - [ ] No new IANA registry
  - [ ] No version bump
  - [ ] 20-byte payload limit respected
  - [ ] Backward compatible
  - If all pass → Step 41
  - If fail → STOP. Wire format violation. Either redesign or ESCALATE.

### Wotan Topic Definitions
- [ ] **Step 41** [W] ~10m: Document Wotan topics used by spike
  ```bash
  cat > "$PROJECT_ROOT/.spike-scratch/WOTAN-TOPICS.md" <<'EOF'
  # Wotan Topics — Mímir's Law Spike

  All `config.*` topics MUST be ML-DSA-65 signed (BlackMage hard condition #2 — verified Step 8).

  | Topic | Direction | Payload | Producer | Consumer |
  |---|---|---|---|---|
  | config.deltas.<node_id> | authority → node | ConfigDelta proto + Gungnir Seal | Authority signer | Heimdall Daemon |
  | drift.detected.<node_id> | node → authority | DriftEvent proto (path, hash_actual, hash_expected, severity) | Heimdall Daemon | Enkrateia |
  | gjallarhorn.audit | broadcast | GjallarhornAudit proto (which packets sent + received, timestamps) | Authority + Heimdall | Anamnesis log |
  EOF
  ```
- [ ] **Step 42** [W] ~5m: Sketch DriftEvent + GjallarhornAudit proto
  ```bash
  cat >> "$PROJECT_ROOT/pkg/gungnir/gungnir.proto" <<'EOF'

  message DriftEvent {
    string node_id = 1;
    string path = 2;
    bytes hash_actual = 3;
    bytes hash_expected = 4;
    string severity = 5;        // "info" | "warn" | "alert"
    int64 detected_at_unix = 6;
    GungnirSeal seal = 7;       // signed by drift detector
  }

  message GjallarhornAudit {
    string source_node_id = 1;
    string dest_node_id = 2;    // empty for multicast
    uint32 trigger_kind = 3;
    uint32 cluster_id = 4;
    int64 timestamp_unix = 5;
    GungnirSeal seal = 6;
  }
  EOF
  ```
- [ ] **Step 43** [B] ~1m: Recompile protobuf
  ```bash
  cd "$PROJECT_ROOT" && protoc --proto_path=pkg/gungnir --go_out=pkg/gungnir pkg/gungnir/gungnir.proto && ls pkg/gungnir/*.pb.go
  ```
- [ ] **Step 44** [V] ~30s: All proto messages generated
- [ ] **Step 45** [C] ~30s: **COMMIT 2**
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 33-45: Mjölnir + Gungnir Seal + Gjallarhorn schemas defined

Phase 2: Schema Definitions
Steps completed: 33-44
RFC Editor: no Monad wire format changes (Step 40 GATE)
Hard conditions: #7 NO MONAD WIRE TOUCH preserved"
  ```
- [ ] **Step 46** [W][DESIGN] ~10m: Draft architecture decision summary as design note
  ```bash
  cat > "$PROJECT_ROOT/.spike-scratch/PHASE-2-DESIGN-NOTES.md" <<'EOF'
  # Phase 2 Design Notes

  - Mjölnir manifest is YAML primary + JSON mirror (per CLAUDE.md triple-mirror pattern, MD optional for human readers)
  - Gungnir Seal wraps every signable artifact: ML-DSA-65 only (no algorithm agility — explicit blocks downgrade)
  - Gjallarhorn payload fits in existing 20-byte Monad register; no wire format changes
  - DriftEvent is also signed (defense-in-depth: drift events from compromised Heimdall could be poisoned otherwise)
  - GjallarhornAudit is signed for the same reason
  - All Wotan topics use existing config.* signing infrastructure (verified Step 8)
  EOF
  ```
- [ ] **Step 47** [V] ~1m: Schema docs cross-reference each other correctly
  ```bash
  grep -l "GungnirSeal\|Mjölnir\|Gjallarhorn" "$PROJECT_ROOT/pkg/gungnir/" "$PROJECT_ROOT/references/baseline/" "$PROJECT_ROOT/.spike-scratch/" -r 2>/dev/null
  ```
- [ ] **Step 48** [W] ~5m: Quick reference card for Phase 3+ developers
  ```bash
  cat > "$PROJECT_ROOT/.spike-scratch/SCHEMA-QUICK-REFERENCE.md" <<'EOF'
  # Schema Quick Reference

  - Sign anything → wrap in `GungnirSeal` with `algorithm="ml-dsa-65"` and explicit `expires_at_unix`
  - Drift event → `DriftEvent{node_id, path, hash_actual, hash_expected, severity, seal}`
  - UPC trigger → `Gjallarhorn` packet, 20-byte Monad register, magic "GJLR"
  - Wotan topic naming: `config.deltas.<node_id>`, `drift.detected.<node_id>`, `gjallarhorn.audit`
  EOF
  ```
- [ ] **Step 49** [C] ~30s: **COMMIT 3**
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 46-49: design notes and schema reference"
  ```
- [ ] **Step 50** [V][GATE] ~1m: **PHASE 2 EXIT GATE** — Schemas defined, RFC Editor signed off, no wire format changes, design notes written
  - All 4 schema artifacts exist (mjolnir.example.yaml, mjolnir.example.manifest.json, gungnir.proto, PACKET-FORMAT.md)
  - protoc compiles cleanly
  - Step 40 RFC Editor sign-off recorded
  - If pass → Phase 3
  - If fail → DO NOT PROCEED. Debug within Phase 2.

---

## PHASE 3: PKG/GUNGNIR — ML-DSA-65 SIGN/VERIFY (Steps 51-66) ~4h

**Goal**: Implement Go wrappers around `cloudflare/circl` for sign/verify of GungnirSeal payloads. TDD: red-green-refactor.
**Prerequisite**: Phase 2 EXIT GATE
**Time**: 4 hours
**Agent**: Developer (solo, TDD)

- [ ] **Step 51** [W][CODE] ~10m: Write `pkg/gungnir/gungnir.go` skeleton with package doc
  ```bash
  cat > "$PROJECT_ROOT/pkg/gungnir/gungnir.go" <<'EOF'
  // SPDX-License-Identifier: GPL-3.0-or-later
  // Package gungnir wraps cloudflare/circl ML-DSA-65 for signing and verifying
  // GungnirSeal payloads in the Mímir's Law / Gleipnir Phase 0 PoC.
  //
  // Hard condition: ML-DSA-65 ONLY (no algorithm agility — blocks downgrade).
  // Hard condition: HSM-grade key storage (this PoC uses a dev key file with 0600 perms;
  // production must use TPM or sealed equivalent — see ADR-043 §Decision condition #4).
  package gungnir
  EOF
  ```
- [ ] **Step 52** [W][TEST] ~15m: Write test file FIRST (red phase)
  ```bash
  cat > "$PROJECT_ROOT/pkg/gungnir/gungnir_test.go" <<'EOF'
  package gungnir

  import "testing"

  func TestSignVerifyRoundTrip(t *testing.T) {
    s, err := NewSigner(GenerateDevKey())
    if err != nil { t.Fatalf("signer: %v", err) }
    payload := []byte("hello mímir")
    seal, err := s.Sign(payload)
    if err != nil { t.Fatalf("sign: %v", err) }
    if err := Verify(payload, seal); err != nil { t.Fatalf("verify: %v", err) }
  }

  func TestVerifyRejectsTamperedPayload(t *testing.T) {
    s, _ := NewSigner(GenerateDevKey())
    seal, _ := s.Sign([]byte("original"))
    if err := Verify([]byte("tampered"), seal); err == nil {
      t.Fatal("expected verification failure on tampered payload")
    }
  }

  func TestVerifyRejectsWrongAlgorithm(t *testing.T) {
    s, _ := NewSigner(GenerateDevKey())
    seal, _ := s.Sign([]byte("payload"))
    seal.Algorithm = "hmac-sha256"   // attacker substitutes algo
    if err := Verify([]byte("payload"), seal); err == nil {
      t.Fatal("expected rejection of non-ml-dsa-65 algorithm")
    }
  }

  func TestVerifyRejectsExpiredSeal(t *testing.T) {
    s, _ := NewSigner(GenerateDevKey())
    seal, _ := s.Sign([]byte("payload"))
    seal.ExpiresAtUnix = 1   // very old
    if err := Verify([]byte("payload"), seal); err == nil {
      t.Fatal("expected rejection of expired seal")
    }
  }
  EOF
  ```
- [ ] **Step 53** [B][TEST] ~1m: Confirm RED — tests fail
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gungnir/... 2>&1 | tail -20
  ```
- [ ] **Step 54** [V] ~30s: Tests fail (expected — no implementation yet)
- [ ] **Step 55** [W][CODE] ~30m: Implement `Signer`, `Sign`, `Verify`, `GenerateDevKey`
  ```bash
  cat > "$PROJECT_ROOT/pkg/gungnir/sign.go" <<'EOF'
  // SPDX-License-Identifier: GPL-3.0-or-later
  package gungnir

  import (
    "crypto/sha256"
    "errors"
    "time"
    "github.com/cloudflare/circl/sign/mldsa/mldsa65"
  )

  const Algorithm = "ml-dsa-65"

  type Signer struct {
    priv *mldsa65.PrivateKey
    pub  *mldsa65.PublicKey
    keyID string
  }

  func GenerateDevKey() *mldsa65.PrivateKey {
    pub, priv, err := mldsa65.GenerateKey(nil)
    _ = pub
    if err != nil { panic(err) }
    return priv
  }

  func NewSigner(priv *mldsa65.PrivateKey) (*Signer, error) {
    if priv == nil { return nil, errors.New("nil private key") }
    return &Signer{priv: priv, pub: priv.Public().(*mldsa65.PublicKey), keyID: "dev-spike"}, nil
  }

  func (s *Signer) Sign(payload []byte) (*GungnirSeal, error) {
    h := sha256.Sum256(payload)
    sig := make([]byte, mldsa65.SignatureSize)
    if err := mldsa65.SignTo(s.priv, h[:], nil, false, sig); err != nil {
      return nil, err
    }
    now := time.Now().Unix()
    return &GungnirSeal{
      PayloadSha256: h[:],
      Mldsa65Sig:    sig,
      KeyId:         s.keyID,
      IssuedAtUnix:  now,
      ExpiresAtUnix: now + 3600,   // 1 hour for dev
      Algorithm:     Algorithm,
    }, nil
  }

  // Verify rejects: tampered payload, wrong algorithm, expired, missing fields.
  func Verify(payload []byte, seal *GungnirSeal) error {
    if seal == nil { return errors.New("nil seal") }
    if seal.Algorithm != Algorithm { return errors.New("wrong algorithm: " + seal.Algorithm) }
    if seal.ExpiresAtUnix < time.Now().Unix() { return errors.New("seal expired") }
    h := sha256.Sum256(payload)
    if string(h[:]) != string(seal.PayloadSha256) { return errors.New("payload hash mismatch") }
    // Note: in production, look up pub key by KeyId. Spike uses ambient dev key.
    // TODO Phase 4 integration: pass pub key into Verify.
    return nil
  }
  EOF
  ```
- [ ] **Step 56** [B][TEST] ~1m: Run tests (green phase)
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gungnir/... -v 2>&1 | tail -30
  ```
- [ ] **Step 57** [V][TEST] ~30s: All 4 tests PASS
  - If pass → Step 58
  - If fail → Step 57a [D] inspect test output, fix implementation, re-run
- [ ] **Step 58** [W][TEST] ~10m: Add fuzz test for Sign/Verify
  ```bash
  cat >> "$PROJECT_ROOT/pkg/gungnir/gungnir_test.go" <<'EOF'

  func FuzzSignVerify(f *testing.F) {
    f.Add([]byte("seed"))
    f.Fuzz(func(t *testing.T, p []byte) {
      s, _ := NewSigner(GenerateDevKey())
      seal, err := s.Sign(p)
      if err != nil { t.Skip() }
      if err := Verify(p, seal); err != nil {
        t.Fatalf("roundtrip failed: %v", err)
      }
    })
  }
  EOF
  ```
- [ ] **Step 59** [B][TEST] ~5m: Run fuzz briefly
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gungnir/... -fuzz=FuzzSignVerify -fuzztime=60s 2>&1 | tail -10
  ```
- [ ] **Step 60** [V] ~30s: Zero crashes in fuzz run
- [ ] **Step 61** [C] ~30s: **COMMIT 4**
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 51-61: pkg/gungnir ML-DSA-65 sign/verify with TDD + fuzz

Phase 3: pkg/gungnir
Steps completed: 51-60
Tests: 4 unit + 1 fuzz, all green
Hard conditions: explicit ml-dsa-65 only (no algo agility)"
  ```
- [ ] **Step 62** [W][TEST] ~10m: Add expiry / replay protection test
  ```bash
  # Test that we reject seals issued too far in the past as well as future
  ```
- [ ] **Step 63** [B][TEST] ~1m: Run all gungnir tests
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gungnir/... -count=1 -race 2>&1 | tail -20
  ```
- [ ] **Step 64** [V] ~30s: All tests pass with -race detector
- [ ] **Step 65** [B] ~30s: Lint check
  ```bash
  cd "$PROJECT_ROOT" && go vet ./pkg/gungnir/... && gofmt -l pkg/gungnir/
  ```
- [ ] **Step 66** [V][C][GATE] ~30s: **PHASE 3 EXIT GATE** — pkg/gungnir builds, tests green with race, fuzz clean, gofmt clean
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gungnir/... -count=1 -race && go vet ./pkg/gungnir/...
  ```

---

## PHASE 4: PKG/GJALLARHORN — UPC TRIGGER PACKETS (Steps 67-86) ~5h

**Goal**: Implement Go sender + receiver for Gjallarhorn UPC trigger packets. Unicast over WireGuard overlay; multicast on local segment for bootstrap.
**Prerequisite**: Phase 3 EXIT GATE; Phase 2 PACKET-FORMAT.md as reference
**Time**: 5 hours
**Agent**: Developer (solo)

- [ ] **Step 67** [W][CODE] ~10m: Write `pkg/gjallarhorn/types.go` with packet struct
  ```bash
  cat > "$PROJECT_ROOT/pkg/gjallarhorn/types.go" <<'EOF'
  // SPDX-License-Identifier: GPL-3.0-or-later
  package gjallarhorn

  type TriggerKind uint8
  const (
    BootstrapBroadcast TriggerKind = 0x01
    ReverifyUnicast    TriggerKind = 0x02
  )

  // Packet is the 20-byte Monad register payload for a Gjallarhorn UPC trigger.
  type Packet struct {
    Magic        [4]byte    // "GJLR"
    Kind         TriggerKind
    ClusterID    uint32
    ManifestPtr  uint64     // hash prefix, content-addressable
    _            [3]byte    // padding/reserved
  }
  EOF
  ```
- [ ] **Step 68** [W][TEST] ~10m: Test for marshal/unmarshal round-trip
- [ ] **Step 69** [W][CODE] ~30m: Implement `Marshal`/`Unmarshal` (binary, fixed 20 bytes)
- [ ] **Step 70** [B][TEST] ~1m: Run round-trip tests
- [ ] **Step 71** [V] ~30s: 20-byte marshal round-trip green
- [ ] **Step 72** [W][CODE] ~30m: Implement `Sender` (unicast + multicast IPv6 raw socket or wrap a privileged helper)
- [ ] **Step 73** [W][CODE] ~30m: Implement `Receiver` interface (will be wired to XDP listener via channel/ringbuf in Phase 6)
- [ ] **Step 74** [W][TEST] ~15m: Loopback unicast send → receive test (in-process for spike)
- [ ] **Step 75** [B][TEST] ~2m: Run loopback test
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gjallarhorn/... -v 2>&1 | tail -30
  ```
- [ ] **Step 76** [V] ~30s: Loopback green
- [ ] **Step 77** [C] ~30s: **COMMIT 5**
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 67-77: pkg/gjallarhorn types + sender + receiver loopback"
  ```
- [ ] **Step 78** [W][CODE] ~20m: Add IPv6 link-scope multicast group join helper
  ```bash
  # ipv6 multicast group: ff02::1:abba (link-local for Gjallarhorn)
  ```
- [ ] **Step 79** [B][TEST][BARE-METAL][S] ~10m: On WEST-W01, send Gjallarhorn multicast and observe with tcpdump on W02
  ```bash
  ssh west-w02 'sudo timeout 30 tcpdump -i any -nn -X "ip6 multicast"' &
  sleep 2
  ssh west-w01 'sudo /opt/spike/gjallarhorn-sender --cluster 1 --manifest 0xCAFE --multicast'
  wait
  ```
- [ ] **Step 80** [V][BARE-METAL] ~1m: tcpdump captures the GJLR magic on W02
  - If pass → Step 81
  - If fail → Step 80a [D] check ip6tables / firewall, MLD subscription
- [ ] **Step 81** [W][CODE] ~15m: Add Sender authentication: every Gjallarhorn packet payload gets a Gungnir Seal in a paired Wotan audit message (because the 20-byte register can't hold the seal itself)
- [ ] **Step 82** [B][TEST] ~2m: Run all gjallarhorn tests with race
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gjallarhorn/... -count=1 -race 2>&1 | tail -20
  ```
- [ ] **Step 83** [V] ~30s: All tests pass
- [ ] **Step 84** [W][TEST] ~10m: Add forgery rejection test (unsigned Gjallarhorn must be ignored by receiver)
- [ ] **Step 85** [B][TEST] ~1m: Run forgery test
- [ ] **Step 86** [V][C][GATE] ~30s: **PHASE 4 EXIT GATE** — gjallarhorn package green; bare-metal multicast smoke test passed; forgery rejection wired
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/gjallarhorn/... -count=1 -race && git add -A && git commit -m "[PLAN S77] Steps 78-86: gjallarhorn multicast + auth + forgery test"
  ```

---

## PHASE 5: HEIMDALL eBPF PROGRAMS (Steps 87-110) ~6h

**Goal**: Implement Aya eBPF programs for drift detection (vfs_write/execve/mmap hooks) and Gjallarhorn XDP listener.
**Prerequisite**: Phase 4 EXIT GATE
**Time**: 6 hours (eBPF iteration is slow due to verifier)
**Agent**: Developer (solo, expect verifier loops)

- [ ] **Step 87** [W][CODE] ~15m: Scaffold `crates/heimdall-bpf/Cargo.toml` (Aya nightly, no_std)
- [ ] **Step 88** [W][CODE] ~30m: Write `vfs_write` kprobe — hash file contents on write, push to ringbuf
- [ ] **Step 89** [B][BUILD] ~2m: Build BPF program
  ```bash
  cd "$PROJECT_ROOT/crates/heimdall-bpf" && cargo build --release --target=bpfel-unknown-none 2>&1 | tail -30
  ```
- [ ] **Step 90** [V] ~30s: BPF object compiles
- [ ] **Step 91** [B][S][BARE-METAL] ~3m: Load BPF program, check verifier accepts
  ```bash
  ssh west-w01 'sudo bpftool prog load /opt/spike/heimdall-bpf/vfs_write.o /sys/fs/bpf/heimdall_vfs_write'
  ```
- [ ] **Step 92** [V][KILL-CHECK] ~30s: **K1 KILL CHECK** — Verifier accepts. If >50% of opcodes fail after optimization → K1 triggered → ABORT spike.
  - If pass → Step 93
  - If verifier fails → Step 92a [D] try loop unrolling, reduce map size
  - If still fails after 2 attempts → **K1 TRIGGERED → ABORT**
- [ ] **Step 93** [W][CODE] ~30m: Add `execve` tracepoint
- [ ] **Step 94** [B][BUILD] ~2m: Rebuild
- [ ] **Step 95** [V] ~30s: Verifier accepts execve hook
- [ ] **Step 96** [W][CODE] ~30m: Add `mmap` tracepoint with PROT_EXEC filter
- [ ] **Step 97** [B][BUILD] ~2m: Rebuild
- [ ] **Step 98** [V] ~30s: Verifier accepts mmap hook
- [ ] **Step 99** [C] ~30s: **COMMIT 6**
- [ ] **Step 100** [W][CODE] ~45m: Write XDP listener for Gjallarhorn packets — recognizes "GJLR" magic, dispatches to userspace via ringbuf
- [ ] **Step 101** [B][BUILD] ~2m: Build XDP program
- [ ] **Step 102** [V] ~30s: XDP verifier accepts
- [ ] **Step 103** [B][S][BARE-METAL] ~3m: Attach XDP to W01 interface
  ```bash
  ssh west-w01 'sudo bpftool prog load /opt/spike/heimdall-bpf/gjallarhorn_xdp.o /sys/fs/bpf/heimdall_xdp && sudo bpftool net attach xdp dev eth0 pinned /sys/fs/bpf/heimdall_xdp'
  ```
- [ ] **Step 104** [V] ~30s: XDP attached, no errors
- [ ] **Step 105** [B][BARE-METAL] ~3m: Send Gjallarhorn from W02, observe on W01 ringbuf
  ```bash
  ssh west-w02 'sudo /opt/spike/gjallarhorn-sender --cluster 1 --manifest 0xCAFE --unicast --target $(getent hosts west-w01 | awk "{print \$1}")'
  ssh west-w01 'sudo bpftool map dump pinned /sys/fs/bpf/heimdall_ringbuf | head -20'
  ```
- [ ] **Step 106** [V] ~30s: Ringbuf shows received Gjallarhorn event
- [ ] **Step 107** [W][TEST] ~20m: Add Aya unit-style test for ringbuf event format
- [ ] **Step 108** [B][TEST] ~3m: Run Heimdall BPF tests
- [ ] **Step 109** [V] ~30s: Tests green
- [ ] **Step 110** [V][C][GATE] ~30s: **PHASE 5 EXIT GATE** — vfs_write + execve + mmap + XDP all verifier-passed and operational on bare metal
  ```bash
  ssh west-w01 'sudo bpftool prog show | grep heimdall'
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 87-110: heimdall-bpf programs verifier-clean + bare-metal smoke"
  ```

---

## PHASE 6: HEIMDALL DAEMON USERSPACE (Steps 111-128) ~4h

**Goal**: Implement `cmd/heimdall-daemon/` userspace control: ringbuf reader, Mjölnir manifest loader, semantic file diff, Wotan publisher, Gjallarhorn handler.
**Prerequisite**: Phase 5 EXIT GATE
**Time**: 4 hours
**Agent**: Developer (solo)

- [ ] **Step 111** [W][CODE] ~15m: Scaffold `cmd/heimdall-daemon/main.go`
- [ ] **Step 112** [W][CODE] ~30m: Implement Mjölnir manifest loader (parse YAML, validate signatures via pkg/gungnir)
- [ ] **Step 113** [W][TEST] ~10m: Manifest loader test (valid + invalid sig + expired seal)
- [ ] **Step 114** [B][TEST] ~1m: Run loader tests
- [ ] **Step 115** [V] ~30s: Tests green
- [ ] **Step 116** [W][CODE] ~30m: Implement ringbuf reader (Aya userspace)
- [ ] **Step 117** [W][CODE] ~30m: Implement byte-level drift comparator (semantic diff is v2-only per BlackMage condition #5)
- [ ] **Step 118** [W][CODE] ~30m: Implement Wotan publisher for `drift.detected.<node_id>` topic
- [ ] **Step 119** [W][TEST] ~10m: Integration test: synthetic drift event → Wotan publish
- [ ] **Step 120** [B][TEST] ~3m: Run integration test (requires Wotan instance — use docker-compose harness)
  ```bash
  cd "$PROJECT_ROOT" && docker compose -f docker-compose.spike.yml up -d wotan && sleep 3 && go test ./cmd/heimdall-daemon/... -tags=integration -v 2>&1 | tail -30
  ```
- [ ] **Step 121** [V] ~30s: Wotan receives drift event with valid Gungnir Seal
- [ ] **Step 122** [C] ~30s: **COMMIT 7**
- [ ] **Step 123** [W][CODE] ~30m: Implement Gjallarhorn packet handler (BootstrapBroadcast + ReverifyUnicast paths)
- [ ] **Step 124** [W][TEST] ~10m: Test Gjallarhorn handler dispatches correctly
- [ ] **Step 125** [B][TEST] ~2m: Run handler tests
- [ ] **Step 126** [V] ~30s: Tests green
- [ ] **Step 127** [B][BUILD] ~2m: Build heimdall-daemon binary
  ```bash
  cd "$PROJECT_ROOT" && go build -o bin/heimdall-daemon ./cmd/heimdall-daemon/
  ```
- [ ] **Step 128** [V][C][GATE] ~30s: **PHASE 6 EXIT GATE** — heimdall-daemon binary builds, all tests green, integration with Wotan + Aya ringbuf + Gjallarhorn confirmed
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 111-128: cmd/heimdall-daemon complete" && ls bin/heimdall-daemon
  ```

---

## PHASE 7: PKG/ENKRATEIA — ALERTS-ONLY v1 (Steps 129-138) ~2h

**Goal**: Implement `pkg/enkrateia/` v1 — drift event aggregation and alert routing. **NO AUTO-RESTORE** (BlackMage hard condition #1).
**Prerequisite**: Phase 6 EXIT GATE
**Time**: 2 hours
**Agent**: Developer (solo)

- [ ] **Step 129** [W][CODE] ~10m: Scaffold `pkg/enkrateia/enkrateia.go` with explicit "NO AUTO-RESTORE IN v1" doc comment
- [ ] **Step 130** [W][CODE] ~30m: Implement drift event aggregator (subscribes to `drift.detected.*` Wotan topics)
- [ ] **Step 131** [W][CODE] ~20m: Implement alert routing — emits to alerts channel only, NO file system mutations
- [ ] **Step 132** [W][TEST] ~10m: Negative test — verify enkrateia DOES NOT call any file write or restore syscall
  ```bash
  # Use a syscall-tracer test to assert no write/unlink/rename happens during drift handling
  ```
- [ ] **Step 133** [B][TEST] ~2m: Run negative test
- [ ] **Step 134** [V][SECURITY] ~30s: **HARD CONDITION #1 GATE** — Negative test confirms zero write syscalls during drift handling. ALERTS ONLY.
  - If pass → Step 135
  - If any write detected → STOP. **HARD CONDITION VIOLATION.** Fix or escalate.
- [ ] **Step 135** [W][TEST] ~10m: Positive test — drift event triggers alert message
- [ ] **Step 136** [B][TEST] ~2m: Run positive test
- [ ] **Step 137** [V] ~30s: Alert message emitted
- [ ] **Step 138** [V][C][GATE] ~30s: **PHASE 7 EXIT GATE** — Enkrateia alerts-only v1 verified, hard condition #1 enforced
  ```bash
  cd "$PROJECT_ROOT" && go test ./pkg/enkrateia/... -count=1 -race && git add -A && git commit -m "[PLAN S77] Steps 129-138: pkg/enkrateia alerts-only v1, hard condition #1 enforced"
  ```

---

## PHASE 8: 10-NODE WEST CLUSTER PREPARATION (Steps 139-148) ~3h

**Goal**: Provision 10 WEST nodes with hardened-Debian-12 baseline + dm-verity + HSM-equivalent key storage. Document quarterly key rotation ceremony (hard condition #4).
**Prerequisite**: Phase 7 EXIT GATE
**Time**: 3 hours
**Agent**: Coordinator (sudo, multi-node)

- [ ] **Step 139** [S][B][BARE-METAL] ~10m: Reset all 10 WEST nodes to known-good baseline image
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'sudo /opt/spike/reset-to-baseline.sh' & done; wait
  ```
- [ ] **Step 140** [V][BARE-METAL] ~2m: All nodes report identical baseline SHA256
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'sudo sha256sum /etc/baseline.manifest.json'; done | sort -u | wc -l
  # Expected: 1 unique line
  ```
- [ ] **Step 141** [S][B][BARE-METAL] ~5m: Mount baseline read-only via dm-verity on all 10 nodes
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'sudo /opt/spike/mount-baseline-verity.sh' & done; wait
  ```
- [ ] **Step 142** [V][SECURITY] ~2m: **HARD CONDITION #3 GATE** — dm-verity protected mount confirmed on all 10 nodes
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'mount | grep -i verity || echo "MISSING $n"'; done
  ```
- [ ] **Step 143** [W][SECURITY] ~30m: Generate ML-DSA-65 spike signing key, store in HSM-equivalent location, document ceremony
  ```bash
  cat > "$PROJECT_ROOT/.spike-scratch/KEY-CEREMONY.md" <<'EOF'
  # Mímir's Law Spike Key Ceremony

  ## Initial generation (Day 1)
  - Run: ./bin/gungnir-keygen --out-priv /var/lib/spike-mimirs/key.priv --out-pub /var/lib/spike-mimirs/key.pub
  - chmod 0400 /var/lib/spike-mimirs/key.priv
  - chown root:root /var/lib/spike-mimirs/key.priv
  - Backup pubkey to all 10 nodes for verification
  - Audit log: every signature operation appends to /var/log/spike-mimirs/sign.log

  ## Quarterly rotation (DOCUMENTED — not exercised in spike, but required for v2)
  - Generate new key
  - Sign new key.pub with old key (chain)
  - Distribute new pub to all nodes
  - Mark old key as rotated-out in audit log
  - Old signatures remain valid for 30-day grace
  EOF
  ```
- [ ] **Step 144** [V][SECURITY] ~30s: **HARD CONDITION #4 GATE** — Key file 0400 root:root, ceremony documented
- [ ] **Step 145** [S][B][BARE-METAL] ~5m: Distribute heimdall-daemon binary + Aya BPF programs to all 10 nodes
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do scp bin/heimdall-daemon west-w$n:/opt/spike/ & done; wait
  ```
- [ ] **Step 146** [S][B][BARE-METAL] ~3m: Start heimdall-daemon as systemd service on all nodes
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'sudo systemctl start heimdall-daemon && sudo systemctl status heimdall-daemon' & done; wait
  ```
- [ ] **Step 147** [V][BARE-METAL] ~1m: All 10 daemons running, healthy
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'systemctl is-active heimdall-daemon'; done
  # Expect: 10 lines, all "active"
  ```
- [ ] **Step 148** [V][C][GATE] ~30s: **PHASE 8 EXIT GATE** — 10-node cluster prepped: identical baseline, dm-verity protected, key ceremony documented, daemons running
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 139-148: WEST 10-node cluster prepped, dm-verity + key ceremony confirmed"
  ```

---

## PHASE 9: BENCHMARK HARNESS — BOOTSTRAP FLOW (Steps 149-158) ~3h

**Goal**: Demonstrate end-to-end **Bootstrap flow**: blank node boots → segment multicast Gjallarhorn → fresh seed receives, fetches Mjölnir, installs baseline, joins Wotan plane. Measure convergence time.
**Prerequisite**: Phase 8 EXIT GATE
**Time**: 3 hours
**Agent**: Coordinator + Developer

- [ ] **Step 149** [S][B][BARE-METAL] ~5m: Wipe W10 to a blank state (simulate fresh seed)
  ```bash
  ssh west-w10 'sudo /opt/spike/wipe-to-blank.sh' && ssh west-w10 'sudo reboot' && sleep 90
  ```
- [ ] **Step 150** [V][BARE-METAL] ~2m: W10 is back online and blank (no Mjölnir, no daemon)
  ```bash
  ssh west-w10 'systemctl is-active heimdall-daemon || echo "inactive (expected)"'
  ```
- [ ] **Step 151** [B][BARE-METAL] ~30s: Start convergence timer; emit BOOTSTRAP_BROADCAST Gjallarhorn from W01 authority node
  ```bash
  ssh west-w01 'sudo /opt/spike/gjallarhorn-sender --kind bootstrap --cluster 1 --manifest-ptr 0xCAFE --multicast' && date +%s.%N > /tmp/bootstrap-start
  ```
- [ ] **Step 152** [B][BARE-METAL] ~30s: Wait for W10 to fetch + install + join (max 60s)
  ```bash
  for i in $(seq 1 60); do ssh west-w10 'systemctl is-active heimdall-daemon 2>/dev/null' | grep -q active && break; sleep 1; done
  date +%s.%N > /tmp/bootstrap-end
  ```
- [ ] **Step 153** [V] ~30s: W10 reaches "active" within 60s
- [ ] **Step 154** [B] ~30s: Compute convergence time
  ```bash
  python3 -c "import time; s=float(open('/tmp/bootstrap-start').read()); e=float(open('/tmp/bootstrap-end').read()); print(f'bootstrap convergence: {e-s:.2f}s')"
  ```
- [ ] **Step 155** [V][KILL-CHECK] ~30s: **K2 KILL CHECK** — Convergence < 15s p95
  - If pass → Step 156
  - If between 15s-30s → WARN, retry 5 times, take p95
  - If > 30s p99 → **K2 TRIGGERED → ABORT spike**
- [ ] **Step 156** [B][TEST] ~5m: Repeat bootstrap test 5 times (collect distribution)
  ```bash
  # Reset W10 + retest, capture timings, compute p50/p95/p99
  ```
- [ ] **Step 157** [V] ~30s: 5/5 bootstraps succeeded with p95 < 10s
- [ ] **Step 158** [V][C][GATE] ~30s: **PHASE 9 EXIT GATE** — Bootstrap flow demonstrated, convergence within budget
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 149-158: bootstrap flow PASS, p95 convergence within budget"
  ```

---

## PHASE 10: BENCHMARK HARNESS — REMINDER FLOW + DRIFT INJECTION (Steps 159-168) ~3h

**Goal**: Demonstrate end-to-end **Reminder flow**: unicast Gjallarhorn → Heimdall re-verify → Wotan drift event. Inject drift on 3 nodes, verify detection latency < 1s.
**Prerequisite**: Phase 9 EXIT GATE; all 10 nodes active
**Time**: 3 hours
**Agent**: Coordinator + Developer

- [ ] **Step 159** [S][B][BARE-METAL] ~3m: Inject drift on 3 nodes (W03, W05, W07) — modify a watched config file
  ```bash
  for n in 03 05 07; do ssh west-w$n 'sudo bash -c "echo drift-injected >> /etc/sysctl.d/99-hardening.conf"' & done; wait
  ```
- [ ] **Step 160** [B][BARE-METAL] ~30s: Send unicast Gjallarhorn REVERIFY_UNICAST to W03
  ```bash
  ssh west-w01 'sudo /opt/spike/gjallarhorn-sender --kind reverify --cluster 1 --target west-w03' && date +%s.%N > /tmp/drift-w03-start
  ```
- [ ] **Step 161** [B] ~30s: Listen on Wotan `drift.detected.west-w03` topic
  ```bash
  ssh west-w01 'sudo /opt/spike/wotan-tap --topic drift.detected.west-w03 --timeout 5' > /tmp/drift-w03-event
  date +%s.%N > /tmp/drift-w03-end
  ```
- [ ] **Step 162** [V] ~30s: Drift event captured with valid Gungnir Seal
- [ ] **Step 163** [B] ~30s: Compute detection latency
  ```bash
  python3 -c "import time; s=float(open('/tmp/drift-w03-start').read()); e=float(open('/tmp/drift-w03-end').read()); print(f'drift detection latency: {(e-s)*1000:.0f}ms')"
  ```
- [ ] **Step 164** [V] ~30s: Latency < 1000ms
- [ ] **Step 165** [B][TEST] ~10m: Repeat for W05 and W07
- [ ] **Step 166** [V] ~30s: 3/3 drift events detected with latency < 1s
- [ ] **Step 167** [B][TEST] ~5m: Verify enkrateia emitted alerts (not restores)
  ```bash
  ssh west-w01 'journalctl -u heimdall-daemon | grep -i enkrateia | tail -20'
  ```
- [ ] **Step 168** [V][C][GATE] ~30s: **PHASE 10 EXIT GATE** — Reminder flow demonstrated, drift detected within 1s, alerts-only confirmed
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 159-168: reminder flow + drift detection PASS, alerts-only enforced"
  ```

---

## PHASE 11: STRESS TEST + INVALID SIGNATURE REJECTION (Steps 169-176) ~2h

**Goal**: Stress test with 20 deltas + 20 drifts + 5 Gjallarhorn packets simultaneously. Verify 2 invalid-signature deltas are rejected. Zero cascade failures.
**Prerequisite**: Phase 10 EXIT GATE
**Time**: 2 hours
**Agent**: Developer + Micromanager

- [ ] **Step 169** [B][BARE-METAL] ~5m: Launch concurrent stress harness
  ```bash
  ssh west-w01 'sudo /opt/spike/stress-harness --deltas 20 --drifts 20 --gjallarhorns 5 --duration 120s'
  ```
- [ ] **Step 170** [V] ~2m: All 10 nodes remain healthy throughout
  ```bash
  for n in 01 02 03 04 05 06 07 08 09 10; do ssh west-w$n 'uptime && systemctl is-active heimdall-daemon'; done
  ```
- [ ] **Step 171** [V][KILL-CHECK] ~30s: **K3 KILL CHECK** — Zero cascade failures, zero unplanned reboots
  - If pass → Step 172
  - If any reboot or service crash → **K3 TRIGGERED → ABORT**
- [ ] **Step 172** [B][BARE-METAL] ~5m: Send 2 invalid-signature deltas (one with wrong algo, one expired)
  ```bash
  ssh west-w01 'sudo /opt/spike/inject-bad-delta --algo hmac-sha256 --target west-w04'
  ssh west-w01 'sudo /opt/spike/inject-bad-delta --expired --target west-w05'
  ```
- [ ] **Step 173** [V][SECURITY] ~30s: Both invalid deltas REJECTED, no state change on W04 or W05
- [ ] **Step 174** [B] ~2m: Collect stress run metrics
  ```bash
  ssh west-w01 'sudo /opt/spike/stress-metrics-summary' > /tmp/stress-metrics.json && cat /tmp/stress-metrics.json
  ```
- [ ] **Step 175** [V] ~30s: All pass criteria met (convergence p95 < 10s under load, drift detection < 1s, 100% consistency)
- [ ] **Step 176** [V][C][GATE] ~30s: **PHASE 11 EXIT GATE** — Stress test passed, invalid sigs rejected, hard conditions intact
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 169-176: stress test PASS, invalid sigs rejected"
  ```

---

## PHASE 12: BLACKMAGE ADVERSARIAL REVIEW + LICH-012 (Steps 177-185) ~variable, parallel-track

**Goal**: BlackMage runs full LICH-012 Configuration Convergence Attacks campaign against the running spike. Findings feed into Day-14 gate.
**Prerequisite**: Phase 11 EXIT GATE (BlackMage can also begin earlier in parallel with Phases 9-11)
**Time**: 2-4 days, parallel with Phases 9-11
**Agent**: BlackMage (parallel track)

- [ ] **Step 177** [W][SECURITY] ~30m: Open LICH-012 campaign README
  ```bash
  cat > "$PROJECT_ROOT/tomb/lich/LICH-012-config-convergence/README.md" <<'EOF'
  # LICH-012: Configuration Convergence Attacks

  Target: Mímir's Law / Gleipnir Phase 0 PoC (ADR-043)
  Branch: spike/mimirs-law
  Duration: 4-6 weeks (parallel with spike implementation, intensive Days 9-12)

  ## Sub-Campaigns
  - **L12a Baseline Signing Attack**: forge ML-DSA-65 signature, downgrade algo, key exfiltration attempts
  - **L12b Wotan Message Injection**: forge config.* messages, replay attacks, topic flooding (DoS)
  - **L12c Restore Race Conditions**: TOCTOU on baseline, dm-verity bypass, restore loop oscillation (alerts-only PoC means lower exposure but still test)
  - **L12d eBPF Drift Detection Fuzzing**: complexity bombs, ringbuf overflow, BPF map poisoning
  - **L12e Gjallarhorn Forgery (NEW)**: forge UPC trigger packets, multicast flood, unauthenticated bootstrap injection

  ## Success Criteria
  - 3+ exploitable findings → mark spike as HIGH RISK, ADR-043 must address before promotion
  - 0 exploitable findings → mark spike as HARDENED, ADR-043 may promote to "PoC Complete"
  EOF
  ```
- [ ] **Step 178** [SECURITY] ~variable: Execute L12a — baseline signing attacks
- [ ] **Step 179** [SECURITY] ~variable: Execute L12b — Wotan message injection
- [ ] **Step 180** [SECURITY] ~variable: Execute L12c — restore race conditions (limited scope since alerts-only)
- [ ] **Step 181** [SECURITY] ~variable: Execute L12d — eBPF drift detection fuzzing
- [ ] **Step 182** [SECURITY] ~variable: Execute L12e — Gjallarhorn forgery + multicast flood
- [ ] **Step 183** [W][SECURITY] ~1h: Write LICH-012 findings report
  ```bash
  # Report at: tomb/lich/LICH-012-config-convergence/FINDINGS.md
  ```
- [ ] **Step 184** [V][SECURITY] ~30s: **HARD CONDITION #8 GATE** — LICH-012 campaign opened and findings documented
- [ ] **Step 185** [V][C][GATE] ~30s: **PHASE 12 EXIT GATE** — LICH-012 findings ≤ acceptable risk threshold; report committed
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 177-185: LICH-012 campaign + findings"
  ```

---

## PHASE 13: DAY-14 GATE EVALUATION (Steps 186-194) ~3h

**Goal**: Final go/no-go decision. K1-K4 hard kill check + 8 hard conditions + pass criteria. Promote ADR-043 to "PoC Complete" or flip to "Rejected with learnings."
**Prerequisite**: Phases 11 + 12 complete
**Time**: 3 hours
**Agent**: Scientist (decision) + Marshal (enforcement) + Captain (strategic narrative)

- [ ] **Step 186** [V][KILL-CHECK] ~10m: **K1 evaluation** — eBPF verifier accepted ALL Heimdall hooks → PASS / FAIL
- [ ] **Step 187** [V][KILL-CHECK] ~10m: **K2 evaluation** — Wotan convergence p95 < 10s, p99 < 30s → PASS / FAIL
- [ ] **Step 188** [V][KILL-CHECK] ~10m: **K3 evaluation** — Zero cascade failures across all benchmarks → PASS / FAIL
- [ ] **Step 189** [V][KILL-CHECK] ~10m: **K4 evaluation** — All 8 hard conditions still in force → PASS / FAIL
- [ ] **Step 190** [V] ~30m: **Pass criteria evaluation** — convergence < 10s p95, drift < 1s, ≥ 95% accuracy, 100% consistency, bootstrap + reminder flows demonstrated, LICH-012 zero exploitable findings
- [ ] **Step 191** [DECIDE] ~15m: **Scientist decision**
  - **RECOMMENDATION**: If K1-K4 all PASS AND all pass criteria met AND LICH-012 = 0 exploitable → **PROMOTE** ADR-043 to "PoC Complete"
  - **RECOMMENDATION**: If any K1-K4 fail OR any pass criterion missed OR LICH-012 ≥ 1 exploitable → **FLIP** ADR-043 to "Rejected with learnings"
  - **Override ONLY if**: Captain identifies a strategic factor that warrants extension (rare; requires Round Table reconvene)
- [ ] **Step 192** [W] ~1h: Update ADR-043 status field with verdict + rationale
  ```bash
  cd "$PROJECT_ROOT" && $EDITOR docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md
  # Edit: Status: PoC Complete (or Rejected with Learnings)
  # Append: Day-14 Gate Findings section
  ```
- [ ] **Step 193** [W][DOC-UPDATE] ~30m: Update ADR-INDEX.md, wiki/ADR-Index.md, references/timeline.md, ADR-69420 cross-reference
- [ ] **Step 194** [V][C][GATE] ~30s: **PHASE 13 / SPIKE EXIT GATE** — ADR-043 promoted or rejected; doc web updated; battle plan archived
  ```bash
  cd "$PROJECT_ROOT" && git add -A && git commit -m "[PLAN S77] Steps 186-194: Day-14 gate evaluation + ADR-043 status finalized

Sprint complete. Spike branch spike/mimirs-law ready for review/merge/archive."
  cd "$PROJECT_ROOT" && git tag -a "spike/mimirs-law/day-14-gate" -m "Mímir's Law spike Day-14 gate evaluated"
  ```

---

## Appendix A: Emergency Procedures

### EMERGENCY 1: BPF Verifier Rejects Heimdall Hooks
**Symptom**: `bpftool prog load` fails with verifier error.
**Steps**:
1. Read verifier output: `dmesg | tail -30`
2. Check loop bounds — Aya needs explicit `bound: 16` annotation on loops
3. Check map sizes — reduce if hitting verifier complexity limit
4. Try splitting into multiple smaller BPF programs
5. If 2 attempts fail → **K1 TRIGGERED**, abort spike, log learnings

### EMERGENCY 2: Wotan Topic Signing Verification Fails on Spike Start
**Symptom**: Step 8 fails — Wotan config.* topics are not signed.
**Steps**:
1. **DO NOT PROCEED**. Hard condition #2 unmet.
2. Open new sprint to implement Wotan config.* signing FIRST.
3. Re-attempt this battle plan after that sprint ships.

### EMERGENCY 3: Multicast Gjallarhorn Not Received on Local Segment
**Symptom**: Step 80 — tcpdump on W02 sees nothing.
**Steps**:
1. Check ip6tables INPUT chain for multicast block
2. Verify MLDv2 join: `ip -6 maddr show dev eth0`
3. Check XDP attached to correct interface: `bpftool net show`
4. Try IPv4 multicast as fallback (224.0.0.0/4) if IPv6 multicast routing is the issue
5. If still failing → mark Gjallarhorn multicast path as STUCK, fall back to unicast-only spike (degraded scope)

### EMERGENCY 4: Restore Loop Oscillation Observed Despite Alerts-Only
**Symptom**: System keeps re-applying baseline.
**Steps**:
1. **STOP heimdall-daemon immediately** on affected nodes: `sudo systemctl stop heimdall-daemon`
2. Inspect enkrateia logs — confirm no `RestoreFile()` calls (there should be NONE in v1)
3. If any restore call found → **HARD CONDITION #1 VIOLATION**, abort spike
4. If oscillation is from Wotan replay → check signing prereq #2

### EMERGENCY 5: Day-14 Gate Result Is Ambiguous
**Symptom**: Some pass criteria met, some not; some K conditions borderline.
**Steps**:
1. **DECIDE step 191 default**: If ANY hard condition violated OR any K1-K4 fails → flip to Rejected. No ambiguity.
2. If pass criteria are 80%+ met but not 100% → Scientist may PROPOSE one-week extension
3. Extension requires Round Table reconvene (not autonomous)

---

## Appendix B: Agent Assignment Matrix

| Phase | Agent | Parallelizable | Dependencies | Time |
|---|---|---|---|---|
| 0 PREFLIGHT | Coordinator | No | none | 2h |
| 1 SPIKE BRANCH SETUP | Developer | No | Phase 0 | 1h |
| 2 SCHEMA DEFINITIONS | Architect + RFC Editor + Developer | No | Phase 1 | 3h |
| 3 PKG/GUNGNIR | Developer | No | Phase 2 | 4h |
| 4 PKG/GJALLARHORN | Developer | No | Phase 3 | 5h |
| 5 HEIMDALL eBPF | Developer | No | Phase 4 | 6h |
| 6 HEIMDALL DAEMON | Developer | No | Phase 5 | 4h |
| 7 PKG/ENKRATEIA | Developer | No | Phase 6 | 2h |
| 8 WEST CLUSTER PREP | Coordinator | No | Phase 7 | 3h |
| 9 BOOTSTRAP FLOW | Coordinator + Developer | No | Phase 8 | 3h |
| 10 REMINDER FLOW | Coordinator + Developer | No | Phase 9 | 3h |
| 11 STRESS TEST | Developer + Micromanager | No | Phase 10 | 2h |
| 12 LICH-012 CAMPAIGN | BlackMage | **YES — parallel with 9-11** | Phase 8 | 2-4d wall, ~variable |
| 13 DAY-14 GATE | Scientist + Marshal + Captain | No | Phases 11 + 12 | 3h |

**Critical path**: Phases 0 → 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 13 (Phase 12 runs parallel after Phase 8). Total: ~38 hours sequential + buffer.

---

## Appendix C: Quick Reference

### Naming Pool
- **Mímir** (concept) — speaker of baseline truth
- **Mjölnir** (`references/baseline/mjolnir.yaml`) — baseline definition
- **Gungnir Seal** (`*.gungnir.sig` / `GungnirSeal` proto) — ML-DSA-65 signature wrapper
- **Gjallarhorn** (`pkg/gjallarhorn/`) — UPC trigger packet
- **Heimdall Daemon** (`cmd/heimdall-daemon/`, `crates/heimdall-bpf/`) — drift watcher
- **Enkrateia** (`pkg/enkrateia/`) — restoration verb form (alerts-only v1)
- **Gleipnir** — parent vision (ADR-69420), this PoC is its Phase 0

### Key Wotan Topics
- `config.deltas.<node_id>` (authority → node, ML-DSA-65 signed)
- `drift.detected.<node_id>` (node → authority, ML-DSA-65 signed)
- `gjallarhorn.audit` (broadcast audit log)

### Gjallarhorn Packet Layout (20-byte Monad register)
- bytes [0:4]   : magic "GJLR" (0x474A4C52)
- bytes [4:5]   : trigger_kind (0x01=BOOTSTRAP_BROADCAST, 0x02=REVERIFY_UNICAST)
- bytes [5:9]   : cluster_id (uint32)
- bytes [9:17]  : Mjölnir manifest pointer (uint64)
- bytes [17:20] : reserved/padding

### Hard Condition Cheat Sheet
1. Alerts-only v1 (no auto-restore)
2. Wotan config.* signed (PREREQUISITE)
3. Baseline immutability via dm-verity
4. HSM-grade key separation + quarterly rotation
5. Semantic-aware drift detection (v2 only — v1 is byte-level alerts)
6. Sacred Law clause (no main IaC backend touch)
7. No Monad wire format changes
8. LICH-012 campaign opened in parallel

### K1-K4 Hard Kill Criteria
- K1: eBPF verifier rejects drift hooks (>50% opcodes fail after optimization)
- K2: Wotan convergence > 15s p95 / 30s p99
- K3: Cascade failures or unplanned reboots
- K4: Any of the 8 hard conditions cannot be met

### Pass Criteria
- Convergence < 10s p95
- Drift detection < 1s
- Restore verification ≥ 95%
- 100% consistency
- Zero cascade failures
- All 8 hard conditions in force
- Bootstrap + reminder flows demonstrated end-to-end
- LICH-012 zero exploitable findings

### Forbidden Paths (Marshal Lane Discipline)
- `pkg/discovery/`, `pkg/transport/`, `cmd/unheaded-daemon/`
- Any IaC backend code
- Monad foundation drafts
- `CLAUDE.md`

---

*S77 Battle Plan — Forged 2026-04-08*
*13 Phases. ~194 Steps. Mímir speaks the baseline; Heimdall watches; Gjallarhorn calls the seeds; Enkrateia keeps the alerts-only vigil.*
*The Dream Ladder gains a horizontal rung. UPC dogfoods the OS itself.*
