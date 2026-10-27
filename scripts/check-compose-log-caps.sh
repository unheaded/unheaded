#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# ADR-092 gate: every compose service must declare its own log cap.
#
# Two ways to satisfy it:
#   json-file   options.max-size set (and the cap is in the compose file)
#   journald    the host journal is the cap, so the profile every host in
#               compliance/hosts/*.yaml is bound to must apply the
#               journald-caps fix (SystemMaxUse/RuntimeMaxUse,
#               compliance/profiles/profiles.yaml); a host whose profile
#               lacks it fails every journald service (ADR-092 step 1,
#               amended 2026-10-27).
# Any other driver fails.
#
# Deliberately reads the compose file, NOT the running containers. Inspecting
# live containers passes on any host whose /etc/docker/daemon.json happens to
# set a default — which is how this went unnoticed: the cap existed on one
# long-lived dev box and nowhere in the repository. The repo is what a fresh
# host, a CI runner and a disaster-recovery rebuild actually get.
set -euo pipefail

COMPOSE_FILE="${1:-docker-compose.yml}"
COMPLIANCE_DIR="${COMPLIANCE_DIR:-$(cd "$(dirname "$0")/.." && pwd)/compliance}"

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

printf '%s' "$RESOLVED" | COMPLIANCE_DIR="$COMPLIANCE_DIR" python3 -c '
import glob, os, sys, yaml

doc = yaml.safe_load(sys.stdin) or {}
services = doc.get("services") or {}
if not services:
    print("check-compose-log-caps: no services found — refusing to pass vacuously")
    sys.exit(2)

def journald_capped():
    """Empty when the profile of every bound host applies journald-caps; else the reason."""
    base = os.environ["COMPLIANCE_DIR"]
    try:
        profs = (yaml.safe_load(open(os.path.join(base, "profiles", "profiles.yaml"))) or {}).get("profiles") or []
    except OSError as e:
        return f"cannot read profiles: {e}"
    fixes = {p.get("name"): (p.get("fixes") or []) for p in profs if isinstance(p, dict)}
    hosts = sorted(glob.glob(os.path.join(base, "hosts", "*.yaml")))
    if not hosts:
        return "no hosts under compliance/hosts"
    lacking = []
    for h in hosts:
        prof = (yaml.safe_load(open(h)) or {}).get("profile")
        if "journald-caps" not in fixes.get(prof, []):
            lacking.append(f"{os.path.basename(h)[:-5]} ({prof})")
    if lacking:
        return "host profile without journald-caps: " + ", ".join(lacking)
    return ""

missing = []
journald_reason = None
for name, spec in sorted(services.items()):
    logging = (spec or {}).get("logging") or {}
    driver = logging.get("driver", "json-file")
    opts = logging.get("options") or {}
    if driver == "journald":
        if journald_reason is None:
            journald_reason = journald_capped()
        if journald_reason:
            missing.append(f"{name} (journald: {journald_reason})")
    elif driver != "json-file":
        missing.append(f"{name} (driver {driver!r}: only json-file with max-size, or journald)")
    elif not opts.get("max-size"):
        missing.append(name)

if missing:
    print(f"[FAIL] {len(missing)} of {len(services)} services have no log cap (ADR-092):")
    for name in missing:
        print(f"         - {name}")
    print("       add to each service:")
    print("         logging:")
    print("           driver: \"json-file\"")
    print("           options: { max-size: \"10m\", max-file: \"3\" }")
    sys.exit(1)

print(f"[PASS] all {len(services)} services declare a log cap (json-file max-size, or journald under journald-caps)")
'
