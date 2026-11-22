# Project Structure

Top-level layout of the monorepo (`develop`). Binaries: [[Microservices]]. Rust crates: [[Rust Components|Rust-Components]].

## Code

| Dir | What |
|-----|------|
| `cmd/` | Every binary's entry point (Go, plus a few Rust/C) |
| `services/` | Service internals: wotan, timeguru, gateway, shield, sword, sabatons, yaldabaoth, anomaly, compliance, firewall, keymgmt, loadbalancer, nfv, qos, selfheal, version |
| `pkg/` | Shared Go packages: auth, audit, transport, discovery, logagg, health, ports, wotan-client, compliance, champion, gungnir, gjallarhorn, enkrateia, pqc_policy, … |
| `ebpf/` | Rust/Aya eBPF workspace (GPL-2.0-only): XDP/TC programs + shared types |
| `crates/` | Other Rust crates: monad-mbc, upc-api, upc-bootstub, upc-cartridge, xv6-mbc, heimdall-bpf, lich-fuzz, objtrust, svc-token, zhend ([[Rust Components|Rust-Components]]) |
| `arch/mbc/` | MBC architecture support for the UPC kernel |
| `drivers/` | UPC guest drivers (tty, block) |
| `cartridges/` | MBC cartridge sources (`.s` plus data and model files) for `cart` and `upc-cartridgectl` ([[UPC-Cartridges]]) |
| `doom/` | The UPC regression fixture only: `doom.mbc`, `doom.rv2mbc` and its sha256 (`scripts/upc-regression.sh`); Doom itself lives in `unheaded-doom` |
| `proto/`, `api/` | Protobuf definitions, `openapi.yaml`. The `.pb.go` files beside each `.proto` are generated (`protoc-gen-go`, `protoc-gen-go-grpc`; Wotan's by `services/wotan/proto/generate.sh`) |
| `dashboard/`, `kanban/` | Vanilla-JS front ends (ADR-006) |
| `raft/` | Zhen's Flask web UI (`zhen_app.py`, its login, redaction and scheduler client), its tests and the generated prompt module `scripts/zhen_prompt_generated.py` ([[Zhen-AI]]) |
| `modules/` | Optional modules (ADR-149): `modules/zhen/module.yaml` names the module's switch (`unheaded-zhen.target`), units and limits; its `prometheus/`, `victoria/` and `dashboard/` fragments are published while the switch is on ([[Systemd-Units]]) |
| `eval/` | Zhen coding-gate evaluation set |
| `tools/` | `mbc-asm`, `gosec` (pinned linter build) |

## Data and policy

| Dir | What |
|-----|------|
| `db/` | The Well: `migrations/` + `MANIFEST`, `converge/`, `init.sh`, `postgresql.conf`, `pg_hba.conf` ([[The-Well]]) |
| `compliance/` | Catalog, findings register, host profiles, remediation model, baselines (ADR-097–099) |
| `references/` | `timeline.md` + JSON/YAML/TOML mirrors, session logs, older battle plans |
| `configs/`, `config/` | Service configs (wotan, services, ports, akira) / third-party configs (grafana, victoria, vector, coredns, wireguard) |
| `observability/` | `alerting/policy.yaml`: the SLOs, tiers and routes `cmd/alert-policy` renders into `monitoring/prometheus/rules/slo.yml` and `monitoring/alertmanager/alertmanager.yml` ([[Alerting]]) |

## Deploy and run

| Dir | What |
|-----|------|
| `deploy/` | systemd units ([[Systemd-Units]]), `sysusers.d/` (one account per service), east offsite, ansible, argocd, k8s, tofu, nftables |
| `docker/`, `docker-compose*.yml` | Compose stack: services, hosts, telemetry, firewall, nginx |
| `containers/`, `lxd/`, `nix/`, `nixos/` | The four container runtimes + NixOS host config ([[Containers]]) |
| (moved) | K8s manifests and chart live in `unheaded-k8s` (DP-17) |
| `monitoring/` | Prometheus rules, Grafana dashboards, Loki, Alertmanager |
| `routing/` | BIRD, FRR, OSPF, IS-IS, MPLS configs |
| `debian/` | Packaging |
| `runbooks/` | YAML runbooks ([[Runbooks]]) |
| `systemd/` | Kingdom startup/shutdown units |

## Docs, tests, security

| Dir | What |
|-----|------|
| `docs/` | Source docs: `adr/`, `architecture/`, `protocol/`, `security/`, `battle-plans/`, … |
| `wiki/` | This wiki (source of truth; `scripts/sync-wiki.sh` publishes it) |
| `ietf-submission/` | Internet-Draft XML/MD for submission ([[Drafts-Index]]) |
| `tests/` | e2e, integration, unit, pqc, mbc-pipeline, runbooks, scripts, … |
| `tomb/` | LICH campaigns, Dark Mirror, grimoire ([[LICH-Campaigns]]) |
| `skills/` | Claude skill definitions ([[Skills-Index]]) |
| `scripts/` | Build, CI gates, Well tooling, sync scripts |
| `LICENSES/`, `sbom/`, `sbom-results/` | Licenses, third-party attribution, SBOM ([[SBOM]]) |
| `demos/`, `notebooks/` | Demo assets, experiment notebooks |
| `man/` | One man page per Go binary, generated from each binary's `--man` by `scripts/gen-man.sh --write` (`make man`); `scripts/gen-man.sh` alone fails when they are stale. Do not edit them |
| `security/` | `cve-fixes-2026-05-11.md`, a dated log of the gosec fixes from one 2026-05 lint drain. Current security docs are under `docs/security/` ([[Security]]) |
| `.github/` | CI workflows (`workflows/`, every action pinned to a SHA), Dependabot config, the CI Python lock (`ci-python/`), `BRANCH_PROTECTION.md` ([[Developer-Guide]]) |
| `.claude/` | Claude Code project settings (`settings.local.json`, `launch.json`) |

Root files: `README.md`, `QUICKSTART.md`, `VISION.md`, `CHANGELOG.md`, `CONTRIBUTING.md` and `CONTRIBUTOR-GUIDE.md`, `CODE_OF_CONDUCT.md`, `CLA.md`, `SECURITY.md` (reporting), `LICENSE` (GPL-3.0) and `LICENSE-PROTOCOLS`, `THIRD_PARTY.md`, `CLAUDE.md` (the development guide), `Makefile`, `Dockerfile` (multi-stage: one builder, one stage per first-party service image, and the dev-only all-in-one stage), `docker-compose.yml` (the dev stack), `services.yaml` (the bundled service manifest `pkg/discovery` falls back to), `flake.nix` (the Nix flake), `go.mod`/`go.sum`.

## Build

```bash
go build ./...                      # all Go
go test ./...                       # all Go tests
cd ebpf && cargo build --release    # eBPF workspace
make install-hooks                  # gofmt + go vet pre-commit hook
```

---

> **Source:** repo tree on `develop` · [docs/architecture/PROJECT_STRUCTURE.md](../docs/architecture/PROJECT_STRUCTURE.md) (2026-02, armor-name tour)
