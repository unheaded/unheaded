-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — one audit taxonomy (W-04), unheaded_ops.
--
-- 004's audit_events had its own event types ('auth.login', ...), severity
-- 'warn' and outcomes 'denied'/'error', and nothing ever wrote to it.
-- pkg/audit, the code that produces audit events, uses another:
-- types authentication/authorization/configuration/data_access/system,
-- severity info/warning/error/critical, outcome success/failure/pending.
-- Stevie 2026-10-15: pick one. The table now follows pkg/audit, column for
-- column with pkg/audit/storage, so that package can write it.
--
-- The table is replaced only when empty (it is, on every Well as of
-- 2026-10-15); with rows this refuses and needs a data migration instead.
-- 015's append-only triggers and grants are re-created.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM audit_events) THEN
        RAISE EXCEPTION 'audit_events has rows: migrate them to the pkg/audit taxonomy instead of replacing the table';
    END IF;
END $$;

DROP TABLE audit_events;

CREATE TABLE audit_events (
    id              TEXT PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL,
    event_type      TEXT NOT NULL
        CHECK (event_type IN ('authentication', 'authorization', 'configuration', 'data_access', 'system')),
    severity        TEXT NOT NULL
        CHECK (severity IN ('info', 'warning', 'error', 'critical')),
    action          TEXT NOT NULL CHECK (action <> ''),
    outcome         TEXT NOT NULL
        CHECK (outcome IN ('success', 'failure', 'pending')),
    actor_id        TEXT,
    actor_type      TEXT,
    actor_name      TEXT,
    resource_id     TEXT,
    resource_type   TEXT,
    source_ip       TEXT,
    source_service  TEXT,
    request_id      TEXT,
    details         JSONB NOT NULL DEFAULT '{}',
    previous_hash   TEXT NOT NULL DEFAULT '',
    hash            CHAR(64) NOT NULL CHECK (hash ~ '^[0-9a-f]{64}$'),
    signature       TEXT NOT NULL DEFAULT '',
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Time range and per-actor queries; event_type and severity have a
-- handful of values each and are not worth an index of their own (W-14).
CREATE INDEX idx_audit_events_occurred_at ON audit_events (occurred_at DESC);
CREATE INDEX idx_audit_events_actor_id ON audit_events (actor_id) WHERE actor_id IS NOT NULL;

CREATE OR REPLACE TRIGGER trg_audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_audit_events_no_truncate
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT ON audit_events TO ops_writer;
GRANT SELECT ON audit_events TO ops_reader;
