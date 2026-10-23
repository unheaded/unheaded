#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Deterministic tune/hold-out split for the two-brains eval.

int(sha256(id)) % 2: 0 -> holdout, 1 -> tune. Fixed by the id alone, so
it is reproducible and was committed before any result existed.

    python3 scripts/two-brains/split.py eval/two-brains/tasks.jsonl > eval/two-brains/split.jsonl
"""
import hashlib
import json
import sys

for line in open(sys.argv[1], encoding="utf-8"):
    t = json.loads(line)
    h = int(hashlib.sha256(t["id"].encode()).hexdigest(), 16)
    t["split"] = "holdout" if h % 2 == 0 else "tune"
    print(json.dumps(t, sort_keys=True))
