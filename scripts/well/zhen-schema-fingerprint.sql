-- SPDX-License-Identifier: GPL-3.0-or-later
-- The shape of Zhen's objects in the current database, one sorted row per
-- fact: kind|name|definition (ADR-149; S-ZHEN-SWITCH Steps 191, 279).
-- Run it in unheaded_app before 053 and in unheaded_zhen after 052: the two
-- outputs differ by the lines scripts/well/zhen-schema-changes.txt names
-- (scripts/well/zhen_schema_diff.py compares them) and by no other.
-- Owners are left out on purpose (well_app_owner there, well_zhen_owner here).
--   psql -X -At -d <db> -f scripts/well/zhen-schema-fingerprint.sql
WITH tabs AS (
    SELECT c.oid, c.relname
      FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relname LIKE 'zhen\_%'
),
trigfuncs AS (
    SELECT DISTINCT t.tgfoid AS oid
      FROM pg_trigger t JOIN tabs ON tabs.oid = t.tgrelid
     WHERE NOT t.tgisinternal
    UNION
    SELECT p.oid FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
     WHERE n.nspname = 'public' AND p.proname LIKE 'zhen\_%'
),
facts AS (
    SELECT 'column' AS kind,
           tabs.relname || '.' || a.attname AS name,
           format_type(a.atttypid, a.atttypmod)
             || CASE WHEN a.attnotnull THEN ' NOT NULL' ELSE '' END
             || coalesce(' DEFAULT ' || pg_get_expr(d.adbin, d.adrelid), '')
             || CASE a.attgenerated WHEN 's' THEN ' GENERATED STORED' ELSE '' END
             || CASE WHEN a.attidentity <> '' THEN ' IDENTITY ' || a.attidentity::text ELSE '' END AS definition
      FROM tabs JOIN pg_attribute a ON a.attrelid = tabs.oid AND a.attnum > 0 AND NOT a.attisdropped
      LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
    UNION ALL
    SELECT 'constraint', tabs.relname || '.' || co.conname, pg_get_constraintdef(co.oid)
      FROM tabs JOIN pg_constraint co ON co.conrelid = tabs.oid
    UNION ALL
    SELECT 'index', i.indexrelid::regclass::text, pg_get_indexdef(i.indexrelid)
      FROM tabs JOIN pg_index i ON i.indrelid = tabs.oid
    UNION ALL
    SELECT 'trigger', tabs.relname || '.' || t.tgname, pg_get_triggerdef(t.oid)
      FROM tabs JOIN pg_trigger t ON t.tgrelid = tabs.oid
     WHERE NOT t.tgisinternal
    UNION ALL
    SELECT 'function', p.oid::regprocedure::text,
           regexp_replace(pg_get_functiondef(p.oid), '\s+', ' ', 'g')
      FROM trigfuncs JOIN pg_proc p ON p.oid = trigfuncs.oid
    UNION ALL
    -- One row per privilege, the privilege in the name: zhen-schema-changes.txt
    -- lists a difference by kind|name, so it names the one privilege that
    -- differs and never a grantee's whole set on a table.
    SELECT 'grant', g.table_name || ' ' || g.grantee || ' ' || g.privilege_type, 'granted'
      FROM information_schema.role_table_grants g
     WHERE g.table_schema = 'public' AND g.table_name LIKE 'zhen\_%'
       AND g.grantee IN ('app_zhen', 'app_kanban', 'retention_runner')
    UNION ALL
    SELECT 'column-grant', c.table_name || '.' || c.column_name || ' ' || c.grantee || ' ' || c.privilege_type, 'granted'
      FROM information_schema.column_privileges c
     WHERE c.table_schema = 'public' AND c.table_name LIKE 'zhen\_%'
       AND c.grantee IN ('app_zhen', 'app_kanban', 'retention_runner')
       -- A table-wide grant shows up on every column too; keep only the
       -- column-level ones.
       AND NOT EXISTS (SELECT 1 FROM information_schema.role_table_grants g
                        WHERE g.table_schema = c.table_schema AND g.table_name = c.table_name
                          AND g.grantee = c.grantee AND g.privilege_type = c.privilege_type)
    UNION ALL
    SELECT 'function-grant', p.oid::regprocedure::text || ' ' || r.rolname, 'EXECUTE'
      FROM trigfuncs JOIN pg_proc p ON p.oid = trigfuncs.oid
      JOIN pg_roles r ON r.rolname IN ('app_zhen', 'app_kanban', 'retention_runner')
     WHERE has_function_privilege(r.oid, p.oid, 'EXECUTE')
       AND p.proacl IS NOT NULL
)
SELECT kind || '|' || name || '|' || definition FROM facts ORDER BY 1;
