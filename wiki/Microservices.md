# Microservices

Every binary under `cmd/`, grouped by role. Ports are in [[Port Registry|Port-Registry]]; systemd units in [[Systemd Units|Systemd-Units]]. Library code for each service lives under `services/` and `pkg/`.

## Long-running services

| Binary | What it does | Wiki |
|--------|--------------|------|
| `wotan` (`services/wotan`) | Message bus, ring buffer, protocol RAM | [[Service-Wotan]] |
| `akira` | Health daemon: ping/pong sweep, percentage consensus, reports and bands on Wotan (dashboard-backend writes them to The Well) | [[Service-Akira]] |
| `kev-poller` | Keeps the CISA KEV catalog current (`:19120`): hourly conditional GET, validated snapshots in `/var/lib/unheaded/threat-feeds/cisa-kev`, deltas on Wotan `threats.cisa-kev.delta` | [[Service-KEV-Poller]] |
| `sword` | Deploys through Cuirass (`:19011`, compose profile `deploy`): rolling, blue-green, canary with analysis from instance metrics and the Kingdom SLO guard, rollback to the previous success; history in `SWORD_DATA_DIR` | [[Service-Sword]] |
| `dashboard-backend` | Metrics + WebSocket backend for the live dashboard (`:20000`) | [[Service-Dashboard-Backend]] |
| `kanban-app` | Task board backed by The Well | [[Service-Kanban-App]] |
| `protocol-api` | Monad encode/decode/validate, Sophia dictionary listing, Anamnesis events | [[Protocol-Specs]] |
| `unheaded-daemon` | Control plane: drift detection + reconciliation | [[Architecture]] |
| `shield` | Shield gateway daemon (Go): evaluates requests against its rule set, zero-trust ingress checks (`:19009`, loopback) | [[Security]] |
| `heimdall-daemon` | Userspace control for Mímir's Law / Gleipnir Phase 0 | [[Mimirs-Law]] |
| `routing-health` | Health endpoint for BIRD + established BGP sessions | [[Load-Balancers]] |
| `zhen-agentd` | Zhen agent daemon (ReAct loop + Champion gate) | [[Zhen-AI]] |
| `huginn` | Huginn, the host metrics agent (ADR-084) | [[Service-Huginn]] |
| `alert-mailer` | The only alert mail sender: pages at once, tickets as a daily digest, daily cap, dry-run mode (`:19130`) | [[Alerting]] |
| `wiki-server` | Renders this wiki (`:20002`): a host unit, `unheaded-wiki-server.service`, over TLS and behind the dashboard's login when an issuer is configured (2026-11-11). A link from a page into the rest of the repository goes to GitHub, and a link written `Page.md` opens the page (the files keep that form for GitHub's own view). With `--vor-url`, vor answers a search box (`/search`: wiki pages, vor sheets, and `docs/` files linked on GitHub; a search term must start a word, so `ccna` does not find `KRB5CCNAME`), `/sheets` (vor's sheets by category) and `/sheet/<category>/<name>` (a sheet's name is unique only within its category; `/sheet/<name>` still answers for a name only one sheet has); links between sheets open the sheet; without `--vor-url` none of them exist. `TestEveryWikiLinkResolves` fails on a wiki link that leads nowhere. The menu to the other apps comes from `/auth/nav.js` (`pkg/bff`), the same script every page loads | — |
| `unheaded-auth` | The built-in OIDC-shaped issuer: login, JWKS and discovery (`:19020`); `unheaded-auth user` manages the users file (ADR-103) | [[Service-Auth]] |
| `anomaly` | Runs the Anomaly Detection service (`services/anomaly`, loopback `:19018`) and bridges MBC cartridge scores into it (ADR-108) | [[UPC-Cartridges]] |
| `syslog-relay` | Receives syslog over UDP and TCP and writes each message to the local journal (ADR-124); socket-activated on 514 | [[Systemd-Units]] |
| `logstore-writer` | Inserts the journal rows Vector posts into the Well's `unheaded_ops.log_events`, capped (S-E1-LIVE Evening 16) | [[Log-Aggregation]] |
| `zhen-ui-front` | Terminates hybrid-only TLS 1.3 for the Zhen web UI and proxies to its Unix socket (ADR-138) | [[Zhen-AI]] |

Timeguru builds from `services/timeguru`: [[Service-Timeguru]]. architect, captain, micromanager and the monad and sophia services were removed in 2026-11 (ADR-147) and archived in `unheaded-role-services`.

## Observability and eBPF

| Binary | What it does |
|--------|--------------|
| `trace-collector-go` | Go BPF trace collector: packet-marker, flow-tracker and latency-probe events to Wotan's `traces.*` topics (`unheaded-trace-collector.service`; the Rust trace-collector was retired 2026-11-04) |
| `ebpf-collector` | Rust/Aya collector workspace (collector + programs) |
| `ebpf-loader` | Loads Unheaded eBPF programs (Rust) |
| `ebpf-load-check` | Runs every program in an ELF through the verifier (CI gate, ADR-012) |
| `demo-trace-injector` | Publishes synthetic trace events for demos |

See [[eBPF Programs|eBPF-Programs]] and [[Observability Backends|Observability-Backends]].

## Compliance and operations

| Binary | What it does | ADR |
|--------|--------------|-----|
| `unheaded-baseline` | Host baseline agent, audit mode only (timer + path units). Runs as `unheaded-baseline` with CAP_DAC_READ_SEARCH and CAP_NET_ADMIN, no mount sandbox (it must see the host), root-only facts from one `ExecStartPre=+` line | 098 |
| `unheaded-remediate` | Applies fixes from the remediation model; backup, undo, hash-chained journal | 099 |
| `compliance-evidence` | Collects evidence for the compliance catalog; `well sync` writes it to The Well | 097 / 098 |
| `yggdrasil-evidence` | Validates, signs and verifies Yggdrasil evidence | — |
| `well-migrate` | Applies `db/migrations/MANIFEST` and records each file in the ledger | 101 |
| `alert-policy` | Renders `observability/alerting/policy.yaml` into Prometheus SLO rules and Alertmanager routing; `--check` in CI | 100 |
| `cert-gen` | TLS certificates for the service mesh | 034 |
| `chaos-controller` | Runs `yaldabaoth.Service` (`:19012`, host unit as `unheaded-chaos` on west and east): allowlisted experiments, three abort paths, revert journal; `revert-all`, `render-polkit`, `packet-chaos` subcommands | 107, [[Chaos-Engineering]] |
| `steel-echo` | Game-day workload: `serve` with fault knobs, `load` availability meter, `monad-send`/`monad-count` for the packet lab; LXD images v1-v4 | [[Steel-Echo]] |
| `lich-security` | Offensive test suite: campaign modules D1–D6 against the stack | 062 |
| `perf-benchmark` | Performance benchmarking harness | — |
| `unheaded-aide` | Runs AIDE quietly: one line per check, a change reported once, the database updated only after a review (`unheaded-aide-check.timer`) | — |
| `unheaded-keygen` | Creates, rotates and retires service identity keys (Ed25519) and ML-DSA-65 signing keys | 103 |
| `well-retention` | Deletes or archives Well rows past their retention class (`compliance/retention/classes.yaml`); `-dry-run` only counts (`unheaded-well-retention.timer`) | [[The-Well]] |
| `cask-verify` | Verifies (and safely extracts) a signed Sealed Cask against trusted build keys | 010 |
| `crypto-inventory` | Builds and checks the cryptographic bill of materials, `compliance/crypto/cbom.cdx.json` | 105 |
| `vulnmatch` | Matches the components a tree pins against OSV records and the CISA KEV catalog | — |
| `depaudit` | Reports the latest-release age of every Go module dependency from `go list -m -json all` on stdin; `scripts/check-dep-staleness.sh` runs it | — |
| `healthprobe` | Container healthcheck: probes a service's `/health` by port or URL, exit 0 when healthy | — |
| `lich-mcp` | Serves the Lich red-team tools over MCP stdio behind the Champion gate, confined to local sandboxes | 062 |

## UPC

| Binary | What it does |
|--------|--------------|
| `upc-bootctl` | UPC kernel image loader + boot dispatcher (ASCEND-LINUX) |
| `upc-tty-bridge` | WebSocket bridge for UPC tty streams; same-origin WebSockets only, extra origins via `WS_ALLOWED_ORIGINS` |
| `gjallarhorn-sender` / `-listener` | Emit / receive UPC trigger packets (Mímir's Law) |
| `upc-cartridgectl` | Registers, signs, loads and swaps MBC cartridges on the pinned cartridge interpreter (`:26610` metrics, loopback; [[UPC-Cartridges]]) |

See [[UPC Overview|UPC-Overview]] and [[Doom on UPC|Doom-on-UPC]].

## CLIs and tools

| Binary | What it does |
|--------|--------------|
| `unheaded-cli` | The Gauntlets: the Kingdom CLI. Containers through Cuirass, service health through Akira, logs through dashboard-backend; commands nothing serves (secrets, network, start/stop/exec, deploy/restart/scale) fail instead of printing sample data |
| `wotan-ctl` | Wotan control CLI |
| `zhen-cli`, `zhen-agent`, `zhen-rag` | Zhen interactive CLI, one-shot agent, RAG glue |
| `zhen-claude` | ZhenAI's bridge to the Claude API (ADR-104): a one-shot prompt or a REPL, every tool call through the Champion gate |
| `zhen-mcp` | Serves the Kingdom tools to Claude Code over MCP stdio, every call through the Champion gate |
| `zhen-codeindex` | Builds Zhen's source shard (or a text corpus index) and searches it: word search, no network |
| `zhen-eval` | Runs the two-brains eval: the same tasks on the local model or on Claude, judged mechanically ([[Zhen-Routing]]) |
| `unheaded` | Placeholder `main` (prints "alive") |
| `tools/` | `anamnesis-lite`, `mimir`, `zhen-on-prem` |

---

> **Source:** `cmd/*/` package docs · [docs/architecture/MICROSERVICES.md](../docs/architecture/MICROSERVICES.md)
