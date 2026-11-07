# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every Rust loader's object_trust tests run somewhere on their own: a CI job
that starts when the loader changes, `make test-rust`, and scripts/ci-local.sh.
Those tests are what notices a loader that stopped asking, or a unit repointed
at a directory its own account can write."""
import re
from pathlib import Path

import pytest
import yaml

REPO = Path(__file__).resolve().parents[2]
UNIT = "deploy/systemd/unheaded-upc-bootctl@.service"
LOADERS = sorted({"/".join(p.relative_to(REPO).parts[:2]) for p in REPO.glob("cmd/**/tests/object_trust.rs")
                  if "target" not in p.parts})


def workflow():
    return yaml.safe_load((REPO / ".github/workflows/ebpf.yml").read_text(encoding="utf-8"))


def triggers(wf):
    return wf.get("on", wf.get(True))  # YAML 1.1 reads the key `on` as true


def test_the_loaders_are_found():
    assert LOADERS == ["cmd/ebpf-collector", "cmd/ebpf-load-check", "cmd/ebpf-loader", "cmd/upc-bootctl"]


def test_a_job_runs_each_loaders_tests():
    job = workflow()["jobs"]["loader-object-trust"]
    assert sorted(job["strategy"]["matrix"]["loader"]) == LOADERS
    assert job["strategy"]["fail-fast"] is False, "one loader's failure hid the others"
    run = [s for s in job["steps"] if "cargo test --locked" in s.get("run", "")]
    assert len(run) == 1
    assert run[0]["working-directory"] == "${{ matrix.loader }}"
    assert not job.get("continue-on-error") and not run[0].get("continue-on-error")


def test_the_job_installs_what_the_collector_builds_with():
    job = workflow()["jobs"]["loader-object-trust"]
    assert any("protobuf-compiler" in s.get("run", "") for s in job["steps"])


@pytest.mark.parametrize("event", ["push", "pull_request"])
def test_a_change_to_a_loader_or_the_unit_starts_the_job(event):
    paths = triggers(workflow())[event]["paths"]
    for loader in LOADERS:
        assert f"{loader}/**" in paths
    assert UNIT in paths
    assert "crates/objtrust/**" in paths


def test_make_test_rust_covers_each_loader():
    makefile = (REPO / "Makefile").read_text(encoding="utf-8")
    prereqs = re.search(r"^test-rust:([^#\n]*)", makefile, re.MULTILINE).group(1).split()
    for loader in LOADERS:
        target = "test-rust-" + loader.removeprefix("cmd/")
        assert target in prereqs
        assert re.search(rf"^{target}:.*\n\tcd {loader} && cargo test --locked$", makefile, re.MULTILINE), target


def test_ci_local_covers_each_loader():
    # West runs this; GitHub's eBPF & Rust workflow may not be on.
    text = (REPO / "scripts/ci-local.sh").read_text(encoding="utf-8")
    block = re.search(r"^\s*for ws in ([^;]*); do\n[^\n]*cargo test --locked", text, re.MULTILINE)
    assert block, "no loop runs cargo test --locked"
    assert sorted(block.group(1).split()) == LOADERS
