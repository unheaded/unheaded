#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# pihole-history.sh — carry the Pi-hole container's old logs into
# unheaded_logs.logs in the shape the syslog relay writes, so the old
# lxd-file sources (config/logging/sources.yaml, pihole/*-log) can be retired
# without losing a line.
#
#   export     logs and FTL's query table out of the container  -> DIR/in
#   transform  DIR/in -> DIR/out, `clickhouse local`, no network
#   load       DIR/out -> ClickHouse. Refuses until the relay's own pihole/dns
#              rows are there and match ours, so the two meet without a gap.
#   verify     every staged row is in the table, or was left to the relay's copy
#   clean      delete DIR (needs a passed verify)
#
# Order on the host: stack up, relay and deploy/pihole/60-unheaded-forward.conf
# live, then export, transform, load, verify, clean.
#
# DIR holds PERSONAL DATA (every LAN client and every name it looked up). It
# is created 0700, never under the repo, and `clean` removes it. The session
# table (web logins) is not exported.
set -euo pipefail
umask 077

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SQL="$REPO_ROOT/scripts/pihole-history"
DIR=/var/lib/unheaded/pihole-history
PIHOLE=pihole
CLICKHOUSE=unheaded-clickhouse
IMAGE=""
ALLOW_NO_OVERLAP=0
UNITS=(dns ftl web)

usage() {
	cat <<'USAGE'
Usage: pihole-history.sh [OPTION]... export|transform|load|verify|clean
Carry the Pi-hole container's old logs into unheaded_logs.logs.

      --dir DIR            staging directory (default /var/lib/unheaded/pihole-history)
      --pihole NAME        LXD container to export from (default pihole)
      --clickhouse NAME    running ClickHouse container (default unheaded-clickhouse)
      --image IMAGE        image for the transform (default: clickhouse in docker-compose.yml)
      --allow-no-overlap   load although no relay row matches a staged line
  -h, --help               show this help
USAGE
}
die() { echo "pihole-history: $*" >&2; exit 1; }
bad() { echo "pihole-history: $*" >&2; echo "Try 'pihole-history.sh --help' for more information." >&2; exit 2; }

stage=""
while [[ $# -gt 0 ]]; do
	case "$1" in
		-h|--help) usage; exit 0 ;;
		--dir) [[ $# -ge 2 ]] || bad "--dir needs a value"; DIR=$2; shift 2 ;;
		--pihole) [[ $# -ge 2 ]] || bad "--pihole needs a value"; PIHOLE=$2; shift 2 ;;
		--clickhouse) [[ $# -ge 2 ]] || bad "--clickhouse needs a value"; CLICKHOUSE=$2; shift 2 ;;
		--image) [[ $# -ge 2 ]] || bad "--image needs a value"; IMAGE=$2; shift 2 ;;
		--allow-no-overlap) ALLOW_NO_OVERLAP=1; shift ;;
		-*) bad "unknown option '$1'" ;;
		*) [[ -z "$stage" ]] || bad "one stage at a time"; stage=$1; shift ;;
	esac
done
[[ -n "$stage" ]] || bad "missing stage"
[[ "$DIR" == /* ]] || bad "--dir must be an absolute path"
case "$(realpath -m "$DIR")/" in "$REPO_ROOT"/*) die "$DIR is inside the repository; collected data stays out of it" ;; esac

# The exported directory's own description of itself.
load_env() {
	[[ -f "$DIR/export.env" ]] || die "$DIR/export.env not found; run export first"
	EXPORT_ID=$(sed -n 's/^EXPORT_ID=//p' "$DIR/export.env")
	REF=$(sed -n 's/^REF=//p' "$DIR/export.env")
	ZONE=$(sed -n 's/^ZONE=//p' "$DIR/export.env")
	[[ "$EXPORT_ID" =~ ^[A-Za-z0-9._-]+$ ]] || die "export.env: bad EXPORT_ID"
	[[ "$REF" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}\ [0-9]{2}:[0-9]{2}:[0-9]{2}$ ]] || die "export.env: bad REF"
	[[ "$ZONE" =~ ^[A-Za-z0-9/_+-]+$ ]] || die "export.env: bad ZONE"
}
# clickhouse-client inside the running server's container. It takes its user
# and password from that container's own environment; none passes through here.
client() { docker exec -i "$CLICKHOUSE" clickhouse-client --database unheaded_logs "$@"; }
# One line of SQL text from a file, comments dropped (the client takes --query).
sql() { grep -v '^--' "$SQL/$1"; }
staged_structure="timestamp DateTime64(3, 'UTC'), host String, source String, unit String, level String, retention String, message String, entry String, fields String, msg_ordinal UInt32, msg_total UInt32, from_db UInt8"
# check UNIT -> "staged live in_table left_to_live not_rebuilt missing"
check() {
	client --param_unit "pihole/$1" --query "$(sql check.sql)" \
		--external --file=- --name=staged --format=Native --structure="$staged_structure" \
		<"$DIR/out/pihole-$1.native" | sed -n 2p
}

do_export() {
	command -v lxc >/dev/null || die "lxc not found"
	[[ ! -e "$DIR/export.env" ]] || die "$DIR already holds an export; clean it or pass another --dir"
	mkdir -p "$DIR/in/logs"
	local ref zone
	ref=$(date -u '+%Y-%m-%d %H:%M:%S')
	zone=$(lxc exec "$PIHOLE" -- readlink -f /etc/localtime)
	zone=${zone#/usr/share/zoneinfo/}
	[[ "$zone" =~ ^[A-Za-z0-9/_+-]+$ && "$zone" != /* ]] || die "cannot read the container's time zone (got '$zone')"
	# The live files grow while tar reads them: tar then exits 1, which is
	# expected here. Anything else, or a failed unpack, stops the export.
	local st
	set +e
	lxc exec "$PIHOLE" -- sh -c "cd /var/log/pihole && find . -maxdepth 1 -type f \\( -name 'pihole.log*' -o -name 'FTL.log*' -o -name 'webserver.log*' \\) -print0 | tar --null -T - --warning=no-file-changed -cf -" \
		| tar -xf - -C "$DIR/in/logs" --no-same-owner --no-same-permissions
	st=("${PIPESTATUS[@]}")
	set -e
	[[ "${st[0]}" -le 1 && "${st[1]}" -eq 0 ]] || die "copying the logs out of '$PIHOLE' failed (tar ${st[0]}, unpack ${st[1]})"
	[[ -f "$DIR/in/logs/pihole.log" ]] || die "no pihole.log came out of '$PIHOLE'"
	lxc exec "$PIHOLE" -- pihole-FTL sqlite3 -readonly -cmd '.timeout 10000' /etc/pihole/pihole-FTL.db \
		<"$SQL/export-queries.sql" >"$DIR/in/ftl-queries.jsonl" || die "exporting FTL's query table failed"
	(cd "$DIR/in" && find . -type f -print0 | sort -z | xargs -0 sha256sum) >"$DIR/MANIFEST-in.sha256"
	printf 'EXPORT_ID=pihole-%s\nREF=%s\nZONE=%s\n' "$(date -u -d "$ref" +%Y%m%dT%H%M%SZ)" "$ref" "$zone" >"$DIR/export.env"
	echo "exported to $DIR/in: $(find "$DIR/in/logs" -type f | wc -l) log files, $(wc -l <"$DIR/in/ftl-queries.jsonl") stored queries (zone $zone)"
}

do_transform() {
	load_env
	command -v docker >/dev/null || die "docker not found"
	[[ -n "$IMAGE" ]] || IMAGE=$(awk '/image: *clickhouse\/clickhouse-server:/ {print $2; exit}' "$REPO_ROOT/docker-compose.yml")
	[[ -n "$IMAGE" ]] || die "no clickhouse image in docker-compose.yml; pass --image"
	[[ -f "$DIR/in/ftl-queries.jsonl" ]] || die "$DIR/in/ftl-queries.jsonl not found"
	[[ ! -e "$DIR/out" ]] || die "$DIR/out exists; a transform already ran (clean, or use another --dir)"
	mkdir "$DIR/out"
	rm -f "$DIR/verified"
	docker run --rm --pull=never --network none --user "$(id -u):$(id -g)" \
		--cap-drop ALL --security-opt no-new-privileges \
		-v "$DIR/in:/in:ro" -v "$DIR/out:/out" -v "$SQL:/sql:ro" "$IMAGE" \
		clickhouse local --param_ref "$REF" --param_tz "$ZONE" --param_export_id "$EXPORT_ID" \
		--queries-file /sql/transform.sql \
		|| { [[ -f "$DIR/out/summary.tsv" ]] && column -t -s $'\t' "$DIR/out/summary.tsv" >&2; die "transform failed"; }
	(cd "$DIR/out" && sha256sum pihole-*.native summary.tsv) >"$DIR/MANIFEST-out.sha256"
	column -t -s $'\t' "$DIR/out/summary.tsv"
}

need_out() {
	load_env
	local u
	for u in "${UNITS[@]}"; do [[ -f "$DIR/out/pihole-$u.native" ]] || die "$DIR/out/pihole-$u.native not found; run transform first"; done
	(cd "$DIR/out" && sha256sum --quiet -c "$DIR/MANIFEST-out.sha256") || die "$DIR/out changed since the transform"
	client --query 'SELECT 1' >/dev/null || die "ClickHouse container '$CLICKHOUSE' does not answer"
}

do_load() {
	need_out
	local staged live in_table left not_rebuilt missing u
	read -r staged live in_table left not_rebuilt missing < <(check dns)
	[[ -n "${missing:-}" ]] || die "the check query returned nothing"
	[[ "$live" -gt 0 ]] || die "no relay rows for pihole/dns yet. Start the relay and the forwarder first: loading now would leave a gap between the old lines and the new."
	if [[ "$left" -eq 0 && "$in_table" -lt "$staged" && "$ALLOW_NO_OVERLAP" -eq 0 ]]; then
		die "the relay holds $live pihole/dns rows and none equals a staged line. Either the export predates the forwarder (export again), or the relay's message is not the file's line and every overlapping line would load twice. --allow-no-overlap loads anyway."
	fi
	rm -f "$DIR/verified"
	for u in "${UNITS[@]}"; do
		[[ -s "$DIR/out/pihole-$u.native" ]] || continue
		client --param_unit "pihole/$u" --query "$(sql load-lines.sql)" <"$DIR/out/pihole-$u.native" || die "loading pihole/$u failed"
	done
	client --query "$(sql load-rebuilt.sql)" <"$DIR/out/pihole-dns.native" || die "loading the rebuilt rows failed"
	do_verify
}

do_verify() {
	need_out
	local staged live in_table left not_rebuilt missing u rc=0
	printf '%-11s %9s %9s %9s %13s %12s %8s\n' unit staged live in_table left_to_live not_rebuilt missing
	for u in "${UNITS[@]}"; do
		if [[ ! -s "$DIR/out/pihole-$u.native" ]]; then
			printf '%-11s %9s\n' "pihole/$u" 0
			continue
		fi
		read -r staged live in_table left not_rebuilt missing < <(check "$u")
		[[ -n "${missing:-}" ]] || die "the check query returned nothing for pihole/$u"
		printf '%-11s %9s %9s %9s %13s %12s %8s\n' "pihole/$u" "$staged" "$live" "$in_table" "$left" "$not_rebuilt" "$missing"
		[[ "$missing" -eq 0 ]] || rc=1
	done
	[[ $rc -eq 0 ]] || die "verify FAILED: staged rows are missing from unheaded_logs.logs. Do not clean; run load again."
	date -u '+%Y-%m-%dT%H:%M:%SZ' >"$DIR/verified"
	echo "verify passed: every staged row is in unheaded_logs.logs or was left to the relay's copy"
}

do_clean() {
	[[ -f "$DIR/export.env" ]] || die "$DIR is not an export directory (no export.env); nothing deleted"
	[[ -f "$DIR/verified" ]] || die "$DIR has no passed verify; nothing deleted"
	rm -rf -- "$DIR/in" "$DIR/out"
	rm -f -- "$DIR/MANIFEST-in.sha256" "$DIR/MANIFEST-out.sha256" "$DIR/export.env" "$DIR/verified"
	rmdir -- "$DIR" 2>/dev/null || echo "left $DIR in place: it holds files this tool did not write" >&2
	echo "staging removed"
}

case "$stage" in
	export) do_export ;;
	transform) do_transform ;;
	load) do_load ;;
	verify) do_verify ;;
	clean) do_clean ;;
	*) bad "unknown stage '$stage'" ;;
esac
