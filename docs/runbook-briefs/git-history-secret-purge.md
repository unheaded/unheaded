<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# Brief: git-history-secret-purge (runbooks/security/git-history-secret-purge.yaml)

**Scenario.** A secret (key, token, password) or personal data was committed
and is in git history, possibly already pushed. Remove it from every ref and
tag, and from every copy that keeps history.

**Owner** blackmage. **Risk** critical. **requires_sudo** false. Duration about 2h.

**Rotate first.** History outlives every deletion (CLAUDE.md, "Secrets
Management"): the first step is manual: rotate or revoke the leaked
credential at its issuer, before any rewrite. A rewrite never makes a leaked
secret safe again; rotation does.

**Facts:**
- Scanner: `gitleaks` 8.x at `~/.local/bin/gitleaks`. Always `--redact`
  (it prints secrets otherwise): `gitleaks git --redact --no-banner
  --log-opts="--all" "$MIRROR"`; exit 1 means findings.
- Rewriter: `git filter-repo`, not installed by default; an operator tool,
  install it into the Zhen venv: `~/.venv/zhen/bin/pip install git-filter-repo`.
  Use `--replace-text "$REPLACEMENTS"`, a file of `literal==>***REMOVED***`
  lines. That file holds the secret: keep it outside the repo, mode 0600,
  `shred -u` it at the end (manual).
- Work in a fresh mirror clone, never the working checkout:
  `git clone --mirror "$ORIGIN" "$MIRROR"`. Parameters: `ORIGIN` (no
  default), `MIRROR` default `$HOME/purge/repo.git`, `REPLACEMENTS` default
  `$HOME/purge/replacements.txt`, `BRANCH` default `develop`.
- Record each branch's current remote tip before rewriting (for the lease):
  `git -C "$MIRROR" for-each-ref --format='%(refname) %(objectname)' refs/heads refs/tags > "$MIRROR.tips"`.
- Verify after rewrite: gitleaks finds 0 over all refs and tags, and
  `git -C "$MIRROR" log --all -p | grep -c -F -f <patterns>` is 0 (patterns
  file outside the repo).
- Cleanup in the mirror only after verification: delete `refs/original`
  and any `refs/backup/*`, then `git reflog expire --expire=now --all &&
  git gc --prune=now --aggressive` (manual, irreversible).
- Push (manual, irreversible, **Stevie only**):
  `git -C "$MIRROR" push --force-with-lease="$BRANCH:<old sha>" origin "$BRANCH"`
  per branch, with the old sha from the tips file; a moved remote makes the
  push refuse, which is the point. Tags the same way.
- GitHub keeps old commits reachable by hash and in pull-request refs:
  ask GitHub Support to purge cached views (manual, Stevie).
- Other copies: every clone must be re-cloned; the wiki mirror too; east's
  git mirror (`scripts/backup/git-mirror.sh`) never prunes and refuses
  deletions, so the old commits stay there until purged on east by hand.
- `.gitleaksignore` may only shrink (`scripts/check-secrets-baseline.sh`):
  never baseline the leaked secret.
- Precedent: the orb repo rewrite of 2026-10-23 (two passes, 34 commits
  rewritten, pushed with `--force-with-lease=master:<old sha>`).

**Rollback:** before the push, nothing changed anywhere but the mirror
(delete it). After the push, the old history is gone from GitHub; the tips
file and east's mirror hold the old tips (that is also why east needs purging).
