#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# check-image-drift.sh: what runs on this host is what the compose files pin
# (ADR-117 step 5). A host check: the other image gates read the files, this
# one reads the containers.
#
# ADR-117 found the monitoring stack upgraded by hand on west (Prometheus 3.15
# under a 2.51 pin, Loki 3.0 under 2.9) with the file never following. Two
# ways a host drifts, both fail here:
#
#   1. a container of a compose project runs an image other than its service's
#      pin: another tag, or the same tag with other bytes (digest);
#   2. a container of a compose project has no service in the files any more
#      (retired in the repo, still running on the host).
#
# Projects and their files:
#   <name: of docker-compose.yml>   docker-compose.yml,
#                                   docker/docker-compose.loadbalancers.yml,
#                                   docker/compose.cuirass-lxd.yml
#   monitoring                      monitoring/docker-compose.yml
# Containers of any other compose project, and containers compose did not
# make, are not this gate's.
#
# Images this repo builds (no image:, unheaded*, ${...}) are not compared: a
# local build has no registry digest and its bytes change with every build.
#
#   ./scripts/check-image-drift.sh              judge this host (docker ps -a)
#   ./scripts/check-image-drift.sh --expected   print the state the files imply
#   IMAGE_DRIFT_STATE=<file> ./scripts/check-image-drift.sh
#                                               judge a recorded state instead
#
# State rows, tab-separated: project, service, the container's image
# reference, its image's repo digests (comma-separated, or -), container name.
#
# Exit 0 no drift, 1 drift, 2 cannot judge (no docker and no state, or nothing
# from these projects on the host: it refuses to pass on nothing).
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT" || exit 2

MODE="judge"
case "${1:-}" in
"") ;;
--expected) MODE="expected" ;;
*)
	echo "usage: $0 [--expected]" >&2
	exit 2
	;;
esac

STATE=""
cleanup() { [[ -z "${TMP_STATE:-}" ]] || rm -f "$TMP_STATE"; }
trap cleanup EXIT

if [[ "$MODE" == "judge" ]]; then
	if [[ -n "${IMAGE_DRIFT_STATE:-}" ]]; then
		[[ -f "$IMAGE_DRIFT_STATE" ]] || { echo "check-image-drift: no such state file: $IMAGE_DRIFT_STATE" >&2; exit 2; }
		STATE="$IMAGE_DRIFT_STATE"
	else
		command -v docker >/dev/null 2>&1 || { echo "check-image-drift: no docker here and no IMAGE_DRIFT_STATE; cannot judge" >&2; exit 2; }
		TMP_STATE="$(mktemp)"
		STATE="$TMP_STATE"
		ids="$(docker ps -aq --filter label=com.docker.compose.project)" || { echo "check-image-drift: docker ps failed; cannot judge" >&2; exit 2; }
		for id in $ids; do
			row="$(docker inspect "$id" --format '{{index .Config.Labels "com.docker.compose.project"}}	{{index .Config.Labels "com.docker.compose.service"}}	{{.Config.Image}}	{{.Image}}	{{.Name}}')" || { echo "check-image-drift: docker inspect $id failed; cannot judge" >&2; exit 2; }
			img="$(printf '%s' "$row" | cut -f4)"
			# An image removed under a running container has no digests left to read.
			digests="$(docker image inspect "$img" --format '{{join .RepoDigests ","}}' 2>/dev/null)" || digests=""
			printf '%s\t%s\t%s\t%s\t%s\n' "$(printf '%s' "$row" | cut -f1)" "$(printf '%s' "$row" | cut -f2)" \
				"$(printf '%s' "$row" | cut -f3)" "${digests:--}" "$(printf '%s' "$row" | cut -f5 | sed 's|^/||')" >>"$STATE"
		done
	fi
fi

MODE="$MODE" STATE="$STATE" python3 - <<'PY'
import os
import re
import sys

import yaml

MAIN = "docker-compose.yml"
MAIN_EXTRA = ["docker/docker-compose.loadbalancers.yml", "docker/compose.cuirass-lxd.yml"]
MONITORING = "monitoring/docker-compose.yml"
DIGEST = re.compile(r"@sha256:[0-9a-f]{64}$")


def load(path):
    with open(path, encoding="utf-8") as f:
        return yaml.safe_load(f) or {}


def pins():
    """{project: {service: image or None}} from the tracked compose files."""
    out = {}
    main = load(MAIN)
    main_name = main.get("name") or os.path.basename(os.getcwd())
    out[main_name] = {}
    for path in [MAIN] + [p for p in MAIN_EXTRA if os.path.exists(p)]:
        for svc, body in (load(path).get("services") or {}).items():
            # The first file to name a service wins: the extras extend the main file.
            out[main_name].setdefault(svc, (body or {}).get("image"))
    mon = load(MONITORING)
    mon_name = mon.get("name") or os.path.basename(os.path.dirname(MONITORING))
    out[mon_name] = {svc: (body or {}).get("image") for svc, body in (mon.get("services") or {}).items()}
    return out


def local(image):
    return image is None or "${" in image or image.startswith(("unheaded-", "unheaded/"))


def split(ref):
    """(repo:tag, digest or '') of an image reference."""
    m = DIGEST.search(ref)
    return (ref[: m.start()], ref[m.start() + 1:]) if m else (ref, "")


P = pins()
if not any(P.values()):
    print("check-image-drift: no services in the compose files; refusing to pass on nothing", file=sys.stderr)
    sys.exit(2)

if os.environ["MODE"] == "expected":
    for project in sorted(P):
        for svc in sorted(P[project]):
            image = P[project][svc]
            if local(image):
                print(f"{project}\t{svc}\t{image or 'local-build'}\t-\t-")
                continue
            tagged, digest = split(image)
            repo = tagged.rsplit(":", 1)[0]
            print(f"{project}\t{svc}\t{image}\t{repo + '@' + digest if digest else '-'}\t-")
    sys.exit(0)

rows = []
with open(os.environ["STATE"], encoding="utf-8") as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        if len(parts) == 5 and parts[0] in P:
            rows.append(parts)
if not rows:
    print("check-image-drift: no container of " + " or ".join(sorted(P)) + " on this host; nothing to judge", file=sys.stderr)
    sys.exit(2)

bad, compared, skipped = [], 0, 0
for project, svc, ref, digests, name in sorted(rows):
    label = f"{project}/{svc}" + (f" ({name})" if name not in ("", "-") else "")
    if svc not in P[project]:
        bad.append(f"{label}: runs {ref}, and no compose file has this service (retired in the repo, still on the host)")
        continue
    pin = P[project][svc]
    if local(pin):
        skipped += 1
        continue
    compared += 1
    pin_tagged, pin_digest = split(pin)
    run_tagged, run_digest = split(ref)
    if run_tagged != pin_tagged:
        bad.append(f"{label}: runs {run_tagged}, the file pins {pin_tagged}")
        continue
    if pin_digest:
        held = {d.rsplit("@", 1)[-1] for d in digests.split(",") if "@" in d}
        if run_digest:
            held.add(run_digest)
        if pin_digest not in held:
            bad.append(f"{label}: {pin_tagged} runs other bytes than the pinned {pin_digest[:19]}... (holds {', '.join(sorted(h[:19] for h in held)) or 'no digest'})")

if bad:
    print("[FAIL] the host runs something the compose files do not say:", file=sys.stderr)
    for b in bad:
        print("         " + b, file=sys.stderr)
    print("       Recreate from the file (docker compose up -d <service>), or remove a retired", file=sys.stderr)
    print("       container by name; docs/runbooks/one-grafana-cutover.md is the worked case.", file=sys.stderr)
    sys.exit(1)
if compared == 0:
    print("check-image-drift: only locally built containers found; nothing pinned to judge", file=sys.stderr)
    sys.exit(2)
print(f"[PASS] {compared} containers run their pinned image; {skipped} locally built, not compared")
PY
