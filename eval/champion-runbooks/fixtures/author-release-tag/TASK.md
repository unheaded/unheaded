<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-release-tag

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `release-tag` that creates an annotated git tag v${VERSION} in ~/tmp/unheaded (parameter VERSION, required) and then pushes the tag to origin. Follow the Champion rules: the runner must never push.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
