<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: branch-promotion-develop-staging-main (runbooks/deploy/branch-promotion-develop-staging-main.yaml)

**Scenario.** Promote a reviewed batch: `develop` → `staging`, later
`staging` → `main` (ADR-089, `docs/adr/ADR-089-dev-staging-main-promotion-workflow.md`).

**Owner** micromanager. **Risk** high. **requires_sudo** false. Duration about 45m.

**Who decides:** promotion to `staging` happens only on Stevie's explicit
word; `main` is promoted by Stevie only. The runbook's first step is a
manual confirmation that Stevie asked for this promotion. Everything that
leaves the machine (every push) is Stevie's.

**Facts:**
- Parameters: `REPO` default `$HOME/tmp/unheaded`, `FROM` default `develop`,
  `TO` default `staging`, `METAGATE` default `$HOME/.cache/unheaded-metagate`.
- CI must be green on the `FROM` head, read from the public Actions API
  (no `gh` on west): `curl -s "https://api.github.com/repos/unheaded/unheaded/actions/runs?branch=$FROM&per_page=50"`
  and compare each run's `head_sha` with `git -C "$REPO" rev-parse "origin/$FROM"`.
  Workflows in state `disabled_manually` do not count; list them with
  `curl -s https://api.github.com/repos/unheaded/unheaded/actions/workflows`
  (on 2026-10-23: ci.yml, ci-protocol.yml, docker.yml, ebpf.yml, release.yml).
  Every active workflow's latest run on that sha must be `success`.
- Every commit in the batch is signed: no line other than `G` in
  `git -C "$REPO" log --format=%G? "$TO..$FROM"`. Unsigned ones go through
  `runbooks/deploy/resign-unpushed-commits.yaml` first (before they are pushed).
- Quick meta-gate in a clean clone, never the working checkout:
  `git clone -q "$REPO" "$METAGATE/run-$(date +%s)"`, then in it
  `scripts/check-gates-can-fail.sh --quick`.
- Merge locally: `git -C "$REPO" switch "$TO"`,
  `git -C "$REPO" merge --no-ff --no-edit "$FROM"`, so the merge's second
  parent is the `FROM` head (`git rev-parse "$TO^2"` equals the old `FROM` head).
- Log entry: append one dated line to
  `docs/battle-plans/STAGING-PROMOTION-LOG.md`:
  `- YYYY-MM-DD: $FROM <short sha> -> $TO (<n> commits), CI green, meta-gate quick pass` and commit it.
- Push (manual, irreversible: true, Stevie): `git -C "$REPO" push origin "$TO"`.
- Switch back: `git -C "$REPO" switch "$FROM"`.

**Rollback:** before the push: `git -C "$REPO" reset --hard "origin/$TO"` on
`TO` (manual, irreversible locally). After the push: `git revert -m 1 <merge>`
on `TO`, and write down what the gate missed (ADR-089).
