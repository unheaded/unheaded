#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Compare two outputs of scripts/well/zhen-schema-fingerprint.sql, unheaded_app's
(before 053) and unheaded_zhen's (after 052), against the deliberate differences
in scripts/well/zhen-schema-changes.txt (ADR-149; S-ZHEN-SWITCH Steps 191, 279).

Passes (exit 0) when every fingerprint line that differs is named there by its
`kind|name`, and every name there differs. Anything else is drift between the
migrations and the database, or a stale list: exit 1, one line per cause.

    python3 -I scripts/well/zhen_schema_diff.py fp-app.txt fp-zhen.txt [changes.txt]
"""
import pathlib
import sys

DEFAULT_CHANGES = pathlib.Path(__file__).resolve().with_name("zhen-schema-changes.txt")


def read_changes(path):
    names = set()
    for raw in pathlib.Path(path).read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if line.count("|") != 1:
            raise ValueError(f"{path}: not kind|name: {line!r}")
        names.add(line)
    return names


def read_fingerprint(path):
    return {line for line in pathlib.Path(path).read_text(encoding="utf-8").splitlines() if line}


def key(line):
    return "|".join(line.split("|", 2)[:2])


def compare(app_lines, zhen_lines, changes):
    """Return the problems, an empty list when the fingerprints differ exactly by `changes`."""
    differing = (app_lines - zhen_lines) | (zhen_lines - app_lines)
    problems = [f"unexpected: {'<' if line in app_lines else '>'} {line}"
                for line in sorted(differing) if key(line) not in changes]
    seen = {key(line) for line in differing}
    problems += [f"listed but the same in both: {name}" for name in sorted(changes - seen)]
    return problems


def main(argv):
    if len(argv) not in (3, 4):
        print(__doc__.strip().splitlines()[-1].strip(), file=sys.stderr)
        return 2
    changes = read_changes(argv[3] if len(argv) == 4 else DEFAULT_CHANGES)
    problems = compare(read_fingerprint(argv[1]), read_fingerprint(argv[2]), changes)
    for p in problems:
        print(p)
    if problems:
        return 1
    print(f"same but for the {len(changes)} listed change(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
