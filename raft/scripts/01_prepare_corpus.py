#!/usr/bin/env python3
"""Ring 1 Corpus Preparation for Zhen RAG system.

Walks the Unheaded source tree, splits files into ~512-token chunks,
and writes them to a JSONL corpus file.
"""

import json
import os
import sys
import time
import uuid
from pathlib import Path

from langchain_text_splitters import RecursiveCharacterTextSplitter

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

SOURCE_ROOT = os.path.expanduser("~/tmp/unheaded")
OUTPUT_FILE = os.path.join(SOURCE_ROOT, "raft", "corpus", "ring1.jsonl")

INCLUDE_EXTENSIONS = frozenset([
    ".go", ".rs", ".md", ".yaml", ".yml", ".nix", ".sh", ".py",
    ".json", ".toml", ".proto", ".c", ".h", ".txt", ".js", ".html", ".css",
])

SKIP_DIRS = frozenset([
    ".git", "node_modules", "vendor", "__pycache__", ".claude",
    "build", "dist", "llama.cpp", "models", ".venv",
])

MAX_FILE_SIZE = 1 * 1024 * 1024  # 1 MB

# Code extensions vs doc vs config
CODE_EXTENSIONS = frozenset([".go", ".rs", ".py", ".c", ".h", ".js", ".proto", ".html", ".css"])
DOC_EXTENSIONS = frozenset([".md", ".txt"])
CONFIG_EXTENSIONS = frozenset([".yaml", ".yml", ".nix", ".sh", ".toml", ".json"])


def classify_file(ext: str) -> str:
    if ext in CODE_EXTENSIONS:
        return "code"
    if ext in DOC_EXTENSIONS:
        return "doc"
    if ext in CONFIG_EXTENSIONS:
        return "config"
    return "other"


def estimate_tokens(text: str) -> int:
    """Rough token estimate: chars / 4."""
    return len(text) // 4


def collect_files(root: str):
    """Walk the source tree and yield (abs_path, rel_path, extension)."""
    for dirpath, dirnames, filenames in os.walk(root):
        # Prune skipped directories in-place
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]

        for fname in filenames:
            ext = os.path.splitext(fname)[1].lower()
            if ext not in INCLUDE_EXTENSIONS:
                continue

            abs_path = os.path.join(dirpath, fname)

            # Skip files larger than 1 MB
            try:
                if os.path.getsize(abs_path) > MAX_FILE_SIZE:
                    continue
            except OSError:
                continue

            rel_path = os.path.relpath(abs_path, root)
            yield abs_path, rel_path, ext


def main():
    t0 = time.time()

    splitter = RecursiveCharacterTextSplitter(
        chunk_size=2000,
        chunk_overlap=200,
        length_function=len,
    )

    os.makedirs(os.path.dirname(OUTPUT_FILE), exist_ok=True)

    stats = {"code": 0, "doc": 0, "config": 0, "other": 0}
    token_stats = {"code": 0, "doc": 0, "config": 0, "other": 0}
    total_chunks = 0
    total_tokens = 0
    files_processed = 0
    files_skipped = 0

    with open(OUTPUT_FILE, "w", encoding="utf-8") as out:
        for abs_path, rel_path, ext in collect_files(SOURCE_ROOT):
            # Read file, handling encoding issues
            try:
                with open(abs_path, "r", encoding="utf-8", errors="replace") as f:
                    content = f.read()
            except (OSError, UnicodeDecodeError):
                files_skipped += 1
                continue

            # Skip files that look binary (high ratio of null/control chars)
            if "\x00" in content:
                files_skipped += 1
                continue

            content = content.strip()
            if not content:
                files_skipped += 1
                continue

            files_processed += 1
            file_type = classify_file(ext)

            chunks = splitter.split_text(content)

            for i, chunk_text in enumerate(chunks):
                tok_count = estimate_tokens(chunk_text)
                chunk_id = str(uuid.uuid4())

                record = {
                    "id": chunk_id,
                    "source": rel_path,
                    "type": file_type,
                    "chunk_index": i,
                    "content": chunk_text,
                    "tokens": tok_count,
                }

                out.write(json.dumps(record, ensure_ascii=False) + "\n")

                stats[file_type] += 1
                token_stats[file_type] += tok_count
                total_chunks += 1
                total_tokens += tok_count

    elapsed = time.time() - t0

    print(f"=== Ring 1 Corpus Preparation Complete ===")
    print(f"Time elapsed:     {elapsed:.1f}s")
    print(f"Files processed:  {files_processed}")
    print(f"Files skipped:    {files_skipped}")
    print(f"Total chunks:     {total_chunks}")
    print(f"Total tokens:     ~{total_tokens:,}")
    print()
    print("Breakdown by type:")
    for t in ("code", "doc", "config", "other"):
        print(f"  {t:10s}  chunks: {stats[t]:>7,}   tokens: ~{token_stats[t]:>10,}")
    print()
    print(f"Output: {OUTPUT_FILE}")


if __name__ == "__main__":
    main()
