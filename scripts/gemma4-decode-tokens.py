#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
"""
WAVE13 Phase 1: decode a token list back to text for forge generate.

Reads a JSON token list on stdin (or as the first argument), prints decoded
text on stdout. Skips special tokens by default; `--all` to include them.

Usage:
    echo '[3689, 563, 649, 103653, 236881]' | scripts/gemma4-decode-tokens.py
    scripts/gemma4-decode-tokens.py '[3689, 563]'
"""
import json
import sys

from transformers import AutoTokenizer


def main() -> int:
    skip_special = "--all" not in sys.argv

    arg = next((a for a in sys.argv[1:] if not a.startswith("--")), None)
    if arg is None:
        raw = sys.stdin.read().strip()
    else:
        raw = arg

    if not raw:
        return 2

    try:
        ids = json.loads(raw)
        if not isinstance(ids, list):
            raise ValueError("expected a JSON array of ints")
        ids = [int(x) for x in ids]
    except (ValueError, json.JSONDecodeError) as e:
        print(f"parse: {e}", file=sys.stderr)
        return 2

    tok = AutoTokenizer.from_pretrained(
        "/home/govan/tmp/gemma-4-E2B-it",
        trust_remote_code=False,
    )
    text = tok.decode(ids, skip_special_tokens=skip_special)
    sys.stdout.write(text)
    if not text.endswith("\n"):
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
