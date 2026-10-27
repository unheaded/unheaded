-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — API keys for automation (S-IRON-GATE Phase 3, ADR-103 §5;
-- Q4 decided in the plan's Step 10: the Well where it runs, a 0600 file
-- elsewhere, same format), unheaded_app.
--
-- A key is `uhk_` + 32 random bytes (base64url), shown once by
-- `unheaded-cli apikey create`. Only its SHA-256 is kept (key_hash), with
-- the first 12 characters (prefix) to name it in `apikey list` and
-- `apikey revoke`. pkg/auth (WellStore) checks a presented key by its hash.
--
-- Every key expires: expires_at is required and at most 366 days after
-- created_at (the CLI caps at 365, default 90). A key is never deleted:
-- revoking sets revoked_at once, and the row stays as the record of who
-- held which role when. key_hash, prefix, owner, role and the dates other
-- than revoked_at/last_used_at never change, the owner included.
--
-- Roles are pkg/auth's RBAC roles a caller can hold (rbac.go): admin,
-- operator, observer, service. (The plan said 'viewer'; pkg/auth has no
-- such role, so a viewer key would have matched no policy.)
--
-- Writer and reader: app_auth (035): SELECT, INSERT, UPDATE of revoked_at
-- and last_used_at; no DELETE. Owner: well_app_owner. No other app role is
-- granted anything; well_backup reads it for dumps (pg_read_all_data).
--
-- Access paths: authentication `WHERE key_hash = $1` (the unique index),
-- once per request that carries a key; revoke and the create-time clash
-- check `WHERE prefix = $1` (prefix index) or `WHERE id = $1` (the primary
-- key); list reads the whole table by created_at (tens of rows).
--
-- Size: one row per key ever issued; hundreds at most. last_used_at is
-- written at most once a minute per key.
--
-- Lock plan: a new table, no lock on anything that exists. Rollback: a
-- later numbered migration drops it. Proof: scripts/well/test-api-keys.sh
-- (throwaway server, with the Go store's tests).

CREATE TABLE api_keys (
    id           UUID PRIMARY KEY,
    key_hash     CHAR(64) NOT NULL UNIQUE CHECK (key_hash ~ '^[0-9a-f]{64}$'),
    prefix       CHAR(12) NOT NULL CHECK (prefix ~ '^uhk_[A-Za-z0-9_-]{8}$'),
    owner        TEXT NOT NULL CHECK (owner ~ '^[A-Za-z0-9._@-]{1,128}$'),
    role         TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'observer', 'service')),
    -- well:ignore open-vocabulary-check the four roles are pkg/auth's RBAC roles (rbac.go); a new role means new policies, so it is a schema change on purpose
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CHECK (expires_at > created_at),
    CHECK (expires_at <= created_at + interval '366 days'),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX api_keys_prefix_idx ON api_keys (prefix);

-- Update: revoked_at is set once and never cleared; last_used_at moves.
-- Everything else is fixed at insert.
CREATE OR REPLACE FUNCTION api_keys_update() RETURNS TRIGGER AS $$
BEGIN
    IF (to_jsonb(NEW) - 'revoked_at' - 'last_used_at') IS DISTINCT FROM
       (to_jsonb(OLD) - 'revoked_at' - 'last_used_at') THEN
        RAISE EXCEPTION 'api_keys: % is immutable except revoked_at and last_used_at', OLD.id;
    END IF;
    IF OLD.revoked_at IS NOT NULL AND NEW.revoked_at IS DISTINCT FROM OLD.revoked_at THEN
        RAISE EXCEPTION 'api_keys: % is revoked; a revocation is never changed or undone', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_api_keys_update BEFORE UPDATE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION api_keys_update();
CREATE OR REPLACE TRIGGER trg_api_keys_no_delete BEFORE DELETE ON api_keys
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_api_keys_no_truncate BEFORE TRUNCATE ON api_keys
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT, UPDATE (revoked_at, last_used_at) ON api_keys TO app_auth;
