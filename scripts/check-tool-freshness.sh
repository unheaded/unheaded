#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-tool-freshness.sh — release-pinned tools are not behind a release that
# is at least COOLDOWN_DAYS old.
#
# A pin fixes what runs; it is not a freeze. This fails once a newer stable
# release has been out COOLDOWN_DAYS (7). Younger releases are left alone on
# purpose: hijacked releases are usually pulled within hours (the Sept 2025
# npm worm), so day-one versions are the risky ones. Reads the GitHub
# releases API only; downloads and runs nothing. GITHUB_TOKEN, when set,
# lifts the anonymous rate limit.
#
# One row per tool: name | file holding the pin | variable | repo | tag prefix.
# Bumping a pin also means updating its sha256 in the same file.
set -euo pipefail

COOLDOWN_DAYS="${COOLDOWN_DAYS:-7}"
cd "$(dirname "$0")/.."

TOOLS=(
	"oxlint|scripts/check-js-lint.sh|OXLINT_VERSION|oxc-project/oxc|oxlint_v"
	"gitleaks|scripts/check-gitleaks.sh|GITLEAKS_VERSION|gitleaks/gitleaks|v"
)

python3 -I - "$COOLDOWN_DAYS" "${TOOLS[@]}" <<'PY'
import datetime, json, os, re, sys, urllib.request

cooldown = datetime.timedelta(days=int(sys.argv[1]))
now = datetime.datetime.now(datetime.timezone.utc)
headers = {"Accept": "application/vnd.github+json"}
if os.environ.get("GITHUB_TOKEN"):
    headers["Authorization"] = "Bearer " + os.environ["GITHUB_TOKEN"]

def key(v):
    return tuple(int(x) for x in v.split("."))

stale = 0
for row in sys.argv[2:]:
    name, path, var, repo, prefix = row.split("|")
    m = re.search(rf'^{var}="([0-9.]+)"', open(path).read(), re.M)
    if not m:
        print(f"[FAIL] {name}: no {var}=\"x.y.z\" line in {path}")
        sys.exit(2)
    pinned = m.group(1)
    ripe, newest = [], None
    for page in range(1, 4):
        req = urllib.request.Request(f"https://api.github.com/repos/{repo}/releases?per_page=100&page={page}", headers=headers)
        with urllib.request.urlopen(req, timeout=30) as r:
            rels = json.load(r)
        for rel in rels:
            tag = rel["tag_name"]
            if rel["draft"] or rel["prerelease"] or not tag.startswith(prefix):
                continue
            v = tag[len(prefix):]
            if not re.fullmatch(r"[0-9]+(\.[0-9]+)*", v):
                continue
            newest = max(newest or v, v, key=key)
            published = datetime.datetime.fromisoformat(rel["published_at"].replace("Z", "+00:00"))
            if now - published >= cooldown:
                ripe.append(v)
        if len(rels) < 100:
            break
    target = max(ripe, key=key) if ripe else pinned
    if key(pinned) >= key(target):
        note = "" if newest in (None, pinned) else f" ({newest} still in its {cooldown.days}d cooldown)"
        print(f"[ok]    {name} {pinned}{note}")
    else:
        print(f"[STALE] {name} {pinned}; {target} has been out {cooldown.days}d or more (bump {var} and its sha256 in {path})")
        stale += 1
sys.exit(1 if stale else 0)
PY
