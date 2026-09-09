#!/usr/bin/env python3
"""Ring 2-4 Corpus Expansion Pipeline for Zhen AI.

Processes all completed Ring 2-4 data sources into chunked JSONL,
then builds a combined FAISS index merging Ring 1 + Ring 2-4.

Sources:
  Ring 2: GitHub repos, Linux kernel (BPF subset), official docs, NIST PQC
  Ring 3: ArXiv papers, seminal papers, blog posts (Cloudflare, Gregg, jvns)
  Ring 4: IETF RFCs

Usage:
  source ~/.venv/zhen/bin/activate
  python 06_expand_corpus.py
"""

import html.parser
import json
import os
import subprocess
import time
import uuid

from langchain_text_splitters import RecursiveCharacterTextSplitter

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------
ZHEN_ROOT = "/mnt/hdd/zhen"
RAFT_DIR = os.path.expanduser("~/tmp/unheaded/raft")
CORPUS_DIR = os.path.join(RAFT_DIR, "corpus")
INDEX_DIR = os.path.join(RAFT_DIR, "index")
OUTPUT_FILE = os.path.join(CORPUS_DIR, "ring234.jsonl")
RING1_CORPUS = os.path.join(CORPUS_DIR, "ring1.jsonl")

COMBINED_INDEX = os.path.join(INDEX_DIR, "combined.index")
COMBINED_IDS = os.path.join(INDEX_DIR, "combined_ids.json")
COMBINED_CORPUS = os.path.join(INDEX_DIR, "combined_corpus.json")

# Intermediate checkpoint
CHECKPOINT_FILE = os.path.join(CORPUS_DIR, "ring234_checkpoint.json")

MAX_FILE_SIZE = 2 * 1024 * 1024  # 2 MB

# ---------------------------------------------------------------------------
# HTML stripper using html.parser (no BeautifulSoup dependency)
# ---------------------------------------------------------------------------
class HTMLStripper(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.reset()
        self.fed = []
        self._skip = False

    def handle_starttag(self, tag, attrs):
        if tag in ("script", "style", "noscript"):
            self._skip = True

    def handle_endtag(self, tag):
        if tag in ("script", "style", "noscript"):
            self._skip = False

    def handle_data(self, data):
        if not self._skip:
            self.fed.append(data)

    def get_text(self):
        return " ".join(self.fed)


def strip_html(text: str) -> str:
    s = HTMLStripper()
    try:
        s.feed(text)
        return s.get_text()
    except Exception:
        # Fallback: crude regex strip
        import re
        return re.sub(r"<[^>]+>", " ", text)


def estimate_tokens(text: str) -> int:
    return len(text) // 4


def pdf_to_text(path: str) -> str:
    """Convert PDF to text using pdftotext."""
    try:
        result = subprocess.run(
            ["pdftotext", "-layout", path, "-"],
            capture_output=True, timeout=60
        )
        return result.stdout.decode("utf-8", errors="ignore")
    except Exception as e:
        print(f"  WARN: pdftotext failed for {path}: {e}")
        return ""


def read_file(path: str) -> str:
    """Read text file with encoding error handling."""
    try:
        with open(path, "r", encoding="utf-8", errors="ignore") as f:
            return f.read()
    except (OSError, UnicodeDecodeError):
        return ""


def is_binary(content: str) -> bool:
    return "\x00" in content[:4096]


# ---------------------------------------------------------------------------
# Source processors
# ---------------------------------------------------------------------------

def process_github_repos(splitter):
    """Walk GitHub repos for .go, .rs, .c, .h, .md files. Ring 2."""
    base = os.path.join(ZHEN_ROOT, "github_repos")
    if not os.path.isdir(base):
        print("SKIP: github_repos not yet available")
        return []

    skip_dirs = {".git", "vendor", "node_modules", "__pycache__", "testdata",
                 "third_party", "examples", "hack", "staging"}
    extensions = {".go", ".rs", ".c", ".h", ".md"}
    chunks = []
    files_processed = 0

    print("  Processing github_repos...")
    for dirpath, dirnames, filenames in os.walk(base):
        dirnames[:] = [d for d in dirnames if d not in skip_dirs]
        for fname in filenames:
            ext = os.path.splitext(fname)[1].lower()
            if ext not in extensions:
                continue
            fpath = os.path.join(dirpath, fname)
            try:
                if os.path.getsize(fpath) > MAX_FILE_SIZE:
                    continue
                if os.path.getsize(fpath) == 0:
                    continue
            except OSError:
                continue

            content = read_file(fpath)
            if not content.strip() or is_binary(content):
                continue

            rel = os.path.relpath(fpath, ZHEN_ROOT)
            file_type = "code" if ext in (".go", ".rs", ".c", ".h") else "doc"
            for i, chunk_text in enumerate(splitter.split_text(content)):
                chunks.append({
                    "id": str(uuid.uuid4()),
                    "source": rel,
                    "ring": 2,
                    "type": file_type,
                    "content": chunk_text,
                    "tokens": estimate_tokens(chunk_text),
                })
            files_processed += 1
            if files_processed % 500 == 0:
                print(f"    github_repos: {files_processed} files, {len(chunks)} chunks so far")

    print(f"    github_repos: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_linux_kernel(splitter):
    """Walk kernel BPF-related dirs only. Ring 2."""
    kernel_root = os.path.join(ZHEN_ROOT, "linux_kernel", "linux")
    if not os.path.isdir(kernel_root):
        print("SKIP: linux_kernel not yet available")
        return []

    # Only index BPF/networking-relevant subtrees
    target_dirs = [
        os.path.join(kernel_root, "kernel", "bpf"),
        os.path.join(kernel_root, "net"),
        os.path.join(kernel_root, "Documentation", "bpf"),
    ]
    # Also grab include/linux/bpf* headers
    include_dir = os.path.join(kernel_root, "include", "linux")

    extensions = {".c", ".h", ".md", ".rst", ".txt"}
    skip_dirs = {".git"}
    chunks = []
    files_processed = 0

    print("  Processing linux_kernel (BPF subset)...")

    # Process target directories
    for tdir in target_dirs:
        if not os.path.isdir(tdir):
            continue
        for dirpath, dirnames, filenames in os.walk(tdir):
            dirnames[:] = [d for d in dirnames if d not in skip_dirs]
            for fname in filenames:
                ext = os.path.splitext(fname)[1].lower()
                if ext not in extensions:
                    continue
                fpath = os.path.join(dirpath, fname)
                try:
                    if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                        continue
                except OSError:
                    continue
                content = read_file(fpath)
                if not content.strip() or is_binary(content):
                    continue
                rel = os.path.relpath(fpath, ZHEN_ROOT)
                file_type = "code" if ext in (".c", ".h") else "doc"
                for chunk_text in splitter.split_text(content):
                    chunks.append({
                        "id": str(uuid.uuid4()),
                        "source": rel,
                        "ring": 2,
                        "type": file_type,
                        "content": chunk_text,
                        "tokens": estimate_tokens(chunk_text),
                    })
                files_processed += 1

    # Process include/linux/bpf* headers
    if os.path.isdir(include_dir):
        for fname in os.listdir(include_dir):
            if not fname.startswith("bpf"):
                continue
            ext = os.path.splitext(fname)[1].lower()
            if ext not in extensions:
                continue
            fpath = os.path.join(include_dir, fname)
            try:
                if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                    continue
            except OSError:
                continue
            content = read_file(fpath)
            if not content.strip() or is_binary(content):
                continue
            rel = os.path.relpath(fpath, ZHEN_ROOT)
            for chunk_text in splitter.split_text(content):
                chunks.append({
                    "id": str(uuid.uuid4()),
                    "source": rel,
                    "ring": 2,
                    "type": "code",
                    "content": chunk_text,
                    "tokens": estimate_tokens(chunk_text),
                })
            files_processed += 1

    print(f"    linux_kernel: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_rfcs(splitter):
    """Process IETF RFCs (.txt files). Ring 4."""
    base = os.path.join(ZHEN_ROOT, "rfcs")
    if not os.path.isdir(base):
        print("SKIP: rfcs not yet available")
        return []

    chunks = []
    files_processed = 0
    print("  Processing rfcs...")

    for fname in sorted(os.listdir(base)):
        if not fname.endswith(".txt"):
            continue
        # Skip index/metadata files
        if fname in ("bcp-index.txt", "bcp-ref.txt", "fyi-index.txt",
                      "ien-index.txt", "RFCs_for_errata.txt"):
            continue
        fpath = os.path.join(base, fname)
        try:
            if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                continue
        except OSError:
            continue
        content = read_file(fpath)
        if not content.strip():
            continue
        rel = os.path.relpath(fpath, ZHEN_ROOT)
        for chunk_text in splitter.split_text(content):
            chunks.append({
                "id": str(uuid.uuid4()),
                "source": rel,
                "ring": 4,
                "type": "rfc",
                "content": chunk_text,
                "tokens": estimate_tokens(chunk_text),
            })
        files_processed += 1
        if files_processed % 1000 == 0:
            print(f"    rfcs: {files_processed} files, {len(chunks)} chunks so far")

    print(f"    rfcs: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_arxiv(splitter):
    """Process ArXiv paper text files. Ring 3."""
    base = os.path.join(ZHEN_ROOT, "arxiv_cs", "text")
    if not os.path.isdir(base):
        print("SKIP: arxiv_cs not yet available")
        return []

    chunks = []
    files_processed = 0
    print("  Processing arxiv_cs...")

    for fname in sorted(os.listdir(base)):
        if not fname.endswith(".txt"):
            continue
        fpath = os.path.join(base, fname)
        try:
            if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                continue
        except OSError:
            continue
        content = read_file(fpath)
        if not content.strip():
            continue
        rel = os.path.relpath(fpath, ZHEN_ROOT)
        for chunk_text in splitter.split_text(content):
            chunks.append({
                "id": str(uuid.uuid4()),
                "source": rel,
                "ring": 3,
                "type": "paper",
                "content": chunk_text,
                "tokens": estimate_tokens(chunk_text),
            })
        files_processed += 1

    print(f"    arxiv_cs: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_official_docs(splitter):
    """Process official HTML docs (Go, Rust, BPF, NixOS, etc). Ring 2."""
    base = os.path.join(ZHEN_ROOT, "official_docs")
    if not os.path.isdir(base):
        print("SKIP: official_docs not yet available")
        return []

    chunks = []
    files_processed = 0
    print("  Processing official_docs...")

    for dirpath, dirnames, filenames in os.walk(base):
        for fname in filenames:
            fpath = os.path.join(dirpath, fname)
            try:
                if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                    continue
            except OSError:
                continue

            ext = os.path.splitext(fname)[1].lower()
            content = read_file(fpath)
            if not content.strip():
                continue

            # Strip HTML tags if it's an HTML file
            if ext in (".html", ".htm"):
                content = strip_html(content)
            elif ext == ".md":
                pass  # Markdown is fine as-is
            elif ext == ".txt":
                pass
            else:
                continue

            if not content.strip():
                continue

            rel = os.path.relpath(fpath, ZHEN_ROOT)
            for chunk_text in splitter.split_text(content):
                chunks.append({
                    "id": str(uuid.uuid4()),
                    "source": rel,
                    "ring": 2,
                    "type": "doc",
                    "content": chunk_text,
                    "tokens": estimate_tokens(chunk_text),
                })
            files_processed += 1

    print(f"    official_docs: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_pdfs(base_dir, ring, doc_type, label):
    """Process PDF files using pdftotext. Returns chunks."""
    if not os.path.isdir(base_dir):
        print(f"SKIP: {label} not yet available")
        return []

    splitter = RecursiveCharacterTextSplitter(
        chunk_size=2000, chunk_overlap=200, length_function=len,
    )
    chunks = []
    files_processed = 0
    print(f"  Processing {label}...")

    for fname in sorted(os.listdir(base_dir)):
        if not fname.lower().endswith(".pdf"):
            continue
        fpath = os.path.join(base_dir, fname)
        try:
            if os.path.getsize(fpath) == 0:
                continue
        except OSError:
            continue

        content = pdf_to_text(fpath)
        if not content.strip():
            continue

        rel = os.path.relpath(fpath, ZHEN_ROOT)
        for chunk_text in splitter.split_text(content):
            chunks.append({
                "id": str(uuid.uuid4()),
                "source": rel,
                "ring": ring,
                "type": doc_type,
                "content": chunk_text,
                "tokens": estimate_tokens(chunk_text),
            })
        files_processed += 1

    print(f"    {label}: {files_processed} files -> {len(chunks)} chunks")
    return chunks


def process_html_blogs(base_dir, ring, label):
    """Process HTML blog posts: strip tags, chunk. Ring 3."""
    if not os.path.isdir(base_dir):
        print(f"SKIP: {label} not yet available")
        return []

    splitter = RecursiveCharacterTextSplitter(
        chunk_size=2000, chunk_overlap=200, length_function=len,
    )
    chunks = []
    files_processed = 0
    print(f"  Processing {label}...")

    for fname in sorted(os.listdir(base_dir)):
        ext = os.path.splitext(fname)[1].lower()
        if ext not in (".html", ".htm", ".txt", ".md"):
            continue
        fpath = os.path.join(base_dir, fname)
        try:
            if os.path.getsize(fpath) > MAX_FILE_SIZE or os.path.getsize(fpath) == 0:
                continue
        except OSError:
            continue

        content = read_file(fpath)
        if not content.strip():
            continue
        if ext in (".html", ".htm"):
            content = strip_html(content)
        if not content.strip():
            continue

        rel = os.path.relpath(fpath, ZHEN_ROOT)
        for chunk_text in splitter.split_text(content):
            chunks.append({
                "id": str(uuid.uuid4()),
                "source": rel,
                "ring": ring,
                "type": "doc",
                "content": chunk_text,
                "tokens": estimate_tokens(chunk_text),
            })
        files_processed += 1

    print(f"    {label}: {files_processed} files -> {len(chunks)} chunks")
    return chunks


# ---------------------------------------------------------------------------
# Main pipeline
# ---------------------------------------------------------------------------

def phase1_chunking():
    """Phase 1: Process all sources into chunks, write ring234.jsonl."""
    t0 = time.time()

    splitter = RecursiveCharacterTextSplitter(
        chunk_size=2000, chunk_overlap=200, length_function=len,
    )

    all_chunks = []
    source_stats = {}

    # --- Ring 2 sources ---
    for label, processor in [
        ("github_repos", lambda: process_github_repos(splitter)),
        ("linux_kernel", lambda: process_linux_kernel(splitter)),
        ("official_docs", lambda: process_official_docs(splitter)),
        ("nist", lambda: process_pdfs(
            os.path.join(ZHEN_ROOT, "nist"), 2, "paper", "nist")),
    ]:
        chunks = processor()
        source_stats[label] = len(chunks)
        all_chunks.extend(chunks)
        print(f"    Running total: {len(all_chunks)} chunks")

    # --- Ring 3 sources ---
    for label, processor in [
        ("arxiv_cs", lambda: process_arxiv(splitter)),
        ("seminal_papers", lambda: process_pdfs(
            os.path.join(ZHEN_ROOT, "seminal_papers"), 3, "paper", "seminal_papers")),
        ("cloudflare", lambda: process_html_blogs(
            os.path.join(ZHEN_ROOT, "cloudflare"), 3, "cloudflare")),
        ("gregg", lambda: process_html_blogs(
            os.path.join(ZHEN_ROOT, "gregg"), 3, "gregg")),
        ("jvns", lambda: process_html_blogs(
            os.path.join(ZHEN_ROOT, "jvns"), 3, "jvns")),
    ]:
        chunks = processor()
        source_stats[label] = len(chunks)
        all_chunks.extend(chunks)
        print(f"    Running total: {len(all_chunks)} chunks")

    # --- Ring 4 sources ---
    rfc_chunks = process_rfcs(splitter)
    source_stats["rfcs"] = len(rfc_chunks)
    all_chunks.extend(rfc_chunks)
    print(f"    Running total: {len(all_chunks)} chunks")

    # --- Skip unavailable sources ---
    wiki_dir = os.path.join(ZHEN_ROOT, "wikipedia")
    so_dir = os.path.join(ZHEN_ROOT, "stackoverflow")
    # Wikipedia: check if extraction is done (would have article dirs)
    wiki_ready = os.path.isdir(wiki_dir) and any(
        f.endswith(".txt") for f in os.listdir(wiki_dir)
    ) if os.path.isdir(wiki_dir) else False
    if not wiki_ready:
        print("SKIP: wikipedia not yet available (still downloading/extracting)")

    so_ready = os.path.isdir(so_dir) and any(
        f.endswith(".xml") and not f.endswith(".7z")
        for f in os.listdir(so_dir)
    ) if os.path.isdir(so_dir) else False
    if not so_ready:
        print("SKIP: stackoverflow not yet available (still downloading)")

    # --- Write output ---
    os.makedirs(CORPUS_DIR, exist_ok=True)
    with open(OUTPUT_FILE, "w", encoding="utf-8") as out:
        for i, chunk in enumerate(all_chunks):
            out.write(json.dumps(chunk, ensure_ascii=False) + "\n")
            if (i + 1) % 1000 == 0:
                print(f"  Written {i + 1}/{len(all_chunks)} chunks...")

    elapsed = time.time() - t0

    # --- Statistics ---
    ring_counts = {}
    ring_tokens = {}
    type_counts = {}
    total_tokens = 0
    for c in all_chunks:
        r = c["ring"]
        ring_counts[r] = ring_counts.get(r, 0) + 1
        ring_tokens[r] = ring_tokens.get(r, 0) + c["tokens"]
        t = c["type"]
        type_counts[t] = type_counts.get(t, 0) + 1
        total_tokens += c["tokens"]

    print(f"\n{'='*60}")
    print("Ring 2-4 Corpus Expansion Complete")
    print(f"{'='*60}")
    print(f"Time:          {elapsed:.1f}s")
    print(f"Total chunks:  {len(all_chunks):,}")
    print(f"Total tokens:  ~{total_tokens:,}")
    print()
    print("Chunks per ring:")
    for r in sorted(ring_counts):
        print(f"  Ring {r}: {ring_counts[r]:>8,} chunks  (~{ring_tokens[r]:>10,} tokens)")
    print()
    print("Chunks per source:")
    for src in sorted(source_stats, key=source_stats.get, reverse=True):
        print(f"  {src:20s}: {source_stats[src]:>8,} chunks")
    print()
    print("Chunks per type:")
    for t in sorted(type_counts, key=type_counts.get, reverse=True):
        print(f"  {t:10s}: {type_counts[t]:>8,}")
    print()
    print(f"Output: {OUTPUT_FILE}")

    # Save checkpoint
    checkpoint = {
        "total_chunks": len(all_chunks),
        "total_tokens": total_tokens,
        "ring_counts": ring_counts,
        "source_stats": source_stats,
        "type_counts": type_counts,
        "elapsed_s": elapsed,
    }
    with open(CHECKPOINT_FILE, "w") as f:
        json.dump(checkpoint, f, indent=2)
    print(f"Checkpoint: {CHECKPOINT_FILE}")

    return all_chunks


def phase2_embed_and_index():
    """Phase 2: Load Ring 1 + Ring 2-4, embed, build combined FAISS index."""
    import faiss
    from sentence_transformers import SentenceTransformer

    MODEL_NAME = "all-MiniLM-L6-v2"
    BATCH_SIZE = 64
    DIMENSION = 384

    # --- Load Ring 1 ---
    print(f"\nLoading Ring 1 corpus from {RING1_CORPUS} ...")
    ring1_chunks = []
    if os.path.exists(RING1_CORPUS):
        with open(RING1_CORPUS, "r") as f:
            for line in f:
                if line.strip():
                    rec = json.loads(line)
                    # Add ring field if not present
                    if "ring" not in rec:
                        rec["ring"] = 1
                    ring1_chunks.append(rec)
        print(f"  Ring 1: {len(ring1_chunks):,} chunks")
    else:
        print(f"  WARNING: Ring 1 corpus not found at {RING1_CORPUS}")

    # --- Load Ring 2-4 ---
    print(f"Loading Ring 2-4 corpus from {OUTPUT_FILE} ...")
    ring234_chunks = []
    with open(OUTPUT_FILE, "r") as f:
        for line in f:
            if line.strip():
                ring234_chunks.append(json.loads(line))
    print(f"  Ring 2-4: {len(ring234_chunks):,} chunks")

    # --- Combine ---
    all_chunks = ring1_chunks + ring234_chunks
    print(f"  Combined: {len(all_chunks):,} chunks")

    # --- Load model ---
    print(f"\nLoading model '{MODEL_NAME}' ...")
    model = SentenceTransformer(MODEL_NAME)

    # --- Embed ---
    texts = [c["content"] for c in all_chunks]
    print(f"Encoding {len(texts):,} chunks (batch_size={BATCH_SIZE}) ...")
    t0 = time.time()
    embeddings = model.encode(
        texts,
        batch_size=BATCH_SIZE,
        show_progress_bar=True,
        convert_to_numpy=True,
        normalize_embeddings=True,
    )
    elapsed = time.time() - t0
    print(f"  Encoding done in {elapsed:.1f}s  ({len(texts)/elapsed:.0f} chunks/s)")

    # --- Build FAISS index ---
    print("Building FAISS IndexFlatL2 ...")
    embeddings = embeddings.astype("float32")
    index = faiss.IndexFlatL2(DIMENSION)
    index.add(embeddings)
    print(f"  Index contains {index.ntotal:,} vectors, dimension={DIMENSION}")

    # --- Save ---
    os.makedirs(INDEX_DIR, exist_ok=True)

    faiss.write_index(index, COMBINED_INDEX)
    idx_mb = os.path.getsize(COMBINED_INDEX) / (1024 * 1024)
    print(f"  Saved FAISS index -> {COMBINED_INDEX}  ({idx_mb:.1f} MB)")

    chunk_ids = [c["id"] for c in all_chunks]
    with open(COMBINED_IDS, "w") as f:
        json.dump(chunk_ids, f)
    print(f"  Saved ID mapping -> {COMBINED_IDS}")

    corpus_data = [
        {"id": c["id"], "source": c["source"], "ring": c.get("ring", 1),
         "type": c.get("type", "code"), "content": c["content"]}
        for c in all_chunks
    ]
    with open(COMBINED_CORPUS, "w") as f:
        json.dump(corpus_data, f)
    corpus_mb = os.path.getsize(COMBINED_CORPUS) / (1024 * 1024)
    print(f"  Saved corpus JSON -> {COMBINED_CORPUS}  ({corpus_mb:.1f} MB)")

    # --- Sanity search ---
    print("\n--- Sanity search: 'eBPF packet tracing with XDP' ---")
    query = "eBPF packet tracing with XDP"
    q_vec = model.encode([query], convert_to_numpy=True,
                         normalize_embeddings=True).astype("float32")
    D, I = index.search(q_vec, 5)

    for rank, (dist, idx) in enumerate(zip(D[0], I[0]), 1):
        c = corpus_data[idx]
        snippet = c["content"][:200].replace("\n", " ")
        print(f"\n  #{rank}  dist={dist:.4f}  ring={c['ring']}  "
              f"source={c['source']}  type={c['type']}")
        print(f"       {snippet}...")

    print(f"\n{'='*60}")
    print("Combined index statistics:")
    print(f"  Total vectors:   {index.ntotal:,}")
    print(f"  Ring 1 chunks:   {len(ring1_chunks):,}")
    print(f"  Ring 2-4 chunks: {len(ring234_chunks):,}")
    print(f"  Index size:      {idx_mb:.1f} MB")
    print(f"  Corpus size:     {corpus_mb:.1f} MB")
    print(f"{'='*60}")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    print("=" * 60)
    print("Zhen AI — Ring 2-4 Corpus Expansion Pipeline")
    print("=" * 60)
    print()

    # Phase 1: Chunking
    print("PHASE 1: Chunking all Ring 2-4 sources")
    print("-" * 40)
    phase1_chunking()

    # Phase 2: Embedding + FAISS index
    print()
    print("PHASE 2: Embedding + Combined FAISS Index")
    print("-" * 40)
    phase2_embed_and_index()

    print("\nAll done.")
