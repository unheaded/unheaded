# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""All twelve SLH-DSA parameter sets run somewhere. The race run skips the
slow ones (PQC_SLOW); until 2026-11-08 nothing set it, so ten of twelve
sets had never been signed and verified in CI."""

from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]
TEST = "TestSLHDSAAllParameterSets"


def test_the_test_still_skips_without_the_variable():
    src = (REPO / "pkg/crypto/pqc/pqc_test.go").read_text(encoding="utf-8")
    assert f"func {TEST}(" in src
    assert 'os.Getenv("PQC_SLOW") == "1"' in src


def test_ci_runs_every_set_and_fails_on_a_skip():
    jobs = yaml.safe_load((REPO / ".github/workflows/ci.yml").read_text(encoding="utf-8"))["jobs"]
    runs = [s.get("run", "") for s in jobs["go-build"]["steps"]]
    step = next(r for r in runs if "PQC_SLOW=1" in r)
    assert TEST in step
    assert "--- SKIP" in step
    assert "-eq 12" in step


def test_ci_local_runs_every_set():
    text = (REPO / "scripts/ci-local.sh").read_text(encoding="utf-8")
    assert "PQC_SLOW=1" in text and TEST in text
