#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Re-sign the commits after BASE on the current branch.

Each commit is recreated with the same tree, author, committer and dates,
on the rewritten parent, signed (or not, with --no-sign). 8-hex references
in messages to commits already rewritten in this run are replaced with
their new abbreviated hashes. The branch is moved only if it still points
at the old head (compare-and-swap); a backup ref keeps the old head.

usage: resign-unpushed.py REPO BASE [--no-sign] [--committer-name NAME]

Use when commits were made unsigned (gpg-agent timed out) and the key is
now cached. BASE is the last pushed commit (e.g. origin/develop). Check
afterwards: git log --format=%G? BASE..HEAD shows G for every commit, and
HEAD^{tree} equals the backup ref's tree.
"""
import os
import re
import subprocess
import sys

repo, base = sys.argv[1], sys.argv[2]
sign = "--no-sign" not in sys.argv
committer_override = None
if "--committer-name" in sys.argv:
    committer_override = sys.argv[sys.argv.index("--committer-name") + 1]


def git(*args, env=None, input=None):
    return subprocess.run(["git", "-C", repo, *args], check=True, capture_output=True,
                          text=True, env=env, input=input).stdout


branch = git("symbolic-ref", "--short", "HEAD").strip()
old_head = git("rev-parse", "HEAD").strip()
olds = git("rev-list", "--reverse", "--topo-order", f"{base}..HEAD").split()
if git("rev-list", "--merges", f"{base}..HEAD").strip():
    sys.exit("merges in range; refusing")

mapping = {}  # old full sha -> new full sha
hex8 = re.compile(r"\b[0-9a-f]{8}\b")

for old in olds:
    fields = git("show", "-s", "--format=%T%n%P%n%an%n%ae%n%ad%n%cn%n%ce%n%cd", "--date=raw", old).split("\n")
    tree, parents, an, ae, ad, cn, ce, cd = fields[:8]
    msg = git("show", "-s", "--format=%B", old)
    msg = msg[:-1] if msg.endswith("\n\n") else msg  # %B adds one trailing newline

    def repl(m):
        for o, n in mapping.items():
            if o.startswith(m.group(0)):
                return n[:8]
        return m.group(0)

    msg = hex8.sub(repl, msg)
    new_parents = [mapping.get(p, p) for p in parents.split()]
    env = dict(os.environ, GIT_AUTHOR_NAME=an, GIT_AUTHOR_EMAIL=ae, GIT_AUTHOR_DATE=ad,
               GIT_COMMITTER_NAME=committer_override or cn, GIT_COMMITTER_EMAIL=ce,
               GIT_COMMITTER_DATE=cd)
    args = ["commit-tree", tree]
    for p in new_parents:
        args += ["-p", p]
    args.append("-S" if sign else "--no-gpg-sign")
    new = git(*args, env=env, input=msg).strip()
    mapping[old] = new
    print(f"{old[:8]} -> {new[:8]}  {msg.splitlines()[0][:70]}", flush=True)

new_head = mapping[olds[-1]]
git("update-ref", f"refs/backup/{branch}-before-resign", old_head)
git("update-ref", f"refs/heads/{branch}", new_head, old_head)  # CAS
with open(os.path.join(repo, ".git", "resign-map.txt"), "w") as f:
    f.writelines(f"{o} {mapping[o]}\n" for o in olds)
print(f"{branch}: {old_head[:8]} -> {new_head[:8]} ({len(olds)} commits); backup refs/backup/{branch}-before-resign")
