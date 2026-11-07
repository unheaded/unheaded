-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — kanban_zhen_optin: what hands a card to Zhen (H4), unheaded_app.
--
-- owner = 'zhen' is a field anyone who can edit the board can set, and a card's
-- title and description are what the worker acts on. A row here says an
-- authenticated operator handed over this card, with this text: the worker takes
-- a card only while the two still hash to card_sha256 (zhenagent.CardSHA256), so
-- an edit needs a fresh opt-in. It is a table of its own so that no path that
-- saves a card, Wotan's tasks.* included, can write it.
--
-- Writer: app_kanban, from kanban-app's operator-only route alone. Reader:
-- app_zhen (the worker), SELECT only; app_kanban, for the card modal.
--
--   SELECT ... FROM kanban_tasks t JOIN kanban_zhen_optin o ON o.card_id = t.id
--
-- served by the primary key. Volume: a row per card handed over, a handful.
-- Retention: rows go with their card (ON DELETE CASCADE from kanban_tasks,
-- whose retention is the kanban-soft-deleted class).
--
-- Lock plan: a new table; the foreign key takes a short SHARE ROW EXCLUSIVE on
-- kanban_tasks, bounded by lock_timeout. Rollback: a later numbered migration
-- drops the table; the worker then finds no card to take. Proof:
-- scripts/well/test-zhen-agent-runs.sh.

SET lock_timeout = '5s';

CREATE TABLE kanban_zhen_optin (
    card_id     TEXT PRIMARY KEY REFERENCES kanban_tasks (id) ON DELETE CASCADE,
    -- well:ignore text-surrogate-pk kanban_tasks.id is TEXT (003); this row is that card's
    card_sha256 CHAR(64) NOT NULL CHECK (card_sha256 ~ '^[0-9a-f]{64}$'),
    opted_in_by TEXT NOT NULL CHECK (opted_in_by <> ''),
    opted_in_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE, DELETE ON kanban_zhen_optin TO app_kanban;
GRANT SELECT ON kanban_zhen_optin TO app_zhen;
