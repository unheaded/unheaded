# The Sacred Hierarchy — Component Architecture in ASCII

The full Unheaded component hierarchy rendered in ASCII art. This is the
fantasy version of `docs/ARCHITECTURE.md` — same components, more dramatic
presentation.

## The Hierarchy

```
                           THE SACRED HIERARCHY
                        Of the Unheaded Kingdom

                                  .
                                 /|\
                                / | \
                               /  |  \
                              /   |   \
                             /    |    \
                            /     |     \
                           /      |      \
                          /   THE CROWN   \
                         /   (Leadership)  \
                        /                   \
                       +--------+|+----------+
                                ||
              ___________________||___________________
             |                   ||                   |
             |     LAYER OF VISION & STRATEGY         |
             |                                        |
             |   +----------+      +-----------+      |
             |   | CAPTAIN  |      | TIMEGURU  |      |
             |   | Strategy |      | Timeline  |      |
             |   | & Vision |      | & Roadmap |      |
             |   +----+-----+      +-----+-----+      |
             |________|__________________|____________|
                      |                  |
              ________v__________________v________
             |                                    |
             |     LAYER OF EXECUTION             |
             |                                    |
             |  +--------------+ +-------------+  |
             |  | MICROMANAGER | | ARCHITECT   |  |
             |  | Execution    | | Design      |  |
             |  | & QA         | | & Infra     |  |
             |  +------+-------+ +------+------+  |
             |_________|________________|_________|
                       |                |
              _________v________________v_________
             |                                    |
             |     THE GNOSTIC LAYER              |
             |     (State Management)             |
             |                                    |
             |  +--------+ +--------+ +--------+  |
             |  |PLEROMA | |KENOMA  | |ANAMNE- |  |
             |  |desired | |actual  | |SIS     |  |
             |  |state   | |state   | |history |  |
             |  +---+----+ +---+----+ +---+----+  |
             |______|__________|__________|_______|
                    |          |          |
              ______v__________v__________v_______
             |                                    |
             |     THE FAE CHAMBER (Wotan)        |
             |     Message Bus / Ring Buffer      |
             |                                    |
             |  Topics: tasks.created             |
             |          timeline.updates          |
             |          alerts.critical           |
             |          state.drift.detected      |
             |          trace.events              |
             |          system.outage.reports      |
             +------------------------------------+
                    |          |          |
              ______v__________v__________v_______
             |                                    |
             |     THE PROTOCOL LAYER             |
             |     Monad + Sophia + Shield        |
             |                                    |
             |  Monad: 20-byte register file      |
             |  Sophia: BPF dictionaries          |
             |  Shield: ingress/egress boundary   |
             |  Shim: per-hop eBPF processor      |
             +------------------------------------+
                    |          |          |
              ______v__________v__________v_______
             |                                    |
             |     THE WHISPERING VOID            |
             |     eBPF Data Plane                |
             |                                    |
             |  XDP: packet_marker, shield-ebpf   |
             |  TC:  flow_tracker                 |
             |  kprobe: latency_probe             |
             |  tracepoint: syscall_tracer        |
             +------------------------------------+
                    |          |          |
              ______v__________v__________v_______
             |                                    |
             |     THE SABATONS                   |
             |     Host OS / Bare Metal           |
             |                                    |
             |  Linux 5.15+ with BPF support      |
             |  EVPN-VXLAN fabric                 |
             |  BGP control plane                 |
             +------------------------------------+
```

## Adversarial Layer (Outside the Hierarchy)

```
    ╔════════════════════════════════════════╗
    ║         YALDABAOTH (The Adversary)     ║
    ║                                        ║
    ║  Orbits the hierarchy. Injects faults  ║
    ║  at any layer. Tests resilience. The   ║
    ║  chaos agent is not part of the system ║
    ║  — it is the force that tests it.      ║
    ╚════════════════════════════════════════╝
```

## Mapping to 6-Layer Architecture

| Layer | Technical Name | Lore Name |
|-------|---------------|-----------|
| 5 | User Interface | The Crown (dashboard, kanban) |
| 4 | Application Services | Vision & Execution layers |
| 3 | Infrastructure Services | Fae Chamber (Wotan), Protocol Layer |
| 2 | Control Plane | Cuirass (unheaded-daemon) |
| 1 | Data Plane | The Whispering Void (eBPF) |
| 0 | Infrastructure | The Sabatons (host OS, EVPN-VXLAN) |

## Arcane Hollows

Each major domain is called a "Hollow" — a chamber within the kingdom:

| Hollow | Domain | Key Components |
|--------|--------|---------------|
| **Crown Hollow** | Leadership & strategy | Captain, Timeguru |
| **Forge Hollow** | Infrastructure & execution | Architect, Micromanager, IaC renderers |
| **Gnostic Hollow** | State management | Pleroma, Kenoma, Anamnesis |
| **Fae Hollow** | Messaging & communication | Wotan, topic contracts |
| **Protocol Hollow** | Wire format & dictionaries | Monad, Sophia, Shield, Shim |
| **Void Hollow** | eBPF data plane | All eBPF programs |
| **Sabaton Hollow** | Host infrastructure | OS, network fabric, BGP |
