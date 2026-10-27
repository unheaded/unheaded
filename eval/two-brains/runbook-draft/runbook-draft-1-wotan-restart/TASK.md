<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook-draft-1-wotan-restart

Write a draft runbook named `wotan-restart` for restarting Wotan, the message bus, on west.

Wotan runs as the systemd unit `unheaded-wotan` and answers `GET http://127.0.0.1:18000/health`.
The runbook should: check first that the unit exists; record the health answer before the restart
(it may fail: record it, do not stop there); keep the last 50 journal lines of the unit for the
record; restart the unit with `sudo systemctl restart`; then, in a `verification:` section, confirm
the unit is active (`systemctl is-active`) and the health endpoint answers.
requires_sudo is true.
