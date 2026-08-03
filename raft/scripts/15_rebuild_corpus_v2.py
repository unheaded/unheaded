#!/usr/bin/env python3
"""
15_rebuild_corpus_v2.py — Rebuild Zhen corpus with SO, SF, Unix.SE, and skills.

Combines all available corpus sources into a single ring_all.jsonl:
  - Ring 1 (21K) — ALL
  - Ring 2-4 (573K) — ALL
  - Skills (355) — ALL
  - Stack Overflow (13.2M) — SAMPLED by score (target ~200K)
  - ServerFault (221K) — ALL
  - Unix.SE (223K) — ALL
  - Wikipedia — SKIPPED (still extracting)

Target: ~1M total chunks.
"""

import json
import random
import sys
import time
from pathlib import Path

CORPUS_DIR = Path.home() / "tmp" / "unheaded" / "raft" / "corpus"

# Input files
RING1 = CORPUS_DIR / "ring1.jsonl"
RING234 = CORPUS_DIR / "ring234.jsonl"
SKILLS = CORPUS_DIR / "ring1_skills.jsonl"
SO = CORPUS_DIR / "stackoverflow.jsonl"
SF = CORPUS_DIR / "serverfault.jsonl"
UNIX_SE = CORPUS_DIR / "unix_se.jsonl"

# Output
OUTPUT = CORPUS_DIR / "ring_all.jsonl"

# SO sampling config
SO_HIGH_SCORE = 50       # Include ALL chunks with score >= 50
SO_MED_SCORE = 10        # Sample from chunks with score >= 10
SO_TARGET_TOTAL = 200_000  # Target total SO chunks (high + sampled medium)

PROGRESS_EVERY = 100_000
random.seed(42)  # Reproducible sampling


def count_lines(path):
    """Fast line count without loading content."""
    count = 0
    with open(path, "r", encoding="utf-8", errors="ignore") as f:
        for _ in f:
            count += 1
    return count


def copy_all(src_path, out_fh, label):
    """Copy all lines from src JSONL to output, return count."""
    count = 0
    with open(src_path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            out_fh.write(line + "\n")
            count += 1
            if count % PROGRESS_EVERY == 0:
                print(f"  [{label}] {count:>10,} written...")
    return count


def sample_stackoverflow(so_path, out_fh):
    """Two-pass SO sampling: collect high-score, reservoir-sample medium-score."""
    print("\n=== Stack Overflow Sampling ===")
    print(f"  Strategy: ALL score>={SO_HIGH_SCORE}, sample score>={SO_MED_SCORE} to {SO_TARGET_TOTAL:,} total")

    # Pass 1: Scan for high-score chunks, count medium-score chunks
    t0 = time.time()
    high_score_lines = []
    med_score_count = 0
    total_scanned = 0

    print("  Pass 1: Scanning for score distribution...")
    with open(so_path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            total_scanned += 1
            if total_scanned % (PROGRESS_EVERY * 5) == 0:
                elapsed = time.time() - t0
                print(f"    Scanned {total_scanned:>12,} lines ({elapsed:.0f}s)...")

            line = line.strip()
            if not line:
                continue

            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue

            score = rec.get("score", 0)
            if score >= SO_HIGH_SCORE:
                high_score_lines.append(line)
            elif score >= SO_MED_SCORE:
                med_score_count += 1

    elapsed1 = time.time() - t0
    print(f"  Pass 1 complete in {elapsed1:.0f}s:")
    print(f"    Total scanned:     {total_scanned:>12,}")
    print(f"    High score (>={SO_HIGH_SCORE}):  {len(high_score_lines):>12,}")
    print(f"    Medium score (>={SO_MED_SCORE}): {med_score_count:>12,}")

    # Write all high-score chunks immediately
    for line in high_score_lines:
        out_fh.write(line + "\n")
    high_count = len(high_score_lines)
    del high_score_lines  # Free memory

    # Calculate how many medium-score to sample
    remaining = max(0, SO_TARGET_TOTAL - high_count)
    if remaining == 0 or med_score_count == 0:
        print(f"  No medium-score sampling needed (high alone = {high_count:,})")
        return high_count

    sample_rate = min(1.0, remaining / med_score_count)
    print(f"\n  Pass 2: Sampling {remaining:,} from {med_score_count:,} medium-score (rate={sample_rate:.4f})...")

    # Pass 2: Reservoir sample medium-score chunks
    t1 = time.time()
    med_written = 0
    scanned2 = 0

    with open(so_path, "r", encoding="utf-8", errors="ignore") as f:
        for line in f:
            scanned2 += 1
            if scanned2 % (PROGRESS_EVERY * 5) == 0:
                elapsed = time.time() - t1
                print(f"    Scanned {scanned2:>12,}, sampled {med_written:>10,} ({elapsed:.0f}s)...")

            if med_written >= remaining:
                break

            line = line.strip()
            if not line:
                continue

            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue

            score = rec.get("score", 0)
            if SO_MED_SCORE <= score < SO_HIGH_SCORE:
                if random.random() < sample_rate:
                    out_fh.write(line + "\n")
                    med_written += 1

    elapsed2 = time.time() - t1
    print(f"  Pass 2 complete in {elapsed2:.0f}s: sampled {med_written:,} medium-score chunks")

    total_so = high_count + med_written
    return total_so


def main():
    print("=" * 70)
    print("ZHEN CORPUS REBUILD v2")
    print("=" * 70)
    t_start = time.time()

    # Check inputs exist
    sources = {
        "Ring 1": RING1,
        "Ring 2-4": RING234,
        "Skills": SKILLS,
        "Stack Overflow": SO,
        "ServerFault": SF,
        "Unix.SE": UNIX_SE,
    }

    for name, path in sources.items():
        if path.exists():
            size_mb = path.stat().st_size / (1024 * 1024)
            print(f"  {name:20s}: {path.name} ({size_mb:,.1f} MB)")
        else:
            print(f"  {name:20s}: MISSING — {path}")
            if name in ("Ring 1", "Ring 2-4"):
                print(f"  ERROR: {name} is required. Aborting.")
                sys.exit(1)

    print()
    stats = {}

    with open(OUTPUT, "w", encoding="utf-8") as out_fh:

        # 1. Ring 1 — ALL
        if RING1.exists():
            print("=== Ring 1 (ALL) ===")
            n = copy_all(RING1, out_fh, "Ring 1")
            stats["ring1"] = n
            print(f"  Ring 1: {n:,} chunks written\n")

        # 2. Ring 2-4 — ALL
        if RING234.exists():
            print("=== Ring 2-4 (ALL) ===")
            n = copy_all(RING234, out_fh, "Ring 2-4")
            stats["ring234"] = n
            print(f"  Ring 2-4: {n:,} chunks written\n")

        # 3. Skills — ALL
        if SKILLS.exists():
            print("=== Skills (ALL) ===")
            n = copy_all(SKILLS, out_fh, "Skills")
            stats["skills"] = n
            print(f"  Skills: {n:,} chunks written\n")

        # 4. Stack Overflow — SAMPLED
        if SO.exists():
            n = sample_stackoverflow(SO, out_fh)
            stats["stackoverflow"] = n
            print(f"  Stack Overflow total: {n:,} chunks written\n")
        else:
            print("  Stack Overflow: SKIPPED (file missing)\n")

        # 5. ServerFault — ALL
        if SF.exists():
            print("=== ServerFault (ALL) ===")
            n = copy_all(SF, out_fh, "ServerFault")
            stats["serverfault"] = n
            print(f"  ServerFault: {n:,} chunks written\n")

        # 6. Unix.SE — ALL
        if UNIX_SE.exists():
            print("=== Unix.SE (ALL) ===")
            n = copy_all(UNIX_SE, out_fh, "Unix.SE")
            stats["unix_se"] = n
            print(f"  Unix.SE: {n:,} chunks written\n")

    elapsed = time.time() - t_start

    # Final stats
    total = sum(stats.values())
    output_size = OUTPUT.stat().st_size / (1024 * 1024)

    print("=" * 70)
    print("CORPUS REBUILD COMPLETE")
    print("=" * 70)
    print(f"\n  Output: {OUTPUT}")
    print(f"  Size:   {output_size:,.1f} MB")
    print(f"  Time:   {elapsed:.0f}s ({elapsed/60:.1f} min)\n")
    print(f"  {'Source':<20s} {'Chunks':>12s} {'%':>7s}")
    print(f"  {'-'*20} {'-'*12} {'-'*7}")
    for source, count in stats.items():
        pct = count / total * 100 if total > 0 else 0
        print(f"  {source:<20s} {count:>12,} {pct:>6.1f}%")
    print(f"  {'-'*20} {'-'*12} {'-'*7}")
    print(f"  {'TOTAL':<20s} {total:>12,} {'100.0':>6s}%")
    print()


if __name__ == "__main__":
    main()
