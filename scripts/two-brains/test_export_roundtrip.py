#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2026 Stevie Bellis.
"""Round trip of a zhen-flywheel export through the Gemma tokenizer
(S-TWO-BRAINS Step 379): closes the WAVE14 H6 class.

    python scripts/two-brains/test_export_roundtrip.py [--sample N] [--tokenizer DIR] <export-dir>

For each split (train, holdout) it reads sft-<split>.jsonl (text),
<split>.tok.jsonl (forge records) and <split>.tok.ids (the capture id of
each forge line), and for up to N examples per split checks that:

  - every forge line is JSON with exactly the keys tokens and answer_start,
    tokens are non-negative ints and 1 <= answer_start < len(tokens);
  - decoding tokens[answer_start:] gives back the answer
    (tokenizer-normalised, special tokens dropped);
  - tokens[:answer_start] decodes to exactly the chat-template prompt for
    the question, and holds none of the answer text.

Before any of that it checks the H6 fixture {"tokens":[1,2,107],
"answer_start":360} is rejected, not absorbed. Exit 0 only when every
check passes. The pure checks also run under pytest (no tokenizer).
"""
import argparse
import json
import random
import sys
from pathlib import Path

DEFAULT_TOKENIZER = "/home/govan/tmp/gemma-4-E2B-it"
H6_FIXTURE = '{"tokens":[1,2,107],"answer_start":360}'


class RoundTripError(Exception):
    pass


def validate_forge_line(line: str, where: str = "") -> dict:
    """The strict forge record, as `zhenai-forge --strict-data` reads it."""
    try:
        obj = json.loads(line)
    except json.JSONDecodeError as e:
        raise RoundTripError(f"{where}not JSON: {e}") from e
    if not isinstance(obj, dict) or set(obj) != {"tokens", "answer_start"}:
        raise RoundTripError(f"{where}keys must be exactly tokens, answer_start")
    toks, start = obj["tokens"], obj["answer_start"]
    if not isinstance(toks, list) or not all(isinstance(t, int) and not isinstance(t, bool) and t >= 0 for t in toks):
        raise RoundTripError(f"{where}tokens must be non-negative ints")
    if not isinstance(start, int) or isinstance(start, bool) or not 1 <= start < len(toks):
        raise RoundTripError(f"{where}answer_start {start!r} outside 1..{len(toks)} (tokens: {len(toks)})")
    return obj


def self_test() -> None:
    try:
        validate_forge_line(H6_FIXTURE)
    except RoundTripError:
        pass
    else:
        raise RoundTripError("H6 fixture accepted: answer_start 360 with 3 tokens")
    ok = validate_forge_line('{"tokens":[1,2,107],"answer_start":2}')
    if ok["tokens"] != [1, 2, 107] or ok["answer_start"] != 2:
        raise RoundTripError("a valid record was misread")
    for bad in ('{"tokens":[1,2,3],"answer_start":1,"q":"x"}', '{"tokens":[1,2,3]}', '{"tokens":[1,2,3],"answer_start":0}',
                '{"tokens":[1,-2,3],"answer_start":1}', '{"tokens":[1,2,3],"answer_start":true}', "[1,2,107,360]"):
        try:
            validate_forge_line(bad)
        except RoundTripError:
            continue
        raise RoundTripError(f"accepted {bad}")


def norm(s: str) -> str:
    return " ".join(s.split())


def check_example(tok, rec: dict, forge: dict, where: str) -> None:
    toks, start = forge["tokens"], forge["answer_start"]
    want_prompt = tok.apply_chat_template([{"role": "user", "content": rec["question"]}], tokenize=False, add_generation_prompt=True)
    got_prompt = tok.decode(toks[:start], skip_special_tokens=False)
    if got_prompt != want_prompt:
        raise RoundTripError(f"{where}tokens[:answer_start] is not the prompt (answer_start off by one?)")
    answer = norm(tok.decode(tok(rec["answer"], add_special_tokens=False)["input_ids"], skip_special_tokens=True))
    got_answer = norm(tok.decode(toks[start:], skip_special_tokens=True))
    if got_answer != answer:
        raise RoundTripError(f"{where}tokens[answer_start:] does not decode to the answer")
    if len(answer) >= 8 and answer not in norm(rec["question"]) and answer in norm(got_prompt):
        raise RoundTripError(f"{where}answer text inside the prompt tokens")


def read_lines(p: Path) -> list:
    return [line for line in p.read_text().splitlines() if line.strip()]


def run(export: Path, sample: int, tokenizer_dir: str) -> int:
    self_test()
    from transformers import AutoTokenizer
    tok = AutoTokenizer.from_pretrained(tokenizer_dir, trust_remote_code=False)
    checked = 0
    for split in ("train", "holdout"):
        text = {}
        for line in read_lines(export / f"sft-{split}.jsonl"):
            r = json.loads(line)
            text[r["id"]] = r
        forge = read_lines(export / f"{split}.tok.jsonl")
        ids = read_lines(export / f"{split}.tok.ids")
        if len(forge) != len(ids):
            raise RoundTripError(f"{split}: {len(forge)} forge lines but {len(ids)} ids")
        idx = list(range(len(forge)))
        random.Random(f"roundtrip-{split}").shuffle(idx)
        for i in sorted(idx[:sample]):
            where = f"{split}.tok.jsonl:{i + 1} ({ids[i][:12]}): "
            f = validate_forge_line(forge[i], where)
            if ids[i] not in text:
                raise RoundTripError(f"{where}id not in sft-{split}.jsonl")
            check_example(tok, text[ids[i]], f, where)
            checked += 1
        print(f"{split}: {min(sample, len(forge))} of {len(forge)} checked")
    print(f"round trip: {checked} examples, 0 failed; H6 fixture rejected")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--sample", type=int, default=200)
    ap.add_argument("--tokenizer", default=DEFAULT_TOKENIZER)
    ap.add_argument("export", type=Path)
    args = ap.parse_args()
    try:
        return run(args.export, args.sample, args.tokenizer)
    except (RoundTripError, OSError, KeyError, json.JSONDecodeError) as e:
        print(f"round trip FAILED: {e}", file=sys.stderr)
        return 1


# --- pytest (no tokenizer needed) ---------------------------------------

def test_h6_fixture_rejected_and_strict_shape():
    self_test()


def test_strict_tokenizer_never_cuts_and_stays_aligned():
    import importlib.util
    import io
    spec = importlib.util.spec_from_file_location("tok", Path(__file__).resolve().parents[1] / "tokenize-coding-for-gemma4.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)

    class Fake:  # one token per character; template "<u>Q</u><m>" then "A</m>"
        def apply_chat_template(self, msgs, tokenize, add_generation_prompt):
            return "<u>" + msgs[0]["content"] + "</u><m>" + (msgs[1]["content"] + "</m>" if len(msgs) > 1 else "")

        def __call__(self, text, add_special_tokens=False):
            return {"input_ids": [ord(c) for c in text]}

    fake = Fake()
    rows = [{"id": "a", "question": "q one", "answer": "the first answer"},
            {"id": "b", "question": "q two", "answer": "x" * 500},
            {"id": "c", "question": "q three", "answer": "the third answer"}]
    src = io.StringIO("".join(json.dumps(r) + "\n" for r in rows))
    out, ids, skipped = io.StringIO(), io.StringIO(), io.StringIO()
    n = mod.run_strict(fake, src, out, ids, skipped, max_tokens=100, min_tokens=4)
    assert n == 2
    lines = out.getvalue().splitlines()
    assert ids.getvalue().split() == ["a", "c"]
    assert json.loads(skipped.getvalue())["id"] == "b"
    for line in lines:
        validate_forge_line(line)
    first = json.loads(lines[0])
    assert "".join(map(chr, first["tokens"][first["answer_start"]:])).startswith("the first answer")
    assert "".join(map(chr, first["tokens"][:first["answer_start"]])) == "<u>q one</u><m>"

    bad = io.StringIO('{"id":"z","question":"q"}\n')
    try:
        mod.run_strict(fake, bad, io.StringIO(), io.StringIO(), io.StringIO(), max_tokens=100)
    except mod.StrictError as e:
        assert "line 1" in str(e)
    else:
        raise AssertionError("a record without an answer was accepted")


if __name__ == "__main__":
    sys.exit(main())
