# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""references/timeline.{json,yaml,toml} follow references/timeline.md (S-RESIDUE Steps 42-59, D-26).

The three mirrors were kept by hand and fell two entries behind the markdown.
scripts/timeline-mirrors.py now writes them (--write) and says when they have drifted (--check).
These tests pin both, on the committed tree and on copies with a planted change."""
import datetime
import hashlib
import json
import pathlib
import shutil
import subprocess
import sys
import tomllib

import pytest
import yaml

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "timeline-mirrors.py"
FILES = ["timeline.md", "timeline.json", "timeline.yaml", "timeline.toml"]
ANCHOR = "**Completed sub-items:**\n"
# Characters that have broken a hand-written emitter before: quotes, a backslash,
# YAML's colon-space and hash, non-ASCII, and TOML's escapes.
NASTY = '**A "quoted" entry: with \\back, #hash, ≥ and → (2026-11-12, `develop`).** It\'s {done}; a\ttab.'


def run(directory, *args):
    return subprocess.run(
        [sys.executable, str(SCRIPT), *args, "--dir", str(directory)],
        capture_output=True, text=True, check=False,
    )


def item_id(text, phase=3):
    return f"phase-{phase}-item-{hashlib.sha256(text.encode()).hexdigest()[:8]}"


def instant(value):
    """last_updated as an aware datetime, whichever parser produced it."""
    if isinstance(value, str):
        value = datetime.datetime.fromisoformat(value)
    if value.tzinfo is None:
        value = value.replace(tzinfo=datetime.UTC)
    return value


def load_all(directory):
    with open(directory / "timeline.json", encoding="utf-8") as fh:
        as_json = json.load(fh)
    with open(directory / "timeline.yaml", encoding="utf-8") as fh:
        as_yaml = yaml.safe_load(fh)
    with open(directory / "timeline.toml", "rb") as fh:
        as_toml = tomllib.load(fh)
    for data in (as_json, as_yaml, as_toml):
        data["last_updated"] = instant(data["last_updated"])
    return as_json, as_yaml, as_toml


def add_bullet(directory, text, anchor=ANCHOR):
    path = directory / "timeline.md"
    body = path.read_text(encoding="utf-8")
    assert body.count(anchor) == 1
    path.write_text(body.replace(anchor, anchor + "- " + text + "\n"), encoding="utf-8")


@pytest.fixture
def tree(tmp_path):
    """A copy of references/ with the mirrors written, so each test starts in step."""
    for name in FILES:
        shutil.copy(ROOT / "references" / name, tmp_path / name)
    done = run(tmp_path, "--write")
    assert done.returncode == 0, done.stdout + done.stderr
    assert run(tmp_path, "--check").returncode == 0
    return tmp_path


def test_check_passes_on_the_committed_tree():
    got = run(ROOT / "references", "--check")
    assert got.returncode == 0, got.stdout + got.stderr


def test_check_is_the_default_and_writes_nothing(tree):
    add_bullet(tree, "A new entry (2026-11-12)")
    before = {name: (tree / name).read_bytes() for name in FILES}
    got = run(tree)
    assert got.returncode == 1
    assert before == {name: (tree / name).read_bytes() for name in FILES}


def test_a_new_bullet_fails_the_check_and_is_named(tree):
    text = "**A planted entry (2026-11-12, `develop`).** It did a thing."
    add_bullet(tree, text)
    got = run(tree, "--check")
    assert got.returncode == 1, got.stdout
    assert item_id(text) in got.stdout
    assert "timeline.md:" in got.stdout


def test_write_adds_the_bullet_to_all_three_mirrors(tree):
    text = "**A planted entry (2026-11-12, `develop`).** It did a thing."
    add_bullet(tree, text)
    before = json.loads((tree / "timeline.json").read_text(encoding="utf-8"))
    got = run(tree, "--write")
    assert got.returncode == 0, got.stdout + got.stderr
    want = {"id": item_id(text), "name": "A planted entry", "status": "completed", "progress": 100, "description": text}
    for data in load_all(tree):
        assert [m for m in data["milestones"] if m["id"] == want["id"]] == [want]
        assert len(data["milestones"]) == len(before["milestones"]) + 1
        phase3 = next(p for p in data["phases"] if p["id"] == "phase-3")
        assert phase3["milestones"][0] == want["id"]
        assert phase3["milestones"] == [m["id"] for m in data["milestones"] if m["id"].startswith("phase-3-")]
    assert load_all(tree)[0]["last_updated"] > instant(before["last_updated"])
    assert run(tree, "--check").returncode == 0


def test_a_bullet_under_remaining_is_pending(tree):
    text = "A thing still to do (someday)"
    add_bullet(tree, text, anchor="**Remaining for Age 3:**\n")
    assert run(tree, "--write").returncode == 0
    for data in load_all(tree):
        got = next(m for m in data["milestones"] if m["id"] == item_id(text))
        assert (got["name"], got["status"], got["progress"]) == ("A thing still to do", "pending", 0)


def test_the_three_mirrors_parse_to_the_same_data(tree):
    add_bullet(tree, NASTY)
    assert run(tree, "--write").returncode == 0
    as_json, as_yaml, as_toml = load_all(tree)
    assert as_json == as_yaml
    assert as_json == as_toml
    got = next(m for m in as_toml["milestones"] if m["id"] == item_id(NASTY))
    assert got["description"] == NASTY
    assert got["name"] == 'A "quoted" entry: with \\back, #hash, ≥ and →'


def test_write_twice_changes_nothing(tree):
    before = {name: (tree / name).read_bytes() for name in FILES}
    got = run(tree, "--write")
    assert got.returncode == 0
    assert before == {name: (tree / name).read_bytes() for name in FILES}


def test_changed_bullet_text_is_reported_not_duplicated(tree):
    path = tree / "timeline.md"
    old = "S78 LOC audit (415K production, 1,137K total)"
    new = "S78 LOC audit (415K production, 1,140K total)"
    body = path.read_text(encoding="utf-8")
    assert body.count("- " + old + "\n") == 1
    path.write_text(body.replace("- " + old + "\n", "- " + new + "\n"), encoding="utf-8")
    count = len(json.loads((tree / "timeline.json").read_text(encoding="utf-8"))["milestones"])

    got = run(tree, "--check")
    assert got.returncode == 1
    assert "text changed" in got.stdout
    assert item_id(old) in got.stdout
    assert item_id(new) in got.stdout

    got = run(tree, "--write")
    assert got.returncode == 0, got.stdout + got.stderr
    assert item_id(old) in got.stdout
    for data in load_all(tree):
        ids = [m["id"] for m in data["milestones"]]
        assert len(ids) == count
        assert item_id(new) in ids
        assert item_id(old) not in ids
        assert next(m for m in data["milestones"] if m["id"] == item_id(new))["description"] == new


def test_a_removed_bullet_is_reported_and_leaves_the_mirrors_on_write(tree):
    path = tree / "timeline.md"
    gone = "README polish"
    body = path.read_text(encoding="utf-8")
    assert body.count("- " + gone + "\n") == 1
    path.write_text(body.replace("- " + gone + "\n", ""), encoding="utf-8")
    got = run(tree, "--check")
    assert got.returncode == 1
    assert item_id(gone) in got.stdout
    assert run(tree, "--write").returncode == 0
    for data in load_all(tree):
        assert item_id(gone) not in [m["id"] for m in data["milestones"]]


@pytest.mark.parametrize("name", ["timeline.yaml", "timeline.toml"])
def test_a_hand_edited_mirror_fails_the_check(tree, name):
    path = tree / name
    body = path.read_text(encoding="utf-8")
    assert "README polish" in body
    path.write_text(body.replace("README polish", "README varnish"), encoding="utf-8")
    got = run(tree, "--check")
    assert got.returncode == 1
    assert name in got.stdout


def test_a_mirror_that_does_not_parse_fails_the_check(tree):
    (tree / "timeline.toml").write_text("version = \n", encoding="utf-8")
    got = run(tree, "--check")
    assert got.returncode == 1
    assert "timeline.toml" in got.stdout
