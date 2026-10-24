<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# author-gpu-lease-report

Write `runbook.yaml`: a strict draft runbook (runbook/v1, kind Runbook; metadata name, description, owner, estimated_duration, risk, requires_sudo, status: draft, authored_by: zhen-local, reviewed_by: "", approved_by: "") named `gpu-lease-report` that shows which process holds the GPU lease file $XDG_RUNTIME_DIR/unheaded/gpu.lock (fuser or lsof) and the VRAM use from rocm-smi. Read-only: it must not kill anything or remove the lock.

Every shell command and verify must pass `bash -n`; a multi-line one starts with `set -euo pipefail`. Irreversible commands (push, pg_restore, rm -rf, ...) go in `action: manual` steps. No credentials in the file. validate_runbook.py checks the shape.
