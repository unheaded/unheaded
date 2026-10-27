<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook-draft-2-well-vacuum-report

Write a draft runbook named `well-vacuum-report`: a read-only report on table bloat in the Well
(PostgreSQL 16).

Connect with `psql` using the connection string in the environment variable `WELL_OPS_DSN`; the
runbook must fail with a clear message when it is unset (`${WELL_OPS_DSN:?...}`) and must never
contain a password. Report, from `pg_stat_user_tables`, the 20 tables with the most dead tuples
(`n_dead_tup`) with their live tuples, `last_autovacuum` and `last_autoanalyze`, and, as a second
step, the tables never autovacuumed. The report changes nothing: no VACUUM of any kind, no DDL.
risk is low; requires_sudo is false.
