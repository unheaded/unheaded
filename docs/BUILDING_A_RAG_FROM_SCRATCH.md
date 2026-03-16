# Building a RAG System from Scratch

**A practical tutorial based on building Zhen, an AI assistant for the Unheaded codebase.**

This guide walks through every step of building a Retrieval-Augmented Generation (RAG) system on local hardware — from raw source files to a fine-tuned model that answers domain-specific questions. No cloud APIs required. Everything runs on a single machine with a consumer GPU.

---

## What You'll Build

A system that:
1. Ingests your codebase/docs into searchable chunks
2. Embeds those chunks as vectors in a FAISS index
3. Retrieves relevant chunks when a user asks a question
4. Feeds those chunks to a local LLM as context
5. Serves it all through a web UI with conversation history
6. (Optional) Fine-tunes the LLM on your domain using RAFT

**Hardware used in this guide:** AMD RX 7700 XT (12 GB VRAM), 16 GB DDR5 RAM, Ubuntu 24.04, ROCm 6.4. The approach works on NVIDIA too — swap ROCm for CUDA.

**Time to first working RAG:** ~4 hours (mostly waiting for embeddings).

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Step 1: Prepare Your Corpus](#2-step-1-prepare-your-corpus)
3. [Step 2: Generate Embeddings + Build FAISS Index](#3-step-2-generate-embeddings--build-faiss-index)
4. [Step 3: Set Up Local Inference](#4-step-3-set-up-local-inference)
5. [Step 4: Build the RAG Pipeline](#5-step-4-build-the-rag-pipeline)
6. [Step 5: Serve It — Web UI + API](#6-step-5-serve-it--web-ui--api)
7. [Step 6: Optimize the Context Window](#7-step-6-optimize-the-context-window)
8. [Step 7: Add Conversation History](#8-step-7-add-conversation-history)
9. [Step 8: Teach and Remember](#9-step-8-teach-and-remember)
10. [Step 9: RAFT Fine-Tuning (Level Up)](#10-step-9-raft-fine-tuning-level-up)
11. [Lessons Learned](#11-lessons-learned)
12. [Architecture Reference](#12-architecture-reference)

---

## 1. Prerequisites

```bash
# Python environment
python3 -m venv ~/.venv/rag
source ~/.venv/rag/bin/activate

# Core dependencies
pip install sentence-transformers faiss-cpu numpy flask flask-cors requests

# If you have a GPU (recommended):
pip install faiss-gpu  # NVIDIA
# or build faiss from source with ROCm support for AMD
```

You'll also need a local LLM server. We use [llama.cpp](https://github.com/ggerganov/llama.cpp) with a quantized model:

```bash
# Clone and build llama.cpp
git clone https://github.com/ggerganov/llama.cpp.git
cd llama.cpp

# For AMD GPU (ROCm):
cmake -B build -DGGML_HIP=ON
cmake --build build --config Release -j$(nproc)

# For NVIDIA GPU (CUDA):
cmake -B build -DGGML_CUDA=ON
cmake --build build --config Release -j$(nproc)

# For CPU only:
cmake -B build
cmake --build build --config Release -j$(nproc)
```

Download a model. We use Mistral-7B-Instruct (Q5_K_M quantization — good balance of quality and speed):

```bash
mkdir -p models
# Download from HuggingFace (search for "mistral-7b-instruct-v0.2 GGUF Q5_K_M")
# Place in models/mistral-7b-instruct-q5_k_m.gguf
```

---

## 2. Step 1: Prepare Your Corpus

The first step is turning your source files into semantic chunks — small, self-contained pieces of text that can be independently retrieved and understood.

### Why Chunk?

LLMs have limited context windows. You can't feed your entire codebase into a prompt. Instead, you retrieve the 3-5 most relevant chunks and include only those. Good chunking means each chunk is meaningful on its own.

### The Script

```python
#!/usr/bin/env python3
"""Prepare corpus: walk source tree, chunk files into retrieval-ready pieces."""
import json
from pathlib import Path

# What to index
SOURCE_DIR = Path("~/your-project").expanduser()
OUTPUT = Path("corpus/ring1.jsonl")
CHUNK_SIZE = 512       # target tokens per chunk (~2048 chars)
CHUNK_OVERLAP = 50     # overlap between chunks for continuity

# File types to include
EXTENSIONS = {
    '.go', '.rs', '.py', '.js', '.ts',      # code
    '.md', '.txt', '.rst',                    # docs
    '.yaml', '.yml', '.toml', '.json',        # config
    '.sh', '.nix', '.html', '.css', '.sql',   # other
}

# Directories to skip
SKIP_DIRS = {'.git', 'node_modules', 'vendor', '.venv', '__pycache__'}

def chunk_text(text, source, chunk_size=CHUNK_SIZE * 4, overlap=CHUNK_OVERLAP * 4):
    """Split text into overlapping chunks of ~chunk_size characters."""
    chunks = []
    start = 0
    chunk_id = 0
    while start < len(text):
        end = start + chunk_size
        # Try to break at a paragraph or line boundary
        if end < len(text):
            # Look for double newline (paragraph break)
            break_at = text.rfind('\n\n', start + chunk_size // 2, end + 200)
            if break_at > start:
                end = break_at
            else:
                # Fall back to single newline
                break_at = text.rfind('\n', start + chunk_size // 2, end + 200)
                if break_at > start:
                    end = break_at

        chunk_text = text[start:end].strip()
        if len(chunk_text) > 50:  # skip tiny fragments
            chunks.append({
                'id': f"{source}:{chunk_id}",
                'content': chunk_text,
                'source': source,
                'type': 'code' if any(source.endswith(e) for e in ['.go','.rs','.py','.js']) else 'doc',
            })
            chunk_id += 1
        start = end - (overlap * 4)  # overlap for continuity
    return chunks

def prepare_corpus():
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    all_chunks = []

    for path in sorted(SOURCE_DIR.rglob('*')):
        if any(skip in path.parts for skip in SKIP_DIRS):
            continue
        if path.suffix not in EXTENSIONS:
            continue
        if not path.is_file():
            continue
        try:
            text = path.read_text(encoding='utf-8', errors='ignore')
        except Exception:
            continue
        if not text.strip():
            continue

        relative = str(path.relative_to(SOURCE_DIR))
        chunks = chunk_text(text, relative)
        all_chunks.extend(chunks)

    # Write JSONL (one JSON object per line)
    with open(OUTPUT, 'w', encoding='utf-8') as f:
        for chunk in all_chunks:
            f.write(json.dumps(chunk) + '\n')

    print(f"Corpus ready: {len(all_chunks):,} chunks from {SOURCE_DIR}")
    print(f"Output: {OUTPUT}")

if __name__ == '__main__':
    prepare_corpus()
```

### What We Learned Building Zhen

Our first commit (`85f8453`) created the basic pipeline. Key decisions:

- **Chunk size of ~512 tokens** is a sweet spot. Too small and you lose context. Too large and retrieval becomes imprecise.
- **Overlap of ~50 tokens** prevents important content from being split across chunk boundaries.
- **Prioritize docs over tests.** In Zhen, we weight `.md` and core `.go` files higher — they contain the architecture knowledge.
- **Ring structure.** Zhen uses "rings" to organize corpus priority:
  - Ring 1: Core project (full content, always in RAM)
  - Ring 2-4: External sources like GitHub repos, RFCs, research papers (metadata in RAM, content on demand)
  - Wikipedia: 29 GB — too large for RAM, loaded via byte-offset index on demand

You don't need rings to start. One flat corpus file works fine. Scale later.

---

## 3. Step 2: Generate Embeddings + Build FAISS Index

Embeddings convert text into dense vectors that capture semantic meaning. Similar texts produce similar vectors. FAISS lets you search millions of vectors in milliseconds.

### The Script

```python
#!/usr/bin/env python3
"""Generate embeddings and build FAISS index from corpus."""
import json
import faiss
import numpy as np
from sentence_transformers import SentenceTransformer
from pathlib import Path

CORPUS_PATH = Path("corpus/ring1.jsonl")
INDEX_DIR = Path("index")

def build_index():
    INDEX_DIR.mkdir(parents=True, exist_ok=True)

    # Load embedding model
    # all-MiniLM-L6-v2: 384 dimensions, fast, good quality for retrieval
    print("Loading embedding model...")
    model = SentenceTransformer('all-MiniLM-L6-v2')

    # Load corpus
    print("Loading corpus...")
    chunks = []
    with open(CORPUS_PATH) as f:
        for line in f:
            if line.strip():
                chunks.append(json.loads(line))
    print(f"  {len(chunks):,} chunks")

    # Generate embeddings (this is the slow part)
    print("Generating embeddings...")
    texts = [c['content'] for c in chunks]
    # Batch processing: ~40-50 chunks/sec on GPU, ~10/sec on CPU
    embeddings = model.encode(texts, batch_size=64, show_progress_bar=True,
                               convert_to_numpy=True)
    embeddings = embeddings.astype('float32')
    print(f"  Embeddings shape: {embeddings.shape}")  # (num_chunks, 384)

    # Normalize for cosine similarity via L2 distance
    faiss.normalize_L2(embeddings)

    # Build FAISS index
    print("Building FAISS index...")
    dimension = embeddings.shape[1]  # 384
    index = faiss.IndexFlatL2(dimension)  # exact search, no approximation
    index.add(embeddings)
    print(f"  Index: {index.ntotal:,} vectors")

    # Save index
    faiss.write_index(index, str(INDEX_DIR / 'ring1.index'))

    # Save ID mapping (FAISS index position → chunk ID)
    ids = [c['id'] for c in chunks]
    with open(INDEX_DIR / 'ring1_ids.json', 'w') as f:
        json.dump(ids, f)

    print(f"Done. Index saved to {INDEX_DIR}/")

if __name__ == '__main__':
    build_index()
```

### Embedding Model Choice

We use `all-MiniLM-L6-v2` because:
- **Fast:** 6-layer transformer, embeds ~50 chunks/sec on GPU
- **Small:** 22M parameters, 80 MB on disk
- **Good enough:** 384-dimensional vectors capture semantic similarity well for retrieval
- **No API needed:** Runs entirely locally via sentence-transformers

For better quality (at the cost of speed), consider `all-mpnet-base-v2` (768 dims) or `bge-large-en-v1.5`.

### FAISS Index Types

We use `IndexFlatL2` (brute-force exact search). It's simple and correct. For larger corpora:

| Index Type | Speed | Accuracy | When to Use |
|-----------|-------|----------|-------------|
| `IndexFlatL2` | O(n) | Exact | < 1M vectors |
| `IndexIVFFlat` | O(√n) | ~95% | 1M-10M vectors |
| `IndexHNSW` | O(log n) | ~97% | Any size, if RAM allows |

Zhen uses `IndexFlatL2` with 594K vectors. Search takes ~100ms, which is fine.

### Scaling: Zhen's Corpus Growth

Our git history tells the story of corpus expansion:

```
85f8453  feat(zhen): complete Zhen AI champion — RAG pipeline (Ring 1: ~30K chunks)
3dce844  fix(zhen): upgrade to combined 594K index
416bc14  feat(zhen): add Stack Overflow corpus processing
a0bb024  feat(zhen): v2 corpus rebuild — 1.52M chunks with Stack Overflow
b63f6c7  feat(zhen): Wikipedia embedded into FAISS — 1.67M vectors live
```

We started with 30K chunks from the project source. Then added Stack Overflow, GitHub repos, RFCs, research papers, and Wikipedia — growing to 1.67M vectors. The architecture didn't change, just the data feeding into it.

---

## 4. Step 3: Set Up Local Inference

The LLM generates answers from retrieved context. We use llama.cpp's built-in HTTP server — it exposes an OpenAI-compatible API.

```bash
# Start the inference server
cd llama.cpp/build

# Set library path (needed for GPU builds)
export LD_LIBRARY_PATH="$(pwd)/src:$(pwd)/ggml/src:$LD_LIBRARY_PATH"

# Launch server
./bin/llama-server \
  -m /path/to/models/mistral-7b-instruct-q5_k_m.gguf \
  -ngl 40 \       # GPU layers (40 = all layers on GPU)
  -c 16384 \      # context window (see Step 6 for why 16384)
  --port 20100    # any available port

# Verify it's running
curl http://localhost:20100/v1/models
```

### Why llama.cpp?

- Runs quantized models efficiently on consumer GPUs
- OpenAI-compatible HTTP API (`/v1/completions`, `/v1/chat/completions`)
- Supports ROCm (AMD), CUDA (NVIDIA), Metal (Apple), and CPU
- One binary, no Python dependencies for inference

### Model Selection

| Model | Size (Q5_K_M) | Quality | Speed (7700 XT) | Good For |
|-------|---------------|---------|-----------------|----------|
| Mistral-7B-Instruct | 5 GB | Good | ~60 tok/s | General purpose, instruction following |
| Llama-3-8B-Instruct | 5.5 GB | Better | ~55 tok/s | Stronger reasoning |
| Phi-3-mini (3.8B) | 2.3 GB | Decent | ~90 tok/s | Fast responses, less GPU |
| CodeLlama-7B | 5 GB | Good (code) | ~60 tok/s | Code-specific questions |

We chose Mistral-7B-Instruct for its good instruction-following ability and the fact that it supports up to 32K context natively with sliding window attention.

---

## 5. Step 4: Build the RAG Pipeline

This is the core — connecting retrieval to generation. When a user asks a question:

1. Embed the question using the same model as the corpus
2. Search FAISS for the top-k most similar chunks
3. Format chunks + question into a prompt
4. Send to LLM, return the answer

### The Pipeline Class

```python
#!/usr/bin/env python3
"""RAG Pipeline: FAISS retrieval + LLM generation."""
import json
import requests
import faiss
import numpy as np
from sentence_transformers import SentenceTransformer
from pathlib import Path


class RAGPipeline:
    def __init__(self, index_dir, corpus_file, inference_url="http://localhost:20100"):
        self.inference_url = inference_url

        # Load the same embedding model used for indexing
        print("Loading embedding model...")
        self.embedding_model = SentenceTransformer('all-MiniLM-L6-v2')

        # Load FAISS index
        print("Loading FAISS index...")
        self.index = faiss.read_index(str(Path(index_dir) / 'ring1.index'))

        # Load chunk ID mapping
        with open(Path(index_dir) / 'ring1_ids.json') as f:
            raw_ids = json.load(f)
            self.id_map = {str(i): v for i, v in enumerate(raw_ids)}

        # Load corpus content
        print("Loading corpus...")
        self.corpus = {}
        with open(corpus_file, encoding='utf-8', errors='ignore') as f:
            for line in f:
                chunk = json.loads(line)
                self.corpus[chunk['id']] = {
                    'content': chunk['content'],
                    'source': chunk.get('source', ''),
                    'type': chunk.get('type', 'unknown'),
                }

        print(f"RAG ready: {self.index.ntotal:,} vectors, {len(self.corpus):,} chunks")

    def retrieve(self, query, k=5):
        """Find the top-k most relevant chunks for a query."""
        # Embed the query
        query_vec = self.embedding_model.encode(query, convert_to_numpy=True)
        query_vec = query_vec.astype('float32').reshape(1, -1)

        # Search FAISS
        distances, indices = self.index.search(query_vec, k)

        results = []
        for idx, dist in zip(indices[0], distances[0]):
            if idx < 0:
                continue
            chunk_id = self.id_map.get(str(idx))
            if chunk_id and chunk_id in self.corpus:
                data = self.corpus[chunk_id]
                results.append({
                    'id': chunk_id,
                    'content': data['content'],
                    'source': data['source'],
                    'type': data['type'],
                    'distance': float(dist),
                })
        return results

    def generate(self, query, context_chunks):
        """Send query + retrieved context to the LLM."""
        # Format context
        context = "\n\n---\n\n".join([
            f"[Source: {c['source']}]\n{c['content'][:2000]}"
            for c in context_chunks[:5]
        ])

        # Build prompt (Mistral instruct format)
        prompt = f"""<s>[INST] You are a helpful assistant with expertise in this codebase.
Use the following retrieved context to answer accurately.
If the context doesn't contain the answer, say so.

CONTEXT:
{context}

QUESTION: {query} [/INST]"""

        # Call llama.cpp server
        try:
            response = requests.post(
                f"{self.inference_url}/v1/completions",
                json={
                    "prompt": prompt,
                    "max_tokens": 500,
                    "temperature": 0.3,
                    "top_p": 0.9,
                    "stop": ["[INST]", "</s>"],
                },
                timeout=60,
            )
            if response.status_code == 200:
                result = response.json()
                return {
                    'answer': result['choices'][0]['text'].strip(),
                    'tokens_used': result.get('usage', {}).get('completion_tokens', 0),
                }
            return {'answer': f"Error: {response.status_code}", 'tokens_used': 0}
        except Exception as e:
            return {'answer': f"Error: {e}", 'tokens_used': 0}

    def query(self, question):
        """Full RAG: retrieve + generate."""
        retrieved = self.retrieve(question, k=5)
        result = self.generate(question, retrieved)
        return {
            'question': question,
            'answer': result['answer'],
            'sources': [r['source'] for r in retrieved],
            'tokens_used': result['tokens_used'],
        }
```

### Test It

```python
rag = RAGPipeline('index', 'corpus/ring1.jsonl')
result = rag.query("How does the message bus work?")
print(result['answer'])
print(f"Sources: {result['sources']}")
```

That's a working RAG. Everything after this is polish and optimization.

---

## 6. Step 5: Serve It — Web UI + API

Wrap the pipeline in a Flask app so users can interact through a browser.

```python
#!/usr/bin/env python3
"""Web UI + REST API for RAG."""
import time
from flask import Flask, request, jsonify
from flask_cors import CORS
from rag_pipeline import RAGPipeline  # the class from Step 4

app = Flask(__name__, static_folder='static')
CORS(app)

rag = RAGPipeline('index', 'corpus/ring1.jsonl')

@app.route('/health')
def health():
    return jsonify({'status': 'ok', 'vectors': rag.index.ntotal})

@app.route('/api/v1/query', methods=['POST'])
def query():
    data = request.json
    question = data.get('question', '').strip()
    if not question:
        return jsonify({'error': 'Question required'}), 400

    start = time.time()
    result = rag.query(question)
    elapsed = time.time() - start

    return jsonify({
        'question': result['question'],
        'answer': result['answer'],
        'sources': result['sources'],
        'tokens_used': result['tokens_used'],
        'elapsed_seconds': round(elapsed, 2),
    })

@app.route('/api/v1/search', methods=['POST'])
def search():
    """Retrieval only — no generation. Good for exploring what's in the index."""
    data = request.json
    query_text = data.get('query', '').strip()
    k = min(data.get('k', 10), 50)
    results = rag.retrieve(query_text, k=k)
    return jsonify({
        'query': query_text,
        'results': [{'source': r['source'], 'content': r['content'][:500],
                      'distance': r['distance']} for r in results],
    })

@app.route('/')
def index():
    return app.send_static_file('index.html')

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=20103)
```

For the frontend, create `static/index.html` with a simple chat interface. The key JavaScript is straightforward:

```javascript
async function ask(question) {
    const response = await fetch('/api/v1/query', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ question }),
    });
    const data = await response.json();
    displayAnswer(data.answer, data.sources, data.elapsed_seconds);
}
```

Zhen's UI grew over multiple iterations (`1d692ed` polished the design, `7a189a4` added a semantic search tab, `bfa35bf` added markdown rendering). Start simple and iterate.

---

## 7. Step 6: Optimize the Context Window

This is the step most tutorials skip. Your LLM has a context window — the maximum number of tokens it can process at once. If your RAG prompt (system prompt + retrieved chunks + question) exceeds it, you get a 400 error or truncated garbage.

### The Experiment

We ran a benchmark testing context window sizes 2048 through 32768 on our hardware:

| Context Size | Speed | VRAM Used | Result |
|-------------|-------|-----------|--------|
| 2048 (default) | 58.3 tok/s | 5.4 GB | Baseline |
| 4096 | 59.7 tok/s | 5.9 GB | No degradation |
| 8192 | 59.4 tok/s | 6.4 GB | No degradation |
| **16384** | **59.5 tok/s** | **7.4 GB** | **8x improvement, zero cost** |
| 32768 | 59.7 tok/s | 9.5 GB | Works, tight VRAM headroom |

**Finding:** The default `-c 2048` was leaving 10 GB of VRAM unused. We got an 8x context window for free.

### How to Run Your Own Benchmark

The idea is simple: for each context size, restart the server, run test queries, measure speed and VRAM.

```python
CONTEXT_VALUES = [2048, 4096, 8192, 16384]

for ctx_size in CONTEXT_VALUES:
    # 1. Kill existing server
    # 2. Start with: llama-server -c {ctx_size} ...
    # 3. Wait for model load
    # 4. Run test queries, measure tokens/sec
    # 5. Record VRAM usage
    # 6. Save results
```

### Smart Context Scaling

Once you know your optimal context window, adapt the RAG pipeline:

```python
# Use more context when the window allows
if self.local_max_tokens <= 2048:
    max_chunks = 3
    chunk_limit = 1500
else:
    max_chunks = 5
    chunk_limit = 2500
```

And add a safety truncation when prompts approach the limit:

```python
estimated_tokens = len(prompt) // 4 + 50
if estimated_tokens > self.local_max_tokens * 0.85:
    context = context[:budget] + "\n[...truncated to fit context window]"
```

---

## 8. Step 7: Add Conversation History

Without conversation history, every query is independent. "Elaborate on point 3" means nothing if the model can't see the previous answer. This was one of the real pain points we hit with Zhen (`198dc49` through `bfa35bf`).

### The Problem

```
User: "List 5 core services"
Zhen: "1. Wotan, 2. Sophia, 3. Captain, 4. Architect, 5. Micromanager"

User: "Elaborate on 2 and 4"
Zhen: "In the context of BibTeX entries, numbers 2 and 4..."  # WRONG — no conversation context
```

### The Solution

Keep a per-session history buffer and include recent turns in the prompt.

**Server side:**
```python
# In-memory conversation history per session
_histories = {}  # session_id -> [{'role': 'user', 'content': '...'}, ...]

def get_history(session_id):
    return _histories.get(session_id, [])

def add_to_history(session_id, role, content):
    if session_id not in _histories:
        _histories[session_id] = []
    _histories[session_id].append({'role': role, 'content': content})
    # Keep last 10 turn pairs
    if len(_histories[session_id]) > 20:
        _histories[session_id] = _histories[session_id][-20:]
```

**Client side:**
```javascript
const sessionId = crypto.randomUUID
    ? crypto.randomUUID()
    : Math.random().toString(36).slice(2) + Date.now().toString(36);

// Send session_id with every query
fetch('/api/v1/query', {
    method: 'POST',
    body: JSON.stringify({ question, session_id: sessionId }),
});
```

**Prompt construction** — build proper multi-turn format for Mistral:

```
<s>[INST] {system prompt + RAG context + question 1} [/INST]{answer 1}</s>[INST] {question 2} [/INST]{answer 2}</s>[INST] {current question} [/INST]
```

The first turn carries the system prompt and RAG context. Follow-up turns are lightweight — just the question and previous answer. The model sees the full conversation.

### Budget Management

With a 16K context window, you need to balance:
- System prompt (~200 tokens)
- RAG context (variable, up to ~4000 tokens)
- Conversation history (variable, up to ~4000 tokens)
- Space for the response (~500 tokens)

Work backwards from most recent turns, including as many as fit:

```python
history_budget = int(self.local_max_tokens * 1.5)  # in chars
for user_msg, asst_msg in reversed(pairs):
    pair_len = len(user_msg) + len(asst_msg) + 30
    if total_len + pair_len > history_budget:
        break
    included_pairs.insert(0, (user_msg, asst_msg))
    total_len += pair_len
```

---

## 9. Step 8: Teach and Remember

Two features that make your RAG system learn from usage without retraining:

### Teach: Add Knowledge at Runtime

FAISS supports `index.add()` — you can insert new vectors without rebuilding. This means users can paste text and Zhen learns it immediately:

```python
def add_to_corpus(self, text, source="user"):
    # Chunk the text
    chunks = split_into_chunks(text)

    # Embed
    embeddings = self.embedding_model.encode(chunks)
    embeddings = embeddings.astype('float32')

    # Add to live FAISS index (no restart needed)
    start_idx = self.index.ntotal
    self.index.add(embeddings)

    # Update corpus + ID map
    for i, chunk_text in enumerate(chunks):
        cid = f"taught_{source}_{int(time.time())}_{i}"
        self.id_map[str(start_idx + i)] = cid
        self.corpus[cid] = {'content': chunk_text, 'source': f'taught:{source}', 'type': 'taught'}

    return {'added': len(chunks)}
```

Expose it as an endpoint: `POST /api/v1/teach {"text": "...", "source": "user"}`.

### Remember: Cache Good Answers

When a user gets a good answer, they click "Remember." That Q&A pair is stored with an embedding. On future queries, check memories first — if a similar question was answered before, return the cached answer instantly.

```python
# Store: embed the question, save Q+A+embedding to PostgreSQL
embedding = model.encode(question).tobytes()
INSERT INTO memories (question, answer, embedding) VALUES (...)

# Retrieve: on each query, compare against stored embeddings
q_emb = model.encode(new_question)
for mem in memories:
    similarity = cosine_sim(q_emb, mem.embedding)
    if similarity > 0.9:
        return mem.answer  # instant, no LLM call needed
```

This creates a fast-path for repeated or similar questions.

---

## 10. Step 9: RAFT Fine-Tuning (Level Up)

Everything above is RAG — retrieval-augmented generation. The model itself hasn't learned anything about your domain. RAFT (Retrieval-Augmented Fine-Tuning) trains the model on Q&A pairs with retrieved context, so it learns:

- Your domain's terminology and patterns
- How to extract answers from retrieved chunks
- How to ignore distractor chunks (irrelevant retrieval results)

### Generate Training Data

Use your running RAG system to generate Q&A pairs:

```python
for chunk in sample_from_corpus(count=2000):
    # Ask the LLM to generate a question from this chunk
    prompt = f"Given this text, generate a question-answer pair:\n{chunk['content']}"
    qa = call_llm(prompt)

    # Retrieve context (source + distractors) via FAISS
    retrieved = rag.retrieve(qa['question'], k=5)

    training_entry = {
        'question': qa['question'],
        'answer': qa['answer'],
        'source_content': chunk['content'],
        'distractor_chunks': [r for r in retrieved if r['id'] != chunk['id']],
    }
```

### Format for Training

Convert to Mistral instruct format with source + distractor context shuffled:

```python
def format_training_example(entry):
    # Mix source chunk with distractors (random order)
    chunks = [entry['source_content']] + [d['content'] for d in entry['distractor_chunks'][:2]]
    random.shuffle(chunks)
    context = "\n\n".join(chunks)

    prompt = f"You are an assistant. Use the context to answer.\n\nContext:\n{context}\n\nQuestion: {entry['question']}"
    return f"<s>[INST] {prompt} [/INST] {entry['answer']}</s>"
```

The shuffle is important — the model shouldn't learn that the answer is always in the first chunk.

### Train with QLoRA

QLoRA lets you fine-tune a 7B model on a 12 GB GPU by quantizing the base model to 4-bit and only training small adapter layers:

```python
# Key config for 12GB VRAM
config = {
    "lora_rank": 16,           # small adapter
    "lora_alpha": 32,          # scaling factor
    "target_modules": ["q_proj", "k_proj", "v_proj", "o_proj"],  # attention layers
    "per_device_train_batch_size": 1,   # must be 1 for 4096 seq length on 12GB
    "gradient_accumulation_steps": 8,   # effective batch = 8
    "gradient_checkpointing": True,     # trade speed for VRAM
    "bf16": True,
    "num_train_epochs": 2,
}
```

### Deploy the Fine-Tuned Model

After training, merge LoRA weights back into the base model and quantize:

```bash
# 1. Merge LoRA → full model (~14 GB)
# 2. Convert to GGUF format
python3 llama.cpp/convert_hf_to_gguf.py merged-model/ --outfile model-f16.gguf --outtype f16

# 3. Quantize to Q5_K_M (~5 GB, production-ready)
llama.cpp/build/bin/llama-quantize model-f16.gguf model-q5km.gguf Q5_K_M

# 4. Swap into your start script
llama-server -m model-q5km.gguf -ngl 40 -c 16384 --port 20100
```

Same RAG pipeline, smarter model underneath.

---

## 11. Lessons Learned

These are things we discovered building Zhen over 16 commits across 2 days that aren't obvious from reading documentation.

### 1. Start with the default context window, then benchmark

We started at `-c 2048` because tutorials say so. Turns out our GPU could handle `-c 16384` with zero speed loss. 10 GB of VRAM was sitting idle. Always benchmark your specific hardware.

### 2. Chunking quality matters more than model quality

A mediocre model with well-chunked, relevant context beats a great model with poorly chunked noise. Spend time on your corpus preparation. Review what chunks your retriever actually returns for common queries.

### 3. Conversation history needs proper multi-turn formatting

We initially injected history as a single text blob. The model ignored it. Switching to proper Mistral multi-turn format (`[/INST]answer</s>[INST]question[/INST]`) made follow-ups work reliably.

### 4. The 400 error problem is a context window problem

When your RAG prompt gets too long, the inference server returns HTTP 400. The fix isn't truncation — it's a bigger context window (Step 6) or smarter context selection (fewer chunks, shorter excerpts).

### 5. Wikipedia was too big for RAM

29 GB of Wikipedia chunks can't fit in memory. We built a byte-offset index: store only the file offset of each chunk, then `seek()` to read individual chunks on demand. Cost: one disk read per Wikipedia hit. Savings: 29 GB of RAM.

### 6. Don't put secrets near the repo

We originally planned to load API keys from `.env` in the project directory. Security review caught it — one `git add .` away from leaking. Secrets belong in `~/.config/yourapp/secrets.env` with `chmod 600`, far from any git tree.

### 7. FAISS index.add() enables runtime learning

You don't need to rebuild the entire index when users teach the system new content. `faiss.IndexFlatL2` supports `index.add()` — embed the new text and append it to the live index. Instant, no restart.

### 8. The model doesn't know your codebase

Base Mistral-7B knows what "eBPF" is from its pretraining, but it doesn't know your service names, port numbers, or architecture decisions. RAG helps by providing context at query time. RAFT goes further by training the model on your domain. Both matter.

---

## 12. Architecture Reference

```
                        ┌──────────────────┐
                        │   Source Files    │
                        │  (.go .md .py)   │
                        └────────┬─────────┘
                                 │
                      ┌──────────▼──────────┐
                      │  01: Chunk + Index   │
                      │  (corpus/ring1.jsonl)│
                      └──────────┬──────────┘
                                 │
                      ┌──────────▼──────────┐
                      │  02: Embed (MiniLM)  │
                      │  → FAISS index       │
                      └──────────┬──────────┘
                                 │
          ┌──────────────────────┼────────────────────────┐
          │                      │                        │
┌─────────▼─────────┐ ┌─────────▼─────────┐  ┌──────────▼──────────┐
│  RAG Pipeline      │ │  QA Generation    │  │  Teach Endpoint     │
│  retrieve(query)   │ │  05: make pairs   │  │  index.add() live   │
│  generate(ctx)     │ │  for RAFT training│  │  no restart needed  │
└─────────┬─────────┘ └─────────┬─────────┘  └─────────────────────┘
          │                      │
┌─────────▼─────────┐ ┌─────────▼─────────┐
│  Flask Web UI      │ │  07: Format for   │
│  /api/v1/query     │ │  Mistral instruct │
│  chat + search     │ └─────────┬─────────┘
│  file upload       │           │
│  remember/forget   │ ┌─────────▼─────────┐
└───────────────────┘ │  08: QLoRA Train   │
                      │  (4-bit, LoRA r16) │
                      └─────────┬─────────┘
                                │
                      ┌─────────▼─────────┐
                      │  09: Merge + GGUF  │
                      │  → Q5_K_M quant    │
                      └─────────┬─────────┘
                                │
                      ┌─────────▼─────────┐
                      │  Production Model  │
                      │  Same pipeline,    │
                      │  smarter model     │
                      └────────────────────┘
```

### Ports (Zhen-specific)

| Service | Port | Purpose |
|---------|------|---------|
| llama-server | 20100 | LLM inference (HTTP, llama.cpp) |
| Zhen Web UI | 20103 | Flask app + static frontend |

### Files

| File | Purpose |
|------|---------|
| `corpus/ring1.jsonl` | Chunked source corpus (JSONL) |
| `index/ring1.index` | FAISS vector index |
| `index/ring1_ids.json` | Vector → chunk ID mapping |
| `scripts/zhen_rag.py` | RAG pipeline class |
| `zhen_app.py` | Flask web app |
| `static/index.html` | Frontend UI |
| `start-zhen.sh` | Startup orchestrator |
| `models/*.gguf` | LLM model files |

---

## Further Reading

- [RAFT paper](https://arxiv.org/abs/2403.10131) — "Adapting Language Model to Domain Specific RAG"
- [llama.cpp](https://github.com/ggerganov/llama.cpp) — Local LLM inference
- [sentence-transformers](https://www.sbert.net/) — Embedding models
- [FAISS](https://github.com/facebookresearch/faiss) — Vector similarity search
- [QLoRA paper](https://arxiv.org/abs/2305.14314) — Efficient fine-tuning

---

*Built during the Zhen sprint (March 15-16, 2026) for the Unheaded project. 16 commits, 2 days, zero cloud costs.*
