#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-alertmanager-email.sh — FND-001's mail path, end to end, on throwaway
# containers: render-alertmanager.sh's output, loaded by the Alertmanager
# that monitoring/docker-compose.yml runs, delivers a critical alert to an
# SMTP sink (Mailpit); a warning with the same name and host is inhibited;
# the stock config and an authenticated render pass amtool; bad env files
# are refused; and monitoring/prometheus/prometheus.yml sends a firing rule
# to Alertmanager at all (it named none until 2026-10-20, so no alert ever
# left Prometheus). Needs docker. Nothing touches the monitoring stack.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
RENDER="$REPO_ROOT/scripts/monitoring/render-alertmanager.sh"
AM_IMAGE="$(awk '/image: *prom\/alertmanager/ {print $2; exit}' "$REPO_ROOT/monitoring/docker-compose.yml")"
PROM_IMAGE="$(awk '/image: *prom\/prometheus/ {print $2; exit}' "$REPO_ROOT/monitoring/docker-compose.yml")"
SINK_IMAGE="axllent/mailpit:v1.27.10"
[[ -n "$AM_IMAGE" ]] || { echo "[FAIL] no prom/alertmanager image in monitoring/docker-compose.yml" >&2; exit 1; }
[[ -n "$PROM_IMAGE" ]] || { echo "[FAIL] no prom/prometheus image in monitoring/docker-compose.yml" >&2; exit 1; }
command -v docker >/dev/null || { echo "[FAIL] docker is required; this test does not pass by skipping" >&2; exit 2; }

T="$(mktemp -d)"
NET="uh-am-test-$$"
cleanup() {
	docker rm -f "$NET-am" "$NET-sink" "$NET-am2" "$NET-prom" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	rm -rf "$T"
}
trap cleanup EXIT
fail() { echo "[FAIL] $*" >&2; exit 1; }
amtool() { docker run --rm -v "$1:/c:ro" --entrypoint amtool "$AM_IMAGE" check-config "/c/$2" >/dev/null; }

# Refusals: missing key, bad address, bad relay, an env file that tries to run code.
printf 'ALERT_EMAIL_FROM=a@example.org\nSMTP_SMARTHOST=h:25\n' >"$T/bad1.env"
printf 'ALERT_EMAIL_TO=not-an-address\nALERT_EMAIL_FROM=a@example.org\nSMTP_SMARTHOST=h:25\n' >"$T/bad2.env"
printf "ALERT_EMAIL_TO=a@example.org\nALERT_EMAIL_FROM=a@example.org\nSMTP_SMARTHOST=h:25'\n" >"$T/bad3.env"
# shellcheck disable=SC2016 # the $(...) is the payload: it must stay literal
printf 'ALERT_EMAIL_TO=a@example.org\n$(touch %s/pwned)\n' "$T" >"$T/bad4.env"
for b in bad1 bad2 bad3 bad4; do
	"$RENDER" "$T/$b.env" "$T/out-$b" >/dev/null 2>&1 && fail "$b.env was rendered"
done
[[ ! -e "$T/pwned" ]] || fail "the env file was executed"
echo "[ok] bad env files refused, none executed"

amtool "$REPO_ROOT/monitoring/alertmanager" alertmanager.yml || fail "stock config does not load"
printf 'ALERT_EMAIL_TO=ops@example.org,stevie@example.org\nALERT_EMAIL_FROM=alerts@example.org\nSMTP_SMARTHOST=smtp.example.org:587\nSMTP_AUTH_USERNAME=alerts@example.org\n' >"$T/auth.env"
"$RENDER" "$T/auth.env" "$T/auth" >/dev/null
grep -q 'smtp_auth_password_file: /etc/alertmanager-secrets/smtp-password' "$T/auth/alertmanager.yml" || fail "auth render has no password file"
amtool "$T/auth" alertmanager.yml || fail "authenticated render does not load"
echo "[ok] stock config and an authenticated render load in $AM_IMAGE"

# Delivery. No auth: Go's SMTP client will not send credentials without TLS,
# and the sink has none; auth is covered by the amtool check above.
printf 'ALERT_EMAIL_TO=ops@example.org\nALERT_EMAIL_FROM=alerts@example.org\nSMTP_SMARTHOST=%s-sink:1025\nSMTP_REQUIRE_TLS=false\n' "$NET" >"$T/sink.env"
"$RENDER" "$T/sink.env" "$T/sink" >/dev/null
chmod 0755 "$T/sink" && chmod 0644 "$T/sink/alertmanager.yml"
docker network create "$NET" >/dev/null
docker run -d --name "$NET-sink" --network "$NET" "$SINK_IMAGE" >/dev/null
docker run -d --name "$NET-am" --network "$NET" -v "$T/sink:/etc/alertmanager:ro" "$AM_IMAGE" \
	--config.file=/etc/alertmanager/alertmanager.yml --storage.path=/tmp >/dev/null
api() { docker run --rm --network "$NET" curlimages/curl:8.11.1 -fsS "$@"; }
for _ in $(seq 1 30); do api "http://$NET-am:9093/-/ready" >/dev/null 2>&1 && break; sleep 1; done
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
api -X POST -H 'Content-Type: application/json' "http://$NET-am:9093/api/v2/alerts" -d "[
 {\"labels\":{\"alertname\":\"UhMailTest\",\"host\":\"west\",\"severity\":\"critical\"},\"annotations\":{\"summary\":\"mail path test\"},\"startsAt\":\"$now\"},
 {\"labels\":{\"alertname\":\"UhMailTest\",\"host\":\"west\",\"severity\":\"warning\"},\"startsAt\":\"$now\"}]" >/dev/null
msgs=""
for _ in $(seq 1 60); do
	msgs="$(api "http://$NET-sink:8025/api/v1/messages" 2>/dev/null || true)"
	grep -q UhMailTest <<<"$msgs" && break
	sleep 1
done
grep -q UhMailTest <<<"$msgs" || fail "no mail for the critical alert within 60s"
grep -q '"Address":"ops@example.org"' <<<"$msgs" || fail "mail not addressed to ops@example.org"
sleep 5
msgs="$(api "http://$NET-sink:8025/api/v1/messages")"
n="$(grep -o '"Subject":"[^"]*UhMailTest[^"]*"' <<<"$msgs" | wc -l)"
[[ "$n" -eq 1 ]] || fail "$n mails for UhMailTest; the warning should be inhibited by the critical"
grep -q 'severity = "critical"\|critical' <<<"$msgs" || fail "the mail is not the critical alert"
echo "[ok] critical alert mailed to ops@example.org; same-name warning inhibited"

# Prometheus -> Alertmanager, with the repo's prometheus.yml: an always-firing
# rule must show up in an Alertmanager reachable as "alertmanager" (the
# compose service name), or every rule fires into nothing.
mkdir -p "$T/prom/rules"
cp "$REPO_ROOT/monitoring/prometheus/prometheus.yml" "$T/prom/"
cat >"$T/prom/rules/uh-path-test.yml" <<'RULES'
groups:
  - name: uh-path-test
    interval: 5s
    rules:
      - alert: UhPromToAlertmanager
        expr: vector(1)
        labels:
          severity: warning
RULES
chmod -R a+rX "$T/prom"
docker run -d --name "$NET-am2" --network "$NET" --network-alias alertmanager \
	-v "$REPO_ROOT/monitoring/alertmanager:/etc/alertmanager:ro" "$AM_IMAGE" \
	--config.file=/etc/alertmanager/alertmanager.yml --storage.path=/tmp >/dev/null
docker run -d --name "$NET-prom" --network "$NET" -v "$T/prom:/etc/prometheus:ro" "$PROM_IMAGE" \
	--config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/tmp >/dev/null
got=""
for _ in $(seq 1 60); do
	got="$(api "http://$NET-am2:9093/api/v2/alerts" 2>/dev/null || true)"
	grep -q UhPromToAlertmanager <<<"$got" && break
	sleep 1
done
grep -q UhPromToAlertmanager <<<"$got" || fail "a firing rule never reached Alertmanager: monitoring/prometheus/prometheus.yml must send alerts to alertmanager:9093"
echo "[ok] a firing rule in monitoring/prometheus reaches Alertmanager"
echo "[PASS] alertmanager email path: 4 checks"
