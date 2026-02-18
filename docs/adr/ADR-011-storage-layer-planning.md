# ADR-011: Storage Layer Planning — Phylactery Vault Internals

## Status: Proposed (Medium-High Priority — spans Alpha P1 through Beta)

## Date: 2026-02-18

## Naming Convention

| Term | Meaning |
|------|---------|
| **Soul Chamber** | The data volume where encrypted blocks reside |
| **Bone Shell** | The read-only OS volume |
| **Incarnation** | Key epoch — each rotation births a new incarnation |
| **Unraveling** | Tombstone/GC — blocks dissolving from the Vault |
| **Soul Split** | Replication across multiple Phylacteries |
| **Brooks** | The Soul King's librarian — lightweight index/catalog service for block metadata |
| **Soul King** | The master orchestrator for Soul Split replication |

## Context

The Phylactery specification (PHYLACTERY.md) defines a strong cryptographic model (Two Seals, AES-256-GCM at rest, Merkle integrity, content-addressed 4KB blocks) and a complete operation set (STORE, RETRIEVE, DELETE, VERIFY, REPLICATE, COMPACT).  The Sophia dictionary, Anamnesis integration, and protocol wire format are specified.

What is NOT yet specified: the storage engine internals.  This ADR captures the open questions and proposes a planning framework for resolving them.

### What We Have (Solid)

- Two-Seal auth model (Sigil + Ward)
- AES-256-GCM encryption at rest
- Content-addressed 4KB blocks (SHA-256)
- Convergent encryption (same content → same ciphertext)
- Merkle tree integrity (root in Sophia)
- Operation codes (0x20–0x25) in Monad registers
- Anamnesis event emission for full audit trail
- Sophia dictionary entries for epoch, keys, Merkle root

### What We Need (Open Questions)

Five areas require design decisions before the Vault is production-ready.

## Decision

### Open Question 1: Backend Engine

**Problem:** The Kingdom has `pkg/storage` (Tassets) — a pluggable abstraction supporting memory, BadgerDB, and BoltDB backends.  The Phylactery could share this abstraction or get its own engine optimized for its specific access pattern.

**The Phylactery access pattern is unusual:**

- All blocks are exactly 4KB (page-aligned)
- All addressing is content-based (SHA-256 hash of plaintext)
- All blocks are encrypted (convergent encryption)
- Reads are random by content hash (no range scans)
- Writes are append-mostly (content-addressed = immutable)
- Deletes are tombstone-based (mark, don't erase)

**Options:**

| Option | Engine | Pros | Cons |
|--------|--------|------|------|
| A | Reuse Tassets (BadgerDB) | Less code, tested | LSM overhead for fixed-size blocks, compaction unpredictable |
| B | Custom block store | Optimized for 4KB aligned, O(1) hash lookup | More code, more bugs initially |
| C | Raw block device | Maximum performance, direct I/O | Complex, platform-specific, hard to test |

**Recommendation:** Start with **Option A** (BadgerDB via Tassets) for Alpha.  Profile.  If block I/O latency matters, migrate to **Option B** for Beta.  Option C is reserved for bare-metal edge cases.

**Brooks — The Block Catalog:**

Regardless of backend choice, the Phylactery needs a lightweight index service for block metadata: which blocks exist, their content hashes, incarnation numbers, sizes, and Unraveling status.  This is **Brooks** — the Soul King's librarian.

Named for Brook of the Straw Hats — the skeleton who keeps records, tells stories, and remembers everything even after death.  Brooks is a thin metadata index that sits between the operation layer and the storage engine:

```
Operation (STORE/RETRIEVE/DELETE)
        ↓
    Brooks (index lookup: hash → location + incarnation + status)
        ↓
    Storage Engine (BadgerDB / custom / raw)
```

Brooks answers: "Where is this block?  What incarnation was it written in?  Is it Unraveling?"  The storage engine answers: "Here are the bytes."  Separation of concerns — Brooks can be rebuilt from a full scan of the storage engine if corrupted.  Brooks is ephemeral metadata over durable blocks.

### Open Question 2: Replication Protocol — The Soul Split

**Problem:** The REPLICATE operation (0x24) exists in the protocol but the consensus model is unspecified.  When the Lich splits its soul across multiple Phylacteries, how does it stay coherent?

The **Soul King** is the master orchestrator for Soul Split replication.  Named for Brook — the skeleton musician of the Straw Hat Pirates who literally conquered death and whose soul transcends his body.  The Soul King conducts the replication symphony across vessels.

**Sub-questions:**

1. **Within a Kingdom:** How many Phylactery replicas?  What consistency model?
2. **Cross-Kingdom (Mode 3):** Eventual consistency?  Quorum writes?
3. **Conflict resolution:** What happens when two Kingdoms write the same content-addressed block simultaneously?

**Options:**

| Option | Model | Consistency | Complexity |
|--------|-------|-------------|------------|
| A | Active-passive (Soul King conducts one writer) | Strong (single writer) | Low |
| B | Raft quorum (Soul King conducts majority vote) | Strong (majority write) | Medium |
| C | Active-active with CRDT-style merge | Eventual | High |

**Recommendation:** **Option A** (active-passive, Soul King as conductor) for Alpha — single writer, async replication to standby.  Evaluate **Option B** (Raft) for Beta when multi-Phylactery becomes a requirement.  Option C deferred to MVP; content-addressed blocks are inherently idempotent (same hash = same content), which simplifies merge but doesn't eliminate metadata conflicts.

**Note:** Content-addressing is naturally convergent — if two Kingdoms STORE the same data, they produce the same block hash and the same ciphertext (convergent encryption).  The "conflict" is only in metadata (which replica wrote it first, Unraveling propagation, etc.).

**Soul King Anamnesis Events:**

```
EVENT_SOUL_SPLIT_START    0x60   Soul King initiating replication
EVENT_SOUL_SPLIT_SYNC     0x61   Block replicated to peer vessel
EVENT_SOUL_SPLIT_COMPLETE 0x62   All peers synchronized
EVENT_SOUL_SPLIT_CONFLICT 0x63   Metadata conflict detected
EVENT_SOUL_KING_FAILOVER  0x64   Soul King role transferred to standby
```

### Open Question 3: Key Hierarchy and Rotation

**Problem:** Sophia manages keys.  The Vault master key is HSM-only.  The derivation chain and epoch rotation semantics need specification.

**Proposed hierarchy:**

```
HSM Master Key (NEVER leaves HSM)
 └── HKDF-SHA3-256(master, "vault-epoch" || epoch_number)
      └── Epoch Key (lives in Sophia, rotates per epoch)
           └── HKDF-SHA3-256(epoch_key, "block" || content_hash)
                └── Block Key (derived per-block, never stored)
```

**Epoch rotation rules:**

1. New writes use the current epoch key
2. Old blocks retain their epoch key — NOT re-encrypted
3. Each block's metadata stores its epoch number
4. On read: derive the block key from the block's epoch (not current epoch)
5. Epoch keys are retained in Sophia until ALL blocks from that epoch are deleted or migrated
6. Sophia prunes epoch keys when block_count for that epoch reaches zero

**Why NOT re-encrypt on rotation:** Re-encrypting all blocks on key rotation is O(n) where n = total blocks.  For a large Vault this could take hours and creates a window where both old and new keys are hot.  Instead, we keep old epoch keys (they're just 32 bytes each in Sophia) and derive per-block keys on demand.  The storage cost is negligible.  The security property is preserved: a compromised epoch key only exposes blocks from THAT epoch.

### Open Question 4: The Unraveling — Garbage Collection

**Problem:** DELETE (0x22) begins an Unraveling — the block's soul dissolving.  COMPACT (0x25) completes it — reclaiming the space.  The lifecycle between these operations is unspecified.

**Proposed Unraveling lifecycle:**

```
DELETE → Unraveling begins (block marked, not erased)
         Brooks updates status: UNRAVELING
         ↓
         Grace period: 72 hours (configurable)
         Purpose: allow Soul Split peers to propagate
                  the Unraveling before final dissolution
         ↓
         Anamnesis: EVENT_UNRAVELING_ELIGIBLE (ready for dissolution)
         Brooks: block status → DISSOLVABLE
         ↓
COMPACT → Dissolution complete, space reclaimed
         Brooks: block removed from index
         Anamnesis: EVENT_BLOCK_DISSOLVED
         ↓
         Merkle tree updated (branch pruned)
         Sophia: vault.merkle_root updated
```

**Cross-Phylactery Unraveling propagation (Soul King conducts):**

- DELETE emits via Wotan to all Soul Split peers
- Soul King ensures all peers mark their own Unraveling with the same grace period
- COMPACT on any replica only runs AFTER grace period expiry
- If a peer was offline during the grace period, it reconciles Unravelings on reconnect (Anamnesis provides the DELETE event history via Brooks)

**Sophia dictionary for Unraveling policy:**

```
sophia.register_type("phylactery.unraveling_policy", {
    key:   "policy_name",
    value: "grace_hours:max_unravelings:compact_interval_minutes",
})
```

### Open Question 5: Bone Shell + Soul Chamber Split

**Problem:** ADR-010 specifies immutable Soul Vessels.  The Phylactery's Soul Chamber (data) must survive Reanimation (node replacement).

**Proposed design (see ADR-010 for full Soul Vessel model):**

```
Bone Shell: /       (read-only, LUKS2, Sophia per-node key)
                     NixOS, services, BPF programs, configs
                     Replaced entirely on Reanimation

Soul Chamber: /vault (read-write, LUKS2, Sophia per-incarnation key)
                     Encrypted blocks, Merkle tree, WAL, Brooks index
                     Mount options: rw,noexec,nosuid,nodev
                     Persists across Reanimations
                     Soul Split to peer Phylacteries via Soul King
```

**Key isolation:** Bone Shell key is per-node (tied to the vessel identity).  Soul Chamber key is per-incarnation (tied to the data incarnation).  Cracking the Bone Shell does not expose the Soul Chamber.  Cracking the Soul Chamber does not let you execute code (noexec mount).

**Reanimation path for Soul Chamber:**

- Soul Chamber is NEVER replaced during a Reanimation
- Old vessel's Soul Chamber detaches, reattaches to new Bone Shell
- New vessel verifies Soul Chamber Merkle root against Sophia before accepting it
- If Merkle root mismatch: Soul Chamber quarantined, Soul King promotes peer replica
- Brooks rebuilds its index from a full Soul Chamber scan if needed

## Consequences

### Positive

- All five open questions now have a proposed path and a phased approach
- Alpha can proceed with simpler options (Tassets backend, active-passive, existing key derivation)
- Beta upgrades are scoped and clear (custom backend, Raft, tombstone lifecycle)
- Key hierarchy avoids expensive re-encryption on rotation
- Tombstone grace period prevents premature reclaim during replication lag
- Two-volume model cleanly separates immutable OS from mutable data

### Negative

- Multiple deferred decisions create Beta tech debt
- Active-passive replication is a SPOF during failover window
- 72-hour Unraveling grace period means deleted data persists longer than expected
- Soul Chamber reattachment during Reanimation is a complex operation

### Neutral

- BadgerDB is well-tested but may not be optimal for the Phylactery's access pattern — profiling will tell
- Epoch key retention in Sophia is cheap but grows linearly with epoch count
- Unraveling grace period is configurable — operators can tune for their Soul Split topology
- Brooks index is reconstructible — ephemeral metadata over durable blocks

## Implementation Phasing

| Question | Alpha | Beta | MVP |
|----------|-------|------|-----|
| Backend engine | Tassets/BadgerDB | Profile, evaluate custom | Custom if needed |
| Soul Split (replication) | Active-passive + Soul King | Raft quorum | Active-active CRDT |
| Key hierarchy (incarnations) | Implement as specified above | Audit, harden | HSM integration testing |
| Unraveling (GC) | Basic Unraveling + manual compact | Grace period + auto-compact | Cross-Kingdom propagation |
| Bone Shell + Soul Chamber | Design + prototype | Production hardening | Multi-cloud volume management |
| Brooks (block index) | In-memory index over BadgerDB | Persistent index, rebuild-on-corruption | Distributed index for Soul Split |

## Related

- PHYLACTERY.md — Storage Layer Open Questions section
- ADR-009 — Parish Boundaries
- ADR-010 — Sealed Cask Deployment Model
- ADR-007 — Container Hardening Strategy
- `pkg/storage/storage.go` — Tassets abstraction (311 LOC)
- `pkg/secrets/store/vault.go` — Crystal Grotto integration (537 LOC)
