# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The ebpf/ common crates' unit tests run in CI. They cannot run with a plain
`cargo test` inside ebpf/ (the workspace forces the BPF target), so until
2026-11-07 no job ran them; `make test-rust-ebpf-common` is the recipe."""
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]


def test_a_job_runs_the_ebpf_common_host_tests():
    wf = yaml.safe_load((REPO / ".github/workflows/ebpf.yml").read_text(encoding="utf-8"))
    hits = [(j, s) for j in wf["jobs"].values() for s in j.get("steps", [])
            if "make test-rust-ebpf-common" in s.get("run", "")]
    assert len(hits) == 1, "no ebpf.yml step runs make test-rust-ebpf-common"
    job, step = hits[0]
    assert not job.get("continue-on-error") and not step.get("continue-on-error")


def test_the_recipe_still_covers_the_three_crates():
    mk = (REPO / "Makefile").read_text(encoding="utf-8")
    assert "for crate in common monad-common pqc-common" in mk
