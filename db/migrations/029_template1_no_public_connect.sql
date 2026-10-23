-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — PUBLIC connects nowhere, template1 included (ERDA 2.2, W-02),
-- cluster-level.
--
-- 025 revoked CONNECT and TEMPORARY from PUBLIC on every database that
-- accepts connections, but skipped templates: template1 kept the default
-- grants, and db/pg_hba.conf admits every database from the data network,
-- so any service role could log in to template1 and make temporary tables
-- there. Nothing needs to: CREATE DATABASE copies template1 without
-- connecting to it, and the superuser needs no grant.

REVOKE CONNECT, TEMPORARY ON DATABASE template1 FROM PUBLIC;
