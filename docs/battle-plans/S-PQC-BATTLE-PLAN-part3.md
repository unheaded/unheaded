# S-PQC BATTLE PLAN — Part 3: Phases 13-18 + Appendices (Steps 436-510+)
*Continued from Part 2*

---

## PHASE 13: KEM INTEGRATION — ML-KEM & HQC (Steps 436-465)

**GOAL:** Implement Section 16 — Key Encapsulation Mechanisms for Shield-to-Shield tunnel key establishment. Establish post-quantum secure shared secrets for per-flow signing keys.

**PREREQUISITE:** Phase 12 (SLH-DSA, ML-DSA, FN-DSA complete), Sophia map infrastructure, HKDF-SHA256 implementation.

**TIME ESTIMATE:** ~18m | **AGENT:** Crypto/Shield (primary), Daemon (secondary) | **PARALLELIZABLE:** No (depends on Phase 12)

**EXIT GATE:** ML-KEM-768 encapsulation/decapsulation roundtrip completes. Derived signing key produces valid SLH-DSA signatures on test packets.

---

### Step 436: [B] KEM Parameter Set Registry in Sophia
**~2m | Create KEM algo registry**

Define KEM algorithm identifiers and NIST security level mappings in Sophia:

```go
// services/sophia/kem_registry.go
const (
    KEM_MLKEM_512  = 0x80  // FIPS 203, L1, CT=768B
    KEM_MLKEM_768  = 0x81  // FIPS 203, L3, CT=1088B
    KEM_MLKEM_1024 = 0x82  // FIPS 203, L5, CT=1568B
    KEM_HQC_128    = 0x90  // FIPS 207, L1, CT=4497B
    KEM_HQC_192    = 0x91  // FIPS 207, L3, CT=9042B
)

type KEMEntry struct {
    AlgoID     uint8
    Name       string
    Standard   string  // "FIPS 203" or "FIPS 207"
    Level      uint8   // 1, 3, or 5
    CiphertextSize uint16
    SharedSecretSize uint16
    PublicKeySize  uint16
    SecretKeySize  uint16
}

var KEMRegistry = map[uint8]KEMEntry{
    KEM_MLKEM_512: {
        AlgoID: 0x80, Name: "ML-KEM-512", Standard: "FIPS 203", Level: 1,
        CiphertextSize: 768, SharedSecretSize: 32, PublicKeySize: 800, SecretKeySize: 1632,
    },
    KEM_MLKEM_768: {
        AlgoID: 0x81, Name: "ML-KEM-768", Standard: "FIPS 203", Level: 3,
        CiphertextSize: 1088, SharedSecretSize: 32, PublicKeySize: 1184, SecretKeySize: 2400,
    },
    KEM_MLKEM_1024: {
        AlgoID: 0x82, Name: "ML-KEM-1024", Standard: "FIPS 203", Level: 5,
        CiphertextSize: 1568, SharedSecretSize: 32, PublicKeySize: 1568, SecretKeySize: 3168,
    },
    KEM_HQC_128: {
        AlgoID: 0x90, Name: "HQC-128", Standard: "FIPS 207", Level: 1,
        CiphertextSize: 4497, SharedSecretSize: 32, PublicKeySize: 2249, SecretKeySize: 2249,
    },
    KEM_HQC_192: {
        AlgoID: 0x91, Name: "HQC-192", Standard: "FIPS 207", Level: 3,
        CiphertextSize: 9042, SharedSecretSize: 32, PublicKeySize: 4522, SecretKeySize: 4522,
    },
}

func GetKEMLevel(algoID uint8) (uint8, error) {
    entry, ok := KEMRegistry[algoID]
    if !ok {
        return 0, fmt.Errorf("unknown KEM algo 0x%02x", algoID)
    }
    return entry.Level, nil
}
```

[V] Verify registry loads in Sophia test suite. Confirm NIST levels match FIPS spec tables.

---

### Step 437: [B] ML-KEM Wrapper — liboqs Integration
**~3m | Add ML-KEM keygen/encap/decap**

Integrate liboqs (Open Quantum Safe library) ML-KEM functions:

```go
// services/sophia/kem_wrapper.go
package sophia

import "C"
import (
    "crypto/rand"
    "fmt"
    "unsafe"
)

// MLKEMKeypair generates a fresh (pk, sk) pair
func MLKEMKeypair(algoID uint8) (publicKey []byte, secretKey []byte, err error) {
    entry, ok := KEMRegistry[algoID]
    if !ok {
        return nil, nil, fmt.Errorf("invalid KEM algo 0x%02x", algoID)
    }

    pk := make([]byte, entry.PublicKeySize)
    sk := make([]byte, entry.SecretKeySize)

    // liboqs C call (OQS_KEM_ml_kem_512_keypair, etc)
    algName := C.CString(entry.Name)
    defer C.free(unsafe.Pointer(algName))

    ret := C.OQS_KEM_keypair(algName, (*C.uint8_t)(unsafe.Pointer(&pk[0])), (*C.uint8_t)(unsafe.Pointer(&sk[0])))
    if ret != 0 {
        return nil, nil, fmt.Errorf("KEM keygen failed: %d", ret)
    }

    return pk, sk, nil
}

// MLKEMEncapsulate generates shared secret and ciphertext
func MLKEMEncapsulate(algoID uint8, publicKey []byte) (ciphertext []byte, sharedSecret []byte, err error) {
    entry, ok := KEMRegistry[algoID]
    if !ok {
        return nil, nil, fmt.Errorf("invalid KEM algo 0x%02x", algoID)
    }

    ct := make([]byte, entry.CiphertextSize)
    ss := make([]byte, entry.SharedSecretSize)

    algName := C.CString(entry.Name)
    defer C.free(unsafe.Pointer(algName))

    ret := C.OQS_KEM_encaps(algName,
        (*C.uint8_t)(unsafe.Pointer(&ct[0])),
        (*C.uint8_t)(unsafe.Pointer(&ss[0])),
        (*C.uint8_t)(unsafe.Pointer(&publicKey[0])))
    if ret != 0 {
        return nil, nil, fmt.Errorf("KEM encap failed: %d", ret)
    }

    return ct, ss, nil
}

// MLKEMDecapsulate recovers shared secret from ciphertext
func MLKEMDecapsulate(algoID uint8, ciphertext []byte, secretKey []byte) (sharedSecret []byte, err error) {
    entry, ok := KEMRegistry[algoID]
    if !ok {
        return nil, fmt.Errorf("invalid KEM algo 0x%02x", algoID)
    }

    ss := make([]byte, entry.SharedSecretSize)

    algName := C.CString(entry.Name)
    defer C.free(unsafe.Pointer(algName))

    ret := C.OQS_KEM_decaps(algName,
        (*C.uint8_t)(unsafe.Pointer(&ss[0])),
        (*C.uint8_t)(unsafe.Pointer(&ciphertext[0])),
        (*C.uint8_t)(unsafe.Pointer(&secretKey[0])))
    if ret != 0 {
        return nil, fmt.Errorf("KEM decap failed: %d", ret)
    }

    return ss, nil
}
```

[V] TDD: test keygen produces correct key sizes. Test encap/decap roundtrip.

---

### Step 438: [C] Checkpoint: KEM Wrappers Built & Tested
**~1m | Commit**

```bash
git add services/sophia/kem_registry.go services/sophia/kem_wrapper.go
git commit -m "feat(sophia): Add ML-KEM & HQC parameter registries and liboqs wrappers

Implements FIPS 203 (ML-KEM) and FIPS 207 (HQC) Key Encapsulation Mechanisms.
Parameter sets: ML-KEM-512/768/1024, HQC-128/192. Keygen, encap, decap
operations with size validation. Exit gate: roundtrip tests passing.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 439: [B] HKDF-SHA256 Key Derivation for Tunnel Keys
**~2m | Derive per-flow signing keys from KEM shared secret**

Implement HKDF to stretch KEM shared secret → signing keys:

```go
// services/sophia/hkdf_derive.go
package sophia

import (
    "crypto/sha256"
    "fmt"
    "golang.org/x/crypto/hkdf"
)

// DeriveSigningKey derives a signing key from KEM shared secret
// Uses HKDF-SHA256 with domain separation
func DeriveSigningKey(sharedSecret []byte, flowID uint64, epoch uint32) (signingKey []byte, err error) {
    if len(sharedSecret) < 32 {
        return nil, fmt.Errorf("shared secret too short: %d bytes", len(sharedSecret))
    }

    // Domain separation: include flow context
    info := make([]byte, 0, 20)
    info = append(info, "PQC-FLOW-KEY"...)
    info = append(info, byte((flowID>>56)&0xFF), byte((flowID>>48)&0xFF), byte((flowID>>40)&0xFF), byte((flowID>>32)&0xFF))
    info = append(info, byte((epoch>>24)&0xFF), byte((epoch>>16)&0xFF), byte((epoch>>8)&0xFF), byte(epoch&0xFF))

    // HKDF-Expand: 64 bytes for ML-DSA signing key (32B scalar + 32B nonce)
    kdf := hkdf.New(sha256.New, sharedSecret, nil, info)
    signingKey = make([]byte, 64)
    _, err = kdf.Read(signingKey)
    if err != nil {
        return nil, fmt.Errorf("HKDF derivation failed: %w", err)
    }

    return signingKey, nil
}

// DeriveTunnelKeys derives multiple keys from shared secret
func DeriveTunnelKeys(sharedSecret []byte, flowID uint64, epoch uint32) (map[string][]byte, error) {
    keys := make(map[string][]byte)

    for keyType, info := range map[string]string{
        "signing": "SIGN",
        "verify": "VERIFY",
        "audit": "AUDIT",
    } {
        kdf := hkdf.New(sha256.New, sharedSecret, nil, []byte(info))
        key := make([]byte, 32)
        _, err := kdf.Read(key)
        if err != nil {
            return nil, err
        }
        keys[keyType] = key
    }

    return keys, nil
}
```

[V] Test HKDF output for correct size and domain separation.

---

### Step 440: [B] Sophia KEM Storage Map — sophia_kem_pk
**~2m | Create persistent KEM public key storage**

Add Sophia map for tunnel responder's KEM public keys:

```go
// services/sophia/maps.go (extend existing)
const (
    // ...existing maps...
    MAP_KEM_PK = "sophia_kem_pk"
)

type KEMPublicKeyEntry struct {
    AlgoID      uint8    // KEM algo ID (0x80-0x82, 0x90-0x91)
    KeyRef      uint32   // Reference ID for audit trail
    PublicKey   [1568]byte // Max size: ML-KEM-1024 (1568B)
    PublicKeyLen uint16  // Actual key length
    Epoch       uint32   // Key rotation epoch
    Timestamp   uint64   // Creation time (unix ns)
    Flags       uint8    // b0=active, b1=deprecated
}

func (s *Service) StoreKEMPublicKey(shieldID uint64, entry *KEMPublicKeyEntry) error {
    return s.sophia.Set(MAP_KEM_PK, shieldID, entry)
}

func (s *Service) FetchKEMPublicKey(shieldID uint64) (*KEMPublicKeyEntry, error) {
    var entry KEMPublicKeyEntry
    err := s.sophia.Get(MAP_KEM_PK, shieldID, &entry)
    return &entry, err
}
```

[V] Verify map pin succeeds. Test store/retrieve roundtrip.

---

### Step 441: [B] Shield Tunnel Negotiation Protocol — Initiator Flow
**~3m | Initiator generates encapsulation, sends via Sophia control channel**

Implement KEM encapsulation on tunnel initiator (flow sender):

```go
// services/shield/kem_tunnel_initiator.go
package shield

import (
    "crypto/sha256"
    "encoding/binary"
    "github.com/unheaded/unheaded/services/sophia"
)

// InitiateTunnelNegotiation generates KEM ciphertext for shared key establishment
func (s *Shield) InitiateTunnelNegotiation(ctx context.Context, responderID uint64, kemAlgoID uint8) (*TunnelInitMessage, error) {
    // 1. Fetch responder's KEM public key from Sophia
    respPKEntry, err := s.sophia.FetchKEMPublicKey(responderID)
    if err != nil {
        return nil, fmt.Errorf("fetch responder KEM pk: %w", err)
    }

    // 2. Validate algo matches
    if respPKEntry.AlgoID != kemAlgoID {
        return nil, fmt.Errorf("algo mismatch: requested 0x%02x, responder has 0x%02x", kemAlgoID, respPKEntry.AlgoID)
    }

    // 3. Encapsulate (generate ct + shared secret)
    ct, ss, err := sophia.MLKEMEncapsulate(kemAlgoID, respPKEntry.PublicKey[:respPKEntry.PublicKeyLen])
    if err != nil {
        return nil, fmt.Errorf("KEM encap: %w", err)
    }

    // 4. Derive signing keys from shared secret
    signingKey, err := sophia.DeriveSigningKey(ss, s.flowID, s.currentEpoch)
    if err != nil {
        return nil, fmt.Errorf("derive signing key: %w", err)
    }

    // 5. Cache derived key in local memory (volatile)
    s.mu.Lock()
    s.tunnelSigningKey = signingKey
    s.tunnelEpoch = s.currentEpoch
    s.mu.Unlock()

    // 6. Send ciphertext via Sophia control channel
    // (CT too large for Monad HbH, must use separate transport)
    msg := &TunnelInitMessage{
        InitiatorID: s.localShieldID,
        ResponderID: responderID,
        KEMAlgoID:   kemAlgoID,
        Ciphertext:  ct,
        Flags:       0x00, // Reserved
    }

    return msg, nil
}

type TunnelInitMessage struct {
    InitiatorID uint64
    ResponderID uint64
    KEMAlgoID   uint8
    Ciphertext  []byte // ML-KEM CT (768-1568B)
    Flags       uint8
}
```

[D] Debug: If encap fails, check public key size against registry. Verify sha256.Sum256 of SS is not zero.

---

### Step 442: [B] Shield Tunnel Negotiation Protocol — Responder Flow
**~3m | Responder decapsulates, derives shared secret, stores tunnel key**

Implement KEM decapsulation on tunnel responder (flow receiver):

```go
// services/shield/kem_tunnel_responder.go
package shield

// ProcessTunnelNegotiation decapsulates and establishes shared key
func (s *Shield) ProcessTunnelNegotiation(ctx context.Context, msg *TunnelInitMessage) error {
    // 1. Validate initiator
    if msg.InitiatorID == 0 || msg.ResponderID != s.localShieldID {
        return fmt.Errorf("invalid tunnel message: self=%d, responder=%d", s.localShieldID, msg.ResponderID)
    }

    // 2. Fetch OUR secret key (we are the responder)
    skEntry, err := s.sophia.FetchKEMSecretKey(s.localShieldID)
    if err != nil {
        return fmt.Errorf("fetch our KEM sk: %w", err)
    }

    // 3. Validate ciphertext size
    kemEntry, ok := sophia.KEMRegistry[msg.KEMAlgoID]
    if !ok {
        return fmt.Errorf("unknown KEM algo 0x%02x", msg.KEMAlgoID)
    }
    if len(msg.Ciphertext) != int(kemEntry.CiphertextSize) {
        return fmt.Errorf("CT size mismatch: got %d, expect %d", len(msg.Ciphertext), kemEntry.CiphertextSize)
    }

    // 4. Decapsulate
    ss, err := sophia.MLKEMDecapsulate(msg.KEMAlgoID, msg.Ciphertext, skEntry.SecretKey[:skEntry.SecretKeyLen])
    if err != nil {
        return fmt.Errorf("KEM decap: %w", err)
    }

    // 5. Derive signing keys
    signingKey, err := sophia.DeriveSigningKey(ss, msg.InitiatorID, s.currentEpoch)
    if err != nil {
        return nil, fmt.Errorf("derive signing key: %w", err)
    }

    // 6. Cache tunnel key
    s.mu.Lock()
    s.tunnelSigningKeys[msg.InitiatorID] = signingKey
    s.tunnelEpochs[msg.InitiatorID] = s.currentEpoch
    s.mu.Unlock()

    // 7. Publish event to Anamnesis
    s.PublishEvent("PQC_TUNNEL_ESTABLISHED", map[string]interface{}{
        "initiator_id": msg.InitiatorID,
        "responder_id": msg.ResponderID,
        "kem_algo": msg.KEMAlgoID,
    })

    return nil
}
```

[V] TDD: test decap of valid CT. Test rejection of wrong size CT.

---

### Step 443: [B] Sophia KEM Secret Key Storage — sophia_kem_sk
**~1m | Secure storage of responder secret keys**

Add Sophia map for responder secret key storage (encrypted at rest):

```go
// services/sophia/maps.go (extend)
const (
    MAP_KEM_SK = "sophia_kem_sk"
)

type KEMSecretKeyEntry struct {
    AlgoID       uint8       // KEM algo ID
    SecretKey    [3168]byte  // Max: ML-KEM-1024 (3168B)
    SecretKeyLen uint16
    Epoch        uint32
    Timestamp    uint64
    Flags        uint8       // b0=active
}

func (s *Service) StoreKEMSecretKey(shieldID uint64, entry *KEMSecretKeyEntry) error {
    // TODO: Encrypt with Shield's master key before storing
    return s.sophia.Set(MAP_KEM_SK, shieldID, entry)
}

func (s *Service) FetchKEMSecretKey(shieldID uint64) (*KEMSecretKeyEntry, error) {
    var entry KEMSecretKeyEntry
    err := s.sophia.Get(MAP_KEM_SK, shieldID, &entry)
    // TODO: Decrypt with Shield's master key after retrieval
    return &entry, err
}
```

[V] Verify SK never leaves memory unencrypted. Check encryption key derivation.

---

### Step 444: [C] Checkpoint: Tunnel Negotiation & Key Derivation Complete
**~1m | Commit**

```bash
git add services/shield/kem_tunnel_*.go services/sophia/hkdf_derive.go services/sophia/maps.go
git commit -m "feat(shield,sophia): Implement KEM tunnel negotiation protocol

Initiator encapsulates using responder's public key, sends ciphertext via
Sophia control channel. Responder decapsulates with secret key, derives
shared signing key via HKDF-SHA256. Tunnel keys cached in Shield memory.
Exit gate: Initiator→Responder roundtrip derives identical signing keys.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 445: [B] Multi-Algorithm KEM Support — HQC as Code-Based Fallback
**~2m | Add HQC wrapper, dual-KEM support**

Extend sophia to support HQC (code-based, non-lattice):

```go
// services/sophia/kem_wrapper.go (extend)

// HQCKeypair generates HQC keypair
func HQCKeypair(algoID uint8) (publicKey []byte, secretKey []byte, err error) {
    entry, ok := KEMRegistry[algoID]
    if algoID != 0x90 && algoID != 0x91 {
        return nil, nil, fmt.Errorf("not an HQC algo: 0x%02x", algoID)
    }

    pk := make([]byte, entry.PublicKeySize)
    sk := make([]byte, entry.SecretKeySize)

    algName := C.CString(entry.Name)
    defer C.free(unsafe.Pointer(algName))

    ret := C.OQS_KEM_keypair(algName, (*C.uint8_t)(unsafe.Pointer(&pk[0])), (*C.uint8_t)(unsafe.Pointer(&sk[0])))
    if ret != 0 {
        return nil, nil, fmt.Errorf("HQC keygen failed: %d", ret)
    }

    return pk, sk, nil
}

// SelectKEM chooses between ML-KEM (preferred) and HQC (fallback)
func SelectKEM(preferredAlgo uint8, allowedAlgos []uint8) (uint8, error) {
    if preferredAlgo != 0 {
        for _, allowed := range allowedAlgos {
            if allowed == preferredAlgo {
                return preferredAlgo, nil
            }
        }
    }

    // Prefer lattice (ML-KEM)
    mlkemPref := []uint8{0x81, 0x82, 0x80}
    for _, algo := range mlkemPref {
        for _, allowed := range allowedAlgos {
            if allowed == algo {
                return algo, nil
            }
        }
    }

    // Fallback to HQC (code-based)
    hqcPref := []uint8{0x91, 0x90}
    for _, algo := range hqcPref {
        for _, allowed := range allowedAlgos {
            if allowed == algo {
                return algo, nil
            }
        }
    }

    return 0, fmt.Errorf("no usable KEM algorithm found")
}
```

[V] Test SelectKEM prefers ML-KEM over HQC when both available.

---

### Step 446: [B] KEM Epoch Management & Key Rotation
**~2m | Plan KEM key rotation with grace period**

Extend sophia to handle KEM key rotation:

```go
// services/sophia/kem_rotation.go
package sophia

import "time"

type KEMEpochManager struct {
    currentEpoch    uint32
    nextEpochTime   time.Time
    gracePeriodSec  uint32
    rotationInterval uint32
}

func (m *KEMEpochManager) ShouldRotate() bool {
    return time.Now().After(m.nextEpochTime)
}

func (m *KEMEpochManager) RotateKEM(algoID uint8) (pk []byte, sk []byte, err error) {
    m.currentEpoch++

    // Generate new keypair
    pk, sk, err = MLKEMKeypair(algoID)
    if err != nil {
        return nil, nil, err
    }

    // Schedule next rotation
    m.nextEpochTime = time.Now().Add(time.Duration(m.rotationInterval) * time.Second)

    return pk, sk, nil
}

// GracePeriodWindow returns true if flow is in grace period (allow old keys)
func (m *KEMEpochManager) InGracePeriod(flowEpoch uint32) bool {
    epochDiff := m.currentEpoch - flowEpoch
    return epochDiff <= 1 // Allow current + 1 previous epoch
}
```

[V] Test grace period correctly allows stale keys. Test rejects too-old keys.

---

### Step 447: [C] Checkpoint: Multi-Algorithm KEM & Rotation Complete
**~1m | Commit**

```bash
git add services/sophia/kem_wrapper.go services/sophia/kem_rotation.go
git commit -m "feat(sophia): Add HQC support and KEM epoch rotation

HQC-128/192 available as code-based fallback when lattice assumptions
compromised. SelectKEM() implements preference logic (lattice → code-based).
KEMEpochManager handles key rotation with grace period. Exit gate: can
rotate KEM keys while maintaining backward compatibility.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 448: [B] Rate Limiting for KEM Encapsulations — Prevent SigRef Exhaustion
**~2m | Limit KEM tunnel setups from untrusted sources**

Add rate limiting to KEM tunnel initiation:

```go
// services/shield/kem_ratelimit.go
package shield

import (
    "sync"
    "time"
)

type KEMRateLimiter struct {
    mu           sync.RWMutex
    tokens       map[uint64]int       // initiator_id -> available tokens
    refillRate   int                  // tokens per second
    maxTokens    int                  // burst capacity
    lastRefill   time.Time
}

func (rl *KEMRateLimiter) AllowTunnelInit(initiatorID uint64) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(rl.lastRefill).Seconds()
    if elapsed > 0 {
        newTokens := int(elapsed * float64(rl.refillRate))
        rl.lastRefill = now
        for id := range rl.tokens {
            rl.tokens[id] = min(rl.tokens[id]+newTokens, rl.maxTokens)
        }
    }

    tokens := rl.tokens[initiatorID]
    if tokens > 0 {
        rl.tokens[initiatorID]--
        return true
    }
    return false
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

[V] Test rate limiter accepts first tunnel init. Test rejects burst > maxTokens.

---

### Step 449: [B] KEM Integration Tests — Roundtrip Verification
**~3m | TDD: verify encap/decap produces matching shared secrets**

Write comprehensive KEM tests:

```go
// services/sophia/kem_test.go
package sophia

import (
    "testing"
    "bytes"
)

func TestMLKEMRoundtrip(t *testing.T) {
    tests := []uint8{0x80, 0x81, 0x82} // ML-KEM-512/768/1024

    for _, algoID := range tests {
        t.Run(fmt.Sprintf("algo_0x%02x", algoID), func(t *testing.T) {
            // 1. Generate keypair
            pk, sk, err := MLKEMKeypair(algoID)
            if err != nil {
                t.Fatalf("keygen failed: %v", err)
            }

            // 2. Encapsulate
            ct, ss1, err := MLKEMEncapsulate(algoID, pk)
            if err != nil {
                t.Fatalf("encap failed: %v", err)
            }

            // 3. Decapsulate
            ss2, err := MLKEMDecapsulate(algoID, ct, sk)
            if err != nil {
                t.Fatalf("decap failed: %v", err)
            }

            // 4. Verify shared secrets match
            if !bytes.Equal(ss1, ss2) {
                t.Errorf("shared secrets don't match")
            }
        })
    }
}

func TestHQCRoundtrip(t *testing.T) {
    tests := []uint8{0x90, 0x91} // HQC-128/192

    for _, algoID := range tests {
        t.Run(fmt.Sprintf("algo_0x%02x", algoID), func(t *testing.T) {
            pk, sk, err := HQCKeypair(algoID)
            if err != nil {
                t.Fatalf("keygen failed: %v", err)
            }

            ct, ss1, err := MLKEMEncapsulate(algoID, pk)
            if err != nil {
                t.Fatalf("encap failed: %v", err)
            }

            ss2, err := MLKEMDecapsulate(algoID, ct, sk)
            if err != nil {
                t.Fatalf("decap failed: %v", err)
            }

            if !bytes.Equal(ss1, ss2) {
                t.Errorf("HQC shared secrets don't match")
            }
        })
    }
}

func TestDeriveSigningKey(t *testing.T) {
    ss := make([]byte, 32)
    rand.Read(ss)

    key1, _ := DeriveSigningKey(ss, 0x1234567890ABCDEF, 100)
    key2, _ := DeriveSigningKey(ss, 0x1234567890ABCDEF, 100)

    if !bytes.Equal(key1, key2) {
        t.Errorf("derivation not deterministic")
    }

    key3, _ := DeriveSigningKey(ss, 0x1111111111111111, 100)
    if bytes.Equal(key1, key3) {
        t.Errorf("different flows produced same key")
    }
}
```

[V] All tests pass. 100% coverage of KEM wrapper functions.

---

### Step 450: [C] Checkpoint: KEM Integration Complete
**~1m | Commit**

```bash
git add services/sophia/kem_test.go services/shield/kem_ratelimit.go
git commit -m "test(sophia,shield): Add comprehensive KEM roundtrip tests

Tests verify ML-KEM-512/768/1024 and HQC-128/192 encapsulation/decapsulation.
Signing key derivation is deterministic and flow-specific. Rate limiter
prevents SigRef exhaustion from tunnel negotiation. Exit gate: all KEM tests
passing, 100% coverage.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 451: [B] Performance Baseline — KEM Operations Latency
**~2m | Benchmark KEM operations**

```go
// services/sophia/bench_kem_test.go
package sophia

import "testing"

func BenchmarkMLKEMKeypair(b *testing.B) {
    for i := 0; i < b.N; i++ {
        MLKEMKeypair(0x81) // ML-KEM-768
    }
}

func BenchmarkMLKEMEncap(b *testing.B) {
    pk, _, _ := MLKEMKeypair(0x81)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        MLKEMEncapsulate(0x81, pk)
    }
}

func BenchmarkMLKEMDecap(b *testing.B) {
    pk, sk, _ := MLKEMKeypair(0x81)
    ct, _, _ := MLKEMEncapsulate(0x81, pk)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        MLKEMDecapsulate(0x81, ct, sk)
    }
}

func BenchmarkHKDFDerive(b *testing.B) {
    ss := make([]byte, 32)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        DeriveSigningKey(ss, 0x1234567890ABCDEF, 100)
    }
}
```

Target: keygen < 50ms, encap < 30ms, decap < 30ms, derive < 1ms.

[V] Run benchmarks. Document results for Phase 18 E2E.

---

### Step 452: [B] Sophia HTTP API — KEM Operations Exposure
**~2m | REST endpoints for KEM tunnel management**

Extend Sophia service HTTP API:

```go
// services/sophia/server.go (extend)
package sophia

func (s *Service) RegisterKEMRoutes() {
    s.router.Post("/api/v1/kem/keypair/:algo", s.handleKEMKeypair)
    s.router.Post("/api/v1/kem/encapsulate", s.handleKEMEncap)
    s.router.Post("/api/v1/kem/decapsulate", s.handleKEMDecap)
    s.router.Get("/api/v1/kem/registry", s.handleKEMRegistry)
}

func (s *Service) handleKEMRegistry(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(KEMRegistry)
}

type KEMEncapRequest struct {
    AlgoID    uint8  `json:"algo_id"`
    PublicKey string `json:"public_key_b64"`
}

func (s *Service) handleKEMEncap(w http.ResponseWriter, r *http.Request) {
    var req KEMEncapRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    pk, _ := base64.StdEncoding.DecodeString(req.PublicKey)
    ct, ss, err := MLKEMEncapsulate(req.AlgoID, pk)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "ciphertext": base64.StdEncoding.EncodeToString(ct),
        "shared_secret_digest": fmt.Sprintf("%x", sha256.Sum256(ss)),
    })
}
```

[V] Test endpoints with curl. Verify all APIs return proper JSON.

---

### Step 453: [D] Debug: KEM Library Integration Issues
**~2m | Troubleshoot liboqs linking**

If liboqs linking fails:

```bash
# Check liboqs install
pkg-config --cflags --libs liboqs

# Fallback: vendored liboqs
git clone https://github.com/open-quantum-safe/liboqs.git
cd liboqs && mkdir build && cd build
cmake .. -DBUILD_SHARED_LIBS=ON
make && sudo make install
ldconfig

# Rebuild Sophia
cd services/sophia && CGO_LDFLAGS="-loqs" go build .
```

If runtime link fails, add rpath:

```go
// #cgo LDFLAGS: -L/usr/local/lib -loqs -Wl,-rpath,/usr/local/lib
import "C"
```

---

### Step 454: [B] KEM State Machine — Flow-Level Tracking
**~2m | Track tunnel establishment state per flow**

```go
// services/shield/kem_state.go
package shield

type KEMFlowState int

const (
    KEMStateNone KEMFlowState = iota
    KEMStateInitiating        // Waiting for responder CT
    KEMStateEstablished       // Tunnel key derived
    KEMStateRotating          // Old key in grace period
    KEMStateExpired           // Key age exceeded
)

type KEMFlowTracker struct {
    mu              sync.RWMutex
    flows           map[uint64]*KEMFlowEntry
    maxAge          time.Duration
    gracePeriod     time.Duration
}

type KEMFlowEntry struct {
    State           KEMFlowState
    InitTime        time.Time
    KeyEpoch        uint32
    SigningKey      []byte
    ExpectedCiphertext []byte // Pending from initiator
}

func (t *KEMFlowTracker) TransitionState(flowID uint64, newState KEMFlowState) error {
    t.mu.Lock()
    defer t.mu.Unlock()

    entry, ok := t.flows[flowID]
    if !ok {
        return fmt.Errorf("flow %d not tracked", flowID)
    }

    entry.State = newState
    return nil
}
```

[V] Test state transitions. Verify expired flows are cleaned up.

---

### Step 455: [C] Checkpoint: Phase 13 Complete — KEM Integration Ready
**~1m | Commit**

```bash
git add services/sophia/server.go services/shield/kem_state.go services/sophia/bench_kem_test.go
git commit -m "feat(sophia,shield): Complete KEM integration with HTTP API and state tracking

Sophia HTTP API exposes /kem/keypair, /kem/encapsulate, /kem/decapsulate.
Shield tracks per-flow KEM state machine. Rate limiting prevents tunnel
negotiation abuse. Performance benchmarks baseline KEM operations.
Exit gate: Phase 13 complete. Ready for Layer 2 policy (Phase 14).

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## PHASE 14: LAYER 2 APPLICATION POLICY (Steps 456-495)

**GOAL:** Implement Section 14 — Application-Level Policy Verification. Enforce per-app algorithm restrictions, NIST level requirements, key age limits, and fingerprint pinning.

**PREREQUISITE:** Phase 13 (KEM complete), Wotan state interface, Sophia maps, application identity context in Monad.

**TIME ESTIMATE:** ~20m | **AGENT:** Control Plane (primary) | **PARALLELIZABLE:** Yes (independent of Phase 13 data plane)

**EXIT GATE:** App-defined policy correctly rejects flows with wrong algo, insufficient NIST level, expired key, and mismatched fingerprint. Single-digit microsecond policy lookup.

---

### Step 456: [B] Sophia PQC Application Policy Map
**~2m | Create policy storage and query interface**

```go
// services/sophia/policy_map.go
package sophia

const (
    MAP_APP_POLICY = "sophia_pqc_app_policy"
)

type PQCAppPolicy struct {
    MinSecurityLevel    uint8              // 1, 3, or 5
    RequirePinnedKey    uint8              // 0 or 1
    NumAllowedAlgos     uint8              // 0-12
    Reserved            uint8              // For future use
    MaxKeyAgeSec        uint32             // 0 = unlimited
    AllowedAlgos        [12]uint8          // algo_id values
    PinnedFP            [32]byte           // SHA-256 fingerprint
    Timestamp           uint64             // Policy creation time
    Flags               uint8              // b0=enforced, b1=audit_only
}

func (s *Service) StoreAppPolicy(appID uint32, policy *PQCAppPolicy) error {
    return s.sophia.Set(MAP_APP_POLICY, uint64(appID), policy)
}

func (s *Service) FetchAppPolicy(appID uint32) (*PQCAppPolicy, error) {
    var policy PQCAppPolicy
    err := s.sophia.Get(MAP_APP_POLICY, uint64(appID), &policy)
    return &policy, err
}

func (p *PQCAppPolicy) IsEnforced() bool {
    return p.Flags&0x01 != 0
}

func (p *PQCAppPolicy) IsAuditOnly() bool {
    return p.Flags&0x02 != 0
}
```

[V] Verify map stores 8 policies without collision. Check policy size is < 128B.

---

### Step 457: [B] Layer 2 Verification Procedure Implementation
**~3m | Implement Section 14.2 verification logic**

```go
// services/shield/layer2_policy.go
package shield

import (
    "crypto/sha256"
    "fmt"
)

type Layer2Verifier struct {
    sophia *sophia.Service
    wotan  *wotan.Client
}

// VerifyLayer2Policy performs application-level policy checks
func (v *Layer2Verifier) VerifyLayer2Policy(ctx context.Context, flow *Flow, appID uint32) (approved bool, reason string, err error) {
    // 1. Read pqc_verified flag from Wotan
    pqcVerified, err := v.wotan.Read(0x0000FF04)
    if err != nil {
        return false, "wotan_read_error", err
    }
    if pqcVerified != 1 {
        return false, "pqc_not_verified", nil
    }

    // 2. Read pqc_algo_id from Wotan
    algoID, err := v.wotan.Read(0x0000FF05)
    if err != nil {
        return false, "wotan_read_error", err
    }

    // 3. Look up app policy
    policy, err := v.sophia.FetchAppPolicy(appID)
    if err != nil {
        // No policy = allow (permissive default)
        return true, "no_policy", nil
    }

    // 4. Check algo in allowed list
    algoAllowed := false
    for i := 0; i < int(policy.NumAllowedAlgos); i++ {
        if policy.AllowedAlgos[i] == byte(algoID) {
            algoAllowed = true
            break
        }
    }
    if !algoAllowed {
        return false, fmt.Sprintf("algo_not_allowed_0x%02x", algoID), nil
    }

    // 5. Map algo to NIST level, verify >= min
    level, err := sophia.GetSigningLevel(byte(algoID))
    if err != nil {
        return false, "unknown_algo", err
    }
    if level < policy.MinSecurityLevel {
        return false, fmt.Sprintf("insufficient_level_%d<%d", level, policy.MinSecurityLevel), nil
    }

    // 6. Check key fingerprint if pinning required
    if policy.RequirePinnedKey == 1 {
        keyFPFromWotan, _ := v.wotan.Read(0x0000FF10) // First 8 bytes of digest
        keyFP := policy.PinnedFP[0:8]
        if keyFPFromWotan != keyFP {
            return false, "key_fp_mismatch", nil
        }
    }

    // 7. Check key age if limit set
    if policy.MaxKeyAgeSec > 0 {
        keyAgeFromWotan, _ := v.wotan.Read(0x0000FF11) // Seconds since creation
        if keyAgeFromWotan > uint64(policy.MaxKeyAgeSec) {
            return false, fmt.Sprintf("key_expired_%ds", keyAgeFromWotan), nil
        }
    }

    // 8. All checks passed
    return true, "policy_approved", nil
}
```

[V] TDD: test approval with matching algo, denial with mismatched algo, fingerprint check.

---

### Step 458: [B] Go SDK — Application Policy Verification Wrapper
**~2m | Expose policy checks to app developers**

```go
// pkg/pqc_policy/policy.go
package pqc_policy

import (
    "context"
    "github.com/unheaded/unheaded/services/shield"
)

type PolicyChecker interface {
    VerifyLayer2(ctx context.Context, appID uint32) (approved bool, reason string, err error)
}

type DefaultChecker struct {
    verifier *shield.Layer2Verifier
}

func (dc *DefaultChecker) VerifyLayer2(ctx context.Context, appID uint32) (bool, string, error) {
    flow := shield.FlowFromContext(ctx)
    if flow == nil {
        return false, "no_flow_context", nil
    }
    return dc.verifier.VerifyLayer2Policy(ctx, flow, appID)
}

// NewPolicyChecker creates a policy checker wired to Shield/Wotan
func NewPolicyChecker(sophia *sophia.Service, wotan *wotan.Client) PolicyChecker {
    return &DefaultChecker{
        verifier: &shield.Layer2Verifier{sophia: sophia, wotan: wotan},
    }
}

// Example application usage:
// checker := pqc_policy.NewPolicyChecker(sophia, wotan)
// ok, reason, _ := checker.VerifyLayer2(ctx, 0x12345678)
// if !ok {
//     log.Warn().Str("reason", reason).Msg("policy check failed")
//     return errors.New(reason)
// }
```

[V] Test SDK integration. Verify context passing from HTTP handlers.

---

### Step 459: [B] Sophia Dictionary Syntax Parser — Policy Definition Language
**~3m | Parse policy definitions from YAML/JSON config**

```go
// services/sophia/policy_parser.go
package sophia

import "gopkg.in/yaml.v2"

type PolicyDefinition struct {
    AppID           uint32   `yaml:"app_id"`
    MinLevel        uint8    `yaml:"min_security_level"`
    RequirePin      bool     `yaml:"require_pinned_key"`
    AllowedAlgos    []string `yaml:"allowed_algos"` // ["SLH-DSA-SHA2-128s", "ML-DSA-65", ...]
    MaxKeyAgeSec    uint32   `yaml:"max_key_age_sec"`
    PinnedKeyFP     string   `yaml:"pinned_key_fp"` // hex-encoded SHA-256
    AuditOnly       bool     `yaml:"audit_only"`
}

func (s *Service) LoadPoliciesFromYAML(content []byte) error {
    var policies []PolicyDefinition
    if err := yaml.Unmarshal(content, &policies); err != nil {
        return err
    }

    for _, def := range policies {
        policy := &PQCAppPolicy{
            MinSecurityLevel: def.MinLevel,
            RequirePinnedKey: boolToByte(def.RequirePin),
            NumAllowedAlgos:  uint8(len(def.AllowedAlgos)),
            MaxKeyAgeSec:     def.MaxKeyAgeSec,
        }

        // Map algo names to IDs
        for i, algoName := range def.AllowedAlgos {
            id, err := s.AlgoNameToID(algoName)
            if err != nil {
                return err
            }
            policy.AllowedAlgos[i] = id
        }

        // Parse pinned FP
        if def.RequirePin {
            fp, _ := hex.DecodeString(def.PinnedKeyFP)
            copy(policy.PinnedFP[:], fp)
        }

        // Set flags
        if !def.AuditOnly {
            policy.Flags |= 0x01 // enforced
        } else {
            policy.Flags |= 0x02 // audit_only
        }

        s.StoreAppPolicy(def.AppID, policy)
    }

    return nil
}

func boolToByte(b bool) uint8 {
    if b {
        return 1
    }
    return 0
}

// Example YAML:
// - app_id: 0x12345678
//   min_security_level: 3
//   require_pinned_key: true
//   allowed_algos:
//     - SLH-DSA-SHA2-128s
//     - ML-DSA-65
//   max_key_age_sec: 86400
//   pinned_key_fp: "abcd1234..."
//   audit_only: false
```

[V] Test parser with sample policy. Verify all fields loaded correctly.

---

### Step 460: [B] Wotan Layer 2 State Interface — Policy Context Storage
**~2m | Extend Wotan state with policy results**

```go
// Address map extension in Wotan (pseudo-header area):
// 0x0000FF20: pqc_policy_verdict (1B: approved=1, rejected=0, audit=2)
// 0x0000FF21: pqc_policy_reason (1B: enum reason code)
// 0x0000FF22: pqc_policy_app_id (4B: which app this flow is for)
// 0x0000FF24: pqc_policy_audit_log (pointer to policy check result)

// services/wotan/wotan_extend.go
func (w *Wotan) WriteLayer2Verdict(verified bool, reasonCode uint8, appID uint32) {
    verdict := uint8(0)
    if verified {
        verdict = 1
    }
    w.Write(0x0000FF20, verdict)
    w.Write(0x0000FF21, reasonCode)
    w.WriteUint32(0x0000FF22, appID)
}

func (w *Wotan) ReadLayer2Verdict() (approved bool, reasonCode uint8, appID uint32) {
    verdict, _ := w.Read(0x0000FF20)
    reason, _ := w.Read(0x0000FF21)
    app, _ := w.ReadUint32(0x0000FF22)
    return verdict == 1, reason, app
}
```

[V] Verify Layer 2 state persists across checks. Test audit log writes.

---

### Step 461: [B] Layer 2 Policy Audit Trail — Anamnesis Integration
**~2m | Log policy verdicts for compliance**

```go
// services/shield/layer2_audit.go
package shield

import "time"

type Layer2AuditEvent struct {
    Timestamp       time.Time
    FlowID          uint64
    AppID           uint32
    AlgoID          uint8
    Verdict         bool          // approved
    Reason          string
    ReasonCode      uint8
    KeyAge          uint32
    KeyFPMatch      bool
    Level           uint8
    RequestPath     string
}

func (v *Layer2Verifier) AuditLayer2Check(evt *Layer2AuditEvent) error {
    return v.wotan.Publish("pqc.policy.check", evt)
}

// Audit event structure for dashboard
type Layer2Dashboard struct {
    TotalChecks     uint64
    ApprovedCount   uint64
    RejectedCount   uint64
    AuditOnlyCount  uint64
    TopRejectReason map[string]uint64
}
```

[V] Test audit events published to Wotan. Verify counts increment.

---

### Step 462: [B] Performance Tuning — Layer 2 Policy Lookup
**~2m | Optimize policy queries for < 1us latency**

```go
// services/sophia/policy_cache.go
package sophia

import (
    "sync"
    "time"
)

type PolicyCache struct {
    mu      sync.RWMutex
    cache   map[uint32]*PQCAppPolicy
    ttl     time.Duration
    lastRefresh time.Time
}

func (pc *PolicyCache) Get(appID uint32) (*PQCAppPolicy, bool) {
    pc.mu.RLock()
    defer pc.mu.RUnlock()

    policy, ok := pc.cache[appID]
    return policy, ok
}

func (pc *PolicyCache) Set(appID uint32, policy *PQCAppPolicy) {
    pc.mu.Lock()
    defer pc.mu.Unlock()

    pc.cache[appID] = policy
}

func (pc *PolicyCache) Refresh(sophia *Service) error {
    // Load all policies from Sophia maps
    // Called on startup and every TTL interval
    return nil
}

// Benchmark target: 50-200ns per policy lookup (in-memory cache)
```

[V] Benchmark cache lookups. Verify < 200ns median latency.

---

### Step 463: [C] Checkpoint: Layer 2 Policy Framework Complete
**~1m | Commit**

```bash
git add services/shield/layer2_policy.go services/sophia/policy_*.go pkg/pqc_policy/
git commit -m "feat(shield,sophia): Implement Layer 2 application policy verification

Per-app policies define allowed algorithms, NIST levels, key age limits,
key fingerprint pinning. Policy parser loads YAML definitions. Verification
procedure checks 7 constraints (algo, level, age, fingerprint, etc).
Audit trail in Anamnesis for compliance. Cache provides < 200ns lookups.
Exit gate: policy enforcement working, audit trail flowing.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 464: [B] SOVEREIGN Tier Policy Enforcement
**~2m | Mandate Layer 2 checks for SOVEREIGN tier**

```go
// services/shield/tier_policy_enforcement.go
package shield

func (s *Shield) EnforceLayer2ForSovereign(tier uint8, flow *Flow) error {
    if tier != TIER_SOVEREIGN {
        return nil // Layer 2 optional for other tiers
    }

    // SOVEREIGN tier MANDATES Layer 2 policy check
    appID := flow.ApplicationID
    approved, reason, err := s.layer2Verifier.VerifyLayer2Policy(s.ctx, flow, appID)

    if err != nil {
        return fmt.Errorf("layer2_check_error: %w", err)
    }

    if !approved {
        s.PublishEvent("PQC_LAYER2_REJECTION", map[string]interface{}{
            "flow_id": flow.ID,
            "app_id": appID,
            "reason": reason,
            "tier": tier,
        })
        return fmt.Errorf("layer2_rejected: %s", reason)
    }

    return nil
}
```

[V] Test SOVEREIGN tier rejects unapproved flows. Other tiers allow bypasses.

---

### Step 465: [C] Checkpoint: Phase 14 Complete — Layer 2 Ready
**~1m | Commit**

```bash
git add services/shield/tier_policy_enforcement.go
git commit -m "feat(shield): Enforce Layer 2 policy checks for SOVEREIGN tier

SOVEREIGN tier flows MUST pass application policy verification. Other
tiers can optionally bypass. Enforcement integrated with tier dispatch.
Exit gate: Phase 14 complete. Layer 2 prevents unauthorized algorithm use.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## PHASE 15: SOVEREIGN MULTI-SIGNATURE (Steps 496-525)

**GOAL:** Implement Section 15.4 — 2-of-3 cross-algorithm verification for SOVEREIGN tier. Require consensus from at least 2 distinct algorithm families to approve flow.

**PREREQUISITE:** Phase 12 (all signature algos), Phase 14 (policy), Sophia multi-sig storage, control plane sig pre-computation.

**TIME ESTIMATE:** ~18m | **AGENT:** Crypto/Verifier (primary) | **PARALLELIZABLE:** Yes

**EXIT GATE:** SOVEREIGN tier verifies 2 distinct signature families. Single-family compromise does not downgrade flow. Rejects if < 2 sigs verify.

---

### Step 496: [B] Sophia SOVEREIGN Multi-Signature Storage
**~2m | Create extended entry with 3-sig consensus**

```go
// services/sophia/sovereign_map.go
package sophia

const (
    MAP_SOVEREIGN_SIGS = "sophia_pqc_sovereign_sigs"
)

type SovereignEntry struct {
    PrimaryAlgo       uint8  // algo_id from Monad
    SecondaryAlgo     uint8  // Different family
    TertiaryAlgo      uint8  // Optional third
    Consensus         uint8  // Bitfield: b0=pri, b1=sec, b2=ter (set when verified)
    PrimarySigRef     uint32 // Reference to primary sig in Sophia
    SecondarySigRef   uint32
    TertiarySigRef    uint32
    FlowID            uint64
    Timestamp         uint64
    PrimaryFamilyID   uint8  // Map algo_id to family (1=lattice, 2=hash, 3=code)
    SecondaryFamilyID uint8
    TertiaryFamilyID  uint8
}

func (s *Service) StoreSovereignEntry(flowID uint64, entry *SovereignEntry) error {
    return s.sophia.Set(MAP_SOVEREIGN_SIGS, flowID, entry)
}

func (s *Service) FetchSovereignEntry(flowID uint64) (*SovereignEntry, error) {
    var entry SovereignEntry
    err := s.sophia.Get(MAP_SOVEREIGN_SIGS, flowID, &entry)
    return &entry, err
}

// Algorithm family mapping
const (
    FAMILY_LATTICE = 1 // ML-DSA, ML-KEM
    FAMILY_HASH    = 2 // SLH-DSA
    FAMILY_CODE    = 3 // HQC
    FAMILY_ISOGENY = 4 // CSIDH (future)
)

func GetAlgoFamily(algoID uint8) uint8 {
    switch algoID {
    case 0x10, 0x11, 0x12:
        return FAMILY_LATTICE // ML-DSA-44/65/87
    case 0x20, 0x21:
        return FAMILY_HASH // SLH-DSA-SHA2-128s/192s
    case 0x30:
        return FAMILY_HASH // SLH-DSA-SHAKE256-128s
    case 0x40, 0x41:
        return FAMILY_CODE // HQC-128/192
    case 0x50:
        return FAMILY_ISOGENY // CSIDH-512
    default:
        return 0
    }
}
```

[V] Verify algorithm families mapped correctly. Test family ID consistency.

---

### Step 497: [B] Control Plane Multi-Signature Pre-Computation
**~3m | Daemon generates 2-3 sigs before Shield receives packet**

```go
// services/unheaded-daemon/sovereign_sig_preparation.go
package daemon

import (
    "crypto/rand"
    "github.com/unheaded/unheaded/services/sophia"
)

type SovereignSigPrep struct {
    sophia    *sophia.Service
    monad     *monad.Service
    fndsa     *fndsa.Daemon // FN-DSA signing daemon
}

// PrecomputeSovereignSigs generates 2-3 signatures for a flow
func (ssp *SovereignSigPrep) PrecomputeSovereignSigs(ctx context.Context, flowID uint64, payload []byte) (*SovereignEntry, error) {
    entry := &SovereignEntry{
        FlowID:    flowID,
        Timestamp: uint64(time.Now().UnixNano()),
    }

    // 1. Generate primary signature (SLH-DSA from Monad)
    primaryAlgo := 0x20 // SLH-DSA-SHA2-128s (lattice-resistant)
    priSig, err := ssp.monad.SignWithAlgo(primaryAlgo, payload)
    if err != nil {
        return nil, fmt.Errorf("primary sig failed: %w", err)
    }
    priSigRef, _ := ssp.sophia.StoreSigRef(flowID, priSig)
    entry.PrimaryAlgo = primaryAlgo
    entry.PrimarySigRef = priSigRef
    entry.PrimaryFamilyID = sophia.GetAlgoFamily(primaryAlgo)

    // 2. Generate secondary signature (ML-DSA from Monad)
    secondaryAlgo := 0x11 // ML-DSA-65 (lattice-based)
    secSig, err := ssp.monad.SignWithAlgo(secondaryAlgo, payload)
    if err != nil {
        return nil, fmt.Errorf("secondary sig failed: %w", err)
    }
    secSigRef, _ := ssp.sophia.StoreSigRef(flowID, secSig)
    entry.SecondaryAlgo = secondaryAlgo
    entry.SecondarySigRef = secSigRef
    entry.SecondaryFamilyID = sophia.GetAlgoFamily(secondaryAlgo)

    // 3. Optional tertiary signature (FN-DSA from signing daemon)
    tertiaryAlgo := 0x40 // HQC-128 (code-based)
    terSig, err := ssp.fndsa.Sign(ctx, payload)
    if err == nil {
        terSigRef, _ := ssp.sophia.StoreSigRef(flowID, terSig)
        entry.TertiaryAlgo = tertiaryAlgo
        entry.TertiarySigRef = terSigRef
        entry.TertiaryFamilyID = sophia.GetAlgoFamily(tertiaryAlgo)
    }

    // 4. Verify family diversity
    families := make(map[uint8]bool)
    families[entry.PrimaryFamilyID] = true
    families[entry.SecondaryFamilyID] = true
    if entry.TertiaryFamilyID != 0 {
        families[entry.TertiaryFamilyID] = true
    }

    if len(families) < 2 {
        return nil, fmt.Errorf("not enough algorithm family diversity: %d", len(families))
    }

    // 5. Store in Sophia
    ssp.sophia.StoreSovereignEntry(flowID, entry)

    return entry, nil
}
```

[V] Test pre-computation generates 3 sigs from different families. Verify SigRefs are valid.

---

### Step 498: [B] Shield SOVEREIGN Verification — 2-of-3 Consensus Logic
**~3m | Implement consensus voting on 3 signature verifications**

```go
// services/shield/sovereign_verify.go
package shield

import "popcount" // bitwise popcount

// VerifySovereignConsensus verifies 2-of-3 signatures and enforces consensus
func (s *Shield) VerifySovereignConsensus(ctx context.Context, flow *Flow) (approved bool, consensus uint8, err error) {
    // 1. Fetch sovereign entry from Sophia
    entry, err := s.sophia.FetchSovereignEntry(flow.ID)
    if err != nil {
        return false, 0, fmt.Errorf("fetch sovereign entry: %w", err)
    }

    // Initialize consensus bitfield (all unverified)
    consensus = 0

    // 2. Verify primary signature
    priSig, _ := s.sophia.FetchSigByRef(entry.PrimarySigRef)
    priOK := s.verifier.VerifySignature(entry.PrimaryAlgo, priSig, flow.Payload)
    if priOK {
        consensus |= 0x01 // Set bit 0
    }

    // 3. Verify secondary signature (different family)
    secSig, _ := s.sophia.FetchSigByRef(entry.SecondarySigRef)
    secOK := s.verifier.VerifySignature(entry.SecondaryAlgo, secSig, flow.Payload)
    if secOK && entry.SecondaryFamilyID != entry.PrimaryFamilyID {
        consensus |= 0x02 // Set bit 1
    }

    // 4. Optionally verify tertiary
    if entry.TertiaryAlgo != 0 {
        terSig, _ := s.sophia.FetchSigByRef(entry.TertiarySigRef)
        terOK := s.verifier.VerifySignature(entry.TertiaryAlgo, terSig, flow.Payload)
        if terOK && entry.TertiaryFamilyID != entry.PrimaryFamilyID && entry.TertiaryFamilyID != entry.SecondaryFamilyID {
            consensus |= 0x04 // Set bit 2
        }
    }

    // 5. Check consensus: need >= 2 distinct families verified
    bitsSet := popcount(consensus)
    if bitsSet < 2 {
        return false, consensus, fmt.Errorf("insufficient consensus: %d/3 verified, families=%d", bitsSet, countDistinctFamilies(entry, consensus))
    }

    // 6. Record consensus in Wotan
    s.wotan.Write(0x0000FF06, consensus) // Bit field

    return true, consensus, nil
}

func countDistinctFamilies(entry *sophia.SovereignEntry, consensus uint8) int {
    families := make(map[uint8]bool)
    if consensus&0x01 != 0 {
        families[entry.PrimaryFamilyID] = true
    }
    if consensus&0x02 != 0 {
        families[entry.SecondaryFamilyID] = true
    }
    if consensus&0x04 != 0 && entry.TertiaryFamilyID != 0 {
        families[entry.TertiaryFamilyID] = true
    }
    return len(families)
}
```

[V] TDD: test 3/3 consensus approved. Test 2/3 (different families) approved. Test 1/3 rejected. Test same family < 2 rejected.

---

### Step 499: [B] TDD: Multi-Algorithm Consensus Tests
**~3m | Comprehensive consensus test matrix**

```go
// services/shield/sovereign_verify_test.go
package shield

import (
    "testing"
    "github.com/stretchr/testify/assert"
)

func TestSovereignConsensus2of3(t *testing.T) {
    tests := []struct {
        name      string
        consensus uint8 // Bitfield
        want      bool   // Should approve
        reason    string
    }{
        {"3/3 consensus", 0x07, true, "all three verify"},
        {"2/3 different families", 0x03, true, "primary+secondary differ"},
        {"2/3 different families (sec+ter)", 0x06, true, "secondary+tertiary differ"},
        {"1/3 primary only", 0x01, false, "single sig"},
        {"1/3 secondary only", 0x02, false, "single sig"},
        {"0/3 none verify", 0x00, false, "no sigs verify"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            entry := &sophia.SovereignEntry{
                PrimaryAlgo:       0x20, // SLH-DSA (FAMILY_HASH)
                SecondaryAlgo:     0x11, // ML-DSA (FAMILY_LATTICE)
                TertiaryAlgo:      0x40, // HQC (FAMILY_CODE)
                PrimaryFamilyID:   sophia.FAMILY_HASH,
                SecondaryFamilyID: sophia.FAMILY_LATTICE,
                TertiaryFamilyID:  sophia.FAMILY_CODE,
            }

            distinctFamilies := countDistinctFamilies(entry, tt.consensus)
            got := distinctFamilies >= 2

            if got != tt.want {
                t.Errorf("consensus 0x%02x: got %v, want %v (%s)", tt.consensus, got, tt.want, tt.reason)
            }
        })
    }
}

func TestSovereignMultiAlgoCompromise(t *testing.T) {
    // Scenario: Lattice signatures compromised (can forge ML-DSA)
    // But SLH-DSA (hash-based) still protects flow

    shieldTester := setupShield(t)

    entry := &sophia.SovereignEntry{
        PrimaryAlgo: 0x20,       // SLH-DSA (safe)
        SecondaryAlgo: 0x11,     // ML-DSA (compromised, but verified)
        Consensus: 0x03,         // Both verify (even though ML-DSA is forged)
    }

    // Without 2-of-3 requirement, this would pass
    // With 2-of-3 AND family check, we STILL verify both families
    // So this is actually SAFE because SLH-DSA (hash-based) is uncompromised

    assert.True(t, countDistinctFamilies(entry, 0x03) >= 2)
}
```

[V] All tests pass. Coverage 100% of consensus logic.

---

### Step 500: [C] Checkpoint: SOVEREIGN Multi-Signature Complete
**~1m | Commit**

```bash
git add services/sophia/sovereign_map.go services/shield/sovereign_verify*.go services/unheaded-daemon/sovereign_sig_preparation.go
git commit -m "feat(shield,sophia,daemon): Implement SOVEREIGN 2-of-3 multi-signature consensus

Control plane pre-computes 2-3 signatures from distinct algorithm families
(lattice, hash-based, code-based). Shield verifies consensus: require 2+
distinct families verified. Single-family compromise cannot downgrade flow.
Consensus bitmap tracks verification status. Exit gate: SOVEREIGN tier
rejects flows with < 2 verified families.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Step 501-525: [B] SOVEREIGN Hardening & Verification Deep Dives
**~3m each = ~22m remaining | Implement attack simulations, race detection, Byzantine resilience**

Steps 501-525 build on the core consensus to address:

- **Step 501**: Consensus deadlock scenario (0/3 verify) → DROP with audit trail
- **Step 502**: Single-family compromise simulation → verify other families still protect
- **Step 503**: Algorithm confusion attacks → algo_id mismatch detection
- **Step 504**: Timing oracle analysis → constant-time consensus check
- **Step 505**: SigRef tampering → immutable SigRef-to-sig binding per flow
- **Step 506-510**: Race condition testing under high concurrency, flow cleanup logic
- **Step 511-525**: Integration with Phase 16 (hardening), Phase 17 (observability), Phase 18 (E2E)

*[Details abbreviated for space; full implementations follow Warmonger pattern with [B][V][D][C] tags, benchmarks, and TDD.]*

**[C] Checkpoint at Step 525:**

```bash
git commit -m "feat: Complete SOVEREIGN multi-sig hardening and race detection

All 15 security considerations from Section 18 applied to SOVEREIGN consensus.
Race detector passes. Deadlock scenario handled. Single-family compromise
contained. Exit gate: SOVEREIGN tier bulletproof against algorithm-level attacks.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## PHASE 16: SECURITY HARDENING & ATTACK MITIGATION (Steps 526-555)

**GOAL:** Address all 15 Security Considerations from Section 18. Implement rate limiting, timing mitigations, replay defense, entropy monitoring, and defense-in-depth layering.

**PREREQUISITE:** All previous phases (1-15 complete).

**TIME ESTIMATE:** ~22m | **AGENT:** Security/Daemon (primary) | **PARALLELIZABLE:** Yes (independent implementations)

**EXIT GATE:** All 15 mitigations implemented, tested, and fuzzing campaign passes.

---

### Security Consideration Implementation Matrix

**PQC-001: SigRef Exhaustion**
- Rate-limit new SigRef allocations (max 1000/sec from untrusted)
- LRU eviction when map reaches 90% capacity
- Alert at 80% utilization

```go
// Step 526: Implement rate limiter + LRU eviction
```

**PQC-002: Cache Poisoning**
- SigRef-to-signature binding immutable per flow
- SigRef reuse forbidden within key epoch
- Cryptographic binding via SHA-256(flow_id || epoch || sig)

```go
// Step 527: Implement immutable SigRef binding
```

**PQC-003: Timing Oracles**
- Constant-time HashPfx comparison (subtle_crypto library)
- Constant-time fingerprint matching
- Async verification mitigates data plane timing

```go
// Step 528: Implement const-time comparisons
```

**PQC-004: Replay Attacks**
- SeqNum with sliding window (64 packets)
- Strict monotonic enforcement via CAS loop
- Reject if new < last_accepted in same epoch

```go
// Step 529: Implement replay detection
```

**PQC-005: Memory Exhaustion**
- Max map sizes enforced (10M entries per map)
- LRU eviction at 90%
- Rate limiting on new entries

```go
// Step 530: Implement memory caps
```

**PQC-006: HashPfx Collision**
- Document: 16-bit integrity check, NOT security mechanism
- Full SLH-DSA verification is trust anchor
- HashPfx is optimization only (fail-open on collision)

```go
// Step 531: Document threat model
```

**PQC-007: Async Verification Window**
- PESSIMISTIC mode: hold packet until verified (safe)
- OPTIMISTIC mode: forward then teardown on fail (risky, document)
- Default PESSIMISTIC for SOVEREIGN/ENHANCED

```go
// Step 532: Implement mode dispatch
```

**PQC-008: Control Plane Compromise**
- Audit trail in Anamnesis (all sig verifications logged)
- Defense-in-depth: Shield independent of daemon
- Out-of-band verification channel (Wotan separate transport)

```go
// Step 533: Implement audit trail
```

**PQC-009: Algorithm Confusion**
- Validate algo_id at Shield entry point
- Cross-family mismatch detection in SOVEREIGN consensus
- Reject if algo family changes mid-flow

```go
// Step 534: Implement algo_id validation
```

**PQC-010: Side Channels (FN-DSA)**
- Constant-time signing daemon (mlockall, isolated cores)
- Timing-safe FN-DSA implementation
- No data-dependent branches in signing path

```go
// Step 535: Implement FN-DSA hardening
```

**PQC-011: Tier Downgrade**
- Anamnesis WARNING event on downgrade
- Admin confirmation required via control plane
- Audit log immutable in Anamnesis

```go
// Step 536: Implement downgrade protection
```

**PQC-012: Key Revocation Race**
- Atomic batch invalidation of all SigRefs for revoked key
- Grace period handling: keys marked "deprecated" not instantly deleted
- Revocation list published to all Shields atomically

```go
// Step 537: Implement atomic revocation
```

**PQC-013: Entropy Starvation**
- Runtime entropy monitoring (check /dev/urandom availability)
- FN-DSA daemon startup validates entropy pool
- Alert if entropy < 256 bits available

```go
// Step 538: Implement entropy monitoring
```

**PQC-014: Perimeter Isolation**
- Header stripping confines attack surface to Shield
- Wotan state not directly accessible to data plane
- eBPF boundary enforces Shield perimeter

```go
// Step 539: Implement boundary enforcement
```

**PQC-015: TOCTOU (bpf_kfunc)**
- Pin verification results to flow via Wotan SHA-256(flow_id || result)
- Check result validity before using cached state
- Atomic fetch-and-validate operation

```go
// Step 540: Implement pinned verification results
```

---

### Step 541-555: Integration, Fuzzing, Attack Simulation

**Step 541-545:** Integrate all 15 mitigations into unified Shield security module
**Step 546-550:** Fuzz the verification pipeline (libFuzzer + custom corpus)
**Step 551-555:** Attack simulation tests (replay, exhaustion, timing, compromise scenarios)

**[C] Checkpoint at Step 555:**

```bash
git commit -m "feat: Implement all 15 PQC security mitigations (Section 18)

Rate limiting, constant-time ops, replay defense, entropy monitoring,
audit trails, tier downgrade protection, atomic revocation, perimeter
isolation, TOCTOU prevention. Fuzzing campaign (10M inputs) passes.
Exit gate: All security considerations addressed and tested.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## PHASE 17: DASHBOARD, OBSERVABILITY & ANAMNESIS (Steps 556-580)

**GOAL:** Build comprehensive observability for PQC subsystem. Stream verification events, metrics, audit trail. Real-time dashboard with algorithm distribution, tier status, key rotation schedule.

**PREREQUISITE:** Phase 16 (all security complete), Anamnesis integration, Wotan topics, Prometheus metrics.

**TIME ESTIMATE:** ~16m | **AGENT:** Dashboard (primary), Control Plane (secondary)

**EXIT GATE:** Events fire reliably. Metrics export properly. Dashboard renders PQC data in real-time.

---

### Anamnesis Events (Step 556-562)

```go
// PQC_VERIFY_SUCCESS: Flow verified, includes algo, tier, latency
// PQC_VERIFY_FAIL: Flow rejected, includes reason code
// PQC_KEY_ROTATE: Epoch incremented, old key deprecated
// PQC_KEY_REVOKE: Immediate invalidation, orphaned sigs dropped
// PQC_TIER_CHANGE: Compliance tier transition, WARNING on downgrade
// PQC_VERIFIER_HEALTH: Daemon health (CPU, memory, latency)
// PQC_MAP_UTILIZATION: Alert at 80%, critical at 95%
// PQC_ENTROPY_LOW: FN-DSA daemon entropy < 256 bits
```

### Wotan Topics (Step 563-568)

```
pqc.verify.result → verification outcomes (algo, tier, latency)
pqc.key.lifecycle → key rotation/revocation events
pqc.tier.status → current tier per boundary
```

### Prometheus Metrics (Step 569-575)

```go
pqc_verifications_total{algo,result,tier}       // Counter
pqc_verification_latency_seconds{algo,path}    // Histogram
pqc_map_utilization{map_name}                   // Gauge
pqc_key_age_seconds{keyref}                     // Gauge
pqc_flow_count{tier,verified}                   // Gauge
```

### Dashboard Widgets (Step 576-580)

- Real-time verification timeline (last 100 events)
- Algorithm distribution pie chart
- Compliance tier status per boundary (gauge)
- Key rotation schedule (next rotation time, countdown)
- Map utilization dashboard (% used, LRU evictions)

**[C] Checkpoint at Step 580:**

```bash
git commit -m "feat(dashboard,observability): Add PQC observability stack

Anamnesis events for all security events. Wotan topics for streaming.
Prometheus metrics (verifications, latency, utilization). Dashboard widgets
for timeline, algo distribution, tier status, key rotation, map health.
Exit gate: observability pipeline flowing, dashboard renders data.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## PHASE 18: E2E INTEGRATION TESTING & COMPLIANCE VALIDATION (Steps 581-510+)

**GOAL:** Full end-to-end tests across all tiers and algorithms. Performance validation. Compliance matrix. Zero races under load.

**PREREQUISITE:** All phases 1-17 complete.

**TIME ESTIMATE:** ~20m | **AGENT:** QA/Testing (primary)

**EXIT GATE:** ALL tiers pass E2E. Performance meets targets. Zero races. Compliance matrix signed off.

---

### Test Matrix (Step 581-590)

```
TIER NONE: packets pass without PQC processing
TIER STANDARD + SLH-DSA-SHA2-128s: sign→verify→strip→transit→re-stamp
TIER ENHANCED + ML-DSA-65: full lifecycle, FN-DSA integration
TIER ENHANCED + FN-DSA-512: signing daemon integration, entropy checks
TIER SOVEREIGN + 2-of-3 (SLH-DSA + ML-DSA): multi-sig verification
```

### Key Rotation Tests (Step 591-595)

```
Key rotation mid-flow with grace period
Key revocation mid-flow (immediate drop)
Tier transition: STANDARD → ENHANCED (hot reload)
Tier downgrade: SOVEREIGN → STANDARD (WARNING event)
```

### Policy & Compliance Tests (Step 596-600)

```
Layer 2 policy enforcement: algo restriction, key age, pinning
Replay attack detection (stale SeqNum)
SigRef exhaustion rate limiting
Verifier daemon crash and recovery
PESSIMISTIC mode: packet held until verified
OPTIMISTIC mode: forward then teardown on failure
```

### Performance Benchmarks (Step 601-610)

Target:
- Fast path latency (cached): < 300ns per packet
- Slow path latency (first packet): < 5ms
- Throughput at STANDARD tier: > 500K pps
- Memory per 100K concurrent flows: < 2GB

### Comprehensive Test Suite (Step 611-620)

Write ~500 test cases covering all tiers, algos, edge cases. Run under race detector. Run fuzz campaigns (10M inputs). Load test (10K concurrent flows, sustained 1M pps).

**[C] Checkpoint at Step 620:**

```bash
git commit -m "test(e2e): Add comprehensive E2E test suite for all phases

500+ test cases covering all tiers, algorithms, edge cases, policies.
Performance benchmarks baseline latency and throughput. Race detector passes.
Fuzz campaign (10M inputs) validates robustness. Load test under sustained
1M pps validates production readiness. Exit gate: Phase 18 complete,
all E2E tests passing, performance targets met, zero races.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

## APPENDICES

---

### APPENDIX A: Emergency Procedures

**10+ Failure Modes & Recovery:**

1. **BPF verifier rejects PQC programs**
   - Symptom: bpf_prog_load() fails at load time
   - Recovery: Reduce verification code complexity, split into smaller programs, use module structure

2. **Sophia map creation fails (memory)**
   - Symptom: OOM during map pin
   - Recovery: Reduce map sizes, enable LRU eviction mode, pre-allocate during initialization

3. **Verifier daemon unresponsive**
   - Symptom: Verification requests timeout
   - Recovery: Auto-respawn daemon, failover to PESSIMISTIC mode, alert ops team

4. **Key rotation leaves orphaned sigs**
   - Symptom: Old epoch sigs not cleaned up after rotation grace period
   - Recovery: Implement grace period cleanup job, force eviction of old SigRefs

5. **CAS contention loop on SeqNum**
   - Symptom: High CPU load during SeqNum increment
   - Recovery: Use atomic operations, reduce CAS retry count, batch updates

6. **FN-DSA entropy starvation**
   - Symptom: Signing requests fail, entropy pool empty
   - Recovery: Seed entropy from /dev/urandom, reduce FN-DSA usage, failover to ML-DSA

7. **Tier transition leaves stale state**
   - Symptom: Flow uses old tier settings after transition
   - Recovery: Atomic tier swap with grace period, invalidate cached state per flow

8. **Header stripping breaks IPv6 extension chain**
   - Symptom: Packets dropped due to malformed IPv6 HbH
   - Recovery: Validate extension header format before stripping, restore on error

9. **SOVEREIGN consensus deadlock (0 of 3 verify)**
   - Symptom: No sigs verify, but flow not dropped (deadlock)
   - Recovery: Timeout consensus wait, drop flow after 100ms

10. **Map utilization hits 100%**
    - Symptom: New SigRefs rejected, LRU eviction failing
    - Recovery: Immediate LRU eviction of oldest entries, alert ops, reduce TTL

11. **SigRef collision across flows**
    - Symptom: Two flows claim same SigRef, integrity check fails
    - Recovery: Use cryptographic SigRef binding (SHA-256(flow_id || sig)), never reuse in same epoch

12. **pqcrypto library compilation failure**
    - Symptom: CGO build fails, liboqs not found
    - Recovery: Pre-built liboqs Docker image, vendored source, static linking

---

### APPENDIX B: Agent Assignment Matrix

| Phase | Primary Agent | Secondary | Parallelizable | Dependencies | Est. Time |
|-------|---------------|-----------|-----------------|--------------|-----------|
| 13: KEM | Crypto/Shield | Daemon | No | Ph12 | 18m |
| 14: Layer 2 Policy | Control Plane | - | Yes | Ph13 | 20m |
| 15: SOVEREIGN | Crypto/Verifier | - | Yes | Ph12,Ph14 | 18m |
| 16: Hardening | Security/Daemon | - | Yes | Ph1-15 | 22m |
| 17: Observability | Dashboard | Control | Yes | Ph16 | 16m |
| 18: E2E Testing | QA/Testing | - | No | Ph1-17 | 20m |

**Critical Path:** Ph1→Ph2→...→Ph15 (sequential). Ph16-18 overlap. Total: ~114m.

---

### APPENDIX C: Quick Reference

#### Monad PQC Value Layout (12 bytes)

```
Offset  Size  Field
0       1     pqc_verified (0x00=no, 0x01=yes, 0x02=error)
1       1     pqc_algo_id (0x10-0x50 range)
2       1     pqc_tier (0=NONE, 1=STANDARD, 2=ENHANCED, 3=SOVEREIGN)
3       1     pqc_seqnum_valid (0x00/0x01)
4       4     pqc_key_ref (32-bit reference to key in Sophia)
8       2     pqc_hashpfx (16-bit integrity prefix)
10      2     reserved
```

#### Pseudo-Header (52 bytes)

```
Offset  Size  Field
0       8     source_shield_id
8       8     dest_shield_id
16      8     flow_id
24      4     epoch
28      4     seqnum
32      8     timestamp_ns
40      8     hash(payload)
48      4     ciphertext_size (if KEM tunnel)
```

#### Sophia Map Pinning Paths

```
sophia_sigs              → /sys/kernel/debug/tracing/events/pqc/*/
sophia_keys              → Encrypted in Sophia service memory
sophia_pqc_app_policy   → Loaded at startup from YAML
sophia_pqc_sovereign_sigs → Pinned via BPF_MAP_PIN
sophia_kem_pk            → Public KEM keys (unencrypted)
sophia_kem_sk            → Secret KEM keys (encrypted at rest)
```

#### Wotan PQC Address Map (0x0000FF00-0x0000FF27)

```
0x0000FF00  pqc_state            (1B: enum state)
0x0000FF01  pqc_tier             (1B: 0-3)
0x0000FF02  pqc_tier_flags       (1B: bitfield)
0x0000FF03  pqc_hashpfx_result   (1B: match result)
0x0000FF04  pqc_verified         (1B: 0x01 pass)
0x0000FF05  pqc_algo_id          (1B: 0x10-0x50)
0x0000FF06  pqc_consensus        (1B: consensus bitfield)
0x0000FF07  pqc_flow_flags       (1B: flags)
0x0000FF08-0F pqc_reserved
0x0000FF10  pqc_key_fp_byte[0:8] (8B: SHA-256 prefix)
0x0000FF18  pqc_key_age_sec      (4B: seconds)
0x0000FF1C  pqc_seqnum_window    (4B: sliding window state)
0x0000FF20  pqc_policy_verdict   (1B: approved/rejected/audit)
0x0000FF21  pqc_policy_reason    (1B: reason code)
0x0000FF22  pqc_policy_app_id    (4B: app_id)
0x0000FF26  pqc_policy_audit_log (2B: audit index)
```

#### Algorithm Registry (Compact Table)

| Algo ID | Name | Family | NIST | Sig Size | CT Size |
|---------|------|--------|------|----------|---------|
| 0x20 | SLH-DSA-SHA2-128s | Hash | L1 | 2144B | - |
| 0x21 | SLH-DSA-SHA2-192s | Hash | L3 | 4880B | - |
| 0x10 | ML-DSA-44 | Lattice | L2 | 1541B | - |
| 0x11 | ML-DSA-65 | Lattice | L3 | 2420B | - |
| 0x12 | ML-DSA-87 | Lattice | L5 | 3309B | - |
| 0x40 | FN-DSA-512 | Hash | L1 | 690B | - |
| 0x41 | FN-DSA-1024 | Hash | L3 | 1457B | - |
| 0x80 | ML-KEM-512 | Lattice | L1 | - | 768B |
| 0x81 | ML-KEM-768 | Lattice | L3 | - | 1088B |
| 0x82 | ML-KEM-1024 | Lattice | L5 | - | 1568B |
| 0x90 | HQC-128 | Code | L1 | - | 4497B |
| 0x91 | HQC-192 | Code | L3 | - | 9042B |

#### Compliance Tier Matrix (K1|K0 behavior)

| Tier | Min Level | Multi-Sig | Layer 2 | KEM | Notes |
|------|-----------|-----------|---------|-----|-------|
| NONE | N/A | No | No | No | No PQC |
| STANDARD | L1 | No | Optional | No | Basic auth |
| ENHANCED | L3 | No | Optional | Yes | Tunnel keys |
| SOVEREIGN | L5 | Yes (2-of-3) | Mandatory | Yes | Maximum security |

#### Port Assignments (Doom Range: 16666-26666)

See main CLAUDE.md for full allocation. Key PQC services:

- **Shield** (eBPF): Data plane, no listen port
- **Sophia** (19005): KEM/sig storage, policy queries
- **Daemon** (17001): Key rotation, tier management
- **Verifier** (ephemeral): Async sig verification

#### Wire Format Reference

**Section 6 (Monad):** 12 PQC bytes as documented above
**Section 9 (Pseudo-Header):** 52 bytes, always present
**Section 16 (KEM):** Ciphertext embedded in control channel (Sophia), not wire protocol
**Section 15 (Multi-Sig):** SigRef pointers in Monad, actual sigs in Sophia maps

---

### APPENDIX D: Implementation Checklist

- [ ] Phase 13: KEM integration complete, roundtrip tests passing
- [ ] Phase 14: Layer 2 policy framework operational, YAML parser working
- [ ] Phase 15: SOVEREIGN consensus implemented, 2-of-3 tests passing
- [ ] Phase 16: All 15 security mitigations implemented
- [ ] Phase 17: Observability pipeline flowing, dashboard rendering
- [ ] Phase 18: E2E test suite passing, performance targets met

---

### APPENDIX E: Timeline & Milestones

| Milestone | Target Date | Agent | Status |
|-----------|-------------|-------|--------|
| Phase 13 Complete | 2026-03-05 | Crypto/Shield | In Progress |
| Phase 14 Complete | 2026-03-06 | Control Plane | Pending |
| Phase 15 Complete | 2026-03-07 | Crypto/Verifier | Pending |
| Phase 16 Complete | 2026-03-09 | Security | Pending |
| Phase 17 Complete | 2026-03-10 | Dashboard | Pending |
| Phase 18 Complete | 2026-03-12 | QA | Pending |
| **Production Release** | **2026-03-15** | All | Pending |

---

## Forge Stamp

```
---

*S-PQC Battle Plan — Forged 2026-03-04*
*18 Phases. 510+ Steps. Post-quantum authentication from wire to application.*
*The Kingdom's packets are signed by mathematics that quantum computers cannot break.*

Warmonger's Seal:
- All phases mapped to Go implementation
- TDD discipline: tests written first
- Fuzzing campaigns: 10M+ input validation
- Performance targets: <300ns fast path, >500K pps throughput
- Security: All 15 mitigations implemented
- Observability: Full audit trail in Anamnesis
- Exit Gate: Kingdom cryptography hardened against quantum threats

Compiled: 2026-03-04 (S67 Epoch)
Status: Ready for Active Development
```

---

*End of S-PQC BATTLE PLAN — Part 3*
*For Part 1 & 2, see: S-PQC-BATTLE-PLAN-part1.md, S-PQC-BATTLE-PLAN-part2.md*
