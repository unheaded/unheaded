-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — drop service_health_hourly (ERDA 5.5), unheaded_ops.
--
-- Stevie 2026-10-15: wire the health bridge for current state and
-- transitions; drop the hourly rollup. Latency and up/down over time are
-- time series and live in VictoriaMetrics; Postgres keeps the latest state
-- and the status changes (events). Nothing ever wrote this table (the
-- bridge was never fed). Refuses if it has rows.

DO $$
BEGIN
    IF to_regclass('public.service_health_hourly') IS NOT NULL
       AND EXISTS (SELECT 1 FROM service_health_hourly) THEN
        RAISE EXCEPTION 'service_health_hourly has rows: export them before dropping';
    END IF;
END $$;

DROP TABLE IF EXISTS service_health_hourly;
