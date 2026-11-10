-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — log_events, a second copy of the host journal rows for the
-- measured week (S-E1-LIVE Evening 16, Steps 277 and 279; ADR-117),
-- cluster-level and unheaded_ops.
--
-- Vector already ships every journal row to ClickHouse
-- (config/clickhouse/schema/001-logs.sql), which stays the record: 90 days,
-- 455 for audit rows. For one week the same rows, after the same redaction,
-- also go to cmd/logstore-writer, which inserts them here, so the two
-- stores can be compared on disk, query time and the load they put on the
-- Well. This copy is derived and rebuildable: 14 days, then dropped.
--
-- Applied twice (MANIFEST), like 042: in `postgres` it creates the login
-- role and grants CONNECT; in unheaded_ops it creates the table and the
-- maintenance function. Every statement sits in a DO block that checks the
-- database, so each application does only its half.
--
-- Shape (Erda pass, Step 277; numbers measured from ClickHouse 2026-11-10:
-- 54k to 81k rows a day, about 1.4 kB a row, 76 MB a day, 1.1 GB at 14 days):
--   log_events           partitioned by RANGE (ts), one partition per UTC
--                        day, named log_events_pYYYYMMDD
--   columns              the ClickHouse columns, same names except ts;
--                        fields stays the JSON text Vector built, as
--                        ClickHouse holds it (jsonb and a GIN index are a
--                        later migration if Postgres stays)
--   PRIMARY KEY          (host, ts, entry): the journal entry once per host,
--                        so a batch Vector sends twice inserts once
--                        (ON CONFLICT DO NOTHING)
--   no other index       access paths, measured on one day of real rows:
--                          last hour by host   primary key, backward   5.5 ms
--                          count by unit, 7 d  partition scans         17 ms/day
--                          text match, 24 h    one partition, seq     565 ms
--   no default partition a row outside the window is refused by the writer
--                        and counted, never parked
--   no append-only trigger  retention drops whole partitions; the writer
--                        holds INSERT only (rows are never updated or
--                        deleted one by one)
--
-- log_events_maintain(): SECURITY DEFINER, owned by well_ops_owner, no
-- arguments. Creates the partitions for 13 days back to 2 days ahead (UTC),
-- drops log_events_pYYYYMMDD older than 13 days back, returns the bytes the
-- remaining partitions hold. The writer calls it at start and every five
-- minutes and refuses rows while the return is over its ceiling. The
-- migration creates no partition itself, so a fresh Well has the same
-- objects on any day; the table accepts rows after the first call.
--
-- Roles: owner well_ops_owner. log_writer: INSERT on log_events and EXECUTE
-- on log_events_maintain(), nothing else (no SELECT: it cannot read the
-- rows back). ops_reader: SELECT. No password here: db/init.sh and
-- scripts/well/set-role-passwords.sh set it from LOG_WRITER_PASSWORD.
-- Connection limit 4: one writer, a pool of three.
--
-- Lock plan: a new role, a new empty table, a new function; no lock on
-- anything that exists. At run time log_events_maintain() takes ACCESS
-- EXCLUSIVE on log_events to add or drop a partition, under lock_timeout
-- 2s: behind a running pg_dump it fails and the writer tries again five
-- minutes later (two days of partitions exist ahead).
-- Rollback (off switch, plan Step 281): stop the writer, then a later
-- numbered migration drops the function, the table and the role.
-- Proof: scripts/well/test-log-events.sh (throwaway server).

SET lock_timeout = '5s';

DO $$
BEGIN
    IF current_database() = 'postgres' THEN
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'log_writer') THEN
            CREATE ROLE log_writer LOGIN CONNECTION LIMIT 4;
        END IF;
        GRANT CONNECT ON DATABASE unheaded_ops TO log_writer;
    END IF;
END $$;

DO $$
BEGIN
    IF current_database() = 'unheaded_ops' THEN
        CREATE TABLE log_events (
            ts        TIMESTAMPTZ NOT NULL,
            host      TEXT NOT NULL CHECK (host <> '' AND length(host) <= 253),
            source    TEXT NOT NULL CHECK (length(source) <= 32),
            unit      TEXT NOT NULL CHECK (length(unit) <= 256),
            level     TEXT NOT NULL CHECK (length(level) <= 16),
            retention TEXT NOT NULL CHECK (retention IN ('audit', 'operational')),
            message   TEXT NOT NULL CHECK (length(message) <= 262144),
            entry     TEXT NOT NULL CHECK (entry <> '' AND length(entry) <= 128),
            fields    TEXT NOT NULL CHECK (length(fields) <= 262144),
            PRIMARY KEY (host, ts, entry)
        ) PARTITION BY RANGE (ts);

        CREATE FUNCTION log_events_maintain() RETURNS BIGINT
            LANGUAGE plpgsql SECURITY DEFINER
            SET search_path = pg_catalog, public
            SET lock_timeout = '2s'
        AS $fn$
        DECLARE
            today DATE := (now() AT TIME ZONE 'UTC')::date;
            d     DATE;
            part  TEXT;
            r     RECORD;
            total BIGINT := 0;
        BEGIN
            FOR d IN SELECT g::date FROM generate_series(today - 13, today + 2, INTERVAL '1 day') AS g LOOP
                part := 'log_events_p' || to_char(d, 'YYYYMMDD');
                IF to_regclass('public.' || part) IS NULL THEN
                    EXECUTE format(
                        'CREATE TABLE public.%I PARTITION OF public.log_events FOR VALUES FROM (%L) TO (%L)',
                        part, d::text || ' 00:00:00+00', (d + 1)::text || ' 00:00:00+00');
                END IF;
            END LOOP;
            FOR r IN
                SELECT c.oid, c.relname
                  FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
                 WHERE i.inhparent = 'public.log_events'::regclass
                 ORDER BY c.relname
            LOOP
                IF r.relname ~ '^log_events_p[0-9]{8}$'
                   AND to_date(substr(r.relname, 13), 'YYYYMMDD') < today - 13 THEN
                    EXECUTE format('DROP TABLE public.%I', r.relname);
                ELSE
                    total := total + pg_total_relation_size(r.oid);
                END IF;
            END LOOP;
            RETURN total;
        END $fn$;

        REVOKE ALL ON FUNCTION log_events_maintain() FROM PUBLIC;
        GRANT EXECUTE ON FUNCTION log_events_maintain() TO log_writer;
        GRANT INSERT ON log_events TO log_writer;
        GRANT SELECT ON log_events TO ops_reader;
    END IF;
END $$;
