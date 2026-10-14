-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — migration ledger (ADR-101). Not a migration: db/init.sh and
-- cmd/well-migrate apply it to each database before recording anything
-- there. Idempotent.
--
-- One row per migration file applied to this database, with the sha256 of
-- the file as it ran. mode: initdb (db/init.sh on an empty volume), apply
-- (well-migrate), baseline (recorded without running, after a shape diff
-- showed the schema already matches). Append-only, like 013-016's history.

CREATE TABLE IF NOT EXISTS well_schema_migrations (
    filename    TEXT PRIMARY KEY CHECK (filename ~ '^[0-9]{3}_[a-z0-9_]+\.sql$'),
    sha256      CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    mode        TEXT NOT NULL CHECK (mode IN ('initdb', 'apply', 'baseline')),
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by  TEXT NOT NULL DEFAULT current_user
);

CREATE OR REPLACE FUNCTION well_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_well_schema_migrations_append_only
    BEFORE UPDATE OR DELETE ON well_schema_migrations
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_well_schema_migrations_no_truncate
    BEFORE TRUNCATE ON well_schema_migrations
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

REVOKE ALL ON well_schema_migrations FROM PUBLIC;
