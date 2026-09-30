#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Every compose service runs least-privilege: read_only root filesystem,
# cap_drop: ALL, security_opt no-new-privileges:true, never privileged.
# Services not there yet are listed in docs/security/compose-hardening-baseline.txt,
# which may only shrink: a listed service that is now hardened, or no longer
# exists, fails the gate too (take it off the list). Compliance evidence for
# UH-CTR-01 (ADR-097).
#
# Reads the compose file as written, like check-compose-log-caps.sh: the
# guarantee must be in the declared configuration, not in a host default.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="${1:-${REPO_ROOT}/docker-compose.yml}"
BASELINE="${REPO_ROOT}/docs/security/compose-hardening-baseline.txt"

[[ -f "$COMPOSE_FILE" ]] || { echo "check-compose-hardening: no such file: $COMPOSE_FILE" >&2; exit 2; }
[[ -f "$BASELINE" ]] || { echo "check-compose-hardening: missing $BASELINE" >&2; exit 2; }

python3 - "$COMPOSE_FILE" "$BASELINE" <<'PY'
import sys, yaml

compose, baseline = sys.argv[1], sys.argv[2]
services = (yaml.safe_load(open(compose)) or {}).get("services") or {}
if not services:
    print("check-compose-hardening: no services found; refusing to pass vacuously")
    sys.exit(2)
allowed = {l.strip() for l in open(baseline) if l.strip() and not l.startswith("#")}

def gaps(spec):
    spec = spec or {}
    g = []
    if spec.get("read_only") is not True:
        g.append("read_only: true")
    if "ALL" not in [str(c).upper() for c in spec.get("cap_drop") or []]:
        g.append("cap_drop: [ALL]")
    if not any(str(o).replace(" ", "") in ("no-new-privileges:true", "no-new-privileges") for o in spec.get("security_opt") or []):
        g.append("security_opt: [no-new-privileges:true]")
    if spec.get("privileged") is True:
        g.append("privileged removed")
    return g

fail = False
for name, spec in sorted(services.items()):
    g = gaps(spec)
    if g and name not in allowed:
        print(f"[FAIL] {name}: needs {', '.join(g)}")
        fail = True
    elif not g and name in allowed:
        print(f"[FAIL] {name} is hardened now: remove it from {baseline}")
        fail = True
for name in sorted(allowed - set(services)):
    print(f"[FAIL] baseline lists {name}, which is not a compose service: remove it")
    fail = True
if fail:
    sys.exit(1)
hardened = sum(1 for s in services.values() if not gaps(s))
print(f"[PASS] {hardened} of {len(services)} services hardened; {len(allowed)} baselined exceptions")
PY
