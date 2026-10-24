<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# fix-negated-guard

`runbook.yaml` is a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") that fails validate_runbook.py. A guard uses `! grep` in the middle of a command, so set -e never stops on it. Keep the guard: the step must still fail when debug: true is set.

Fix it so the check passes, keeping what the runbook does. Do not change validate_runbook.py.
