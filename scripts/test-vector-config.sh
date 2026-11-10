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
# and its transforms block must equal vector.yaml's (less well_rows, which
# feeds west's well_logs sink only).
#
# The well_logs sink (the measured week's second copy, in The Well) is run
# against a writer that never answers: every row must still reach the other
# sink.
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
		-e LOGSTORE_INGEST_TOKEN=placeholder \
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
# well_rows feeds west's well_logs sink only (the measured week); nothing
# east has reads it, so east must not carry it and the rest must be equal.
if "well_rows" in east:
    print("[FAIL] vector-east.yaml carries well_rows; it is west's only", file=sys.stderr)
    sys.exit(1)
west.pop("well_rows", None)
if west != east:
    diff = sorted(k for k in set(west) | set(east) if west.get(k) != east.get(k))
    print(f"[FAIL] vector-east.yaml transforms differ from vector.yaml: {', '.join(diff)}", file=sys.stderr)
    sys.exit(1)
PY
# The well_logs sink (the measured week's Well copy) must never hold the
# record back. Its block is lifted from vector.yaml as written and run in
# this image with the network off, against the worst writer: one that takes
# the connection and never answers (a TCP listener inside the same Vector,
# under the writer's name), so every request runs to its timeout. A file
# sink stands in for ClickHouse on the same input. 20,000 rows, four times
# the sink's buffer, must all reach the file sink meanwhile. With
# `when_full: block` there instead, the file sink stalls behind the full
# buffer and this fails (tried 2026-11-10). East must not carry the sink.
iso="$(mktemp -d)"
ISO_NAME="vector-isolation-$$"
trap 'docker rm -f "$ISO_NAME" >/dev/null 2>&1; rm -rf "$iso"' EXIT
if python3 - "$REPO_ROOT/config/vector/vector.yaml" "$REPO_ROOT/config/vector/vector-east.yaml" "$iso/vector.yaml" <<'ISOPY'
import sys, yaml
west = yaml.safe_load(open(sys.argv[1]))
east = yaml.safe_load(open(sys.argv[2]))
sink = west.get("sinks", {}).get("well_logs")
if sink is None:
    sys.exit("[FAIL] vector.yaml has no well_logs sink (delete this proof with it)")
if "well_logs" in east.get("sinks", {}):
    sys.exit("[FAIL] vector-east.yaml carries the well_logs sink; it is west's only")
rows = west.get("transforms", {}).get("well_rows", {})
if sink.get("inputs") != ["well_rows"] or rows.get("inputs") != west["sinks"]["clickhouse"].get("inputs"):
    sys.exit("[FAIL] well_logs must read well_rows, and well_rows what clickhouse reads: the Well copy is the redacted rows, less the relayed ones")
sink = dict(sink, inputs=["rows"])
yaml.safe_dump({
    "data_dir": "/out/data",
    "sources": {
        "rows": {"type": "demo_logs", "format": "json", "interval": 0.0005, "count": 20000},
        "silent_writer": {"type": "socket", "mode": "tcp", "address": "127.0.0.1:19140"},
    },
    "sinks": {
        "record": {"type": "file", "inputs": ["rows"], "path": "/out/record.log", "encoding": {"codec": "json"}},
        "well_logs": sink,
        "nowhere": {"type": "blackhole", "inputs": ["silent_writer"]},
    },
}, open(sys.argv[3], "w"))
ISOPY
then
	mkdir -p "$iso/data" "$iso/tls"
	# Any certificate will do as the CA file: nothing answers.
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 1 \
		-subj "/CN=vector-isolation-test" -keyout "$iso/tls/key.pem" -out "$iso/tls/ca.pem" >/dev/null 2>&1
	docker run -d --name "$ISO_NAME" --network none --add-host logstore-writer:127.0.0.1 \
		--user "$(id -u):$(id -g)" \
		-e LOGSTORE_INGEST_TOKEN=placeholder \
		-v "$iso:/out" -v "$iso/tls/ca.pem:/etc/vector/tls/ca.pem:ro" \
		"$IMAGE" --config /out/vector.yaml >/dev/null || rc=1
	got=0
	for _ in $(seq 1 45); do
		got="$({ wc -l <"$iso/record.log"; } 2>/dev/null || echo 0)"
		[[ "$got" -ge 20000 ]] && break
		sleep 1
	done
	if [[ "$got" -ge 20000 ]]; then
		echo "[PASS] well_logs against a writer that never answers: all $got rows still reached the record sink"
	else
		echo "[FAIL] well_logs against a writer that never answers: only $got of 20000 rows reached the record sink in 45s (the Well copy is holding the record back)" >&2
		docker logs "$ISO_NAME" 2>&1 | tail -5 >&2
		rc=1
	fi
else
	rc=1
fi
if [[ $rc -eq 0 ]]; then
	echo "[PASS] vector.yaml and vector-east.yaml validate, transforms match, and ${#tests[@]} test file(s) pass in ${IMAGE}"
else
	echo "[FAIL] vector config ($IMAGE)" >&2
fi
exit "$rc"
