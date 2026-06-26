<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Unheaded Kubernetes — Documentation Index

Start here, then follow the path that fits what you need to do.

## Documents

| Document | Read it when you want to… |
|----------|---------------------------|
| [`README.md`](README.md) | Understand what runs here and the directory layout. |
| [`QUICKSTART.md`](QUICKSTART.md) | Get the stack running on an existing cluster, fast. |
| [`BUILD-PLAN.md`](BUILD-PLAN.md) | Bootstrap a control plane *the hard way* (PKI → etcd → apiserver → kubelet → CNI), then deploy. |
| [`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md) | Operate it: overlays, secrets, scaling, storage, upgrades, troubleshooting. |
| [`KUBERNETES_SETUP_SUMMARY.md`](KUBERNETES_SETUP_SUMMARY.md) | See how each Docker artifact maps to a Kubernetes object. |
| [`MANIFEST.md`](MANIFEST.md) | Look up the full file tree, every service, and every port. |

## Paths

**"I have a cluster, just deploy it."**
→ [`QUICKSTART.md`](QUICKSTART.md) → `kubectl apply -k manifests/overlays/dev`

**"I have bare metal and no cluster yet."**
→ [`BUILD-PLAN.md`](BUILD-PLAN.md) (control-plane bring-up) → [`QUICKSTART.md`](QUICKSTART.md)

**"I'm taking this to production."**
→ [`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md) (secrets, storage, HA, prod overlay)

**"I'm migrating from the Docker stack."**
→ [`KUBERNETES_SETUP_SUMMARY.md`](KUBERNETES_SETUP_SUMMARY.md) (one-to-one mapping)

## Key references in the wider repo

- `pkg/ports/ports.go` — canonical port registry ("the Doom Range"). The manifests
  never deviate from it.
- `docker-compose.yml` + `docker/**/docker-compose.*.yml` — the Docker stack these
  manifests mirror.
- `db/migrations/` — The Well schema (3 databases, 7 users) reproduced in
  `manifests/base/the-well/configmap-initdb.yaml`.
- Root `CLAUDE.md` — architecture, the Doom Range table, and the hardening baseline.

## Decisions before you ship

Image registry, StorageClass, CNI choice, GPU nodes, edge type (HAProxy vs Ingress),
and a real secret backend. All are tabulated in [`BUILD-PLAN.md`](BUILD-PLAN.md) §9 and
[`DEPLOYMENT_GUIDE.md`](DEPLOYMENT_GUIDE.md).
