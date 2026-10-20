#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# render-alertmanager.sh — write Alertmanager's email configuration from an
# env file (FND-001: alerts reach nobody).
#
#   scripts/monitoring/render-alertmanager.sh /etc/unheaded/alertmanager.env
#
# The env file names the relay and the people; none of it is a secret:
#
#   ALERT_EMAIL_TO=stevie@example.org          # one or more, comma-separated
#   ALERT_EMAIL_FROM=alerts@example.org
#   SMTP_SMARTHOST=smtp.example.org:587
#   SMTP_AUTH_USERNAME=alerts@example.org      # optional (relay without auth)
#   SMTP_REQUIRE_TLS=true                      # optional, default true
#
# The password never passes through here or the repo: Alertmanager reads it
# from /etc/alertmanager-secrets/smtp-password, the container path of
# ALERTMANAGER_SECRETS_DIR (monitoring/docker-compose.yml), e.g.
# /etc/unheaded/alertmanager/smtp-password, 0400, owned by uid 16709 (the
# compose user for alertmanager, pkg/uids.Alertmanager).
#
# Output: monitoring/alertmanager/rendered/alertmanager.yml (gitignored).
# Then in monitoring/.env:
#   ALERTMANAGER_CONFIG_FILE=rendered/alertmanager.yml
#   ALERTMANAGER_SECRETS_DIR=/etc/unheaded/alertmanager
# Without them compose keeps monitoring/alertmanager/alertmanager.yml, whose
# only receiver is null.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
ENVFILE="${1:?usage: $0 ENVFILE [OUTDIR]}"
OUTDIR="${2:-${REPO_ROOT}/monitoring/alertmanager/rendered}"

# Read KEY=VALUE lines without executing the file.
declare -A cfg=()
while IFS= read -r line || [[ -n "$line" ]]; do
	[[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
	[[ "$line" =~ ^([A-Z_]+)=(.*)$ ]] || { echo "[FAIL] $ENVFILE: not KEY=VALUE: $line" >&2; exit 1; }
	cfg[${BASH_REMATCH[1]}]="${BASH_REMATCH[2]}"
done <"$ENVFILE"

need() { [[ -n "${cfg[$1]:-}" ]] || { echo "[FAIL] $ENVFILE: $1 is required" >&2; exit 1; }; }
need ALERT_EMAIL_TO
need ALERT_EMAIL_FROM
need SMTP_SMARTHOST
tls="${cfg[SMTP_REQUIRE_TLS]:-true}"

email_re='^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$'
IFS=',' read -r -a tos <<<"${cfg[ALERT_EMAIL_TO]}"
for t in "${tos[@]}" "${cfg[ALERT_EMAIL_FROM]}"; do
	[[ "$t" =~ $email_re ]] || { echo "[FAIL] not an email address: '$t'" >&2; exit 1; }
done
[[ "${cfg[SMTP_SMARTHOST]}" =~ ^[A-Za-z0-9.-]+:[0-9]{1,5}$ ]] || { echo "[FAIL] SMTP_SMARTHOST must be host:port" >&2; exit 1; }
[[ -z "${cfg[SMTP_AUTH_USERNAME]:-}" || "${cfg[SMTP_AUTH_USERNAME]}" =~ ^[A-Za-z0-9._%+@-]+$ ]] || { echo "[FAIL] SMTP_AUTH_USERNAME has characters a relay login does not" >&2; exit 1; }
[[ "$tls" == true || "$tls" == false ]] || { echo "[FAIL] SMTP_REQUIRE_TLS must be true or false" >&2; exit 1; }

mkdir -p "$OUTDIR"
tmp="$(mktemp "$OUTDIR/.alertmanager.yml.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
{
	echo "# Rendered by scripts/monitoring/render-alertmanager.sh from $ENVFILE. Do not edit."
	echo "global:"
	echo "  resolve_timeout: 5m"
	echo "  smtp_smarthost: '${cfg[SMTP_SMARTHOST]}'"
	echo "  smtp_from: '${cfg[ALERT_EMAIL_FROM]}'"
	if [[ -n "${cfg[SMTP_AUTH_USERNAME]:-}" ]]; then
		echo "  smtp_auth_username: '${cfg[SMTP_AUTH_USERNAME]}'"
		echo "  smtp_auth_password_file: /etc/alertmanager-secrets/smtp-password"
	fi
	echo "  smtp_require_tls: $tls"
	cat <<'YAML'

# Every alert is mailed; critical ones repeat sooner. A critical alert
# silences the warning of the same name on the same host.
route:
  receiver: email
  group_by: ['alertname', 'host', 'instance']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  routes:
    - matchers: ['severity="critical"']
      receiver: email
      repeat_interval: 1h

inhibit_rules:
  - source_matchers: ['severity="critical"']
    target_matchers: ['severity="warning"']
    equal: ['alertname', 'host']

receivers:
  - name: email
    email_configs:
YAML
	for t in "${tos[@]}"; do
		echo "      - to: '$t'"
		echo "        send_resolved: true"
	done
} >"$tmp"
chmod 0644 "$tmp"
mv -f "$tmp" "$OUTDIR/alertmanager.yml"
trap - EXIT
echo "[OK] $OUTDIR/alertmanager.yml (${#tos[@]} recipient(s), relay ${cfg[SMTP_SMARTHOST]})"
