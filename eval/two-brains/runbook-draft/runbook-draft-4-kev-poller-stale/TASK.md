<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook-draft-4-kev-poller-stale

Write a draft runbook named `kev-poller-stale`: diagnose and recover a stale CISA KEV poller.

kev-poller runs as the systemd unit `unheaded-kev-poller` and listens on 127.0.0.1:19120.
`GET /health` says the process is up; `GET /ready` answers 503 until a valid catalog is stored;
`GET /api/v1/kev` returns JSON with `last_error`, `next_pull` and `interval`. The runbook prints the
readiness answer and the `last_error`, keeps the last 100 journal lines of the unit, restarts the
unit with `sudo systemctl restart`, and confirms in a `verification:` section that `/ready` answers
200 again.
requires_sudo is true.
