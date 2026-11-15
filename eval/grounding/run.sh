#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# run.sh — the grounding probe: does an answer name the right sources?
#
# Posts each prompt in prompts.jsonl to a running Zhen page and checks the
# source line of the answer, not the answer's facts (README.md).
#
# Usage: ZHEN_SESSION_FILE=/path/to/cookie-jar eval/grounding/run.sh BASE_URL
#   BASE_URL            e.g. https://127.0.0.1:20103
#   ZHEN_SESSION_FILE   a curl cookie jar holding a logged-in session, mode
#                       0600, made by the operator (README.md). This script
#                       never takes a password and never logs in.
#   ZHEN_CA             CA bundle for the front (default /etc/unheaded/tls/ca.pem)
#   ZHEN_GROUNDING_PROMPTS  prompt file (default: prompts.jsonl beside this script)
#
# Exit: 0 every row passed, 1 a row failed, 2 usage, missing tool or session.

set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd -P)"
prompts="${ZHEN_GROUNDING_PROMPTS:-$here/prompts.jsonl}"
ca="${ZHEN_CA:-/etc/unheaded/tls/ca.pem}"
base="${1:-}"
base="${base%/}"

[ -n "$base" ] || { echo "usage: ZHEN_SESSION_FILE=<cookie jar> run.sh BASE_URL" >&2; exit 2; }
command -v curl >/dev/null || { echo "grounding: curl not found" >&2; exit 2; }
command -v python3 >/dev/null || { echo "grounding: python3 not found" >&2; exit 2; }
[ -r "$prompts" ] || { echo "grounding: cannot read $prompts" >&2; exit 2; }

jar="${ZHEN_SESSION_FILE:-}"
[ -n "$jar" ] || { echo "grounding: set ZHEN_SESSION_FILE to a cookie jar from a login (README.md)" >&2; exit 2; }
[ -r "$jar" ] || { echo "grounding: cannot read ZHEN_SESSION_FILE" >&2; exit 2; }
mode="$(stat -c '%a' "$jar")"
case "$mode" in
    600|400) ;;
    *) echo "grounding: ZHEN_SESSION_FILE is mode $mode; it holds a session, make it 0600" >&2; exit 2 ;;
esac

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl_base=(curl --silent --max-time 600 --curves X25519MLKEM768 --cookie "$jar")
case "$base" in
    https://*) curl_base+=(--cacert "$ca") ;;
esac

rc=0
n=0
while IFS= read -r row; do
    [ -n "$row" ] || continue
    n=$((n + 1))
    # The row is passed as an argument, never spliced into program text.
    id="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$row")"
    python3 -c 'import json,sys; print(json.dumps({"question": json.loads(sys.argv[1])["prompt"], "session_id": "grounding-" + json.loads(sys.argv[1])["id"]}))' "$row" >"$tmp/req.json"
    code="$("${curl_base[@]}" --output "$tmp/resp.json" --write-out '%{http_code}' \
        --header 'Content-Type: application/json' --data "@$tmp/req.json" "$base/api/v1/query" || true)"
    if [ "$code" != 200 ]; then
        echo "FAIL $id: HTTP $code"
        [ "$code" = 401 ] && echo "     the session in ZHEN_SESSION_FILE has ended; log in again"
        rc=1
        continue
    fi
    verdict="$(python3 - "$row" "$tmp/resp.json" <<'PY'
import json
import sys

row = json.loads(sys.argv[1])
with open(sys.argv[2], encoding='utf-8') as f:
    resp = json.load(f)
ids = [str(s.get('id', '')) for s in resp.get('sources') or []]
tokens = resp.get('tokens_used')
problems = []
if row.get('expect') == 'no_source' and ids:
    problems.append('wanted no source, got ' + ', '.join(ids))
wanted = row.get('expect_sources_any')
if wanted and not any(i.startswith(p) for i in ids for p in wanted):
    problems.append('wanted a source starting with ' + ' or '.join(wanted) + ', got ' + (', '.join(ids) or 'none'))
if not isinstance(tokens, int) or tokens <= 0:
    problems.append(f'tokens_used is {tokens!r}, want more than 0')
print(('FAIL' if problems else 'PASS') + f' {row["id"]}: {tokens} tok, sources [{", ".join(ids)}]')
for p in problems:
    print('     ' + p)
PY
)"
    echo "$verdict"
    case "$verdict" in
        PASS*) ;;
        *) rc=1 ;;
    esac
done <"$prompts"

[ "$n" -gt 0 ] || { echo "grounding: no prompts in $prompts" >&2; exit 2; }
exit "$rc"
