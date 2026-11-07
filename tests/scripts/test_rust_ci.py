# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every Rust crate with tests has them run by a CI job. Until 2026-11-07
five crates' tests (upc-api, upc-bootstub, upc-cartridge, xv6-mbc, zhend)
ran nowhere: clippy built them, nothing ran them."""
import subprocess
from pathlib import Path

import yaml

REPO = Path(__file__).resolve().parents[2]

# Run some other way, named with how.
ELSEWHERE = {
    "cmd/ebpf-collector/collector": "a member of cmd/ebpf-collector's workspace, tested by ebpf.yml",
    "cmd/ebpf-collector/common": "a member of cmd/ebpf-collector's workspace, tested by ebpf.yml",
}


def crates_with_tests():
    tracked = subprocess.run(["git", "ls-files", "*Cargo.toml"], cwd=REPO, check=True,
                             capture_output=True, text=True).stdout.split()
    out = []
    for toml in tracked:
        d = Path(toml).parent
        if (d.parts and d.parts[0] == "ebpf") or str(d) == ".":
            continue  # ebpf/ forces the BPF target: test_ebpf_host_ci.py holds its job
        if any("#[test]" in p.read_text(encoding="utf-8", errors="replace")
               for p in (REPO / d).rglob("*.rs") if "target" not in p.parts):
            out.append(str(d))
    return sorted(out)


def dirs_with_a_test_job():
    found = set()
    for wf in (REPO / ".github/workflows").glob("*.yml"):
        doc = yaml.safe_load(wf.read_text(encoding="utf-8"))
        for job in (doc.get("jobs") or {}).values():
            matrix = (job.get("strategy") or {}).get("matrix") or {}
            for step in job.get("steps", []):
                run = step.get("run", "")
                if "cargo test" not in run:
                    continue
                wd = step.get("working-directory", "")
                if wd.startswith("${{ matrix."):
                    key = wd[len("${{ matrix."):].rstrip("} ").strip()
                    found.update(matrix.get(key, []))
                elif wd:
                    found.add(wd)
                for word in run.split():
                    if word.endswith("Cargo.toml"):
                        found.add(str(Path(word).parent))
    return found


def test_every_crate_with_tests_has_a_job():
    crates = crates_with_tests()
    assert "crates/upc-api" in crates, "the crate scan found too little"
    missing = [c for c in crates if c not in dirs_with_a_test_job() and c not in ELSEWHERE]
    assert not missing, f"Rust crates whose tests no CI job runs: {missing}"
