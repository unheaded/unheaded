#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-host-state-textfile.sh: host-state-textfile.sh against fake docker and
# systemctl on PATH (ADR-147, S-SEE-AND-SURVIVE Phase 5). Touches no Docker
# daemon and no unit; runs as any user.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$REPO_ROOT/scripts/monitoring/host-state-textfile.sh"
command -v jq >/dev/null || { echo "[FAIL] jq is required" >&2; exit 2; }

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/out"
pass=0
fail=0
check() { # check <name> <command...>
    local name="$1"; shift
    if "$@"; then echo "[PASS] $name"; pass=$((pass + 1)); else echo "[FAIL] $name"; fail=$((fail + 1)); fi
}

H64=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
cat >"$T/inspect.json" <<JSON
[
 {"Id":"a$H64","Name":"/unheaded-wotan","State":{"Running":true,"ExitCode":0,"OOMKilled":false,"StartedAt":"2026-11-15T02:00:35.123456789Z","Health":{"Status":"healthy"}},
  "Config":{"Labels":{"com.docker.compose.project":"unheaded-dev","com.docker.compose.service":"wotan"}}},
 {"Id":"b$H64","Name":"/unheaded-postgres","State":{"Running":false,"ExitCode":137,"OOMKilled":true,"StartedAt":"2026-11-14T00:00:00Z"},
  "Config":{"Labels":{"com.docker.compose.project":"unheaded-dev","com.docker.compose.service":"postgres"}}},
 {"Id":"c$H64","Name":"/unheaded-volume-init","State":{"Running":false,"ExitCode":0,"OOMKilled":false,"StartedAt":"2026-11-14T00:00:00Z"},
  "Config":{"Labels":{"com.docker.compose.project":"unheaded-dev","com.docker.compose.service":"volume-init"}}},
 {"Id":"d$H64","Name":"/netbox-docker-redis-1","State":{"Running":false,"ExitCode":0,"OOMKilled":false,"StartedAt":"2026-04-01T00:00:00Z"},
  "Config":{"Labels":{"com.docker.compose.project":"netbox-docker","com.docker.compose.service":"redis"}}},
 {"Id":"e$H64","Name":"/unheaded-\"odd\"","State":{"Running":true,"ExitCode":0,"OOMKilled":false,"StartedAt":"2026-11-15T00:00:00Z"},
  "Config":{"Labels":{"com.docker.compose.project":"unheaded-dev","com.docker.compose.service":"odd"}}}
]
JSON
cat >"$T/show.txt" <<'SHOW'
Id=unheaded-kev-poller.service
Type=simple
LoadState=loaded
ActiveState=active
NRestarts=0

Id=unheaded-wiki-server.service
Type=simple
LoadState=loaded
ActiveState=failed
NRestarts=15

Id=unheaded-well-backup.service
Type=oneshot
LoadState=loaded
ActiveState=inactive
NRestarts=0

Id=nginx.service
Type=
LoadState=not-found
ActiveState=inactive
NRestarts=0
SHOW
cat >"$T/bin/docker" <<FAKE
#!/usr/bin/env bash
[[ -n "\${FAKE_DOCKER_FAIL:-}" ]] && exit 1
case "\$1" in
  ps) printf '%s\n' a b c d e ;;
  inspect) cat "$T/inspect.json" ;;
  *) exit 1 ;;
esac
FAKE
cat >"$T/bin/systemctl" <<FAKE
#!/usr/bin/env bash
[[ "\$1" == show ]] || exit 1
cat "$T/show.txt"
FAKE
chmod +x "$T/bin/docker" "$T/bin/systemctl"

run() { PATH="$T/bin:$PATH" bash "$SCRIPT" "$T/out"; }
F="$T/out/host_state.prom"
has() { grep -qxF "$1" "$F"; }
hasnt() { ! grep -qF "$1" "$F"; }

if ! run; then echo "[FAIL] host-state-textfile.sh exited non-zero"; exit 1; fi

check "1 running container with project and service" has 'unheaded_container_running{name="unheaded-wotan",project="unheaded-dev",service="wotan"} 1'
check "2 stopped, exit code, memory kill" bash -c "grep -qxF 'unheaded_container_running{name=\"unheaded-postgres\",project=\"unheaded-dev\",service=\"postgres\"} 0' '$F' &&
    grep -qxF 'unheaded_container_exit_code{name=\"unheaded-postgres\"} 137' '$F' && grep -qxF 'unheaded_container_oom_killed{name=\"unheaded-postgres\"} 1' '$F'"
check "3 health only where a healthcheck exists" bash -c "grep -qxF 'unheaded_container_healthy{name=\"unheaded-wotan\"} 1' '$F' && ! grep -qF 'unheaded_container_healthy{name=\"unheaded-postgres\"}' '$F'"
check "4 one-shots and unwatched projects skipped" bash -c "! grep -qF 'unheaded-volume-init' '$F' && ! grep -qF netbox '$F'"
check "5 a quote in a name is escaped" has 'unheaded_container_running{name="unheaded-\"odd\"",project="unheaded-dev",service="odd"} 1'
check "6 units: failed, restarts, active; oneshot and not-found skipped" bash -c "grep -qxF 'unheaded_unit_failed{unit=\"unheaded-wiki-server.service\"} 1' '$F' &&
    grep -qxF 'unheaded_unit_restarts{unit=\"unheaded-wiki-server.service\"} 15' '$F' && grep -qxF 'unheaded_unit_active{unit=\"unheaded-kev-poller.service\"} 1' '$F' &&
    ! grep -qF well-backup '$F' && ! grep -qF nginx '$F'"
check "7 last run and docker_ok" bash -c "grep -qE '^unheaded_host_state_last_run_timestamp_seconds [0-9]+$' '$F' && grep -qxF 'unheaded_host_state_docker_ok 1' '$F'"
# Every line must pass huginn's filter (cmd/huginn/textfile.go: textfileSample,
# textfileComment, textfileLabel), or huginn drops it.
sample='^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^{}]*\})?[ 	]+([-+]?([0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?|Inf|NaN))$'
comment='^# (HELP|TYPE) [a-zA-Z_:][a-zA-Z0-9_:]*( .*)?$'
all_lines_ok() {
    local l
    while IFS= read -r l; do
        [[ -z "$l" ]] && continue
        [[ "$l" =~ $sample || "$l" =~ $comment ]] || { echo "  bad line: $l"; return 1; }
    done <"$F"
    [[ "$(wc -c <"$F")" -lt 65536 ]]
}
check "9 every line passes huginn's regex; under 64 KiB" all_lines_ok
check "10 atomic: nothing but host_state.prom left in the directory" bash -c "[[ \"\$(ls -A '$T/out')\" == host_state.prom ]]"
check "10 file mode 0644" bash -c "[[ \"\$(stat -c %a '$F')\" == 644 ]]"

FAKE_DOCKER_FAIL=1 run || true
check "8 docker down: units still written, docker_ok 0, no container lines" bash -c "grep -qxF 'unheaded_host_state_docker_ok 0' '$F' &&
    grep -qF 'unheaded_unit_failed{unit=\"unheaded-wiki-server.service\"} 1' '$F' && ! grep -q '^unheaded_container_' '$F'"

HOST_STATE_PROJECTS='bad;rm' run >/dev/null 2>&1 && st=0 || st=$?
check "a bad setting is refused with exit 2" test "$st" -eq 2

echo "host-state-textfile: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
