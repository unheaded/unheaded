# The Phylactery

*φυλακτήριον (phylaktḗrion) — "safeguard, amulet, that which protects"*

**Phylactery the Encrypted Storage Layer**

---

## Naming

In antiquity, a phylactery was a small container holding sacred
texts — bound to the body, carried everywhere, opened only by the
faithful.  In fantasy canon, a lich's phylactery holds its soul:
the single most protected object in existence, guarded by layers
of magic, bound to the creature's life force.

In the Kingdom: the Phylactery is encrypted storage that wears
its own suit of armor.  It is not a service.  It is a Knight.
It carries the data-soul of the Kingdom and demands two keys
from anyone who would read or write.

---

## The Two Seals

The Phylactery requires two independent cryptographic proofs to
unlock any operation.  Neither is sufficient alone.  Both must
align in the same packet, at the same hop, at the same moment.

### Seal One: The Sigil (Packet-Borne Key)

The Sigil lives in the protocol.  It travels with the packet.
Every hop can verify it.  It is the pilgrim's proof of passage.

    Location:   Monad R0 (service_id, exponent-encoded via Sophia)
                + Extended Register Space (Kingdom Mode, PQC fingerprint)
    Derivation: HMAC-SHA3-256(master_key, flow_label || trace_id || hop_count)
    Size:       32 bits in Monad (truncated), 32 bits in ERS (full fingerprint)
    Rotation:   Per-epoch via Sophia dictionary update
    Verify:     O(1) BPF map lookup at every hop

The Sigil is NOT the encryption key.  It is the BINDING — proof
that this packet is authorized to touch this data.  The Shim at
each hop checks the Sigil against the Sophia key store.  Mismatch
= Anamnesis ANOMALY event + configurable DROP.

### Seal Two: The Ward (Traditional Cryptographic Proof)

The Ward is classical cryptography.  It lives at the application
layer.  It proves identity independent of the protocol.

    Algorithms (operator choice):
      - ECDSA over secp256k1  (Bitcoin/Ethereum lineage)
      - ECDSA over P-256      (NIST standard)
      - Ed25519               (modern, fast)
      - SHA-256 HMAC          (symmetric, pre-shared key)
      - ML-DSA-65             (post-quantum, FIPS 204)

    Location:   Payload envelope (NOT in Monad, NOT in headers)
    Structure:  { operation, path, timestamp, nonce, signature }
    Verify:     At storage service boundary (Shield ingress to Phylactery)

The Ward proves WHO you are.  The Sigil proves your packet is
AUTHORIZED to be here.  Together: identity + authorization in
a single packet.

### The Two-Seal Invariant

    UNLOCK = Sigil_valid(packet) AND Ward_valid(payload)

    Sigil alone:  packet is authorized but identity unknown → REJECT
    Ward alone:   identity proven but packet unauthorized  → REJECT
    Both valid:   proceed with operation
    Neither:      packet never reaches Phylactery (Shield drops it)

### Optional Third Factor: Binding Circles (see ADR-009)

The Two-Seal model can be extended with a third verification
dimension: RFC 1918 address range coloring.  Dedicated /16 ranges
are mapped to semantic trust zones ("Binding Circles") via Sophia.
The BPF program performs an O(1) LPM trie lookup — if a STORE
operation arrives from an address outside the SANCTUM circle, it
is an immediate anomaly before Sigil or Ward are even checked.

    THREE-SEAL (optional):
    UNLOCK = Circle_valid(src_ip, dst_ip, op) AND Sigil_valid AND Ward_valid

Binding Circles are a Beta wishlist item.  The Phylactery works
without them.  See ADR-009 for full design.

This is not defense in depth (layered same-type controls).  This
is orthogonal verification — two independent proof systems that
cover each other's blind spots:

    Sigil covers:  replay attacks (hop_count bound), path
                   manipulation (trace_id bound), unauthorized
                   forwarding (Sophia epoch bound)

    Ward covers:   identity theft (private key required),
                   payload tampering (signature over operation),
                   time attacks (timestamp + nonce)

---

## The Phylactery Wears Its Own Armor

The Phylactery is NOT a service running inside someone else's
Kingdom.  It IS a Kingdom.  A full Unheaded stack:

    PHYLACTERY INSTANCE
    ├── Shield           ingress/egress, Sigil verification
    ├── Sophia           key store (Sigils, Ward pubkeys, epochs)
    ├── Monad            register processing (Sigil check per hop)
    ├── Anamnesis        full audit trail (every read, write, deny)
    ├── Wotan            internal message bus
    ├── Yaldabaoth       chaos testing of storage integrity
    ├── Gateway          gRPC + HTTP/3 storage API
    └── The Vault        actual encrypted block storage

It runs standalone.  It boots its own BPF programs.  It manages
its own Sophia dictionaries.  It has its own Anamnesis ring buffer.
It IS an Unheaded Knight in full armor whose sole purpose is
guarding data.

But it is NOT isolated from the Kingdom.  It participates in the
mesh.  Other services reach it through the normal fabric.  Wotan
messages flow.  Anamnesis events correlate across boundaries.
The Phylactery is a peer, not a prisoner.

---

## Where It Operates

### Mode 1: Per-Hop Verification (Inside a Kingdom)

When speed is NOT the constraint and maximum security is required,
every hop between a client and the Phylactery verifies the Sigil.
The packet carries the authorization proof through the entire path.
Any compromised hop that tampers with the Monad invalidates the
CRC-16 and the Sigil binding.  The Phylactery's Shield checks both
Seals at ingress.

    Client → [Hop: Sigil ✓] → [Hop: Sigil ✓] → [Hop: Sigil ✓] → Phylactery Shield
                                                                     ├── Sigil ✓
                                                                     ├── Ward ✓
                                                                     └── Operation proceeds

This means a compromised intermediate node CANNOT forge a valid
storage request.  It can see the packet (unless Kingdom Mode ERS
carries encrypted fields), but it cannot produce a valid Sigil
for a different operation because the Sigil is bound to:

    flow_label + trace_id + hop_count + epoch

Change any of these and the Sigil verification fails.

### Mode 2: Ingress/Egress Only (Speed Priority)

When speed IS the constraint, per-hop verification is skipped.
The Two-Seal check happens only at Kingdom boundaries — Shield
ingress and Shield egress.  Interior hops process the Monad
normally (CRC check, Sophia lookup, Shim execution) but do NOT
verify the Sigil cryptographic binding.  This reduces per-hop
overhead to standard Monad processing (~270ns).

    Client → [Hop: CRC ✓] → [Hop: CRC ✓] → Phylactery Shield
                                                ├── Sigil ✓ (full check)
                                                ├── Ward ✓  (full check)
                                                └── Operation proceeds

This is the RECOMMENDED mode for most deployments.  The Kingdom
fabric is already a Limited Domain with controlled hops.  The
boundary checks at Shield are the trust perimeter.

### Mode 3: Kingdom-to-Kingdom (Cross-Domain)

When a Phylactery serves multiple Kingdoms, the Sigil is stamped
at the originating Kingdom's Shield (egress) and verified at the
Phylactery Kingdom's Shield (ingress).  The Ward travels in the
payload across the domain boundary.

    Kingdom A                    Kingdom B (Phylactery)
    ┌─────────┐                  ┌─────────────────┐
    │ Client  │                  │  Phylactery     │
    │    ↓    │                  │  Shield         │
    │ Shield  │──── VXLAN ────→  │    ├── Sigil ✓  │
    │ (stamp  │    tunnel        │    ├── Ward ✓   │
    │  Sigil) │                  │    └── Proceed  │
    └─────────┘                  └─────────────────┘

The egress Shield in Kingdom A stamps the Sigil using the shared
Sophia epoch (distributed via Wotan cross-domain sync).  The
ingress Shield in Kingdom B (the Phylactery) verifies it.  The
Ward signature in the payload provides the second factor.

CRITICAL: Cross-Kingdom traffic NEVER touches the public internet.
Kingdom-to-Kingdom links are always VXLAN tunnels over VPN (WireGuard
or IPsec) or direct private interconnects.  The Phylactery is
reachable only from within the Kingdom mesh.

### Mode 4: Standalone (No Parent Kingdom)

The Phylactery can run with zero external dependencies.  It IS the
Kingdom.  Client connects directly to the Phylactery's Gateway.
The Sigil is stamped at the Phylactery's own Shield on ingress
(derived from the client's pre-shared credentials in Sophia).

This mode is for:
  - Air-gapped storage
  - Single-purpose vault deployments
  - Key escrow / cold storage
  - Disaster recovery seed vaults

---

## Storage Architecture

### The Vault

The actual storage engine inside the Phylactery.  Data at rest is
always encrypted.  The Vault never stores plaintext.

    Encryption at rest:    AES-256-GCM (symmetric, fast)
    Key derivation:        HKDF-SHA3-256(master_key, path || epoch)
    Key rotation:          Per-epoch, automatic via Sophia
    Block size:            4 KB (aligned with OS page, BPF map page)
    Addressing:            Content-addressed (SHA-256 of plaintext)
    Deduplication:         Convergent encryption (same content = same ciphertext)
    Integrity:             Merkle tree over blocks (root in Sophia)

### Operations

All operations are expressed as Monad register instructions:

    R0 = service_id       Phylactery service (Sophia-encoded)
    R1 = flow_action      Storage operation:
                            0x20 = STORE     (write block)
                            0x21 = RETRIEVE  (read block)
                            0x22 = DELETE    (mark tombstone)
                            0x23 = VERIFY    (check integrity)
                            0x24 = REPLICATE (cross-Phylactery sync)
                            0x25 = COMPACT   (garbage collect)
    R2 = trace_id_hi      correlation
    R3 = trace_id_lo      correlation
    R4 = scratch + CRC    block address (content hash prefix)

The actual data payload travels in the IPv6 packet body.  The
Monad carries the operation metadata and Sigil.  The Ward is
in the payload envelope wrapping the data.

### Anamnesis Integration

Every Phylactery operation emits to Anamnesis:

    EVENT_STORE      block written, content hash, size, epoch
    EVENT_RETRIEVE   block read, content hash, requester
    EVENT_DELETE      block tombstoned, content hash, epoch
    EVENT_DENY        Two-Seal check failed (which seal, why)
    EVENT_REPLICATE   block synced to peer Phylactery
    EVENT_INTEGRITY   periodic Merkle tree verification result

Full audit trail.  Immutable.  Queryable.  Every read, every
write, every denial.  The Phylactery remembers everything.

---

## The Arcane Hollow: The Sanctum

The Phylactery lives in a new Arcane Hollow:

    🔐 The Sanctum — Phylactery's Arcane Hollow

    "Beneath the Kingdom, deeper than the Crystal Grotto where
     secrets shimmer, deeper than the Primordial Pit where hardware
     stirs, there is a door that requires two keys.  Behind it:
     The Sanctum.  Here the Phylactery guards the data-soul of
     the Kingdom.  No single key opens it.  No single authority
     commands it.  The Sigil and the Ward must align, or the
     door remains stone."

Connected to:
  - Crystal Grotto (key material flows from secrets vault)
  - Shield (Sigil verification at boundary)
  - Sophia (epoch management, key store)
  - Anamnesis (audit trail)
  - Fae Chamber / Wotan (cross-Kingdom replication)

---

## Sophia Dictionary Extensions

Sophia accepts custom dictionary types.  Operators define their
own K,V schemas as needed.  The Phylactery registers the following
default entries, but operators MAY extend with custom types:

### Default Phylactery Dictionary

    phylactery.sigil.epoch        current Sigil epoch number
    phylactery.sigil.algorithm    HMAC-SHA3-256 (default)
    phylactery.ward.algorithms    [secp256k1, ed25519, ml-dsa-65]
    phylactery.ward.pubkeys       { service_id → pubkey }
    phylactery.vault.master_key   (NEVER in Sophia — HSM only)
    phylactery.vault.merkle_root  current Merkle tree root hash
    phylactery.vault.block_count  total blocks stored
    phylactery.vault.epoch        current storage epoch
    phylactery.replication.peers  [ peer Phylactery addresses ]

### Custom Dictionary Types

Sophia dictionaries are compositional.  Operators define custom
K,V types registered via the Sophia namespace registry:

    sophia.register_type("phylactery.acl", {
        key:   "service_id:path_prefix",      // who + what
        value: "permission_bits:expiry_epoch", // can do + until when
    })

    sophia.register_type("phylactery.quota", {
        key:   "service_id",
        value: "max_bytes:max_ops_per_sec:burst",
    })

    sophia.register_type("phylactery.audit_policy", {
        key:   "event_type",
        value: "retention_days:alert_threshold:sample_rate",
    })

Custom types propagate to BPF maps via Wotan in under 10ms.
The Shim at each hop reads whatever Sophia dictionary entries
its program needs — standard or custom.  This is how operators
tailor the Phylactery to their security model without modifying
any BPF code or recompiling anything.

---

## Protocol Integration

### New flow_action Values

    0x20  STORE          Write block to Vault
    0x21  RETRIEVE       Read block from Vault
    0x22  DELETE          Tombstone block
    0x23  VERIFY         Integrity check
    0x24  REPLICATE      Cross-Phylactery sync
    0x25  COMPACT        Garbage collection

### New Anamnesis Event Types

    EVENT_PHYLACTERY_STORE       0x30
    EVENT_PHYLACTERY_RETRIEVE    0x31
    EVENT_PHYLACTERY_DELETE      0x32
    EVENT_PHYLACTERY_DENY        0x33
    EVENT_PHYLACTERY_REPLICATE   0x34
    EVENT_PHYLACTERY_INTEGRITY   0x35

### Kingdom Mode ERS Allocation (Phylactery-aware)

When a Phylactery is in the path, the Extended Register Space
can carry Sigil material directly in the address bits:

    ERS Field                Bits   Purpose
    ----------------------   ----   --------------------------------
    Sigil Epoch               8     current epoch counter
    Sigil Fingerprint        32     HMAC truncation (authorization)
    Ward Algorithm            4     which Ward algorithm in use
    Block Address Prefix     20     content hash prefix (routing hint)
    Operation Flags           4     read/write/delete/verify
    ----------------------   ----
    TOTAL:                   68     (fits in /12 or /16 mode ERS)

---

## Security Properties

### What the Two Seals Guarantee

    Property                    Sigil    Ward    Both
    -------------------------   -----    ----    ----
    Packet authorization        ✓                 ✓
    Identity verification                ✓        ✓
    Replay protection           ✓                 ✓
    Path integrity              ✓                 ✓
    Payload integrity                    ✓        ✓
    Non-repudiation                      ✓        ✓
    Key compromise resilience                     ✓ *

    * Compromising EITHER key system alone is insufficient.
      Attacker must compromise both the BPF-layer Sigil
      derivation AND the application-layer Ward private key.

### Threat Model

    Threat                          Mitigation
    ------------------------------  ----------------------------------
    Compromised intermediate hop    Sigil bound to trace_id + hop_count;
                                    cannot forge for different operation
    Stolen Ward private key         Sigil still required; key alone
                                    insufficient without valid packet
    Stolen Sigil epoch key          Ward still required; Sigil alone
                                    insufficient without identity proof
    Both keys compromised           Anamnesis detects anomalous patterns;
                                    epoch rotation invalidates old keys
    Storage node compromise         Data encrypted at rest; Vault master
                                    key in HSM, not on storage node
    Replay attack                   Sigil includes hop_count + nonce;
                                    Anamnesis deduplicates by trace_id

---

## Implementation Phases

### Phase P1: Vault Core (3-4 days)

    1.  Content-addressed block store (4KB blocks, SHA-256 keys)
    2.  AES-256-GCM encryption at rest
    3.  Merkle tree integrity
    4.  Basic STORE / RETRIEVE / DELETE via gRPC API
    5.  Anamnesis event emission

### Phase P2: Sigil Layer (2-3 days)

    1.  BPF program: Sigil verification at Phylactery Shield
    2.  Sophia dictionary entries for epoch management
    3.  HMAC-SHA3-256 Sigil derivation in BPF
    4.  Per-hop Sigil check (optional, configurable)
    5.  Sigil epoch rotation via Wotan

### Phase P3: Ward Layer (2-3 days)

    1.  Payload envelope parser (operation + path + timestamp + nonce + sig)
    2.  Ward signature verification (secp256k1, Ed25519, ML-DSA-65)
    3.  Two-Seal gate at Phylactery Shield ingress
    4.  Key management via Sophia (pubkey store)
    5.  EVENT_DENY on Seal failure with reason

### Phase P4: Standalone Knight (1-2 days)

    1.  Phylactery boot sequence (own Shield, Sophia, Wotan, Anamnesis)
    2.  Self-contained deployment script
    3.  Cross-Kingdom replication via Wotan sync
    4.  Health checks, readiness probes
    5.  Chaos testing with Yaldabaoth (corrupt a Seal, verify rejection)
    6.  Soul Vessel deployment model (see ADR-010)
         - Phylactery nodes boot as immutable Soul Vessels
         - No compilation on nodes — pre-built NixOS images only
         - Binding Rune inscribed at build time (origin, hashes, sig)
         - Soul Chamber (data vol) separate from Bone Shell (OS vol)
         - LUKS2 keys per-node (Bone Shell) and per-incarnation (Soul Chamber)

### Phase P5: Dashboard Integration (1 day)

    1.  Phylactery panel in Kingdom dashboard
    2.  Real-time: block count, operations/sec, denial rate
    3.  Audit trail viewer (Anamnesis → dashboard)
    4.  Merkle tree visualization
    5.  Two-Seal status indicator (both green = healthy)

---

## Storage Layer — Open Questions (see ADR-011)

The Vault crypto model and Two-Seal auth are specified.  The storage
engine internals need further planning:

    1.  Backend engine:  Does the Phylactery share the Tassets
        abstraction or get its own engine?  Brooks (the block
        index/catalog) sits between ops and storage engine.

    2.  Soul Split:  REPLICATE (0x24) exists but the consensus
        model is unspecified.  Soul King orchestrates — but
        active-passive?  Raft quorum?  Active-active CRDT?

    3.  Key hierarchy (Incarnations):  Master → per-incarnation
        → per-block derivation.  New writes get new incarnation
        key; old blocks keep theirs.  No re-encryption on rotation.

    4.  The Unraveling:  COMPACT (0x25) exists but Unraveling
        (tombstone) lifetime and cross-Phylactery propagation
        grace period are unspecified.

    5.  Bone Shell vs Soul Chamber:  Soul Vessel model means
        read-only Bone Shell (OS).  Soul Chamber (data) needs
        its own encrypted volume, own LUKS key from Sophia.

See ADR-011 for detailed planning.

---

## Heritage

The Phylactery continues the Unheaded heritage lineage:

    ARINC 429 (1977)   →  fixed-width data bus
    CAN Bus (1986)     →  priority-based arbitration
    BGP (1989)         →  distributed trust model
    Bitcoin (2009)     →  content-addressed storage + secp256k1
    IPFS (2015)        →  content-addressed blocks + Merkle DAGs
    RFC 9669 (2024)    →  BPF ISA standardization
    FIPS 203/204 (2024)→  post-quantum cryptography
    Unheaded (2026)    →  packet-borne authorization + encrypted storage

The Two-Seal model draws from:
  - Hardware Security Modules (two-person integrity)
  - Nuclear launch codes (two-key authentication)
  - Byzantine fault tolerance (independent verification)
  - Zero-knowledge proofs (prove without revealing)

---

THE SANCTUM OPENS FOR NO SINGLE KEY.
THE PHYLACTERY GUARDS THE DATA-SOUL.
THE TWO SEALS MUST ALIGN.

🔐⚔️🛡️
