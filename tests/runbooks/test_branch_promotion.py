# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/deploy/branch-promotion-develop-staging-main.yaml (R8).

Sandbox repos only (bare origin in the temp dir). The CI step (GitHub API)
and the meta-gate clone are not run here; the signature preflight, the merge,
the log entry and the switch back are, with REPO/FROM/TO pointed at the
sandbox. Every push stays manual."""
import os
import subprocess

import harness
import pytest

RB = "runbooks/deploy/branch-promotion-develop-staging-main.yaml"
LOG = "docs/battle-plans/STAGING-PROMOTION-LOG.md"


def env_for(repo):
    return {"REPO": str(repo), "FROM": "develop", "TO": "staging"}


@pytest.fixture
def signed_repo(tmp_path, git_env):
    gnupg = tmp_path / "g"
    gnupg.mkdir(mode=0o700)
    env = {**git_env, "GNUPGHOME": str(gnupg)}
    r = subprocess.run(["gpg", "--batch", "--passphrase", "", "--quick-gen-key", "Sandbox <sandbox@localhost>", "ed25519", "sign", "never"],
                       capture_output=True, text=True, env={**os.environ, **env}, timeout=60, check=False)
    if r.returncode != 0:
        pytest.skip(f"cannot make an ephemeral gpg key: {r.stderr[-200:]}")
    origin, work = tmp_path / "origin.git", tmp_path / "work"
    harness.git(tmp_path, "init", "-q", "--bare", "-b", "main", str(origin), env=env)
    harness.git(tmp_path, "init", "-q", "-b", "main", str(work), env=env)
    harness.git(work, "config", "commit.gpgsign", "true", env=env)
    harness.git(work, "config", "user.signingkey", "sandbox@localhost", env=env)
    (work / "docs/battle-plans").mkdir(parents=True)
    (work / LOG).write_text("# Staging promotion log\n")
    harness.git(work, "add", ".", env=env)
    harness.git(work, "commit", "-q", "-m", "base", env=env)
    harness.git(work, "remote", "add", "origin", str(origin), env=env)
    for b in ("staging", "develop"):
        harness.git(work, "branch", b, env=env)
    harness.git(work, "switch", "-q", "develop", env=env)
    for i in range(2):
        (work / f"f{i}").write_text(f"{i}\n")
        harness.git(work, "add", f"f{i}", env=env)
        harness.git(work, "commit", "-q", "-m", f"work {i}", env=env)
    harness.git(work, "push", "-q", "origin", "main", "staging", "develop", env=env)
    yield work, env
    subprocess.run(["gpgconf", "--kill", "gpg-agent"], env={**os.environ, **env}, check=False)


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_pushes_are_manual_and_confirmation_comes_first():
    st = harness.steps(harness.load(RB))
    assert st[0]["action"] == "manual" and "Stevie" in st[0]["command"]
    for s in st + list(harness.load(RB).get("rollback") or []):
        if any(c.startswith("git") and " push " in f" {c} " for c in harness.commands(s.get("command"))):
            assert s["action"] == "manual", s["name"]


def test_preflight_refuses_an_unsigned_commit(sandbox_git_repo):
    rb = harness.load(RB)
    r = harness.run_step(rb, "check-signed-commits", env={**sandbox_git_repo.env, **env_for(sandbox_git_repo.work)}, check=False)
    assert r.returncode != 0


@harness.requires("gpg", "gpgconf")
def test_signed_batch_merges_no_ff_and_logs_one_line(signed_repo):
    work, env = signed_repo
    rb = harness.load(RB)
    renv = {**env, **env_for(work)}
    develop_head = harness.git(work, "rev-parse", "develop", env=env)
    harness.run_step(rb, "check-signed-commits", env=renv)
    harness.run_step(rb, "merge-branches", env=renv)
    harness.run_step(rb, "merge-branches", env=renv, field="verify")
    parents = harness.git(work, "rev-list", "--parents", "-n", "1", "staging", env=env).split()
    assert len(parents) == 3 and parents[2] == develop_head, "not a --no-ff merge of develop's head"
    before = (work / LOG).read_text().count("\n")
    harness.run_step(rb, "log-promotion", env=renv)
    harness.run_step(rb, "log-promotion", env=renv, field="verify")
    lines = (work / LOG).read_text().splitlines()
    assert len(lines) == before + 1 and "develop" in lines[-1] and "-> staging (2 commits)" in lines[-1]
    harness.run_step(rb, "switch-back-to-develop", env=renv)
    harness.run_step(rb, "switch-back-to-develop", env=renv, field="verify")
    # nothing left the machine: origin's staging is where it was
    assert harness.git(work, "rev-parse", "origin/staging", env=env) != harness.git(work, "rev-parse", "staging", env=env)
