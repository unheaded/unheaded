-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — hand this database's objects to its owner role (ERDA 2.2),
-- per database (app, ops, config). Runs as the superuser (MANIFEST flag
-- `superuser`): only an object's current owner can give it away.
--
-- Tables, views and free-standing sequences in public, and the functions
-- there, go to the database's owner (025). Sequences a table owns follow
-- their table. Left with the superuser: the ledger (well_schema_migrations)
-- and well_append_only(), which LEDGER.sql re-creates as the superuser on
-- every run. Grants are unchanged; their grantor becomes the new owner.

DO $$
DECLARE
    owner text;
    r record;
BEGIN
    SELECT pg_get_userbyid(datdba) INTO owner FROM pg_database WHERE datname = current_database();
    IF owner IS NULL OR owner !~ '^well_[a-z]+_owner$' THEN
        RAISE EXCEPTION '026: % is owned by %, not a well_*_owner role (run 025 first)', current_database(), owner;
    END IF;
    FOR r IN
        SELECT c.relname, c.relkind FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p', 'v', 'm', 'S')
          AND c.relname <> 'well_schema_migrations'
          AND NOT (c.relkind = 'S' AND EXISTS (
                SELECT 1 FROM pg_depend dep WHERE dep.classid = 'pg_class'::regclass
                  AND dep.objid = c.oid AND dep.deptype IN ('a', 'i')))
        ORDER BY c.relname
    LOOP
        EXECUTE format('ALTER %s %I OWNER TO %I',
            CASE r.relkind WHEN 'S' THEN 'SEQUENCE' WHEN 'v' THEN 'VIEW' WHEN 'm' THEN 'MATERIALIZED VIEW' ELSE 'TABLE' END,
            r.relname, owner);
    END LOOP;
    FOR r IN
        SELECT p.oid::regprocedure AS sig FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
        WHERE n.nspname = 'public' AND p.proname <> 'well_append_only'
    LOOP
        EXECUTE format('ALTER FUNCTION %s OWNER TO %I', r.sig, owner);
    END LOOP;
END $$;
