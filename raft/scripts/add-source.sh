#!/bin/bash
#
# add-source.sh — Add text files to Zhen AI corpus
#
# Usage:
#   ./scripts/add-source.sh file.txt [file2.txt ...] [OPTIONS]
#   ./scripts/add-source.sh /path/to/directory/ [OPTIONS]
#
# Options:
#   --ring N          Knowledge ring (1-4, default: 3)
#   --type TYPE       Content type: doc|code|paper|rfc|skill|other (default: doc)
#   --source LABEL    Source label for retrieval results (default: filename)
#   --chunk-size N    Characters per chunk (default: 2000)
#   --chunk-overlap N Overlap between chunks (default: 200)
#   --dry-run         Show what would be added without writing
#
# Examples:
#   ./scripts/add-source.sh rfc9999.txt --ring 4 --type rfc
#   ./scripts/add-source.sh /tmp/papers/ --ring 3 --type paper --source "ML Papers"
#   ./scripts/add-source.sh book.txt --chunk-size 3000 --source "DDIA Book"
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
RAFT_DIR="$(dirname "$SCRIPT_DIR")"
CORPUS_DIR="$RAFT_DIR/corpus"
OUTPUT="$CORPUS_DIR/ring_all.jsonl"

# Defaults
RING=3
TYPE="doc"
SOURCE=""
CHUNK_SIZE=2000
CHUNK_OVERLAP=200
DRY_RUN=false

# Parse args — separate files from flags
FILES=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --ring)        RING="$2"; shift 2 ;;
        --type)        TYPE="$2"; shift 2 ;;
        --source)      SOURCE="$2"; shift 2 ;;
        --chunk-size)  CHUNK_SIZE="$2"; shift 2 ;;
        --chunk-overlap) CHUNK_OVERLAP="$2"; shift 2 ;;
        --dry-run)     DRY_RUN=true; shift ;;
        --help|-h)
            head -22 "$0" | tail -20
            exit 0
            ;;
        *)
            FILES+=("$1"); shift ;;
    esac
done

if [[ ${#FILES[@]} -eq 0 ]]; then
    echo "Error: No input files specified."
    echo "Usage: $0 file.txt [file2.txt ...] [OPTIONS]"
    echo "Run with --help for details."
    exit 1
fi

# Expand directories into file lists
EXPANDED_FILES=()
for f in "${FILES[@]}"; do
    if [[ -d "$f" ]]; then
        while IFS= read -r -d '' file; do
            EXPANDED_FILES+=("$file")
        done < <(find "$f" -type f \( -name "*.txt" -o -name "*.md" -o -name "*.rst" \
            -o -name "*.go" -o -name "*.py" -o -name "*.rs" -o -name "*.c" -o -name "*.h" \
            -o -name "*.cpp" -o -name "*.java" -o -name "*.js" -o -name "*.ts" \
            -o -name "*.toml" -o -name "*.yaml" -o -name "*.yml" -o -name "*.json" \
            -o -name "*.sh" -o -name "*.nix" -o -name "*.html" -o -name "*.css" \
            -o -name "*.sql" -o -name "*.csv" -o -name "*.log" -o -name "*.conf" \
            -o -name "*.cfg" -o -name "*.ini" -o -name "*.xml" -o -name "*.proto" \
            -o -name "*.rb" -o -name "*.php" -o -name "*.lua" \) -print0 | sort -z)
    elif [[ -f "$f" ]]; then
        EXPANDED_FILES+=("$f")
    else
        echo "Warning: '$f' not found, skipping."
    fi
done

if [[ ${#EXPANDED_FILES[@]} -eq 0 ]]; then
    echo "Error: No valid files found."
    exit 1
fi

echo "========================================"
echo "Zhen Corpus — Add Source"
echo "========================================"
echo "  Files:         ${#EXPANDED_FILES[@]}"
echo "  Ring:           $RING"
echo "  Type:           $TYPE"
echo "  Source:          ${SOURCE:-<auto from filename>}"
echo "  Chunk size:      $CHUNK_SIZE chars"
echo "  Chunk overlap:   $CHUNK_OVERLAP chars"
echo "  Output:          $OUTPUT"
echo "  Dry run:         $DRY_RUN"
echo "========================================"
echo ""

# Export for Python subprocess
export RING TYPE SOURCE CHUNK_SIZE CHUNK_OVERLAP DRY_RUN OUTPUT

# Use Python for chunking — matches the same logic as the pipeline scripts
python3 - "${EXPANDED_FILES[@]}" <<'PYEOF'
import json
import os
import sys
import time
import uuid

# Config from environment (set by shell above)
RING = int(os.environ.get("RING", "3"))
TYPE = os.environ.get("TYPE", "doc")
SOURCE = os.environ.get("SOURCE", "")
CHUNK_SIZE = int(os.environ.get("CHUNK_SIZE", "2000"))
CHUNK_OVERLAP = int(os.environ.get("CHUNK_OVERLAP", "200"))
DRY_RUN = os.environ.get("DRY_RUN", "false") == "true"
OUTPUT = os.environ.get("OUTPUT", "")

files = sys.argv[1:]
total_chunks = 0
total_files = 0
total_bytes = 0

# Chunk text using the same approach as RecursiveCharacterTextSplitter
# (paragraph -> sentence -> word -> char boundaries)
SEPARATORS = ["\n\n", "\n", ". ", " ", ""]

def split_text(text, chunk_size, chunk_overlap):
    """Split text into chunks, respecting natural boundaries."""
    chunks = []

    def _split(text, seps):
        if len(text) <= chunk_size:
            return [text] if text.strip() else []

        sep = seps[0] if seps else ""
        remaining_seps = seps[1:] if len(seps) > 1 else []

        if sep:
            parts = text.split(sep)
        else:
            # Character-level split as last resort
            result = []
            for i in range(0, len(text), chunk_size - chunk_overlap):
                result.append(text[i:i + chunk_size])
            return result

        current = []
        current_len = 0
        result = []

        for part in parts:
            part_len = len(part) + len(sep)

            if current_len + part_len > chunk_size and current:
                merged = sep.join(current)
                if len(merged) <= chunk_size:
                    result.append(merged)
                else:
                    result.extend(_split(merged, remaining_seps))

                # Keep overlap
                overlap_parts = []
                overlap_len = 0
                for p in reversed(current):
                    if overlap_len + len(p) + len(sep) > chunk_overlap:
                        break
                    overlap_parts.insert(0, p)
                    overlap_len += len(p) + len(sep)
                current = overlap_parts
                current_len = overlap_len

            current.append(part)
            current_len += part_len

        if current:
            merged = sep.join(current)
            if len(merged) <= chunk_size:
                result.append(merged)
            else:
                result.extend(_split(merged, remaining_seps))

        return result

    raw = _split(text, SEPARATORS)
    return [c for c in raw if c.strip()]

# Process files
records = []
for filepath in files:
    try:
        with open(filepath, "r", encoding="utf-8", errors="ignore") as f:
            content = f.read()
    except Exception as e:
        print(f"  WARN: Cannot read {filepath}: {e}")
        continue

    if not content.strip():
        print(f"  SKIP: {filepath} (empty)")
        continue

    # Skip files > 10 MB
    if len(content) > 10 * 1024 * 1024:
        print(f"  SKIP: {filepath} (>10 MB)")
        continue

    basename = os.path.basename(filepath)
    source_label = SOURCE if SOURCE else basename

    chunks = split_text(content, CHUNK_SIZE, CHUNK_OVERLAP)

    for i, chunk_text in enumerate(chunks):
        record = {
            "id": f"custom_{basename.replace('.', '_')}_{int(time.time())}_{i}",
            "source": source_label,
            "ring": RING,
            "type": TYPE,
            "content": chunk_text,
            "tokens": len(chunk_text) // 4,
        }
        records.append(record)

    total_files += 1
    total_bytes += len(content)
    total_chunks += len(chunks)
    print(f"  {basename}: {len(content):,} bytes -> {len(chunks)} chunks")

print(f"\n  Total: {total_files} files, {total_bytes:,} bytes -> {total_chunks} chunks")

if DRY_RUN:
    print("\n  [DRY RUN] No files written.")
    print("\n  Sample record:")
    if records:
        print(f"  {json.dumps(records[0], indent=2)[:500]}")
    sys.exit(0)

if not records:
    print("\n  Nothing to add.")
    sys.exit(0)

# Append to ring_all.jsonl
with open(OUTPUT, "a", encoding="utf-8") as f:
    for rec in records:
        f.write(json.dumps(rec, ensure_ascii=False) + "\n")

print(f"\n  Appended {total_chunks} chunks to {OUTPUT}")
print(f"  New corpus size: {os.path.getsize(OUTPUT) / (1024*1024):,.1f} MB")
print(f"\n  Next steps:")
print(f"    1. python3 scripts/16_embed_v2.py    # Rebuild FAISS index (2-3 hours)")
print(f"    2. ln -sf v2.index index/active.index")
print(f"    3. ln -sf v2_ids.json index/active_ids.json")
print(f"    4. Restart zhen_app.py")
PYEOF
