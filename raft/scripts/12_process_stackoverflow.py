#!/usr/bin/env python3
"""Process Stack Overflow / Stack Exchange Posts.xml into JSONL corpus chunks.

Streaming XML parser (iterparse) — never loads the full file into memory.
Handles the 60-80GB Posts.xml from SO as well as smaller SE site dumps.

Usage:
    python 12_process_stackoverflow.py                          # SO default
    python 12_process_stackoverflow.py /path/to/Posts.xml out.jsonl sitename
"""

import html
import json
import os
import re
import sys
import time
from xml.etree.ElementTree import iterparse

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

DEFAULT_XML = "/mnt/hdd/zhen/stackoverflow/extracted/Posts.xml"
DEFAULT_OUT = "/mnt/hdd/zhen/corpus/stackoverflow.jsonl"
DEFAULT_SOURCE = "stackoverflow"

MIN_SCORE = 3
CHUNK_SIZE = 2000  # chars
RING = 4
PROGRESS_EVERY = 100_000

# ---------------------------------------------------------------------------
# HTML stripping
# ---------------------------------------------------------------------------

_TAG_RE = re.compile(r"<[^>]+>")
_WS_RE = re.compile(r"\n{3,}")


def strip_html(text: str) -> str:
    """Remove HTML tags, decode entities, collapse whitespace."""
    text = _TAG_RE.sub("", text)
    text = html.unescape(text)
    text = _WS_RE.sub("\n\n", text)
    return text.strip()


# ---------------------------------------------------------------------------
# Chunking
# ---------------------------------------------------------------------------


def chunk_text(text: str, max_chars: int = CHUNK_SIZE):
    """Split long text into chunks at paragraph or sentence boundaries."""
    if len(text) <= max_chars:
        yield text
        return

    paragraphs = text.split("\n\n")
    current = ""
    for para in paragraphs:
        if current and len(current) + len(para) + 2 > max_chars:
            yield current.strip()
            current = ""
        if len(para) > max_chars:
            # Hard-split very long paragraphs
            while para:
                space = max_chars - len(current) - 2 if current else max_chars
                current += ("\n\n" if current else "") + para[:space]
                para = para[space:]
                if len(current) >= max_chars:
                    yield current.strip()
                    current = ""
        else:
            current += ("\n\n" if current else "") + para

    if current.strip():
        yield current.strip()


# ---------------------------------------------------------------------------
# Main processing
# ---------------------------------------------------------------------------


def process_posts(xml_path: str, output_path: str, source: str):
    """Stream-parse Posts.xml and write JSONL."""
    os.makedirs(os.path.dirname(output_path), exist_ok=True)

    t0 = time.time()
    total_read = 0
    total_kept = 0
    total_chunks = 0

    print(f"Processing: {xml_path}")
    print(f"Output:     {output_path}")
    print(f"Source:     {source}")
    print(f"Min score:  {MIN_SCORE}")
    print()

    with open(output_path, "w", encoding="utf-8") as out:
        context = iterparse(xml_path, events=("end",))
        for event, elem in context:
            if elem.tag != "row":
                continue

            total_read += 1

            # Filter: only questions (1) and answers (2)
            post_type = elem.get("PostTypeId", "")
            if post_type not in ("1", "2"):
                elem.clear()
                continue

            # Filter: minimum score
            try:
                score = int(elem.get("Score", "0"))
            except ValueError:
                score = 0
            if score < MIN_SCORE:
                elem.clear()
                continue

            total_kept += 1

            # Extract fields
            post_id = elem.get("Id", "")
            body = elem.get("Body", "")
            tags = elem.get("Tags", "")
            title = elem.get("Title", "")

            # Clean
            content = strip_html(body)
            if title:
                content = f"{title}\n\n{content}"

            # Parse tags: "<python><regex>" -> "python,regex"
            tag_list = re.findall(r"<([^>]+)>", tags) if tags else []
            tags_str = ",".join(tag_list)

            ptype = "question" if post_type == "1" else "answer"

            # Chunk and write
            for i, chunk in enumerate(chunk_text(content)):
                if not chunk:
                    continue
                record = {
                    "id": f"{source}-{post_id}" + (f"-{i}" if i > 0 else ""),
                    "source": source,
                    "type": "qa",
                    "ring": RING,
                    "content": chunk,
                    "score": score,
                    "tags": tags_str,
                    "post_type": ptype,
                }
                out.write(json.dumps(record, ensure_ascii=False) + "\n")
                total_chunks += 1

            elem.clear()

            # Progress
            if total_read % PROGRESS_EVERY == 0:
                elapsed = time.time() - t0
                rate = total_read / elapsed if elapsed > 0 else 0
                print(
                    f"  [{elapsed:7.1f}s] Read {total_read:>12,} | "
                    f"Kept {total_kept:>10,} | "
                    f"Chunks {total_chunks:>10,} | "
                    f"{rate:,.0f} posts/s"
                )

    elapsed = time.time() - t0
    print()
    print(f"Done in {elapsed:.1f}s")
    print(f"  Total read:   {total_read:>12,}")
    print(f"  Kept (score≥{MIN_SCORE}): {total_kept:>10,}")
    print(f"  Chunks written: {total_chunks:>10,}")
    print(f"  Output: {output_path}")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    xml_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_XML
    output_path = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_OUT
    source = sys.argv[3] if len(sys.argv) > 3 else DEFAULT_SOURCE

    if not os.path.isfile(xml_path):
        print(f"ERROR: {xml_path} not found.")
        print("The 7z extraction may still be running. Check:")
        print("  tail -f /mnt/hdd/zhen/stackoverflow/extraction.log")
        sys.exit(1)

    process_posts(xml_path, output_path, source)
