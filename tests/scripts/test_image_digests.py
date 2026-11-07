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
