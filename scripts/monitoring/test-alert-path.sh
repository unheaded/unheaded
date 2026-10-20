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
#   4. past mailer.max_per_day nothing is mailed; the cap is reported
#   5. a firing Prometheus rule reaches Alertmanager at all
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
	docker rm -f "$NET-am" "$NET-sink" "$NET-mailer" "$NET-prom" >/dev/null 2>&1 || true
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
sed -e 's/^\(      group_wait: \)2m$/\11s/' -e '0,/^      group_interval: 5m$/s//      group_interval: 2s/' \
	"$REPO_ROOT/monitoring/alertmanager/alertmanager.yml" >"$T/am/alertmanager.yml"
{ grep -q 'group_wait: 1s' "$T/am/alertmanager.yml" && grep -q 'group_interval: 2s' "$T/am/alertmanager.yml"; } ||
	fail "page timing anchors missing in alertmanager.yml"
sed -e 's/^  mode: dry-run$/  mode: send/' -e 's/^  max_per_day: [0-9]*$/  max_per_day: 2/' \
	"$REPO_ROOT/observability/alerting/policy.yaml" >"$T/mailer/policy.yaml"
{ grep -q '^  mode: send$' "$T/mailer/policy.yaml" && grep -q '^  max_per_day: 2$' "$T/mailer/policy.yaml"; } || fail "policy anchors missing"
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
[[ "$n" -eq 2 ]] || fail "$n mails with max_per_day 2"
echo "[ok] max_per_day holds: 2 mails, the rest held back and counted"

# Prometheus -> Alertmanager with the repo's prometheus.yml.
cp "$REPO_ROOT/monitoring/prometheus/prometheus.yml" "$T/prom/"
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
echo "[PASS] alert path: 5 checks"
