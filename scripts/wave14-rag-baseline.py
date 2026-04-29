#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""
WAVE14 T1.5 — RAG-only baseline.

The control arm for Run B's LoRA: what would Champion (FAISS retrieval +
no LoRA) have answered for the same 8 pinned prompts?

For each of the 8 pinned prompts at /tmp/wave14-phase2/p[1-8].tokens.json:
  1. Decode the prompt tokens to text via gemma4-venv tokenizer.
  2. Call RAGPipeline.retrieve(k=3) to get the top 3 chunks.
  3. Score the top-1 chunk's content against the same gates as the LoRA
     scorer (G-OPEN, G-TOPIC, G-NO-COLLAPSE — though COLLAPSE is N/A for
     retrieval since it returns existing corpus content, not generation).

Output: notes/wave14-runB-rag-baseline.md with side-by-side comparison
of RAG vs LoRA per prompt.

The plan §3 Track 1 exit gate requires LoRA to beat RAG on ≥4/8 prompts.
This script computes the comparison.

Memory: loads FAISS index (~2.7 GB) + embedding model (~80 MB). Run AFTER
Run B finishes — DO NOT run in parallel on the 14 GB dev box.

Usage:
  /home/govan/tmp/gemma4-venv/bin/python3 scripts/wave14-rag-baseline.py
"""
import json
import os
import sys

# Reuse the gate logic from the LoRA scorer.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

PROMPT_DIR = "/tmp/wave14-phase2"
INDEX_DIR = "/home/govan/tmp/unheaded/raft/index"
CORPUS_FILE = "/home/govan/tmp/unheaded/raft/index/combined_corpus.jsonl"
OUT_PATH = "/home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave14-runB-rag-baseline.md"

# Reuse gate constants
TOP5_OPENER_DECODED = [" The", "A", "According", "When", "An"]
KINGDOM_ALLOWLIST = [
    "Wotan", "Sophia", "Monad", "Anamnesis", "Kingdom", "eBPF", "BPF",
    "XDP", "Aya", "Champion", "IPv6", "packet", "Gleipnir",
    "Mímir", "Mimir", "gjallarhorn", "gungnir", "heimdall", "Sleipnir",
    "Kenoma", "Pleroma", "Yaldabaoth", "Yggdrasil", "trace", "NixOS",
    "Wotan-", "monad-", "sophia-", "kingdom-mode", "doom-runner",
    "zhenai", "zhend", "lich", "sealed-cask", "S67", "S68", "S78",
    "ADR-", "GPL-3.0", "raft", "kingdom RAFT", "distillation", "JetBrains",
]


def decode_prompt(tokens, tokenizer):
    """Decode token IDs back to text, skipping special tokens."""
    return tokenizer.decode(tokens, skip_special_tokens=True)


def score_text(text):
    """Apply OPEN + TOPIC gates to a chunk of text. (NO-COLLAPSE n/a for RAG.)"""
    import re
    stripped = text.lstrip()
    open_match = None
    for opener in TOP5_OPENER_DECODED:
        if stripped.startswith(opener) and (
            len(stripped) == len(opener)
            or not stripped[len(opener)].isalnum()
        ):
            open_match = opener
            break
    topic_hits = [t for t in KINGDOM_ALLOWLIST if re.search(
        r"\b" + re.escape(t) + r"\b", text, re.IGNORECASE
    )]
    return open_match, topic_hits


def main():
    print("=" * 78)
    print("WAVE14 T1.5 — RAG-only baseline")
    print("=" * 78)

    # Lazy import — failures here are visible
    print("Loading tokenizer...")
    from transformers import AutoTokenizer
    tokenizer = AutoTokenizer.from_pretrained("/home/govan/tmp/gemma-4-E2B-it")

    print("Loading RAG pipeline (FAISS index + embedding model + corpus)...")
    print("  This takes ~30 seconds + ~3 GB RAM.")
    sys.path.insert(0, "/home/govan/tmp/unheaded/raft/scripts")
    from zhen_rag import RAGPipeline
    rag = RAGPipeline(
        index_dir=INDEX_DIR,
        corpus_file=CORPUS_FILE,
    )
    print("  Loaded.")

    rows = []
    for i in range(1, 9):
        path = os.path.join(PROMPT_DIR, f"p{i}.tokens.json")
        if not os.path.exists(path):
            print(f"  p{i}: prompt missing ({path})")
            continue
        with open(path) as f:
            tokens = json.load(f)
        prompt_text = decode_prompt(tokens, tokenizer)
        # Trim prompt to last ~500 chars to focus on the actual user question
        # (the 332+ token chat-template prefix is mostly RAFT scaffolding).
        focused_query = prompt_text[-500:].strip()
        retrieved = rag.retrieve(focused_query, k=3)
        top1 = retrieved[0] if retrieved else None
        if top1 is None:
            print(f"  p{i}: no retrieval result")
            rows.append((i, focused_query, None, None, []))
            continue
        open_match, topic_hits = score_text(top1["content"])
        rows.append((i, focused_query, top1, open_match, topic_hits))
        print(f"  p{i}: source={top1['source']!r:30s}  "
              f"OPEN={open_match!r:15s}  TOPICS={topic_hits[:3]}")

    # Write the baseline note
    with open(OUT_PATH, "w") as out:
        out.write("# WAVE14 T1.5 — RAG-only baseline (control arm)\n\n")
        out.write(f"**Generated:** by `scripts/wave14-rag-baseline.py`\n")
        out.write(f"**Prompts:** `/tmp/wave14-phase2/p[1-8].tokens.json`\n")
        out.write(f"**Index:** `{INDEX_DIR}/active.index` (1.52M-vector FAISS)\n\n")
        out.write("Per-prompt: top-1 retrieved chunk + OPEN/TOPIC gate scoring.\n\n")
        for i, query, top1, open_match, topics in rows:
            out.write(f"\n## p{i}\n\n")
            out.write(f"**Query (last 500 chars of decoded prompt):**\n\n```\n{query[:500]}\n```\n\n")
            if top1 is None:
                out.write("**No retrieval result.**\n")
                continue
            out.write(f"**Top-1 chunk (distance={top1['distance']:.3f}, source={top1['source']}):**\n\n")
            out.write("```\n")
            out.write(top1["content"][:1500])
            if len(top1["content"]) > 1500:
                out.write(f"\n... ({len(top1['content']) - 1500} more chars)")
            out.write("\n```\n\n")
            out.write(f"- G-OPEN match: `{open_match!r}`\n")
            out.write(f"- G-TOPIC hits: `{topics[:5]}`\n")
        out.write("\n## Comparison vs Run B LoRA\n\n")
        out.write("(populate this section after Run B's T1.4 scoring lands. "
                  "Per long-term plan §3 Track 1 exit, LoRA must beat RAG on ≥4/8 prompts.)\n")
    print(f"\nWrote: {OUT_PATH}")
    print(f"Rows: {len(rows)} prompts scored")


if __name__ == "__main__":
    main()
