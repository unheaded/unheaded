#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
"""
WAVE10F Phase 7.1 vertical slice: Gemma 4 tokenization of the Kingdom RAFT
corpus for zhenai-forge.

Reads Mistral-formatted JSONL from stdin (one `{"text": "<s>[INST] ... [/INST] ...</s>"}`
object per line), re-formats to Gemma 4's native chat template
(`<start_of_turn>user ... <end_of_turn>\\n<start_of_turn>model ... <end_of_turn>`),
tokenizes with the HF AutoTokenizer backed by the official Gemma 4 vocab,
and emits `{"tokens":[int,...], "answer_start": int}` to stdout.

`answer_start` is the token index of the first model-response token — forge's
train-step loss is computed from `answer_start + 1` onward (next-token
prediction).

Sequences longer than MAX_TOKENS are truncated to the last MAX_TOKENS tokens
(keep the answer, lose the earliest context) and answer_start is recomputed
against the truncated prefix.

Intended invocation (from repo root):
    /home/govan/tmp/gemma4-venv/bin/python \\
        scripts/tokenize-kingdom-for-gemma4.py \\
        < raft/training/train.jsonl > /tmp/24h-kingdom-train.jsonl
"""
import json
import re
import sys

MAX_TOKENS = 384  # budget-compatible with ~2s/step warm forward
MIN_TOKENS = 8    # drop anything shorter than a useful prompt

# Mistral chat markers we need to strip / replace.
MISTRAL_INST_OPEN = "[INST]"
MISTRAL_INST_CLOSE = "[/INST]"
MISTRAL_BOS = "<s>"
MISTRAL_EOS = "</s>"


def mistral_to_gemma4(text: str) -> tuple[str, str]:
    """Split a Mistral <s>[INST] X [/INST] Y </s> string into (user, model).

    Returns (user_text, model_text). Raises ValueError if the markers aren't
    found or are malformed.
    """
    # Strip BOS / EOS if present.
    t = text.strip()
    if t.startswith(MISTRAL_BOS):
        t = t[len(MISTRAL_BOS):].lstrip()
    if t.endswith(MISTRAL_EOS):
        t = t[: -len(MISTRAL_EOS)].rstrip()

    # [INST] ... [/INST] ... pattern.
    m = re.search(
        r"\[INST\](.*?)\[/INST\]\s*(.*)",
        t,
        flags=re.DOTALL,
    )
    if m is None:
        raise ValueError("no [INST]/[/INST] markers")
    user = m.group(1).strip()
    model = m.group(2).strip()
    if not user or not model:
        raise ValueError("empty user or model portion")
    return user, model


def build_gemma4_messages(user: str, model: str) -> list[dict]:
    """Construct the HF chat-template-friendly message list for Gemma 4."""
    return [
        {"role": "user", "content": user},
        {"role": "model", "content": model},
    ]


def tokenize_one(tokenizer, text: str):
    """Return (token_ids, answer_start). answer_start is the index of the
    first model-response token in the flat token list."""
    user, model = mistral_to_gemma4(text)

    # Prompt only (user portion + start-of-model turn): answer_start = len(prompt)
    prompt_messages = [{"role": "user", "content": user}]
    prompt_text = tokenizer.apply_chat_template(
        prompt_messages, tokenize=False, add_generation_prompt=True,
    )
    prompt_ids = tokenizer(prompt_text, add_special_tokens=False)["input_ids"]

    # Full conversation: prompt + model answer.
    full_messages = build_gemma4_messages(user, model)
    full_text = tokenizer.apply_chat_template(
        full_messages, tokenize=False, add_generation_prompt=False,
    )
    full_ids = tokenizer(full_text, add_special_tokens=False)["input_ids"]

    # Sanity: full must start with prompt_ids.
    if full_ids[: len(prompt_ids)] != prompt_ids:
        raise ValueError("chat template inconsistency: prompt not prefix of full")

    answer_start = len(prompt_ids)

    # Truncation: keep the last MAX_TOKENS tokens, but ensure answer_start
    # stays ≥1 so forge's loss has at least one context position.
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
            text = obj["text"]
        except (json.JSONDecodeError, KeyError) as e:
            n_err += 1
            print(f"[skip line {n_in}] parse: {e}", file=sys.stderr)
            continue
        try:
            ids, answer_start = tokenize_one(tok, text)
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
