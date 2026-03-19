# Naming Map — Fantasy → Technical

Complete mapping of every lore name in the Unheaded codebase to its
technical component, origin mythology, and rationale.

## Four Pillars of Naming

Unheaded's naming draws from four mythological traditions, each mapping
to a distinct architectural domain:

| Pillar | Tradition | Domain | Example |
|--------|-----------|--------|---------|
| **1. Gnostic Cosmology** | Valentinian Gnosticism | State management | Pleroma, Kenoma, Anamnesis, Monad, Sophia |
| **2. Medieval Armory** | European arms & armor | Infrastructure layers | Cuirass, Hauberk, Shield, Pauldrons, Vambraces |
| **3. Norse/Wagnerian** | Norse mythology, Wagner's Ring Cycle | Protocol & messaging | Wotan, Mysteltainn, Tyrfing, Nagan |
| **4. Contemplative Traditions** | Sufi, Taoist, Hindu, Kabbalistic, Christian mysticism, Shamanistic | Age 2/3 operational systems | Sleipnir (BGP), Yggdrasil (OS), Gleipnir (Config) |

## Pillar 1: Gnostic Cosmology → State Management

The Gnostic cosmological model maps directly to infrastructure state management.
This is not coincidence — it is the reason we chose it.

| Gnostic Concept | Greek | Technical Component | Why It Fits |
|----------------|-------|-------------------|-------------|
| **Monad** | μονάς ("unity") | 20-byte register file (5×u32) in IPv6 HbH | The Monad is the indivisible unit of divine emanation. Our Monad is the indivisible unit of per-packet computation — 5 registers that travel with every packet as a single atomic unit. |
| **Sophia** | σοφία ("wisdom") | BPF dictionary service | Sophia is divine wisdom, the knowledge that makes creation possible. Our Sophia holds the exponent-encoded dictionaries that give meaning to the Monad's register values — without Sophia, the Monad is just bytes. |
| **Pleroma** | πλήρωμα ("fullness") | Desired state service | The Pleroma is the fullness of the divine realm — perfect, complete, the ideal. Our Pleroma holds the desired state: what the infrastructure SHOULD be. The configuration truth. |
| **Kenoma** | κένωμα ("emptiness") | Actual state / drift detection | The Kenoma is the void, the material world that falls short of the ideal. Our Kenoma holds the observed state: what the infrastructure ACTUALLY is. The gap between Pleroma and Kenoma is drift. |
| **Anamnesis** | ἀνάμνησις ("remembrance") | Event sourcing, audit trail | Anamnesis is the soul's remembrance of its divine origin. Our Anamnesis is the system's memory — event sourcing, WAL, audit logs. How we got from there to here. |
| **Yaldabaoth** | The Demiurge | Chaos injection service | Yaldabaoth is the false creator god who introduces disorder into the material world. Our Yaldabaoth is the chaos engineering service — deliberately introducing faults to test resilience. The adversary is part of the system. |

### The Reconciliation Loop

```
Pleroma (desired state)
    ↓ compare
Kenoma (actual state)
    ↓ if drift detected
Anamnesis (record event)
    ↓ remediate
Kenoma → Pleroma (state converges)

Meanwhile, Yaldabaoth randomly breaks things to ensure this loop actually works.
```

This is a standard reconciliation loop (Kubernetes uses the same pattern with
desired/actual state). We just gave it better names.

## Pillar 2: Medieval Armory → Infrastructure Layers

The "suit of armor" metaphor maps each infrastructure component to a piece
of medieval armor. The user's application is "the head" — Unheaded provides
everything else the knight needs.

| Armor Piece | Technical Component | Why It Fits |
|------------|-------------------|-------------|
| **The Crown** | Project leadership | Commands the kingdom |
| **Cuirass** | Control plane (unheaded-daemon) | Core chest armor — protects the vital organs. The control plane protects the core of the infrastructure. |
| **Hauberk** | Service mesh (circuit breakers, mTLS) | Chain mail worn under the cuirass — flexible protection between rigid plates. The mesh provides flexible inter-service protection. |
| **Shield** | WAF / ingress-egress boundary | Obvious — blocks incoming attacks. Our Shield is the WAF and Monad stamp/strip boundary. |
| **Pauldrons** | Load balancer | Shoulder armor — bears the weight. The load balancer bears the traffic weight. |
| **Vambraces** | Observability layer (eBPF tracing) | Forearm armor with fine detail work. Observability gives you the fine-grained detail of what's happening. |
| **Gauntlets** | CLI tooling | Hand armor — what you use to interact with things. The CLI is how operators interact with the system. |
| **Sabatons** | Host OS / bare metal | Foot armor — the foundation you stand on. The host OS is the foundation everything runs on. |
| **Sword** | Deployment pipeline | Offensive capability — how you strike (deploy). |

### The Complete Knight

```
    [APPLICATION]  ← "the head" — user brings this
    ─────────────
    [  CUIRASS  ]  ← control plane (unheaded-daemon)
    [ HAUBERK   ]  ← service mesh (circuit breakers, mTLS)
    [  SHIELD   ]  ← WAF, ingress/egress
    [ PAULDRONS ]  ← load balancer
    [ VAMBRACES ]  ← observability (eBPF)
    [ GAUNTLETS ]  ← CLI tooling
    [  SABATONS ]  ← host OS / bare metal
```

## Pillar 3: Norse/Wagnerian → Protocol & Messaging

| Norse Name | Origin | Technical Component | Why It Fits |
|-----------|--------|-------------------|-------------|
| **Wotan** | Odin (Germanic form) | Message bus / ring buffer / BPF map substrate | Wotan (Odin) is the all-seeing, all-knowing father god who sits at the center of all knowledge. Our Wotan sits at the center of all inter-service communication. Also: The Ascension of Busboy — Wotan started as a simple coordinator ("busboy") and evolved into the central intelligence of the system. |
| **Mysteltainn** | Mistletoe (killed Baldr) | Reserved: future component | The weapon that killed the invulnerable — represents finding the one weakness. |
| **Tyrfing** | Cursed sword (must kill when drawn) | Reserved: future component | A weapon that must be used once invoked — represents irreversible operations. |
| **Nagan** | Jörmungandr (World Serpent) | Reserved: future component | Encircles the world — represents global state or ring topology. |
| **Halcyon** | Greek: calm seas (from Alcyone) | Reserved: graceful shutdown / steady-state | Named for the calm after the storm. |

### The Ascension of Busboy to Wotan

Wotan began its life as "Busboy" — a simple message relay that cleared tables
between services. As the architecture evolved, Busboy took on more roles:
message routing, ring buffer storage, BPF map substrate. It became the
all-knowing center of the system. The rename from Busboy to Wotan reflects
this evolution — from humble servant to the all-father of the service mesh.

This is documented because the skill named "Busboy" still exists as the
coordination/translation layer. Busboy the skill remembers its origins.
Wotan the service has ascended.

## Pillar 4: Contemplative Traditions → Age 2/3 Operational Systems

The Fourth Naming Pillar encompasses Age 2/3 long-term vision components,
drawn from mystical traditions emphasizing cosmic structure and restoration.

| Component | Norse Mythology | Ragnarok Online | Technical Role | Why It Fits |
|-----------|-----------------|-----------------|---|---|
| **Sleipnir** | Odin's 8-legged horse (fastest mount) | First Seal (foundation unlock) | BGP routing daemon (8 ECMP paths) | Eight legs = eight equal-cost multipath routes. Speed = fast convergence. The mount that travels all nine realms mirrors routing across all Kingdom clusters. |
| **Yggdrasil** | World Tree (connects 9 realms) | Full HP/MP restore, sustains cosmos | Hardened Debian base OS image builder | The tree that holds all worlds together mirrors the OS that holds all Kingdom infrastructure together. Full restore (fresh install, fully provisioned, everything healthy). |
| **Gleipnir** | Magical binding chain (holds Fenrir) | Megingjard (binding component) | Config convergence daemon (daily eventual consistency) | The chain that cannot break represents immutable convergence of configurations across all runtimes, IaC backends, and observability targets. |
| **Sentinel** | Heimdall (Norse watchman of Bifrost, sees/hears all nine realms) | Lookout tower | Blue team defense skill (network monitoring, CVE triage, daily adversarial loop) | Heimdall guards the boundary between realms — Sentinel guards the Kingdom boundary via network monitoring, firewall management, Pi-hole DNS defense, IoT device inventory, and coordinated adversarial testing with BlackMage. Sees everything (observability) and hears everything (alerting). |

### Future Naming Pools (12 documented, 4 currently active)

Unheaded reserves 12 additional thematic naming pools for future Age 2/3 components:

1. **Norse Weapons** (Mysteltainn, Tyrfing, Surtr's sword)
2. **Wagnerian Operas** (Tristan und Isolde, Parsifal, Lohengrin)
3. **Greek Atmospheric** (Zephyr, Aeolus, Aether, Helios)
4. **Hindu Deities** (Indra, Agni, Vayu, Soma) — ACTIVE: 3 mappings
5. **Hindu Cosmology** (Chakra, Kundalini, Prana)
6. **Taoist Cosmology** (Qi, Yin-Yang, Wu Wei, Tao)
7. **Japanese Shinto** (Kami, Torii, Yama, Shimenawa)
8. **Pagan Earth Traditions** (Stonehenge, Druid groves, sacred wells)
9. **Shamanistic Journeys** (Axis Mundi, Spirit Guide, Underworld)
10. **Kabbalistic Tree of Life** (Sephiroth, Paths, En Sof)
11. **Sufi Mysticism** (Faqir, Dhikr, Annihilation, Unity)
12. **Christian Mysticism** (Theodicy, Beatific Vision, Theosis)

Each pool has 4-6 reserved terms for future operational components, ensuring
naming never repeats across domains and maintaining mythological consistency
as the Kingdom scales.

## Other Names

| Name | Origin | Technical Component |
|------|--------|-------------------|
| **Phylactery** | Greek/D&D: soul vessel | Encrypted state persistence |
| **Dark Grimoire** | Medieval: book of dark magic | Attack surface taxonomy |
| **LICH** | D&D: undead wizard | Lethal Infrastructure Chaos Hunter (fuzzing framework) |
| **Fae Chamber** | Fairy court | Service interface contracts (Wotan pub/sub topics) |
| **Kingdom Mode** | Political entity | Extended register space via EVPN-VXLAN IPv6 address reclamation |
| **The Whispering Void** | Original | eBPF data plane — "whispers" because it observes silently at kernel level |
| **The Meta Moment** | Self-reference | Unheaded hosting its own development tools on its own infrastructure |
| **Parish Boundaries** | Church governance | Network segmentation / service isolation boundaries |
| **Sealed Cask** | Wine aging | Immutable deployment artifacts |

## Anti-Patterns (Names We Don't Use)

Per the Lore skill's Sacred Laws:

- Never name something after a deity that implies infallibility (our systems fail)
- Never use names that imply user data awareness (we never touch it)
- Never reuse a name across architectural domains
- Humor is encouraged but clarity comes first — if the name confuses the mapping, pick a different name
