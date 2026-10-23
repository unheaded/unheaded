#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Compliance evidence for UH-TLS-01 (ADR-097), not a CI gate: it fails
# today, and the dashboard shows that. The edge (traefik in compose) must
#   - redirect the plain-HTTP entrypoint to HTTPS
#       --entrypoints.web.http.redirections.entrypoint.to=websecure
#   - apply a TLS policy with TLS 1.3 minimum: a --providers.file config whose
#     tls.options.default.minVersion is VersionTLS13
#   - not serve its API/dashboard insecurely (--api.insecure=true absent)
# Reads the compose file as written. Usage: check-edge-tls.sh [compose-file]
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_FILE="${1:-${REPO_ROOT}/docker-compose.yml}"
[[ -f "$COMPOSE_FILE" ]] || { echo "check-edge-tls: no such file: $COMPOSE_FILE" >&2; exit 2; }

python3 - "$COMPOSE_FILE" <<'PY'
import os, sys, yaml

compose = sys.argv[1]
svc = ((yaml.safe_load(open(compose)) or {}).get("services") or {}).get("traefik")
if not svc:
    print("check-edge-tls: no traefik service; nothing terminates TLS")
    sys.exit(1)
cmd = [str(c) for c in (svc.get("command") or [])]
# traefik's flag names are case-insensitive (--entryPoints and --entrypoints
# are one flag) and its booleans are Go's strconv.ParseBool, so
# --api.insecure=True or =1 is as insecure as =true.
flags = {c.split("=", 1)[0].lower(): (c.split("=", 1)[1] if "=" in c else "true") for c in cmd}
GO_TRUE = {"1", "t", "T", "TRUE", "true", "True"}
problems = []

if flags.get("--entrypoints.web.http.redirections.entrypoint.to") != "websecure":
    problems.append("HTTP entrypoint 'web' is not redirected to 'websecure'")
if flags.get("--api.insecure") in GO_TRUE:
    problems.append("--api.insecure serves the API/dashboard without TLS or auth")

# traefik reads one static configuration source, and a configuration file
# (--configfile, or traefik.yml/.yaml/.toml in /etc/traefik or /) wins over
# the command line: the flags judged here would not be what it runs.
STATIC = ("traefik.yml", "traefik.yaml", "traefik.toml")
if "--configfile" in flags:
    problems.append("--configfile: static configuration comes from a file this check does not read")
for v in svc.get("volumes") or []:
    parts = str(v).split(":")
    if len(parts) < 2:
        continue
    src, dst = os.path.join(os.path.dirname(os.path.abspath(compose)), parts[0]), parts[1].rstrip("/") or "/"
    if os.path.basename(dst) in STATIC or (dst in ("/etc/traefik", "/") and any(os.path.isfile(os.path.join(src, n)) for n in STATIC)):
        problems.append(f"static configuration file mounted ({v}): traefik ignores the command flags this check reads")

min_tls = None
base = os.path.dirname(os.path.abspath(compose))
target = flags.get("--providers.file.filename")
if target:
    # map the container path back to the host through the service's volumes
    for v in svc.get("volumes") or []:
        parts = str(v).split(":")
        if len(parts) >= 2 and target.startswith(parts[1]):
            host = os.path.join(base, parts[0] + target[len(parts[1]):])
            if os.path.isfile(host):
                cfg = yaml.safe_load(open(host)) or {}
                min_tls = (((cfg.get("tls") or {}).get("options") or {}).get("default") or {}).get("minVersion")
if min_tls != "VersionTLS13":
    problems.append(f"no TLS policy with minVersion VersionTLS13 (found {min_tls!r})")

if problems:
    for p in problems:
        print(f"[FAIL] {p}")
    sys.exit(1)
print("[PASS] edge redirects HTTP, requires TLS 1.3, API not insecure")
PY
