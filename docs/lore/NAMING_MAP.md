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
| **Huginn** | Odin's raven, "thought" | Host metrics agent (`cmd/huginn`, ADR-084, live 2026-10-25) | Flies every host each day and reports back. Reads `/proc`, `statfs`, `pg_stat_database`; serves `/host-summary`, `/metrics`; pushes to VictoriaMetrics. Native on WEST and EAST. |
| **Muninn** | Odin's raven, "memory" | Observability fan-out pipeline (ADR-086) — **superseded by Vector 2026-10-27** | Huginn observes, Muninn remembers and routes. Never built; Vector took the job. Name held in reserve for a Kingdom-native router. |
| **Kvasir** | Wisest being; his blood became the Mead of Poetry | IPFIX / NetFlow / sFlow collector (ADR-087, Planned) | Distils raw flows into intelligence. Wraps GoFlow2; publishes `flow.network.<device>` to Wotan. |
| **Mímir / Mjölnir / Gungnir / Gjallarhorn / Heimdall** | See [NORSE_MYTHOLOGY.md](NORSE_MYTHOLOGY.md) | Mímir's Law — UPC baseline delivery + drift detection (ADR-043) | Phases 9/11/13 re-run across hosts 2026-10-28 (west sender → east seed). |

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
| **LICH** | D&D: undead wizard | Lethal Infrastructure Chaos Hunter (red-team / offense — fuzzing + future autonomous adversary) |
| **Cerydwyn** | Welsh myth: Cerridwen, keeper of the cauldron of Awen (knowledge, healing, transformation); Pagan/Druidic pool | Tomb's **white-hat / blue-team** triage LLM (Mistral-7B + RAG over the Grimoire) — CVE/patch/threat-assessment. The **counter to LICH**: where Lich attacks, Cerydwyn brews remedies. Renamed from "Oracle" 2026-05-29. Sentinel-aligned. |
| **Fae Chamber** | Fairy court | Service interface contracts (Wotan pub/sub topics) |
| **Kingdom Mode** | Political entity | Extended register space via EVPN-VXLAN IPv6 address reclamation |
| **The Whispering Void** | Original | eBPF data plane — "whispers" because it observes silently at kernel level |
| **The Meta Moment** | Self-reference | Unheaded hosting its own development tools on its own infrastructure |
| **Parish Boundaries** | Church governance | Network segmentation / service isolation boundaries |
| **Sealed Cask** | Wine aging | Immutable deployment artifacts (`cmd/cask-verify`) |
| **The Well** | Mímisbrunnr — the well Odin paid an eye to drink from (Lore reading; origin not recorded in an ADR) | PostgreSQL 16 on west: `unheaded_app` / `unheaded_ops` / `unheaded_config`. Migrations `cmd/well-migrate` (ADR-101), init order ADR-091. ADR-019: "The Well is Anamnesis — the Kingdom's memory." |
| **Akira** | The 1988 film (outside the four pillars — see Naming Debt) | Health consensus monitor (`cmd/akira`, 19100, since 2026-04-03). Percentage-consensus doctrine (ADR-029); signs `system.health.*` with ML-DSA-65. |
| **Zhenai** (真爱, "true love") | Chinese — a champion's name, not a product label (ADR-019) | The local fine-tuned Mistral-7B agent. **Seer → Champion** ascension 2026-04-03. Second brain = Claude API bridge (ADR-104); both behind one gate (ADR-106). |
| **Champion / Confirmer** | Chivalric — the knight who acts for the Crown; the one who nods | Zhenai's action gate (`pkg/champion`); out-of-band write confirmation (`deny`/`tty`, ADR-106). The model never sees the confirmation code. |
| **Steel Echo** | Forge register (S-REAL-STEEL) | Game-day workload (`cmd/steel-echo`): deployable HTTP target, availability meter, Monad frame sender/counter. An echo that never echoes user data. |
| **Cartridge** | Game-console ROM in a slot — swappable, signed, small | MBC cartridge (ADR-108): `.cart` image (magic `MBCC`), `xdp` + `tick` stages, run per flow by `ebpf/cartridge-ebpf`. Three shipped: token-bucket limiter, 1:N sampler, int8 anomaly scorer. Signed with Gungnir ML-DSA-65; loader `cmd/upc-cartridgectl` (26610). UPC naming stays engineering-first. |
| **EVOLUTION-1** | Engineering — the first evolution question (2026-07-07) | Waking the owned xv6 kernel's syscall path. Stage 1 closed; Stage 2 rungs 0-1 closed 2026-10-28 (ADR-109: a kernel row per BPF slot, `GETPID_INJECT` + `UPC_RUNG0` on); rung 2 ADR-110 Proposed (kernel clock from BPF ticks). |
| **Rung** | Dream Ladder sub-step | One gated flip inside a Stage. Ladder → Stage → Rung → Gate. |
| **Unheaded Linux** | Plain — ours | Own minimal OS kernel on the UPC, not uClinux/busybox (ADR-081, Accepted 2026-07-03). Grows toward the Yggdrasil golden image. |
| **unheadnix** | Portmanteau, working name | Single-`.mbc` ephemeral compute unit on the UPC (ADR-082, brainstorm only). |
| **Yggdrasil Evidence** | The World Tree, extended | `cmd/yggdrasil-evidence`: ML-DSA-65 evidence seals with signed custody (S-QUANTUM-ARMOR). Yggdrasil now spans OS image *and* compliance custody. |
| **Quantum Armor** | Sprint register | Hybrid ML-KEM (`X25519MLKEM768`) TLS, ML-DSA-65 signing, CycloneDX 1.6 crypto inventory (ADR-105). The armor got a new alloy. |

## Addendum — What develop named, 2026-05-29 → 2026-10-28

Lore last wrote here on 2026-05-29. Since then develop shipped ~30 sprints and
ADR-080 → ADR-110. This section records the naming consequences. Each entry
cites the ADR or wiki page; nothing below is invented.

### Status changes to existing names

| Name | Was | Is now | Source |
|------|-----|--------|--------|
| **Yaldabaoth** | Four injectors that logged and returned nil | Real faults inside `cmd/chaos-controller` (19012, host unit `unheaded-chaos`): tc netem, container kill, systemd stop-kill, `packet_chaos` (skb mark `0xC4A05`, lab netns only). Allowlist, three abort paths, revert journal. Game day 1 (2026-10-26): a 300 ms netem canary rolled itself back in 62.4 s. *The Demiurge got hands — and a leash.* | ADR-107, `wiki/Game-Days.md` |
| **Sword** | Deploy stub | Rolling / blue-green / canary / recreate through `pkg/deploy`; canary analysis under the Kingdom SLO guard (`pkg/sloguard`); history in the Well (migration 039). | S-REAL-STEEL, `wiki/Service-Sword.md` |
| **Cuirass** | Control plane in name | Talks to west's LXD over HTTPS with a restricted client cert (project `unheaded`); manages the stable proxy endpoint; allowlisted stop/kill. | S-REAL-STEEL |
| **Anamnesis** | Event ring | Event codes moved into the spec: `BORN 0x00 … CHAOS 0x04 … ANOMALY 0x08`; flow actions `FORWARD 0x00 … MIRROR 0x04` (foundation-06, "spec wins", S-TRUE-WIRE E2). | S-TRUE-WIRE |
| **Wotan** | Topic signing on `config.*` | Signed prefixes: `config.` `drift.` `audit.` `alerts.critical.` `system.health.consensus.` `system.health.remediate.` — producers sign or refuse. SIGHUP reloads allowlists. WAL replication halts on a rejected entry. | S-IRON-GATE Phase 9, S-TRUE-WIRE |
| **Heimdall** | Scaffold | Every hook written and verifier-clean (vfs_write, execve, Gjallarhorn XDP observer); publishes signed `drift.*`; joins the Gjallarhorn group on a real NIC. Watches from the bridge on both hosts. | S5, S-ASCEND-2 |
| **Gleipnir** | Age 2b vision | Phase 0 (Mímir's Law) phases 9/11/13 re-run on two netns, then west → east bare metal. | ADR-043 (2026-10-27 correction) |
| **Dream Ladder** | L1 → L6 | Summit reached and climbed past: xv6 shell + 5 cmds on the UPC (Gate D, 2026-07-02); then EVOLUTION-1 Stages and Rungs above it. Ladder → Stage → Rung → Gate is the current vocabulary. | `wiki/Linux-on-UPC.md`, EVOLUTION-1 |
| **Muninn** | Reserved raven | Superseded by Vector (ADR-092 step 8). The raven did not fly. | ADR-086 |
| **Sealed Cask** | Metaphor | `cmd/cask-verify` exists. | tree |

### Second Ascension: Seer → Champion

Busboy → Wotan (2026-02-18) was the first. The second is **Zhenai**:
the read-only **Seer** (RAG oracle) became the **Champion** (ADR-019,
2026-04-03) — an agent that reads Kanban, edits config, runs runbooks,
and writes every action to the Well *before* it acts. Then it grew a second
brain (Claude API, ADR-104) and both brains were put behind one gate
(ADR-106, "Two brains, one gate"): same Champion gate, same audit, same
privacy tier; the human **Confirmer** sits out of band and the model never
sees the confirmation code.

Same template as Busboy: the working name becomes the origin story.
"Seer" is history, not garbage.

### The sprint register (a naming convention in its own right)

Since July, sprints carry forge names, not numbers:

`S-IRON-GATE` · `S-QUANTUM-ARMOR` · `S-REAL-STEEL` · `S-TRUE-WIRE` ·
`S-CLEAN-SLATE` · `S-TWO-BRAINS` · `S-ASCEND-2` · `S-UPC-WIRE-COMPUTE`

Pattern: `S-<ADJECTIVE>-<ARMORY NOUN>`. The adjective says what the sprint
*proves* (real, true, iron, clean); the noun is the piece being forged. This is
Pillar 2 applied to time instead of components. Keep it.

### ADR-083 — The Two Tongues

Lore now has a formal counterpart: ADR-083 (Accepted 2026-07-23) keeps a
**terminology register** of internal-vs-user-facing names. First row:
"zero user data access" (internal, precise) ↔ "NIST 800-207 Zero Trust
Architecture" (user-facing, recognised). Rule: internal surfaces (CLAUDE.md,
ADRs, code) keep the sharp term; the wiki at `:20002` and adopter docs use the
framework name; history is never rewritten.

Lore's job is the mythology; ADR-083's job is the adopter translation. When a
lore name reaches a user-facing surface, check the register first.

### Protocol spec state (names are frozen here)

`foundation-06` · `sophia-03` · `wotan-03` / `wotan-04`. S-TRUE-WIRE's E2
ruling: **spec wins** over code wherever they disagreed. Error-code registry
symbols exist in both Go (`pkg/protocol/errors`) and Rust
(`monad_common::ErrorCode`), parity-tested against
`docs/protocol/error-registry.md`. Renaming anything in this paragraph is an
RFC amendment, not a lore decision.

### Naming debt (gungnir/mjolnir resolved; rest OPEN — Stevie decides)

| Collision | Where | Rule broken | Lore recommendation |
|-----------|-------|-------------|---------------------|
| **gungnir** = `wotan-ctl` alias (binary-lore-names.md, 2026-02-24) **and** Gungnir Seal = `pkg/gungnir` ML-DSA-65 wrapper (ADR-043) | CLI vs crypto | "Never reuse a name across architectural domains" | **RESOLVED 2026-10-28** — alias withdrawn from binary-lore-names.md. Gungnir is the seal. |
| **mjolnir** = `doom-go-injector` alias **and** Mjölnir = the baseline manifest (`references/baseline/mjolnir.*`) | Doom tooling vs config truth | same | **RESOLVED 2026-10-28** — alias withdrawn. The hammer is the law, not the injector. |
| **bifrost** = `doom-bridge` alias, while Heimdall "watches every bridge" (NORSE_MYTHOLOGY.md) | Doom vs Mímir's Law | mythological adjacency without a binding | Harmless today; rename if a real Bifrost (host-to-host overlay, WireGuard west↔east) ever wants the name. |
| **Akira** | Health monitor | Outside all four pillars (a film, not a tradition) | Grandfather it. It has 7 months of runtime and a signed topic. Record it, don't rename it. |
| **The Well** | PostgreSQL | No recorded origin | Adopt Mímisbrunnr formally (Odin's eye for wisdom = Wotan's memory in the Well). One line in ADR-091 or ADR-101 closes it. |
| **Yggdrasil** | OS image (ADR-69420) **and** evidence seals (`cmd/yggdrasil-evidence`) | one name, two functions | Acceptable stretch — the tree holds the realms *and* the record of them. Note it; don't split it. |

### Hosts and places

| Name | What |
|------|------|
| **WEST** / **EAST** | The two bare-metal Kingdom hosts. West: the Well, LXD, Zhenai. East: the seed node, hybrid-PQ sshd only since 2026-10-27, `node-guard.sh`. Joined by a WireGuard overlay (PSK rotated 2026-10-27). |
| **Doom Range** | Port Authority: every service on 16666–26666. |
| **`upc0`** | The network namespace the UPC boots inside (S-ASCEND-2): both veth ends inside, no link to the host. |
| **The Tomb** | `tomb/` — Lich campaigns, Grimoire, Cerydwyn. See [TOMB_OF_KNOWLEDGE.md](TOMB_OF_KNOWLEDGE.md). |

### Counts (from the tree, 2026-10-28)

ADR numbers reach 110 (105 distinct numbers present, + ADR-69420). `cmd/`: 67
entries. `crates/`: 11. `services/`: 21. Runbooks: 9 categories under
`runbooks/` with a sha256 MANIFEST. ADR-088 counted 56 deployable components.
Numbers go stale; the paths don't.

## Anti-Patterns (Names We Don't Use)

Per the Lore skill's Sacred Laws:

- Never name something after a deity that implies infallibility (our systems fail)
- Never use names that imply user data awareness (we never touch it)
- Never reuse a name across architectural domains
- Humor is encouraged but clarity comes first — if the name confuses the mapping, pick a different name
