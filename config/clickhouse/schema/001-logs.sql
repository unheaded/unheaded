-- SPDX-License-Identifier: GPL-3.0-or-later
-- Host logs shipped by Vector (config/vector/vector.yaml), ADR-092 step 8.
-- Applied on every `docker compose up` by the clickhouse-schema one-shot, so
-- every statement must be idempotent.
-- well:ignore schema-outside-ledger ClickHouse log store, not the Well: ADR-101's ledger is Postgres-only, and this file is the whole ClickHouse schema, applied idempotently by clickhouse-schema
--
-- Retention (docs/policy/retention-policy-v1-2026-05-06.md):
--   audit        auth/authpriv facilities and kernel audit records (sshd,
--                sudo, PAM, login): 90 days online + 1 year archive, both
--                kept here until an archive tier exists -> 455 days
--   operational  everything else: 90 days
CREATE TABLE IF NOT EXISTS unheaded_logs.logs
-- well:ignore mysql-syntax,no-primary-key ClickHouse DDL: LowCardinality/DateTime64 are its types and ORDER BY is a MergeTree's key
(
    timestamp  DateTime64(3, 'UTC'),
    host       LowCardinality(String),
    source     LowCardinality(String),  -- journald
    unit       LowCardinality(String),  -- systemd unit, else syslog identifier
    level      LowCardinality(String),  -- syslog priority name
    retention  LowCardinality(String),  -- audit | operational
    message    String,
    entry      String,                  -- boot:monotonic_us:hash(message), one per journal entry
    fields     String                   -- every other journal field, JSON
)
-- Vector is at-least-once: after a crash it resends from its last
-- checkpoint. Rows with the same key (one journal entry) collapse on merge;
-- query with FINAL for an exact count before that.
ENGINE = ReplacingMergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (host, unit, timestamp, entry)
TTL toDateTime(timestamp) + INTERVAL 90 DAY DELETE WHERE retention != 'audit',
    toDateTime(timestamp) + INTERVAL 455 DAY DELETE WHERE retention = 'audit';
