-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — browser sessions of the login fronts (ADR-114 N5, decision
-- pack 2026-11-18 row 44), unheaded_app.
--
-- A front (pkg/bff) running with AUTH_SESSION_STORE=well writes one row per
-- browser login. The browser holds a cookie of 32 random bytes; the row is
-- found by id_hash, the SHA-256 of the cookie value. Neither the value nor
-- any token is stored: nothing in this table can be presented by a browser.
--
-- A session ends at expires_at, which moves forward while the session is in
-- use (the idle time) and never past absolute_expires_at (the maximum age,
-- fixed at login); or earlier, when revoked_at is set, by a logout or by
-- `unheaded session revoke`. An ended row stays for the front's retention
-- time, then the front's sweep deletes it.
--
-- Two ways in, and no third:
--
--   bff_sessions_own   the view a front's role (055) is granted. It shows,
--                      and accepts, the rows whose front is the one the
--                      connected role is named after: bff_kanban_app sees
--                      front 'kanban-app'. WITH CHECK OPTION refuses an
--                      insert or update that would leave a row outside
--                      that, so one front cannot write a session in
--                      another front's name. No front role has any grant
--                      on the table itself.
--   bff_sessions       the table. app_auth (035; unheaded-cli) may read it
--                      and set revoked_at, and nothing else: the command
--                      line lists and revokes, and cannot create a session.
--
-- The front's name carries no underscore (CHECK), so the role name is
-- unambiguous: 'a-b' and 'a_b' cannot both exist.
--
-- What may change in a row: last_used_at, expires_at, revoked_at. revoked_at
-- is set once; a revoked row never changes again. The trigger holds the
-- owner to this too.
--
-- Access paths (pkg/bff/sessions/well.go):
--   every request with a session cookie:
--     SELECT ... FROM bff_sessions_own WHERE id_hash = $1 AND front = $2   (primary key)
--   at most once a minute per session in use:
--     UPDATE bff_sessions_own SET last_used_at, expires_at WHERE id_hash = $1 ...
--   each login: one INSERT ... SELECT with a count of the front's live rows
--     (bff_sessions_front_expires_idx), bounded by the front's cap (10000);
--   each minute per front: DELETE of at most 1000 ended rows, oldest first;
--   the command line: list (LIMIT 1000), revoke by id prefix or by subject
--     (bff_sessions_subject_idx).
--
-- Size: one row per login, deleted the retention time (default 24 h) after
-- it ends; a session lives at most 31 days (CHECK; the default maximum age
-- is 12 h). Logins are rate-limited per client address, and a front refuses
-- a login past its cap of live sessions, so the table is bounded: tens of
-- rows on a lab, at most the caps on a busy deployment.
--
-- Lock plan: a new table, view and function; no lock on anything that
-- exists. Rollback: a later numbered migration drops the view and the
-- table (every front then answers logins 503 until AUTH_SESSION_STORE is
-- unset). Proof: scripts/well/test-bff-sessions.sh (throwaway server, with
-- the Go store's suite).

CREATE TABLE bff_sessions (
    id_hash             TEXT PRIMARY KEY CHECK (id_hash ~ '^[0-9a-f]{64}$'),
    front               TEXT NOT NULL CHECK (front ~ '^[a-z0-9][a-z0-9-]{0,58}$'),
    subject             TEXT NOT NULL CHECK (length(subject) BETWEEN 1 AND 255 AND subject !~ '[[:space:][:cntrl:]]'),
    roles               TEXT NOT NULL CHECK (roles ~ '^([a-z][a-z0-9_-]{0,31}(,[a-z][a-z0-9_-]{0,31}){0,15})?$'),
    created_at          TIMESTAMPTZ NOT NULL,
    last_used_at        TIMESTAMPTZ NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at          TIMESTAMPTZ,
    CHECK (last_used_at >= created_at),
    CHECK (expires_at > created_at),
    CHECK (expires_at <= absolute_expires_at),
    CHECK (absolute_expires_at <= created_at + interval '31 days'),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX bff_sessions_front_expires_idx ON bff_sessions (front, expires_at);
CREATE INDEX bff_sessions_subject_idx ON bff_sessions (subject);

-- Update: the three moving columns only; a revoked row is final; use and
-- expiry do not move backwards in a way that would hide a use.
CREATE OR REPLACE FUNCTION bff_sessions_update() RETURNS TRIGGER AS $$
BEGIN
    IF (to_jsonb(NEW) - 'last_used_at' - 'expires_at' - 'revoked_at') IS DISTINCT FROM
       (to_jsonb(OLD) - 'last_used_at' - 'expires_at' - 'revoked_at') THEN
        RAISE EXCEPTION 'bff_sessions: a session is immutable except last_used_at, expires_at and revoked_at';
    END IF;
    IF OLD.revoked_at IS NOT NULL AND to_jsonb(NEW) IS DISTINCT FROM to_jsonb(OLD) THEN
        RAISE EXCEPTION 'bff_sessions: a revoked session is never changed';
    END IF;
    IF NEW.last_used_at < OLD.last_used_at THEN
        RAISE EXCEPTION 'bff_sessions: last_used_at does not move backwards';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_bff_sessions_update BEFORE UPDATE ON bff_sessions
    FOR EACH ROW EXECUTE FUNCTION bff_sessions_update();

-- A front's window on the table: the rows of the front its role is named
-- after. security_barrier keeps a caller's own conditions from running on
-- rows the view does not show.
CREATE VIEW bff_sessions_own WITH (security_barrier = true) AS
    SELECT id_hash, front, subject, roles, created_at, last_used_at, expires_at, absolute_expires_at, revoked_at
    FROM bff_sessions
    WHERE 'bff_' || replace(front, '-', '_') = current_user::text
    WITH CASCADED CHECK OPTION;

GRANT SELECT, INSERT, UPDATE (last_used_at, expires_at, revoked_at), DELETE ON bff_sessions_own
    TO bff_dashboard_backend, bff_kanban_app, bff_wiki_server;

GRANT SELECT, UPDATE (revoked_at) ON bff_sessions TO app_auth;
