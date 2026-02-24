# Unheaded

A mapped data bus over IPv6 Hop-by-Hop Options, with configuration
management automation for immutable infrastructure.

You bring the application.  Unheaded provides everything else:
packet-level observability, service mesh, control plane, security
baseline, and a protocol that turns every packet into a 20-byte
register file executing eBPF programs at wire speed.

Interchangeable container runtimes (LXD, containerd, NixOS, Docker)
and IaC backends (Ansible, Terraform, Puppet, Kubernetes, Chef, Salt).
Your tools, our state model.

## Status

Alpha (~99%).  ~261K production LOC (220K Go, 16K Rust, 13K JS, 5K Nix, 7K scripts)
plus 203K test LOC (~93% test-to-production ratio).  25 services, 37 packages,
8 eBPF programs (16K LOC Rust/Aya), 16 protocol packages (Go).

Core protocol specified in three Internet-Drafts (IETF Experimental track):

- [draft-bellis-unheaded-protocol-foundation-04](docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md) — Monad wire format
- [draft-bellis-unheaded-sophia-dictionary-01](docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md) — Sophia BPF dictionaries
- [draft-bellis-unheaded-wotan-memory-01](docs/protocol/draft-bellis-unheaded-wotan-memory-01.md) — Wotan memory model

## The Protocol

The Unheaded Protocol encodes a 20-byte Monad (5 × u32
register file) in the IPv6 Hop-by-Hop Options extension header.
At each hop, an eBPF Shim reads and writes the Monad.  The packet
itself is the working memory of a distributed computation.

Key properties:

    Monad:       20 bytes, 5 registers (R0-R4), CRC-16 integrity
    Encoding:    Exponent-encoded fields via Sophia dictionaries
    Processing:  Per-hop eBPF (XDP/TC), O(1) per packet
    Scope:       Limited Domain [RFC 8799] — every hop is controlled
    Heritage:    ARINC 429 → I2C → CAN Bus → BGP → BPF → IPv6 → uIP → Unheaded

With Kingdom Mode (optional, requires EVPN-VXLAN), deterministic
IPv6 address bits are reclaimed as Extended Register Space:

    /8 mode:   208 bits reclaimed (26 bytes)  — 16.7M hosts
    /12 mode:  216 bits reclaimed (27 bytes)  — 1M hosts
    /16 mode:  224 bits reclaimed (28 bytes)  — 65K hosts

    Formula:   reclaimed = 2 * (128 - host_bits)

Combined with the Monad, a /16 deployment carries 48 bytes of
computational register state per packet with zero wire overhead.

Post-quantum identity binding (ML-KEM-768, ML-DSA-65 per FIPS
203/204) is integrated via Sophia key store — service identifiers
are cryptographically bound to PQC keypairs.  Zero additional
wire bytes.

Full protocol specification: [docs/protocol/](docs/protocol/)

## Architecture

    BARE METAL / VM
    ├── unheaded-daemon          control plane, drift detection
    ├── eBPF programs            XDP packet marking, flow tracking, latency
    └── services (containers, VMs, or bare metal processes)
        ├── wotan                message bus (gRPC streaming, pub/sub)
        ├── shield               ingress/egress boundary (WAF, Monad stamp/strip)
        ├── sophia               dictionary service (BPF map management)
        ├── monad                register file processor
        ├── anamnesis            event ring buffer, trace correlation
        ├── kenoma / pleroma     outer/inner domain separation
        ├── trace-collector      eBPF → Wotan bridge (Rust)
        ├── dashboard-backend    metrics aggregator, WebSocket, log viewer
        ├── kanban-app           self-hosting proof
        ├── gateway              HTTP/3, QUIC, gRPC-Web, WebSocket
        └── yaldabaoth           chaos injection (controlled fault testing)

### Port Allocation — "The Doom Range" (16666-26666)

All services use high ports to avoid conflicts with common dev tools:

    Infrastructure   16666-16999   doom-bridge (16666), trace-collector (16670/16671)
    Control Plane    17000-17999   unheaded-daemon HTTP (17000), gRPC (17001)
    Wotan            18000-18099   wotan HTTP (18000), gRPC (18001)
    Core Services    19000-19999   timeguru (19000), architect (19001), captain (19002),
                                  micromanager (19003), monad (19004), sophia (19005)
    Applications     20000-20999   dashboard (20000), kanban (20001), wiki (20002)
    Gateway          21000-21443   HTTP (21000), HTTPS (21443)
    User Apps        26000-26666   reserved for user applications

### Transport — gRPC-First

All inter-service communication defaults to Wotan gRPC streaming
(port 18001).  HTTP serves as fallback (HTTP/3 -> HTTP/2 -> HTTP/1.1).
Every service exposes both gRPC and HTTP health checks.  If gRPC
fails but HTTP responds, the service reports DEGRADED status.

### Log Aggregation — "The Chronicler's Well"

All services publish structured logs to Wotan topic `logs.<service>.<level>`.
Ring buffer retains 10,000 lines per service.  Dashboard serves
`GET /api/v1/logs` for queries and `WebSocket /ws/logs` for live tail.

### Service Discovery — "The Cartographer's Eye"

Three-layer discovery: (1) convention-based `/opt/unheaded/<service>/config.yaml`,
(2) port scanning to verify declared ports are listening,
(3) Wotan registration via `system.discovery.*` topics with automatic
deregistration on shutdown.

### Network Fabric

EVPN-VXLAN with BGP control plane.  All inter-node
traffic is IPv6 over VXLAN tunnels.  eBPF programs attach at XDP
(ingress) and TC (egress) on every interface.  Services run as
containers (LXD, Docker, Podman), VMs, or bare metal processes —
the protocol is runtime-agnostic.  Only requirement: Linux kernel
5.15+ with BPF support.

## Repository Layout

    unheaded/
    ├── cmd/                     service binaries
    │   ├── unheaded-daemon/     control plane agent
    │   ├── monad/               register file processor
    │   ├── sophia/              dictionary management
    │   ├── trace-collector/     eBPF → Wotan bridge (Rust)
    │   ├── dashboard-backend/   metrics (Go)
    │   ├── kanban-app/          self-hosting demo (Go + JS)
    │   ├── waf/                 web application firewall
    │   ├── ebpf-loader/         BPF program lifecycle
    │   ├── ebpf-collector/      BPF map reader
    │   └── unheaded-cli/        operator CLI
    │
    ├── services/                microservice definitions (24 services)
    │   ├── wotan/               message bus
    │   ├── shield/              ingress/egress
    │   ├── sophia/              dictionaries
    │   ├── monad/               register processor
    │   ├── anamnesis/           event tracing
    │   ├── kenoma/              outer domain
    │   ├── pleroma/             inner domain
    │   ├── yaldabaoth/          chaos injection
    │   └── ...                  (see services/ for full list)
    │
    ├── ebpf/                    eBPF programs (Rust, aya framework)
    │   ├── packet-marker/       trace ID injection at XDP
    │   ├── flow-tracker/        connection state tracking
    │   ├── latency-probe/       RTT measurement
    │   ├── hop-ebpf/            per-hop Monad processing
    │   ├── shield-ebpf/         ingress/egress stamping
    │   ├── monad-cpu-ebpf/      register file computation
    │   ├── monad-common/        shared Monad types
    │   ├── yaldabaoth-ebpf/     chaos fault injection
    │   └── syscall-tracer/      syscall auditing
    │
    ├── pkg/                     shared Go packages (37+ packages)
    │   ├── ebpf/                BPF loader, map management, anamnesis
    │   ├── protocol/            RFC cross-pollination packages (16)
    │   │   ├── encoding/        varint (RFC 9000), exponent, CRC, TLV
    │   │   ├── registry/        generic IANA-style Registry[K,V]
    │   │   ├── bpfschema/       BPF map key/value structs (all 16 maps)
    │   │   ├── errors/          error codes (RFC 9114 §8 pattern)
    │   │   ├── sequence/        namespace sequence counters (RFC 9000)
    │   │   ├── amplification/   3× ring path limiter (RFC 9000)
    │   │   ├── migration/       flow migration tokens (RFC 9000 §9)
    │   │   ├── integrity/       HMAC-SHA256 per-flow auth
    │   │   ├── settings/        capability negotiation (RFC 9114)
    │   │   ├── tlv/             TLV encoding with greasing
    │   │   ├── flowtype/        flow type classification
    │   │   ├── lifecycle/       GOAWAY + cancel flow
    │   │   ├── prefetch/        explicit prefetch hints
    │   │   ├── intermediary/    hop validation + authority
    │   │   ├── dos/             backpressure management
    │   │   └── sophiasync/      dictionary synchronization
    │   ├── wotan-client/        message bus client
    │   ├── waf/                 firewall rules
    │   ├── network/             VXLAN, BGP, netlink
    │   ├── container/           container/VM lifecycle
    │   ├── nix/                 NixOS integration
    │   ├── telemetry/           Prometheus, tracing
    │   ├── health/              circuit breakers, retries
    │   ├── secrets/             key management
    │   ├── compliance/          policy enforcement
    │   └── ...                  (see pkg/ for full list)
    │
    ├── nix/                     NixOS definitions (optional, one supported runtime)
    │   ├── flake.nix            Nix flake entry point
    │   ├── containers/          per-service configs
    │   ├── modules/             shared modules
    │   └── packages/            custom packages
    │
    ├── docs/                    documentation
    │   ├── protocol/            Internet-Drafts, patches, error registry
    │   │   ├── references/      RFC crossref, IANA guide, wire patterns
    │   │   │   └── rfcs/        19 raw RFC texts (9000, 9114, 8200, etc.)
    │   │   └── patches/         draft-04/01/01 patch specifications
    │   ├── security/            Dark Grimoire addendum, LICH campaigns
    │   ├── sessions/            all session handoffs + battleplans (25)
    │   ├── adr/                 architecture decision records (12)
    │   ├── archive/             historical docs, old skill updates
    │   └── ...                  architecture, security, API specs
    │
    ├── scripts/                 deployment automation
    ├── references/              timeline (md, json, yaml triple-format)
    └── dashboard/               packet flow visualization UI

## Technology

    Go 1.24          services, control plane, CLI
    Rust             eBPF programs (aya), trace-collector
    Linux 5.15+      minimum kernel (BPF, XDP, TC)
    gRPC             primary inter-service transport (Wotan, port 18001)
    HTTP/3 + QUIC    fallback transport, external gateway
    eBPF (XDP/TC)    packet processing, tracing, chaos
    EVPN-VXLAN       L2 overlay fabric
    BGP              control plane routing
    Prometheus       metrics collection
    zerolog          structured JSON logging → Wotan log aggregation

Runtime-agnostic: runs on bare metal, LXD, Docker, Podman,
systemd-nspawn, Firecracker, QEMU/KVM, or any Linux environment
with BPF support.  NixOS is provided as one declarative option.

## Build

Requires: Linux (kernel 5.15+), Go 1.24+, Rust nightly,
root or sudo for eBPF loading.

    make build          # build all Go services
    make ebpf           # build eBPF programs (Rust)
    make containers     # build container images (NixOS default)
    make test           # run test suite
    make dev            # local development (docker-compose)

## Deploy

    # automated
    sudo ./scripts/deploy-alpha.sh

    # manual
    sudo ./scripts/setup-host.sh
    make build && make ebpf && make containers
    sudo ./scripts/load-ebpf.sh

## Protocol Documents

    docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md   Monad wire format (current)
    docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md     Sophia BPF dictionaries
    docs/protocol/draft-bellis-unheaded-wotan-memory-01.md          Wotan memory model
    docs/protocol/patches/                                          Next-draft patch specs
    docs/protocol/references/                                       RFC crossref, IANA guide
    docs/protocol/error-registry.md                                 Protocol error codes
    docs/security/dark-grimoire-addendum.md                         Attack surface taxonomy
    docs/security/lich-campaigns.md                                 Fuzzing campaign specs
    pkg/protocol/PATTERN_MATRIX.md                                  RFC→Package→BPF map matrix
    pkg/protocol/bpfschema/BPF_IPV6_INTERFACE_MAP.md                Top-down BPF/IPv6 mapping

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
    FIPS 203    ML-KEM (Kyber) Key Encapsulation
    FIPS 204    ML-DSA (Dilithium) Digital Signatures

## License

Pending license audit of all dependencies.  See [LICENSES/](LICENSES/).

## Contact

    Stevie Bellis <stevie@bellis.tech>
    https://unheaded.org
    https://github.com/unheaded
