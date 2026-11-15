#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-alert-path.sh — the alert path end to end on throwaway containers
# (ADR-100, FND-001): Prometheus with the repo's prometheus.yml sends to
# Alertmanager with the generated alertmanager.yml, which hands pages and
# tickets to alert-mailer (built from this tree), which mails an SMTP sink
# (Mailpit). Pins the promises that keep the inbox small:
#
#   1. urgency none reaches no one
#   2. a page is mailed, once, with the SLO in the subject
#   3. a ticket is not mailed (it waits for the daily digest)
#   4. past the mailer's cap nothing is mailed; the cap is reported
#   5. a firing Prometheus rule reaches Alertmanager at all
#   6. a run day (mailer.run_days) raises the cap
#   7. the pipeline heartbeat reaches the mailer (policy.yaml pipeline:)
#
# The only edits to the repo's configs: Alertmanager's page group_wait and
# group_interval are shortened so this runs in minutes, and the policy's mode/cap are set
# for the test (send, cap 2). Needs docker. Nothing touches the monitoring stack.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
AM_IMAGE="$(awk '/image: *prom\/alertmanager/ {print $2; exit}' "$REPO_ROOT/monitoring/docker-compose.yml")"
PROM_IMAGE="$(awk '/image: *prom\/prometheus/ {print $2; exit}' "$REPO_ROOT/monitoring/docker-compose.yml")"
SINK_IMAGE="axllent/mailpit:v1.27.10"
CURL_IMAGE="curlimages/curl:8.11.1"
MAILER_IMAGE="unheaded/alert-mailer:path-test"
[[ -n "$AM_IMAGE" && -n "$PROM_IMAGE" ]] || { echo "[FAIL] no prom/alertmanager or prom/prometheus image in monitoring/docker-compose.yml" >&2; exit 1; }
command -v docker >/dev/null || { echo "[FAIL] docker is required; this test does not pass by skipping" >&2; exit 2; }

T="$(mktemp -d)"
NET="uh-alert-path-$$"
cleanup() {
	docker rm -f "$NET-am" "$NET-sink" "$NET-mailer" "$NET-mailer2" "$NET-prom" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	rm -rf "$T"
}
trap cleanup EXIT
fail() { echo "[FAIL] $*" >&2; exit 1; }
api() { docker run --rm --network "$NET" "$CURL_IMAGE" -fsS "$@"; }
wait_for() { for _ in $(seq 1 60); do api "$1" >/dev/null 2>&1 && return 0; sleep 1; done; fail "$1 never answered"; }
mails() { api "http://$NET-sink:8025/api/v1/messages"; }

docker run --rm -v "$REPO_ROOT/monitoring/alertmanager:/c:ro" --entrypoint amtool "$AM_IMAGE" check-config /c/alertmanager.yml >/dev/null ||
	fail "the generated alertmanager.yml does not load"
echo "[ok] generated alertmanager.yml loads in $AM_IMAGE"

docker build -q -f "$REPO_ROOT/docker/alert-mailer/Dockerfile" -t "$MAILER_IMAGE" "$REPO_ROOT" >/dev/null

mkdir -p "$T/am" "$T/mailer" "$T/prom/rules"
# Every route whose receiver is page gets the short timing, not only the first
# one: the generator puts per-rule routes ahead of the urgency="page" catch-all
# (2026-10-27), and a page route left at group_interval 5m holds the second
# batch below past this test's wait.
awk '/^    - receiver: /{rcv=$3} rcv=="page" && /^      group_wait: 2m$/{sub(/2m$/,"1s")} rcv=="page" && /^      group_interval: 5m$/{sub(/5m$/,"2s")} {print}' \
	"$REPO_ROOT/monitoring/alertmanager/alertmanager.yml" >"$T/am/alertmanager.yml"
pages="$(grep -c '^    - receiver: page$' "$T/am/alertmanager.yml")"
{ [[ "$pages" -ge 1 ]] && [[ "$(grep -c '^      group_wait: 1s$' "$T/am/alertmanager.yml")" -eq "$pages" ]] &&
	[[ "$(grep -c '^      group_interval: 2s$' "$T/am/alertmanager.yml")" -eq "$pages" ]]; } ||
	fail "page timing anchors missing in alertmanager.yml (want group_wait 2m and group_interval 5m on each of $pages page routes)"
# The policy refuses an hourly cap above the daily one (DP-710), so both go to
# 2. The hourly cap is read first; which of the two holds a mail back is
# cmd/alert-mailer's unit tests. Here: two mails leave, the rest are held.
sed -e 's/^  mode: dry-run$/  mode: send/' -e 's/^  max_per_day: [0-9]*$/  max_per_day: 2/' \
	-e 's/^  max_per_hour: [0-9]*$/  max_per_hour: 2/' \
	"$REPO_ROOT/observability/alerting/policy.yaml" >"$T/mailer/policy.yaml"
{ grep -q '^  mode: send$' "$T/mailer/policy.yaml" && grep -q '^  max_per_day: 2$' "$T/mailer/policy.yaml" &&
	grep -q '^  max_per_hour: 2$' "$T/mailer/policy.yaml"; } || fail "policy anchors missing"
chmod -R a+rX "$T"

docker network create "$NET" >/dev/null
docker run -d --name "$NET-sink" --network "$NET" "$SINK_IMAGE" >/dev/null
# Plaintext, no login: the sink has no TLS. STARTTLS and the login are the
# default and are covered by cmd/alert-mailer's tests.
docker run -d --name "$NET-mailer" --network "$NET" --network-alias alert-mailer --read-only --tmpfs /var/lib/alert-mailer:uid=16710 \
	-e SMTP_SMARTHOST="$NET-sink:1025" -e SMTP_STARTTLS=false -e ALERT_MAIL_FROM=alerts@example.org -e ALERT_MAIL_TO=ops@example.org \
	-v "$T/mailer/policy.yaml:/etc/alert-mailer/policy.yaml:ro" "$MAILER_IMAGE" >/dev/null
docker run -d --name "$NET-am" --network "$NET" --network-alias alertmanager -v "$T/am:/etc/alertmanager:ro" "$AM_IMAGE" \
	--config.file=/etc/alertmanager/alertmanager.yml --storage.path=/tmp >/dev/null
wait_for "http://$NET-sink:8025/api/v1/messages"
wait_for "http://$NET-mailer:19130/health"
wait_for "http://$NET-am:9093/-/ready"

post() { api -X POST -H 'Content-Type: application/json' "http://$NET-am:9093/api/v2/alerts" -d "$1" >/dev/null; }
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
alert() { printf '{"labels":{"alertname":"%s","slo":"%s","urgency":"%s","tier":"fast"},"annotations":{"summary":"%s"},"startsAt":"%s"}' "$1" "$2" "$3" "$1 $2" "$now"; }
post "[$(alert HostDiskFull "" none),$(alert ErrorBudgetBurn wotan page),$(alert ErrorBudgetBurn overlay ticket)]"

msgs=""
for _ in $(seq 1 60); do
	msgs="$(mails 2>/dev/null || true)"
	grep -q 'error budget burning' <<<"$msgs" && break
	sleep 1
done
grep -q '"Subject":"\[page\] wotan: error budget burning"' <<<"$msgs" || fail "no page mail for wotan within 60s: $msgs"
grep -q '"Address":"ops@example.org"' <<<"$msgs" || fail "mail not addressed to ops@example.org"
sleep 8
msgs="$(mails)"
[[ "$(grep -o '"Subject":"[^"]*"' <<<"$msgs" | wc -l)" -eq 1 ]] || fail "expected exactly one mail (the page): $(grep -o '"Subject":"[^"]*"' <<<"$msgs")"
grep -q HostDiskFull <<<"$msgs" && fail "an urgency none alert was mailed"
# Tickets go to the mailer after their own group_wait (1m).
for _ in $(seq 1 90); do
	status="$(api "http://$NET-mailer:19130/v1/status")"
	grep -q '"open_tickets":1' <<<"$status" && break
	sleep 1
done
grep -q '"open_tickets":1' <<<"$status" || fail "the ticket is not waiting for the digest: $status"
subjects="$(mails | grep -o '"Subject":"[^"]*"')"
[[ "$(wc -l <<<"$subjects")" -eq 1 ]] || fail "mails besides the page (a ticket or an urgency none alert): $subjects"
echo "[ok] one page mailed; urgency none mailed to no one; the ticket waits for the digest"

# Two more SLOs start burning: the cap (2) lets one through and holds one.
post "[$(alert ErrorBudgetBurn wotan page),$(alert ErrorBudgetBurn well page)]"
sleep 3
post "[$(alert ErrorBudgetBurn wotan page),$(alert ErrorBudgetBurn well page),$(alert ErrorBudgetBurn dashboard page)]"
for _ in $(seq 1 30); do
	status="$(api "http://$NET-mailer:19130/v1/status")"
	grep -q '"held_today":[1-9]' <<<"$status" && break
	sleep 1
done
grep -q '"held_today":[1-9]' <<<"$status" || fail "nothing held back by the cap: $status"
n="$(mails | grep -o '"Subject":"[^"]*"' | wc -l)"
[[ "$n" -eq 2 ]] || fail "$n mails with max_per_day 2 and max_per_hour 2"
echo "[ok] the cap holds: 2 mails, the rest held back and counted"

# Prometheus -> Alertmanager with the repo's prometheus.yml.
cp "$REPO_ROOT/monitoring/prometheus/prometheus.yml" "$T/prom/"
cp "$REPO_ROOT/monitoring/prometheus/rules/alert-pipeline-heartbeat.yml" "$T/prom/rules/"
cat >"$T/prom/rules/uh-path-test.yml" <<'RULES'
groups:
  - name: uh-path-test
    interval: 5s
    rules:
      - alert: UhPromToAlertmanager
        expr: vector(1)
        labels:
          urgency: none
RULES
chmod -R a+rX "$T/prom"
docker run -d --name "$NET-prom" --network "$NET" -v "$T/prom:/etc/prometheus:ro" "$PROM_IMAGE" \
	--config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/tmp >/dev/null
got=""
for _ in $(seq 1 60); do
	got="$(api "http://$NET-am:9093/api/v2/alerts" 2>/dev/null || true)"
	grep -q UhPromToAlertmanager <<<"$got" && break
	sleep 1
done
grep -q UhPromToAlertmanager <<<"$got" || fail "a firing rule never reached Alertmanager: monitoring/prometheus/prometheus.yml must send alerts to alertmanager:9093"
echo "[ok] a firing rule in monitoring/prometheus reaches Alertmanager"

# 7. The route reaches the mailer (ADR-116 R4, policy.yaml pipeline:): the
# repo's AlertPipelineHeartbeat, through the generated pipeline route, sets
# the mailer's receipt gauge. The silent and back mails are cmd/alert-mailer's
# unit tests and the live drill (Step 332).
seen=""
for _ in $(seq 1 150); do
	seen="$(api "http://$NET-mailer:19130/metrics" 2>/dev/null | awk '/^unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds / {print $2}')"
	[[ -n "$seen" && "$seen" != "0" ]] && break
	sleep 1
done
[[ -n "$seen" && "$seen" != "0" ]] || fail "no pipeline receipt reached the mailer within 150s (gauge: ${seen:-absent}): check the pipeline route in alertmanager.yml"
echo "[ok] AlertPipelineHeartbeat reaches the mailer's pipeline receipt"

# 6. A run day raises the cap (ADR-147 decision 2, mailer.run_days): a second
# mailer, cap 2, with today listed at 3, takes three distinct pages straight
# on its webhook and mails all three.
today="$(TZ=America/Chicago date +%F)"
sed -e 's/^  max_per_day: [0-9]*$/  max_per_day: 2/' -e 's/^  max_per_hour: [0-9]*$/  max_per_hour: 2/' \
	-e 's/^    max_per_day: [0-9]*$/    max_per_day: 3/' -e 's/^    max_per_hour: [0-9]*$/    max_per_hour: 3/' \
	-e "s/^    dates: \[\]\$/    dates: [\"$today\"]/" \
	"$REPO_ROOT/observability/alerting/policy.yaml" >"$T/mailer/policy-runday.yaml"
{ grep -q '^  mode: send$' "$T/mailer/policy-runday.yaml" && grep -q '^  max_per_day: 2$' "$T/mailer/policy-runday.yaml" &&
	grep -q '^    max_per_day: 3$' "$T/mailer/policy-runday.yaml" && grep -q "^    dates: \[\"$today\"\]\$" "$T/mailer/policy-runday.yaml"; } ||
	fail "run_days anchors missing in policy.yaml (want mode send, run_days max_per_day and dates: [])"
chmod a+r "$T/mailer/policy-runday.yaml"
docker run -d --name "$NET-mailer2" --network "$NET" --read-only --tmpfs /var/lib/alert-mailer:uid=16710 \
	-e SMTP_SMARTHOST="$NET-sink:1025" -e SMTP_STARTTLS=false -e ALERT_MAIL_FROM=alerts@example.org -e ALERT_MAIL_TO=ops@example.org \
	-v "$T/mailer/policy-runday.yaml:/etc/alert-mailer/policy.yaml:ro" "$MAILER_IMAGE" >/dev/null
wait_for "http://$NET-mailer2:19130/health"
before="$(mails | grep -o '"Subject":"[^"]*"' | wc -l)"
for i in 1 2 3 4; do
	api -X POST -H 'Content-Type: application/json' "http://$NET-mailer2:19130/v1/alerts/page" \
		-d "{\"version\":\"4\",\"alerts\":[{\"status\":\"firing\",\"labels\":{\"alertname\":\"UhRunDay$i\",\"urgency\":\"none\"},\"annotations\":{\"summary\":\"run day $i\"},\"startsAt\":\"$now\"}]}" >/dev/null
done
sleep 3
after="$(mails | grep -o '"Subject":"[^"]*"' | wc -l)"
[[ $((after - before)) -eq 3 ]] || fail "run day: $((after - before)) mails from cap 2 raised to 3 on $today (want 3)"
grep -q '"max_per_day":3' <<<"$(api "http://$NET-mailer2:19130/v1/status")" || fail "run day: /v1/status does not show the raised cap"
echo "[ok] a run day raises the cap: 3 mails with max_per_day 2 and run_days.max_per_day 3"
echo "[PASS] alert path: 7 checks"
