# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/deploy/rolling-upgrade.yaml. Every command here
runs against stand-ins in a temp dir: only the runbook's own preconditions and
the steps named below, never the runbook as a whole."""
import re
import stat
import subprocess

import harness
import pytest

RB = "runbooks/deploy/rolling-upgrade.yaml"
PUBLISH = "runbooks/deploy/apt-repo-publish.yaml"

DPKG_ROW = "ii  {} {} amd64 stand-in\n"


def stand_in(bin_dir, name, body):
    p = bin_dir / name
    p.write_text("#!/bin/bash\n" + body)
    p.chmod(p.stat().st_mode | stat.S_IXUSR)


def hosts(tmp_path, west, east):
    """PATH stand-ins for dpkg and ssh: west's and east's packages, as rows.
    east=None is a host ssh cannot reach."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (tmp_path / "west").write_text("".join(DPKG_ROW.format(*r) for r in west))
    stand_in(bin_dir, "dpkg", f'cat "{tmp_path}/west"\n')
    if east is None:
        stand_in(bin_dir, "ssh", "echo 'ssh: connect to host east: refused' >&2\nexit 255\n")
    else:
        (tmp_path / "east").write_text("".join(DPKG_ROW.format(*r) for r in east))
        stand_in(bin_dir, "ssh", f'cat "{tmp_path}/east"\n')
    return bin_dir


def preconditions(monkeypatch, capsys, bin_dir):
    """The runner's own precondition pass, with the given stand-ins first on
    PATH and an E4 answer given. Returns (passed, what it printed)."""
    rb = harness.load(RB)
    monkeypatch.setenv("PATH", f"{bin_dir}:/usr/bin:/bin")
    monkeypatch.setenv("GNUPGHOME", "/nonexistent/given")
    ok = harness._RUNNER.check_preconditions(rb, harness.runner_env(rb, {"GNUPGHOME": "given"}), False)
    return ok, capsys.readouterr().out


CURRENT = [("unheaded-wotan", "0.2.0~alpha+20261105"), ("unheaded-daemon", "0.2.0~alpha+20261105")]


def test_hosts_past_the_first_move_pass(tmp_path, monkeypatch, capsys):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, CURRENT))
    assert ok, out


@pytest.mark.parametrize("west,east,named", [
    (CURRENT, [("unheaded-wotan", "1.0.0")], "unheaded-wotan=1.0.0"),
    ([("unheaded-wotan", "1.0.0")], CURRENT, "unheaded-wotan=1.0.0"),
    (CURRENT, [("unheaded-akira", "1.0.1")], "unheaded-akira=1.0.1"),
    (CURRENT, [("unheaded-unheaded-daemon", "0.1.0~alpha+1.gabc")], "unheaded-unheaded-daemon"),
])
def test_first_move_is_refused_and_names_the_runbook_that_does_it(tmp_path, monkeypatch, capsys, west, east, named):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, west, east))
    assert not ok, out
    assert "apt-repo-publish" in out
    assert named in out


def test_a_host_that_cannot_be_read_is_refused(tmp_path, monkeypatch, capsys):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, None))
    assert not ok, out


def test_runner_finds_no_unset_variable_once_the_parameters_are_given():
    rb = harness.load(RB)
    given = {p["name"]: "given" for p in rb.get("parameters") or []}
    given.update(rb.get("env") or {})
    assert harness._RUNNER.find_unset_params(rb, given) == {}


def test_every_package_named_is_one_the_table_builds():
    control = subprocess.run(["bash", "scripts/build-all-debs.sh", "--control"], cwd=harness.REPO,
                             capture_output=True, text=True, check=True, timeout=60).stdout
    built = set(re.findall(r"^Package: (\S+)$", control, re.MULTILINE))
    named = set(harness.load(RB)["env"]["UNHEADED_PKGS"].split())
    assert named - built == set()


def test_upgrades_what_the_first_move_installed():
    install = harness.step(harness.load(PUBLISH), "install-east")["command"]
    first_move = set(re.search(r"for p in (.*?); do", install, re.DOTALL).group(1).replace("\\", " ").split())
    assert "unheaded-wotan" in first_move
    assert set(harness.load(RB)["env"]["UNHEADED_PKGS"].split()) == first_move


def sandbox(tmp_path, repo_script):
    """A stand-in checkout whose scripts/apt-repo.sh is repo_script, and a sudo
    that passes its environment assignments and command through."""
    repo = tmp_path / "repo"
    (repo / "scripts").mkdir(parents=True)
    (repo / "dist" / "debs" / "rolling-abcd1234").mkdir(parents=True)
    (repo / "dist" / "debs" / "rolling-abcd1234" / "new.deb").write_text("")
    (repo / "dist" / "debs" / "stale.deb").write_text("")
    stand_in(repo / "scripts", "apt-repo.sh", repo_script)
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    stand_in(bin_dir, "sudo", 'exec env "$@"\n')
    stand_in(bin_dir, "git", 'echo abcd1234\n')
    return repo, bin_dir


def publish(tmp_path, script):
    repo, bin_dir = sandbox(tmp_path, script)
    rb = harness.load(RB)
    env = {"REPO": str(repo), "GNUPGHOME": "/nonexistent/given", "APT_REPO_BASE": "/nonexistent/base",
           "PATH": f"{bin_dir}:/usr/bin:/bin"}
    return harness.run_step(rb, "publish-to-repo", env=env, check=False), repo


def test_publish_goes_through_the_signed_repo_script(tmp_path):
    log = tmp_path / "calls"
    r, _repo = publish(tmp_path, f'echo "$APT_REPO_BASE $GNUPGHOME $*" >> "{log}"\n')
    assert r.returncode == 0, r.stderr
    assert log.read_text().splitlines() == [
        "/nonexistent/base /nonexistent/given include dist/debs/rolling-abcd1234/new.deb",
        "/nonexistent/base /nonexistent/given check",
    ]


def test_a_failed_publish_stops_the_runbook(tmp_path):
    r, _repo = publish(tmp_path, 'echo "reprepro: refused" >&2\nexit 1\n')
    assert r.returncode != 0
    assert "refused" in r.stderr


@pytest.mark.parametrize("name", ["upgrade-east-first", "verify-east-health", "upgrade-west", "verify-west-health"])
def test_a_failure_before_tail_fails_the_step(tmp_path, name):
    # The stand-ins fail whatever the step runs on either host, and tail would
    # otherwise answer for it.
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    stand_in(bin_dir, "ssh", 'shift\nexec bash -c "$*"\n')
    for tool in ("sudo", "go"):
        stand_in(bin_dir, tool, "exit 7\n")
    rb = harness.load(RB)
    text = harness.step(rb, name)["command"].replace("/opt/unheaded/bin/akira", "sudo").replace("cd ~/tmp/unheaded && ", "")
    cmd = harness._RUNNER.expand_env(text, harness.runner_env(rb, {}))
    r = subprocess.run(["/bin/bash", "-c", cmd], capture_output=True, text=True, timeout=30, check=False,
                       env={"PATH": f"{bin_dir}:/usr/bin:/bin"}, cwd=tmp_path)
    assert r.returncode != 0, cmd


def test_build_has_the_time_the_publish_runbook_gives_it():
    rb = harness.load(RB)
    assert harness.step(rb, "build-new-packages")["timeout"] >= harness.step(harness.load(PUBLISH), "build-debs")["timeout"]


def test_versions_are_shown_for_every_service_package():
    rb = harness.load(RB)
    for name in ("pre-flight-check", "post-upgrade-check"):
        assert re.search(r"for svc in .*\bdaemon\b", harness.step(rb, name)["command"]), name
