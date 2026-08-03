#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""
Wikipedia extraction — bypasses wikiextractor's Python 3.13 regex bug.
Streams the bz2 XML dump, extracts article text, writes JSONL chunks.

Usage:
    python3 14_extract_wikipedia.py

Input:  /mnt/hdd/zhen/wikipedia/enwiki-latest-pages-articles.xml.bz2
Output: ~/tmp/unheaded/raft/corpus/wikipedia.jsonl
"""

import bz2
import json
import re
import sys
import time
from pathlib import Path
from xml.etree import ElementTree as ET

INPUT = Path("/mnt/hdd/zhen/wikipedia/enwiki-latest-pages-articles.xml.bz2")
OUTPUT = Path.home() / "tmp" / "unheaded" / "raft" / "corpus" / "wikipedia.jsonl"
CHUNK_SIZE = 2000  # chars per chunk
MIN_ARTICLE_LEN = 200  # skip stubs
PROGRESS_EVERY = 50000  # print every N articles

# Simple markup removal (no wikiextractor dependency)
RE_MARKUP = re.compile(r"\{\{[^}]*\}\}|\[\[Category:[^\]]*\]\]|<[^>]+>|'{2,3}")
RE_LINKS = re.compile(r"\[\[(?:[^|\]]*\|)?([^\]]*)\]\]")
RE_REFS = re.compile(r"<ref[^>]*>.*?</ref>", re.DOTALL)
RE_WHITESPACE = re.compile(r"\n{3,}")


def clean_wikitext(text):
    """Strip wiki markup to plain text (fast, not perfect)."""
    text = RE_REFS.sub("", text)
    text = RE_MARKUP.sub("", text)
    text = RE_LINKS.sub(r"\1", text)
    text = RE_WHITESPACE.sub("\n\n", text)
    return text.strip()


def chunk_text(text, title, chunk_size=CHUNK_SIZE):
    """Split article into chunks."""
    chunks = []
    paragraphs = text.split("\n\n")
    current = ""

    for para in paragraphs:
        if len(current) + len(para) + 2 <= chunk_size:
            current = current + "\n\n" + para if current else para
        else:
            if current and len(current) >= 100:
                chunks.append(current)
            current = para

    if current and len(current) >= 100:
        chunks.append(current)

    return chunks


def main():
    if not INPUT.exists():
        print(f"ERROR: {INPUT} not found")
        sys.exit(1)

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)

    print(f"Extracting Wikipedia from {INPUT}")
    print(f"Output: {OUTPUT}")
    print(f"Chunk size: {CHUNK_SIZE} chars")
    print()

    articles = 0
    chunks_written = 0
    skipped = 0
    start = time.time()

    # Namespace for MediaWiki XML
    ns = "{http://www.mediawiki.org/xml/export-0.11/}"

    with bz2.open(INPUT, "rt", encoding="utf-8", errors="ignore") as bz_in, \
         open(OUTPUT, "w", encoding="utf-8") as out:

        # Stream parse — never loads full XML into memory
        context = ET.iterparse(bz_in, events=("end",))

        for event, elem in context:
            if elem.tag == f"{ns}page":
                # Extract title and text
                title_elem = elem.find(f"{ns}title")
                text_elem = elem.find(f".//{ns}text")
                ns_elem = elem.find(f"{ns}ns")

                # Skip non-article pages (ns != 0)
                if ns_elem is not None and ns_elem.text != "0":
                    elem.clear()
                    continue

                if title_elem is not None and text_elem is not None and text_elem.text:
                    title = title_elem.text
                    raw_text = text_elem.text

                    # Skip redirects
                    if raw_text.strip().lower().startswith("#redirect"):
                        elem.clear()
                        skipped += 1
                        continue

                    # Clean markup
                    clean = clean_wikitext(raw_text)

                    if len(clean) < MIN_ARTICLE_LEN:
                        elem.clear()
                        skipped += 1
                        continue

                    # Chunk and write
                    article_chunks = chunk_text(clean, title)
                    for i, chunk_text_content in enumerate(article_chunks):
                        record = {
                            "id": f"wiki_{articles}_{i}",
                            "source": f"wikipedia/{title}",
                            "type": "doc",
                            "ring": 4,
                            "content": f"{title}\n\n{chunk_text_content}",
                            "tokens": len(chunk_text_content.split()),
                        }
                        out.write(json.dumps(record, ensure_ascii=False) + "\n")
                        chunks_written += 1

                    articles += 1

                    if articles % PROGRESS_EVERY == 0:
                        elapsed = time.time() - start
                        rate = articles / elapsed
                        print(
                            f"  {articles:,} articles | {chunks_written:,} chunks | "
                            f"{skipped:,} skipped | {rate:.0f} articles/sec | "
                            f"{elapsed:.0f}s elapsed"
                        )

                # Free memory
                elem.clear()

    elapsed = time.time() - start
    print(f"\n{'='*60}")
    print(f"DONE in {elapsed:.0f}s ({elapsed/3600:.1f}h)")
    print(f"  Articles processed: {articles:,}")
    print(f"  Articles skipped:   {skipped:,}")
    print(f"  Chunks written:     {chunks_written:,}")
    print(f"  Output: {OUTPUT}")
    print(f"  Size: {OUTPUT.stat().st_size / (1024**3):.1f} GB")


if __name__ == "__main__":
    main()
