# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""scripts/modules/module-fragments.sh publishes a module's fragments under
/run/unheaded/modules/<name> while its switch is on and withdraws them when
it goes off (ADR-149). Run against directories in tmp_path."""
import os
import pathlib
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
    for d in (fx["run"], fx["tf"]):
        names = [p.name for p in d.iterdir()]
        assert not [n for n in names if n.startswith(".zhen") or n.startswith(".module_")], names


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


@pytest.mark.parametrize("args,code", [
    (["publish", "../x"], 2), (["publish", "Zhen"], 2), (["publish", ""], 2),
    (["publish"], 2), (["frob", "zhen"], 2), (["publish", "nosuch"], 1),
])
def test_refusals_change_nothing(fx, args, code):
    before = (tree(fx["run"]), tree(fx["tf"]))
    r = call(fx, *args)
    assert r.returncode == code, (args, r.returncode, r.stderr)
    assert (tree(fx["run"]), tree(fx["tf"])) == before
