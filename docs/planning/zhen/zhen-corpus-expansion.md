# UNHEADED-ZHEN: Expanded Corpus Architecture

## Addendum to RAFT Spec — Multi-Source Knowledge Ingestion

**Date**: 2026-02-27
**Scope**: Source code + Documents + Programming Books + RFCs

---

## 1. FOUR-RING CORPUS ARCHITECTURE

The Zhen doesn't just know Unheaded — it knows the **entire knowledge foundation** underneath it. Four concentric rings, innermost = highest priority.

```
┌─────────────────────────────────────────────────────────────────────┐
│                    RING 1: DOMAIN CORE                              │
│                    (Unheaded-specific)                               │
│                                                                     │
│  Source code (857K LOC) + Protocol specs + Skills + Lore + ADRs     │
│  + Your research notes/dumps + Session handoffs                     │
│  WEIGHT: 50% of training examples                                   │
│  PRIORITY: Highest — this is what makes Zhen unique             │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ builds on
┌─────────────────────────────────────────────────────────────────────┐
│                    RING 2: TECHNICAL FOUNDATION                     │
│                    (Stack-relevant books & docs)                     │
│                                                                     │
│  Go books + Rust books + Linux/kernel + eBPF + Networking +         │
│  Docker/containers + Security/crypto + Systems programming          │
│  WEIGHT: 25% of training examples                                   │
│  PRIORITY: High — domain fluency requires stack fluency             │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ builds on
┌─────────────────────────────────────────────────────────────────────┐
│                    RING 3: GENERAL ENGINEERING                       │
│                    (Broad CS/SRE knowledge)                          │
│                                                                     │
│  Algorithms + Data structures + Distributed systems + SRE +         │
│  Observability + IaC + CI/CD + Testing patterns                     │
│  WEIGHT: 15% of training examples                                   │
│  PRIORITY: Medium — fills domain gaps, prevents blind spots         │
└─────────────────────────────────────────────────────────────────────┘
                              ↓ builds on
┌─────────────────────────────────────────────────────────────────────┐
│                    RING 4: COMPLETE LIBRARY                          │
│                    (All books + All RFCs)                             │
│                                                                     │
│  ALL free-programming-books (~1,636 resources across 40+ langs)     │
│  + ALL IETF RFCs (IPv4/IPv6/TCP/UDP/QUIC/HTTP3/HBH/CBOR/etc.)      │
│  + Official language docs (Go stdlib, Rust std, Python, etc.)       │
│  WEIGHT: 10% of training examples                                   │
│  PRIORITY: Background — massive breadth, the full knowledge base    │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. FREE-PROGRAMMING-BOOKS ANALYSIS

The uploaded zip (`free-programming-books-main.zip`) is the **EbookFoundation/free-programming-books** repository — the largest curated index of free programming resources on the internet.

**It contains**: 216 markdown files with ~1,636 book/resource links across 40+ languages and dozens of subjects.

**It is NOT**: The actual books. It's the index. We use it as a **download manifest**.

### 2.1 Relevant Categories for Zhen

**RING 2 — Stack-Critical** (must download):

| Category | Est. Books | Why |
|----------|-----------|-----|
| Go/Golang | ~31 | Primary implementation language |
| Rust | ~28 | eBPF programs, performance-critical paths |
| Linux / Kernel | ~20 | OS foundation, eBPF host |
| Networking (TCP/IP/IPv6) | ~15 | Wire format, protocol stack |
| Docker / Containers | ~12 | Container fleet |
| Security / Cryptography | ~18 | PQC, TLS, mTLS |
| Assembly / Low-level | ~8 | eBPF ISA, MBC bytecode |
| Nix / NixOS | ~5 | Deployment platform |
| gRPC / Protobuf | ~4 | Service communication |

**RING 3 — General Engineering** (selective download):

| Category | Est. Books | Why |
|----------|-----------|-----|
| Algorithms & Data Structures | ~20 | Ring buffers, HNSW, CRC |
| Distributed Systems | ~10 | Saga pattern, eventual consistency |
| Operating Systems | ~8 | Process scheduling, memory model |
| Databases | ~6 | SQLite, ClickHouse patterns |
| CI/CD / DevOps | ~8 | Pipeline design |
| Testing | ~5 | TDD patterns, property-based testing |

**TOTAL ESTIMATE**: ~190 books → ~50-100GB raw → ~10-20GB cleaned text

### 2.2 Download Manifest Generator

```python
#!/usr/bin/env python3
"""
scripts/zhen/00_build_book_manifest.py

Parses free-programming-books index and generates a download manifest
of books relevant to the Unheaded Zhen's knowledge domain.

Output: /mnt/hdd/zhen/books/manifest.json
"""

import re
import json
from pathlib import Path

BOOKS_DIR = Path("/tmp/fpb/free-programming-books-main")
OUTPUT = Path("/mnt/hdd/zhen/books/manifest.json")

# Keywords that indicate relevance to Unheaded's stack
STACK_KEYWORDS = {
    # Ring 2: Stack-critical
    "go": 2, "golang": 2, "rust": 2,
    "linux": 2, "kernel": 2, "ebpf": 2, "bpf": 2,
    "networking": 2, "tcp": 2, "ip": 2, "ipv6": 2, "udp": 2,
    "docker": 2, "container": 2, "kubernetes": 2,
    "security": 2, "cryptography": 2, "tls": 2,
    "assembly": 2, "nix": 2, "nixos": 2,
    "grpc": 2, "protobuf": 2, "protocol buffer": 2,
    # Ring 3: General engineering
    "algorithm": 3, "data structure": 3,
    "distributed system": 3, "operating system": 3,
    "database": 3, "testing": 3, "devops": 3,
    "site reliability": 3, "observability": 3,
    "systems programming": 3, "concurrency": 3,
}

def extract_links(md_content: str) -> list:
    """Extract markdown links: [title](url)"""
    pattern = r'\[([^\]]+)\]\((https?://[^\)]+)\)'
    return [(title, url) for title, url in re.findall(pattern, md_content)]

def classify_link(title: str, section_context: str) -> dict:
    """Classify a book link by relevance ring and topic."""
    combined = f"{title} {section_context}".lower()

    best_ring = None
    matched_keywords = []

    for keyword, ring in STACK_KEYWORDS.items():
        if keyword in combined:
            if best_ring is None or ring < best_ring:
                best_ring = ring
            matched_keywords.append(keyword)

    return {
        "ring": best_ring,
        "keywords": matched_keywords,
    }

def build_manifest():
    """Parse all book lists and build download manifest."""
    manifest = {"ring_2": [], "ring_3": [], "total": 0}

    # Parse English language-specific books
    for md_file in [
        BOOKS_DIR / "books" / "free-programming-books-langs.md",
        BOOKS_DIR / "books" / "free-programming-books-subjects.md",
    ]:
        if not md_file.exists():
            continue

        content = md_file.read_text()
        current_section = ""

        for line in content.split("\n"):
            if line.startswith("#"):
                current_section = line.strip("# ").strip()

            links = extract_links(line)
            for title, url in links:
                classification = classify_link(title, current_section)

                if classification["ring"] is None:
                    continue  # Not relevant

                entry = {
                    "title": title,
                    "url": url,
                    "section": current_section,
                    "ring": classification["ring"],
                    "keywords": classification["keywords"],
                    "format": guess_format(url),
                }

                if classification["ring"] == 2:
                    manifest["ring_2"].append(entry)
                else:
                    manifest["ring_3"].append(entry)

    manifest["total"] = len(manifest["ring_2"]) + len(manifest["ring_3"])

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    with open(OUTPUT, "w") as f:
        json.dump(manifest, f, indent=2)

    print(f"Ring 2 (stack-critical): {len(manifest['ring_2'])} books")
    print(f"Ring 3 (general eng):    {len(manifest['ring_3'])} books")
    print(f"Total:                   {manifest['total']} books")

    return manifest

def guess_format(url: str) -> str:
    """Guess the book format from URL."""
    url_lower = url.lower()
    if url_lower.endswith(".pdf"):
        return "pdf"
    elif "github" in url_lower or "gitbook" in url_lower:
        return "html_book"
    elif "readthedocs" in url_lower:
        return "html_docs"
    elif url_lower.endswith(".epub"):
        return "epub"
    else:
        return "html"

if __name__ == "__main__":
    build_manifest()
```

---

## 3. BOOK INGESTION PIPELINE

### 3.1 Download Strategy

```
┌──────────────────────────────────────────────────────────────────┐
│                    STORAGE LAYOUT                                 │
│                                                                   │
│  /mnt/hdd/zhen/books/                                        │
│  ├── manifest.json          # What to download                   │
│  ├── raw/                   # Original downloads (PDF/HTML/epub) │
│  │   ├── ring_2/            # Stack-critical books               │
│  │   └── ring_3/            # General engineering                │
│  ├── text/                  # Extracted plain text               │
│  │   ├── ring_2/                                                 │
│  │   └── ring_3/                                                 │
│  └── chunks/                # Chunked and embedded               │
│      ├── ring_2/                                                 │
│      └── ring_3/                                                 │
└──────────────────────────────────────────────────────────────────┘
```

### 3.2 Format-Specific Extraction

```python
#!/usr/bin/env python3
"""
scripts/zhen/00b_download_and_extract_books.py

Downloads books from manifest, extracts text, handles multiple formats.
Respects rate limits. Resumable (skips already-downloaded).
"""

import json
import os
import subprocess
import time
from pathlib import Path

RAW_DIR = Path("/mnt/hdd/zhen/books/raw")
TEXT_DIR = Path("/mnt/hdd/zhen/books/text")

def download_book(entry: dict) -> Path:
    """Download a single book. Returns path to raw file."""
    ring_dir = RAW_DIR / f"ring_{entry['ring']}"
    ring_dir.mkdir(parents=True, exist_ok=True)

    # Sanitize filename
    safe_title = "".join(
        c if c.isalnum() or c in "-_ " else "_"
        for c in entry["title"][:80]
    ).strip()

    fmt = entry["format"]
    ext = {"pdf": ".pdf", "epub": ".epub"}.get(fmt, ".html")
    output = ring_dir / f"{safe_title}{ext}"

    if output.exists():
        return output  # Already downloaded

    try:
        if fmt in ("pdf", "epub"):
            subprocess.run(
                ["wget", "-q", "--timeout=30", "-O", str(output), entry["url"]],
                check=True, timeout=60,
            )
        else:
            # HTML: use wget for full page + assets
            subprocess.run(
                ["wget", "-q", "--timeout=30",
                 "--convert-links", "-p",
                 "-O", str(output), entry["url"]],
                check=True, timeout=60,
            )

        time.sleep(1)  # Rate limit: 1 request/sec
        return output

    except Exception as e:
        print(f"  SKIP: {entry['title']} — {e}")
        if output.exists():
            output.unlink()
        return None

def extract_text(raw_path: Path, ring: int) -> Path:
    """Extract plain text from downloaded book."""
    text_dir = TEXT_DIR / f"ring_{ring}"
    text_dir.mkdir(parents=True, exist_ok=True)

    output = text_dir / f"{raw_path.stem}.txt"
    if output.exists():
        return output

    ext = raw_path.suffix.lower()

    try:
        if ext == ".pdf":
            # pdftotext (poppler-utils) — fast and reliable
            subprocess.run(
                ["pdftotext", "-layout", str(raw_path), str(output)],
                check=True, timeout=120,
            )
        elif ext == ".epub":
            # pandoc for epub → plain text
            subprocess.run(
                ["pandoc", "-f", "epub", "-t", "plain",
                 "-o", str(output), str(raw_path)],
                check=True, timeout=120,
            )
        elif ext == ".html":
            # pandoc for html → plain text
            subprocess.run(
                ["pandoc", "-f", "html", "-t", "plain",
                 "-o", str(output), str(raw_path)],
                check=True, timeout=120,
            )
        else:
            return None

        return output

    except Exception as e:
        print(f"  EXTRACT FAIL: {raw_path.name} — {e}")
        return None

def process_manifest():
    """Download and extract all books from manifest."""
    manifest = json.loads(
        Path("/mnt/hdd/zhen/books/manifest.json").read_text()
    )

    for ring_key in ["ring_2", "ring_3"]:
        ring_num = int(ring_key[-1])
        entries = manifest[ring_key]
        print(f"\n=== {ring_key.upper()}: {len(entries)} books ===")

        for i, entry in enumerate(entries):
            print(f"  [{i+1}/{len(entries)}] {entry['title'][:60]}...")
            raw = download_book(entry)
            if raw:
                extract_text(raw, ring_num)

    print("\n=== DONE ===")
    # Report sizes
    for ring in [2, 3]:
        text_dir = TEXT_DIR / f"ring_{ring}"
        if text_dir.exists():
            total = sum(f.stat().st_size for f in text_dir.glob("*.txt"))
            count = len(list(text_dir.glob("*.txt")))
            print(f"Ring {ring}: {count} books, {total/1e6:.1f} MB text")

if __name__ == "__main__":
    process_manifest()
```

---

## 4. UPDATED CORPUS INVENTORY

### Before (Unheaded Only)
| Source | Chunks | Weight |
|--------|--------|--------|
| Unheaded code + docs | ~20K | 100% |

### After (Four-Ring)
| Ring | Source | Est. Chunks | Weight | Est. Size |
|------|--------|-------------|--------|-----------|
| 1 | Unheaded source code | ~15K | 35% | ~30MB |
| 1 | Unheaded docs/specs/skills/notes | ~8K | 15% | ~15MB |
| 2 | Go/Rust/Linux/eBPF/Network books | ~30K | 15% | ~200MB |
| 2 | Stack-specific official docs | ~5K | 10% | ~40MB |
| 3 | General CS/SRE/distributed systems books | ~10K | 15% | ~80MB |
| 4 | ALL free-programming-books resources | ~50K | 5% | ~500MB |
| 4 | ALL IETF RFCs + standards | ~20K | 5% | ~200MB |
| **TOTAL** | | **~138K** | **100%** | **~1.1GB** |

### Ring Weighting During RAFT Construction

The 60/30/10 split is enforced during Phase 4 (RAFT dataset construction):

```python
# In 04_build_raft_dataset.py — updated sampling strategy

RING_WEIGHTS = {
    1: 0.50,  # 50% of QA pairs from Unheaded domain
    2: 0.25,  # 25% from stack-relevant books
    3: 0.15,  # 15% from general engineering
    4: 0.10,  # 10% from complete library (all books + all RFCs)
}

def sample_qa_by_ring(all_qa: list, target_size: int) -> list:
    """Sample QA pairs respecting ring weights."""
    by_ring = {1: [], 2: [], 3: [], 4: []}
    for qa in all_qa:
        ring = qa.get("source_ring", 4)
        if ring in by_ring:
            by_ring[ring].append(qa)
        else:
            by_ring[4].append(qa)  # Default: Ring 4 (library)

    sampled = []
    for ring, weight in RING_WEIGHTS.items():
        n = int(target_size * weight)
        pool = by_ring.get(ring, [])
        if len(pool) >= n:
            sampled.extend(random.sample(pool, n))
        else:
            sampled.extend(pool)  # Take all if not enough

    random.shuffle(sampled)
    return sampled[:target_size]
```

---

## 5. ADDITIONAL SOURCES TO INGEST

Beyond the free-programming-books index, we should also pull:

### 5.1 Official Documentation (HTML scrape → text)

| Source | URL | Why |
|--------|-----|-----|
| Go Standard Library | pkg.go.dev | Go service implementation |
| The Rust Book | doc.rust-lang.org/book | eBPF Rust programs |
| Aya-rs eBPF docs | aya-rs.dev/book | Our eBPF framework |
| NixOS Manual | nixos.org/manual | Deployment platform |
| CBOR (RFC 8949) | rfc-editor.org | Sophia serialization |
| IPv6 (RFC 8200) | rfc-editor.org | Wire format transport |
| HBH Options (RFC 8200 §4.3) | rfc-editor.org | Monad carries here |
| QUIC (RFC 9000) | rfc-editor.org | Future transport |
| vLLM docs | docs.vllm.ai | Self-serving (meta!) |
| BPF & XDP Reference | docs.kernel.org/bpf | eBPF verifier, maps |

### 5.2 IETF RFCs (Already in repo!)

We already have 19 RFCs in `docs/protocol/rfc-references/`:
```
rfc768.txt rfc791.txt rfc2460.txt rfc2474.txt rfc5095.txt
rfc5722.txt rfc5871.txt rfc6437.txt rfc6564.txt rfc6864.txt
rfc6935.txt rfc6946.txt rfc6964.txt rfc8436.txt rfc9000.txt
rfc9114.txt rfc9293.txt rfc9868.txt
```

These go straight into Ring 2 — zero download needed.

### 5.3 Your Own Notes/Research

Any markdown notes, research dumps, chat exports, or design docs you've written outside the repo. These are **Ring 1** material — they contain YOUR mental model of the system.

---

## 6. UPDATED PHASE 1: MULTI-SOURCE CORPUS PREPARATION

```python
#!/usr/bin/env python3
"""
scripts/zhen/01_prepare_corpus.py (UPDATED)

Multi-source corpus preparation with three-ring architecture.
"""

import os
import json
import hashlib
from pathlib import Path
from typing import List, Dict

from langchain.text_splitter import (
    RecursiveCharacterTextSplitter,
    Language,
    MarkdownHeaderTextSplitter,
)

# Source directories
SOURCES = {
    # Ring 1: Domain Core (Unheaded-specific)
    "unheaded": {
        "path": Path(os.path.expanduser("~/tmp/unheaded")),
        "ring": 1,
        "description": "Unheaded source code, docs, specs, skills",
    },
    # Ring 2: Technical Foundation (stack-relevant)
    "books_stack": {
        "path": Path("/mnt/hdd/zhen/books/text/ring_2"),
        "ring": 2,
        "description": "Go, Rust, Linux, eBPF, networking, security books",
    },
    "official_docs": {
        "path": Path("/mnt/hdd/zhen/official_docs"),
        "ring": 2,
        "description": "Go stdlib, Rust book, aya-rs, NixOS manual, BPF docs",
    },
    "github_go_rust": {
        "path": Path("/mnt/hdd/zhen/github_repos"),
        "ring": 2,  # Go/Rust repos = Ring 2, infra = Ring 3
        "description": "KILLER COMBO: k8s, moby, prometheus, tokio, aya, cilium",
    },
    "linux_kernel": {
        "path": Path("/mnt/hdd/zhen/linux_kernel/linux"),
        "ring": 2,
        "description": "KILLER COMBO: Linux kernel source + eBPF subsystem + docs",
    },
    # Ring 3: General Engineering (broad CS/SRE)
    "books_general": {
        "path": Path("/mnt/hdd/zhen/books/text/ring_3"),
        "ring": 3,
        "description": "Algorithms, distributed systems, SRE, testing, databases",
    },
    "arxiv_cs": {
        "path": Path("/mnt/hdd/zhen/arxiv_cs/text"),
        "ring": 3,
        "description": "KILLER COMBO: eBPF, QUIC, BBR, PQC, RAFT, LoRA papers",
    },
    # Ring 4: Complete Library (ALL books + ALL RFCs)
    "books_all": {
        "path": Path("/mnt/hdd/zhen/books/text/ring_4"),
        "ring": 4,
        "description": "ALL free-programming-books across all languages",
    },
    "rfcs_all": {
        "path": Path("/mnt/hdd/zhen/rfcs"),
        "ring": 4,
        "description": "ALL IETF RFCs — complete standards corpus",
    },
    "wikipedia": {
        "path": Path("/mnt/hdd/zhen/wikipedia/text"),
        "ring": 4,
        "description": "KILLER COMBO: Full English Wikipedia dump",
    },
    "stackoverflow": {
        "path": Path("/mnt/hdd/zhen/stackoverflow"),
        "ring": 4,
        "description": "KILLER COMBO: SO + ServerFault + Unix.SE (score >= 3)",
    },
}

OUTPUT_DIR = Path("/mnt/hdd/zhen/corpus")

def prepare_multi_source_corpus():
    """Process all sources into unified chunked corpus."""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    all_chunks = []

    for source_name, source_config in SOURCES.items():
        source_path = source_config["path"]
        ring = source_config["ring"]

        if not source_path.exists():
            print(f"SKIP {source_name}: {source_path} not found")
            continue

        print(f"\n=== Processing {source_name} (Ring {ring}) ===")
        source_chunks = process_source(source_path, ring, source_name)
        all_chunks.extend(source_chunks)
        print(f"  → {len(source_chunks)} chunks")

    # Write unified corpus
    output_path = OUTPUT_DIR / "corpus_multi.jsonl"
    with open(output_path, "w") as fh:
        for chunk in all_chunks:
            fh.write(json.dumps(chunk) + "\n")

    # Stats
    print(f"\n{'='*60}")
    print(f"TOTAL CHUNKS: {len(all_chunks)}")
    for ring in range(1, 4):
        ring_chunks = [c for c in all_chunks if c["ring"] == ring]
        total_tokens = sum(c["token_count_approx"] for c in ring_chunks)
        print(f"  Ring {ring}: {len(ring_chunks)} chunks, ~{total_tokens:,} tokens")

def process_source(source_path: Path, ring: int, source_name: str) -> list:
    """Process a single source directory into chunks."""
    chunks = []
    skip_dirs = {".git", "node_modules", "vendor", "__pycache__", ".claude"}

    ALLOWED_EXTS = {
        ".go", ".rs", ".md", ".yaml", ".yml", ".nix", ".sh",
        ".py", ".json", ".toml", ".proto", ".c", ".h",
        ".txt", ".cfg", ".conf", ".ts", ".tsx", ".js",
    }

    for root, dirs, files in os.walk(source_path):
        dirs[:] = [d for d in dirs if d not in skip_dirs]
        for f in files:
            fp = Path(root) / f
            if fp.suffix in ALLOWED_EXTS:
                file_chunks = chunk_file(fp, ring, source_name)
                chunks.extend(file_chunks)

    return chunks

# ... (chunk_file implementation same as original spec)

if __name__ == "__main__":
    prepare_multi_source_corpus()
```

---

## 7. STORAGE IMPACT

### 2TB HDD Budget — Alpha (Killer Combo)

**ALPHA IMPLEMENTATION** focuses on the 5 highest-value sources that make Zhen
terrifyingly knowledgeable. Everything else is documented in the roadmap for later phases.

```
/mnt/hdd/zhen/
│
│  ── RING 1: DOMAIN CORE ──────────────────────────────────
├── unheaded/              (symlink → ~/tmp/unheaded)
│
│  ── RING 2: TECHNICAL FOUNDATION ─────────────────────────
├── books/
│   ├── raw/
│   │   └── ring_2/        ~50 GB  (stack-critical PDFs/HTML/epub)
│   └── text/
│       └── ring_2/        ~10 GB  (extracted plain text)
│
│  ── RING 3: GENERAL ENGINEERING ──────────────────────────
├── books/
│   ├── raw/
│   │   └── ring_3/        ~30 GB  (general CS PDFs/HTML/epub)
│   └── text/
│       └── ring_3/        ~6 GB   (extracted plain text)
│
│  ── RING 4: COMPLETE LIBRARY ─────────────────────────────
├── books/
│   ├── raw/
│   │   └── ring_4/        ~120 GB (ALL free-programming-books)
│   └── text/
│       └── ring_4/        ~25 GB  (extracted plain text)
├── rfcs/                  ~3 GB   (ALL IETF RFCs — ~9,000 docs)
│
│  ── KILLER COMBO (ALPHA) ─────────────────────────────────
├── wikipedia/             ~90 GB  (full English dump, extracted text)
├── stackoverflow/         ~200 GB (SO + ServerFault + Unix.SE dumps)
├── github_repos/          ~100 GB (targeted Go/Rust/infra repos)
├── linux_kernel/          ~3 GB   (source + docs, depth=1)
├── arxiv_cs/              ~50 GB  (selected networking/systems/security/ML papers)
│
│  ── TRAINING INFRASTRUCTURE ──────────────────────────────
├── official_docs/         ~5 GB   (Go stdlib, Rust, aya-rs, NixOS)
├── corpus/                ~5 GB   (chunked JSONL — all rings + killer combo)
├── qa_pairs.jsonl         ~1 GB   (QA from all sources)
├── raft_train.jsonl       ~2 GB   (final RAFT dataset)
├── models/                ~200 GB (base + merged + checkpoints)
├── logs/                  ~5 GB   (TensorBoard)
└── huggingface/           ~200 GB (HF cache)
────────────────────────────────────
TOTAL:                     ~1,105 GB of 2 TB
FREE:                      ~895 GB (breathing room for experiments)
```

### The Killer Combo — Alpha Priority

These 5 sources alone (~440GB) transform Zhen from a domain expert into a
**systems engineering omniscient**:

| Source | Size | Ring | Why It's Killer |
|--------|------|------|-----------------|
| **Wikipedia** | ~90GB | 4 | "What is X" for anything — networking, crypto, CS, math, physics |
| **Stack Overflow** | ~200GB | 4 | Every Go/Rust/Linux/Docker/eBPF Q&A ever asked + answered |
| **GitHub repos** | ~100GB | 2-3 | Real-world code from k8s, moby, prometheus, tokio, aya-rs, nixpkgs |
| **Linux kernel** | ~3GB | 2 | eBPF at the kernel level — `bpf()` syscall, verifier, maps, helpers |
| **ArXiv CS** | ~50GB | 3-4 | QUIC, BBR, eBPF, PQC, CRDTs, LoRA/RAFT papers (meta!) |

#### Alpha Download Scripts

```bash
#!/bin/bash
# scripts/zhen/00c_download_killer_combo.sh
set -euo pipefail

ZHEN_DIR="/mnt/hdd/zhen"

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  KILLER COMBO — Alpha Corpus Download                    ║"
echo "║  ~440 GB total — estimated 8-24 hrs on gigabit           ║"
echo "╚══════════════════════════════════════════════════════════╝"

# ================================================================
# 1. WIKIPEDIA (~22GB compressed → ~90GB extracted)
# ================================================================
echo ""
echo "=== [1/5] Wikipedia English dump ==="
mkdir -p "$ZHEN_DIR/wikipedia"
cd "$ZHEN_DIR/wikipedia"

# Download latest article dump (bz2 compressed XML)
wget -c "https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles.xml.bz2"

# Extract to plain text using wikiextractor
pip install wikiextractor --break-system-packages 2>/dev/null
python3 -m wikiextractor.WikiExtractor \
    enwiki-latest-pages-articles.xml.bz2 \
    --output text/ \
    --bytes 1M \
    --processes 4 \
    --no-templates \
    --json

echo "  Wikipedia: DONE"

# ================================================================
# 2. STACK OVERFLOW (~60GB compressed → ~200GB extracted)
# ================================================================
echo ""
echo "=== [2/5] Stack Overflow + ServerFault + Unix.SE dumps ==="
mkdir -p "$ZHEN_DIR/stackoverflow"
cd "$ZHEN_DIR/stackoverflow"

# Stack Exchange data dumps from archive.org
# Focus on the sites most relevant to our stack
for site in stackoverflow.com serverfault.com unix.stackexchange.com \
            superuser.com security.stackexchange.com \
            networkengineering.stackexchange.com; do
    echo "  Downloading $site..."
    wget -c "https://archive.org/download/stackexchange/${site}.7z" || true
done

# Extract (requires p7zip)
sudo apt install -y p7zip-full 2>/dev/null
for archive in *.7z; do
    echo "  Extracting $archive..."
    7z x -o"${archive%.7z}" "$archive" -aoa || true
done

# Convert XML posts to JSONL (Posts.xml → posts.jsonl)
python3 << 'PYEOF'
import xml.etree.ElementTree as ET
import json, glob, html, re, os

for xml_file in glob.glob("*/Posts.xml"):
    site = os.path.dirname(xml_file)
    out_file = f"{site}/posts.jsonl"
    print(f"  Converting {xml_file} → {out_file}")
    count = 0
    with open(out_file, "w") as fh:
        for event, elem in ET.iterparse(xml_file, events=("end",)):
            if elem.tag == "row":
                body = elem.get("Body", "")
                title = elem.get("Title", "")
                score = int(elem.get("Score", "0"))
                post_type = elem.get("PostTypeId", "0")

                # Only keep questions (1) and answers (2) with score >= 3
                if post_type in ("1", "2") and score >= 3:
                    clean_body = re.sub(r'<[^>]+>', '', html.unescape(body))
                    fh.write(json.dumps({
                        "title": title,
                        "body": clean_body,
                        "score": score,
                        "type": "question" if post_type == "1" else "answer",
                        "tags": elem.get("Tags", ""),
                        "site": site,
                    }) + "\n")
                    count += 1
                elem.clear()
    print(f"    → {count} high-quality posts")
PYEOF

echo "  Stack Overflow: DONE"

# ================================================================
# 3. GITHUB REPOS (~100GB targeted clones)
# ================================================================
echo ""
echo "=== [3/5] GitHub targeted repo clones ==="
mkdir -p "$ZHEN_DIR/github_repos"
cd "$ZHEN_DIR/github_repos"

# Go ecosystem (Ring 2 — directly relevant to Unheaded)
declare -a GO_REPOS=(
    "kubernetes/kubernetes"
    "moby/moby"
    "prometheus/prometheus"
    "grafana/grafana"
    "etcd-io/etcd"
    "containerd/containerd"
    "hashicorp/consul"
    "hashicorp/terraform"
    "traefik/traefik"
    "cilium/cilium"
    "cloudflare/cloudflared"
    "grpc/grpc-go"
    "nats-io/nats-server"
    "loft-sh/vcluster"
)

# Rust ecosystem (Ring 2 — eBPF + performance)
declare -a RUST_REPOS=(
    "tokio-rs/tokio"
    "serde-rs/serde"
    "aya-rs/aya"
    "libbpf/libbpf-rs"
    "rustls/rustls"
    "hyperium/hyper"
    "cloudflare/quiche"
    "quinn-rs/quinn"
    "tikv/tikv"
)

# Infrastructure (Ring 2-3)
declare -a INFRA_REPOS=(
    "NixOS/nixpkgs"
    "ansible/ansible"
    "containers/podman"
    "opencontainers/runc"
    "lxc/lxd"
    "coreos/flannel"
    "projectcalico/calico"
    "FRRouting/frr"
    "suricata/suricata"
    "vectordotdev/vector"
)

for repo in "${GO_REPOS[@]}" "${RUST_REPOS[@]}" "${INFRA_REPOS[@]}"; do
    dir_name=$(echo "$repo" | tr '/' '_')
    if [ -d "$dir_name" ]; then
        echo "  SKIP: $repo (already cloned)"
        continue
    fi
    echo "  Cloning $repo (shallow)..."
    git clone --depth 1 --single-branch "https://github.com/$repo.git" "$dir_name" || true
done

echo "  GitHub repos: DONE"

# ================================================================
# 4. LINUX KERNEL (~3GB)
# ================================================================
echo ""
echo "=== [4/5] Linux kernel source + docs ==="
mkdir -p "$ZHEN_DIR/linux_kernel"
cd "$ZHEN_DIR/linux_kernel"

if [ ! -d "linux" ]; then
    git clone --depth 1 "https://github.com/torvalds/linux.git"
fi

echo "  Linux kernel: DONE"

# ================================================================
# 5. ARXIV CS PAPERS (~50GB selected)
# ================================================================
echo ""
echo "=== [5/5] ArXiv CS papers (selected topics) ==="
mkdir -p "$ZHEN_DIR/arxiv_cs"
cd "$ZHEN_DIR/arxiv_cs"

# Use arxiv bulk data access for selected categories
# cs.NI (Networking), cs.DC (Distributed Computing), cs.CR (Cryptography),
# cs.OS (Operating Systems), cs.PF (Performance), cs.LG (Machine Learning)
#
# Option A: Use arxiv API for targeted papers
# Option B: Use Semantic Scholar API for citation-filtered papers
# Option C: Use kaggle arxiv dataset (metadata) + targeted PDF download

cat > fetch_papers.py << 'PYEOF'
"""
Fetch ArXiv papers relevant to Unheaded Zhen's domain.
Uses ArXiv API with targeted search queries.
Downloads PDFs, extracts text with pdftotext.
"""
import urllib.request
import urllib.parse
import xml.etree.ElementTree as ET
import subprocess
import time
import os
from pathlib import Path

OUTPUT_DIR = Path("/mnt/hdd/zhen/arxiv_cs")
TEXT_DIR = OUTPUT_DIR / "text"
TEXT_DIR.mkdir(parents=True, exist_ok=True)

# Targeted queries — papers Zhen MUST know
QUERIES = [
    # Networking & Protocols
    ("eBPF XDP packet processing", 100),
    ("QUIC protocol performance", 50),
    ("BBR congestion control", 30),
    ("IPv6 extension headers", 30),
    ("software defined networking dataplane", 50),
    # Distributed Systems
    ("distributed consensus Raft Paxos", 50),
    ("CRDTs conflict-free replicated", 30),
    ("microservice architecture observability", 50),
    ("service mesh networking", 30),
    # Security & Crypto
    ("post-quantum cryptography lattice", 50),
    ("TLS 1.3 security analysis", 30),
    ("eBPF security verification", 30),
    ("zero trust network architecture", 30),
    # ML/AI (meta — Zhen knows how it was built)
    ("retrieval augmented generation RAG", 50),
    ("LoRA low-rank adaptation fine-tuning", 30),
    ("RAFT retrieval augmented fine-tuning", 20),
    ("domain specific language model", 30),
    # Systems
    ("Linux kernel eBPF BPF", 50),
    ("container runtime security", 30),
    ("NixOS reproducible builds", 20),
    ("immutable infrastructure deployment", 20),
]

ARXIV_API = "http://export.arxiv.org/api/query"

def fetch_papers(query: str, max_results: int):
    """Fetch paper metadata from ArXiv API."""
    params = urllib.parse.urlencode({
        "search_query": f"all:{query}",
        "start": 0,
        "max_results": max_results,
        "sortBy": "relevance",
    })
    url = f"{ARXIV_API}?{params}"

    try:
        response = urllib.request.urlopen(url, timeout=30)
        root = ET.fromstring(response.read())
        ns = {"atom": "http://www.w3.org/2005/Atom"}

        papers = []
        for entry in root.findall("atom:entry", ns):
            paper_id = entry.find("atom:id", ns).text.split("/")[-1]
            title = entry.find("atom:title", ns).text.strip()

            # Get PDF link
            pdf_link = None
            for link in entry.findall("atom:link", ns):
                if link.get("title") == "pdf":
                    pdf_link = link.get("href")

            if pdf_link:
                papers.append({
                    "id": paper_id,
                    "title": title,
                    "pdf_url": pdf_link,
                })
        return papers

    except Exception as e:
        print(f"  API error for '{query}': {e}")
        return []

def download_and_extract(paper: dict):
    """Download PDF and extract text."""
    pdf_path = OUTPUT_DIR / "pdfs" / f"{paper['id']}.pdf"
    txt_path = TEXT_DIR / f"{paper['id']}.txt"

    if txt_path.exists():
        return  # Already processed

    pdf_path.parent.mkdir(parents=True, exist_ok=True)

    # Download PDF
    if not pdf_path.exists():
        try:
            urllib.request.urlretrieve(paper["pdf_url"], str(pdf_path))
            time.sleep(3)  # ArXiv rate limit: 1 req per 3 sec
        except Exception as e:
            print(f"  DL fail: {paper['id']} — {e}")
            return

    # Extract text
    try:
        subprocess.run(
            ["pdftotext", "-layout", str(pdf_path), str(txt_path)],
            check=True, timeout=60,
        )
    except Exception:
        pass

if __name__ == "__main__":
    total = 0
    for query, max_results in QUERIES:
        print(f"  Fetching: {query} (max {max_results})")
        papers = fetch_papers(query, max_results)
        print(f"    Found {len(papers)} papers")
        for p in papers:
            download_and_extract(p)
            total += 1

    print(f"\nTotal papers processed: {total}")
    txt_count = len(list(TEXT_DIR.glob("*.txt")))
    print(f"Text files extracted: {txt_count}")
PYEOF

python3 fetch_papers.py

echo "  ArXiv papers: DONE"

# ================================================================
echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  KILLER COMBO DOWNLOAD COMPLETE                          ║"
echo "║  Run Phase 1 corpus preparation next                     ║"
echo "╚══════════════════════════════════════════════════════════╝"

du -sh "$ZHEN_DIR"/{wikipedia,stackoverflow,github_repos,linux_kernel,arxiv_cs} 2>/dev/null
```

---

## 8. COMPLETE ROADMAP (POST-ALPHA)

The following sources are documented for future phases. Each adds incremental value
on top of the Killer Combo alpha.

### Phase 2: High Value (~300GB)

| Source | Size | Ring | Download Method |
|--------|------|------|-----------------|
| Internet-Drafts (bleeding edge IETF) | ~30GB | 4 | `rsync` from IETF |
| Man pages (all Linux man pages) | ~500MB | 2 | `apt install man-db` + export |
| O'Reilly open books (Rust, BPF Perf Tools) | ~20GB | 2-3 | Selective download |
| Cloud provider docs (AWS, GCP networking) | ~50GB | 4 | Sitemap crawl |
| CVE/NVD database (every CVE published) | ~5GB | 2 | NVD JSON feeds |

### Phase 3: Medium Value (~300GB)

| Source | Size | Ring | Download Method |
|--------|------|------|-----------------|
| Technical blogs (Cloudflare, Netflix, Gregg, Julia Evans, Tailscale, Fly.io) | ~200GB | 3 | RSS + wget |
| Package registry metadata (Go modules, crates.io, nixpkgs) | ~30GB | 3 | Index APIs |
| Conference transcripts (FOSDEM, KubeCon, DEFCON, Black Hat) | ~20GB | 4 | YouTube transcript API |
| Changelog/release notes (Go, Rust, Linux, Docker, K8s) | ~10GB | 3 | GitHub releases API |
| Docker Hub configs (top 1000 Dockerfiles) | ~10GB | 3 | Docker Hub API |

### Phase 4: Nice to Have (~300GB)

| Source | Size | Ring | Download Method |
|--------|------|------|-----------------|
| Multi-language programming books | ~100GB | 4 | free-programming-books non-EN |
| Open textbooks (OpenStax, MIT OCW, Stanford) | ~50GB | 4 | Direct download |
| Hardware datasheets (AMD RDNA3 ISA, PCIe, NVMe specs) | ~20GB | 4 | Vendor sites |
| Test suites (Go stdlib, Rust, Linux Test Project, eBPF tests) | ~30GB | 3 | Git clone |

### Full Roadmap Budget

```
Alpha (Killer Combo):     ~440 GB  ← SHIP THIS FIRST
Rings 1-4 (books+RFCs):  ~660 GB  ← Already speced
Phase 2 additions:        ~106 GB
Phase 3 additions:        ~270 GB
Phase 4 additions:        ~200 GB
──────────────────────────────────
THEORETICAL MAX:          ~1,676 GB of 2 TB
REMAINING:                ~324 GB (checkpoints, experiments, future)
```

---

## 9. WHAT GETS FED TO ZHEN (ALPHA)

### Alpha Corpus Map:

| Source | Type | Ring | Ingestion Method |
|--------|------|------|-----------------|
| ~/tmp/unheaded/ source code | Go/Rust/C | 1 | Direct chunk |
| ~/tmp/unheaded/ docs & specs | Markdown | 1 | Header-aware chunk |
| ~/tmp/unheaded/ skills & lore | Markdown | 1 | Section-aware chunk |
| Your research notes/dumps | Markdown/text | 1 | Direct chunk |
| Go/Rust/Linux/eBPF/networking books | PDF/HTML→text | 2 | Download → extract → chunk |
| Official docs (Go stdlib, aya-rs, NixOS) | HTML→text | 2 | Scrape → extract → chunk |
| CS/SRE/distributed systems/algo books | PDF/HTML→text | 3 | Download → extract → chunk |
| ALL free-programming-books | PDF/HTML→text | 4 | Download → extract → chunk |
| ALL IETF RFCs | Plain text | 4 | Bulk download → chunk |
| **Wikipedia (full EN dump)** | XML→JSON→text | **4** | **wikiextractor → chunk** |
| **Stack Overflow + ServerFault + Unix.SE** | XML→JSONL | **4** | **7z extract → filter score≥3 → chunk** |
| **GitHub repos (Go/Rust/infra)** | Source code | **2-3** | **Shallow clone → chunk** |
| **Linux kernel source + docs** | C/docs | **2** | **Shallow clone → chunk** |
| **ArXiv CS papers (selected)** | PDF→text | **3-4** | **API fetch → pdftotext → chunk** |

### Updated Chunk Estimates:

| Ring | Source | Est. Chunks |
|------|--------|-------------|
| 1 | Unheaded (code + docs + specs + skills) | ~23K |
| 2 | Stack books + GitHub Go/Rust + Linux kernel + official docs | ~80K |
| 3 | General books + GitHub infra + ArXiv papers | ~50K |
| 4 | ALL books + ALL RFCs + Wikipedia + Stack Overflow | ~350K |
| **TOTAL** | | **~503K** |

### Updated FAISS Index:

```
503K chunks × 384 dims × 4 bytes = ~773 MB index
With HNSW overhead: ~2-3 GB total
→ Fits entirely on NVMe with room to spare
```

### Alpha Pipeline:

```
Phase 0:  Build book download manifest from free-programming-books index
Phase 0b: Download + extract text from books (all rings)
Phase 0c: Download killer combo (Wikipedia, SO, GitHub, kernel, ArXiv)  ← NEW
Phase 1:  Multi-source corpus preparation (4-ring chunking)
Phase 2:  Embedding + FAISS indexing (~503K chunks)
Phase 3:  QA generation (ring-weighted: 50/25/15/10)
Phase 4:  RAFT dataset construction (oracle + distractor mixing)
Phase 5:  QLoRA fine-tuning (same hardware budget — GPU doesn't care)
Phase 6:  Merge + quantize
Phase 7:  Evaluate
```

The beauty: **training time STILL barely changes.** 503K chunks vs 138K chunks = bigger FAISS index (~3GB vs ~200MB), slightly longer embedding time (~8hrs vs ~2hrs on CPU). But the QLoRA training stays fixed at ~5K-10K RAFT examples. The GPU trains on curated examples, not raw corpus.

---

*Addendum to unheaded-zhen-raft-spec.md*
*Scientist + Developer fusion, 2026-02-27*
*Alpha focus: THE KILLER COMBO*
