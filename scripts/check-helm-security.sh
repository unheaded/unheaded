#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-helm-security.sh — every container the chart renders is locked down,
# for the default, dev and prod values.
#
# Replaces greps in helm.yml that could not say what they claimed. "All
# deployments must have readOnlyRootFilesystem: true" was
# `grep -c ... | grep -q "0"`: true when the chart contained the string once
# anywhere, and a false failure at a count of 10 or 20. The hostNetwork and
# privileged checks used `grep -v "0"`, which passes at a count of 10.
#
# Per workload (Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob,
# Pod), per container and init container:
#   readOnlyRootFilesystem: true, allowPrivilegeEscalation: false,
#   capabilities.drop contains ALL, privileged not true;
# and the pod spec has no hostNetwork, hostPID or hostIPC.
# Renders with the pinned Helm (lib/tool-pins.sh, sha256 via lib/fetch-tool.sh).
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck source=scripts/lib/tool-pins.sh
source scripts/lib/tool-pins.sh
# shellcheck source=scripts/lib/fetch-tool.sh
source scripts/lib/fetch-tool.sh
HELM="$(fetch_tool helm "$HELM_VERSION" \
	"https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" \
	"$HELM_SHA256" linux-amd64/helm)"

CHART=helm/unheaded
OUT="$(mktemp -d)"
trap 'rm -rf "$OUT"' EXIT
"$HELM" template unheaded "$CHART" >"$OUT/default.yaml"
"$HELM" template unheaded "$CHART" -f "$CHART/values-dev.yaml" >"$OUT/dev.yaml"
"$HELM" template unheaded "$CHART" -f "$CHART/values-prod.yaml" >"$OUT/prod.yaml"

python3 -I - "$OUT"/default.yaml "$OUT"/dev.yaml "$OUT"/prod.yaml <<'PY'
import os, sys, yaml

WORKLOADS = {"Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Job", "CronJob", "Pod"}
failed = 0
for path in sys.argv[1:]:
    values = os.path.basename(path).removesuffix(".yaml")
    bad, containers = [], 0
    for doc in yaml.safe_load_all(open(path)):
        if not doc or doc.get("kind") not in WORKLOADS:
            continue
        spec = doc["spec"]
        if doc["kind"] == "CronJob":
            spec = spec["jobTemplate"]["spec"]
        pod = spec if doc["kind"] == "Pod" else spec["template"]["spec"]
        name = f'{doc["kind"]}/{doc["metadata"]["name"]}'
        for key in ("hostNetwork", "hostPID", "hostIPC"):
            if pod.get(key):
                bad.append(f"{name}: {key}: true")
        for c in pod.get("containers", []) + pod.get("initContainers", []):
            containers += 1
            sc = c.get("securityContext") or {}
            where = f'{name} container {c["name"]}'
            if sc.get("readOnlyRootFilesystem") is not True:
                bad.append(f"{where}: readOnlyRootFilesystem is not true")
            if sc.get("allowPrivilegeEscalation") is not False:
                bad.append(f"{where}: allowPrivilegeEscalation is not false")
            if sc.get("privileged"):
                bad.append(f"{where}: privileged: true")
            if "ALL" not in ((sc.get("capabilities") or {}).get("drop") or []):
                bad.append(f"{where}: capabilities.drop lacks ALL")
    if containers == 0:
        print(f"[FAIL] {values}: no containers rendered; refusing to pass vacuously")
        failed = 1
        continue
    for b in bad:
        print(f"[FAIL] {values}: {b}")
    if bad:
        failed = 1
    else:
        print(f"[ok]   {values}: {containers} containers locked down")
sys.exit(failed)
PY
