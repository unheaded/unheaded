# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Dry-run test for runbooks/deploy/rolling-upgrade.yaml. Every command here
runs against stand-ins in a temp dir: only the runbook's own preconditions and
the steps named below, never the runbook as a whole."""
import re
import shutil
import stat
import subprocess

import harness
import pytest

RB = "runbooks/deploy/rolling-upgrade.yaml"
PUBLISH = "runbooks/deploy/apt-repo-publish.yaml"

DPKG_ROW = "{} {} {} amd64 stand-in\n"
BUILD = "0.2.0~alpha+20261106000000.gabcd1234"
AHEAD = "0.2.0~alpha+20261107000000.g11112222"


def stand_in(bin_dir, name, body):
    p = bin_dir / name
    p.write_text("#!/bin/bash\n" + body)
    p.chmod(p.stat().st_mode | stat.S_IXUSR)


def dpkg_rows(pkgs):
    """(name, version) is an installed package; a leading state overrides ii."""
    return "".join(DPKG_ROW.format(*(p if len(p) == 3 else ("ii", *p))) for p in pkgs)


def dpkg_stand_in(bin_dir, rows, works=True):
    """dpkg -l as dpkg answers it: its header, the rows matching what was asked
    for, and a failure when none match. A dpkg that does not work fails every
    call. Comparing versions stays real dpkg."""
    rows.parent.mkdir(exist_ok=True)
    stand_in(bin_dir, "dpkg", "exit 1\n" if not works else f"""case "$1" in
--compare-versions) exec /usr/bin/dpkg "$@" ;;
--version) exit 0 ;;
esac
shift
found=""
while read -r state name rest; do
  for pattern in "$@"; do
    case "${{name%%:*}}" in $pattern) found="$found$state $name $rest"$'\\n' ;; esac
  done
done < "{rows}"
[ -n "$found" ] || exit 1
echo 'Desired=Unknown/Install/Remove/Purge/Hold'
echo '| Status=Not/Inst/Conf-files/Unpacked/halF-conf/Half-inst/trig-aWait/Trig-pend'
echo '||/ Name Version Architecture Description'
echo '+++-====-=======-============-==========='
printf '%s' "$found"
""")


def hosts(tmp_path, west, east, west_dpkg=True, east_dpkg=True, east_config=True):
    """PATH stand-ins for both hosts: their dpkg, as rows, and a checkout whose
    build version is BUILD. east=None is a host ssh cannot reach."""
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    (tmp_path / "porcelain").write_text("")
    stand_in(bin_dir, "git", f'echo -n "$(<"{tmp_path}/porcelain")"\n')
    stand_in(bin_dir, "reprepro", "exit 0\n")
    repo = tmp_path / "repo"
    (repo / "scripts").mkdir(parents=True)
    stand_in(repo / "scripts", "build-all-debs.sh", f'[ "$1" = --print-version ] && echo "{BUILD}"\n')
    (tmp_path / "apt" / "conf").mkdir(parents=True)
    (tmp_path / "apt" / "conf" / "distributions").write_text("")
    (tmp_path / "west.rows").write_text(dpkg_rows(west))
    dpkg_stand_in(bin_dir, tmp_path / "west.rows", works=west_dpkg)
    if east is None:
        stand_in(bin_dir, "ssh", "echo 'ssh: connect to host east: refused' >&2\nexit 255\n")
    else:
        east_bin = tmp_path / "east-bin"
        east_bin.mkdir()
        (tmp_path / "east.rows").write_text(dpkg_rows(east))
        dpkg_stand_in(east_bin, tmp_path / "east.rows", works=east_dpkg)
        stand_in(east_bin, "systemctl", "exit 0\n")
        stand_in(bin_dir, "ssh", f'case "$*" in *akira.yaml*) exit {0 if east_config else 1} ;; esac\n'
                                 f'shift\nPATH="{east_bin}" exec /bin/bash -c "$*"\n')
    stand_in(bin_dir, "systemctl", "exit 0\n")
    return bin_dir


def preconditions(monkeypatch, capsys, bin_dir, gnupghome="/nonexistent/given"):
    """The runner's own precondition pass, with the given stand-ins as the whole
    PATH and an E4 answer given. Returns (passed, what it printed)."""
    rb = harness.load(RB)
    given = {"REPO": str(bin_dir.parent / "repo"), "APT_REPO_BASE": str(bin_dir.parent / "apt"), "GNUPGHOME": gnupghome}
    monkeypatch.setenv("PATH", str(bin_dir))
    for k, v in given.items():
        monkeypatch.setenv(k, v)
    ok = harness._RUNNER.check_preconditions(rb, harness.runner_env(rb, given), False)
    return ok, capsys.readouterr().out


CURRENT = [("unheaded-wotan", "0.2.0~alpha+20261105"), ("unheaded-daemon", "0.2.0~alpha+20261105")]


def test_hosts_past_the_first_move_pass(tmp_path, monkeypatch, capsys):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, CURRENT))
    assert ok, out


@pytest.mark.parametrize("rows", [
    [("unheaded-wotan", BUILD)],
    [("rc", "unheaded-unheaded-daemon", "1.0.0")],
    [],
])
def test_only_what_cannot_be_upgraded_is_refused(tmp_path, monkeypatch, capsys, rows):
    # At this build already, removed with its config left, or nothing installed.
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, rows))
    assert ok, out


@pytest.mark.parametrize("west,east,named", [
    (CURRENT, [("unheaded-wotan", "1.0.0")], "unheaded-wotan=1.0.0"),
    ([("unheaded-wotan", "1.0.0")], CURRENT, "unheaded-wotan=1.0.0"),
    (CURRENT, [("unheaded-akira", "1.0.1")], "unheaded-akira=1.0.1"),
    (CURRENT, [("unheaded-unheaded-daemon", "0.1.0~alpha+1.gabc")], "unheaded-unheaded-daemon"),
    (CURRENT, [("hi", "unheaded-wotan", "1.0.0")], "unheaded-wotan=1.0.0"),
    (CURRENT, [("unheaded-akira", "1.1.0")], "unheaded-akira=1.1.0"),
    (CURRENT, [("unheaded-wotan", AHEAD)], f"unheaded-wotan={AHEAD}"),
    ([("unheaded-wotan:amd64", "1.0.0")], CURRENT, "unheaded-wotan=1.0.0"),
])
def test_what_only_upgrade_would_keep_is_refused_and_names_the_way_out(tmp_path, monkeypatch, capsys, west, east, named):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, west, east))
    assert not ok, out
    assert "apt-repo-publish" in out
    assert named in out


@pytest.mark.parametrize("kw", [
    {"east": None},
    {"east": CURRENT, "west_dpkg": False},
    {"east": CURRENT, "east_dpkg": False},
])
def test_a_host_that_cannot_be_read_is_refused(tmp_path, monkeypatch, capsys, kw):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, **kw))
    assert not ok, out


def test_a_dirty_checkout_is_refused_before_anything_runs(tmp_path, monkeypatch, capsys):
    bin_dir = hosts(tmp_path, CURRENT, CURRENT)
    (tmp_path / "porcelain").write_text(" M scripts/x\n")
    ok, out = preconditions(monkeypatch, capsys, bin_dir)
    assert not ok, out


def test_a_host_without_reprepro_is_refused_before_anything_runs(tmp_path, monkeypatch, capsys):
    bin_dir = hosts(tmp_path, CURRENT, CURRENT)
    (bin_dir / "reprepro").unlink()
    ok, out = preconditions(monkeypatch, capsys, bin_dir)
    assert not ok, out


def test_a_repo_not_initialised_is_refused_before_anything_runs(tmp_path, monkeypatch, capsys):
    bin_dir = hosts(tmp_path, CURRENT, CURRENT)
    (tmp_path / "apt" / "conf" / "distributions").unlink()
    ok, out = preconditions(monkeypatch, capsys, bin_dir)
    assert not ok, out


def test_no_e4_answer_is_refused(tmp_path, monkeypatch, capsys):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, CURRENT), gnupghome="")
    assert not ok, out


def test_east_without_the_health_sweeps_config_is_refused_before_it_is_upgraded(tmp_path, monkeypatch, capsys):
    ok, out = preconditions(monkeypatch, capsys, hosts(tmp_path, CURRENT, CURRENT, east_config=False))
    assert not ok, out
    assert "akira" in out


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


def sandbox(tmp_path, repo_script="exit 0\n", build_script="exit 0\n"):
    """A stand-in checkout whose scripts are the ones given, and a sudo that
    passes its environment assignments and command through."""
    repo = tmp_path / "repo"
    (repo / "scripts").mkdir(parents=True)
    (repo / "dist" / "debs").mkdir(parents=True)
    (repo / "dist" / "debs" / "stale.deb").write_text("")
    stand_in(repo / "scripts", "apt-repo.sh", repo_script)
    stand_in(repo / "scripts", "build-all-debs.sh", build_script)
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    stand_in(bin_dir, "sudo", 'exec env "$@"\n')
    stand_in(bin_dir, "git", 'echo abcd1234\n')
    return repo, bin_dir


def publish(tmp_path, script):
    repo, bin_dir = sandbox(tmp_path, script)
    (repo / "dist" / "debs" / "rolling-abcd1234").mkdir()
    (repo / "dist" / "debs" / "rolling-abcd1234" / "new.deb").write_text("")
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
    log = tmp_path / "calls"
    r, _repo = publish(tmp_path, f'[ "$1" != include ] || {{ echo "reprepro: refused" >&2; exit 1; }}\necho "$1" >> "{log}"\n')
    assert r.returncode != 0
    assert "refused" in r.stderr
    assert not log.exists(), "the repo was checked after the include failed"


def compare(tmp_path, script):
    repo, bin_dir = sandbox(tmp_path, script)
    (repo / "dist" / "debs" / "rolling-abcd1234").mkdir()
    (repo / "dist" / "debs" / "rolling-abcd1234" / "new.deb").write_text("")
    env = {"REPO": str(repo), "APT_REPO_BASE": "/nonexistent/base", "PATH": f"{bin_dir}:/usr/bin:/bin"}
    return harness.run_step(harness.load(RB), "compare-repo-with-build", env=env, check=False)


def test_no_host_moves_before_the_repo_is_compared_with_the_build():
    rb = harness.load(RB)
    order = harness.dry_run(RB)
    assert order.index("publish-to-repo") < order.index("compare-repo-with-build")
    assert order.index("compare-repo-with-build") < order.index("upgrade-east-first")
    assert order.index("compare-repo-with-build") < order.index("upgrade-west")
    assert harness.index(rb, "compare-repo-with-build") == order.index("compare-repo-with-build")


def test_the_comparison_reads_this_commits_debs_in_this_repo(tmp_path):
    log = tmp_path / "calls"
    r = compare(tmp_path, f'echo "$APT_REPO_BASE $*" >> "{log}"\n')
    assert r.returncode == 0, r.stderr
    assert log.read_text().splitlines() == ["/nonexistent/base compare dist/debs/rolling-abcd1234/new.deb"]


def test_a_repo_that_does_not_serve_the_build_stops_the_runbook(tmp_path):
    r = compare(tmp_path, 'echo "apt-repo: compare: the repo serves unheaded-wotan newer" >&2\nexit 1\n')
    assert r.returncode != 0
    assert "serves unheaded-wotan newer" in r.stderr


def build(tmp_path, script):
    repo, bin_dir = sandbox(tmp_path, build_script=script)
    env = {"REPO": str(repo), "PATH": f"{bin_dir}:/usr/bin:/bin"}
    return harness.run_step(harness.load(RB), "build-new-packages", env=env, check=False), repo


# Stands in for build-all-debs.sh: writes where it was told, or the default.
BUILDS = 'mkdir -p "${{DEB_OUTPUT_DIR:-dist/debs}}"\ntouch "${{DEB_OUTPUT_DIR:-dist/debs}}/new.deb"\necho built >> "{}"\n'


def test_the_build_goes_in_its_commits_own_directory(tmp_path):
    r, repo = build(tmp_path, BUILDS.format(tmp_path / "builds"))
    assert r.returncode == 0, r.stderr
    assert (repo / "dist" / "debs" / "rolling-abcd1234" / "new.deb").exists()
    assert not (repo / "dist" / "debs" / "new.deb").exists()


def test_a_retry_at_the_same_commit_keeps_the_debs_the_repo_may_hold(tmp_path):
    # The repo refuses one version with other bytes, and a rebuild has other bytes.
    builds = tmp_path / "builds"
    repo, bin_dir = sandbox(tmp_path, build_script=BUILDS.format(builds))
    env = {"REPO": str(repo), "PATH": f"{bin_dir}:/usr/bin:/bin"}
    rb = harness.load(RB)
    harness.run_step(rb, "build-new-packages", env=env)
    harness.run_step(rb, "build-new-packages", env=env)
    assert builds.read_text().splitlines() == ["built"]


def test_a_build_cut_short_is_not_taken_for_a_finished_one(tmp_path):
    builds = tmp_path / "builds"
    r, repo = build(tmp_path, BUILDS.format(builds) + "exit 1\n")
    assert r.returncode != 0
    assert not (repo / "dist" / "debs" / "rolling-abcd1234").exists()
    stand_in(repo / "scripts", "build-all-debs.sh", BUILDS.format(builds))
    env = {"REPO": str(repo), "PATH": f"{tmp_path / 'bin'}:/usr/bin:/bin"}
    harness.run_step(harness.load(RB), "build-new-packages", env=env)
    assert len(builds.read_text().splitlines()) == 2
    assert (repo / "dist" / "debs" / "rolling-abcd1234" / "new.deb").exists()


def run_with_real_tail(rb, name, bin_dir, tmp_path):
    cmd = harness._RUNNER.expand_env(harness.step(rb, name)["command"], harness.runner_env(rb, {}))
    return subprocess.run(["/bin/bash", "-c", cmd], capture_output=True, text=True, timeout=30, check=False,
                          env={"PATH": f"{bin_dir}:/usr/bin:/bin"}, cwd=tmp_path)


def test_a_failure_before_tail_fails_the_step(tmp_path):
    # The tool fails where tail would answer for it. apt-get update passes: it
    # sits before the pipeline, and a failure there would prove nothing.
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    stand_in(bin_dir, "ssh", 'shift\nexec bash -c "$*"\n')
    stand_in(bin_dir, "sudo", 'case "$*" in *"apt-get update"*) exit 0 ;; esac\nexit 7\n')
    r = run_with_real_tail(harness.load(RB), "upgrade-east-first", bin_dir, tmp_path)
    assert r.returncode == 7, r.stderr


def upgrade_west(tmp_path, rows, built=("unheaded-keygen", "unheaded-wotan", "unheaded-daemon"), sudo_exit=0):
    """upgrade-west on a west holding rows, with these debs built. Returns the
    result, the directory the debs are in, and what sudo was asked."""
    repo, bin_dir = sandbox(tmp_path, build_script=f'[ "$1" = --print-version ] && echo "{BUILD}"\n')
    debs = repo / "dist" / "debs" / "rolling-abcd1234"
    debs.mkdir()
    for pkg in built:
        (debs / f"{pkg}_{BUILD}_amd64.deb").write_text("")
    (tmp_path / "west.rows").write_text(dpkg_rows(rows))
    dpkg_stand_in(bin_dir, tmp_path / "west.rows")
    calls = tmp_path / "sudo"
    stand_in(bin_dir, "sudo", f'echo "$*" >> "{calls}"\nexit {sudo_exit}\n')
    env = {"REPO": str(repo), "PATH": f"{bin_dir}:/usr/bin:/bin"}
    r = harness.run_step(harness.load(RB), "upgrade-west", env=env, check=False)
    return r, debs, calls.read_text().splitlines() if calls.exists() else []


def test_west_installs_the_debs_this_run_built_and_asks_no_apt_source(tmp_path):
    r, debs, calls = upgrade_west(tmp_path, [("unheaded-wotan", "0.2.0~alpha+20261105")])
    assert r.returncode == 0, r.stderr
    assert calls == [f"apt-get install -y {debs}/unheaded-keygen_{BUILD}_amd64.deb {debs}/unheaded-wotan_{BUILD}_amd64.deb"]


def test_west_gains_nothing_it_does_not_hold_but_the_key_tool_its_packages_need(tmp_path):
    r, debs, calls = upgrade_west(tmp_path, [("unheaded-keygen", "0.2.0~alpha+20261105"), ("unheaded-daemon", "0.2.0~alpha+20261105")])
    assert r.returncode == 0, r.stderr
    assert calls == [f"apt-get install -y {debs}/unheaded-daemon_{BUILD}_amd64.deb {debs}/unheaded-keygen_{BUILD}_amd64.deb"]


def test_a_package_west_holds_that_this_run_did_not_build_stops_the_step(tmp_path):
    r, _debs, calls = upgrade_west(tmp_path, [("unheaded-wotan", "0.2.0~alpha+20261105")], built=("unheaded-keygen",))
    assert r.returncode != 0
    assert "unheaded-wotan" in r.stderr
    assert calls == []


def test_a_west_holding_none_of_the_packages_stops_the_step(tmp_path):
    r, _debs, calls = upgrade_west(tmp_path, [])
    assert r.returncode != 0
    assert calls == []


def test_a_failed_install_on_west_fails_the_step(tmp_path):
    r, _debs, _calls = upgrade_west(tmp_path, [("unheaded-wotan", "0.2.0~alpha+20261105")], sudo_exit=7)
    assert r.returncode == 7, r.stderr


UPGRADED = [("unheaded-wotan", BUILD), ("unheaded-daemon", BUILD)]


def reached(tmp_path, name, rows):
    """A host's verify after its upgrade, with rows as what that host holds."""
    bin_dir = hosts(tmp_path, rows, rows)
    env = {"REPO": str(tmp_path / "repo"), "PATH": f"{bin_dir}:/usr/bin:/bin"}
    return harness.run_step(harness.load(RB), name, env=env, field="verify", check=False)


@pytest.mark.parametrize("name", ["upgrade-east-first", "upgrade-west"])
def test_a_host_at_the_published_version_passes(tmp_path, name):
    r = reached(tmp_path, name, [*UPGRADED, ("unheaded-huginn", "0.2.0~alpha+20261001")])
    assert r.returncode == 0, r.stderr


@pytest.mark.parametrize("name", ["upgrade-east-first", "upgrade-west"])
def test_a_host_that_did_not_reach_the_published_version_fails(tmp_path, name):
    r = reached(tmp_path, name, [UPGRADED[0], ("unheaded-daemon", "0.2.0~alpha+20261105")])
    assert r.returncode != 0
    assert "unheaded-daemon=0.2.0~alpha+20261105" in r.stderr


@pytest.mark.parametrize("name", ["upgrade-east-first", "upgrade-west"])
def test_a_host_showing_none_of_the_packages_fails(tmp_path, name):
    assert reached(tmp_path, name, []).returncode != 0


def test_the_version_checked_is_the_version_built(tmp_path):
    # A copy of the build script in a throwaway commit: it answers for that
    # commit in the form the debs carry, and refuses a tree no commit names.
    (tmp_path / "scripts" / "lib").mkdir(parents=True)
    for f in ("build-all-debs.sh", "lib/deb-packages.sh", "lib/deb-maintscripts.sh"):
        shutil.copy(harness.REPO / "scripts" / f, tmp_path / "scripts" / f)
    harness.git(tmp_path, "init", "-q")
    harness.git(tmp_path, "add", "scripts")
    harness.git(tmp_path, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-qm", "t")

    def asked(*args):
        return subprocess.run(["bash", "scripts/build-all-debs.sh", "--print-version", *args], cwd=tmp_path,
                              capture_output=True, text=True, timeout=60, check=False)

    head = harness.git(tmp_path, "rev-parse", "--short=8", "HEAD")
    r = asked()
    assert r.returncode == 0, r.stderr
    assert re.fullmatch(rf"0\.2\.0~alpha\+\d{{14}}\.g{head}\n", r.stdout), r.stdout
    assert asked("0.3.0~beta").stdout == "0.3.0~beta\n"
    assert not (tmp_path / "dist").exists()
    (tmp_path / "untracked").write_text("")
    assert asked().returncode != 0


@pytest.mark.parametrize("name", ["upgrade-east-first", "upgrade-west"])
def test_an_upgrade_has_the_time_the_first_move_gives_it(name):
    rb = harness.load(RB)
    assert harness.step(rb, name)["timeout"] >= harness.step(harness.load(PUBLISH), "install-east")["timeout"]


def test_build_has_the_time_the_publish_runbook_gives_it():
    rb = harness.load(RB)
    assert harness.step(rb, "build-new-packages")["timeout"] >= harness.step(harness.load(PUBLISH), "build-debs")["timeout"]


def test_versions_are_shown_for_every_service_package():
    rb = harness.load(RB)
    for name in ("pre-flight-check", "post-upgrade-check"):
        assert re.search(r"for svc in .*\bdaemon\b", harness.step(rb, name)["command"]), name
