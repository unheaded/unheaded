# SPDX-License-Identifier: GPL-3.0-or-later
"""A rollback's `when` clause: AND within an alternative, || between them."""
import importlib.util
import pathlib

import pytest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("run_runbook", ROOT / "scripts" / "run-runbook.py")
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


@pytest.mark.parametrize(
    "when, failed, completed, expected",
    [
        ("step.a.failed || step.b.failed", "a", set(), True),
        ("step.a.failed || step.b.failed", "b", {"a"}, True),
        ("step.a.failed || step.b.failed", "c", {"a", "b"}, False),
        ("step.a.failed OR step.b.failed", "b", {"a"}, True),
        ("step.a.failed", "a", set(), True),
        ("step.a.failed", "b", {"a"}, False),
        ("step.a.completed AND step.b.failed", "b", {"a"}, True),
        ("step.a.completed AND step.b.failed", "b", set(), False),
        ("step.a.completed AND step.b.failed || step.c.failed", "c", set(), True),
        ("step.a.completed AND step.b.failed || step.c.failed", "b", set(), False),
        ("any step fails", "a", set(), True),
        ("", "a", set(), True),
    ],
)
def test_rollback_applies(when, failed, completed, expected):
    assert RUNNER.rollback_applies(when, failed, completed) is expected


def test_every_or_clause_in_the_tree_can_fire():
    import yaml

    for path in (ROOT / "runbooks").rglob("*.yaml"):
        book = yaml.safe_load(path.read_text())
        rollbacks = (book or {}).get("rollback") or []
        if isinstance(rollbacks, dict):
            rollbacks = rollbacks.get("steps") or []
        for rollback in rollbacks:
            when = str(rollback.get("when", ""))
            if "||" not in when:
                continue
            for step in RUNNER.re.findall(r"step\.([\w-]+)\.failed", when):
                assert RUNNER.rollback_applies(when, step, set()), (path, when)
