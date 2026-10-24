<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-well-dead-tuples

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `well-dead-tuples` that reports the ten tables of database unheaded_app with the most dead tuples (n_dead_tup from pg_stat_user_tables) with psql. It is read-only: no VACUUM FULL anywhere. risk is low.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
