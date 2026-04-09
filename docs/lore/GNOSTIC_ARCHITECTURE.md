# Gnostic Architecture — State Management Through Cosmology

## The Theological Model

Valentinian Gnosticism describes a cosmos with a clear hierarchy:

1. **Pleroma** (πλήρωμα) — The divine fullness. Perfect, complete, ideal.
2. **Kenoma** (κένωμα) — The material void. Imperfect, fallen, real.
3. **Monad** (μονάς) — The indivisible divine unit. Source of all emanation.
4. **Sophia** (σοφία) — Divine wisdom. Knowledge that bridges Pleroma and Kenoma.
5. **Anamnesis** (ἀνάμνησις) — Remembrance of divine origin. The soul's memory.
6. **Yaldabaoth** — The Demiurge. False creator who introduces disorder.

## The Engineering Model

Every term maps one-to-one to an infrastructure state management concept:

```
┌───────────────────────────────────────────────────────┐
│              PLEROMA (Configuration Truth)              │
│         What the infrastructure SHOULD be               │
│    Declarative configs, desired state, version control  │
└──────────────────────────┬────────────────────────────┘
                           ↓ Reconciliation (every 30s)
┌───────────────────────────────────────────────────────┐
│                KENOMA (Observed Reality)                │
│         What the infrastructure ACTUALLY is             │
│    Runtime state, drift detection, health probes        │
└──────────────────────────┬────────────────────────────┘
                           ↓ Event stream
┌───────────────────────────────────────────────────────┐
│             ANAMNESIS (Historical Memory)               │
│         How we got from there to here                   │
│    Event sourcing, WAL, audit logs, trace correlation   │
└──────────────────────────┬────────────────────────────┘
                           ↓ Adversarial testing
┌───────────────────────────────────────────────────────┐
│              YALDABAOTH (The Adversary)                 │
│         Controlled chaos to test resilience             │
│    Fault injection, latency spikes, partition tests     │
└───────────────────────────────────────────────────────┘
```

## Why Gnostic Naming?

The mapping is not forced — it is natural:

| Concept | Gnostic Tradition | Infrastructure Tradition | Existing Art |
|---------|------------------|------------------------|-------------|
| Ideal vs. actual | Pleroma vs. Kenoma | Desired state vs. drift | Kubernetes, configuration management tools |
| Atomic data unit | Monad (indivisible unity) | Register file, packet header | CPU registers, ARINC 429 |
| Knowledge layer | Sophia (wisdom) | Dictionary, schema, encoding | BPF maps, CBOR, Protobuf |
| Memory / history | Anamnesis (remembrance) | Event sourcing, audit log | Kafka, EventStore, WAL |
| Adversarial testing | Yaldabaoth (chaos agent) | Chaos engineering | Netflix Simian Army, Litmus |

Kubernetes uses "desired state" and "actual state." Traditional IaC tools use
"catalog" and "facts." We use "Pleroma" and "Kenoma." The pattern is identical —
the vocabulary is more memorable.

## Technical Mapping Details

### Monad — The 20-Byte Register File

The Gnostic Monad is the indivisible unit from which all existence emanates.
Our Monad is the indivisible 20-byte register file (5 × u32) carried in the
IPv6 Hop-by-Hop Options extension header. Every packet carries one. It cannot
be split. At each hop, an eBPF Shim reads and writes the registers. The packet
itself is the working memory of a distributed computation.

```
Monad layout (20 bytes):
    R0 (u32)  — General purpose / trace ID fragment
    R1 (u32)  — General purpose / flow state
    R2 (u32)  — General purpose / metrics accumulator
    R3 (u32)  — General purpose / routing hint
    R4 (u32)  — CRC-16 integrity + flags + reserved
```

### Sophia — The Dictionary Service

Sophia holds the exponent-encoded dictionaries that give meaning to the Monad's
raw register values. Without Sophia, R0=0x4F2B is meaningless bytes. With Sophia,
it decodes to `{service: "timeguru", method: "GET", status: 200}`.

Sophia dictionaries are stored as BPF maps and synchronized across the mesh via
the Wotan message bus. They use exponent encoding (variable-precision fields
packed into fixed-width registers) to maximize information density.

### Pleroma & Kenoma — The Reconciliation Loop

Pleroma stores the declared desired state (Git-backed YAML/TOML configs).
Kenoma observes the actual running state (probes, metrics, health checks).
The unheaded-daemon compares them every 30 seconds. If drift is detected:

1. Anamnesis records the drift event
2. Remediation is attempted (restart, reconfig, redeploy)
3. If remediation fails, alert escalation via Wotan

### Enkrateia — The Restoration Verb (ADR-043)

Greek ἐγκράτεια — *"self-control,"* *"mastery over oneself,"* *"continence."*
In Gnostic ethical writing, Enkrateia is not a state — it is the *action* by
which the soul aligns the material with the divine. Where Pleroma is the ideal
that exists, Kenoma is the fallen reality, and Anamnesis is the memory of how
they diverged, Enkrateia is the *will* that pulls Kenoma back into harmony with
Pleroma.

In the Mímir's Law PoC ([ADR-043](../adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md)),
Enkrateia (`pkg/enkrateia/`) is the restoration loop — the daemon that
processes drift events from Heimdall and routes them. **In v1 of the PoC,
Enkrateia is alerts-only**: it emits human-reviewable alerts and does NOT
mutate filesystem state. This is a hard BlackMage condition: a machine that
heals too aggressively becomes a machine that thrashes; a machine that trusts
unsigned messages becomes a machine an attacker controls. Enkrateia must earn
the right to act through the LICH-012 Configuration Convergence Attacks
campaign before any auto-restore version (v2) is enabled.

Enkrateia is not a new state layer in the Pleroma/Kenoma/Anamnesis trinity —
it is the verb form of the reconciliation loop itself. The trinity describes
*what is*; Enkrateia describes *what acts*.

### Yaldabaoth — Chaos Engineering

Named for the Demiurge who introduces disorder to test creation, Yaldabaoth
is the chaos injection service. It runs controlled fault scenarios:

- Network partition between service pairs
- Latency injection on specific Wotan topics
- CPU/memory pressure on target containers
- eBPF program detach (simulating data plane failure)
- Clock skew injection

Every scenario runs through the reconciliation loop — if the system can't
self-heal under Yaldabaoth's interference, it's not production-ready.

## ADR Reference

- ADR-001: Gnostic State Management — decision record for this naming scheme
- ADR-002: Kingdom Naming Convention — broader naming conventions

## Recommended Reading

- Jonas, Hans. *The Gnostic Religion*. Beacon Press, 1958.
- Pagels, Elaine. *The Gnostic Gospels*. Random House, 1979.
- Burns, Brendan et al. *Kubernetes: Up and Running*. O'Reilly, 2022. (for the reconciliation loop pattern)
