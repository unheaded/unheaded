#!/usr/bin/env python3
"""
16_embed_v2.py — Embed ring_all.jsonl and build FAISS v2 index.

Processes the combined corpus in 50K-chunk batches to avoid OOM on 14GB RAM.
Uses all-MiniLM-L6-v2 with normalized embeddings for cosine similarity (IndexFlatIP).

Output:
  - ~/tmp/unheaded/raft/index/v2.index      (FAISS IndexFlatIP, 384-dim)
  - ~/tmp/unheaded/raft/index/v2_ids.json    (ordered list of chunk IDs)

Expected runtime: 2-3 hours for ~1M chunks.
"""

import gc
import json
import os
import sys
import time
import numpy as np
import faiss
from pathlib import Path
from sentence_transformers import SentenceTransformer

CORPUS = Path.home() / "tmp" / "unheaded" / "raft" / "corpus" / "ring_all.jsonl"
INDEX_DIR = Path.home() / "tmp" / "unheaded" / "raft" / "index"
INDEX_FILE = INDEX_DIR / "v2.index"
IDS_FILE = INDEX_DIR / "v2_ids.json"

EMBED_BATCH = 512       # Sentences per model.encode() call (512 optimal for RX 7700 XT)
PROCESS_BATCH = 5_000   # Chunks per FAISS add cycle (smaller = faster progress, less tokenization lag)
DIM = 384               # all-MiniLM-L6-v2 dimension
PROGRESS_EVERY = 5_000
CHECKPOINT_EVERY = 200_000  # Save checkpoint index periodically


def count_lines(path):
    count = 0
    with open(path, "r", encoding="utf-8", errors="ignore") as f:
        for _ in f:
            count += 1
    return count


def load_jsonl_lazy(path):
    """Generator yielding parsed JSON objects from JSONL."""
    with open(path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue


def get_text(chunk):
    """Extract text content from a chunk (handles different field names)."""
    # Ring 1 uses 'content', skills might use 'content' or 'text'
    text = chunk.get("content", "") or chunk.get("text", "")
    return text[:2000]  # Cap for embedding


def embed_and_add(model, index, texts, ids_list, chunk_ids):
    """Embed texts and add to FAISS index."""
    if not texts:
        return

    embeddings = model.encode(
        texts,
        batch_size=EMBED_BATCH,
        show_progress_bar=False,
        convert_to_numpy=True,
        normalize_embeddings=True,  # L2 normalize for cosine similarity via IP
    )
    embeddings = embeddings.astype("float32")
    index.add(embeddings)
    ids_list.extend(chunk_ids)

    # Free memory
    del embeddings
    gc.collect()


def save_checkpoint(index, ids_list, suffix=""):
    """Save intermediate checkpoint."""
    cp_index = INDEX_DIR / f"v2_checkpoint{suffix}.index"
    cp_ids = INDEX_DIR / f"v2_checkpoint{suffix}_ids.json"
    faiss.write_index(index, str(cp_index))
    with open(cp_ids, "w") as f:
        json.dump(ids_list, f)
    size_mb = cp_index.stat().st_size / (1024 * 1024)
    print(f"  Checkpoint saved: {cp_index.name} ({size_mb:.1f} MB, {index.ntotal:,} vectors)")


def main():
    print("=" * 70)
    print("ZHEN EMBEDDING v2 — ring_all.jsonl -> v2.index")
    print("=" * 70)

    if not CORPUS.exists():
        print(f"ERROR: {CORPUS} not found.")
        print("Run 15_rebuild_corpus_v2.py first.")
        sys.exit(1)

    INDEX_DIR.mkdir(parents=True, exist_ok=True)

    # Count total
    print(f"\nCounting chunks in {CORPUS.name}...")
    total = count_lines(CORPUS)
    corpus_mb = CORPUS.stat().st_size / (1024 * 1024)
    print(f"  Total chunks: {total:,}")
    print(f"  Corpus size:  {corpus_mb:,.1f} MB")

    # Estimate time (GPU: ~3M chunks/hour on RX 7700 XT, CPU: ~370K chunks/hour)
    import torch
    if torch.cuda.is_available():
        est_hours = total / 3_000_000
        print(f"  GPU detected: {torch.cuda.get_device_name(0)}")
    else:
        est_hours = total / 370_000
        print(f"  GPU: not available, using CPU")
    print(f"  Estimated time: {est_hours:.1f} hours")

    # Load model
    print(f"\nLoading embedding model (all-MiniLM-L6-v2)...")
    model = SentenceTransformer("all-MiniLM-L6-v2")
    print(f"  Model loaded. Dimension: {DIM}")

    # Build index
    index = faiss.IndexFlatIP(DIM)
    all_ids = []

    batch_texts = []
    batch_ids = []
    processed = 0
    skipped = 0
    t_start = time.time()
    last_progress = 0

    print(f"\n{'='*70}")
    print(f"Starting embedding... (batch_size={EMBED_BATCH}, process_batch={PROCESS_BATCH:,})")
    print(f"{'='*70}\n")

    for chunk in load_jsonl_lazy(CORPUS):
        chunk_id = chunk.get("id", "")
        text = get_text(chunk)

        if not text.strip():
            skipped += 1
            continue

        batch_texts.append(text)
        batch_ids.append(chunk_id)

        if len(batch_texts) >= PROCESS_BATCH:
            embed_and_add(model, index, batch_texts, all_ids, batch_ids)
            processed += len(batch_texts)
            batch_texts = []
            batch_ids = []

            # Progress report
            elapsed = time.time() - t_start
            rate = processed / elapsed if elapsed > 0 else 0
            eta_s = (total - processed) / rate if rate > 0 else 0
            eta_h = eta_s / 3600
            pct = processed / total * 100

            print(
                f"  [{elapsed/60:6.1f}m] "
                f"{processed:>10,}/{total:,} ({pct:5.1f}%) | "
                f"{rate:,.0f} chunks/s | "
                f"ETA: {eta_h:.1f}h | "
                f"Vectors: {index.ntotal:,}"
            )

            # Periodic checkpoint
            if processed // CHECKPOINT_EVERY > last_progress // CHECKPOINT_EVERY:
                save_checkpoint(index, all_ids, f"_{processed//1000}k")
                last_progress = processed

    # Final batch
    if batch_texts:
        embed_and_add(model, index, batch_texts, all_ids, batch_ids)
        processed += len(batch_texts)

    elapsed = time.time() - t_start

    # Save final index
    print(f"\n{'='*70}")
    print(f"Saving final index...")
    faiss.write_index(index, str(INDEX_FILE))
    index_mb = INDEX_FILE.stat().st_size / (1024 * 1024)
    print(f"  Index: {INDEX_FILE} ({index_mb:,.1f} MB)")

    # Save IDs
    print(f"Saving ID map...")
    with open(IDS_FILE, "w") as f:
        json.dump(all_ids, f)
    ids_mb = IDS_FILE.stat().st_size / (1024 * 1024)
    print(f"  IDs: {IDS_FILE} ({ids_mb:,.1f} MB)")

    # Clean up checkpoint files
    for cp in INDEX_DIR.glob("v2_checkpoint*"):
        cp.unlink()
        print(f"  Cleaned up checkpoint: {cp.name}")

    # Final report
    print(f"\n{'='*70}")
    print(f"EMBEDDING COMPLETE")
    print(f"{'='*70}")
    print(f"  Total vectors: {index.ntotal:,}")
    print(f"  Dimension:     {DIM}")
    print(f"  Skipped:       {skipped:,} (empty content)")
    print(f"  Time:          {elapsed:.0f}s ({elapsed/3600:.1f} hours)")
    print(f"  Rate:          {processed/elapsed:,.0f} chunks/s")
    print(f"  Index file:    {INDEX_FILE} ({index_mb:,.1f} MB)")
    print(f"  ID map:        {IDS_FILE} ({ids_mb:,.1f} MB)")
    print()


if __name__ == "__main__":
    main()
