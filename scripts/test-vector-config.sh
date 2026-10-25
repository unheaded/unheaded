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
if [[ $rc -eq 0 ]]; then
	echo "[PASS] vector.yaml validates and ${#tests[@]} test file(s) pass in ${IMAGE}"
else
	echo "[FAIL] vector config ($IMAGE)" >&2
fi
exit "$rc"
