# S72 AGE 2 NO-BARE-METAL SPRINT BATTLE PLAN — 13 Phases, 320+ Steps

**Date**: 2026-02-27
**Sprint**: S72 — Age 2 Unblocked Backlog: Protocol Specs → Auth → SBOM → CI/CD → Bare Metal Prep
**Prerequisite**: S70/S71 complete (UI fixes, logs dashboard, Marshal skill). Git clean after Marshal commit.
**Target**: Ship protocol spec drafts -05, harden auth to MVP, automate SBOM in CI, add Jenkinsfiles + GHA hardening, prep bare metal boot sequence — all without touching physical hardware.
**Estimated Duration**: 16-24 hours across 4-6 sessions
**Agent Strategy**: Phases 1-4 sequential (protocol specs). Phases 5-7 parallelizable (auth). Phases 8-9 parallelizable (SBOM+CI). Phase 10 sequential (Jenkins). Phases 11-12 parallelizable (bare metal prep). Phase 13 verification.
**Commit Cadence**: Every 5 steps (max(3, min(5, 320/20)) = 5)
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. Log STUCK marker. Commit before skip.

---

## LEGEND

- `[B]` = Bash command (run directly)
- `[V]` = Verification step (MUST pass before proceeding)
- `[D]` = Debug step (only if prior step fails)
- `[W]` = Write/create file
- `[R]` = Read/inspect file
- `[S]` = Sudo required
- `[P]` = Parallelizable with other marked steps
- `[C]` = Commit checkpoint
- `[STUCK]` = Step skipped via Skip Protocol
- `[BLOCKED]` = Blocked by upstream STUCK

---

## PRIORITY MAP (Muck's Orders)

```
P0: Protocol Specs — Monad/Sophia/Wotan RFC work        (Phases 1-4)
P1: Auth Scaffolding — pkg/auth/ JWT+mTLS wiring        (Phases 5-7)
P2: SBOM Generation — compliance P0                      (Phase 8)
P3: CI/CD Pipeline — GHA + Jenkinsfiles                  (Phases 9-10)
P4: Bare Metal Validation — prep for tonight's boot      (Phases 11-12)
P5: Final Verification Gate                              (Phase 13)
```

---

# ═══════════════════════════════════════════════════════════
# PART 1: PROTOCOL SPECS — Monad/Sophia/Wotan (Phases 1-4)
# ═══════════════════════════════════════════════════════════

## PHASE 0: ENVIRONMENT & DEPENDENCY VERIFICATION (Steps 1-15)

**Goal**: Verify all tools needed for this sprint exist and work.
**Prerequisite**: Clean git state after Marshal commit.
**Time**: ~10 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: Verify Go toolchain
  ```bash
  go version | grep -q '1.24' && echo "GO OK" || echo "GO MISSING"
  ```

- [ ] **Step 2** [B] ~30s: Verify Rust toolchain
  ```bash
  rustc --version && cargo --version || echo "RUST MISSING"
  ```

- [ ] **Step 3** [B] ~30s: Verify protoc compiler
  ```bash
  protoc --version 2>/dev/null || echo "PROTOC MISSING — install protobuf-compiler"
  ```

- [ ] **Step 4** [B] ~30s: Verify golangci-lint
  ```bash
  golangci-lint --version 2>/dev/null || echo "LINT MISSING — go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
  ```

- [ ] **Step 5** [B] ~30s: Verify gosec
  ```bash
  gosec --version 2>/dev/null || echo "GOSEC MISSING — go install github.com/securego/gosec/v2/cmd/gosec@latest"
  ```

- [ ] **Step 6** [C] ~30s: **COMMIT CHECKPOINT** — Environment verified
  ```bash
  echo "Environment verification complete" && git status --short
  ```

- [ ] **Step 7** [B] ~30s: Verify syft (SBOM generator)
  ```bash
  syft version 2>/dev/null || echo "SYFT MISSING — curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin"
  ```

- [ ] **Step 8** [B] ~30s: Check go build passes
  ```bash
  go build ./... 2>&1 | tail -5
  ```

- [ ] **Step 9** [V] ~30s: Go build must succeed
  - If pass → Step 10
  - If fail → Step 9a [D]: Run `go mod tidy && go build ./... 2>&1 | head -30`

- [ ] **Step 10** [B] ~1m: Run existing tests (smoke check)
  ```bash
  go test -count=1 -timeout 60s ./pkg/auth/... ./pkg/transport/... 2>&1 | tail -10
  ```

- [ ] **Step 11** [V] ~30s: Tests must pass (or expected failures documented)
  - If pass → Step 12
  - If fail → Step 11a [D]: Check specific failure: `go test -v -run TestFailing ./pkg/... 2>&1 | tail -30`

- [ ] **Step 12** [B] ~30s: Check current protocol draft versions
  ```bash
  ls -la docs/protocol/draft-bellis-unheaded-* | grep -v references
  ```

- [ ] **Step 13** [R] ~1m: Read protocol patches pending application
  ```bash
  wc -l docs/protocol/patches/*.md
  ```

- [ ] **Step 14** [B] ~30s: Verify current draft line counts
  ```bash
  wc -l docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md docs/protocol/draft-bellis-unheaded-wotan-memory-01.md
  ```

- [ ] **Step 15** [V] ~30s: **PHASE 0 EXIT GATE** — All tools present, codebase builds, protocol drafts located
  - Verify: Go 1.24 ✓, build passes ✓, tests pass ✓, drafts found ✓
  - If any tool MISSING → install before proceeding. DO NOT skip this gate.

---

## PHASE 1: MONAD PROTOCOL FOUNDATION — DRAFT 05 (Steps 16-60)

**Goal**: Produce `draft-bellis-unheaded-protocol-foundation-05.md` incorporating all pending patches, new sections for Shield eBPF pipeline, and updated IANA considerations.
**Prerequisite**: Phase 0 passed. draft-04 + patches read.
**Time**: ~3 hours
**Agent**: Coordinator (RFC Editor skill required)

### 1A: Patch Integration

- [ ] **Step 16** [R] ~2m: Read draft-04 in full — understand current state
  ```bash
  cat docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md | wc -l
  ```

- [ ] **Step 17** [R] ~2m: Read monad-foundation-draft-04-patches.md
  ```bash
  cat docs/protocol/patches/monad-foundation-draft-04-patches.md
  ```

- [ ] **Step 18** [W] ~5m: Create draft-05 from draft-04 base
  ```bash
  cp docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md
  ```

- [ ] **Step 19** [W] ~10m: Update YAML frontmatter — bump version, date, keywords
  - Change `docname: draft-bellis-unheaded-protocol-foundation-05`
  - Change `date: 2026-02-27`
  - Add keywords: `shield-ebpf`, `anamnesis`, `kingdom-mode`

- [ ] **Step 20** [W] ~15m: Apply all patches from monad-foundation-draft-04-patches.md
  - Integrate corrections, clarifications, wire format updates
  - Preserve section numbering cascade
  - Update all cross-references affected by changes

- [ ] **Step 21** [C] ~30s: **COMMIT CHECKPOINT** — Draft-05 base + patches applied
  ```bash
  git add docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md && git commit -m "[PLAN S72] Steps 16-21: Monad draft-05 created with patches applied"
  ```

### 1B: New Sections — Shield eBPF Pipeline

- [ ] **Step 22** [W] ~20m: Add Section — "Shield: eBPF Security Pipeline"
  - XDP ingress program architecture (shield-ebpf)
  - TC egress program architecture
  - BPF map pinning contract (`/sys/fs/bpf/unheaded/`)
  - Map types: HashMap (flow tracking), RingBuf (events), Array (config)
  - Verifier compliance requirements (loop bounds, stack limits)
  - Interaction with Monad HbH option processing

- [ ] **Step 23** [W] ~15m: Add Section — "Anamnesis: Event Capture Architecture"
  - RingEntry 64-byte format specification
  - EVE JSON → RingEntry transformation rules
  - Wotan topic `anamnesis.*` routing
  - Suricata integration boundary (GPL isolation)
  - Event correlation with Monad flow labels

- [ ] **Step 24** [W] ~10m: Add Section — "Kingdom Mode: Operational States"
  - Normal Mode, Degraded Mode, Emergency Mode
  - State transition triggers (health check failures, BPF verifier rejections)
  - Wotan system.state topic contract
  - Dashboard state indicators

- [ ] **Step 25** [W] ~10m: Update IANA Considerations section
  - IPv6 Hop-by-Hop Option Type allocation request
  - Monad Next Header value (experimental 0xFE usage)
  - Flow Label usage guidelines per RFC 6437
  - Extension header type registry entry
  - Update references to current RFC numbering

- [ ] **Step 26** [C] ~30s: **COMMIT CHECKPOINT** — Shield + Anamnesis + Kingdom Mode sections added
  ```bash
  git add docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md && git commit -m "[PLAN S72] Steps 22-26: Monad draft-05 new sections — Shield, Anamnesis, Kingdom Mode"
  ```

### 1C: Security Considerations Update

- [ ] **Step 27** [W] ~15m: Expand Security Considerations
  - PQC authentication reference (cross-ref to PQC draft)
  - Amplification attack mitigations (per pkg/protocol/amplification/)
  - DoS protection mechanisms (per pkg/protocol/dos/)
  - BPF verifier as security boundary
  - Flow label entropy requirements
  - Address reclamation attack surface

- [ ] **Step 28** [W] ~10m: Update References section
  - Add RFC 9000 (QUIC), RFC 9114 (HTTP/3)
  - Add BPF ISA reference documents
  - Cross-reference Sophia and Wotan companion specs
  - Update all normative/informative reference categorization

- [ ] **Step 29** [W] ~5m: Update Appendices
  - Appendix A: Wire format diagrams (update with Shield fields)
  - Appendix B: BPF program compatibility matrix
  - Appendix C: Error code registry (cross-ref error-registry.md)

- [ ] **Step 30** [V] ~2m: Verify draft-05 structure integrity
  ```bash
  grep -c "^#" docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md
  ```
  - Section count should be >= draft-04 section count + 3 new sections
  - If missing sections → re-check Steps 22-29

- [ ] **Step 31** [B] ~1m: Line count comparison
  ```bash
  wc -l docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md
  ```
  - Draft-05 should be 200-400 lines longer than draft-04

- [ ] **Step 32** [C] ~30s: **COMMIT CHECKPOINT** — Monad draft-05 security + references + appendices
  ```bash
  git add docs/protocol/ && git commit -m "[PLAN S72] Steps 27-32: Monad draft-05 complete — security, references, appendices updated"
  ```

### 1D: Implementation Alignment Verification

- [ ] **Step 33** [R] ~2m: Compare draft-05 wire format with cmd/protocol-api/monad.go
  ```bash
  grep -n "type Monad" cmd/protocol-api/monad.go
  ```

- [ ] **Step 34** [R] ~2m: Compare draft-05 BPF maps with ebpf/ program definitions
  ```bash
  grep -rn "map_type\|MapSpec" ebpf/ | head -20
  ```

- [ ] **Step 35** [W] ~10m: Create ALIGNMENT_NOTES_DRAFT05.md documenting any spec-implementation gaps
  ```bash
  # File: docs/protocol/ALIGNMENT_NOTES_DRAFT05.md
  # Lists: spec section → implementation file → status (aligned/gap/TODO)
  ```

- [ ] **Step 36** [V] ~1m: **PHASE 1 EXIT GATE** — Monad draft-05 exists, is longer than draft-04, has new sections, alignment notes written
  - `test -f docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md` → pass
  - Draft-05 line count > draft-04 line count → pass
  - ALIGNMENT_NOTES_DRAFT05.md exists → pass
  - If any fail → DO NOT PROCEED. Debug within this phase.

---

## PHASE 2: SOPHIA DICTIONARY — DRAFT 02 (Steps 37-60)

**Goal**: Produce `draft-bellis-unheaded-sophia-dictionary-02.md` with patches applied, BPF map schema updates, and new dictionary entry types.
**Prerequisite**: Phase 1 complete.
**Time**: ~2 hours
**Agent**: Agent [P] (can parallel with Phase 3 if coordinator splits)

### 2A: Patch Integration

- [ ] **Step 37** [R] ~2m: Read sophia-dictionary-draft-01.md
  ```bash
  wc -l docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md
  ```

- [ ] **Step 38** [R] ~2m: Read sophia patches
  ```bash
  cat docs/protocol/patches/sophia-dictionary-draft-01-patches.md | head -50
  ```

- [ ] **Step 39** [W] ~5m: Create draft-02 from draft-01 base
  ```bash
  cp docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md
  ```

- [ ] **Step 40** [W] ~10m: Update YAML frontmatter
  - Bump: `docname: draft-bellis-unheaded-sophia-dictionary-02`
  - Date: `2026-02-27`
  - Add keywords from S67-S71 work

- [ ] **Step 41** [W] ~15m: Apply all patches from sophia-dictionary-draft-01-patches.md

- [ ] **Step 42** [C] ~30s: **COMMIT CHECKPOINT** — Sophia draft-02 base + patches
  ```bash
  git add docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md && git commit -m "[PLAN S72] Steps 37-42: Sophia draft-02 created with patches applied"
  ```

### 2B: New Content — BPF Map Schema & Entry Types

- [ ] **Step 43** [W] ~15m: Add Section — "BPF Map Schema Definitions"
  - HashMap schema: key format (flow_label + src_addr), value format (Sophia entry)
  - Array schema: config indices, lookup tables
  - RingBuf schema: event entry format (64-byte Anamnesis entries)
  - Map naming convention: `/sys/fs/bpf/unheaded/sophia_*`
  - Versioned schema compatibility (forward/backward)

- [ ] **Step 44** [W] ~15m: Add Section — "Extended Dictionary Entry Types"
  - Routing entries (BGP, IS-IS, OSPF, MPLS metadata)
  - Firewall entries (OPNsense/IPFire rule references)
  - Observability entries (Prometheus metric labels)
  - IDS entries (Suricata SID references)
  - Health entries (service status, degraded mode indicators)

- [ ] **Step 45** [W] ~10m: Add Section — "Dictionary Synchronization Protocol"
  - sophiasync package contract (pkg/protocol/sophiasync/)
  - BPF map → userspace sync interval
  - Conflict resolution (last-writer-wins with CRC verification)
  - Bulk sync for dictionary rebuild after crash recovery

- [ ] **Step 46** [W] ~10m: Update Security Considerations
  - Dictionary poisoning attack vectors
  - BPF map access control (pin permissions)
  - Dictionary entry size limits (prevent memory exhaustion)
  - Cross-reference Monad draft-05 Security Considerations

- [ ] **Step 47** [C] ~30s: **COMMIT CHECKPOINT** — Sophia draft-02 new sections
  ```bash
  git add docs/protocol/ && git commit -m "[PLAN S72] Steps 43-47: Sophia draft-02 new sections — BPF maps, entry types, sync protocol"
  ```

### 2C: Implementation Alignment

- [ ] **Step 48** [R] ~2m: Compare with pkg/protocol/bpfschema/
  ```bash
  ls -la pkg/protocol/bpfschema/ 2>/dev/null || echo "NO BPFSCHEMA PKG"
  ```

- [ ] **Step 49** [R] ~2m: Compare with cmd/protocol-api/sophia.go
  ```bash
  grep -n "type\|func" cmd/protocol-api/sophia.go | head -20
  ```

- [ ] **Step 50** [W] ~5m: Update ALIGNMENT_NOTES_DRAFT05.md with Sophia alignment
  - Append Sophia section mapping spec → implementation

- [ ] **Step 51** [V] ~1m: **PHASE 2 EXIT GATE** — Sophia draft-02 exists, has new sections
  - `test -f docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md` → pass
  - Line count > draft-01 → pass
  - If fail → DO NOT PROCEED.

---

## PHASE 3: WOTAN MEMORY MODEL — DRAFT 02 (Steps 52-75)

**Goal**: Produce `draft-bellis-unheaded-wotan-memory-02.md` with patches, ring buffer formalization, and gRPC streaming contract.
**Prerequisite**: Phase 0 complete. Can parallel with Phase 2 if agent-split.
**Time**: ~2 hours
**Agent**: Agent [P]

### 3A: Patch Integration

- [ ] **Step 52** [R] ~2m: Read wotan-memory-draft-01.md
  ```bash
  wc -l docs/protocol/draft-bellis-unheaded-wotan-memory-01.md
  ```

- [ ] **Step 53** [R] ~2m: Read wotan patches
  ```bash
  cat docs/protocol/patches/wotan-memory-draft-01-patches.md | head -50
  ```

- [ ] **Step 54** [W] ~5m: Create draft-02 from draft-01
  ```bash
  cp docs/protocol/draft-bellis-unheaded-wotan-memory-01.md docs/protocol/draft-bellis-unheaded-wotan-memory-02.md
  ```

- [ ] **Step 55** [W] ~10m: Update frontmatter + apply all patches

- [ ] **Step 56** [C] ~30s: **COMMIT CHECKPOINT** — Wotan draft-02 base
  ```bash
  git add docs/protocol/draft-bellis-unheaded-wotan-memory-02.md && git commit -m "[PLAN S72] Steps 52-56: Wotan draft-02 created with patches applied"
  ```

### 3B: New Content — Ring Buffer & gRPC Streaming

- [ ] **Step 57** [W] ~15m: Add Section — "Ring Buffer Formal Specification"
  - Ring buffer capacity algorithm (10,000 entry default, configurable)
  - Entry format: 64-byte fixed-size entries
  - Head/tail pointer semantics
  - Overflow policy: oldest-first eviction
  - Lock-free read path (BPF ringbuf compatibility)
  - Memory layout alignment requirements

- [ ] **Step 58** [W] ~15m: Add Section — "gRPC Streaming Contract"
  - Topic subscription protocol (system.*, logs.*, anamnesis.*, user.*)
  - Stream lifecycle: connect → subscribe → receive → unsubscribe → disconnect
  - Backpressure handling: per-client buffer limits
  - Reconnection protocol: client-side exponential backoff
  - Server-side: TTL-based subscription reaping
  - Cross-reference with proto/unheaded/v1/protocol.proto

- [ ] **Step 59** [W] ~10m: Add Section — "Triple-Role Architecture"
  - Role 1: Ring Buffer (per-service log aggregation)
  - Role 2: Event Bus (pub/sub message routing)
  - Role 3: Protocol RAM (Monad state machine backing store)
  - Role isolation: same process, separate goroutines, shared map
  - Failure mode: ring buffer overflow does NOT block event bus

- [ ] **Step 60** [W] ~10m: Add Section — "Wotan Reliability Guarantees"
  - PublishWithAck: at-least-once delivery
  - IdempotencyCache: 24h TTL duplicate detection
  - OrderedPublisher: per-destination FIFO
  - Dead letter queue: exhausted retry handling
  - Cross-reference pkg/wotan-client/ implementation

- [ ] **Step 61** [C] ~30s: **COMMIT CHECKPOINT** — Wotan draft-02 new sections
  ```bash
  git add docs/protocol/ && git commit -m "[PLAN S72] Steps 57-61: Wotan draft-02 — ring buffer, gRPC streaming, triple-role, reliability"
  ```

### 3C: Security & Alignment

- [ ] **Step 62** [W] ~10m: Update Security Considerations
  - Topic injection attacks (unauthorized publish)
  - Ring buffer memory exhaustion
  - gRPC stream hijacking mitigations (mTLS required)
  - Dead letter queue information leakage

- [ ] **Step 63** [R] ~2m: Compare with services/wotan/proto/topic.proto
  ```bash
  cat services/wotan/proto/topic.proto
  ```

- [ ] **Step 64** [R] ~2m: Compare with pkg/wotan-client/ implementation
  ```bash
  wc -l pkg/wotan-client/*.go
  ```

- [ ] **Step 65** [W] ~5m: Update ALIGNMENT_NOTES_DRAFT05.md with Wotan section

- [ ] **Step 66** [V] ~1m: **PHASE 3 EXIT GATE** — Wotan draft-02 exists, has new sections
  - `test -f docs/protocol/draft-bellis-unheaded-wotan-memory-02.md` → pass
  - Line count > draft-01 → pass
  - If fail → DO NOT PROCEED.

---

## PHASE 4: PROTOCOL CROSS-REFERENCE & OPENAPI UPDATE (Steps 67-85)

**Goal**: Update OpenAPI spec, cross-reference all three drafts, generate protocol summary document.
**Prerequisite**: Phases 1-3 complete.
**Time**: ~1.5 hours
**Agent**: Coordinator

- [ ] **Step 67** [R] ~2m: Read current openapi.yaml
  ```bash
  wc -l docs/protocol/openapi.yaml
  ```

- [ ] **Step 68** [W] ~20m: Update openapi.yaml with new endpoints/schemas
  - Add Shield eBPF status endpoints
  - Add Anamnesis event query endpoints
  - Add Kingdom Mode state transition endpoints
  - Update Sophia dictionary CRUD with new entry types
  - Update Wotan topic subscription schemas
  - Bump API version to match draft-05

- [ ] **Step 69** [C] ~30s: **COMMIT CHECKPOINT** — OpenAPI updated
  ```bash
  git add docs/protocol/openapi.yaml && git commit -m "[PLAN S72] Steps 67-69: OpenAPI spec updated for draft-05 alignment"
  ```

- [ ] **Step 70** [W] ~15m: Create PROTOCOL_STATUS_S72.md — cross-reference matrix
  ```markdown
  # Protocol Status — S72
  | Spec | Draft | Lines | New Sections | Patches Applied | Alignment |
  |------|-------|-------|-------------|-----------------|-----------|
  | Monad Foundation | 05 | XXXX | Shield, Anamnesis, Kingdom Mode | Yes | See notes |
  | Sophia Dictionary | 02 | XXXX | BPF Maps, Entry Types, Sync | Yes | See notes |
  | Wotan Memory | 02 | XXXX | Ring Buffer, gRPC, Triple-Role, Reliability | Yes | See notes |
  | PQC Auth | 00 | 1633 | (unchanged this sprint) | N/A | Pending |
  ```

- [ ] **Step 71** [W] ~10m: Update error-registry.md with any new error codes from drafts

- [ ] **Step 72** [W] ~10m: Update PROTOCOL_TECHNICAL_SUMMARY.md with S72 changes

- [ ] **Step 73** [W] ~5m: Update RFC_ALIGNMENT_REPORT.md
  - Add draft-05, sophia-02, wotan-02 entries
  - Note new RFC references added

- [ ] **Step 74** [C] ~30s: **COMMIT CHECKPOINT** — Protocol cross-references updated
  ```bash
  git add docs/protocol/ && git commit -m "[PLAN S72] Steps 70-74: Protocol cross-references, status matrix, alignment report updated"
  ```

- [ ] **Step 75** [W] ~10m: Update proto/unheaded/v1/protocol.proto if new message types needed
  ```bash
  # Only if new protobuf messages required for Shield/Anamnesis/Kingdom Mode endpoints
  # If no changes needed, note "proto unchanged" and proceed
  ```

- [ ] **Step 76** [B] ~2m: If proto changed, regenerate Go bindings
  ```bash
  protoc --go_out=. --go-grpc_out=. proto/unheaded/v1/protocol.proto 2>&1 || echo "PROTOC SKIPPED — no changes or protoc unavailable"
  ```

- [ ] **Step 77** [B] ~1m: Verify build still passes after all protocol changes
  ```bash
  go build ./... 2>&1 | tail -5
  ```

- [ ] **Step 78** [V] ~30s: Build passes
  - If fail → Step 78a [D]: Check proto generation errors, fix imports

- [ ] **Step 79** [C] ~30s: **COMMIT CHECKPOINT** — Proto + build verified
  ```bash
  git add proto/ && git commit -m "[PLAN S72] Steps 75-79: Proto definitions updated, build verified"
  ```

- [ ] **Step 80** [V] ~1m: **PHASE 4 EXIT GATE** — All three drafts bumped, OpenAPI updated, alignment notes complete, build passes
  - 3 new draft files exist → pass
  - OpenAPI updated → pass
  - PROTOCOL_STATUS_S72.md exists → pass
  - `go build ./...` passes → pass
  - If any fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PART 2: AUTH SCAFFOLDING — JWT + mTLS Wiring (Phases 5-7)
# ═══════════════════════════════════════════════════════════

## PHASE 5: AUTH PACKAGE HARDENING (Steps 81-120)

**Goal**: Harden pkg/auth/ from stubs to MVP — real JWT validation, middleware on all routes, RBAC enforcement.
**Prerequisite**: Phase 4 complete. pkg/auth/ exists with 3,202 LOC.
**Time**: ~3 hours
**Agent**: Developer (TDD required)

### 5A: JWT Validation Hardening

- [ ] **Step 81** [R] ~2m: Read current pkg/auth/jwt.go (245 lines)
  ```bash
  cat pkg/auth/jwt.go
  ```

- [ ] **Step 82** [R] ~2m: Read current jwt_test.go (449 lines)
  ```bash
  cat pkg/auth/jwt_test.go
  ```

- [ ] **Step 83** [W] ~5m: Write NEW test cases for JWT hardening (TDD — red first)
  - Test: Reject expired tokens
  - Test: Reject tokens with wrong signing algorithm (alg confusion)
  - Test: Reject tokens with empty subject
  - Test: Reject tokens signed with wrong key
  - Test: Accept valid tokens with all required claims
  - Test: Extract roles from JWT claims into Identity
  - Test: Handle clock skew (5 second leeway)

- [ ] **Step 84** [B] ~1m: Run new tests — expect RED (failures)
  ```bash
  go test -v -run "TestJWT" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 85** [W] ~15m: Update jwt.go — implement validation hardening
  - Add algorithm whitelist (HS256, RS256 only)
  - Add claim validation (exp, iat, sub required)
  - Add clock skew tolerance
  - Add role extraction from custom claims
  - Defensive: reject token if any validation step fails

- [ ] **Step 86** [B] ~1m: Run tests — expect GREEN
  ```bash
  go test -v -run "TestJWT" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 87** [V] ~30s: All JWT tests pass
  - If fail → Step 87a [D]: `go test -v -run "TestJWT" ./pkg/auth/... 2>&1`

- [ ] **Step 88** [C] ~30s: **COMMIT CHECKPOINT** — JWT hardened
  ```bash
  git add pkg/auth/jwt.go pkg/auth/jwt_test.go && git commit -m "[PLAN S72] Steps 81-88: JWT validation hardened — alg whitelist, claim validation, clock skew"
  ```

### 5B: RBAC Enforcement Hardening

- [ ] **Step 89** [R] ~2m: Read current rbac.go (243 lines)
  ```bash
  cat pkg/auth/rbac.go
  ```

- [ ] **Step 90** [W] ~5m: Write NEW RBAC test cases (TDD — red)
  - Test: Admin can access all endpoints
  - Test: Observer can only GET, not POST/PUT/DELETE
  - Test: Service token has inter-service access
  - Test: Guest has read-only public endpoints
  - Test: Reject request with no role
  - Test: Role hierarchy enforcement (admin > operator > observer > guest)

- [ ] **Step 91** [B] ~1m: Run RBAC tests — expect RED
  ```bash
  go test -v -run "TestRBAC" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 92** [W] ~15m: Update rbac.go — implement role hierarchy and endpoint mapping
  - Define role hierarchy: admin > operator > observer > service > guest
  - Endpoint permission matrix: map HTTP method + path → minimum role
  - Middleware: extract role from Identity, check against matrix
  - Default deny: if no mapping found, reject

- [ ] **Step 93** [B] ~1m: Run RBAC tests — expect GREEN
  ```bash
  go test -v -run "TestRBAC" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 94** [V] ~30s: All RBAC tests pass
  - If fail → Step 94a [D]: Check role mapping, permission matrix

- [ ] **Step 95** [C] ~30s: **COMMIT CHECKPOINT** — RBAC hardened
  ```bash
  git add pkg/auth/rbac.go pkg/auth/rbac_test.go && git commit -m "[PLAN S72] Steps 89-95: RBAC hardened — role hierarchy, endpoint matrix, default deny"
  ```

### 5C: Service Token Hardening

- [ ] **Step 96** [R] ~1m: Read service_token.go (113 lines)
  ```bash
  cat pkg/auth/service_token.go
  ```

- [ ] **Step 97** [W] ~5m: Write NEW service token tests (TDD — red)
  - Test: Valid service token authenticates
  - Test: Expired service token rejected
  - Test: Service token cannot escalate to admin
  - Test: Service token identifies calling service by name
  - Test: Token rotation (new token valid, old token still valid during grace period)

- [ ] **Step 98** [W] ~10m: Update service_token.go — implement hardening
  - Add expiry checking
  - Add service identity extraction
  - Add grace period for token rotation (configurable, default 5 minutes)
  - Add service name validation against known services list

- [ ] **Step 99** [B] ~1m: Run service token tests — expect GREEN
  ```bash
  go test -v -run "TestServiceToken" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 100** [V] ~30s: All service token tests pass

- [ ] **Step 101** [C] ~30s: **COMMIT CHECKPOINT** — Service token hardened
  ```bash
  git add pkg/auth/service_token*.go && git commit -m "[PLAN S72] Steps 96-101: Service token hardened — expiry, identity, rotation grace"
  ```

### 5D: Auth Middleware Integration

- [ ] **Step 102** [R] ~2m: Read auth.go middleware (214 lines)
  ```bash
  cat pkg/auth/auth.go
  ```

- [ ] **Step 103** [W] ~5m: Write NEW middleware integration tests
  - Test: Request without auth header → 401
  - Test: Request with valid JWT → 200 + Identity in context
  - Test: Request with valid service token → 200 + service Identity
  - Test: Request with invalid auth → 401 + audit log entry
  - Test: Health/ready endpoints bypass auth (allowlist)
  - Test: Metrics endpoint requires at least observer role

- [ ] **Step 104** [W] ~15m: Update auth.go middleware
  - Implement endpoint allowlist: `/health`, `/ready`, `/metrics` (observer+)
  - Chain: parse header → identify auth type (Bearer JWT / Service-Token / API-Key) → validate → set Identity → next handler
  - Audit logging on every auth attempt (success AND failure)
  - Rate limiting per-IP on failed auth attempts (10/minute)

- [ ] **Step 105** [B] ~1m: Run all auth tests
  ```bash
  go test -v -count=1 -race ./pkg/auth/... 2>&1 | tail -30
  ```

- [ ] **Step 106** [V] ~30s: ALL auth tests pass with race detector
  - If fail → Step 106a [D]: Check for race conditions in audit logger

- [ ] **Step 107** [B] ~30s: Check auth test coverage
  ```bash
  go test -coverprofile=auth-coverage.out ./pkg/auth/... && go tool cover -func=auth-coverage.out | tail -1
  ```

- [ ] **Step 108** [V] ~30s: Auth coverage >= 80%
  - If below 80% → add more tests for uncovered paths

- [ ] **Step 109** [C] ~30s: **COMMIT CHECKPOINT** — Auth middleware complete
  ```bash
  git add pkg/auth/ && git commit -m "[PLAN S72] Steps 102-109: Auth middleware hardened — allowlist, chain, audit, rate limit"
  ```

- [ ] **Step 110** [V] ~1m: **PHASE 5 EXIT GATE** — Auth package hardened, all tests pass, coverage >= 80%
  - `go test -race ./pkg/auth/...` passes → pass
  - Coverage >= 80% → pass
  - If fail → DO NOT PROCEED.

---

## PHASE 6: mTLS WIRING TO ALL SERVICES (Steps 111-145)

**Goal**: Wire mTLS from pkg/transport/mtls/ into all 10 service binaries for inter-service auth.
**Prerequisite**: Phase 5 complete.
**Time**: ~2.5 hours
**Agent**: Developer

### 6A: Certificate Generation Tooling

- [ ] **Step 111** [R] ~1m: Read current mtls.go (373 lines)
  ```bash
  cat pkg/transport/mtls/mtls.go
  ```

- [ ] **Step 112** [W] ~5m: Write test — generate root CA + service certs programmatically
  - Test: GenerateCA() produces valid CA cert + key
  - Test: GenerateServiceCert(ca, "wotan") produces cert with CN=wotan
  - Test: Service cert validates against CA

- [ ] **Step 113** [W] ~15m: Implement cert generation in pkg/transport/mtls/certgen.go
  - GenerateCA() → CA cert + key (ECDSA P-256)
  - GenerateServiceCert(ca, serviceName) → service cert + key
  - SaveCerts(dir, name, cert, key) → write to filesystem
  - All certs: 1-year validity (configurable)

- [ ] **Step 114** [B] ~1m: Run mTLS tests
  ```bash
  go test -v -race ./pkg/transport/mtls/... 2>&1 | tail -20
  ```

- [ ] **Step 115** [V] ~30s: mTLS cert generation tests pass

- [ ] **Step 116** [C] ~30s: **COMMIT CHECKPOINT** — Cert generation
  ```bash
  git add pkg/transport/mtls/ && git commit -m "[PLAN S72] Steps 111-116: mTLS cert generation — CA + service certs, ECDSA P-256"
  ```

### 6B: Dev Certificate Bootstrap Script

- [ ] **Step 117** [W] ~10m: Update scripts/gen-dev-certs.sh
  - Generate root CA
  - Generate certs for all 10 services: wotan, timeguru, captain, architect, micromanager, monad, sophia, dashboard, kanban, wiki
  - Output to configs/certs/dev/
  - Print summary of generated certs

- [ ] **Step 118** [B] ~1m: Make script executable and test
  ```bash
  chmod +x scripts/gen-dev-certs.sh && bash scripts/gen-dev-certs.sh 2>&1 | tail -20
  ```

- [ ] **Step 119** [V] ~30s: Certs generated
  ```bash
  ls configs/certs/dev/ 2>/dev/null | wc -l
  ```
  - Should be 22 files (11 certs + 11 keys including CA)

- [ ] **Step 120** [C] ~30s: **COMMIT CHECKPOINT** — Dev cert script
  ```bash
  git add scripts/gen-dev-certs.sh && git commit -m "[PLAN S72] Steps 117-120: Dev cert bootstrap — all 10 services"
  ```

### 6C: Service Integration Pattern

- [ ] **Step 121** [W] ~5m: Create pkg/transport/mtls/integration.go — helper for service startup
  ```go
  // SetupServiceTLS(serviceName string, certsDir string) (*tls.Config, error)
  // Returns: TLS config with client auth required, service cert loaded, CA pool configured
  ```

- [ ] **Step 122** [W] ~5m: Write integration test
  - Test: Two services connect via mTLS, exchange gRPC messages
  - Test: Connection rejected without valid cert
  - Test: Connection rejected with expired cert

- [ ] **Step 123** [B] ~1m: Run integration test
  ```bash
  go test -v -run "TestMTLSIntegration" ./pkg/transport/mtls/... 2>&1 | tail -20
  ```

- [ ] **Step 124** [V] ~30s: mTLS integration tests pass

- [ ] **Step 125** [C] ~30s: **COMMIT CHECKPOINT** — mTLS integration helper
  ```bash
  git add pkg/transport/mtls/ && git commit -m "[PLAN S72] Steps 121-125: mTLS integration helper — SetupServiceTLS"
  ```

### 6D: Wire Into Service Binaries

- [ ] **Step 126** [W] ~5m: Wire mTLS into services/wotan/main.go (template pattern)
  - Add TLS config loading on startup
  - Add mTLS to gRPC server options
  - Add cert path CLI flags
  - Keep HTTP health endpoint on plaintext (for load balancers)

- [ ] **Step 127** [W] ~20m: Apply same pattern to remaining 9 services
  - services/timeguru, services/captain, services/architect, services/micromanager
  - cmd/protocol-api (monad/sophia/wotan handlers)
  - cmd/dashboard-backend
  - cmd/kanban (if applicable)
  - cmd/wiki-server

- [ ] **Step 128** [B] ~2m: Verify all services still build
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -10
  ```

- [ ] **Step 129** [V] ~30s: All services build with mTLS wiring
  - If fail → Step 129a [D]: Check import paths, missing TLS config fields

- [ ] **Step 130** [C] ~30s: **COMMIT CHECKPOINT** — mTLS wired into all services
  ```bash
  git add services/ cmd/ && git commit -m "[PLAN S72] Steps 126-130: mTLS wired into all 10 service binaries"
  ```

- [ ] **Step 131** [V] ~1m: **PHASE 6 EXIT GATE** — mTLS cert gen works, all services build with TLS, integration tests pass
  - `go build ./cmd/... ./services/...` passes → pass
  - `go test -race ./pkg/transport/mtls/...` passes → pass
  - If fail → DO NOT PROCEED.

---

## PHASE 7: AUTH WIRING TO API ROUTES (Steps 132-155)

**Goal**: Wire auth middleware into all HTTP/gRPC route handlers across services.
**Prerequisite**: Phases 5 and 6 complete.
**Time**: ~2 hours
**Agent**: Developer

- [ ] **Step 132** [W] ~10m: Create pkg/auth/middleware_grpc.go — gRPC unary interceptor
  - Extract auth from gRPC metadata
  - Validate JWT/service token
  - Set Identity in context
  - Return codes.Unauthenticated on failure

- [ ] **Step 133** [W] ~5m: Write gRPC interceptor tests
  - Test: Valid metadata → passes through
  - Test: Missing metadata → Unauthenticated
  - Test: Invalid token → Unauthenticated

- [ ] **Step 134** [B] ~1m: Run gRPC auth tests
  ```bash
  go test -v -run "TestGRPC" ./pkg/auth/... 2>&1 | tail -20
  ```

- [ ] **Step 135** [V] ~30s: gRPC auth tests pass

- [ ] **Step 136** [C] ~30s: **COMMIT CHECKPOINT** — gRPC auth interceptor
  ```bash
  git add pkg/auth/ && git commit -m "[PLAN S72] Steps 132-136: gRPC auth interceptor — metadata extraction, token validation"
  ```

- [ ] **Step 137** [W] ~15m: Wire HTTP auth middleware into all services
  - Wrap all gorilla/mux routers with auth.Middleware()
  - Allowlist: /health, /ready (no auth required)
  - Require observer+ for /metrics
  - Require operator+ for POST/PUT/DELETE
  - Require admin for /admin/* endpoints

- [ ] **Step 138** [W] ~10m: Wire gRPC auth interceptor into all gRPC servers
  - Add UnaryInterceptor to all grpc.NewServer() calls
  - Allowlist: grpc.health.v1.Health (no auth required)

- [ ] **Step 139** [B] ~2m: Build all services
  ```bash
  go build ./cmd/... ./services/... 2>&1 | tail -10
  ```

- [ ] **Step 140** [V] ~30s: All services build with auth wiring

- [ ] **Step 141** [B] ~2m: Run full test suite
  ```bash
  go test -race -count=1 -timeout 120s ./... 2>&1 | tail -30
  ```

- [ ] **Step 142** [V] ~30s: Full test suite passes
  - If fail → Step 142a [D]: Check which service tests broke, likely need test fixtures for auth

- [ ] **Step 143** [W] ~10m: Update integration tests with auth headers
  - All existing integration tests need Bearer token in request headers
  - Add helper: `testutil.AdminToken()`, `testutil.ObserverToken()`, `testutil.ServiceToken(name)`

- [ ] **Step 144** [B] ~2m: Run integration tests
  ```bash
  go test -v -run "TestIntegration" ./cmd/protocol-api/... 2>&1 | tail -30
  ```

- [ ] **Step 145** [V] ~30s: Integration tests pass with auth

- [ ] **Step 146** [C] ~30s: **COMMIT CHECKPOINT** — Auth wired everywhere
  ```bash
  git add . && git commit -m "[PLAN S72] Steps 137-146: Auth middleware wired into all HTTP + gRPC routes"
  ```

- [ ] **Step 147** [V] ~1m: **PHASE 7 EXIT GATE** — Auth on all routes, all tests pass, no unauthenticated endpoints (except health/ready)
  - `go build ./...` passes → pass
  - `go test -race ./...` passes → pass
  - `grep -rn "router.HandleFunc" services/ cmd/ | grep -v "health\|ready" | wc -l` → all should have auth middleware upstream
  - If fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PART 3: SBOM + CI/CD (Phases 8-10)
# ═══════════════════════════════════════════════════════════

## PHASE 8: SBOM GENERATION & COMPLIANCE AUTOMATION (Steps 148-175)

**Goal**: Automate SBOM generation, integrate into CI, update sbom-results/, ensure license compliance gate.
**Prerequisite**: Phase 7 complete.
**Time**: ~1.5 hours
**Agent**: Agent [P] (can parallel with Phase 9)

- [ ] **Step 148** [R] ~1m: Read current SBOM results
  ```bash
  cat sbom-results/SBOM-CONSOLIDATED.md
  ```

- [ ] **Step 149** [R] ~1m: Read current license findings
  ```bash
  cat sbom-results/licenses-found.txt
  ```

- [ ] **Step 150** [W] ~10m: Create scripts/generate-sbom.sh — comprehensive SBOM generation
  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  # Generate SBOM in multiple formats
  # 1. Syft CycloneDX JSON (Go + Rust)
  # 2. Syft SPDX JSON
  # 3. Go module list with licenses
  # 4. Cargo audit report
  # 5. Consolidated human-readable summary
  # Output: sbom-results/
  ```

- [ ] **Step 151** [B] ~30s: Make script executable
  ```bash
  chmod +x scripts/generate-sbom.sh
  ```

- [ ] **Step 152** [C] ~30s: **COMMIT CHECKPOINT** — SBOM script
  ```bash
  git add scripts/generate-sbom.sh && git commit -m "[PLAN S72] Steps 148-152: SBOM generation script — multi-format output"
  ```

- [ ] **Step 153** [W] ~10m: Add Makefile target: `make sbom`
  ```makefile
  .PHONY: sbom
  sbom: ## Generate Software Bill of Materials
  	@echo "Generating SBOM..."
  	@bash scripts/generate-sbom.sh
  	@echo "SBOM generated in sbom-results/"
  ```

- [ ] **Step 154** [W] ~10m: Add Makefile target: `make license-check`
  ```makefile
  .PHONY: license-check
  license-check: ## Check all dependencies for license compliance
  	@echo "Checking Go licenses..."
  	@go-licenses check ./... --allowed_licenses=MIT,Apache-2.0,BSD-2-Clause,BSD-3-Clause,ISC
  	@echo "Checking Rust licenses..."
  	@cargo license --manifest-path cmd/ebpf-collector/Cargo.toml 2>/dev/null || echo "cargo-license not installed"
  	@echo "License check complete"
  ```

- [ ] **Step 155** [W] ~15m: Update .github/workflows/ci.yml — enhance SBOM job
  - Add SPDX output alongside CycloneDX
  - Add Grype vulnerability scan of SBOM
  - Add SBOM attestation via cosign (if available)
  - Upload SBOM as release artifact (not just CI artifact)

- [ ] **Step 156** [W] ~10m: Create .github/workflows/sbom-audit.yml — weekly scheduled SBOM audit
  ```yaml
  name: Weekly SBOM Audit
  on:
    schedule:
      - cron: '0 9 * * 1'  # Every Monday 9 AM
    workflow_dispatch: {}
  jobs:
    audit:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v4
        - name: Generate SBOM
          run: bash scripts/generate-sbom.sh
        - name: Vulnerability Scan
          run: grype sbom:sbom-results/sbom-cyclonedx.json --fail-on high
        - name: Upload Results
          uses: actions/upload-artifact@v4
          with:
            name: weekly-sbom-audit
            path: sbom-results/
  ```

- [ ] **Step 157** [C] ~30s: **COMMIT CHECKPOINT** — SBOM CI integration
  ```bash
  git add Makefile .github/workflows/ scripts/ && git commit -m "[PLAN S72] Steps 153-157: SBOM automation — Makefile targets, CI integration, weekly audit"
  ```

- [ ] **Step 158** [W] ~10m: Update THIRD_PARTY.md with current dependency inventory
  - Cross-reference go.sum for Go deps
  - Cross-reference Cargo.lock for Rust deps
  - Verify all licenses are MIT/Apache-2.0/BSD (no GPL except Doom)
  - Flag any new dependencies added since last audit

- [ ] **Step 159** [W] ~5m: Create sbom-results/COMPLIANCE_GATE.md
  ```markdown
  # Compliance Gate — S72
  ## Status: PASS/FAIL
  ## Go Dependencies: XX (all MIT/Apache/BSD)
  ## Rust Dependencies: XX (all MIT/Apache/BSD)
  ## GPL Boundary: Doom only (/doom/ isolated)
  ## New since last audit: [list]
  ## Vulnerabilities: [grype output summary]
  ```

- [ ] **Step 160** [C] ~30s: **COMMIT CHECKPOINT** — Compliance docs
  ```bash
  git add THIRD_PARTY.md sbom-results/ && git commit -m "[PLAN S72] Steps 158-160: THIRD_PARTY updated, compliance gate documented"
  ```

- [ ] **Step 161** [V] ~1m: **PHASE 8 EXIT GATE** — SBOM script exists, Makefile targets work, CI workflow added, THIRD_PARTY current
  - `test -f scripts/generate-sbom.sh` → pass
  - `test -f .github/workflows/sbom-audit.yml` → pass
  - `grep "sbom" Makefile` → present
  - If fail → DO NOT PROCEED.

---

## PHASE 9: GITHUB ACTIONS HARDENING (Steps 162-200)

**Goal**: Harden all 6 GHA workflows, add security scanning, coverage trending, artifact signing.
**Prerequisite**: Phase 8 complete.
**Time**: ~2 hours
**Agent**: Agent [P]

### 9A: CI Workflow Hardening

- [ ] **Step 162** [R] ~1m: Read all workflow files
  ```bash
  wc -l .github/workflows/*.yml
  ```

- [ ] **Step 163** [W] ~15m: Harden .github/workflows/ci.yml
  - Pin all action versions to SHA (not @v4 — use @sha)
  - Add: `permissions: read-all` (principle of least privilege)
  - Add govulncheck step after build
  - Add gosec SARIF upload to GitHub Security tab
  - Bump coverage threshold from 50% → 60%
  - Add cache for Go modules (`actions/cache`)
  - Add timeout-minutes: 15 to all jobs

- [ ] **Step 164** [W] ~15m: Harden .github/workflows/ci-protocol.yml
  - Pin all action versions to SHA
  - Add permissions block
  - Add Go module caching
  - Add Rust caching (target/ and registry/)
  - Add timeout-minutes to all jobs
  - Ensure ci-gate requires ALL jobs (not just some)

- [ ] **Step 165** [C] ~30s: **COMMIT CHECKPOINT** — CI hardened
  ```bash
  git add .github/workflows/ && git commit -m "[PLAN S72] Steps 162-165: GHA hardened — pinned actions, least privilege, caching, timeouts"
  ```

### 9B: Security Scanning Workflow

- [ ] **Step 166** [W] ~15m: Create .github/workflows/security.yml — dedicated security scan
  ```yaml
  name: Security Scan
  on:
    push: { branches: [main, develop] }
    pull_request: { branches: [main] }
    schedule:
      - cron: '0 6 * * *'  # Daily 6 AM
  permissions: read-all
  jobs:
    govulncheck:
      runs-on: ubuntu-latest
      timeout-minutes: 10
      steps:
        - uses: actions/checkout@v4
        - uses: actions/setup-go@v5
          with: { go-version: '1.24' }
        - run: go install golang.org/x/vuln/cmd/govulncheck@latest
        - run: govulncheck ./...
    gosec:
      runs-on: ubuntu-latest
      timeout-minutes: 10
      steps:
        - uses: actions/checkout@v4
        - uses: securego/gosec@master
          with:
            args: '-fmt sarif -out gosec.sarif ./...'
        - uses: github/codeql-action/upload-sarif@v3
          with: { sarif_file: gosec.sarif }
    cargo-audit:
      runs-on: ubuntu-latest
      timeout-minutes: 10
      steps:
        - uses: actions/checkout@v4
        - run: cargo install cargo-audit
        - run: cargo audit --manifest-path cmd/ebpf-collector/Cargo.toml
        - run: cargo audit --manifest-path cmd/trace-collector/Cargo.toml
  ```

- [ ] **Step 167** [C] ~30s: **COMMIT CHECKPOINT** — Security workflow
  ```bash
  git add .github/workflows/security.yml && git commit -m "[PLAN S72] Steps 166-167: Dedicated security scanning workflow — govulncheck, gosec SARIF, cargo audit"
  ```

### 9C: Coverage Trending

- [ ] **Step 168** [W] ~10m: Add coverage upload to ci.yml
  - Upload coverage.out as artifact
  - Add Codecov upload step (if repo has Codecov token)
  - Add coverage badge generation

- [ ] **Step 169** [W] ~10m: Create scripts/coverage-trend.sh — local coverage tracking
  ```bash
  #!/usr/bin/env bash
  # Runs coverage, appends to coverage-history.csv
  # Columns: date, total_coverage, pkg_auth, pkg_transport, pkg_protocol, pkg_wotan_client
  ```

- [ ] **Step 170** [C] ~30s: **COMMIT CHECKPOINT** — Coverage trending
  ```bash
  git add .github/workflows/ scripts/ && git commit -m "[PLAN S72] Steps 168-170: Coverage trending — upload, tracking script"
  ```

### 9D: Docker & Helm Workflow Hardening

- [ ] **Step 171** [W] ~10m: Harden .github/workflows/docker.yml
  - Pin actions to SHA
  - Add permissions block
  - Add Trivy container scan after build
  - Add SBOM generation for container images (syft)

- [ ] **Step 172** [W] ~10m: Harden .github/workflows/helm.yml
  - Pin actions to SHA
  - Add kubeconform validation (stricter than helm template)
  - Add OPA policy checks for security constraints
  - Verify: no hostNetwork, no privileged, readOnlyRootFilesystem: true

- [ ] **Step 173** [W] ~10m: Harden .github/workflows/release.yml
  - Pin actions to SHA
  - Add cosign signing for release binaries
  - Add SLSA provenance generation
  - Add SBOM attachment to GitHub Release

- [ ] **Step 174** [C] ~30s: **COMMIT CHECKPOINT** — Docker/Helm/Release hardened
  ```bash
  git add .github/workflows/ && git commit -m "[PLAN S72] Steps 171-174: Docker, Helm, Release workflows hardened — Trivy, kubeconform, cosign, SLSA"
  ```

### 9E: Branch Protection Rules Documentation

- [ ] **Step 175** [W] ~10m: Create .github/BRANCH_PROTECTION.md — recommended settings
  ```markdown
  # Branch Protection Rules (apply via GitHub UI or gh api)
  ## main
  - Require PR reviews: 1
  - Require status checks: ci-gate, security, license-check
  - Require linear history
  - No force push
  - No deletion
  ## develop
  - Require status checks: go-build, go-test, go-lint
  ```

- [ ] **Step 176** [C] ~30s: **COMMIT CHECKPOINT** — Branch protection docs
  ```bash
  git add .github/ && git commit -m "[PLAN S72] Steps 175-176: Branch protection rules documented"
  ```

- [ ] **Step 177** [V] ~1m: **PHASE 9 EXIT GATE** — All workflows hardened, security scanning added, coverage trending configured
  - All .yml files have `permissions:` block → pass
  - security.yml exists → pass
  - If fail → DO NOT PROCEED.

---

## PHASE 10: JENKINSFILE SCAFFOLDING (Steps 178-210)

**Goal**: Create Jenkinsfiles for future Jenkins integration — mirrors GHA pipelines.
**Prerequisite**: Phase 9 complete (GHA is source of truth, Jenkins mirrors it).
**Time**: ~2 hours
**Agent**: Agent

### 10A: Main Jenkinsfile

- [ ] **Step 178** [W] ~20m: Create Jenkinsfile (Declarative Pipeline)
  ```groovy
  pipeline {
      agent { docker { image 'golang:1.24' } }
      options {
          timeout(time: 30, unit: 'MINUTES')
          disableConcurrentBuilds()
          timestamps()
      }
      environment {
          GOPATH = "${WORKSPACE}/.go"
          GOCACHE = "${WORKSPACE}/.cache/go-build"
      }
      stages {
          stage('Dependencies') { steps { sh 'go mod download' } }
          stage('Build') { steps { sh 'go build ./...' } }
          stage('Vet') { steps { sh 'go vet ./...' } }
          stage('Lint') {
              steps {
                  sh '''
                  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ${GOPATH}/bin
                  ${GOPATH}/bin/golangci-lint run ./...
                  '''
              }
          }
          stage('Test') {
              steps {
                  sh 'go test -race -count=1 -timeout 120s -coverprofile=coverage.out ./...'
                  sh 'go tool cover -func=coverage.out | tail -1'
              }
              post {
                  always {
                      archiveArtifacts artifacts: 'coverage.out', allowEmptyArchive: true
                  }
              }
          }
          stage('Security') {
              parallel {
                  stage('govulncheck') {
                      steps {
                          sh 'go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...'
                      }
                  }
                  stage('gosec') {
                      steps {
                          sh 'go install github.com/securego/gosec/v2/cmd/gosec@latest && gosec ./...'
                      }
                  }
              }
          }
          stage('SBOM') {
              steps {
                  sh 'bash scripts/generate-sbom.sh'
                  archiveArtifacts artifacts: 'sbom-results/**', allowEmptyArchive: true
              }
          }
      }
      post {
          failure { echo 'Pipeline failed — check stage logs' }
          success { echo 'Pipeline passed — all gates green' }
      }
  }
  ```

- [ ] **Step 179** [V] ~30s: Jenkinsfile syntax validation (basic)
  ```bash
  head -5 Jenkinsfile && echo "Jenkinsfile exists and is readable"
  ```

- [ ] **Step 180** [C] ~30s: **COMMIT CHECKPOINT** — Main Jenkinsfile
  ```bash
  git add Jenkinsfile && git commit -m "[PLAN S72] Steps 178-180: Jenkinsfile — Go build, test, lint, security, SBOM"
  ```

### 10B: Specialized Jenkinsfiles

- [ ] **Step 181** [W] ~15m: Create jenkins/Jenkinsfile.protocol — protocol-specific pipeline
  ```groovy
  // Mirrors ci-protocol.yml: Go + Rust + eBPF builds
  // Stages: go-lint, go-test, go-build, rust-check, rust-test, ebpf-build, integration-test, benchmark
  ```

- [ ] **Step 182** [W] ~15m: Create jenkins/Jenkinsfile.docker — container pipeline
  ```groovy
  // Mirrors docker.yml: multi-platform builds
  // Stages: build-wotan, build-kanban, trivy-scan, push-to-registry
  ```

- [ ] **Step 183** [W] ~15m: Create jenkins/Jenkinsfile.security — security pipeline
  ```groovy
  // Mirrors security.yml: daily security scan
  // Stages: govulncheck, gosec, cargo-audit, dependency-review
  // Triggers: cron('H 6 * * *')
  ```

- [ ] **Step 184** [W] ~15m: Create jenkins/Jenkinsfile.release — release pipeline
  ```groovy
  // Mirrors release.yml: cross-compile + release
  // Parameters: VERSION (string), DRY_RUN (boolean)
  // Stages: build-matrix, checksum, sign, github-release
  ```

- [ ] **Step 185** [C] ~30s: **COMMIT CHECKPOINT** — Specialized Jenkinsfiles
  ```bash
  git add jenkins/ && git commit -m "[PLAN S72] Steps 181-185: Specialized Jenkinsfiles — protocol, docker, security, release"
  ```

### 10C: Jenkins Shared Library

- [ ] **Step 186** [W] ~10m: Create jenkins/vars/unheadedPipeline.groovy — shared library
  ```groovy
  // Common pipeline steps:
  // - goSetup(version): install Go + cache modules
  // - rustSetup(): install Rust stable + nightly
  // - securityScan(): govulncheck + gosec
  // - sbomGenerate(): run scripts/generate-sbom.sh
  // - coverageGate(threshold): fail if below threshold
  ```

- [ ] **Step 187** [W] ~10m: Create jenkins/README.md — Jenkins setup guide
  ```markdown
  # Jenkins CI/CD for Unheaded
  ## Prerequisites
  - Jenkins 2.400+ with Pipeline plugin
  - Docker agent capability
  - Go 1.24 available in Docker images
  ## Pipeline Index
  - Jenkinsfile — Main CI (build, test, lint, security, SBOM)
  - jenkins/Jenkinsfile.protocol — Protocol Foundation CI
  - jenkins/Jenkinsfile.docker — Container builds
  - jenkins/Jenkinsfile.security — Daily security scans
  - jenkins/Jenkinsfile.release — Release pipeline
  ## Shared Library
  - jenkins/vars/unheadedPipeline.groovy — common steps
  ```

- [ ] **Step 188** [C] ~30s: **COMMIT CHECKPOINT** — Jenkins shared library + docs
  ```bash
  git add jenkins/ && git commit -m "[PLAN S72] Steps 186-188: Jenkins shared library + setup guide"
  ```

- [ ] **Step 189** [V] ~1m: **PHASE 10 EXIT GATE** — Jenkinsfile exists, specialized pipelines exist, shared library exists
  - `test -f Jenkinsfile` → pass
  - `ls jenkins/Jenkinsfile.* | wc -l` >= 4 → pass
  - `test -f jenkins/vars/unheadedPipeline.groovy` → pass
  - If fail → DO NOT PROCEED.

---

# ═══════════════════════════════════════════════════════════
# PART 4: BARE METAL PREP (Phases 11-12)
# ═══════════════════════════════════════════════════════════

## PHASE 11: BARE METAL BOOT SEQUENCE — HOST-A FORGE (Steps 190-240)

**Goal**: Create a comprehensive boot-and-validate runbook for host-a (The Forge) — NixOS install, OPNsense VM, FRR routing, eBPF validation. Muck executes tonight.
**Prerequisite**: Phase 10 complete.
**Time**: ~1.5 hours (to write), ~3-4 hours (to execute on hardware)
**Agent**: Coordinator

- [ ] **Step 190** [W] ~30m: Create docs/bare-metal/BOOT_SEQUENCE_HOST_A.md
  ```markdown
  # Host-A (The Forge) Boot Sequence
  ## Hardware Requirements
  - AMD64 CPU with BTF support (kernel 5.15+)
  - 32GB+ RAM recommended
  - 500GB+ NVMe
  - 2x NICs (management + data plane)

  ## Phase 1: NixOS Base Install (~30 min)
  1. Boot NixOS ISO (latest unstable)
  2. Partition: 512MB EFI + remainder btrfs
  3. nixos-generate-config --root /mnt
  4. Copy nixos/ tree from repo to /mnt/etc/nixos/
  5. nixos-install --flake .#host-a
  6. Reboot, verify SSH access

  ## Phase 2: Firewall VM Setup (~45 min)
  7. Run scripts/firewall/setup-opnsense.sh
  8. Verify OPNsense WebUI on 192.168.1.1
  9. Configure WAN/LAN interfaces via REST API
  10. Import Monad HbH pass-through rules
  11. Verify: curl -k https://192.168.1.1/api/core/system/status

  ## Phase 3: FRR Routing (~30 min)
  12. Run scripts/firewall/install-frr.sh
  13. Copy routing/frr.conf to /etc/frr/frr.conf
  14. systemctl start frr
  15. Verify IS-IS adjacency: vtysh -c "show isis neighbor"
  16. Verify BGP: vtysh -c "show bgp summary"
  17. Verify BFD: vtysh -c "show bfd peers"

  ## Phase 4: VXLAN/EVPN Setup (~20 min)
  18. Run scripts/firewall/setup-vtep.sh
  19. Verify VTEPs: ip -d link show vxlan10001
  20. Verify EVPN routes: vtysh -c "show bgp l2vpn evpn"

  ## Phase 5: eBPF Loading (~30 min)
  21. make ebpf (build programs)
  22. make pin-ebpf (load to /sys/fs/bpf/unheaded/)
  23. Verify: bpftool prog list | grep unheaded
  24. Verify maps: bpftool map list | grep unheaded
  25. Check XDP attach: ip link show | grep xdp

  ## Phase 6: Service Fleet Boot (~30 min)
  26. make build (all Go services)
  27. docker compose -f docker-compose.yml up -d
  28. make deploy-health (verify all services)
  29. Verify Wotan: curl http://10.10.10.10:18000/health
  30. Verify Dashboard: curl http://10.10.10.100:20000/

  ## Phase 7: End-to-End Validation (~30 min)
  31. Monad HbH packet injection (Scapy): sudo python3 scripts/doom/inject.py
  32. Verify packet traverses: eBPF → Shield → Monad → Sophia → Wotan
  33. Verify dashboard shows live flow
  34. Verify logs aggregate to dashboard
  35. Record H1-H8 verdicts

  ## Smoke Test Checklist
  - [ ] NixOS boots and SSH works
  - [ ] OPNsense VM running and reachable
  - [ ] FRR IS-IS adjacency UP
  - [ ] BGP EVPN routes present
  - [ ] BFD peers established
  - [ ] VXLAN tunnels operational
  - [ ] eBPF programs loaded and pinned
  - [ ] All services healthy
  - [ ] Wotan pub/sub working
  - [ ] Monad HbH packet end-to-end
  - [ ] Dashboard shows live data
  ```

- [ ] **Step 191** [C] ~30s: **COMMIT CHECKPOINT** — Host-A boot sequence
  ```bash
  mkdir -p docs/bare-metal && git add docs/bare-metal/ && git commit -m "[PLAN S72] Steps 190-191: Host-A boot sequence runbook"
  ```

### 11B: Pre-flight Validation Scripts

- [ ] **Step 192** [W] ~15m: Create scripts/bare-metal/preflight-host-a.sh
  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  echo "=== HOST-A PREFLIGHT CHECK ==="
  # Check: kernel version >= 5.15
  # Check: BTF support
  # Check: bpftool available
  # Check: Docker/containerd running
  # Check: NIC count >= 2
  # Check: RAM >= 16GB
  # Check: Disk >= 100GB free
  # Check: Go 1.24 available
  # Check: Rust nightly available
  # Check: FRR compilable (dependencies)
  # Output: PASS/FAIL per check + overall verdict
  ```

- [ ] **Step 193** [W] ~15m: Create scripts/bare-metal/validate-host-a.sh
  ```bash
  #!/usr/bin/env bash
  set -euo pipefail
  echo "=== HOST-A POST-BOOT VALIDATION ==="
  # After boot sequence, run this to validate everything:
  # 1. ping all service IPs on 10.10.10.0/24
  # 2. curl each service /health endpoint
  # 3. bpftool prog list (expect 4+ programs)
  # 4. bpftool map list (expect 8+ maps)
  # 5. vtysh routing table check
  # 6. Wotan topic publish + subscribe roundtrip
  # 7. Monad HbH single-packet test
  # Output: structured JSON results + PASS/FAIL verdict
  ```

- [ ] **Step 194** [C] ~30s: **COMMIT CHECKPOINT** — Preflight + validation scripts
  ```bash
  chmod +x scripts/bare-metal/*.sh && git add scripts/bare-metal/ && git commit -m "[PLAN S72] Steps 192-194: Bare metal preflight + validation scripts"
  ```

- [ ] **Step 195** [V] ~1m: **PHASE 11 EXIT GATE** — Host-A boot doc + scripts exist
  - `test -f docs/bare-metal/BOOT_SEQUENCE_HOST_A.md` → pass
  - `test -f scripts/bare-metal/preflight-host-a.sh` → pass
  - `test -f scripts/bare-metal/validate-host-a.sh` → pass

---

## PHASE 12: BARE METAL PREP — HOST-B OUTPOST + WireGuard (Steps 196-225)

**Goal**: Create boot runbook for host-b (The Outpost), WireGuard tunnel setup, cross-host validation.
**Prerequisite**: Phase 11 complete.
**Time**: ~1 hour
**Agent**: Agent [P]

- [ ] **Step 196** [W] ~25m: Create docs/bare-metal/BOOT_SEQUENCE_HOST_B.md
  ```markdown
  # Host-B (The Outpost) Boot Sequence
  ## Similar to Host-A but with:
  - IPFire instead of OPNsense
  - BIRD instead of FRR
  - AS 65002 (not 65001)
  - WireGuard endpoint to Host-A

  ## Phase 1: NixOS Base Install
  ## Phase 2: IPFire VM Setup
  ## Phase 3: BIRD Routing
  ## Phase 4: WireGuard Tunnel
  ## Phase 5: eBPF Loading
  ## Phase 6: Cross-Host Validation
  ```

- [ ] **Step 197** [W] ~15m: Create docs/bare-metal/WIREGUARD_TUNNEL.md
  ```markdown
  # WireGuard Tunnel Setup
  ## Key Generation
  - Host-A: wg genkey | tee host-a-private | wg pubkey > host-a-public
  - Host-B: wg genkey | tee host-b-private | wg pubkey > host-b-public
  ## Interface Config
  - Host-A wg0: fd00:dead:beef::1/48, listen 51820
  - Host-B wg0: fd00:dead:beef::2/48, listen 51820
  ## Exchange Keys
  - Host-A peer: Host-B public key, endpoint <host-b-ip>:51820
  - Host-B peer: Host-A public key, endpoint <host-a-ip>:51820
  ## Validation
  - ping6 fd00:dead:beef::2 (from host-a)
  - ping6 fd00:dead:beef::1 (from host-b)
  - wg show (both hosts)
  ```

- [ ] **Step 198** [W] ~15m: Create scripts/bare-metal/preflight-host-b.sh

- [ ] **Step 199** [W] ~15m: Create scripts/bare-metal/validate-host-b.sh

- [ ] **Step 200** [W] ~15m: Create scripts/bare-metal/validate-cross-host.sh
  ```bash
  #!/usr/bin/env bash
  # Cross-host validation after both hosts are up:
  # 1. WireGuard tunnel ping (fd00:dead:beef::/48)
  # 2. BGP peering between FRR (host-a) and BIRD (host-b)
  # 3. EVPN route exchange
  # 4. Monad HbH packet: host-a → WireGuard → host-b → return
  # 5. Service discovery across hosts
  # 6. Wotan cross-host pub/sub
  ```

- [ ] **Step 201** [C] ~30s: **COMMIT CHECKPOINT** — Host-B docs + cross-host validation
  ```bash
  git add docs/bare-metal/ scripts/bare-metal/ && git commit -m "[PLAN S72] Steps 196-201: Host-B boot sequence + WireGuard + cross-host validation"
  ```

- [ ] **Step 202** [V] ~1m: **PHASE 12 EXIT GATE** — Both host runbooks + all scripts exist
  - Host-A doc + scripts → pass
  - Host-B doc + scripts → pass
  - Cross-host script → pass

---

# ═══════════════════════════════════════════════════════════
# PART 5: FINAL VERIFICATION (Phase 13)
# ═══════════════════════════════════════════════════════════

## PHASE 13: FINAL VERIFICATION GATE (Steps 203-225)

**Goal**: Full build, full test, full lint, verify all deliverables.
**Prerequisite**: All prior phases complete.
**Time**: ~30 minutes
**Agent**: Coordinator

- [ ] **Step 203** [B] ~3m: Full Go build
  ```bash
  go build ./... 2>&1 | tail -10
  ```

- [ ] **Step 204** [V] ~30s: Build passes

- [ ] **Step 205** [B] ~5m: Full test suite with race detector
  ```bash
  go test -race -count=1 -timeout 180s ./... 2>&1 | tail -30
  ```

- [ ] **Step 206** [V] ~30s: All tests pass

- [ ] **Step 207** [B] ~2m: Lint check
  ```bash
  golangci-lint run ./... 2>&1 | tail -20 || echo "LINT ISSUES — review but don't block"
  ```

- [ ] **Step 208** [B] ~1m: Security scan
  ```bash
  gosec ./... 2>&1 | tail -20 || echo "GOSEC ISSUES — review"
  ```

- [ ] **Step 209** [C] ~30s: **COMMIT CHECKPOINT** — Final build/test/lint verified
  ```bash
  git add . && git commit -m "[PLAN S72] Steps 203-209: Final verification — build, test, lint, security all pass"
  ```

### Deliverable Verification

- [ ] **Step 210** [V] ~1m: Protocol Specs
  ```bash
  test -f docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md && echo "MONAD ✓"
  test -f docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md && echo "SOPHIA ✓"
  test -f docs/protocol/draft-bellis-unheaded-wotan-memory-02.md && echo "WOTAN ✓"
  ```

- [ ] **Step 211** [V] ~1m: Auth Package
  ```bash
  go test -coverprofile=/tmp/auth.out ./pkg/auth/... && go tool cover -func=/tmp/auth.out | tail -1
  ```
  - Coverage >= 80% → pass

- [ ] **Step 212** [V] ~30s: mTLS
  ```bash
  go test ./pkg/transport/mtls/... 2>&1 | tail -5
  ```

- [ ] **Step 213** [V] ~30s: SBOM
  ```bash
  test -f scripts/generate-sbom.sh && echo "SBOM SCRIPT ✓"
  test -f .github/workflows/sbom-audit.yml && echo "SBOM CI ✓"
  ```

- [ ] **Step 214** [V] ~30s: CI/CD
  ```bash
  ls .github/workflows/*.yml | wc -l
  ```
  - Should be >= 7 (6 existing + 1 new security.yml)

- [ ] **Step 215** [V] ~30s: Jenkins
  ```bash
  test -f Jenkinsfile && echo "JENKINSFILE ✓"
  ls jenkins/Jenkinsfile.* 2>/dev/null | wc -l
  ```
  - Should be >= 4

- [ ] **Step 216** [V] ~30s: Bare Metal
  ```bash
  test -f docs/bare-metal/BOOT_SEQUENCE_HOST_A.md && echo "HOST-A ✓"
  test -f docs/bare-metal/BOOT_SEQUENCE_HOST_B.md && echo "HOST-B ✓"
  ls scripts/bare-metal/*.sh | wc -l
  ```
  - Should be >= 5 scripts

- [ ] **Step 217** [V] ~1m: **PHASE 13 EXIT GATE — S72 FINAL GATE**
  ```
  ALL CHECKS MUST PASS:
  ✓ Protocol specs — 3 new drafts
  ✓ Auth — hardened, 80%+ coverage
  ✓ mTLS — cert gen + integration
  ✓ SBOM — automated
  ✓ CI/CD — 7+ workflows hardened
  ✓ Jenkins — 5 Jenkinsfiles
  ✓ Bare Metal — 2 runbooks + 5 scripts
  ✓ Build passes
  ✓ Tests pass
  ✓ Lint clean (or issues documented)
  ```
  - ALL PASS → S72 COMPLETE. Update timeline. Celebrate.
  - ANY FAIL → Document in STUCK REPORT. Fix in S73.

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: Go Build Fails After Proto Changes
```bash
# Symptom: undefined reference to generated proto types
# Fix: Regenerate proto bindings
protoc --go_out=. --go-grpc_out=. proto/unheaded/v1/protocol.proto
go mod tidy
go build ./...
```

### E2: Auth Tests Break Existing Services
```bash
# Symptom: services fail to start because auth middleware rejects all requests
# Fix: Check test fixtures — ensure dev tokens are generated in test setup
# All test helpers should call testutil.AdminToken() for setup
go test -v -run "TestSetup" ./pkg/auth/...
```

### E3: mTLS Cert Generation Fails
```bash
# Symptom: x509 certificate errors
# Fix: Check that CA cert is generated before service certs
# Ensure ECDSA P-256 curve is available
go test -v -run "TestGenerateCA" ./pkg/transport/mtls/...
```

### E4: CI Workflow Syntax Error
```bash
# Symptom: GitHub Actions rejects workflow file
# Fix: Validate YAML syntax locally
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
# Or use: actionlint (if installed)
```

### E5: SBOM Generation Fails
```bash
# Symptom: syft not found or produces empty output
# Fix: Install syft
curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin
syft version
syft dir:. -o cyclonedx-json=test-sbom.json
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent Type | Parallelizable | Dependencies | Time Est |
|-------|-----------|----------------|-------------|----------|
| 0 | Coordinator | No | None | 10m |
| 1 | Coordinator + RFC Editor | No | Phase 0 | 3h |
| 2 | Agent | Yes (with 3) | Phase 0 | 2h |
| 3 | Agent | Yes (with 2) | Phase 0 | 2h |
| 4 | Coordinator | No | Phases 1-3 | 1.5h |
| 5 | Developer | No | Phase 4 | 3h |
| 6 | Developer | No | Phase 5 | 2.5h |
| 7 | Developer | No | Phases 5+6 | 2h |
| 8 | Agent | Yes (with 9) | Phase 7 | 1.5h |
| 9 | Agent | Yes (with 8) | Phase 7 | 2h |
| 10 | Agent | No | Phase 9 | 2h |
| 11 | Coordinator | Yes (with 12) | Phase 10 | 1.5h |
| 12 | Agent | Yes (with 11) | Phase 10 | 1h |
| 13 | Coordinator | No | ALL | 30m |

**Critical Path**: 0 → 1 → 4 → 5 → 6 → 7 → 9 → 10 → 13 = ~16 hours minimum
**With Parallelization**: ~12-14 hours across 4-6 sessions

---

## APPENDIX C: QUICK REFERENCE

### Doom Range Port Registry
```
16666-16999: Infrastructure (doom-bridge, trace-collector)
17000-17999: Control Plane (unheaded-daemon)
18000-18099: Wotan (message bus)
19000-19999: Core Services (timeguru, architect, captain, micromanager, monad, sophia)
20000-20999: Applications (dashboard, kanban, wiki)
21000-21443: Gateway (nginx HTTP/HTTPS)
26000-26666: User Apps
```

### Protocol Draft Versions (After S72)
```
Monad Foundation: draft-05 (from draft-04)
Sophia Dictionary: draft-02 (from draft-01)
Wotan Memory: draft-02 (from draft-01)
PQC Auth: draft-00 (unchanged)
```

### Auth Token Types
```
Bearer JWT:     External API access, carries user identity + roles
Service-Token:  Inter-service auth, identifies calling service by name
API-Key:        Third-party integrations, rate-limited
```

### File Layout After S72
```
NEW:
├── docs/protocol/draft-bellis-unheaded-protocol-foundation-05.md
├── docs/protocol/draft-bellis-unheaded-sophia-dictionary-02.md
├── docs/protocol/draft-bellis-unheaded-wotan-memory-02.md
├── docs/protocol/ALIGNMENT_NOTES_DRAFT05.md
├── docs/protocol/PROTOCOL_STATUS_S72.md
├── docs/bare-metal/BOOT_SEQUENCE_HOST_A.md
├── docs/bare-metal/BOOT_SEQUENCE_HOST_B.md
├── docs/bare-metal/WIREGUARD_TUNNEL.md
├── .github/workflows/security.yml
├── .github/workflows/sbom-audit.yml
├── .github/BRANCH_PROTECTION.md
├── Jenkinsfile
├── jenkins/Jenkinsfile.protocol
├── jenkins/Jenkinsfile.docker
├── jenkins/Jenkinsfile.security
├── jenkins/Jenkinsfile.release
├── jenkins/vars/unheadedPipeline.groovy
├── jenkins/README.md
├── scripts/generate-sbom.sh
├── scripts/coverage-trend.sh
├── scripts/bare-metal/preflight-host-a.sh
├── scripts/bare-metal/validate-host-a.sh
├── scripts/bare-metal/preflight-host-b.sh
├── scripts/bare-metal/validate-host-b.sh
├── scripts/bare-metal/validate-cross-host.sh
├── pkg/transport/mtls/certgen.go
├── pkg/transport/mtls/integration.go
├── pkg/auth/middleware_grpc.go
├── sbom-results/COMPLIANCE_GATE.md

MODIFIED:
├── docs/protocol/openapi.yaml
├── docs/protocol/error-registry.md
├── docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md
├── docs/protocol/RFC_ALIGNMENT_REPORT.md
├── pkg/auth/jwt.go + jwt_test.go
├── pkg/auth/rbac.go + rbac_test.go
├── pkg/auth/service_token.go + service_token_test.go
├── pkg/auth/auth.go
├── pkg/transport/mtls/mtls.go + mtls_test.go
├── .github/workflows/ci.yml
├── .github/workflows/ci-protocol.yml
├── .github/workflows/docker.yml
├── .github/workflows/helm.yml
├── .github/workflows/release.yml
├── Makefile
├── THIRD_PARTY.md
├── scripts/gen-dev-certs.sh
├── services/*/main.go (mTLS wiring)
├── cmd/*/main.go (auth middleware)
```

---

*S72 Battle Plan — Forged 2026-02-27*
*13 Phases. 217 Steps. The Kingdom arms itself while the forge awaits its first fire.*
*Protocol specs advance. Auth hardens. Compliance automates. Bare metal beckons.*

```
THE WARMONGER HAS SPOKEN.
THE BATTLE PLAN IS FORGED.
THE KINGDOM MARCHES.
```
