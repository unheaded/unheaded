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
import fnmatch
import glob
import hashlib
import json
import os
import sys
import time
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
    "lore": {
        "pattern": f"{PROJECT_ROOT}/docs/lore/*.md",
        "prompt_prefix": "Kingdom lore, mythology, and naming conventions",
        "qa_count": 12,
    },
    "architecture": {
        "pattern": f"{PROJECT_ROOT}/docs/architecture/*.md",
        "prompt_prefix": "system architecture documentation",
        "qa_count": 10,
    },
    "history": {
        "pattern": f"{PROJECT_ROOT}/docs/history/*.md",
        "prompt_prefix": "project history and session records",
        "qa_count": 6,
    },
    "binder_book": {
        "pattern": f"{PROJECT_ROOT}/docs/binder-book/**/*.md",
        "prompt_prefix": "educational material from the Unheaded binder book",
        "qa_count": 8,
    },
    "research": {
        "pattern": f"{PROJECT_ROOT}/docs/research/*.md",
        "prompt_prefix": "research notes and technical analysis",
        "qa_count": 8,
    },
    "zhen_docs": {
        "pattern": f"{PROJECT_ROOT}/docs/zhen/*.md",
        "prompt_prefix": "Zhenai AI champion documentation",
        "qa_count": 10,
    },
    "talks": {
        "pattern": f"{PROJECT_ROOT}/docs/talks/*.md",
        "prompt_prefix": "conference talk and presentation material",
        "qa_count": 6,
    },
    "skill_references": {
        "glob_dirs": [os.path.expanduser("~/.claude/skills/")],
        "pattern": "*.md",
        "search_subdir": "references",
        "prompt_prefix": "Kingdom skill reference material",
        "qa_count": 8,
    },
    "battle_plans": {
        "pattern": f"{PROJECT_ROOT}/docs/battle-plans/*.md",
        "prompt_prefix": "sprint battle plan and execution strategy",
        "qa_count": 8,
    },
    "cs_sheets": {
        "root_dir": "/var/zhen/cs/sheets/",
        "extensions": [".md"],
        "prompt_prefix": "technical reference cheat sheet",
        "qa_count": 8,
    },
    "learning_books": {
        "root_dir": "/var/zhen/learning/txt/",
        "extensions": [".txt"],
        "prompt_prefix": "technical reference book",
        "qa_count": 12,
        "max_content": 8000,
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
        search_subdir = category.get("search_subdir", "")
        pat = category.get("pattern", "")
        for d in category["glob_dirs"]:
            for skill_dir in sorted(glob.glob(os.path.join(d, "*"))):
                if not os.path.isdir(skill_dir):
                    continue
                search_root = os.path.join(skill_dir, search_subdir) if search_subdir else skill_dir
                if not os.path.isdir(search_root):
                    continue
                for root, dirs, fnames in os.walk(search_root):
                    for fn in fnames:
                        if pat and not fnmatch.fnmatch(fn, pat):
                            continue
                        fp = os.path.join(root, fn)
                        try:
                            content = Path(fp).read_text(errors="replace")
                            if len(content) > 100:
                                files.append((fp, content))
                        except Exception:
                            pass

    if "root_dir" in category:
        exts = set(category.get("extensions", []))
        max_content = category.get("max_content", 0)
        root_dir = category["root_dir"]
        if os.path.isdir(root_dir):
            for root, dirs, fnames in os.walk(root_dir):
                for fn in sorted(fnames):
                    if any(fn.endswith(e) for e in exts):
                        fp = os.path.join(root, fn)
                        try:
                            content = Path(fp).read_text(errors="replace")
                            if len(content) > 100:
                                if max_content > 0 and len(content) > max_content:
                                    content = content[:max_content] + "\n\n[... truncated for length ...]"
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


def collect_full_repo() -> list[tuple[str, dict, str, str]]:
    """Collect ALL source files from the entire repo for --repo mode."""
    extensions = {
        ".md": ("documentation", 8),
        ".go": ("Go source code", 6),
        ".rs": ("Rust source code", 6),
        ".py": ("Python source code", 6),
        ".js": ("JavaScript source code", 5),
        ".html": ("HTML template", 4),
        ".css": ("CSS stylesheet", 3),
        ".yaml": ("YAML configuration", 5),
        ".yml": ("YAML configuration", 5),
        ".toml": ("TOML configuration", 4),
        ".nix": ("NixOS configuration", 5),
        ".proto": ("Protocol Buffers definition", 8),
        ".sh": ("Shell script", 5),
        ".sql": ("SQL schema", 6),
        ".c": ("C source code", 6),
        ".h": ("C header file", 5),
    }
    skip_dirs = {".git", "vendor", "node_modules", "target", "llama.cpp", "build", "__pycache__"}
    skip_suffixes = ("_test.go", "_extended_test.go", "_bench_test.go")

    files = []
    for root, dirs, fnames in os.walk(PROJECT_ROOT):
        dirs[:] = [d for d in dirs if d not in skip_dirs]
        for fn in fnames:
            ext = os.path.splitext(fn)[1]
            if ext not in extensions:
                continue
            if any(fn.endswith(s) for s in skip_suffixes):
                continue
            fp = os.path.join(root, fn)
            try:
                content = Path(fp).read_text(errors="replace")
                if len(content) < 100:
                    continue
                prompt_prefix, qa_count = extensions[ext]
                cat = {"prompt_prefix": f"{prompt_prefix} from the Unheaded Kingdom", "qa_count": qa_count}
                files.append(("repo", cat, fp, content))
            except Exception:
                pass

    # Also add skill files
    skills_dir = os.path.expanduser("~/.claude/skills/")
    if os.path.isdir(skills_dir):
        for skill_dir in sorted(glob.glob(os.path.join(skills_dir, "unheaded-*"))):
            for root, dirs, fnames in os.walk(skill_dir):
                for fn in fnames:
                    if fn.endswith(".md"):
                        fp = os.path.join(root, fn)
                        try:
                            content = Path(fp).read_text(errors="replace")
                            if len(content) > 100:
                                cat = {"prompt_prefix": "Kingdom skill definition", "qa_count": 10}
                                files.append(("skills", cat, fp, content))
                        except Exception:
                            pass

    return files


def generate_qa_pairs_local(inference_url: str, filepath: str,
                            content: str, category: dict, qa_count: int) -> list[dict]:
    """Generate QA pairs using local Mistral-7B via llama.cpp."""
    import requests

    max_content = 8000  # Smaller context for local model
    if len(content) > max_content:
        content = content[:max_content] + "\n\n[... truncated ...]"

    rel_path = filepath.replace(PROJECT_ROOT, "").replace(os.path.expanduser("~"), "~").lstrip("/")
    prompt_prefix = category.get("prompt_prefix", "document")

    prompt = f"""<s>[INST] You are generating training data for Zhenai, an AI assistant for the Unheaded Kingdom infrastructure platform.

Read this {prompt_prefix} and generate exactly {qa_count} question-answer pairs.
Questions should be natural. Answers must be grounded in the document (2-5 sentences).
Output ONLY a JSON array: [{{"question": "...", "answer": "..."}}]

File: {rel_path}

{content}
[/INST]"""

    try:
        resp = requests.post(
            f"{inference_url}/v1/completions",
            json={
                "prompt": prompt,
                "max_tokens": 2048,
                "temperature": 0.7,
                "stop": ["</s>", "[INST]"],
            },
            timeout=120,
        )
        resp.raise_for_status()
        text = resp.json()["choices"][0]["text"].strip()

        # Extract JSON from response
        if "[" in text:
            text = text[text.index("["):]
        if text.rfind("]") > 0:
            text = text[:text.rfind("]") + 1]

        pairs = json.loads(text)
        if not isinstance(pairs, list):
            return []

        result = []
        for p in pairs:
            if isinstance(p, dict) and "question" in p and "answer" in p:
                result.append({
                    "question": p["question"],
                    "answer": p["answer"],
                    "source": rel_path,
                })
        return result

    except json.JSONDecodeError:
        return []
    except Exception as e:
        print(f"  Local error for {rel_path}: {e}", file=sys.stderr)
        return []


def generate_qa_pairs(client, model: str, filepath: str,
                      content: str, category: dict, qa_count: int) -> list[dict]:
    """Generate QA pairs for a single document using Claude API."""
    # Truncate very long documents to fit context
    max_content = 12000  # ~3K tokens, leaves room for generation
    if len(content) > max_content:
        content = content[:max_content] + "\n\n[... truncated for length ...]"

    rel_path = filepath.replace(PROJECT_ROOT, "").replace(os.path.expanduser("~"), "~").lstrip("/")
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
            text = text.removeprefix("json")
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
    """Short hash of file path for checkpoint tracking.

    Not a security primitive: this is a filename-shortening cache key for
    resuming distillation runs. Collisions cost a redone chunk, nothing more.
    usedforsecurity=False says so to the reader and to bandit (B324) instead
    of leaving a bare md5() that looks like a bad crypto choice.
    """
    return hashlib.md5(path.encode(), usedforsecurity=False).hexdigest()[:12]


def main():
    parser = argparse.ArgumentParser(description="QA distillation for Zhenai (Claude API or local Mistral-7B)")
    parser.add_argument("--output", "-o", default="/var/zhen/distilled_qa.jsonl",
                        help="Output JSONL file path")
    parser.add_argument("--model", "-m", default="claude-haiku-4-5-20251001",
                        help="Claude model to use (ignored with --local)")
    parser.add_argument("--category", "-c", default=None,
                        help="Only process this category (adrs, docs, runbooks, etc)")
    parser.add_argument("--limit", type=int, default=0,
                        help="Max documents to process (0=all)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print QA pairs to stdout, don't write file")
    parser.add_argument("--qa-count", type=int, default=0,
                        help="Override QA pairs per document")
    parser.add_argument("--local", action="store_true",
                        help="Use local Mistral-7B via llama.cpp instead of Claude API")
    parser.add_argument("--inference-url", default="http://localhost:20100",
                        help="Local inference server URL (default: http://localhost:20100)")
    parser.add_argument("--repo", action="store_true",
                        help="Crawl entire ~/tmp/unheaded/ repo (2300+ files)")
    args = parser.parse_args()

    # Setup inference backend
    client = None
    if args.local:
        # Verify local inference is up
        import requests
        try:
            r = requests.get(f"{args.inference_url}/health", timeout=5)
            r.raise_for_status()
            print("=== Zhenai LOCAL Distillation (Mistral-7B) ===")
        except Exception as e:
            print(f"ERROR: Local inference not reachable at {args.inference_url}: {e}")
            print("Start llama-server first, or run without --local for Claude API")
            sys.exit(1)
    else:
        client = anthropic.Anthropic()  # Uses ANTHROPIC_API_KEY env var
        print("=== Zhenai Training Data Distillation (Claude API) ===")

    mode = "local (Mistral-7B)" if args.local else args.model
    print(f"  Mode:   {mode}")
    print(f"  Output: {args.output}")
    if args.repo:
        print("  Scope:  FULL REPO (all source + docs + skills)")
    print()

    # Load checkpoint (for resumability)
    checkpoint_path = args.output + ".checkpoint"
    completed = set()
    if os.path.exists(checkpoint_path):
        with open(checkpoint_path) as f:
            completed = {line.strip() for line in f}
        print(f"  Resuming: {len(completed)} documents already processed")

    # Collect all files
    categories = CATEGORIES
    if args.category:
        if args.category not in categories:
            print(f"ERROR: Unknown category '{args.category}'. Available: {', '.join(categories.keys())}")
            sys.exit(1)
        categories = {args.category: categories[args.category]}

    # Collect files
    if args.repo:
        all_files = collect_full_repo()
        print(f"  Full repo scan: {len(all_files)} files")
    else:
        all_files = []
        for cat_name, cat_cfg in categories.items():
            files = collect_files(cat_name, cat_cfg)
            for fp, content in files:
                all_files.append((cat_name, cat_cfg, fp, content))
        print(f"  Categories: {', '.join(categories.keys())}")

    if args.limit > 0:
        all_files = all_files[:args.limit]

    print(f"  Documents to process: {len(all_files)}")
    print()

    # Process documents — batch small docs together to respect rate limits
    # Trial tier: 5 requests/minute → 12s between requests
    total_pairs = 0
    total_docs = 0
    api_calls = 0
    start = time.time()

    output_file = None
    checkpoint_file = None
    if not args.dry_run:
        output_file = open(args.output, "a")  # Append mode for resumability
        checkpoint_file = open(checkpoint_path, "a")

    # Filter already-completed files
    pending = []
    for cat_name, cat_cfg, filepath, content in all_files:
        fhash = file_hash(filepath)
        if fhash not in completed:
            pending.append((cat_name, cat_cfg, filepath, content))

    print(f"  Pending: {len(pending)} documents ({len(all_files) - len(pending)} already done)")
    print()

    i = 0
    while i < len(pending):
        cat_name, cat_cfg, filepath, content = pending[i]
        qa_count = args.qa_count if args.qa_count > 0 else cat_cfg.get("qa_count", 10)
        rel_path = filepath.replace(PROJECT_ROOT, "").replace(os.path.expanduser("~"), "~").lstrip("/")

        print(f"  [{i+1}/{len(pending)}] {cat_name}: {rel_path} ({qa_count} QA)...", end=" ", flush=True)

        if args.local:
            pairs = generate_qa_pairs_local(args.inference_url, filepath, content, cat_cfg, qa_count)
        else:
            pairs = generate_qa_pairs(client, args.model, filepath, content, cat_cfg, qa_count)
        api_calls += 1

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
                checkpoint_file.write(file_hash(filepath) + "\n")
                checkpoint_file.flush()
        else:
            print("0 pairs (skipped)")
            # Still checkpoint to avoid retrying failed docs
            if checkpoint_file:
                checkpoint_file.write(file_hash(filepath) + "\n")
                checkpoint_file.flush()

        i += 1

        # Rate limiting: API needs 13s between calls (5 req/min), local has no limit
        if not args.dry_run and not args.local and i < len(pending):
            wait = 13.0  # 13s to stay safely under 5/min
            print(f"    (rate limit: waiting {wait:.0f}s, {api_calls} calls so far)", flush=True)
            time.sleep(wait)

    elapsed = time.time() - start

    print()
    print("=== Distillation Complete ===")
    print(f"  Documents: {total_docs}")
    print(f"  QA Pairs:  {total_pairs}")
    print(f"  Time:      {elapsed:.0f}s ({elapsed/60:.1f} min)")
    print(f"  Output:    {args.output}")

    if output_file:
        output_file.close()
        checkpoint_file.close()


if __name__ == "__main__":
    main()
