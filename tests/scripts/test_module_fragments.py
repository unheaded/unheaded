# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""The platform's monitoring files name nothing of Zhen's; Zhen's rules,
scrape jobs and health targets live in modules/zhen and are read through the
module globs only while the switch is on (ADR-149)."""
import pathlib
import re

import yaml

REPO = pathlib.Path(__file__).resolve().parents[2]
MODULE = REPO / "modules/zhen"

PLATFORM_FILES = [
    "monitoring/prometheus/prometheus.yml",
    "config/victoria/scrape.yml",
    "config/dashboard/services.txt",
    "config/akira/akira.yaml",
    "configs/akira-west.yaml",
]


def code_lines(path):
    """Lines with any # comment removed (a # inside a quoted value is rare in
    these files and never next to a Zhen name)."""
    for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        yield n, line.split("#", 1)[0]


def test_platform_files_name_no_zhen():
    files = [REPO / f for f in PLATFORM_FILES]
    files += sorted((REPO / "monitoring/prometheus/rules").glob("*.yml"))
    found = []
    for f in files:
        for n, line in code_lines(f):
            if re.search(r"zhen|llama", line, re.IGNORECASE):
                found.append(f"{f.relative_to(REPO)}:{n}: {line.strip()}")
    assert not found, "\n".join(found)


def test_every_fragment_exists():
    manifest = yaml.safe_load((MODULE / "module.yaml").read_text(encoding="utf-8"))
    for rel in manifest["fragments"]:
        assert (MODULE / rel).is_file(), rel


def test_scrape_fragments_shape():
    # Prometheus's scrape_config_files: a file with only scrape_configs:.
    for f in sorted(MODULE.glob("prometheus/scrape/*.yml")):
        doc = yaml.safe_load(f.read_text(encoding="utf-8"))
        assert set(doc) == {"scrape_configs"}, f
        jobs = doc["scrape_configs"]
        assert jobs and all(j["job_name"].startswith("zhen") for j in jobs), f
    # VictoriaMetrics's scrape_config_files: a bare list of scrape configs.
    for f in sorted(MODULE.glob("victoria/scrape/*.yml")):
        jobs = yaml.safe_load(f.read_text(encoding="utf-8"))
        assert isinstance(jobs, list) and jobs, f
        assert all(j["job_name"].startswith("zhen") for j in jobs), f


def test_platform_reads_module_globs():
    prom = yaml.safe_load((REPO / "monitoring/prometheus/prometheus.yml").read_text(encoding="utf-8"))
    assert "/etc/unheaded-modules/*/prometheus/rules/*.yml" in prom["rule_files"]
    assert "/etc/unheaded-modules/*/prometheus/scrape/*.yml" in prom["scrape_config_files"]
    vm = yaml.safe_load((REPO / "config/victoria/scrape.yml").read_text(encoding="utf-8"))
    assert "/etc/unheaded-modules/*/victoria/scrape/*.yml" in vm["scrape_config_files"]
