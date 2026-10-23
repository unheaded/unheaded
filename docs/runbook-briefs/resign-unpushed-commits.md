<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: resign-unpushed-commits (runbooks/deploy/resign-unpushed-commits.yaml)

**Scenario.** Commits were made unsigned (`--no-gpg-sign`) while the GPG key
was not cached (Claude works while Stevie is away; the agent cannot unlock
the key). Before Stevie pushes, re-sign exactly the unpushed commits.

**Owner** warmonger. **Risk** medium. **requires_sudo** false. Duration about 10m.

**Facts:**
- Tool: `scripts/resign-unpushed.py REPO BASE [--no-sign] [--committer-name NAME]`.
  It recreates each commit after BASE on the current branch with the same
  tree, author, committer and dates, signed; rewrites 8-hex references in
  messages to commits it already rewrote; moves the branch only if it still
  points at the old head (compare-and-swap); keeps the old head at
  `refs/backup/<branch>-before-resign`; writes `.git/resign-map.txt`
  (old new per line). It refuses a range with merges.
- Stevie caches the key (about 8 h). Probe signing with
  `git commit-tree -S -m probe "$(git write-tree)"` (exit 0 = the key is
  usable); NOT `gpg --clearsign` (it can pass while commit signing fails).
- Parameters: `REPO` default `$HOME/tmp/unheaded`, `BRANCH` default `develop`,
  `BASE` default `origin/develop` (the last pushed commit).
- Read-only first: `git -C "$REPO" status --porcelain` empty (a dirty tree
  is not touched, but say so); `git -C "$REPO" rev-parse --abbrev-ref HEAD`
  equals `$BRANCH`; list what will be rewritten:
  `git -C "$REPO" log --format='%h %G? %s' "$BASE..$BRANCH"`.
- Re-sign: `python3 "$REPO/scripts/resign-unpushed.py" "$REPO" "$BASE"`.
  It rewrites hashes of the unpushed commits only; commits at or before BASE
  keep their hashes. Never re-sign pushed commits (BASE must be what the
  remote has).
- Verify: every commit in `$BASE..$BRANCH` shows `G`:
  `test -z "$(git -C "$REPO" log --format=%G? "$BASE..$BRANCH" | grep -v '^G$')"`,
  and the tree did not change:
  `test "$(git -C "$REPO" rev-parse HEAD^{tree})" = "$(git -C "$REPO" rev-parse "refs/backup/$BRANCH-before-resign^{tree}")"`.
- After re-signing, regenerate ADR-098's findings section, since its
  git-signatures evidence reads commit signatures:
  `go run ./cmd/compliance-evidence findings -adr docs/adr/ADR-098-findings-register-and-baseline-modes.md`
  (a separate commit).
- The push is Stevie's (manual, irreversible: true): `git -C "$REPO" push origin "$BRANCH"`.

**Rollback:** before the push: `git -C "$REPO" update-ref "refs/heads/$BRANCH" "refs/backup/$BRANCH-before-resign"`
(manual). After the push nothing to undo.
