#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# maintenance.sh — silence every notifying alert while you work on purpose
# (ADR-100). A reboot or a stack restart burns error budget like an outage
# does; telling yourself about it by mail is the self-made spam the policy
# exists to prevent.
#
#   scripts/monitoring/maintenance.sh 30m "reboot west for the kernel"
#   scripts/monitoring/maintenance.sh --list
#   scripts/monitoring/maintenance.sh --end      # end every maintenance silence now
#
# Silences urgency page and ticket (urgency none notifies no one anyway).
# Alerts still fire and show in Alertmanager and Grafana; only mail stops.
# Alertmanager's API is on 127.0.0.1:9093 on west (AM_URL to override).
#
# The duration is how long the work takes. A burn-rate tier keeps firing
# until its short window is clean again, so each silence runs a tail past
# it: pages 30m (medium tier), tickets 6h (steady tier). Ending the silence
# with the work mailed about the planned outage once it was over.
set -euo pipefail

AM_URL="${AM_URL:-http://127.0.0.1:9093}"
PREFIX="maintenance:"
# Longest short window per urgency (cmd/alert-policy tiers; its tests check).
PAGE_TAIL_MIN=30
TICKET_TAIL_MIN=360
die() { echo "[FAIL] $*" >&2; exit 1; }
command -v curl >/dev/null || die "curl is required"
command -v python3 >/dev/null || die "python3 is required"

silences() { curl -fsS -m 5 "$AM_URL/api/v2/silences"; }

case "${1:-}" in
--list)
	silences | python3 -c '
import json, sys
for s in json.load(sys.stdin):
    if s["status"]["state"] == "active" and s["comment"].startswith(sys.argv[1]):
        print(s["id"], "until", s["endsAt"], "by", s["createdBy"], "-", s["comment"])' "$PREFIX"
	;;
--end)
	ids="$(silences | python3 -c '
import json, sys
for s in json.load(sys.stdin):
    if s["status"]["state"] == "active" and s["comment"].startswith(sys.argv[1]):
        print(s["id"])' "$PREFIX")"
	for id in $ids; do
		curl -fsS -m 5 -X DELETE "$AM_URL/api/v2/silence/$id" >/dev/null
		echo "[ok] ended $id"
	done
	[[ -n "$ids" ]] || echo "[ok] no maintenance silence active"
	;;
"" | -h | --help)
	sed -n '5,22p' "$0"
	;;
*)
	duration="$1"
	reason="${2:-}"
	[[ "$duration" =~ ^[0-9]+[mh]$ ]] || die "duration like 30m or 2h, got '$duration'"
	[[ -n "$reason" ]] || die "say why: maintenance.sh $duration \"reason\""
	n="${duration%[mh]}"
	[[ "$duration" == *h ]] && secs=$((n * 3600)) || secs=$((n * 60))
	((secs <= 24 * 3600)) || die "at most 24h: a longer silence is a policy change (observability/alerting/policy.yaml)"
	silence() { # urgency, seconds
		body="$(python3 - "$1" "$2" "$PREFIX $reason" "${USER:-unknown}" <<'PY'
import datetime, json, sys
urgency, secs, comment, who = sys.argv[1], int(sys.argv[2]), sys.argv[3], sys.argv[4]
now = datetime.datetime.now(datetime.timezone.utc)
print(json.dumps({
    "matchers": [{"name": "urgency", "value": urgency, "isRegex": False, "isEqual": True}],
    "startsAt": now.isoformat(),
    "endsAt": (now + datetime.timedelta(seconds=secs)).isoformat(),
    "createdBy": who,
    "comment": comment,
}))
PY
)"
		curl -fsS -m 5 -X POST -H 'Content-Type: application/json' "$AM_URL/api/v2/silences" -d "$body" |
			python3 -c 'import json,sys; print(json.load(sys.stdin)["silenceID"])'
	}
	pid="$(silence page $((secs + PAGE_TAIL_MIN * 60)))"
	tid="$(silence ticket $((secs + TICKET_TAIL_MIN * 60)))"
	echo "[ok] silenced for $duration of work: pages +${PAGE_TAIL_MIN}m ($pid), tickets +$((TICKET_TAIL_MIN / 60))h ($tid): $reason"
	echo "     end early: $0 --end"
	;;
esac
