-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (c) 2024-2026 Stevie Bellis.
--
-- Staged file lines (transform.sql, one unit's .native on stdin) ->
-- unheaded_logs.logs. ClickHouse dialect, one statement, run through
-- clickhouse-client by scripts/pihole-history.sh. Parameter: unit.
--
-- The files and the relay overlap: every line written since the forwarder
-- started is in both. A line the relay already shipped is not imported.
-- `shipped` is how many rows the relay holds with this exact message; the relay
-- has the newest copies, so of msg_total copies the oldest (msg_total - shipped)
-- are imported. A line the relay changed on the way (it escapes control
-- characters) matches nothing and is imported too: a duplicate, never a loss.
-- well:ignore schema-outside-ledger ClickHouse log store, not the Well (ADR-101's ledger is Postgres-only)
INSERT INTO unheaded_logs.logs (timestamp, host, source, unit, level, retention, message, entry, fields)
SELECT i.timestamp, i.host, i.source, i.unit, i.level, i.retention, i.message, i.entry, i.fields
FROM input('timestamp DateTime64(3, ''UTC''), host String, source String, unit String, level String, retention String, message String, entry String, fields String, msg_ordinal UInt32, msg_total UInt32, from_db UInt8') AS i
LEFT JOIN
(
    SELECT message, toUInt32(count()) AS shipped
    FROM unheaded_logs.logs FINAL
    WHERE host = 'pihole' AND unit = {unit:String} AND source != 'import'
    GROUP BY message
) AS l ON l.message = i.message
WHERE i.unit = {unit:String} AND i.from_db = 0 AND i.msg_ordinal + l.shipped <= i.msg_total
FORMAT Native
