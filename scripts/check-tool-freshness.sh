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
# All pins live in scripts/lib/tool-pins.sh; a bump updates the sha256 too.
set -euo pipefail

COOLDOWN_DAYS="${COOLDOWN_DAYS:-7}"
cd "$(dirname "$0")/.."

TOOLS=(
	"oxlint|scripts/lib/tool-pins.sh|OXLINT_VERSION|oxc-project/oxc|oxlint_v"
	"gitleaks|scripts/lib/tool-pins.sh|GITLEAKS_VERSION|gitleaks/gitleaks|v"
	"syft|scripts/lib/tool-pins.sh|SYFT_VERSION|anchore/syft|v"
	"grype|scripts/lib/tool-pins.sh|GRYPE_VERSION|anchore/grype|v"
	"helm|scripts/lib/tool-pins.sh|HELM_VERSION|helm/helm|v"
	"kubeconform|scripts/lib/tool-pins.sh|KUBECONFORM_VERSION|yannh/kubeconform|v"
)

python3 -I - "$COOLDOWN_DAYS" "${TOOLS[@]}" <<'PY'
import datetime, json, os, re, sys, urllib.error, urllib.request

cooldown = datetime.timedelta(days=int(sys.argv[1]))
now = datetime.datetime.now(datetime.timezone.utc)
headers = {"Accept": "application/vnd.github+json"}
if os.environ.get("GITHUB_TOKEN"):
    headers["Authorization"] = "Bearer " + os.environ["GITHUB_TOKEN"]

def key(v):
    return tuple(int(x) for x in v.split("."))

# Release lists are cached for an hour: the anonymous API allows 60 calls an
# hour, and the meta-gate runs this several times. CI sets GITHUB_TOKEN.
CACHE = os.path.join(os.environ.get("XDG_CACHE_HOME") or os.path.expanduser("~/.cache"), "unheaded-tools", "releases")

def releases(repo):
    path = os.path.join(CACHE, repo.replace("/", "__") + ".json")
    try:
        if now.timestamp() - os.path.getmtime(path) < 3600:
            return json.load(open(path))
    except OSError:
        pass
    out = []
    for page in range(1, 4):
        req = urllib.request.Request(f"https://api.github.com/repos/{repo}/releases?per_page=100&page={page}", headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                rels = json.load(r)
        except urllib.error.HTTPError as e:
            msg = "rate-limited; set GITHUB_TOKEN" if e.code in (403, 429) else f"HTTP {e.code}"
            print(f"[FAIL] {repo} releases: {msg}. This gate does not pass by skipping.")
            sys.exit(2)
        out += [{k: r.get(k) for k in ("tag_name", "draft", "prerelease", "published_at")} for r in rels]
        if len(rels) < 100:
            break
    os.makedirs(CACHE, exist_ok=True)
    json.dump(out, open(path, "w"))
    return out

stale = 0
for row in sys.argv[2:]:
    name, path, var, repo, prefix = row.split("|")
    m = re.search(rf'^{var}="([0-9.]+)"', open(path).read(), re.M)
    if not m:
        print(f"[FAIL] {name}: no {var}=\"x.y.z\" line in {path}")
        sys.exit(2)
    pinned = m.group(1)
    ripe, newest = [], None
    for rel in releases(repo):
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
    target = max(ripe, key=key) if ripe else pinned
    if key(pinned) >= key(target):
        note = "" if newest in (None, pinned) else f" ({newest} still in its {cooldown.days}d cooldown)"
        print(f"[ok]    {name} {pinned}{note}")
    else:
        print(f"[STALE] {name} {pinned}; {target} has been out {cooldown.days}d or more (bump {var} and its sha256 in {path})")
        stale += 1
sys.exit(1 if stale else 0)
PY
