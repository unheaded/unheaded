

# Unheaded

Configuration management automation platform. Provisions backend infrastructure — service mesh, observability, security, control plane — from declarative configuration. The Unheaded Protocol encodes state in IPv6 Hop-by-Hop headers, processed at each hop via eBPF.

## Building

Requires Linux (kernel 5.15+), Go 1.24+, Rust, Docker.

```bash
go build ./...
sudo docker compose up -d
```

See [QUICKSTART.md](QUICKSTART.md) for details.

## Architecture

```
Layer 5  Dashboard, Kanban, Zhenai Web UI
Layer 4  timeguru, captain, architect, micromanager, monad, sophia
Layer 3  Wotan (message bus), trace-collector, gateway, Akira (health)
Layer 2  unheaded-daemon (drift detection, reconciliation)
Layer 1  23 eBPF programs (XDP/TC, Rust/Aya, Go/cilium)
Layer 0  LXD / Docker / NixOS / bare metal
```

Ports 16666-26666. gRPC with mTLS default. See `pkg/ports/ports.go`.

## Protocol

Monad wire format: 20-byte register file in IPv6 HbH extension header. Wire format frozen at v0x01. 12 IANA registries in foundation spec draft-06.

- [draft-bellis-unheaded-protocol-foundation-06](docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md)
- [draft-bellis-unheaded-sophia-dictionary-03](docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md)
- [draft-bellis-unheaded-wotan-memory-03](docs/protocol/draft-bellis-unheaded-wotan-memory-03.md)
- [draft-bellis-unheaded-mbc-isa-00](docs/protocol/draft-bellis-unheaded-mbc-isa-00.md)
- [draft-bellis-unheaded-shim-00](docs/protocol/draft-bellis-unheaded-shim-00.md)
- [draft-bellis-unheaded-pqc-authentication-00](docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md)

## Stack

Go services, Rust eBPF (Aya), Wotan message bus (gRPC + HTTP), PostgreSQL, llama.cpp + Mistral-7B (ROCm), vanilla JS frontend, .deb packaging + systemd, SLH-DSA post-quantum crypto.

## Status

~385K production LOC. 34 services, 23 eBPF programs, 39 ADRs, 55 runbooks, 20 skills. Dual bare metal (WEST + EAST). Wire format frozen. See [CLAUDE.md](CLAUDE.md) for development guide.

## License

GPL-3.0-or-later. Protocol specs dual-licensed GPL-3.0-or-later / Apache-2.0.

## Author

Stevie Bellis — stevie@bellis.tech
