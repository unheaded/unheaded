#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# Every scrape target at host.docker.internal has a publish that answers there.
#
# host.docker.internal is docker0's gateway. A port published on 127.0.0.1 is
# refused at that address, so a scrape job can point at a healthy service and
# read up=0 for good. Nothing else fails when that happens: the compose file
# parses, the service is fine, and only the series go missing.
#
# Reads the scrape configs of the stack whose compose file carries the publish.
# A publish passes when the address written in the file, or the default of the
# variable there, is the gateway. Reads the files as written, as
# check-compose-publish-bind.sh does.
#
# A host unit can answer at the gateway too: it is not in the compose file, it
# binds the address itself. Its listener counts when a tracked unit or drop-in
# under deploy/systemd says so, in one of two ways:
#   ExecStart=... --listen 172.17.0.1:PORT       the unit as the repo ships it
#   X-Unheaded-Listen=172.17.0.1:PORT            a drop-in whose ExecStart is
#                                                kept on the host (systemd
#                                                ignores X- options)
# The second is a statement about a host file this script cannot read. What
# proves it on the host is the target itself: up == 1 in VictoriaMetrics.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE="${SCRAPE_PUBLISH_COMPOSE:-${REPO_ROOT}/docker-compose.yml}"
UNITS="${SCRAPE_PUBLISH_UNITS:-${REPO_ROOT}/deploy/systemd}"
if [[ $# -gt 0 ]]; then
	configs=("$@")
else
	configs=("${REPO_ROOT}/config/victoria/scrape.yml" "${REPO_ROOT}/monitoring/prometheus/prometheus.yml")
fi

python3 - "$COMPOSE" "$UNITS" "${configs[@]}" <<'PY'
import os, re, sys, yaml

GATEWAY = "172.17.0.1"
compose, units, configs = sys.argv[1], sys.argv[2], sys.argv[3:]


def split_short(entry):
    """Split on colons that are outside ${...} and [...]."""
    parts, cur, depth = [], "", 0
    for ch in entry:
        if ch in "{[":
            depth += 1
        elif ch in "}]":
            depth -= 1
        if ch == ":" and depth == 0:
            parts.append(cur)
            cur = ""
        else:
            cur += ch
    return parts + [cur]


def address_of(text):
    m = re.fullmatch(r"\$\{[A-Za-z_][A-Za-z0-9_]*:-([^}]*)\}", text)
    return m.group(1) if m else text


on_gateway = set()
services = (yaml.safe_load(open(compose)) or {}).get("services") or {}
for spec in services.values():
    for entry in (spec or {}).get("ports") or []:
        if isinstance(entry, dict):
            if address_of(str(entry.get("host_ip"))) == GATEWAY:
                on_gateway.add(str(entry.get("published")))
            continue
        parts = split_short(str(entry).rsplit("/", 1)[0])
        if len(parts) == 3 and address_of(parts[0]) == GATEWAY:
            on_gateway.add(parts[1])

# Host units that bind the gateway themselves (see the header).
DECLARED = re.compile(r"X-Unheaded-Listen=" + re.escape(GATEWAY) + r":(\d+)")
IN_EXEC = re.compile(r"\s--listen[= ]" + re.escape(GATEWAY) + r":(\d+)(?=\s|$)")
host_units = set()
for root, _, names in sorted(os.walk(units)):
    for name in sorted(names):
        if not name.endswith((".service", ".conf")):
            continue
        with open(os.path.join(root, name), encoding="utf-8", errors="replace") as fh:
            for line in fh:
                line = line.strip()
                m = DECLARED.fullmatch(line)
                if m:
                    host_units.add(m.group(1))
                elif line.startswith("ExecStart="):
                    host_units.update(IN_EXEC.findall(line))
on_gateway |= host_units

fail, total = False, 0
for config in configs:
    for job in (yaml.safe_load(open(config)) or {}).get("scrape_configs") or []:
        for group in job.get("static_configs") or []:
            for target in group.get("targets") or []:
                m = re.fullmatch(r"host\.docker\.internal:(\d+)", str(target))
                if not m:
                    continue
                total += 1
                if m.group(1) not in on_gateway:
                    print(f"[FAIL] {config}: job {job.get('job_name')} scrapes {target}, "
                          f"and {compose} publishes nothing on {GATEWAY}:{m.group(1)}"
                          f" (nor does a unit under {units} listen there)")
                    fail = True
if total == 0:
    print("check-scrape-publish: no host.docker.internal targets; refusing to pass vacuously")
    sys.exit(2)
if fail:
    sys.exit(1)
print(f"[PASS] {total} scrape targets at host.docker.internal have a publish on {GATEWAY}"
      f" ({len(host_units)} of them a host unit's own listener)")
PY
