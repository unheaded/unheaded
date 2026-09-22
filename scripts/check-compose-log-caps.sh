#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-092 gate: every compose service must declare its own log cap.
#
# Deliberately reads the compose file, NOT the running containers. Inspecting
# live containers passes on any host whose /etc/docker/daemon.json happens to
# set a default — which is how this went unnoticed: the cap existed on one
# long-lived dev box and nowhere in the repository. The repo is what a fresh
# host, a CI runner and a disaster-recovery rebuild actually get.
set -euo pipefail

COMPOSE_FILE="${1:-docker-compose.yml}"

if [[ ! -f "$COMPOSE_FILE" ]]; then
    echo "check-compose-log-caps: no such file: $COMPOSE_FILE" >&2
    exit 2
fi

# `docker compose config` resolves extends/anchors/overrides; fall back to the
# raw file when the docker CLI is unavailable (CI images without it).
if command -v docker >/dev/null 2>&1 && docker compose -f "$COMPOSE_FILE" config >/dev/null 2>&1; then
    RESOLVED="$(docker compose -f "$COMPOSE_FILE" config)"
else
    echo "check-compose-log-caps: docker compose unavailable, reading $COMPOSE_FILE directly" >&2
    RESOLVED="$(cat "$COMPOSE_FILE")"
fi

printf '%s' "$RESOLVED" | python3 -c '
import sys, yaml

doc = yaml.safe_load(sys.stdin) or {}
services = doc.get("services") or {}
if not services:
    print("check-compose-log-caps: no services found — refusing to pass vacuously")
    sys.exit(2)

missing = []
for name, spec in sorted(services.items()):
    opts = ((spec or {}).get("logging") or {}).get("options") or {}
    if not opts.get("max-size"):
        missing.append(name)

if missing:
    print(f"[FAIL] {len(missing)} of {len(services)} services have no logging max-size (ADR-092):")
    for name in missing:
        print(f"         - {name}")
    print("       add to each service:")
    print("         logging:")
    print("           driver: \"json-file\"")
    print("           options: { max-size: \"10m\", max-file: \"3\" }")
    sys.exit(1)

print(f"[PASS] all {len(services)} services declare a logging max-size")
'
