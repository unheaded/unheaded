#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.
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

--strict (used by `zhen-flywheel export`, S-TWO-BRAINS Step 379): input
records carry an `id`; every line must parse and have id, question and
answer; nothing is cut (an example longer than --max-tokens, or one too
short to train, is reported in --skipped as {"id", "reason"} and left
out); --ids gets the id of every output line, so the forge file
({"tokens","answer_start"} only) stays line-aligned with the text export.
Any other problem exits 1 naming the input line. Without --strict the
behaviour above is unchanged.

    python scripts/tokenize-coding-for-gemma4.py --strict --max-tokens 384 \
        --ids train.tok.ids --skipped train.tok.skipped < sft-train.jsonl > train.tok.jsonl
"""
import argparse
import json
import sys

MAX_TOKENS = 384
MIN_TOKENS = 8
DEFAULT_TOKENIZER = "/home/govan/tmp/gemma-4-E2B-it"


def encode_pair(tokenizer, question: str, answer: str):
    """Return (full_ids, answer_start) without any truncation."""
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

    return full_ids, len(prompt_ids)


def tokenize_one(tokenizer, question: str, answer: str):
    """Return (token_ids, answer_start), keeping the last MAX_TOKENS tokens."""
    full_ids, answer_start = encode_pair(tokenizer, question, answer)

    if len(full_ids) > MAX_TOKENS:
        drop = len(full_ids) - MAX_TOKENS
        full_ids = full_ids[drop:]
        answer_start = max(1, answer_start - drop)

    return full_ids, answer_start


class StrictError(Exception):
    pass


def run_strict(tokenizer, lines, out, ids_out, skipped_out, max_tokens: int, min_tokens: int = MIN_TOKENS) -> int:
    """Tokenize `{id, question, answer, ...}` lines; return lines written.
    Raises StrictError naming the line on anything but too long/too short."""
    written = 0
    for n, line in enumerate(lines, 1):
        line = line.strip()
        if not line:
            continue
        try:
            obj = json.loads(line)
        except json.JSONDecodeError as e:
            raise StrictError(f"line {n}: not JSON: {e}") from e
        rid, q, a = (obj.get(k) if isinstance(obj, dict) else None for k in ("id", "question", "answer"))
        if not all(isinstance(v, str) and v for v in (rid, q, a)):
            raise StrictError(f"line {n}: needs non-empty string id, question and answer")
        try:
            ids, answer_start = encode_pair(tokenizer, q, a)
        except ValueError as e:
            raise StrictError(f"line {n} ({rid}): {e}") from e
        reason = ""
        if len(ids) > max_tokens:
            reason = "too_long"
        elif len(ids) < min_tokens or answer_start >= len(ids) - 1 or answer_start < 1:
            reason = "too_short"
        if reason:
            skipped_out.write(json.dumps({"id": rid, "reason": reason, "tokens": len(ids)}) + "\n")
            continue
        out.write(json.dumps({"tokens": ids, "answer_start": answer_start}, separators=(",", ":")) + "\n")
        ids_out.write(rid + "\n")
        written += 1
    return written


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--strict", action="store_true")
    ap.add_argument("--max-tokens", type=int, default=MAX_TOKENS)
    ap.add_argument("--ids", help="with --strict: write the id of each output line here")
    ap.add_argument("--skipped", help="with --strict: write {id, reason} of each left-out example here")
    ap.add_argument("--tokenizer", default=DEFAULT_TOKENIZER)
    args = ap.parse_args()

    from transformers import AutoTokenizer
    tok = AutoTokenizer.from_pretrained(
        args.tokenizer,
        trust_remote_code=False,
    )

    if args.strict:
        if not args.ids or not args.skipped:
            print("--strict needs --ids and --skipped", file=sys.stderr)
            return 2
        with open(args.ids, "x") as ids_out, open(args.skipped, "x") as skipped_out:
            try:
                n = run_strict(tok, sys.stdin, sys.stdout, ids_out, skipped_out, args.max_tokens)
            except StrictError as e:
                print(f"tokenize --strict: {e}", file=sys.stderr)
                return 1
        print(f"tokenized --strict: out={n}", file=sys.stderr)
        return 0

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
