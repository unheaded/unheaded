# UNHEADED-CHAMPION: RAFT Implementation Specification

## Master of the Unheaded Universe — A Domain-Specific LLM

**Authors**: Scientist + Developer Skill Fusion
**Date**: 2026-02-27
**Status**: DESIGN SPEC — Ready for Implementation
**Hardware Target**: AMD RX 7700 XT (12GB VRAM) + 1TB NVMe + 2TB HDD

---

## 1. EXECUTIVE SUMMARY

**The Vision**: Fine-tune a 7B/8B parameter LLM via RAFT (Retrieval-Augmented Fine-Tuning) to become the **undisputed expert** on all things Unheaded — protocol wire formats, Sophia dictionaries, Wotan memory model, eBPF programs, Kingdom lore, architecture decisions, security posture, and 465K+ LOC of Go/Rust/C code.

**Why RAFT over pure RAG or pure Fine-Tuning**:

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| Pure RAG | No training needed, instant updates | Model doesn't understand domain deeply, retrieval errors propagate | Insufficient |
| Pure Fine-Tuning (DSF) | Deep domain knowledge | Stale when docs change, hallucination risk, massive VRAM for 70B | Insufficient |
| **RAFT** | **Best of both: deep domain + retrieval grounding** | Requires dataset construction | **THE WAY** |

**RAFT Core Insight** (Zhang et al., 2024 — arxiv:2403.10131):
Train the model to be an expert at **open-book exams**. Don't just teach it facts — teach it to **find and cite the right answer from retrieved documents while ignoring irrelevant noise**.

---

## 2. THEORETICAL FOUNDATION

### 2.1 From Lewis et al. (2020) RAG Paper to RAFT

The uploaded paper (Lewis et al., "Retrieval-Augmented Generation for Knowledge-Intensive NLP Tasks") establishes the foundation:

**RAG Architecture** (Fig. 1 of the paper):
```
Query x → Retriever p_η(z|x) → top-K documents z₁...zₖ
                                        ↓
                              Generator p_θ(yᵢ|x, z, y₁:ᵢ₋₁) → output y
```

**Two RAG Variants**:
- **RAG-Sequence**: Same document conditions ALL output tokens
- **RAG-Token**: Different document can condition EACH output token

**Key Results from the Paper**:
- RAG achieves SOTA on Natural Questions (44.5 EM), TriviaQA (56.8 EM), WebQuestions (45.5 EM)
- RAG-Token outperforms RAG-Sequence on Jeopardy QGen (diversity)
- RAG more factual than BART (42.7% vs 7.1% in human eval)
- **Critical**: Non-parametric memory (document index) can be **hot-swapped** without retraining

**The Gap**: RAG doesn't teach the model HOW to use retrieved documents. It just concatenates them. RAFT closes this gap.

### 2.2 RAFT: The Training Recipe

**Analogy**: RAG = giving a student a textbook during the exam. RAFT = training that student to ace open-book exams.

**RAFT Dataset Construction** (per Zhang et al., 2024):

For each training example:
```
Input:  Question Q + [D₁, D₂, ..., Dₖ]    (k retrieved documents)
Output: Chain-of-Thought reasoning + Answer A

Where:
- D* = Oracle document (contains the answer)  — present in P% of examples
- Dᵢ = Distractor documents (irrelevant noise) — always present
```

**The P% Oracle Ratio** (critical hyperparameter):
- P% of training examples include the oracle document among the retrieved set
- (100-P)% include ONLY distractor documents (model must answer from parametric memory)
- Recommended: P = 60-80% (teaches both retrieval-grounded AND parametric answering)

**Chain-of-Thought Format**:
```
<answer>
The Monad wire format is defined in draft-bellis-unheaded-protocol-foundation-03.
##begin_quote## The Monad register is exactly 20 bytes, carried in an IPv6
Hop-by-Hop extension header at option type 0x3E ##end_quote##. This means
each packet carries a 20-byte metadata register that includes version (1 byte),
service IDs (2 bytes), hop count, trace ID, QoS class, flow action, circuit
state, flags, latency budget, deployment ring, mesh flags, reserved, and a
CRC-16/CCITT-FALSE checksum. The checksum covers bytes 0x00-0x11 and uses
polynomial 0x1021 with initial value 0xFFFF.
</answer>
```

The `##begin_quote##` / `##end_quote##` markers teach the model to **cite its sources verbatim**.

---

## 3. THE UNHEADED CORPUS

### 3.1 Corpus Inventory

| Category | Files | LOC | RAFT Role |
|----------|-------|-----|-----------|
| Go source (.go) | 858 | 466,967 | Code understanding, API knowledge |
| Rust/eBPF (.rs) | 85 | ~24,000 | Wire format, XDP/TC programs |
| Markdown docs (.md) | 558 | 242,006 | Architecture, lore, decisions |
| Protocol specs (draft-*) | 12 | ~15,000 | Wire format, Sophia, Wotan canonical |
| YAML configs | 134 | 18,104 | Infrastructure patterns |
| Nix configs | 86 | 10,731 | NixOS deployment |
| Shell scripts | 63 | 15,810 | Operational procedures |
| Skills (SKILL.md) | 28+ | ~30,000 | Domain expertise, role definitions |
| C/C++ (doom, libc) | 225 | 75,172 | Low-level integration |
| Wiki pages | 64 | ~20,000 | Cross-referenced documentation |
| ADRs | 15 | ~5,000 | Architectural decisions |
| **TOTAL** | **5,329** | **~857K** | |

### 3.2 Four-Ring Corpus Architecture

All sources map to exactly one ring. Inner rings = higher training weight.

**Ring 1 — Domain Core** (50% of training) — Unheaded-specific:
- Protocol specs: `draft-bellis-unheaded-protocol-foundation-03.md` and later
- `draft-bellis-unheaded-sophia-dictionary-02.md`, `wotan-memory-02.md`
- `CLAUDE.md`, `ARCHITECTURE.md`, `VISION.md`
- Skills: All 28 SKILL.md files
- Go services: `services/*/` (23 services, 467K LOC)
- eBPF programs: `ebpf/` (4 programs, 24K LOC Rust)
- Proto definitions, key packages, ADRs, lore, battle plans, wiki
- NixOS/Docker/LXD configs, scripts, monitoring
- Your research notes and session handoffs

**Ring 2 — Technical Foundation** (25% of training) — stack-relevant books:
- Go books (~31), Rust books (~28), Linux/kernel (~20)
- eBPF/XDP docs, networking/TCP/IPv6 (~15), Docker/containers (~12)
- Security/cryptography (~18), assembly (~8), Nix/NixOS (~5)
- Official docs: Go stdlib, Rust book, aya-rs, NixOS manual, BPF reference

**Ring 3 — General Engineering** (15% of training) — broad CS/SRE:
- Algorithms & data structures, distributed systems, operating systems
- Databases, CI/CD, DevOps, testing, SRE, observability

**Ring 4 — Complete Library** (10% of training) — ALL books + ALL RFCs:
- ALL ~1,636 free-programming-books resources (every language, every subject)
- ALL ~9,000 IETF RFCs (complete standards corpus)
- Official language docs beyond our stack (Python, Java, JS, etc.)

---

## 4. TRAINING PIPELINE ARCHITECTURE

### 4.1 Hardware Layout

```
┌─────────────────────────────────────────────────────────────────┐
│                    RX 7700 XT (12GB VRAM)                       │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  QLoRA Training                                           │  │
│  │  - Base model: 4-bit quantized (~4-5GB)                   │  │
│  │  - LoRA adapters: ~200MB                                  │  │
│  │  - KV cache + activations: ~4-6GB                         │  │
│  │  - Optimizer states: ~500MB                               │  │
│  │  TOTAL: ~10-11GB of 12GB                                  │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    1TB NVMe SSD (Runway)                         │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────┐  │
│  │ OS + Tools   │  │ Active Model │  │ Training Checkpoints  │  │
│  │ ~100GB       │  │ ~5GB (4-bit) │  │ ~20GB (rotating)      │  │
│  └──────────────┘  └──────────────┘  └───────────────────────┘  │
│  ┌──────────────┐  ┌──────────────────────────────────────────┐ │
│  │ FAISS Index  │  │ Active Training Data (preprocessed)      │ │
│  │ ~2-4GB       │  │ ~10GB                                    │ │
│  └──────────────┘  └──────────────────────────────────────────┘ │
│  FREE: ~860GB for NVMe swap / workspace                         │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    2TB HDD (Vault)                               │
│  ┌────────────────┐  ┌────────────────┐  ┌──────────────────┐  │
│  │ Model Library  │  │ Raw Corpus     │  │ All Checkpoints  │  │
│  │ ~200GB         │  │ ~50GB          │  │ ~100GB           │  │
│  └────────────────┘  └────────────────┘  └──────────────────┘  │
│  ┌────────────────┐  ┌────────────────┐  ┌──────────────────┐  │
│  │ Embeddings     │  │ Training Logs  │  │ HF Cache         │  │
│  │ ~20GB          │  │ ~5GB           │  │ ~200GB           │  │
│  └────────────────┘  └────────────────┘  └──────────────────┘  │
│  FREE: ~1.4TB for expansion                                     │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 The Pipeline (7 Phases)

```
Phase 1: CORPUS PREPARATION
  ~/tmp/unheaded/ → chunk → clean → deduplicate → store
                                                      ↓
Phase 2: EMBEDDING + INDEXING
  chunks → embedding model → FAISS HNSW index
                                            ↓
Phase 3: QA GENERATION (synthetic dataset)
  For each chunk: generate Q/A pairs using teacher LLM
                                                        ↓
Phase 4: RAFT DATASET CONSTRUCTION
  Q + [oracle_doc + distractor_docs] → CoT answer
                                                    ↓
Phase 5: QLoRA FINE-TUNING
  Base 7B/8B model + RAFT dataset → adapter weights
                                                      ↓
Phase 6: MERGE + QUANTIZE
  Base + LoRA adapters → merged model → GGUF 4-bit
                                                      ↓
Phase 7: SERVE + EVALUATE
  vLLM/Ollama serve → benchmark against RAG baseline
```

---

## 5. DETAILED PHASE SPECIFICATIONS

### Phase 1: Corpus Preparation

**Goal**: Transform 857K LOC across 5,329 files into clean, overlapping text chunks suitable for embedding and retrieval.

**Chunking Strategy**:

| Content Type | Chunk Size | Overlap | Splitter |
|-------------|-----------|---------|----------|
| Markdown docs | 512 tokens | 64 tokens | Header-aware (## boundaries) |
| Go source | 256 tokens | 32 tokens | Function-boundary aware |
| Rust source | 256 tokens | 32 tokens | Function-boundary aware |
| Protocol specs | 512 tokens | 128 tokens | Section-boundary (RFC format) |
| YAML/Nix configs | 256 tokens | 32 tokens | Block-boundary |
| Skills | 1024 tokens | 128 tokens | Section-boundary (preserve role context) |

**Metadata per Chunk**:
```json
{
  "chunk_id": "uuid",
  "source_file": "services/wotan/internal/core/memory.go",
  "source_type": "go",
  "ring": 2,
  "section": "func (m *MemoryManager) Read()",
  "line_start": 142,
  "line_end": 198,
  "token_count": 256,
  "last_modified": "2026-02-19T00:00:00Z"
}
```

**Implementation** (Python):
```python
# scripts/champion/01_prepare_corpus.py

import os
import json
import hashlib
from pathlib import Path
from typing import List, Dict

# Use langchain's text splitters for intelligent chunking
from langchain.text_splitter import (
    RecursiveCharacterTextSplitter,
    Language,
    MarkdownHeaderTextSplitter,
)

REPO_ROOT = Path(os.path.expanduser("~/tmp/unheaded"))
OUTPUT_DIR = Path("/mnt/hdd/champion/corpus")

# Ring classification
RING_1_PATTERNS = [
    "draft-bellis-*.md",
    "CLAUDE.md",
    "ARCHITECTURE.md",
    "VISION.md",
    "**/SKILL.md",
    "*.skill",
]

RING_1B_PATTERNS = [
    "services/**/*.go",
    "ebpf/**/*.rs",
    "pkg/**/*.go",
    "proto/**/*.proto",
    "cmd/**/*.go",
]

RING_1C_PATTERNS = [
    "docs/**/*.md",
    "wiki/**/*.md",
    "references/**/*.md",
]

RING_1D_PATTERNS = [
    "nixos/**/*.nix",
    "nix/**/*.nix",
    "docker/**/*",
    "lxd/**/*",
    "scripts/**/*.sh",
    "monitoring/**/*",
]

# Language-aware splitters
SPLITTERS = {
    ".go": RecursiveCharacterTextSplitter.from_language(
        language=Language.GO, chunk_size=1024, chunk_overlap=128
    ),
    ".rs": RecursiveCharacterTextSplitter.from_language(
        language=Language.RUST, chunk_size=1024, chunk_overlap=128
    ),
    ".md": MarkdownHeaderTextSplitter(
        headers_to_split_on=[("#", "h1"), ("##", "h2"), ("###", "h3")]
    ),
    ".py": RecursiveCharacterTextSplitter.from_language(
        language=Language.PYTHON, chunk_size=1024, chunk_overlap=128
    ),
    ".nix": RecursiveCharacterTextSplitter(
        chunk_size=1024, chunk_overlap=128
    ),
    "default": RecursiveCharacterTextSplitter(
        chunk_size=1024, chunk_overlap=128
    ),
}

def classify_ring(filepath: str) -> int:
    """Assign ring. All Unheaded repo content = Ring 1."""
    # Everything from ~/tmp/unheaded/ is Ring 1 (domain core)
    # Rings 2-4 are assigned by the multi-source pipeline
    # (see champion-corpus-expansion.md)
    return 1

def chunk_file(filepath: Path) -> List[Dict]:
    """Chunk a single file with metadata."""
    ext = filepath.suffix
    splitter = SPLITTERS.get(ext, SPLITTERS["default"])

    try:
        content = filepath.read_text(encoding="utf-8", errors="replace")
    except Exception:
        return []

    if len(content.strip()) < 50:  # Skip trivially small files
        return []

    chunks = splitter.split_text(content)
    ring = classify_ring(str(filepath))

    return [
        {
            "chunk_id": hashlib.sha256(
                f"{filepath}:{i}:{c[:100]}".encode()
            ).hexdigest()[:16],
            "text": c,
            "source_file": str(filepath.relative_to(REPO_ROOT)),
            "source_type": ext.lstrip("."),
            "ring": ring,
            "chunk_index": i,
            "token_count_approx": len(c.split()),  # rough estimate
        }
        for i, c in enumerate(chunks)
        if len(c.strip()) > 20  # Skip empty chunks
    ]

def prepare_corpus():
    """Walk the repo, chunk everything, output JSONL."""
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    all_chunks = []
    skip_dirs = {".git", "node_modules", "vendor", "__pycache__", ".claude"}

    ALLOWED_EXTS = {
        ".go", ".rs", ".md", ".yaml", ".yml", ".nix", ".sh",
        ".py", ".json", ".toml", ".proto", ".c", ".h",
        ".txt", ".cfg", ".conf", ".ts", ".tsx", ".js",
    }

    for root, dirs, files in os.walk(REPO_ROOT):
        dirs[:] = [d for d in dirs if d not in skip_dirs]
        for f in files:
            fp = Path(root) / f
            if fp.suffix in ALLOWED_EXTS:
                chunks = chunk_file(fp)
                all_chunks.extend(chunks)

    # Write JSONL
    output_path = OUTPUT_DIR / "corpus.jsonl"
    with open(output_path, "w") as fh:
        for chunk in all_chunks:
            fh.write(json.dumps(chunk) + "\n")

    # Stats
    print(f"Total chunks: {len(all_chunks)}")
    for ring in range(1, 5):
        ring_chunks = [c for c in all_chunks if c["ring"] == ring]
        print(f"  Ring {ring}: {len(ring_chunks)} chunks")

    return all_chunks

if __name__ == "__main__":
    prepare_corpus()
```

**Expected Output**: ~15,000-25,000 chunks across all rings.

---

### Phase 2: Embedding + FAISS Indexing

**Embedding Model**: `BAAI/bge-small-en-v1.5` (33M params, 384-dim)
- Fits easily on CPU or GPU alongside training
- Fast inference: ~1000 chunks/sec on CPU
- Quality: Top-10 on MTEB for its size class

**Why not a larger embedding model?**
- Scientist's Fermi estimate: 25K chunks × 384 dims × 4 bytes = ~37MB index
- Even with HNSW overhead: ~200MB total
- Fits entirely on NVMe with room to spare
- Larger models (768/1024 dim) would 3x the index for marginal retrieval gain

**Implementation**:
```python
# scripts/champion/02_build_index.py

import json
import numpy as np
import faiss
from sentence_transformers import SentenceTransformer
from pathlib import Path

CORPUS_PATH = Path("/mnt/hdd/champion/corpus/corpus.jsonl")
INDEX_DIR = Path("/mnt/ssd/champion/index")
EMBED_MODEL = "BAAI/bge-small-en-v1.5"
EMBED_DIM = 384

def build_index():
    INDEX_DIR.mkdir(parents=True, exist_ok=True)

    # Load corpus
    chunks = []
    with open(CORPUS_PATH) as fh:
        for line in fh:
            chunks.append(json.loads(line))

    texts = [c["text"] for c in chunks]
    print(f"Embedding {len(texts)} chunks...")

    # Embed with ring-weighted prefix for retrieval boost
    model = SentenceTransformer(EMBED_MODEL)

    # BGE models use "Represent this sentence: " prefix for docs
    embeddings = model.encode(
        texts,
        batch_size=256,
        show_progress_bar=True,
        normalize_embeddings=True,  # cosine similarity via dot product
    )

    # Build FAISS HNSW index (as used in RAG paper, Section 3)
    # HNSW = Hierarchical Navigable Small World (Malkov & Yashunin, 2016)
    # Same approach as Lewis et al. used with DPR
    index = faiss.IndexHNSWFlat(EMBED_DIM, 32)  # 32 neighbors
    index.hnsw.efConstruction = 200
    index.hnsw.efSearch = 128
    index.add(np.array(embeddings, dtype=np.float32))

    # Save index
    faiss.write_index(index, str(INDEX_DIR / "unheaded.faiss"))

    # Save chunk metadata for retrieval
    with open(INDEX_DIR / "chunks_meta.jsonl", "w") as fh:
        for chunk in chunks:
            fh.write(json.dumps(chunk) + "\n")

    print(f"Index built: {index.ntotal} vectors, {EMBED_DIM} dims")
    print(f"Index size: ~{index.ntotal * EMBED_DIM * 4 / 1e6:.1f} MB")

if __name__ == "__main__":
    build_index()
```

---

### Phase 3: Synthetic QA Generation

**The Critical Step**: Generate high-quality Question-Answer pairs from the Unheaded corpus. This is where we use a **teacher model** (Claude, GPT-4, or a local 70B) to create training data.

**QA Categories**:

| Category | Example Question | Source Ring |
|----------|-----------------|-------------|
| Wire Format | "What is the byte offset of trace_id in the Monad?" | 1 |
| Protocol Logic | "How does CRC-16/CCITT-FALSE verify Monad integrity?" | 1 |
| Architecture | "Which services communicate through Busboy pub/sub?" | 2 |
| Code Understanding | "How does the XDP program handle HBH option parsing?" | 2 |
| Lore/Naming | "Why is the memory service called 'Wotan'?" | 3 |
| Operational | "How do you deploy Unheaded on NixOS?" | 4 |
| Security | "What are the Kingdom Mode bit restrictions at egress?" | 1 |
| Debugging | "What causes HSA_STATUS_ERROR_INVALID_ISA on RX 7700 XT?" | 4 |
| Design Decisions | "Why was CBOR chosen for Sophia dictionary serialization?" | 1-3 |
| Cross-Domain | "How does the eBPF Shield pipeline feed Anamnesis events?" | 1-2 |

**Target**: 5,000-10,000 QA pairs (RAFT paper used ~1,000-10,000 for domain-specific tasks)

**Generation Script**:
```python
# scripts/champion/03_generate_qa.py

"""
QA Generation using a teacher model.

Strategy:
1. For each high-value chunk, generate 2-5 QA pairs
2. For code chunks, generate "what does this do" + "how would you modify" pairs
3. For protocol chunks, generate "what is" + "why" + "how" triples
4. Chain-of-Thought answers MUST cite the source chunk
"""

import json
import os
from pathlib import Path

CORPUS_PATH = Path("/mnt/hdd/champion/corpus/corpus.jsonl")
QA_OUTPUT = Path("/mnt/hdd/champion/qa_pairs.jsonl")

# QA generation prompt template
QA_PROMPT = """You are generating training data for a domain-specific LLM.
Given the following document chunk from the "Unheaded" infrastructure project,
generate {n_questions} question-answer pairs.

RULES:
1. Questions should be specific and answerable from the chunk
2. Answers MUST quote the relevant text using ##begin_quote## and ##end_quote##
3. Answers should include Chain-of-Thought reasoning
4. Mix question types: factual, conceptual, procedural, comparative
5. Answers should be 2-5 sentences with at least one direct quote

DOCUMENT CHUNK:
Source: {source_file}
Type: {source_type}
---
{text}
---

Generate {n_questions} QA pairs in this JSON format:
[
  {{
    "question": "...",
    "answer": "<answer>... ##begin_quote## exact quote ##end_quote## ...</answer>",
    "difficulty": "easy|medium|hard",
    "category": "wire_format|architecture|code|lore|security|operations|debugging"
  }}
]
"""

def generate_qa_for_chunk(chunk: dict, n_questions: int = 3) -> list:
    """Generate QA pairs for a single chunk using teacher model."""
    # NOTE: Replace with your teacher model API call
    # Options:
    #   - Claude API (claude-3-5-sonnet) — highest quality
    #   - Local Ollama with Llama-3-70B — free but slower
    #   - vLLM serving Qwen-2.5-72B — best open-source quality

    prompt = QA_PROMPT.format(
        n_questions=n_questions,
        source_file=chunk["source_file"],
        source_type=chunk["source_type"],
        text=chunk["text"],
    )

    # TODO: Call teacher model here
    # response = teacher_model.generate(prompt)
    # qa_pairs = json.loads(response)
    # return qa_pairs

    return []  # Placeholder

def generate_all_qa():
    """Generate QA pairs for the entire corpus."""
    chunks = []
    with open(CORPUS_PATH) as fh:
        for line in fh:
            chunks.append(json.loads(line))

    # Ring-weighted generation: more QA for inner-ring chunks
    ring_questions = {1: 5, 2: 3, 3: 2, 4: 1}

    all_qa = []
    for chunk in chunks:
        n_q = ring_questions.get(chunk["ring"], 1)
        qa_pairs = generate_qa_for_chunk(chunk, n_q)
        for qa in qa_pairs:
            qa["source_chunk_id"] = chunk["chunk_id"]
            qa["source_file"] = chunk["source_file"]
            qa["source_ring"] = chunk["ring"]
            all_qa.append(qa)

    with open(QA_OUTPUT, "w") as fh:
        for qa in all_qa:
            fh.write(json.dumps(qa) + "\n")

    print(f"Generated {len(all_qa)} QA pairs")

if __name__ == "__main__":
    generate_all_qa()
```

---

### Phase 4: RAFT Dataset Construction

**THE CORE OF RAFT** — This is where the magic happens.

**Dataset Format** (per Zhang et al., 2024):
```
For each QA pair:
  With probability P (e.g., 0.7):
    Input = Q + [D_oracle, D_distractor1, D_distractor2, ..., D_distractorK]
    Label = CoT answer with ##begin_quote## citations from D_oracle

  With probability (1-P):
    Input = Q + [D_distractor1, D_distractor2, ..., D_distractorK]
    Label = CoT answer from parametric memory (no quotes available)
```

**Why the (1-P) case matters**: If EVERY training example has the oracle doc, the model learns to always rely on retrieval. When retrieval fails at inference time (wrong doc retrieved), the model becomes useless. The (1-P) case teaches **parametric fallback** — answering from baked-in knowledge when retrieval fails.

**Implementation**:
```python
# scripts/champion/04_build_raft_dataset.py

"""
RAFT Dataset Builder

Constructs the final training dataset with oracle + distractor mixing.
This is the Zhang et al. (2024) recipe adapted for Unheaded.
"""

import json
import random
import numpy as np
import faiss
from pathlib import Path
from sentence_transformers import SentenceTransformer

# Paths
QA_PATH = Path("/mnt/hdd/champion/qa_pairs.jsonl")
INDEX_PATH = Path("/mnt/ssd/champion/index/unheaded.faiss")
META_PATH = Path("/mnt/ssd/champion/index/chunks_meta.jsonl")
OUTPUT_PATH = Path("/mnt/hdd/champion/raft_dataset.jsonl")

# RAFT Hyperparameters
P_ORACLE = 0.70        # 70% of examples include the oracle document
K_DISTRACTORS = 4      # Number of distractor documents per example
K_TOTAL = 5            # Total documents per example (1 oracle + 4 distractor)
MAX_CONTEXT_TOKENS = 3072  # Max tokens for all documents combined

# Prompt template for RAFT training
RAFT_TEMPLATE = """<|system|>
You are the Unheaded Champion — the master of all knowledge about the Unheaded
Kingdom infrastructure project. Answer questions using the provided documents.
Cite relevant passages using ##begin_quote## and ##end_quote## markers.
If the documents don't contain the answer, use your knowledge but note the uncertainty.
<|end|>
<|user|>
## Question
{question}

## Documents
{documents}
<|end|>
<|assistant|>
{answer}
<|end|>
"""

def load_index_and_meta():
    """Load FAISS index and chunk metadata."""
    index = faiss.read_index(str(INDEX_PATH))
    chunks = []
    with open(META_PATH) as fh:
        for line in fh:
            chunks.append(json.loads(line))
    return index, chunks

def get_distractors(
    query_embedding: np.ndarray,
    oracle_chunk_id: str,
    index: faiss.Index,
    chunks: list,
    k: int = K_DISTRACTORS
) -> list:
    """
    Retrieve K hard-negative distractor documents.

    Hard negatives = documents that are SIMILAR to the query
    but DON'T contain the answer. This forces the model to
    learn precise discrimination, not just topic matching.
    """
    # Retrieve more than we need, then filter out the oracle
    scores, indices = index.search(
        query_embedding.reshape(1, -1), k * 3
    )

    distractors = []
    for idx in indices[0]:
        if idx < 0 or idx >= len(chunks):
            continue
        if chunks[idx]["chunk_id"] == oracle_chunk_id:
            continue  # Skip the oracle document
        distractors.append(chunks[idx])
        if len(distractors) >= k:
            break

    return distractors

def format_documents(docs: list, include_oracle: bool, oracle_doc: dict = None) -> str:
    """Format documents for the prompt."""
    formatted = []

    if include_oracle and oracle_doc:
        all_docs = [oracle_doc] + docs
    else:
        all_docs = docs

    # Shuffle so oracle position isn't predictable
    random.shuffle(all_docs)

    for i, doc in enumerate(all_docs):
        formatted.append(
            f"### Document {i+1} [{doc['source_file']}]\n{doc['text']}"
        )

    return "\n\n".join(formatted)

def build_raft_dataset():
    """Build the complete RAFT training dataset."""
    # Load components
    index, chunks = load_index_and_meta()
    embed_model = SentenceTransformer("BAAI/bge-small-en-v1.5")

    qa_pairs = []
    with open(QA_PATH) as fh:
        for line in fh:
            qa_pairs.append(json.loads(line))

    # Build chunk ID → chunk lookup
    chunk_by_id = {c["chunk_id"]: c for c in chunks}

    raft_examples = []

    for qa in qa_pairs:
        oracle_chunk = chunk_by_id.get(qa["source_chunk_id"])
        if not oracle_chunk:
            continue

        # Embed the question for distractor retrieval
        q_emb = embed_model.encode(
            qa["question"], normalize_embeddings=True
        )

        # Get hard-negative distractors
        distractors = get_distractors(
            q_emb, oracle_chunk["chunk_id"], index, chunks
        )

        if len(distractors) < K_DISTRACTORS:
            continue  # Skip if not enough distractors

        # RAFT mixing: P% include oracle, (1-P)% don't
        include_oracle = random.random() < P_ORACLE

        documents = format_documents(
            distractors[:K_DISTRACTORS],
            include_oracle=include_oracle,
            oracle_doc=oracle_chunk,
        )

        # Format the full training example
        example = RAFT_TEMPLATE.format(
            question=qa["question"],
            documents=documents,
            answer=qa["answer"],
        )

        raft_examples.append({
            "text": example,
            "has_oracle": include_oracle,
            "category": qa.get("category", "general"),
            "difficulty": qa.get("difficulty", "medium"),
            "source_ring": qa.get("source_ring", 4),
        })

    # Shuffle
    random.shuffle(raft_examples)

    # Split: 90% train, 5% validation, 5% test
    n = len(raft_examples)
    train = raft_examples[:int(n * 0.90)]
    val = raft_examples[int(n * 0.90):int(n * 0.95)]
    test = raft_examples[int(n * 0.95):]

    # Write splits
    for split_name, split_data in [("train", train), ("val", val), ("test", test)]:
        path = OUTPUT_PATH.parent / f"raft_{split_name}.jsonl"
        with open(path, "w") as fh:
            for ex in split_data:
                fh.write(json.dumps(ex) + "\n")
        print(f"{split_name}: {len(split_data)} examples")

    # Stats
    oracle_pct = sum(1 for e in train if e["has_oracle"]) / len(train) * 100
    print(f"\nOracle ratio: {oracle_pct:.1f}% (target: {P_ORACLE*100:.0f}%)")

    by_cat = {}
    for e in train:
        cat = e["category"]
        by_cat[cat] = by_cat.get(cat, 0) + 1
    print("\nCategory distribution:")
    for cat, count in sorted(by_cat.items(), key=lambda x: -x[1]):
        print(f"  {cat}: {count}")

if __name__ == "__main__":
    build_raft_dataset()
```

---

### Phase 5: QLoRA Fine-Tuning

**Base Model Selection**:

| Model | Params | 4-bit Size | VRAM (QLoRA) | Quality | Verdict |
|-------|--------|-----------|--------------|---------|---------|
| Llama-3.1-8B-Instruct | 8B | ~4.5GB | ~10GB | Excellent | **PRIMARY** |
| Qwen-2.5-7B-Instruct | 7B | ~4GB | ~9GB | Excellent | BACKUP |
| Mistral-7B-v0.3 | 7B | ~4GB | ~9GB | Good | ALTERNATE |
| CodeLlama-7B-Instruct | 7B | ~4GB | ~9GB | Code-focused | FOR CODE-HEAVY |

**Recommendation**: **Llama-3.1-8B-Instruct** — best instruction-following, broadest pre-training, Meta's RAFT blog specifically validates this family.

**QLoRA Configuration**:
```python
# scripts/champion/05_train_qlora.py

"""
QLoRA RAFT Training on RX 7700 XT (12GB VRAM)

Using Unsloth for 2x speedup on AMD ROCm via Triton kernels.
Falls back to HuggingFace PEFT if Unsloth ROCm not available.
"""

import os
os.environ["HSA_OVERRIDE_GFX_VERSION"] = "11.0.1"
os.environ["PYTORCH_ROCM_ARCH"] = "gfx1101"

import torch
from datasets import load_dataset
from transformers import (
    AutoModelForCausalLM,
    AutoTokenizer,
    BitsAndBytesConfig,
    TrainingArguments,
)
from peft import LoraConfig, get_peft_model, prepare_model_for_kbit_training
from trl import SFTTrainer

# ============================================================================
# MODEL CONFIGURATION
# ============================================================================

MODEL_ID = "meta-llama/Meta-Llama-3.1-8B-Instruct"
OUTPUT_DIR = "/mnt/ssd/champion/checkpoints"
FINAL_DIR = "/mnt/hdd/champion/models/unheaded-champion-v1"

# 4-bit quantization config
bnb_config = BitsAndBytesConfig(
    load_in_4bit=True,
    bnb_4bit_quant_type="nf4",           # Normal Float 4
    bnb_4bit_compute_dtype=torch.bfloat16, # Compute in bf16
    bnb_4bit_use_double_quant=True,       # Nested quantization
)

# LoRA config — targeting attention + MLP layers
lora_config = LoraConfig(
    r=32,                    # LoRA rank (32 = good quality/VRAM tradeoff)
    lora_alpha=64,           # Alpha = 2*r (standard scaling)
    target_modules=[         # All attention + gate/up/down projections
        "q_proj", "k_proj", "v_proj", "o_proj",
        "gate_proj", "up_proj", "down_proj",
    ],
    lora_dropout=0.05,       # Light regularization
    bias="none",
    task_type="CAUSAL_LM",
)

# ============================================================================
# TRAINING HYPERPARAMETERS
# ============================================================================
# Per RAFT paper recommendations:
# - LR at least 1 magnitude lower than pre-training
# - No more than 3 epochs for large datasets
# - Large effective batch size via gradient accumulation

training_args = TrainingArguments(
    output_dir=OUTPUT_DIR,

    # Batch size: micro_batch=1 × grad_accum=16 = effective 16
    per_device_train_batch_size=1,
    gradient_accumulation_steps=16,

    # Learning rate
    learning_rate=2e-5,             # Conservative for RAFT
    lr_scheduler_type="cosine",
    warmup_ratio=0.03,

    # Duration
    num_train_epochs=3,
    max_steps=-1,                   # Use epochs, not steps

    # Precision
    bf16=True,                      # ROCm supports bf16
    fp16=False,

    # Logging
    logging_steps=10,
    eval_strategy="steps",
    eval_steps=100,
    save_strategy="steps",
    save_steps=200,
    save_total_limit=5,            # Keep last 5 checkpoints on NVMe

    # Memory optimization
    gradient_checkpointing=True,    # Trade compute for VRAM
    optim="paged_adamw_8bit",       # 8-bit optimizer saves ~2GB
    max_grad_norm=0.3,

    # Monitoring
    report_to="tensorboard",
    logging_dir="/mnt/hdd/champion/logs",

    # Reproducibility
    seed=42,
    data_seed=42,
)

# ============================================================================
# TRAINING LOOP
# ============================================================================

def train():
    print("Loading tokenizer...")
    tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)
    tokenizer.pad_token = tokenizer.eos_token
    tokenizer.padding_side = "right"

    print("Loading model (4-bit quantized)...")
    model = AutoModelForCausalLM.from_pretrained(
        MODEL_ID,
        quantization_config=bnb_config,
        device_map="auto",
        torch_dtype=torch.bfloat16,
    )
    model = prepare_model_for_kbit_training(model)
    model = get_peft_model(model, lora_config)

    trainable = sum(p.numel() for p in model.parameters() if p.requires_grad)
    total = sum(p.numel() for p in model.parameters())
    print(f"Trainable: {trainable:,} / {total:,} ({trainable/total*100:.2f}%)")

    print("Loading RAFT dataset...")
    dataset = load_dataset("json", data_files={
        "train": "/mnt/hdd/champion/raft_train.jsonl",
        "validation": "/mnt/hdd/champion/raft_val.jsonl",
    })

    print("Starting training...")
    trainer = SFTTrainer(
        model=model,
        train_dataset=dataset["train"],
        eval_dataset=dataset["validation"],
        args=training_args,
        tokenizer=tokenizer,
        dataset_text_field="text",
        max_seq_length=4096,        # Context window for RAFT
        packing=True,               # Pack short examples together
    )

    trainer.train()

    # Save final adapter
    model.save_pretrained(FINAL_DIR)
    tokenizer.save_pretrained(FINAL_DIR)
    print(f"Adapter saved to {FINAL_DIR}")

if __name__ == "__main__":
    train()
```

**VRAM Budget** (Scientist's calculation):
```
Base model (4-bit):           ~4.5 GB
LoRA adapters (r=32):        ~0.2 GB
Optimizer states (8-bit):     ~0.4 GB
Activations (grad ckpt):     ~3.0 GB
KV cache (seq_len=4096):     ~2.5 GB
Overhead:                     ~1.0 GB
──────────────────────────────────
TOTAL:                        ~11.6 GB / 12 GB
```

Tight but viable. Gradient checkpointing is the key — trades ~30% compute speed for ~40% VRAM savings.

---

### Phase 6: Merge + Quantize

```bash
#!/bin/bash
# scripts/champion/06_merge_and_quantize.sh

set -euo pipefail

BASE_MODEL="meta-llama/Meta-Llama-3.1-8B-Instruct"
ADAPTER_DIR="/mnt/hdd/champion/models/unheaded-champion-v1"
MERGED_DIR="/mnt/hdd/champion/models/unheaded-champion-v1-merged"
GGUF_DIR="/mnt/ssd/champion/models"

echo "=== Phase 6a: Merge LoRA adapters into base model ==="
python3 -c "
from peft import PeftModel
from transformers import AutoModelForCausalLM, AutoTokenizer
import torch

print('Loading base model...')
base = AutoModelForCausalLM.from_pretrained('$BASE_MODEL', torch_dtype=torch.float16)
tokenizer = AutoTokenizer.from_pretrained('$BASE_MODEL')

print('Loading adapter...')
model = PeftModel.from_pretrained(base, '$ADAPTER_DIR')

print('Merging...')
merged = model.merge_and_unload()

print('Saving merged model...')
merged.save_pretrained('$MERGED_DIR')
tokenizer.save_pretrained('$MERGED_DIR')
print('Done!')
"

echo "=== Phase 6b: Convert to GGUF (for Ollama/llama.cpp) ==="
# Using llama.cpp's convert script
python3 llama.cpp/convert_hf_to_gguf.py \
    "$MERGED_DIR" \
    --outfile "$GGUF_DIR/unheaded-champion-v1-Q4_K_M.gguf" \
    --outtype q4_k_m

echo "=== Phase 6c: Create Ollama Modelfile ==="
cat > "$GGUF_DIR/Modelfile" << 'MODELFILE'
FROM ./unheaded-champion-v1-Q4_K_M.gguf

PARAMETER temperature 0.3
PARAMETER top_p 0.9
PARAMETER repeat_penalty 1.1
PARAMETER num_ctx 4096

SYSTEM """You are the Unheaded Champion — the master of all knowledge about the
Unheaded Kingdom infrastructure project. You are an expert on the Monad wire format,
Sophia dictionaries, Wotan memory model, eBPF Shield pipeline, all 23 services,
NixOS deployment, Kingdom lore, and the complete 465K+ LOC codebase.

When answering, cite relevant sources using ##begin_quote## and ##end_quote## markers.
Be precise, technical, and thorough. If you're uncertain, say so explicitly."""
MODELFILE

echo "=== Phase 6d: Register with Ollama ==="
cd "$GGUF_DIR"
ollama create unheaded-champion -f Modelfile

echo "=== DONE ==="
echo "Run: ollama run unheaded-champion"
echo "Or via vLLM: vllm serve $MERGED_DIR --dtype float16 --max-model-len 4096"
```

---

### Phase 7: Serve + Evaluate

**Serving Options**:

| Method | Speed | Use Case |
|--------|-------|----------|
| Ollama | ~20-40 t/s (Q4) | Interactive chat, quick queries |
| vLLM | ~15-30 t/s (FP16) | API serving, batch inference |
| llama.cpp | ~25-45 t/s (Q4) | Lightweight, CPU offload |

**Evaluation Benchmark** — The Unheaded Exam:

```python
# scripts/champion/07_evaluate.py

"""
Evaluate unheaded-champion against baselines:
1. Base Llama-3.1-8B (no training, no RAG)
2. Base + RAG (retrieval only, no fine-tuning)
3. Base + DSF (fine-tuning only, no retrieval)
4. Unheaded Champion (RAFT = fine-tuning + retrieval)
"""

EVAL_QUESTIONS = [
    # Ring 1 — Wire Format (must know exactly)
    {
        "q": "What is the byte offset of the checksum field in the Monad register?",
        "a": "0x12 (bytes 18-19)",
        "category": "wire_format",
    },
    {
        "q": "What polynomial does CRC-16/CCITT-FALSE use?",
        "a": "0x1021 with initial value 0xFFFF",
        "category": "wire_format",
    },
    {
        "q": "What is the HBH option type for the Monad extension header?",
        "a": "0x3E (001xxxxx format, act=00, chg=1)",
        "category": "wire_format",
    },

    # Ring 1 — Architecture (must understand relationships)
    {
        "q": "How many cache levels does the Wotan memory model define?",
        "a": "Five: L0 (Monad/wire), L1 (BPF map), L2 (ring buffer), L3 (WAL), L4 (Sophia dicts)",
        "category": "architecture",
    },
    {
        "q": "What topic naming pattern does Busboy/Wotan use?",
        "a": "{service}.{component}.{detail}",
        "category": "architecture",
    },

    # Ring 1 — Lore (must know the mythology)
    {
        "q": "What are the three pillars of Unheaded naming conventions?",
        "a": "Gnostic Cosmology, Chronicles of Amber, Medieval Armory",
        "category": "lore",
    },
    {
        "q": "What is the Ascension of Busboy?",
        "a": "Busboy's evolution to become Wotan (the memory/wisdom service)",
        "category": "lore",
    },

    # Ring 1 — Operational (must know how to deploy)
    {
        "q": "What environment variable forces ROCm to recognize the RX 7700 XT?",
        "a": "HSA_OVERRIDE_GFX_VERSION=11.0.1",
        "category": "operations",
    },

    # Cross-domain: Security + Protocol
    {
        "q": "What happens to Kingdom Mode ERS bits at egress?",
        "a": "They are zeroed (cleared) to prevent leaking internal state",
        "category": "security",
    },
    {
        "q": "What serialization format does Sophia use for dictionary distribution?",
        "a": "CBOR (RFC 8949) for Wotan topic distribution",
        "category": "protocol",
    },
]

# Scoring: Exact match, F1 (token overlap), and human-eval factuality
```

---

## 6. RAFT vs RAG vs DSF: SCIENTIST'S ANALYSIS

### Hypothesis

**H1**: RAFT-trained unheaded-champion will score ≥20% higher than pure RAG on Unheaded-specific questions, particularly on wire format and protocol questions requiring precise byte-level knowledge.

**H2**: RAFT will show the largest improvement gap on "cross-domain" questions that require synthesizing information from multiple sources (e.g., "How does Shield feed Anamnesis?").

**H3**: The (1-P) training examples (no oracle doc) will prevent catastrophic failure when retrieval returns irrelevant documents.

### Predictions

| Metric | Base (no RAG) | RAG Only | DSF Only | RAFT (Champion) |
|--------|---------------|----------|----------|-----------------|
| Wire Format Exact Match | <5% | ~40% | ~60% | **~85%** |
| Architecture F1 | ~10% | ~50% | ~55% | **~80%** |
| Lore/Naming Accuracy | ~0% | ~35% | ~70% | **~85%** |
| Cross-Domain F1 | ~5% | ~30% | ~40% | **~75%** |
| Retrieval Failure Recovery | N/A | ~0% (fails) | ~60% | **~55%** |

### Experiment Design

1. Hold out 500 QA pairs for evaluation (never seen during training)
2. Test all 4 configurations on identical hardware
3. Measure: Exact Match, Token F1, Citation Accuracy, Latency
4. Statistical significance: paired t-test, N=500, α=0.05

---

## 7. IMPLEMENTATION TIMELINE

| Phase | Duration | Compute | Storage |
|-------|----------|---------|---------|
| 1. Corpus Prep | ~30 min | CPU | ~500MB (JSONL) |
| 2. Embedding + Index | ~1 hr | CPU (GPU optional) | ~200MB (FAISS) |
| 3. QA Generation | ~4-8 hrs | Teacher LLM (API or local) | ~100MB (QA pairs) |
| 4. RAFT Dataset | ~1 hr | CPU + GPU (embedding) | ~2GB (RAFT JSONL) |
| 5. QLoRA Training | ~6-12 hrs | GPU (RX 7700 XT) | ~20GB (checkpoints) |
| 6. Merge + Quantize | ~30 min | CPU | ~10GB (merged + GGUF) |
| 7. Evaluate | ~1 hr | GPU | ~1GB (results) |
| **TOTAL** | **~15-24 hrs** | | **~34GB** |

---

## 8. MASTER SCRIPT

```bash
#!/bin/bash
# scripts/champion/train-champion.sh
# Master orchestrator for the full RAFT pipeline

set -euo pipefail

export HSA_OVERRIDE_GFX_VERSION=11.0.1
export PYTORCH_ROCM_ARCH=gfx1101
export HF_HOME="/mnt/hdd/huggingface"
export HF_HUB_ENABLE_HF_TRANSFER=1

CHAMPION_DIR="/mnt/hdd/champion"
SCRIPTS_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "╔══════════════════════════════════════════════════════════╗"
echo "║  UNHEADED CHAMPION — RAFT Training Pipeline             ║"
echo "║  Target: Llama-3.1-8B-Instruct + QLoRA + RAFT           ║"
echo "║  GPU: AMD RX 7700 XT (12GB) — gfx1101                   ║"
echo "╚══════════════════════════════════════════════════════════╝"

mkdir -p "$CHAMPION_DIR"/{corpus,index,models,logs}
mkdir -p /mnt/ssd/champion/{index,models,checkpoints}

echo ""
echo "=== PHASE 1/7: Corpus Preparation ==="
python3 "$SCRIPTS_DIR/01_prepare_corpus.py"

echo ""
echo "=== PHASE 2/7: Embedding + FAISS Indexing ==="
python3 "$SCRIPTS_DIR/02_build_index.py"

echo ""
echo "=== PHASE 3/7: Synthetic QA Generation ==="
echo "[MANUAL STEP] Run with teacher model access"
echo "python3 $SCRIPTS_DIR/03_generate_qa.py"
echo "Press ENTER when QA generation is complete..."
read -r

echo ""
echo "=== PHASE 4/7: RAFT Dataset Construction ==="
python3 "$SCRIPTS_DIR/04_build_raft_dataset.py"

echo ""
echo "=== PHASE 5/7: QLoRA Fine-Tuning ==="
python3 "$SCRIPTS_DIR/05_train_qlora.py"

echo ""
echo "=== PHASE 6/7: Merge + Quantize ==="
bash "$SCRIPTS_DIR/06_merge_and_quantize.sh"

echo ""
echo "=== PHASE 7/7: Evaluate ==="
python3 "$SCRIPTS_DIR/07_evaluate.py"

echo ""
echo "╔══════════════════════════════════════════════════════════╗"
echo "║  CHAMPION TRAINING COMPLETE                              ║"
echo "║                                                          ║"
echo "║  Run: ollama run unheaded-champion                       ║"
echo "║  API: vllm serve /mnt/hdd/champion/models/merged         ║"
echo "╚══════════════════════════════════════════════════════════╝"
```

---

## 9. RISK ANALYSIS (Scientist's Assessment)

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| VRAM OOM during training | Medium | Training crashes | Reduce seq_len to 2048, reduce r to 16 |
| QA generation quality too low | Medium | Poor training signal | Use Claude API as teacher, manual review 100 samples |
| Retrieval collapse (Appendix H of RAG paper) | Low | Model ignores retrieved docs | Monitor retrieval loss separately, verify P% ratio |
| SSD wear from checkpointing | Low | Drive degradation | Checkpoint to HDD, only active model on NVMe |
| Model hallucination on wire format | Medium | Wrong byte offsets | Include exact-match wire format questions in eval |
| ROCm compatibility issues | Medium | Training fails | Test with small run first, fallback to CPU offload |

---

## 10. FUTURE EVOLUTION

### v2: Live Index Updates
Per Lewis et al. (2020) Section 4.5 "Index hot-swapping": RAG's non-parametric memory can be replaced without retraining. As Unheaded evolves (new commits, new specs), rebuild the FAISS index and the Champion instantly knows the new code — no retraining needed.

### v3: Multi-Modal Champion
Add code-specific embeddings (CodeBERT/StarEncoder) alongside text embeddings for better code retrieval.

### v4: Agent Mode
Connect Champion to tools: `git log`, `grep`, `go test`, `cargo test`. RAFT-trained model + tool use = autonomous Unheaded developer agent.

### v5: Distributed Training
When you upgrade to 24GB+ GPU, retrain at higher rank (r=64/128) with larger context (8K/16K tokens) for even deeper domain understanding.

---

## APPENDIX A: Dependencies

```bash
# Python packages (pip install --break-system-packages)
pip install torch torchvision torchaudio --index-url https://download.pytorch.org/whl/rocm6.1
pip install transformers datasets peft trl accelerate
pip install bitsandbytes  # ROCm fork: pip install bitsandbytes-rocm
pip install sentence-transformers faiss-cpu  # or faiss-gpu for ROCm
pip install langchain langchain-text-splitters
pip install hf_transfer
pip install tensorboard
pip install unsloth  # Optional: 2x speedup if ROCm supported

# System packages
sudo apt install -y python3-venv python3-pip git-lfs
```

## APPENDIX B: The Uploaded Paper's Key Equations

From Lewis et al. (2020), the two RAG formulations that RAFT builds upon:

**RAG-Sequence** (same doc for all tokens):
```
p_RAG-Seq(y|x) ≈ Σ_{z ∈ top-k} p_η(z|x) Π_i p_θ(y_i|x, z, y_{1:i-1})
```

**RAG-Token** (different doc per token):
```
p_RAG-Tok(y|x) ≈ Π_i Σ_{z ∈ top-k} p_η(z|x) p_θ(y_i|x, z, y_{1:i-1})
```

RAFT adds: **Train p_θ to discriminate oracle from distractor documents** via the Chain-of-Thought + citation training signal. The retriever p_η can remain frozen (simpler) or be jointly trained (Lewis et al. showed both work).

---

*Generated by Scientist + Developer skill fusion, 2026-02-27*
*For the Unheaded Kingdom — Master of the Universe awaits training.*
