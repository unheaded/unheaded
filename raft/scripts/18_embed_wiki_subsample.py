#!/usr/bin/env python3
"""
18_embed_wiki_subsample.py — Embed Wikipedia subsample and add to FAISS index.

Takes the subsampled Wikipedia JSONL, embeds with all-MiniLM-L6-v2,
adds vectors to existing v2 index, and appends chunks to ring_all.jsonl.

SPDX-License-Identifier: GPL-3.0-or-later
"""
import json
import sys
import time
import numpy as np
import faiss
from pathlib import Path
from sentence_transformers import SentenceTransformer

INDEX_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
CORPUS_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
WIKI_SUBSAMPLE = CORPUS_DIR / 'wikipedia_subsample.jsonl'
RING_ALL = CORPUS_DIR / 'ring_all.jsonl'

def main():
    if not WIKI_SUBSAMPLE.exists():
        print(f"Run 17_subsample_wikipedia.py first: {WIKI_SUBSAMPLE}")
        sys.exit(1)

    # Load existing index
    index_path = INDEX_DIR / 'v2.index'
    ids_path = INDEX_DIR / 'v2_ids.json'
    print(f"Loading existing index: {index_path}")
    index = faiss.read_index(str(index_path))
    print(f"  Existing vectors: {index.ntotal:,}")

    with open(ids_path) as f:
        ids = json.load(f)
    print(f"  Existing IDs: {len(ids):,}")

    # Load embedding model
    print("Loading embedding model...")
    model = SentenceTransformer('all-MiniLM-L6-v2')

    # Load Wikipedia subsample
    print(f"Loading Wikipedia subsample: {WIKI_SUBSAMPLE}")
    chunks = []
    with open(WIKI_SUBSAMPLE, 'r', encoding='utf-8', errors='ignore') as f:
        for line in f:
            try:
                chunk = json.loads(line)
                content = chunk.get('content', '')
                if content and len(content) > 50:  # skip tiny chunks
                    chunks.append(chunk)
            except (json.JSONDecodeError, KeyError):
                continue
    print(f"  Loaded {len(chunks):,} chunks")

    if not chunks:
        print("No chunks to embed!")
        sys.exit(1)

    # Embed in batches
    print(f"Embedding {len(chunks):,} chunks...")
    batch_size = 256
    all_embeddings = []
    start = time.time()

    for i in range(0, len(chunks), batch_size):
        batch = chunks[i:i+batch_size]
        texts = [c['content'][:512] for c in batch]  # truncate for embedding
        embeddings = model.encode(texts, convert_to_numpy=True, show_progress_bar=False)
        embeddings = embeddings.astype('float32')
        # Normalize for cosine similarity
        norms = np.linalg.norm(embeddings, axis=1, keepdims=True)
        norms[norms == 0] = 1
        embeddings = embeddings / norms
        all_embeddings.append(embeddings)

        if (i // batch_size) % 50 == 0:
            elapsed = time.time() - start
            rate = (i + len(batch)) / elapsed if elapsed > 0 else 0
            print(f"  [{i+len(batch):,}/{len(chunks):,}] {rate:.0f} chunks/sec")

    all_embeddings = np.vstack(all_embeddings)
    elapsed = time.time() - start
    print(f"  Embedded {all_embeddings.shape[0]:,} chunks in {elapsed:.1f}s ({all_embeddings.shape[0]/elapsed:.0f}/sec)")

    # Add to FAISS index
    print("Adding to FAISS index...")
    index.add(all_embeddings)
    print(f"  New total vectors: {index.ntotal:,}")

    # Update IDs
    for chunk in chunks:
        ids.append(chunk['id'])
    print(f"  New total IDs: {len(ids):,}")

    # Save updated index
    print("Saving updated index...")
    faiss.write_index(index, str(index_path))
    with open(ids_path, 'w') as f:
        json.dump(ids, f)
    print(f"  Saved: {index_path} ({index_path.stat().st_size / 1e9:.2f} GB)")

    # Append to ring_all.jsonl
    print("Appending Wikipedia chunks to ring_all.jsonl...")
    with open(RING_ALL, 'a', encoding='utf-8') as f:
        for chunk in chunks:
            f.write(json.dumps(chunk, ensure_ascii=False) + '\n')
    print(f"  ring_all.jsonl: {RING_ALL.stat().st_size / 1e9:.2f} GB")

    print(f"\nDone! Added {len(chunks):,} Wikipedia chunks to FAISS + corpus.")
    print(f"Total vectors: {index.ntotal:,}")

if __name__ == '__main__':
    main()
