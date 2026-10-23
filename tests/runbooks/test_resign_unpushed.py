# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/deploy/resign-unpushed-commits.yaml (R7).

A sandbox repo with 2 pushed and 3 unpushed unsigned commits and a bare
origin, signed with an ephemeral passphrase-less key in a temp GNUPGHOME.
The runbook's own probe, re-sign and verify steps run against it (REPO,
BRANCH, BASE pointed at the sandbox); the push stays manual and is not run."""
import os
import subprocess

import pytest

import harness

RB = "runbooks/deploy/resign-unpushed-commits.yaml"


def step_with(rb, needle, action=None):
    hits = [s for s in harness.steps(rb) if needle in (s.get("command") or "") and (action is None or s["action"] == action)]
    assert hits, f"no step runs {needle!r}"
    return hits[0]["name"]


@pytest.fixture
def signing_sandbox(sandbox_git_repo, tmp_path):
    gnupg = tmp_path / "g"
    gnupg.mkdir(mode=0o700)
    env = {**sandbox_git_repo.env, "GNUPGHOME": str(gnupg)}
    r = subprocess.run(["gpg", "--batch", "--passphrase", "", "--quick-gen-key", "Sandbox <sandbox@localhost>", "ed25519", "sign", "never"],
                       capture_output=True, text=True, env={**os.environ, **env}, timeout=60, check=False)
    if r.returncode != 0:
        pytest.skip(f"cannot make an ephemeral gpg key: {r.stderr[-200:]}")
    fpr = subprocess.run(["gpg", "--batch", "--with-colons", "--list-secret-keys"], capture_output=True, text=True,
                         env={**os.environ, **env}, check=True).stdout
    fpr = [ln.split(":")[9] for ln in fpr.splitlines() if ln.startswith("fpr:")][0]
    work = sandbox_git_repo.work
    harness.git(work, "config", "user.signingkey", fpr, env=env)
    # origin/develop has 2 commits ("base", "develop work"); add 3 unpushed, unsigned
    for i in range(3):
        (work / f"u{i}.txt").write_text(f"{i}\n")
        harness.git(work, "add", f"u{i}.txt", env=env)
        harness.git(work, "commit", "-q", "--no-gpg-sign", "-m", f"unpushed {i}", env=env)
    yield sandbox_git_repo, env
    subprocess.run(["gpgconf", "--kill", "gpg-agent"], env={**os.environ, **env}, check=False)


def test_dry_run_lists_every_step():
    assert harness.dry_run(RB) == [s["name"] for s in harness.steps(harness.load(RB))]


def test_push_is_manual_and_signing_probe_is_commit_tree():
    rb = harness.load(RB)
    for s in harness.steps(rb) + list(rb.get("rollback") or []):
        if "git" in (s.get("command") or "") and " push" in (s.get("command") or ""):
            assert s["action"] == "manual" and s.get("irreversible") is True, s["name"]
    text = open(harness.REPO / RB).read()
    assert "commit-tree -S" in text
    for s in harness.steps(rb):
        assert not any("--clearsign" in c for c in harness.commands(s.get("command"))), s["name"]


@harness.requires("gpg", "gpgconf")
def test_resign_signs_the_unpushed_and_leaves_the_pushed(signing_sandbox):
    repo, env = signing_sandbox
    rb = harness.load(RB)
    work = repo.work
    pushed_before = harness.git(work, "rev-list", "origin/develop", env=env).split()
    renv = {**env, "REPO": str(work), "BRANCH": "develop", "BASE": "origin/develop"}
    # the tool under test, untracked and excluded so the tree stays clean
    (work / "scripts").mkdir()
    (work / "scripts/resign-unpushed.py").write_text((harness.REPO / "scripts/resign-unpushed.py").read_text())
    (work / ".git/info/exclude").write_text("scripts/\n")
    for pc in rb.get("preconditions") or []:
        harness.run_step({"steps": [{"name": "pc", "command": pc["check"]}], "parameters": rb.get("parameters")}, "pc", env=renv, cwd=work)
    for name in [s["name"] for s in harness.steps(rb) if s["action"] == "shell"]:
        harness.run_step(rb, name, env=renv, cwd=work)
    assert step_with(rb, "commit-tree -S", "shell") and step_with(rb, "resign-unpushed.py", "shell")
    sigs = harness.git(work, "log", "--format=%G?", "origin/develop..develop", env=env).split()
    assert sigs == ["G", "G", "G"], sigs
    assert harness.git(work, "rev-list", "origin/develop", env=env).split() == pushed_before
    harness.git(work, "merge-base", "--is-ancestor", "origin/develop", "develop", env=env)  # raises if not
    # every shell verify of the runbook agrees
    for s in harness.steps(rb):
        if s["action"] == "shell" and s.get("verify"):
            harness.run_step(rb, s["name"], env=renv, field="verify", cwd=work)


@harness.requires("gpg")
def test_verify_refuses_unsigned_commits(sandbox_git_repo):
    rb = harness.load(RB)
    env = sandbox_git_repo.env
    work = sandbox_git_repo.work
    (work / "x").write_text("x\n")
    harness.git(work, "add", "x", env=env)
    harness.git(work, "commit", "-q", "--no-gpg-sign", "-m", "unsigned", env=env)
    renv = {**env, "REPO": str(work), "BRANCH": "develop", "BASE": "origin/develop"}
    verifies = [s for s in harness.steps(rb) if s["action"] == "shell" and "%G?" in (s.get("verify") or "") + (s.get("command") or "")]
    assert verifies, "no step checks %G?"
    failed = False
    for s in verifies:
        for field in ("verify", "command"):
            if "%G?" in (s.get(field) or ""):
                failed |= harness.run_step(rb, s["name"], env=renv, field=field, cwd=work, check=False).returncode != 0
    assert failed, "an unsigned commit passed the signature check"
