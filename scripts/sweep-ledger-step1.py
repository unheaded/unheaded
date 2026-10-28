#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""ADR-090 sweep order, step 1: mark generated and vendored files SKIPPED in the ledger.

Run with `python3 -I scripts/sweep-ledger-step1.py [--apply]` from the repository root.

Only PENDING rows move. A row becomes `SKIPPED` with notes `vendored` when its path is under a
vendored tree (VENDORED_PREFIXES, or any `node_modules/` directory), and `SKIPPED` with notes
`generated` when the file exists and is a protobuf `.pb.go` or carries a generator marker in its
first lines (Go's `// Code generated ... DO NOT EDIT.` convention, or `@generated`). SWEPT,
SKIPPED and DEFERRED rows are never touched, and neither is a PENDING row whose file is gone:
what a missing file means is the sweep's call, not step 1's. Every other line is written back
byte for byte. Dry run is the default; `--apply` writes the ledger in place.

The vendored list is ADR-090 open question 1's proposal (S-TRUE-WIRE E5 question 6)."""
import argparse
import collections
import pathlib
import re
import sys

HEADER = ["path", "status", "verdict", "commit", "notes"]
VENDORED_PREFIXES = ("llama.cpp/", "crates/xv6-mbc/upstream/", "doom/", "LICENSES/")
MARKER_LINES = 10
GENERATED_RE = re.compile(r"^\s*(//|#|--|/\*|;|\*)\s*Code generated\b.*\bDO NOT EDIT\.?\s*(\*/)?\s*$")
AT_GENERATED_RE = re.compile(r"^\s*(//|#|--|/\*|;|\*)!?\s*@generated\b")


def classify(path, root):
    """Return "vendored", "generated" or None for a ledger path relative to root."""
    if path.startswith(VENDORED_PREFIXES) or path.startswith("node_modules/") or "/node_modules/" in path:
        return "vendored"
    full = pathlib.Path(root) / path
    if not full.is_file():
        return None
    if path.endswith(".pb.go"):
        return "generated"
    try:
        with open(full, "r", encoding="utf-8", errors="replace") as fh:
            for _, line in zip(range(MARKER_LINES), fh):
                if GENERATED_RE.match(line) or AT_GENERATED_RE.match(line):
                    return "generated"
    except OSError:
        return None
    return None


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    mode = ap.add_mutually_exclusive_group()
    mode.add_argument("--dry-run", action="store_true", help="report only (the default)")
    mode.add_argument("--apply", action="store_true", help="write the ledger in place")
    ap.add_argument("--ledger", default="docs/sweeps/TOTAL-SOURCE-SWEEP.tsv")
    ap.add_argument("--root", default=".", help="tree the ledger paths are relative to")
    args = ap.parse_args(argv)

    ledger = pathlib.Path(args.ledger)
    with open(ledger, "r", encoding="utf-8", newline="") as fh:
        lines = fh.readlines()
    if not lines or lines[0].rstrip("\r\n").split("\t") != HEADER:
        print(f"[FAIL] {ledger}: first line is not the ADR-090 header {HEADER}", file=sys.stderr)
        return 2

    before = collections.Counter()
    after = collections.Counter()
    moved = collections.Counter()
    missing = 0
    out = [lines[0]]
    for line in lines[1:]:
        body = line.rstrip("\r\n")
        ending = line[len(body):]
        cols = body.split("\t")
        if len(cols) != len(HEADER):
            print(f"[FAIL] {ledger}: malformed row {body!r}", file=sys.stderr)
            return 2
        before[cols[1]] += 1
        if cols[1] == "PENDING":
            kind = classify(cols[0], args.root)
            if kind is None and not (pathlib.Path(args.root) / cols[0]).exists():
                missing += 1
            if kind is not None:
                cols = [cols[0], "SKIPPED", "", "", kind]
                moved[kind] += 1
                line = "\t".join(cols) + ending
        after[cols[1]] += 1
        out.append(line)

    label = "applied" if args.apply else "dry run"
    print(f"{label}: generated {moved['generated']} vendored {moved['vendored']} "
          f"(PENDING rows whose file is gone, left PENDING: {missing})")
    for status in ("PENDING", "SWEPT", "SKIPPED", "DEFERRED"):
        print(f"  {status:<8} {before[status]:>5} -> {after[status]:>5}")
    print(f"  total    {sum(before.values()):>5} -> {sum(after.values()):>5}")
    if args.apply and moved:
        tmp = ledger.with_name(ledger.name + ".tmp")
        with open(tmp, "w", encoding="utf-8", newline="") as fh:
            fh.writelines(out)
        tmp.replace(ledger)
    return 0


if __name__ == "__main__":
    sys.exit(main())
