<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-102: Container images use exact version tags; digests later, with a bot

**Status:** Accepted (Stevie, 2026-10-15: "ok sounds good, just note it in an adr or something for down the line to reassess at some point").
**Date:** 2026-10-15
**Reassess:** when Dependabot (or another bot) can open image bumps, or before the first public deployment, whichever is first.
**Related:** ADR-095 track two (supply chain; step 4 pinning), ADR-016 (The Well), feedback "develop against latest".

## Context

A tag such as `postgres:16-alpine` is a label the publisher moves: two pulls a
week apart can run different bits, and nothing in the repo records which. A
digest (`image@sha256:...`) names exact bytes, but freezes them: no security
fix lands until someone edits the digest, which pulls against developing on
current versions unless a bot does the editing.

Inventory 2026-10-15 (ADR-095): ~12 distinct `FROM` bases and ~40
third-party compose images across many compose files, with floating tags
(`16-alpine`, `v3.3`, `24.12`, `latest` on qdrant, open-webui, wireguard,
vllm) and the same image at several versions (Prometheus v2.50.1 / v2.51.0 /
latest; Loki 2.9.0 / 2.9.4 / 3.0.0).

## Decision

1. **Exact version tags**, no `latest` and no major- or minor-only tags:
   `postgres:16.13-alpine`, `traefik:v3.3.7`,
   `clickhouse/clickhouse-server:24.12.6.70`. An upgrade is an edit to the
   tag, reviewed like code.
2. **Pin what runs today.** Converting a floating tag pins the version
   currently pulled (read from the local image), so the change itself
   upgrades nothing. Upgrades are their own commits; for The Well, only after
   a backup and a passing restore drill (`scripts/well/restore-drill.sh`).
3. **One version per image** across compose files, as files are touched.
4. **No digests yet.** Revisit with a bot: Dependabot's `docker` ecosystem
   bumps tags and digests in Dockerfiles and compose files. Actions already
   work this way (`.github/dependabot.yml`, ADR-095).

## Done 2026-10-15

- `docker-compose.yml`: `postgres:16.13-alpine`, `traefik:v3.3.7`,
  `clickhouse/clickhouse-server:24.12.6.70` (the versions running).
- Kubernetes `the-well` StatefulSet and every `scripts/well/*` throwaway
  server: `postgres:16.13-alpine`, so tests run what production runs.
- Root `Dockerfile` runtime stages: `alpine:3.24` (off end-of-life 3.19).

## Open

- The other compose files (monitoring, observability, hosts, zhen): floating
  and `latest` tags, and duplicate versions to consolidate.
- `FROM` lines: `golang:1.27-alpine`, `debian:bookworm-slim`,
  `ubuntu:24.04`, `gcr.io/distroless/static-debian12:nonroot`,
  `node:22-alpine`, `nginx:alpine`; BIRD's two Dockerfiles on `alpine:3.19`.
- ~~Postgres 16.13 is behind the current 16.x minor (W-13)~~ *Done
  2026-10-15:* `postgres:16.15-alpine` everywhere (compose, k8s, every
  throwaway server, CI). Every Well suite passed on 16.15 first, a drill
  passed on the live set, then the container was recreated on the same
  volume. 16.14/16.15 need no dump, no reindex here (no btree_gist, ltree,
  pgcrypto or logical decoding); 16.15 fixes a pg_stat_statements overrun
  (CVE-2026-14676), which The Well now preloads.
