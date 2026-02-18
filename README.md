# Unheaded

A mapped data bus over IPv6 Hop-by-Hop Options, with configuration
management automation for immutable infrastructure.

You bring the application ("the head").  Unheaded provides everything
else: packet-level observability, service mesh, control plane,
security baseline, and a protocol that turns every packet into
a 20-byte register file executing eBPF programs at wire speed.

## Status

Alpha.  Core protocol specified in
[draft-bellis-unheaded-protocol-foundation-02](docs/protocol/draft-bellis-unheaded-protocol-foundation-02.md)
(Internet-Draft format, targeting IETF Experimental).

## The Protocol

The Unheaded Protocol Foundation encodes a 20-byte Monad (5 × u32
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

Combined with the Monad, a /16 Kingdom carries 48 bytes of
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
        ├── dashboard-backend    metrics aggregator, WebSocket
        ├── kanban-app           self-hosting proof (the meta moment)
        ├── gateway              HTTP/3, QUIC, gRPC-Web, WebSocket
        └── yaldabaoth           chaos injection (controlled fault testing)

Network fabric: EVPN-VXLAN with BGP control plane.  All inter-node
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
    │   ├── kanban-app/          self-hosting proof (Go + JS)
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
    ├── pkg/                     shared Go packages (34 packages)
    │   ├── ebpf/                BPF loader, map management
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
    │   ├── protocol/            Internet-Draft, protocol specs
    │   ├── adr/                 architecture decision records
    │   └── ...                  architecture, security, handoffs
    │
    ├── scripts/                 deployment automation
    ├── references/              timeline (md, json, yaml triple-format)
    └── dashboard/               packet flow visualization UI

## Technology

    Go 1.24          services, control plane, CLI
    Rust             eBPF programs (aya), trace-collector
    Linux 5.15+      minimum kernel (BPF, XDP, TC)
    gRPC             inter-service transport (Wotan)
    eBPF (XDP/TC)    packet processing, tracing, chaos
    EVPN-VXLAN       L2 overlay fabric
    BGP              control plane routing
    HTTP/3 + QUIC    external gateway
    Prometheus       metrics collection

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

    docs/protocol/draft-bellis-unheaded-protocol-foundation-02.md   Internet-Draft (current)
    docs/protocol/PROTOCOL_FOUNDATION.md                            Vision and design rationale
    docs/protocol/PROTOCOL_MATH_AND_MAPS.md                        Byte maps, proofs, heritage
    docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md                     Extracted technical spec
    docs/protocol/the_first_packet.md                               The first packet walks the Pattern

## References

    RFC 8200    IPv6 Specification
    RFC 9673    IPv6 Hop-by-Hop Options Processing
    RFC 8799    Limited Domains and Internet Protocols
    RFC 9197    IOAM Data Fields
    RFC 1918    Address Allocation for Private Internets
    FIPS 203    ML-KEM (Kyber) Key Encapsulation
    FIPS 204    ML-DSA (Dilithium) Digital Signatures

## License

Pending license audit of all dependencies.  See [LICENSES/](LICENSES/).

## Contact

    Steven Bellis <stevenrbellis@gmail.com>
    https://unheaded.org
    https://github.com/unheaded
