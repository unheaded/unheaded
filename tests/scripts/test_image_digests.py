# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Compose images name their bytes, with one exception that says why: the Well's
postgres stays on its tag until a restore drill has passed on the new bytes.
check-image-digests.sh holds the rule; this holds the exception to one row."""
import re
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
BASELINE = REPO / "docs/policy/image-digest-baseline.txt"
COMPOSE = REPO / "docker-compose.yml"
MONITORING = REPO / "monitoring/docker-compose.yml"
LOADBALANCERS = REPO / "docker/docker-compose.loadbalancers.yml"
GATE = REPO / "scripts/check-image-digests.sh"
DIGEST = r"@sha256:[0-9a-f]{64}"


def rows():
    return [line for line in BASELINE.read_text(encoding="utf-8").splitlines() if line and not line.startswith("#")]


def test_the_wells_postgres_is_the_only_image_without_a_digest():
    assert rows() == ["docker-compose.yml postgres:16.15-alpine"]


def test_the_baseline_says_what_lets_postgres_go():
    assert "restore drill" in BASELINE.read_text(encoding="utf-8")


def test_the_well_stays_on_its_tag_and_its_line_says_why():
    line = next(line for line in COMPOSE.read_text(encoding="utf-8").splitlines() if "image: postgres:" in line)
    assert "@sha256" not in line
    assert "drill" in line


def test_alpine_carries_one_digest_in_both_files():
    found = {m for f in (COMPOSE, MONITORING)
             for m in re.findall(rf"image: (alpine:3\.24{DIGEST})", f.read_text(encoding="utf-8"))}
    assert len(found) == 1


def test_every_load_balancer_image_names_its_bytes():
    """haproxy x2 and nginx x4 are all third-party: none is built here, so
    each carries the digest the registry serves (ADR-102, Update 2026-11-15)."""
    images = re.findall(r"^\s*image:\s*(\S+)", LOADBALANCERS.read_text(encoding="utf-8"), re.MULTILINE)
    assert len(images) == 6  # nginx-monad and nginx-sophia went with the role services (ADR-147 decision 7)
    assert [i for i in images if not re.search(rf"{DIGEST}$", i)] == []
    # One digest per tag: the same tag never names two builds.
    by_tag = {}
    for i in images:
        tag, digest = i.split("@")
        by_tag.setdefault(tag, set()).add(digest)
    assert sorted(by_tag) == ["haproxy:2.9.15-alpine", "nginx:1.25.5-alpine"]
    assert all(len(d) == 1 for d in by_tag.values())


def test_the_gate_reads_the_load_balancer_file():
    assert "docker/docker-compose.loadbalancers.yml" in re.search(r"^files=\((.*)\)$", GATE.read_text(encoding="utf-8"), re.MULTILINE).group(1)
