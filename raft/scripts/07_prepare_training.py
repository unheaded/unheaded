#!/usr/bin/env python3
"""
07_prepare_training.py - Prepare RAFT QA pairs for QLoRA fine-tuning.

Loads raft_dataset.jsonl and converts each entry into Mistral instruct format
training examples with source context + distractor chunks. Splits into
train (90%) and eval (10%) sets.

Usage:
    source ~/.venv/zhen/bin/activate
    python 07_prepare_training.py
"""

import json
import os
import random
import sys
from pathlib import Path

# Paths
SCRIPT_DIR = Path(__file__).resolve().parent
RAFT_DIR = SCRIPT_DIR.parent
DATASET_PATH = RAFT_DIR / "raft_dataset.jsonl"
TRAINING_DIR = RAFT_DIR / "training"
TRAIN_OUTPUT = TRAINING_DIR / "train.jsonl"
EVAL_OUTPUT = TRAINING_DIR / "eval.jsonl"

# Config
TRAIN_RATIO = 0.9
SEED = 42
NUM_DISTRACTORS = 2  # Number of distractor chunks to include alongside source

SYSTEM_PROMPT = (
    "You are Zhen, the AI champion of Unheaded. "
    "Use the provided context to answer accurately. If unsure, say so."
)


def load_dataset(path: Path) -> list[dict]:
    """Load RAFT dataset from JSONL file."""
    entries = []
    with open(path, "r", encoding="utf-8") as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                entry = json.loads(line)
                entries.append(entry)
            except json.JSONDecodeError as e:
                print(f"WARNING: Skipping line {line_num}: {e}", file=sys.stderr)
    return entries


def build_context(entry: dict) -> str:
    """Build context string from source content + random distractor chunks."""
    chunks = []

    # Add source content
    source_doc = entry.get("source_content", "")
    if source_doc:
        chunks.append(source_doc)

    # Add distractor chunks (pick NUM_DISTRACTORS randomly)
    distractors = entry.get("distractor_chunks", [])
    if isinstance(distractors, list) and distractors:
        selected = random.sample(
            distractors, min(NUM_DISTRACTORS, len(distractors))
        )
        for d in selected:
            content = d.get("content", "") if isinstance(d, dict) else str(d)
            if content:
                chunks.append(content)

    # Shuffle so source document position varies
    random.shuffle(chunks)

    return "\n\n".join(chunks)


def format_mistral_instruct(question: str, context: str, answer: str) -> str:
    """Format a single training example in Mistral instruct format."""
    prompt = (
        f"{SYSTEM_PROMPT}\n\n"
        f"Context:\n{context}\n\n"
        f"Question: {question}"
    )
    return f"<s>[INST] {prompt} [/INST] {answer}</s>"


def entry_to_training_example(entry: dict) -> dict:
    """Convert a RAFT dataset entry into a training example."""
    question = entry.get("question", "")
    answer = entry.get("answer", "")
    context = build_context(entry)
    text = format_mistral_instruct(question, context, answer)

    return {
        "text": text,
        "question": question,
        "source_file": entry.get("source_file", ""),
        "source_chunk_id": entry.get("source_chunk_id", ""),
    }


def estimate_tokens(text: str) -> int:
    """Rough token estimate: ~4 chars per token for English text."""
    return len(text) // 4


def main():
    random.seed(SEED)

    # Ensure output directory exists
    TRAINING_DIR.mkdir(parents=True, exist_ok=True)

    # Load dataset
    print(f"Loading dataset from {DATASET_PATH}")
    entries = load_dataset(DATASET_PATH)
    if not entries:
        print("ERROR: No entries loaded from dataset.", file=sys.stderr)
        sys.exit(1)
    print(f"  Loaded {len(entries)} entries")

    # Convert to training examples
    print("Converting to Mistral instruct format...")
    examples = [entry_to_training_example(e) for e in entries]

    # Shuffle and split
    random.shuffle(examples)
    split_idx = int(len(examples) * TRAIN_RATIO)
    train_examples = examples[:split_idx]
    eval_examples = examples[split_idx:]

    # Write train set
    with open(TRAIN_OUTPUT, "w", encoding="utf-8") as f:
        for ex in train_examples:
            f.write(json.dumps(ex, ensure_ascii=False) + "\n")

    # Write eval set
    with open(EVAL_OUTPUT, "w", encoding="utf-8") as f:
        for ex in eval_examples:
            f.write(json.dumps(ex, ensure_ascii=False) + "\n")

    # Compute statistics
    all_texts = [ex["text"] for ex in examples]
    train_texts = [ex["text"] for ex in train_examples]
    eval_texts = [ex["text"] for ex in eval_examples]

    token_counts = [estimate_tokens(t) for t in all_texts]
    train_tokens = [estimate_tokens(t) for t in train_texts]
    eval_tokens = [estimate_tokens(t) for t in eval_texts]

    char_counts = [len(t) for t in all_texts]

    print()
    print("=" * 60)
    print("RAFT Training Data Statistics")
    print("=" * 60)
    print(f"  Total examples:        {len(examples)}")
    print(f"  Train examples:        {len(train_examples)} ({len(train_examples)/len(examples)*100:.1f}%)")
    print(f"  Eval examples:         {len(eval_examples)} ({len(eval_examples)/len(examples)*100:.1f}%)")
    print()
    print("Token estimates (approx ~4 chars/token):")
    print(f"  Avg tokens/example:    {sum(token_counts)/len(token_counts):.0f}")
    print(f"  Min tokens:            {min(token_counts)}")
    print(f"  Max tokens:            {max(token_counts)}")
    print(f"  Total tokens (all):    {sum(token_counts):,}")
    print(f"  Total tokens (train):  {sum(train_tokens):,}")
    print(f"  Total tokens (eval):   {sum(eval_tokens):,}")
    print()
    print("Character counts:")
    print(f"  Avg chars/example:     {sum(char_counts)/len(char_counts):.0f}")
    print(f"  Min chars:             {min(char_counts)}")
    print(f"  Max chars:             {max(char_counts)}")
    print()

    # Check for examples that exceed max seq length (2048 tokens)
    over_2048 = sum(1 for t in token_counts if t > 2048)
    if over_2048:
        print(f"  WARNING: {over_2048} examples exceed 2048 tokens (will be truncated)")
    else:
        print(f"  All examples fit within 2048 token limit")

    print()
    print(f"Output files:")
    print(f"  Train: {TRAIN_OUTPUT}")
    print(f"  Eval:  {EVAL_OUTPUT}")
    print()

    # Show a sample
    print("=" * 60)
    print("Sample training example (first 500 chars):")
    print("=" * 60)
    print(train_examples[0]["text"][:500])
    print("...")
    print()


if __name__ == "__main__":
    main()
