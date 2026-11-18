#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
#
# test-store-backups.sh: the ClickHouse and VictoriaMetrics backups and
# drills end to end (ADR-147 decision 6, S-SEE-AND-SURVIVE Phase 9), never
# touching a live store: throwaway source containers of the images
# docker-compose.yml deploys, sets in a temporary directory, as the normal
# user (CH_UID and VM_UID are this user's uid; no chown).
#
#   1. ClickHouse source: no network, the log schema, 1,000 rows an hour old
#   2. clickhouse-backup.sh: one set, LATEST, side files, ROWS=1000
#   3. clickhouse-drill.sh: passes
#   4. a flipped byte in a copy of the set: the drill says "set damaged"
#   5. ROWS=999 in a copy's meta: the drill names 999 and 1000
#   6. three more backups with CH_BACKUP_KEEP=2: two sets left
#   7. VictoriaMetrics source: 50 series two minutes old; backup, then the
#      drill passes; a flipped byte is refused
#   8. CH_BACKUP_OTHER_DISK=1 with the sets on the data's disk: "refusing"
#   9. no ch-drill-* or vm-drill-* container is left
#
# Needs docker, jq, curl, python3. The temporary directory is made under
# STORE_TEST_TMP (default /var/tmp), which must be on a real disk for case 8
# (a tmpfs has no disk to compare).

# pass and fail always return 0, so "A && pass || fail" is an if-then-else here.
# shellcheck disable=SC2015
set -euo pipefail

R="$(cd "$(dirname "$0")/../.." && pwd)"
B="$R/scripts/backup"
for t in docker jq curl python3; do command -v "$t" >/dev/null || { echo "[FAIL] $t is required" >&2; exit 2; }; done
DOCKER=(docker); docker info >/dev/null 2>&1 || DOCKER=(sudo -n docker)
CH_IMAGE="$(awk '/image: *clickhouse\/clickhouse-server/ {print $2; exit}' "$R/docker-compose.yml")"
VM_IMAGE="$(awk '/image: *victoriametrics\/victoria-metrics/ {print $2; exit}' "$R/docker-compose.yml")"
[[ -n "$CH_IMAGE" && -n "$VM_IMAGE" ]] || { echo "[FAIL] no clickhouse or victoria-metrics image in docker-compose.yml" >&2; exit 1; }

U="$(id -u)"
T="$(mktemp -d -p "${STORE_TEST_TMP:-/var/tmp}" store-backups-test.XXXXXX)"
CH="store-test-ch-$$" VM="store-test-vm-$$"
cleanup() {
	"${DOCKER[@]}" rm -f "$CH" "$VM" >/dev/null 2>&1 || true
	rm -rf -- "$T"
}
trap cleanup EXIT
mkdir -p "$T/chdata" "$T/chsets" "$T/vmdata" "$T/vmauth" "$T/vmsets" "$T/prom"
fails=0
pass() { echo "[PASS] $*"; }
fail() { echo "[FAIL] $*"; fails=$((fails + 1)); }
freeport() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])'; }

export CH_CONTAINER="$CH" CH_BACKUP_DIR="$T/chsets" CH_UID="$U" CH_BACKUPS_XML="$R/config/clickhouse/backups.xml" \
	CH_DRILL_SCRATCH="$T/chscratch" CH_BACKUP_OTHER_DISK=0 STORE_METRICS_DIR="$T/prom"
export VM_CONTAINER="$VM" VM_BACKUP_DIR="$T/vmsets" VM_UID="$U" VM_DRILL_SCRATCH="$T/vmscratch" VM_BACKUP_OTHER_DISK=0

# 1. ClickHouse source.
"${DOCKER[@]}" run -d --name "$CH" --network none --user "$U:$U" --read-only \
	--tmpfs /tmp:size=64m --tmpfs "/var/log/clickhouse-server:uid=$U,gid=$U,size=16m" \
	--tmpfs "/etc/clickhouse-server/users.d:uid=$U,gid=$U,size=1m" \
	-v "$T/chdata:/var/lib/clickhouse" -v "$T/chsets:/backups" \
	-v "$R/config/clickhouse/backups.xml:/etc/clickhouse-server/config.d/backups.xml:ro" "$CH_IMAGE" >/dev/null
chq() { "${DOCKER[@]}" exec -i "$CH" clickhouse-client --multiquery "$@"; }
up=0
for _ in $(seq 1 90); do chq --query 'SELECT 1' >/dev/null 2>&1 && { up=1; break; }; sleep 1; done
((up)) || { "${DOCKER[@]}" logs --tail 20 "$CH"; echo "[FAIL] ClickHouse source did not start"; exit 1; }
chq --query 'CREATE DATABASE IF NOT EXISTS unheaded_logs'
chq <"$R/config/clickhouse/schema/001-logs.sql"
chq --query "INSERT INTO unheaded_logs.logs (timestamp, host, source, unit, level, retention, message, entry, fields)
	SELECT now64(3) - INTERVAL 1 HOUR + toIntervalMillisecond(number), 'west', 'journald', 'test.service', 'info', 'operational',
	       concat('line ', toString(number)), concat('e', toString(number)), '{}' FROM numbers(1000)"
[[ "$(chq --query 'SELECT count() FROM unheaded_logs.logs')" == 1000 ]] && pass "1 source holds 1000 rows" || fail "1 source rows"

# 2. One backup.
if "$B/clickhouse-backup.sh" >"$T/ch1.log" 2>&1; then
	name="$(cat "$T/chsets/LATEST")"
	[[ "$name" =~ ^ch-[0-9]{8}T[0-9]{6}Z$ && -d "$T/chsets/$name" && -s "$T/chsets/$name.SHA256SUMS" ]] &&
		grep -qx 'ROWS=1000' "$T/chsets/$name.meta.env" && grep -q 'unheaded_store_backup_last_success_timestamp_seconds{store="clickhouse"}' "$T/prom/store_backup_clickhouse.prom" &&
		pass "2 one set, LATEST, side files, ROWS=1000, .prom" || fail "2 set incomplete: $(cat "$T/chsets/$name.meta.env" 2>/dev/null)"
else
	fail "2 clickhouse-backup.sh: $(tail -3 "$T/ch1.log")"
	name=""
fi

# 3. The drill passes.
if [[ -n "$name" ]] && "$B/clickhouse-drill.sh" >"$T/chd.log" 2>&1; then pass "3 ClickHouse drill"; else fail "3 ClickHouse drill: $(tail -3 "$T/chd.log")"; fi

# 4 and 5. Broken copies are refused.
if [[ -n "$name" ]]; then
	bad=ch-20000101T000000Z
	cp -a "$T/chsets/$name" "$T/chsets/$bad"
	sed "s/$name/$bad/" "$T/chsets/$name.meta.env" >"$T/chsets/$bad.meta.env"
	cp "$T/chsets/$name.SHA256SUMS" "$T/chsets/$bad.SHA256SUMS"
	f="$(find "$T/chsets/$bad" -type f -size +0 | head -1)"
	printf '\x00' | dd of="$f" bs=1 seek=0 count=1 conv=notrunc status=none
	out="$("$B/clickhouse-drill.sh" "$bad" 2>&1)" && fail "4 a damaged set passed" ||
		{ grep -q 'set damaged' <<<"$out" && pass "4 a flipped byte: set damaged" || fail "4 wrong refusal: $out"; }
	off=ch-20000102T000000Z
	cp -a "$T/chsets/$name" "$T/chsets/$off"
	sed -e "s/$name/$off/" -e 's/^ROWS=1000$/ROWS=999/' "$T/chsets/$name.meta.env" >"$T/chsets/$off.meta.env"
	cp "$T/chsets/$name.SHA256SUMS" "$T/chsets/$off.SHA256SUMS"
	out="$("$B/clickhouse-drill.sh" "$off" 2>&1)" && fail "5 a wrong count passed" ||
		{ grep -q '1000' <<<"$out" && grep -q '999' <<<"$out" && pass "5 ROWS=999: refused, both numbers named" || fail "5 wrong refusal: $out"; }
fi

# 6. Retention.
for _ in 1 2 3; do sleep 1; CH_BACKUP_KEEP=2 "$B/clickhouse-backup.sh" >/dev/null 2>&1 || fail "6 a backup failed"; done
n="$(find "$T/chsets" -mindepth 1 -maxdepth 1 -type d -name 'ch-*' | wc -l)"
[[ "$n" -eq 2 ]] && pass "6 CH_BACKUP_KEEP=2 keeps two sets" || fail "6 $n sets after retention"

# 7. VictoriaMetrics.
od -An -tx1 -N16 /dev/urandom | tr -d ' \n' >"$T/vmauth/admin.key"
P1="$(freeport)" P2="$(freeport)"
export VM_URL="http://127.0.0.1:$P1" VM_DRILL_PORT="$P2"
"${DOCKER[@]}" run -d --name "$VM" --user "$U:$U" --read-only --tmpfs /tmp:size=64m -p "127.0.0.1:$P1:8428" \
	-v "$T/vmdata:/victoria-data" -v "$T/vmauth:/victoria-auth:ro" "$VM_IMAGE" \
	-storageDataPath=/victoria-data -retentionPeriod=100y -httpListenAddr=:8428 \
	-snapshotAuthKey=file:///victoria-auth/admin.key >/dev/null
up=0
for _ in $(seq 1 60); do curl -fsS -m 2 "$VM_URL/health" >/dev/null 2>&1 && { up=1; break; }; sleep 1; done
((up)) || { "${DOCKER[@]}" logs --tail 20 "$VM"; echo "[FAIL] VictoriaMetrics source did not start"; exit 1; }
ts=$((($(date +%s) - 150) * 1000))
for i in $(seq 1 50); do echo "uh_store_test{i=\"$i\"} 1 $ts"; done | curl -fsS -m 10 --data-binary @- "$VM_URL/api/v1/import/prometheus"
curl -fsS -m 10 "$VM_URL/internal/force_flush" >/dev/null
if "$B/victoria-backup.sh" >"$T/vm1.log" 2>&1; then
	vname="$(cat "$T/vmsets/LATEST")"
	grep -qx 'SERIES=50' "$T/vmsets/$vname.meta.env" && pass "7 VictoriaMetrics set, SERIES=50" || fail "7 meta: $(cat "$T/vmsets/$vname.meta.env")"
	if "$B/victoria-drill.sh" >"$T/vmd.log" 2>&1; then pass "7 VictoriaMetrics drill"; else fail "7 VictoriaMetrics drill: $(tail -3 "$T/vmd.log")"; fi
	f="$(find "$T/vmsets/$vname" -type f -size +0 | head -1)"
	printf '\x00' | dd of="$f" bs=1 seek=0 count=1 conv=notrunc status=none
	out="$("$B/victoria-drill.sh" 2>&1)" && fail "7 a damaged set passed" ||
		{ grep -q 'set damaged' <<<"$out" && pass "7 a flipped byte: set damaged" || fail "7 wrong refusal: $out"; }
else
	fail "7 victoria-backup.sh: $(tail -3 "$T/vm1.log")"
fi

# 8. The other-disk guard, sets and data both under $T.
out="$(CH_BACKUP_OTHER_DISK=1 "$B/clickhouse-backup.sh" 2>&1)" && fail "8 a set on the data's disk was written" ||
	{ grep -q 'refusing' <<<"$out" && pass "8 refusing: same disk" || fail "8 wrong refusal: $out"; }

# 9. Nothing left behind.
left="$("${DOCKER[@]}" ps -a -q --filter name=ch-drill- --filter name=vm-drill-)"
[[ -z "$left" ]] && pass "9 no drill container left" || fail "9 drill containers left: $left"

echo "test-store-backups: 9 cases, $fails failure(s)"
((fails == 0))
