# Unheaded

**Production-ready infrastructure in hours, not months.**

Unheaded is a configuration management automation platform delivering complete infrastructure for modern SaaS applications. You bring your app ("the head"), we provide everything else.

## What It Does

- **eBPF-based observability** from Layer 2 to Layer 7 -- every packet traced from wire to browser
- **Immutable infrastructure** across LXD, containerd, NixOS, and Docker
- **Zero user data access** -- architectural isolation enforced, not promised
- **Service mesh** on the Wotan message bus (gRPC streaming, pub/sub, protocol RAM)
- **Declarative everything** -- version-controlled configs, interchangeable IaC backends
- **Self-hosting proof** -- Unheaded builds and tracks itself (the Meta Moment)
- **Post-quantum cryptography** -- ML-DSA, ML-KEM, SLH-DSA (FIPS 203/204/205)

## The Unheaded Protocol Computer

The Monad wire format is not just metadata -- it is a computational substrate.

Every IPv6 packet carries a 20-byte Monad register file (5 x u32) in the Hop-by-Hop Options header. At each hop, an eBPF program reads and writes the registers. The packet *is* the working memory of a distributed computation.

We built a complete virtual CPU inside eBPF XDP that boots Linux.

See the [UPC Reference Manual](docs/UPC_REFERENCE_MANUAL.md) for the full architecture.

## Architecture

```
Layer 5  User Interface       Dashboard, Kanban (self-hosting proof)
Layer 4  Application Services timeguru, captain, micromanager, architect
Layer 3  Infrastructure Svcs  wotan, trace-collector, gateway
Layer 2  Control Plane        unheaded-daemon (drift detection, reconciliation)
Layer 1  Data Plane           8 eBPF programs (XDP/TC, Rust/Aya)
Layer 0  Infrastructure       LXD / Docker / NixOS / bare metal
```

Two bare-metal hosts (WEST + EAST) connected via WireGuard IPv6 overlay (`fd00:dead:beef::/48`) with BGP EVPN underlay (VXLAN VNIs). Cross-host BPF flow graph operational.

## Quick Start

```bash
# Prerequisites: Linux (kernel 5.15+), Go 1.24+, Docker

# Build
go build ./...

# Run with Docker Compose
docker compose up -d

# Verify all services
for port in 17000 18000 19000 19001 19002 19003 19004 19005 20000 20001; do
  curl -sf "http://localhost:$port/health" && echo " :$port OK" || echo " :$port UNREACHABLE"
done

# Open the dashboard
xdg-open http://localhost:20000
```

See [QUICKSTART.md](QUICKSTART.md) for the full walkthrough.

## Services

| Service | Port | Role |
|---------|------|------|
| unheaded-daemon | 17000 | Control plane, drift detection |
| wotan | 18000/18001 | Message bus (HTTP/gRPC) |
| timeguru | 19000 | Timeline tracking |
| architect | 19001 | Infrastructure design |
| captain | 19002 | Strategy service |
| micromanager | 19003 | Task execution |
| monad | 19004 | Register file processor |
| sophia | 19005 | BPF dictionary management |
| dashboard | 20000 | Metrics, packet-flow visualization |
| kanban | 20001 | Self-hosting proof (the Meta Moment) |

## The Protocol

The Monad encodes 20 bytes (5 registers, CRC-16/CCITT) in IPv6 Hop-by-Hop Options. With Kingdom Mode (EVPN-VXLAN), deterministic address bits are reclaimed as extended register space -- up to 48 bytes of computational state per packet with zero wire overhead.

Core specification in three Internet-Drafts (IETF Experimental track):

- [Monad Wire Format](docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md) -- register file encoding
- [Sophia Dictionaries](docs/protocol/draft-bellis-unheaded-sophia-dictionary-01.md) -- BPF map management
- [Wotan Memory Model](docs/protocol/draft-bellis-unheaded-wotan-memory-01.md) -- distributed protocol RAM

## Security

- **Post-quantum cryptography**: ML-KEM-768, ML-DSA-65, SLH-DSA (FIPS 203/204/205) via cloudflare/circl
- **4-tier compliance**: NONE, STANDARD, HARDENED, SOVEREIGN
- **eBPF tracing from packet zero**: every packet marked at XDP, correlated end-to-end
- **Container hardening**: seccomp, capabilities, read-only filesystems, default-deny networking
- **IDS**: Suricata with custom Monad rules (SID 9000001-9000099), GPL-2.0 process-isolated
- **Authentication**: Pluggable (Noop/APIKey/JWT), RBAC, audit logging

## AI: Zhen

Local RAG system with 1.52M indexed knowledge chunks. Mistral-7B inference via llama.cpp. No data leaves the network.

- **Web UI**: port 20103
- **Inference API**: port 20100
- **Capabilities**: Codebase search, protocol Q&A, architecture exploration

## Technology

| Component | Stack |
|-----------|-------|
| Services | Go 1.24 |
| eBPF programs | Rust (Aya framework) |
| Frontend | Vanilla JS |
| Transport | gRPC-first, HTTP fallback |
| Routing | BGP EVPN (default), OSPFv3, IS-IS+SR-MPLS, MPLS LDP |
| Tunnel | WireGuard (fd00:dead:beef::/48) |
| Observability | Prometheus, Loki, VictoriaMetrics, Grafana |
| IaC backends | Ansible, Terraform, Puppet, Kubernetes, Chef, Salt |

## Codebase

~450K lines of production code. ~1M+ total with tests and documentation.

- 25 services, 37+ packages, 8 eBPF programs
- 16 protocol packages (Go), 16K lines eBPF (Rust/Aya)
- Three deployment platforms: NixOS, Docker Compose, LXD
- Four routing options: BGP EVPN, OSPFv3, IS-IS+SR-MPLS, MPLS LDP

## License

GPL-3.0-or-later. See [LICENSE](./LICENSE).

Protocol specifications dual-licensed GPL-3.0/Apache-2.0 for ecosystem adoption.

Suricata (GPL-2.0) is process-isolated. See [GPL Isolation Boundary](docs/legal/SURICATA_GPL_ISOLATION.md).

## Author

Stevie Bellis -- stevie@bellis.tech
