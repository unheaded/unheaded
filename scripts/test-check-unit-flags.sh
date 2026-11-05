#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# scripts/check-unit-flags.sh against fixture units: the two units east
# carried from its 1.0.0 packages fail, each for its own reason, and the
# forms it must read (continuations, --flag=value, ExecStart=-, a unit
# template indented in YAML) are read.
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CHECK="$REPO_ROOT/scripts/check-unit-flags.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

fail=0
verdict() { # verdict DESC WANT_RC WANT_TEXT FILE: run the gate on FILE
	local rc=0 out
	out="$("$CHECK" "$4" 2>&1)" || rc=$?
	if [[ $rc -ne $2 || "$out" != *"$3"* ]]; then
		printf '[FAIL] %s\n  rc %s, want %s; wanted output with: %s\n  got: %s\n' "$1" "$rc" "$2" "$3" "$out"
		fail=1
	else
		printf '[ok] %s\n' "$1"
	fi
}

cat >"$T/east-wotan.service" <<'U'
[Service]
ExecStart=/opt/unheaded/bin/wotan --config /etc/unheaded/wotan.yaml
U
verdict "a flag the binary lacks fails" 1 "wotan has no flag -config" "$T/east-wotan.service"

cat >"$T/east-timeguru.service" <<'U'
[Service]
User=unheaded
ExecStart=/opt/unheaded/bin/timeguru
U
verdict "a relative default with no working directory fails" 1 "needs a WorkingDirectory=" "$T/east-timeguru.service"

cat >"$T/not-state.service" <<'U'
[Service]
WorkingDirectory=/var/lib/unheaded/timeguru
StateDirectory=unheaded/other
ExecStart=/usr/local/bin/timeguru
U
verdict "a working directory no StateDirectory= creates fails" 1 "needs a WorkingDirectory=" "$T/not-state.service"

cat >"$T/good-timeguru.service" <<'U'
[Service]
WorkingDirectory=/var/lib/unheaded/timeguru
StateDirectory=unheaded/timeguru
ExecStart=/usr/local/bin/timeguru
U
verdict "a state directory that is the working directory passes" 0 "[PASS]" "$T/good-timeguru.service"

cat >"$T/continued.service" <<'U'
[Service]
ExecStart=-/usr/local/bin/wotan \
    --http-port 18000 \
    --no-such-flag
U
verdict "a flag on a continuation line, under ExecStart=-, is read" 1 "wotan has no flag -no-such-flag" "$T/continued.service"

cat >"$T/lxd.yaml" <<'U'
write_files:
  - content: |
      [Service]
      ExecStart=/opt/unheaded/bin/wotan --grpc-addr=0.0.0.0:18001
U
verdict "--flag=value in an indented unit template is read" 1 "wotan has no flag -grpc-addr" "$T/lxd.yaml"

cat >"$T/third-party.service" <<'U'
[Service]
ExecStart=/usr/bin/vector --config /etc/vector.yaml
U
verdict "nothing built here to check refuses to pass" 2 "refusing to pass vacuously" "$T/third-party.service"

[[ $fail -eq 0 ]] || exit 1
echo "[PASS] check-unit-flags verdicts hold"
