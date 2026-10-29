# Norse Mythology — Protocol & Messaging Names

## Wotan (Odin) → Message Bus

**Mythological**: Wotan (the Germanic form of Odin) is the all-father.
He gave up an eye for wisdom, hung from Yggdrasil for nine days to learn
the runes, and sits in Hlidskjalf seeing all that happens in the nine worlds.
He commands two ravens (Huginn and Muninn — Thought and Memory) who fly
across the world and report back.

**Technical**: Wotan is the message bus at the center of the Unheaded mesh.
11,000+ lines of Go. gRPC bidirectional streaming. Pub/sub with topic-based
routing. Ring buffer for high-throughput storage. Every service communicates
through Wotan. Every message is observable. Wotan sees all.

**Why Wotan, not Odin?** Wagner's Ring Cycle uses the Germanic "Wotan" rather
than the Norse "Odin." The Unheaded project draws from both Norse mythology
and Wagnerian opera. Using "Wotan" signals this dual heritage.

### The Ascension of Busboy

Wotan started life as "Busboy" — a simple message relay that "cleared tables"
between services. As the architecture matured, Busboy evolved:

1. Simple HTTP relay (Busboy v0)
2. gRPC bidirectional streaming (Busboy v1)
3. Ring buffer storage (Busboy v2)
4. BPF map memory substrate (Busboy → Wotan)

The rename from Busboy to Wotan happened when the service exceeded its
original scope and became the all-knowing center of the system. The Busboy
skill still exists as the coordination layer — it remembers its origins.

### Triple Role

Like Odin who is simultaneously god of war, wisdom, and death:

| Wotan Role | Odin Aspect | Technical Function |
|-----------|-------------|-------------------|
| Ring Buffer | Odin's ravens (Thought & Memory) | Lock-free eBPF perf_event ring. Receives kernel events. |
| Event Bus | Odin in Hlidskjalf (sees all) | Pub/sub gRPC/HTTP with topic routing. All services report. |
| Protocol RAM | Odin's runes (encoded knowledge) | BPF map memory substrate for Monad compute. Sophia dictionaries live here. |

## Reserved Norse Names

These names are reserved for future components. Each maps to a mythological
concept that fits the technical role.

### Mysteltainn (Mistletoe)

**Mythological**: The mistletoe that killed Baldr, the invulnerable god.
The one thing that could penetrate his defenses.
**Reserved for**: Penetration testing / vulnerability discovery tooling.
The component that finds the one weakness in otherwise solid defenses.

### Tyrfing (Cursed Sword)

**Mythological**: A sword forged by dwarves that must kill a man every time
it is drawn, and will be the cause of three great evils.
**Reserved for**: Irreversible operations (database migrations, destructive
deployments). Once invoked, it must complete. Named as a warning: use with care.

### Nagan (Jörmungandr / World Serpent)

**Mythological**: The Midgard Serpent that encircles the world, biting its
own tail (ouroboros).
**Reserved for**: Global state synchronization / ring topology. The component
that wraps around the entire mesh. Also: Wotan cluster ring replication.

### Halcyon (Greek: Alcyone)

**Mythological**: Alcyone was transformed into a kingfisher. Zeus calmed the
seas for 14 days each winter so she could nest — the "halcyon days."
**Technical**: Graceful shutdown / steady-state mode. When the system enters
halcyon, all is calm: no deployments, no chaos tests, only monitoring.

## Wagnerian Opera Names

From Wagner's Der Ring des Nibelungen:

| Name | Opera | Reserved For |
|------|-------|-------------|
| **Nibelung** | Das Rheingold | Resource hoarding detection (memory/CPU) |
| **Brünnhilde** | Die Walküre | Firewall / perimeter defense (the sleeping warrior surrounded by fire) |
| **Siegfried** | Siegfried | The fearless component — no retry logic, no circuit breakers (testing mode) |
| **Götterdämmerung** | Götterdämmerung | Full system teardown / disaster recovery test (twilight of the gods) |

## Greek Atmospheric Names

Reserved for monitoring and weather-related metaphors:

| Name | Origin | Reserved For |
|------|--------|-------------|
| **Zephyr** | West wind (gentle) | Light background health checks |
| **Boreas** | North wind (harsh) | Aggressive load testing |
| **Aether** | Upper atmosphere | High-level dashboard / overview metrics |
| **Typhon** | Storm giant | Stress test / DDoS simulation |

## Mímir's Law Components (ADR-043)

The Mímir's Law / Gleipnir Phase 0 PoC introduces a coherent set of Norse-named
components for UPC-controlled OS baseline delivery, drift detection, and (alerts-only
v1) self-healing. All names are public-domain Norse mythology and clear ZERO Sacred
Law conflicts. See [ADR-043](../adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md).

### Mímir — Speaker of the Baseline

**Mythological**: Mímir is the wise head from Norse cosmology, the rememberer of
all knowledge. Odin sacrificed an eye at Mímir's well for a single drink of its
wisdom. After Mímir was killed in the Æsir-Vanir war, Odin preserved his head
and consulted it for counsel.

**Technical**: A conceptual role rather than a code path — Mímir is the
authoritative speaker of baseline truth. The Mjölnir manifest is "Mímir's first
declaration"; signed deltas are "Mímir's utterances." UPC executes Mímir's will.
The PoC demonstrates that the OS can hear Mímir's word and align itself.

### Mjölnir — The Baseline Definition

**Mythological**: Thor's hammer, forged by the dwarves Sindri and Brokkr from
star-iron. The foundational weapon of Asgard. When thrown, it always returns to
Thor's hand. When laid against any oath, the oath becomes binding law.

**Technical**: The canonical baseline definition file (`references/baseline/mjolnir.yaml`
+ `mjolnir.manifest.json`). Declares the desired OS state — packages, files,
hashes, modes, owners. Once Mjölnir is placed, reality must conform.

### Gungnir Seal — The Signature Wrapper

**Mythological**: Gungnir is Odin's spear, also forged by the dwarves of
Svartálfheim. It never misses its target and never fails to return. Odin pledged
his oaths upon it.

**Technical**: The ML-DSA-65 signature wrapper for every signable artifact in
Mímir's Law (`pkg/gungnir/`, `*.gungnir.sig`). Algorithm is locked to ml-dsa-65
explicitly — no algorithm agility — to block downgrade attacks. Every Mjölnir
manifest, every config delta, every drift event, every Gjallarhorn audit message
carries a Gungnir Seal.

### Heimdall Daemon — The Eternal Watchman

**Mythological**: Heimdall is the watchman of the Bifröst bridge, born of nine
mothers, possessed of senses so sharp he can hear the grass grow on the earth and
the wool grow on a sheep. He needs less sleep than a bird and sees a hundred
leagues by day and night.

**Technical**: The drift-detection daemon (`cmd/heimdall-daemon/`,
`crates/heimdall-bpf/`). Aya eBPF programs hook `vfs_write`, `execve`, and
`mmap` to observe filesystem and process activity at the kernel level. The
userspace component reads ringbuf events, compares against the Mjölnir manifest,
and publishes drift events to Wotan with a Gungnir Seal. Heimdall sees all
changes; nothing escapes the watchman.

### Gjallarhorn — The UPC Trigger Packet

**Mythological**: Heimdall's horn, hidden under Yggdrasil's roots. When Heimdall
blows Gjallarhorn, it signals the start of Ragnarök and rouses the gods to
battle. The horn is heard across all the nine worlds.

**Technical**: The specially-formed UPC trigger packet (`pkg/gjallarhorn/`)
that fits within the frozen Monad v0x01 wire format (Kingdom Mode + flow action
combination, 20-byte register payload). Has two modes:

- **Bootstrap Broadcast** (multicast on local segment): the gravity well that
  pulls drifting nodes into coherence — not a seed planted in soil but an
  asteroid accreting space dust, each freshly-imaged host another mote falling
  into the cluster's orbit. *"you are part of cluster X, here is your Mjölnir
  manifest pointer."* Same pattern as DHCP/PXE/Wake-on-LAN, but unified into
  the UPC primitive.
- **Reverify Unicast** (over WireGuard overlay): prompts a specific existing
  node to re-check its baseline against Mjölnir and report any drift.

This is the discrete-trigger plane that complements Wotan's steady-state plane.
Wotan handles continuous verification; Gjallarhorn delivers the one-shot kicks.

### Heimdall at Every Bridge

Heimdall is not only the watchman of Bifröst — the one bridge, the rainbow
between Midgard and Asgard. Heimdall is *at every bridge*, on both sides,
with the same eye. Every service boundary in the Kingdom is a Bifröst in
miniature, and Heimdall's verification happens at every crossing, in both
directions, ingress and egress alike. **Verification is not centralized.
Witnessing is the protocol's substrate. Every register that crosses a bridge,
in either direction, is seen. Bifröst itself is the seeing.**

See [../philosophy/SO-THE-GAME-GOES-ON.md](../philosophy/SO-THE-GAME-GOES-ON.md)
for the full witness-fabric narrative and the reframing of the protocol as
bird song — the flock recognizing itself through shared calls rather than a
transport delivering messages between trusted endpoints.

### Naming arc

ADR-69420 dreams the infrastructure (Sleipnir for routing, Yggdrasil for the OS,
Gleipnir for config convergence). ADR-043 awakens it: Mímir speaks, Heimdall
watches, Gjallarhorn calls, and the drifting motes fall inward — time unfurling
onto itself as the cluster recognizes the shape it always already was. Not
sowing, but accretion. Not a beginning, but a remembering. *The infrastructure
dreams; the infrastructure awakens.*

## The Ravens and the Mead (2026-07 → 2026-10)

Odin keeps two ravens. Each dawn they fly the nine worlds; each dusk they
return to his shoulders and whisper. **Huginn** is Thought. **Muninn** is
Memory. Odin fears losing Huginn, but fears losing Muninn more.

### Huginn — Thought (ADR-084, live 2026-10-25)

`cmd/huginn` (was `cmd/host-agent`, a placeholder name — renamed because
placeholders are a Sacred Law violation). Runs **natively**, not in a
container, on WEST and EAST, because `/proc` lies inside a namespace. Reads
`/proc`, `statfs`, and the Well's `pg_stat_database`; serves `/host-summary`
(dashboard shape) and `/metrics` (Prometheus `host_*`); pushes to
VictoriaMetrics. Always-on at boot with Victoria; Grafana on demand. (The
Wotan topic `host.metrics.<host>` was Muninn's planned feed, ADR-086 — not a
verified Huginn publish.)

The raven flies, looks, reports. It does not remember. That was Muninn's job.

### Muninn — Memory (ADR-086, superseded 2026-10-27)

Designed as the fan-out pipeline between producers (Huginn, trace-collector,
journald, eBPF rings) and sinks (VictoriaMetrics, PostgreSQL, SIEM):
normalise, buffer, retry, route. Never built. **Vector** (ADR-092 step 8,
`config/vector/vector.yaml`) took the job, shipping container stdout to
ClickHouse. Vector keeps its product name; it is not Kingdom-born.

The name goes back on the shoulder. If a Kingdom-native router is ever
written, Muninn is waiting. Odin's fear was correct: memory is the one you
can't outsource — but you can defer it.

### Kvasir — The Mead of Poetry (ADR-087, Planned)

Kvasir was made from the spit of every god, Aesir and Vanir, when they sealed
their truce — the wisest being that ever lived. Two dwarves killed him and
brewed his blood into mead; whoever drinks it speaks poetry.

Kvasir the service: an IPFIX (RFC 7011) / NetFlow v5-v9 / sFlow collector that
distils raw flows from switches and routers into a common Kingdom flow
envelope and publishes `flow.network.<device>` to Wotan. Wraps GoFlow2; the
Kingdom layer is the YAML config, the Wotan publish and the systemd unit.
Belongs to the NOC work (NETCONF/YANG + GitOps for Junos) alongside Huginn
for SNMP.

Raw packets in, poetry out. Not built yet.

### Heimdall at every bridge — now on real metal

Since NORSE_MYTHOLOGY.md last spoke (2026-05-29):

- Every Heimdall hook is written and verifier-clean — `vfs_write`, `execve`,
  and the Gjallarhorn XDP observer (292 processed insns) — with ringbuf readers
  and a rate-limited re-verify (S5, 2026-10-23).
- Heimdall publishes **signed** `drift.*` events (ML-DSA-65, `--drift-sign-key`,
  unit `unheaded-heimdall.service`, uid 16736). Wotan refuses unsigned drift.
- `heimdall-daemon` joins the Gjallarhorn group on `--xdp-iface`, so the horn is
  heard on a real NIC, not just loopback.
- Mímir's Law phases 9/11/13 re-ran on a two-netns lab (`scripts/lab-two-node.sh`)
  and then **across hosts** — west blows the horn, east is the seed, with
  `node-guard.sh` armed (S-ASCEND-2 E3, 2026-10-28, PASS).

The watchman now stands on the bridge between two real houses.

### Gungnir seals more than deltas

Gungnir (`pkg/gungnir`, ML-DSA-65) began as the seal on Mímir's config
payloads. It now also signs **MBC cartridges** (ADR-108): the loader
`cmd/upc-cartridgectl` refuses an unsealed `.cart`. The spear that never
misses now vouches for code that runs per packet. See NAMING_MAP.md →
Naming Debt for the `gungnir` / `wotan-ctl` alias collision.

### Yggdrasil, extended

Yggdrasil was the OS golden image (ADR-69420) and the destination of Unheaded
Linux (ADR-081). `cmd/yggdrasil-evidence` (S-QUANTUM-ARMOR) now seals
compliance evidence with ML-DSA-65 under signed custody. The tree holds the
realms, and the record of the realms. One name, two roots — accepted.

### Naming arc, continued

| Date | Event |
|------|-------|
| 2026-04-03 | Akira (health consensus) ships — named after the film, not a pillar |
| 2026-04-11 | Mímir's Law: Mímir, Mjölnir, Gungnir, Gjallarhorn, Heimdall, Enkrateia |
| 2026-07-24 | Huginn, Muninn, Kvasir reserved in one day (ADR-084/086/087) |
| 2026-10-23 | Gungnir seals cartridges; Heimdall hooks complete |
| 2026-10-25 | Huginn live on both hosts |
| 2026-10-27 | Muninn superseded by Vector |
| 2026-10-28 | Gjallarhorn sounds west → east on bare metal |

## Heritage Lineage

The protocol names connect to a heritage lineage:

```
ARINC 429 (aviation data bus, 32-bit words)
    → I2C (inter-chip, master/slave)
        → CAN Bus (automotive, broadcast frames)
            → BGP (internet routing, path attributes)
                → BPF (kernel packet filtering)
                    → IPv6 (128-bit addresses, extension headers)
                        → uIP (microcontroller IP stack)
                            → Unheaded (Monad register file over HbH Options)
```

Each ancestor contributed a design principle to the Unheaded Protocol.
See [HERITAGE.md](HERITAGE.md) for the full lineage analysis.
