-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (c) 2024-2026 Stevie Bellis.
--
-- Staged rows rebuilt from FTL's database (transform.sql, pihole-dns.native
-- on stdin) -> unheaded_logs.logs. ClickHouse dialect, one statement, run
-- after load-lines.sql by scripts/pihole-history.sh.
--
-- A rebuilt row stands in for a dnsmasq line that no longer exists, so it
-- goes in only before the oldest real pihole/dns row the table holds (a file
-- line or a relay row). A second export, taken after the oldest file has
-- rotated away, therefore cannot rebuild days the first one loaded from files.
-- well:ignore schema-outside-ledger ClickHouse log store, not the Well (ADR-101's ledger is Postgres-only)
INSERT INTO unheaded_logs.logs (timestamp, host, source, unit, level, retention, message, entry, fields)
SELECT timestamp, host, source, unit, level, retention, message, entry, fields
FROM input('timestamp DateTime64(3, ''UTC''), host String, source String, unit String, level String, retention String, message String, entry String, fields String, msg_ordinal UInt32, msg_total UInt32, from_db UInt8')
WHERE from_db = 1 AND timestamp <
(
    SELECT if(count() = 0, toDateTime64('2299-01-01 00:00:00', 3, 'UTC'), min(timestamp))
    FROM unheaded_logs.logs
    WHERE host = 'pihole' AND unit = 'pihole/dns' AND NOT startsWith(entry, 'pihole-import-db:')
)
FORMAT Native
