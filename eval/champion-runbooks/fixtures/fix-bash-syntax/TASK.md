<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# fix-bash-syntax

`runbook.yaml` is a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") that fails validate_runbook.py. A shell command does not parse (bash -n rejects it).

Fix it so the check passes, keeping what the runbook does. Do not change validate_runbook.py.
