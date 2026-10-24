<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# fix-bad-meta

`runbook.yaml` is a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") that fails validate_runbook.py. metadata is missing keys and has a risk level that does not exist.

Fix it so the check passes, keeping what the runbook does. Do not change validate_runbook.py.
