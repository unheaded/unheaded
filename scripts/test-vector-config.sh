#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# test-vector-config.sh — config/vector/vector.yaml validates and its unit
# tests (config/vector/tests/*.yaml) pass, in the Vector image
# docker-compose.yml deploys (read from it, so this tests what runs).
#
# The tests pin the log-volume and auth-failure SLIs (ADR-100 Phase 3): a
# line is counted with host/unit/level only, transient units collapse, and
# the fixed sshd/sudo/PAM failure patterns match. Network off; the ClickHouse
# password is a placeholder, nothing connects.
#
# config/vector/vector-east.yaml (east's host shipper) must validate too,
# and its transforms block must equal vector.yaml's.
#
# Needs docker; without it this FAILS (exit 2), it never passes by skipping.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
if ! docker info >/dev/null 2>&1; then
	echo "[FAIL] docker is required to run vector; this test does not pass by skipping." >&2
	exit 2
fi
IMAGE="$(awk '/image: *timberio\/vector:/ {print $2; exit}' "$REPO_ROOT/docker-compose.yml")"
[[ -n "$IMAGE" ]] || { echo "[FAIL] no timberio/vector image in docker-compose.yml" >&2; exit 1; }
mapfile -t tests < <(cd "$REPO_ROOT/config/vector" && find tests -maxdepth 1 -name '*.yaml' | sort)
[[ ${#tests[@]} -gt 0 ]] || { echo "[FAIL] no tests under config/vector/tests" >&2; exit 1; }

run() {
	docker run --rm --network none -e CLICKHOUSE_PASSWORD=placeholder \
		-v "$REPO_ROOT/config/vector:/cfg:ro" "$IMAGE" "$@"
}
rc=0
run validate --no-environment /cfg/vector.yaml || rc=1
run test /cfg/vector.yaml "${tests[@]/#//cfg/}" || rc=1
# East's host shipper: validates, and counts lines exactly as west does
# (its transforms block must equal vector.yaml's, so the unit tests above
# cover both hosts).
run validate --no-environment /cfg/vector-east.yaml || rc=1
python3 - "$REPO_ROOT/config/vector/vector.yaml" "$REPO_ROOT/config/vector/vector-east.yaml" <<'PY' || rc=1
import sys, yaml
west, east = (yaml.safe_load(open(p))["transforms"] for p in sys.argv[1:3])
if west != east:
    diff = sorted(k for k in set(west) | set(east) if west.get(k) != east.get(k))
    print(f"[FAIL] vector-east.yaml transforms differ from vector.yaml: {', '.join(diff)}", file=sys.stderr)
    sys.exit(1)
PY
if [[ $rc -eq 0 ]]; then
	echo "[PASS] vector.yaml and vector-east.yaml validate, transforms match, and ${#tests[@]} test file(s) pass in ${IMAGE}"
else
	echo "[FAIL] vector config ($IMAGE)" >&2
fi
exit "$rc"
