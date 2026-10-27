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

# pkg/auth enables authentication only for the exact string "true". Read
# scalars as written: safe_load turns yes, on, True and TRUE into a boolean,
# and str() of it read as "true" here while the service, given "yes" or a
# quoted "TRUE", ran unauthenticated.
class Loader(yaml.SafeLoader):
    pass
Loader.yaml_implicit_resolvers = {
    k: [r for r in v if r[0] != "tag:yaml.org,2002:bool"]
    for k, v in yaml.SafeLoader.yaml_implicit_resolvers.items()
}

services = (yaml.load(open(sys.argv[1]), Loader=Loader) or {}).get("services") or {}
first_party = {n: s or {} for n, s in services.items() if (s or {}).get("build")}
if not first_party:
    print("check-service-auth: no first-party (build:) services; refusing to pass vacuously")
    sys.exit(2)

def env(spec):
    e = spec.get("environment") or {}
    if isinstance(e, list):
        e = dict(x.split("=", 1) if "=" in x else (x, "") for x in e)
    return {k: str(v) for k, v in e.items()}

# Services whose routes are public by design, each with the reason. Keep
# this short; a new entry needs a reason a reviewer can check in the code.
EXEMPT = {
    # The built-in OIDC issuer (ADR-103 §3): its routes are login, JWKS and
    # discovery, which every client must reach unauthenticated; login checks
    # the password itself, and user management is a CLI over its users file,
    # not an HTTP route (cmd/unheaded-auth/issuer.go).
    "auth": "OIDC issuer: login/JWKS/discovery are public by design",
}
for n in sorted(set(EXEMPT) & set(first_party)):
    print(f"[EXEMPT] {n}: {EXEMPT[n]}")
open_ = sorted(n for n, s in first_party.items() if n not in EXEMPT and env(s).get("AUTH_ENABLED", "") != "true")
if open_:
    print(f"[FAIL] {len(open_)} of {len(first_party)} first-party services accept unauthenticated requests (no AUTH_ENABLED=true):")
    for n in open_:
        print(f"         - {n}")
    sys.exit(1)
print(f"[PASS] all {len(first_party)} first-party services set AUTH_ENABLED=true")
PY
