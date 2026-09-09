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

DEFAULT_OUTPUTS_DIR = "/home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave-14/wave14-phase2-quality"
DEFAULT_FILE_PREFIX = "lora-w14b"  # Run B default; override via env or argv

# Resolve at runtime so the scorer is reusable across runs (B, C, D, ...).
OUTPUTS_DIR = os.environ.get("WAVE14_OUTPUTS_DIR", DEFAULT_OUTPUTS_DIR)
FILE_PREFIX = os.environ.get("WAVE14_FILE_PREFIX", DEFAULT_FILE_PREFIX)
# CLI override: python wave14-score-runB.py [outputs_dir] [file_prefix]
if len(sys.argv) >= 2 and not sys.argv[1].startswith("-"):
    OUTPUTS_DIR = sys.argv[1]
if len(sys.argv) >= 3 and not sys.argv[2].startswith("-"):
    FILE_PREFIX = sys.argv[2]


def completion_token_ids(path):
    """
    If the forge log contains a `--- raw completion token IDs (N): [...]`
    line (added 2026-04-30 in main.rs cmd_generate_gemma4), parse the
    token IDs out. Returns a list of ints, or None if missing.
    """
    import re
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            content = f.read()
    except OSError:
        return None
    m = re.search(
        r"--- raw completion token IDs \(\d+\): \[([0-9, ]*)\]",
        content,
    )
    if not m:
        return None
    body = m.group(1).strip()
    if not body:
        return []
    return [int(x) for x in body.split(",") if x.strip()]


def collapse_score_token_ids(ids):
    """
    Given a list of token IDs, return (pass: bool, info: dict).
    Pass requires BOTH:
      (1) No single token occupies > 60 % of the sequence (catches Run B's
          100 % newline collapse).
      (2) Unique-token fraction > 20 % of length (catches Run C's
          3-token cycle collapse: 3/52 = 5.8 % unique → FAIL).
    Coherent generation typically has unique-fraction > 50 %.
    """
    from collections import Counter
    if not ids:
        return False, {"top_id": None, "top_n": 0, "frac": 0.0,
                       "unique": 0, "unique_frac": 0.0, "len": 0}
    c = Counter(ids)
    top_id, top_n = c.most_common(1)[0]
    frac = top_n / len(ids)
    unique = len(c)
    unique_frac = unique / len(ids)
    single_token_pass = frac < 0.60
    cycle_pass = unique_frac > 0.20
    overall_pass = single_token_pass and cycle_pass
    return overall_pass, {
        "top_id": top_id, "top_n": top_n, "frac": frac,
        "unique": unique, "unique_frac": unique_frac, "len": len(ids),
    }


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
        "Loaded LoRA:",  # WAVE14 fix — was triggering G-TOPIC false-positives
        "Prompt tokens:", "max_new_tokens", "GPU upload:", "--- raw completion",
        "--- generated", "[stop token", "    Loading", "  --- ",
    )
    INIT_SUBSTRINGS = (
        "VRAM USED", "VRAM Used", "loaded]", "weights:", "chrt -f",
        "kingdom-w14", "lora.gguf", "rank=16, alpha",  # LoRA path mentions
    )
    lines = text.splitlines()
    out_start = 0
    for i, ln in enumerate(lines):
        s = ln.lstrip()
        if not s:
            continue
        is_init = (
            s.startswith(INIT_PREFIXES)
            or s.startswith(("[", "="))
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

    # G-NO-COLLAPSE: detect collapse in TWO ways now (Run B taught us a
    # gate scoped to one specific attractor misses other shapes):
    #   (1) Run-A attractor pattern (`\t}\n\nQuestion:\n` cycle): >2 cycles fail.
    #   (2) Generic single-token / single-character dominance: any single
    #       non-whitespace character occupying >60 % of the visible
    #       (non-whitespace-stripped) generated text fails. Catches Run B's
    #       newline collapse, cp-50's `, the, the,` collapse, and other
    #       single-pattern attractors.
    # Both checks must clear for collapse_pass = True.
    n_close_brace = text.count("\t}") + text.count(" }")
    n_question = text.count("Question:")
    collapse_marker_count = min(n_close_brace, n_question)
    runA_pattern_pass = collapse_marker_count <= 2

    # Generic single-token-dominance check. Strip whitespace, look at the
    # visible characters; if one character class is >60 % of total length,
    # it's collapsed. We check both raw (catches `\n` collapse since `\n`
    # IS a character) AND stripped (catches `, the,` repetition by looking
    # at the most-common 1-3 char ngram).
    raw = text  # do not strip — `\n` collapse must surface
    if raw:
        from collections import Counter
        # Most common single character in raw text
        char_counts = Counter(c for c in raw if c.strip() or c == "\n")
        if char_counts:
            _top_char, top_n = char_counts.most_common(1)[0]
            top_char_frac = top_n / max(len(raw), 1)
        else:
            top_char_frac = 0.0
        # Most common 3-gram in stripped text (catches `, the` repetition)
        stripped = " ".join(text.split())
        if len(stripped) >= 9:
            grams = [stripped[i:i+3] for i in range(len(stripped) - 2)]
            gram_counts = Counter(grams)
            top_gram, top_gram_n = gram_counts.most_common(1)[0]
            top_gram_frac = top_gram_n / max(len(grams), 1)
        else:
            top_gram, top_gram_n, top_gram_frac = "", 0, 0.0
        single_token_pass = (top_char_frac < 0.6) and (top_gram_frac < 0.4)
    else:
        # Empty completion is collapse-pass under the original gate (no
        # attractor) but we should call it FAIL — empty != generation.
        single_token_pass = False
        top_char_frac = 0.0
        top_gram, top_gram_n, top_gram_frac = "", 0, 0.0

    collapse_pass = runA_pattern_pass and single_token_pass

    return open_pass, topic_pass, collapse_pass, {
        "first_chars": stripped[:80],
        "open_match": open_match,
        "topic_hits": topic_hits[:5],
        "n_close_brace": n_close_brace,
        "n_question": n_question,
        "collapse_marker_count": collapse_marker_count,
        "top_char_frac": top_char_frac,
        "top_gram": top_gram,
        "top_gram_frac": top_gram_frac,
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
        path = os.path.join(OUTPUTS_DIR, f"{FILE_PREFIX}-p{i}.txt")
        if not os.path.exists(path):
            print(f"  p{i}: MISSING ({path})")
            rows.append((i, False, False, False, "MISSING"))
            continue
        text = last_decoded_line(path)
        op, tp, cp, info = score_one(text)
        # If the log contains raw completion token IDs (post 2026-04-30
        # debug print), use those for the authoritative collapse check —
        # robust against decode-stripping or trim_end() hiding `\n` runs.
        ids = completion_token_ids(path)
        if ids is not None:
            id_pass, id_info = collapse_score_token_ids(ids)
            cp = cp and id_pass
            info["token_id_collapse"] = id_info
        open_pass_n += int(op)
        topic_pass_n += int(tp)
        collapse_pass_n += int(cp)
        marker_open = "✓" if op else "✗"
        marker_topic = "✓" if tp else "✗"
        marker_collapse = "✓" if cp else "✗"
        opener_label = info["open_match"] or "—"
        topics_label = ", ".join(info["topic_hits"]) or "—"
        # Collapse label includes Run-A pattern stats AND single-token
        # dominance stats (the gate that caught Run B).
        collapse_label = (
            "runA-pair x" + str(info["collapse_marker_count"]) + " · "
            + "top-char " + str(round(info["top_char_frac"] * 100)) + "% · "
            + "top-3gram '" + (info["top_gram"][:8].replace("\n", "\\n")) + "' "
            + str(round(info["top_gram_frac"] * 100)) + "%"
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
        print("  Verdict (behavioral): PASS  → if G-CE also PASS, Run B passes.")
    else:
        print("  Verdict (behavioral): FAIL  → STOP per long-term plan §3.")
    print("=" * 78)
    sys.exit(0 if behavioral_passes >= 2 else 2)


if __name__ == "__main__":
    main()
