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
