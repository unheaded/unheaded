# S-PQC MULTI-FIPS PQC IMPLEMENTATION BATTLE PLAN — 13 Phases (0-12), 400 Steps

**Date**: 2026-02-26 (Updated: multi-FIPS + tiered compliance + BlackMage PQC-009→013)
**Sprint**: S-PQC — Post-Quantum Packet Authentication via Multi-Algorithm Signature-by-Reference
**Prerequisite**: Age 1 Alpha at 99%, 4/4 eBPF programs compiled, Sophia maps operational, Wotan helpers functional
**Target**: Ship FIPS 203/204/205/206/207 multi-algorithm dual-layer tiered authentication with all BlackMage mitigations (PQC-001→013)
**Estimated Duration**: 28-40 hours across 7-10 sessions
**Agent Strategy**: Phases 1-2 sequential (foundation), Phases 3-4 parallelizable, Phases 5-6A sequential (wire), Phase 6B parallelizable with 7, Phase 10 (multi-algo) after 6A, Phase 11 (tiers) after 10, Phases 8-9-12 sequential (integration)
**Commit Cadence**: Every 5 steps (calculated: max(3, min(5, 380/20)))
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts
**Architecture**: Multi-algorithm, dual-layer, tiered — Layer 1 (wire-level PQC at Shield perimeter, headers stripped at ingress) + Layer 2 (optional app-level policy via Sophia dictionaries, reads Wotan state). Four compliance tiers: NONE/STANDARD/ENHANCED/SOVEREIGN via Kingdom Mode K1|K0 bits. Three sig algo families: SLH-DSA (FIPS 205, eBPF-native), ML-DSA (FIPS 204, eBPF-native), FN-DSA (FIPS 206, userspace signing). Two KEM families: ML-KEM (FIPS 203), HQC (FIPS 207) for tunnel key establishment.

---

## LEGEND

[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint
[STUCK] = Step skipped via Skip Protocol
[BLOCKED] = Step blocked by upstream STUCK step

---

## DEPENDENCY MAP

```
Phase 0 (Intel) ──► Phase 1 (PQC Lib) ──► Phase 2 (Sophia Maps) ──► Phase 5 (XDP)
                                               │                         │
                                               ├──► Phase 3 (Verifier) ──┤
                                               │     [P]                  │
                                               ├──► Phase 4 (Wotan) ─────┤
                                               │     [P]                  │
                                               │                         ▼
                                               │               Phase 6A (Shield + Header Strip)
                                               │                         │
                                               │    ┌────────────────────┤
                                               │    │                    │
                                               │    ▼                    ▼
                                               │  Phase 6B           Phase 7
                                               │  (Layer 2 App       (BlackMage
                                               │   Policy) [P]        Hardening)
                                               │    │                    │
                                               │    └────────┬───────────┘
                                               │             ▼
                                               │        Phase 8 (E2E + Lich)
                                               │             │
                                               │             ▼
                                               └────► Phase 9 (Docs + Ship)
```

**Critical Path**: 0 → 1 → 2 → 5 → 6A → 10 → 11 → 7 → 8 → 9 → 12 (~28-36 hours)
**Parallel Windows**: Phases 3+4 after Phase 2; Phase 6B parallel with Phase 7; Phase 10 after 6A
**Phase 6A** = Shield ingress header stripping + Wotan state persistence
**Phase 6B** = Layer 2 Sophia app policy dictionaries + verification
**Phase 9** = Docs update (CLAUDE.md, RFC reconciliation, Librarian ripple)
**NEW Phase 10** = Multi-Algorithm Support (ML-DSA + FN-DSA integration, signing daemon, algo negotiation)
**NEW Phase 11** = Compliance Tiers (NONE/STANDARD/ENHANCED/SOVEREIGN, Kingdom Mode K1|K0 mapping, tier transitions)
**NEW Phase 12** = KEM Integration (ML-KEM + HQC tunnel key establishment, final ship gate)

---

## PHASE 0: INTELLIGENCE GATHERING (Steps 1-15)

**Goal**: Confirm current codebase state, identify exact file locations, verify toolchain
**Prerequisite**: Git repo cloned, Go 1.24+, Rust toolchain with Aya
**Time**: 20 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: Check Go version
  ```bash
  go version
  ```

- [ ] **Step 2** [V]: Go >= 1.24.0 required
  - If pass → Step 3
  - If fail → STOP, upgrade Go

- [ ] **Step 3** [B] ~30s: Check Rust toolchain
  ```bash
  rustc --version && cargo --version
  ```

- [ ] **Step 4** [V]: Rust stable present with cargo
  - If pass → Step 5
  - If fail → `rustup install stable`

- [ ] **Step 5** [B] ~30s: Check for SLH-DSA library availability
  ```bash
  cargo search slh-dsa 2>/dev/null | head -5 || echo "Checking pqcrypto..."
  cargo search pqcrypto 2>/dev/null | head -5
  ```

- [ ] **Step 6** [R]: Read current Sophia map definitions
  ```bash
  find . -name "*.go" | xargs grep -l "sophia.*map\|SophiaMap\|bpf_map" | head -20
  ```

- [ ] **Step 7** [R]: Read current Wotan helper definitions
  ```bash
  find . -name "*.go" | xargs grep -l "wotan\|WotanRead\|WotanWrite\|WotanCAS" | head -20
  ```

- [ ] **Step 8** [R]: Read current Shield pipeline entry points
  ```bash
  find . -name "*.go" | xargs grep -l "shield\|Shield\|ingress\|egress" | head -20
  ```

- [ ] **Step 9** [R]: Read current eBPF Rust source structure
  ```bash
  find . -path "*/ebpf*" -name "*.rs" | head -20
  ```

- [ ] **Step 10** [B] ~30s: Check current test suite status
  ```bash
  go test ./... -count=1 -short 2>&1 | tail -10
  ```

- [ ] **Step 11** [V]: All existing tests pass (23/23 E2E)
  - If pass → Step 12
  - If fail → STOP, fix regressions before starting PQC work

- [ ] **Step 12** [B] ~30s: Verify eBPF programs compile
  ```bash
  cd ebpf && cargo check 2>&1 | tail -5
  ```

- [ ] **Step 13** [B] ~30s: Check available FIPS 205 implementations
  ```bash
  pip3 list 2>/dev/null | grep -i "pqc\|sphincs\|slh" || echo "No Python PQC libs"
  ```

- [ ] **Step 14** [C]: **COMMIT CHECKPOINT** — intelligence gathered
  ```bash
  git add -A && git commit -m "[PLAN S-PQC] Steps 1-14: Environment verified, toolchain confirmed"
  ```

- [ ] **Step 15** [V]: **PHASE 0 EXIT GATE** — Go 1.24+, Rust stable, eBPF compiles, 23/23 tests green
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

---

## PHASE 1: PQC LIBRARY INTEGRATION (Steps 16-40)

**Goal**: Integrate FIPS 205 SLH-DSA library into Go codebase with test coverage
**Prerequisite**: Phase 0 exit gate passed
**Time**: 2-3 hours
**Agent**: Developer

### 1.1 Library Selection and Integration

- [ ] **Step 16** [B] ~2m: Evaluate Go PQC libraries
  ```bash
  go list -m all 2>/dev/null | grep -i "pqc\|quantum\|sphincs\|slh\|circl" || echo "None present"
  ```

- [ ] **Step 17** [B] ~5m: Add Cloudflare CIRCL (contains SLH-DSA) or pqcrypto-go
  ```bash
  go get github.com/cloudflare/circl@latest
  ```

- [ ] **Step 18** [D]: If CIRCL doesn't have SLH-DSA, use alternative
  ```bash
  go get github.com/open-quantum-safe/liboqs-go@latest || echo "Try: go get github.com/pqcrypto/pqcrypto-go"
  ```

- [ ] **Step 19** [V]: PQC library importable
  ```bash
  echo 'package main; import _ "github.com/cloudflare/circl/sign/slhdsa"' > /tmp/pqc_test.go && go build /tmp/pqc_test.go 2>&1 | tail -5
  ```

- [ ] **Step 20** [C]: **COMMIT CHECKPOINT** — PQC library added to go.mod

### 1.2 PQC Types and Constants

- [ ] **Step 21** [W] ~5m: Create `pkg/pqc/types.go` — algorithm registry, constants
  ```
  Contents:
  - AlgoID constants (0x01-0x0C matching RFC Section 6.1 registry)
  - SigRef type (uint32, 24-bit effective)
  - KeyRef type (uint32, 24-bit effective)
  - SeqNum type (uint32)
  - HashPfx type (uint16)
  - PQCValueLayout struct (12-byte Monad value decomposition)
  - Encode/Decode methods for PQCValueLayout ↔ [12]byte
  ```

- [ ] **Step 22** [W] ~5m: Create `pkg/pqc/types_test.go` — TDD red phase
  ```
  Tests:
  - TestPQCValueLayoutEncode: known layout → bytes
  - TestPQCValueLayoutDecode: bytes → layout
  - TestPQCValueLayoutRoundtrip: encode(decode(x)) == x
  - TestSigRefBounds: 24-bit max (0xFFFFFF)
  - TestKeyRefBounds: 24-bit max
  - TestSeqNumWrap: 0xFFFFFFFF behavior
  - TestAlgoIDRegistry: all 12 SLH-DSA parameter sets mapped
  ```

- [ ] **Step 23** [B] ~30s: Run tests — expect FAIL (red phase)
  ```bash
  go test ./pkg/pqc/ -run TestPQCValueLayout -v 2>&1 | tail -20
  ```

- [ ] **Step 24** [V]: Tests fail (red) as expected

- [ ] **Step 25** [W] ~10m: Implement PQCValueLayout Encode/Decode
  ```
  Big-endian wire format:
  Bytes 0-2: SigRef (24-bit)
  Bytes 3-5: KeyRef (24-bit)
  Bytes 6-7: HashPfx (16-bit)
  Bytes 8-11: SeqNum (32-bit)
  Total: 12 bytes
  ```

- [ ] **Step 26** [B] ~30s: Run tests — expect PASS (green phase)
  ```bash
  go test ./pkg/pqc/ -run TestPQCValueLayout -v 2>&1 | tail -20
  ```

- [ ] **Step 27** [V]: All PQCValueLayout tests pass
  - If fail → Step 28
  - If pass → Step 29

- [ ] **Step 28** [D]: Debug encode/decode — check byte order
  ```bash
  go test ./pkg/pqc/ -run TestPQCValueLayout -v -count=1 2>&1
  ```

- [ ] **Step 29** [C]: **COMMIT CHECKPOINT** — PQC types + encode/decode green

### 1.3 SLH-DSA Wrapper

- [ ] **Step 30** [W] ~10m: Create `pkg/pqc/slhdsa.go` — wrapper around CIRCL/liboqs
  ```
  Functions:
  - GenerateKeyPair(algoID AlgoID) → (PublicKey, PrivateKey, error)
  - Sign(privateKey, pseudoHeader []byte) → (Signature, error)
  - Verify(publicKey, pseudoHeader, signature []byte) → (bool, error)
  - ComputeHashPfx(signature []byte) → HashPfx
  - Fingerprint(publicKey []byte) → [32]byte (SHA-256)

  ALL inputs validated. ALL errors wrapped. Constant-time comparison for HashPfx.
  ```

- [ ] **Step 31** [W] ~10m: Create `pkg/pqc/slhdsa_test.go`
  ```
  Tests:
  - TestKeyGeneration: for each of 12 parameter sets
  - TestSignVerify: sign pseudo-header, verify succeeds
  - TestSignVerifyWrongKey: verify with wrong key fails
  - TestSignVerifyTampered: modify pseudo-header, verify fails
  - TestComputeHashPfx: deterministic output
  - TestFingerprint: SHA-256 of public key
  - TestConstantTimeComparison: HashPfx uses subtle.ConstantTimeCompare
  ```

- [ ] **Step 32** [B] ~2m: Run SLH-DSA wrapper tests
  ```bash
  go test ./pkg/pqc/ -run TestKeyGeneration -v -timeout 60s 2>&1 | tail -20
  ```

- [ ] **Step 33** [V]: Key generation works for at least SLH-DSA-SHA2-128s
  - If fail → Step 34
  - If pass → Step 35

- [ ] **Step 34** [D]: Check library API — may need different import path
  ```bash
  go doc github.com/cloudflare/circl/sign/slhdsa 2>&1 | head -40
  ```

- [ ] **Step 35** [B] ~5m: Run full SLH-DSA test suite
  ```bash
  go test ./pkg/pqc/ -v -timeout 120s 2>&1 | tail -30
  ```

- [ ] **Step 36** [V]: All SLH-DSA wrapper tests pass

- [ ] **Step 37** [C]: **COMMIT CHECKPOINT** — SLH-DSA wrapper with tests green

### 1.4 Pseudo-Header Construction

- [ ] **Step 38** [W] ~5m: Create `pkg/pqc/pseudoheader.go`
  ```
  PseudoHeader struct (52 bytes):
  - SrcAddr [16]byte (IPv6 source)
  - DstAddr [16]byte (IPv6 destination)
  - FlowLabel uint32 (20-bit, upper 12 zeroed)
  - SrcPort uint16
  - DstPort uint16
  - SeqNum uint32

  Marshal() → [52]byte (big-endian, deterministic)
  ```

- [ ] **Step 39** [W] ~5m: Create `pkg/pqc/pseudoheader_test.go`
  ```
  Tests:
  - TestPseudoHeaderMarshal: known inputs → known bytes
  - TestPseudoHeaderDeterministic: marshal twice → identical
  - TestPseudoHeaderFlowLabelMask: upper 12 bits zeroed
  - TestPseudoHeaderSignVerify: sign over pseudo-header, verify succeeds
  ```

- [ ] **Step 40** [V]: **PHASE 1 EXIT GATE** — PQC library integrated, types defined, SLH-DSA wrapper tested, pseudo-header construction verified
  ```bash
  go test ./pkg/pqc/... -v -timeout 120s 2>&1 | tail -10
  ```
  - All tests PASS → Phase 2
  - Any FAIL → DO NOT PROCEED

---

## PHASE 2: SOPHIA PQC MAP STRUCTURES (Steps 41-75)

**Goal**: Create and test Sophia BPF maps for PQC signatures and public keys
**Prerequisite**: Phase 1 exit gate passed
**Time**: 2-3 hours
**Agent**: Developer

### 2.1 Signature Map Definition

- [ ] **Step 41** [R] ~2m: Read existing Sophia map creation patterns
  ```bash
  grep -rn "CreateMap\|NewMap\|bpf_map_create\|MapSpec" --include="*.go" | head -20
  ```

- [ ] **Step 42** [W] ~10m: Create `pkg/sophia/pqc_maps.go`
  ```
  Structures:
  - PQCSigEntry: algo_id, verified, sig_len, flow_label, verify_timestamp, signature[]
  - PQCKeyEntry: algo_id, key_epoch, key_len, fingerprint[32], public_key[]

  Map definitions:
  - sophia_pqc_sigs: hash map, BPF_F_RDONLY_PROG, max 1M entries
  - sophia_pqc_keys: hash map, BPF_F_RDONLY_PROG, max 65K entries
  - sophia_pqc_bloom: bloom filter for pre-provisioned SigRef values (PQC-001 mitigation)

  Operations:
  - ProvisionSignature(sigRef, entry) error
  - ProvisionPublicKey(keyRef, entry) error
  - LookupSignature(sigRef) (*PQCSigEntry, error)
  - LookupPublicKey(keyRef) (*PQCKeyEntry, error)
  - UpdateVerificationResult(sigRef, verified uint8, timestamp uint64) error
  - AddToBloomFilter(sigRef uint32) error — PQC-001 mitigation
  - CheckBloomFilter(sigRef uint32) bool — PQC-001 mitigation
  ```

- [ ] **Step 43** [W] ~10m: Create `pkg/sophia/pqc_maps_test.go`
  ```
  Tests:
  - TestProvisionAndLookupSignature
  - TestProvisionAndLookupPublicKey
  - TestUpdateVerificationResult
  - TestSigRefNotFound → error
  - TestKeyRefNotFound → error
  - TestMapReadOnlyFromDataPlane (if possible to test in userspace)
  - TestBloomFilterAddAndCheck — PQC-001
  - TestBloomFilterFalsePositiveRate < 1% at 1M entries — PQC-001
  ```

- [ ] **Step 44** [B] ~30s: Run tests (red phase)
  ```bash
  go test ./pkg/sophia/ -run TestPQC -v 2>&1 | tail -20
  ```

- [ ] **Step 45** [C]: **COMMIT CHECKPOINT** — PQC map definitions + test skeletons

- [ ] **Step 46** [W] ~15m: Implement PQC map operations

- [ ] **Step 47** [B] ~1m: Run tests (green phase)
  ```bash
  go test ./pkg/sophia/ -run TestPQC -v -timeout 60s 2>&1 | tail -20
  ```

- [ ] **Step 48** [V]: All Sophia PQC map tests pass
  - If fail → Step 49
  - If pass → Step 50

- [ ] **Step 49** [D]: Debug map operations — check struct alignment, byte order
  ```bash
  go test ./pkg/sophia/ -run TestPQC -v -count=1 2>&1
  ```

- [ ] **Step 50** [C]: **COMMIT CHECKPOINT** — Sophia PQC maps operational

### 2.2 SigRef Allocation with CSPRNG (PQC-002 Mitigation)

- [ ] **Step 51** [W] ~5m: Create `pkg/pqc/sigref_allocator.go`
  ```
  SigRefAllocator:
  - Uses crypto/rand for CSPRNG-based SigRef generation (NOT sequential!)
  - Tracks allocated SigRefs in bloom filter
  - Rejects collisions (regenerate on collision)
  - Thread-safe (sync.Mutex or atomic)
  - Rate-limited: max N allocations/sec per source (configurable, default 100)
  ```

- [ ] **Step 52** [W] ~5m: Create `pkg/pqc/sigref_allocator_test.go`
  ```
  Tests:
  - TestSigRefAllocatorRandomness: 10K allocations, no sequential pattern
  - TestSigRefAllocatorUniqueness: 100K allocations, zero collisions
  - TestSigRefAllocatorRateLimit: exceeding rate → ErrRateLimited
  - TestSigRefAllocatorThreadSafety: 100 goroutines allocating concurrently
  ```

- [ ] **Step 53** [B] ~1m: Run allocator tests
  ```bash
  go test ./pkg/pqc/ -run TestSigRefAllocator -v -race 2>&1 | tail -20
  ```

- [ ] **Step 54** [V]: All allocator tests pass with -race flag

- [ ] **Step 55** [C]: **COMMIT CHECKPOINT** — CSPRNG SigRef allocator (PQC-002)

### 2.3 Key Lifecycle Management

- [ ] **Step 56** [W] ~10m: Create `pkg/pqc/key_lifecycle.go`
  ```
  KeyLifecycleManager:
  - GenerateAndProvision(algoID) → (KeyRef, error)
  - RotateKey(oldKeyRef) → (newKeyRef, error)
    - Increments key_epoch
    - 60s grace period for old key
    - Cryptographic erasure of old key material (zeroize) — PQC-005 mitigation
  - RevokeKey(keyRef) error
    - Sets all associated sigs to verified=2
    - Emits Anamnesis event
  - GetCurrentEpoch(keyRef) uint8
  ```

- [ ] **Step 57** [W] ~5m: Create `pkg/pqc/key_lifecycle_test.go`
  ```
  Tests:
  - TestGenerateAndProvision
  - TestRotateKeyEpochIncrement
  - TestRotateKeyCryptographicErasure — verify old key bytes zeroed
  - TestRevokeKeyInvalidatesSignatures
  - TestKeyEpochRollbackRejected — PQC-005: epoch < current → error
  ```

- [ ] **Step 58** [B] ~1m: Run lifecycle tests
  ```bash
  go test ./pkg/pqc/ -run TestKey -v 2>&1 | tail -20
  ```

- [ ] **Step 59** [V]: All key lifecycle tests pass

- [ ] **Step 60** [C]: **COMMIT CHECKPOINT** — Key lifecycle with PQC-005 mitigations

### 2.4 Verification Policy Engine

- [ ] **Step 61** [W] ~10m: Create `pkg/pqc/verification_policy.go`
  ```
  VerificationPolicy enum: PESSIMISTIC, OPTIMISTIC, EXPERIMENTAL

  PolicyEngine:
  - GetPolicy(kingdomMode uint8) VerificationPolicy
  - ConfigureTrustBoundary(source string, policy VerificationPolicy)
  - DefaultPolicy() → PESSIMISTIC

  Kingdom Mode mapping (Section 9.3):
  - K1=0,K0=0 → PESSIMISTIC (default)
  - K1=0,K0=1 → OPTIMISTIC
  - K1=1,K0=0 → EXPERIMENTAL (log-only)
  - K1=1,K0=1 → RESERVED (reject)
  ```

- [ ] **Step 62** [W] ~5m: Create `pkg/pqc/verification_policy_test.go`

- [ ] **Step 63** [B] ~30s: Run policy tests
  ```bash
  go test ./pkg/pqc/ -run TestPolicy -v 2>&1 | tail -20
  ```

- [ ] **Step 64** [V]: Policy engine tests pass

- [ ] **Step 65** [C]: **COMMIT CHECKPOINT** — Verification policy engine

### 2.5 Separate PQC Ring Buffer (PQC-004 Mitigation)

- [ ] **Step 66** [R] ~2m: Read existing Anamnesis ring buffer implementation
  ```bash
  grep -rn "RingBuf\|ring_buf\|PACKET_EVENTS" --include="*.rs" --include="*.go" | head -20
  ```

- [ ] **Step 67** [W] ~10m: Create `pkg/pqc/ring_buffer.go`
  ```
  PQCRingBuffer:
  - Separate from Anamnesis trace ring buffer (PQC-004)
  - Default size: 64MB (1M events)
  - Priority queue: pre-provisioned SigRef entries get priority
  - Backpressure signal at 80% capacity
  - SubmitVerificationRequest(flowKey, sigRef, keyRef, seqNum, timestamp) error
  - DrainVerificationRequests(batchSize int) []VerificationRequest
  ```

- [ ] **Step 68** [W] ~5m: Create `pkg/pqc/ring_buffer_test.go`

- [ ] **Step 69** [B] ~30s: Run ring buffer tests
  ```bash
  go test ./pkg/pqc/ -run TestRingBuffer -v -race 2>&1 | tail -20
  ```

- [ ] **Step 70** [V]: Ring buffer tests pass with -race

- [ ] **Step 71** [C]: **COMMIT CHECKPOINT** — Separate PQC ring buffer (PQC-004)

### 2.6 Phase 2 Integration Test

- [ ] **Step 72** [W] ~5m: Create `pkg/pqc/integration_test.go`
  ```
  TestFullPQCFlow:
  1. Generate key pair (SLH-DSA-SHA2-128s)
  2. Provision key via KeyLifecycleManager
  3. Allocate SigRef via CSPRNG allocator
  4. Create pseudo-header
  5. Sign pseudo-header
  6. Provision signature in Sophia map
  7. Encode PQCValueLayout
  8. Decode PQCValueLayout
  9. Lookup signature by SigRef
  10. Verify signature
  11. Assert verified=1
  ```

- [ ] **Step 73** [B] ~2m: Run integration test
  ```bash
  go test ./pkg/pqc/ -run TestFullPQCFlow -v -timeout 120s 2>&1 | tail -20
  ```

- [ ] **Step 74** [V]: Integration test passes

- [ ] **Step 75** [V]: **PHASE 2 EXIT GATE** — All PQC package tests green, maps operational, allocator randomized, lifecycle managed, ring buffer separate
  ```bash
  go test ./pkg/pqc/... -v -race -timeout 180s 2>&1 | tail -15
  ```
  - All PASS → Phase 3+4 (parallel)
  - Any FAIL → DO NOT PROCEED

---

## PHASE 3: USERSPACE PQC VERIFIER DAEMON (Steps 76-110) [P]

**Goal**: Build the async verification daemon that reads from PQC ring buffer and performs SLH-DSA verification
**Prerequisite**: Phase 2 exit gate passed
**Time**: 2-3 hours
**Agent**: Developer [P] — parallelizable with Phase 4

### 3.1 Verifier Daemon Core

- [ ] **Step 76** [W] ~15m: Create `cmd/pqc-verifier/main.go`
  ```
  PQC Verifier Daemon:
  - Reads VerificationRequests from PQC ring buffer
  - Batch processing (configurable batch size, default 64)
  - For each request:
    1. Lookup signature from sophia_pqc_sigs
    2. Lookup public key from sophia_pqc_keys
    3. Construct pseudo-header
    4. SLH-DSA.Verify()
    5. Update verified field in map
  - Health check endpoint (HTTP /healthz)
  - Metrics: verifications/sec, avg latency, failure rate
  - Graceful shutdown on SIGTERM
  ```

- [ ] **Step 77** [W] ~10m: Create `cmd/pqc-verifier/main_test.go`

- [ ] **Step 78** [B] ~1m: Build verifier daemon
  ```bash
  go build -o /tmp/pqc-verifier ./cmd/pqc-verifier/ 2>&1 | tail -5
  ```

- [ ] **Step 79** [V]: Daemon builds without errors

- [ ] **Step 80** [C]: **COMMIT CHECKPOINT** — Verifier daemon skeleton

### 3.2 Health Check and Auto-Restart (PQC-008 Mitigation)

- [ ] **Step 81** [W] ~10m: Create `pkg/pqc/health.go`
  ```
  VerifierHealthMonitor:
  - 5-second health check interval
  - 2 missed checks → failure detected
  - On failure: mark all pending (verified=0) entries older than 10s as failed
  - Auto-restart via systemd or supervisor
  - Anamnesis ERROR event on failure
  ```

- [ ] **Step 82** [W] ~5m: Create health check tests

- [ ] **Step 83** [B] ~30s: Run health tests
  ```bash
  go test ./pkg/pqc/ -run TestHealth -v 2>&1 | tail -20
  ```

- [ ] **Step 84** [V]: Health check tests pass

- [ ] **Step 85** [C]: **COMMIT CHECKPOINT** — Health monitoring

### 3.3 Rate Limiting (PQC-001 Mitigation)

- [ ] **Step 86** [W] ~10m: Create `pkg/pqc/rate_limiter.go`
  ```
  VerificationRateLimiter:
  - Per-source-IP token bucket (100 verifications/sec default)
  - Global rate limit (10K verifications/sec default)
  - When exceeded: DROP unknown SigRef, log rate limit event
  - Bloom filter pre-check: reject SigRef not in bloom filter BEFORE rate limiting
  ```

- [ ] **Step 87** [W] ~5m: Rate limiter tests

- [ ] **Step 88** [B] ~30s: Run rate limiter tests
  ```bash
  go test ./pkg/pqc/ -run TestRateLimit -v 2>&1 | tail -20
  ```

- [ ] **Step 89** [V]: Rate limiter tests pass

- [ ] **Step 90** [C]: **COMMIT CHECKPOINT** — Rate limiting (PQC-001)

### 3.4 Control Plane Audit Trail (PQC-008 Defense-in-Depth)

- [ ] **Step 91** [W] ~10m: Create `pkg/pqc/audit.go`
  ```
  PQCAuditLog:
  - Logs every Sophia map write (key provision, sig provision, verification update)
  - Separate Anamnesis ring buffer (not shared with data plane or PQC verification)
  - Includes: timestamp, operation, sigRef/keyRef, caller identity
  - Tamper-evident: each entry includes HMAC of previous entry (hash chain)
  ```

- [ ] **Step 92** [W] ~5m: Audit log tests

- [ ] **Step 93** [B] ~30s: Run audit tests
  ```bash
  go test ./pkg/pqc/ -run TestAudit -v 2>&1 | tail -20
  ```

- [ ] **Step 94** [V]: Audit tests pass

- [ ] **Step 95** [C]: **COMMIT CHECKPOINT** — Audit trail (PQC-008)

### 3.5 Timing Normalization (PQC-003 Mitigation)

- [ ] **Step 96** [W] ~5m: Create `pkg/pqc/timing.go`
  ```
  NormalizeResponseTiming:
  - Pad DROP path to match PASS path timing (~265ns target)
  - Add ±500μs jitter to PESSIMISTIC hold duration
  - Uses time.Sleep with crypto/rand jitter source
  ```

- [ ] **Step 97** [W] ~5m: Timing normalization tests

- [ ] **Step 98** [B] ~30s: Run timing tests
  ```bash
  go test ./pkg/pqc/ -run TestTiming -v 2>&1 | tail -20
  ```

- [ ] **Step 99** [V]: Timing tests pass

- [ ] **Step 100** [C]: **COMMIT CHECKPOINT** — Timing normalization (PQC-003)

### 3.6 Full Verifier Integration Test

- [ ] **Step 101** [W] ~10m: Create `cmd/pqc-verifier/integration_test.go`
  ```
  TestVerifierDaemonFullCycle:
  1. Start daemon
  2. Provision key + signature
  3. Submit verification request via ring buffer
  4. Wait for verification result
  5. Assert verified=1 in Sophia map
  6. Submit request with wrong key → assert verified=2
  7. Check health endpoint responds
  8. Check audit log has entries
  9. Test rate limiting under burst
  10. Graceful shutdown
  ```

- [ ] **Step 102** [B] ~5m: Run verifier integration test
  ```bash
  go test ./cmd/pqc-verifier/ -run TestVerifierDaemon -v -timeout 300s 2>&1 | tail -30
  ```

- [ ] **Step 103** [V]: Verifier integration test passes

- [ ] **Step 104** [B] ~30s: Run full PQC package test suite
  ```bash
  go test ./pkg/pqc/... ./cmd/pqc-verifier/... -v -race -timeout 300s 2>&1 | tail -20
  ```

- [ ] **Step 105** [V]: All PQC tests pass with -race

- [ ] **Step 106** [C]: **COMMIT CHECKPOINT** — Verifier daemon complete

- [ ] **Step 107** [B] ~1m: Benchmark verification throughput
  ```bash
  go test ./pkg/pqc/ -bench=BenchmarkSLHDSAVerify -benchtime=10s 2>&1 | tail -10
  ```

- [ ] **Step 108** [R]: Record verification throughput for Appendix B validation

- [ ] **Step 109** [B] ~30s: Run gosec on PQC package
  ```bash
  gosec ./pkg/pqc/... 2>&1 | tail -20
  ```

- [ ] **Step 110** [V]: **PHASE 3 EXIT GATE** — Verifier daemon builds, tests green, rate limited, health checked, audit logged, gosec clean
  - All PASS → Phase 5 (after Phase 4 also passes)
  - Any FAIL → DO NOT PROCEED

---

## PHASE 4: WOTAN PQC STATE INTEGRATION (Steps 111-140) [P]

**Goal**: Reserve Wotan address space for PQC state, implement SeqNum management with replay protection
**Prerequisite**: Phase 2 exit gate passed
**Time**: 2 hours
**Agent**: Developer [P] — parallelizable with Phase 3

### 4.1 Wotan PQC Address Reservation

- [ ] **Step 111** [R] ~2m: Read current Wotan address space allocation
  ```bash
  grep -rn "0x0000FF\|WotanAddr\|WOTAN_ADDR" --include="*.go" | head -20
  ```

- [ ] **Step 112** [W] ~10m: Create `pkg/wotan/pqc_state.go`
  ```
  PQC address space (0x0000FF00 - 0x0000FF0F):
  - 0x0000FF00: last_seen_seq (uint32) — replay counter
  - 0x0000FF04: pqc_verified (uint8) — 0=no, 1=yes, 2=failed
  - 0x0000FF05: pqc_algo_id (uint8)
  - 0x0000FF06: pqc_key_epoch (uint8) — PQC-005 mitigation
  - 0x0000FF07: reserved
  - 0x0000FF08: pqc_verify_count (uint32)
  - 0x0000FF0C: pqc_fail_count (uint32)
  - 0x0000FF10: pqc_key_fingerprint[32] (SHA-256) — PQC-008 key pinning

  Functions:
  - InitPQCState(flowLabel uint32, algoID, keyEpoch uint8, fingerprint [32]byte) error
  - ReadLastSeenSeq(flowLabel uint32) (uint32, error)
  - CASLastSeenSeq(flowLabel, expected, newVal uint32) error
  - CheckKeyEpoch(flowLabel uint32, epoch uint8) error — PQC-005: reject if epoch < stored
  - PinKeyFingerprint(flowLabel uint32, fp [32]byte) error — PQC-008
  - VerifyKeyFingerprint(flowLabel uint32, fp [32]byte) (bool, error) — PQC-008
  ```

- [ ] **Step 113** [W] ~10m: Create `pkg/wotan/pqc_state_test.go`
  ```
  Tests:
  - TestInitPQCState
  - TestSeqNumReplayDetection: send seq 5, then seq 3 → rejected
  - TestSeqNumCASConcurrency: 10 goroutines racing → exactly one wins
  - TestKeyEpochEnforcement: epoch 3, send epoch 2 → rejected (PQC-005)
  - TestKeyFingerprintPinning: pin fp, check with correct → true (PQC-008)
  - TestKeyFingerprintMismatch: pin fp, check with wrong → false + alert (PQC-008)
  - TestSeqNumWraparound: max uint32 → behavior defined (PQC-006)
  ```

- [ ] **Step 114** [B] ~30s: Run tests (red)
  ```bash
  go test ./pkg/wotan/ -run TestPQC -v 2>&1 | tail -20
  ```

- [ ] **Step 115** [C]: **COMMIT CHECKPOINT** — Wotan PQC state definitions

- [ ] **Step 116** [W] ~15m: Implement Wotan PQC state operations

- [ ] **Step 117** [B] ~1m: Run tests (green)
  ```bash
  go test ./pkg/wotan/ -run TestPQC -v -race 2>&1 | tail -20
  ```

- [ ] **Step 118** [V]: All Wotan PQC tests pass with -race

- [ ] **Step 119** [D]: If race detected — check CAS implementation
  ```bash
  go test ./pkg/wotan/ -run TestSeqNumCAS -v -race -count=10 2>&1
  ```

- [ ] **Step 120** [C]: **COMMIT CHECKPOINT** — Wotan PQC state operational

### 4.2 SeqNum Wraparound Handling (PQC-006 Mitigation)

- [ ] **Step 121** [W] ~5m: Add wraparound detection to `pkg/wotan/pqc_state.go`
  ```
  SeqNumWraparoundPolicy:
  - When SeqNum > 0xFFFF0000: emit WARNING event
  - When SeqNum > 0xFFFFFFF0: trigger mandatory key rotation
  - Signal KeyLifecycleManager to rotate key + reassign SigRef
  - New flow authentication context starts with SeqNum=0
  ```

- [ ] **Step 122** [W] ~5m: Wraparound tests

- [ ] **Step 123** [B] ~30s: Run wraparound tests
  ```bash
  go test ./pkg/wotan/ -run TestSeqNumWrap -v 2>&1 | tail -20
  ```

- [ ] **Step 124** [V]: Wraparound tests pass

- [ ] **Step 125** [C]: **COMMIT CHECKPOINT** — SeqNum wraparound handling (PQC-006)

### 4.3 Sliding Window for Reordering Tolerance

- [ ] **Step 126** [W] ~10m: Add sliding window to SeqNum checker
  ```
  SlidingWindow:
  - Window size: 64 packets (configurable)
  - Bitmap tracks which SeqNum values within window have been seen
  - SeqNum > window_top: advance window, accept
  - SeqNum within window AND unseen: accept, mark seen
  - SeqNum within window AND seen: DROP (duplicate)
  - SeqNum < window_bottom: DROP (too old)
  ```

- [ ] **Step 127** [W] ~5m: Sliding window tests

- [ ] **Step 128** [B] ~30s: Run sliding window tests
  ```bash
  go test ./pkg/wotan/ -run TestSlidingWindow -v 2>&1 | tail -20
  ```

- [ ] **Step 129** [V]: Sliding window tests pass

- [ ] **Step 130** [C]: **COMMIT CHECKPOINT** — Sliding window replay protection

### 4.4 Phase 4 Integration

- [ ] **Step 131** [W] ~5m: Create `pkg/wotan/pqc_integration_test.go`
  ```
  TestWotanPQCFullFlow:
  1. Init PQC state for flow
  2. Pin key fingerprint
  3. Process 100 packets with incrementing SeqNum → all accepted
  4. Replay packet with old SeqNum → rejected
  5. Process out-of-order packets within window → accepted
  6. Process out-of-order packets outside window → rejected
  7. Verify key epoch enforcement
  8. Trigger wraparound → key rotation signal emitted
  ```

- [ ] **Step 132** [B] ~1m: Run integration test
  ```bash
  go test ./pkg/wotan/ -run TestWotanPQCFullFlow -v -race 2>&1 | tail -20
  ```

- [ ] **Step 133** [V]: Integration test passes

- [ ] **Step 134** [C]: **COMMIT CHECKPOINT** — Wotan PQC integration complete

- [ ] **Step 135-139**: [Reserved for debug/iteration]

- [ ] **Step 140** [V]: **PHASE 4 EXIT GATE** — Wotan PQC state operational, replay protected, key epoch enforced, fingerprint pinned, sliding window functional, wraparound handled
  ```bash
  go test ./pkg/wotan/... -v -race -timeout 120s 2>&1 | tail -15
  ```
  - All PASS → Phase 5
  - Any FAIL → DO NOT PROCEED

---

## PHASE 5: XDP FAST PATH INTEGRATION (Steps 141-165)

**Goal**: Integrate PQC verification into the XDP eBPF fast path (Rust/Aya)
**Prerequisite**: Phase 3 AND Phase 4 exit gates passed
**Time**: 3-4 hours
**Agent**: Developer (Coordinator — may need iterative eBPF debugging)

### 5.1 eBPF PQC Constants and Structs

- [ ] **Step 141** [R] ~2m: Read existing XDP program structure
  ```bash
  find . -path "*/ebpf*" -name "*.rs" | head -10 && head -50 ebpf/packet-marker/src/main.rs
  ```

- [ ] **Step 142** [W] ~15m: Add PQC structures to eBPF Rust code
  ```
  In ebpf/packet-marker/src/pqc.rs:
  - PQCSigEntry (C-compatible struct matching Go side)
  - PQCKeyEntry (C-compatible struct matching Go side)
  - PQC_SIG_MAP: HashMap<u32, PQCSigEntry>  (BPF_F_RDONLY_PROG)
  - PQC_KEY_MAP: HashMap<u32, PQCKeyEntry>  (BPF_F_RDONLY_PROG)
  - PQC_BLOOM: BloomFilter map for SigRef pre-check
  - PQC_EVENTS: RingBuf for verification requests (SEPARATE from PACKET_EVENTS)
  ```

- [ ] **Step 143** [B] ~2m: Compile eBPF with new maps
  ```bash
  cd ebpf && cargo build 2>&1 | tail -20
  ```

- [ ] **Step 144** [V]: eBPF compiles with PQC maps
  - If fail → Step 145
  - If pass → Step 146

- [ ] **Step 145** [D]: Fix struct alignment — eBPF requires packed/repr(C)

- [ ] **Step 146** [C]: **COMMIT CHECKPOINT** — eBPF PQC structures

### 5.2 XDP Fast Path Logic

- [ ] **Step 147** [W] ~20m: Add PQC verification to XDP main path
  ```rust
  // In try_packet_marker():
  // After parsing Monad register:

  if monad.flags & FLAG_SIGNED != 0 {
      let pqc_value = PQCValueLayout::decode(&monad.value);

      // PQC-001: Bloom filter pre-check
      if !bloom_check(&PQC_BLOOM, pqc_value.sig_ref) {
          increment_stat(STAT_PQC_UNKNOWN_SIGREF);
          return Ok(xdp_action::XDP_DROP);  // Unknown SigRef → instant DROP
      }

      // Fast path: cached verification lookup
      if let Some(sig_entry) = unsafe { PQC_SIG_MAP.get(&pqc_value.sig_ref) } {
          // PQC-002: flow_label binding check
          if sig_entry.flow_label != flow_key.flow_label {
              // Flow label mismatch → cache miss, submit to slow path
              submit_pqc_verification(&PQC_EVENTS, &flow_key, &pqc_value);
          } else if sig_entry.verified == 1 {
              // Verified! Check SeqNum via Wotan
              // (SeqNum check delegated to userspace for eBPF complexity budget)
              increment_stat(STAT_PQC_CACHE_HIT);
              return Ok(xdp_action::XDP_PASS);
          } else if sig_entry.verified == 2 {
              increment_stat(STAT_PQC_REJECTED);
              return Ok(xdp_action::XDP_DROP);
          } else {
              // verified == 0 (pending) → apply verification policy
              // Kingdom Mode bits determine policy
              let policy = (monad.flags & 0x03); // K1|K0
              if policy == 0x01 { // OPTIMISTIC
                  return Ok(xdp_action::XDP_PASS);
              }
              // PESSIMISTIC: hold (submit to ring, let userspace decide)
              submit_pqc_verification(&PQC_EVENTS, &flow_key, &pqc_value);
          }
      } else {
          // SigRef in bloom but not in map → submit for verification
          submit_pqc_verification(&PQC_EVENTS, &flow_key, &pqc_value);
      }
  }
  ```

- [ ] **Step 148** [B] ~3m: Compile eBPF with fast path
  ```bash
  cd ebpf && cargo build 2>&1 | tail -20
  ```

- [ ] **Step 149** [V]: eBPF compiles — verifier complexity budget check
  - If fail → Step 150
  - If pass → Step 151

- [ ] **Step 150** [D]: If verifier rejects — simplify logic, reduce branches, use tail calls

- [ ] **Step 151** [C]: **COMMIT CHECKPOINT** — XDP PQC fast path

### 5.3 eBPF Unit Tests

- [ ] **Step 152** [W] ~10m: Create eBPF PQC test harness
  ```
  Tests (Rust unit tests within eBPF crate):
  - test_pqc_value_decode
  - test_bloom_filter_check
  - test_sig_entry_lookup
  - test_verification_policy_selection
  ```

- [ ] **Step 153** [B] ~1m: Run eBPF unit tests
  ```bash
  cd ebpf && cargo test 2>&1 | tail -20
  ```

- [ ] **Step 154** [V]: eBPF tests pass

- [ ] **Step 155** [C]: **COMMIT CHECKPOINT** — eBPF PQC tests

### 5.4 Go ↔ eBPF Map Interop Tests

- [ ] **Step 156** [W] ~10m: Create `pkg/pqc/ebpf_interop_test.go`
  ```
  Tests:
  - TestGoWriteEbpfRead: Go provisions sig entry, eBPF reads correctly
  - TestEbpfStatsIncrement: XDP increments PQC counters, Go reads them
  - TestBloomFilterInterop: Go adds to bloom, eBPF checks
  ```

- [ ] **Step 157** [B] ~2m: Run interop tests (requires BPF-capable kernel)
  ```bash
  go test ./pkg/pqc/ -run TestEbpf -v -timeout 120s 2>&1 | tail -20
  ```

- [ ] **Step 158** [V]: Interop tests pass (or SKIP if no BPF kernel)

- [ ] **Step 159** [C]: **COMMIT CHECKPOINT** — Go/eBPF interop verified

- [ ] **Step 160-164**: [Reserved for debug/iteration on eBPF verifier issues]

- [ ] **Step 165** [V]: **PHASE 5 EXIT GATE** — XDP fast path compiles, bloom filter pre-check works, cached verification lookup works, verification policy applied, ring buffer submission works
  - All PASS → Phase 6
  - Any FAIL → DO NOT PROCEED

---

## PHASE 6A: SHIELD INTEGRATION + HEADER STRIPPING (Steps 166-195)

**Goal**: Integrate PQC verification into Shield, STRIP Monad HbH headers at ingress, persist PQC state to Wotan, RE-STAMP at egress
**Prerequisite**: Phase 5 exit gate passed
**Time**: 3 hours
**Agent**: Developer
**KEY ARCHITECTURAL CHANGE**: PQC wire-level auth is PERIMETER ONLY. Headers stripped at ingress. Internal kingdom = zero PQC wire overhead.

### 6A.1 Shield Ingress — Verify + Strip

- [ ] **Step 166** [R] ~2m: Read current Shield pipeline code
  ```bash
  find . -path "*shield*" -name "*.go" | head -20
  ```

- [ ] **Step 167** [W] ~20m: Add PQC to Shield ingress path with header stripping
  ```
  In shield/ingress.go:
  After CRC-16 validation:
  1. Check S flag
  2. If S=1: extract PQCValueLayout from Monad value
  3. Validate SigRef != 0, KeyRef != 0
  4. Check bloom filter (PQC-001)
  5. Check Sophia cached result
  6. Apply verification policy
  7. If PESSIMISTIC + uncached: hold packet, submit to verifier
  8. If OPTIMISTIC + uncached: forward, submit async
  9. On verification SUCCESS:
     a. Write Wotan per-flow PQC state:
        - 0x0000FF04: pqc_verified = 1
        - 0x0000FF05: pqc_algo_id from sig entry
        - 0x0000FF06: pqc_key_epoch from key entry
        - 0x0000FF10: truncated key fingerprint (8 bytes)
        - 0x0000FF18: verification timestamp (nanos)
        - 0x0000FF20: key creation epoch (seconds)
     b. STRIP Monad Hop-by-Hop extension header from packet
     c. Forward stripped packet into kingdom
  10. Log Anamnesis BORN event with PQC metadata
  ```

- [ ] **Step 168** [W] ~10m: Implement header stripping function
  ```
  In shield/strip.go:
  StripMonadHbH(packet []byte) ([]byte, error):
  - Remove IPv6 HbH extension header containing Monad register
  - Adjust IPv6 Next Header field to skip HbH
  - Adjust IPv6 Payload Length
  - Return stripped packet
  - MUST be constant-time (no timing oracle on strip vs no-strip)
  ```

- [ ] **Step 169** [W] ~5m: Header stripping tests
  ```
  Tests:
  - TestStripMonadHbH: packet with HbH → stripped, valid IPv6
  - TestStripMonadHbHNoHbH: packet without HbH → no-op
  - TestStripMonadHbHPayloadLength: payload length adjusted correctly
  - TestStripMonadHbHNextHeader: next header chain preserved
  ```

- [ ] **Step 170** [B] ~1m: Run header stripping tests
  ```bash
  go test ./shield/ -run TestStripMonad -v 2>&1 | tail -20
  ```

- [ ] **Step 171** [V]: Header stripping tests pass

- [ ] **Step 172** [C]: **COMMIT CHECKPOINT** — Shield ingress + header stripping

### 6A.2 Shield Egress — Re-stamp

- [ ] **Step 173** [W] ~15m: Add PQC to Shield egress path with re-stamping
  ```
  In shield/egress.go:
  1. Read Wotan per-flow PQC state
  2. If pqc_verified == 1:
     a. Re-stamp fresh Monad HbH extension header
     b. Populate PQCValueLayout: current SigRef, KeyRef, incremented SeqNum, fresh HashPfx
     c. Set S flag in Monad flags
  3. Zero Kingdom Mode bits (existing behavior)
  4. Log Anamnesis DEATH event with PQC metadata
  ```

- [ ] **Step 174** [W] ~10m: Implement header re-stamping function
  ```
  In shield/restamp.go:
  RestampMonadHbH(packet []byte, wotanState PQCState) ([]byte, error):
  - Insert IPv6 HbH extension header with fresh Monad register
  - Populate from Wotan PQC state + fresh SeqNum
  - Adjust IPv6 Next Header and Payload Length
  - Compute CRC-16
  ```

- [ ] **Step 175** [W] ~5m: Re-stamping tests
  ```
  Tests:
  - TestRestampMonadHbH: stripped packet → re-stamped, valid Monad
  - TestRestampSeqNumIncrement: SeqNum > last egress SeqNum
  - TestRestampCRC16Valid: CRC-16 recomputed correctly
  - TestRestampKingdomModeZeroed: K1|K0 cleared on egress
  ```

- [ ] **Step 176** [B] ~1m: Run re-stamping tests
  ```bash
  go test ./shield/ -run TestRestamp -v 2>&1 | tail -20
  ```

- [ ] **Step 177** [V]: Re-stamping tests pass

- [ ] **Step 178** [C]: **COMMIT CHECKPOINT** — Shield egress re-stamping

### 6A.3 Wotan PQC State Persistence (Layer 2 Foundation)

- [ ] **Step 179** [W] ~10m: Extend `pkg/wotan/pqc_state.go` with Layer 2 fields
  ```
  New Wotan addresses (in addition to existing 0xFF00-0xFF0F):
  - 0x0000FF10: pqc_key_fp (8 bytes) — truncated SHA-256 of pubkey
  - 0x0000FF18: pqc_verify_ts (8 bytes) — verification timestamp nanos
  - 0x0000FF20: pqc_key_created (4 bytes) — key creation epoch seconds
  - 0x0000FF24: pqc_app_policy_id (4 bytes) — Layer 2 policy ref (0=none)

  Functions:
  - PersistPQCState(flowLabel uint32, state PQCVerificationResult) error
  - ReadFullPQCState(flowLabel uint32) (PQCFullState, error)
  ```

- [ ] **Step 180** [W] ~5m: Layer 2 foundation tests
  ```
  Tests:
  - TestPersistPQCState: write all fields, read back
  - TestReadFullPQCState: all fields present after persist
  - TestPQCKeyFingerprintTruncation: SHA-256 → 8 bytes
  - TestPQCVerifyTimestamp: nanosecond precision
  ```

- [ ] **Step 181** [B] ~30s: Run Layer 2 foundation tests
  ```bash
  go test ./pkg/wotan/ -run TestPersist -v -race 2>&1 | tail -20
  ```

- [ ] **Step 182** [V]: Layer 2 foundation tests pass

- [ ] **Step 183** [C]: **COMMIT CHECKPOINT** — Wotan Layer 2 fields

### 6A.4 Full Shield Integration Test

- [ ] **Step 184** [W] ~10m: Shield PQC integration tests
  ```
  Tests:
  - TestShieldIngressSFlagSet: packet with S=1 triggers PQC pipeline
  - TestShieldIngressSFlagUnset: packet without S=1 bypasses PQC
  - TestShieldIngressInvalidSigRef: SigRef=0 with S=1 → DROP
  - TestShieldIngressPessimistic: uncached → held
  - TestShieldIngressOptimistic: uncached → forwarded
  - TestShieldIngressCachedValid: verified=1 → PASS + strip headers
  - TestShieldIngressCachedInvalid: verified=2 → DROP
  - TestShieldIngressWotanPersistence: after verify, Wotan has PQC state
  - TestShieldEgressRestamped: outbound packet has fresh Monad HbH
  - TestShieldEgressKingdomModeZeroed: K1|K0 cleared
  - TestShieldRoundTrip: ingress strip → internal (no HbH) → egress restamp
  ```

- [ ] **Step 185** [B] ~2m: Run Shield PQC tests
  ```bash
  go test ./shield/... -run TestShield.*PQC -v -race 2>&1 | tail -30
  ```

- [ ] **Step 186** [V]: Shield PQC tests pass

- [ ] **Step 187** [C]: **COMMIT CHECKPOINT** — Shield PQC integration complete

- [ ] **Step 188-194**: [Reserved for Shield integration edge cases and debug]

- [ ] **Step 195** [V]: **PHASE 6A EXIT GATE** — Shield ingress verifies + strips, egress re-stamps, Wotan persists PQC state, internal traffic has no HbH, -race clean
  ```bash
  go test ./shield/... ./pkg/wotan/... -v -race -timeout 180s 2>&1 | tail -15
  ```
  - All PASS → Phase 6B + Phase 7 (parallel)
  - Any FAIL → DO NOT PROCEED

---

## PHASE 6B: LAYER 2 APPLICATION POLICY (Steps 196-225) [P]

**Goal**: Build Sophia application policy dictionary system for optional app-level PQC verification
**Prerequisite**: Phase 6A exit gate passed (Wotan PQC state available)
**Time**: 2-3 hours
**Agent**: Developer [P] — parallelizable with Phase 7
**BUSINESS CONTEXT**: This is the open-core monetization wedge. Wire L1 = Apache-2.0. App policy L2 = enterprise feature.

### 6B.1 Sophia Application Policy Map

- [ ] **Step 196** [W] ~10m: Create `pkg/pqc/app_policy.go`
  ```
  sophia_pqc_app_policy BPF hash map:
  - Key: uint32 (application_id)
  - Value: struct sophia_pqc_policy
  - Max entries: 4,096
  - Flags: BPF_F_RDONLY_PROG
  - Pinned: /sys/fs/bpf/sophia/pqc_app_policy

  struct sophia_pqc_policy {
      min_security_level uint8    // NIST level: 1, 3, or 5
      require_pinned_key uint8    // 0=no, 1=yes
      num_allowed_algos  uint8    // count of allowed algo_ids
      reserved           uint8
      max_key_age_sec    uint32   // max key epoch age in seconds
      allowed_algos      [12]uint8 // up to 12 allowed algo_id values
      pinned_fp          [32]byte  // expected key fingerprint
  }

  Functions:
  - LoadAppPolicy(appID uint32, policy PQCPolicy) error
  - ReadAppPolicy(appID uint32) (PQCPolicy, error)
  - DeleteAppPolicy(appID uint32) error
  - ListAppPolicies() ([]uint32, error)
  ```

- [ ] **Step 197** [W] ~5m: Create `pkg/pqc/app_policy_test.go` — TDD red
  ```
  Tests:
  - TestLoadAppPolicy: provision, read back, match
  - TestReadAppPolicyNotFound: missing → error
  - TestDeleteAppPolicy: delete, read → not found
  - TestListAppPolicies: load 5, list → 5 IDs
  - TestAppPolicyMaxEntries: 4096 → OK, 4097 → error
  ```

- [ ] **Step 198** [B] ~30s: Run tests (red)
  ```bash
  go test ./pkg/pqc/ -run TestAppPolicy -v 2>&1 | tail -20
  ```

- [ ] **Step 199** [V]: Tests fail as expected (red)

- [ ] **Step 200** [W] ~10m: Implement app policy CRUD

- [ ] **Step 201** [B] ~30s: Run tests (green)
  ```bash
  go test ./pkg/pqc/ -run TestAppPolicy -v -race 2>&1 | tail -20
  ```

- [ ] **Step 202** [V]: App policy CRUD tests pass

- [ ] **Step 203** [C]: **COMMIT CHECKPOINT** — Sophia app policy map

### 6B.2 Layer 2 Verification Engine

- [ ] **Step 204** [W] ~15m: Create `pkg/pqc/layer2_verifier.go`
  ```
  Layer2Verifier:
  - VerifyAppPolicy(flowLabel uint32, appID uint32) (Layer2Result, error)
    1. Read pqc_verified from Wotan 0xFF04 — if != 1, reject (L1 must pass)
    2. Read pqc_algo_id from Wotan 0xFF05
    3. Lookup policy from sophia_pqc_app_policy[appID]
    4. If policy.num_allowed_algos > 0: check algo_id in allowed set
    5. Map algo_id to NIST level, check >= min_security_level
    6. If require_pinned_key: read Wotan 0xFF10, compare with policy.pinned_fp[0:8]
    7. If max_key_age_sec > 0: read Wotan 0xFF20, check age
    8. All pass → accept; any fail → reject with reason code

  Layer2Result:
  - Accepted bool
  - RejectReason string (empty if accepted)
  - ChecksPerformed []string (audit trail)
  - VerificationTimeNs uint64
  ```

- [ ] **Step 205** [W] ~10m: Create `pkg/pqc/layer2_verifier_test.go`
  ```
  Tests:
  - TestLayer2AcceptValidFlow: L1 passed, meets policy → accept
  - TestLayer2RejectNoL1: pqc_verified != 1 → reject "Layer 1 not verified"
  - TestLayer2RejectWeakAlgo: Level 1 algo, policy requires Level 3 → reject
  - TestLayer2RejectDisallowedAlgo: algo not in allowed set → reject
  - TestLayer2RejectFingerprintMismatch: pinned key, wrong fp → reject
  - TestLayer2RejectKeyTooOld: key age > max_key_age_sec → reject
  - TestLayer2AcceptNoPolicy: no policy loaded → accept (Layer 2 is optional)
  - TestLayer2MultiplePoliciesSameFlow: app A accepts, app B rejects → both correct
  - TestLayer2Performance: < 500ns per verification
  ```

- [ ] **Step 206** [B] ~30s: Run tests (red)
  ```bash
  go test ./pkg/pqc/ -run TestLayer2 -v 2>&1 | tail -20
  ```

- [ ] **Step 207** [V]: Tests fail as expected

- [ ] **Step 208** [W] ~15m: Implement Layer2Verifier

- [ ] **Step 209** [B] ~30s: Run tests (green)
  ```bash
  go test ./pkg/pqc/ -run TestLayer2 -v -race 2>&1 | tail -20
  ```

- [ ] **Step 210** [V]: Layer 2 verifier tests pass with -race

- [ ] **Step 211** [C]: **COMMIT CHECKPOINT** — Layer 2 verification engine

### 6B.3 Sophia Dictionary Definition Parser

- [ ] **Step 212** [W] ~10m: Create `pkg/pqc/policy_parser.go`
  ```
  ParseAppPolicy(dictDef string) (PQCPolicy, error):
  - Parses Sophia dictionary syntax:
    dictionary "enterprise-auth-policy" {
        field pqc_min_security_level  uint8   = 3;
        field pqc_require_pinned_key  uint8   = 1;
        field pqc_max_key_age_sec     uint32  = 86400;
        field pqc_allowed_algos       uint8[] = [0x03, 0x04, 0x09, 0x0A];
        field pqc_pinned_fp           bytes32 = 0xa1b2c3...;
    }
  - Returns PQCPolicy struct
  - Validates all fields, rejects unknown fields
  ```

- [ ] **Step 213** [W] ~5m: Parser tests
  ```
  Tests:
  - TestParsePolicyValid: full policy → correct struct
  - TestParsePolicyMinimal: only min_security_level → defaults for rest
  - TestParsePolicyInvalidLevel: level 4 → error
  - TestParsePolicyBadSyntax: missing semicolons → error
  - TestParsePolicyUnknownField: extra field → error
  ```

- [ ] **Step 214** [B] ~30s: Run parser tests
  ```bash
  go test ./pkg/pqc/ -run TestParsePolicy -v 2>&1 | tail -20
  ```

- [ ] **Step 215** [V]: Parser tests pass

- [ ] **Step 216** [C]: **COMMIT CHECKPOINT** — Policy parser

### 6B.4 Layer 2 Integration Test

- [ ] **Step 217** [W] ~10m: Create `pkg/pqc/layer2_integration_test.go`
  ```
  TestLayer2FullFlow:
  1. Simulate Shield ingress: verify L1, persist Wotan PQC state
  2. Load enterprise policy (Level 3, pinned key, 24h max age)
  3. Verify flow against policy → accept (algo=SHA2-192s, key fresh, fp matches)
  4. Modify policy to Level 5 → reverify → reject (algo only Level 3)
  5. Load different app policy (Level 1, no pinning) → accept
  6. Verify two apps see different results for same flow
  7. Verify audit trail has all checks recorded
  ```

- [ ] **Step 218** [B] ~1m: Run integration test
  ```bash
  go test ./pkg/pqc/ -run TestLayer2Full -v -race 2>&1 | tail -20
  ```

- [ ] **Step 219** [V]: Layer 2 integration test passes

- [ ] **Step 220** [B] ~30s: Benchmark Layer 2 verification
  ```bash
  go test ./pkg/pqc/ -bench=BenchmarkLayer2Verify -benchtime=5s 2>&1 | tail -5
  ```

- [ ] **Step 221** [V]: Layer 2 verification < 500ns (target: ~200-300ns)

- [ ] **Step 222** [C]: **COMMIT CHECKPOINT** — Layer 2 integration complete

- [ ] **Step 223-224**: [Reserved for Layer 2 edge cases]

- [ ] **Step 225** [V]: **PHASE 6B EXIT GATE** — App policy CRUD operational, Layer 2 verifier tested, parser working, two-app divergent policy test passes, performance meets target
  ```bash
  go test ./pkg/pqc/ -run "TestAppPolicy|TestLayer2|TestParsePolicy" -v -race -timeout 120s 2>&1 | tail -15
  ```
  - All PASS → Phase 8 (after Phase 7 also passes)
  - Any FAIL → DO NOT PROCEED

---

## PHASE 7: BLACKMAGE MANDATORY HARDENING (Steps 226-260)

**Goal**: Implement and verify all mandatory mitigations from BlackMage findings + header stripping threat model update
**Prerequisite**: Phase 6A exit gate passed
**Time**: 2-3 hours
**Agent**: Developer + BlackMage (adversarial verification)
**NOTE**: Header stripping reduces blast radius for PQC-001, PQC-002, PQC-003 (perimeter-only). PQC-008 remains CRITICAL.

### 7.1 Verification Checklist

- [ ] **Step 226** [V]: **PQC-001 — SigRef Exhaustion**: Bloom filter pre-check implemented? (Perimeter-only after header stripping)
  ```bash
  grep -rn "bloom\|BloomFilter\|bloom_check" --include="*.go" --include="*.rs" | wc -l
  ```
  - Must be > 0 in both Go and Rust

- [ ] **Step 227** [V]: **PQC-001 — Rate Limiting**: Per-source rate limit implemented?
  ```bash
  grep -rn "RateLimit\|rate_limit\|token_bucket" --include="*.go" | wc -l
  ```

- [ ] **Step 228** [V]: **PQC-002 — CSPRNG SigRef**: No sequential allocation? (Perimeter-only after header stripping)
  ```bash
  grep -rn "crypto/rand\|CSPRNG\|SecureRandom" --include="*.go" pkg/pqc/sigref_allocator.go | wc -l
  ```

- [ ] **Step 229** [V]: **PQC-002 — Flow Label Binding**: flow_label checked on cache hit?
  ```bash
  grep -rn "flow_label\|FlowLabel" --include="*.rs" ebpf/ | head -10
  ```

- [ ] **Step 230** [C]: **COMMIT CHECKPOINT** — hardening verification pass 1

- [ ] **Step 231** [V]: **PQC-004 — Separate Ring Buffer**: PQC events NOT in PACKET_EVENTS?
  ```bash
  grep -rn "PQC_EVENTS\|pqc_events" --include="*.rs" | head -5
  ```

- [ ] **Step 232** [V]: **PQC-005 — Key Epoch Check**: epoch < stored → reject?
  ```bash
  grep -rn "key_epoch\|KeyEpoch\|CheckKeyEpoch" --include="*.go" | head -10
  ```

- [ ] **Step 233** [V]: **PQC-005 — Cryptographic Erasure**: old keys zeroized?
  ```bash
  grep -rn "zeroize\|Zeroize\|memset.*0\|CryptographicErase" --include="*.go" | head -5
  ```

- [ ] **Step 234** [V]: **PQC-008 — Key Pinning in Wotan**: fingerprint stored + checked?
  ```bash
  grep -rn "PinKeyFingerprint\|VerifyKeyFingerprint\|pqc_key_fingerprint" --include="*.go" | head -5
  ```

- [ ] **Step 235** [C]: **COMMIT CHECKPOINT** — hardening verification pass 2

### 7.2 Header Stripping Threat Model Validation

- [ ] **Step 236** [V]: **Header Stripping Isolation**: internal packets have no Monad HbH?
  ```bash
  # Verify no HbH present after Shield ingress in test captures
  go test ./shield/ -run TestInternalNoHbH -v 2>&1 | tail -10
  ```

- [ ] **Step 237** [V]: **Perimeter Confinement**: PQC attack vectors only exploitable from external?
  ```
  Verify:
  - SigRef exhaustion: requires Monad HbH → external only (headers stripped internally)
  - Cache poisoning: requires SigRef in wire → external only
  - Timing oracle: verification only at Shield → external only
  - Control plane compromise: NOT confined (still CRITICAL)
  ```

- [ ] **Step 238** [C]: **COMMIT CHECKPOINT** — header stripping threat model validated

### 7.3 Adversarial Testing (Lich Mini-Campaign)

- [ ] **Step 239** [W] ~10m: Create `pkg/pqc/fuzz_test.go`
  ```
  Fuzz targets:
  - FuzzPQCValueLayoutDecode: random 12 bytes → no panic
  - FuzzPseudoHeaderMarshal: random fields → no panic
  - FuzzSigRefAllocator: rapid allocations → no collision, no panic
  ```

- [ ] **Step 240** [B] ~5m: Run fuzz tests (30 seconds each)
  ```bash
  go test ./pkg/pqc/ -fuzz=FuzzPQCValueLayoutDecode -fuzztime=30s 2>&1 | tail -10
  go test ./pkg/pqc/ -fuzz=FuzzPseudoHeaderMarshal -fuzztime=30s 2>&1 | tail -10
  ```

- [ ] **Step 241** [V]: No panics from fuzzing

- [ ] **Step 242** [W] ~5m: Create PQC-001 stress test
  ```
  TestSigRefExhaustionResilience:
  - Send 100K packets with random unknown SigRef values
  - Assert: bloom filter rejects > 99.9%
  - Assert: rate limiter caps verifier at configured rate
  - Assert: legitimate flows are NOT impacted
  ```

- [ ] **Step 243** [B] ~2m: Run stress test
  ```bash
  go test ./pkg/pqc/ -run TestSigRefExhaustion -v -timeout 120s 2>&1 | tail -20
  ```

- [ ] **Step 244** [V]: Stress test passes — DoS resilient

- [ ] **Step 245** [W] ~5m: Create PQC-005 epoch rollback test
  ```
  TestEpochRollbackAttack:
  - Provision key at epoch 5
  - Rotate to epoch 6
  - Attempt verification with epoch 5 packet → REJECTED
  - Verify old key material is zeroed
  ```

- [ ] **Step 246** [B] ~1m: Run epoch test
  ```bash
  go test ./pkg/pqc/ -run TestEpochRollback -v 2>&1 | tail -20
  ```

- [ ] **Step 247** [V]: Epoch rollback rejected

- [ ] **Step 248** [C]: **COMMIT CHECKPOINT** — adversarial tests complete

- [ ] **Step 249** [W] ~5m: Create Layer 2 policy bypass test
  ```
  TestLayer2PolicyBypassAttempt:
  - Attempt to write to sophia_pqc_app_policy from data plane → REJECTED (BPF_F_RDONLY_PROG)
  - Attempt to modify Wotan PQC state from userspace without control plane auth → REJECTED
  - Attempt to inject fake pqc_verified=1 into Wotan → REJECTED (only Shield writes this)
  ```

- [ ] **Step 250** [B] ~1m: Run Layer 2 bypass test
  ```bash
  go test ./pkg/pqc/ -run TestLayer2PolicyBypass -v 2>&1 | tail -20
  ```

- [ ] **Step 251** [V]: Layer 2 bypass attempts all rejected

- [ ] **Step 252-259**: [Reserved for additional hardening]

- [ ] **Step 260** [V]: **PHASE 7 EXIT GATE** — All 8 BlackMage findings mitigated, header stripping threat model validated, fuzz clean, stress tested, Layer 2 bypass tested
  - All PASS → Phase 8
  - Any FAIL → DO NOT PROCEED

---

## PHASE 8: END-TO-END VALIDATION (Steps 261-300)

**Goal**: Full dual-layer PQC authentication E2E test: key gen → signing → packet marking → XDP verification → header stripping → Wotan state → Layer 2 app policy → cached fast path → replay rejection → egress re-stamp
**Prerequisite**: Phase 6B AND Phase 7 exit gates passed
**Time**: 3-4 hours
**Agent**: Coordinator

### 8.1 Full E2E Test (Layer 1 + Layer 2)

- [ ] **Step 261** [W] ~20m: Create `test/e2e/pqc_e2e_test.go`
  ```
  TestPQCDualLayerEndToEnd:
  Phase A: Setup
    1. Generate SLH-DSA-SHA2-192s key pair (Level 3)
    2. Provision public key in Sophia PQC key map
    3. Pin key fingerprint in Wotan
    4. Start PQC verifier daemon
    5. Load enterprise app policy (Level 3 min, pinned key, 24h max age)
    6. Load relaxed app policy for second app (Level 1 min, no pinning)

  Phase B: First Packet (Slow Path + Header Stripping)
    7. Construct pseudo-header for test flow
    8. Sign pseudo-header with private key
    9. Provision signature in Sophia PQC sig map
    10. Allocate SigRef via CSPRNG allocator
    11. Add SigRef to bloom filter
    12. Encode PQCValueLayout into Monad register
    13. Set S flag in Monad flags
    14. Submit packet to Shield ingress
    15. Assert: ring buffer receives verification request
    16. Wait for verifier daemon to process
    17. Assert: Sophia map verified=1
    18. Assert: Monad HbH header STRIPPED from forwarded packet
    19. Assert: Wotan PQC state persisted (all fields)

  Phase C: Subsequent Packets (Fast Path, No HbH)
    20. Send 100 packets with incrementing SeqNum
    21. Assert: all pass via cached verification
    22. Assert: internal packets have NO Monad HbH headers
    23. Assert: PQC cache hit counter = 100

  Phase D: Layer 2 Application Policy
    24. Enterprise app verifies flow → ACCEPT (Level 3 met, key pinned, age OK)
    25. Relaxed app verifies same flow → ACCEPT (Level 1 met, no pin required)
    26. Modify enterprise policy to Level 5 → reverify → REJECT (algo is Level 3)
    27. Assert: both apps produced independent audit trails
    28. Assert: Layer 2 rejection does NOT affect Layer 1 state

  Phase E: Attack Scenarios
    29. Send replay packet (old SeqNum) → assert DROP
    30. Send packet with unknown SigRef (not in bloom) → assert DROP
    31. Send packet with wrong flow_label → assert cache miss
    32. Send packet with revoked key → assert DROP
    33. Send burst of 10K unknown SigRef → assert rate limited
    34. Verify internal lateral movement CANNOT inject SigRef (no HbH)

  Phase F: Key Rotation
    35. Trigger key rotation
    36. Verify new key epoch in Wotan
    37. Verify old key material zeroed
    38. Verify new signatures accepted
    39. Verify old signatures rejected (after grace period)
    40. Verify Layer 2 enterprise policy re-accepts with new key (fp updated)

  Phase G: Egress Re-stamping
    41. Route verified flow to egress
    42. Assert: Shield egress re-stamps Monad HbH header
    43. Assert: S flag set, fresh SeqNum, valid CRC-16
    44. Assert: Kingdom Mode bits zeroed

  Phase H: Cleanup
    45. Stop verifier daemon
    46. Verify audit log contains all L1 + L2 operations
    47. Verify Anamnesis events logged correctly
  ```

- [ ] **Step 262** [B] ~5m: Run E2E test
  ```bash
  go test ./test/e2e/ -run TestPQCDualLayer -v -timeout 600s 2>&1 | tail -50
  ```

- [ ] **Step 263** [V]: E2E test passes ALL phases (A-H)
  - If fail → Step 264
  - If pass → Step 265

- [ ] **Step 264** [D]: Debug E2E failure — check which phase fails, isolate

- [ ] **Step 265** [C]: **COMMIT CHECKPOINT** — Dual-layer E2E test green

### 8.2 Full Regression Suite

- [ ] **Step 266** [B] ~5m: Run ENTIRE test suite (existing 23 E2E + new PQC + Layer 2)
  ```bash
  go test ./... -v -race -timeout 600s 2>&1 | tail -30
  ```

- [ ] **Step 267** [V]: ALL tests pass — no regressions

- [ ] **Step 268** [D]: If regression — isolate which existing test broke

- [ ] **Step 269** [C]: **COMMIT CHECKPOINT** — full regression pass

### 8.3 Coverage Report

- [ ] **Step 270** [B] ~3m: Generate coverage report
  ```bash
  go test ./pkg/pqc/... -coverprofile=/tmp/pqc_coverage.out -covermode=atomic 2>&1 | tail -10
  go tool cover -func=/tmp/pqc_coverage.out | tail -20
  ```

- [ ] **Step 271** [V]: PQC package coverage >= 80% (including Layer 2)

- [ ] **Step 272** [B] ~2m: Run gosec + staticcheck
  ```bash
  gosec ./pkg/pqc/... 2>&1 | tail -20
  staticcheck ./pkg/pqc/... 2>&1 | tail -20
  ```

- [ ] **Step 273** [V]: No security findings, no staticcheck errors

- [ ] **Step 274** [C]: **COMMIT CHECKPOINT** — coverage + security scan clean

### 8.4 Performance Validation

- [ ] **Step 275** [B] ~5m: Benchmark full pipeline
  ```bash
  go test ./pkg/pqc/ -bench=. -benchmem -benchtime=10s 2>&1
  ```

- [ ] **Step 276** [R]: Validate against targets
  ```
  Expected:
  - L1 fast path (cached): < 300ns per packet
  - L1 slow path (one-time): < 5ms per flow
  - L2 verification: < 500ns per flow establishment
  - Header stripping: < 100ns per packet
  - Egress re-stamping: < 200ns per packet
  - SLH-DSA-SHA2-128s verification: < 3ms
  ```

- [ ] **Step 277** [V]: Performance meets or exceeds ALL targets

- [ ] **Step 278** [C]: **COMMIT CHECKPOINT** — performance validated

- [ ] **Step 279-299**: [Reserved for iteration]

- [ ] **Step 300** [V]: **PHASE 8 EXIT GATE** — Dual-layer E2E passes, all tests green, coverage >= 80%, no gosec findings, performance validated, zero regressions
  - All PASS → Phase 9
  - Any FAIL → DO NOT PROCEED

---

## PHASE 9: DOCUMENTATION + SHIP (Steps 301-320)

**Goal**: Update all docs, reconcile RFC draft, Librarian ripple, final ship gate
**Prerequisite**: Phase 8 exit gate passed
**Time**: 1-2 hours
**Agent**: Coordinator + Librarian

### 9.1 Documentation Update

- [ ] **Step 301** [W] ~10m: Update CLAUDE.md with PQC section
  ```
  - New package: pkg/pqc/
  - New command: cmd/pqc-verifier/
  - New eBPF maps: PQC_SIG_MAP, PQC_KEY_MAP, PQC_BLOOM, PQC_EVENTS
  - New Wotan addresses: 0x0000FF00-0x0000FF27 (PQC state + Layer 2 fields)
  - New Sophia map types: 0x10 (PQC Signatures), 0x11 (PQC Public Keys), 0x12 (PQC App Policies)
  - FIPS 205 compliance: SLH-DSA-SHA2-128s default parameter set
  - Dual-layer architecture: Wire L1 (Shield perimeter) + App Policy L2 (Sophia dict)
  - Header stripping: Monad HbH stripped at ingress, re-stamped at egress
  - BlackMage findings: 8 found, 8 mitigated
  ```

- [ ] **Step 302** [C]: **COMMIT CHECKPOINT** — CLAUDE.md updated

### 9.2 RFC Draft Reconciliation

- [ ] **Step 303** [V]: Verify RFC draft matches implementation
  ```
  Check:
  - Section 5 (Monad Value Layout) matches pkg/pqc/types.go
  - Section 6 (Sophia Maps) matches BPF map definitions
  - Section 12 (Shield) matches shield/ingress.go + shield/egress.go
  - Section 13 (Layer 2) matches pkg/pqc/layer2_verifier.go
  - Section 11 (Wotan) matches pkg/wotan/pqc_state.go
  - Appendix B (Performance) matches actual benchmarks
  ```

- [ ] **Step 304** [W] ~5m: Update RFC Appendix B with actual benchmark numbers

- [ ] **Step 305** [C]: **COMMIT CHECKPOINT** — RFC reconciled

### 9.3 Final Count + Ship

- [ ] **Step 306** [B] ~30s: LOC count
  ```bash
  find pkg/pqc cmd/pqc-verifier shield -name "*.go" | xargs wc -l | tail -1
  ```

- [ ] **Step 307** [B] ~30s: Test count
  ```bash
  grep -rn "func Test\|func Fuzz\|func Benchmark" pkg/pqc/ cmd/pqc-verifier/ shield/ | wc -l
  ```

- [ ] **Step 308** [B] ~30s: Final full test run
  ```bash
  go test ./... -race -timeout 600s 2>&1 | grep -E "^(ok|FAIL)" | tail -30
  ```

- [ ] **Step 309** [V]: ZERO failures

- [ ] **Step 310** [V]: **PHASE 9 EXIT GATE — FINAL** — Dual-layer PQC authentication shipped. E2E passes. All tests green. Coverage >= 80%. No gosec findings. Performance validated. Docs updated. RFC reconciled. Zero regressions. Header stripping operational. Layer 2 app policy functional.
  - **SHIP IT.**

---

## PHASE 10: MULTI-ALGORITHM SUPPORT (Steps 321-355)

**Goal**: Integrate ML-DSA (FIPS 204) and FN-DSA (FIPS 206) alongside SLH-DSA
**Depends**: Phase 6A (Shield operational with SLH-DSA)
**Duration**: 4-6 hours

### ML-DSA Integration (Steps 321-335)

- [ ] **Step 321** [R]: Read FIPS 204 spec — ML-DSA-44/65/87 parameter sets, NTT mod q, key/sig sizes
- [ ] **Step 322** [B]: `go get github.com/cloudflare/circl/sign/mldsa` — add ML-DSA library dependency
- [ ] **Step 323** [W]: Create `pkg/pqc/mldsa/mldsa.go` — ML-DSA sign/verify wrapper matching PQC interface
- [ ] **Step 324** [W]: Create `pkg/pqc/mldsa/mldsa_test.go` — unit tests, known-answer tests from FIPS 204
- [ ] **Step 325** [V]: `go test -race -v ./pkg/pqc/mldsa/...` — all ML-DSA tests PASS
- [ ] **Step 326** [B]: Register algo_ids 0x10 (ML-DSA-44), 0x11 (ML-DSA-65), 0x12 (ML-DSA-87) in algo registry
- [ ] **Step 327** [W]: Update Sophia PQC sig map to accept ML-DSA signature sizes (2,420-4,627B)
- [ ] **Step 328** [W]: Update XDP verifier fast path — branch on algo_id: 0x01-0x0C → SLH-DSA, 0x10-0x12 → ML-DSA
- [ ] **Step 329** [V]: XDP verifier accepts ML-DSA sig entries, verification succeeds with test vectors
- [ ] **Step 330** [B]: Benchmark ML-DSA verify vs SLH-DSA — expect 10-20x speedup

### FN-DSA Integration (Steps 331-345)

- [ ] **Step 331** [R]: Read FIPS 206 IPD — FN-DSA-512/1024, FFT basis, LDL tree, randomized signing
- [ ] **Step 332** [B]: `go get github.com/cloudflare/circl/sign/fndsa` — add FN-DSA library dependency
- [ ] **Step 333** [W]: Create `pkg/pqc/fndsa/fndsa.go` — FN-DSA sign/verify wrapper
- [ ] **Step 334** [W]: Create `pkg/pqc/fndsa/signing_daemon.go` — dedicated FN-DSA signing daemon
  - Process isolation: separate cgroup, dedicated CPU cores (isolcpus)
  - Unix domain socket with SO_PEERCRED auth
  - mlocked memory for private keys
  - Entropy check at startup (RDSEED / >= 256 bits)
  - Randomized signing only (FIPS 206 requirement)
- [ ] **Step 335** [W]: Create `pkg/pqc/fndsa/fndsa_test.go` — unit tests + Eurocrypt 2025 timing regression
- [ ] **Step 336** [V]: `go test -race -v ./pkg/pqc/fndsa/...` — all FN-DSA tests PASS
- [ ] **Step 337** [W]: Register algo_ids 0x20 (FN-DSA-512), 0x21 (FN-DSA-1024) in algo registry
- [ ] **Step 338** [W]: Update XDP verifier — FN-DSA algo_ids route to bpf_kfunc upcall OR integer NTT verify
- [ ] **Step 339** [V]: FN-DSA signing daemon starts, signs test pseudo-header, verification succeeds
- [ ] **Step 340** [B]: Benchmark FN-DSA verify latency — expect ~0.2ms (5x faster than SLH-DSA-128s)

### Algorithm Negotiation (Steps 341-345)

- [ ] **Step 341** [W]: Create `pkg/pqc/negotiate.go` — per-flow algorithm selection logic
  - Peer capability advertisement (Unheaded-to-Unheaded)
  - Administrative policy from Sophia app policy dict
  - Performance preference fallback (FN-DSA min BW, ML-DSA min latency, SLH-DSA max safety)
- [ ] **Step 342** [W]: Enforce invariant: algo_id MUST NOT change mid-flow
- [ ] **Step 343** [V]: Algorithm mismatch on existing flow → DROP + Anamnesis ERROR
- [ ] **Step 344** [W]: Create `pkg/pqc/negotiate_test.go` — negotiation unit tests
- [ ] **Step 345** [V]: `go test -race -v ./pkg/pqc/negotiate/...` — PASS

### BlackMage Mitigations: PQC-009 through PQC-013 (Steps 346-355)

- [ ] **Step 346** [W]: PQC-009 mitigation — FN-DSA signing daemon constant-time checks + FPU isolation test
- [ ] **Step 347** [W]: PQC-010 mitigation — TOCTOU: pin verification result SHA-256 hash in Wotan, apps compare hash
- [ ] **Step 348** [W]: PQC-011 mitigation — algo_id validation: Shield cross-checks algo_id in Monad vs Sophia map entry
- [ ] **Step 349** [W]: PQC-011 mitigation — tier enforcement: STANDARD rejects ML-DSA/FN-DSA algo_ids
- [ ] **Step 350** [W]: PQC-013 mitigation — signing daemon entropy check: refuse signing if pool < 256 bits
- [ ] **Step 351** [V]: PQC-009 test — timing differential across 10K signatures < 1μs variance
- [ ] **Step 352** [V]: PQC-010 test — simulate TOCTOU by swapping sig between verify and consume → detected
- [ ] **Step 353** [V]: PQC-011 test — send ML-DSA packet to STANDARD tier Shield → DROP confirmed
- [ ] **Step 354** [V]: PQC-013 test — deplete entropy pool → signing daemon refuses + CRITICAL event emitted
- [ ] **Step 355** [V]: **PHASE 10 EXIT GATE** — All 3 signature algorithms operational. BlackMage PQC-009→013 mitigated.

---

## PHASE 11: COMPLIANCE TIERS (Steps 356-380)

**Goal**: Implement NONE/STANDARD/ENHANCED/SOVEREIGN tier system via Kingdom Mode K1|K0
**Depends**: Phase 10 (multi-algorithm operational)
**Duration**: 3-4 hours

### Tier Infrastructure (Steps 356-365)

- [ ] **Step 356** [W]: Create `pkg/pqc/tiers/tiers.go` — tier enum (NONE=0x00, STANDARD=0x01, ENHANCED=0x02, SOVEREIGN=0x03)
- [ ] **Step 357** [W]: Create `pkg/pqc/tiers/config.go` — tier configuration loader from YAML/API
  - Parse: `pqc.tier`, `pqc.algorithms.primary/secondary/tertiary`, `pqc.layer2.enabled`, `pqc.audit.enabled`
- [ ] **Step 358** [W]: Map K1|K0 bits ↔ tier enum in Monad flags byte
- [ ] **Step 359** [W]: Update Shield XDP — read K1|K0 from Monad flags, branch on tier for processing
- [ ] **Step 360** [V]: Tier NONE (K1=0,K0=0) → Shield skips all PQC processing, PASS confirmed

### Tier STANDARD (Steps 361-365)

- [ ] **Step 361** [W]: STANDARD path — accept only algo_ids 0x01-0x0C (SLH-DSA). Reject all others.
- [ ] **Step 362** [W]: STANDARD loads only sophia_pqc_sigs + sophia_pqc_keys maps. No app policy.
- [ ] **Step 363** [W]: STANDARD Wotan state — basic fields only (0x0000FF00-0x0000FF0F)
- [ ] **Step 364** [V]: STANDARD tier processes SLH-DSA packet correctly
- [ ] **Step 365** [V]: STANDARD tier rejects ML-DSA packet (algo_id 0x10) → DROP confirmed

### Tier ENHANCED (Steps 366-370)

- [ ] **Step 366** [W]: ENHANCED path — accept all sig algo_ids (SLH-DSA + ML-DSA + FN-DSA)
- [ ] **Step 367** [W]: ENHANCED loads all PQC maps including sophia_pqc_app_policy
- [ ] **Step 368** [W]: ENHANCED Wotan state — full fields (0x0000FF00-0x0000FF27)
- [ ] **Step 369** [V]: ENHANCED tier accepts ML-DSA + FN-DSA packets
- [ ] **Step 370** [V]: ENHANCED tier Layer 2 optional — app policy applied when dict present, skipped when absent

### Tier SOVEREIGN (Steps 371-378)

- [ ] **Step 371** [W]: Create `pkg/pqc/sovereign/multi_verify.go` — 2-of-3 cross-verification engine
- [ ] **Step 372** [W]: Create sophia_pqc_sovereign_sigs map — struct with primary/secondary/tertiary SigRefs
- [ ] **Step 373** [W]: SOVEREIGN ingress — verify primary algo, then retrieve + verify secondary from different family
- [ ] **Step 374** [W]: Consensus tracking — bitfield: b0=primary, b1=secondary, b2=tertiary. popcount >= 2 → PASS
- [ ] **Step 375** [W]: SOVEREIGN mandates Layer 2 — reject flows without sophia_pqc_app_policy entry
- [ ] **Step 376** [W]: SOVEREIGN Anamnesis — emit audit event for every verification result
- [ ] **Step 377** [V]: SOVEREIGN 2-of-3 test — compromise one algo → still authenticates via remaining two
- [ ] **Step 378** [V]: SOVEREIGN Layer 2 mandatory — flow without app policy → REJECT confirmed

### Tier Transitions (Steps 379-380)

- [ ] **Step 379** [W]: Control plane API: `POST /api/v1/pqc/tier` — hot-swap tier, hot-reload Sophia maps, zero downtime
- [ ] **Step 380** [V]: **PHASE 11 EXIT GATE** — All 4 tiers operational. Tier transitions emit Anamnesis events. Downgrade = WARNING. SOVEREIGN 2-of-3 cross-verification confirmed.

---

## PHASE 12: KEM INTEGRATION + FINAL SHIP (Steps 381-400)

**Goal**: ML-KEM + HQC tunnel key establishment, final E2E with all features, ship gate
**Depends**: Phase 11 (tiers operational), Phase 8 (E2E framework)
**Duration**: 3-4 hours

### KEM Key Establishment (Steps 381-390)

- [ ] **Step 381** [R]: Read FIPS 203 (ML-KEM) spec — ML-KEM-512/768/1024, encaps/decaps
- [ ] **Step 382** [R]: Read FIPS 207 (HQC) spec — HQC-128/192, code-based KEM
- [ ] **Step 383** [B]: `go get github.com/cloudflare/circl/kem/mlkem` — add ML-KEM dependency
- [ ] **Step 384** [W]: Create `pkg/pqc/kem/mlkem.go` — ML-KEM encapsulate/decapsulate wrapper
- [ ] **Step 385** [W]: Create `pkg/pqc/kem/hqc.go` — HQC encapsulate/decapsulate wrapper
- [ ] **Step 386** [W]: Create `pkg/pqc/kem/tunnel.go` — Shield-to-Shield tunnel key establishment via control channel
  - Initiator encaps with responder pubkey → ciphertext via Sophia control channel
  - Responder decaps → shared secret → HKDF-SHA256 → per-flow signing keys
- [ ] **Step 387** [W]: Register KEM algo_ids: 0x80-0x82 (ML-KEM), 0x90-0x91 (HQC) in Sophia key maps
- [ ] **Step 388** [V]: ML-KEM-768 tunnel establishment between two Shield instances — shared secret matches
- [ ] **Step 389** [V]: HQC-128 tunnel establishment — shared secret matches (non-lattice diversity)
- [ ] **Step 390** [C]: Commit KEM integration

### Final E2E Validation (Steps 391-398)

- [ ] **Step 391** [V]: E2E: Tier NONE — packet traverses kingdom, no PQC processing, zero overhead
- [ ] **Step 392** [V]: E2E: Tier STANDARD — SLH-DSA sig-by-ref, L1 verify, header strip, Wotan state
- [ ] **Step 393** [V]: E2E: Tier ENHANCED — ML-DSA fast verify + Layer 2 Sophia policy + FN-DSA signing daemon
- [ ] **Step 394** [V]: E2E: Tier SOVEREIGN — 2-of-3 cross-verify + mandatory L2 + Anamnesis audit trail
- [ ] **Step 395** [V]: E2E: Tier transition STANDARD → SOVEREIGN — zero downtime, Anamnesis event confirmed
- [ ] **Step 396** [V]: E2E: KEM tunnel — ML-KEM-768 key establishment → per-flow SLH-DSA signing → verify
- [ ] **Step 397** [B]: Full benchmark suite — all tiers, all algos, fast path + slow path + tier transition latency
- [ ] **Step 398** [C]: Commit final E2E validation

### Ship Gate (Steps 399-400)

- [ ] **Step 399** [V]: **FINAL SHIP GATE CHECKLIST**:
  - [ ] All 5 FIPS standards integrated (203, 204, 205, 206, 207)
  - [ ] All 4 compliance tiers operational (NONE, STANDARD, ENHANCED, SOVEREIGN)
  - [ ] All 13 BlackMage findings mitigated (PQC-001 through PQC-013)
  - [ ] Dual-layer architecture (L1 wire + L2 app policy) verified
  - [ ] Header stripping at ingress + re-stamping at egress operational
  - [ ] Multi-algorithm 2-of-3 cross-verification (SOVEREIGN) confirmed
  - [ ] KEM tunnel key establishment operational
  - [ ] All tests green, coverage >= 80%, no gosec findings
  - [ ] RFC draft consistent with implementation
  - [ ] Battle plan 100% complete
- [ ] **Step 400** [V]: **SHIP IT. 🚀 Multi-FIPS PQC authentication for the Unheaded Kingdom.**

---

## APPENDIX A: Emergency Procedures

### EP-1: eBPF Verifier Rejects PQC Logic
**Symptom**: `cargo build` fails with verifier complexity error
**Fix**:
1. Reduce branches in PQC fast path
2. Move bloom filter check to separate tail call program
3. Reduce map lookups — cache result in stack variable
4. If still failing: move SeqNum check entirely to userspace

### EP-2: SLH-DSA Library Not Available in Go
**Symptom**: No Go library implements FIPS 205
**Fix**:
1. Use CGo wrapper around liboqs C library
2. Or: implement SLH-DSA verification in Rust, expose via FFI
3. Or: use Python subprocess for verification (slower but works)

### EP-3: Sophia Map Size Exceeds Kernel Memory
**Symptom**: `bpf_map_update_elem` returns -ENOMEM
**Fix**:
1. Reduce max_entries (1M → 256K)
2. Implement LRU eviction of old verified entries
3. Use per-CPU hash maps to reduce locking overhead

### EP-4: Race Condition in Wotan CAS
**Symptom**: -race detector fires on SeqNum update
**Fix**:
1. Verify CAS implementation uses `sync/atomic.CompareAndSwapUint32`
2. Check that fallback path retries correctly (max 3 attempts)
3. Add spinlock if atomic CAS insufficient

---

## APPENDIX B: Agent Assignment Matrix

| Phase | Agent | Parallel | Depends On | Time Est |
|-------|-------|----------|------------|----------|
| 0 | Coordinator | No | — | 20m |
| 1 | Developer | No | Phase 0 | 2-3h |
| 2 | Developer | No | Phase 1 | 2-3h |
| 3 | Developer | **Yes [P]** | Phase 2 | 2-3h |
| 4 | Developer | **Yes [P]** | Phase 2 | 2h |
| 5 | Developer | No | Phase 3+4 | 3-4h |
| 6A | Developer | No | Phase 5 | 3h |
| 6B | Developer | **Yes [P]** | Phase 6A | 2-3h |
| 7 | Dev+BlackMage | **Yes [P]** | Phase 6A | 2-3h |
| 8 | Coordinator | No | Phase 6B+7 | 3-4h |
| 9 | Coord+Librarian | No | Phase 8 | 1-2h |
| 10 | Developer | No | Phase 6A | 4-6h |
| 11 | Developer | No | Phase 10 | 3-4h |
| 12 | Dev+Coordinator | No | Phase 11+8 | 3-4h |

**Critical Path**: 0 → 1 → 2 → 5 → 6A → 10 → 11 → 8 → 9 → 12 = ~28-36h
**With parallelism**: Phases 3+4 overlap + 6B||7 overlap = save ~4h → **~24-32h**

---

## APPENDIX C: Quick Reference

### Monad PQC Value Layout (12 bytes)
```
Offset  Field    Size    Notes
0       SigRef   3B      24-bit, CSPRNG-assigned, big-endian
3       KeyRef   3B      24-bit, big-endian
6       HashPfx  2B      SHA-256(sig)[0:2], big-endian
8       SeqNum   4B      32-bit monotonic counter, big-endian
```

### Wotan PQC Addresses
```
0x0000FF00  last_seen_seq    uint32
0x0000FF04  pqc_verified     uint8
0x0000FF05  pqc_algo_id      uint8
0x0000FF06  pqc_key_epoch    uint8
0x0000FF07  reserved         uint8
0x0000FF08  pqc_verify_count uint32
0x0000FF0C  pqc_fail_count   uint32
0x0000FF10  key_fingerprint  [32]byte
```

### BlackMage Finding → Mitigation Map
```
PQC-001 (CRIT) SigRef exhaustion     → Bloom filter + rate limit
PQC-002 (HIGH) Cache poisoning       → CSPRNG SigRef + flow_label binding
PQC-003 (MED)  Timing oracle         → Constant-time padding + jitter
PQC-004 (HIGH) Ring buffer flood     → Separate PQC ring buffer + priority
PQC-005 (HIGH) Key epoch rollback    → Epoch check in Wotan + zeroize
PQC-006 (MED)  SeqNum wraparound     → Wraparound-triggered rekeying
PQC-007 (LOW)  HashPfx confidence    → Code comments + consider removal
PQC-008 (CRIT) Control plane         → Key pinning + audit trail + HSM
PQC-009 (CRIT) FN-DSA FP side-chan   → Constant-time impl + isolcpus + mlocked mem
PQC-010 (CRIT) TOCTOU kernel/user    → Sig pin via SHA-256 hash in Wotan + BTF auth
PQC-011 (CRIT) Algo confusion/dwngrade → algo_id cross-check + tier enforcement
PQC-013 (CRIT) FN-DSA entropy        → RDSEED check + entropy pool monitor + refuse if < 256b
```

### Compliance Tier Quick Reference
```
K1|K0  Tier       Algos              Layer 2    Multi-Algo  Default Policy
0 | 0  NONE       —                  —          —           —
0 | 1  STANDARD   SLH-DSA only       OFF        NO          OPTIMISTIC
1 | 0  ENHANCED   SLH+ML+FN-DSA      Optional   NO          OPTIMISTIC
1 | 1  SOVEREIGN  All 3, 2-of-3      MANDATORY  YES         PESSIMISTIC
```

### Algorithm Registry Quick Reference
```
0x01-0x0C  SLH-DSA (FIPS 205)  7,856-49,856B   eBPF: YES
0x10-0x12  ML-DSA  (FIPS 204)  2,420-4,627B     eBPF: YES
0x20-0x21  FN-DSA  (FIPS 206)  666-1,280B       eBPF: verify YES, sign NO
0x80-0x82  ML-KEM  (FIPS 203)  768-1,568B CT    KEM only (tunnel)
0x90-0x91  HQC     (FIPS 207)  4,497-9,042B CT  KEM only (tunnel)
```

---

*S-PQC Battle Plan — Forged 2026-02-26*
*12 Phases. 400 Steps. Multi-FIPS post-quantum authentication across all five NIST PQC standards.*
*Four compliance tiers. Three signature families. Two KEM families. One Kingdom.*
*THE MOAT IS QUANTUM-PROOF. THE KINGDOM STANDS SOVEREIGN.*
