# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Every relative Markdown link in a tracked .md file points at a file that
exists. On 2026-11-08, 24 did not (docs moved under docs/architecture/, links
written from the repo root). Archived docs are left as they were written; wiki
pages may link a page by name without .md, as GitHub's wiki does."""
import re
import subprocess
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SKIP = ("docs/archive/", "demos/")
FENCE = re.compile(r"```.*?```", re.DOTALL)
CODE = re.compile(r"`[^`\n]*`")
LINK = re.compile(r"\]\(([^)\s#]+)(?:#[^)]*)?\)")


def tracked_markdown():
    out = subprocess.run(["git", "ls-files", "-z", "*.md"], cwd=REPO, capture_output=True, text=True, check=True)
    return [REPO / f for f in out.stdout.split("\0") if f and not f.startswith(SKIP)]


def broken(path):
    text = CODE.sub("", FENCE.sub("", path.read_text(encoding="utf-8", errors="replace")))
    for target in LINK.findall(text):
        if re.match(r"[a-z][a-z0-9+.-]*:", target) or target.startswith("/"):
            continue
        dest = path.parent / target
        if dest.exists():
            continue
        if path.parent.name == "wiki" and (path.parent / f"{target}.md").exists():
            continue
        yield target


def test_relative_links_resolve():
    bad = [f"{p.relative_to(REPO)} -> {t}" for p in tracked_markdown() for t in broken(p)]
    assert not bad, "\n".join(bad)


def test_the_check_sees_a_broken_link(tmp_path):
    page = tmp_path / "page.md"
    page.write_text("[ok](page.md) [gone](nowhere.md) `[code](x.md)` [web](https://example.org)\n")
    assert list(broken(page)) == ["nowhere.md"]
