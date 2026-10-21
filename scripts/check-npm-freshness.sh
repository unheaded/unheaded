#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-npm-freshness.sh — the npm packages package.json pins (eslint, lint
# tooling only, ADR-006) are not behind a release that is at least
# COOLDOWN_DAYS old. Newer than that is left alone on purpose: hijacked
# versions are usually pulled within hours. Reads the registry only; installs
# and runs nothing.
set -euo pipefail

COOLDOWN_DAYS="${COOLDOWN_DAYS:-7}"
REGISTRY="${NPM_REGISTRY:-https://registry.npmjs.org}"
cd "$(dirname "$0")/.."

python3 -I - "$COOLDOWN_DAYS" "$REGISTRY" <<'PY'
import datetime, json, sys, urllib.request

cooldown = datetime.timedelta(days=int(sys.argv[1]))
registry = sys.argv[2]
now = datetime.datetime.now(datetime.timezone.utc)
pkg = json.load(open("package.json"))
pins = {**pkg.get("dependencies", {}), **pkg.get("devDependencies", {})}
stale = 0
def key(v):
    return tuple(int(x) for x in v.split("."))

for name, pinned in sorted(pins.items()):
    with urllib.request.urlopen(f"{registry}/{name}", timeout=30) as r:
        meta = json.load(r)
    # Newest stable release (no prerelease tag) that has cleared the cooldown.
    ripe = [v for v in meta.get("versions", {})
            if all(p.isdigit() for p in v.split("."))
            and now - datetime.datetime.fromisoformat(meta["time"][v].replace("Z", "+00:00")) >= cooldown]
    target = max(ripe, key=key) if ripe else pinned
    latest = meta["dist-tags"]["latest"]
    if key(pinned) >= key(target):
        note = "" if pinned == latest else f" ({latest} still in its {cooldown.days}d cooldown)"
        print(f"[ok]    {name} {pinned}{note}")
    else:
        print(f"[STALE] {name} {pinned}; {target} has been out {cooldown.days}d or more")
        stale += 1
if stale:
    print("Bump package.json, npm install --package-lock-only --ignore-scripts, and add new names to docs/policy/dependency-baseline.txt.")
    sys.exit(1)
PY
