<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-zhen-index-swap

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `zhen-index-swap` that switches /var/zhen/index/active.index to a new index file (parameter NEW_INDEX, required): a precondition checks with test -f that NEW_INDEX exists, the switch uses ln -sfn, a verify reads the link back with readlink, and a rollback entry points the link back to the previous index (parameter OLD_INDEX) with ln -sfn.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
