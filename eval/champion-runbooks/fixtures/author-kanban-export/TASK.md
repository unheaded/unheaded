<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-kanban-export

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `kanban-export` that exports the Kanban tasks from http://localhost:20001/api/v1/tasks to a dated JSON file under $HOME/kanban-exports, and has a verify that the file parses (python3 -m json.tool or jq).

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
