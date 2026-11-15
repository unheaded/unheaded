-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — timeline_milestones: timeguru's one home (ADR-147), unheaded_app.
--
-- timeguru kept its timeline in a SQLite file on a tmpfs (/data/timeguru.db),
-- so every restart lost what the API had changed, while this table (003's
-- sprint/title shape, which no code ever wrote) stayed empty. timeguru now
-- reads references/timeline.md at start and on change, writes one row per
-- milestone here (ReplaceMilestones: upsert every row, delete the rest), and
-- overlays rows changed through its API after the file's modification time.
--
-- Writer: timeguru as app_timeguru (pkg/database AppTimeguruConfig). Readers:
-- app_kanban, app_zhen and ops_reader (as 007 had it), SELECT only.
--
--   SELECT ... FROM timeline_milestones WHERE updated_at > $1   -- the overlay
--
-- a sequential scan of under a hundred rows. Volume: one row per milestone of
-- references/timeline.md, under a hundred. Retention: rows follow the file;
-- a milestone removed from it is deleted by the next load.
--
-- Lock plan: the old table is empty (the DO block refuses otherwise), so it
-- is dropped and created, an ACCESS EXCLUSIVE lock on an empty table bounded
-- by lock_timeout. Rollback: a later numbered migration; timeguru then waits,
-- read-only, for a table it can write. Proof: scripts/well/test-migrate.sh
-- (Step 321).

SET lock_timeout = '5s';

DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM timeline_milestones) THEN
    RAISE EXCEPTION 'timeline_milestones is not empty: read it before 050 replaces it';
  END IF;
END $$;

DROP TABLE timeline_milestones; -- well:ignore destructive the DO block above refuses unless the table is empty; 003's shape was never written by any code

CREATE TABLE timeline_milestones (
    milestone_key  VARCHAR(64) PRIMARY KEY CHECK (milestone_key <> ''),
    name           TEXT NOT NULL CHECK (name <> '' AND length(name) <= 500),
    status         VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'blocked')),
    progress_pct   SMALLINT NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
    eta            TIMESTAMPTZ,
    owner          TEXT NOT NULL DEFAULT '',
    risk           VARCHAR(10) NOT NULL DEFAULT '' CHECK (risk IN ('', 'low', 'medium', 'high')),
    description    TEXT NOT NULL DEFAULT '',
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON timeline_milestones TO app_timeguru;
GRANT SELECT ON timeline_milestones TO app_kanban, app_zhen, ops_reader; -- ops_reader: as 007 granted the old table
