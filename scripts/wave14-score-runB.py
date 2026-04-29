#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""
WAVE14 T1.4 — score Run B's 8 generation outputs against the pre-registered
gates G-OPEN, G-TOPIC, G-NO-COLLAPSE.

Inputs (8 raw output files, one per prompt):
    crates/zhenai-forge/notes/wave14-phase2-quality/lora-w14b-p[1-8].txt

Each file is the raw stdout of `zhenai-forge generate-gemma4` for that prompt.
The forge generate path detokenizes inline — the LAST line of each .txt is the
decoded text we score against.

Pre-registered gates (frozen 2026-04-29 in
crates/zhenai-forge/notes/wave14-runB-pre-registration.md):

    G-OPEN:        ≥3/8 outputs begin (post-stop/newline) with one of the
                   corpus's top-5 answer openers (token IDs 818, 236776,
                   10450, 4420, 2267 — decoded as " The", "A", "According",
                   "When", "An").

    G-TOPIC:       ≥1/8 outputs contain a Kingdom-term hit on the frozen
                   allowlist (case-insensitive substring match).

    G-NO-COLLAPSE: 0/8 outputs have the Run-A attractor pattern
                   (\\t}\\n\\nQuestion:\\n) occupying >40 tokens of the
                   first 100 generated. Approximated here by counting
                   '}' + 'Question:' occurrences in the first ~400 chars
                   (proxy for tokens since we don't re-tokenize).

A separate manual G-CE check (see wave14-runB-pre-registration.md §G-CE) is
NOT scored by this script — that's a single number from `eval-gemma4`.

Verdict: G-CE PASS plus ≥2 of (G-OPEN, G-TOPIC, G-NO-COLLAPSE) = Run B PASS.

Usage:
    /home/govan/tmp/gemma4-venv/bin/python3 scripts/wave14-score-runB.py
"""
import os
import re
import sys

# Frozen at pre-registration time (do NOT edit without rewriting the gate doc)
TOP5_OPENER_DECODED = [" The", "A", "According", "When", "An"]
KINGDOM_ALLOWLIST = [
    "Wotan", "Sophia", "Monad", "Anamnesis", "Kingdom", "eBPF", "BPF",
    "XDP", "Aya", "Champion", "IPv6", "packet", "Gleipnir", "Mímir",
    "Mimir", "gjallarhorn", "gungnir", "heimdall", "Sleipnir", "Kenoma",
    "Pleroma", "Yaldabaoth", "Yggdrasil", "trace", "NixOS", "Wotan-",
    "monad-", "sophia-", "kingdom-mode", "doom-runner", "zhenai", "zhend",
    "lich", "sealed-cask", "S67", "S68", "S78", "ADR-", "GPL-3.0",
    "raft", "kingdom RAFT", "distillation", "JetBrains",
]
COLLAPSE_FILL_FRACTION_THRESHOLD = 0.40  # of 100 generated tokens

OUTPUTS_DIR = "/home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave14-phase2-quality"


def decoded_text(path):
    """
    Return the decoded-generation portion of a forge generate-gemma4 stdout.

    The forge writes:
      <header lines: loading, GPU upload, prompt info>
      <one or more lines of decoded model output>
      <maybe stop-token reason / timing footer>

    Concretely we want everything AFTER the last init-style header line and
    before any final timing footer — but a simple, robust heuristic is:
    return the full file (init lines don't contain '}' or 'Question:'
    tokens that affect our gates), with leading init-style lines removed
    so first-token analysis isn't confused.
    """
    with open(path, encoding="utf-8", errors="replace") as f:
        text = f.read()
    # Drop leading lines that are clearly init/header (start with whitespace
    # then a known init prefix). Stop at the first line that looks like
    # generation output (anything not matching the prefix list).
    INIT_PREFIXES = (
        "Loading", "  Loading", "Uploading", "  Uploading", "  Active targets",
        "  LoRA size", "  Per-example", "  GPU", "  Architecture",
        "  RoPE", "  Logit", "  Layer pattern", "  token_embd", "  per_layer",
        "  per-layer", "  TOTAL", "  Active targets", "Building", "Model:",
        "Zhenai", "===", "    [", "  [", "Phase", "ZHENAI", "===========",
        "token_embd",  # bare line in some run outputs
        "ple_token",
    )
    INIT_SUBSTRINGS = (
        "VRAM USED", "VRAM Used", "loaded]", "weights:", "chrt -f",
    )
    lines = text.splitlines()
    out_start = 0
    for i, ln in enumerate(lines):
        s = ln.lstrip()
        if not s:
            continue
        is_init = (
            s.startswith(INIT_PREFIXES)
            or s.startswith("[")
            or s.startswith("=")
            or any(sub in s for sub in INIT_SUBSTRINGS)
        )
        if is_init:
            out_start = i + 1
        else:
            break
    return "\n".join(lines[out_start:])


# Backward-compat alias used elsewhere
def last_decoded_line(path):
    return decoded_text(path)


def score_one(text):
    """Return (open_pass, topic_pass, collapse_pass, summary_dict)."""
    # Strip leading whitespace/end-of-turn markers to find first content token.
    stripped = text.lstrip()

    # G-OPEN: does the start match one of the top-5 openers?
    open_pass = False
    open_match = None
    for opener in TOP5_OPENER_DECODED:
        # Match word-boundary'd to avoid "Annotated" matching "An"
        if stripped.startswith(opener) and (
            len(stripped) == len(opener)
            or not stripped[len(opener)].isalnum()
        ):
            open_pass = True
            open_match = opener
            break

    # G-TOPIC: any Kingdom-allowlist term anywhere in the text?
    topic_hits = [t for t in KINGDOM_ALLOWLIST if re.search(
        r"\b" + re.escape(t) + r"\b", text, re.IGNORECASE
    )]
    topic_pass = len(topic_hits) > 0

    # G-NO-COLLAPSE: count attractor cycles in the decoded region.
    # The Run A attractor `\t}\n\nQuestion:\n` is ~5 tokens per cycle;
    # pre-registration spec: fail if >40 of first 100 generated tokens
    # are attractor-fill, i.e. >8 cycles. We use a tighter threshold of
    # 2 cycles (1 pair would still pass) so Run A's 5-11 marker patterns
    # all fail cleanly.
    n_close_brace = text.count("\t}") + text.count(" }")
    n_question = text.count("Question:")
    collapse_marker_count = min(n_close_brace, n_question)
    collapse_pass = collapse_marker_count <= 2

    return open_pass, topic_pass, collapse_pass, {
        "first_chars": stripped[:80],
        "open_match": open_match,
        "topic_hits": topic_hits[:5],
        "n_close_brace": n_close_brace,
        "n_question": n_question,
        "collapse_marker_count": collapse_marker_count,
        "len": len(text),
    }


def main():
    if not os.path.isdir(OUTPUTS_DIR):
        print(f"ERROR: outputs dir missing: {OUTPUTS_DIR}", file=sys.stderr)
        print("hint: run T1.4 generation first (8 prompts via generate-gemma4)",
              file=sys.stderr)
        sys.exit(1)

    print("=" * 78)
    print("WAVE14 Run B — gate scorer (T1.4)")
    print("=" * 78)

    open_pass_n = 0
    topic_pass_n = 0
    collapse_pass_n = 0
    rows = []

    for i in range(1, 9):
        path = os.path.join(OUTPUTS_DIR, f"lora-w14b-p{i}.txt")
        if not os.path.exists(path):
            print(f"  p{i}: MISSING ({path})")
            rows.append((i, False, False, False, "MISSING"))
            continue
        text = last_decoded_line(path)
        op, tp, cp, info = score_one(text)
        open_pass_n += int(op)
        topic_pass_n += int(tp)
        collapse_pass_n += int(cp)
        marker_open = "✓" if op else "✗"
        marker_topic = "✓" if tp else "✗"
        marker_collapse = "✓" if cp else "✗"
        opener_label = info["open_match"] or "—"
        topics_label = ", ".join(info["topic_hits"]) or "—"
        collapse_label = (
            "}}"[:1] + " x" + str(info["n_close_brace"]) + ", "
            + "Q: x" + str(info["n_question"]) + " "
            + "(coll-pair x" + str(info["collapse_marker_count"]) + ")"
        )
        rows.append((i, op, tp, cp, info))
        print(f"  p{i}:  OPEN={marker_open} ({opener_label})  "
              f"TOPIC={marker_topic} ({topics_label})  "
              f"COLLAPSE={marker_collapse} ({collapse_label})")
        print(f"      first 80 chars: {info['first_chars']!r}")

    print()
    print("-" * 78)
    print(f"  G-OPEN:        {open_pass_n}/8  (gate: ≥3)  "
          f"{'PASS' if open_pass_n >= 3 else 'FAIL'}")
    print(f"  G-TOPIC:       {topic_pass_n}/8  (gate: ≥1)  "
          f"{'PASS' if topic_pass_n >= 1 else 'FAIL'}")
    print(f"  G-NO-COLLAPSE: {collapse_pass_n}/8  (gate: =8 i.e. all clean)  "
          f"{'PASS' if collapse_pass_n == 8 else 'FAIL'}")
    print()
    behavioral_passes = (
        int(open_pass_n >= 3)
        + int(topic_pass_n >= 1)
        + int(collapse_pass_n == 8)
    )
    print(f"  Behavioral gates passed: {behavioral_passes}/3  (need ≥2)")
    print()
    print("  G-CE:          NOT SCORED HERE — read eval-gemma4 output and "
          "verify mean_CE(LoRA) < mean_CE(base) by ≥ 8.0")
    print()
    if behavioral_passes >= 2:
        print(f"  Verdict (behavioral): PASS  → if G-CE also PASS, Run B passes.")
    else:
        print(f"  Verdict (behavioral): FAIL  → STOP per long-term plan §3.")
    print("=" * 78)
    sys.exit(0 if behavioral_passes >= 2 else 2)


if __name__ == "__main__":
    main()
