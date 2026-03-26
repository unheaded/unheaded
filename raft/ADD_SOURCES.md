# Adding Sources to Zhen AI

How to add new knowledge sources to Zhen's RAG corpus and rebuild the index.

---

## Quick Start (TL;DR)

```bash
# 1. Convert your source to plain text
pdftotext -layout myfile.pdf myfile.txt

# 2. Add it to the corpus
cd ~/tmp/unheaded/raft
source ~/.venv/zhen/bin/activate
./scripts/add-source.sh myfile.txt --ring 3 --type doc --source "My Document"

# 3. Rebuild the index (2-3 hours for full corpus)
python3 scripts/15_rebuild_corpus_v2.py
python3 scripts/16_embed_v2.py

# 4. Activate the new index
ln -sf v2.index index/active.index
ln -sf v2_ids.json index/active_ids.json

# 5. Restart Zhen web UI to pick up new index
pkill -f zhen_app.py
source ~/.venv/zhen/bin/activate
cd ~/tmp/unheaded/raft
nohup python3 zhen_app.py &>/tmp/zhen-webapp.log &
```

---

## Step-by-Step Guide

### Step 1: Prepare Your Source Material

Zhen ingests **plain text** in JSONL format. Convert your sources first.

#### From PDF
```bash
# Single file
pdftotext -layout input.pdf output.txt

# Batch (all PDFs in a directory)
for f in /path/to/pdfs/*.pdf; do
    pdftotext -layout "$f" "${f%.pdf}.txt"
done

# Requires: sudo apt install poppler-utils
```

#### From HTML
```bash
# Strip HTML tags (crude but effective)
python3 -c "
import html.parser, sys

class S(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.out = []
        self.skip = False
    def handle_starttag(self, t, a):
        if t in ('script','style','noscript'): self.skip = True
    def handle_endtag(self, t):
        if t in ('script','style','noscript'): self.skip = False
    def handle_data(self, d):
        if not self.skip: self.out.append(d)
    def text(self): return ' '.join(self.out)

s = S()
s.feed(open(sys.argv[1]).read())
print(s.text())
" input.html > output.txt
```

#### From EPUB
```bash
# Requires: sudo apt install calibre
ebook-convert input.epub output.txt
```

#### From Word/DOCX
```bash
# Requires: sudo apt install pandoc
pandoc input.docx -t plain -o output.txt
```

#### Already Plain Text
`.txt`, `.md`, `.rst`, `.go`, `.py`, `.rs`, `.c`, etc. — no conversion needed.

---

### Step 2: Add Source to Corpus

Use the helper script to chunk and append your text file(s) to the corpus:

```bash
cd ~/tmp/unheaded/raft
source ~/.venv/zhen/bin/activate

# Single file
./scripts/add-source.sh myfile.txt \
    --ring 3 \
    --type doc \
    --source "RFC 9999 - My Protocol"

# Directory of text files
./scripts/add-source.sh /path/to/texts/ \
    --ring 4 \
    --type paper \
    --source "Research Papers 2025"

# Multiple files
./scripts/add-source.sh file1.txt file2.txt file3.txt \
    --ring 2 \
    --type code \
    --source "Custom Code Snippets"
```

**Parameters:**

| Flag | Required | Description | Values |
|------|----------|-------------|--------|
| `--ring` | No (default: 3) | Knowledge ring | 1=Unheaded core, 2=tech foundation, 3=engineering, 4=general |
| `--type` | No (default: doc) | Content type | `doc`, `code`, `paper`, `rfc`, `skill`, `other` |
| `--source` | No (auto from filename) | Source label | Any string — shows up in retrieval results |
| `--chunk-size` | No (default: 2000) | Chars per chunk | Integer — larger = more context, fewer vectors |
| `--chunk-overlap` | No (default: 200) | Overlap between chunks | Integer — prevents losing context at boundaries |

**What this does:**
- Reads each text file
- Splits into chunks (2000 chars, 200 overlap)
- Appends JSONL records to `corpus/ring_all.jsonl`
- Reports how many chunks were added

**JSONL format** (each line):
```json
{
  "id": "custom_myfile_1711100000_0",
  "source": "My Document",
  "ring": 3,
  "type": "doc",
  "content": "The actual text content of this chunk...",
  "tokens": 487
}
```

---

### Step 3: Rebuild the FAISS Index

After adding sources, you must re-embed and rebuild the index.

#### Option A: Full Rebuild (recommended after adding many sources)

```bash
cd ~/tmp/unheaded/raft
source ~/.venv/zhen/bin/activate

# Rebuild corpus from all ring sources (optional — only if you modified ring1/ring234 directly)
python3 scripts/15_rebuild_corpus_v2.py

# Embed everything and build FAISS index
python3 scripts/16_embed_v2.py
```

**Runtime:** ~2-3 hours for ~1.5M chunks on this machine (14GB RAM, AMD 7700 XT).

Checkpoints are saved every 200K chunks, so if it crashes you can see progress.

#### Option B: Quick Test (embed just ring1)

```bash
python3 scripts/02_embeddings.py
```

This only embeds Ring 1 (~21K chunks, takes ~2 minutes). Good for testing but doesn't include your new sources unless they were added to ring1.jsonl.

---

### Step 4: Activate the New Index

Zhen looks for `active.index` and `active_ids.json` symlinks first, falling back to `ring1.index`.

```bash
cd ~/tmp/unheaded/raft/index

# Point active symlinks to v2 index
ln -sf v2.index active.index
ln -sf v2_ids.json active_ids.json

# Verify
ls -la active.*
```

---

### Step 5: Restart Zhen

The RAG pipeline loads the index on startup, so restart to pick up changes:

```bash
# Kill the web UI (inference server stays running)
pkill -f zhen_app.py

# Restart
source ~/.venv/zhen/bin/activate
cd ~/tmp/unheaded/raft
export ZHEN_LOCAL_MAX_TOKENS=16384
nohup python3 zhen_app.py &>/tmp/zhen-webapp.log &

# Verify
sleep 3
curl -s http://localhost:20103/health | python3 -m json.tool
curl -s http://localhost:20103/api/v1/stats | python3 -m json.tool
```

---

### Step 6: Verify Your Sources

```bash
# Search for content from your new source
curl -s http://localhost:20103/api/v1/search \
  -H "Content-Type: application/json" \
  -d '{"question": "something from your new document", "k": 5}' | python3 -m json.tool

# Check corpus stats
curl -s http://localhost:20103/api/v1/corpus/stats | python3 -m json.tool
```

---

## Live Teaching (No Rebuild Required)

For small additions, use the `/teach` endpoint — adds text to the live index immediately:

```bash
curl -s http://localhost:20103/api/v1/teach \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Your text content here. Can be multiple paragraphs.\n\nEach paragraph becomes a separate chunk.",
    "source": "manual-teaching"
  }' | python3 -m json.tool
```

This is great for quick additions but:
- Only persists to `ring_all.jsonl` (survives restart)
- Uses smaller chunks (500 chars vs 2000)
- Won't survive a full corpus rebuild unless you also add the source properly

---

## Corpus Inventory

Current sources in the corpus (as of March 2026):

| Ring | Source | File | Size | Chunks |
|------|--------|------|------|--------|
| 1 | Unheaded codebase | `ring1.jsonl` | 41 MB | ~21K |
| 1 | Kingdom skills | `ring1_skills.jsonl` | 703 KB | ~355 |
| 2-4 | GitHub repos, RFCs, papers, blogs | `ring234.jsonl` | 1.1 GB | ~573K |
| — | Stack Overflow | `stackoverflow.jsonl` | 13 GB | ~200K sampled |
| — | ServerFault | `serverfault.jsonl` | 204 MB | ~221K |
| — | Unix.SE | `unix_se.jsonl` | 215 MB | ~223K |
| — | Wikipedia | `wikipedia.jsonl` | 29 GB | on-demand |
| — | Wikipedia subsample | `wikipedia_subsample.jsonl` | 4.0 GB | ~265K |

**Combined:** `ring_all.jsonl` (2.2 GB, ~1.5M chunks)

---

## Architecture Notes

- **Embedding model:** `all-MiniLM-L6-v2` (384 dimensions, 22MB)
- **FAISS index type:** `IndexFlatIP` (exact inner product search, cosine similarity with normalized vectors)
- **Chunk size:** 2000 characters with 200 character overlap
- **Splitter:** `RecursiveCharacterTextSplitter` from langchain — splits on paragraph/sentence/word boundaries
- **Token estimation:** `len(text) // 4`

---

## Troubleshooting

### "pdftotext not found"
```bash
sudo apt install poppler-utils
```

### Index too large for RAM
The embedding step (16_embed_v2.py) processes in 50K-chunk batches to avoid OOM. With 46 GB swap this should not be an issue. If it still OOMs, reduce `PROCESS_BATCH` in the script.

### "active.index not found" / Zhen using old index
```bash
cd ~/tmp/unheaded/raft/index
ln -sf v2.index active.index
ln -sf v2_ids.json active_ids.json
# Then restart zhen_app.py
```

### Want to undo an addition
Remove the lines from `ring_all.jsonl` and re-embed. Or simpler: rebuild from source rings:
```bash
python3 scripts/15_rebuild_corpus_v2.py  # Regenerates ring_all.jsonl from ring sources
python3 scripts/16_embed_v2.py           # Re-embed
```
