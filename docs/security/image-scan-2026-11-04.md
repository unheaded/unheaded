<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# Image scan, 2026-11-04

S-SUPPLY-FIRST Phase 6. What the pinned third-party images carry at HIGH and CRITICAL with a fix available. This measures; it does not close DP-722 for images.

- **Tool**: trivy 0.76.1, the pinned image from `scripts/lib/tool-pins.sh`, `--scanners vuln --severity HIGH,CRITICAL --ignore-unfixed`.
- **DB**: trivy-db schema 2, updated 2026-11-04T13:16:53Z.
- **KEV**: CISA catalogue 2026.11.03. **One CRITICAL is on it: CVE-2026-111497 (brotli) in the open-webui digest this tree pins**, see below. One HIGH is too: CVE-2023-4911 (glibc) in `grafana/promtail:2.9.0`, gone with the move to 2.9.17. No other CRITICAL or HIGH in any scan is on it.
- **Counts** are raw scanner rows: a CVE found in two packages or two binaries counts twice.
- Nothing was started, restarted or exercised live. The stack is down.

## Results

29 images scanned, none failed.

| Image | Critical | High | Result |
|---|---|---|---|
| `alpine:3.24` | 0 | 0 | clean |
| `busybox:1.36.1` | 0 | 0 | clean |
| `clickhouse/clickhouse-server:24.12.6.70` | 0 | 5 | waits |
| `coredns/coredns:1.12.0` | 2 | 58 | moved to 1.12.4 |
| `debian:bookworm-slim` | 0 | 0 | clean |
| `gcr.io/distroless/static-debian12:nonroot` | 0 | 0 | clean |
| `ghcr.io/huggingface/text-embeddings-inference:cpu-1.5` | 8 | 49 | waits |
| `ghcr.io/open-webui/open-webui:main` | 4 | 28 | waits; not the pinned digest, see below |
| `golang:1.27.3-alpine3.24` | 0 | 0 | clean |
| `golang:1.27.3-bookworm` | 0 | 0 | clean |
| `grafana/grafana:10.3.3` | 9 | 139 | waits |
| `grafana/grafana:11.5.1` | 9 | 140 | waits |
| `grafana/loki:2.9.4` | 3 | 63 | moved to 2.9.17 |
| `grafana/loki:3.0.0` | 3 | 62 | moved to 3.0.1 |
| `grafana/promtail:2.9.0` | 10 | 118 | moved to 2.9.17 |
| `grafana/promtail:2.9.4` | 10 | 109 | moved to 2.9.17 |
| `haproxy:2.9.15-alpine` | 2 | 23 | waits |
| `nginx:1.25.5-alpine` | 3 | 17 | waits |
| `postgres:16.15-alpine` | 1 | 25 | waits (never unattended) |
| `prom/alertmanager:v0.34.1` | 0 | 12 | waits (tag too young) |
| `prom/node-exporter:v1.8.0` | 2 | 50 | moved to v1.8.2 |
| `prom/prometheus:v2.50.1` | 8 | 114 | waits |
| `prom/prometheus:v3.15.0` | 0 | 10 | moved to v3.15.1 |
| `python:3.12.15-slim` | 0 | 0 | clean |
| `qdrant/qdrant:v1.19.2` | 2 | 11 | waits |
| `timberio/vector:0.43.1-debian` | 7 | 38 | waits |
| `traefik:v3.3.7` | 4 | 76 | waits |
| `ubuntu:24.04` | 0 | 0 | clean |
| `victoriametrics/victoria-metrics:v1.108.1` | 3 | 59 | waits |

## What moved

The rule (plan Step 65): same minor series, tag at least 14 days old, removes a CRITICAL or HIGH finding.

| Image | From | To | Tag created | Before (C/H) | After (C/H) | Removed | Offline check |
|---|---|---|---|---|---|---|---|
| `coredns/coredns` | 1.12.0 | 1.12.4 | 2025-09-09 | 2/58 | 2/66 | 7 HIGH | starts on `config/coredns/Corefile` with the compose hardening, no network |
| `grafana/loki` | 3.0.0 | 3.0.1 | 2024-08-09 | 3/62 | 4/83 | 1 CRITICAL, 1 HIGH | `-verify-config` on `monitoring/loki/loki.yaml` |
| `grafana/loki` | 2.9.4 | 2.9.17 | 2025-11-07 | 3/63 | 4/74 | 1 CRITICAL, 13 HIGH | `-verify-config` on `tomb/dark-mirror/loki-config.yml` |
| `grafana/promtail` | 2.9.0 | 2.9.17 | 2025-11-07 | 10/118 | 2/55 | 8 CRITICAL, 66 HIGH | `-check-syntax` on `docker/telemetry/promtail/promtail.yml` |
| `grafana/promtail` | 2.9.4 | 2.9.17 | 2025-11-07 | 10/109 | 2/55 | 8 CRITICAL, 57 HIGH | `-check-syntax` on `tomb/dark-mirror/promtail-config.yml` |
| `prom/node-exporter` | v1.8.0 | v1.8.2 | 2024-07-14 | 2/50 | 1/50 | 1 CRITICAL | `--version` only |
| `prom/prometheus` | v3.15.0 | v3.15.1 | 2026-10-21 | 0/10 | 0/0 | all | `check-prometheus-rules.sh`: 12 rule files parse, 12 test files pass |

"Removed" counts distinct (CVE, package) pairs, so it does not subtract from the raw columns.

**Three of these raise the raw count, and were taken anyway:**

- `grafana/loki` 3.0.0 and 2.9.4 run on Alpine 3.18.5, past end of life, where the scanner reports 4 OS findings against OpenSSL 3.1.4. 3.0.1 (Alpine 3.20.2, OpenSSL 3.3.1) reports 28 and 2.9.17 (Alpine 3.22.2, OpenSSL 3.5.4) reports 26. The new rows are all Alpine packages, including 2 CRITICAL (CVE-2026-31789, `libcrypto3` and `libssl3`). The likely reading is that the old base stopped receiving advisories, so its low count is blindness, not health. That is an inference from the versions; the old images were not tested against each new CVE.
- `coredns/coredns` 1.12.4 adds 11 rows on `github.com/coredns/coredns` that 1.12.0 cannot show, because its binary reports no module version. 1.12.4 reports a pseudo-version, which sorts below every release. By the scanner's own fixed versions, 2 of the 11 are false (fixed in 1.11.0 and 1.12.2), and 9 are fixed only in 1.14.x, so they apply to 1.12.0 just the same and are simply invisible there. The other 4 new rows are new dependencies (`dd-trace-go`, `otel`, `otel/sdk`).

## What waits

| Image | C/H | Why it waits |
|---|---|---|
| `postgres:16.15-alpine` | 1/25 | Never bumped unattended. ADR-102: backup and restore drill first (S-E1-LIVE). |
| `grafana/grafana:11.5.1` | 9/140 | The plan names Grafana as one to leave. 11.5.10 exists and scans 9/150, no gain. |
| `grafana/grafana:10.3.3` | 9/139 | Same. 10.3.12 scans 8/141. |
| `clickhouse/clickhouse-server:24.12.6.70` | 0/5 | Last tag of 24.12. Minor move of a data store. |
| `victoriametrics/victoria-metrics:v1.108.1` | 3/59 | Last tag of 1.108. Minor move of a data store. |
| `qdrant/qdrant:v1.19.2` | 2/11 | Last tag of 1.19. Minor move of a data store. |
| `nginx:1.25.5-alpine` | 3/17 | Last tag of 1.25, on Alpine 3.19.1 (past end of life). Minor move. |
| `haproxy:2.9.15-alpine` | 2/23 | Last tag of 2.9, on Alpine 3.21.3 (past end of life). Minor move. |
| `traefik:v3.3.7` | 4/76 | Last tag of 3.3, on Alpine 3.21.3 (past end of life). Minor move. |
| `timberio/vector:0.43.1-debian` | 7/38 | Last tag of 0.43. Minor move. |
| `prom/prometheus:v2.50.1` | 8/114 | Last tag of 2.50 (`tomb/dark-mirror`). Minor or major move. |
| `prom/alertmanager:v0.34.1` | 0/12 | v0.34.2 was created 2026-10-29, six days old. Eligible from 2026-11-12; not scanned yet. |
| `ghcr.io/huggingface/text-embeddings-inference:cpu-1.5` | 8/49 | No later `cpu-1.5.x` tag. Minor move. |
| `ghcr.io/open-webui/open-webui` | 14/137 | **Urgent: a CRITICAL on KEV.** See below. |

Moved images still carry findings and need a minor move to shed more: `coredns` 2/66 (nine of its rows are fixed only in 1.14.x), `loki` 3.0.1 4/83, `loki` 2.9.17 4/74, `promtail` 2/55, `node-exporter` 1/50.

## Where this scan is weaker than it looks

- **open-webui**: the list scans `main`, but `deploy/sophia-eye/docker-compose.yml` pins a digest, and `main` has moved. The pinned digest (`sha256:4860ba00...`, created 2026-10-16) scans **14/137** and carries CVE-2026-111497, which is on KEV; today's `main` (created 2026-11-03) scans 4/28 and does not. The 14/137 is the true figure for this tree. `main` is a branch build one day old, so the bump rule does not admit it and the pin was not moved: Step 65 sends a KEV CRITICAL to Stevie as urgent. No open-webui container runs on west. `text-embeddings-inference` and `qdrant` also carry digests; both still match their tags.
- **Past-end-of-life Alpine**: `nginx`, `haproxy`, `traefik`, `victoria-metrics`, `grafana` and `loki` run on Alpine releases the scanner flags as past end of life. Their OS counts are likely low for the reason given above.
- **Not scanned**: five images in the tree are neither on Step 63's list nor in its stated exclusions: `grafana/grafana:10.4.0` (`monitoring/docker-compose.yml`, `docker/telemetry/`), `grafana/loki:2.9.0`, `prom/prometheus:v2.51.0`, `victoriametrics/victoria-metrics:v1.99.0` (all `docker/telemetry/`), `lscr.io/linuxserver/wireguard:1.0.20250521` (`docker/wireguard/`).
- **Left out on purpose** (Step 63): `rocm/pytorch` (FND-013), `docker/hosts/` (DP-401), `vllm/vllm-openai:latest-rocm` (DP-713), the rest of the k8s manifests (S-MOVE-OUT).
