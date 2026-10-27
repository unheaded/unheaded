<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook-draft-3-akira-band-check

Write a draft runbook named `akira-band-check`: check Akira, the cross-service health consensus
daemon, and report the consensus band.

Akira answers `GET http://127.0.0.1:19100/health` and returns every service's state as JSON at
`GET http://127.0.0.1:19100/api/v1/status`. The runbook checks health, prints the status, and fails
its check step when any service is CRITICAL or PANIC. Escalation is by hand: add an `action: manual`
step that tells the operator what to do on CRITICAL or PANIC. Akira runs detection-only here: the
runbook restarts nothing and never enables remediation.
risk is low; requires_sudo is false.
