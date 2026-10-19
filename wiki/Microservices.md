# Microservices

Every binary under `cmd/`, grouped by role. Ports are in [[Port Registry|Port-Registry]]; systemd units in [[Systemd Units|Systemd-Units]]. Library code for each service lives under `services/` and `pkg/`.

## Long-running services

| Binary | What it does | Wiki |
|--------|--------------|------|
| `wotan` (`services/wotan`) | Message bus, ring buffer, protocol RAM | [[Service-Wotan]] |
| `akira` | Health daemon: ping/pong sweep, percentage consensus, reports into The Well | [[Service-Akira]] |
| `kev-poller` | Keeps the CISA KEV catalog current (`:19120`): hourly conditional GET, validated snapshots in `/var/lib/unheaded/threat-feeds/cisa-kev`, deltas on Wotan `threats.cisa-kev.delta` | [[ADR-055-KEV-Poller-Always-on-Service]] |
| `dashboard-backend` | Metrics + WebSocket backend for the live dashboard (`:20000`) | [[Service-Dashboard-Backend]] |
| `kanban-app` | Task board backed by The Well | [[Service-Kanban-App]] |
| `monad` | HTTP API for the Monad state service | [[Protocol-Technical-Summary]] |
| `sophia` | Knowledge graph + microcode ROM API | [[Sophia-Dictionaries]] |
| `protocol-api` | Monad encode/decode/validate, Sophia dictionary listing, Anamnesis events | [[Protocol-Specs]] |
| `unheaded-daemon` | Control plane: drift detection + reconciliation | [[Architecture]] |
| `shield` | Shield WAF / Zero-Trust gateway daemon (Go) | [[Security]] |
| `waf` | Shield command-line entry point (Rust) | [[Security]] |
| `pqc-verifier` | Post-quantum signature verification service | [[PQC-Authentication]] |
| `heimdall-daemon` | Userspace control for Mímir's Law / Gleipnir Phase 0 | [[Mimirs-Law]] |
| `routing-health` | Health endpoint for BIRD + established BGP sessions | [[Load-Balancers]] |
| `zhen-agentd` | Zhen agent daemon (ReAct loop + Champion gate) | [[Zhen-AI]] |
| `host-agent` | Small bare-metal host metrics agent (Huginn is the live one) | [[Service-Huginn]] |
| `wiki-server` | Renders this wiki locally over HTTP | — |

Timeguru, Captain, Architect and Micromanager build from `services/<name>`: [[Service-Timeguru]], [[Service-Captain]], [[Service-Architect]], [[Service-Micromanager]].

## Observability and eBPF

| Binary | What it does |
|--------|--------------|
| `trace-collector` | Rust eBPF event collector ("the Whispering Void") |
| `trace-collector-go` | Go BPF trace collector |
| `ebpf-collector` | Rust/Aya collector workspace (collector + programs) |
| `ebpf-exporter` | eBPF maps → Prometheus metrics |
| `ebpf-loader` | Loads Unheaded eBPF programs (Rust) |
| `ebpf-load-check` | Runs every program in an ELF through the verifier (CI gate, ADR-012) |
| `demo-trace-injector` | Publishes synthetic trace events for demos |

See [[eBPF Programs|eBPF-Programs]] and [[Observability Backends|Observability-Backends]].

## Compliance and operations

| Binary | What it does | ADR |
|--------|--------------|-----|
| `unheaded-baseline` | Host baseline agent, audit mode only (timer + path units) | 098 |
| `unheaded-remediate` | Applies fixes from the remediation model; backup, undo, hash-chained journal | 099 |
| `compliance-evidence` | Collects evidence for the compliance catalog; `well sync` writes it to The Well | 097 / 098 |
| `yggdrasil-evidence` | Validates, signs and verifies Yggdrasil evidence | — |
| `well-migrate` | Applies `db/migrations/MANIFEST` and records each file in the ledger | 101 |
| `cert-gen` | TLS certificates for the service mesh | 034 |
| `chaos-controller` | Chaos engineering controller | — |
| `lich-security` | Offensive test suite: campaign modules D1–D6 against the stack | 062 |
| `perf-benchmark` | Performance benchmarking harness | — |

## UPC and Doom

| Binary | What it does |
|--------|--------------|
| `upc-bootctl` | UPC kernel image loader + boot dispatcher (ASCEND-LINUX) |
| `upc-tty-bridge` | WebSocket bridge for UPC tty streams |
| `gjallarhorn-sender` / `-listener` | Emit / receive UPC trigger packets (Mímir's Law) |
| `doom`, `doom-loader`, `doom-bridge`, `doom-cpu-dump` | Doom engine CLI, ROM/RAM loader, BPF map access, CPU state dump |
| `doom-go-injector`, `doom-injector-c` | Packet injectors (Go, raw AF_PACKET C) |
| `test_batch` | Scratch tool: batch-read the Doom screen map |

See [[UPC Overview|UPC-Overview]] and [[Doom on UPC|Doom-on-UPC]].

## CLIs and tools

| Binary | What it does |
|--------|--------------|
| `unheaded-cli` | The Gauntlets: the Kingdom CLI |
| `wotan-ctl` | Wotan control CLI |
| `zhen-cli`, `zhen-agent`, `zhen-rag` | Zhen interactive CLI, one-shot agent, RAG glue |
| `unheaded` | Placeholder `main` (prints "alive") |
| `tools/` | `anamnesis-lite`, `mimir`, `zhen-on-prem` |

---

> **Source:** `cmd/*/` package docs (2026-10-16) · [docs/architecture/MICROSERVICES.md](../docs/architecture/MICROSERVICES.md)
