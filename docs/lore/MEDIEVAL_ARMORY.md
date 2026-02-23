# Medieval Armory — Infrastructure as Armor

## The Metaphor

A medieval knight's armor is a layered defensive system. Each piece protects
a different body part, all pieces work together, and removing any one weakens
the whole. The same is true of production infrastructure.

Unheaded's infrastructure components map to pieces of a full suit of plate armor.
The customer's application is "the head" — unheaded (headless) means we provide
everything below the neck.

## The Armor Pieces

### Sabatons → Host OS / Bare Metal

**Historical**: Steel foot armor. The foundation the knight stands on.
**Technical**: The host operating system (Linux 5.15+), bare metal server,
or VM. eBPF programs attach here at the lowest level (XDP on physical NICs).
Without sabatons, the knight can't stand. Without a host, nothing runs.

### Gauntlets → CLI Tooling (unheaded-cli)

**Historical**: Articulated hand armor. Fine motor control for wielding weapons.
**Technical**: The operator CLI. How humans interact with the platform.
`unheaded generate`, `unheaded deploy`, `unheaded status`. The gauntlets
give operators precision control over the infrastructure.

### Vambraces → Observability (eBPF Layer)

**Historical**: Forearm armor. Protects the arms that do the detailed work.
**Technical**: The eBPF observability layer — packet marking, flow tracking,
latency probes, syscall tracing. Vambraces give you the fine-grained detail
of what's happening inside the mesh. Internally called "The Whispering Void"
because eBPF observes silently at kernel level without the application knowing.

Programs: packet_marker (XDP), flow_tracker (TC), latency_probe (kprobe),
syscall_tracer (tracepoint), shield-ebpf (XDP/TC), monad-cpu-ebpf (XDP).

### Shield → WAF / Ingress-Egress Boundary

**Historical**: Carried in the off-hand. Blocks incoming attacks.
**Technical**: The Shield service handles WAF (Web Application Firewall)
functions, Monad stamping (injecting the register file into packets entering
the mesh), and Monad stripping (removing it from packets leaving). Shield is
the boundary between the trusted internal mesh and the untrusted outside.

### Hauberk → Service Mesh

**Historical**: Chain mail shirt worn under the cuirass. Flexible, protective,
allows movement.
**Technical**: The service mesh layer — circuit breakers, mutual TLS, service
discovery, retry policies. The hauberk provides flexible protection between
rigid components. If one service fails, the hauberk (circuit breaker) prevents
cascade failure. Default-deny networking. Explicit allow only.

### Cuirass → Control Plane (unheaded-daemon)

**Historical**: The chest plate. Core body armor protecting vital organs.
**Technical**: The unheaded-daemon control plane. Manages container lifecycle,
drift detection, reconciliation, health monitoring, and auto-remediation.
The cuirass protects the core of the knight — the control plane protects
the core of the infrastructure.

### Pauldrons → Load Balancer

**Historical**: Shoulder armor. Bears the weight of the arms and weapons.
**Technical**: The load balancer (Maglev consistent hashing). Bears the
weight of incoming traffic and distributes it across service instances.

### Sword → Deployment Pipeline

**Historical**: The primary offensive weapon.
**Technical**: The deployment pipeline — canary, blue-green, rolling strategies.
The sword is how the knight strikes. The deployment pipeline is how new code
reaches production.

## The Complete Knight

```
         [APPLICATION]    ← customer brings this
         ─────────────
    ┌───[ PAULDRONS  ]───┐  ← load balancer
    │   [  CUIRASS   ]   │  ← control plane
    │   [  HAUBERK   ]   │  ← service mesh
    │ ┌─[  SHIELD    ]   │  ← WAF / boundary
    │ │ [ VAMBRACES  ]   │  ← observability (eBPF)
    │ │ [ GAUNTLETS  ]   │  ← CLI tooling
    └─┘ [  SABATONS  ]   │  ← host OS
        └────────────────┘
```

You bring the head. We provide the armor.

## Why This Metaphor Works

1. **Layered defense** — armor is not one piece, it's many pieces working
   together. Infrastructure security is the same.
2. **Each piece has a job** — pauldrons don't protect feet. Load balancers
   don't do observability.
3. **Removing one weakens all** — a knight without a hauberk has gaps in
   chain mail. Infrastructure without a service mesh has gaps in failure handling.
4. **The head is separate** — historically, the helmet was the most
   personalized piece. Your application is yours. We don't touch it.

## Mapping to the 6-Layer Architecture

| Layer | Technical Name | Armor Piece |
|-------|---------------|-------------|
| Layer 5 | User Interface | — (the head) |
| Layer 4 | Application Services | Sword (deployment) |
| Layer 3 | Infrastructure Services | Hauberk (mesh), Cuirass (control plane) |
| Layer 2 | Control Plane | Cuirass, Pauldrons |
| Layer 1 | Data Plane | Vambraces (eBPF), Shield (WAF) |
| Layer 0 | Infrastructure | Sabatons (host OS) |
