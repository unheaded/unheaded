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
#
# Also every workflow action pinned to a branch commit (`uses: o/r@<sha> #
# master, <date>`), which Dependabot cannot bump: stale once the branch has a
# commit COOLDOWN_DAYS old that the pin lacks. A pin that is no longer on its
# branch (force-push, rewritten history) fails outright: find out why first.
#
# And the CI Python tools' exact pins in .github/ci-python/requirements.in,
# against PyPI (re-lock with scripts/ci-python-lock.sh after a bump).
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
	"hadolint|scripts/lib/tool-pins.sh|HADOLINT_VERSION|hadolint/hadolint|v"
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

def fail(msg):
    # A failure must be readable without the job log (logs need auth; check-run
    # annotations do not): under Actions it is also an ::error:: annotation.
    print(f"[FAIL] {msg}. This gate does not pass by skipping.")
    if os.environ.get("GITHUB_ACTIONS") == "true":
        print(f"::error title=check-tool-freshness::{msg}")
    sys.exit(2)

def gh_get(url, what):
    """GET a GitHub API URL. With a token, a 403/404 that is not a rate limit
    is retried once anonymously: an Actions installation token can be refused
    on another organisation's repository where anonymous reads succeed."""
    tries = [headers]
    if "Authorization" in headers:
        tries.append({k: v for k, v in headers.items() if k != "Authorization"})
    last = None
    for i, h in enumerate(tries):
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers=h), timeout=30) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            try:
                body = json.loads(e.read().decode("utf-8", "replace") or "{}").get("message", "")
            except ValueError:
                body = ""
            limited = e.headers.get("X-RateLimit-Remaining") == "0" or e.code == 429
            last = f"{what}: HTTP {e.code}{' (rate-limited)' if limited else ''}{(' ' + repr(body[:120])) if body else ''}{' anonymous' if i else (' with token' if 'Authorization' in h else '')}"
            if limited or e.code not in (401, 403, 404) or i == len(tries) - 1:
                break
        except urllib.error.URLError as e:
            last = f"{what}: {e.reason}"
            break
    fail(last)


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
        rels = gh_get(f"https://api.github.com/repos/{repo}/releases?per_page=100&page={page}", f"{repo} releases")
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
        fail(f"{name}: no {var}=\"x.y.z\" line in {path}")
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

def compare(repo, base, branch):
    path = os.path.join(CACHE, f"{repo.replace('/', '__')}__{base[:12]}__{branch}.json")
    try:
        if now.timestamp() - os.path.getmtime(path) < 3600:
            return json.load(open(path))
    except OSError:
        pass
    d = gh_get(f"https://api.github.com/repos/{repo}/compare/{base}...{branch}", f"{repo} compare {base[:12]}...{branch}")
    out = {"status": d["status"], "commits": [(c["sha"], c["commit"]["committer"]["date"]) for c in d["commits"]]}
    os.makedirs(CACHE, exist_ok=True)
    json.dump(out, open(path, "w"))
    return out

# Branch pins: the comment names a branch (not a v1.2.3 tag) and the pin date.
pins = {}
pin_re = re.compile(r"uses:\s*([\w.-]+/[\w.-]+)(?:/[^@\s]*)?@([0-9a-f]{40})\s*#\s*([A-Za-z][\w.-]*)(?: branch)?, \d{4}-\d{2}-\d{2}\s*$")
wf = ".github/workflows"
for f in sorted(os.listdir(wf)):
    if f.endswith((".yml", ".yaml")):
        for line in open(os.path.join(wf, f)):
            m = pin_re.search(line)
            if m and not re.fullmatch(r"v?[0-9][\w.-]*", m.group(3)):
                pins.setdefault(m.groups(), []).append(f)
for (repo, sha, branch), files in sorted(pins.items()):
    name = f"{repo}@{branch}"
    d = compare(repo, sha, branch)
    if d["status"] == "identical":
        print(f"[ok]    {name} {sha[:12]}")
        continue
    if d["status"] != "ahead":
        print(f"[FAIL]  {name}: pinned {sha[:12]} is no longer on {branch} ({d['status']}); history was rewritten, check why before re-pinning ({', '.join(sorted(set(files)))})")
        stale += 1
        continue
    ripe = [c for c in d["commits"] if now - datetime.datetime.fromisoformat(c[1].replace("Z", "+00:00")) >= cooldown]
    if ripe:
        print(f"[STALE] {name} {sha[:12]}; {ripe[-1][0][:12]} ({ripe[-1][1][:10]}) has been on {branch} {cooldown.days}d or more (re-pin in {', '.join(sorted(set(files)))})")
        stale += 1
    else:
        print(f"[ok]    {name} {sha[:12]} ({len(d['commits'])} newer commit(s) still in their {cooldown.days}d cooldown)")

# CI Python tools: "name==x.y.z" lines of requirements.in vs PyPI.
def pypi(name):
    path = os.path.join(CACHE, "pypi__" + name + ".json")
    try:
        if now.timestamp() - os.path.getmtime(path) < 3600:
            return json.load(open(path))
    except OSError:
        pass
    try:
        with urllib.request.urlopen(f"https://pypi.org/pypi/{name}/json", timeout=30) as r:
            d = json.load(r)
    except urllib.error.HTTPError as e:
        fail(f"PyPI {name}: HTTP {e.code}")
    except urllib.error.URLError as e:
        fail(f"PyPI {name}: {e.reason}")
    out = {}
    for v, files in d["releases"].items():
        live = [f for f in files if not f.get("yanked")]
        if live and re.fullmatch(r"[0-9]+(\.[0-9]+)*", v):
            out[v] = min(f["upload_time_iso_8601"] for f in live)
    os.makedirs(CACHE, exist_ok=True)
    json.dump(out, open(path, "w"))
    return out

for line in open(".github/ci-python/requirements.in"):
    m = re.match(r"^([A-Za-z0-9._-]+)==([0-9.]+)\s*$", line)
    if not m:
        continue
    name, pinned = m.groups()
    rels = pypi(name)
    ripe = [v for v, t in rels.items() if now - datetime.datetime.fromisoformat(t.replace("Z", "+00:00")) >= cooldown]
    newest = max(rels, key=key) if rels else pinned
    target = max(ripe, key=key) if ripe else pinned
    if key(pinned) >= key(target):
        note = "" if newest == pinned else f" ({newest} still in its {cooldown.days}d cooldown)"
        print(f"[ok]    pypi {name} {pinned}{note}")
    else:
        print(f"[STALE] pypi {name} {pinned}; {target} has been out {cooldown.days}d or more (bump requirements.in, then scripts/ci-python-lock.sh)")
        stale += 1
sys.exit(1 if stale else 0)
PY
