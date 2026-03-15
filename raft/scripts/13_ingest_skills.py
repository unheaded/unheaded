#!/usr/bin/env python3
"""
13_ingest_skills.py — Ingest Kingdom skill files into Zhen's corpus

Walks ~/tmp/unheaded/skills/ and extracts SKILL.md from:
  - .skill files (zip archives with <name>/SKILL.md inside)
  - Directories with SKILL.md directly
  - Directories with files.zip containing SKILL.md
  - Standalone .md files (skill updates, additions)

Also ingests CLAUDE.md as a high-priority chunk.

Output: ~/tmp/unheaded/raft/corpus/ring1_skills.jsonl
"""
import json
import os
import subprocess
import zipfile
from pathlib import Path


SKILLS_DIR = Path.home() / 'tmp' / 'unheaded' / 'skills'
CORPUS_OUT = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1_skills.jsonl'
CLAUDE_MD = Path.home() / 'tmp' / 'unheaded' / 'CLAUDE.md'
CHUNK_SIZE = 2000  # characters per chunk


def chunk_text(text, chunk_size=CHUNK_SIZE):
    """Split text into chunks of ~chunk_size chars, breaking at paragraph boundaries."""
    paragraphs = text.split('\n\n')
    chunks = []
    current = ""

    for para in paragraphs:
        if len(current) + len(para) + 2 > chunk_size and current:
            chunks.append(current.strip())
            current = para
        else:
            current = current + "\n\n" + para if current else para

    if current.strip():
        chunks.append(current.strip())

    return chunks


def count_tokens_approx(text):
    """Rough token count: ~4 chars per token for English."""
    return len(text) // 4


def extract_skill_metadata(text):
    """Extract name and description from YAML front matter if present."""
    description = ""
    if text.startswith('---'):
        end = text.find('---', 3)
        if end > 0:
            front = text[3:end]
            for line in front.split('\n'):
                if line.strip().startswith('description:'):
                    description = line.split(':', 1)[1].strip().strip('"').strip("'")
                    # Collect multiline description
                    break
    # Also grab first heading as fallback description
    for line in text.split('\n'):
        if line.startswith('# '):
            if not description:
                description = line[2:].strip()
            break
    return description


def extract_from_zip_file(zip_path, inner_pattern="SKILL.md"):
    """Extract SKILL.md content from a zip archive."""
    try:
        with zipfile.ZipFile(zip_path, 'r') as zf:
            for name in zf.namelist():
                if name.endswith(inner_pattern):
                    return zf.read(name).decode('utf-8', errors='ignore')
    except (zipfile.BadZipFile, Exception) as e:
        print(f"  Warning: Could not read {zip_path}: {e}")
    return None


def make_chunk_record(skill_name, source, chunk_index, content, chunk_type="skill"):
    """Create a corpus record in the expected JSONL format."""
    return {
        "id": f"skill-{skill_name}-{chunk_index}",
        "source": source,
        "type": chunk_type,
        "content": content,
        "tokens": count_tokens_approx(content),
    }


def ingest_skills():
    records = []
    skills_found = set()

    print(f"Scanning {SKILLS_DIR} for skill files...\n")

    # --- 1. .skill files (zip archives) ---
    for skill_file in sorted(SKILLS_DIR.glob('*.skill')):
        name = skill_file.stem  # e.g. "unheaded-kingdom"
        text = extract_from_zip_file(skill_file)
        if text:
            skills_found.add(name)
            chunks = chunk_text(text)
            source = f"skills/{name}/SKILL.md"
            for i, chunk in enumerate(chunks):
                records.append(make_chunk_record(name, source, i, chunk))
            print(f"  [zip]  {name}: {len(chunks)} chunks ({len(text):,} chars)")
        else:
            print(f"  [zip]  {name}: SKIP (no SKILL.md found)")

    # --- 2. Directories with SKILL.md ---
    for subdir in sorted(SKILLS_DIR.iterdir()):
        if not subdir.is_dir():
            continue
        name = subdir.name

        # Direct SKILL.md
        skill_md = subdir / 'SKILL.md'
        if skill_md.exists() and name not in skills_found:
            text = skill_md.read_text(encoding='utf-8', errors='ignore')
            skills_found.add(name)
            chunks = chunk_text(text)
            source = f"skills/{name}/SKILL.md"
            for i, chunk in enumerate(chunks):
                records.append(make_chunk_record(name, source, i, chunk))
            print(f"  [dir]  {name}: {len(chunks)} chunks ({len(text):,} chars)")
            continue

        # files.zip containing SKILL.md
        files_zip = subdir / 'files.zip'
        if files_zip.exists() and name not in skills_found:
            text = extract_from_zip_file(files_zip, "SKILL.md")
            if text:
                skills_found.add(name)
                chunks = chunk_text(text)
                source = f"skills/{name}/SKILL.md"
                for i, chunk in enumerate(chunks):
                    records.append(make_chunk_record(name, source, i, chunk))
                print(f"  [fzip] {name}: {len(chunks)} chunks ({len(text):,} chars)")

    # --- 3. Standalone .md files (skill updates, additions) ---
    for md_file in sorted(SKILLS_DIR.glob('*.md')):
        name = md_file.stem
        text = md_file.read_text(encoding='utf-8', errors='ignore')
        if len(text.strip()) < 50:
            continue
        chunks = chunk_text(text)
        source = f"skills/{md_file.name}"
        for i, chunk in enumerate(chunks):
            records.append(make_chunk_record(name, source, i, chunk, chunk_type="skill-update"))
        print(f"  [md]   {md_file.name}: {len(chunks)} chunks ({len(text):,} chars)")

    # --- 4. CLAUDE.md as high-priority context ---
    if CLAUDE_MD.exists():
        text = CLAUDE_MD.read_text(encoding='utf-8', errors='ignore')
        chunks = chunk_text(text)
        for i, chunk in enumerate(chunks):
            records.append({
                "id": f"skill-claude-md-{i}",
                "source": "CLAUDE.md",
                "type": "project-context",
                "content": chunk,
                "tokens": count_tokens_approx(chunk),
            })
        print(f"  [ctx]  CLAUDE.md: {len(chunks)} chunks ({len(text):,} chars)")

    # --- Write output ---
    CORPUS_OUT.parent.mkdir(parents=True, exist_ok=True)
    with open(CORPUS_OUT, 'w', encoding='utf-8') as f:
        for rec in records:
            f.write(json.dumps(rec, ensure_ascii=False) + '\n')

    total_tokens = sum(r['tokens'] for r in records)
    print(f"\n{'='*60}")
    print(f"Skills ingested: {len(skills_found)}")
    print(f"Standalone MDs:  {len(list(SKILLS_DIR.glob('*.md')))}")
    print(f"Total chunks:    {len(records)}")
    print(f"Total tokens:    {total_tokens:,} (~approx)")
    print(f"Output:          {CORPUS_OUT}")
    print(f"{'='*60}")

    return records


if __name__ == '__main__':
    ingest_skills()
