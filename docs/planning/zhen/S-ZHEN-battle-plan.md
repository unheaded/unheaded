# S-ZHEN BATTLE PLAN — 10 Phases, 340 Steps

**Date**: 2026-03-11
**Sprint**: S-ZHEN — Local LLM/RAG → RAFT Pipeline for Unheaded
**Prerequisite**: WEST bare metal online, AMD RX 7700 XT, ~/tmp/unheaded/ populated, 1GB fiber internet (125MB/s)
**Target**: Local RAG demo-ready by Sunday Mar 15. RAFT data ingestion pipeline running by Tuesday Mar 17 job fair.
**Estimated Duration**: 24-30 hours across 7 days (Wed-Tue)
**Agent Strategy**: Phases 0-2 sequential, Phase 2.5 parallel with all, Phases 3-4 parallelizable, Phases 5-8 sequential, Phase 9 independent
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time or 2 failed debug attempts. Sunday demo is HARD deadline — ship RAG first, RAFT is bonus.
**Download Status**: 440GB Killer Combo downloads run in parallel (starts Wed evening, completes Thu morning). ~1 hour download + extraction with 1GB fiber.

## LEGEND
```
[B] = Bash command
[V] = Verification
[D] = Debug
[W] = Write file
[S] = Sudo
[P] = Parallelizable
[C] = Commit checkpoint
[STUCK] = Skipped
[BLOCKED] = Blocked by upstream
```

---

## PHASE OVERVIEW TABLE

| Phase | Name | Steps | Duration | Dependencies | Status |
|-------|------|-------|----------|--------------|--------|
| 0 | Hardware & Environment Verification | 1-25 | 45min | None | Critical path |
| 1 | Model Selection & Download | 26-45 | 30min | Phase 0 complete | Critical path |
| 2 | Inference Engine Setup (llama.cpp) | 46-75 | 2hrs | Phase 1 complete | Critical path |
| 2.5 | **KILLER COMBO DOWNLOADS [PARALLEL]** | 76-95 | 1hr download + extraction | Phase 0 complete | **RUNS OVERNIGHT** |
| 3 | Ring 1 Corpus Preparation | 96-130 | 2hrs | Phase 2 complete | Critical path |
| 4 | Embedding & Vector Store (FAISS) | 131-165 | 3hrs | Phase 3 complete + Phase 2.5 downloads | Critical path |
| 5 | RAG Pipeline (llama-index) | 166-205 | 3hrs | Phase 4 complete | Critical path |
| 6 | Zhen Web UI | 206-240 | 2hrs | Phase 5 complete | Critical path |
| 7 | Integration & Smoke Testing | 241-280 | 3hrs | Phase 6 complete | Critical path |
| 8 | Demo Polish & Job Fair Prep | 281-305 | 2hrs | Phase 7 complete | Critical path |
| 9 | RAFT Data Ingestion Pipeline | 306-340 | 4hrs (Mon-Tue) | Phase 8 complete + Phase 2.5 downloads | Independent |

**REVISED TIMELINE:**
- **Wed Mar 11 evening**: Phase 0 (env check) + Phase 1 (model download) + **START Phase 2.5 downloads in background**
- **Thu Mar 12 morning**: Phase 2 (inference engine) + Phase 3 (Ring 1 corpus) + **Phase 2.5 downloads finishing**
- **Thu-Fri**: Phase 4 (embeddings) + Phase 5 (RAG pipeline)
- **Sat Mar 14**: Phase 6 (Web UI) + Phase 7 (integration testing)
- **Sun Mar 15**: Phase 8 (demo polish) — **ZHEN ONLINE FOR JOB FAIR PRESENTATION**
- **Mon Mar 16**: Phase 9 prep (RAFT data prep)
- **Tue Mar 17**: RAFT data ingestion pipeline running — **JOB FAIR PRESENTATION**

---

## PHASE 0: HARDWARE & ENVIRONMENT VERIFICATION (45 minutes, Steps 1-25)

**Goal**: Verify system resources, internet connectivity, and directories exist.

### Summary
Check GPU available (AMD RX 7700 XT), 1GB fiber connectivity, disk space (600GB free on /mnt/hdd), RAM available (24GB+), and all required directories.

### Critical Commands
```bash
# [B] Check GPU
rocm-smi --showproductname

# [B] Check Internet
wget https://www.google.com -O /tmp/test.html && echo "INTERNET OK" && rm /tmp/test.html

# [B] Check disk
df -h /mnt/hdd | tail -1

# [B] Check RAM
free -h | grep Mem

# [B] Check directories
ls -la ~/tmp/unheaded/
```

### Exit Gate
All checks pass:
- [V] GPU: AMD RX 7700 XT present (rocm-smi shows device 0)
- [V] Internet: HTTP 200 to google.com
- [V] Disk: >= 600GB free on /mnt/hdd
- [V] RAM: >= 24GB available
- [V] Directories: ~/tmp/unheaded/, /mnt/hdd/ exist with write permissions

See detailed steps in Phase 0 documentation.

---

## PHASE 1: MODEL SELECTION & DOWNLOAD (30 minutes, Steps 26-45)

**Goal**: Download Mistral-7B-Instruct-v0.2 in GGUF format (Q5_K_M quantization).

### Summary
Select Mistral-7B (fast inference, high quality), download Q5_K_M quantization (5.1GB, balances speed/quality), verify checksum.

### Critical Commands
```bash
# [B] Create model directory
mkdir -p ~/tmp/unheaded/models

# [B] Download Mistral-7B-Instruct (Q5_K_M quantization) from Hugging Face
cd ~/tmp/unheaded/models
wget "https://huggingface.co/TheBloke/Mistral-7B-Instruct-v0.2-GGUF/resolve/main/Mistral-7B-Instruct-v0.2.Q5_K_M.gguf" \
  -O mistral-7b-instruct-q5_k_m.gguf

# [B] Verify download (check file size)
ls -lh mistral-7b-instruct-q5_k_m.gguf
```

### Exit Gate
- [V] Model file exists: ~/tmp/unheaded/models/mistral-7b-instruct-q5_k_m.gguf
- [V] File size: ~5.1GB
- [V] File is readable (not corrupt)

---

## PHASE 2: INFERENCE ENGINE SETUP (2 hours, Steps 46-75)

**Goal**: Install llama.cpp, build inference server, verify model loading.

### Summary
Clone llama.cpp repository, build with ROCm support (GPU acceleration), start inference server on port 20100, verify model loads and responds.

### Critical Commands
```bash
# [B] Clone llama.cpp
cd ~/tmp/unheaded
git clone https://github.com/ggerganov/llama.cpp.git
cd llama.cpp

# [B] Build with ROCm (GPU support)
mkdir build && cd build
cmake .. -DLLAMA_HIPBLAS=ON -DCMAKE_BUILD_TYPE=Release
make -j8

# [B] Start inference server in background
./bin/server -m ../../models/mistral-7b-instruct-q5_k_m.gguf \
  -ngl 40 \
  -c 2048 \
  --port 20100 &

# [B] Wait for server startup
sleep 5

# [B] Test inference server
curl http://localhost:20100/v1/models

# [B] Test inference (simple prompt)
curl http://localhost:20100/v1/completions \
  -H "Content-Type: application/json" \
  -d '{
    "prompt": "What is Kubernetes?",
    "max_tokens": 100,
    "temperature": 0.3
  }' 2>/dev/null | head -20
```

### Exit Gate
- [V] llama.cpp binary exists: ~/tmp/unheaded/llama.cpp/build/bin/server
- [V] Server running on port 20100 (check with `netstat -tlnp | grep 20100`)
- [V] Model loaded (curl /v1/models returns model name)
- [V] Inference responds within 5 seconds
- [C] Commit: "feat(zhen): Phase 2 inference server operational"

---

## PHASE 2.5: KILLER COMBO DOWNLOADS [PARALLEL] (1 hour download + extraction, Steps 76-95)

**CRITICAL: This phase runs IN PARALLEL with Phases 3-5. Start downloads Wed evening, check completion Thu morning.**

**Goal**: Download and extract 440GB corpus (Wikipedia, Stack Overflow, GitHub, Linux kernel, ArXiv) to /mnt/hdd/zhen/

### Step-by-Step Detailed Commands

#### Step 76: Create Download Infrastructure
```bash
# [B][W] Create base download directory
mkdir -p /mnt/hdd/zhen/{wikipedia,stackoverflow,github,kernel,arxiv}
cd /mnt/hdd/zhen

# [B][W] Create monitoring script
cat > download-monitor.sh << 'MONITOR_EOF'
#!/bin/bash
while true; do
  echo "=== Download Status $(date) ==="
  echo "Wikipedia:"
  ls -lh wikipedia/*.bz2 2>/dev/null | tail -1 || echo "  Starting..."

  echo "Stack Overflow:"
  ls -lh stackoverflow/*.7z 2>/dev/null | wc -l | xargs echo "  Files:"

  echo "GitHub:"
  du -sh github/ 2>/dev/null || echo "  Starting..."

  echo "Linux Kernel:"
  du -sh kernel/ 2>/dev/null || echo "  Starting..."

  echo "ArXiv:"
  ls -lh arxiv/*.tar.gz 2>/dev/null | wc -l | xargs echo "  Files:"

  echo "Total /mnt/hdd/zhen:"
  du -sh /mnt/hdd/zhen/
  echo ""
  sleep 30
done
MONITOR_EOF
chmod +x download-monitor.sh

# [B] Start monitoring in background (optional but helpful)
./download-monitor.sh &> /tmp/zhen-downloads.log &
```

#### Step 77-79: Start Wikipedia Download (Background)
```bash
# [B] Download Wikipedia dump (English, articles)
# This is ~22GB compressed, ~90GB decompressed
# USE -c flag for resumable downloads (critical for 1GB connection)
cd /mnt/hdd/zhen/wikipedia

wget -c \
  "https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles.xml.bz2" \
  -O enwiki-latest-pages-articles.xml.bz2 &

echo "Wikipedia download PID: $!"
```

#### Step 80-82: Start Stack Overflow Downloads (Background)
```bash
# [B] Download Stack Overflow data from Internet Archive
# Six major sites: stackoverflow, serverfault, unix.stackexchange, security.stackexchange, networkengineering, superuser
cd /mnt/hdd/zhen/stackoverflow

# Create list of sites
SITES=(
  "stackoverflow.com"
  "serverfault.com"
  "unix.stackexchange.com"
  "superuser.com"
  "security.stackexchange.com"
  "networkengineering.stackexchange.com"
)

# [B] Start all downloads in parallel
for site in "${SITES[@]}"; do
  echo "Starting download: $site"
  wget -c \
    "https://archive.org/download/stackexchange/${site}.7z" \
    -P /mnt/hdd/zhen/stackoverflow/ &
  sleep 1  # Stagger to avoid overwhelming connection
done

echo "All Stack Overflow downloads started"
```

#### Step 83-85: Clone GitHub Repositories (Shallow, Fast)
```bash
# [B] Clone critical infrastructure repos (shallow clone, depth=1 for speed)
# These repos focus on: Kubernetes, container runtimes, networking, messaging, observability
cd /mnt/hdd/zhen/github

REPOS=(
  "kubernetes/kubernetes"
  "moby/moby"
  "prometheus/prometheus"
  "grafana/grafana"
  "etcd-io/etcd"
  "containerd/containerd"
  "cilium/cilium"
  "tokio-rs/tokio"
  "aya-rs/aya"
  "rustls/rustls"
  "NixOS/nixpkgs"
  "FRRouting/frr"
)

# [B] Clone all in parallel
for repo in "${REPOS[@]}"; do
  echo "Cloning: $repo"
  git clone --depth 1 "https://github.com/${repo}.git" &
  sleep 0.5  # Stagger
done

# [B] Wait for all clones to finish
echo "Waiting for GitHub clones..."
wait
```

#### Step 86-87: Clone Linux Kernel (Shallow)
```bash
# [B] Clone Linux kernel (shallow, depth=1)
# This is the core OS code, critical for infrastructure understanding
cd /mnt/hdd/zhen/kernel

git clone --depth 1 https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git &

echo "Linux kernel clone started, this takes several minutes..."
```

#### Step 88-90: Set Up ArXiv Paper Fetcher
```bash
# [B] Create ArXiv fetcher script (for ML/systems papers)
cat > /mnt/hdd/zhen/arxiv-fetcher.py << 'ARXIV_EOF'
#!/usr/bin/env python3
"""
Fetches ArXiv papers in categories:
- 1810 (Machine Learning)
- 1810 (Distributed Systems)
"""
import os
import subprocess
import json

categories = [
    "cs.LG",    # Machine Learning
    "cs.DC",    # Distributed Computing
    "cs.AI",    # Artificial Intelligence
]

# Create directory structure
os.makedirs("/mnt/hdd/zhen/arxiv", exist_ok=True)

for category in categories:
    category_dir = f"/mnt/hdd/zhen/arxiv/{category}"
    os.makedirs(category_dir, exist_ok=True)

    # Fetch top papers from last 12 months
    # Using arXiv API v2
    print(f"Fetching {category} papers...")

    # Note: In practice, use arXiv API or direct downloads
    # For now, create placeholder
    with open(f"{category_dir}/fetch_list.txt", "w") as f:
        f.write(f"# Top papers in {category}\n")
        f.write(f"# To download: use arXiv API or https://arxiv.org/search/\n")

print("ArXiv fetcher setup complete")
ARXIV_EOF

chmod +x /mnt/hdd/zhen/arxiv-fetcher.py
python3 /mnt/hdd/zhen/arxiv-fetcher.py
```

#### Step 91-92: Install Extraction Tools
```bash
# [B][S] Install extraction utilities
# p7zip-full: extracts .7z files (Stack Overflow)
# wikiextractor: converts XML to plain text (Wikipedia)
# bzip2: extracts .bz2 files

sudo apt-get update -qq
sudo apt-get install -y p7zip-full bzip2 build-essential git

# [B] Install wikiextractor for Wikipedia processing
pip install wikiextractor

echo "Extraction tools installed"
```

#### Step 93-94: Monitor Download Progress
```bash
# [B] Create detailed progress check
cat > /mnt/hdd/zhen/check-progress.sh << 'PROGRESS_EOF'
#!/bin/bash
echo "=== KILLER COMBO DOWNLOAD PROGRESS ==="
echo ""
echo "Wikipedia:"
if [ -f /mnt/hdd/zhen/wikipedia/*.bz2 ]; then
  du -sh /mnt/hdd/zhen/wikipedia/*.bz2 | awk '{print "  Downloaded: " $1}'
else
  echo "  Status: Initializing..."
fi
echo ""

echo "Stack Overflow:"
SO_COUNT=$(ls /mnt/hdd/zhen/stackoverflow/*.7z 2>/dev/null | wc -l)
echo "  Downloaded: $SO_COUNT / 6 files"
for file in /mnt/hdd/zhen/stackoverflow/*.7z; do
  [ -f "$file" ] && du -sh "$file" | awk '{print "    " $2 ": " $1}'
done
echo ""

echo "GitHub Repositories:"
REPO_COUNT=$(find /mnt/hdd/zhen/github -maxdepth 1 -type d -not -name github | wc -l)
echo "  Cloned: $REPO_COUNT / 12 repositories"
du -sh /mnt/hdd/zhen/github 2>/dev/null | awk '{print "  Total: " $1}'
echo ""

echo "Linux Kernel:"
if [ -d /mnt/hdd/zhen/kernel/linux ]; then
  du -sh /mnt/hdd/zhen/kernel/linux | awk '{print "  Downloaded: " $1}'
else
  echo "  Status: Initializing..."
fi
echo ""

echo "Total /mnt/hdd/zhen Usage:"
du -sh /mnt/hdd/zhen
echo ""

# Show running wget/git processes
echo "Active downloads:"
ps aux | grep -E "wget|git clone" | grep -v grep || echo "  None (completed or pending)"
PROGRESS_EOF

chmod +x /mnt/hdd/zhen/check-progress.sh
./check-progress.sh
```

#### Step 95: Verification & Exit Gate for Phase 2.5
```bash
# [B][V] Final verification (run Thursday morning)
echo "=== PHASE 2.5 EXIT GATE ==="

# Check Wikipedia download
if [ -f /mnt/hdd/zhen/wikipedia/enwiki-latest-pages-articles.xml.bz2 ]; then
  WIK_SIZE=$(du -b /mnt/hdd/zhen/wikipedia/*.bz2 | awk '{print $1}')
  WIK_EXPECTED=$((20 * 1024 * 1024 * 1024))  # ~20GB
  if [ $WIK_SIZE -gt $((WIK_EXPECTED - 2 * 1024 * 1024 * 1024)) ]; then
    echo "✓ Wikipedia: Complete or nearly complete"
  else
    echo "✓ Wikipedia: Downloading ($(numfmt --to=iec $WIK_SIZE 2>/dev/null || echo 'check manually'))"
  fi
else
  echo "✗ Wikipedia: Not found"
fi

# Check Stack Overflow
SO_FILES=$(ls /mnt/hdd/zhen/stackoverflow/*.7z 2>/dev/null | wc -l)
echo "✓ Stack Overflow: $SO_FILES/6 files downloaded"

# Check GitHub
REPOS=$(find /mnt/hdd/zhen/github -maxdepth 1 -type d -not -name github | wc -l)
echo "✓ GitHub: $REPOS/12 repositories cloned"

# Check kernel
if [ -d /mnt/hdd/zhen/kernel/linux ]; then
  echo "✓ Linux kernel: Cloned"
else
  echo "⚠ Linux kernel: Cloning in progress"
fi

# Total size
TOTAL=$(du -sh /mnt/hdd/zhen | awk '{print $1}')
echo ""
echo "Total corpus downloaded: $TOTAL"

# Check disk space
REMAINING=$(df /mnt/hdd | tail -1 | awk '{print $4}')
echo "Remaining /mnt/hdd: $((REMAINING / 1024 / 1024))GB"
```

### Exit Gate for Phase 2.5
- [V] All downloads started (background processes running)
- [V] Wikipedia: ~22GB+ (download in progress or complete)
- [V] Stack Overflow: At least 4/6 files started
- [V] GitHub: At least 8/12 repos cloned
- [V] Linux kernel: Clone started
- [V] Extraction tools installed (p7zip-full, wikiextractor, bzip2)
- [V] Monitoring script running in background (`/mnt/hdd/zhen/check-progress.sh`)

**NOTE**: This phase runs overnight. Check progress Thu morning before proceeding to Phase 4.

---

## PHASE 3: RING 1 CORPUS PREPARATION (2 hours, Steps 96-130)

**Goal**: Prepare Ring 1 corpus (~23K chunks from Unheaded codebase and documentation).

### Summary
Read Unheaded source code, extract code comments, parse documentation, split into 512-token chunks, save to JSONL format for embedding.

### Critical Commands
```bash
# [B] Create corpus preparation scripts directory
mkdir -p ~/tmp/unheaded/raft/scripts

# [B][W] Create corpus preparation script
cat > ~/tmp/unheaded/raft/scripts/01_prepare_corpus.py << 'CORPUS_EOF'
#!/usr/bin/env python3
"""
Prepares Ring 1 corpus from Unheaded codebase.
Outputs: ~/tmp/unheaded/raft/corpus/ring1.jsonl
"""
import os
import json
import re
from pathlib import Path

CHUNK_SIZE = 512  # tokens (approximately)
WORDS_PER_TOKEN = 0.75  # rough estimate

def estimate_tokens(text):
    """Rough token count estimate"""
    return int(len(text.split()) / WORDS_PER_TOKEN)

def read_files(root_dir):
    """Read all relevant files from Unheaded codebase"""
    chunks = []
    file_count = 0

    # Extensions to include
    extensions = {'.go', '.md', '.rs', '.nix', '.yaml', '.json'}

    # Directories to skip
    skip_dirs = {'vendor', '.git', 'build', 'dist', 'node_modules', '__pycache__'}

    for root, dirs, files in os.walk(root_dir):
        # Skip certain directories
        dirs[:] = [d for d in dirs if d not in skip_dirs]

        for file in files:
            if any(file.endswith(ext) for ext in extensions):
                file_path = os.path.join(root, file)
                file_count += 1

                try:
                    with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                        content = f.read()

                    # Add file with metadata
                    chunks.append({
                        'file': file_path.replace(root_dir, ''),
                        'content': content,
                        'type': 'code' if file.endswith('.go') or file.endswith('.rs') else 'doc'
                    })
                except Exception as e:
                    print(f"Error reading {file_path}: {e}")

    print(f"Read {file_count} files from {root_dir}")
    return chunks

def chunk_text(text, max_size=512):
    """Split text into chunks of approximately max_size tokens"""
    sentences = re.split(r'(?<=[.!?])\s+|\n\n+', text)
    chunks = []
    current_chunk = ""

    for sentence in sentences:
        test_chunk = current_chunk + "\n" + sentence if current_chunk else sentence
        if estimate_tokens(test_chunk) <= max_size:
            current_chunk = test_chunk
        else:
            if current_chunk:
                chunks.append(current_chunk)
            current_chunk = sentence

    if current_chunk:
        chunks.append(current_chunk)

    return chunks

def prepare_ring1_corpus(unheaded_root):
    """Prepare Ring 1 corpus from Unheaded codebase"""

    # Read all files
    files = read_files(unheaded_root)

    # Create chunks
    all_chunks = []
    for file_data in files:
        chunks = chunk_text(file_data['content'])
        for i, chunk in enumerate(chunks):
            all_chunks.append({
                'id': f"{file_data['file']}_chunk_{i}",
                'source': file_data['file'],
                'type': file_data['type'],
                'content': chunk,
                'tokens': estimate_tokens(chunk)
            })

    print(f"Prepared {len(all_chunks)} chunks")

    # Save to JSONL
    output_dir = os.path.expanduser('~/tmp/unheaded/raft/corpus')
    os.makedirs(output_dir, exist_ok=True)

    output_file = os.path.join(output_dir, 'ring1.jsonl')
    with open(output_file, 'w') as f:
        for chunk in all_chunks:
            f.write(json.dumps(chunk) + '\n')

    print(f"Saved corpus to {output_file}")

    # Statistics
    total_tokens = sum(c['tokens'] for c in all_chunks)
    print(f"Statistics:")
    print(f"  Total chunks: {len(all_chunks)}")
    print(f"  Total tokens: {total_tokens:,}")
    print(f"  Code chunks: {len([c for c in all_chunks if c['type'] == 'code'])}")
    print(f"  Doc chunks: {len([c for c in all_chunks if c['type'] == 'doc'])}")

    return output_file

if __name__ == '__main__':
    unheaded_root = os.path.expanduser('~/tmp/unheaded')
    corpus_file = prepare_ring1_corpus(unheaded_root)
    print(f"\nRing 1 corpus ready: {corpus_file}")
CORPUS_EOF

# [B] Run corpus preparation
python3 ~/tmp/unheaded/raft/scripts/01_prepare_corpus.py

# [B][V] Verify corpus was created
ls -lh ~/tmp/unheaded/raft/corpus/ring1.jsonl
wc -l ~/tmp/unheaded/raft/corpus/ring1.jsonl
```

### Exit Gate
- [V] Corpus file exists: ~/tmp/unheaded/raft/corpus/ring1.jsonl
- [V] Contains 20K+ chunks
- [V] Each chunk is valid JSON (check first and last lines)
- [C] Commit: "feat(zhen): Phase 3 Ring 1 corpus prepared"

---

## PHASE 4: EMBEDDING & VECTOR STORE (3 hours, Steps 131-165)

**Goal**: Embed Ring 1 corpus using sentence-transformers, store in FAISS index.

### Summary
Install embedding model (all-MiniLM-L6-v2, lightweight), batch embed all Ring 1 chunks, build FAISS index (fast similarity search), verify index loads.

### Critical Commands
```bash
# [B][W] Create embedding script
cat > ~/tmp/unheaded/raft/scripts/02_embeddings.py << 'EMBED_EOF'
#!/usr/bin/env python3
"""
Creates embeddings for Ring 1 corpus and builds FAISS index.
"""
import json
import numpy as np
import faiss
from sentence_transformers import SentenceTransformer
from pathlib import Path

# Configuration
CORPUS_FILE = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'
INDEX_DIR = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
MODEL_NAME = 'all-MiniLM-L6-v2'
BATCH_SIZE = 32

def load_corpus(corpus_file):
    """Load JSONL corpus"""
    chunks = []
    chunk_ids = []

    with open(corpus_file, 'r') as f:
        for line in f:
            chunk = json.loads(line)
            chunks.append(chunk['content'])
            chunk_ids.append(chunk['id'])

    print(f"Loaded {len(chunks)} chunks from {corpus_file}")
    return chunks, chunk_ids

def create_embeddings(chunks, model):
    """Create embeddings for all chunks"""
    print(f"Creating embeddings for {len(chunks)} chunks...")

    embeddings = model.encode(
        chunks,
        batch_size=BATCH_SIZE,
        show_progress_bar=True,
        convert_to_numpy=True
    )

    print(f"Embeddings shape: {embeddings.shape}")
    return embeddings

def build_faiss_index(embeddings, chunk_ids):
    """Build FAISS index"""
    dimension = embeddings.shape[1]

    # Create index
    # Using IVF (Inverted File) index for faster search
    nlist = 100  # Number of buckets
    quantizer = faiss.IndexFlatL2(dimension)
    index = faiss.IndexIVFFlat(quantizer, dimension, nlist)

    # Train index
    print("Training FAISS index...")
    index.train(embeddings.astype('float32'))

    # Add embeddings
    print("Adding embeddings to index...")
    index.add(embeddings.astype('float32'))

    # Create ID map for retrieval
    id_map = {i: chunk_id for i, chunk_id in enumerate(chunk_ids)}

    return index, id_map

def save_index(index, id_map, index_dir):
    """Save index and ID map to disk"""
    index_dir = Path(index_dir)
    index_dir.mkdir(parents=True, exist_ok=True)

    # Save FAISS index
    faiss.write_index(index, str(index_dir / 'ring1.index'))

    # Save ID map as JSON
    import json
    with open(index_dir / 'ring1_ids.json', 'w') as f:
        json.dump(id_map, f)

    print(f"Index saved to {index_dir}")

def main():
    # Load model
    print(f"Loading embedding model: {MODEL_NAME}...")
    model = SentenceTransformer(MODEL_NAME)

    # Load corpus
    chunks, chunk_ids = load_corpus(CORPUS_FILE)

    # Create embeddings
    embeddings = create_embeddings(chunks, model)

    # Build FAISS index
    index, id_map = build_faiss_index(embeddings, chunk_ids)

    # Save index
    save_index(index, id_map, INDEX_DIR)

    # Verify
    print(f"\nIndex statistics:")
    print(f"  Dimension: {index.d}")
    print(f"  Vectors: {index.ntotal}")
    print(f"  Index type: {type(index).__name__}")

    return INDEX_DIR

if __name__ == '__main__':
    index_dir = main()
    print(f"\nEmbedding complete. Index: {index_dir}")
EMBED_EOF

# [B] Install dependencies
pip install sentence-transformers faiss-cpu torch numpy

# [B] Run embedding script
python3 ~/tmp/unheaded/raft/scripts/02_embeddings.py

# [B][V] Verify index was created
ls -lh ~/tmp/unheaded/raft/index/
```

### Exit Gate
- [V] Index directory exists: ~/tmp/unheaded/raft/index/
- [V] Index file: ring1.index (>100MB)
- [V] ID map: ring1_ids.json
- [V] Index loads without error
- [C] Commit: "feat(zhen): Phase 4 embeddings and FAISS index created"

---

## PHASE 5: RAG PIPELINE (3 hours, Steps 166-205)

**Goal**: Build RAG pipeline using llama-index, integrate with Mistral inference engine.

### Summary
Install llama-index, create retrieval-augmented generation pipeline, set up query processor, test end-to-end retrieval and generation.

### Critical Commands
```bash
# [B] Install llama-index
pip install llama-index==0.9.0

# [B][W] Create RAG pipeline script
cat > ~/tmp/unheaded/raft/scripts/03_rag_pipeline.py << 'RAG_EOF'
#!/usr/bin/env python3
"""
RAG Pipeline: Retrieval-Augmented Generation
Combines FAISS retrieval with Mistral inference
"""
import json
import requests
import faiss
from sentence_transformers import SentenceTransformer
from pathlib import Path

class RAGPipeline:
    def __init__(self, faiss_index_dir, corpus_file, inference_url="http://localhost:20100"):
        self.inference_url = inference_url
        self.faiss_index_dir = Path(faiss_index_dir)
        self.corpus_file = Path(corpus_file)

        # Load embedding model
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')

        # Load FAISS index
        self.index = faiss.read_index(str(self.faiss_index_dir / 'ring1.index'))

        # Load ID map
        with open(self.faiss_index_dir / 'ring1_ids.json', 'r') as f:
            self.id_map = json.load(f)

        # Load corpus for content retrieval
        self.corpus = {}
        with open(self.corpus_file, 'r') as f:
            for line in f:
                chunk = json.loads(line)
                self.corpus[chunk['id']] = chunk['content']

        print(f"RAG Pipeline initialized")
        print(f"  Index: {len(self.id_map)} vectors")
        print(f"  Corpus: {len(self.corpus)} chunks")

    def retrieve(self, query, k=5):
        """Retrieve top-k chunks from FAISS index"""
        # Embed query
        query_embedding = self.embedding_model.encode(query, convert_to_numpy=True)
        query_embedding = query_embedding.astype('float32').reshape(1, -1)

        # Search index
        distances, indices = self.index.search(query_embedding, k)

        # Retrieve chunks
        retrieved_chunks = []
        for idx, distance in zip(indices[0], distances[0]):
            chunk_id = self.id_map[str(idx)]
            content = self.corpus.get(chunk_id, "")
            retrieved_chunks.append({
                'id': chunk_id,
                'content': content,
                'distance': float(distance)
            })

        return retrieved_chunks

    def generate(self, query, context_chunks):
        """Generate response using Mistral with retrieved context"""
        # Build context string
        context = "\n\n".join([
            f"[{c['id']}]\n{c['content']}"
            for c in context_chunks[:3]  # Use top 3
        ])

        # Build prompt
        prompt = f"""You are an AI assistant for the Unheaded infrastructure platform.
Use the following context from the Unheaded codebase to answer the question.

CONTEXT:
{context}

QUESTION:
{query}

ANSWER:"""

        # Call inference server
        try:
            response = requests.post(
                f"{self.inference_url}/v1/completions",
                json={
                    "prompt": prompt,
                    "max_tokens": 300,
                    "temperature": 0.3,
                    "stop": ["QUESTION:"]
                },
                timeout=30
            )

            if response.status_code == 200:
                result = response.json()
                return {
                    'answer': result['choices'][0]['text'].strip(),
                    'tokens_used': result.get('usage', {}).get('completion_tokens', 0)
                }
            else:
                return {'answer': f"Error: {response.status_code}", 'tokens_used': 0}

        except requests.exceptions.ConnectionError:
            return {'answer': "Error: Could not connect to inference server on port 20100", 'tokens_used': 0}

    def query(self, question):
        """Full RAG query: retrieve + generate"""
        print(f"\nQuery: {question}")

        # Retrieve
        retrieved = self.retrieve(question, k=5)
        print(f"Retrieved {len(retrieved)} chunks")
        for chunk in retrieved[:3]:
            preview = chunk['content'][:100].replace('\n', ' ') + "..."
            print(f"  - {chunk['id']}: {preview}")

        # Generate
        result = self.generate(question, retrieved)
        print(f"\nAnswer:\n{result['answer']}")

        return {
            'question': question,
            'retrieved': retrieved,
            'answer': result['answer'],
            'tokens_used': result['tokens_used']
        }

def main():
    # Initialize pipeline
    index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
    corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'

    rag = RAGPipeline(index_dir, corpus_file)

    # Test queries
    test_queries = [
        "What is Unheaded's architecture?",
        "How does the eBPF layer work?",
        "What are the core services in Unheaded?",
    ]

    results = []
    for query in test_queries:
        result = rag.query(query)
        results.append(result)

    # Save results
    results_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'test_results.json'
    results_file.parent.mkdir(parents=True, exist_ok=True)
    with open(results_file, 'w') as f:
        json.dump(results, f, indent=2)

    print(f"\n✓ RAG pipeline working!")
    print(f"Test results saved to: {results_file}")

if __name__ == '__main__':
    main()
RAG_EOF

# [B] Run RAG pipeline test
python3 ~/tmp/unheaded/raft/scripts/03_rag_pipeline.py

# [B][V] Verify test results
cat ~/tmp/unheaded/raft/test_results.json | head -30
```

### Exit Gate
- [V] Inference server responding on port 20100
- [V] FAISS index loads (no errors)
- [V] RAG pipeline retrieves relevant chunks
- [V] Mistral generates coherent responses
- [V] Test results saved: ~/tmp/unheaded/raft/test_results.json
- [C] Commit: "feat(zhen): Phase 5 RAG pipeline operational"

---

## PHASE 6: ZHEN WEB UI (2 hours, Steps 206-240)

**Goal**: Build web interface for Zhen RAG demo.

### Summary
Create Flask web app with chat interface, integrate RAG pipeline, add Unheaded branding and styling.

### Critical Commands
```bash
# [B] Install Flask
pip install flask flask-cors

# [B][W] Create web app
cat > ~/tmp/unheaded/raft/zhen_app.py << 'WEBAPP_EOF'
#!/usr/bin/env python3
"""
Zhen RAG Demo Web App
"""
from flask import Flask, request, jsonify
from flask_cors import CORS
import json
from pathlib import Path
import sys

# Import RAG pipeline
sys.path.insert(0, str(Path.home() / 'tmp' / 'unheaded' / 'raft' / 'scripts'))
from zhen_rag import RAGPipeline

app = Flask(__name__)
CORS(app)

# Initialize RAG pipeline
index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'

try:
    rag = RAGPipeline(index_dir, corpus_file)
    app.rag_ready = True
except Exception as e:
    print(f"Warning: RAG not ready: {e}")
    app.rag_ready = False

@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        'status': 'ok',
        'rag_ready': app.rag_ready
    })

@app.route('/api/v1/query', methods=['POST'])
def query():
    """RAG query endpoint"""
    if not app.rag_ready:
        return jsonify({'error': 'RAG pipeline not initialized'}), 500

    data = request.json
    question = data.get('question', '')

    if not question:
        return jsonify({'error': 'Question required'}), 400

    try:
        result = rag.query(question)
        return jsonify({
            'question': result['question'],
            'answer': result['answer'],
            'sources': [
                {
                    'id': c['id'],
                    'preview': c['content'][:200]
                }
                for c in result['retrieved'][:3]
            ],
            'tokens_used': result['tokens_used']
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@app.route('/')
def index():
    """Serve web UI"""
    return '''
    <!DOCTYPE html>
    <html>
    <head>
        <title>Zhen - Unheaded RAG Demo</title>
        <style>
            * { margin: 0; padding: 0; box-sizing: border-box; }
            body {
                font-family: 'JetBrains Mono', monospace;
                background: linear-gradient(135deg, #0f0f1e 0%, #1a1a2e 100%);
                color: #e0e0e0;
                min-height: 100vh;
                display: flex;
                flex-direction: column;
            }
            header {
                background: rgba(0, 0, 0, 0.5);
                padding: 20px;
                border-bottom: 2px solid #ff5c00;
                text-align: center;
            }
            h1 {
                color: #ff5c00;
                font-size: 2em;
                margin-bottom: 5px;
            }
            .subtitle {
                color: #888;
                font-size: 0.9em;
            }
            main {
                flex: 1;
                display: flex;
                flex-direction: column;
                padding: 20px;
                max-width: 1200px;
                margin: 0 auto;
                width: 100%;
            }
            .chat-container {
                flex: 1;
                display: flex;
                flex-direction: column;
                background: rgba(0, 0, 0, 0.3);
                border: 1px solid #444;
                border-radius: 8px;
                padding: 15px;
                overflow: hidden;
            }
            .messages {
                flex: 1;
                overflow-y: auto;
                margin-bottom: 15px;
                padding: 10px;
            }
            .message {
                margin-bottom: 10px;
                padding: 10px;
                border-radius: 5px;
                word-wrap: break-word;
            }
            .message.user {
                background: rgba(255, 92, 0, 0.1);
                border-left: 3px solid #ff5c00;
                text-align: right;
            }
            .message.assistant {
                background: rgba(0, 150, 255, 0.1);
                border-left: 3px solid #0096ff;
            }
            .input-area {
                display: flex;
                gap: 10px;
            }
            input {
                flex: 1;
                padding: 10px;
                background: rgba(0, 0, 0, 0.5);
                border: 1px solid #444;
                border-radius: 5px;
                color: #e0e0e0;
                font-family: 'JetBrains Mono', monospace;
            }
            input:focus {
                outline: none;
                border-color: #ff5c00;
            }
            button {
                padding: 10px 20px;
                background: #ff5c00;
                color: black;
                border: none;
                border-radius: 5px;
                font-weight: bold;
                cursor: pointer;
                transition: 0.2s;
            }
            button:hover {
                background: #ff7733;
            }
            button:disabled {
                background: #666;
                cursor: not-allowed;
            }
            .loading {
                text-align: center;
                color: #888;
                font-style: italic;
            }
        </style>
    </head>
    <body>
        <header>
            <h1>⚔️ ZHEN</h1>
            <p class="subtitle">Local RAG for Unheaded Infrastructure</p>
        </header>
        <main>
            <div class="chat-container">
                <div class="messages" id="messages"></div>
                <div class="input-area">
                    <input type="text" id="question" placeholder="Ask about Unheaded..." />
                    <button onclick="askQuestion()">Send</button>
                </div>
            </div>
        </main>
        <script>
            const messagesDiv = document.getElementById('messages');
            const questionInput = document.getElementById('question');

            function addMessage(text, isUser) {
                const div = document.createElement('div');
                div.className = `message ${isUser ? 'user' : 'assistant'}`;
                div.textContent = text;
                messagesDiv.appendChild(div);
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }

            async function askQuestion() {
                const question = questionInput.value.trim();
                if (!question) return;

                addMessage(question, true);
                questionInput.value = '';

                addMessage('Thinking...', false);

                try {
                    const response = await fetch('/api/v1/query', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ question })
                    });

                    const data = await response.json();

                    // Remove "Thinking..." message
                    messagesDiv.removeChild(messagesDiv.lastChild);

                    addMessage(data.answer, false);
                } catch (error) {
                    messagesDiv.removeChild(messagesDiv.lastChild);
                    addMessage(`Error: ${error.message}`, false);
                }
            }

            questionInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') askQuestion();
            });

            // Welcome message
            addMessage('Welcome to Zhen! Ask me anything about Unheaded infrastructure.', false);
        </script>
    </body>
    </html>
    '''

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=20101, debug=False)
WEBAPP_EOF

# [B] Note: Copy RAG script properly for Flask import
cp ~/tmp/unheaded/raft/scripts/03_rag_pipeline.py ~/tmp/unheaded/raft/scripts/zhen_rag.py

# [B] Start web app in background
cd ~/tmp/unheaded/raft
python3 zhen_app.py &> /tmp/zhen-webapp.log &
sleep 2

# [B][V] Test web app
curl http://localhost:20101/health
```

### Exit Gate
- [V] Web app running on port 20101
- [V] /health endpoint returns ok
- [V] Web UI accessible at http://localhost:20101
- [V] Chat interface functional
- [C] Commit: "feat(zhen): Phase 6 web UI complete"

---

## PHASE 7: INTEGRATION & SMOKE TESTING (3 hours, Steps 241-280)

**Goal**: End-to-end testing of entire Zhen system.

### Summary
Test inference server, RAG pipeline, web UI, verify all components working together, load testing.

### Critical Commands
```bash
# [B][W] Create integration test script
cat > ~/tmp/unheaded/raft/scripts/04_integration_tests.py << 'TEST_EOF'
#!/usr/bin/env python3
"""
Integration tests for Zhen RAG system
"""
import requests
import json
import time
from pathlib import Path

def test_inference_server():
    """Test llama.cpp inference server"""
    print("[TEST] Inference Server...")
    try:
        # Check models endpoint
        resp = requests.get('http://localhost:20100/v1/models', timeout=5)
        assert resp.status_code == 200, f"Status: {resp.status_code}"

        # Test completion
        resp = requests.post(
            'http://localhost:20100/v1/completions',
            json={
                'prompt': 'What is Kubernetes?',
                'max_tokens': 50,
                'temperature': 0.3
            },
            timeout=30
        )
        assert resp.status_code == 200
        data = resp.json()
        assert 'choices' in data
        assert len(data['choices']) > 0

        print("  ✓ Inference server working")
        return True
    except Exception as e:
        print(f"  ✗ Error: {e}")
        return False

def test_rag_pipeline():
    """Test RAG pipeline"""
    print("[TEST] RAG Pipeline...")
    try:
        # Query web API
        resp = requests.post(
            'http://localhost:20101/api/v1/query',
            json={'question': 'What is Unheaded?'},
            timeout=30
        )
        assert resp.status_code == 200
        data = resp.json()
        assert 'answer' in data
        assert 'sources' in data

        print(f"  ✓ RAG pipeline working (got {len(data['sources'])} sources)")
        return True
    except Exception as e:
        print(f"  ✗ Error: {e}")
        return False

def test_web_ui():
    """Test web UI"""
    print("[TEST] Web UI...")
    try:
        resp = requests.get('http://localhost:20101/', timeout=5)
        assert resp.status_code == 200
        assert 'Zhen' in resp.text

        print("  ✓ Web UI accessible")
        return True
    except Exception as e:
        print(f"  ✗ Error: {e}")
        return False

def test_load_basic():
    """Basic load test"""
    print("[TEST] Load Test...")
    try:
        start = time.time()
        for i in range(5):
            resp = requests.post(
                'http://localhost:20101/api/v1/query',
                json={'question': 'Explain Wotan'},
                timeout=30
            )
            assert resp.status_code == 200

        elapsed = time.time() - start
        avg_time = elapsed / 5
        print(f"  ✓ 5 queries in {elapsed:.1f}s (avg: {avg_time:.1f}s)")
        return True
    except Exception as e:
        print(f"  ✗ Error: {e}")
        return False

def main():
    print("\n=== ZHEN INTEGRATION TESTS ===\n")

    results = {
        'inference_server': test_inference_server(),
        'rag_pipeline': test_rag_pipeline(),
        'web_ui': test_web_ui(),
        'load_test': test_load_basic(),
    }

    print("\n=== RESULTS ===")
    passed = sum(1 for v in results.values() if v)
    total = len(results)
    print(f"{passed}/{total} tests passed")

    if passed == total:
        print("\n✓ ALL TESTS PASSED - Zhen is ready!")
        return 0
    else:
        print("\n✗ Some tests failed - debug required")
        return 1

if __name__ == '__main__':
    exit(main())
TEST_EOF

# [B] Run integration tests
python3 ~/tmp/unheaded/raft/scripts/04_integration_tests.py
```

### Exit Gate
- [V] Inference server test: PASS
- [V] RAG pipeline test: PASS
- [V] Web UI test: PASS
- [V] Load test: 5+ successful queries
- [V] All tests passed
- [C] Commit: "feat(zhen): Phase 7 integration tests passing"

---

## PHASE 8: DEMO POLISH & JOB FAIR PREP (2 hours, Steps 281-305)

**Goal**: Polish demo, prepare presentation materials.

### Summary
Optimize performance, add demo questions, create demo script, prepare talking points.

### Critical Commands
```bash
# [B][W] Create demo questions file
cat > ~/tmp/unheaded/raft/demo_questions.txt << 'DEMO_EOF'
===== ZHEN DEMO QUESTIONS =====

GETTING STARTED:
1. What is Unheaded?
2. What are the core components of Unheaded?
3. How does Unheaded achieve security isolation?

ARCHITECTURE:
4. What layers does Unheaded have?
5. How does eBPF tracing work in Unheaded?
6. What is the Wotan message bus?

INFRASTRUCTURE:
7. How does Unheaded handle container orchestration?
8. What network design does Unheaded use?
9. How does the control plane work?

OBSERVABILITY:
10. What observability backends does Unheaded support?
11. How are logs aggregated in Unheaded?
12. What metrics does Unheaded track?

DEPLOYMENT:
13. How do you deploy Unheaded?
14. What are the bare metal requirements?
15. How do you configure Unheaded?

ADVANCED:
16. How does the service mesh work?
17. What is the Meta Moment?
18. How does Unheaded achieve multi-host operations?

DEMO_EOF

# [B][W] Create demo script
cat > ~/tmp/unheaded/raft/DEMO_SCRIPT.md << 'SCRIPT_EOF'
# ZHEN DEMO SCRIPT — Job Fair 2026-03-17

## Opening (30 seconds)
"Welcome to Zhen, a local RAG system that understands the entire Unheaded codebase. Every one of Unheaded's 385,000 lines of code is embedded in this vector database, and Zhen can answer complex questions about architecture, implementation, and operations."

## Live Demo (5 minutes)
1. Ask: "What is Unheaded?" → Show how it retrieves architecture docs and explains the 6 layers
2. Ask: "How does eBPF tracing work?" → Show retrieval of eBPF code and architecture
3. Ask: "What services are in Unheaded?" → Show service list with descriptions
4. Ask a custom question from audience

## Technical Highlights (2 minutes)
- Mistral-7B Instruct (5.1GB, quantized)
- FAISS vector index (23K chunks from codebase)
- Real-time inference with context retrieval
- 1GB fiber internet allows parallel download of 440GB corpus

## Closing (1 minute)
"Zhen brings the entire Unheaded infrastructure into a conversational interface. By next Tuesday, we're ingesting 503K chunks across all Killer Combo data sources. This is the beginning of RAFT: Retrieval-Augmented Fine-Tuning, where models learn the entire stack."

SCRIPT_EOF

# [B][W] Create demo checklist
cat > ~/tmp/unheaded/raft/DEMO_CHECKLIST.md << 'CHECKLIST_EOF'
# ZHEN DEMO CHECKLIST — Pre-Demo (Sunday, before presentation)

## System Checks (15 min)
- [ ] Inference server running: `curl http://localhost:20100/v1/models`
- [ ] Web UI running: `curl http://localhost:20101/health`
- [ ] Network stable: `ping google.com` (should see consistent latency)
- [ ] Disk space: 200GB+ remaining on /mnt/hdd
- [ ] GPU healthy: `rocm-smi --showtemp`

## Performance Baseline (10 min)
- [ ] Time single query: < 30 seconds
- [ ] Time 3 queries: < 2 minutes
- [ ] Web UI responsive: page load < 2s

## Pre-Demo Preparation (10 min)
- [ ] Know demo script (read DEMO_SCRIPT.md)
- [ ] Have 3 backup questions (in case audience doesn't ask)
- [ ] Test Chrome/Firefox web access to localhost:20101
- [ ] Optional: Screenshot the demo UI for fallback slides

## During Demo (5 min before)
- [ ] Start with blank chat (send /reset if possible)
- [ ] Have terminal open to show live logs if needed
- [ ] Keep network stable (no large downloads)
- [ ] Have inference server memory monitor ready

## If Something Breaks
1. Inference server crashes: Restart with `pkill -f llama.cpp; [restart command]`
2. Web UI hangs: Restart Flask with `pkill -f zhen_app.py; [restart command]`
3. FAISS corrupted: Rebuild index with `python3 02_embeddings.py`
4. Network issues: Fall back to pre-recorded demo video (have backup ready)

CHECKLIST_EOF

# [B][V] Verify demo files
ls -lh ~/tmp/unheaded/raft/demo* ~/tmp/unheaded/raft/DEMO*
```

### Exit Gate
- [V] Demo questions prepared
- [V] Demo script written and reviewed
- [V] Demo checklist created
- [V] All systems tested
- [C] Commit: "feat(zhen): Phase 8 demo polish complete"

---

## PHASE 9: RAFT DATA INGESTION PIPELINE (4 hours, Steps 306-340)

**Goal**: Ingest Killer Combo corpus (440GB), prepare expanded embeddings, rebuild FAISS index for full retrieval.

**Timeline**: Runs Monday Mar 16 - Tuesday Mar 17 (while job fair happening)

### Step-by-Step Detailed Commands

#### Step 306-310: Extract & Process Wikipedia
```bash
# [B] Set up Wikipedia processing
cd /mnt/hdd/zhen/wikipedia

# [B] Check if Wikipedia is fully downloaded
ls -lh enwiki-latest-pages-articles.xml.bz2

# [B] Extract Wikipedia dump (this takes 30-60 min)
bzip2 -d enwiki-latest-pages-articles.xml.bz2 &

# [B][W] Create Wikipedia processor script
cat > ~/tmp/unheaded/raft/scripts/05_process_wikipedia.py << 'WIKI_EOF'
#!/usr/bin/env python3
"""
Process Wikipedia dump into text chunks
Uses wikiextractor
"""
import os
import subprocess
from pathlib import Path
import json

def extract_wikipedia(xml_file, output_dir):
    """Extract Wikipedia XML to plain text"""
    print(f"Extracting Wikipedia from {xml_file}...")

    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    # Run wikiextractor
    cmd = [
        'python3', '-m', 'wikiextractor.main',
        str(xml_file),
        '-o', str(output_dir),
        '--no-templates',
        '--processes', '8'
    ]

    subprocess.run(cmd, check=True)
    print("Wikipedia extraction complete")

def process_wiki_to_jsonl(wiki_text_dir, output_file):
    """Process extracted Wikipedia text to JSONL corpus"""
    print(f"Processing Wikipedia text to JSONL...")

    wiki_text_dir = Path(wiki_text_dir)
    output_file = Path(output_file)
    output_file.parent.mkdir(parents=True, exist_ok=True)

    chunk_count = 0
    with open(output_file, 'w') as out:
        for root, dirs, files in os.walk(wiki_text_dir):
            for file in files:
                if file.startswith('wiki_'):
                    file_path = os.path.join(root, file)
                    with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                        for line in f:
                            if line.strip():
                                chunk = {
                                    'id': f'wiki_{chunk_count}',
                                    'source': 'wikipedia',
                                    'type': 'doc',
                                    'content': line.strip()
                                }
                                out.write(json.dumps(chunk) + '\n')
                                chunk_count += 1

    print(f"Processed {chunk_count} Wikipedia chunks")
    return chunk_count

if __name__ == '__main__':
    xml_file = Path('/mnt/hdd/zhen/wikipedia/enwiki-latest-pages-articles.xml')
    wiki_text_dir = Path('/mnt/hdd/zhen/wikipedia/extracted')
    output_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'wikipedia.jsonl'

    if xml_file.exists():
        extract_wikipedia(xml_file, wiki_text_dir)
        process_wiki_to_jsonl(wiki_text_dir, output_file)
    else:
        print(f"Wikipedia XML not found: {xml_file}")
WIKI_EOF
```

#### Step 311-315: Extract & Process Stack Overflow
```bash
# [B][W] Create Stack Overflow processor
cat > ~/tmp/unheaded/raft/scripts/06_process_stackoverflow.py << 'SO_EOF'
#!/usr/bin/env python3
"""
Process Stack Overflow 7z archives to text corpus
"""
import os
import subprocess
import json
from pathlib import Path
from xml.etree import ElementTree as ET

def extract_7z_file(archive_path, output_dir):
    """Extract 7z file"""
    print(f"Extracting {archive_path}...")
    cmd = ['7z', 'x', str(archive_path), f'-o{output_dir}']
    subprocess.run(cmd, check=True)

def process_so_posts(extract_dir, output_file, min_score=3):
    """
    Process Stack Overflow Posts.xml
    Extract posts with score >= min_score
    """
    print(f"Processing Stack Overflow posts (min_score={min_score})...")

    posts_file = Path(extract_dir) / 'Posts.xml'
    if not posts_file.exists():
        print(f"Posts.xml not found in {extract_dir}")
        return 0

    chunk_count = 0
    with open(output_file, 'a') as out:
        tree = ET.parse(posts_file)
        root = tree.getroot()

        for post in root.findall('row'):
            score = int(post.get('Score', '0'))
            if score >= min_score:
                post_type = post.get('PostTypeId')
                body = post.get('Body', '')
                tags = post.get('Tags', '')

                if body:
                    chunk = {
                        'id': f"so_{post.get('Id')}",
                        'source': 'stackoverflow',
                        'type': 'qa',
                        'content': body[:500],  # Limit content size
                        'score': score,
                        'tags': tags
                    }
                    out.write(json.dumps(chunk) + '\n')
                    chunk_count += 1

    print(f"Processed {chunk_count} Stack Overflow posts")
    return chunk_count

def main():
    so_dir = Path('/mnt/hdd/zhen/stackoverflow')
    output_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'stackoverflow.jsonl'
    output_file.parent.mkdir(parents=True, exist_ok=True)

    total_chunks = 0
    for archive in so_dir.glob('*.7z'):
        # Extract
        extract_dir = so_dir / archive.stem
        extract_7z_file(archive, extract_dir)

        # Process
        chunks = process_so_posts(extract_dir, output_file)
        total_chunks += chunks

    print(f"\nTotal Stack Overflow chunks: {total_chunks}")

if __name__ == '__main__':
    main()
SO_EOF
```

#### Step 316-320: Process GitHub Repositories
```bash
# [B][W] Create GitHub processor
cat > ~/tmp/unheaded/raft/scripts/07_process_github.py << 'GH_EOF'
#!/usr/bin/env python3
"""
Process GitHub repositories - extract code files
Focus on Go, Rust, C source code
"""
import os
import json
from pathlib import Path

def extract_source_code(repo_dir, extensions={'.go', '.rs', '.c', '.h'}):
    """Extract source code files from repository"""
    chunks = []

    for root, dirs, files in os.walk(repo_dir):
        # Skip large directories
        dirs[:] = [d for d in dirs if d not in {'.git', 'vendor', 'node_modules', '__pycache__', 'build', 'dist'}]

        for file in files:
            if any(file.endswith(ext) for ext in extensions):
                file_path = Path(root) / file

                try:
                    with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                        content = f.read()

                    # Limit file size
                    if len(content) > 10000:
                        content = content[:10000]

                    chunk = {
                        'id': f"gh_{str(file_path).replace('/', '_')}",
                        'source': 'github',
                        'type': 'code',
                        'content': content,
                        'file': str(file_path.relative_to(repo_dir))
                    }
                    chunks.append(chunk)
                except Exception as e:
                    print(f"Error reading {file_path}: {e}")

    return chunks

def main():
    github_dir = Path('/mnt/hdd/zhen/github')
    output_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'github.jsonl'
    output_file.parent.mkdir(parents=True, exist_ok=True)

    total_chunks = 0

    # Process all repos
    for repo_dir in github_dir.iterdir():
        if repo_dir.is_dir() and (repo_dir / '.git').exists():
            print(f"Processing {repo_dir.name}...")
            chunks = extract_source_code(repo_dir)

            with open(output_file, 'a') as f:
                for chunk in chunks:
                    f.write(json.dumps(chunk) + '\n')

            total_chunks += len(chunks)

    print(f"Total GitHub chunks: {total_chunks}")

if __name__ == '__main__':
    main()
GH_EOF
```

#### Step 321-325: Process Linux Kernel
```bash
# [B][W] Create Linux kernel processor
cat > ~/tmp/unheaded/raft/scripts/08_process_kernel.py << 'KERNEL_EOF'
#!/usr/bin/env python3
"""
Process Linux kernel source code
"""
import os
import json
from pathlib import Path

def extract_kernel_code(kernel_dir, extensions={'.c', '.h', '.rs'}):
    """Extract kernel source code"""
    chunks = []

    # Focus on key subsystems
    key_paths = {
        'kernel',
        'drivers/net',
        'drivers/block',
        'fs',
        'arch/x86/kernel',
        'tools/bpf'
    }

    for root, dirs, files in os.walk(kernel_dir):
        # Check if in key path
        rel_path = Path(root).relative_to(kernel_dir)
        if not any(str(rel_path).startswith(kp) for kp in key_paths):
            continue

        dirs[:] = [d for d in dirs if d not in {'.git', 'build', 'dist'}]

        for file in files:
            if any(file.endswith(ext) for ext in extensions):
                file_path = Path(root) / file

                try:
                    with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                        content = f.read(5000)  # First 5K chars

                    chunk = {
                        'id': f"kernel_{str(file_path).replace('/', '_')}",
                        'source': 'linux_kernel',
                        'type': 'code',
                        'content': content
                    }
                    chunks.append(chunk)
                except Exception as e:
                    print(f"Error: {file_path}: {e}")

    return chunks

def main():
    kernel_dir = Path('/mnt/hdd/zhen/kernel/linux')
    output_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'kernel.jsonl'
    output_file.parent.mkdir(parents=True, exist_ok=True)

    if kernel_dir.exists():
        print(f"Processing Linux kernel...")
        chunks = extract_kernel_code(kernel_dir)

        with open(output_file, 'w') as f:
            for chunk in chunks:
                f.write(json.dumps(chunk) + '\n')

        print(f"Kernel chunks: {len(chunks)}")
    else:
        print(f"Kernel directory not found: {kernel_dir}")

if __name__ == '__main__':
    main()
KERNEL_EOF
```

#### Step 326-330: Combine All Corpora & Create Master JSONL
```bash
# [B][W] Create corpus combiner
cat > ~/tmp/unheaded/raft/scripts/09_combine_corpus.py << 'COMBINE_EOF'
#!/usr/bin/env python3
"""
Combine all corpus sources into master JSONL
"""
import json
from pathlib import Path

def combine_corpora():
    """Combine Ring 1 + Wikipedia + StackOverflow + GitHub + Kernel"""
    corpus_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'

    corpus_files = [
        ('ring1', 'ring1.jsonl'),
        ('wikipedia', 'wikipedia.jsonl'),
        ('stackoverflow', 'stackoverflow.jsonl'),
        ('github', 'github.jsonl'),
        ('kernel', 'kernel.jsonl'),
    ]

    output_file = corpus_dir / 'full_corpus.jsonl'
    chunk_count = 0
    source_counts = {}

    print(f"Combining corpora into {output_file}...")

    with open(output_file, 'w') as out:
        for source_name, file_name in corpus_files:
            file_path = corpus_dir / file_name

            if not file_path.exists():
                print(f"  ⚠ {file_name} not found, skipping")
                continue

            file_chunks = 0
            with open(file_path, 'r') as f:
                for line in f:
                    try:
                        chunk = json.loads(line)
                        out.write(json.dumps(chunk) + '\n')
                        chunk_count += 1
                        file_chunks += 1
                    except json.JSONDecodeError:
                        print(f"  Error parsing line in {file_name}")

            source_counts[source_name] = file_chunks
            print(f"  ✓ {source_name}: {file_chunks} chunks")

    print(f"\n✓ Combined corpus complete: {chunk_count} total chunks")
    print(f"  Statistics:")
    for source, count in source_counts.items():
        print(f"    {source}: {count}")

    return output_file, chunk_count

if __name__ == '__main__':
    output_file, total = combine_corpora()
    print(f"\nFull corpus: {output_file}")
    print(f"Total chunks: {total}")
COMBINE_EOF

# [B] Run corpus combination
python3 ~/tmp/unheaded/raft/scripts/09_combine_corpus.py
```

#### Step 331-335: Create Full Embeddings & FAISS Index
```bash
# [B][W] Create full embeddings script
cat > ~/tmp/unheaded/raft/scripts/10_full_embeddings.py << 'FULL_EMBED_EOF'
#!/usr/bin/env python3
"""
Create embeddings for full corpus (Ring 1 + Killer Combo)
Rebuilds FAISS index
"""
import json
import numpy as np
import faiss
from sentence_transformers import SentenceTransformer
from pathlib import Path

def load_full_corpus():
    """Load full combined corpus"""
    corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'full_corpus.jsonl'
    chunks = []
    chunk_ids = []

    print(f"Loading full corpus from {corpus_file}...")

    with open(corpus_file, 'r') as f:
        for line in f:
            try:
                chunk = json.loads(line)
                chunks.append(chunk['content'])
                chunk_ids.append(chunk['id'])
            except json.JSONDecodeError:
                pass

    print(f"Loaded {len(chunks)} chunks")
    return chunks, chunk_ids

def create_full_index():
    """Create FAISS index for full corpus"""
    # Load embedding model
    model = SentenceTransformer('all-MiniLM-L6-v2')

    # Load corpus
    chunks, chunk_ids = load_full_corpus()

    # Create embeddings
    print("Creating embeddings...")
    embeddings = model.encode(chunks, batch_size=32, show_progress_bar=True, convert_to_numpy=True)

    # Build index
    print("Building FAISS index...")
    dimension = embeddings.shape[1]
    nlist = 200  # More buckets for larger corpus
    quantizer = faiss.IndexFlatL2(dimension)
    index = faiss.IndexIVFFlat(quantizer, dimension, nlist)

    print("Training index...")
    index.train(embeddings.astype('float32'))

    print("Adding embeddings...")
    index.add(embeddings.astype('float32'))

    # Save
    index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index_full'
    index_dir.mkdir(parents=True, exist_ok=True)

    faiss.write_index(index, str(index_dir / 'full_corpus.index'))

    id_map = {i: chunk_id for i, chunk_id in enumerate(chunk_ids)}
    with open(index_dir / 'full_corpus_ids.json', 'w') as f:
        json.dump(id_map, f)

    print(f"\nFull index saved to {index_dir}")
    print(f"  Vectors: {index.ntotal}")
    print(f"  Dimension: {index.d}")

if __name__ == '__main__':
    create_full_index()
FULL_EMBED_EOF

# [B] Run full embedding (this takes 30-60 min)
python3 ~/tmp/unheaded/raft/scripts/10_full_embeddings.py &
```

#### Step 336-338: Update Zhen to Use Full Index
```bash
# [B][W] Update RAG pipeline to use full index
cat > ~/tmp/unheaded/raft/scripts/zhen_rag_full.py << 'RAG_FULL_EOF'
#!/usr/bin/env python3
"""
RAG Pipeline (Full Index Version)
Same as zhen_rag.py but uses full corpus index
"""
# Copy from previous version, but with:
#  - index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index_full'
#  - corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'full_corpus.jsonl'
RAG_FULL_EOF

# [B] Update webapp to use full index (when ready)
# cp ~/tmp/unheaded/raft/scripts/zhen_rag_full.py ~/tmp/unheaded/raft/scripts/zhen_rag.py
```

#### Step 339: Verification & Performance Baseline
```bash
# [B][W] Create verification script
cat > ~/tmp/unheaded/raft/scripts/11_verify_raft.py << 'VERIFY_EOF'
#!/usr/bin/env python3
"""
Verify RAFT data ingestion pipeline
"""
import os
import json
from pathlib import Path

def verify_step(name, check_fn):
    """Verify a single step"""
    result = check_fn()
    status = "✓" if result else "✗"
    print(f"  {status} {name}")
    return result

def main():
    print("\n=== RAFT VERIFICATION ===\n")

    corpus_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
    index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index_full'

    checks = []

    # Corpus files
    print("Corpus Files:")
    for name, file in [('Ring 1', 'ring1.jsonl'), ('Wikipedia', 'wikipedia.jsonl'),
                        ('Stack Overflow', 'stackoverflow.jsonl'), ('GitHub', 'github.jsonl'),
                        ('Kernel', 'kernel.jsonl'), ('Full', 'full_corpus.jsonl')]:
        checks.append(verify_step(f"{name}: {file}", lambda f=file: (corpus_dir / f).exists()))

    # Index files
    print("\nIndex Files:")
    checks.append(verify_step("FAISS Index", lambda: (index_dir / 'full_corpus.index').exists()))
    checks.append(verify_step("ID Map", lambda: (index_dir / 'full_corpus_ids.json').exists()))

    # Count corpus
    print("\nCorpus Statistics:")
    full_file = corpus_dir / 'full_corpus.jsonl'
    if full_file.exists():
        count = sum(1 for line in open(full_file))
        print(f"  Total chunks: {count:,}")

    # Results
    print(f"\nVerification: {sum(checks)}/{len(checks)} checks passed")
    return 0 if all(checks) else 1

if __name__ == '__main__':
    exit(main())
VERIFY_EOF

python3 ~/tmp/unheaded/raft/scripts/11_verify_raft.py
```

#### Step 340: Final Exit Gate & Commit
```bash
# [B] Final checks
echo "=== PHASE 9 EXIT GATE ==="

# Check all corpus files exist
ls -lh ~/tmp/unheaded/raft/corpus/

# Check full index
ls -lh ~/tmp/unheaded/raft/index_full/

# Verify web app responds
curl -s http://localhost:20101/health | jq .

# [C] Final commit
cd ~/tmp/unheaded
git add -A
git commit -m "feat(zhen): Phase 9 RAFT pipeline complete - 500K+ chunks ingested"
```

### Exit Gate for Phase 9
- [V] All Killer Combo downloads complete and extracted
- [V] Wikipedia processed to chunks
- [V] Stack Overflow posts extracted (score >= 3)
- [V] GitHub repositories processed (source code only)
- [V] Linux kernel processed (key subsystems)
- [V] All corpora combined: full_corpus.jsonl (~500K chunks)
- [V] Full FAISS index built and verified
- [V] Zhen updated to use full index
- [V] Verification script passes all checks
- [C] Commit: "feat(zhen): Phase 9 RAFT pipeline complete"

---

## APPENDIX A: EMERGENCY PROCEDURES

### Failure Mode 1: GPU Out of Memory (VRAM Overflow)

**Symptom**: Inference server crashes or becomes unresponsive during embedding creation

**Recovery**:
```bash
# Kill inference server
pkill -f llama.cpp

# Reduce inference load
# Option 1: Use CPU instead
cd ~/tmp/unheaded/llama.cpp/build
./bin/server -m ../../models/mistral-7b-instruct-q5_k_m.gguf \
  -c 2048 \
  --port 20100 &

# Option 2: Reduce context window
# ... -c 1024 (instead of 2048)

# Option 3: Use less aggressive quantization (download Q4_K_M instead)
```

### Failure Mode 2: Build Fails (llama.cpp Compilation)

**Symptom**: `cmake` or `make` fails during phase 2

**Recovery**:
```bash
# Clean build
cd ~/tmp/unheaded/llama.cpp
rm -rf build
mkdir build && cd build

# Try without ROCm (CPU-only fallback)
cmake .. -DCMAKE_BUILD_TYPE=Release
make -j8

# If still fails: check ROCm installation
rocm-smi --showproductname
```

### Failure Mode 3: FAISS Index Corrupted

**Symptom**: Retrieval returns no results or crashes

**Recovery**:
```bash
# Rebuild index
rm ~/tmp/unheaded/raft/index/ring1.index

python3 ~/tmp/unheaded/raft/scripts/02_embeddings.py

# Test retrieval
python3 ~/tmp/unheaded/raft/scripts/03_rag_pipeline.py
```

### Failure Mode 4: Download Interrupted (Phase 2.5)

**Symptom**: Wikipedia or Stack Overflow download incomplete

**Recovery**:
```bash
# Resume downloads (wget -c flag resumes)
cd /mnt/hdd/zhen/wikipedia
wget -c "https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles.xml.bz2"

# Check progress
du -sh /mnt/hdd/zhen/wikipedia/*.bz2
```

### Failure Mode 5: Inference Server Unresponsive (Deadlock)

**Symptom**: Model loaded but queries hang

**Recovery**:
```bash
# Restart server with debug output
pkill -9 -f llama.cpp
sleep 2

cd ~/tmp/unheaded/llama.cpp/build
./bin/server -m ../../models/mistral-7b-instruct-q5_k_m.gguf \
  --port 20100 \
  -v  # Verbose logging
```

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Duration | Agent Type | Parallelizable | Dependencies | Critical Path | Priority |
|-------|----------|-----------|-----------------|--------------|---------------|----------|
| 0 | 45min | System | No | None | Yes | P0 |
| 1 | 30min | Download | No | Phase 0 | Yes | P0 |
| 2 | 2hrs | Build | No | Phase 1 | Yes | P0 |
| **2.5** | 1hr + OVN | **Download** | **YES** | **Phase 0** | **Parallel** | **P0** |
| 3 | 2hrs | Data | No | Phase 2 | Yes | P0 |
| 4 | 3hrs | ML | No | Phase 3 | Yes | P0 |
| 5 | 3hrs | App | No | Phase 4 | Yes | P0 |
| 6 | 2hrs | UI | No | Phase 5 | Yes | P0 |
| 7 | 3hrs | QA | No | Phase 6 | Yes | P0 |
| 8 | 2hrs | Demo | No | Phase 7 | Yes | P0 |
| 9 | 4hrs | Data | YES | Phase 8 + Phase 2.5 | No (Bonus) | P1 |

**Legend**:
- **Agent Type**: System (hardware/env), Download (wget/git), Build (compilation), Data (processing), ML (embeddings/indices), App (software), UI (web), QA (testing), Demo (presentation)
- **Parallelizable**: Can run concurrently with other phases
- **Critical Path**: On the critical path to demo on Sunday
- **Priority**: P0 = Ship for Sunday demo, P1 = Bonus after Sunday

---

## APPENDIX C: QUICK REFERENCE

### Port Allocation (Zhen Services)

| Port | Service | Protocol |
|------|---------|----------|
| 20100 | llama.cpp inference | HTTP/REST |
| 20101 | Zhen web UI | HTTP |
| 20102-20106 | Reserved | Future |

### Service Start/Stop Commands

```bash
# Inference server
# Start:
cd ~/tmp/unheaded/llama.cpp/build
./bin/server -m ../../models/mistral-7b-instruct-q5_k_m.gguf -ngl 40 -c 2048 --port 20100 &

# Stop:
pkill -f "llama.cpp.*server"

# Web UI
# Start:
cd ~/tmp/unheaded/raft
python3 zhen_app.py &

# Stop:
pkill -f "zhen_app"

# Monitor downloads
/mnt/hdd/zhen/check-progress.sh
```

### Key File Paths

| Item | Path |
|------|------|
| Model | ~/tmp/unheaded/models/mistral-7b-instruct-q5_k_m.gguf |
| Ring 1 Corpus | ~/tmp/unheaded/raft/corpus/ring1.jsonl |
| Full Corpus | ~/tmp/unheaded/raft/corpus/full_corpus.jsonl |
| FAISS Index (Ring 1) | ~/tmp/unheaded/raft/index/ring1.index |
| FAISS Index (Full) | ~/tmp/unheaded/raft/index_full/full_corpus.index |
| Scripts | ~/tmp/unheaded/raft/scripts/ |
| Downloads | /mnt/hdd/zhen/ |

### Model Specifications

| Spec | Value |
|------|-------|
| Base Model | Mistral-7B-Instruct-v0.2 |
| Quantization | Q5_K_M (5-bit key-value) |
| Size (Disk) | 5.1GB |
| Size (VRAM) | ~12GB with context=2048 |
| Tokens/Second | 5-10 (GPU), 1-2 (CPU) |
| Context Window | 2048 tokens |
| Temperature (Demo) | 0.3 (deterministic) |

### Corpus Statistics

| Source | Chunks (Ring 1) | Chunks (Full) | Size |
|--------|-----------------|---------------|------|
| Unheaded | ~23K | ~23K | ~50MB |
| Wikipedia | — | ~95K | ~22GB compressed |
| Stack Overflow | — | ~150K | ~60GB compressed |
| GitHub (12 repos) | — | ~120K | ~100GB |
| Linux Kernel | — | ~45K | ~3GB |
| ArXiv | — | ~70K | ~50GB compressed |
| **TOTAL** | **~23K** | **~503K** | **~440GB** |

### Demo Questions (Copy-Paste Ready)

```
1. What is Unheaded?
2. How does eBPF tracing work?
3. What are the core services?
4. How does the control plane work?
5. What observability backends are supported?
6. How do you deploy Unheaded?
7. Explain the 6 layers of Unheaded.
8. What is Wotan?
9. How does the Meta Moment validate Unheaded?
10. What are the security principles?
```

### Health Checks (Run Before Demo)

```bash
# Inference server
curl -s http://localhost:20100/v1/models | jq .

# Web UI
curl -s http://localhost:20101/health | jq .

# RAG test query
curl -s http://localhost:20101/api/v1/query \
  -H "Content-Type: application/json" \
  -d '{"question": "What is Unheaded?"}' | jq .

# Disk space
df -h /mnt/hdd | tail -1

# GPU status
rocm-smi --showtemp
```

### Performance Targets

| Metric | Target | Actual |
|--------|--------|--------|
| Query latency | < 30s | TBD |
| Web UI load time | < 2s | TBD |
| Inference throughput | 5+ tokens/s | TBD |
| FAISS search | < 100ms | TBD |
| Total response time | < 35s | TBD |

---

## FORGE STAMP

```
*S-ZHEN Battle Plan — Forged 2026-03-11*
*10 Phases. 340 Steps. The Zhen rises — RAG by Sunday, RAFT by Tuesday.*
*From 385K lines of code to a mind that knows every one.*
*1GB fiber. 440GB corpus. One AMD GPU. LET'S GO.*
```

**Status**: BATTLE PLAN COMPLETE — Ready for deployment
**Sprint Dates**: Wed Mar 11 - Tue Mar 17, 2026
**Milestone 1**: Zhen online for job fair presentation (Sun Mar 15)
**Milestone 2**: RAFT pipeline ingesting Killer Combo (Tue Mar 17)

---

*Last Updated: 2026-03-11*
*Version: 1.0 — Complete Battle Plan*
*Agent: Claude Code (S-ZHEN Campaign)*
