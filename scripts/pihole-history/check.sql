-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (c) 2024-2026 Stevie Bellis.
--
-- Where does every staged row stand? One unit's .native arrives as the
-- external table `staged` (clickhouse-client --external); one row comes
-- back. ClickHouse dialect, read-only, run by scripts/pihole-history.sh
-- before the load (is the relay there, do its rows match ours) and after it
-- (verify). Parameter: unit.
--
--   staged        rows in the file
--   live          rows the relay holds for this unit
--   in_table      staged rows already in unheaded_logs.logs
--   left_to_live  file lines not imported because the relay has that copy
--   not_rebuilt   rebuilt rows held back: the table has real rows that old
--   missing       none of the above. After a load this must be 0.
-- well:ignore schema-outside-ledger ClickHouse log store, not the Well (ADR-101's ledger is Postgres-only)
WITH
    (
        SELECT if(count() = 0, toDateTime64('2299-01-01 00:00:00', 3, 'UTC'), min(timestamp))
        FROM unheaded_logs.logs
        WHERE host = 'pihole' AND unit = 'pihole/dns' AND NOT startsWith(entry, 'pihole-import-db:')
    ) AS oldest_real,
    (
        SELECT count() FROM unheaded_logs.logs FINAL
        WHERE host = 'pihole' AND unit = {unit:String} AND source != 'import'
    ) AS live_rows
SELECT
    count() AS staged,
    live_rows AS live,
    countIf(present) AS in_table,
    countIf(NOT present AND from_db = 0 AND msg_ordinal + shipped > msg_total) AS left_to_live,
    countIf(NOT present AND from_db = 1 AND timestamp >= oldest_real) AS not_rebuilt,
    staged - in_table - left_to_live - not_rebuilt AS missing
FROM
(
    SELECT
        s.timestamp AS timestamp, s.from_db AS from_db, s.msg_ordinal AS msg_ordinal, s.msg_total AS msg_total, l.shipped AS shipped,
        (s.timestamp, s.entry) IN
        (
            SELECT timestamp, entry FROM unheaded_logs.logs
            WHERE host = 'pihole' AND unit = {unit:String} AND source = 'import'
        ) AS present
    FROM staged AS s
    LEFT JOIN
    (
        SELECT message, toUInt32(count()) AS shipped
        FROM unheaded_logs.logs FINAL
        WHERE host = 'pihole' AND unit = {unit:String} AND source != 'import'
        GROUP BY message
    ) AS l ON l.message = s.message
    WHERE s.unit = {unit:String}
)
FORMAT TSVWithNames
