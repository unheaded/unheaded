#!/usr/bin/env python3
"""
H2 + H6 combined analysis for WAVE14 mode-collapse diagnosis.

H2 (corpus opening-token frequency):
  For each training example, look up the first token AT answer_start.
  Count the distribution. Compare against:
    - "Question:" first-token (does corpus literally contain "Question:" as
      an answer opener?)
    - " }" / "\\t}" first-token
    - " The" / " " (the WAVE13 ground-truth pattern, token 818)
  Pre-registered:
    - >10% of any structural opener -> H2 fires (corpus shape leaked)
    - <2%                            -> H2 falsified, H6 carries weight

H6 (cmd_train_gemma4 line-parser audit per ADR-050 negative section):
  Simulate Rust parser on the first 50 training records:
    let toks: Vec<u32> = line
        .split(|c: char| !c.is_ascii_digit() && c != '-')
        .filter(|s| !s.is_empty())
        .filter_map(|s| s.parse().ok())
        .collect();
  This regex splits on every non-digit character, including JSON delimiters.
  If the JSON line ends with `,"answer_start":N}`, the integer N gets parsed
  and appended to the tokens vector.
  Pre-registered:
    - If parsed.len() == json["tokens"].len() + 1 AND
       parsed[-1] == json["answer_start"]
       in >=90% of records -> H6 confirmed (parser corrupts every example).
"""
import json
import re
from collections import Counter

CORPUS = "/tmp/24h-kingdom-train.jsonl"

# ----- H2: opening-token frequency at answer_start -----
opening = Counter()
n_records = 0
n_with_as = 0
answer_start_dist = Counter()

with open(CORPUS) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        n_records += 1
        d = json.loads(line)
        toks = d["tokens"]
        if "answer_start" not in d:
            continue
        n_with_as += 1
        a_s = d["answer_start"]
        answer_start_dist[a_s] += 1
        if a_s < len(toks):
            opening[toks[a_s]] += 1

print("=== H2: corpus opening-token frequency ===")
print(f"records:               {n_records}")
print(f"with answer_start:     {n_with_as}")
print(f"unique opening tokens: {len(opening)}")
print("top 20 opening tokens (token_id : count : pct):")
for tid, n in opening.most_common(20):
    pct = 100.0 * n / n_with_as
    print(f"  {tid:>8} : {n:>5} : {pct:>5.2f}%")

print()
print("answer_start value distribution (top 10):")
for a, n in answer_start_dist.most_common(10):
    pct = 100.0 * n / n_with_as
    print(f"  answer_start={a:>4} : {n:>5} : {pct:>5.2f}%")
print()

# ----- H6: parser-corruption audit -----
# Simulate the Rust parser exactly:
#   split on (!ascii_digit && c != '-')
parser_re = re.compile(r"[^0-9-]+")

def rust_parse(line):
    """Mirror cmd_train_gemma4 line parser at main.rs:126-137"""
    parts = parser_re.split(line)
    out = []
    for p in parts:
        if not p:
            continue
        try:
            v = int(p)
            if v >= 0:  # u32, so reject negatives
                out.append(v)
            # else: dropped silently (matches filter_map(parse::<u32>().ok()))
        except ValueError:
            pass
    return out

print("=== H6: cmd_train_gemma4 line-parser corruption audit ===")
print("Rule:  split(|c| !c.is_ascii_digit() && c != '-').filter_map(parse::<u32>())")
print()

# Audit first 50 records
n_audit = 0
n_corrupted = 0  # parsed.len() == tokens.len() + 1 AND parsed[-1] == answer_start
n_other_corruption = 0
example_drift = []

with open(CORPUS) as f:
    for i, line in enumerate(f):
        if i >= 50:
            break
        line = line.strip()
        if not line:
            continue
        d = json.loads(line)
        true_toks = d["tokens"]
        a_s = d.get("answer_start")

        parsed = rust_parse(line)
        n_audit += 1

        delta = len(parsed) - len(true_toks)
        if a_s is not None and delta == 1 and parsed[-1] == a_s:
            n_corrupted += 1
        elif delta != 0:
            n_other_corruption += 1
            example_drift.append((i, delta, len(true_toks), len(parsed),
                                   parsed[-3:] if len(parsed) >= 3 else parsed))

print(f"records audited: {n_audit}")
print(f"clean H6 corruption (parsed = tokens + [answer_start]): {n_corrupted} / {n_audit}  "
      f"({100*n_corrupted/n_audit:.0f}%)")
print(f"other parser drift: {n_other_corruption}")
if example_drift[:3]:
    print("  sample drift cases:")
    for idx, d, lt, lp, tail in example_drift[:3]:
        print(f"    record[{idx}]: delta={d} len(true)={lt} len(parsed)={lp} parsed_tail={tail}")

print()
print("=== H6 demonstration on record[0] ===")
with open(CORPUS) as f:
    line = next(f).strip()
d = json.loads(line)
parsed = rust_parse(line)
print(f"  json['tokens'] last-5:        {d['tokens'][-5:]}")
print(f"  rust-parser output last-5:    {parsed[-5:]}")
print(f"  json['answer_start']:         {d['answer_start']}")
print(f"  parsed.len() - tokens.len():  {len(parsed) - len(d['tokens'])}")

# ----- H2-supplementary: what does "Question:" tokenize to? -----
print()
print("=== Token-ID lookup (need gemma4-venv tokenizer) ===")
import subprocess

script = """
from transformers import AutoTokenizer
tok = AutoTokenizer.from_pretrained("google/gemma-3-1b-it") if False else None
# Use the local gemma-4 if present:
import os
local_path = "/home/govan/tmp/gemma-4-E2B-it"
if os.path.isdir(local_path):
    tok = AutoTokenizer.from_pretrained(local_path)
print("Question:        ->", tok.encode("Question:", add_special_tokens=False))
print(" Question:       ->", tok.encode(" Question:", add_special_tokens=False))
print("\\nQuestion:      ->", tok.encode("\\nQuestion:", add_special_tokens=False))
print("\\tif            ->", tok.encode("\\tif", add_special_tokens=False))
print("\\t}             ->", tok.encode("\\t}", add_special_tokens=False))
print(" The            ->", tok.encode(" The", add_special_tokens=False))
print("end_of_turn     ->", tok.eos_token_id, "(EOS)")
print("[1, 2, 105, 106, 107, 108]:")
for tid in [1, 2, 105, 106, 107, 108, 818]:
    print(f"  {tid} ->", repr(tok.decode([tid])))
# decode the top-5 H2 opening tokens
"""
try:
    out = subprocess.check_output(
        ["/home/govan/tmp/gemma4-venv/bin/python", "-c", script],
        text=True, stderr=subprocess.STDOUT, timeout=60)
    print(out)
except Exception as e:
    print(f"  (gemma4-venv tokenize failed: {e}; skip)")

# Decode the top-5 opening tokens via gemma-venv
top_5_tids = [tid for tid, _ in opening.most_common(5)]
script2 = f"""
from transformers import AutoTokenizer
import os
local_path = "/home/govan/tmp/gemma-4-E2B-it"
tok = AutoTokenizer.from_pretrained(local_path)
print("=== Decoding top-5 opening tokens ===")
for tid in {top_5_tids}:
    print(f'  {{tid}} -> ', repr(tok.decode([tid])))
"""
try:
    out2 = subprocess.check_output(
        ["/home/govan/tmp/gemma4-venv/bin/python", "-c", script2],
        text=True, stderr=subprocess.STDOUT, timeout=60)
    print(out2)
except Exception as e:
    print(f"  (top-5 decode failed: {e})")

# ----- VERDICT -----
print()
print("=" * 60)
print("VERDICT")
print("=" * 60)
top_pct = 100 * opening.most_common(1)[0][1] / n_with_as
print(f"H2 top-opener pct: {top_pct:.2f}%")
if top_pct >= 10:
    print("  -> H2 FIRES (>10% structural opener; corpus shape leaked)")
elif top_pct < 2:
    print("  -> H2 falsified (<2% top-opener; H6 carries weight)")
else:
    print("  -> H2 inconclusive (between 2 and 10%)")

if n_corrupted >= 0.9 * n_audit:
    print(f"H6: CONFIRMED ({n_corrupted}/{n_audit} = "
          f"{100*n_corrupted/n_audit:.0f}% corrupted by line-parser bug)")
    print("     Every training example trains on 1 stray integer token at end.")
    print("     Fix: replace cmd_train_gemma4 line parser with json::from_str.")
else:
    print(f"H6: NOT CONFIRMED ({n_corrupted}/{n_audit} matches expected pattern)")
