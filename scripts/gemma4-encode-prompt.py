#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
"""
WAVE13 Phase 1: encode a single user prompt for forge generate.

Reads a prompt as the first CLI argument (or stdin if "-"), wraps it with the
Gemma-4 chat template using the same HF AutoTokenizer the WAVE12 corpus
tokenizer used, prints `{"tokens":[int,...]}` on a single line.

Usage:
    /home/govan/tmp/gemma4-venv/bin/python scripts/gemma4-encode-prompt.py "What is Wotan?"
    echo "What is Wotan?" | /home/govan/tmp/gemma4-venv/bin/python scripts/gemma4-encode-prompt.py -

Output (single line, JSON):
    {"tokens":[2,106,...,4521,107,108,106,...,107]}

Where the tokens end with the start-of-model-turn marker so forge can begin
generating the assistant's response immediately.
"""
import json
import sys

from transformers import AutoTokenizer


def main() -> int:
    if len(sys.argv) < 2:
        print('Usage: gemma4-encode-prompt.py "<prompt>" | -', file=sys.stderr)
        return 2

    if sys.argv[1] == "-":
        prompt = sys.stdin.read().strip()
    else:
        prompt = sys.argv[1]

    if not prompt:
        print("empty prompt", file=sys.stderr)
        return 2

    tok = AutoTokenizer.from_pretrained(
        "/home/govan/tmp/gemma-4-E2B-it",
        trust_remote_code=False,
    )
    messages = [{"role": "user", "content": prompt}]
    text = tok.apply_chat_template(
        messages, tokenize=False, add_generation_prompt=True,
    )
    ids = tok(text, add_special_tokens=False)["input_ids"]
    sys.stdout.write(json.dumps({"tokens": ids}, separators=(",", ":")) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
