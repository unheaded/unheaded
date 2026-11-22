<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# Documentation coverage (2026-11-22)

Stevie, 2026-11-22: "librarian should make sure every package, derive, and part of unheaded is documented". This file is the gap list for that request; a dated section appended at the end records what the fill changed and what is still missing.

## How this was made

1. Tree: branch `docs/doc-coverage` from `develop` at `8f3825e0`. Read-only pass: grep, git and a Python reader over the files; nothing was built, run or started (a host measurement held the quiet flag).
2. Made by an AI agent (Claude) acting as the Librarian. The reader is not committed: it parses Go package clauses, Cargo manifests and the `wiki/` pages.
3. **Go**: a package is documented when a non-test file has a comment directly above `package` that is more than the SPDX/copyright header. Test-only directories are judged on their test files. "Weak" marks a doc comment that `go doc` shows badly: the SPDX line glued to it (`spdx-merged`), or a comment about one file sitting directly above the package clause (`file-comment`), which `go doc` joins into the package doc.
4. **Rust**: a crate root (`src/lib.rs`, `src/main.rs`, a `[[bin]]`/`[lib]` path, or each fuzz target) is documented when it has a `//!` line.
5. **Wiki**: a part counts as named when a page in `wiki/` (the source of truth; `CLAUDE.md` and `README.md` there excluded) has its path, or its name as a whole word (names of four letters or fewer and generic names like `common` count only by path). Units also count by stem (`unheaded-foo` for `unheaded-foo.timer`). A service under `services/` also counts when Project-Structure's `services/` row names it. A top-level directory counts only when `wiki/Project-Structure.md` has a row for it.
6. **Scripts**: operator scripts and CI gate scripts (`check-*.sh`) are in scope. Tests (`test-*`, `*_test.go`, `test_*.py`), libraries (`scripts/lib/`, `lib*.sh`) and data files (`.sql`, `.txt`, `.md`) are counted but out of scope: nobody runs them as a tool.
7. **"derive"** is read as derived and generated files: anything a tool writes from a source in the tree (rules, Alertmanager config, man pages, manifests, catalogs, mirrors, protobuf code, baselines written by a gate's `--update`). Each should say, in the file or on the wiki, what writes it and from what. Lock files (`go.sum`, `Cargo.lock`) are out of scope.

## Counts at the start

| Kind | Parts | Documented | Missing |
|---|---|---|---|
| go | 255 | 225 | 30 |
| rust-doc | 40 | 36 | 4 |
| rust-wiki | 40 | 33 | 7 |
| top-dirs | 50 | 42 | 8 |
| cmd | 60 | 57 | 3 |
| services | 16 | 16 | 0 |
| modules | 1 | 1 | 0 |
| units | 92 | 77 | 15 |
| scripts | 236 | 111 | 125 |

`go` counts packages with a doc comment (weak ones included); `rust-doc` and `rust-wiki` count crates (workspace roots excluded); `top-dirs` counts Project-Structure rows; the others count wiki coverage.

## Generated and derived files

| File(s) | Written by | From | Says so in the file | Wiki | Action |
|---|---|---|---|---|---|
| `monitoring/alertmanager/alertmanager.yml` | `go run ./cmd/alert-policy` | `observability/alerting/policy.yaml` | yes | Alerting | none needed |
| `monitoring/prometheus/rules/slo.yml` | `go run ./cmd/alert-policy` | `observability/alerting/policy.yaml` | yes | Alerting | none needed |
| `monitoring/prometheus/rule-series.txt` | `scripts/gen-prom-keep-list.py` | `monitoring/prometheus/rules/*.yml` | yes | Alerting | none needed |
| `compliance/catalog/frameworks/*.yaml`, `compliance/catalog/references/nist-csf2-informative.yaml` | `scripts/compliance/import-frameworks.py` | the publishers' machine-readable releases (URL, time, sha256 in each header) | yes | none | add to Compliance-Paperwork |
| `compliance/baseline/baseline.yaml` | `go run ./cmd/compliance-evidence baseline` | `compliance/catalog/controls.yaml` | yes | none | add to Compliance-Paperwork |
| `deploy/ansible/compliance/**` | `go run ./cmd/compliance-evidence ansible` | `compliance/remediation/fixes.yaml` | yes | Security | none needed |
| `configs/zhen-routing.yaml` | `scripts/two-brains/thresholds.py` | the tune half of `eval/two-brains/results-*.jsonl` | yes | Zhen-Routing | none needed |
| `debian/control` | `scripts/build-all-debs.sh --control` | `scripts/lib/deb-packages.sh` | yes | Releases | none needed |
| `raft/scripts/zhen_prompt_generated.py` | `go generate ./pkg/zhenrag/...` (`pkg/zhenrag/gen`) | `pkg/zhenrag` | yes | Zhen-AI | none needed |
| `runbooks/MANIFEST.sha256` | `scripts/runbook-manifest.sh --write` | `runbooks/**/*.yaml` | no (sha256sum format) | Zhen-AI | add to Runbooks |
| `man/*.1` | `scripts/gen-man.sh --write` (`make man`) | each Go binary's hidden `--man` (`pkg/cmdline`) | no (roff, overwritten) | none | add to Developer-Guide |
| `compliance/crypto/cbom.cdx.json` | `go run ./cmd/crypto-inventory scan`; checked by `scripts/check-crypto-inventory.sh` | the tree | no (JSON) | Post-Quantum-Key-Exchange | none needed |
| `references/timeline.{json,yaml,toml}` | timeguru, `POST /api/v1/timeline/sync` (`services/timeguru/internal/sync`) | `references/timeline.md` | no (JSON has no comments) | Service-Timeguru says "the mirrors", not the files | add file names to Timeline |
| `proto/unheaded/v1/*.pb.go` | `protoc-gen-go` v1.36.11, protoc 3.21.12 (per header) | `proto/unheaded/v1/protocol.proto` | yes | none | add to Project-Structure; the command is not in the tree |
| `services/wotan/proto/*.pb.go` | `services/wotan/proto/generate.sh` | `services/wotan/proto/*.proto` | yes | Wotan-Topic-Signing (pb.go only) | add to Service-Wotan |
| zhend gRPC code (`OUT_DIR`, not committed) | `crates/zhend/build.rs` (tonic-build) | `crates/zhend/proto/zhen.proto` | yes (`build.rs`, `src/api/grpc.rs`) | none | none needed: not a tracked file |
| `docs/LIVE-PATHS.md` | `scripts/live-path-inventory.sh --report` | `go list` over the tree | yes | none | name the script on Developer-Guide |
| `docs/UNREACHED-GO-PACKAGES.md` | the command in its own text | `go list` | yes | Sophia-Dictionaries (link only) | none needed |
| `sbom/*.json`, `sbom/SBOM.md` | `scripts/generate-sbom.sh` | lock data and installed licences | partly (`Generated:` date) | SBOM | none needed |
| `docs/policy/*-baseline.*`, `*-over-budget.txt`, `*-known-failing.txt`, `*-pins.txt` | each gate's `--update` (e.g. `check-netip.sh --update`) | the gate's own count | yes, header lines, except `ebpf-insns-baseline.tsv` | eBPF-Programs, Security, Post-Quantum-Key-Exchange for some | name the rest with their gates on Developer-Guide |
| `wiki/ADR-0*.md` stubs | one Marshal sweep, 2026-05-06; edited by hand since | `docs/adr/` | yes | ADR-Index | none needed |
| `sbom-results/`, `LICENSES/sbom-reports/` | S37-era SBOM runs (2026-02) | lock data then | yes (`sbom-results/SBOM-SUMMARY.md`: superseded by `sbom/SBOM.md`) | SBOM | none needed |

## Config files a person edits

Keys described in the file (comments) or on a wiki page. Grafana dashboard JSON is exported from Grafana, not edited by hand.

| File | Comment lines / lines | Wiki | Action |
|---|---|---|---|
| `observability/alerting/policy.yaml` | 110 / 408 | Alerting (+5) | none needed |
| `configs/wotan.yaml` | 114 / 228 | 7 pages | none needed |
| `configs/zhen-schedules.yaml`, `configs/zhen-checks.yaml`, `configs/chaos/allowlist.yaml`, `configs/health-sweep-west.targets`, `configs/lich-mcp.yaml`, `configs/lich-ctf-eligible.yaml`, `configs/networking/sysctl-tuning.yaml`, `configs/akira-*.yaml` | commented | most named | none needed |
| `configs/services.yaml` | 4 / 64 | 2 pages | none needed |
| `configs/port-registry.yaml` | 4 / 31 | none | read; describe on Port-Registry |
| `config/akira/akira.yaml`, `config/kev-poller/stack.yaml`, `config/vector/*.yaml`, `config/victoria/scrape.yml`, `config/wireguard/*.conf`, `config/coredns/Corefile`, `config/traefik/dynamic.yml`, `config/dashboard/services.txt`, `config/monitoring/expectations.yaml`, `config/grafana/provisioning/*.yml` | commented | some | none needed |
| `config/clickhouse/backups.xml`, `config/clickhouse/logger.xml`, `config/clickhouse/schema/001-logs.sql` | 0 | none | add a line on Log-Aggregation |
| `config/supervisor/all-in-one.ini` | 0 / 53 | none | read; describe beside its siblings |
| `config/grafana/provisioning/dashboards/kingdom/*.json` | n/a (exported) | 08 only | add one line on Observability-Backends |

## Parts, by kind

### Go packages

| Package | Name | Doc | Wiki | Action |
|---|---|---|---|---|
| `arch/mbc/boot` | main | yes | ADR-067-MBC-ISA-V2-and-UPC-ABI-V1, ADR-Index (+16) | none needed |
| `arch/mbc/boot/init` | main | yes | Doom-on-UPC, Home (+9) | none needed |
| `arch/mbc/userspace/cat` | main | yes | none | none needed |
| `arch/mbc/userspace/echo` | main | yes | Home, Linux-on-UPC (+6) | none needed |
| `arch/mbc/userspace/ls` | main | yes | none | none needed |
| `arch/mbc/userspace/ps` | main | yes | none | none needed |
| `arch/mbc/userspace/uname` | main | yes | none | none needed |
| `arch/mbc/userspace/uptime` | main | yes | ADR-Index, UPC-Dream-Ladder | none needed |
| `cmd/akira` | main | no | Architecture, Microservices (+7) | add package doc |
| `cmd/alert-mailer` | main | yes | ADR-Index, Alerting (+9) | none needed |
| `cmd/alert-policy` | main | yes | Alerting, Microservices (+1) | none needed |
| `cmd/anomaly` | main | yes | Port-Registry, Project-Structure (+5) | none needed |
| `cmd/cask-verify` | main | yes | ADR-010-Sealed-Cask-Deployment | none needed |
| `cmd/cert-gen` | main | yes | Microservices, Port-Registry | none needed |
| `cmd/chaos-controller` | main | yes | ADR-Index, Chaos-Engineering (+9) | none needed |
| `cmd/compliance-evidence` | main | yes | ADR-Index, Alerting (+4) | none needed |
| `cmd/crypto-inventory` | main | yes | Post-Quantum-Key-Exchange | none needed |
| `cmd/dashboard-backend` | main | yes | ADR-Index, Alerting (+18) | none needed |
| `cmd/dashboard-backend/internal/ebpf` | ebpf | yes | none | none needed |
| `cmd/dashboard-backend/internal/events` | events | yes | Architecture, Bug-Kill-Chain (+20) | none needed |
| `cmd/dashboard-backend/internal/health` | health | yes | ADR-Index, Architecture (+29) | none needed |
| `cmd/dashboard-backend/internal/logs` | logs | yes | ADR-Index, Alerting (+26) | none needed |
| `cmd/dashboard-backend/internal/server` | server | yes | none | none needed |
| `cmd/dashboard-backend/internal/verdicts` | verdicts | yes | ADR-Index, Service-Dashboard-Backend (+1) | none needed |
| `cmd/dashboard-backend/internal/websocket` | websocket | yes | none | none needed |
| `cmd/dashboard-backend/jstest` | test-only | yes | none | none needed |
| `cmd/demo-trace-injector` | main | yes | Microservices, Service-Auth | fix: spdx-merged:main.go |
| `cmd/depaudit` | main | yes | none | none needed |
| `cmd/gjallarhorn-listener` | main | yes | none | none needed |
| `cmd/gjallarhorn-sender` | main | yes | Microservices, Mimirs-Law | none needed |
| `cmd/healthprobe` | main | yes | Service-Akira | none needed |
| `cmd/heimdall-daemon` | main | yes | Alerting, Microservices (+4) | fix: file-comment:bpf.go; file-comment:bpf_linux.go |
| `cmd/huginn` | main | yes | ADR-Index, Alerting (+10) | fix: spdx-merged:main.go |
| `cmd/kanban-app` | main | yes | Architecture, Demo-Script (+12) | none needed |
| `cmd/kev-poller` | main | yes | ADR-Index, Home (+7) | none needed |
| `cmd/lich-mcp` | main | yes | Zhen-AI | none needed |
| `cmd/lich-security` | main | yes | LICH-Campaigns, Microservices | none needed |
| `cmd/lich-security/campaigns` | campaigns | yes | ADR-062-Fuzz-Redteam-Pentest-Framework, ADR-063-Akira-Summons-LICH-Randomized-CTF (+5) | none needed |
| `cmd/logstore-writer` | main | yes | Log-Aggregation, Port-Registry | none needed |
| `cmd/perf-benchmark` | main | yes | Benchmarks, Home (+1) | none needed |
| `cmd/protocol-api` | main | no | Microservices, Port-Registry (+1) | add package doc |
| `cmd/routing-health` | main | no | Microservices | add package doc |
| `cmd/shield` | main | yes | Microservices, Port-Registry (+2) | none needed |
| `cmd/steel-echo` | main | yes | Chaos-Engineering, Containers (+6) | none needed |
| `cmd/sword` | main | yes | Alerting, Game-Days (+8) | none needed |
| `cmd/syslog-relay` | main | yes | ADR-Index, Port-Registry | none needed |
| `cmd/trace-collector-go` | main | yes | Demo-Script, Log-Aggregation (+4) | fix: file-comment:correlator.go; file-comment:enrichment.go; file-comment:flow_reader.go; file-comment:flow_stats.go; file-comment:health.go; file-comment:latency_kprobe.go; file-comment:latency_reader.go; file-comment:latency_stats.go; file-comment:latency_sweep.go; file-comment:loader.go; file-comment:loader_native.go; file-comment:loader_native_stub.go; file-comment:map_walk.go; file-comment:publisher.go; file-comment:reader.go; file-comment:types.go |
| `cmd/unheaded` | main | no | ADR-022-Pihole-Docker-to-LXD, Alerting (+19) | add package doc |
| `cmd/unheaded-aide` | main | yes | Security, Systemd-Units | none needed |
| `cmd/unheaded-auth` | main | yes | ADR-Index, Port-Registry (+5) | none needed |
| `cmd/unheaded-baseline` | main | yes | ADR-Index, Compliance-Paperwork (+3) | none needed |
| `cmd/unheaded-cli` | main | yes | IaC-Backends, Kingdom-Architecture (+6) | none needed |
| `cmd/unheaded-cli/cmd` | cmd | yes | IaC-Backends, Observability-Backends | none needed |
| `cmd/unheaded-cli/output` | output | yes | ADR-Index, Architecture (+18) | none needed |
| `cmd/unheaded-daemon` | main | yes | Architecture, Containers (+15) | none needed |
| `cmd/unheaded-keygen` | main | yes | Releases, Service-Auth (+2) | none needed |
| `cmd/unheaded-remediate` | main | yes | Microservices, Security | none needed |
| `cmd/upc-cartridgectl` | main | yes | Port-Registry, Systemd-Units (+1) | none needed |
| `cmd/upc-tty-bridge` | main | no | Linux-on-UPC, Microservices (+2) | add package doc |
| `cmd/vulnmatch` | main | yes | SBOM, Security | none needed |
| `cmd/well-migrate` | main | yes | ADR-Index, Home (+2) | none needed |
| `cmd/well-retention` | main | yes | The-Well, Zhen-Training-Data | none needed |
| `cmd/wiki-server` | main | yes | Microservices, Port-Registry (+2) | none needed |
| `cmd/wotan-ctl` | main | yes | Microservices | none needed |
| `cmd/yggdrasil-evidence` | main | yes | ADR-010-Sealed-Cask-Deployment, Microservices | none needed |
| `cmd/zhen-agent` | main | yes | Microservices, Zhen-AI (+1) | none needed |
| `cmd/zhen-agentd` | main | yes | ADR-057-Unheaded-Source-Code-Indexing, ADR-059-Zhenai-Interactive-CLI (+12) | none needed |
| `cmd/zhen-claude` | main | yes | ADR-Index, The-Well (+7) | none needed |
| `cmd/zhen-cli` | main | yes | ADR-060-Zhenai-Multi-Model-Selector, ADR-Index (+2) | none needed |
| `cmd/zhen-codeindex` | main | yes | ADR-057-Unheaded-Source-Code-Indexing, Dark-Grimoire (+1) | none needed |
| `cmd/zhen-eval` | main | yes | Runbooks, Zhen-Audit (+2) | none needed |
| `cmd/zhen-mcp` | main | yes | Zhen-AI, Zhen-Audit (+1) | none needed |
| `cmd/zhen-rag` | main | yes | Microservices, Zhen-AI | none needed |
| `cmd/zhen-ui-front` | main | yes | Port-Registry, Service-Auth (+2) | none needed |
| `demos/mbc/boot_demo` | main | yes | none | none needed |
| `demos/mbc/fuzix_compat` | main | yes | none | none needed |
| `demos/mbc/minikernel` | main | yes | none | none needed |
| `demos/mbc/shell` | main | yes | Alerting, Battle-Plan (+12) | none needed |
| `deploy/nftables` | test-only | yes | Chaos-Engineering, Host-Settings (+3) | none needed |
| `deploy/sophia-eye/sophia-gateway` | main | no | none | add package doc |
| `eval/two-brains/live/go-nil-map` | fixture | no | Zhen-Agents | none: eval fixture, not a package of the platform |
| `nix/tests` | test-only | no | none | add package doc |
| `pkg/agent` | agent | yes | Zhen-AI | none needed |
| `pkg/audit` | audit | yes | ADR-066-Tonic-Minor-Bump-Trace-Collector, ADR-Index (+28) | none needed |
| `pkg/audit/storage` | storage | yes | ADR-045-Wave-10D-GPU-Backward-First-Real-Training, Draft-MBC-ISA-00 (+6) | none needed |
| `pkg/auth` | auth | yes | ADR-Index, Alerting (+23) | none needed |
| `pkg/auth/authtest` | authtest | yes | none | none needed |
| `pkg/baremetal` | baremetal | yes | none | none needed |
| `pkg/baremetal/image` | image | yes | ADR-Index, Benchmarks (+16) | none needed |
| `pkg/baremetal/inventory` | inventory | yes | ADR-066-Tonic-Minor-Bump-Trace-Collector, ADR-Index (+6) | none needed |
| `pkg/baremetal/ipmi` | ipmi | yes | none | none needed |
| `pkg/baremetal/pxe` | pxe | yes | none | none needed |
| `pkg/benchmark` | benchmark | yes | ADR-Index, Mimirs-Law (+3) | none needed |
| `pkg/bff` | bff | yes | Microservices, Service-Auth | none needed |
| `pkg/bff/bfftest` | bfftest | yes | Service-Auth | none needed |
| `pkg/bff/sessions` | sessions | yes | ADR-Index, Alerting (+8) | none needed |
| `pkg/cartridge` | cartridge | yes | Systemd-Units, UPC-Cartridges | none needed |
| `pkg/champion` | champion | yes | Project-Structure, Roadmap (+3) | none needed |
| `pkg/champion/pgstore` | pgstore | yes | Service-Kanban-App | none needed |
| `pkg/claudebridge` | claudebridge | yes | ADR-Index, Zhen-AI (+5) | none needed |
| `pkg/claudebridge/privacy` | privacy | yes | ADR-Index, Compliance-Paperwork (+6) | none needed |
| `pkg/cmdline` | cmdline | yes | ADR-Index, The-First-Packet | none needed |
| `pkg/codeindex` | codeindex | yes | ADR-057-Unheaded-Source-Code-Indexing | none needed |
| `pkg/compliance/crosswalk` | crosswalk | yes | ADR-Index, Compliance-Paperwork (+3) | none needed |
| `pkg/compliance/crosswalk/collect` | collect | yes | Compliance-Paperwork | none needed |
| `pkg/compliance/executor` | executor | yes | ADR-Index, Doom-on-UPC (+1) | none needed |
| `pkg/compliance/remediation` | remediation | yes | ADR-029-Wotan-Consensus-Health-Remediation, ADR-Index (+10) | none needed |
| `pkg/compliance/snapshot` | snapshot | yes | Alerting, Architecture (+7) | none needed |
| `pkg/compliance/well` | well | yes | Alerting, Chaos-Engineering (+12) | none needed |
| `pkg/critalert` | critalert | yes | Alerting, Architecture (+5) | none needed |
| `pkg/crypto/pqc` | pqc | yes | PQC-Authentication, Wotan-Topic-Signing | none needed |
| `pkg/database` | database | no | ADR-Index, Alerting (+11) | add package doc |
| `pkg/database/migrate` | migrate | yes | none | none needed |
| `pkg/deploy/cuirassdriver` | cuirassdriver | yes | Service-Sword | none needed |
| `pkg/deploy/cuirassdriver/cuirasstest` | cuirasstest | yes | none | none needed |
| `pkg/deploy/rollback` | rollback | yes | ADR-060-Zhenai-Multi-Model-Selector, ADR-Index (+12) | none needed |
| `pkg/deploy/strategy` | strategy | yes | Compliance-Paperwork, Sacred-Hierarchy (+2) | none needed |
| `pkg/discovery` | discovery | yes | ADR-065-Aya-Major-Version-Migration, ADR-Index (+14) | none needed |
| `pkg/ebpf` | ebpf | yes | none | none needed |
| `pkg/ebpf/objtrust` | objtrust | yes | none | none needed |
| `pkg/energy` | energy | yes | ADR-Index, Zhen-Audit | none needed |
| `pkg/enkrateia` | enkrateia | yes | Mimirs-Law, Project-Structure | fix: spdx-merged:enkrateia.go |
| `pkg/gjallarhorn` | gjallarhorn | yes | Mimirs-Law, Project-Structure | fix: spdx-merged:types.go |
| `pkg/gpulease` | gpulease | yes | Zhen-Agents | none needed |
| `pkg/gungnir` | gungnir | yes | Mimirs-Law, Project-Structure (+1) | fix: spdx-merged:gungnir.go |
| `pkg/health` | health | yes | ADR-Index, Architecture (+29) | none needed |
| `pkg/healthreport` | healthreport | yes | Architecture, Service-Akira (+2) | none needed |
| `pkg/hoprules` | hoprules | yes | eBPF-Programs | none needed |
| `pkg/httputil` | httputil | no | ADR-015-Go-Fiber-HTTP-Layer, ADR-Index (+2) | add package doc |
| `pkg/iac` | iac | yes | IaC-Backends | none needed |
| `pkg/journald` | journald | yes | Log-Aggregation, Observability-Backends (+1) | none needed |
| `pkg/lichtools` | lichtools | yes | Linux-on-UPC | none needed |
| `pkg/lifecycle` | lifecycle | no | Linux-on-UPC, Protocol-Specs | add package doc |
| `pkg/localllm` | localllm | yes | Zhen-Agents | none needed |
| `pkg/logagg` | logagg | yes | Fae-Chamber-Contracts, Log-Aggregation (+4) | none needed |
| `pkg/logger` | logger | yes | Log-Aggregation, Service-Wotan | none needed |
| `pkg/logsources` | logsources | yes | none | none needed |
| `pkg/mcpclient` | mcpclient | yes | none | none needed |
| `pkg/mcpserver` | mcpserver | yes | Zhen-AI | none needed |
| `pkg/metrics` | metrics | yes | ADR-060-Zhenai-Multi-Model-Selector, ADR-Index (+29) | none needed |
| `pkg/metrics/auto` | auto | yes | Transport-Cascade, Zhen-AI (+1) | none needed |
| `pkg/metrics/metricstest` | metricstest | yes | none | none needed |
| `pkg/metrics/prom` | prom | yes | none | none needed |
| `pkg/modules` | modules | yes | none | none needed |
| `pkg/monad` | monad | no | ADR-Index, Alerting (+4) | add package doc |
| `pkg/monadpkt` | monadpkt | yes | Steel-Echo | none needed |
| `pkg/observability` | observability | yes | Observability-Backends | none needed |
| `pkg/policy` | policy | yes | ADR-066-Tonic-Minor-Bump-Trace-Collector, Alerting (+23) | none needed |
| `pkg/ports` | ports | yes | ADR-Index, Containers (+18) | none needed |
| `pkg/pqc_policy` | pqc_policy | yes | PQC-Authentication, Project-Structure | none needed |
| `pkg/probenet` | probenet | yes | Chaos-Engineering | none needed |
| `pkg/protocol/bpfschema` | bpfschema | yes | none | none needed |
| `pkg/protocol/encoding` | encoding | yes | Draft-MBC-ISA-00, Draft-Protocol-Foundation-06 (+8) | none needed |
| `pkg/protocol/errors` | errors | yes | Alerting, Benchmarks (+9) | none needed |
| `pkg/protocol/fuzz` | test-only | no | Developer-Guide, LICH-Campaigns (+1) | add package doc |
| `pkg/protocol/integrity` | integrity | no | Dark-Grimoire, Draft-Protocol-Foundation-06 (+7) | add package doc |
| `pkg/protocol/migration` | migration | yes | ADR-015-Go-Fiber-HTTP-Layer, ADR-065-Aya-Major-Version-Migration (+19) | none needed |
| `pkg/protocol/sophiasync` | sophiasync | no | Sophia-Dictionaries | add package doc |
| `pkg/protocol/tlv` | tlv | yes | none | none needed |
| `pkg/repolink` | repolink | yes | none | none needed |
| `pkg/retrieval` | retrieval | yes | ADR-057-Unheaded-Source-Code-Indexing, Systemd-Units (+1) | none needed |
| `pkg/sandbox` | sandbox | yes | ADR-019-Zhen-Champion-Agent, ADR-Index (+9) | none needed |
| `pkg/sealedcask` | sealedcask | yes | ADR-010-Sealed-Cask-Deployment | none needed |
| `pkg/sealedcask/casktest` | casktest | yes | none | none needed |
| `pkg/secrets` | secrets | yes | Demo-Script, Developer-Guide (+11) | none needed |
| `pkg/secrets/encryption` | encryption | yes | Phylactery, Post-Quantum-Key-Exchange (+1) | none needed |
| `pkg/secrets/store` | store | yes | none | none needed |
| `pkg/sloguard` | sloguard | yes | Chaos-Engineering, Service-Sword | none needed |
| `pkg/sophia` | sophia | yes | ADR-Index, Architecture (+6) | none needed |
| `pkg/statefile` | statefile | yes | Service-Sword | none needed |
| `pkg/storage` | storage | yes | ADR-045-Wave-10D-GPU-Backward-First-Real-Training, Draft-MBC-ISA-00 (+6) | none needed |
| `pkg/storage/wal` | wal | yes | none | none needed |
| `pkg/syslog` | syslog | yes | ADR-Index, Log-Aggregation (+2) | none needed |
| `pkg/threatfeed/inventory` | inventory | yes | ADR-066-Tonic-Minor-Bump-Trace-Collector, ADR-Index (+6) | none needed |
| `pkg/threatfeed/kev` | kev | yes | Service-KEV-Poller | none needed |
| `pkg/threatfeed/osv` | osv | yes | none | none needed |
| `pkg/tls` | tls | yes | ADR-Index, Post-Quantum-Key-Exchange (+1) | none needed |
| `pkg/tls/pqtest` | pqtest | yes | Post-Quantum-Key-Exchange | none needed |
| `pkg/trace` | trace | yes | ADR-Index, Alerting (+23) | none needed |
| `pkg/transport` | transport | yes | Developer-Guide, Doom-on-UPC (+10) | none needed |
| `pkg/transport/mtls` | mtls | yes | PQC-Authentication, Post-Quantum-Key-Exchange | none needed |
| `pkg/uids` | uids | yes | Architecture, Chaos-Engineering (+6) | none needed |
| `pkg/upc` | upc | yes | none | none needed |
| `pkg/vmquery` | vmquery | yes | none | none needed |
| `pkg/vorclient` | vorclient | yes | Zhen-AI | none needed |
| `pkg/vorquery` | vorquery | yes | Zhen-AI | none needed |
| `pkg/wotan` | wotan | yes | Alerting, Architecture (+18) | none needed |
| `pkg/wotan-client` | wotanClient | yes | Post-Quantum-Key-Exchange, Project-Structure (+7) | none needed |
| `pkg/wotan-client/mock` | mock | yes | Containers | none needed |
| `pkg/wotan-client/wotantest` | wotantest | yes | Service-Wotan | none needed |
| `pkg/wotanerr` | wotanerr | yes | Service-Wotan | none needed |
| `pkg/wotansig` | wotansig | yes | Wotan-Topic-Signing | none needed |
| `pkg/zhenagent` | zhenagent | yes | none | none needed |
| `pkg/zhencapture` | zhencapture | yes | The-Well, Zhen-AI (+2) | none needed |
| `pkg/zhenrag` | zhenrag | yes | Zhen-AI | none needed |
| `pkg/zhenrag/gen` | main | yes | Zhen-AI | none needed |
| `pkg/zhenroute` | zhenroute | yes | Zhen-Agents | none needed |
| `pkg/zhentools` | zhentools | yes | ADR-Index | none needed |
| `proto/unheaded/v1` | unheadedv1 | no | none | add package doc |
| `scripts/lib` | test-only | yes | Log-Aggregation, Releases (+4) | none needed |
| `services/anomaly` | anomaly | yes | ADR-Index, Port-Registry (+6) | none needed |
| `services/compliance` | compliance | yes | Project-Structure | none needed |
| `services/firewall` | firewall | yes | ADR-Index, Port-Registry (+5) | none needed |
| `services/gateway` | gateway | yes | Architecture, Benchmarks (+11) | none needed |
| `services/gateway/cmd` | main | yes | none | none needed |
| `services/gateway/config` | config | yes | none | none needed |
| `services/gateway/middleware` | middleware | yes | ADR-015-Go-Fiber-HTTP-Layer, Service-Kanban-App (+1) | none needed |
| `services/gateway/proxy` | proxy | yes | Benchmarks, Chaos-Engineering (+9) | none needed |
| `services/gateway/routes` | routes | yes | ADR-Index, Alerting (+6) | none needed |
| `services/keymgmt` | keymgmt | yes | ADR-Index, Project-Structure | none needed |
| `services/keymgmt/handler` | handler | yes | ADR-Index, Containers (+5) | none needed |
| `services/keymgmt/lifecycle` | lifecycle | yes | Linux-on-UPC, Protocol-Specs | none needed |
| `services/keymgmt/store` | store | yes | none | none needed |
| `services/loadbalancer` | loadbalancer | yes | Port-Registry, Project-Structure (+1) | none needed |
| `services/nfv` | nfv | yes | Project-Structure | none needed |
| `services/qos` | qos | yes | Project-Structure | none needed |
| `services/sabatons` | sabatons | yes | Project-Structure | none needed |
| `services/selfheal` | selfheal | yes | Project-Structure | none needed |
| `services/shield` | shield | yes | Microservices, Port-Registry (+2) | none needed |
| `services/sword` | sword | yes | Alerting, Game-Days (+8) | none needed |
| `services/timeguru` | main | no | Architecture, Benchmarks (+19) | add package doc |
| `services/timeguru/cmd/timeguru` | main | yes | Architecture, Benchmarks (+18) | none needed |
| `services/timeguru/internal/api` | api | no | none | add package doc |
| `services/timeguru/internal/parser` | parser | yes | Doom-on-UPC, Doom-over-IPv6 (+6) | none needed |
| `services/timeguru/internal/storage` | storage | yes | ADR-045-Wave-10D-GPU-Backward-First-Real-Training, Draft-MBC-ISA-00 (+6) | none needed |
| `services/timeguru/internal/sync` | sync | yes | Architecture, Chaos-Engineering (+11) | none needed |
| `services/timeguru/internal/timeline` | timeline | no | Agent-Operating-Procedure, Architecture (+11) | add package doc |
| `services/version` | version | yes | ADR-Index, Agent-Operating-Procedure (+25) | none needed |
| `services/wotan/cmd/wotan` | main | no | Alerting, Architecture (+18) | add package doc |
| `services/wotan/internal/api` | api | no | Fae-Chamber-Contracts, Wotan-Topic-Signing | add package doc |
| `services/wotan/internal/cluster` | cluster | yes | Battle-Plan, Mimirs-Law (+7) | none needed |
| `services/wotan/internal/compute` | compute | yes | Architecture, Doom-on-UPC (+7) | none needed |
| `services/wotan/internal/grpc` | grpc | no | Service-Auth, Transport-Cascade (+1) | add package doc |
| `services/wotan/internal/logger` | logger | no | Service-Wotan | add package doc |
| `services/wotan/internal/member` | member | no | ADR-010-Sealed-Cask-Deployment, Service-Auth (+1) | add package doc |
| `services/wotan/internal/metrics` | metrics | no | ADR-060-Zhenai-Multi-Model-Selector, ADR-Index (+27) | add package doc |
| `services/wotan/internal/middleware` | middleware | no | ADR-015-Go-Fiber-HTTP-Layer, Service-Kanban-App (+1) | add package doc |
| `services/wotan/internal/ringbuffer` | ringbuffer | no | Log-Aggregation | add package doc |
| `services/wotan/internal/room` | room | no | Doom-over-IPv6, Protocol-Specs (+6) | add package doc |
| `services/wotan/internal/signing` | signing | yes | ADR-Index, Alerting (+12) | fix: spdx-merged:signing.go |
| `services/wotan/internal/store` | store | yes | none | none needed |
| `services/wotan/internal/topicpattern` | topicpattern | no | none | add package doc |
| `services/wotan/internal/wotan` | wotan | yes | Alerting, Architecture (+18) | none needed |
| `services/wotan/proto` | chatpb | no | Protocol-Specs, Wotan-Topic-Signing | add package doc |
| `services/yaldabaoth` | yaldabaoth | yes | ADR-Index, Chaos-Engineering (+4) | none needed |
| `services/yaldabaoth/packet` | packet | yes | none | none needed |
| `tests/e2e` | test-only | yes | none | none needed |
| `tests/hopchecks` | test-only | yes | none | none needed |
| `tests/integration` | test-only | yes | Architecture, Battle-Plan (+7) | none needed |
| `tests/pqc` | test-only | yes | none | none needed |
| `tests/s77` | test-only | yes | none | none needed |
| `tests/svctoken` | test-only | yes | Service-Auth | none needed |
| `tomb/lich/LICH-017-baseline-agent` | test-only | yes | Compliance-Paperwork | none needed |
| `tomb/lich/harnesses` | test-only | yes | Developer-Guide, LICH-Campaigns | none needed |
| `tomb/lich/seeds/auth/gen` | main | yes | none | none needed |

### Rust crates

| Crate dir | Crate | Crate doc (`//!`) per root | Wiki | Action |
|---|---|---|---|---|
| `cmd/ebpf-collector` | workspace | - | Microservices, Rust-Components, Service-Auth | none needed |
| `cmd/ebpf-collector/collector` | collector | src/main.rs=yes | none | add wiki row |
| `cmd/ebpf-collector/common` | ebpf-common | src/lib.rs=yes | none | add wiki row |
| `cmd/ebpf-collector/ebpf-programs` | ebpf-programs | src/packet_counter.rs=yes; src/tcp_latency.rs=yes | none | add wiki row |
| `cmd/ebpf-load-check` | ebpf-load-check | src/main.rs=yes | Microservices, Rust-Components, eBPF-Programs | none needed |
| `cmd/ebpf-loader` | ebpf-loader | src/main.rs=yes | ADR-065-Aya-Major-Version-Migration, ADR-066-Tonic-Minor-Bump-Trace-Collector, Microservices (+3) | none needed |
| `cmd/upc-bootctl` | upc-bootctl | src/main.rs=no | ADR-072-Boot-Magic-Byte-Ordering, Linux-on-UPC, MBC-ISA-Reference (+5) | add `//!` doc |
| `crates/heimdall-bpf` | heimdall-bpf | src/vfs_write.rs=yes; src/execve.rs=yes; src/gjallarhorn_xdp.rs=yes | Mimirs-Law, Project-Structure, Rust-Components | none needed |
| `crates/lich-fuzz` | lich-fuzz | lich_007_mbc.rs=no; lich_008_wotan_cache.rs=no; lich_009_flow_collision.rs=no; lich_010_wal_integrity.rs=no | LICH-Campaigns, Project-Structure, Rust-Components (+1) | add `//!` doc |
| `crates/monad-mbc` | monad-mbc | src/lib.rs=yes; src/bin/rv32i_to_mbc.rs=yes; src/bin/mbc_asm.rs=yes; src/bin/mbc_emulate.rs=yes | ADR-067-MBC-ISA-V2-and-UPC-ABI-V1, Doom-on-UPC, LICH-Campaigns (+8) | none needed |
| `crates/monad-mbc/fuzz` | monad-mbc-fuzz | fuzz_targets/fuzz_mbc_decode.rs=no; fuzz_targets/fuzz_mbc_execute.rs=no; fuzz_targets/fuzz_mbc_roundtrip.rs=no; fuzz_targets/fuzz_cart_step.rs=yes; fuzz_targets/fuzz_cart_differential.rs=yes; fuzz_targets/fuzz_cart_image.rs=yes | LICH-Campaigns, Security, UPC-Cartridges | add `//!` doc |
| `crates/objtrust` | unheaded-objtrust | src/lib.rs=yes | none | add wiki row |
| `crates/svc-token` | unheaded-svc-token | src/lib.rs=yes | Service-Auth | none needed |
| `crates/upc-api` | upc-api | src/lib.rs=yes | Post-Quantum-Key-Exchange, Project-Structure, Rust-Components | none needed |
| `crates/upc-api/fuzz` | upc-api-fuzz | fuzz_targets/fuzz_object_facts.rs=yes; fuzz_targets/fuzz_slice_geometry.rs=yes | none | add wiki row |
| `crates/upc-bootstub` | upc-bootstub | src/lib.rs=yes | Project-Structure, Rust-Components, UPC-Dream-Ladder | none needed |
| `crates/upc-cartridge` | upc-cartridge | src/lib.rs=yes; src/bin/cart.rs=yes | UPC-Cartridges | none needed |
| `crates/xv6-mbc` | xv6-mbc | src/lib.rs=yes | ADR-072-Boot-Magic-Byte-Ordering, Linux-on-UPC, Project-Structure (+3) | none needed |
| `crates/zhend` | zhend | src/lib.rs=yes; src/main.rs=yes | ADR-066-Tonic-Minor-Bump-Trace-Collector, ADR-Index, Post-Quantum-Key-Exchange (+2) | none needed |
| `ebpf` | workspace | - | ADR-065-Aya-Major-Version-Migration, ADR-Index, Architecture (+23) | none needed |
| `ebpf/anomaly-ebpf` | anomaly-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/canary-ebpf` | canary-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/cartridge-ebpf` | cartridge-ebpf | src/main.rs=yes | ADR-Index, Home, UPC-Cartridges | none needed |
| `ebpf/common` | unheaded-common | src/lib.rs=yes | none | add wiki row |
| `ebpf/compliance-ebpf` | compliance-ebpf | src/main.rs=yes | Compliance-Paperwork, Rust-Components, eBPF-Programs | none needed |
| `ebpf/failover-ebpf` | failover-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/firewall-ebpf` | firewall-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/flow-tracker` | flow-tracker | src/main.rs=yes | Demo-Script, Protocol-Specs, Rust-Components (+2) | none needed |
| `ebpf/hop-ebpf` | hop-ebpf | src/main.rs=yes | ADR-Index, Protocol-Specs, Rust-Components (+1) | none needed |
| `ebpf/latency-probe` | latency-probe | src/main.rs=yes | Demo-Script, Rust-Components, Systemd-Units (+1) | none needed |
| `ebpf/maglev-ebpf` | maglev-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/monad-common` | monad-common | src/lib.rs=yes | Architecture, Protocol-Specs, Protocol-Technical-Summary (+5) | none needed |
| `ebpf/monad-cpu-ebpf` | monad-cpu-ebpf | src/main.rs=yes | ADR-012-BPF-Verifier-Risk-Mitigation, ADR-067-MBC-ISA-V2-and-UPC-ABI-V1, ADR-Index (+11) | none needed |
| `ebpf/nfv-ebpf` | nfv-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/packet-marker` | packet-marker | src/main.rs=yes | Demo-Script, Rust-Components, Systemd-Units (+1) | none needed |
| `ebpf/pqc-common` | pqc-common | src/lib.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/qos-ebpf` | qos-ebpf | src/main.rs=yes | ADR-Index, Rust-Components, UPC-Cartridges (+1) | none needed |
| `ebpf/shield-ebpf` | shield-ebpf | src/main.rs=yes | Chaos-Engineering, PQC-Authentication, Rust-Components (+1) | none needed |
| `ebpf/syscall-tracer` | syscall-tracer | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/version-ebpf` | version-ebpf | src/main.rs=yes | Rust-Components, eBPF-Programs | none needed |
| `ebpf/yaldabaoth-ebpf` | yaldabaoth-ebpf | src/main.rs=yes | Chaos-Engineering, Rust-Components, eBPF-Programs | none needed |
| `tomb/lich/LICH-015-mbc-differential/harnesses/diff_harness` | lich015-diff-harness | src/main.rs=no | none | add `//!` doc; add wiki row |

### Binaries under `cmd/`

| Binary dir | Wiki | Action |
|---|---|---|
| `cmd/akira` | Architecture, Microservices, Port-Registry (+6) | none needed |
| `cmd/alert-mailer` | ADR-Index, Alerting, Architecture (+8) | none needed |
| `cmd/alert-policy` | Alerting, Microservices, The-Well | none needed |
| `cmd/anomaly` | Port-Registry, Project-Structure, Rust-Components (+4) | none needed |
| `cmd/cask-verify` | ADR-010-Sealed-Cask-Deployment | none needed |
| `cmd/cert-gen` | Microservices, Port-Registry | none needed |
| `cmd/chaos-controller` | ADR-Index, Chaos-Engineering, Game-Days (+8) | none needed |
| `cmd/compliance-evidence` | ADR-Index, Alerting, Compliance-Paperwork (+3) | none needed |
| `cmd/crypto-inventory` | Post-Quantum-Key-Exchange | none needed |
| `cmd/dashboard-backend` | ADR-Index, Alerting, Architecture (+17) | none needed |
| `cmd/demo-trace-injector` | Microservices, Service-Auth | none needed |
| `cmd/depaudit` | none | add row to Microservices |
| `cmd/ebpf-collector` | Microservices, Rust-Components, Service-Auth | none needed |
| `cmd/ebpf-load-check` | Microservices, Rust-Components, eBPF-Programs | none needed |
| `cmd/ebpf-loader` | ADR-065-Aya-Major-Version-Migration, ADR-066-Tonic-Minor-Bump-Trace-Collector, Microservices (+3) | none needed |
| `cmd/gjallarhorn-listener` | none | add row to Microservices |
| `cmd/gjallarhorn-sender` | Microservices, Mimirs-Law | none needed |
| `cmd/healthprobe` | Service-Akira | none needed |
| `cmd/heimdall-daemon` | Alerting, Microservices, Mimirs-Law (+3) | none needed |
| `cmd/huginn` | ADR-Index, Alerting, Architecture (+9) | none needed |
| `cmd/kanban-app` | Architecture, Demo-Script, Fae-Chamber-Contracts (+11) | none needed |
| `cmd/kev-poller` | ADR-Index, Home, Microservices (+6) | none needed |
| `cmd/lich-mcp` | Zhen-AI | none needed |
| `cmd/lich-security` | LICH-Campaigns, Microservices | none needed |
| `cmd/logstore-writer` | Log-Aggregation, Port-Registry | none needed |
| `cmd/perf-benchmark` | Benchmarks, Home, Microservices | none needed |
| `cmd/protocol-api` | Microservices, Port-Registry, Protocol-Specs | none needed |
| `cmd/routing-health` | Microservices | none needed |
| `cmd/shield` | Microservices, Port-Registry, Project-Structure (+1) | none needed |
| `cmd/steel-echo` | Chaos-Engineering, Containers, Game-Days (+5) | none needed |
| `cmd/sword` | Alerting, Game-Days, Microservices (+7) | none needed |
| `cmd/syslog-relay` | ADR-Index, Port-Registry | none needed |
| `cmd/tools` | none | add row to Microservices |
| `cmd/trace-collector-go` | Demo-Script, Log-Aggregation, Microservices (+3) | none needed |
| `cmd/unheaded` | ADR-022-Pihole-Docker-to-LXD, Alerting, Chaos-Engineering (+18) | none needed |
| `cmd/unheaded-aide` | Security, Systemd-Units | none needed |
| `cmd/unheaded-auth` | ADR-Index, Port-Registry, Service-Auth (+4) | none needed |
| `cmd/unheaded-baseline` | ADR-Index, Compliance-Paperwork, Microservices (+2) | none needed |
| `cmd/unheaded-cli` | IaC-Backends, Kingdom-Architecture, Microservices (+5) | none needed |
| `cmd/unheaded-daemon` | Architecture, Containers, Demo-Script (+14) | none needed |
| `cmd/unheaded-keygen` | Releases, Service-Auth, Sophia-Dictionaries (+1) | none needed |
| `cmd/unheaded-remediate` | Microservices, Security | none needed |
| `cmd/upc-bootctl` | ADR-072-Boot-Magic-Byte-Ordering, Linux-on-UPC, MBC-ISA-Reference (+5) | none needed |
| `cmd/upc-cartridgectl` | Port-Registry, Systemd-Units, UPC-Cartridges | none needed |
| `cmd/upc-tty-bridge` | Linux-on-UPC, Microservices, Profiling (+1) | none needed |
| `cmd/vulnmatch` | SBOM, Security | none needed |
| `cmd/well-migrate` | ADR-Index, Home, Microservices (+1) | none needed |
| `cmd/well-retention` | The-Well, Zhen-Training-Data | none needed |
| `cmd/wiki-server` | Microservices, Port-Registry, Service-Auth (+1) | none needed |
| `cmd/wotan-ctl` | Microservices | none needed |
| `cmd/yggdrasil-evidence` | ADR-010-Sealed-Cask-Deployment, Microservices | none needed |
| `cmd/zhen-agent` | Microservices, Zhen-AI, Zhen-Confirmation | none needed |
| `cmd/zhen-agentd` | ADR-057-Unheaded-Source-Code-Indexing, ADR-059-Zhenai-Interactive-CLI, ADR-Index (+11) | none needed |
| `cmd/zhen-claude` | ADR-Index, The-Well, Zhen-AI (+6) | none needed |
| `cmd/zhen-cli` | ADR-060-Zhenai-Multi-Model-Selector, ADR-Index, Microservices (+1) | none needed |
| `cmd/zhen-codeindex` | ADR-057-Unheaded-Source-Code-Indexing, Dark-Grimoire, Zhen-AI | none needed |
| `cmd/zhen-eval` | Runbooks, Zhen-Audit, Zhen-Routing (+1) | none needed |
| `cmd/zhen-mcp` | Zhen-AI, Zhen-Audit, Zhen-Training-Data | none needed |
| `cmd/zhen-rag` | Microservices, Zhen-AI | none needed |
| `cmd/zhen-ui-front` | Port-Registry, Service-Auth, Systemd-Units (+1) | none needed |

### Services under `services/`

| Service dir | Wiki | Action |
|---|---|---|
| `services/anomaly` | ADR-Index, Port-Registry, Project-Structure (+5) | none needed |
| `services/compliance` | Project-Structure | none needed |
| `services/firewall` | ADR-Index, Port-Registry, Project-Structure (+4) | none needed |
| `services/gateway` | Architecture, Benchmarks, Load-Balancers (+10) | none needed |
| `services/keymgmt` | ADR-Index, Project-Structure | none needed |
| `services/loadbalancer` | Port-Registry, Project-Structure, Service-Wotan | none needed |
| `services/nfv` | Project-Structure | none needed |
| `services/qos` | Project-Structure | none needed |
| `services/sabatons` | Project-Structure | none needed |
| `services/selfheal` | Project-Structure | none needed |
| `services/shield` | Microservices, Port-Registry, Project-Structure (+1) | none needed |
| `services/sword` | Alerting, Game-Days, Microservices (+7) | none needed |
| `services/timeguru` | Architecture, Benchmarks, Chaos-Engineering (+18) | none needed |
| `services/version` | ADR-Index, Agent-Operating-Procedure, Architecture (+24) | none needed |
| `services/wotan` | Alerting, Architecture, Chaos-Engineering (+21) | none needed |
| `services/yaldabaoth` | ADR-Index, Chaos-Engineering, Game-Days (+3) | none needed |

### Modules under `modules/`

| Module | Wiki | Action |
|---|---|---|
| `modules/zhen` | ADR-Index, Service-Kanban-App, Zhen-AI (+4) | none needed |

### systemd units (`deploy/systemd/`, `systemd/`)

| Unit file | Wiki | Action |
|---|---|---|
| `deploy/systemd/dropins/10-start-limit.conf` | Systemd-Units | none needed |
| `deploy/systemd/huginn.service` | Alerting, Architecture, Host-Settings (+8) | none needed |
| `deploy/systemd/node-exporter.service` | Host-Settings, Systemd-Units | none needed |
| `deploy/systemd/slice-dropins/unheaded-env.conf` | Host-Settings | none needed |
| `deploy/systemd/slice-dropins/unheaded-host.conf` | none | add row to Systemd-Units |
| `deploy/systemd/tenant.slice` | Draft-Sophia-Dictionary-03, Sophia-Dictionaries | none needed |
| `deploy/systemd/unheaded-aide-check.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-aide-check.timer` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-akira.service` | Service-Akira, Systemd-Units | none needed |
| `deploy/systemd/unheaded-anomaly.service` | Systemd-Units, UPC-Cartridges | none needed |
| `deploy/systemd/unheaded-auth.service` | ADR-Index, Port-Registry, Service-Auth (+4) | none needed |
| `deploy/systemd/unheaded-baseline.path` | ADR-Index, Compliance-Paperwork, Microservices (+2) | none needed |
| `deploy/systemd/unheaded-baseline.service` | ADR-Index, Compliance-Paperwork, Microservices (+2) | none needed |
| `deploy/systemd/unheaded-baseline.timer` | ADR-Index, Compliance-Paperwork, Microservices (+2) | none needed |
| `deploy/systemd/unheaded-boot-event.service` | Alerting, Host-Settings, Systemd-Units | none needed |
| `deploy/systemd/unheaded-chaos-controller.service` | Chaos-Engineering, Systemd-Units | none needed |
| `deploy/systemd/unheaded-chaos-controller.service.d/east.conf` | ADR-022-Pihole-Docker-to-LXD, ADR-Index, Alerting (+30) | none needed |
| `deploy/systemd/unheaded-chaos-controller.service.d/west-packet-lab.conf` | Chaos-Engineering, Systemd-Units | none needed |
| `deploy/systemd/unheaded-chaos-guard.service` | Chaos-Engineering, Systemd-Units | none needed |
| `deploy/systemd/unheaded-ci-status.service` | Alerting | none needed |
| `deploy/systemd/unheaded-ci-status.timer` | Alerting | none needed |
| `deploy/systemd/unheaded-clickhouse-backup.service` | Well-Backups | none needed |
| `deploy/systemd/unheaded-clickhouse-backup.timer` | Well-Backups | none needed |
| `deploy/systemd/unheaded-daemon.service` | Architecture, Containers, Demo-Script (+13) | none needed |
| `deploy/systemd/unheaded-dashboard.service` | Service-Dashboard-Backend, Systemd-Units | none needed |
| `deploy/systemd/unheaded-env.slice` | Host-Settings | none needed |
| `deploy/systemd/unheaded-health-sweep.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-health-sweep.timer` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-heartbeat.service` | Alerting | none needed |
| `deploy/systemd/unheaded-heartbeat.timer` | Alerting | none needed |
| `deploy/systemd/unheaded-heimdall.service` | Wotan-Topic-Signing | none needed |
| `deploy/systemd/unheaded-host-settings.service` | Host-Settings | none needed |
| `deploy/systemd/unheaded-host-settings.timer` | Host-Settings | none needed |
| `deploy/systemd/unheaded-host-state.service` | Alerting, Systemd-Units | none needed |
| `deploy/systemd/unheaded-host-state.timer` | Alerting, Systemd-Units | none needed |
| `deploy/systemd/unheaded-host.slice` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-ioc-audit.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-ioc-audit.timer` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-kanban.service` | Service-Kanban-App, Systemd-Units | none needed |
| `deploy/systemd/unheaded-kev-poller.service` | Chaos-Engineering, Service-KEV-Poller, Systemd-Units | none needed |
| `deploy/systemd/unheaded-llama.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-module-guard.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-module-guard.timer` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-nft.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-store-drill.service` | Well-Backups | none needed |
| `deploy/systemd/unheaded-store-drill.timer` | Well-Backups | none needed |
| `deploy/systemd/unheaded-sword.service` | Port-Registry, Service-Sword | none needed |
| `deploy/systemd/unheaded-syslog-relay.service` | Port-Registry, Systemd-Units | none needed |
| `deploy/systemd/unheaded-syslog-relay.socket` | Port-Registry, Systemd-Units | none needed |
| `deploy/systemd/unheaded-timeguru.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-trace-collector.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-upc-bootctl@.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-upc-netns.service` | Linux-on-UPC, Systemd-Units | none needed |
| `deploy/systemd/unheaded-victoria-backup.service` | Well-Backups | none needed |
| `deploy/systemd/unheaded-victoria-backup.timer` | Well-Backups | none needed |
| `deploy/systemd/unheaded-vor-refresh.path` | Zhen-AI | none needed |
| `deploy/systemd/unheaded-vor-refresh.service` | Zhen-AI | none needed |
| `deploy/systemd/unheaded-vor.service` | Systemd-Units, Zhen-AI | none needed |
| `deploy/systemd/unheaded-well-backup.service` | Systemd-Units, The-Well, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-backup.timer` | Systemd-Units, The-Well, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-drill.service` | Systemd-Units, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-drill.timer` | Systemd-Units, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-offsite-check.service` | Systemd-Units, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-offsite-check.timer` | Systemd-Units, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-offsite.service` | Systemd-Units, Well-Backups | none needed |
| `deploy/systemd/unheaded-well-retention.service` | The-Well | none needed |
| `deploy/systemd/unheaded-well-retention.timer` | The-Well | none needed |
| `deploy/systemd/unheaded-wg-psk-rotate.service` | Overlay-Network | none needed |
| `deploy/systemd/unheaded-wg-psk-rotate.timer` | Overlay-Network | none needed |
| `deploy/systemd/unheaded-wiki-server.service` | Microservices, Port-Registry, Systemd-Units | none needed |
| `deploy/systemd/unheaded-wiki-server.service.d/west.conf` | ADR-022-Pihole-Docker-to-LXD, ADR-Index, Alerting (+44) | none needed |
| `deploy/systemd/unheaded-wotan.service` | Log-Aggregation, Releases, Service-Auth (+3) | none needed |
| `deploy/systemd/unheaded-zhen-app.service` | Service-Auth | none needed |
| `deploy/systemd/unheaded-zhen-backup.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-zhen-backup.timer` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-zhen-module.service` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded-zhen-ui-front.service` | Systemd-Units | none needed |
| `deploy/systemd/unheaded-zhen.target` | none | add row to Systemd-Units |
| `deploy/systemd/unheaded.slice` | ADR-022-Pihole-Docker-to-LXD, Alerting, Chaos-Engineering (+18) | none needed |
| `deploy/systemd/user-1000.slice.d/50-unheaded-caps.conf` | none | add row to Systemd-Units |
| `deploy/systemd/user/unheaded-compliance-evidence.service` | Systemd-Units | none needed |
| `deploy/systemd/user/unheaded-compliance-evidence.timer` | Systemd-Units | none needed |
| `deploy/systemd/user/unheaded-git-mirror.service` | none | add row to Systemd-Units |
| `deploy/systemd/user/unheaded-git-mirror.timer` | Well-Backups | none needed |
| `deploy/systemd/user/unheaded-logstore-measure.service` | Log-Aggregation, Systemd-Units | none needed |
| `deploy/systemd/user/unheaded-logstore-measure.timer` | Log-Aggregation, Systemd-Units | none needed |
| `deploy/systemd/user/zhen-agentd-overnight.service` | Zhen-Agents | none needed |
| `deploy/systemd/user/zhen-agentd-overnight.timer` | Zhen-Agents | none needed |
| `deploy/systemd/vector-east.service` | Log-Aggregation | none needed |
| `deploy/systemd/zhen-agentd.service` | ADR-057-Unheaded-Source-Code-Indexing, ADR-059-Zhenai-Interactive-CLI, ADR-Index (+11) | none needed |
| `systemd/kingdom-shutdown.service` | Alerting | none needed |
| `systemd/kingdom-startup.service` | Alerting | none needed |

### Scripts under `scripts/`

| Script | Kind | Wiki | Action |
|---|---|---|---|
| `scripts/apt-repo.sh` | operator | Releases | none needed |
| `scripts/ascend-linux-smoke.sh` | operator | none | add wiki line |
| `scripts/backup/clickhouse-backup.sh` | operator | none | add wiki line |
| `scripts/backup/clickhouse-drill.sh` | operator | none | add wiki line |
| `scripts/backup/git-mirror.sh` | operator | Well-Backups | none needed |
| `scripts/backup/store-drill.sh` | operator | Well-Backups | none needed |
| `scripts/backup/victoria-backup.sh` | operator | none | add wiki line |
| `scripts/backup/victoria-drill.sh` | operator | none | add wiki line |
| `scripts/bare-metal/preflight-host-a.sh` | operator | none | add wiki line |
| `scripts/bare-metal/preflight-host-b.sh` | operator | none | add wiki line |
| `scripts/bare-metal/validate-cross-host.sh` | operator | Service-Auth | none needed |
| `scripts/bare-metal/validate-host-a.sh` | operator | none | add wiki line |
| `scripts/bare-metal/validate-host-b.sh` | operator | none | add wiki line |
| `scripts/boot-linux.sh` | operator | none | add wiki line |
| `scripts/bpf-profile.sh` | operator | Profiling | none needed |
| `scripts/bpf-verifier-check.sh` | operator | ADR-065-Aya-Major-Version-Migration | none needed |
| `scripts/build-all-debs.sh` | operator | Releases | none needed |
| `scripts/build-binaries.sh` | operator | Releases, Service-Auth | none needed |
| `scripts/build-sealed-cask.sh` | operator | ADR-010-Sealed-Cask-Deployment | none needed |
| `scripts/build.sh` | operator | none | add wiki line |
| `scripts/bulk_inject.py` | operator | Doom-over-IPv6, Performance | none needed |
| `scripts/capture-baseline.sh` | operator | none | add wiki line |
| `scripts/check-actions-pinned.sh` | gate | none | add wiki line |
| `scripts/check-cargo-locked.sh` | gate | none | add wiki line |
| `scripts/check-ci-verdicts.sh` | gate | none | add wiki line |
| `scripts/check-clippy.sh` | gate | none | add wiki line |
| `scripts/check-compose-bind-nesting.sh` | gate | none | add wiki line |
| `scripts/check-compose-hardening.sh` | gate | Architecture, Security | none needed |
| `scripts/check-compose-log-caps.sh` | gate | Log-Aggregation | none needed |
| `scripts/check-compose-publish-bind.sh` | gate | Port-Registry | none needed |
| `scripts/check-compose-secrets.sh` | gate | Security | none needed |
| `scripts/check-crypto-inventory.sh` | gate | Post-Quantum-Key-Exchange | none needed |
| `scripts/check-dep-staleness.sh` | gate | none | add wiki line |
| `scripts/check-dependency-baseline.sh` | gate | Security | none needed |
| `scripts/check-ebpf-insns.sh` | gate | eBPF-Programs | none needed |
| `scripts/check-ebpf-loads.sh` | gate | eBPF-Programs | none needed |
| `scripts/check-ebpf-size-pin.sh` | gate | Security | none needed |
| `scripts/check-evidence-pack.sh` | gate | none | add wiki line |
| `scripts/check-gates-can-fail.sh` | gate | Alerting, Home (+5) | none needed |
| `scripts/check-gitleaks.sh` | gate | Security | none needed |
| `scripts/check-go-env.sh` | gate | none | add wiki line |
| `scripts/check-gosec-ratchet.sh` | gate | Security | none needed |
| `scripts/check-hadolint.sh` | gate | Security | none needed |
| `scripts/check-haproxy-cfg.sh` | gate | Load-Balancers, Security | none needed |
| `scripts/check-host-settings.sh` | gate | Host-Settings | none needed |
| `scripts/check-http-auth.sh` | gate | none | add wiki line |
| `scripts/check-http-timeouts.sh` | gate | none | add wiki line |
| `scripts/check-image-digests.sh` | gate | none | add wiki line |
| `scripts/check-image-drift.sh` | gate | none | add wiki line |
| `scripts/check-image-tags.sh` | gate | Security | none needed |
| `scripts/check-js-lint.sh` | gate | Security | none needed |
| `scripts/check-json-responses.sh` | gate | none | add wiki line |
| `scripts/check-kanban-genesis.sh` | gate | Service-Kanban-App | none needed |
| `scripts/check-log-sources.sh` | gate | none | add wiki line |
| `scripts/check-manifest-yaml.sh` | gate | none | add wiki line |
| `scripts/check-mod-integrity.sh` | gate | none | add wiki line |
| `scripts/check-module-binds.sh` | gate | none | add wiki line |
| `scripts/check-netip.sh` | gate | none | add wiki line |
| `scripts/check-no-client-golang.sh` | gate | none | add wiki line |
| `scripts/check-nosec-ratchet.sh` | gate | none | add wiki line |
| `scripts/check-perf-regression.sh` | gate | Benchmarks | none needed |
| `scripts/check-pq-kex.sh` | gate | Post-Quantum-Key-Exchange | none needed |
| `scripts/check-prometheus-rules.sh` | gate | Alerting | none needed |
| `scripts/check-python-audit.sh` | gate | SBOM, Security | none needed |
| `scripts/check-python-syntax.sh` | gate | none | add wiki line |
| `scripts/check-ruff.sh` | gate | none | add wiki line |
| `scripts/check-runbooks.py` | gate | Runbooks | none needed |
| `scripts/check-runbooks.sh` | gate | Runbooks, Zhen-AI | none needed |
| `scripts/check-sbom-trend.sh` | gate | none | add wiki line |
| `scripts/check-sbom-version.sh` | gate | Releases | none needed |
| `scripts/check-scrape-publish.sh` | gate | Port-Registry | none needed |
| `scripts/check-secrets-baseline.sh` | gate | Security | none needed |
| `scripts/check-spdx.sh` | gate | SBOM | none needed |
| `scripts/check-systemd-units.sh` | gate | Security | none needed |
| `scripts/check-timeline-freshness.sh` | gate | Timeline | none needed |
| `scripts/check-tmp-log-baseline.sh` | gate | none | add wiki line |
| `scripts/check-tool-freshness.sh` | gate | Security | none needed |
| `scripts/check-trivy-misconfig.sh` | gate | none | add wiki line |
| `scripts/check-unit-binaries.sh` | gate | Security | none needed |
| `scripts/check-unit-ceilings.sh` | gate | Profiling | none needed |
| `scripts/check-unit-flags.sh` | gate | none | add wiki line |
| `scripts/check-unit-start-limits.sh` | gate | Systemd-Units | none needed |
| `scripts/check-usage-errors.sh` | gate | none | add wiki line |
| `scripts/check-version-flag.sh` | gate | none | add wiki line |
| `scripts/check-well-lint.sh` | gate | none | add wiki line |
| `scripts/check-well-migrations.sh` | gate | none | add wiki line |
| `scripts/check-zhen-mcp.sh` | gate | Zhen-AI | none needed |
| `scripts/ci-install-tools.sh` | operator | Releases, SBOM | none needed |
| `scripts/ci-local.sh` | operator | Benchmarks, eBPF-Programs | none needed |
| `scripts/ci-python-lock.sh` | operator | none | add wiki line |
| `scripts/clean-artifacts.sh` | operator | none | add wiki line |
| `scripts/coding-gate-agent.sh` | operator | none | add wiki line |
| `scripts/coding-gate-ci.sh` | operator | none | add wiki line |
| `scripts/coding-gate-probe.sh` | operator | none | add wiki line |
| `scripts/compliance/check-edge-tls.sh` | operator | none | add wiki line |
| `scripts/compliance/check-service-auth.sh` | operator | none | add wiki line |
| `scripts/compliance/import-frameworks.py` | operator | none | add wiki line |
| `scripts/compliance/sign-attestation.sh` | operator | Compliance-Paperwork | none needed |
| `scripts/coverage-trend.sh` | operator | none | add wiki line |
| `scripts/cross-compile.sh` | operator | none | add wiki line |
| `scripts/cuirass-lxd-trust.sh` | operator | Containers | none needed |
| `scripts/db-backup.sh` | operator | The-Well, Well-Backups | none needed |
| `scripts/demo-kanban.sh` | operator | none | add wiki line |
| `scripts/demo-reset.sh` | operator | Demo-Script | none needed |
| `scripts/demo-start.sh` | operator | Demo-Script, Service-Auth | none needed |
| `scripts/dep-diff.sh` | operator | none | add wiki line |
| `scripts/deploy-alpha.sh` | operator | none | add wiki line |
| `scripts/drift-detect.sh` | operator | none | add wiki line |
| `scripts/east-bootstrap.sh` | operator | none | add wiki line |
| `scripts/fast_inject.py` | operator | none | add wiki line |
| `scripts/firewall/firewall-health-check.sh` | operator | none | add wiki line |
| `scripts/firewall/install-bird.sh` | operator | none | add wiki line |
| `scripts/firewall/install-frr.sh` | operator | none | add wiki line |
| `scripts/firewall/setup-ipfire.sh` | operator | none | add wiki line |
| `scripts/firewall/setup-opnsense.sh` | operator | none | add wiki line |
| `scripts/flamegraph.sh` | operator | Profiling | none needed |
| `scripts/gen-dev-certs.sh` | operator | Post-Quantum-Key-Exchange, Service-Akira (+1) | none needed |
| `scripts/gen-dev-keys.sh` | operator | Service-Auth, Wotan-Topic-Signing | none needed |
| `scripts/gen-haproxy-dev-cert.sh` | operator | Load-Balancers | none needed |
| `scripts/gen-man.sh` | operator | none | add wiki line |
| `scripts/gen-prom-keep-list.py` | operator | Alerting | none needed |
| `scripts/generate-sbom.sh` | operator | Releases, SBOM | none needed |
| `scripts/git-hooks/post-commit` | operator | Zhen-AI | none needed |
| `scripts/git-hooks/pre-commit` | operator | Developer-Guide, Project-Structure | none needed |
| `scripts/host/nft-apply-confirm.sh` | operator | Host-Settings, Security | none needed |
| `scripts/install-chaos-east.sh` | operator | Chaos-Engineering, Systemd-Units | none needed |
| `scripts/install-chaos-west.sh` | operator | Chaos-Engineering, Systemd-Units | none needed |
| `scripts/install-nerdctl.sh` | operator | none | add wiki line |
| `scripts/install-prerequisites.sh` | operator | Host-Settings, Quick-Start | none needed |
| `scripts/kanban-git-link.sh` | operator | Service-Kanban-App | none needed |
| `scripts/kanban/genesis-timestamps.py` | operator | Service-Kanban-App | none needed |
| `scripts/kanban/render-genesis-sql.py` | operator | Service-Kanban-App | none needed |
| `scripts/kingdom-health.sh` | operator | none | add wiki line |
| `scripts/kingdom-shutdown.sh` | operator | Alerting | none needed |
| `scripts/kingdom-startup.sh` | operator | Alerting, Chaos-Engineering (+1) | none needed |
| `scripts/lab-two-node.sh` | operator | none | add wiki line |
| `scripts/lich-cargo-fuzz.sh` | operator | LICH-Campaigns | none needed |
| `scripts/live-path-inventory.sh` | operator | none | add wiki line |
| `scripts/load-ebpf.sh` | operator | none | add wiki line |
| `scripts/load_rom.py` | operator | none | add wiki line |
| `scripts/load_rom_fast.py` | operator | Doom-over-IPv6 | none needed |
| `scripts/logstore-measure.sh` | operator | Log-Aggregation, Systemd-Units | none needed |
| `scripts/logstore-reckon.py` | operator | Log-Aggregation | none needed |
| `scripts/modules/module-fragments.sh` | operator | none | add wiki line |
| `scripts/monitoring/boot-event.sh` | operator | Alerting, Systemd-Units | none needed |
| `scripts/monitoring/ci-status-textfile.sh` | operator | Alerting | none needed |
| `scripts/monitoring/heartbeat.sh` | operator | Alerting | none needed |
| `scripts/monitoring/host-state-textfile.sh` | operator | Host-Settings, Systemd-Units | none needed |
| `scripts/monitoring/maintenance.sh` | operator | Alerting, Game-Days | none needed |
| `scripts/monitoring/module-listener-guard.sh` | operator | none | add wiki line |
| `scripts/monitoring/planned-stop.sh` | operator | Alerting | none needed |
| `scripts/monitoring/platform-schedule.sh` | operator | none | add wiki line |
| `scripts/node-guard.sh` | operator | none | add wiki line |
| `scripts/pihole-add-list.sh` | operator | Host-Settings | none needed |
| `scripts/pihole-history.sh` | operator | Log-Aggregation | none needed |
| `scripts/pre-commit.sh` | operator | none | add wiki line |
| `scripts/pre-flight-check.sh` | operator | none | add wiki line |
| `scripts/probe-auto-grade.py` | operator | none | add wiki line |
| `scripts/probe-sanitization-check.sh` | operator | none | add wiki line |
| `scripts/probe-tls-groups.sh` | operator | Post-Quantum-Key-Exchange | none needed |
| `scripts/qa-smoke.sh` | operator | none | add wiki line |
| `scripts/read_cpu.py` | operator | none | add wiki line |
| `scripts/relicense-mit-to-gpl.sh` | operator | none | add wiki line |
| `scripts/reset_cpu.py` | operator | none | add wiki line |
| `scripts/resign-unpushed.py` | operator | Agent-Operating-Procedure | none needed |
| `scripts/ringbuf-drain.py` | operator | none | add wiki line |
| `scripts/rolling-previous.sh` | operator | none | add wiki line |
| `scripts/routing/select-routing.sh` | operator | none | add wiki line |
| `scripts/run-coding-gate-via-webui.sh` | operator | none | add wiki line |
| `scripts/run-coding-gate.sh` | operator | none | add wiki line |
| `scripts/run-runbook.py` | operator | Releases, Runbooks (+3) | none needed |
| `scripts/runbook-manifest.sh` | operator | Runbooks, Zhen-AI | none needed |
| `scripts/session-to-runbook.py` | operator | none | add wiki line |
| `scripts/setup-host.sh` | operator | none | add wiki line |
| `scripts/shield-monitor.sh` | operator | none | add wiki line |
| `scripts/spdx-backfill.sh` | operator | SBOM | none needed |
| `scripts/steel-echo-image.sh` | operator | Steel-Echo | none needed |
| `scripts/stress-cannon.py` | operator | none | add wiki line |
| `scripts/svc_token.py` | operator | Service-Auth, Service-Kanban-App | none needed |
| `scripts/sweep-ledger-step1.py` | operator | none | add wiki line |
| `scripts/sweep/unreached-evidence.sh` | operator | none | add wiki line |
| `scripts/switch-model.sh` | operator | ADR-060-Zhenai-Multi-Model-Selector, Linux-on-UPC (+3) | none needed |
| `scripts/sync-wiki.sh` | operator | Project-Structure | none needed |
| `scripts/three-crowns.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase0-prerequisites.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase1-tunnel.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase2-docker.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase3-lxd.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase3b-nixos-lxd.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase4-containerd.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase5-tunnel-teardown.sh` | operator | none | add wiki line |
| `scripts/three-crowns/phase6-final-sweep.sh` | operator | none | add wiki line |
| `scripts/timeline-mirrors.py` | operator | Timeline | none needed |
| `scripts/two-brains/author-runbook.sh` | operator | none | add wiki line |
| `scripts/two-brains/run-fixture.sh` | operator | Zhen-Routing | none needed |
| `scripts/two-brains/split.py` | operator | none | add wiki line |
| `scripts/two-brains/thresholds.py` | operator | Zhen-Routing | none needed |
| `scripts/unload-ebpf.sh` | operator | none | add wiki line |
| `scripts/upc-mbc-patch-opcode.py` | operator | none | add wiki line |
| `scripts/upc-netns.sh` | operator | Linux-on-UPC, Profiling (+1) | none needed |
| `scripts/upc-regression.sh` | operator | Linux-on-UPC, UPC-Cartridges (+1) | none needed |
| `scripts/upc-rung0-assert.awk` | operator | none | add wiki line |
| `scripts/upc-tty-split.py` | operator | none | add wiki line |
| `scripts/vault-to-runway.py` | operator | none | add wiki line |
| `scripts/verdicts-record.sh` | operator | none | add wiki line |
| `scripts/verify-binding-rune.sh` | operator | ADR-010-Sealed-Cask-Deployment | none needed |
| `scripts/verify-gpl-boundary.sh` | operator | SBOM | none needed |
| `scripts/verify-mbc-pipeline.sh` | operator | none | add wiki line |
| `scripts/verify-release.sh` | operator | Releases, Security | none needed |
| `scripts/vor-export-source.sh` | operator | Zhen-AI | none needed |
| `scripts/vor-fuzz.sh` | operator | none | add wiki line |
| `scripts/vulnmatch-fixture.sh` | operator | SBOM, Security | none needed |
| `scripts/wait-for-healthy.sh` | operator | none | add wiki line |
| `scripts/well-reconcile-kanban.sh` | operator | none | add wiki line |
| `scripts/well/batch-apply.sh` | operator | Service-Auth, The-Well | none needed |
| `scripts/well/batch-rollback.sh` | operator | The-Well | none needed |
| `scripts/well/index-drops-after-2026-11-15.sh` | operator | The-Well | none needed |
| `scripts/well/make-tls.sh` | operator | The-Well | none needed |
| `scripts/well/offsite-check.sh` | operator | none | add wiki line |
| `scripts/well/offsite-copy.sh` | operator | none | add wiki line |
| `scripts/well/restore-drill.sh` | operator | none | add wiki line |
| `scripts/well/set-role-limits.sh` | operator | Host-Settings, The-Well | none needed |
| `scripts/well/set-role-passwords.sh` | operator | Service-Auth, The-Well (+1) | none needed |
| `scripts/well/well_lint.py` | operator | none | add wiki line |
| `scripts/well/zhen-copy-rows.sh` | operator | none | add wiki line |
| `scripts/well/zhen_schema_diff.py` | operator | none | add wiki line |
| `scripts/wireguard/psk-gate.sh` | operator | Overlay-Network | none needed |
| `scripts/wireguard/rotate-psk.sh` | operator | Overlay-Network, Post-Quantum-Key-Exchange | none needed |
| `scripts/wireguard/setup-wireguard.sh` | operator | none | add wiki line |
| `scripts/wireguard/verify-wireguard.sh` | operator | none | add wiki line |
| `scripts/wotan-host-config.py` | operator | Wotan-Topic-Signing | none needed |
| `scripts/write-zhen-app-env.sh` | operator | none | add wiki line |
| `scripts/zhen-agent-preflight.sh` | operator | none | add wiki line |
| `scripts/zhen-lan-proof.sh` | operator | none | add wiki line |
| `scripts/zhen-mcp.sh` | operator | Zhen-AI | none needed |
| `scripts/zhen-morning-report.sh` | operator | Zhen-Agents | none needed |
| (65 test files) | test | 15 named on a page | out of scope: not run by a person as a tool |
| (14 lib files) | lib | 6 named on a page | out of scope: not run by a person as a tool |
| (9 data files) | data | 0 named on a page | out of scope: not run by a person as a tool |

### Top-level entries

| Directory | Wiki | Action |
|---|---|---|
| `.claude/` | Zhen-AI | add row to Project-Structure |
| `.github/` | Developer-Guide, Releases, SBOM (+1) | add row to Project-Structure |
| `LICENSES/` | Project-Structure(row), Developer-Guide, Observability-Backends (+2) | none needed |
| `api/` | Project-Structure(row) | none needed |
| `arch/` | Project-Structure(row) | none needed |
| `cartridges/` | UPC-Cartridges | add row to Project-Structure |
| `cmd/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, ADR-065-Aya-Major-Version-Migration (+44) | none needed |
| `compliance/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, ADR-Index (+9) | none needed |
| `config/` | Project-Structure(row), Alerting, Fae-Chamber-Contracts (+7) | none needed |
| `configs/` | Project-Structure(row), Benchmarks, Chaos-Engineering (+11) | none needed |
| `containers/` | Project-Structure(row), Containers | none needed |
| `crates/` | Project-Structure(row), ADR-072-Boot-Magic-Byte-Ordering, Architecture (+16) | none needed |
| `dashboard/` | Project-Structure(row), Linux-on-UPC, Service-Discovery (+1) | none needed |
| `db/` | Project-Structure(row), ADR-Index, Host-Settings (+6) | none needed |
| `debian/` | Project-Structure(row), Releases | none needed |
| `demos/` | Project-Structure(row), Demo-Script, Doom-on-UPC (+2) | none needed |
| `deploy/` | Project-Structure(row), Alerting, Chaos-Engineering (+18) | none needed |
| `docker/` | Project-Structure(row), Chaos-Engineering, Containers (+6) | none needed |
| `docs/` | Project-Structure(row), ADR-001-Gnostic-State-Management, ADR-002-Kingdom-Naming-Convention (+143) | none needed |
| `doom/` | Architecture, Runbooks, SBOM | add row to Project-Structure |
| `drivers/` | Project-Structure(row) | none needed |
| `ebpf/` | Project-Structure(row), ADR-065-Aya-Major-Version-Migration, ADR-Index (+18) | none needed |
| `eval/` | Project-Structure(row), ADR-Index, Runbooks (+3) | none needed |
| `ietf-submission/` | Project-Structure(row) | none needed |
| `kanban/` | Project-Structure(row), Service-Kanban-App | none needed |
| `lxd/` | Project-Structure(row), Architecture, Chaos-Engineering (+1) | none needed |
| `man/` | none | add row to Project-Structure |
| `modules/` | Zhen-AI | add row to Project-Structure |
| `monitoring/` | Project-Structure(row), Alerting, Chaos-Engineering (+12) | none needed |
| `nix/` | Project-Structure(row), Architecture, Containers (+3) | none needed |
| `nixos/` | Project-Structure(row), Containers | none needed |
| `notebooks/` | Project-Structure(row), SBOM | none needed |
| `observability/` | Alerting, Host-Settings, Microservices (+3) | add row to Project-Structure |
| `pkg/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, ADR-015-Go-Fiber-HTTP-Layer (+47) | none needed |
| `proto/` | Project-Structure(row) | none needed |
| `raft/` | Project-Structure(row), SBOM, Service-Auth (+1) | none needed |
| `references/` | Project-Structure(row), ADR-065-Aya-Major-Version-Migration, ADR-067-MBC-ISA-V2-and-UPC-ABI-V1 (+15) | none needed |
| `routing/` | Project-Structure(row), Host-Settings, Overlay-Network | none needed |
| `runbooks/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, ADR-022-Pihole-Docker-to-LXD (+14) | none needed |
| `sbom/` | Project-Structure(row), Releases, SBOM | none needed |
| `sbom-results/` | Project-Structure(row), SBOM | none needed |
| `scripts/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, ADR-065-Aya-Major-Version-Migration (+43) | none needed |
| `security/` | ADR-066-Tonic-Minor-Bump-Trace-Collector, Runbooks | add row to Project-Structure |
| `services/` | Project-Structure(row), ADR-Index, Chaos-Engineering (+15) | none needed |
| `skills/` | Project-Structure(row), Skills-Index | none needed |
| `systemd/` | Project-Structure(row), Chaos-Engineering | none needed |
| `tests/` | Project-Structure(row), ADR-010-Sealed-Cask-Deployment, Host-Settings (+5) | none needed |
| `tomb/` | Project-Structure(row), ADR-057-Unheaded-Source-Code-Indexing, ADR-060-Zhenai-Multi-Model-Selector (+5) | none needed |
| `tools/` | Project-Structure(row), Microservices, SBOM | none needed |
| `wiki/` | Project-Structure(row), Protocol-Specs, Systemd-Units (+1) | none needed |

## Re-audit after the fill (2026-11-22)

Same reader, same rules, on `docs/doc-coverage` after commits `839d324e` (Go), `cca98446` (Rust), `f6dc6788`, `05596615`, `9ae91e36` (wiki). The tables above are the starting state and are not edited; this section is the state after.

| Kind | Parts | Documented before | Documented after (reader) | After, checked by hand | Still missing |
|---|---|---|---|---|---|
| go | 255 | 225 (7 of them weak) | 254, none weak | 254 | 0 (the one left is `eval/two-brains/live/go-nil-map`, an eval fixture) |
| rust-doc | 40 | 36 | 40 | 40 | 0 |
| rust-wiki | 40 | 33 | 37 | 40 | 0 |
| top-dirs | 50 | 42 | 50 | 50 | 0 |
| cmd | 60 | 57 | 58 | 60 | 0 |
| services | 16 | 16 | 16 | 16 | 0 |
| modules | 1 | 1 | 1 | 1 | 0 |
| units | 92 | 77 | 90 | 92 | 0 |
| scripts (operator and gate) | 236 | 111 | 236 | 236 | 0 |

"Checked by hand": parts the wiki now names in a form the reader does not match. `cmd/ebpf-collector/collector` and `ebpf/common` (named as `collector` and `common`, generic names the reader matches only by path), `crates/upc-api/fuzz` (named as `upc-api/fuzz`), `cmd/gjallarhorn-listener` (Microservices: `gjallarhorn-sender` / `-listener`), `cmd/tools` (Microservices: `tools/`), `deploy/systemd/user-1000.slice.d/50-unheaded-caps.conf` (Security: `deploy/systemd/user-1000.slice.d/`), `deploy/systemd/user/unheaded-git-mirror.service` (Well-Backups names its timer and the service's header).

What changed, by kind:

- **Go**: 29 packages got a package or command doc (two as `doc.go` beside generated protobuf code). Seven weak docs were fixed: the SPDX line split from the doc (huginn, demo-trace-injector, upc-tty-bridge, enkrateia, gjallarhorn, gungnir, Wotan's `signing`), and per-file comments sitting on the package clause moved off it (16 files of `trace-collector-go`, two of `heimdall-daemon`). `akira`, `heimdall-daemon` and `upc-tty-bridge` had a describing comment one blank line above `package`; the blank line is gone and the first line reads "Command NAME ...". `gofmt` rewrapped `akira`'s usage block into a code block.
- **Rust**: `//!` docs on `upc-bootctl`, the four `lich-fuzz` targets, three `monad-mbc` fuzz targets and the LICH-015 harness.
- **Wiki, parts**: Microservices +19 binaries; Rust-Components +objtrust, svc-token, upc-cartridge, the fuzz crates, the ebpf-collector workspace, the LICH-015 harness; Systemd-Units +the health sweep, IOC audit, module guard, Zhen switch, its module and backup units, llama, the slices; Project-Structure +cartridges, doom, modules, observability, man, security, `.github`, `.claude`, the root files.
- **Wiki, generated and config files**: Compliance-Paperwork (framework catalogs, `baseline.yaml`, the Ansible output, the two evidence scripts), Runbooks (`MANIFEST.sha256`, `rolling-previous.sh`), Service-Wotan (proto and `generate.sh`), Project-Structure (`man/`, `.pb.go`), Log-Aggregation (ClickHouse schema and drop-ins), Observability-Backends (Grafana provisioning), Port-Registry (`configs/port-registry.yaml`).
- **Wiki, scripts**: 30 gate scripts and 18 developer tools on Developer-Guide; the rest beside their siblings on Well-Backups, The-Well, UPC-Overview, eBPF-Programs, Zhen-AI, Overlay-Network, Containers, Host-Settings.
- **Wiki, corrected**: Microservices no longer lists the Rust `trace-collector` (retired, `363f17bd`, DP-210) or `ebpf-exporter` (deleted, `94b919b8`, DP-401.5); Project-Structure's `raft/` row described a Python MCP server and scheduler that are retired.

**Correction to the gap list above.** Its row for `references/timeline.{json,yaml,toml}` is wrong: the mirrors are written by `scripts/timeline-mirrors.py --write` (`--check` in CI), not by timeguru, and `wiki/Timeline.md` already said so. No change was needed.

**Could not determine.**

- The command that wrote `proto/unheaded/v1/*.pb.go`: no script or Makefile target in the tree; only the tool versions in the generated headers (`protoc-gen-go` v1.36.11, `protoc-gen-go-grpc` v1.6.1, protoc 3.21.12).
- Whether anything outside the repository reads `configs/port-registry.yaml`; nothing inside it does.
- Whether the older host scripts (`scripts/bare-metal/*`, `setup-host.sh`, `east-bootstrap.sh`, `deploy-alpha.sh`, `three-crowns*`) still run on today's hosts. They are described from their text and were not run.

**Looked wrong, not changed** (outside this task, or a dated record):

- `scripts/README.md` describes `init-project.sh`, which does not exist, and marks `setup-host.sh`, `deploy-alpha.sh`, `load-ebpf.sh` and `demo-kanban.sh` "To Implement"; all four exist.
- `crates/lich-fuzz/Cargo.toml`'s header says the crate is at `ebpf/fuzz/` and to run `cargo +nightly fuzz -C ebpf/fuzz run`; it is at `crates/lich-fuzz/`.
- `cmd/tools/README.md` names `pkg/anamnesis/` as a source of `anamnesis-lite`; there is no such package.
- `scripts/verify-mbc-pipeline.sh` calls `scripts/doom-ring.sh`, which is gone, and with `load_rom.py`, `read_cpu.py` and `reset_cpu.py` uses the `doom-ring` pins the Doom runner made before it moved to `unheaded-doom`.
- `scripts/probe-auto-grade.py` reads fixed paths under one home directory.
- `scripts/east-bootstrap.sh` assumes NixOS on east; east runs Ubuntu.
- `lxd/hosts/INDEX.md` says "Generated: February 26, 2026" and gives a base directory from another machine's session sandbox.
- `docs/LIVE-PATHS.md` (generated) marks `services/timeguru` SUPERVISED, but that directory's own `main` is built by nothing: the Makefile, the Dockerfile and `scripts/build-binaries.sh` all build `services/timeguru/cmd/timeguru`.
- `cmd/upc-bootctl/src/main.rs`'s older comment still says "skeleton with `validate` and `boot --dry-run`"; `boot` and `console` are implemented.
- `.claude/settings.local.json` is tracked (permissions only, no credentials seen).
- `scripts/pre-commit.sh` and `scripts/git-hooks/pre-commit` are two different pre-commit hooks; `make install-hooks` uses the second.
- The root `CLAUDE.md` project tree lists `ebpf/packet_marker/`, `flow_tracker/`, `latency_probe/`; the directories are `packet-marker`, `flow-tracker`, `latency-probe`.
- Several Go packages carry more than one doc comment, which `go doc` joins: `cmd/unheaded-cli/cmd` (11 files), `pkg/wotan-client` (5), `cmd/kanban-app`, `pkg/deploy/strategy`, `pkg/ebpf` and `tests/e2e` (4 each), `cmd/unheaded-cli/output` and `tomb/lich/harnesses` (3), and two each in `cmd/dashboard-backend/internal/ebpf`, `pkg/auth`, `pkg/deploy/rollback`, `pkg/health`, `services/yaldabaoth`.

**Gates.** The quiet flag was up for the whole pass, so no build, test or lint was started by hand. The repository's pre-commit hook (`scripts/git-hooks/pre-commit`) ran `gofmt` and `go vet` on the packages with staged Go changes when `839d324e` was committed, and both passed after `gofmt -w cmd/akira/main.go`; it also checks rustfmt on staged Rust files and did not object to `cca98446` (it skips with a warning where rustfmt is missing). Not run: `go build ./...`, `go vet ./pkg/... ./cmd/... ./services/...`, cargo builds and clippy for the Rust files, `scripts/check-spdx.sh`.
