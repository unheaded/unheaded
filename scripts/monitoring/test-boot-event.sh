#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-boot-event.sh: boot-event.sh against a fake /proc, a fake docker and a
# fake systemctl on PATH (decision pack 2026-11-18 row 22, ADR-150 choice 15).
# Touches no Docker daemon, no unit and no file outside its temp directory;
# runs as any user.
set -euo pipefail

SCRIPT="$(cd "$(dirname "$0")" && pwd)/boot-event.sh"
command -v jq >/dev/null || { echo "[FAIL] jq is required" >&2; exit 2; }

T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/bin" "$T/state" "$T/textfile" "$T/proc/sys/kernel/random"
pass=0
fail=0
check() { # check <name> <command...>
    local name="$1"; shift
    if "$@"; then echo "[PASS] $name"; pass=$((pass + 1)); else echo "[FAIL] $name"; fail=$((fail + 1)); fi
}

BOOT_A=11111111-1111-4111-8111-111111111111
BOOT_B=22222222-2222-4222-8222-222222222222
BOOT_C=33333333-3333-4333-8333-333333333333
BOOT_D=44444444-4444-4444-8444-444444444444
BOOT_E=55555555-5555-4555-8555-555555555555
H="$T/state/boots.jsonl"
STAMP="$T/state/clean-shutdown"
F="$T/textfile/host_boot.prom"
MARKS="$T/textfile/planned_stop.prom"

# boot <id> <btime>: the fake /proc now says this boot.
boot() {
    printf '%s\n' "$1" >"$T/proc/sys/kernel/random/boot_id"
    printf 'cpu  1 2 3 4\nbtime %s\nprocesses 5\n' "$2" >"$T/proc/stat"
}
# ps <project> <"service state" lines...>: what the fake docker lists.
ps() {
    local p="$1"; shift
    printf '%s\n' "$@" >"$T/ps-$p.txt"
}

cat >"$T/bin/docker" <<FAKE
#!/usr/bin/env bash
[[ -n "\${FAKE_DOCKER_FAIL:-}" ]] && exit 1
[[ "\$1" == ps ]] || exit 1
p=""
for a in "\$@"; do
    case "\$a" in label=com.docker.compose.project=*) p="\${a#label=com.docker.compose.project=}" ;; esac
done
[[ -f "$T/ps-\$p.txt" ]] && cat "$T/ps-\$p.txt"
exit 0
FAKE
cat >"$T/bin/systemctl" <<FAKE
#!/usr/bin/env bash
[[ "\$1" == is-system-running ]] || exit 1
cat "$T/system-state"
[[ "\$(cat "$T/system-state")" == running ]]
FAKE
chmod +x "$T/bin/docker" "$T/bin/systemctl"
echo running >"$T/system-state"

run() { PATH="$T/bin:$PATH" BOOT_EVENT_PROC="$T/proc" BOOT_EVENT_HOST="${BOOT_EVENT_HOST:-west}" bash "$SCRIPT" "$@"; }
record() { run record "$T/state" "$T/textfile"; }
last() { tail -n 1 "$H" | jq -r "$1"; }
lines() { wc -l <"$H"; }

# 1. The first boot on record: nothing to compare with, the stack down.
boot "$BOOT_A" 1763689440
ps unheaded-dev "wotan exited" "postgres exited" "volume-init exited" "clickhouse-schema exited"
ps monitoring "alert-mailer running" "prometheus exited"
if ! record >/dev/null; then echo "[FAIL] boot-event.sh record exited non-zero"; exit 1; fi
check "1 one record, valid JSON" bash -c "[[ \$(wc -l <'$H') -eq 1 ]] && jq -e . '$H' >/dev/null"
check "1 boot id, boot time and host" test "$(last '[.boot_id, .booted_at, .host] | @tsv')" = "$BOOT_A	1763689440	west"
check "1 no earlier boot: previous unknown" test "$(last .previous)" = unknown
check "1 recorded_at is now" bash -c "v=$(last .recorded_at); n=\$(date +%s); (( v >= n - 5 && v <= n + 5 ))"
check "1 stack down: the mailer alone and one-shots do not count" test "$(last '[.stack, .stack_running, .stack_total] | @tsv')" = "down	0	2"
check "1 the projects that are the stack are in the record" test "$(last '.stack_projects | join(",")')" = unheaded-dev

# 2. The same boot again (a restart of the unit): no second record.
record >/dev/null
check "2 recording the same boot twice keeps one record" test "$(lines)" -eq 1

# 3. A clean shutdown under a planned-stop mark for all, then a boot.
echo stopping >"$T/system-state"
run shutdown "$T/state" >/dev/null
check "3 a shutdown writes the stamp with this boot's id" bash -c "grep -qE '^$BOOT_A [0-9]+\$' '$STAMP'"
printf 'unheaded_planned_stop_until_timestamp_seconds{scope="all"} %s\n' "$(( $(date +%s) + 3600 ))" >"$MARKS"
echo running >"$T/system-state"
boot "$BOOT_B" 1763700000
ps unheaded-dev "wotan running" "postgres running" "volume-init exited"
record >/dev/null
check "3 two records" test "$(lines)" -eq 2
check "3 previous shutdown planned, its time kept" bash -c "[[ $(last .previous) == planned ]] && (( $(last .shutdown_at) > 0 ))"
check "3 the stamp is used once" test ! -e "$STAMP"
check "3 stack up: every container running" test "$(last '[.stack, .stack_running, .stack_total] | @tsv')" = "up	2	2"

# 4. A clean shutdown with no mark for all open at that time: clean.
echo stopping >"$T/system-state"
run shutdown "$T/state" >/dev/null
echo running >"$T/system-state"
printf 'unheaded_planned_stop_until_timestamp_seconds{scope="all"} 1000\nunheaded_planned_stop_until_timestamp_seconds{scope="wotan"} %s\n' "$(( $(date +%s) + 3600 ))" >"$MARKS"
boot "$BOOT_C" 1763710000
ps unheaded-dev "wotan running" "postgres exited"
record >/dev/null
check "4 a past mark and another scope's mark are not a plan: clean" test "$(last .previous)" = clean
check "4 stack partial" test "$(last '[.stack, .stack_running, .stack_total] | @tsv')" = "partial	1	2"

# 5. No stamp (power loss, hang, forced reset): unclean.
boot "$BOOT_D" 1763720000
FAKE_DOCKER_FAIL=1 record >/dev/null
check "5 no stamp: previous unclean" test "$(last '[.previous, .shutdown_at] | @tsv')" = "unclean	0"
check "5 Docker not answering: stack unknown, the boot still recorded" test "$(last '[.boot_id, .stack] | @tsv')" = "$BOOT_D	unknown"

# 6. The unit stopped by hand (the system is not shutting down): no stamp.
run shutdown "$T/state" >/dev/null
check "6 a stop that is not a shutdown writes no stamp" test ! -e "$STAMP"

# 7. A stamp of some other boot than the last one on record is not this
# shutdown's: unclean.
printf '%s %s\n' "$BOOT_A" "$(date +%s)" >"$STAMP"
boot "$BOOT_E" 1763730000
ps unheaded-dev "wotan exited"
record >/dev/null
check "7 a stamp of another boot: unclean, and it is removed" bash -c "[[ $(last .previous) == unclean ]] && [[ ! -e '$STAMP' ]]"

# 8. The textfile: this boot, for huginn.
has() { grep -qxF "$1" "$F"; }
check "8 boot time" has 'unheaded_host_boot_timestamp_seconds 1763730000'
check "8 previous shutdown: 1 on the state that applies, 0 on the others" bash -c "
    grep -qxF 'unheaded_host_boot_previous_shutdown{state=\"unclean\"} 1' '$F' &&
    grep -qxF 'unheaded_host_boot_previous_shutdown{state=\"planned\"} 0' '$F' &&
    grep -qxF 'unheaded_host_boot_previous_shutdown{state=\"clean\"} 0' '$F' &&
    grep -qxF 'unheaded_host_boot_previous_shutdown{state=\"unknown\"} 0' '$F'"
check "8 stack state at boot" has 'unheaded_host_boot_stack_up 0'
# Every line must pass huginn's filter (cmd/huginn/textfile.go).
sample='^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^{}]*\})?[ 	]+([-+]?([0-9]*\.?[0-9]+([eE][-+]?[0-9]+)?|Inf|NaN))$'
comment='^# (HELP|TYPE) [a-zA-Z_:][a-zA-Z0-9_:]*( .*)?$'
all_lines_ok() {
    local l
    while IFS= read -r l; do
        [[ -z "$l" ]] && continue
        [[ "$l" =~ $sample || "$l" =~ $comment ]] || { echo "  bad line: $l"; return 1; }
    done <"$F"
}
check "8 every line passes huginn's regex" all_lines_ok
check "8 atomic: no temp file left beside the textfile or the history" bash -c "
    [[ \"\$(ls -A '$T/textfile' | sort | tr '\n' ' ')\" == 'host_boot.prom planned_stop.prom ' ]] && [[ \"\$(ls -A '$T/state')\" == boots.jsonl ]]"
check "8 both files mode 0644 (the mailer's container reads the history)" bash -c "[[ \$(stat -c %a '$F') == 644 && \$(stat -c %a '$H') == 644 ]]"
# With Docker not answering the stack line is left out, not guessed.
boot "$BOOT_D" 1763740000
FAKE_DOCKER_FAIL=1 record >/dev/null
check "8 stack unknown: no stack_up line" bash -c "! grep -q '^unheaded_host_boot_stack_up' '$F'"

# 9. The history is bounded.
before="$(lines)"
boot "$BOOT_A" 1763750000
BOOT_EVENT_KEEP=3 record >/dev/null
check "9 BOOT_EVENT_KEEP bounds the history, newest last" bash -c "[[ $before -gt 3 && \$(wc -l <'$H') -eq 3 ]] && [[ $(last .boot_id) == $BOOT_A ]]"

# 10. Bad settings and a bad boot id are refused; nothing is written.
cp "$H" "$T/before"
boot "$BOOT_B" 1763760000
st=0; BOOT_EVENT_STACK_PROJECTS='bad;rm' record >/dev/null 2>&1 || st=$?
check "10 a bad project list exits 2" test "$st" -eq 2
st=0; BOOT_EVENT_HOST='we st' record >/dev/null 2>&1 || st=$?
check "10 a bad host name exits 2" test "$st" -eq 2
st=0; BOOT_EVENT_KEEP=0 record >/dev/null 2>&1 || st=$?
check "10 a bad keep count exits 2" test "$st" -eq 2
boot 'not-a-boot-id"; rm' 1763760000
st=0; record >/dev/null 2>&1 || st=$?
check "10 a boot id that is not one exits non-zero" test "$st" -ne 0
check "10 the history is unchanged" cmp -s "$H" "$T/before"
st=0; run frobnicate >/dev/null 2>&1 || st=$?
check "10 an unknown verb exits 2" test "$st" -eq 2

echo "boot-event: $pass passed, $fail failed"
[[ "$fail" -eq 0 ]]
