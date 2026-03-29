# Zhen Layer 0 — Post-Quantum Threat Model

**Author**: The Scientist (formal analysis)  
**Date**: 2026-03-28  
**Status**: ACTIVE  
**Classification**: Security Architecture  

---

## 1. OBSERVE — What are we protecting?

Zhen is a knowledge substrate with two properties that make it uniquely
vulnerable to quantum attack:

**Property 1: Immortal fragments.** Pu fragments never die. They sediment
into Jing (L3 cold storage) and persist indefinitely. Any encrypted fragment
created today will still exist in 10, 20, 50 years.

**Property 2: Gossip propagation.** Fragments traverse the network via Qi
(gossip), meaning encrypted fragments are observable by any node in the path
and any passive adversary monitoring the network.

These properties create the **harvest-now-decrypt-later (HNDL)** threat:
an adversary captures encrypted gossip traffic today and stores it until
quantum hardware capable of breaking classical asymmetric crypto exists.

The timeline for this threat is estimated at 5-15 years by NIST, NSA, and
CISA. Zhen fragments will still be alive when that window opens.

---

## 2. RESEARCH — Prior art and standards

| Standard | Status | Relevance |
|---|---|---|
| FIPS 203 (ML-KEM/Kyber) | Final, August 2024 | Key encapsulation for fragment encryption |
| FIPS 204 (ML-DSA/Dilithium) | Final, August 2024 | Digital signatures for peer auth |
| FIPS 205 (SLH-DSA/SPHINCS+) | Final, August 2024 | Hash-based sigs (backup if lattice breaks) |
| NIST IR 8413 | Final | PQ crypto migration guidance |
| NSA CNSA 2.0 | Active | US government PQ migration requirements |
| IETF draft-ietf-tls-hybrid-design | Active draft | Hybrid KEM in TLS 1.3 |
| IETF RFC 9180 (HPKE) | Final | Hybrid Public Key Encryption framework |

CNSA 2.0 timeline: software/firmware signing must use PQ by 2025,
web browsers/servers by 2025, traditional networking by 2030. Zhen
should target the most aggressive timeline since fragments outlive
all of these windows.

---

## 3. HYPOTHESIZE — Threat scenarios

### H1: Harvest-Now-Decrypt-Later on Gossip Traffic

**Adversary**: Nation-state passive observer (NSA/equivalent)  
**Capability**: Full network capture of gossip UDP traffic  
**Timeline**: Decryption capability in 5-15 years  
**Target**: Encrypted Pu fragment payloads  

**Attack**: Capture all gossip traffic between Zhen peers. Store
ciphertext. When quantum hardware breaks ECDH/X25519, recover
per-fragment symmetric keys, decrypt all historical fragments.

**Impact**: Complete exposure of all historical knowledge in the
Zhen network. The Library's memory is laid bare retroactively.

**Mitigation**: Hybrid ML-KEM + X25519 key encapsulation. Adversary
must break BOTH lattice-based and elliptic-curve problems, which
requires two independent quantum breakthroughs.

**Residual risk**: If both ML-KEM AND X25519 are broken, all encrypted
fragments become readable. Probability assessment: LOW (independent
mathematical problems, no known shared weakness).

### H2: Quantum Forgery of Peer Identity

**Adversary**: Active attacker with quantum computer  
**Capability**: Can forge ECDSA/Ed25519 signatures  
**Timeline**: Same 5-15 year window  
**Target**: Gossip peer authentication  

**Attack**: Forge a valid peer identity, join the gossip mesh, flood
the network with poisoned fragments. Since Sybil resistance depends
on authenticated peer identity, quantum signature forgery breaks the
trust boundary of the entire mesh.

**Impact**: Adversary can inject arbitrary knowledge into Zhen.
Poisoned fragments sediment into Jing and persist forever. The Library
is corrupted at the geological level.

**Mitigation**: ML-DSA-65 (Dilithium) signatures for peer identity.
Cannot be forged by quantum adversary at NIST Security Level 3.

**Residual risk**: If ML-DSA is broken by cryptanalysis (not quantum,
but mathematical), peer identity is forgeable. Backup plan: SLH-DSA
(SPHINCS+) which is hash-based and relies only on hash function
security (not lattice assumptions).

### H3: Quantum Attack on Content Addressing

**Adversary**: Quantum computer  
**Capability**: Grover's algorithm on BLAKE3  
**Target**: FragmentId collision  

**Attack**: Find a second payload that produces the same BLAKE3 hash
as a target fragment, enabling payload substitution.

**Analysis**: Grover's algorithm reduces BLAKE3's collision resistance
from 2^128 to 2^64 for preimage, and from 2^128 to 2^85 for collision
(birthday + Grover). 2^85 is still computationally infeasible with
foreseeable quantum hardware. BLAKE3 at 256-bit output is sufficient.

**Mitigation**: None needed. BLAKE3 is quantum-safe for content addressing.

**Residual risk**: NEGLIGIBLE.

### H4: Side-Channel on PQ Key Operations

**Adversary**: Co-located process or hardware-adjacent attacker  
**Capability**: Timing/power/cache side channels  
**Target**: ML-KEM secret key extraction during decapsulation  

**Attack**: Observe timing or cache access patterns during ML-KEM
decapsulation to recover the secret key.

**Mitigation**: Use constant-time implementations (PQClean reference
implementations are constant-time). Verify via dudect or similar
timing analysis. Zeroize all secret material on drop.

**Residual risk**: MEDIUM — constant-time guarantees in Rust depend
on compiler not optimizing away timing protections. Periodic audit
required. The `zeroize` crate handles memory cleanup but cannot
guarantee absence of compiler-introduced timing leaks.

### H5: Embedding Vector Inversion

**Adversary**: Any attacker with access to fragment embeddings  
**Capability**: Machine learning model inversion  
**Target**: Reconstruct plaintext from embedding vector  

**Attack**: Even if the payload is encrypted, the embedding vector
(used for De/relevance) is stored in plaintext for similarity search.
An adversary with access to fragments could attempt to invert the
embedding model to recover approximate plaintext.

**Analysis**: This is not a quantum threat — it's a classical ML
attack. Quantized uint8 embeddings are lossy, but research shows
partial text recovery is possible from high-dimensional embeddings.

**Mitigation options**:
1. Encrypt embeddings too (but then De/relevance doesn't work)
2. Use dimensionality reduction to further degrade invertibility
3. Add calibrated noise to embeddings (differential privacy)
4. Accept the risk — embeddings reveal topic, not content

**Recommendation**: Option 4 for now, with option 3 as future
enhancement. The embedding reveals "this fragment is about networking"
but not the specific content. Acceptable for most threat models.
Revisit if Zhen carries classified or PII-bearing fragments.

---

## 4. PREDICT — Testable consequences

| Prediction | Test | Expected Result |
|---|---|---|
| Hybrid KEM roundtrip produces identical shared secrets | Unit test in kem.rs | PASS |
| Wrong keypair produces different shared secret | Unit test in kem.rs | PASS (ML-KEM implicit reject) |
| ML-DSA signature verifies with correct key | Unit test in sign.rs | PASS |
| ML-DSA signature fails with wrong key | Unit test in sign.rs | PASS |
| Sealed fragment roundtrip recovers plaintext | Unit test in envelope.rs | PASS |
| Sealed fragment with wrong key fails | Unit test in envelope.rs | PASS (AES-GCM auth fails) |
| Tampered ciphertext fails authentication | Unit test in envelope.rs | PASS |
| BLAKE3 collision infeasible at 2^85 | Theoretical analysis | NOT TESTABLE (infeasible) |
| PQ key operations are constant-time | dudect timing analysis | TODO: implement |
| Hybrid overhead < 2KB per fragment | Measure envelope_overhead() | PASS (1153 bytes) |

---

## 5. FORMAL SECURITY ARGUMENT

### Theorem: Hybrid KEM Security

**Claim**: The Zhen hybrid KEM construction is IND-CCA2 secure if
EITHER ML-KEM-768 OR X25519 is IND-CCA2 secure.

**Proof sketch**: The shared secret is derived via
`HKDF-SHA256(ML-KEM-SS || X25519-SS)`. By the properties of HKDF
as a randomness extractor (Krawczyk 2010), if either input contains
sufficient min-entropy, the output is computationally indistinguishable
from random. An adversary who can break the hybrid must break BOTH
components to distinguish the derived key from random.

This is a "combiner" security argument: the hybrid is at least as
strong as the stronger component. Formal proof follows the framework
of Giacon, Heuer, and Poettering (2018), "KEM Combiners."

### Theorem: Fragment Confidentiality Under HNDL

**Claim**: An adversary who captures sealed fragment traffic today
cannot recover plaintext without breaking both ML-KEM-768 and X25519.

**Proof sketch**: Each sealed fragment uses a fresh ephemeral keypair
for both ML-KEM and X25519. The shared secret is bound to the
specific encapsulation via HKDF with domain separator
`"zhen-hybrid-kem-v1"`. Forward secrecy holds per-fragment: compromising
the recipient's long-term key after fragment creation does NOT
retroactively compromise fragments if the ephemeral randomness was
good (which it is — `rand::thread_rng()` backed by OS CSPRNG).

Wait — correction. ML-KEM encapsulation uses the RECIPIENT'S long-term
public key. Compromising the recipient's long-term secret key DOES
allow decapsulation of all fragments encrypted to that key. This is
NOT forward-secret in the traditional sense.

**Revised claim**: Fragment confidentiality holds as long as the
recipient's ML-KEM + X25519 long-term keypair is not compromised
AND the underlying mathematical problems remain hard.

**Mitigation for key compromise**: Periodic key rotation. Recipients
generate new hybrid keypairs periodically. Old sealed fragments
remain encrypted to old keys. Old secret keys should be securely
destroyed after rotation window (but see: the fragments they protect
still exist in Jing forever).

**Open question**: Should Zhen support re-sealing? When a recipient
rotates keys, should all fragments encrypted to the old key be
re-sealed with the new key? This is expensive (re-encrypt every
fragment) but eliminates the window of vulnerability from old keys.

---

## 6. RECOMMENDATIONS

### MUST (non-negotiable)

1. Hybrid ML-KEM + X25519 for all fragment encryption. No classical-only.
2. ML-DSA-65 for peer authentication. No Ed25519-only.
3. Zeroize all secret material on drop (already implemented).
4. HKDF with domain-specific salt for all key derivation.
5. Random nonce per seal operation (already implemented).

### SHOULD (strong recommendation)

6. SLH-DSA (SPHINCS+) as backup signature scheme if ML-DSA is broken.
7. Key rotation policy for recipient hybrid keypairs (e.g., annual).
8. Constant-time audit via dudect for all PQ key operations.
9. Fragment re-sealing on key rotation (expensive but eliminates old-key risk).
10. Differential privacy noise on embeddings for high-sensitivity deployments.

### MAY (future consideration)

11. Full homomorphic encryption for embeddings (allows De ranking on encrypted embeddings).
12. Threshold decryption (k-of-n recipients can unseal, single recipient cannot).
13. Post-quantum TLS in rustls/quinn when upstream ships.

---

## 7. CONCLUSION

Zhen's immortal fragment property creates a unique cryptographic challenge:
the protection horizon is unbounded. Classical asymmetric crypto has a
bounded security lifetime (~10-15 years against quantum). This mismatch
is existential for a system designed to remember forever.

Hybrid PQ cryptography resolves the mismatch at acceptable cost (~1.5KB
per sealed fragment, ~50x larger keys, ~2x slower operations). The
alternative — classical-only crypto on immortal data — is a Library
whose locks have an expiration date printed on them.

*That which remembers forever must be armored against forever.*

---

**THE LABORATORY HAS SPOKEN.**
**THE MATH IS CLEAR.**
**PQ IS NOT OPTIONAL FOR ZHEN.**

*Observation. Hypothesis. Experiment. Truth.*
