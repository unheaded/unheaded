#!/bin/bash
# SPDX-License-Identifier: MIT
# check-manifest-yaml.sh — every tracked YAML manifest must actually parse.
#
# deploy/k8s/monitoring/pleg-alerts.yaml sat in the tree unparseable: it used
# "{{ \$labels.node }}" inside a DOUBLE-quoted scalar, and \$ is not a valid
# YAML escape. kubectl apply would have rejected the file outright, so those
# PLEG alert rules were never deployable. Nothing noticed, because nothing in
# CI ever parsed the manifests — they were only ever scanned by trivy, which
# reports on what it can read and stays quiet about what it cannot.
#
# A manifest that does not parse is not a hardening problem, it is a silently
# missing control: the alerts, network policies or security contexts it
# declares simply are not in the cluster.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${REPO_ROOT}" || exit 1

if ! command -v python3 >/dev/null 2>&1; then
    echo "SKIP: python3 not available"
    exit 0
fi

python3 - <<'PY'
import subprocess, sys
try:
    import yaml
except ImportError:
    print("SKIP: PyYAML not installed")
    sys.exit(0)

files = subprocess.run(
    ['git', 'ls-files', '*.yaml', '*.yml'],
    capture_output=True, text=True).stdout.split()

# Helm templates are Go templates, not YAML, until rendered. helm lint covers
# those; parsing them here would only produce noise.
def skip(p):
    return p.startswith('helm/') and '/templates/' in p

bad = []
checked = 0
for f in files:
    if skip(f):
        continue
    try:
        with open(f, encoding='utf-8') as fh:
            list(yaml.safe_load_all(fh))
        checked += 1
    except Exception as e:                      # noqa: BLE001 - report anything unparseable
        bad.append((f, str(e).split('\n')[0][:110]))

if bad:
    print("=" * 60)
    print("  FAIL: unparseable YAML — these manifests cannot be applied")
    print("=" * 60)
    for f, err in bad:
        print(f"    {f}")
        print(f"      {err}")
    print()
    print("  A manifest that does not parse is a MISSING control, not a")
    print("  cosmetic problem. Common cause: an invalid escape such as \\$")
    print("  inside a double-quoted scalar — use single quotes, which do no")
    print("  escape processing.")
    print("=" * 60)
    sys.exit(1)

print(f"PASS: {checked} tracked YAML files parse cleanly.")
PY
