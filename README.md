# Unheaded

https://github.com/user-attachments/assets/59049105-a48c-40e1-a6fc-051649e6c59c

Unheaded is a configuration-management automation platform. It provisions backend
infrastructure — service mesh, observability, security, control plane — from declarative
configuration. State is encoded in a 20-byte register carried in IPv6 Hop-by-Hop headers
(the Monad wire format) and processed at each hop with eBPF.

The repository also contains the Unheaded Protocol Computer (UPC), a virtual CPU built on
the protocol, on which Doom and a Unix kernel run.

Solo, experimental, in active development.

## Building

Requires Linux (kernel 5.15+; 6.17+ for the UPC eBPF verifier features), Go 1.25+,
Rust nightly, Docker.

```
go build ./...
sudo docker compose up -d
```

See [QUICKSTART.md](QUICKSTART.md).

## Components

```
Layer 5  Dashboard, Kanban, Zhenai web UI
Layer 4  timeguru, captain, architect, micromanager, monad, sophia
Layer 3  Wotan (message bus), trace-collector, gateway, Akira (health)
Layer 2  unheaded-daemon (drift detection, reconciliation)
Layer 1  23 eBPF programs (XDP/TC; Rust/Aya, Go/cilium)
Layer 0  LXD / Docker / NixOS / bare metal
```

Services use ports 16666–26666, gRPC with mTLS by default (`pkg/ports/ports.go`). Two
bare-metal hosts (WEST, EAST) run a cross-host BPF flow graph.

## Protocol

Monad: a 20-byte register file in an IPv6 Hop-by-Hop extension header, frozen at v0x01.

- [foundation-06](docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md)
- [sophia-dictionary-03](docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md)
- [wotan-memory-03](docs/protocol/draft-bellis-unheaded-wotan-memory-03.md)
- [mbc-isa-00](docs/protocol/draft-bellis-unheaded-mbc-isa-00.md)
- [shim-00](docs/protocol/draft-bellis-unheaded-shim-00.md)
- [pqc-authentication-00](docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md)

## UPC

A virtual CPU built on the protocol: Monad as the transport bus, Wotan as memory, Sophia
as microcode, eBPF as the interpreter. One interpreter runs multiple guest workloads,
feature-partitioned to fit the eBPF verifier's 1M-instruction budget.

- **Doom** — runs; playable in a browser via `doom-runner`.
- **xv6** — runs; interactive shell, fork/exec/wait, per-pid MMU isolation, in-BPF
  filesystem reader (`ls`/`cat`/`echo`/`wc` over `fs.img`).
- **Unheaded Linux** — a from-scratch minimal OS, evolving from the xv6 substrate.
  In development.

Code: `ebpf/monad-cpu-ebpf/` (interpreter), `crates/xv6-mbc/`, `crates/doom-runner/`,
`cmd/upc-bootctl/`. Docs: [`wiki/UPC-Overview.md`](wiki/UPC-Overview.md),
[`wiki/Linux-on-UPC.md`](wiki/Linux-on-UPC.md), [`wiki/Doom-on-UPC.md`](wiki/Doom-on-UPC.md),
[`wiki/MBC-ISA-Reference.md`](wiki/MBC-ISA-Reference.md).

## Stack

Go services, Rust eBPF (Aya), Wotan message bus (gRPC + HTTP), PostgreSQL (multi-DB "The
Well"), llama.cpp + Mistral-7B (ROCm), vanilla-JS frontend, `.deb` packaging + systemd,
SLH-DSA / ML-DSA-65 post-quantum crypto, deterministic Sealed Cask builds.

## Zhen

`crates/zhend/` — anti-fragile gossip knowledge substrate, PQC-secured. `crates/zhenai-forge/`
— LoRA fine-tuning on Gemma-4 / Mistral-7B via ROCm. Flask web UI on port 20103 over a
1.52M-vector RAG corpus.

## Security

Every push and pull request to `develop`, `staging` and `main` runs these gates
(`.github/workflows/`; most are `scripts/check-*.sh` scripts that also run locally):

- gitleaks over the working tree, with a baseline (`.gitleaksignore`) that may only shrink;
  Trivy secret, vulnerability and IaC scans.
- gosec with a ratchet that may only shrink; govulncheck; cargo-audit; pip-audit.
- A third-party dependency baseline that may only shrink (`docs/policy/dependency-baseline.txt`).
- An SBOM per build, scanned with grype; a GPL boundary check.
- Pinned CI tools checked against upstream releases after a 7-day cooldown; GitHub Actions
  pinned to commit SHAs.
- Runbook schema and dry-run tests; eBPF programs held to verifier instruction budgets.

Open findings, their severity, owner and due date live in the findings register,
[`compliance/findings/register.yaml`](compliance/findings/register.yaml)
([ADR-098](docs/adr/ADR-098-findings-register-and-baseline-modes.md)).

To report a vulnerability, follow [SECURITY.md](SECURITY.md). The same contact is served
at `/.well-known/security.txt` (RFC 9116) by the dashboard backend.

## License

GPL-3.0-or-later. Protocol specs dual-licensed GPL-3.0-or-later / Apache-2.0.

Export control: this repository publishes encryption source code (ECCN 5D002) under the
EAR's publicly-available provisions; see the [export-control notice](docs/compliance/export-control-notice.md).

## Author

Stevie Bellis — stevie@bellis.tech
