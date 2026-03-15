#!/usr/bin/env python3
"""Phase 9 — RAFT QA Pair Generation from Ring 1 corpus.

Generates question-answer pairs by sending corpus chunks to Mistral-7B,
then retrieves source document + distractor chunks via FAISS for RAFT training.

Usage:
    source ~/.venv/zhen/bin/activate
    python 05_generate_qa.py [--count 500] [--dry-run]
"""

import argparse
import json
import os
import random
import re
import sys
import time
from pathlib import Path

import faiss
import numpy as np
import requests
from sentence_transformers import SentenceTransformer

# ---------- paths ----------
CORPUS_PATH = os.path.expanduser("~/tmp/unheaded/raft/corpus/ring1.jsonl")
INDEX_DIR = os.path.expanduser("~/tmp/unheaded/raft/index")
INDEX_PATH = os.path.join(INDEX_DIR, "ring1.index")
IDS_PATH = os.path.join(INDEX_DIR, "ring1_ids.json")
OUTPUT_PATH = os.path.expanduser("~/tmp/unheaded/raft/raft_dataset.jsonl")

# ---------- config ----------
INFERENCE_URL = "http://localhost:20100/v1/completions"
EMBEDDING_MODEL = "all-MiniLM-L6-v2"
REQUEST_TIMEOUT = 120
TEMPERATURE = 0.7
MAX_TOKENS = 300
MIN_CHUNK_LEN = 200
SAVE_INTERVAL = 50  # save intermediate results every N chunks
TOP_K = 5  # FAISS retrieval count


def load_corpus():
    """Load corpus from ring1.jsonl and return list of chunk dicts."""
    print("Loading corpus ...")
    chunks = []
    with open(CORPUS_PATH, "r") as f:
        for line in f:
            line = line.strip()
            if line:
                chunks.append(json.loads(line))
    print(f"  Loaded {len(chunks):,} chunks")
    return chunks


def sample_chunks(chunks, count):
    """Sample high-quality chunks, prioritizing .md and .go files, skipping short ones."""
    # Filter out short chunks
    eligible = [c for c in chunks if len(c.get("content", "")) >= MIN_CHUNK_LEN]
    print(f"  {len(eligible):,} chunks pass minimum length filter ({MIN_CHUNK_LEN} chars)")

    # Categorize by priority
    high_priority = []  # .md and .go files
    normal_priority = []

    for c in eligible:
        source = c.get("source", "")
        if source.endswith(".md") or source.endswith(".go"):
            high_priority.append(c)
        else:
            normal_priority.append(c)

    print(f"  High priority (.md/.go): {len(high_priority):,}")
    print(f"  Normal priority: {len(normal_priority):,}")

    # Sample: take as many high-priority as we can (up to 70%), fill rest from normal
    high_count = min(len(high_priority), int(count * 0.7))
    normal_count = min(len(normal_priority), count - high_count)

    # If we still need more, take additional from whichever pool has surplus
    if high_count + normal_count < count:
        remaining = count - high_count - normal_count
        if len(high_priority) > high_count:
            extra = min(len(high_priority) - high_count, remaining)
            high_count += extra
            remaining -= extra
        if remaining > 0 and len(normal_priority) > normal_count:
            normal_count += min(len(normal_priority) - normal_count, remaining)

    random.seed(42)
    sampled = random.sample(high_priority, high_count) + random.sample(normal_priority, normal_count)
    random.shuffle(sampled)

    print(f"  Sampled {len(sampled)} chunks total")
    return sampled


def generate_qa_pair(chunk_content, source_file):
    """Call Mistral-7B to generate a QA pair from chunk content."""
    # Truncate very long chunks to avoid exceeding context
    content = chunk_content[:3000]

    prompt = f"""<s>[INST] You are a dataset generator. Given the following source code or documentation from the Unheaded infrastructure platform, generate exactly ONE question-answer pair.

The question should be specific and answerable from the given context.
The answer should quote or closely reference the source material.

SOURCE:
{content}

Respond in this exact JSON format:
{{"question": "...", "answer": "..."}}
[/INST]"""

    try:
        response = requests.post(
            INFERENCE_URL,
            json={
                "prompt": prompt,
                "max_tokens": MAX_TOKENS,
                "temperature": TEMPERATURE,
                "top_p": 0.9,
                "stop": ["[INST]", "</s>"],
            },
            timeout=REQUEST_TIMEOUT,
        )

        if response.status_code != 200:
            return None, f"HTTP {response.status_code}"

        result = response.json()
        text = result["choices"][0]["text"].strip()

        # Try to extract JSON from the response
        qa = parse_qa_json(text)
        if qa is None:
            return None, f"JSON parse failed: {text[:100]}"

        return qa, None

    except requests.exceptions.Timeout:
        return None, "Timeout (120s)"
    except requests.exceptions.ConnectionError:
        return None, "Connection refused"
    except Exception as e:
        return None, str(e)


def parse_qa_json(text):
    """Extract a QA JSON object from model output. Tries multiple strategies."""
    # Strategy 1: direct parse
    try:
        obj = json.loads(text)
        if "question" in obj and "answer" in obj:
            return obj
    except json.JSONDecodeError:
        pass

    # Strategy 2: find JSON block with regex
    match = re.search(r'\{[^{}]*"question"\s*:\s*"[^"]*"[^{}]*"answer"\s*:\s*"[^"]*"[^{}]*\}', text, re.DOTALL)
    if match:
        try:
            obj = json.loads(match.group())
            if "question" in obj and "answer" in obj:
                return obj
        except json.JSONDecodeError:
            pass

    # Strategy 3: more permissive — find any JSON-like block
    match = re.search(r'\{.*?\}', text, re.DOTALL)
    if match:
        try:
            obj = json.loads(match.group())
            if "question" in obj and "answer" in obj:
                return obj
        except json.JSONDecodeError:
            pass

    return None


def retrieve_chunks(question, index, id_map, corpus_lookup, embedding_model, k=TOP_K):
    """Retrieve top-k chunks from FAISS for the given question."""
    q_vec = embedding_model.encode(
        [question], convert_to_numpy=True, normalize_embeddings=True
    ).astype("float32")

    distances, indices = index.search(q_vec, k)

    retrieved = []
    for idx, dist in zip(indices[0], distances[0]):
        if idx < 0:
            continue
        chunk_id = id_map.get(str(idx))
        if chunk_id and chunk_id in corpus_lookup:
            data = corpus_lookup[chunk_id]
            retrieved.append({
                "chunk_id": chunk_id,
                "content": data["content"],
                "source": data["source"],
                "distance": float(dist),
            })
    return retrieved


def save_results(results, path):
    """Save results as JSONL."""
    with open(path, "w") as f:
        for r in results:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")


def main():
    parser = argparse.ArgumentParser(description="RAFT QA Pair Generator")
    parser.add_argument("--count", type=int, default=500, help="Number of chunks to sample (default: 500)")
    parser.add_argument("--dry-run", action="store_true", help="Sample and print stats without calling inference")
    args = parser.parse_args()

    # Load corpus
    chunks = load_corpus()

    # Sample chunks
    sampled = sample_chunks(chunks, args.count)

    if args.dry_run:
        print("\n--- Dry run: showing 3 sample chunks ---")
        for c in sampled[:3]:
            print(f"\n  Source: {c['source']}  Len: {len(c['content'])} chars")
            print(f"  Preview: {c['content'][:150]}...")
        print(f"\nDry run complete. Would process {len(sampled)} chunks.")
        return

    # Load FAISS index and embedding model
    print("\nLoading FAISS index ...")
    index = faiss.read_index(INDEX_PATH)
    print(f"  Index: {index.ntotal:,} vectors")

    print("Loading ID mapping ...")
    with open(IDS_PATH, "r") as f:
        raw_ids = json.load(f)
        if isinstance(raw_ids, list):
            id_map = {str(i): v for i, v in enumerate(raw_ids)}
        else:
            id_map = raw_ids

    # Build corpus lookup from the full corpus
    print("Building corpus lookup ...")
    corpus_lookup = {}
    for c in chunks:
        corpus_lookup[c["id"]] = {
            "content": c["content"],
            "source": c.get("source", ""),
            "type": c.get("type", "unknown"),
        }

    print(f"Loading embedding model '{EMBEDDING_MODEL}' ...")
    embedding_model = SentenceTransformer(EMBEDDING_MODEL)

    # Check inference server
    print("\nChecking inference server ...")
    try:
        resp = requests.get("http://localhost:20100/v1/models", timeout=10)
        if resp.status_code == 200:
            print("  Inference server is up.")
        else:
            print(f"  Warning: server returned {resp.status_code}")
    except Exception as e:
        print(f"  WARNING: Cannot reach inference server: {e}")
        print("  Proceeding anyway — errors will be logged per-chunk.")

    # Generate QA pairs
    results = []
    errors = []
    total = len(sampled)
    t_start = time.time()

    print(f"\n{'='*60}")
    print(f"Generating QA pairs for {total} chunks ...")
    print(f"{'='*60}\n")

    for i, chunk in enumerate(sampled):
        chunk_id = chunk["id"]
        source_file = chunk.get("source", "unknown")
        content = chunk["content"]

        # Generate QA pair via inference
        qa, err = generate_qa_pair(content, source_file)

        if err:
            errors.append({"chunk_id": chunk_id, "source": source_file, "error": err})
            if (i + 1) % 10 == 0 or i == 0:
                print(f"  [{i+1}/{total}] SKIP {source_file} — {err}")
            continue

        # Retrieve top-5 chunks for RAFT source/distractor pattern
        retrieved = retrieve_chunks(qa["question"], index, id_map, corpus_lookup, embedding_model)

        # Determine source document and distractors
        # The source document is the chunk we used to generate the question
        distractor_chunks = []
        for r in retrieved:
            if r["chunk_id"] != chunk_id:
                distractor_chunks.append({
                    "chunk_id": r["chunk_id"],
                    "content": r["content"][:500],  # truncate for storage
                    "source": r["source"],
                    "distance": r["distance"],
                })

        record = {
            "question": qa["question"],
            "answer": qa["answer"],
            "source_chunk_id": chunk_id,
            "source_content": content[:2000],  # truncate for storage
            "distractor_chunks": distractor_chunks[:4],  # at most 4 distractors
            "source_file": source_file,
        }
        results.append(record)

        # Progress reporting
        if (i + 1) % 10 == 0 or i == 0:
            elapsed = time.time() - t_start
            rate = (i + 1) / elapsed if elapsed > 0 else 0
            eta = (total - i - 1) / rate if rate > 0 else 0
            print(f"  Generated {len(results)}/{total} QA pairs  "
                  f"({len(errors)} errors)  "
                  f"[{rate:.1f} chunks/s, ETA {eta/60:.0f}min]")

        # Save intermediate results
        if len(results) > 0 and len(results) % SAVE_INTERVAL == 0:
            save_results(results, OUTPUT_PATH)
            print(f"  >> Intermediate save: {len(results)} pairs -> {OUTPUT_PATH}")

    # Final save
    save_results(results, OUTPUT_PATH)

    # Summary
    elapsed = time.time() - t_start
    print(f"\n{'='*60}")
    print(f"RAFT QA Generation Complete")
    print(f"{'='*60}")
    print(f"  Total chunks processed: {total}")
    print(f"  QA pairs generated:     {len(results)}")
    print(f"  Errors/skipped:         {len(errors)}")
    print(f"  Time elapsed:           {elapsed:.0f}s ({elapsed/60:.1f}min)")
    print(f"  Output:                 {OUTPUT_PATH}")

    if results:
        print(f"\n--- Sample QA pairs (first 3) ---")
        for j, r in enumerate(results[:3]):
            print(f"\n  [{j+1}] Source: {r['source_file']}")
            print(f"      Q: {r['question']}")
            print(f"      A: {r['answer'][:200]}...")
            print(f"      Distractors: {len(r['distractor_chunks'])}")

    if errors:
        print(f"\n--- Error summary ---")
        error_types = {}
        for e in errors:
            t = e["error"].split(":")[0] if ":" in e["error"] else e["error"]
            error_types[t] = error_types.get(t, 0) + 1
        for t, c in sorted(error_types.items(), key=lambda x: -x[1]):
            print(f"  {t}: {c}")

    print("\nDone.")


if __name__ == "__main__":
    main()
