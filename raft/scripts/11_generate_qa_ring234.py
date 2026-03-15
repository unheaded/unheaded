#!/usr/bin/env python3
"""
11_generate_qa_ring234.py — Generate QA pairs from Ring 2-4 corpus

Samples high-quality chunks from ring234.jsonl and generates question-answer
pairs using the Mistral inference server, for RAFT training augmentation.

Ring distribution targets:
  - Ring 2 (GitHub repos, kernel BPF, docs): 120 chunks
  - Ring 3 (ArXiv papers, seminal papers):    50 chunks
  - Ring 4 (RFCs):                            30 chunks

Filters:
  - Skip chunks < 300 chars
  - Skip chunks that are mostly code with no comments

Output: raft_dataset_ring234.jsonl (same format as raft_dataset.jsonl)
"""
import sys
import json
import time
import random
import re
import argparse
from pathlib import Path

CORPUS_FILE = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring234.jsonl'
OUTPUT_FILE = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'raft_dataset_ring234.jsonl'
CHECKPOINT_FILE = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'raft_dataset_ring234_checkpoint.json'
INFERENCE_URL = 'http://localhost:20100/v1/completions'

# Sampling targets
RING_TARGETS = {2: 120, 3: 50, 4: 30}


def is_mostly_code(content):
    """Return True if chunk is mostly code with little commentary."""
    lines = content.strip().split('\n')
    if not lines:
        return True

    comment_chars = 0
    code_chars = 0

    for line in lines:
        stripped = line.strip()
        # Count comment lines
        if stripped.startswith(('#', '//', '/*', '*', '--', ';;', '"""', "'''")):
            comment_chars += len(stripped)
        elif stripped:
            code_chars += len(stripped)

    total = comment_chars + code_chars
    if total == 0:
        return True

    # If less than 15% is comments/prose, it's mostly code
    comment_ratio = comment_chars / total
    return comment_ratio < 0.15 and code_chars > 200


def is_quality_chunk(chunk):
    """Filter for high-quality chunks suitable for QA generation."""
    content = chunk.get('content', '')

    # Must be at least 300 chars
    if len(content) < 300:
        return False

    # Skip mostly-code chunks
    if chunk.get('type') == 'code' and is_mostly_code(content):
        return False

    # Skip chunks that are just tables or lists of imports
    if content.count('import ') > 10:
        return False
    if content.count('|') > content.count('\n') * 0.5 and content.count('|') > 20:
        return False

    return True


def sample_chunks(limit_per_ring=None):
    """Sample high-quality chunks from ring234.jsonl."""
    targets = dict(RING_TARGETS)
    if limit_per_ring:
        for ring in targets:
            targets[ring] = min(targets[ring], limit_per_ring)

    # Collect candidates per ring
    candidates = {2: [], 3: [], 4: []}
    print(f"  Scanning {CORPUS_FILE.name}...")

    with open(CORPUS_FILE, 'r') as f:
        for line_num, line in enumerate(f):
            if line_num % 100000 == 0 and line_num > 0:
                print(f"    Scanned {line_num:,} lines...")
            try:
                chunk = json.loads(line)
            except json.JSONDecodeError:
                continue

            ring = chunk.get('ring', 0)
            if ring not in candidates:
                continue

            if is_quality_chunk(chunk):
                candidates[ring].append(chunk)

    for ring, chunks in sorted(candidates.items()):
        print(f"    Ring {ring}: {len(chunks):,} quality candidates")

    # Sample
    sampled = []
    for ring, target in sorted(targets.items()):
        pool = candidates.get(ring, [])
        if len(pool) <= target:
            sampled.extend(pool)
            print(f"    Ring {ring}: using all {len(pool)} candidates (target: {target})")
        else:
            # Prefer doc-type chunks over code, then random
            docs = [c for c in pool if c.get('type') == 'doc']
            codes = [c for c in pool if c.get('type') != 'doc']
            random.shuffle(docs)
            random.shuffle(codes)

            selected = docs[:target]
            if len(selected) < target:
                selected.extend(codes[:target - len(selected)])

            sampled.extend(selected[:target])
            print(f"    Ring {ring}: sampled {len(selected[:target])} from {len(pool)} candidates")

    random.shuffle(sampled)
    print(f"  Total sampled: {len(sampled)} chunks")
    return sampled


def generate_qa(chunk_content, source, max_retries=2):
    """Call inference server to generate a QA pair from a chunk."""
    import requests

    prompt = f"""<s>[INST] You are a dataset generator for training an AI about systems engineering.
Given the following technical content, generate exactly ONE question-answer pair.
The question should test understanding of the concepts described.
The answer should be specific and reference the source material.

SOURCE:
{chunk_content[:3000]}

Respond in this exact JSON format:
{{"question": "...", "answer": "..."}} [/INST]"""

    for attempt in range(max_retries + 1):
        try:
            response = requests.post(
                INFERENCE_URL,
                json={
                    "prompt": prompt,
                    "max_tokens": 400,
                    "temperature": 0.4,
                    "top_p": 0.9,
                    "stop": ["[INST]", "</s>"],
                },
                timeout=90,
            )

            if response.status_code != 200:
                if attempt < max_retries:
                    time.sleep(2)
                    continue
                return None

            result = response.json()
            text = result['choices'][0]['text'].strip()

            # Parse JSON from response
            # Try to find JSON in the response
            json_match = re.search(r'\{[^{}]*"question"[^{}]*"answer"[^{}]*\}', text, re.DOTALL)
            if not json_match:
                # Try more lenient parse
                json_match = re.search(r'\{.*\}', text, re.DOTALL)

            if json_match:
                qa = json.loads(json_match.group())
                if 'question' in qa and 'answer' in qa:
                    if len(qa['question']) > 10 and len(qa['answer']) > 20:
                        return qa

            if attempt < max_retries:
                time.sleep(1)
                continue
            return None

        except requests.exceptions.ConnectionError:
            print("    ERROR: Inference server not reachable")
            return None
        except requests.exceptions.Timeout:
            if attempt < max_retries:
                time.sleep(2)
                continue
            return None
        except (json.JSONDecodeError, KeyError, IndexError):
            if attempt < max_retries:
                time.sleep(1)
                continue
            return None

    return None


def load_checkpoint():
    """Load progress checkpoint."""
    if CHECKPOINT_FILE.exists():
        try:
            with open(CHECKPOINT_FILE, 'r') as f:
                return json.load(f)
        except Exception:
            pass
    return {'completed_ids': [], 'total_generated': 0, 'total_failed': 0}


def save_checkpoint(state):
    """Save progress checkpoint."""
    with open(CHECKPOINT_FILE, 'w') as f:
        json.dump(state, f)


def main():
    parser = argparse.ArgumentParser(description='Generate QA pairs from Ring 2-4 corpus')
    parser.add_argument('--batch', type=int, default=200,
                        help='Number of chunks to process (default: 200)')
    parser.add_argument('--timeout', type=int, default=600,
                        help='Max runtime in seconds (default: 600)')
    parser.add_argument('--dry-run', action='store_true',
                        help='Sample chunks but do not call inference')
    parser.add_argument('--resume', action='store_true',
                        help='Resume from checkpoint')
    args = parser.parse_args()

    print("=" * 60)
    print("RING 2-4 QA PAIR GENERATION")
    print("=" * 60)

    start_time = time.time()

    # Load checkpoint if resuming
    checkpoint = load_checkpoint() if args.resume else {
        'completed_ids': [], 'total_generated': 0, 'total_failed': 0
    }

    # Scale targets proportionally if batch != 200
    if args.batch != 200:
        scale = args.batch / 200
        for ring in RING_TARGETS:
            RING_TARGETS[ring] = max(1, int(RING_TARGETS[ring] * scale))
        print(f"  Scaled targets: {RING_TARGETS}")

    # Sample chunks
    print("\n[1/3] Sampling chunks...")
    chunks = sample_chunks()

    # Filter out already-completed chunks
    completed_set = set(checkpoint['completed_ids'])
    chunks = [c for c in chunks if c['id'] not in completed_set]
    print(f"  After filtering completed: {len(chunks)} remaining")

    if args.dry_run:
        print("\n[DRY RUN] Would generate QA for these chunks:")
        for i, c in enumerate(chunks[:10]):
            print(f"  {i+1}. Ring {c['ring']} | {c['source'][:60]} | {len(c['content'])} chars")
        print(f"  ... and {len(chunks) - 10} more" if len(chunks) > 10 else "")
        return

    # Check inference server
    print("\n[2/3] Checking inference server...")
    import requests
    try:
        r = requests.get('http://localhost:20100/v1/models', timeout=5)
        if r.status_code == 200:
            models = r.json()
            print(f"  Server OK: {models.get('models', [{}])[0].get('name', 'unknown')}" if 'models' in models else f"  Server OK")
        else:
            print(f"  WARNING: Server returned {r.status_code}")
    except Exception as e:
        print(f"  ERROR: Cannot reach inference server: {e}")
        print(f"  Make sure llama-server is running on port 20100")
        sys.exit(1)

    # Generate QA pairs
    print(f"\n[3/3] Generating QA pairs (timeout: {args.timeout}s)...")

    generated = checkpoint['total_generated']
    failed = checkpoint['total_failed']
    batch_generated = 0
    save_interval = 20

    # Open output file in append mode
    with open(OUTPUT_FILE, 'a') as out_f:
        for i, chunk in enumerate(chunks):
            # Check timeout
            elapsed = time.time() - start_time
            if elapsed > args.timeout:
                print(f"\n  TIMEOUT reached ({args.timeout}s). Stopping.")
                break

            chunk_id = chunk['id']
            source = chunk.get('source', 'unknown')
            ring = chunk.get('ring', 0)

            print(f"  [{i+1}/{len(chunks)}] Ring {ring} | {source[:50]}...", end=' ', flush=True)

            qa = generate_qa(chunk['content'], source)

            if qa:
                # Build output record (matching existing raft_dataset.jsonl format)
                record = {
                    'question': qa['question'],
                    'answer': qa['answer'],
                    'oracle_chunk_id': chunk_id,
                    'oracle_content': chunk['content'][:2000],
                    'distractor_chunks': [],
                    'source_file': source,
                }
                out_f.write(json.dumps(record) + '\n')
                out_f.flush()

                generated += 1
                batch_generated += 1
                print(f"OK ({len(qa['question'])} / {len(qa['answer'])} chars)")
            else:
                failed += 1
                print("FAILED")

            # Save checkpoint
            checkpoint['completed_ids'].append(chunk_id)
            checkpoint['total_generated'] = generated
            checkpoint['total_failed'] = failed

            if (i + 1) % save_interval == 0:
                save_checkpoint(checkpoint)
                elapsed = time.time() - start_time
                rate = batch_generated / elapsed * 60 if elapsed > 0 else 0
                print(f"    [Checkpoint] {generated} generated, {failed} failed, "
                      f"{elapsed:.0f}s elapsed, {rate:.1f}/min")

    # Final checkpoint
    save_checkpoint(checkpoint)

    # Summary
    elapsed = time.time() - start_time
    print("\n" + "=" * 60)
    print("GENERATION COMPLETE")
    print("=" * 60)
    print(f"  Generated: {batch_generated} new QA pairs (total: {generated})")
    print(f"  Failed:    {failed}")
    print(f"  Elapsed:   {elapsed:.0f}s")
    print(f"  Rate:      {batch_generated / elapsed * 60:.1f} pairs/min" if elapsed > 0 else "")
    print(f"  Output:    {OUTPUT_FILE}")

    # Count total in output file
    if OUTPUT_FILE.exists():
        with open(OUTPUT_FILE) as f:
            total_lines = sum(1 for _ in f)
        print(f"  Total QA pairs in file: {total_lines}")
    print()


if __name__ == '__main__':
    main()
