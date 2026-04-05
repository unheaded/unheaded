#!/usr/bin/env python3
"""
distill_qa.py — Claude-powered training data distillation for Zhenai.

Uses Claude Haiku/Sonnet to generate expert-quality QA pairs from Kingdom
documentation, source code, ADRs, and runbooks. The generated QA pairs are
formatted for zhenai-forge (Rust LoRA training pipeline).

This is distillation: a stronger model (Claude) teaches a weaker one (Mistral-7B).

Usage:
  # Dry run — 1 document, print to stdout
  python3 distill_qa.py --dry-run --limit 1

  # Full run — all documents
  python3 distill_qa.py --output /var/zhen/distilled_qa_haiku.jsonl

  # Specific category only
  python3 distill_qa.py --category adrs --output /var/zhen/distilled_qa_adrs.jsonl

  # Use Sonnet for higher quality
  python3 distill_qa.py --model claude-sonnet-4-6 --output /var/zhen/distilled_qa_sonnet.jsonl
"""

import argparse
import json
import os
import sys
import time
import glob
import hashlib
from pathlib import Path

try:
    import anthropic
except ImportError:
    print("ERROR: anthropic package not installed. Run: pip install anthropic")
    sys.exit(1)


# === Configuration ===

PROJECT_ROOT = os.path.expanduser("~/tmp/unheaded")

CATEGORIES = {
    "adrs": {
        "pattern": f"{PROJECT_ROOT}/docs/adr/ADR-*.md",
        "prompt_prefix": "architecture decision record (ADR)",
        "qa_count": 15,
    },
    "docs": {
        "pattern": f"{PROJECT_ROOT}/docs/*.md",
        "prompt_prefix": "project documentation",
        "qa_count": 10,
    },
    "doom_docs": {
        "pattern": f"{PROJECT_ROOT}/docs/doom/*.md",
        "prompt_prefix": "DOOM-over-IPv6 technical documentation",
        "qa_count": 10,
    },
    "runbooks": {
        "pattern": f"{PROJECT_ROOT}/runbooks/**/*.yaml",
        "prompt_prefix": "operational runbook",
        "qa_count": 8,
    },
    "skills": {
        "glob_dirs": [os.path.expanduser("~/.claude/skills/")],
        "pattern": "SKILL.md",
        "prompt_prefix": "Kingdom skill definition",
        "qa_count": 12,
    },
    "go_services": {
        "dirs": [
            f"{PROJECT_ROOT}/cmd/akira/",
            f"{PROJECT_ROOT}/pkg/health/",
            f"{PROJECT_ROOT}/pkg/transport/",
            f"{PROJECT_ROOT}/pkg/auth/",
            f"{PROJECT_ROOT}/pkg/wotan-client/",
            f"{PROJECT_ROOT}/services/wotan/cmd/wotan/",
            f"{PROJECT_ROOT}/services/wotan/internal/store/",
        ],
        "extensions": [".go"],
        "prompt_prefix": "Go source code from the Unheaded Kingdom",
        "qa_count": 8,
        "max_files": 80,
    },
    "rust_crates": {
        "dirs": [
            f"{PROJECT_ROOT}/crates/zhenai-forge/src/",
            f"{PROJECT_ROOT}/crates/zhend/src/",
        ],
        "extensions": [".rs"],
        "prompt_prefix": "Rust source code from the Unheaded Kingdom",
        "qa_count": 8,
        "max_files": 30,
    },
    "claude_md": {
        "files": [
            f"{PROJECT_ROOT}/CLAUDE.md",
            f"{PROJECT_ROOT}/raft/CLAUDE.md",
        ],
        "prompt_prefix": "project development guide (CLAUDE.md)",
        "qa_count": 25,
    },
    "protocols": {
        "pattern": f"{PROJECT_ROOT}/docs/protocol/*.md",
        "prompt_prefix": "network protocol specification",
        "qa_count": 10,
    },
}

SYSTEM_PROMPT = """You are an expert technical writer generating training data for a local AI assistant called Zhenai.
Zhenai is the operations champion of the Unheaded Kingdom — an infrastructure automation platform.

Your task: Read the provided document and generate question-answer pairs that would help Zhenai
answer questions about this specific content accurately.

Rules:
- Questions should be natural — how a developer or sysadmin would ask
- Answers must be grounded in the document — no hallucination
- Answers should be detailed but concise (2-5 sentences)
- Include a mix of: factual, conceptual, how-to, and troubleshooting questions
- Reference specific details: port numbers, file paths, function names, config values
- For code: ask about what functions do, how components interact, error handling
- For ADRs: ask about the decision, rationale, consequences, alternatives rejected
- For runbooks: ask about steps, verification, rollback procedures

Output ONLY valid JSON array of objects with "question" and "answer" fields.
No markdown, no commentary, no explanation. Just the JSON array."""


def collect_files(category_name: str, category: dict) -> list[tuple[str, str]]:
    """Collect files for a category. Returns list of (path, content)."""
    files = []

    if "files" in category:
        for f in category["files"]:
            if os.path.exists(f):
                try:
                    content = Path(f).read_text(errors="replace")
                    if len(content) > 100:
                        files.append((f, content))
                except Exception:
                    pass

    if "pattern" in category and "glob_dirs" not in category:
        for f in sorted(glob.glob(category["pattern"], recursive=True)):
            if os.path.isfile(f):
                try:
                    content = Path(f).read_text(errors="replace")
                    if len(content) > 100:
                        files.append((f, content))
                except Exception:
                    pass

    if "glob_dirs" in category:
        for d in category["glob_dirs"]:
            for root, dirs, fnames in os.walk(d):
                for fn in fnames:
                    if fn == category.get("pattern", ""):
                        fp = os.path.join(root, fn)
                        try:
                            content = Path(fp).read_text(errors="replace")
                            if len(content) > 100:
                                files.append((fp, content))
                        except Exception:
                            pass

    if "dirs" in category:
        exts = set(category.get("extensions", []))
        for d in category["dirs"]:
            if not os.path.isdir(d):
                continue
            for root, dirs, fnames in os.walk(d):
                for fn in fnames:
                    if any(fn.endswith(e) for e in exts):
                        fp = os.path.join(root, fn)
                        try:
                            content = Path(fp).read_text(errors="replace")
                            if len(content) > 100:
                                files.append((fp, content))
                        except Exception:
                            pass

    max_files = category.get("max_files", 500)
    if len(files) > max_files:
        # Prioritize larger files (more content = better QA)
        files.sort(key=lambda x: len(x[1]), reverse=True)
        files = files[:max_files]

    return files


def generate_qa_pairs(client: anthropic.Anthropic, model: str, filepath: str,
                      content: str, category: dict, qa_count: int) -> list[dict]:
    """Generate QA pairs for a single document using Claude."""
    # Truncate very long documents to fit context
    max_content = 12000  # ~3K tokens, leaves room for generation
    if len(content) > max_content:
        content = content[:max_content] + "\n\n[... truncated for length ...]"

    rel_path = filepath.replace(PROJECT_ROOT, "").lstrip("/")
    prompt_prefix = category.get("prompt_prefix", "document")

    user_prompt = f"""Here is a {prompt_prefix} from the Unheaded Kingdom project.

**File:** `{rel_path}`

```
{content}
```

Generate exactly {qa_count} question-answer pairs about this document.
Output as a JSON array: [{{"question": "...", "answer": "..."}}]"""

    try:
        response = client.messages.create(
            model=model,
            max_tokens=4096,
            system=SYSTEM_PROMPT,
            messages=[{"role": "user", "content": user_prompt}],
        )

        text = response.content[0].text.strip()

        # Parse JSON — handle common issues
        if text.startswith("```"):
            text = text.split("```")[1]
            if text.startswith("json"):
                text = text[4:]
        text = text.strip()

        pairs = json.loads(text)
        if not isinstance(pairs, list):
            return []

        # Add source metadata
        result = []
        for p in pairs:
            if "question" in p and "answer" in p:
                result.append({
                    "question": p["question"],
                    "answer": p["answer"],
                    "source": rel_path,
                })
        return result

    except json.JSONDecodeError as e:
        print(f"  JSON parse error for {rel_path}: {e}", file=sys.stderr)
        return []
    except anthropic.APIError as e:
        print(f"  API error for {rel_path}: {e}", file=sys.stderr)
        return []
    except Exception as e:
        print(f"  Error for {rel_path}: {e}", file=sys.stderr)
        return []


def file_hash(path: str) -> str:
    """Short hash of file path for checkpoint tracking."""
    return hashlib.md5(path.encode()).hexdigest()[:12]


def main():
    parser = argparse.ArgumentParser(description="Claude-powered QA distillation for Zhenai")
    parser.add_argument("--output", "-o", default="/var/zhen/distilled_qa_haiku.jsonl",
                        help="Output JSONL file path")
    parser.add_argument("--model", "-m", default="claude-haiku-4-5-20251001",
                        help="Claude model to use")
    parser.add_argument("--category", "-c", default=None,
                        help="Only process this category (adrs, docs, runbooks, etc)")
    parser.add_argument("--limit", type=int, default=0,
                        help="Max documents to process (0=all)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print QA pairs to stdout, don't write file")
    parser.add_argument("--qa-count", type=int, default=0,
                        help="Override QA pairs per document")
    args = parser.parse_args()

    # Check API key
    client = anthropic.Anthropic()  # Uses ANTHROPIC_API_KEY env var

    print(f"=== Zhenai Training Data Distillation ===")
    print(f"  Model: {args.model}")
    print(f"  Output: {args.output}")
    print()

    # Load checkpoint (for resumability)
    checkpoint_path = args.output + ".checkpoint"
    completed = set()
    if os.path.exists(checkpoint_path):
        with open(checkpoint_path) as f:
            completed = set(line.strip() for line in f)
        print(f"  Resuming: {len(completed)} documents already processed")

    # Collect all files
    categories = CATEGORIES
    if args.category:
        if args.category not in categories:
            print(f"ERROR: Unknown category '{args.category}'. Available: {', '.join(categories.keys())}")
            sys.exit(1)
        categories = {args.category: categories[args.category]}

    all_files = []
    for cat_name, cat_cfg in categories.items():
        files = collect_files(cat_name, cat_cfg)
        for fp, content in files:
            all_files.append((cat_name, cat_cfg, fp, content))

    if args.limit > 0:
        all_files = all_files[:args.limit]

    print(f"  Documents to process: {len(all_files)}")
    print(f"  Categories: {', '.join(categories.keys())}")
    print()

    # Process each document
    total_pairs = 0
    total_docs = 0
    start = time.time()

    output_file = None
    if not args.dry_run:
        output_file = open(args.output, "a")  # Append mode for resumability
        checkpoint_file = open(checkpoint_path, "a")

    for i, (cat_name, cat_cfg, filepath, content) in enumerate(all_files):
        fhash = file_hash(filepath)
        if fhash in completed:
            continue

        rel_path = filepath.replace(PROJECT_ROOT, "").lstrip("/")
        qa_count = args.qa_count if args.qa_count > 0 else cat_cfg.get("qa_count", 10)

        print(f"  [{i+1}/{len(all_files)}] {cat_name}: {rel_path} ({qa_count} QA)...", end=" ", flush=True)

        pairs = generate_qa_pairs(client, args.model, filepath, content, cat_cfg, qa_count)

        if pairs:
            total_pairs += len(pairs)
            total_docs += 1
            print(f"{len(pairs)} pairs")

            if args.dry_run:
                for p in pairs[:3]:  # Show first 3 in dry run
                    print(f"    Q: {p['question'][:100]}")
                    print(f"    A: {p['answer'][:100]}")
                    print()
            else:
                for p in pairs:
                    output_file.write(json.dumps(p, ensure_ascii=False) + "\n")
                output_file.flush()
                checkpoint_file.write(fhash + "\n")
                checkpoint_file.flush()
        else:
            print("0 pairs (skipped)")

        # Rate limiting — be kind to the API
        time.sleep(0.5)

    elapsed = time.time() - start

    print()
    print(f"=== Distillation Complete ===")
    print(f"  Documents: {total_docs}")
    print(f"  QA Pairs:  {total_pairs}")
    print(f"  Time:      {elapsed:.0f}s ({elapsed/60:.1f} min)")
    print(f"  Output:    {args.output}")

    if output_file:
        output_file.close()
        checkpoint_file.close()


if __name__ == "__main__":
    main()
