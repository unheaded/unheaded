# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/security/git-history-secret-purge.yaml (R6).

A sandbox repo with a synthetic secret (made at test time, never committed
to this repo) planted in 3 commits and a tag, and a local bare "origin".
The runbook's clone, scan, rewrite and verify steps run against it with
ORIGIN/MIRROR/REPLACEMENTS/PATTERNS pointed at temp paths; the manual
force-with-lease push is then run as its step describes, against the
sandbox origin only. A stale lease (origin moved) must refuse."""
import os
import random
import string
import subprocess
from pathlib import Path

import pytest

import harness

RB = "runbooks/security/git-history-secret-purge.yaml"
GITLEAKS = Path.home() / ".local/bin/gitleaks"
FILTER_REPO = Path.home() / ".venv/zhen/bin/git-filter-repo"
needs_tools = pytest.mark.skipif(not (GITLEAKS.exists() and FILTER_REPO.exists()),
                                 reason="needs ~/.local/bin/gitleaks and git-filter-repo in ~/.venv/zhen (operator tools)")


def synthetic_key():
    # gitleaks' aws-access-token rule: AKIA + 16 of [A-Z2-7]
    rnd = random.SystemRandom()
    return "AKIA" + "".join(rnd.choice(string.ascii_uppercase + "234567") for _ in range(16))


@pytest.fixture
def leaked(sandbox_git_repo, tmp_path):
    r = sandbox_git_repo
    secret = synthetic_key()
    for i in range(3):
        (r.work / f"conf{i}.ini").write_text(f"[aws]\naws_access_key_id = {secret}\nnote = {i}\n")
        harness.git(r.work, "add", f"conf{i}.ini", env=r.env)
        harness.git(r.work, "commit", "-q", "-m", f"config {i}", env=r.env)
        if i == 1:
            harness.git(r.work, "tag", "v-leak", env=r.env)
    harness.git(r.work, "push", "-q", "origin", "develop", "--tags", env=r.env)
    purge = tmp_path / "purge"
    purge.mkdir()
    repl = purge / "replacements.txt"
    repl.write_text(f"{secret}==>***REMOVED***\n")
    pats = purge / "patterns.txt"
    pats.write_text(secret + "\n")
    for f in (repl, pats):
        f.chmod(0o600)
    env = {**r.env, "ORIGIN": str(r.origin), "MIRROR": str(purge / "repo.git"), "REPLACEMENTS": str(repl),
           "PATTERNS": str(pats), "BRANCH": "develop"}
    return r, env, secret


def gitleaks(repo):
    return subprocess.run([str(GITLEAKS), "git", "--redact", "--no-banner", "--log-opts=--all", str(repo)],
                          capture_output=True, text=True, timeout=120, check=False).returncode


def tips(env):
    out = {}
    for line in Path(env["MIRROR"] + ".tips").read_text().splitlines():
        ref, sha = line.split()
        out[ref] = sha
    return out


def lease_push(env, ref, old):
    return subprocess.run(["git", "-C", env["MIRROR"], "push", f"--force-with-lease={ref}:{old}", "origin", f"{ref}:{ref}"],
                          capture_output=True, text=True, timeout=60, env={**os.environ, **env}, check=False)


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_rotation_first_and_every_push_manual():
    st = harness.steps(harness.load(RB))
    assert st[0]["action"] == "manual" and "rotat" in st[0]["name"]
    for s in st:
        c = s.get("command") or ""
        if "push" in c.split() or "--force" in c or "shred" in c or "gc --prune" in c:
            assert s["action"] == "manual" and s.get("irreversible") is True, s["name"]
    for s in st:
        if "gitleaks git" in (s.get("command") or ""):
            assert "--redact" in s["command"], f"{s['name']} would print secrets"


@needs_tools
def test_purge_in_a_mirror_then_lease_push(leaked):
    r, env, secret = leaked
    rb = harness.load(RB)
    assert gitleaks(r.origin) == 1, "the planted secret is not found before the purge"
    for name in ("clone-mirror", "record-tips", "scan-before", "check-replacements-file", "rewrite-history",
                 "verify-rewrite", "check-gitleaks"):
        harness.run_step(rb, name, env=env, cwd=harness.REPO)
    assert gitleaks(env["MIRROR"]) == 0
    # the manual push, as push-changes describes it: one lease per ref, old sha from the tips file
    for ref, old in tips(env).items():
        if ref.startswith(("refs/heads/", "refs/tags/")):
            p = lease_push(env, ref, old)
            assert p.returncode == 0, p.stderr
    assert gitleaks(r.origin) == 0, "the secret is still reachable in origin"
    log = subprocess.run(["git", "-C", str(r.origin), "log", "--all", "-p"], capture_output=True, text=True, check=True).stdout
    assert secret not in log


@needs_tools
def test_stale_lease_refuses(leaked):
    r, env, _ = leaked
    rb = harness.load(RB)
    for name in ("clone-mirror", "record-tips", "rewrite-history"):
        harness.run_step(rb, name, env=env, cwd=harness.REPO)
    # someone pushes to origin after the tips were recorded
    (r.work / "late.txt").write_text("late\n")
    harness.git(r.work, "add", "late.txt", env=r.env)
    harness.git(r.work, "commit", "-q", "-m", "late", env=r.env)
    harness.git(r.work, "push", "-q", "origin", "develop", env=r.env)
    p = lease_push(env, "refs/heads/develop", tips(env)["refs/heads/develop"])
    assert p.returncode != 0 and ("stale info" in p.stderr or "rejected" in p.stderr), p.stderr


def test_replacement_files_inside_a_repo_are_refused(sandbox_git_repo):
    rb = harness.load(RB)
    w = sandbox_git_repo.work
    for n in ("r.txt", "p.txt"):
        (w / n).write_text("x==>y\n")
        (w / n).chmod(0o600)
    r = harness.run_step(rb, "check-replacements-file", env={"REPLACEMENTS": str(w / "r.txt"), "PATTERNS": str(w / "p.txt")},
                         check=False)
    assert r.returncode != 0
