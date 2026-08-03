#!/usr/bin/env python3
"""Phase 4 — Embed Ring 1 corpus and build FAISS index."""

import json
import os
import time

import faiss
from sentence_transformers import SentenceTransformer

# ---------- paths ----------
CORPUS_PATH = os.path.expanduser("~/tmp/unheaded/raft/corpus/ring1.jsonl")
INDEX_DIR = os.path.expanduser("~/tmp/unheaded/raft/index")
INDEX_PATH = os.path.join(INDEX_DIR, "ring1.index")
IDS_PATH = os.path.join(INDEX_DIR, "ring1_ids.json")
CORPUS_JSON_PATH = os.path.join(INDEX_DIR, "ring1_corpus.json")

# ---------- config ----------
MODEL_NAME = "all-MiniLM-L6-v2"
BATCH_SIZE = 64
DIMENSION = 384

# ---------- load corpus ----------
print("Loading corpus …")
chunks = []
with open(CORPUS_PATH, "r") as f:
    for line in f:
        if line.strip():
            chunks.append(json.loads(line))
print(f"  Loaded {len(chunks):,} chunks")

# ---------- load model ----------
print(f"Loading model '{MODEL_NAME}' …")
model = SentenceTransformer(MODEL_NAME)

# ---------- embed ----------
texts = [c["content"] for c in chunks]
print(f"Encoding {len(texts):,} chunks (batch_size={BATCH_SIZE}) …")
t0 = time.time()
embeddings = model.encode(
    texts,
    batch_size=BATCH_SIZE,
    show_progress_bar=True,
    convert_to_numpy=True,
    normalize_embeddings=True,  # L2-normalise so L2 distance ≈ cosine
)
elapsed = time.time() - t0
print(f"  Encoding done in {elapsed:.1f}s  ({len(texts)/elapsed:.0f} chunks/s)")

# ---------- build FAISS index ----------
print("Building FAISS IndexFlatL2 …")
embeddings = embeddings.astype("float32")
index = faiss.IndexFlatL2(DIMENSION)
index.add(embeddings)
print(f"  Index contains {index.ntotal:,} vectors, dimension={DIMENSION}")

# ---------- save ----------
os.makedirs(INDEX_DIR, exist_ok=True)

faiss.write_index(index, INDEX_PATH)
index_size_mb = os.path.getsize(INDEX_PATH) / (1024 * 1024)
print(f"  Saved FAISS index → {INDEX_PATH}  ({index_size_mb:.1f} MB)")

chunk_ids = [c["id"] for c in chunks]
with open(IDS_PATH, "w") as f:
    json.dump(chunk_ids, f)
print(f"  Saved ID mapping → {IDS_PATH}")

corpus_data = [{"id": c["id"], "source": c["source"], "content": c["content"]} for c in chunks]
with open(CORPUS_JSON_PATH, "w") as f:
    json.dump(corpus_data, f)
corpus_size_mb = os.path.getsize(CORPUS_JSON_PATH) / (1024 * 1024)
print(f"  Saved corpus JSON → {CORPUS_JSON_PATH}  ({corpus_size_mb:.1f} MB)")

# ---------- sanity search ----------
print("\n--- Sanity search: 'What is Unheaded?' ---")
query = "What is Unheaded?"
q_vec = model.encode([query], convert_to_numpy=True, normalize_embeddings=True).astype("float32")
D, I = index.search(q_vec, 3)

for rank, (dist, idx) in enumerate(zip(D[0], I[0]), 1):
    c = corpus_data[idx]
    snippet = c["content"][:200].replace("\n", " ")
    print(f"\n  #{rank}  dist={dist:.4f}  source={c['source']}  id={c['id'][:12]}…")
    print(f"       {snippet}…")

print("\nDone.")
