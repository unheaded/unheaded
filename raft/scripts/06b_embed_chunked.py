#!/usr/bin/env python3
"""
Chunked embedding for large corpus — processes in batches to avoid OOM.
Builds combined FAISS index from Ring 1 + Ring 2-4 corpus.
"""
import json
import time
from pathlib import Path

import faiss
from sentence_transformers import SentenceTransformer

RING1_CORPUS = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'
RING234_CORPUS = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring234.jsonl'
INDEX_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
EMBED_BATCH = 64
PROCESS_BATCH = 50000  # Process 50K chunks at a time to limit memory

def load_jsonl_lazy(path):
    """Generator that yields chunks from JSONL without loading all into memory."""
    with open(path, 'r', encoding='utf-8', errors='ignore') as f:
        for line in f:
            try:
                yield json.loads(line)
            except json.JSONDecodeError:
                continue

def count_lines(path):
    count = 0
    with open(path, 'r') as f:
        for _ in f:
            count += 1
    return count

def main():
    INDEX_DIR.mkdir(parents=True, exist_ok=True)

    print("Loading embedding model...")
    model = SentenceTransformer('all-MiniLM-L6-v2')
    dim = 384

    # Count total chunks
    r1_count = count_lines(RING1_CORPUS) if RING1_CORPUS.exists() else 0
    r234_count = count_lines(RING234_CORPUS) if RING234_CORPUS.exists() else 0
    total = r1_count + r234_count
    print(f"Ring 1: {r1_count:,} chunks")
    print(f"Ring 2-4: {r234_count:,} chunks")
    print(f"Total: {total:,} chunks")

    # Build index incrementally
    index = faiss.IndexFlatIP(dim)  # Inner product (cosine after normalization)
    all_ids = []
    corpus_out = INDEX_DIR / 'combined_corpus.jsonl'  # Use JSONL to avoid loading all into memory

    start_time = time.time()
    processed = 0

    with open(corpus_out, 'w') as corpus_fh:
        # Process Ring 1
        if RING1_CORPUS.exists():
            print(f"\n=== Processing Ring 1 ({r1_count:,} chunks) ===")
            batch_texts = []
            batch_meta = []

            for chunk in load_jsonl_lazy(RING1_CORPUS):
                chunk_id = chunk['id']
                content = chunk.get('content', '')
                if not content.strip():
                    continue

                batch_texts.append(content[:2000])  # Cap content length for embedding
                batch_meta.append({
                    'id': chunk_id,
                    'source': chunk.get('source', ''),
                    'type': chunk.get('type', 'unknown'),
                    'ring': 1,
                })

                if len(batch_texts) >= PROCESS_BATCH:
                    _embed_and_add(model, index, batch_texts, batch_meta, all_ids, corpus_fh, dim)
                    processed += len(batch_texts)
                    elapsed = time.time() - start_time
                    rate = processed / elapsed
                    print(f"  Processed {processed:,}/{total:,} ({processed*100/total:.1f}%) — {rate:.0f} chunks/s")
                    batch_texts = []
                    batch_meta = []

            if batch_texts:
                _embed_and_add(model, index, batch_texts, batch_meta, all_ids, corpus_fh, dim)
                processed += len(batch_texts)

            print(f"  Ring 1 complete: {processed:,} chunks indexed")

        # Process Ring 2-4
        if RING234_CORPUS.exists():
            print(f"\n=== Processing Ring 2-4 ({r234_count:,} chunks) ===")
            batch_texts = []
            batch_meta = []

            for chunk in load_jsonl_lazy(RING234_CORPUS):
                chunk_id = chunk['id']
                content = chunk.get('content', '')
                if not content.strip():
                    continue

                batch_texts.append(content[:2000])
                batch_meta.append({
                    'id': chunk_id,
                    'source': chunk.get('source', ''),
                    'type': chunk.get('type', 'unknown'),
                    'ring': chunk.get('ring', 4),
                })

                if len(batch_texts) >= PROCESS_BATCH:
                    _embed_and_add(model, index, batch_texts, batch_meta, all_ids, corpus_fh, dim)
                    processed += len(batch_texts)
                    elapsed = time.time() - start_time
                    rate = processed / elapsed
                    print(f"  Processed {processed:,}/{total:,} ({processed*100/total:.1f}%) — {rate:.0f} chunks/s")
                    batch_texts = []
                    batch_meta = []

            if batch_texts:
                _embed_and_add(model, index, batch_texts, batch_meta, all_ids, corpus_fh, dim)
                processed += len(batch_texts)

    elapsed = time.time() - start_time

    # Save index
    print(f"\nSaving FAISS index ({index.ntotal:,} vectors)...")
    faiss.write_index(index, str(INDEX_DIR / 'combined.index'))

    # Save ID map
    print("Saving ID map...")
    with open(INDEX_DIR / 'combined_ids.json', 'w') as f:
        json.dump(all_ids, f)

    print(f"\n{'='*60}")
    print(f"DONE in {elapsed:.0f}s ({elapsed/60:.1f} min)")
    print(f"Total vectors: {index.ntotal:,}")
    print(f"Dimension: {dim}")
    print(f"Index file: {INDEX_DIR / 'combined.index'}")
    print(f"Corpus file: {corpus_out}")
    print(f"ID map: {INDEX_DIR / 'combined_ids.json'}")

    # File sizes
    for f in [INDEX_DIR / 'combined.index', INDEX_DIR / 'combined_ids.json', corpus_out]:
        if f.exists():
            size = f.stat().st_size / (1024 * 1024)
            print(f"  {f.name}: {size:.1f} MB")


def _embed_and_add(model, index, texts, metas, all_ids, corpus_fh, dim):
    """Embed a batch of texts and add to FAISS index."""
    embeddings = model.encode(
        texts,
        batch_size=EMBED_BATCH,
        show_progress_bar=False,
        convert_to_numpy=True,
        normalize_embeddings=True,  # L2 normalize for cosine similarity
    )
    embeddings = embeddings.astype('float32')
    index.add(embeddings)

    for meta in metas:
        all_ids.append(meta['id'])
        corpus_fh.write(json.dumps({
            'id': meta['id'],
            'source': meta['source'],
            'type': meta['type'],
            'ring': meta['ring'],
        }) + '\n')


if __name__ == '__main__':
    main()
