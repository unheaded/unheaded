#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (c) 2025-2026 Steven Bellis. All rights reserved.
"""
Build a coding-shape SFT corpus for the Zhenai coding-gate.

Per long-term plan §3 Track 2 + project_zhenai_coding_gate memory: the
goal is an offline coding agent across bash/python/go/rust/html/css/js
producing syntax help + code-review on pasted snippets. The Kingdom
RAFT corpus (raft/training/{train,eval}.jsonl) is wrong-shaped for
this — it's `[INST] Context: <code> Question: <q> [/INST] <answer>`
pairs about Unheaded internals, not bare-snippet coding-help.

This script extracts coding-shape Q→A pairs from sources we already
have on disk:

  Source A — Unheaded codebase (Go + Rust):
    Walk repo, extract function-with-doc-comment pairs as
        Q: "What does this function do?" + <function>
        A: <doc comment as natural-language description>
    Plus error-handling snippets, pkg/ patterns, etc.
    Encodes Unheaded house style by construction.

  Source B — StackOverflow dump (raft/corpus/stackoverflow):
    Filter by language tag, accepted answers only with code blocks.
    Q = SO question title + body
    A = accepted answer body (with code).
    Preserves attribution metadata for license compliance.

  Source C — Wikipedia code-articles subset (raft/corpus/wiki*):
    Lightweight explainer-shape pairs about syntax / patterns.

Output:
  raft/training/coding-train.jsonl  (~80 % of pairs)
  raft/training/coding-eval.jsonl   (~20 % of pairs, disjoint by chunk-id)

Each line: `{"question": "...", "answer": "...", "source": "...",
"language": "go|rust|python|...", "source_id": "..."}`

Then the existing `scripts/tokenize-kingdom-for-gemma4.py` (or a
dedicated `scripts/tokenize-coding-for-gemma4.py`) converts the JSONL
to Gemma-4 chat-template tokens.

This is the Track 2 ENTRY POINT. T2.1 (Champion API contract ADR-056)
and T2.2 (corpus fuzz harness) are SEPARATE sub-tasks, not auto-pivot
scope per Stevie's directive ("kick off coding-corpus build via
scripts/build-coding-corpus.py, do not wait for prompt").

Usage:
  /home/govan/tmp/gemma4-venv/bin/python3 scripts/build-coding-corpus.py \
      [--unheaded-only] [--max-pairs 5000]

Conservative default: Source A (Unheaded codebase) only. SO + Wikipedia
are gated behind `--include-stackoverflow` and `--include-wikipedia`
flags so the auto-pivot doesn't accidentally consume hours of disk I/O.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

REPO_ROOT = Path("/home/govan/tmp/unheaded")
OUT_TRAIN = REPO_ROOT / "raft" / "training" / "coding-train.jsonl"
OUT_EVAL = REPO_ROOT / "raft" / "training" / "coding-eval.jsonl"
OUT_MANIFEST = REPO_ROOT / "raft" / "training" / "coding-corpus-manifest.json"

# Languages we walk in Source A. Ordered by importance for the gate.
SOURCE_A_LANG_DIRS = {
    "go":  ["pkg", "cmd", "services", "internal"],
    "rust": ["crates"],
}

# Files to skip even within walked dirs.
SKIP_PATH_PATTERNS = (
    "vendor/", "target/", "node_modules/", ".git/",
    "_test.go",  # tests are mostly assertion-shape, not function-shape
    "test_",
    "tests/",
)


def is_walked(path: Path) -> bool:
    p = str(path)
    return not any(s in p for s in SKIP_PATH_PATTERNS)


def walk_unheaded_go() -> list[dict]:
    """
    Extract Go func-with-doc-comment pairs.
    Format: doc-comment block above `func ... {` -> (Q, A) pair.
    """
    out = []
    for d in SOURCE_A_LANG_DIRS["go"]:
        root = REPO_ROOT / d
        if not root.exists():
            continue
        for go_file in root.rglob("*.go"):
            if not is_walked(go_file):
                continue
            try:
                src = go_file.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            # Match: optional `//` doc lines above a `func ...` declaration.
            # Captures: doc block (group 1), function signature (group 2).
            pattern = re.compile(
                r"((?:^//.*\n)+)^(func\s+(?:\([^)]*\)\s+)?[A-Z]\w*\s*\([^)]*\)[^{\n]*)",
                re.MULTILINE,
            )
            for m in pattern.finditer(src):
                doc_raw = m.group(1).strip()
                sig = m.group(2).strip()
                # Strip leading "// " from doc lines.
                doc = "\n".join(
                    re.sub(r"^//\s?", "", ln) for ln in doc_raw.splitlines()
                ).strip()
                if len(doc) < 30 or len(sig) < 10:
                    continue
                # Function name is first identifier after "func"
                fn_name_m = re.search(r"func\s+(?:\([^)]*\)\s+)?(\w+)", sig)
                fn_name = fn_name_m.group(1) if fn_name_m else "<unknown>"
                out.append({
                    "question": f"What does the Go function `{fn_name}` do?",
                    "answer": doc + "\n\nSignature:\n```go\n" + sig + "\n```",
                    "source": str(go_file.relative_to(REPO_ROOT)),
                    "language": "go",
                    "source_id": hashlib.sha256(
                        (str(go_file) + sig).encode()
                    ).hexdigest()[:16],
                })
    return out


def walk_unheaded_rust() -> list[dict]:
    """
    Extract Rust fn-with-doc-comment pairs (`/// ... fn name(...)`).
    """
    out = []
    for d in SOURCE_A_LANG_DIRS["rust"]:
        root = REPO_ROOT / d
        if not root.exists():
            continue
        for rs_file in root.rglob("*.rs"):
            if not is_walked(rs_file):
                continue
            try:
                src = rs_file.read_text(encoding="utf-8", errors="replace")
            except OSError:
                continue
            # Match: `///` doc lines (>=2) above a `fn ...` declaration.
            pattern = re.compile(
                r"((?:^\s*///.*\n){2,})^\s*(?:pub\s+)?(?:async\s+)?(fn\s+\w+[^{\n]*)",
                re.MULTILINE,
            )
            for m in pattern.finditer(src):
                doc_raw = m.group(1)
                sig = m.group(2).strip()
                doc = "\n".join(
                    re.sub(r"^\s*///\s?", "", ln) for ln in doc_raw.splitlines()
                ).strip()
                if len(doc) < 30 or len(sig) < 10:
                    continue
                fn_name_m = re.search(r"fn\s+(\w+)", sig)
                fn_name = fn_name_m.group(1) if fn_name_m else "<unknown>"
                out.append({
                    "question": f"What does the Rust function `{fn_name}` do?",
                    "answer": doc + "\n\nSignature:\n```rust\n" + sig + "\n```",
                    "source": str(rs_file.relative_to(REPO_ROOT)),
                    "language": "rust",
                    "source_id": hashlib.sha256(
                        (str(rs_file) + sig).encode()
                    ).hexdigest()[:16],
                })
    return out


def split_train_eval(rows: list[dict], eval_frac: float = 0.20,
                    seed: int = 0xC0DE) -> tuple[list[dict], list[dict]]:
    """Deterministic split by source_id hash."""
    train, eval_ = [], []
    for r in rows:
        # Mix seed into the source_id hash so a different seed gives a
        # different split.
        h = hashlib.sha256(
            f"{seed:x}-{r['source_id']}".encode()
        ).hexdigest()
        # Use top-12 hex digits as a uniform-on-[0,1) draw.
        x = int(h[:12], 16) / float(1 << 48)
        (eval_ if x < eval_frac else train).append(r)
    return train, eval_


def write_jsonl(path: Path, rows: list[dict]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")


def write_manifest(path: Path, n_train: int, n_eval: int,
                   args: argparse.Namespace) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = {
        "kind": "coding-corpus",
        "version": "0.1.0",
        "generated_at": "2026-04-30",
        "generator": "scripts/build-coding-corpus.py",
        "sources": {
            "unheaded_codebase": {
                "enabled": True,
                "path_root": str(REPO_ROOT),
                "languages": list(SOURCE_A_LANG_DIRS.keys()),
                "license_note": "GPL-3.0-or-later (matches repo)",
            },
            "stackoverflow": {
                "enabled": bool(args.include_stackoverflow),
                "path_root": str(REPO_ROOT / "raft" / "corpus"),
                "license_note": "CC BY-SA 4.0 — attribution required",
            },
            "wikipedia": {
                "enabled": bool(args.include_wikipedia),
                "path_root": str(REPO_ROOT / "raft" / "corpus"),
                "license_note": "CC BY-SA 4.0 — attribution required",
            },
        },
        "split": {"train": n_train, "eval": n_eval, "eval_frac": 0.20,
                  "seed": "0xC0DE"},
        "outputs": {
            "train_jsonl": str(OUT_TRAIN),
            "eval_jsonl": str(OUT_EVAL),
        },
        "next_step": (
            "tokenize via scripts/tokenize-coding-for-gemma4.py (TODO) "
            "or adapt scripts/tokenize-kingdom-for-gemma4.py for the "
            "{question, answer} schema"
        ),
    }
    path.write_text(json.dumps(payload, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--unheaded-only", action="store_true",
        help="Only walk Source A (Unheaded codebase). Default.")
    parser.add_argument("--include-stackoverflow", action="store_true",
        help="Also pull from raft/corpus/stackoverflow (NOT IMPLEMENTED YET).")
    parser.add_argument("--include-wikipedia", action="store_true",
        help="Also pull from raft/corpus/wiki* (NOT IMPLEMENTED YET).")
    parser.add_argument("--max-pairs", type=int, default=10_000,
        help="Cap total pairs (post-split). Default 10k.")
    args = parser.parse_args()

    print(f"[corpus] walking Unheaded codebase under {REPO_ROOT}...")
    rows = []
    rows.extend(walk_unheaded_go())
    print(f"[corpus]   Go pairs:    {len([r for r in rows if r['language'] == 'go'])}")
    rows.extend(walk_unheaded_rust())
    print(f"[corpus]   Rust pairs:  {len([r for r in rows if r['language'] == 'rust'])}")

    if args.include_stackoverflow:
        print("[corpus] WARN: --include-stackoverflow not implemented yet")
    if args.include_wikipedia:
        print("[corpus] WARN: --include-wikipedia not implemented yet")

    print(f"[corpus] total raw pairs: {len(rows)}")

    if len(rows) > args.max_pairs:
        # Deterministic prune by source_id hash so we keep a stable subset.
        rows.sort(key=lambda r: r["source_id"])
        rows = rows[:args.max_pairs]
        print(f"[corpus] capped to {args.max_pairs} pairs")

    train, eval_ = split_train_eval(rows)
    print(f"[corpus] split: {len(train)} train + {len(eval_)} eval "
          f"({len(eval_)/(len(rows) or 1)*100:.1f} % eval)")

    write_jsonl(OUT_TRAIN, train)
    write_jsonl(OUT_EVAL, eval_)
    write_manifest(OUT_MANIFEST, len(train), len(eval_), args)

    print(f"[corpus] wrote {OUT_TRAIN}  ({OUT_TRAIN.stat().st_size:>10} bytes)")
    print(f"[corpus] wrote {OUT_EVAL}   ({OUT_EVAL.stat().st_size:>10} bytes)")
    print(f"[corpus] wrote {OUT_MANIFEST} ({OUT_MANIFEST.stat().st_size:>5} bytes)")
    print()
    print("Next step: tokenize the JSONL with scripts/tokenize-coding-for-gemma4.py")
    print("(TODO — adapt scripts/tokenize-kingdom-for-gemma4.py for the new schema)")


if __name__ == "__main__":
    main()
