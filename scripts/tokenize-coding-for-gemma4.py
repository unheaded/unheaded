#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
"""
Coding-corpus tokenizer — adapter of `tokenize-kingdom-for-gemma4.py`
for the `{question, answer}` schema produced by `build-coding-corpus.py`.

Reads JSONL from stdin, one object per line:
    {"question": "...", "answer": "...", "source": "...", "language": "go|rust|...",
     "source_id": "..."}

Re-formats each as a Gemma-4 chat (user=question, model=answer), tokenizes
via the HF AutoTokenizer at /home/govan/tmp/gemma-4-E2B-it, and emits
forge-compatible records:
    {"tokens": [int, ...], "answer_start": int}

`answer_start` is the index of the first model-response token (forge's
train-step masks loss to positions [answer_start..len-1]).

Truncation rule: keep the last MAX_TOKENS tokens (preserves the answer,
loses earliest context); answer_start is recomputed against the truncated
prefix. Coding-corpus records are typically much shorter than Kingdom RAFT
(few hundred tokens vs 384 for RAFT) so truncation will be rare.

Usage (from repo root):
    /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-coding-for-gemma4.py \
        < raft/training/coding-train.jsonl > /tmp/coding-train.jsonl
    /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-coding-for-gemma4.py \
        < raft/training/coding-eval.jsonl  > /tmp/coding-eval.jsonl

Sister-script: `scripts/tokenize-kingdom-for-gemma4.py` (Kingdom RAFT path).
"""
import json
import sys

MAX_TOKENS = 384
MIN_TOKENS = 8


def tokenize_one(tokenizer, question: str, answer: str):
    """Return (token_ids, answer_start)."""
    if not question.strip() or not answer.strip():
        raise ValueError("empty question or answer")

    # Prompt only — answer_start = number of tokens in the prompt portion.
    prompt_messages = [{"role": "user", "content": question}]
    prompt_text = tokenizer.apply_chat_template(
        prompt_messages, tokenize=False, add_generation_prompt=True,
    )
    prompt_ids = tokenizer(prompt_text, add_special_tokens=False)["input_ids"]

    # Full conversation: prompt + model answer.
    full_messages = [
        {"role": "user", "content": question},
        {"role": "model", "content": answer},
    ]
    full_text = tokenizer.apply_chat_template(
        full_messages, tokenize=False, add_generation_prompt=False,
    )
    full_ids = tokenizer(full_text, add_special_tokens=False)["input_ids"]

    # Sanity: full should start with prompt_ids.
    if full_ids[: len(prompt_ids)] != prompt_ids:
        raise ValueError("chat template inconsistency: prompt not prefix of full")

    answer_start = len(prompt_ids)

    if len(full_ids) > MAX_TOKENS:
        drop = len(full_ids) - MAX_TOKENS
        full_ids = full_ids[drop:]
        answer_start = max(1, answer_start - drop)

    return full_ids, answer_start


def main() -> int:
    from transformers import AutoTokenizer
    tok = AutoTokenizer.from_pretrained(
        "/home/govan/tmp/gemma-4-E2B-it",
        trust_remote_code=False,
    )

    n_in = 0
    n_out = 0
    n_err = 0
    n_short = 0
    n_trunc = 0
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        n_in += 1
        try:
            obj = json.loads(line)
            question = obj["question"]
            answer = obj["answer"]
        except (json.JSONDecodeError, KeyError) as e:
            n_err += 1
            print(f"[skip line {n_in}] parse: {e}", file=sys.stderr)
            continue
        try:
            ids, answer_start = tokenize_one(tok, question, answer)
        except ValueError as e:
            n_err += 1
            print(f"[skip line {n_in}] tokenize: {e}", file=sys.stderr)
            continue

        if len(ids) < MIN_TOKENS or answer_start >= len(ids) - 1:
            n_short += 1
            continue
        if len(ids) == MAX_TOKENS:
            n_trunc += 1

        out = {"tokens": ids, "answer_start": answer_start}
        sys.stdout.write(json.dumps(out, separators=(",", ":")) + "\n")
        n_out += 1

    print(
        f"tokenized: in={n_in} out={n_out} err={n_err} short={n_short} trunc={n_trunc}",
        file=sys.stderr,
    )
    return 0 if n_out > 0 else 1


if __name__ == "__main__":
    sys.exit(main())
