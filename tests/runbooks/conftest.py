# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Fixtures for runbook dry-run tests (S-TWO-BRAINS Step 268).

  git_env           an environment whose git ignores the operator's global
                    and system config (no signing key, no hooks, no remotes)
  sandbox_git_repo  a work clone with main, staging and develop, and a bare
                    "origin" in the temp dir: pushes go nowhere else
  throwaway_wotan   a Wotan built from services/wotan, on free ports, with a
                    temp topic config; skipped with the reason if it cannot build

Marker `live`: a test that needs a lab component (an FRR VM, a Well); it is
deselected in CI (-m "not live") and skips itself unless its env flag is set.
"""
import os
import shutil
import subprocess
from types import SimpleNamespace

import harness
import pytest


def pytest_configure(config):
    config.addinivalue_line("markers", "live: needs a lab component; CI runs -m 'not live'")


@pytest.fixture
def git_env(tmp_path):
    cfg = tmp_path / "gitconfig"
    cfg.write_text(
        "[user]\n\tname = Sandbox\n\temail = sandbox@localhost\n"
        "[commit]\n\tgpgsign = false\n[tag]\n\tgpgsign = false\n"
        "[init]\n\tdefaultBranch = main\n[advice]\n\tdetachedHead = false\n"
    )
    return {"GIT_CONFIG_GLOBAL": str(cfg), "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0"}


@pytest.fixture
def sandbox_git_repo(tmp_path, git_env):
    env = git_env
    origin = tmp_path / "origin.git"
    work = tmp_path / "work"
    harness.git(tmp_path, "init", "-q", "--bare", "-b", "main", str(origin), env=env)
    harness.git(tmp_path, "init", "-q", "-b", "main", str(work), env=env)
    (work / "README").write_text("sandbox\n")
    harness.git(work, "add", "README", env=env)
    harness.git(work, "commit", "-q", "-m", "base", env=env)
    harness.git(work, "remote", "add", "origin", str(origin), env=env)
    harness.git(work, "push", "-q", "origin", "main", env=env)
    for b in ("staging", "develop"):
        harness.git(work, "branch", b, env=env)
        harness.git(work, "push", "-q", "origin", b, env=env)
    harness.git(work, "switch", "-q", "develop", env=env)
    (work / "feature.txt").write_text("work on develop\n")
    harness.git(work, "add", "feature.txt", env=env)
    harness.git(work, "commit", "-q", "-m", "develop work", env=env)
    harness.git(work, "push", "-q", "origin", "develop", env=env)
    harness.git(work, "fetch", "-q", "origin", env=env)
    return SimpleNamespace(work=work, origin=origin, env=env)


@pytest.fixture(scope="session")
def wotan_binary(tmp_path_factory):
    if shutil.which("go") is None:
        pytest.skip("services/wotan does not build here: no go on PATH")
    out = tmp_path_factory.mktemp("wotan-bin") / "wotan"
    r = subprocess.run(["go", "build", "-o", str(out), "./services/wotan/cmd/wotan"],
                       cwd=harness.REPO, capture_output=True, text=True, timeout=600, check=False)
    if r.returncode != 0:
        pytest.skip(f"services/wotan does not build here: {r.stderr.strip()[-300:]}")
    return out


@pytest.fixture
def throwaway_wotan(wotan_binary, tmp_path):
    w = harness.Wotan(wotan_binary, tmp_path)
    try:
        w.start()
    except RuntimeError as e:
        pytest.skip(f"throwaway wotan did not start: {e}")
    yield w
    w.stop()


def pytest_collection_modifyitems(config, items):
    # A live test that is not asked for says why it did not run.
    for item in items:
        if item.get_closest_marker("live") is None:
            continue
        flag = getattr(item.module, "LIVE_FLAG", None)
        if flag and os.environ.get(flag) != "1":
            item.add_marker(pytest.mark.skip(reason=f"live: set {flag}=1 on a lab host to run"))
