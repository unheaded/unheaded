#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Compliance evidence for UH-AUTHN-01 (ADR-097), not a CI gate. It reads a
# file, not what runs (FND-037). Every first-party compose service (built from this repository)
# must set AUTH_ENABLED=true: pkg/auth enables authentication only then
# (pkg/auth/setup.go) and otherwise installs the Noop authenticator, which
# admits every request. A service that checks a bearer token itself, without
# pkg/auth, is named in TOKEN_AUTH below with the variable that carries the
# token; it passes only while compose gives it that variable. Reads the
# compose file as written.
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
# Services that authenticate without pkg/auth: each checks a bearer token
# on every route that is not a probe. The entry names the variable that
# carries the token and where the check is. The name must match exactly: a
# token-looking variable on any other service counts for nothing. A new
# entry needs a reason a reviewer can check in the code.
TOKEN_AUTH = {
    # POST /ingest is the only route besides /health, /ready and /metrics
    # (cmd/logstore-writer/main.go handler). Ingest.ServeHTTP refuses a
    # request without the token before it reads the body, and the binary
    # will not start on a token shorter than 32 bytes (ingest.go).
    "logstore-writer": ("LOGSTORE_INGEST_TOKEN",
                        "bearer token checked in cmd/logstore-writer/ingest.go ServeHTTP before the body is read; "
                        "POST /ingest is the only write route (main.go handler)"),
}
for n in sorted(set(EXEMPT) & set(first_party)):
    print(f"[EXEMPT] {n}: {EXEMPT[n]}")

def authenticates(name, spec):
    """How the service authenticates as the file is written: "auth", "token" or ""."""
    e = env(spec)
    if e.get("AUTH_ENABLED", "") == "true":
        return "auth"
    # Present and not empty. A bare list entry ("- NAME", the caller's own
    # environment) cannot be read from the file, so it does not count.
    if name in TOKEN_AUTH and e.get(TOKEN_AUTH[name][0], "") != "":
        return "token"
    return ""

how = {n: authenticates(n, s) for n, s in first_party.items() if n not in EXEMPT}
for n in sorted(n for n, h in how.items() if h == "token"):
    print(f"[TOKEN] {n}: {TOKEN_AUTH[n][1]} ({TOKEN_AUTH[n][0]})")
open_ = sorted(n for n, h in how.items() if not h)
if open_:
    print(f"[FAIL] {len(open_)} of {len(first_party)} first-party services accept unauthenticated requests "
          "(no AUTH_ENABLED=true, and no token named in TOKEN_AUTH):")
    for n in open_:
        missing = f" (token-auth service: {TOKEN_AUTH[n][0]} is not set in its environment)" if n in TOKEN_AUTH else ""
        print(f"         - {n}{missing}")
    sys.exit(1)
by_auth = sum(1 for h in how.values() if h == "auth")
by_token = sum(1 for h in how.values() if h == "token")
exempt = len(first_party) - len(how)
print(f"[PASS] all {len(first_party)} first-party services authenticate: "
      f"{by_auth} set AUTH_ENABLED=true, {by_token} check a token, {exempt} exempt")
PY
