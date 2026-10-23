<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# runbook/v1, as the local model must write it

What `scripts/check-runbooks.py` enforces on a new runbook (ADR-024,
S-TWO-BRAINS Phase 8). The brief next to this file is the only source of
facts: no command, path, port, unit or host that the brief does not name.

```yaml
apiVersion: runbook/v1
kind: Runbook
metadata:
  name: <the file name without .yaml>
  description: <one line>
  owner: <owner from the brief>
  estimated_duration: <e.g. 30m>
  risk: <low | medium | high | critical>
  requires_sudo: <true | false>
  status: draft
  authored_by: zhen-local
  reviewed_by: ""
  approved_by: ""
  dry_run_test: tests/runbooks/test_<name>.py
notes: |
  <why this runbook exists, what it does not cover, links>
parameters:          # values the operator may set in the environment
  - name: <UPPER_CASE>
    default: "<value>"
    description: <what it is>
preconditions:
  - check: "<read-only shell test>"
    description: <what it proves>
steps:
  - name: <kebab-case, unique>
    action: shell     # the runner executes it
    host: west        # or east; only when the step must run on that host
    command: |
      set -euo pipefail
      <commands>
    verify: "<shell test that exits 0 when the step worked>"
    on_fail: "<what to look at>"
    description: <one line>
  - name: <kebab-case>
    action: manual    # the operator does it by hand; the runner only prints it
    irreversible: true
    command: |
      <exact commands; <placeholders> allowed in manual steps only>
    description: "MANUAL. <why a human does this>"
rollback:
  - name: <kebab-case>
    action: manual
    command: |
      <how to undo>
```

Rules:

1. `action` is `shell` or `manual`. Every multi-line `shell` command or
   verify starts with `set -euo pipefail` (comment lines may come first).
2. Anything that cannot be taken back is `action: manual` with
   `irreversible: true`: `git push` (any kind), force-push, `pg_restore`
   into a real server, `DROP`, `rm -rf`, `shred`, key deletion, history
   rewrites outside a throwaway clone, Console or GitHub UI actions, a
   restart that loses in-memory state. Stevie alone pushes.
3. `shell` steps must parse with `bash -n`. Manual steps may hold prose and
   `<placeholders>`; shell steps may not.
4. Use `${NAME}` only for a declared parameter or `env:` key; give
   parameters defaults so the operator's environment can override them.
5. Never print a secret: no `echo`/`cat` of a key, token, password or
   private key file; name the variable or the file instead.
6. Read-only checks first, then changes, then verification, then the
   write-up. Every change has a `verify` or a following check step.
7. `host:` only `west` or `east`. No host IP addresses anywhere (loopback
   `127.0.0.1` is fine); a peer or host address is a parameter without a
   default, guarded with `: "${NAME:?set NAME}"` before use.
