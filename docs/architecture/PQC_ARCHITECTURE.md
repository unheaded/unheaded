# Post-Quantum Cryptography Architecture

**Version:** Alpha | **Last Updated:** March 4, 2026 | **Status:** Canonical

> **"Quantum-resistant authentication from packet zero."**

Unheaded's PQC subsystem provides post-quantum cryptographic authentication for all intra-Kingdom traffic. Every packet carries a 12-byte PQC value in the **Monad** wire format, referencing signatures and keys stored in **Sophia** BPF maps. The **Shield** verification engine validates packets through a 7-point pipeline, while the **Wotan** message bus propagates verification state and audit events.

---

## Architecture Layers

```
┌─────────────────────────────────────────────────────────────────┐
│  Layer 5: Daemon & API                                          │
│  PQC Verifier on port 19008 — HTTP API + Wotan events           │
│  cmd/pqc-verifier/                                              │
├─────────────────────────────────────────────────────────────────┤
│  Layer 4: Key Exchange                                          │
│  ML-KEM tunnel negotiation + HKDF-SHA256 key derivation         │
│  pkg/crypto/pqc/kem_tunnel.go                                   │
├─────────────────────────────────────────────────────────────────┤
│  Layer 3: Policy & Compliance                                   │
│  Compliance tiers (NONE → SOVEREIGN) + per-app YAML policies    │
│  pkg/policy/ + pkg/pqc_policy/ + services/shield/tier_verify.go │
├─────────────────────────────────────────────────────────────────┤
│  Layer 2: Verification Engine                                   │
│  7-point pipeline + 2-of-3 sovereign multi-sig + security       │
│  services/shield/pqc_verify.go + sovereign_verify.go            │
├─────────────────────────────────────────────────────────────────┤
│  Layer 1: Wire Format                                           │
│  Monad PQC Value (12 bytes) — SigRef + KeyRef + HashPfx + Seq   │
│  pkg/monad/pqc_value.go                                         │
└─────────────────────────────────────────────────────────────────┘
```

---

## Algorithm Registry

Five NIST-standardized algorithms are registered in `pkg/crypto/pqc/registry.go`:

| AlgoID | Name | FIPS | Type | Family | BPF-Safe | Available |
|--------|------|------|------|--------|----------|-----------|
| `0x01` | SLH-DSA | FIPS 205 | Signature | Hash-based | Yes | Stub |
| `0x02` | ML-DSA | FIPS 204 | Signature | Lattice | Yes | **Full** (circl) |
| `0x03` | FN-DSA | FIPS 206 | Signature | Lattice | No (FPU) | Stub |
| `0x04` | ML-KEM | FIPS 203 | KEM | Lattice | Yes | **Full** (circl) |
| `0x05` | HQC | FIPS 207 | KEM | Code-based | Yes | Stub |

### NIST Security Level Mapping

Each algorithm/parameter-set combination maps to a NIST security level (`pkg/pqc_policy/nist_level.go`):

| Algorithm | ParamSet 1 | ParamSet 2 | ParamSet 3 |
|-----------|-----------|-----------|-----------|
| SLH-DSA | Level 1 | Level 3 | Level 5 |
| ML-DSA | Level 2 (ML-DSA-44) | Level 3 (ML-DSA-65) | Level 5 (ML-DSA-87) |
| FN-DSA | Level 1 (FN-DSA-512) | Level 5 (FN-DSA-1024) | — |
| ML-KEM | Level 1 (ML-KEM-512) | Level 3 (ML-KEM-768) | Level 5 (ML-KEM-1024) |
| HQC | Level 1 (HQC-128) | Level 3 (HQC-192) | — |

**NIST Levels:** Level 1 = AES-128 equivalent, Level 2 = SHA-256 collision, Level 3 = AES-192, Level 4 = SHA-384 collision, Level 5 = AES-256.

### Algorithm Families

Sovereign tier requires signatures from **distinct** families to prevent single-family cryptanalytic breaks:

| Family | Algorithms | Basis |
|--------|-----------|-------|
| Hash-based | SLH-DSA | Hash function security (SHA-2/SHAKE) |
| Lattice | ML-DSA, FN-DSA, ML-KEM | Module-LWE/NTRU lattice hardness |
| Code-based | HQC | Syndrome decoding of random linear codes |

---

## Wire Format — Monad PQC Value

Every PQC-authenticated packet carries a 12-byte `PQCValue` embedded in the 20-byte Monad register (`pkg/monad/pqc_value.go`):

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                         SigRef (32)                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          KeyRef (16)          |        HashPfx[0:1] (16)      |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|        HashPfx[2:3] (16)     |          SeqNum (16)           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

| Field | Size | Description |
|-------|------|-------------|
| `SigRef` | 4 bytes | Index into **Sophia** `PQC_SIG_MAP` BPF map |
| `KeyRef` | 2 bytes | Index into **Sophia** `PQC_KEY_MAP` BPF map |
| `HashPfx` | 4 bytes | `SHA-256(signature)[0:4]` — fast mismatch detection |
| `SeqNum` | 2 bytes | Per-flow replay protection sequence number |

**Design principle:** Signatures and keys are stored by reference in **Sophia** BPF maps, not inline. This keeps the wire format compact (12 bytes) while supporting signatures up to 49,856 bytes (SLH-DSA).

---

## Compliance Tiers

Four compliance tiers govern verification strictness (`pkg/policy/verification.go`):

| Tier | ID | Mode | Required Algos | Max Sig Age | Multi-Sig | Teardown |
|------|----|------|---------------|-------------|-----------|----------|
| **NONE** | `0x00` | Optimistic | — | 24h | No | None |
| **STANDARD** | `0x01` | Optimistic | Any 1 PQC | 1h | No | None |
| **ENHANCED** | `0x02` | Pessimistic | ML-DSA | 30m | No | TCP RST / Drop |
| **SOVEREIGN** | `0x03` | Pessimistic | SLH-DSA + ML-DSA | 15m | 2-of-3 | TCP RST / Drop |

### Verification Behavior

- **NONE / OPTIMISTIC:** Packets are logged but not rejected. Used during migration.
- **STANDARD:** At least one valid PQC signature required. Unsigned packets generate warnings.
- **ENHANCED:** ML-DSA signature mandatory. Unsigned or wrongly-signed packets are **dropped**.
- **SOVEREIGN:** 2-of-3 cross-algorithm multi-sig from distinct families. Highest assurance.

### Tier Transitions

Tier changes are managed by `TierVerifier.TransitionTier()` in `services/shield/tier_verify.go`:

- **Upgrades** (NONE → STANDARD → ENHANCED → SOVEREIGN): Immediate, audit event emitted.
- **Downgrades** (SOVEREIGN → ENHANCED → STANDARD → NONE): Triggers a `DowngradeCallback` plus audit event. Downgrades indicate potential security regression.

### Flow Teardown Actions

When verification fails in pessimistic mode (`pkg/policy/verification.go`):

| Action | ID | Description |
|--------|----|-------------|
| `NONE` | `0x00` | No action (optimistic mode) |
| `TCP_RST` | `0x01` | Send TCP RST to tear down connection |
| `ICMPv6` | `0x02` | Send ICMPv6 Destination Unreachable (Admin Prohibited) |
| `SILENT_DROP` | `0x03` | Silently drop the packet |

---

## 7-Point Verification Pipeline

The core verification engine runs a fail-fast 7-point pipeline (`services/shield/pqc_verify.go`):

```
  Packet
    │
    ▼
┌──────────────────────────────────────┐
│  Step 1: Flag Check                  │
│  S+CUSTOM flags set in Monad header? │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 2: SigRef Lookup               │
│  SigRef → Sophia PQC_SIG_MAP cache   │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 3: KeyRef Lookup               │
│  KeyRef → PQC_KEY_MAP + status check │
│  (Active / Rotating / Revoked)       │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 4: Algorithm Compliance        │
│  AlgoID in tier's allowed set?       │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 5: Cryptographic Verify        │
│  Verify(pubKey, pseudoHeader, sig)   │
│  via circl ML-DSA / ML-KEM          │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 6: HashPfx Match              │
│  SHA-256(sig)[0:4] == PQCValue.Pfx? │
│  (constant-time compare)            │
└──────────────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────┐
│  Step 7: SeqNum Replay Check        │
│  Per-flow sliding window (16384-bit) │
└──────────────────┬───────────────────┘
                   ▼
              ┌─────────┐
              │ PASS ✓  │
              └─────────┘
```

Each step returns immediately on failure with the step number and failure message. Step 5 (cryptographic verification) is the most expensive — steps 1-4 and 6-7 serve as cheap pre-filters that reject invalid packets before touching public-key cryptography.

### Verification Result

```go
type PQCVerifyResult struct {
    Passed   bool
    FailStep PQCVerificationStep  // Which step failed (1-7)
    FailMsg  string               // Human-readable failure reason
    AlgoID   uint8                // Algorithm used
    KeyRef   uint16               // Key reference
    SigRef   uint32               // Signature reference
    Duration time.Duration        // Total verification time
    Tier     uint8                // Trust tier of signing key
    SeqNum   uint16               // Packet sequence number
}
```

---

## Sovereign Multi-Signature

The SOVEREIGN tier requires 2-of-3 cross-algorithm multi-signatures from distinct algorithm families (`services/shield/sovereign_verify.go`).

### Verification Flow

```
┌────────────────────────────────────────────────────┐
│  SovereignSig (3 slots)                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐         │
│  │ Slot 0   │  │ Slot 1   │  │ Slot 2   │         │
│  │ SLH-DSA  │  │ ML-DSA   │  │ FN-DSA   │         │
│  │ hash     │  │ lattice  │  │ lattice  │         │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘         │
│       │              │              │               │
│       ▼              ▼              ▼               │
│   ┌───────┐      ┌───────┐      ┌───────┐          │
│   │Verify │      │Verify │      │Verify │          │
│   └───┬───┘      └───┬───┘      └───┬───┘          │
│       │              │              │               │
│       ▼              ▼              ▼               │
│  Family diversity check: ≥ 2 distinct families?     │
│  Valid count check: ≥ 2 valid signatures?           │
└────────────────────────────────────────────────────┘
```

**Requirements for SOVEREIGN PASS:**
1. At least **2 of 3** signature slots must be cryptographically valid.
2. Valid signatures must come from at least **2 distinct algorithm families**.

### Control Plane Pre-Computation

`PrecomputeSovereignSigs()` signs a pseudo-header with multiple signers in the control plane, validates family diversity up front (fail-fast), and stores the resulting signatures in **Sophia** BPF maps via `PQCMapManager`.

### Data Plane Verification

`VerifySovereignFromSophia()` loads the sovereign signature entry from **Sophia**, verifies through the standard multi-sig path, and updates the entry with verification results and consensus status.

---

## KEM Tunnel & Key Derivation

The KEM tunnel provides post-quantum key exchange for establishing authenticated tunnels between services (`pkg/crypto/pqc/kem_tunnel.go`).

### HKDF-SHA256 Key Derivation

All tunnel keys are derived from ML-KEM shared secrets using HKDF-SHA256 with explicit domain separation:

```
                   ML-KEM Shared Secret (32 bytes)
                              │
                    ┌─────────┴─────────┐
                    │   HKDF-SHA256     │
                    │   Salt + Info     │
                    └─────────┬─────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐
  │ SigningKey    │    │ AuthKey      │    │ IVKey        │
  │ (32 bytes)   │    │ (32 bytes)   │    │ (16 bytes)   │
  └──────────────┘    └──────────────┘    └──────────────┘
```

**Domain separation info strings:**
- `DeriveSigningKey`: `"unheaded-pqc-signing" || flowID || epoch`
- `DeriveTunnelKeys`: `"unheaded-pqc-tunnel-signing"`, `"unheaded-pqc-tunnel-auth"`, `"unheaded-pqc-tunnel-iv"` (each with context)

### Tunnel Negotiation

```
  Initiator                              Responder
      │                                       │
      │  1. Generate ML-KEM keypair           │
      │                                       │
      │  2. Encapsulate(responderPubKey)       │
      │     → ciphertext + sharedSecret       │
      │                                       │
      │  ─── ciphertext ──────────────────►   │
      │                                       │
      │     3. Decapsulate(ciphertext, sk)     │
      │        → same sharedSecret            │
      │                                       │
      │  4. HKDF derive tunnel keys           │
      │     SigningKey, AuthKey, IVKey         │
      │                                       │
      ▼                                       ▼
   TunnelKeySet                         TunnelKeySet
   (identical)                          (identical)
```

### Rate Limiting

`KEMTunnelRateLimiter` restricts handshake frequency per second to prevent tunnel-setup DoS attacks. The limiter uses a sliding window with configurable maximum rate.

### Key Zeroing

Both `TunnelKeySet.Zero()` and `HandshakeResult.Zero()` securely wipe all key material from memory after use.

---

## Application Policy SDK

The `pkg/pqc_policy/` package provides a 7-point Layer 2 policy checker that enforces per-application PQC requirements.

### PolicyChecker Interface

```go
type PolicyChecker interface {
    Check(ctx context.Context, req PolicyCheckRequest) PolicyCheckResult
}
```

### 7-Point Policy Check

| Step | Check | Fail Condition |
|------|-------|---------------|
| 1 | PQC verified flag | Packet not PQC-authenticated (if `RequirePQC`) |
| 2 | Algorithm ID known | Unknown or unregistered algorithm |
| 3 | App policy lookup | No policy found for service ID |
| 4 | Allowed algorithms | Algorithm not in service's whitelist |
| 5 | NIST security level | Algo/params below minimum NIST level |
| 6 | Key fingerprint pinning | Key doesn't match pinned fingerprint |
| 7 | Key age | Signing key exceeds `MaxKeyAgeSec` |

### YAML Policy Format

```yaml
version: "1.0"
policies:
  - service_id: 100
    service_name: "critical-service"
    allowed_algos: [2]           # ML-DSA only
    min_nist_level: 3            # AES-192 equivalent minimum
    max_key_age_sec: 3600        # 1 hour
    require_pqc: true
  - service_id: 200
    service_name: "internal-tool"
    allowed_algos: [1, 2, 3]     # SLH-DSA, ML-DSA, FN-DSA
    min_nist_level: 1            # AES-128 equivalent minimum
    max_key_age_sec: 86400       # 24 hours
    require_pqc: false
```

Policies are loaded via `ParsePolicyFile(path)` or `ParsePolicyBytes(data)` and registered with a `Checker` instance.

---

## Security Hardening

The `SecurityHardening` module integrates five mitigation layers into a single validation pipeline (`services/shield/pqc_security.go`):

| Mitigation | Component | Purpose |
|-----------|-----------|---------|
| Algorithm confusion | `AlgoConfusionGuard` | Detects mid-flow algorithm changes that could indicate downgrade attacks |
| SigRef exhaustion | `SigRefRateLimiter` | Limits signature reference creation rate to prevent map exhaustion |
| Compact replay | `PacketBitmapWindow` | 64-bit per-flow sliding window — memory-efficient replay protection |
| Entropy monitoring | `EntropyMonitor` | Alerts if `/proc/sys/kernel/random/entropy_avail` drops below 256 bits |
| TOCTOU prevention | `TOCTOUCache` | Pins verification results with configurable TTL to prevent races |

### PacketBitmapWindow

A compact alternative to the full 16,384-bit `SeqTracker`. Uses a 64-bit sliding bitmap per flow to detect replays within a 64-sequence window:

- Sequence numbers **ahead** of the window advance it (shift + mark).
- Sequence numbers **within** the window are checked against the bitmap.
- Sequence numbers **behind** the window (>64 positions old) are rejected.

### EntropyMonitor

Reads `/proc/sys/kernel/random/entropy_avail` with a 1-second cache to avoid excessive `/proc` reads. Returns `-1` on non-Linux systems. Threshold default: 256 bits.

### TOCTOUCache

Caches `(flowID, sigRef) → verifiedAt` with a configurable TTL. Once a packet passes verification, subsequent lookups within the TTL short-circuit to avoid re-verification races. The `Evict()` method removes expired entries.

---

## PQC Verifier Daemon

A standalone HTTP service on port **19008** (Doom Range) that exposes PQC verification as an API (`cmd/pqc-verifier/`).

### HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/verify` | Verify a PQC-authenticated packet |
| `POST` | `/verify/sovereign` | Verify 2-of-3 sovereign multi-sig |
| `GET` | `/health` | Health check (200 if healthy) |
| `GET` | `/ready` | Readiness probe (200 if ready, 503 if shutting down) |
| `GET` | `/metrics` | Verification metrics (JSON) |

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PQC_VERIFIER_PORT` | `19008` | Listen port |
| `WOTAN_ADDR` | `localhost:18001` | Wotan gRPC address |
| `PQC_LOG_LEVEL` | `info` | Log verbosity |
| `PQC_POLICY_FILE` | — | YAML policy file path |
| `PQC_DEFAULT_TIER` | `1` (STANDARD) | Default compliance tier |
| `PQC_AUTH_ENABLED` | `false` | Enable authentication middleware |

### Wotan Event Integration

The daemon publishes and subscribes to **Wotan** topics for event-driven PQC state management:

**Published topics:**

| Topic | Trigger |
|-------|---------|
| `pqc.sig.verified` | Successful signature verification |
| `pqc.sovereign.validated` | Sovereign multi-sig validation |
| `pqc.anomaly` | Security anomaly detected |

**Subscribed topics:**

| Topic | Handler |
|-------|---------|
| `pqc.sig.created` | New signature registered |
| `pqc.key.generated` | New key pair generated |
| `pqc.key.rotated` | Key rotation event |
| `pqc.key.revoked` | Key revocation event |
| `pqc.policy.changed` | Policy configuration change |

### Lifecycle

1. Load configuration from environment.
2. Initialize `PQCVerifier`, `SovereignVerifier`, `Layer2PolicyEngine`, `PQCStateManager`.
3. Create `TierVerifier` orchestrator with default tier.
4. Register HTTP routes, start listening on configured port.
5. On `SIGINT`/`SIGTERM`: set readiness to false, drain connections, zero sensitive state.

---

## Audit Events

Eight PQC event types flow through the **Anamnesis** audit system (`pkg/audit/pqc_events.go`):

| Event Type | Description |
|-----------|-------------|
| `PQC_VERIFY_SUCCESS` | Packet passed 7-point verification |
| `PQC_VERIFY_FAIL` | Packet failed verification at step N |
| `PQC_KEY_ROTATE` | Key rotation initiated or completed |
| `PQC_KEY_REVOKE` | Key revoked (compromise or expiry) |
| `PQC_TIER_CHANGE` | Compliance tier transition |
| `PQC_VERIFIER_HEALTH` | Verifier health check result |
| `PQC_MAP_UTILIZATION` | Sophia BPF map utilization report |
| `PQC_ENTROPY_LOW` | System entropy below threshold |

---

## Data Flow

End-to-end packet verification flow through the PQC subsystem:

```
  Incoming Packet
        │
        ▼
  ┌──────────────┐     ┌──────────────┐
  │ Shield XDP   │ ──► │ Monad Header │  Extract 12-byte PQC Value
  │ (Layer 1)    │     │ Parse        │  from 20-byte Monad register
  └──────────────┘     └──────┬───────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │ Sophia BPF Maps  │  SigRef → PQC_SIG_MAP
                    │ (Signature/Key)  │  KeyRef → PQC_KEY_MAP
                    └──────────┬───────┘
                               │
                               ▼
                    ┌──────────────────┐
                    │ PQC Verifier     │  7-point pipeline
                    │ (Layer 2)        │  Flags → SigRef → KeyRef →
                    │                  │  Algo → Crypto → Hash → Seq
                    └──────────┬───────┘
                               │
                  ┌────────────┼────────────┐
                  ▼            ▼            ▼
          ┌─────────┐  ┌────────────┐  ┌──────────┐
          │  NONE/  │  │ STANDARD/  │  │SOVEREIGN │
          │ OPTIM.  │  │ ENHANCED   │  │ 2-of-3   │
          │ (skip)  │  │ (single)   │  │ multisig │
          └────┬────┘  └─────┬──────┘  └────┬─────┘
               │             │              │
               └─────────────┼──────────────┘
                             ▼
                    ┌──────────────────┐
                    │ Policy Check     │  Layer 2 per-app policy
                    │ (Layer 3)        │  7-point: algo whitelist,
                    │                  │  NIST level, key age, pins
                    └──────────┬───────┘
                               │
                    ┌──────────┴───────┐
                    ▼                  ▼
          ┌──────────────┐    ┌──────────────┐
          │ Wotan State  │    │ Audit Event  │
          │ Update       │    │ (Anamnesis)  │
          │ (40-byte)    │    │ PQC_VERIFY_* │
          └──────────────┘    └──────────────┘
```

---

## File Map

All files created or modified during the S-PQC sprint:

| File | Package | Purpose |
|------|---------|---------|
| `services/shield/tier_verify.go` | shield | Tier-based verification orchestrator |
| `services/shield/tier_verify_test.go` | shield | Tier verifier tests (8 tests) |
| `services/shield/pqc_security.go` | shield | Security hardening module |
| `services/shield/pqc_security_test.go` | shield | Security hardening tests (14 tests) |
| `services/shield/sovereign_verify.go` | shield | Sovereign multi-sig (modified: +PrecomputeSovereignSigs, +VerifySovereignFromSophia) |
| `services/shield/sovereign_verify_test.go` | shield | Sovereign verification tests (7 tests) |
| `pkg/pqc_policy/policy.go` | pqc_policy | PolicyChecker interface + 7-point check |
| `pkg/pqc_policy/parser.go` | pqc_policy | YAML policy parsing |
| `pkg/pqc_policy/nist_level.go` | pqc_policy | NIST security level mapping |
| `pkg/pqc_policy/policy_test.go` | pqc_policy | Policy SDK tests (9 tests) |
| `pkg/crypto/pqc/kem_tunnel.go` | pqc | KEM tunnel key derivation + HKDF |
| `pkg/crypto/pqc/kem_tunnel_test.go` | pqc | KEM tunnel tests (6 tests) |
| `cmd/pqc-verifier/main.go` | main | PQC verifier daemon entry point |
| `cmd/pqc-verifier/config.go` | main | Daemon configuration |
| `cmd/pqc-verifier/handlers.go` | main | HTTP API handlers |
| `cmd/pqc-verifier/wotan.go` | main | Wotan event integration |
| `pkg/ports/ports.go` | ports | Added `PQCVerifier = 19008` |
| `tests/pqc/integration_test.go` | pqc (test) | E2E tests (+6 new tests) |
| `tests/pqc/benchmark_test.go` | pqc (test) | Benchmarks (+5 new benchmarks) |

### Pre-Existing Foundation

These files were built before the sprint and provide the foundation:

| File | Package | Purpose |
|------|---------|---------|
| `pkg/crypto/pqc/registry.go` | pqc | Algorithm registry (5 algorithms) |
| `pkg/crypto/pqc/ml_dsa.go` | pqc | ML-DSA implementation (circl) |
| `pkg/crypto/pqc/ml_kem.go` | pqc | ML-KEM implementation (circl) |
| `pkg/crypto/pqc/security.go` | pqc | AlgoConfusionGuard, SigRefRateLimiter |
| `services/shield/pqc_verify.go` | shield | 7-point verification pipeline |
| `services/shield/kem_tunnel.go` | shield | KEM tunnel state machine |
| `services/shield/layer2_policy.go` | shield | Per-app YAML policy engine |
| `pkg/monad/pqc_value.go` | monad | PQC value (12 bytes) marshal/unmarshal |
| `pkg/monad/pseudo_header.go` | monad | Pseudo-header (52 bytes) |
| `pkg/wotan/state.go` | wotan | PQC state (40 bytes) in protocol RAM |
| `pkg/policy/verification.go` | policy | Compliance tiers + tier policies |
| `pkg/audit/pqc_events.go` | audit | 8 event types, 10 Wotan topics |
| `pkg/metrics/pqc_metrics.go` | metrics | 8 Prometheus metrics |
| `pkg/sophia/pqc_maps.go` | sophia | 5 BPF map types + PQCMapManager |
| `services/keymgmt/` | keymgmt | Key lifecycle service |

---

## Performance

Benchmark results on AMD Ryzen 5 7600X (6-core):

| Operation | ops/sec | Latency | Allocs/op |
|-----------|---------|---------|-----------|
| Cached verification (steps 1-7) | 56,184 | 2.4 µs | 4 |
| First packet verification | 1,815 | 63 µs | 20 |
| ML-DSA-65 sign + verify | 967 | 433 µs | 9 |
| ML-KEM-768 encap + decap | 2,151 | 55 µs | 6 |
| HKDF key derivation | 100,417 | 1.1 µs | 17 |
| Tier verification dispatch | 48,492 | 2.8 µs | 7 |
| Sovereign verification | 50,545 | 2.1 µs | 12 |
| KEM handshake (full) | 1,956 | 59 µs | 50 |
| Policy check (7-point) | 2,644,575 | 46 ns | 0 |
| PQCValue marshal | 164M | 0.7 ns | 0 |
| PQCValue unmarshal | 341M | 0.4 ns | 0 |

---

## References

- `docs/ARCHITECTURE.md` — Top-level system architecture
- `docs/architecture/MICROSERVICES.md` — Gnostic microservice layer
- `docs/architecture/KINGDOM_ARCHITECTURE.md` — Kingdom topology
- `pkg/ports/ports.go` — Doom Range port registry (single source of truth)
- `CLAUDE.md` — Development conventions and project context
- [NIST Post-Quantum Cryptography Standards](https://csrc.nist.gov/projects/post-quantum-cryptography)
- [Cloudflare CIRCL](https://github.com/cloudflare/circl) — Go cryptographic library (ML-DSA, ML-KEM)
- [RFC 5869](https://tools.ietf.org/html/rfc5869) — HKDF: HMAC-based Extract-and-Expand Key Derivation Function
