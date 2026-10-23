<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# rb-missing-verify

The runbook fails the runbook/v1 shape check: one step has no `verify`
command. Add a sensible `verify` to that step (a read-only command that
exits 0 when the step worked). Do not change validate_runbook.py.
