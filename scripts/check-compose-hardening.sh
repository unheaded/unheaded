#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# Every compose service runs least-privilege: read_only root filesystem,
# cap_drop: ALL, security_opt no-new-privileges:true, never privileged, and an
# explicit user that is neither 1000 (govan on the hosts) nor root (root only
# for a one-shot with network_mode: none, like volume-init).
#
# Covers docker-compose.yml (the stack west runs) and every other tracked
# compose file outside tomb/. Services not there yet are listed in
# docs/security/compose-hardening-baseline.txt (docker-compose.yml, by service
# name) and docs/security/compose-hardening-baseline-alt.txt (the other files,
# as path:service). Both may only shrink: a listed service that is now
# hardened, or no longer exists, fails the gate too (take it off the list).
# Compliance evidence for UH-CTR-01 (ADR-097).
#
# Reads the compose files as written, like check-compose-log-caps.sh: the
# guarantee must be in the declared configuration, not in a host default.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BASELINE="${REPO_ROOT}/docs/security/compose-hardening-baseline.txt"
BASELINE_ALT="${REPO_ROOT}/docs/security/compose-hardening-baseline-alt.txt"
[[ -f "$BASELINE" ]] || { echo "check-compose-hardening: missing $BASELINE" >&2; exit 2; }
[[ -f "$BASELINE_ALT" ]] || { echo "check-compose-hardening: missing $BASELINE_ALT" >&2; exit 2; }

if [[ $# -gt 0 ]]; then
	files=("$@")
else
	mapfile -t files < <(cd "$REPO_ROOT" && git ls-files | grep -E '(^|/)docker-compose[^/]*\.ya?ml$' | grep -v '^tomb/' | sort)
fi
[[ ${#files[@]} -gt 0 ]] || { echo "check-compose-hardening: no compose files; refusing to pass vacuously" >&2; exit 2; }

python3 - "$REPO_ROOT" "$BASELINE" "$BASELINE_ALT" "${files[@]}" <<'PY'
import os, re, sys, yaml
root, baseline, baseline_alt, files = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4:]

def entries(path):
    return {l.strip() for l in open(path) if l.strip() and not l.startswith("#")}

allowed_main, allowed_alt = entries(baseline), entries(baseline_alt)

def gaps(spec):
    spec = spec or {}
    g = []
    if spec.get("read_only") is not True:
        g.append("read_only: true")
    if "ALL" not in [str(c).upper() for c in spec.get("cap_drop") or []]:
        g.append("cap_drop: [ALL]")
    if not any(str(o).replace(" ", "") in ("no-new-privileges:true", "no-new-privileges") for o in spec.get("security_opt") or []):
        g.append("security_opt: [no-new-privileges:true]")
    if spec.get("privileged") is True:
        g.append("privileged removed")
    user = str(spec.get("user") or "")
    uid = user.split(":")[0]
    if not uid:
        g.append("user: <own uid>")
    elif uid in ("1000", "root"):
        g.append(f"user other than {user}")
    elif uid == "0" and spec.get("network_mode") != "none":
        g.append("user other than root (root only for a network_mode: none one-shot)")
    return g

fail, total, hardened, seen = False, 0, 0, set()
for f in files:
    rel = os.path.relpath(os.path.join(root, f) if not os.path.isabs(f) else f, root)
    main = rel == "docker-compose.yml"
    try:
        services = (yaml.safe_load(open(os.path.join(root, rel))) or {}).get("services") or {}
    except yaml.YAMLError as e:
        print(f"[FAIL] {rel}: not parseable YAML: {e}")
        fail = True
        continue
    if main and not services:
        print("check-compose-hardening: no services in docker-compose.yml; refusing to pass vacuously")
        sys.exit(2)
    for name, spec in sorted(services.items()):
        key = name if main else f"{rel}:{name}"
        allowed = allowed_main if main else allowed_alt
        seen.add((main, key))
        total += 1
        g = gaps(spec)
        if not g:
            hardened += 1
        if g and key not in allowed:
            print(f"[FAIL] {key}: needs {', '.join(g)}")
            fail = True
        elif not g and key in allowed:
            print(f"[FAIL] {key} is hardened now: remove it from {baseline if main else baseline_alt}")
            fail = True

for main, allowed, path in ((True, allowed_main, baseline), (False, allowed_alt, baseline_alt)):
    for key in sorted(allowed - {k for m, k in seen if m == main}):
        print(f"[FAIL] {path} lists {key}, which is not a service in the checked files: remove it")
        fail = True
if fail:
    sys.exit(1)
print(f"[PASS] {hardened} of {total} services hardened across {len(files)} compose files; "
      f"{len(allowed_main)} + {len(allowed_alt)} baselined exceptions")
PY
