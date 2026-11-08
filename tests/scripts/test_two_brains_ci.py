# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every scripts/two-brains/test_*.py runs somewhere: the static-analysis
workflow or scripts/ci-local.sh. test_thresholds.py ran nowhere until
2026-11-07 and test_author_runbook.py until 2026-11-08."""

from pathlib import Path

REPO = Path(__file__).resolve().parents[2]


def test_each_two_brains_test_is_run():
    ci = (REPO / ".github/workflows/static-analysis.yml").read_text(encoding="utf-8")
    local = (REPO / "scripts/ci-local.sh").read_text(encoding="utf-8")
    tests = sorted((REPO / "scripts/two-brains").glob("test_*.py"))
    assert tests
    unrun = [t.name for t in tests if f"scripts/two-brains/{t.name}" not in ci + local]
    assert not unrun, f"no CI job or ci-local step runs {unrun}"


def test_author_runbook_needs_user_systemd_in_ci_local():
    local = (REPO / "scripts/ci-local.sh").read_text(encoding="utf-8")
    at = local.index("scripts/two-brains/test_author_runbook.py")
    assert "systemd-run --user" in local[max(0, at - 800):at]
