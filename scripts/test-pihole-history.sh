#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2024-2026 Stevie Bellis.
#
# test-pihole-history.sh — scripts/pihole-history.sh loses no line and loads
# none twice. Synthetic fixtures only (example.org, 192.0.2.0/24), built here;
# the ClickHouse image is the one docker-compose.yml deploys; every container
# runs with the network off.
#
# What is proved:
#   export SQL   runs against an FTL-shaped SQLite file and does not touch
#                the session table
#   transform    every line becomes one row, word for word; a dnsmasq stamp
#                gets the right year across a new year; a line with no stamp
#                takes its neighbour's; rows FTL alone remembers are rebuilt;
#                rows past 90 days are counted, not written; a file with no
#                time at all stops the run
#   load         refuses with no relay rows, and with relay rows that match
#                nothing; skips exactly the lines the relay already holds;
#                run twice, adds nothing; verify fails before, passes after
#   clean        refuses without a passed verify, then removes the staging
#
# The export stage's lxc calls are not run here (no Pi-hole in CI); its SQL is.
# Needs docker; without it this FAILS (exit 2), it never passes by skipping.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TOOL="$REPO_ROOT/scripts/pihole-history.sh"
if ! docker info >/dev/null 2>&1; then
	echo "[FAIL] docker is required; this test does not pass by skipping." >&2
	exit 2
fi
IMAGE="$(awk '/image: *clickhouse\/clickhouse-server:/ {print $2; exit}' "$REPO_ROOT/docker-compose.yml")"
[[ -n "$IMAGE" ]] || { echo "[FAIL] no clickhouse image in docker-compose.yml" >&2; exit 1; }
docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull -q "$IMAGE" >/dev/null || { echo "[FAIL] cannot get $IMAGE" >&2; exit 2; }

tmp="$(mktemp -d)"
server="unheaded-pihole-history-test-$$"
cleanup() {
	docker rm -f "$server" >/dev/null 2>&1
	rm -rf -- "$tmp"
}
trap cleanup EXIT

fails=0
ok() { echo "  ok    $1"; }
no() { echo "  FAIL  $1" >&2; fails=$((fails + 1)); }
# expect NAME WANT GOT
expect() { if [[ "$2" == "$3" ]]; then ok "$1"; else no "$1: want '$2', got '$3'"; fi; }
# refused NAME PATTERN CMD...: the command must fail and say PATTERN
refused() {
	local name=$1 pat=$2 out
	shift 2
	if out=$("$@" 2>&1); then no "$name: ran, should have refused"
	elif grep -q -- "$pat" <<<"$out"; then ok "$name"
	else no "$name: refused without saying '$pat': $out"; fi
}
# local SQL over a staging directory's /out, in clickhouse local
out_sql() {
	docker run -i --rm --pull=never --network none -v "$1/out:/out:ro" "$IMAGE" clickhouse local -q "$2"
}
db() { docker exec -i "$server" clickhouse-client --database unheaded_logs "$@"; }

# --- fixtures -------------------------------------------------------------
fx=$(python3 - "$tmp" "$REPO_ROOT/scripts/pihole-history/export-queries.sql" <<'PY'
import gzip, json, os, sqlite3, sys
root, export_sql = sys.argv[1], sys.argv[2]

def stage(name, ref, files, queries=None):
    d = os.path.join(root, name)
    logs = os.path.join(d, "in", "logs")
    os.makedirs(logs)
    for fname, lines in files.items():
        data = "".join(l + "\n" for l in lines).encode()
        p = os.path.join(logs, fname)
        with (gzip.open(p, "wb") if fname.endswith(".gz") else open(p, "wb")) as f:
            f.write(data)
    with open(os.path.join(d, "in", "ftl-queries.jsonl"), "w") as f:
        for q in queries or []:
            f.write(q + "\n")
    with open(os.path.join(d, "export.env"), "w") as f:
        f.write(f"EXPORT_ID=fixture-{name}\nREF={ref}\nZONE=Etc/UTC\n")

# An FTL-shaped database (v6 schema: ids in query_storage, text in *_by_id).
db = os.path.join(root, "fixture-ftl.db")
c = sqlite3.connect(db)
c.executescript("""
CREATE TABLE query_storage (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp INTEGER NOT NULL, type INTEGER NOT NULL,
 status INTEGER NOT NULL, domain INTEGER NOT NULL, client INTEGER NOT NULL, forward INTEGER, additional_info INTEGER,
 reply_type INTEGER, reply_time REAL, dnssec INTEGER, list_id INTEGER, ede INTEGER);
CREATE TABLE domain_by_id (id INTEGER PRIMARY KEY, domain TEXT NOT NULL);
CREATE TABLE client_by_id (id INTEGER PRIMARY KEY, ip TEXT NOT NULL, name TEXT);
CREATE TABLE forward_by_id (id INTEGER PRIMARY KEY, forward TEXT NOT NULL);
CREATE TABLE addinfo_by_id (id INTEGER PRIMARY KEY, type INTEGER NOT NULL, content NOT NULL);
CREATE TABLE session (id INTEGER PRIMARY KEY, sid TEXT, csrf TEXT);
INSERT INTO session VALUES (1, 'MUST-NOT-BE-EXPORTED', 'MUST-NOT-BE-EXPORTED');
INSERT INTO domain_by_id VALUES (1,'ancient.example.org'),(2,'early.example.org'),(3,'a.example.org'),(4,'svc.example.org');
INSERT INTO client_by_id VALUES (1,'192.0.2.10','laptop.example.org'),(2,'192.0.2.11',NULL);
INSERT INTO forward_by_id VALUES (1,'192.0.2.53#53');
INSERT INTO addinfo_by_id VALUES (1,2,'cname.example.org');
""")
rows = [  # time (UTC epoch, fractional as FTL stores it), type, status, domain, client, forward, addinfo
    (1751328000.25, 1, 2, 1, 1, 1, None),   # 2025-07-01: past 90 days at ref, counted and left out
    (1793491200.5, 1, 2, 2, 1, 1, None),    # 2026-11-01 00:00:00.5: before the oldest file line, rebuilt
    (1793491201.0, 16, 3, 2, 2, None, 1),   # HTTPS, client without a name, additional info
    (1793491202.0, 165, 2, 4, 2, 1, None),  # type=65
    (1793750401.0, 1, 2, 3, 1, 1, None),    # 2026-11-04 00:00:01: inside the files, not rebuilt
]
for r in rows:
    c.execute("INSERT INTO query_storage (timestamp,type,status,domain,client,forward,additional_info,"
              "reply_type,reply_time,dnssec,list_id,ede) VALUES (?,?,?,?,?,?,?,4,0.0123,0,NULL,-1)", r)
c.commit()
sql = "".join(l for l in open(export_sql) if not l.lstrip().startswith("--"))
queries = [j for (j,) in c.execute(sql)]

stage("main", "2026-11-09 12:00:00", {
    "pihole.log.2.gz": [
        "Nov  4 00:00:01 dnsmasq[411]: query[A] a.example.org from 192.0.2.10",
        "Nov  4 00:00:01 dnsmasq[411]: forwarded a.example.org to 192.0.2.53",
        "Nov  4 00:00:01 dnsmasq[411]: query[A] a.example.org from 192.0.2.10",
        "a line with no time of its own",
        "Nov  4 00:00:02 dnsmasq[411]: reply a.example.org is 192.0.2.80",
    ],
    "pihole.log.1": ["Nov  7 10:00:00 dnsmasq[411]: query[AAAA] b.example.org from 192.0.2.11"] * 3,
    "pihole.log": [
        "Nov  9 09:00:00 dnsmasq[411]: query[A] c.example.org from 192.0.2.12",
        "Nov  9 09:00:01 dnsmasq[411]: gravity blocked ads.example.org is 0.0.0.0",
        "Nov  9 09:00:02 dnsmasq[411]: query[TXT] quote\"and\\slash.example.org from 192.0.2.12",
    ],
    "FTL.log.1": ["2026-11-05 08:00:00.123 UTC [411M] INFO: ########## FTL started on pihole! ##########"],
    "FTL.log": [
        "continuation with nothing above it",
        "2026-11-08 08:00:00.500 UTC [411/T412] WARNING: Long-term load is 5.1",
        "   second line of the warning",
        "2026-11-09 08:00:00.000 UTC [411/T412] INFO: Optimized database in 0.012 seconds",
    ],
    "webserver.log": [
        "[2026-07-01 12:00:00.250 UTC 411] Initializing HTTP server on ports \"80o,443os\"",
        "[2026-11-09 07:00:00.001 UTC 411] Authentication required, redirecting to /admin/login",
    ],
}, queries)
# Only a dnsmasq log, written across a new year, exported on 2 January.
stage("newyear", "2027-01-02 00:00:00", {"pihole.log": [
    "Dec 31 23:59:59 dnsmasq[9]: query[A] wrap.example.org from 192.0.2.10",
    "Jan  1 00:00:00 dnsmasq[9]: query[A] wrap.example.org from 192.0.2.10",
]})
# A file in which nothing has a time.
stage("notime", "2026-11-09 12:00:00", {"pihole.log": ["Nov  9 09:00:00 dnsmasq[411]: started"], "FTL.log": ["no time anywhere"]})
print(len(queries), int(any("MUST-NOT-BE-EXPORTED" in q for q in queries)))
PY
) || { echo "[FAIL] building fixtures" >&2; exit 1; }

echo "export SQL"
expect "five stored queries come out, joined to their text" "5 0" "$fx"
expect "the client's name comes with the query" 3 "$(grep -c '"client_name":"laptop.example.org"' "$tmp/main/in/ftl-queries.jsonl")"

# --- transform --------------------------------------------------------------
echo "transform"
if "$TOOL" --dir "$tmp/main" transform >"$tmp/main.log" 2>&1; then ok "runs"; else no "transform failed: $(cat "$tmp/main.log")"; fi
summary() { awk -F'\t' -v w="$1" -v u="$2" -v o="$3" '$1==w && $2==u && $3==o {print $4}' "$tmp/main/out/summary.tsv"; }
expect "dns: 11 lines read" 11 "$(summary read pihole/dns file)"
expect "dns: 11 lines written" 11 "$(summary written pihole/dns file)"
expect "dns: 3 rows rebuilt from FTL's database" 3 "$(summary written pihole/dns ftl-db)"
expect "dns: 1 stored query past 90 days, counted" 1 "$(summary older-than-90-days pihole/dns ftl-db)"
expect "ftl: 5 lines written" 5 "$(summary written pihole/ftl file)"
expect "web: 2 lines written (audit keeps the July line)" 2 "$(summary written pihole/web file)"
expect "ftl: 2 lines took a neighbour's time" 2 "$(summary time-carried pihole/ftl file)"
expect "every row is the file's line, word for word, and every line is a row" same "$(
	if diff <(out_sql "$tmp/main" "SELECT message FROM file('/out/pihole-*.native', 'Native') WHERE from_db = 0 FORMAT TSVRaw" | LC_ALL=C sort) \
		<(cat "$tmp/main/in/logs/"*.log "$tmp/main/in/logs/"*.log.1 <(zcat "$tmp/main/in/logs/"*.gz) | LC_ALL=C sort) >/dev/null; then echo same; else echo differ; fi)"
expect "an unstamped first line takes the next stamp" "2026-11-08 08:00:00.500" "$(out_sql "$tmp/main" "
	SELECT timestamp FROM file('/out/pihole-ftl.native', 'Native') WHERE message = 'continuation with nothing above it'")"
expect "an unstamped later line takes the stamp above" "2026-11-04 00:00:01.000 carried" "$(out_sql "$tmp/main" "
	SELECT timestamp, JSONExtractString(fields, 'UNHEADED_IMPORT_TIME') FROM file('/out/pihole-dns.native', 'Native')
	WHERE message = 'a line with no time of its own' FORMAT TSV" | tr '\t' ' ')"
expect "three equal lines are three rows, numbered oldest first" "1/3,2/3,3/3 3" "$(out_sql "$tmp/main" "
	SELECT arrayStringConcat(groupArray(concat(toString(msg_ordinal), '/', toString(msg_total))), ','), uniqExact(entry)
	FROM (SELECT * FROM file('/out/pihole-dns.native', 'Native') WHERE message LIKE '%b.example.org%' ORDER BY msg_ordinal) FORMAT TSV" | tr '\t' ' ')"
expect "rebuilt rows read like dnsmasq, typed" "Nov  1 00:00:00 dnsmasq: query[A] early.example.org from 192.0.2.10|Nov  1 00:00:01 dnsmasq: query[HTTPS] early.example.org from 192.0.2.11|Nov  1 00:00:02 dnsmasq: query[type=65] svc.example.org from 192.0.2.11" "$(out_sql "$tmp/main" "
	SELECT arrayStringConcat(groupArray(message), '|') FROM (SELECT message FROM file('/out/pihole-dns.native', 'Native') WHERE from_db = 1 ORDER BY timestamp)")"
expect "a rebuilt row keeps what FTL stored" "2026-11-01 00:00:00.500 laptop.example.org 192.0.2.53#53 ftl-db" "$(out_sql "$tmp/main" "
	SELECT timestamp, JSONExtractString(fields, 'FTL_CLIENT_NAME'), JSONExtractString(fields, 'FTL_FORWARD'), JSONExtractString(fields, 'UNHEADED_IMPORT_FROM')
	FROM file('/out/pihole-dns.native', 'Native') WHERE JSONExtractString(fields, 'FTL_ID') = '2' FORMAT TSV" | tr '\t' ' ')"
expect "rows carry the relay's unit, host, retention and the import mark" "pihole/dns pihole operational import pihole-dns fixture-main|pihole/ftl pihole operational import pihole-ftl fixture-main|pihole/web pihole audit import pihole-web fixture-main" "$(out_sql "$tmp/main" "
	SELECT arrayStringConcat(groupArray(x), '|') FROM (SELECT DISTINCT concat(unit, ' ', host, ' ', retention, ' ', source, ' ',
	JSONExtractString(fields, 'SYSLOG_IDENTIFIER'), ' ', JSONExtractString(fields, 'UNHEADED_IMPORT_ID')) AS x
	FROM file('/out/pihole-*.native', 'Native') WHERE level = 'info' ORDER BY x)")"

if "$TOOL" --dir "$tmp/newyear" transform >"$tmp/newyear.log" 2>&1; then ok "a dnsmasq log alone transforms"; else no "newyear transform failed: $(cat "$tmp/newyear.log")"; fi
expect "31 December is last year, 1 January this year" "2026-12-31 23:59:59.000,2027-01-01 00:00:00.000" "$(out_sql "$tmp/newyear" "
	SELECT arrayStringConcat(groupArray(toString(timestamp)), ',') FROM (SELECT timestamp FROM file('/out/pihole-dns.native', 'Native') ORDER BY timestamp)")"
refused "a file with no time at all stops the run" "transform failed" "$TOOL" --dir "$tmp/notime" transform
expect "and the summary names the file" FTL.log "$(awk -F'\t' '$1=="no-time-in-file" {print $3}' "$tmp/notime/out/summary.tsv" 2>/dev/null)"
refused "a staging directory inside the repository is refused" "inside the repository" "$TOOL" --dir "$REPO_ROOT/scripts/pihole-history/x" transform

# --- load, against a real server ----------------------------------------------
echo "load"
docker run -d --pull=never --network none --name "$server" \
	-e CLICKHOUSE_DB=unheaded_logs -e CLICKHOUSE_USER=unheaded -e CLICKHOUSE_PASSWORD=placeholder \
	-e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 "$IMAGE" >/dev/null || { echo "[FAIL] cannot start $IMAGE" >&2; exit 1; }
up=0
for _ in $(seq 1 60); do
	if db --query 'SELECT 1' >/dev/null 2>&1; then up=1; break; fi
	sleep 1
done
[[ $up -eq 1 ]] || { echo "[FAIL] ClickHouse did not come up: $(docker logs "$server" 2>&1 | tail -5)" >&2; exit 1; }
db --multiquery <"$REPO_ROOT/config/clickhouse/schema/001-logs.sql" || { echo "[FAIL] applying 001-logs.sql" >&2; exit 1; }
load() { "$TOOL" --dir "$tmp/main" --clickhouse "$server" "$@"; }
count() { db --query "SELECT count() FROM logs FINAL WHERE host = 'pihole' AND $1"; }
# A row as the relay writes it: the file's line as the message, stamped when it was read.
live() {
	db --query "INSERT INTO logs (timestamp, host, source, unit, level, retention, message, entry, fields) FORMAT TSV" \
		<<<"$1"$'\t'"pihole"$'\t'"journald"$'\t'"$2"$'\t'"info"$'\t'"operational"$'\t'"$3"$'\t'"$4"$'\t'"{}"
}

refused "refuses while the relay has shipped nothing" "no relay rows" load load
live "2026-11-09 11:59:00.000" pihole/dns "<30>Nov  9 11:59:00 pihole dnsmasq[411]: a shape that is not the file line" "boot:1:a"
refused "refuses when no relay row equals a staged line" "none equals a staged line" load load
expect "a refused load wrote nothing" 0 "$(count "source = 'import'")"

# The forwarder started between the first and second of the three equal lines.
live "2026-11-07 10:00:00.412" pihole/dns "Nov  7 10:00:00 dnsmasq[411]: query[AAAA] b.example.org from 192.0.2.11" "boot:2:b"
live "2026-11-07 10:00:00.413" pihole/dns "Nov  7 10:00:00 dnsmasq[411]: query[AAAA] b.example.org from 192.0.2.11" "boot:3:b"
live "2026-11-09 09:00:00.250" pihole/dns "Nov  9 09:00:00 dnsmasq[411]: query[A] c.example.org from 192.0.2.12" "boot:4:c"
live "2026-11-09 08:00:00.090" pihole/ftl "2026-11-09 08:00:00.000 UTC [411/T412] INFO: Optimized database in 0.012 seconds" "boot:5:d"
refused "verify fails before the load" "verify FAILED" load verify
refused "clean refuses without a passed verify" "no passed verify" load clean

if load load >"$tmp/load.log" 2>&1; then ok "loads and verifies"; else no "load failed: $(cat "$tmp/load.log")"; fi
after_load() {
	expect "$1: dns file lines in, minus the 3 the relay holds" 8 "$(count "unit = 'pihole/dns' AND startsWith(entry, 'pihole-import:')")"
	expect "$1: 3 rebuilt rows" 3 "$(count "unit = 'pihole/dns' AND startsWith(entry, 'pihole-import-db:')")"
	expect "$1: ftl lines in, minus the 1 the relay holds" 4 "$(count "unit = 'pihole/ftl' AND source = 'import'")"
	expect "$1: web lines in" 2 "$(count "unit = 'pihole/web' AND source = 'import' AND retention = 'audit'")"
	expect "$1: the three equal lines are three rows, one old and two live" "1 2" "$(db --query "
		SELECT countIf(source = 'import'), countIf(source = 'journald') FROM logs FINAL
		WHERE host = 'pihole' AND message LIKE '%b.example.org%' FORMAT TSV" | tr '\t' ' ')"
	expect "$1: each line is in the table once" 0 "$(db --query "
		SELECT count() FROM (SELECT message, count() AS c FROM logs FINAL WHERE host = 'pihole' AND unit = 'pihole/dns'
		GROUP BY message HAVING c != if(message LIKE '%b.example.org%', 3, if(message LIKE '%query[A] a.example.org%', 2, 1)))")"
}
after_load "first load"
if load load >"$tmp/load2.log" 2>&1; then ok "loads a second time"; else no "second load failed: $(cat "$tmp/load2.log")"; fi
after_load "second load"
expect "verify reports nothing missing" "0 0 0" "$(awk '$1 ~ /^pihole\// {print $NF}' "$tmp/load2.log" | tr '\n' ' ' | sed 's/ $//')"

echo "clean"
if load clean >/dev/null 2>&1 && [[ ! -e "$tmp/main" ]]; then ok "removes the staging after a passed verify"; else no "clean left $tmp/main"; fi

if [[ $fails -eq 0 ]]; then
	echo "[PASS] pihole-history: export SQL, transform, load, verify and clean hold on synthetic data in ${IMAGE}"
else
	echo "[FAIL] pihole-history: $fails check(s) failed" >&2
	exit 1
fi
