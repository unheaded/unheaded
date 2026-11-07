# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every raft/tests/test_*.py runs in raft-python.yml, or is named here as
left out with its reason. Until 2026-11-07 two files (the Zhen web UI's gate
and the agentd proxy, 101 tests) ran nowhere: the job lists files by name, and
nothing noticed a new one."""
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]

# Left out of the job on purpose. Each needs what CI does not install.
LEFT_OUT = {
    "test_memory_poison.py": "imports zhen_rag, which needs sentence-transformers; the job only compiles it",
}


def pytest_lines():
    wf = yaml.safe_load((REPO / ".github/workflows/raft-python.yml").read_text(encoding="utf-8"))
    runs = [s.get("run", "") for j in wf["jobs"].values() for s in j.get("steps", [])]
    return [line for r in runs for line in r.splitlines() if line.strip().startswith("pytest ")]


def test_every_raft_test_file_runs_or_is_left_out_by_name():
    files = sorted(p.name for p in (REPO / "raft/tests").glob("test_*.py"))
    assert files, "found no raft tests; the glob is wrong"
    listed = {Path(w).name for line in pytest_lines() for w in line.split() if w.endswith(".py")}
    missing = [f for f in files if f not in listed and f not in LEFT_OUT]
    assert not missing, f"raft tests no CI job runs: {missing}; add them to raft-python.yml"


def test_the_left_out_list_is_not_stale():
    for name in LEFT_OUT:
        assert (REPO / "raft/tests" / name).exists(), f"{name} is gone; drop it from LEFT_OUT"
