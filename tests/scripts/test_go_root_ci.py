# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The Go tests that skip unless root run somewhere: scripts/test-go-root.sh,
in the eBPF workflow's load-gate job and in scripts/ci-local.sh. A change to
any package that holds one starts that workflow."""
import subprocess
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
SKIP_MESSAGE = "needs root (CAP_BPF)"


def root_test_packages():
    out = subprocess.run(
        ["git", "grep", "-l", SKIP_MESSAGE, "--", "*_test.go"], cwd=REPO, capture_output=True, text=True, check=True
    )
    return sorted({str(Path(f).parent) for f in out.stdout.split()})


def workflow():
    return yaml.safe_load((REPO / ".github/workflows/ebpf.yml").read_text(encoding="utf-8"))


def test_the_script_searches_for_the_same_message():
    assert SKIP_MESSAGE in (REPO / "scripts/test-go-root.sh").read_text(encoding="utf-8")


def test_root_tests_exist():
    assert "tests/hopchecks" in root_test_packages()


def test_the_load_gate_job_runs_the_script_after_building_objects():
    steps = [s.get("run", "") for s in workflow()["jobs"]["ebpf-load-gate"]["steps"]]
    loads = next(i for i, r in enumerate(steps) if "check-ebpf-loads.sh" in r)
    root = next(i for i, r in enumerate(steps) if "test-go-root.sh" in r)
    assert loads < root


def test_a_change_to_a_root_test_package_starts_the_workflow():
    on = workflow().get("on") or workflow()[True]  # YAML 1.1 reads `on` as true
    for event in ("push", "pull_request"):
        paths = on[event]["paths"]
        for pkg in root_test_packages():
            assert f"{pkg}/**" in paths, f"{event}: {pkg}"
        assert "scripts/test-go-root.sh" in paths


def test_ci_local_runs_the_script():
    assert "scripts/test-go-root.sh" in (REPO / "scripts/ci-local.sh").read_text(encoding="utf-8")
