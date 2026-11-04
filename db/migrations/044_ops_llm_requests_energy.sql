-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — estimated energy per LLM request (ADR-120), unheaded_ops.
--
-- 030 stays as it is. Two new columns on llm_requests:
--   energy_joules_est  NULL means no estimate; never 0 for unknown;
--   energy_method      '' with no estimate; gpu_sampled is pkg/energy's
--                      integration of hwmon power1_average above an idle
--                      read; gpu_cpu_sampled and published_figure are
--                      reserved by ADR-120. Claude rows stay NULL and ''.
-- llm_requests_energy_shape ties the two: a method needs a figure and a
-- figure needs a method, so "unknown" cannot be written as 0.
--
-- Writer: ops_writer, through 030's table grant (no new grant). Reader:
-- ops_reader. Access paths: 030's time range scans; nothing new is
-- indexed. Volume: 030's, plus 8 bytes and a short string per row.
-- Retention: unchanged. Append-only by 030's triggers: existing rows are
-- not touched, and need not be, both defaults being the "no estimate"
-- values.
--
-- Lock plan: ADD COLUMN with a constant default is a catalog update
-- under a short ACCESS EXCLUSIVE, bounded by lock_timeout. The CHECKs are
-- added NOT VALID and validated in a second statement (SHARE UPDATE
-- EXCLUSIVE while it scans). Rollback: a later numbered migration drops
-- the columns; writers must first return to the 030 column list.
-- Applied live with the E1 batch. Proof: scripts/well/test-llm-requests.sh
-- (step 5b).

SET lock_timeout = '5s';

ALTER TABLE llm_requests ADD COLUMN IF NOT EXISTS energy_joules_est DOUBLE PRECISION;
ALTER TABLE llm_requests ADD COLUMN IF NOT EXISTS energy_method TEXT NOT NULL DEFAULT '';

ALTER TABLE llm_requests ADD CONSTRAINT llm_requests_energy_method_check
    CHECK (energy_method IN ('', 'gpu_sampled', 'gpu_cpu_sampled', 'published_figure')) NOT VALID;
ALTER TABLE llm_requests VALIDATE CONSTRAINT llm_requests_energy_method_check;

ALTER TABLE llm_requests ADD CONSTRAINT llm_requests_energy_shape
    CHECK ((energy_joules_est IS NULL) = (energy_method = '')
           AND (energy_joules_est IS NULL OR energy_joules_est >= 0)) NOT VALID;
ALTER TABLE llm_requests VALIDATE CONSTRAINT llm_requests_energy_shape;
