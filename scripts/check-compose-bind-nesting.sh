#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-091: never nest a bind mount inside another bind mount's mountpoint.
#
# The Well's initdb ordering bug was this shape. docker-compose.yml mounted
# both ./db/migrations and ./db/init.sh into /docker-entrypoint-initdb.d, the
# second inside the first's mountpoint. Docker had to create the inner
# mountpoint before the outer mount existed, so it made an empty root-owned
# init.sh on the host, littering the source tree — and the entrypoint then ran
# every migration against the wrong database and exited 3. Nothing caught it
# for months because /docker-entrypoint-initdb.d only runs on a fresh volume:
# the broken path was reachable only by a new contributor, a clean checkout or
# a disaster-recovery restore.
#
# That was fixed in B7. This keeps it fixed. It is a ratchet, not a cleanup
# request: the tree has zero nested pairs as of 2026-09-23.
#
# Reads the resolved compose config, not the running containers — a running
# stack reflects whatever was up when it started, which is ADR-093 rule 4.
set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE_FILE="${1:-docker-compose.yml}"

if [[ ! -f "$COMPOSE_FILE" ]]; then
    echo "check-compose-bind-nesting: no such file: $COMPOSE_FILE" >&2
    exit 2
fi

if command -v docker >/dev/null 2>&1 && docker compose -f "$COMPOSE_FILE" config >/dev/null 2>&1; then
    RESOLVED="$(docker compose -f "$COMPOSE_FILE" config)"
else
    echo "check-compose-bind-nesting: docker compose unavailable, reading $COMPOSE_FILE directly" >&2
    RESOLVED="$(cat "$COMPOSE_FILE")"
fi

printf '%s' "$RESOLVED" | python3 -c '
import sys, yaml

doc = yaml.safe_load(sys.stdin) or {}
services = doc.get("services") or {}
if not services:
    print("check-compose-bind-nesting: no services found — refusing to pass vacuously")
    sys.exit(2)

def targets(spec):
    out = []
    for v in (spec or {}).get("volumes") or []:
        if isinstance(v, dict):
            if v.get("type") == "bind" and v.get("target"):
                out.append(v["target"])
        elif isinstance(v, str):
            parts = v.split(":")
            # host:container[:mode] — a named volume has no leading / or .
            if len(parts) >= 2 and (parts[0].startswith("/") or parts[0].startswith(".")):
                out.append(parts[1])
    return out

nested = []
for name, spec in sorted(services.items()):
    tg = targets(spec)
    for outer in tg:
        for inner in tg:
            if outer != inner and inner.startswith(outer.rstrip("/") + "/"):
                nested.append((name, outer, inner))

if nested:
    print(f"[FAIL] {len(nested)} nested bind mount(s) (ADR-091):")
    for name, outer, inner in nested:
        print(f"         {name}: {inner} is inside {outer}")
    print()
    print("       Docker creates the inner mountpoint before the outer mount")
    print("       exists, littering the source tree with root-owned paths and")
    print("       shadowing whatever the outer mount was meant to provide.")
    print("       Mount them at separate paths instead — see ADR-091.")
    sys.exit(1)

total = sum(len(targets(s)) for s in services.values())
print(f"[PASS] no nested bind mounts ({total} bind targets across {len(services)} services)")
'
