# ADR-010: Soul Vessel Deployment Model — Immutable Encrypted VMs

## Status: Proposed (High Priority — Alpha P4)

## Date: 2026-02-18

## Naming Convention

This ADR uses Lich naming conventions consistent with the Phylactery:

| Term | Meaning |
|------|---------|
| **Soul Vessel** | Immutable encrypted VM image (the lich's body) |
| **Binding Rune** | Cryptographic provenance label stamped at build time |
| **Bone Shell** | Read-only OS volume (the skeletal structure) |
| **Soul Chamber** | Read-write data volume (where the soul/data resides) |
| **Incarnation** | Key epoch — each rotation is a new incarnation |
| **Unraveling** | Tombstone/GC — the soul dissolving from a block |
| **Soul Split** | Replication across multiple Phylacteries |

## Context

The Kingdom deploys infrastructure as NixOS containers and services.  Currently, build and deployment are coupled — configuration is applied, packages are built or fetched, and services start on the target node.  This creates attack surface:

1. **Compilers on production nodes** — supply chain attacks can inject at build time
2. **Mutable filesystems** — a compromised process can modify binaries or configs
3. **Unclear provenance** — no cryptographic proof that a running image matches what was intended
4. **Unencrypted at rest** — a stolen disk exposes the full OS and application state

The Phylactery intensifies these concerns.  A storage node holding the Kingdom's data-soul cannot afford any of these risks.

### The Principle

**Nothing compiles on a running node.  EVER.**

A VM is a Soul Vessel.  It arrives full, sealed, stamped with its Binding Rune.  The Kingdom either trusts the rune or rejects the vessel entirely.  There is no "open the vessel and add ingredients."

## Decision

### The Soul Vessel Pipeline

```
BUILD PIPELINE (Sword)
┌──────────────────────────────────────────────┐
│                                              │
│  1. NixOS flake.nix → deterministic build    │
│  2. nix build → /nix/store/<hash>-image      │
│  3. dm-crypt LUKS2 encrypt the image         │
│  4. Sign with deploy key (Ed25519)           │
│  5. Inscribe THE BINDING RUNE (see below)    │
│  6. Push to artifact store (content-addressed)│
│                                              │
└──────────────────────────────────────────────┘
                    ↓
DEPLOYMENT (Greaves schedules, Gauntlets runs)
┌──────────────────────────────────────────────┐
│                                              │
│  1. Greaves selects target node              │
│  2. Gauntlets pulls Soul Vessel              │
│  3. Verify Ed25519 signature on Binding Rune │
│  4. Verify nix hash matches build_hash       │
│  5. LUKS2 key from Sophia (per-node derived) │
│  6. Mount read-only, boot                    │
│  7. Node registers with Hauberk (mesh join)  │
│  8. Shield stamps the Binding Circle from    │
│     the Rune's parish field                  │
│                                              │
│  AT NO POINT IS ANYTHING COMPILED.           │
│  AT NO POINT IS THE BONE SHELL WRITABLE.     │
│  THE VESSEL IS SEALED.                       │
│                                              │
└──────────────────────────────────────────────┘
```

### The Binding Rune

The Binding Rune is not just metadata.  It is a **cryptographic provenance chain** — the inscription that binds a Soul Vessel to its origin.  Every Kingdom component can verify where a vessel was forged.

```json
{
  "origin":      "sword-pipeline-prod",
  "build_hash":  "sha256:abc123...",
  "nix_hash":    "/nix/store/...-image",
  "signed_by":   "deploy-key-2026-02",
  "built_at":    "2026-02-18T...",
  "git_commit":  "a1b2c3d",
  "parish":      "PHYLACTERY_STORAGE",
  "incarnation": 42,
  "seal":        "<Ed25519 signature over all above fields>"
}
```

The `parish` field ties directly into ADR-009 (Binding Circles) — the Rune tells the Kingdom which circle of power this vessel belongs in.  The `incarnation` field tracks which key epoch the vessel was built for.

### Binding Rune Verification at Every Layer

| Layer | Check | Failure Action |
|-------|-------|----------------|
| **Shield** | Is this vessel's Binding Rune signed by a known deploy key? | DROP all traffic from this IP |
| **Hauberk** | Does this vessel's Rune match the mesh registration? | Refuse service discovery |
| **Sophia** | Is this Rune's build_hash in my known-good set? | Revoke LUKS key — vessel can't animate |
| **Anamnesis** | Log every Rune verification, pass or fail | Full audit trail of what lives and where it was forged |
| **Gauntlets** | Does nix_hash match the content-addressed store? | Refuse to boot — vessel tampered |

### Why NixOS Is Perfect

Nix gives deterministic builds.  Same `flake.nix` → same `/nix/store` hash → same image → every time.  The `nix_hash` in the label IS the verification.  Rebuild from source on any trusted machine and get the EXACT same hash.  If hashes don't match, someone tampered between build and deploy.

**Reproducible builds as a security primitive.**

### Why Encrypted at Rest

If someone physically steals a disk (or in cloud, snapshots a volume), they get ciphertext.  The LUKS key is per-node, derived from Sophia, and Sophia rotates on epoch.  Stolen disk + no Sophia access = useless block of entropy.

### Why No Compilers on Nodes

Eliminating compilation at runtime removes an entire attack class:

- No compiler to exploit
- No build cache to poison
- No `make install` to hijack
- No package manager to supply-chain attack
- No source code on the node at all

The attack surface shrinks to: "Can you forge a valid label?"  Which requires the deploy signing key, which lives in HSM/Sophia, never on any node.

### Phylactery-Specific: Bone Shell + Soul Chamber

The Phylactery's Soul Vessel has a unique challenge: the Bone Shell (OS) is read-only, but the Soul Chamber (data) must survive reboots.

```
PHYLACTERY NODE DISK LAYOUT
┌─────────────────────────────────┐
│  THE BONE SHELL (read-only)     │
│  LUKS2 encrypted                │
│  Key: Sophia per-node derived   │
│  Contains: NixOS, services,     │
│            configs, BPF progs   │
│  Mounts: / (ro)                 │
├─────────────────────────────────┤
│  THE SOUL CHAMBER (read-write)  │
│  LUKS2 encrypted (separate key) │
│  Key: Sophia per-incarnation    │
│  Contains: encrypted blocks,    │
│            Merkle tree, WAL     │
│  Mounts: /vault (rw, noexec,   │
│           nosuid, nodev)        │
└─────────────────────────────────┘
```

The Bone Shell is the lich's skeletal body — never written to after animation.  The Soul Chamber holds the actual soul (data) — writable but with strict mount options (`noexec, nosuid, nodev`) so no executable code can inhabit the soul's domain.  The two keys are independent: cracking the Bone Shell doesn't expose the Soul Chamber, and vice versa.

### Reanimation Path (Upgrades)

To update a Soul Vessel node — a **Reanimation**:

1. Sword forges a new vessel from updated `flake.nix`
2. New Binding Rune inscribed with new `build_hash`, `nix_hash`, `git_commit`, `incarnation`
3. Greaves schedules a rolling replacement (not in-place mutation)
4. New vessel animates, joins mesh, verifies healthy
5. Old vessel drains connections, leaves mesh, is **Unraveled** (shut down)
6. Soul Chamber detaches from old Bone Shell, reattaches to new one
7. Zero downtime.  Zero in-place mutation.  The old vessel returns to dust.

This is the same blue-green / canary model Sword already supports — applied to the infrastructure itself, not just the user's app.

### Anamnesis Events

```
EVENT_VESSEL_ANIMATE      0x50   Soul Vessel animated, Binding Rune verified
EVENT_RUNE_FORGED_FALSE   0x51   Binding Rune signature verification failed
EVENT_VESSEL_CORRUPTED    0x52   nix_hash doesn't match build_hash
EVENT_VESSEL_BANISHED     0x53   Sophia revoked LUKS key — vessel can't animate
EVENT_REANIMATION         0x54   Vessel replaced via rolling update
EVENT_VESSEL_UNRAVELING   0x55   Old vessel draining before final death
```

## Consequences

### Positive

- Eliminates supply chain attacks at runtime (no compilers, no package managers)
- Cryptographic provenance for every running image
- Encrypted at rest protects against physical theft
- NixOS reproducibility makes verification trivial
- Label ties into Parish Boundaries (ADR-009) for defense-in-depth
- Rolling replacement means zero-downtime upgrades
- Two-volume model isolates OS from data with independent keys

### Negative

- Build pipeline becomes critical path — if Sword is down, no new deploys
- Image size may be larger than minimal container images (full NixOS closure)
- LUKS key distribution via Sophia adds a boot-time dependency
- Debugging is harder on read-only nodes (must attach ephemeral debug containers)

### Neutral

- Nix build times can be cached aggressively (content-addressed, deterministic)
- The artifact store needs its own HA story (it holds every sealed cask)
- Node replacement is fast but not instant — LUKS decrypt + boot + mesh join

## Priority

**High — Alpha P4.**  The Phylactery MUST be a Sealed Cask before Beta.  Other services SHOULD adopt the model during Beta hardening.

## Related

- PHYLACTERY.md — Phase P4 item 6
- ADR-007 — Container Hardening Strategy
- ADR-008 — Security Hardening Baseline
- ADR-009 — Parish Boundaries (label carries parish assignment)
- ADR-011 — Storage Layer Planning (two-volume model details)
