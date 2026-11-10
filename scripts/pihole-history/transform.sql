-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (c) 2024-2026 Stevie Bellis.
--
-- Old Pi-hole history -> rows shaped like the ones the syslog relay writes
-- (config/vector/vector.yaml, journal_rows; config/clickhouse/schema/001-logs.sql).
-- ClickHouse dialect, run by scripts/pihole-history.sh in `clickhouse local`
-- with no network. Reads /in, writes /out. Nothing here talks to a server.
--
-- In:  /in/logs/pihole.log*     dnsmasq lines            -> unit pihole/dns
--      /in/logs/FTL.log*        FTL events               -> unit pihole/ftl
--      /in/logs/webserver.log*  admin UI server          -> unit pihole/web
--      /in/ftl-queries.jsonl    export-queries.sql       -> unit pihole/dns
--      A log that does not exist is no rows; ftl-queries.jsonl must exist
--      (it may be empty).
-- Out: /out/pihole-{dns,ftl,web}.native, /out/summary.tsv
--
-- A file line becomes one row, word for word: the relay ships a file line
-- as the whole message too (rsyslog imfile), so old and new rows read alike.
-- Days older than the oldest pihole.log line exist only in FTL's database:
-- one row per query, the message rebuilt in dnsmasq's wording without the
-- pid the database never had ("dnsmasq: query[A] name from client"), and
-- every column the database holds kept in `fields` as FTL_*.
--
-- Every row is marked in `fields`: UNHEADED_IMPORT=pihole-history,
-- UNHEADED_IMPORT_FROM=file|ftl-db, the export id, and the file and line it
-- came from. `source` is 'import', not 'journald': these rows never passed
-- the journal.
--
-- Parameters: ref  export time, 'YYYY-MM-DD hh:mm:ss' UTC. dnsmasq lines
--                  carry no year; a line takes the latest year that does not
--                  put it more than a day after ref.
--             tz   the container's time zone (IANA name).
--             export_id  names this export in every row.
-- well:ignore schema-outside-ledger ClickHouse log store, not the Well (ADR-101's ledger is Postgres-only)

SET max_threads = 1;                       -- file order is line order
SET engine_file_truncate_on_insert = 1;

CREATE TABLE raw (unit String, file String, n UInt64, line String) ENGINE = Memory;
INSERT INTO raw SELECT 'pihole/dns', _file, rowNumberInAllBlocks(), line
FROM file('/in/logs/pihole.log*', 'LineAsString', 'line String');
INSERT INTO raw SELECT 'pihole/ftl', _file, rowNumberInAllBlocks(), line
FROM file('/in/logs/FTL.log*', 'LineAsString', 'line String');
INSERT INTO raw SELECT 'pihole/web', _file, rowNumberInAllBlocks(), line
FROM file('/in/logs/webserver.log*', 'LineAsString', 'line String');

-- Each line's own time. NULL when the line has no stamp.
CREATE TABLE stamped ENGINE = Memory AS
WITH
    toDateTime({ref:String}, 'UTC') AS ref,
    extractGroups(line, '^([A-Z][a-z]{2}) ([ 0-9][0-9]) ([0-9]{2}):([0-9]{2}):([0-9]{2}) ') AS g,
    indexOf(['Jan','Feb','Mar','Apr','May','Jun','Jul','Aug','Sep','Oct','Nov','Dec'], g[1]) AS mon,
    toYear(ref, {tz:String}) AS y,
    makeDateTime(y, mon, toUInt8OrZero(trimLeft(g[2])), toUInt8OrZero(g[3]), toUInt8OrZero(g[4]), toUInt8OrZero(g[5]), {tz:String}) AS this_year,
    makeDateTime(y - 1, mon, toUInt8OrZero(trimLeft(g[2])), toUInt8OrZero(g[3]), toUInt8OrZero(g[4]), toUInt8OrZero(g[5]), {tz:String}) AS last_year,
    extract(line, '^([0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{3}) [A-Z]+ \\[') AS ftl_stamp,
    extract(line, '^\\[([0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}\\.[0-9]{3}) [A-Z]+ [0-9]+\\]') AS web_stamp
SELECT
    unit, file, n, line,
    toUInt32OrZero(extract(file, '\\.log\\.([0-9]+)')) AS rotation,
    multiIf(
        unit = 'pihole/dns' AND mon > 0,
            toDateTime64(if(this_year > ref + INTERVAL 1 DAY, last_year, this_year), 3, 'UTC'),
        unit = 'pihole/ftl' AND ftl_stamp != '',
            toTimeZone(parseDateTime64BestEffortOrNull(ftl_stamp, 3, {tz:String}), 'UTC'),
        unit = 'pihole/web' AND web_stamp != '',
            toTimeZone(parseDateTime64BestEffortOrNull(web_stamp, 3, {tz:String}), 'UTC'),
        NULL) AS own
FROM raw;

-- A line without a stamp takes the last stamp above it in its file, else the
-- first one below it. A file with no stamp at all stops the run (see the end).
CREATE TABLE lines ENGINE = Memory AS
SELECT
    unit, file, rotation, line,
    n - min(n) OVER (PARTITION BY unit, file) + 1 AS lineno,
    own IS NULL AS carried,
    coalesce(
        anyLast(own) OVER (PARTITION BY unit, file ORDER BY n ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW),
        any(own) OVER (PARTITION BY unit, file ORDER BY n ROWS BETWEEN CURRENT ROW AND UNBOUNDED FOLLOWING)) AS ts
FROM stamped;

-- The row shape of unheaded_logs.logs, plus three columns the load needs:
-- which copy of this exact line in its unit this is (oldest first), how
-- many copies there are, and whether the row was rebuilt. The relay ships
-- the newest copies; the load leaves those to it and imports the rest.
--
-- `entry` is the row's identity (the table collapses equal keys): the copy
-- number within one instant plus a hash of the line, so loading the same
-- export twice, or a later export of the same files, adds nothing.
CREATE TABLE rows_out
(
    timestamp DateTime64(3, 'UTC'), host String, source String, unit String, level String,
    retention String, message String, entry String, fields String,
    msg_ordinal UInt32, msg_total UInt32, from_db UInt8
) ENGINE = Memory;

INSERT INTO rows_out
WITH
    if(unit = 'pihole/web', 'audit', 'operational') AS keep,
    transform(unit, ['pihole/dns', 'pihole/ftl', 'pihole/web'], ['pihole-dns', 'pihole-ftl', 'pihole-web'], '') AS tag,
    row_number() OVER (PARTITION BY unit, ts, line ORDER BY rotation DESC, lineno ASC) AS ordinal
SELECT
    assumeNotNull(ts), 'pihole', 'import', unit, 'info', keep, line,
    concat('pihole-import:', toString(ordinal), ':', toString(cityHash64(line))),
    toJSONString(mapFromArrays(
        arrayConcat(['SYSLOG_IDENTIFIER', 'UNHEADED_SOURCE', 'UNHEADED_SOURCE_HOST', 'UNHEADED_RETENTION',
                     'UNHEADED_IMPORT', 'UNHEADED_IMPORT_FROM', 'UNHEADED_IMPORT_ID', 'UNHEADED_IMPORT_FILE', 'UNHEADED_IMPORT_LINE'],
                    if(carried, ['UNHEADED_IMPORT_TIME'], [])),
        arrayConcat([tag, unit, 'pihole', keep,
                     'pihole-history', 'file', {export_id:String}, file, toString(lineno)],
                    if(carried, ['carried'], [])))),
    row_number() OVER (PARTITION BY unit, line ORDER BY ts ASC, rotation DESC, lineno ASC),
    count() OVER (PARTITION BY unit, line),
    0
FROM lines
WHERE ts IS NOT NULL;

-- FTL's database, for the time before the oldest dnsmasq line only.
INSERT INTO rows_out
WITH
    (SELECT min(timestamp) FROM rows_out WHERE unit = 'pihole/dns') AS oldest_line,
    fromUnixTimestamp64Milli(toInt64(floor(ts * 1000)), 'UTC') AS t,
    transform(type, [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16],
              ['A', 'AAAA', 'ANY', 'SRV', 'SOA', 'PTR', 'TXT', 'NAPTR', 'MX', 'DS', 'RRSIG', 'DNSKEY', 'NS', 'OTHER', 'SVCB', 'HTTPS'],
              if(type > 100, concat('type=', toString(type - 100)), concat('ftl-type=', toString(type)))) AS qtype,
    concat(formatDateTime(t, '%b %e %H:%i:%S', {tz:String}), ' dnsmasq: query[', qtype, '] ', domain, ' from ', client) AS msg,
    arrayFilter(p -> p.2 IS NOT NULL, [
        ('SYSLOG_IDENTIFIER', 'pihole-dns'), ('UNHEADED_SOURCE', 'pihole/dns'), ('UNHEADED_SOURCE_HOST', 'pihole'),
        ('UNHEADED_RETENTION', 'operational'), ('UNHEADED_IMPORT', 'pihole-history'), ('UNHEADED_IMPORT_FROM', 'ftl-db'),
        ('UNHEADED_IMPORT_ID', {export_id:String}),
        ('FTL_ID', toString(id)), ('FTL_TYPE', toString(type)), ('FTL_STATUS', toString(status)),
        ('FTL_CLIENT_NAME', client_name), ('FTL_FORWARD', forward), ('FTL_ADDITIONAL_INFO', additional_info),
        ('FTL_REPLY_TYPE', toString(reply_type)), ('FTL_REPLY_TIME', toString(reply_time)),
        ('FTL_DNSSEC', toString(dnssec)), ('FTL_LIST_ID', toString(list_id)), ('FTL_EDE', toString(ede))]) AS kv
SELECT
    t, 'pihole', 'import', 'pihole/dns', 'info', 'operational', msg,
    concat('pihole-import-db:', toString(id), ':', toString(cityHash64(msg))),
    toJSONString(mapFromArrays(arrayMap(p -> p.1, kv), arrayMap(p -> assumeNotNull(p.2), kv))),
    1, 1, 1
FROM file('/in/ftl-queries.jsonl', 'JSONEachRow',
          'id Int64, ts Float64, type Int32, status Int32, domain String, client String,
           client_name Nullable(String), forward Nullable(String), additional_info Nullable(String),
           reply_type Nullable(Int32), reply_time Nullable(Float64), dnssec Nullable(Int32),
           list_id Nullable(Int64), ede Nullable(Int32)')
WHERE oldest_line IS NULL OR t < oldest_line;

-- Operational rows live 90 days (the table's TTL); older ones would be
-- deleted at the next merge, so they are counted and left out.
CREATE TABLE kept ENGINE = Memory AS
SELECT * FROM rows_out
WHERE retention = 'audit' OR timestamp >= toDateTime({ref:String}, 'UTC') - INTERVAL 90 DAY;

INSERT INTO FUNCTION file('/out/summary.tsv', 'TSVWithNames')
SELECT * FROM
(
    SELECT 'read' AS what, unit, 'file' AS origin, toString(count()) AS rows_, '' AS first_, '' AS last_ FROM raw GROUP BY unit
    UNION ALL
    SELECT 'read', 'pihole/dns', 'ftl-db', toString(count()), '', '' FROM file('/in/ftl-queries.jsonl', 'JSONEachRow', 'id Int64')
    UNION ALL
    SELECT 'written', unit, if(from_db = 1, 'ftl-db', 'file'), toString(count()), toString(min(timestamp)), toString(max(timestamp)) FROM kept GROUP BY unit, from_db
    UNION ALL
    SELECT 'older-than-90-days', unit, if(from_db = 1, 'ftl-db', 'file'), toString(count()), toString(min(timestamp)), toString(max(timestamp))
    FROM rows_out WHERE NOT (retention = 'audit' OR timestamp >= toDateTime({ref:String}, 'UTC') - INTERVAL 90 DAY) GROUP BY unit, from_db
    UNION ALL
    SELECT 'time-carried', unit, 'file', toString(count()), '', '' FROM lines WHERE carried AND ts IS NOT NULL GROUP BY unit
    UNION ALL
    -- A cross-check, not a filter: inside the time the files cover, dnsmasq's
    -- query lines and FTL's stored queries should be close to the same number.
    SELECT 'queries-in-file-range', 'pihole/dns', 'file', toString(count()), toString(min(timestamp)), toString(max(timestamp))
    FROM rows_out WHERE unit = 'pihole/dns' AND from_db = 0 AND message LIKE '%: query[%'
    UNION ALL
    SELECT 'queries-in-file-range', 'pihole/dns', 'ftl-db', toString(count()), '', ''
    FROM file('/in/ftl-queries.jsonl', 'JSONEachRow', 'id Int64, ts Float64')
    WHERE fromUnixTimestamp64Milli(toInt64(floor(ts * 1000)), 'UTC') >= (SELECT min(timestamp) FROM rows_out WHERE unit = 'pihole/dns' AND from_db = 0)
    UNION ALL
    SELECT 'no-time-in-file', unit, file, toString(count()), '', '' FROM lines WHERE ts IS NULL GROUP BY unit, file
)
ORDER BY what, unit, origin;

INSERT INTO FUNCTION file('/out/pihole-dns.native', 'Native') SELECT * FROM kept WHERE unit = 'pihole/dns';
INSERT INTO FUNCTION file('/out/pihole-ftl.native', 'Native') SELECT * FROM kept WHERE unit = 'pihole/ftl';
INSERT INTO FUNCTION file('/out/pihole-web.native', 'Native') SELECT * FROM kept WHERE unit = 'pihole/web';

-- Last, so the summary above is on disk to read: a file in which no line
-- has a time cannot be placed, and leaving it out would lose it silently.
SELECT throwIf(count() > 0, 'pihole-history: a log file has no line with a time; see no-time-in-file in summary.tsv')
FROM lines WHERE ts IS NULL FORMAT Null;
