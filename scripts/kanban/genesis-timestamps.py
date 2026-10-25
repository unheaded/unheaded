#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Give every Genesis card the dates git knows for it (S-TWO-BRAINS Step 418).

    python3 scripts/kanban/genesis-timestamps.py [db/seed/kanban-genesis.json]

The Genesis seed used to invent its times (time.Now() minus N days at
startup). Here each card's

  created_at = committer date of the first commit that added `ID: "<id>"`
               to cmd/kanban-app/main.go (git log -S, oldest first);
  updated_at = committer date of the newest commit whose diff touched a
               line holding `ID: "<id>"` there (git log -G), i.e. the
               card's last edit,

both in UTC. History is read up to the last commit in which main.go still
held getInitialTasks() (HEAD while it does; the parent of the commit that
removed it afterwards), so the result is the same on every run. The file
is rewritten in place (2-space JSON, sorted by id, UTF-8). A card git
cannot date is an error.
"""
import json
import subprocess
import sys
from datetime import UTC, datetime
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SRC = "cmd/kanban-app/main.go"


def git(*args: str) -> str:
    return subprocess.run(["git", "-C", str(REPO), *args], check=True, capture_output=True, text=True).stdout


def history_rev() -> str:
    head = git("show", f"HEAD:{SRC}")
    if "func getInitialTasks" in head:
        return "HEAD"
    removed = git("log", "-1", "--format=%H", "-S", "func getInitialTasks", "HEAD", "--", SRC).strip()
    if not removed:
        sys.exit("genesis-timestamps: no commit ever held getInitialTasks")
    return removed + "^"


def utc(iso: str) -> str:
    return datetime.fromisoformat(iso).astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def dates(rev: str, card_id: str) -> tuple:
    needle = f'ID: "{card_id}"'
    added = git("log", "--reverse", "--format=%cI", "-S", needle, rev, "--", SRC).split()
    touched = git("log", "--format=%cI", "-G", needle.replace(".", r"\."), rev, "--", SRC).split()
    if not added or not touched:
        sys.exit(f"genesis-timestamps: git has no commit adding {needle} to {SRC}")
    return utc(added[0]), utc(touched[0])


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else REPO / "db/seed/kanban-genesis.json")
    cards = json.loads(path.read_text())
    rev = history_rev()
    first = utc(git("log", "--reverse", "--format=%cI", rev).split()[0])
    for c in cards:
        c["created_at"], c["updated_at"] = dates(rev, c["id"])
        if not first <= c["created_at"] <= c["updated_at"]:
            sys.exit(f"genesis-timestamps: {c['id']}: dates out of order ({c['created_at']} .. {c['updated_at']})")
    cards.sort(key=lambda c: c["id"])
    path.write_text(json.dumps(cards, indent=2, ensure_ascii=False) + "\n")
    print(f"{len(cards)} cards dated from git history up to {rev}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
