-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Zhen's own database, cluster-level (ADR-149, ADR-147 decision 15
-- (b); S-ZHEN-SWITCH D2 (a), D6).
--
-- Zhen is an optional module: its tables leave unheaded_app (052 builds
-- them here, 053 drops them there once the rows are copied) so the platform
-- holds no Zhen data, and Zhen's data has its own owner, backup set and
-- connection limit. notx: CREATE DATABASE cannot run in a transaction, so
-- this runs statement by statement in `postgres`.
--
-- Like 002, 025 and 029 for unheaded_app: a NOLOGIN owner that nobody logs
-- in as, PUBLIC connects nowhere, and each role connects only where it has
-- a grant:
--   app_zhen          Zhen's own tables
--   app_kanban        the board's card modal reads a card's newest run
--                     (041's columns), only while the module is on
--   retention_runner  the Zhen classes of cmd/well-retention (042)
--   well_backup       Zhen's own backup set (pg_read_all_data, 023)
--   huginn_reader     pg_monitor's statistics (012)
--
-- app_zhen's connection limit is 12 (D6): zhen-agentd's two pools cap at
-- 6 + 2, zhen-claude's at 2, plus 2. A cap, not a share (ADR-147 decision 13).

CREATE ROLE well_zhen_owner NOLOGIN;
CREATE DATABASE unheaded_zhen OWNER well_zhen_owner;
REVOKE CONNECT, TEMPORARY ON DATABASE unheaded_zhen FROM PUBLIC;
GRANT CONNECT ON DATABASE unheaded_zhen TO app_zhen, app_kanban, retention_runner, well_backup, huginn_reader;
ALTER ROLE app_zhen CONNECTION LIMIT 12;
