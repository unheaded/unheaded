#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-zhen-switch.sh: proves, from outside, that the Zhen module is on or off
# everywhere it shows (ADR-149, ADR-147 decision 15). It reads and never
# changes anything. Run it with sudo on the host: C10 asks the Well's
# container, and the dashboard's CA is root-readable.
#
#   sudo scripts/test-zhen-switch.sh --expect on|off [--wait N] [--no-well]
#
#   C1  unheaded-zhen.target active / not active
#   C2  zhen-agentd, unheaded-zhen-ui-front active (and unheaded-zhen-app,
#       unheaded-llama when enabled) / all four not active
#   C3  $MODULES_RUN/zhen is a directory / absent
#   C4  Prometheus has rule group zhen / has not
#   C5  Prometheus has an active zhen-agentd target / has none
#   C6  VictoriaMetrics has an active zhen-agentd target / has none
#   C7  dashboard unheaded_dashboard_health_targets{module="zhen"} is 2 / 0 or absent
#   C8  the menu script names zhenai / does not
#   C9  20103 and 20111 listen / none of 8081, 20103, 20111, 20112 listens
#   C10 not checked when on / no session of app_zhen (SKIP with --no-well)
#   C11 unheaded_module_enabled{module="zhen"} is 1 / 0
#   C12 a zhen-agentd process runs / none runs, whoever started it: the
#       overnight run is a user unit and timer outside the target (its
#       ExecCondition and -worker's own check keep it off; this sees it)
#
# --wait N repeats the whole set every 5 s until it passes or N seconds pass.
# Exit 0 pass, 1 fail, 2 usage.
set -uo pipefail

PROM_URL="${PROM_URL:-http://127.0.0.1:9090}"
VM_URL="${VM_URL:-http://127.0.0.1:8428}"
DASH_URL="${DASH_URL:-https://127.0.0.1:20000}"
CA="${UNHEADED_TLS_CA_CERT:-/etc/unheaded/tls/ca.pem}"
MODULES_RUN="${MODULES_RUN:-/run/unheaded/modules}"

usage() {
    echo "usage: test-zhen-switch.sh --expect on|off [--wait N] [--no-well]" >&2
    exit 2
}

expect="" wait=0 well=true
while [ "$#" -gt 0 ]; do
    case "$1" in
        --expect) [ "$#" -ge 2 ] || usage; expect="$2"; shift 2 ;;
        --wait) [ "$#" -ge 2 ] || usage; wait="$2"; shift 2 ;;
        --no-well) well=false; shift ;;
        *) usage ;;
    esac
done
case "$expect" in on|off) ;; *) usage ;; esac
[[ "$wait" =~ ^[0-9]{1,5}$ ]] || usage

OUT="" FAILED=0
pass() { OUT+="PASS $1: $2"$'\n'; }
fail() { OUT+="FAIL $1: $2"$'\n'; FAILED=1; }
skip() { OUT+="SKIP $1: $2"$'\n'; }
verdict() { # id, ok(0/1), what
    if [ "$2" -eq 0 ]; then pass "$1" "$3"; else fail "$1" "$3"; fi
}

get() { curl -s -m5 --cacert "$CA" "$1" 2>/dev/null; }

# json_has FILTER: reads JSON on stdin, prints yes/no for the named test.
json_test() {
    python3 -I -c '
import json, sys
kind = sys.argv[1]
try:
    d = json.load(sys.stdin)["data"]
except Exception:
    print("error"); sys.exit(0)
if kind == "rules":
    print("yes" if any(g.get("name") == "zhen" for g in d.get("groups", [])) else "no")
elif kind == "targets":
    print("yes" if any(t.get("labels", {}).get("job") == "zhen-agentd" for t in d.get("activeTargets", [])) else "no")
elif kind == "value":
    r = d.get("result", [])
    print(r[0]["value"][1] if r else "absent")
' "$1"
}

active() { [ "$(systemctl is-active "$1" 2>/dev/null)" = active ]; }
enabled() { [ "$(systemctl is-enabled "$1" 2>/dev/null)" = enabled ]; }

c1() {
    if [ "$expect" = on ]; then active unheaded-zhen.target; verdict C1 $? "unheaded-zhen.target active"
    else ! active unheaded-zhen.target; verdict C1 $? "unheaded-zhen.target not active"; fi
}

c2() {
    local bad="" u
    if [ "$expect" = on ]; then
        for u in zhen-agentd.service unheaded-zhen-ui-front.service; do active "$u" || bad+=" $u"; done
        for u in unheaded-zhen-app.service unheaded-llama.service; do
            if enabled "$u"; then active "$u" || bad+=" $u"; fi
        done
        [ -z "$bad" ]; verdict C2 $? "Zhen units active${bad:+; not active:$bad}"
    else
        for u in zhen-agentd.service unheaded-zhen-ui-front.service unheaded-zhen-app.service unheaded-llama.service; do
            active "$u" && bad+=" $u"
        done
        [ -z "$bad" ]; verdict C2 $? "no Zhen unit active${bad:+; active:$bad}"
    fi
}

c3() {
    if [ "$expect" = on ]; then [ -d "${MODULES_RUN}/zhen" ]; verdict C3 $? "${MODULES_RUN}/zhen present"
    else [ ! -e "${MODULES_RUN}/zhen" ]; verdict C3 $? "${MODULES_RUN}/zhen absent"; fi
}

want_yes() { # id, answer, what
    if [ "$expect" = on ]; then [ "$2" = yes ]; verdict "$1" $? "$3 (${2})"
    else [ "$2" = no ]; verdict "$1" $? "no $3 (${2})"; fi
}

c4() { want_yes C4 "$(get "${PROM_URL}/api/v1/rules" | json_test rules)" "Prometheus rule group zhen"; }
c5() { want_yes C5 "$(get "${PROM_URL}/api/v1/targets" | json_test targets)" "Prometheus target job=zhen-agentd"; }
c6() { want_yes C6 "$(get "${VM_URL}/api/v1/targets" | json_test targets)" "VictoriaMetrics target job=zhen-agentd"; }

c7() {
    local v
    v="$(get "${DASH_URL}/metrics" | awk '$1 == "unheaded_dashboard_health_targets{module=\"zhen\"}" {print $2}' | tail -1)"
    if [ "$expect" = on ]; then [ "$v" = 2 ]; verdict C7 $? "dashboard module targets = ${v:-absent}, want 2"
    else [ -z "$v" ] || [ "$v" = 0 ]; verdict C7 $? "dashboard module targets = ${v:-absent}, want 0 or absent"; fi
}

c8() {
    local body has=no
    body="$(get "${DASH_URL}/auth/nav.js")"
    [ -n "$body" ] || { fail C8 "menu script unreadable"; return; }
    grep -q '"zhenai"' <<<"$body" && has=yes
    want_yes C8 "$has" "menu entry zhenai"
}

c9() {
    local ports
    ports="$(ss -ltnH 2>/dev/null | awk '{print $4}' | sed -n 's/.*:\([0-9][0-9]*\)$/\1/p' | sort -u)"
    if [ "$expect" = on ]; then
        grep -qx 20103 <<<"$ports" && grep -qx 20111 <<<"$ports"; verdict C9 $? "20103 and 20111 listen"
    else
        local hit
        hit="$(grep -x -E '8081|20103|20111|20112' <<<"$ports" | tr '\n' ' ')"
        [ -z "$hit" ]; verdict C9 $? "no Zhen port listens${hit:+; listening: $hit}"
    fi
}

c10() {
    if [ "$expect" = on ]; then skip C10 "app_zhen sessions not checked while on (idle pools)"; return; fi
    if [ "$well" = false ]; then skip C10 "app_zhen sessions (--no-well)"; return; fi
    local n
    n="$(docker exec -i unheaded-postgres psql -X -At -U "${WELL_SUPERUSER:-unheaded}" -d postgres \
        -c "SELECT count(*) FROM pg_stat_activity WHERE usename = 'app_zhen'" 2>/dev/null | tr -d '[:space:]')"
    [ "$n" = 0 ]; verdict C10 $? "app_zhen sessions = ${n:-unreadable}, want 0"
}

c11() {
    local q v
    q="$(python3 -I -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1], safe=""))' 'unheaded_module_enabled{module="zhen"}')"
    v="$(get "${VM_URL}/api/v1/query?query=${q}" | json_test value)"
    if [ "$expect" = on ]; then [ "$v" = 1 ]; verdict C11 $? "unheaded_module_enabled = ${v}, want 1"
    else [ "$v" = 0 ]; verdict C11 $? "unheaded_module_enabled = ${v}, want 0"; fi
}

c12() {
    local procs
    procs="$(pgrep -a -x zhen-agentd 2>/dev/null | awk '{print $1}' | tr '\n' ' ')"
    procs="${procs% }"
    if [ "$expect" = on ]; then [ -n "$procs" ]; verdict C12 $? "zhen-agentd process running${procs:+ (pid $procs)}"
    else [ -z "$procs" ]; verdict C12 $? "no zhen-agentd process (system, user unit or by hand)${procs:+; running: pid $procs}"; fi
}

start="$(date +%s)"
while :; do
    OUT="" FAILED=0
    c1; c2; c3; c4; c5; c6; c7; c8; c9; c10; c11; c12
    elapsed=$(( $(date +%s) - start ))
    if [ "$FAILED" -eq 0 ] || [ "$elapsed" -ge "$wait" ]; then
        break
    fi
    left=$(( wait - elapsed ))
    sleep $(( left < 5 ? left : 5 ))
done

printf '%s' "$OUT"
if [ "$FAILED" -eq 0 ]; then
    echo "RESULT ${expect}: pass, ELAPSED=${elapsed}"
    exit 0
fi
echo "RESULT ${expect}: fail, ELAPSED=${elapsed}"
exit 1
