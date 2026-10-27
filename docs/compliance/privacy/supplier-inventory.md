<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Supplier Inventory

Every outside party this project depends on or sends data to, 2026-10-27. Sources: `scripts/lib/tool-pins.sh`, `scripts/lib/model-pins.sh`, `THIRD_PARTY.md`, the compose files (`docker-compose.yml`, `monitoring/docker-compose.yml`), the CI workflows. Supports 800-53 SR-2, SR-3; ISO 27001:2022 A.5.19-A.5.22; SOC 2 CC9.2. No supplier is under contract; each is used on its public terms.

| Supplier | What it provides | Data it receives from this project | Integrity control | Exit |
|---|---|---|---|---|
| GitHub | source hosting, Actions CI, release attestations, security advisories | source, commit authorship, CI logs | signed commits; pinned action SHAs; keyless signed releases | the east git mirror holds every ref |
| Anthropic | Claude API (zhen-claude routes); Claude Code sessions | prompts after T0/T1/T2 redaction; Claude Code sessions see the working tree | spend cap, write confirmation gate (ADR-106); no training on Claude outputs without written authorization | local models (`llama-server`) |
| Canonical | Ubuntu on west and east; apt archive; security updates | package requests (host address) | apt signatures | another distribution (Yggdrasil track) |
| Docker Hub (and the image publishers) | images: postgres, clickhouse-server, prometheus, alertmanager, grafana, loki, victoria-metrics, vector, haproxy, nginx, traefik, coredns, alpine | pulls (host address) | versions pinned (ADR-102); first-party images built here | rebuild from source; other registries |
| Go module proxy and checksum DB (proxy.golang.org, sum.golang.org) | Go dependencies | module requests | `go.sum`; govulncheck | `GOPROXY=direct` |
| crates.io | Rust dependencies | crate requests | `Cargo.lock`; cargo-audit | vendoring |
| PyPI | Python dependencies (Zhen, RAFT, CI tools) | package requests | pinned versions; pip-audit | vendoring |
| GitHub release downloads (tool publishers) | gitleaks, syft, grype, cosign, helm, hadolint, kubeconform, oxlint | downloads | sha256 pins in `tool-pins.sh` | pinned copies |
| Vector (Datadog), apt.vector.dev | the `.deb` installed on east | download | sha256 pin (`VECTOR_DEB_*`) | the compose image |
| Model publishers | GGUF and embedder files under `/var/zhen/models` | downloads | sha256 pins in `model-pins.sh`; `switch-model.sh` refuses a mismatch | other open-weight models |
| CDNs (jsDelivr and similar) | xterm.js, xterm-addon-fit, SheetJS loaded by two pages | page loads from the browser | Subresource Integrity hashes | vendor the files |
| Sigstore (Fulcio, Rekor) | keyless release signing, transparency log | the workflow identity of each release | public log | signing key (S-IRON-GATE Phase 13) |
| CISA | the KEV feed (`cmd/kev-poller`) | feed requests | feed freshness alerts | none needed |
| Mail relay (configured in `/etc/unheaded/alertmanager.env`, outside git) | delivery of alert mail | alert text, the operator's address | TLS to the relay | another relay |
| Internet service provider | uplink for west and east | all outbound traffic metadata | edge firewall drops unsolicited inbound (FND-004) | n/a |

Review with the policy each year, and when a supplier is added: a new image, tool, model source, or any service that receives data.
