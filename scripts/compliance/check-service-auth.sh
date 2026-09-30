#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Compliance evidence for UH-AUTHN-01 (ADR-097), not a CI gate: it fails
# today. Every first-party compose service (built from this repository)
# must set AUTH_ENABLED=true: pkg/auth enables authentication only then
# (pkg/auth/setup.go) and otherwise installs the Noop authenticator, which
# admits every request. Reads the compose file as written.
# Usage: check-service-auth.sh [compose-file]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_FILE="${1:-${REPO_ROOT}/docker-compose.yml}"
[[ -f "$COMPOSE_FILE" ]] || { echo "check-service-auth: no such file: $COMPOSE_FILE" >&2; exit 2; }

python3 - "$COMPOSE_FILE" <<'PY'
import sys, yaml

services = (yaml.safe_load(open(sys.argv[1])) or {}).get("services") or {}
first_party = {n: s or {} for n, s in services.items() if (s or {}).get("build")}
if not first_party:
    print("check-service-auth: no first-party (build:) services; refusing to pass vacuously")
    sys.exit(2)

def env(spec):
    e = spec.get("environment") or {}
    if isinstance(e, list):
        e = dict(x.split("=", 1) if "=" in x else (x, "") for x in e)
    return {k: str(v) for k, v in e.items()}

open_ = sorted(n for n, s in first_party.items() if env(s).get("AUTH_ENABLED", "").lower() != "true")
if open_:
    print(f"[FAIL] {len(open_)} of {len(first_party)} first-party services accept unauthenticated requests (no AUTH_ENABLED=true):")
    for n in open_:
        print(f"         - {n}")
    sys.exit(1)
print(f"[PASS] all {len(first_party)} first-party services set AUTH_ENABLED=true")
PY
