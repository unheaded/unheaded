# Project Structure

Top-level layout of the monorepo (`develop`, 2026-10-16). Binaries: [[Microservices]]. Rust crates: [[Rust Components|Rust-Components]].

## Code

| Dir | What |
|-----|------|
| `cmd/` | Every binary's entry point (Go, plus a few Rust/C) |
| `services/` | Service internals: wotan, timeguru, captain, architect, micromanager, monad, sophia, anamnesis, shield, plus the armor-named components (cuirass, gauntlets, vambraces…) |
| `pkg/` | Shared Go packages: auth, audit, transport, discovery, logagg, health, ports, wotan-client, compliance, champion, gungnir, gjallarhorn, enkrateia, pqc_policy, … |
| `internal/` | Repo-private Go helpers (bpfmap, doom, dashboard) |
| `ebpf/` | Rust/Aya eBPF workspace (GPL-2.0-only): XDP/TC programs + shared types |
| `crates/` | Other Rust crates: monad-mbc, upc-api, upc-bootstub, xv6-mbc, doom-runner, heimdall-bpf, lich-fuzz, zhend, zhenai-forge |
| `arch/mbc/` | MBC architecture support for the UPC kernel |
| `drivers/` | UPC guest drivers (tty, block) |
| `proto/`, `api/` | Protobuf definitions, `openapi.yaml` |
| `dashboard/`, `kanban/` | Vanilla-JS front ends (ADR-006) |
| `raft/` | Zhen RAFT training scripts, MCP server, scheduler |
| `eval/` | Zhen coding-gate evaluation set |
| `tools/` | `mbc-asm` |

## Data and policy

| Dir | What |
|-----|------|
| `db/` | The Well: `migrations/` + `MANIFEST`, `converge/`, `init.sh`, `postgresql.conf`, `pg_hba.conf` ([[The-Well]]) |
| `compliance/` | Catalog, findings register, host profiles, remediation model, baselines (ADR-097–099) |
| `references/` | `timeline.md` + JSON/YAML/TOML mirrors, session logs, older battle plans |
| `configs/`, `config/` | Service configs (wotan, services, ports, akira) / third-party configs (grafana, victoria, vector, coredns, wireguard) |

## Deploy and run

| Dir | What |
|-----|------|
| `deploy/` | systemd units ([[Systemd-Units]]), east offsite, ansible, argocd, k8s, tofu, nftables |
| `docker/`, `docker-compose*.yml` | Compose stack: services, hosts, telemetry, firewall, nginx |
| `containers/`, `lxd/`, `nix/`, `nixos/` | The four container runtimes + NixOS host config ([[Containers]]) |
| `kubernetes/`, `helm/` | K8s manifests and chart |
| `monitoring/` | Prometheus rules, Grafana dashboards, Loki, Alertmanager |
| `routing/` | BIRD, FRR, OSPF, IS-IS, MPLS, Suricata configs |
| `debian/`, `jenkins/` | Packaging, Jenkins pipeline |
| `runbooks/` | YAML runbooks ([[Runbooks]]) |
| `systemd/` | Kingdom startup/shutdown units |

## Docs, tests, security

| Dir | What |
|-----|------|
| `docs/` | Source docs: `adr/`, `architecture/`, `protocol/`, `security/`, `battle-plans/`, `doom/`, … |
| `wiki/` | This wiki (source of truth; `scripts/sync-wiki.sh` publishes it) |
| `ietf-submission/` | Internet-Draft XML/MD for submission ([[Drafts-Index]]) |
| `tests/` | e2e, integration, security, pqc, mbc-pipeline, doom-reference |
| `tomb/` | LICH campaigns, Dark Mirror, grimoire ([[LICH-Campaigns]]) |
| `skills/` | Claude skill definitions ([[Skills-Index]]) |
| `scripts/` | Build, CI gates, Well tooling, sync scripts |
| `LICENSES/`, `sbom/`, `sbom-results/` | Licenses, third-party attribution, SBOM ([[SBOM]]) |
| `demos/`, `notebooks/` | Demo assets, experiment notebooks |

## Build

```bash
go build ./...                      # all Go
go test ./...                       # all Go tests
cd ebpf && cargo build --release    # eBPF workspace
make install-hooks                  # gofmt + go vet pre-commit hook
```

---

> **Source:** repo tree on `develop` · [docs/architecture/PROJECT_STRUCTURE.md](../docs/architecture/PROJECT_STRUCTURE.md) (2026-02, armor-name tour)
