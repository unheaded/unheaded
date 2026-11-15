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
-- app_zhen's connection limit is 20 (D6; REV-5, independent review
-- 2026-11-15: the plan's 12 counted one zhen-agentd and zhen-claude's 2).
-- A role's limit counts its connections in every database. Clients that
-- can run at once, at their caps:
--   zhen-agentd daemon          6 (Zhen pool) + 2 (board pool, -worker only)  8
--   zhen-agentd -overnight      2 + 1 (cmd/zhen-agentd/pools.go)              3
--   zhen-claude review          2 (runs) + 2 (cards)                          4
--   a second zhen-claude        confirm, or a session with -confirm queue     2
--   raft/zhen_app.py            Zhen's rows + the board's reads                2
--   an operator's psql                                                         1
--                                                                    total   20
-- A cap, not a share (ADR-147 decision 13). cmd/zhen-agentd TestPoolCaps
-- fails when the sum outgrows this line.

-- Provisioning stays in the ledger on purpose: the Well makes every database
-- and role here, in `postgres` (002 made unheaded_app, unheaded_ops and
-- unheaded_config; 025 their NOLOGIN owners; 012 huginn_reader), and both
-- db/init.sh and well-migrate apply it from the MANIFEST on all four
-- deployment targets. No password: app_zhen exists since 002 and gets its
-- password from db/init.sh or scripts/well/set-role-passwords.sh.
CREATE ROLE well_zhen_owner NOLOGIN; -- well:ignore provisioning-in-ledger the Well's databases and owner roles are made by the ledger in postgres (002, 012, 025), not by a separate provisioning path; this follows them
CREATE DATABASE unheaded_zhen OWNER well_zhen_owner;
REVOKE CONNECT, TEMPORARY ON DATABASE unheaded_zhen FROM PUBLIC;
GRANT CONNECT ON DATABASE unheaded_zhen TO app_zhen, app_kanban, retention_runner, well_backup, huginn_reader;
ALTER ROLE app_zhen CONNECTION LIMIT 20;
