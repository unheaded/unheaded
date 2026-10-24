<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-wotan-health-restart

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `wotan-health-restart` that checks Wotan's health on http://localhost:18000/health, restarts the systemd unit unheaded-wotan with sudo systemctl, and verifies health again afterwards. requires_sudo is true.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
