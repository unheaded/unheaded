#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-planned-stop.sh: planned-stop.sh against a temp directory, no sudo
# (ADR-147 decision 4, S-SEE-AND-SURVIVE Phase 8).
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/planned-stop.sh"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
export PLANNED_STOP_DIR="$T" PLANNED_STOP_SUDO=
F="$T/planned_stop.prom"
pass=0
fail=0
check() { # check <name> <command...>
    local name="$1"; shift
    if "$@"; then echo "[PASS] $name"; pass=$((pass + 1)); else echo "[FAIL] $name"; fail=$((fail + 1)); fi
}
samples() { grep -c '^unheaded_planned_stop_until_timestamp_seconds' "$F" || true; }
value() { awk -v s="{scope=\"$1\"}" 'index($1, s) { print $2 }' "$F"; }
near() { # near <value> <expected> : within 5 s
    local v="$1" e="$2"
    [[ -n "$v" ]] && (( v >= e - 5 && v <= e + 5 ))
}

bash "$SCRIPT" all 30m >/dev/null
check "1 all 30m" near "$(value all)" "$(( $(date +%s) + 1800 ))"
bash "$SCRIPT" wotan 1h >/dev/null
check "2 a second scope: two sample lines" test "$(samples)" -eq 2
bash "$SCRIPT" all 15m >/dev/null
check "3 the all line is replaced" bash -c "[[ $(samples) -eq 2 ]]"
check "3 its new value is now+900" near "$(value all)" "$(( $(date +%s) + 900 ))"
bash "$SCRIPT" --end wotan >/dev/null
check "4 --end wotan leaves one line" test "$(samples)" -eq 1

# 5. A mark already past is dropped on the next write.
printf 'unheaded_planned_stop_until_timestamp_seconds{scope="old"} 1000\n' >>"$F"
bash "$SCRIPT" kanban 10m >/dev/null
check "5 a past mark is dropped" bash -c "! grep -q 'scope=\"old\"' '$F' && [[ $(samples) -eq 2 ]]"

# 6. Bad input exits 2 and changes nothing.
cp "$F" "$T/before"
st=0; bash "$SCRIPT" 'Bad Scope' 10m >/dev/null 2>&1 || st=$?
check "6 a bad scope exits 2" test "$st" -eq 2
st=0; bash "$SCRIPT" all 31d >/dev/null 2>&1 || st=$?
check "6 over 30d exits 2" test "$st" -eq 2
st=0; bash "$SCRIPT" all 10x >/dev/null 2>&1 || st=$?
check "6 a bad duration exits 2" test "$st" -eq 2
check "6 the file is unchanged" cmp -s "$F" "$T/before"

check "7 --list prints the scope and a UTC time" bash -c "bash '$SCRIPT' --list | grep -qE '^all [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z\$'"

# 8. Every line passes huginn's filter (cmd/huginn/textfile.go:
# textfileSample, textfileComment); the only comments are HELP and TYPE.
sample='^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^{}]*\})?[ 	]+([-+]?([0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?|Inf|NaN))$'
comment='^# (HELP|TYPE) [a-zA-Z_:][a-zA-Z0-9_:]*( .*)?$'
lines_ok() {
    local l
    while IFS= read -r l; do
        [[ "$l" =~ $sample || "$l" =~ $comment ]] || { echo "  bad line: $l"; return 1; }
    done <"$F"
}
check "8 every line is one huginn forwards" lines_ok
check "8 no file is left beside it" bash -c "[[ \"\$(ls -A '$T' | grep -v -x -e planned_stop.prom -e before)\" == '' ]]"

bash "$SCRIPT" --end >/dev/null
check "9 --end with no scope empties the marks" bash -c "[[ $(samples) -eq 0 ]] && [[ \$(grep -c '^# ' '$F') -eq 2 ]] && [[ \$(wc -l <'$F') -eq 2 ]]"

# 10. kingdom-shutdown.sh runs on every reboot (kingdom-shutdown.service,
# ExecStop) while kingdom-startup.service may not run at boot, so its "all"
# mark must end on its own within hours, not 30 days: a stale "all" mark mutes
# ContainerStopped and reads every SLO good (review 2026-11-15, REV-2). Read
# from the script, never run: it stops the stack.
SHUTDOWN="$(cd "$(dirname "$0")/.." && pwd)/kingdom-shutdown.sh"
default="$(sed -n 's/^PLANNED_FOR=\([0-9][0-9]*[mh]\)$/\1/p' "$SHUTDOWN")"
hours_ok() { # the default is minutes or hours, at most 12h
    local d="$1"
    case "$d" in
    *m) (( ${d%m} > 0 && ${d%m} <= 720 )) ;;
    *h) (( ${d%h} > 0 && ${d%h} <= 12 )) ;;
    *) return 1 ;;
    esac
}
check "10 kingdom-shutdown's mark defaults to hours ('${default}')" hours_ok "$default"
check "10 kingdom-shutdown marks for PLANNED_FOR, no fixed 30d" bash -c "grep -q 'planned-stop.sh\" all \"\${PLANNED_FOR}\"' '$SHUTDOWN' && ! grep -q 'planned-stop.sh\" all 30d' '$SHUTDOWN'"
check "10 kingdom-shutdown --planned refuses a bad duration" bash -c "! bash '$SHUTDOWN' --planned 99x --help >/dev/null 2>&1 && ! bash '$SHUTDOWN' --planned 31d --help >/dev/null 2>&1"
check "10 kingdom-shutdown --help names --planned" bash -c "bash '$SHUTDOWN' --help | grep -q -- '--planned'"

echo "planned-stop: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
