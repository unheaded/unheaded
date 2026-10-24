<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-cert-expiry-report

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `cert-expiry-report` that lists every certificate /etc/unheaded/certs/*.pem with its end date (openssl x509 -enddate) and warns for any that expires within 30 days, using openssl x509 -checkend 2592000.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
