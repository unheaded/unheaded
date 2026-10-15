#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-101: the Kubernetes Well bootstraps from the same db/ compose uses.
# kubernetes/manifests/base/the-well/configmap-initdb.yaml is rendered from
# db/init.sh and db/migrations by scripts/well/render-k8s-initdb.py; this
# fails when it is stale. It drifted once already: the hand-written copy
# created no schema and interpolated passwords into SQL.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 scripts/well/render-k8s-initdb.py --check
