-- SPDX-License-Identifier: GPL-3.0-or-later
-- Copyright (c) 2024-2026 Stevie Bellis.
--
-- Pi-hole FTL query database -> one JSON object per query, for
-- transform.sql. SQLite dialect; scripts/pihole-history.sh runs it read-only
-- inside the container (pihole-FTL sqlite3). FTL v6 schema: query_storage
-- holds ids, the *_by_id tables hold the text (this is FTL's own `queries`
-- view, plus the client name the view leaves out).
--
-- Not exported: `session` (web logins; holds live session ids and CSRF
-- tokens) and `message` (FTL diagnostics, also written to FTL.log).
-- well:ignore schema-outside-ledger reads Pi-hole's SQLite file, not the Well
SELECT json_object(
    'id', q.id,
    'ts', CAST(q.timestamp AS REAL),
    'type', q.type,
    'status', q.status,
    'domain', CAST(COALESCE(d.domain, q.domain) AS TEXT),
    'client', CAST(COALESCE(c.ip, q.client) AS TEXT),
    'client_name', c.name,
    'forward', CAST(COALESCE(f.forward, q.forward) AS TEXT),
    'additional_info', CAST(COALESCE(a.content, q.additional_info) AS TEXT),
    'reply_type', q.reply_type,
    'reply_time', q.reply_time,
    'dnssec', q.dnssec,
    'list_id', q.list_id,
    'ede', q.ede)
FROM query_storage q
LEFT JOIN domain_by_id d ON q.domain = d.id
LEFT JOIN client_by_id c ON q.client = c.id
LEFT JOIN forward_by_id f ON q.forward = f.id
LEFT JOIN addinfo_by_id a ON q.additional_info = a.id
ORDER BY q.id;
