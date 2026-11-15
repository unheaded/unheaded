#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# check-unit-ceilings.sh — every systemd unit that runs one of our Go
# binaries has a memory ceiling the Go runtime knows about (S-LOW-FRUIT
# Phase 3): MemoryMax= and Environment=GOMEMLIMIT=. Without GOMEMLIMIT the
# GC ignores the cgroup limit and the kernel OOM-kills a heap the GC could
# have shrunk.
#
# Compose too (ADR-147 Phase 14, Developer D22): in docker-compose.yml and
# monitoring/docker-compose.yml, every service built from this repository's
# Dockerfile (build.target: one of our Go binaries) with a memory limit sets
# GOMEMLIMIT below that limit. Override the files with COMPOSE_FILES.
#
#   scripts/check-unit-ceilings.sh [UNIT_FILE...]   default: systemd/*.service deploy/systemd/*.service
set -euo pipefail
cd "$(dirname "$0")/.."

units=("$@")
if [[ ${#units[@]} -eq 0 ]]; then
	shopt -s nullglob
	units=(systemd/*.service deploy/systemd/*.service)
fi
[[ ${#units[@]} -eq 0 ]] && { echo "FAIL: no unit files found"; exit 1; }

# Names of our Go programs: the last path element of every main package.
mapfile -t gomains < <(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./... 2>/dev/null | sed 's#.*/##' | sort -u)
[[ ${#gomains[@]} -eq 0 ]] && { echo "FAIL: go list found no main packages"; exit 1; }
is_go() { local n=$1 m; for m in "${gomains[@]}"; do [[ $m == "$n" ]] && return 0; done; return 1; }

checked=0 fail=0
for u in "${units[@]}"; do
	exe=$(grep -m1 '^ExecStart=' "$u" | sed -E 's/^ExecStart=[-@+!:]*//' | awk '{print $1}')
	[[ -z $exe ]] && continue
	name=$(basename "$exe")
	is_go "$name" || continue
	checked=$((checked + 1))
	grep -q '^MemoryMax=' "$u" || { echo "FAIL ${u}: runs Go binary ${name} without MemoryMax="; fail=1; }
	grep -q '^Environment=GOMEMLIMIT=' "$u" || { echo "FAIL ${u}: runs Go binary ${name} without Environment=GOMEMLIMIT="; fail=1; }
done
(( checked == 0 )) && { echo "FAIL: no unit runs a Go binary (wrong directory?)"; exit 1; }

# The compose files: one line per checked service, "FAIL <file>: ..." or "ok".
# The parser runs in a command substitution, not a process substitution, so
# its exit code is seen: a file it cannot read or parse fails the check.
read -r -a compose_files <<<"${COMPOSE_FILES:-docker-compose.yml monitoring/docker-compose.yml}"
services=0
out=$(python3 -c '
import re, sys, yaml
units = {"": 1, "b": 1, "k": 1024, "m": 1024**2, "g": 1024**3, "t": 1024**4}
def compose_bytes(v):
    m = re.fullmatch(r"(\d+(?:\.\d+)?)\s*([bkmgt])?b?", str(v).strip().lower())
    return None if not m else float(m.group(1)) * units[m.group(2) or ""]
go_units = {"B": 1, "KiB": 1024, "MiB": 1024**2, "GiB": 1024**3, "TiB": 1024**4}
def go_bytes(v):
    m = re.fullmatch(r"(\d+(?:\.\d+)?)(B|KiB|MiB|GiB|TiB)?", str(v).strip())
    return None if not m else float(m.group(1)) * go_units[m.group(2) or "B"]
bad = False
for f in sys.argv[1:]:
    try:
        with open(f) as fh:
            doc = yaml.safe_load(fh) or {}
    except OSError as e:
        print(f"FAIL {f}: {e.strerror}"); bad = True; continue
    except yaml.YAMLError as e:
        print(f"FAIL {f}: not YAML: {str(e).splitlines()[0]}"); bad = True; continue
    services = doc.get("services") if isinstance(doc, dict) else None
    if not isinstance(services, dict):
        print(f"FAIL {f}: no services mapping"); bad = True; continue
    for name, svc in services.items():
        b = svc.get("build")
        if not isinstance(b, dict) or not b.get("target"):
            continue
        limit = (((svc.get("deploy") or {}).get("resources") or {}).get("limits") or {}).get("memory") or svc.get("mem_limit")
        if not limit:
            continue
        env = svc.get("environment") or {}
        if isinstance(env, list):
            env = dict(str(e).split("=", 1) for e in env if "=" in str(e))
        g = env.get("GOMEMLIMIT")
        lim, gm = compose_bytes(limit), go_bytes(g) if g is not None else None
        if g is None:
            print(f"FAIL {f}: {name} has memory limit {limit} and no GOMEMLIMIT")
        elif lim is None or gm is None:
            print(f"FAIL {f}: {name}: cannot read memory limit {limit!r} or GOMEMLIMIT {g!r}")
        elif gm >= lim:
            print(f"FAIL {f}: {name} GOMEMLIMIT {g} is not below its memory limit {limit}")
        else:
            print("ok")
sys.exit(1 if bad else 0)
' "${compose_files[@]}") || { printf '%s\n' "$out" | grep '^FAIL' || true; echo "FAIL: the compose files could not be checked (${compose_files[*]})"; exit 1; }
while IFS= read -r line; do
	[[ -z $line ]] && continue
	case $line in
		ok) services=$((services + 1)) ;;
		*) echo "$line"; fail=1 ;;
	esac
done <<<"$out"
(( services == 0 )) && { echo "FAIL: no compose service built from this repository has a memory limit (wrong files?)"; exit 1; }
(( fail )) && exit 1
echo "OK: ${checked} Go units carry MemoryMax and GOMEMLIMIT; ${services} compose services set GOMEMLIMIT below their memory limit"
