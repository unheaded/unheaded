<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-well-restore-drill

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `well-restore-drill` that proves the newest Well backup set restores by running /usr/local/lib/unheaded/well/restore-drill.sh on the set named in /srv/backups/unheaded-well/LATEST, then leaves the real restore (pg_restore --clean into the affected database) to the operator.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
