# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/modules/module-fragments.sh publishes a module's fragments under
/run/unheaded/modules/<name> while its switch is on and withdraws them when
it goes off (ADR-149). Prometheus takes a wildcard only in the last component
of a rule_files or scrape_config_files path, so the module's Prometheus files
are also copied to one flat directory per kind, RUN/_prometheus/rules and
RUN/_prometheus/scrape, as <name>.<file>. Run against directories in tmp_path."""
import os
import pathlib
import shutil
import subprocess
import time

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/modules/module-fragments.sh"


@pytest.fixture
def fx(tmp_path):
    src = tmp_path / "src"
    for rel, text in {
        "zhen/module.yaml": "name: zhen\n",
        "zhen/prometheus/rules/zhen.yml": "groups: []\n",
        "zhen/victoria/scrape/zhen.yml": "[]\n",
        "zhen/dashboard/services.txt": "zhenai=127.0.0.1:1\n",
        "zhen/hosts/west/x.conf": "[Service]\n",
    }.items():
        p = src / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)
    run = tmp_path / "run"
    run.mkdir()
    tf = tmp_path / "textfile"
    tf.mkdir()
    env = dict(os.environ, MODULES_SRC=str(src), MODULES_RUN=str(run), MODULES_TEXTFILE=str(tf))
    return {"src": src, "run": run, "tf": tf, "env": env}


def call(fx, *args):
    return subprocess.run(["bash", str(SCRIPT), *args], env=fx["env"], capture_output=True,
                          text=True, timeout=30, check=False)


def tree(root):
    return sorted(str(p.relative_to(root)) for p in root.rglob("*"))


def prom(fx):
    out = {}
    for line in (fx["tf"] / "module_zhen.prom").read_text().splitlines():
        if line and not line.startswith("#"):
            k, v = line.rsplit(" ", 1)
            out[k] = float(v)
    return out


def no_temp(fx):
    dirs = [fx["run"], fx["tf"]] + [d for d in (fx["run"] / "_prometheus").glob("*") if d.is_dir()]
    for d in dirs:
        names = [p.name for p in d.iterdir()]
        assert not [n for n in names if n.startswith((".zhen", ".module_"))], names


def flat(fx):
    """The flat directories' entries, as kind/name."""
    root = fx["run"] / "_prometheus"
    return sorted(str(p.relative_to(root)) for p in root.glob("*/*"))


def add_module(fx, name, files):
    for rel, text in files.items():
        p = fx["src"] / name / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(text)


def test_publish(fx):
    r = call(fx, "publish", "zhen")
    assert r.returncode == 0, r.stderr
    z = fx["run"] / "zhen"
    assert tree(z) == sorted([
        "dashboard", "dashboard/services.txt",
        "prometheus", "prometheus/rules", "prometheus/rules/zhen.yml",
        "victoria", "victoria/scrape", "victoria/scrape/zhen.yml",
    ])
    for p in z.rglob("*"):
        assert (p.stat().st_mode & 0o777) == (0o755 if p.is_dir() else 0o644), p
    assert (z.stat().st_mode & 0o777) == 0o755
    m = prom(fx)
    assert m['unheaded_module_enabled{module="zhen"}'] == 1
    assert abs(m['unheaded_module_changed_timestamp_seconds{module="zhen"}'] - time.time()) <= 5
    no_temp(fx)


def test_publish_again_takes_the_new_text(fx):
    assert call(fx, "publish", "zhen").returncode == 0
    (fx["src"] / "zhen/prometheus/rules/zhen.yml").write_text("groups: [changed]\n")
    assert call(fx, "publish", "zhen").returncode == 0
    assert (fx["run"] / "zhen/prometheus/rules/zhen.yml").read_text() == "groups: [changed]\n"
    no_temp(fx)


def test_withdraw(fx):
    assert call(fx, "publish", "zhen").returncode == 0
    r = call(fx, "withdraw", "zhen")
    assert r.returncode == 0, r.stderr
    assert not (fx["run"] / "zhen").exists()
    assert prom(fx)['unheaded_module_enabled{module="zhen"}'] == 0
    r = call(fx, "withdraw", "zhen")
    assert r.returncode == 0
    assert prom(fx)['unheaded_module_enabled{module="zhen"}'] == 0
    no_temp(fx)


def test_symlink_is_not_copied(fx):
    (fx["src"] / "zhen/prometheus/rules/link.yml").symlink_to("/etc/passwd")
    assert call(fx, "publish", "zhen").returncode == 0
    assert not (fx["run"] / "zhen/prometheus/rules/link.yml").exists()


def test_publish_copies_prometheus_files_to_the_flat_directories(fx):
    add_module(fx, "zhen", {
        "prometheus/scrape/zhen.yml": "scrape_configs: []\n",
        "prometheus/rules/tests/zhen_test.yml": "tests: []\n",
        "prometheus/rules/notes.txt": "not a rule file\n",
    })
    (fx["src"] / "zhen/prometheus/rules/link.yml").symlink_to("/etc/passwd")
    assert call(fx, "publish", "zhen").returncode == 0
    # Only the *.yml directly under rules/ and scrape/: what Prometheus loads.
    assert flat(fx) == ["rules/zhen.zhen.yml", "scrape/zhen.zhen.yml"]
    root = fx["run"] / "_prometheus"
    for rel in flat(fx):
        p = root / rel
        assert p.is_file() and not p.is_symlink(), p
        assert (p.stat().st_mode & 0o777) == 0o644, p
    for d in (root, root / "rules", root / "scrape"):
        assert (d.stat().st_mode & 0o777) == 0o755, d
    assert (root / "rules/zhen.zhen.yml").read_text() == "groups: []\n"
    assert (root / "scrape/zhen.zhen.yml").read_text() == "scrape_configs: []\n"
    # The module's own directory is as before: it is how everything else
    # (pkg/modules, VictoriaMetrics, the dashboard) reads the module.
    assert (fx["run"] / "zhen/prometheus/rules/zhen.yml").is_file()
    no_temp(fx)


def test_publish_again_updates_the_flat_copies(fx):
    add_module(fx, "zhen", {"prometheus/rules/extra.yml": "groups: [extra]\n"})
    assert call(fx, "publish", "zhen").returncode == 0
    assert flat(fx) == ["rules/zhen.extra.yml", "rules/zhen.zhen.yml"]
    (fx["src"] / "zhen/prometheus/rules/extra.yml").unlink()
    (fx["src"] / "zhen/prometheus/rules/zhen.yml").write_text("groups: [changed]\n")
    assert call(fx, "publish", "zhen").returncode == 0
    assert flat(fx) == ["rules/zhen.zhen.yml"]
    assert (fx["run"] / "_prometheus/rules/zhen.zhen.yml").read_text() == "groups: [changed]\n"
    no_temp(fx)


def test_flat_copies_of_two_modules_do_not_touch_each_other(fx):
    # "zhen-x" starts with "zhen": withdrawing or republishing zhen must leave it.
    add_module(fx, "zhen-x", {"prometheus/rules/zhen.yml": "groups: [x]\n",
                              "prometheus/scrape/x.yml": "scrape_configs: []\n"})
    assert call(fx, "publish", "zhen").returncode == 0
    assert call(fx, "publish", "zhen-x").returncode == 0
    both = ["rules/zhen-x.zhen.yml", "rules/zhen.zhen.yml", "scrape/zhen-x.x.yml"]
    assert flat(fx) == both
    assert call(fx, "publish", "zhen").returncode == 0
    assert flat(fx) == both
    assert call(fx, "withdraw", "zhen").returncode == 0
    assert flat(fx) == ["rules/zhen-x.zhen.yml", "scrape/zhen-x.x.yml"]
    assert (fx["run"] / "_prometheus/rules/zhen-x.zhen.yml").read_text() == "groups: [x]\n"
    assert call(fx, "withdraw", "zhen-x").returncode == 0
    assert flat(fx) == []


def test_withdraw_removes_the_flat_copies(fx):
    add_module(fx, "zhen", {"prometheus/scrape/zhen.yml": "scrape_configs: []\n"})
    assert call(fx, "publish", "zhen").returncode == 0
    assert flat(fx) == ["rules/zhen.zhen.yml", "scrape/zhen.zhen.yml"]
    assert call(fx, "withdraw", "zhen").returncode == 0
    assert flat(fx) == []
    # Nothing of the module is left: the flat directories stay, empty.
    assert tree(fx["run"]) == ["_prometheus", "_prometheus/rules", "_prometheus/scrape"]
    assert call(fx, "withdraw", "zhen").returncode == 0
    no_temp(fx)


def test_withdraw_removes_flat_copies_whose_module_directory_is_gone(fx):
    # Someone removed RUN/zhen by hand: withdraw still clears Prometheus's copies.
    assert call(fx, "publish", "zhen").returncode == 0
    shutil.rmtree(fx["run"] / "zhen")
    assert call(fx, "withdraw", "zhen").returncode == 0
    assert flat(fx) == []


@pytest.mark.parametrize("args,code", [
    (["publish", "../x"], 2), (["publish", "Zhen"], 2), (["publish", ""], 2),
    (["publish"], 2), (["frob", "zhen"], 2), (["publish", "nosuch"], 1),
])
def test_refusals_change_nothing(fx, args, code):
    before = (tree(fx["run"]), tree(fx["tf"]))
    r = call(fx, *args)
    assert r.returncode == code, (args, r.returncode, r.stderr)
    assert (tree(fx["run"]), tree(fx["tf"])) == before
