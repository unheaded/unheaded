# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""A host's copy of Wotan's topic config is the repository's file plus that
host's enrolled public keys (scripts/wotan-host-config.py, plan 2 Step 317
option B). The copy must differ from configs/wotan.yaml in the keys lines
only, a key must never be dropped, and --check must see a repository change
that has not reached the host."""
import base64
import importlib.util
import os
import pathlib
import subprocess
import sys

import pytest

REPO = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = REPO / "scripts/wotan-host-config.py"
SOURCE = REPO / "configs/wotan.yaml"

spec = importlib.util.spec_from_file_location("wotan_host_config", SCRIPT)
whc = importlib.util.module_from_spec(spec)
spec.loader.exec_module(whc)


def key(fill):
    """A well-formed stand-in for an ML-DSA-65 public key (right length, not a real key)."""
    return base64.b64encode(bytes([fill]) * whc.MLDSA65_PUBLIC_KEY_BYTES).decode()


def run(tmp_path, keys_text, *extra, source=SOURCE):
    keys = tmp_path / "signing-keys.conf"
    keys.write_text(keys_text, encoding="utf-8")
    out = tmp_path / "wotan.yaml"
    proc = subprocess.run(
        [sys.executable, str(SCRIPT), "--source", str(source), "--keys", str(keys), "--out", str(out), *extra],
        capture_output=True, text=True, check=False,
    )
    return proc, out


def test_repository_file_ships_no_keys():
    """The premise: every signed prefix in the checked-in file has keys: []."""
    assert whc.render(SOURCE.read_text(encoding="utf-8"), {}) == SOURCE.read_text(encoding="utf-8")


def test_only_the_keys_lines_differ(tmp_path):
    a, b = key(1), key(2)
    proc, out = run(tmp_path, f"system.health.consensus. {a}  # akira-health\nsystem.health.remediate. {a}\nalerts.critical. {b}\n")
    assert proc.returncode == 0, proc.stderr
    src = SOURCE.read_text(encoding="utf-8").splitlines()
    got = out.read_text(encoding="utf-8").splitlines()
    assert len(src) == len(got)
    changed = [(s, g) for s, g in zip(src, got) if s != g]
    assert len(changed) == 3
    for s, g in changed:
        assert s.strip().startswith("keys: []")
        assert g.strip().startswith('keys: ["')
        assert len(g) - len(g.lstrip()) == len(s) - len(s.lstrip())  # indentation kept
    text = out.read_text(encoding="utf-8")
    assert text.count(a) == 2 and text.count(b) == 1
    # the prefixes with no enrolled key keep refusing every publish
    assert text.count("keys: []") == SOURCE.read_text(encoding="utf-8").count("keys: []") - 3


def test_two_keys_for_one_prefix(tmp_path):
    a, b = key(3), key(4)
    proc, out = run(tmp_path, f"drift. {a}\ndrift. {b}\n")
    assert proc.returncode == 0, proc.stderr
    assert f'keys: ["{a}", "{b}"]' in out.read_text(encoding="utf-8")


def test_second_run_is_in_step_and_keeps_the_inode(tmp_path):
    proc, out = run(tmp_path, f"drift. {key(5)}\n")
    assert proc.returncode == 0
    inode = out.stat().st_ino
    proc, out = run(tmp_path, f"drift. {key(5)}\n")
    assert proc.returncode == 0 and "in step" in proc.stdout
    proc, out = run(tmp_path, f"drift. {key(6)}\n")  # a rotation rewrites the same file
    assert proc.returncode == 0 and "wrote" in proc.stdout
    assert out.stat().st_ino == inode


def test_check_sees_a_missing_and_a_stale_copy(tmp_path):
    proc, out = run(tmp_path, f"drift. {key(7)}\n", "--check")
    assert proc.returncode == 4 and "missing" in proc.stderr and not out.exists()
    assert run(tmp_path, f"drift. {key(7)}\n")[0].returncode == 0
    assert run(tmp_path, f"drift. {key(7)}\n", "--check")[0].returncode == 0
    # the repository's file gains a permission the host copy does not have yet
    newer = tmp_path / "newer.yaml"
    newer.write_text(SOURCE.read_text(encoding="utf-8") + "\n# a later repository change\n", encoding="utf-8")
    before = out.read_text(encoding="utf-8")
    proc, out = run(tmp_path, f"drift. {key(7)}\n", "--check", source=newer)
    assert proc.returncode == 4 and "DRIFT" in proc.stderr
    assert out.read_text(encoding="utf-8") == before  # --check writes nothing


@pytest.mark.parametrize(
    "keys_text, why",
    [
        ("drift. not-base64!!\n", "base64"),
        ("drift. " + base64.b64encode(b"short").decode() + "\n", "1952"),
        ("drift " + "A" * 8 + "\n", "must end in"),
        ("drift.\n", "field"),
        ("no.such.prefix. KEY\n", "no signed prefix"),
        ("drift. KEY\ndrift. KEY\n", "twice"),
    ],
)
def test_refusals_write_nothing(tmp_path, keys_text, why):
    proc, out = run(tmp_path, keys_text.replace("KEY", key(8)))
    assert proc.returncode == 1, proc.stdout
    assert why in proc.stderr
    assert not out.exists()


def test_source_that_already_lists_a_key_is_refused(tmp_path):
    src = tmp_path / "src.yaml"
    src.write_text(
        SOURCE.read_text(encoding="utf-8").replace(
            '    - prefix: "drift."\n      keys: []', f'    - prefix: "drift."\n      keys: ["{key(9)}"]', 1
        ),
        encoding="utf-8",
    )
    assert "drift." in src.read_text(encoding="utf-8")
    proc, out = run(tmp_path, f"drift. {key(8)}\n", source=src)
    assert proc.returncode == 1 and "already lists keys" in proc.stderr
    assert not out.exists()


def test_unwritable_out_is_an_error_not_a_pass(tmp_path):
    if os.geteuid() == 0:
        pytest.skip("root writes anywhere")
    proc, _ = run(tmp_path, f"drift. {key(8)}\n", "--out", str(tmp_path / "no-such-dir" / "wotan.yaml"))
    assert proc.returncode == 1
