<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-compose-kanban-up

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `compose-kanban-up` that starts postgres first and kanban-app after it with docker compose up -d (in ~/tmp/unheaded), verifies http://localhost:20001/health, and has a rollback entry that stops kanban-app with docker compose stop (or down).

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
