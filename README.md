# Unheaded

A mapped data bus over IPv6 Hop-by-Hop Options, with configuration
management automation for immutable infrastructure.

Unheaded provides packet-level observability, service mesh, control plane, security
baseline, and a protocol that turns every packet into a 20-byte
register file executing eBPF programs at wire speed.

Interchangeable container runtimes (LXD, containerd, NixOS, Docker)
and IaC backends (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt).
Three-platform deployment (NixOS + Docker + LXD) with BGP EVPN underlay.
Your tools, our state model.

## Status

Age 1 (Alpha) ~98%. Age 2 (Beta IaC) ~40%.

~260K production LOC (220K Go, 16K Rust, 13K JS, 5K Nix, 7K scripts)
plus 203K test LOC (~93% test-to-production ratio). 25 services, 37+
packages, 8 eBPF programs (16K LOC Rust/Aya), 16 protocol packages (Go).

Age 2 IaC shipped: NixOS modules + Docker Compose + LXD profiles across
three deployment platforms. Firewall ingress/egress (OPNsense + IPFire),
routing fabric (BGP EVPN default; OSPFv3/IS-IS+SR-MPLS/MPLS LDP alternate),
Suricata IDS (GPL-2.0 isolated), full observability stack
(Prometheus/Loki/VictoriaMetrics/Grafana/Alertmanager). Bare metal
live validation pending (S70+).

Core protocol specified in three Internet-Drafts (IETF Experimental track):

- [draft-bellis-unheaded-protocol-foundation-04](docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md) — Monad wire format
- [draft-bellis-unheaded-sophia-dictionary-01](docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md) — Sophia BPF dictionaries
- [draft-bellis-unheaded-wotan-memory-01](docs/protocol/draft-bellis-unheaded-wotan-memory-01.md) — Wotan memory model

## The Protocol

The Unheaded Protocol encodes a 20-byte Monad (5 × u32 register file)
in the IPv6 Hop-by-Hop Options extension header (next-header 0x00,
HOPOPT). At each hop, an eBPF Shim reads and writes the Monad. The
packet is the working memory of a distributed computation.

Key properties:

    Monad:       20 bytes, 5 registers (R0-R4), CRC-16/CCITT integrity
    Encoding:    Exponent-encoded fields via Sophia dictionaries
    Processing:  Per-hop eBPF (XDP/TC), O(1) per packet
    Scope:       Limited Domain [RFC 8799] — every hop is controlled
    HbH type:    0x3E (option type), len=18, MUST NOT be stripped
    Heritage:    ARINC 429 → I2C → CAN Bus → BGP → BPF → IPv6 → Unheaded

With Kingdom Mode (optional, requires EVPN-VXLAN), deterministic
IPv6 address bits are reclaimed as Extended Register Space:

    /8 mode:   208 bits reclaimed (26 bytes)  — 16.7M hosts
    /12 mode:  216 bits reclaimed (27 bytes)  — 1M hosts
    /16 mode:  224 bits reclaimed (28 bytes)  — 65K hosts

    Formula:   reclaimed = 2 * (128 - host_bits)

Combined with the Monad, a /16 deployment carries 48 bytes of
computational register state per packet with zero wire overhead.

Post-quantum identity binding (ML-KEM-768, ML-DSA-65 per FIPS
203/204) is integrated via Sophia key store. Zero additional wire bytes.

Full specification: [docs/protocol/](docs/protocol/)

## Architecture

    BARE METAL / VM (host-a: Forge, host-b: Outpost)
    ├── OPNsense 26.1.2 (BSD-2-Clause) / IPFire 2.29 (GPL-3.0)
    ├── FRR (BGP EVPN AS65001) / BIRD (BGP AS65002)
    ├── WireGuard tunnel fd00:dead:beef::/48, MTU=1380
    ├── EVPN-VXLAN VNIs 10001/10002/10100, BFD 300ms
    ├── unheaded-daemon          control plane, drift detection
    ├── eBPF programs            XDP packet marking, flow tracking, latency
    └── services (containers, VMs, or bare metal processes)
        ├── wotan                message bus (gRPC streaming, pub/sub)
        ├── shield               ingress/egress boundary (Monad stamp/strip)
        ├── sophia               dictionary service (BPF map management)
        ├── monad                register file processor
        ├── anamnesis            event ring buffer, trace correlation
        ├── kenoma / pleroma     outer/inner domain separation
        ├── trace-collector      eBPF → Wotan bridge (Rust)
        ├── dashboard-backend    metrics aggregator, WebSocket
        ├── kanban-app           self-hosting proof
        ├── gateway              HTTP/3, QUIC, gRPC-Web
        └── yaldabaoth           chaos injection (controlled fault testing)

Routing options (switchable via scripts/routing/select-routing.sh):

    bgp-evpn    BGP EVPN (default) — full L2 extension, VXLAN VNIs
    ospf        OSPFv3 Option A    — simple, no AS numbers
    isis        IS-IS+SR-MPLS      — segment routing, level-2-only
    mpls        MPLS LDP           — full TE, label-switched paths

IDS: Suricata (GPL-2.0) isolated via EVE JSON file I/O + BPF fd
sharing. Custom Monad rules SID 9000001-9000099. pkg/anamnesis/
bridges EVE JSON events to Wotan topic security.suricata.alert.

## Repository Layout

    unheaded/
    ├── cmd/                     service binaries
    │   ├── unheaded-daemon/     control plane agent
    │   ├── protocol-api/        Monad/Sophia/Wotan wire handlers
    │   ├── monad/               register file processor
    │   ├── sophia/              dictionary management
    │   ├── trace-collector/     eBPF → Wotan bridge (Rust)
    │   ├── dashboard-backend/   metrics (Go + WebSocket)
    │   ├── kanban-app/          self-hosting demo
    │   ├── routing-health/      HTTP :8080 /health /ready /metrics
    │   ├── ebpf-exporter/       eBPF metrics → Prometheus
    │   ├── ebpf-loader/         BPF program lifecycle
    │   ├── ebpf-collector/      BPF map reader
    │   └── unheaded-cli/        operator CLI
    │
    ├── services/                microservice definitions (25 services)
    │   ├── wotan/               message bus
    │   ├── shield/              ingress/egress
    │   ├── sophia/              dictionaries
    │   ├── anamnesis/           event tracing
    │   ├── yaldabaoth/          chaos injection
    │   └── ...
    │
    ├── ebpf/                    eBPF programs (Rust, aya framework)
    │   ├── packet-marker/       trace ID injection at XDP
    │   ├── flow-tracker/        connection state tracking
    │   ├── latency-probe/       RTT measurement
    │   ├── hop-ebpf/            per-hop Monad processing
    │   ├── shield-ebpf/         ingress/egress stamping
    │   ├── monad-cpu-ebpf/      register file computation
    │   └── ...
    │
    ├── pkg/                     shared Go packages (37+ packages)
    │   ├── protocol/            16 RFC cross-pollination packages
    │   │   ├── encoding/        varint (RFC 9000), exponent, CRC-16, TLV
    │   │   ├── registry/        generic IANA-style Registry[K,V]
    │   │   ├── bpfschema/       BPF map key/value structs
    │   │   └── ...              amplification, migration, integrity, dos
    │   ├── metrics/             Collector interface, baremetal/lxd/docker/nixos
    │   ├── anamnesis/           EVE JSON → 64-byte RingEntry → Wotan
    │   ├── iac/                 6 IaC renderer backends
    │   ├── observability/       8 observability adapter backends
    │   ├── transport/           gRPC-first with HTTP fallback
    │   ├── discovery/           four-layer service discovery
    │   ├── logagg/              log aggregation, 10K-line ring buffer
    │   ├── auth/                JWT, mTLS, RBAC (skeleton)
    │   └── ...
    │
    ├── nixos/                   NixOS deployment (one supported runtime)
    │   ├── flake.nix            Nix flake entry point
    │   ├── hosts/               host-a (Forge) + host-b (Outpost) configs
    │   ├── modules/             per-subsystem NixOS modules
    │   │   ├── firewall-bridge.nix    br-unheaded, HbH passthrough nftables
    │   │   ├── frr.nix / bird.nix     routing daemons
    │   │   ├── suricata.nix           IDS (decode-events: no)
    │   │   ├── observability.nix      Prometheus/Loki/Grafana/VictoriaMetrics
    │   │   ├── wireguard.nix          WireGuard fd00:dead:beef::/48
    │   │   ├── opnsense-vm.nix        OPNsense QEMU via libvirtd
    │   │   ├── ipfire-vm.nix          IPFire QEMU via libvirtd
    │   │   └── services/              25 per-service .nix modules
    │   └── tests/               NixOS test modules (nixosTest framework)
    │
    ├── docker/                  Docker Compose deployment
    │   ├── hosts/               host-a + host-b docker-compose.yml
    │   ├── suricata/            Dockerfile + suricata.yaml (decode-events: no)
    │   └── routing/             FRR + BIRD containerized routing daemons
    │
    ├── lxd/                     LXD container/VM deployment
    │   ├── profiles/            base, eBPF, GPU, firewall profiles
    │   ├── containers/          per-service YAML definitions
    │   └── hosts/               init + launch scripts
    │
    ├── routing/                 routing daemon configs
    │   ├── frr/                 BGP EVPN + IS-IS + BFD (FRR, host-a)
    │   ├── bird/                BGP + BFD + radv (BIRD, host-b)
    │   ├── ospf/                OSPFv3 alternate (Option A)
    │   ├── isis/                IS-IS+SR-MPLS alternate (Option B)
    │   ├── mpls/                MPLS LDP alternate (Option C)
    │   └── suricata/rules/      Monad IDS signatures SID 9000001-9000099
    │
    ├── monitoring/              observability stack configs
    │   ├── prometheus/          scrape configs (25 services + routing)
    │   ├── loki/                30d retention, 90d for security streams
    │   ├── promtail/            host-a + host-b log shipping
    │   ├── victoriametrics/     long-term metrics storage
    │   ├── alertmanager/        alert routing + Monad HbH drop rate rules
    │   ├── grafana/dashboards/  infrastructure, containers, routing, firewall, eBPF
    │   └── docker-compose.yml   full observability stack
    │
    ├── scripts/                 operational automation
    │   ├── routing/             select-routing.sh (live switcher)
    │   └── firewall/            setup-opnsense.sh, setup-ipfire.sh, health-check.sh
    │
    ├── docs/                    documentation
    │   ├── protocol/            Internet-Drafts, patches, error registry
    │   ├── network/             topology, HbH passthrough rules, routing options
    │   ├── legal/               SURICATA_GPL_ISOLATION.md, IP-INVENTORY.md
    │   ├── security/            lich-campaigns.md, dark-grimoire-addendum.md
    │   ├── sessions/            session handoffs + battle plans
    │   └── adr/                 architecture decision records
    │
    └── references/              timeline.md (living roadmap)

## Technology

    Go 1.24          services, control plane, CLI
    Rust             eBPF programs (aya), trace-collector
    Linux 5.15+      minimum kernel (BPF, XDP, TC, MPLS)
    gRPC             inter-service transport (Wotan)
    eBPF (XDP/TC)    packet processing, tracing, chaos
    EVPN-VXLAN       L2 overlay fabric (VNIs 10001/10002/10100)
    BGP / IS-IS      control plane routing (FRR host-a, BIRD host-b)
    WireGuard        east-west encrypted tunnel fd00:dead:beef::/48
    Suricata         IDS, GPL-2.0 isolated via EVE JSON boundary
    HTTP/3 + QUIC    external gateway
    Prometheus       metrics collection + alerting
    Loki             log aggregation
    VictoriaMetrics  long-term metrics storage
    NixOS            declarative host configuration (one supported runtime)

Runtime-agnostic: bare metal, LXD, Docker, Podman, systemd-nspawn,
Firecracker, or any Linux environment with BPF support. NixOS is
provided as one declarative option.

## Build

Requires: Linux (kernel 5.15+), Go 1.24+, Rust nightly,
root or sudo for eBPF loading.

    make build          # build all Go services
    make ebpf           # build eBPF programs (Rust)
    make containers     # build container images
    make test           # run test suite
    make dev            # local development (docker-compose)

## Deploy

    # observability stack (no root required)
    docker compose -f monitoring/docker-compose.yml up -d

    # routing health probe
    ./bin/routing-health --addr :8080

    # full bare metal (Linux 5.15+, root required)
    sudo ./scripts/setup-host.sh
    make build && make ebpf
    sudo ./scripts/load-ebpf.sh
    sudo nixos-rebuild switch --flake ./nixos#host-a   # NixOS only

    # routing selection
    sudo scripts/routing/select-routing.sh bgp-evpn|ospf|isis|mpls

## Protocol Documents

    docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md   Monad wire format
    docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md     Sophia BPF dictionaries
    docs/protocol/draft-bellis-unheaded-wotan-memory-01.md          Wotan memory model
    docs/network/FIREWALL_TOPOLOGY.md                               Host-a/b topology, addressing
    docs/network/MONAD_HBH_FIREWALL_RULES.md                        HbH passthrough per platform
    docs/network/ALTERNATE_ROUTING_OPTIONS.md                       Routing option comparison
    docs/legal/SURICATA_GPL_ISOLATION.md                            GPL-2.0 isolation boundary
    docs/security/dark-grimoire-addendum.md                         Attack surface taxonomy
    pkg/protocol/PATTERN_MATRIX.md                                  RFC→Package→BPF map matrix

## References

    RFC 8200    IPv6 Specification (Monad HbH container)
    RFC 9673    IPv6 Hop-by-Hop Options Processing
    RFC 9669    BPF Instruction Set Architecture (Sophia/Shim)
    RFC 9000    QUIC Transport (varint, amplification, migration patterns)
    RFC 9114    HTTP/3 (error codes, settings, GOAWAY, stream types)
    RFC 8949    CBOR (Sophia dictionary serialization)
    RFC 8126    IANA Considerations (registry management)
    RFC 8799    Limited Domains and Internet Protocols
    RFC 9197    IOAM Data Fields
    RFC 3031    MPLS Architecture (label stack outer, IPv6+HbH inner)
    RFC 5308    IS-IS for IPv6 (TLV 236 routing)
    FIPS 203    ML-KEM (Kyber) Key Encapsulation
    FIPS 204    ML-DSA (Dilithium) Digital Signatures

## License

MIT — see [LICENSE](./LICENSE).

Protocol specifications: MIT — see LICENSE-PROTOCOLS.

Suricata (GPL-2.0) is process-isolated; see
[docs/legal/SURICATA_GPL_ISOLATION.md](docs/legal/SURICATA_GPL_ISOLATION.md).

## Contact

    Stevie Bellis <stevie@bellis.tech>
    stevenrbellis@gmail.com
    https://github.com/stevenrbellis
