# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""rolling-upgrade's rollback puts back the packages each host held. The script
and the runbook's rollback entries run for real against stand-ins for both hosts:
real debs in a pool, dpkg, apt, ssh and scp replaced in a temp dir. Nothing here
reaches west, east or a repo."""
import os
import shutil
import subprocess

import harness
import pytest
from test_rolling_upgrade import RB, dpkg_rows, dpkg_stand_in, stand_in

SCRIPT = harness.REPO / "scripts" / "rolling-previous.sh"
OLD = "0.2.0~alpha+20261105000000.gaaaa1111"
NEW = "0.2.0~alpha+20261106000000.gbbbb2222"
PKGS = ["unheaded-keygen", "unheaded-wotan", "unheaded-daemon"]
MOVED = ("upgrade-east-first", "verify-east-health", "upgrade-west", "verify-west-health")

# apt as far as this test needs it: a named .deb lands in the host's rows, unless
# it is told to fail or to do nothing and say nothing.
APT = """echo "$*" >> "{calls}"
[ "$1 $2" = "apt-get install" ] || exit 0
[ ! -e "{root}/apt-fails" ] || exit 7
[ ! -e "{root}/apt-idle" ] || exit 0
for a in "$@"; do
  case "$a" in
    *.deb)
      base="${{a##*/}}"; pkg="${{base%%_*}}"; rest="${{base#*_}}"; ver="${{rest%_*}}"
      sed -i "s/^\\(..\\) $pkg .*/\\1 $pkg $ver amd64 stand-in/" "{rows}"
      ;;
  esac
done
"""


def make_deb(directory, pkg, version):
    """A real package, so the script's dpkg-deb reads a real control file."""
    tree = directory / f".{pkg}-{version}"
    (tree / "DEBIAN").mkdir(parents=True)
    (tree / "DEBIAN" / "control").write_text(
        f"Package: {pkg}\nVersion: {version}\nArchitecture: amd64\nMaintainer: t <t@invalid>\nDescription: t\n")
    deb = directory / f"{pkg}_{version}_amd64.deb"
    subprocess.run(["dpkg-deb", "--root-owner-group", "--build", str(tree), str(deb)],
                   capture_output=True, timeout=30, check=True)
    shutil.rmtree(tree)
    return deb


class Hosts:
    """West and east as this test sees them: dpkg rows, apt, ssh and scp."""

    def __init__(self, tmp_path, west, east):
        self.root = tmp_path
        self.west_bin = tmp_path / "bin"
        self.east_bin = tmp_path / "east-bin"
        self.west_bin.mkdir()
        self.east_bin.mkdir()
        self.west_rows = tmp_path / "west.rows"
        self.east_rows = tmp_path / "east.rows"
        self.west_calls = tmp_path / "west.calls"
        self.east_calls = tmp_path / "east.calls"
        self.west_rows.write_text(dpkg_rows(west))
        self.east_rows.write_text(dpkg_rows(east))
        dpkg_stand_in(self.west_bin, self.west_rows)
        dpkg_stand_in(self.east_bin, self.east_rows)
        stand_in(self.west_bin, "sudo", APT.format(calls=self.west_calls, root=tmp_path, rows=self.west_rows))
        stand_in(self.east_bin, "sudo", APT.format(calls=self.east_calls, root=tmp_path, rows=self.east_rows))
        stand_in(self.west_bin, "ssh", f'shift\nPATH="{self.east_bin}:/usr/bin:/bin" exec /bin/bash -c "$*"\n')
        stand_in(self.west_bin, "scp", 'shift\nargs=("$@")\ndest="${args[-1]#*:}"\nunset "args[-1]"\ncp -- "${args[@]}" "$dest/"\n')
        self.checkout = tmp_path / "checkout"
        (self.checkout / "dist" / "debs").mkdir(parents=True)
        self.pool = tmp_path / "apt" / "pool"
        self.pool.mkdir(parents=True)
        self.keep = "dist/debs/rollback-abcd1234"

    def publish(self, pkg, version):
        directory = self.pool / "main" / "u" / pkg
        directory.mkdir(parents=True, exist_ok=True)
        return make_deb(directory, pkg, version)

    def env(self):
        return {**os.environ, "PATH": f"{self.west_bin}:/usr/bin:/bin"}

    def run(self, *args):
        return subprocess.run(["bash", str(SCRIPT), *args], cwd=self.checkout, env=self.env(),
                              capture_output=True, text=True, timeout=60, check=False)

    def record(self):
        return self.run("record", self.keep, str(self.root / "apt"), *PKGS)

    def rows(self, host):
        rows = self.west_rows if host == "west" else self.east_rows
        return {line.split()[1]: line.split()[2] for line in rows.read_text().splitlines()}

    def moved_to(self, version):
        for rows in (self.west_rows, self.east_rows):
            rows.write_text(rows.read_text().replace(OLD, version))


WEST = [("unheaded-wotan", OLD)]
EAST = [("unheaded-keygen", OLD), ("unheaded-wotan", OLD), ("unheaded-daemon", OLD)]


@pytest.fixture
def recorded(tmp_path):
    h = Hosts(tmp_path, WEST, EAST)
    for pkg in PKGS:
        h.publish(pkg, OLD)
    r = h.record()
    assert r.returncode == 0, r.stderr
    return h


def test_the_record_is_made_before_the_publish_drops_the_old_debs():
    order = harness.dry_run(RB)
    assert order.index("build-new-packages") < order.index("record-previous") < order.index("publish-to-repo")
    assert order.index("record-previous") < min(order.index(s) for s in MOVED)


def test_record_keeps_what_each_host_holds_and_the_debs_to_put_it_back(recorded):
    keep = recorded.checkout / recorded.keep
    assert (keep / "west.versions").read_text() == f"unheaded-wotan {OLD}\n"
    assert sorted((keep / "east.versions").read_text().splitlines()) == sorted(f"{p} {OLD}" for p in PKGS)
    assert sorted(f.name for f in (keep / "debs").iterdir()) == sorted(f"{p}_{OLD}_amd64.deb" for p in PKGS)


def test_a_host_with_a_package_the_repo_dropped_is_served_from_an_earlier_build(tmp_path):
    h = Hosts(tmp_path, WEST, [])
    earlier = h.checkout / "dist" / "debs" / "rolling-aaaa1111"
    earlier.mkdir()
    make_deb(earlier, "unheaded-wotan", OLD)
    r = h.record()
    assert r.returncode == 0, r.stderr
    assert (h.checkout / h.keep / "debs" / f"unheaded-wotan_{OLD}_amd64.deb").exists()


def test_a_version_with_no_deb_stops_the_run_before_any_host_moves(tmp_path):
    h = Hosts(tmp_path, WEST, EAST)
    h.publish("unheaded-keygen", OLD)
    h.publish("unheaded-daemon", OLD)
    r = h.record()
    assert r.returncode != 0
    assert f"west:unheaded-wotan={OLD}" in r.stderr
    assert f"east:unheaded-wotan={OLD}" in r.stderr
    assert "nothing moved" in r.stderr
    assert not (h.checkout / h.keep).exists()


def test_a_deb_named_for_a_version_it_does_not_carry_is_not_taken(tmp_path):
    h = Hosts(tmp_path, WEST, [])
    deb = h.publish("unheaded-wotan", NEW)
    deb.rename(deb.with_name(f"unheaded-wotan_{OLD}_amd64.deb"))
    r = h.record()
    assert r.returncode != 0
    assert f"west:unheaded-wotan={OLD}" in r.stderr


def test_an_unreachable_east_stops_the_record(tmp_path):
    h = Hosts(tmp_path, WEST, EAST)
    for pkg in PKGS:
        h.publish(pkg, OLD)
    stand_in(h.west_bin, "ssh", "exit 255\n")
    r = h.record()
    assert r.returncode != 0
    assert not (h.checkout / h.keep).exists()


def test_a_retry_keeps_the_record_a_moved_host_can_no_longer_give(recorded):
    recorded.moved_to(NEW)
    r = recorded.record()
    assert r.returncode == 0, r.stderr
    assert (recorded.checkout / recorded.keep / "west.versions").read_text() == f"unheaded-wotan {OLD}\n"


def test_west_goes_back_to_what_it_held(recorded):
    recorded.moved_to(NEW)
    r = recorded.run("restore", "west", recorded.keep)
    assert r.returncode == 0, r.stderr
    assert recorded.rows("west") == {"unheaded-wotan": OLD}
    keep = recorded.checkout / recorded.keep / "debs"
    assert recorded.west_calls.read_text().splitlines() == [
        f"apt-get install -y --allow-downgrades {keep}/unheaded-wotan_{OLD}_amd64.deb"]
    assert recorded.rows("east")["unheaded-wotan"] == NEW, "restoring west moved east"


def test_east_goes_back_to_what_it_held_from_debs_sent_to_it(recorded):
    recorded.moved_to(NEW)
    r = recorded.run("restore", "east", recorded.keep)
    assert r.returncode == 0, r.stderr
    assert recorded.rows("east") == {p: OLD for p in PKGS}
    calls = recorded.east_calls.read_text().splitlines()
    assert len(calls) == 1
    assert calls[0].startswith("apt-get install -y --allow-downgrades /var/tmp/unheaded-rollback.")
    assert calls[0].count(".deb") == len(PKGS)
    assert not list(harness.Path("/var/tmp").glob("unheaded-rollback.*")), "the copy on east was left behind"


@pytest.mark.parametrize("host", ["west", "east"])
def test_a_failed_install_fails_the_rollback(recorded, host):
    recorded.moved_to(NEW)
    (recorded.root / "apt-fails").touch()
    r = recorded.run("restore", host, recorded.keep)
    assert r.returncode != 0
    assert not list(harness.Path("/var/tmp").glob("unheaded-rollback.*"))


@pytest.mark.parametrize("host", ["west", "east"])
def test_a_host_that_did_not_get_back_fails_the_rollback_and_says_which(recorded, host):
    recorded.moved_to(NEW)
    (recorded.root / "apt-idle").touch()
    r = recorded.run("restore", host, recorded.keep)
    assert r.returncode != 0
    assert f"unheaded-wotan {NEW}" in r.stderr


def test_a_kept_deb_that_changed_is_not_installed(recorded):
    recorded.moved_to(NEW)
    with (recorded.checkout / recorded.keep / "debs" / f"unheaded-wotan_{OLD}_amd64.deb").open("ab") as f:
        f.write(b"x")
    r = recorded.run("restore", "west", recorded.keep)
    assert r.returncode != 0
    assert not recorded.west_calls.exists()


def test_no_record_is_a_refusal_not_a_quiet_pass(tmp_path):
    h = Hosts(tmp_path, WEST, EAST)
    r = h.run("restore", "west", h.keep)
    assert r.returncode != 0
    assert "no record" in r.stderr


def test_a_host_that_held_nothing_has_nothing_to_put_back(tmp_path):
    h = Hosts(tmp_path, [], EAST)
    for pkg in PKGS:
        h.publish(pkg, OLD)
    assert h.record().returncode == 0
    r = h.run("restore", "west", h.keep)
    assert r.returncode == 0, r.stderr
    assert not h.west_calls.exists()


def rollback(name):
    return next(e for e in harness.load(RB)["rollback"] if e["name"] == name)


def run_rollback(tmp_path, name):
    """A rollback entry as the runner expands it, with the script as a stand-in."""
    rb = harness.load(RB)
    repo = tmp_path / "repo"
    (repo / "scripts").mkdir(parents=True)
    log = tmp_path / "calls"
    stand_in(repo / "scripts", "rolling-previous.sh", f'echo "$*" >> "{log}"\n')
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    stand_in(bin_dir, "git", "echo abcd1234\n")
    env = {**harness.runner_env(rb, {"REPO": str(repo)}), "REPO": str(repo)}
    cmd = harness._RUNNER.expand_env(rollback(name)["command"], env)
    r = subprocess.run(["/bin/bash", "-c", cmd], capture_output=True, text=True, timeout=30, check=False,
                       env={"PATH": f"{bin_dir}:/usr/bin:/bin"})
    assert r.returncode == 0, r.stderr
    return log.read_text().splitlines()


@pytest.mark.parametrize("name,host", [("restore-west", "west"), ("restore-east", "east")])
def test_the_rollback_runs_the_restore_for_this_commit(tmp_path, name, host):
    assert run_rollback(tmp_path, name) == [f"restore {host} dist/debs/rollback-abcd1234"]


@pytest.mark.parametrize("name", ["restore-west", "restore-east"])
def test_the_rollback_has_the_time_the_upgrade_has(name):
    assert rollback(name)["timeout"] >= harness.step(harness.load(RB), "upgrade-west")["timeout"]


def applies(name, failed):
    rb = harness.load(RB)
    names = [s["name"] for s in rb["steps"]]
    completed = set(names[:names.index(failed)])
    return harness._RUNNER.rollback_applies(str(rollback(name).get("when", "")), failed, completed)


@pytest.mark.parametrize("failed", MOVED)
def test_east_is_restored_whenever_it_may_have_moved(failed):
    assert applies("restore-east", failed)


@pytest.mark.parametrize("failed", ["upgrade-west", "verify-west-health"])
def test_west_is_restored_whenever_it_may_have_moved(failed):
    assert applies("restore-west", failed)


@pytest.mark.parametrize("failed", ["upgrade-east-first", "verify-east-health"])
def test_west_is_left_alone_while_it_has_not_moved(failed):
    assert not applies("restore-west", failed)


@pytest.mark.parametrize("failed", ["pre-flight-check", "backup-state", "build-new-packages", "record-previous",
                                    "publish-to-repo", "compare-repo-with-build"])
def test_nothing_is_restored_when_no_host_moved(failed):
    assert not applies("restore-west", failed)
    assert not applies("restore-east", failed)


def test_west_goes_back_first_as_it_moved_last():
    assert [e["name"] for e in harness.load(RB)["rollback"]][:2] == ["restore-west", "restore-east"]
