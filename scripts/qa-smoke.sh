#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# qa-smoke.sh — measure the running dev stack against the demo baseline.
#
# The staging ladder promotes ~124 commits in batches. The bar is "everything
# works as well or better than demo through these pulls". That is only checkable
# if the same probes run the same way on every batch, so this script is the
# definition of the bar rather than a per-batch judgement call.
#
# Exit codes:
#   0  every check passed
#   1  at least one check regressed
#
# Usage:
#   scripts/qa-smoke.sh              # human-readable
#   scripts/qa-smoke.sh --json       # machine-readable, for diffing batches
#
# Records to docs/battle-plans/qa-baseline/<ref>.json when --record is passed.

set -uo pipefail

JSON=0
RECORD=""
for a in "$@"; do
  case "$a" in
    --json)     JSON=1 ;;
    --record=*) RECORD="${a#--record=}" ;;
    -h|--help)  sed -n '3,20p' "$0"; exit 0 ;;
    *) echo "unknown argument: $a" >&2; exit 2 ;;
  esac
done

pass=0; fail=0
declare -a RESULTS

# check <name> <expected> <actual>
check() {
  local name="$1" expected="$2" actual="$3" ok
  if [ "$expected" = "$actual" ]; then ok=1; pass=$((pass+1)); else ok=0; fail=$((fail+1)); fi
  RESULTS+=("$(printf '{"check":"%s","expected":"%s","actual":"%s","ok":%s}' \
    "$name" "$expected" "$actual" "$ok")")
  if [ "$JSON" -eq 0 ]; then
    if [ "$ok" -eq 1 ]; then
      printf '  \033[32mPASS\033[0m  %-34s %s\n' "$name" "$actual"
    else
      printf '  \033[31mFAIL\033[0m  %-34s expected %s, got %s\n' "$name" "$expected" "$actual"
    fi
  fi
}

# curl prints "000" and exits non-zero when it cannot connect, so a `|| echo`
# fallback would concatenate and yield "000000". Capture, then substitute.
http() {
  local code
  code=$(curl -s -o /dev/null -w '%{http_code}' -m 5 "$1" 2>/dev/null)
  echo "${code:-000}"
}

# http_method <METHOD> <url>: status code for an arbitrary method (HEAD via -I).
http_method() {
  local code
  if [ "$1" = HEAD ]; then
    code=$(curl -s -o /dev/null -I -w '%{http_code}' -m 5 "$2" 2>/dev/null)
  else
    code=$(curl -s -o /dev/null -X "$1" -w '%{http_code}' -m 5 "$2" 2>/dev/null)
  fi
  echo "${code:-000}"
}

[ "$JSON" -eq 0 ] && echo "== container health =="

# Every compose service must be running. Health is only asserted where the
# service declares a healthcheck — services without one never report healthy.
for svc in wotan monad sophia dashboard-backend kanban-app timeguru \
           architect captain micromanager postgres clickhouse victoria \
           grafana traefik coredns vector cuirass akira; do
  state=$(docker compose ps --format '{{.Service}} {{.State}}' 2>/dev/null \
          | awk -v s="$svc" '$1==s{print $2; found=1} END{if(!found) print "absent"}')
  check "container/$svc" "running" "$state"
done

# A compose `command` is appended to the image ENTRYPOINT. One that starts
# with the binary again hands the binary its own path as the first argument,
# Go's flag parsing stops there, and every flag is silently ignored (akira,
# micromanager and architect ran like that until 2026-09-29).
for c in $(docker compose ps -q 2>/dev/null); do
  name=$(docker inspect -f '{{.Name}}' "$c" | tr -d /)
  dup=$(docker inspect -f '{{if and .Config.Entrypoint .Config.Cmd}}{{if eq (index .Config.Entrypoint 0) (index .Config.Cmd 0)}}repeated{{end}}{{end}}' "$c" 2>/dev/null)
  [ -n "$dup" ] && check "command/$name" "flags-reach-binary" "entrypoint-repeated"
done
check "command/no-repeated-entrypoint" "ok" "ok"

[ "$JSON" -eq 0 ] && echo "== HTTP endpoints =="

check "http/dashboard"   "200" "$(http http://localhost:20000/)"
check "http/kanban"      "200" "$(http http://localhost:20001/)"
check "http/victoria"    "200" "$(http http://localhost:8428/)"
check "http/clickhouse"  "200" "$(http http://localhost:8123/)"
check "http/grafana"     "302" "$(http http://localhost:3001/)"

for sp in timeguru:19000 architect:19001 captain:19002 micromanager:19003 \
          monad:19004 sophia:19005 cuirass:19006 wotan:18000; do
  check "health/${sp%%:*}" "200" "$(http "http://localhost:${sp##*:}/health")"
done

# Probe endpoints answer GET and HEAD, and 405 to anything else
# (httputil.ProbeMethods). Services disagreed: some returned 200 to any
# method, others refused HEAD.
for sp in timeguru:19000 architect:19001 captain:19002 micromanager:19003 \
          monad:19004 sophia:19005 cuirass:19006 wotan:18000 \
          dashboard:20000 kanban:20001 akira:19100; do
  check "probe-methods/${sp%%:*}" "200/405" \
    "$(http_method HEAD "http://localhost:${sp##*:}/health")/$(http_method BREW "http://localhost:${sp##*:}/health")"
done

# Akira's consensus (ADR-029): every service has a fresh vote and sits in the
# OK band. Before its first sweep (30 s after start) this reads 0/10.
cons=$(curl -s -m 5 http://localhost:19100/api/v1/status 2>/dev/null | python3 -c '
import sys, json
try:
    v = json.load(sys.stdin)
    ok = sum(1 for x in v.values() if x.get("severity") == "OK")
    print("%d/%d" % (ok, len(v)))
except Exception:
    print("unreadable")' 2>/dev/null)
check "akira/consensus-ok" "10/10" "${cons:-unreadable}"

[ "$JSON" -eq 0 ] && echo "== data plane =="

# The Well: kanban must serve a non-empty task set. WELL_DB decides which
# database is read, so this catches the routing defect as well as an outage.
rows=$(docker exec unheaded-postgres psql -U unheaded -d "${WELL_DB:-unheaded_app}" \
       -tAc "SELECT count(*) FROM kanban_tasks;" 2>/dev/null | tr -d ' ')
[ -z "$rows" ] && rows="query-failed"
check "well/kanban_tasks>0" "true" "$([ "${rows:-0}" -gt 0 ] 2>/dev/null && echo true || echo "false($rows)")"

# Flow Graph: static is the failure mode, so assert movement, not presence.
# demo-trace-injector is a bare-metal daemon (ADR-088) and is not started by
# docker compose — a static graph usually means it is simply not running.
f1=$(curl -s -m 5 http://localhost:20000/api/v1/flows 2>/dev/null \
     | python3 -c 'import json,sys; print(json.load(sys.stdin).get("stats",{}).get("total_packets",0))' 2>/dev/null || echo 0)
sleep 4
f2=$(curl -s -m 5 http://localhost:20000/api/v1/flows 2>/dev/null \
     | python3 -c 'import json,sys; print(json.load(sys.stdin).get("stats",{}).get("total_packets",0))' 2>/dev/null || echo 0)
check "flows/advancing" "true" "$([ "${f2:-0}" -gt "${f1:-0}" ] 2>/dev/null && echo true || echo "false($f1->$f2)")"

[ "$JSON" -eq 0 ] && echo "== bare-metal daemons (ADR-088) =="

# ADR-088 classes 11 services as native bare-metal daemons rather than compose
# services, so `docker compose up` never starts them and `docker compose ps`
# never shows them missing. The stack reads as fully healthy while parts of the
# product are simply absent — this check scored 30/32 with the wiki down and
# never noticed, which is the blind spot being closed here.
#
# Only the demo surface is scored. The rest are reported so they are visible,
# but not counted: most are host-specific (heimdall/gjallarhorn need real
# hardware, akira and routing-health are bare-metal-host roles) and scoring them
# would make this check permanently red, which is how a gate gets ignored.

check "wiki/http"           "200"  "$(http http://localhost:20002/health)"
check "wiki/pages"          "200"  "$(http http://localhost:20002/wiki/)"

# trace-collector publishes the real eBPF events behind the Flow Graph,
# Latency and Events pages (packet_marker/flow_tracker on br-unhe-data, TCP
# kprobes). It replaced demo-trace-injector's synthetic events on
# 2026-09-30; the injector must not run alongside it.
if pgrep -f '^/tmp/trace-collector-go --unified' >/dev/null 2>&1; then tc="running"; else tc="absent"; fi
check "trace-collector/running" "running" "$tc"
if pgrep -f '^/tmp/demo-trace-injector' >/dev/null 2>&1; then inj="running"; else inj="absent"; fi
check "demo-injector/absent" "absent" "$inj"

if [ "$JSON" -eq 0 ]; then
    for d in unheaded-daemon akira trace-collector-go protocol-api \
             heimdall-daemon gjallarhorn-listener upc-tty-bridge routing-health; do
        if pgrep -f "$d" >/dev/null 2>&1; then st="running"; else st="absent"; fi
        printf '  \033[2mINFO\033[0m  %-34s %s (not scored)\n' "daemon/$d" "$st"
    done
fi

if [ "$JSON" -eq 1 ]; then
  printf '{"pass":%d,"fail":%d,"checks":[%s]}\n' "$pass" "$fail" \
    "$(IFS=,; echo "${RESULTS[*]}")"
else
  echo
  printf 'passed %d, failed %d\n' "$pass" "$fail"
fi

if [ -n "$RECORD" ]; then
  mkdir -p docs/battle-plans/qa-baseline
  printf '{"pass":%d,"fail":%d,"checks":[%s]}\n' "$pass" "$fail" \
    "$(IFS=,; echo "${RESULTS[*]}")" > "docs/battle-plans/qa-baseline/${RECORD}.json"
  [ "$JSON" -eq 0 ] && echo "recorded to docs/battle-plans/qa-baseline/${RECORD}.json"
fi

[ "$fail" -eq 0 ]
