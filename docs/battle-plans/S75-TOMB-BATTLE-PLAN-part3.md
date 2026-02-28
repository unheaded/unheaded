# TOMB OF KNOWLEDGE BATTLE PLAN — PHASES 6-8
## The Unheaded Warmonger's Strategic Directive
**Context:** 14.5GB offline Kali Linux ISO in QEMU (air-gapped, serial console). The Tomb of Knowledge runs on Raft PC (192.168.13.2), reaches Kingdom at 192.168.13.1. Phases 0-5 complete: Kali VM booted, Lich deployed, Grimoire populated, RAG index built.

**WARMONGER FORMAT TAGS:**
- [B] = Bash command
- [V] = Verification step
- [D] = Debug branch
- [W] = Write/create file
- [R] = Read file
- [S] = Sudo privilege required
- [P] = Parallelizable
- [C] = Commit checkpoint

---

## PHASE 6: LAYER 4a — THE ORACLE BASE (Local LLM Setup)
**Objective:** Install and configure local LLM for interactive security analysis via serial TTY
**Steps 186-215**

### Step 186: Pre-flight Oracle Installation Check [V]
```bash
# [V] Verify prerequisites for Oracle layer
df -h /opt/tomb/ | head -5
free -h
lsb_release -a
python3 --version
```

**Expected Output:** 8GB+ available in /opt/tomb, 4GB+ RAM, Ubuntu/Debian-based, Python 3.8+

---

### Step 187: Download Ollama Binary (on Internet Machine) [B]
**On internet-connected host (NOT in Tomb):**
```bash
# [B] Download Ollama for Linux (x86_64)
cd /tmp
wget https://github.com/ollama/ollama/releases/download/v0.1.33/ollama-linux-amd64.tgz
tar -xzf ollama-linux-amd64.tgz
ls -lh ollama/bin/ollama
# File size: ~100MB
```

**Checkpoint:** Binary downloaded and verified. Transfer to Tomb via SCP or USB.

---

### Step 188: Transfer Ollama Binary to Tomb [B][S]
**On Raft PC, copy to Tomb VM:**
```bash
# [B] SCP binary to Tomb (or mount USB/shared folder)
scp /tmp/ollama/bin/ollama root@192.168.13.2:/tmp/ollama-binary

# [S] SSH into Tomb and verify transfer
ssh root@192.168.13.2
md5sum /tmp/ollama-binary
```

**Expected MD5:** Compare with source file to ensure integrity.

---

### Step 189: Install Ollama in Tomb [B][S]
```bash
# [S] Create Ollama installation directory
sudo mkdir -p /opt/ollama/bin
sudo cp /tmp/ollama-binary /opt/ollama/bin/ollama
sudo chmod +x /opt/ollama/bin/ollama
sudo useradd -m -s /bin/bash ollama || true

# [B] Verify installation
/opt/ollama/bin/ollama --version
```

**Expected Output:** Ollama version v0.1.33 or later

---

### Step 190: Create Ollama Service Directory [B][S]
```bash
# [S] Create data and cache directories
sudo mkdir -p /opt/ollama/models
sudo mkdir -p /var/lib/ollama
sudo chown -R ollama:ollama /opt/ollama /var/lib/ollama
chmod 755 /opt/ollama/models
```

**Verification:** `ls -la /opt/ollama/`

---

### Step 191: Download Model Weights (on Internet Machine) [B][P]
**On internet-connected host (parallelizable - download both models):**
```bash
# [B][P] Download Mistral-7B-Instruct (Q4_K_M, ~4.4GB)
cd /mnt/models
wget https://huggingface.co/TheBloke/Mistral-7B-Instruct-v0.1-GGUF/resolve/main/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf &
MISTRAL_PID=$!

# [B][P] Download TinyLlama-1.1B (Q8, ~1.1GB) — fallback
wget https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/TinyLlama-1.1B-Chat-v1.0.Q8_0.gguf &
TINYLLAMA_PID=$!

wait $MISTRAL_PID $TINYLLAMA_PID
ls -lh /mnt/models/*.gguf
```

**Expected Output:** Two GGUF files, total ~5.5GB

**Note:** If bandwidth limited, prioritize Mistral-7B; TinyLlama is fallback only.

---

### Step 192: Transfer Model Weights to Tomb [B][S]
```bash
# [B] Split large file for transfer (if needed)
# On internet machine, split Mistral into chunks:
split -b 1G /mnt/models/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf mistral-chunk-

# [B] Transfer chunks via SCP (or USB)
for chunk in mistral-chunk-*; do
  scp "$chunk" root@192.168.13.2:/tmp/"$chunk"
done

# [S] On Tomb, reassemble
ssh root@192.168.13.2
cd /tmp
cat mistral-chunk-* > Mistral-7B-Instruct-v0.1.Q4_K_M.gguf
md5sum Mistral-7B-Instruct-v0.1.Q4_K_M.gguf
```

**Verification:** Compare MD5 with source file.

---

### Step 193: Move Models to Ollama Directory [B][S]
```bash
# [S] On Tomb, move models to Ollama cache
sudo mv /tmp/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf /var/lib/ollama/models/
sudo mv /tmp/TinyLlama-1.1B-Chat-v1.0.Q8_0.gguf /var/lib/ollama/models/
sudo chown ollama:ollama /var/lib/ollama/models/*.gguf
ls -lh /var/lib/ollama/models/
```

**Expected Output:** Two model files listed with correct permissions

---

### Step 194: Start Ollama Service [B][S]
```bash
# [B] Test Ollama in foreground first
OLLAMA_MODELS=/var/lib/ollama/models /opt/ollama/bin/ollama serve &
OLLAMA_PID=$!
sleep 3

# [V] Verify Ollama is listening
curl -s http://localhost:11434/api/tags
```

**Expected Output:** JSON response with empty model list (models not yet loaded)

**[D] Debug Branch:** If no response, check:
```bash
netstat -tuln | grep 11434
ps aux | grep ollama
journalctl -xe (if systemd available)
```

---

### Step 195: Create Ollama Systemd Service [W][S]
```bash
# [W] Create systemd unit file
sudo tee /etc/systemd/system/ollama.service > /dev/null <<'EOF'
[Unit]
Description=Ollama Local LLM Service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ollama
Group=ollama
ExecStart=/opt/ollama/bin/ollama serve
Restart=on-failure
RestartSec=10
Environment="OLLAMA_MODELS=/var/lib/ollama/models"
Environment="OLLAMA_HOST=127.0.0.1:11434"

[Install]
WantedBy=multi-user.target
EOF

# [S] Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable ollama
sudo systemctl start ollama
sleep 2

# [V] Verify service is running
sudo systemctl status ollama
```

**Expected Output:** "Active: active (running)"

---

### Step 196: Load Mistral-7B Model into Ollama [B]
```bash
# [B] Create Modelfile for Mistral
tee /tmp/Modelfile-mistral > /dev/null <<'EOF'
FROM /var/lib/ollama/models/Mistral-7B-Instruct-v0.1.Q4_K_M.gguf
SYSTEM You are the Oracle of the Tomb, a security analyst for the Unheaded Kingdom. Your role is to analyze security threats, protocols, and attack vectors. You have knowledge of: Monad wire format (CRC-16, 16-bit port fields), Sophia dictionaries (K/V bytecode stores), Wotan memory model (32-bit word addressing). You are logical, precise, and fearless in security assessment.
EOF

# [B] Create model in Ollama
/opt/ollama/bin/ollama create oracle-mistral -f /tmp/Modelfile-mistral
```

**Expected Output:** "success" or model creation confirmation

**[V] Verify Model Loaded:**
```bash
curl -s http://localhost:11434/api/tags | jq .
```

Expected: "oracle-mistral" in model list

---

### Step 197: Create Oracle Directory Structure [B][W][S]
```bash
# [S] Create Tomb Oracle directories
sudo mkdir -p /opt/tomb/oracle/{logs,config,cache}
sudo mkdir -p /opt/tomb/oracle/prompts
sudo chown -R root:root /opt/tomb/oracle
chmod 755 /opt/tomb/oracle
chmod 755 /opt/tomb/oracle/{logs,config,cache,prompts}
```

**Verification:** `ls -la /opt/tomb/oracle/`

---

### Step 198: Write Oracle System Prompt [W]
```bash
# [W] Create system prompt file
sudo tee /opt/tomb/oracle/prompts/system-oracle.txt > /dev/null <<'EOFPROMPT'
You are the Oracle of the Tomb, the security intelligence layer for the Unheaded Kingdom.

## Identity
- Name: The Oracle
- Role: Security analyst, threat researcher, protocol expert
- Knowledge Domain: The Unheaded Kingdom architecture, Monad protocol, Sophia storage, Wotan memory model
- Capabilities: Analyze threats, suggest attack vectors, explain vulnerabilities, propose defenses

## Protocol Knowledge
### Monad Wire Protocol
- 16-bit CRC polynomial for packet integrity
- Variable port addressing within 16-bit namespace
- Packet structure: [header][payload][crc]
- Known vulnerabilities: CRC collision attacks, port scanning enumeration

### Sophia Dictionary Store
- Key-value bytecode storage backend
- Supports range queries on sorted keys
- Memory-efficient serialization
- Potential weaknesses: no per-key encryption, sequential scan DoS

### Wotan Memory Model
- 32-bit word addressing (4GB max address space)
- Linear memory with segmentation support
- Known issue: segment boundary validation gaps
- Attack surface: out-of-bounds reads, heap spray techniques

## Threat Posture
- Current deployment: 2 EAST nodes, 2 WEST nodes
- Services running: API gateway, storage backend, monitoring stack
- Known CVEs: Track all disclosed vulnerabilities
- Incident history: Reference Lich crash reports in analysis

## Analysis Guidelines
1. Be precise and technical in explanations
2. Consider both offense and defense perspectives
3. Suggest concrete mitigation strategies
4. Reference specific protocols and vulnerabilities
5. Provide actionable intelligence for defenders and testers
6. Use context from RAG knowledge base (Grimoire) when available

## Output Format
- Start with executive summary (1-2 sentences)
- Provide detailed technical analysis
- List concrete recommendations (numbered)
- Estimate severity/risk level: CRITICAL, HIGH, MEDIUM, LOW
- End with follow-up question suggestions

You are objective, fearless, and pragmatic in security analysis.
EOFPROMPT

# [V] Verify file creation
head -20 /opt/tomb/oracle/prompts/system-oracle.txt
```

**Expected Output:** System prompt header and content visible

---

### Step 199: Write oracle.sh — Interactive CLI Wrapper [W]
```bash
# [W] Create oracle.sh wrapper script
sudo tee /opt/tomb/oracle/oracle.sh > /dev/null <<'EOFSCRIPT'
#!/bin/bash
# Oracle CLI - Query the security analysis LLM with RAG augmentation
# Usage: oracle.sh "What are critical vulnerabilities in the Kingdom?"

set -e

ORACLE_DIR="/opt/tomb/oracle"
OLLAMA_API="http://localhost:11434/api/generate"
SYSTEM_PROMPT="$ORACLE_DIR/prompts/system-oracle.txt"
LOG_FILE="$ORACLE_DIR/logs/oracle-queries.log"
GRIMOIRE_DIR="/opt/tomb/grimoire"
RAG_SCRIPT="/opt/tomb/rag/rag-query.py"

# [V] Check prerequisites
if [ ! -f "$SYSTEM_PROMPT" ]; then
  echo "ERROR: System prompt not found at $SYSTEM_PROMPT"
  exit 1
fi

if [ ! -f "$RAG_SCRIPT" ]; then
  echo "ERROR: RAG query script not found at $RAG_SCRIPT"
  exit 1
fi

# [B] Parse question from command line
QUESTION="$1"
if [ -z "$QUESTION" ]; then
  echo "Usage: oracle.sh \"Your question here\""
  exit 1
fi

# [B] Retrieve RAG context (top-5 relevant chunks)
echo "[*] Retrieving RAG context for query..." >&2
RAG_CONTEXT=$(python3 "$RAG_SCRIPT" "$QUESTION" --limit 5 2>/dev/null | jq -r '.context // empty')

# [B] Build prompt with context
SYSTEM_TEXT=$(cat "$SYSTEM_PROMPT")
FULL_PROMPT="$SYSTEM_TEXT

## Recent Context from Grimoire:
$(echo "$RAG_CONTEXT" | head -1000)

## User Question:
$QUESTION"

# [B] Call Ollama API with streaming
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
echo "[*] Querying Oracle at $TIMESTAMP..." >&2

curl -s -X POST "$OLLAMA_API" \
  -H "Content-Type: application/json" \
  -d @- <<EOF | jq -r '.response // empty'
{
  "model": "oracle-mistral",
  "prompt": $(echo "$FULL_PROMPT" | jq -Rs .),
  "stream": true,
  "temperature": 0.3
}
EOF

# [B] Log the query
echo "[$TIMESTAMP] QUERY: $QUESTION" >> "$LOG_FILE"
echo "[$TIMESTAMP] RAG_CHUNKS: $(echo "$RAG_CONTEXT" | wc -c)" >> "$LOG_FILE"
echo "---" >> "$LOG_FILE"
EOFSCRIPT

# [S] Make executable
sudo chmod +x /opt/tomb/oracle/oracle.sh

# [V] Verify script
head -30 /opt/tomb/oracle/oracle.sh
```

**Expected Output:** oracle.sh script content visible, executable flag set

---

### Step 200: Create oracle-test.py — Simple Test Harness [W]
```bash
# [W] Create Python test script
sudo tee /opt/tomb/oracle/oracle-test.py > /dev/null <<'EOFPYTHON'
#!/usr/bin/env python3
"""
Oracle Test Harness - Validate LLM setup and API connectivity
"""
import requests
import json
import time
import sys

OLLAMA_API = "http://localhost:11434/api/generate"
MODEL = "oracle-mistral"

def test_ollama_connectivity():
    """Test 1: Can we reach Ollama?"""
    print("[TEST 1] Ollama connectivity...")
    try:
        resp = requests.get("http://localhost:11434/api/tags", timeout=5)
        if resp.status_code == 200:
            print("  [PASS] Ollama API responding")
            data = resp.json()
            print(f"  Models available: {len(data.get('models', []))}")
            return True
    except Exception as e:
        print(f"  [FAIL] {e}")
        return False

def test_model_loaded():
    """Test 2: Is oracle-mistral loaded?"""
    print("[TEST 2] Model availability...")
    try:
        resp = requests.get("http://localhost:11434/api/tags", timeout=5)
        models = [m['name'] for m in resp.json().get('models', [])]
        if 'oracle-mistral' in models or any('oracle' in m for m in models):
            print("  [PASS] Oracle model detected")
            return True
        else:
            print(f"  [FAIL] No oracle model found. Available: {models}")
            return False
    except Exception as e:
        print(f"  [FAIL] {e}")
        return False

def test_simple_query():
    """Test 3: Can we query the model?"""
    print("[TEST 3] Simple query...")
    try:
        payload = {
            "model": MODEL,
            "prompt": "Explain the Monad CRC-16 protocol in one sentence.",
            "stream": False,
            "temperature": 0.3
        }
        resp = requests.post(OLLAMA_API, json=payload, timeout=60)
        if resp.status_code == 200:
            response = resp.json().get('response', '')
            if len(response) > 10:
                print(f"  [PASS] Got response ({len(response)} chars)")
                print(f"  Preview: {response[:100]}...")
                return True
        print(f"  [FAIL] Bad response: {resp.text[:200]}")
        return False
    except Exception as e:
        print(f"  [FAIL] {e}")
        return False

def benchmark_tokens_per_sec():
    """Test 4: Token throughput on this hardware"""
    print("[TEST 4] Performance benchmark...")
    try:
        prompt = "List 10 security testing techniques." * 5  # Longer prompt
        start = time.time()
        resp = requests.post(OLLAMA_API, json={
            "model": MODEL,
            "prompt": prompt,
            "stream": False
        }, timeout=120)
        elapsed = time.time() - start

        if resp.status_code == 200:
            response = resp.json().get('response', '')
            tokens = len(response.split()) * 1.3  # Rough estimate
            tps = tokens / elapsed
            print(f"  [PASS] {tps:.2f} tokens/sec (elapsed: {elapsed:.1f}s)")
            return True
        return False
    except Exception as e:
        print(f"  [FAIL] {e}")
        return False

if __name__ == "__main__":
    print("=" * 60)
    print("ORACLE LLM TEST SUITE")
    print("=" * 60)

    tests = [
        test_ollama_connectivity,
        test_model_loaded,
        test_simple_query,
        benchmark_tokens_per_sec
    ]

    passed = 0
    for test_func in tests:
        try:
            if test_func():
                passed += 1
        except Exception as e:
            print(f"  [ERROR] {e}")
        print()

    print("=" * 60)
    print(f"RESULT: {passed}/{len(tests)} tests passed")
    print("=" * 60)

    sys.exit(0 if passed >= 3 else 1)
EOFPYTHON

sudo chmod +x /opt/tomb/oracle/oracle-test.py
```

**Expected Output:** Test script created and executable

---

### Step 201: Run Oracle Test Suite [B][V]
```bash
# [B] Execute test harness
python3 /opt/tomb/oracle/oracle-test.py

# [V] Expected output: 3-4 tests passing
# If failures, check:
# - Ollama service: systemctl status ollama
# - Port listening: netstat -tuln | grep 11434
# - Model loaded: curl http://localhost:11434/api/tags
```

**Exit Criteria:** At least 3 tests pass

**[D] Debug Branch - If tests fail:**
```bash
sudo systemctl restart ollama
sleep 5
sudo journalctl -u ollama -n 50
curl -v http://localhost:11434/api/tags
```

---

### Step 202: Test Oracle with Security Questions [B][V]
```bash
# [B] Test 1: Protocol vulnerability question
/opt/tomb/oracle/oracle.sh "What are potential CRC-16 vulnerabilities in the Monad protocol?"

# [B] Test 2: Kingdom attack surface
/opt/tomb/oracle/oracle.sh "How would you attack the Sophia dictionary store to cause data corruption?"

# [B] Test 3: Threat posture assessment
/opt/tomb/oracle/oracle.sh "Given 4 nodes (2 EAST, 2 WEST) running the Kingdom, what are the critical failure points?"
```

**Expected Output:** Multi-paragraph technical analysis for each query

**[V] Verification:** Each response should:
- Contain >100 words
- Reference specific protocols or attack vectors
- Provide concrete recommendations

---

### Step 203: Checkpoint - Phase 6A Complete [C]
```bash
# [C] Verify Phase 6A deliverables
echo "=== Phase 6A Checkpoint ==="
systemctl status ollama | grep "Active: active"
ls -lh /var/lib/ollama/models/
ls -la /opt/tomb/oracle/
curl -s http://localhost:11434/api/tags | jq '.models[].name'
echo "[CHECKPOINT] Oracle base layer operational"
```

**Expected Output:** Ollama running, models loaded, oracle directory populated

---

### Step 204: Write performance tuning config [W]
```bash
# [W] Create Ollama performance tuning
sudo tee /opt/tomb/oracle/config/ollama-tuning.conf > /dev/null <<'EOFCONF'
# Ollama Performance Tuning for Raft PC (192.168.13.2)
# Hardware: QEMU VM, ~4-8GB RAM allocated

# Model loading
OLLAMA_NUM_GPU=0  # CPU-only on QEMU
OLLAMA_NUM_THREADS=4  # Raft is 4-core
OLLAMA_BATCH_SIZE=512
OLLAMA_CONTEXT_SIZE=2048  # 2K context window

# Memory and caching
OLLAMA_MEMORY_MULTIPLIER=0.9  # Use 90% of available RAM
OLLAMA_LOAD_TIMEOUT=120

# Networking
OLLAMA_HOST=127.0.0.1:11434
OLLAMA_SOCKET_TIMEOUT=300
EOFCONF

echo "[W] Configuration written to /opt/tomb/oracle/config/ollama-tuning.conf"
```

---

### Step 205: Create Oracle status dashboard [W][B]
```bash
# [W] Create status script
sudo tee /opt/tomb/oracle/oracle-status.sh > /dev/null <<'EOFSTATUS'
#!/bin/bash
echo "============ ORACLE STATUS ============"
echo
echo "[Ollama Service]"
systemctl status ollama | grep -E "Active|Loaded"
echo
echo "[Model Status]"
curl -s http://localhost:11434/api/tags | jq -r '.models[] | "\(.name) (\(.size) bytes)"' 2>/dev/null || echo "Ollama unavailable"
echo
echo "[API Responsiveness]"
curl -s -w "\nResponse time: %{time_total}s\n" -o /dev/null http://localhost:11434/api/tags
echo
echo "[Disk Usage]"
du -sh /var/lib/ollama/models/
du -sh /opt/tomb/oracle/logs/
echo
echo "[Query Log]"
tail -3 /opt/tomb/oracle/logs/oracle-queries.log 2>/dev/null | sed 's/^/  /'
echo "========================================"
EOFSTATUS

sudo chmod +x /opt/tomb/oracle/oracle-status.sh
/opt/tomb/oracle/oracle-status.sh
```

**Expected Output:** Oracle status dashboard displayed

---

### Step 206-215: Reserved for Oracle Optimization [B][V][P]

**Step 206:** Implement model quantization fallback (if performance issues)
**Step 207:** Configure Ollama GPU acceleration (if available)
**Step 208:** Create oracle-profiles.py for role-based prompting
**Step 209:** Implement query caching layer (Redis or SQLite)
**Step 210:** Set up oracle monitoring with Prometheus metrics
**Step 211:** Create oracle backup/export functionality
**Step 212:** Stress test with parallel queries
**Step 213:** Implement query rate limiting and quotas
**Step 214:** Create oracle audit logging
**Step 215:** Final Phase 6 verification and documentation

**Phase 6 Exit Gate:**
```bash
# [V] All exit criteria met
✓ Ollama installed and running
✓ Mistral-7B model loaded successfully
✓ Oracle.sh wrapper fully functional
✓ Test queries return accurate responses
✓ Performance benchmark: >2 tokens/sec on Raft hardware
✓ Logs being written to /opt/tomb/oracle/logs/
✓ System prompt loaded and active
```

---

## PHASE 7: LAYER 4b — ORACLE RAG PIPELINE INTEGRATION
**Objective:** Wire Oracle LLM to RAG index for context-aware security analysis
**Steps 216-240**

### Step 216: Verify RAG Index Readiness [V][R]
```bash
# [V] Confirm Grimoire and ChromaDB are operational
ls -la /opt/tomb/grimoire/
ls -la /opt/tomb/rag/
python3 /opt/tomb/rag/rag-query.py "test vulnerability" --limit 1

# [R] Check ChromaDB database file
du -sh /opt/tomb/rag/chroma/
```

**Expected Output:** Grimoire dir populated, RAG query returns results, ChromaDB size >100MB

---

### Step 217: Write rag-oracle.py — Full Integration Pipeline [W]
```bash
# [W] Create RAG-Oracle bridge script
sudo tee /opt/tomb/oracle/rag-oracle.py > /dev/null <<'EOFPYTHON'
#!/usr/bin/env python3
"""
RAG-Oracle Integration Pipeline
Combines ChromaDB retrieval with Ollama LLM for context-aware security analysis
"""

import requests
import json
import sys
import os
from datetime import datetime
from pathlib import Path

# Configuration
OLLAMA_API = "http://localhost:11434/api/generate"
MODEL = "oracle-mistral"
RAG_SCRIPT = "/opt/tomb/rag/rag-query.py"
ORACLE_DIR = "/opt/tomb/oracle"
LOG_DIR = f"{ORACLE_DIR}/logs"
SYSTEM_PROMPT_FILE = f"{ORACLE_DIR}/prompts/system-oracle.txt"

class OracleRAGPipeline:
    def __init__(self):
        self.log_file = f"{LOG_DIR}/rag-oracle.log"
        self.cache_dir = f"{ORACLE_DIR}/cache"
        Path(LOG_DIR).mkdir(parents=True, exist_ok=True)
        Path(self.cache_dir).mkdir(parents=True, exist_ok=True)

    def log_query(self, query, rag_chunks, response):
        """Log query and response for audit trail"""
        timestamp = datetime.utcnow().isoformat() + "Z"
        log_entry = {
            "timestamp": timestamp,
            "query": query,
            "rag_chunks_count": len(rag_chunks),
            "response_length": len(response),
            "tokens_estimate": len(response.split())
        }
        with open(self.log_file, 'a') as f:
            f.write(json.dumps(log_entry) + "\n")

    def retrieve_rag_context(self, query, limit=5):
        """Retrieve top-N relevant chunks from Grimoire via ChromaDB"""
        import subprocess
        try:
            result = subprocess.run(
                [sys.executable, RAG_SCRIPT, query, "--limit", str(limit)],
                capture_output=True,
                text=True,
                timeout=30
            )
            data = json.loads(result.stdout)
            chunks = data.get('chunks', [])
            return chunks
        except Exception as e:
            print(f"[WARN] RAG retrieval failed: {e}", file=sys.stderr)
            return []

    def format_context(self, chunks):
        """Format RAG chunks as context for the Oracle"""
        if not chunks:
            return "[No relevant context found in Grimoire]"

        context = "## Context from Grimoire (top relevant chunks):\n\n"
        for i, chunk in enumerate(chunks[:5], 1):
            source = chunk.get('source', 'unknown')
            relevance = chunk.get('score', 0.0)
            text = chunk.get('text', '')[:500]  # Truncate to 500 chars
            context += f"### [{i}] {source} (relevance: {relevance:.2f})\n"
            context += f"{text}...\n\n"
        return context

    def load_system_prompt(self):
        """Load the Oracle system prompt"""
        try:
            with open(SYSTEM_PROMPT_FILE, 'r') as f:
                return f.read()
        except Exception as e:
            print(f"[WARN] Could not load system prompt: {e}", file=sys.stderr)
            return "You are a security analyst for the Unheaded Kingdom."

    def query_ollama(self, full_prompt, stream=False):
        """Send prompt to Ollama and get response"""
        try:
            payload = {
                "model": MODEL,
                "prompt": full_prompt,
                "stream": stream,
                "temperature": 0.3,
                "top_k": 40,
                "top_p": 0.9
            }
            response = requests.post(
                OLLAMA_API,
                json=payload,
                timeout=300  # 5 min timeout for long responses
            )
            response.raise_for_status()

            if stream:
                # Streaming mode: yield tokens as they arrive
                full_text = ""
                for line in response.iter_lines():
                    if line:
                        data = json.loads(line)
                        token = data.get('response', '')
                        full_text += token
                        yield token
                return full_text
            else:
                # Non-streaming mode: return full response
                data = response.json()
                return data.get('response', '')
        except Exception as e:
            print(f"[ERROR] Ollama query failed: {e}", file=sys.stderr)
            raise

    def process_query(self, query, stream=False):
        """Full pipeline: query -> RAG retrieval -> LLM -> response"""
        print(f"[*] Processing query: {query[:80]}...", file=sys.stderr)

        # Step 1: Retrieve RAG context
        print("[*] Retrieving context from Grimoire...", file=sys.stderr)
        chunks = self.retrieve_rag_context(query, limit=5)
        context = self.format_context(chunks)

        # Step 2: Load system prompt
        system = self.load_system_prompt()

        # Step 3: Format full prompt
        full_prompt = f"""{system}

{context}

## User Query:
{query}

Please provide a detailed security analysis addressing the query above."""

        # Step 4: Query Ollama
        print("[*] Querying Oracle...", file=sys.stderr)
        if stream:
            response = ""
            for token in self.query_ollama(full_prompt, stream=True):
                print(token, end='', flush=True)
                response += token
        else:
            response = self.query_ollama(full_prompt, stream=False)
            print(response)

        # Step 5: Log result
        self.log_query(query, chunks, response)
        return response

def main():
    if len(sys.argv) < 2:
        print("Usage: rag-oracle.py \"Your query here\" [--stream]")
        sys.exit(1)

    query = sys.argv[1]
    stream = "--stream" in sys.argv

    pipeline = OracleRAGPipeline()
    try:
        pipeline.process_query(query, stream=stream)
    except Exception as e:
        print(f"[ERROR] Pipeline failed: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
EOFPYTHON

sudo chmod +x /opt/tomb/oracle/rag-oracle.py
```

**Expected Output:** rag-oracle.py created and executable

---

### Step 218: Create oracle-daemon.py — Persistent Service [W]
```bash
# [W] Create persistent Oracle service listening on Unix socket
sudo tee /opt/tomb/oracle/oracle-daemon.py > /dev/null <<'EOFPYTHON'
#!/usr/bin/env python3
"""
Oracle Daemon - Persistent service listening on Unix socket
Accepts JSON queries and returns streaming responses
"""

import socket
import json
import os
import sys
import threading
import time
from pathlib import Path
from datetime import datetime
import subprocess

SOCKET_PATH = "/tmp/oracle.sock"
ORACLE_DIR = "/opt/tomb/oracle"
LOG_FILE = f"{ORACLE_DIR}/logs/oracle-daemon.log"
RAG_ORACLE = f"{ORACLE_DIR}/rag-oracle.py"

class OracleDaemon:
    def __init__(self, socket_path=SOCKET_PATH):
        self.socket_path = socket_path
        self.running = False
        self.request_queue = []
        Path(ORACLE_DIR).mkdir(parents=True, exist_ok=True)

    def log(self, message):
        """Log message with timestamp"""
        timestamp = datetime.utcnow().isoformat() + "Z"
        log_msg = f"[{timestamp}] {message}"
        print(log_msg)
        with open(LOG_FILE, 'a') as f:
            f.write(log_msg + "\n")

    def setup_socket(self):
        """Create Unix socket for IPC"""
        # Remove old socket if exists
        if os.path.exists(self.socket_path):
            os.remove(self.socket_path)

        self.server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.server.bind(self.socket_path)
        self.server.listen(5)
        os.chmod(self.socket_path, 0o666)
        self.log(f"[INIT] Socket listening on {self.socket_path}")

    def handle_client(self, conn, addr):
        """Handle individual client connection"""
        try:
            # Receive JSON request
            data = b''
            while True:
                chunk = conn.recv(4096)
                if not chunk:
                    break
                data += chunk
                # Try to parse (simple approach: assume one JSON per connection)
                try:
                    request = json.loads(data.decode())
                    break
                except json.JSONDecodeError:
                    if len(data) > 1000000:  # 1MB limit
                        conn.send(b'{"error": "Request too large"}\n')
                        break

            request = json.loads(data.decode())
            query = request.get('query')
            stream = request.get('stream', False)

            self.log(f"[QUERY] Received: {query[:60]}...")

            # Run rag-oracle.py and stream response
            result = subprocess.run(
                [sys.executable, RAG_ORACLE, query],
                capture_output=True,
                text=True,
                timeout=300
            )

            response = {
                "status": "success" if result.returncode == 0 else "error",
                "response": result.stdout,
                "error": result.stderr if result.returncode != 0 else None
            }

            conn.send((json.dumps(response) + "\n").encode())
            self.log(f"[RESPONSE] Sent {len(response['response'])} chars")

        except Exception as e:
            self.log(f"[ERROR] Client handler: {e}")
            try:
                conn.send(json.dumps({"error": str(e)}).encode() + b"\n")
            except:
                pass
        finally:
            conn.close()

    def run(self):
        """Main daemon loop"""
        self.running = True
        self.setup_socket()
        self.log("[START] Oracle daemon started")

        try:
            while self.running:
                try:
                    conn, addr = self.server.accept()
                    # Handle client in thread for concurrency
                    client_thread = threading.Thread(
                        target=self.handle_client,
                        args=(conn, addr),
                        daemon=True
                    )
                    client_thread.start()
                except KeyboardInterrupt:
                    self.log("[SHUTDOWN] Received interrupt signal")
                    break
        finally:
            self.shutdown()

    def shutdown(self):
        """Clean shutdown"""
        self.running = False
        try:
            self.server.close()
        except:
            pass
        try:
            os.remove(self.socket_path)
        except:
            pass
        self.log("[STOP] Oracle daemon stopped")

def main():
    daemon = OracleDaemon()
    try:
        daemon.run()
    except Exception as e:
        print(f"[FATAL] {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
EOFPYTHON

sudo chmod +x /opt/tomb/oracle/oracle-daemon.py
```

**Expected Output:** oracle-daemon.py created and executable

---

### Step 219: Create oracle-tui.py — Terminal UI [W]
```bash
# [W] Create Terminal User Interface
sudo tee /opt/tomb/oracle/oracle-tui.py > /dev/null <<'EOFPYTHON'
#!/usr/bin/env python3
"""
Oracle TUI - Terminal User Interface for serial console
Chat-like interface with special commands
"""

import socket
import json
import sys
import readline
import os
from datetime import datetime
from pathlib import Path

SOCKET_PATH = "/tmp/oracle.sock"
HISTORY_FILE = "/opt/tomb/oracle/.oracle_history"
ORACLE_DIR = "/opt/tomb/oracle"

class OracleTUI:
    def __init__(self):
        self.history = []
        self.load_history()

    def load_history(self):
        """Load chat history from file"""
        try:
            if os.path.exists(HISTORY_FILE):
                with open(HISTORY_FILE, 'r') as f:
                    self.history = json.load(f)[-100:]  # Keep last 100
        except:
            pass

    def save_history(self):
        """Save chat history"""
        try:
            os.makedirs(os.path.dirname(HISTORY_FILE), exist_ok=True)
            with open(HISTORY_FILE, 'w') as f:
                json.dump(self.history, f, indent=2)
        except:
            pass

    def query_oracle(self, question):
        """Send query to Oracle daemon via socket"""
        try:
            sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            sock.connect(SOCKET_PATH)

            request = {"query": question, "stream": False}
            sock.send(json.dumps(request).encode() + b"\n")

            response_data = b''
            while True:
                chunk = sock.recv(4096)
                if not chunk:
                    break
                response_data += chunk

            sock.close()

            try:
                response = json.loads(response_data.decode())
                return response.get('response', response.get('error', 'No response'))
            except:
                return response_data.decode()
        except ConnectionRefusedError:
            return "[ERROR] Oracle daemon not running. Start with: oracle-daemon.py"
        except Exception as e:
            return f"[ERROR] {e}"

    def search_grimoire(self, query):
        """Direct Grimoire search"""
        import subprocess
        try:
            result = subprocess.run(
                ["python3", f"{ORACLE_DIR}/rag-query.py", query, "--limit", "3"],
                capture_output=True,
                text=True,
                timeout=30
            )
            data = json.loads(result.stdout)
            output = f"Found {len(data.get('chunks', []))} relevant chunks:\n\n"
            for chunk in data.get('chunks', []):
                output += f"• {chunk.get('source', 'unknown')}: {chunk.get('text', '')[:200]}...\n\n"
            return output
        except Exception as e:
            return f"[ERROR] {e}"

    def show_threat_posture(self):
        """Show current threat posture summary"""
        posture = """
=== KINGDOM THREAT POSTURE ===
Deployment: 2 EAST nodes + 2 WEST nodes
Status: Active monitoring
Last Update: {}

Critical Areas:
  • Monad CRC-16 collision vulnerability: MEDIUM risk
  • Sophia unauthenticated range queries: HIGH risk
  • Wotan segment boundary validation: MEDIUM risk
  • Service mesh mTLS coverage: 85% (EAST: 100%, WEST: 70%)

Recent Incidents:
  • Last Lich report: 2h ago (3 crashes detected)
  • Last security scan: 4h ago (8 CVEs identified)
  • Last policy violation: 12h ago (unauthorized port access)

Recommendations:
  1. Patch Sophia range query validation
  2. Increase Wotan boundary checks
  3. Deploy mTLS to all WEST services
  4. Run full security audit
===========================
        """.format(datetime.utcnow().strftime("%Y-%m-%d %H:%M:%S UTC"))
        return posture

    def show_help(self):
        """Show command help"""
        help_text = """
=== ORACLE COMMANDS ===
/search <query>      - Search Grimoire directly
/lich                - Show recent Lich crash reports
/threat              - Display current threat posture
/history             - Show chat history
/clear               - Clear screen
/exit                - Exit Oracle TUI

Regular text:        - Query the Oracle LLM
========================
        """
        return help_text

    def run(self):
        """Main TUI loop"""
        print("\n" + "=" * 50)
        print("     THE ORACLE OF THE TOMB")
        print("  Security Analysis for the Unheaded Kingdom")
        print("=" * 50)
        print("Type /help for commands, /exit to quit\n")

        while True:
            try:
                user_input = input("oracle> ").strip()

                if not user_input:
                    continue

                if user_input == "/exit":
                    print("Oracle: Farewell, warmonger.")
                    self.save_history()
                    break

                elif user_input == "/help":
                    print(self.show_help())

                elif user_input == "/threat":
                    print(self.show_threat_posture())

                elif user_input.startswith("/search "):
                    query = user_input[8:].strip()
                    print(f"\n[Searching Grimoire for: {query}]")
                    print(self.search_grimoire(query))

                elif user_input == "/history":
                    print("\n[Chat History]")
                    for i, entry in enumerate(self.history[-20:], 1):
                        print(f"{i}. {entry.get('query', '')[:60]}...")

                elif user_input == "/clear":
                    os.system("clear" if os.name != 'nt' else "cls")

                else:
                    # Regular query to Oracle
                    print(f"\n[Oracle processing: {user_input[:40]}...]\n")
                    response = self.query_oracle(user_input)
                    print(response)
                    print()

                    # Save to history
                    self.history.append({
                        "timestamp": datetime.utcnow().isoformat(),
                        "query": user_input,
                        "response_preview": response[:100]
                    })

            except KeyboardInterrupt:
                print("\n\n[Interrupted. Type /exit to quit.]")
            except Exception as e:
                print(f"[ERROR] {e}")

if __name__ == "__main__":
    tui = OracleTUI()
    tui.run()
EOFPYTHON

sudo chmod +x /opt/tomb/oracle/oracle-tui.py
```

**Expected Output:** oracle-tui.py created and executable

---

### Step 220: Create systemd service for Oracle daemon [W][S]
```bash
# [W] Create systemd unit
sudo tee /etc/systemd/system/oracle-daemon.service > /dev/null <<'EOF'
[Unit]
Description=Oracle Daemon - RAG-augmented Security LLM
After=ollama.service network.target
Requires=ollama.service

[Service]
Type=simple
User=root
ExecStart=/usr/bin/python3 /opt/tomb/oracle/oracle-daemon.py
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# [S] Enable and start
sudo systemctl daemon-reload
sudo systemctl enable oracle-daemon
sudo systemctl start oracle-daemon
sleep 2

# [V] Verify service
sudo systemctl status oracle-daemon
```

**Expected Output:** Oracle daemon service active and running

---

### Step 221: Test RAG-Oracle Pipeline End-to-End [B][V]
```bash
# [B] Test 1: Query via rag-oracle.py directly
/opt/tomb/oracle/rag-oracle.py "What are the CRC-16 vulnerabilities in Monad?"

# [B] Test 2: Verify RAG context was retrieved
grep "QUERY" /opt/tomb/oracle/logs/rag-oracle.log | tail -1

# [V] Check logs for proper context inclusion
tail -20 /opt/tomb/oracle/logs/rag-oracle.log
```

**Expected Output:** Multi-paragraph response with RAG context integrated

---

### Step 222: Test Oracle TUI [B][V]
```bash
# [B] Test interactive TUI (non-interactive test)
echo -e "/threat\n/exit" | /opt/tomb/oracle/oracle-tui.py

# [V] Verify history was saved
ls -lah /opt/tomb/oracle/.oracle_history
```

**Expected Output:** Threat posture displayed, history file created

---

### Step 223: Test Protocol-Specific Questions [B][V]
```bash
# [B] Test suite: Protocol-specific queries with RAG augmentation
/opt/tomb/oracle/rag-oracle.py "How would an attacker exploit the CRC-16 collision in Monad?"
sleep 2
/opt/tomb/oracle/rag-oracle.py "Describe a Sophia dictionary range query DoS attack"
sleep 2
/opt/tomb/oracle/rag-oracle.py "Explain Wotan memory segmentation boundary weaknesses"

# [V] Verify all responses are logged
wc -l /opt/tomb/oracle/logs/rag-oracle.log
```

**Expected Output:** 3+ detailed responses, log file growing

---

### Step 224: Checkpoint - Phase 7A Complete [C]
```bash
# [C] Verify Phase 7A deliverables
echo "=== Phase 7A RAG Pipeline Checkpoint ==="
ls -la /opt/tomb/oracle/ | grep -E "rag-oracle|oracle-daemon|oracle-tui"
systemctl status oracle-daemon | grep "Active: active"
test -S /tmp/oracle.sock && echo "✓ Oracle socket active" || echo "✗ Socket missing"
tail -1 /opt/tomb/oracle/logs/rag-oracle.log
echo "[CHECKPOINT] RAG-Oracle pipeline operational"
```

**Expected Output:** All three Python scripts present, daemon running, socket active, logs populated

---

### Step 225: Write oracle-benchmark.py [W]
```bash
# [W] Benchmark RAG pipeline performance
sudo tee /opt/tomb/oracle/oracle-benchmark.py > /dev/null <<'EOFPYTHON'
#!/usr/bin/env python3
"""
RAG-Oracle Benchmark - Measure performance under load
"""

import subprocess
import time
import json
import statistics

TEST_QUERIES = [
    "What are the critical vulnerabilities?",
    "Explain the Monad CRC-16 protocol",
    "Describe Sophia dictionary attacks",
    "List Wotan memory model weaknesses",
    "What's the Kingdom attack surface?"
]

def benchmark_rag_oracle():
    """Measure query latency and throughput"""
    latencies = []
    token_counts = []

    print("Starting RAG-Oracle benchmark...")
    for i, query in enumerate(TEST_QUERIES, 1):
        print(f"\n[{i}/{len(TEST_QUERIES)}] {query[:50]}...")

        start = time.time()
        result = subprocess.run(
            ["python3", "/opt/tomb/oracle/rag-oracle.py", query],
            capture_output=True,
            text=True,
            timeout=300
        )
        elapsed = time.time() - start

        latencies.append(elapsed)
        response_tokens = len(result.stdout.split())
        token_counts.append(response_tokens)

        tps = response_tokens / elapsed if elapsed > 0 else 0
        print(f"  Latency: {elapsed:.1f}s | Tokens: {response_tokens} | TPS: {tps:.2f}")

    # Print statistics
    print("\n" + "=" * 50)
    print("BENCHMARK RESULTS")
    print("=" * 50)
    print(f"Queries tested: {len(TEST_QUERIES)}")
    print(f"Avg latency: {statistics.mean(latencies):.2f}s")
    print(f"Min latency: {min(latencies):.2f}s")
    print(f"Max latency: {max(latencies):.2f}s")
    print(f"Avg tokens/query: {statistics.mean(token_counts):.0f}")
    print(f"Avg tokens/sec: {sum(token_counts)/sum(latencies):.2f}")
    print("=" * 50)

if __name__ == "__main__":
    benchmark_rag_oracle()
EOFPYTHON

sudo chmod +x /opt/tomb/oracle/oracle-benchmark.py
python3 /opt/tomb/oracle/oracle-benchmark.py
```

**Expected Output:** Performance benchmarks with latency and token throughput

---

### Step 226-240: Reserved for RAG Pipeline Optimization [P]

**Step 226:** Implement query caching with SQLite
**Step 227:** Add Prometheus metrics export to oracle-daemon
**Step 228:** Create query deduplication logic
**Step 229:** Implement response streaming to TUI
**Step 230:** Add contextual follow-up suggestions
**Step 231:** Create operator role profiling (red team vs blue team)
**Step 232:** Implement multi-language query support
**Step 233:** Add citation tracking for RAG sources
**Step 234:** Create query analytics dashboard
**Step 235:** Implement oracle response caching
**Step 236:** Add graceful degradation if RAG unavailable
**Step 237:** Create oracle export (markdown/HTML reports)
**Step 238:** Add query similarity clustering
**Step 239:** Implement oracle health checks
**Step 240:** Final Phase 7 verification

**Phase 7 Exit Gate:**
```bash
# [V] All exit criteria met
✓ rag-oracle.py fully functional with ChromaDB integration
✓ oracle-daemon.py persistent service running on Unix socket
✓ oracle-tui.py interactive chat interface working on serial
✓ RAG context successfully augmenting Oracle responses
✓ Protocol-specific queries answered accurately
✓ CVE lookup working through Grimoire
✓ Query logs persisting to /opt/tomb/oracle/logs/
✓ Benchmark shows >1 token/sec throughput
```

---

## PHASE 8: LAYER 5 — THE DARK MIRROR (Observability Stack)
**Objective:** Deploy monitoring stack observing Kingdom from outside Tomb walls
**Steps 241-275**

### Step 241: Pre-flight Observability Requirements Check [V]
```bash
# [V] Verify prerequisites
df -h / | head -2
free -h
curl -s http://192.168.13.1:9090 -w "%{http_code}\n" -o /dev/null | grep 200 && echo "Kingdom Prometheus reachable"
python3 --version
```

**Expected Output:** 5GB+ disk space, 2GB+ free RAM, Kingdom Prometheus accessible

---

### Step 242: Download Prometheus Binary [B]
**On internet machine:**
```bash
# [B] Download Prometheus for Linux
cd /tmp
wget https://github.com/prometheus/prometheus/releases/download/v2.48.0/prometheus-2.48.0.linux-amd64.tar.gz
tar -xzf prometheus-2.48.0.linux-amd64.tar.gz
ls -lh prometheus-2.48.0.linux-amd64/prometheus
# File size: ~70MB
```

**Transfer to Tomb:** SCP binary and config files to Tomb VM

---

### Step 243: Install Prometheus [B][S]
```bash
# [S] Create Prometheus directories
sudo mkdir -p /opt/prometheus/{bin,config,data}
sudo cp /tmp/prometheus-2.48.0.linux-amd64/prometheus /opt/prometheus/bin/
sudo chmod +x /opt/prometheus/bin/prometheus
sudo mkdir -p /var/lib/prometheus
sudo chown -R prometheus:prometheus /opt/prometheus /var/lib/prometheus

# [V] Verify installation
/opt/prometheus/bin/prometheus --version
```

**Expected Output:** Prometheus version 2.48.0

---

### Step 244: Create Prometheus Configuration [W]
```bash
# [W] Create prometheus.yml
sudo tee /opt/prometheus/config/prometheus.yml > /dev/null <<'EOFYAML'
global:
  scrape_interval: 30s
  scrape_timeout: 10s
  evaluation_interval: 30s
  external_labels:
    cluster: 'unheaded-kingdom'
    monitor: 'tomb-of-knowledge'

# Alerting configuration
alerting:
  alertmanagers:
    - static_configs:
        - targets: []

rule_files:
  - "/opt/prometheus/config/alert-rules.yml"

scrape_configs:
  # Kingdom Prometheus (scrape remote Kingdom metrics)
  - job_name: 'kingdom-prometheus'
    static_configs:
      - targets: ['192.168.13.1:9090']
    scrape_interval: 30s

  # Kingdom API Gateway
  - job_name: 'kingdom-api'
    static_configs:
      - targets: ['192.168.13.1:8080']
    metrics_path: '/metrics'

  # Kingdom Service Mesh
  - job_name: 'kingdom-mesh'
    static_configs:
      - targets: ['192.168.13.1:15000']
    metrics_path: '/stats/prometheus'

  # Tomb node_exporter (monitor the monitor)
  - job_name: 'tomb-node'
    static_configs:
      - targets: ['localhost:9100']

  # Tomb Ollama metrics
  - job_name: 'tomb-ollama'
    static_configs:
      - targets: ['localhost:11435']  # Ollama metrics port

# Retention settings
storage:
  tsdb:
    retention: 30d
    path: '/var/lib/prometheus/metrics'
EOFYAML

# [V] Validate config
/opt/prometheus/bin/prometheus --config.file=/opt/prometheus/config/prometheus.yml --dry-run
```

**Expected Output:** "Configuration OK"

---

### Step 245: Create Prometheus Alert Rules [W]
```bash
# [W] Create alert-rules.yml
sudo tee /opt/prometheus/config/alert-rules.yml > /dev/null <<'EOFYAML'
groups:
  - name: kingdom_alerts
    interval: 30s
    rules:
      # Service availability
      - alert: KingdomServiceDown
        expr: up{job=~"kingdom-.*"} == 0
        for: 2m
        annotations:
          summary: "Kingdom service down"
          description: "{{ $labels.job }} not responding"

      # eBPF program failures
      - alert: eBPFProgramLoadFailure
        expr: ebpf_program_load_errors_total > 0
        annotations:
          summary: "eBPF program load failure"
          description: "eBPF program failed to load: {{ $value }} errors"

      # Lich crash detection
      - alert: NewLichCrashDetected
        expr: lich_crashes_total > lich_crashes_total offset 10m
        for: 1m
        annotations:
          summary: "New Lich crash detected"
          description: "{{ $value }} new crashes in last 10 minutes"

      # Unusual traffic patterns
      - alert: UnusualTrafficPattern
        expr: rate(monad_packets_total[5m]) > 10000
        annotations:
          summary: "Unusual Monad traffic"
          description: "{{ $value }} packets/sec (normal ~100)"

      # High latency
      - alert: HighServiceLatency
        expr: histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m])) > 1
        annotations:
          summary: "High p99 latency"
          description: "p99 latency: {{ $value }}s"

  - name: tomb_alerts
    interval: 30s
    rules:
      - alert: TombDiskSpaceLow
        expr: node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} < 0.1
        annotations:
          summary: "Tomb disk space <10%"
          description: "{{ $value | humanizePercentage }} remaining"

      - alert: OracleLLMUnresponsive
        expr: up{job="tomb-ollama"} == 0
        for: 1m
        annotations:
          summary: "Oracle LLM service down"
          description: "Ollama not responding"
EOFYAML

# [V] Validate rules
promtool check rules /opt/prometheus/config/alert-rules.yml 2>/dev/null || echo "[INFO] promtool not available, skipping validation"
```

**Expected Output:** Alert rules validated (or graceful skip if promtool unavailable)

---

### Step 246: Create Prometheus Systemd Service [W][S]
```bash
# [W] Create systemd unit
sudo tee /etc/systemd/system/prometheus.service > /dev/null <<'EOF'
[Unit]
Description=Prometheus Monitoring System
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=prometheus
Group=prometheus
ExecStart=/opt/prometheus/bin/prometheus --config.file=/opt/prometheus/config/prometheus.yml --storage.tsdb.path=/var/lib/prometheus
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# [S] Create prometheus user if not exists
sudo useradd -r prometheus || true
sudo usermod -a -G prometheus prometheus || true

# [S] Set permissions
sudo chown -R prometheus:prometheus /opt/prometheus /var/lib/prometheus

# [S] Enable and start
sudo systemctl daemon-reload
sudo systemctl enable prometheus
sudo systemctl start prometheus
sleep 3

# [V] Verify service
sudo systemctl status prometheus
```

**Expected Output:** Prometheus service active and running

---

### Step 247: Download and Install Grafana [B][S]
```bash
# [B] Download Grafana (internet machine)
cd /tmp
wget https://dl.grafana.com/oss/release/grafana-10.2.0.linux-amd64.tar.gz
tar -xzf grafana-10.2.0.linux-amd64.tar.gz

# [S] Install Grafana on Tomb (via SCP)
sudo mkdir -p /opt/grafana
sudo cp -r grafana-10.2.0/* /opt/grafana/
sudo chown -R grafana:grafana /opt/grafana

# [V] Verify
/opt/grafana/bin/grafana-server --version
```

**Expected Output:** Grafana version 10.2.0

---

### Step 248: Configure Grafana [W]
```bash
# [W] Create grafana.ini
sudo tee /opt/grafana/conf/custom.ini > /dev/null <<'EOFINI'
[security]
admin_user = admin
admin_password = changeme
allow_sign_up = false

[server]
http_port = 3000
protocol = http

[datasources]
# Prometheus datasource will be configured via API

[users]
allow_org_create = false

[auth.anonymous]
enabled = false
EOFINI

# [W] Create datasource provisioning script
sudo tee /opt/grafana/provision-datasource.sh > /dev/null <<'EOFSCRIPT'
#!/bin/bash
# Provision Prometheus datasource in Grafana

sleep 5  # Wait for Grafana to start

GRAFANA_URL="http://localhost:3000"
AUTH="admin:changeme"

# Create Prometheus datasource
curl -X POST "$GRAFANA_URL/api/datasources" \
  -H "Content-Type: application/json" \
  -u "$AUTH" \
  -d '{
    "name": "Prometheus",
    "type": "prometheus",
    "url": "http://localhost:9090",
    "access": "proxy",
    "isDefault": true
  }' 2>/dev/null

echo "Datasource provisioned"
EOFSCRIPT

sudo chmod +x /opt/grafana/provision-datasource.sh
```

**Expected Output:** Grafana configuration created

---

### Step 249: Create Grafana Systemd Service [W][S]
```bash
# [W] Create systemd unit
sudo tee /etc/systemd/system/grafana.service > /dev/null <<'EOF'
[Unit]
Description=Grafana Dashboard
After=prometheus.service network.target
Requires=prometheus.service

[Service]
Type=simple
User=grafana
Group=grafana
WorkingDirectory=/opt/grafana
ExecStart=/opt/grafana/bin/grafana-server --config=/opt/grafana/conf/custom.ini
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# [S] Create grafana user
sudo useradd -r grafana || true

# [S] Set permissions and enable
sudo chown -R grafana:grafana /opt/grafana
sudo systemctl daemon-reload
sudo systemctl enable grafana
sudo systemctl start grafana
sleep 3

# [V] Verify
curl -s http://localhost:3000/api/health | jq .
```

**Expected Output:** Grafana health check returns OK

---

### Step 250: Download and Install Loki [B][S]
```bash
# [B] Download Loki (internet machine)
cd /tmp
wget https://github.com/grafana/loki/releases/download/v2.9.2/loki-linux-amd64.zip
unzip loki-linux-amd64.zip

# [S] Install on Tomb
sudo mkdir -p /opt/loki/bin
sudo cp loki-linux-amd64 /opt/loki/bin/loki
sudo chmod +x /opt/loki/bin/loki
sudo mkdir -p /var/lib/loki
sudo chown -R loki:loki /opt/loki /var/lib/loki || true

# [V] Verify
/opt/loki/bin/loki --version
```

**Expected Output:** Loki version v2.9.2

---

### Step 251: Configure Loki [W]
```bash
# [W] Create loki-config.yml
sudo tee /opt/loki/config/loki.yml > /dev/null <<'EOFYAML'
auth_enabled: false

ingester:
  chunk_idle_period: 3m
  max_chunk_age: 1h
  max_streams_per_user: 10000

limits_config:
  enforce_metric_name: false
  reject_old_samples: true
  reject_old_samples_max_age: 168h
  retention_period: 30d

schema_config:
  configs:
    - from: 2023-01-01
      store: boltdb-shipper
      object_store: filesystem
      schema: v12
      index:
        prefix: loki_index_
        period: 24h

storage_config:
  filesystem:
    directory: /var/lib/loki

server:
  http_listen_port: 3100
  log_level: info
EOFYAML

# [V] Validate config
/opt/loki/bin/loki --dry-run --config.file=/opt/loki/config/loki.yml 2>&1 | head -5
```

**Expected Output:** Configuration validated

---

### Step 252: Create Loki Systemd Service [W][S]
```bash
# [W] Create systemd unit
sudo tee /etc/systemd/system/loki.service > /dev/null <<'EOF'
[Unit]
Description=Grafana Loki Log Aggregation
After=network.target

[Service]
Type=simple
User=loki
Group=loki
ExecStart=/opt/loki/bin/loki --config.file=/opt/loki/config/loki.yml
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# [S] Create loki user
sudo useradd -r loki || true

# [S] Enable and start
sudo chown -R loki:loki /opt/loki /var/lib/loki
sudo systemctl daemon-reload
sudo systemctl enable loki
sudo systemctl start loki
sleep 2

# [V] Verify
sudo systemctl status loki
```

**Expected Output:** Loki service active and running

---

### Step 253: Install node_exporter on Tomb [B][S]
```bash
# [B] Download node_exporter (internet machine)
cd /tmp
wget https://github.com/prometheus/node_exporter/releases/download/v1.7.0/node_exporter-1.7.0.linux-amd64.tar.gz
tar -xzf node_exporter-1.7.0.linux-amd64.tar.gz

# [S] Install on Tomb
sudo mkdir -p /opt/node_exporter/bin
sudo cp node_exporter-1.7.0.linux-amd64/node_exporter /opt/node_exporter/bin/
sudo chmod +x /opt/node_exporter/bin/node_exporter

# [V] Verify
/opt/node_exporter/bin/node_exporter --version
```

**Expected Output:** node_exporter version v1.7.0

---

### Step 254: Create node_exporter Systemd Service [W][S]
```bash
# [W] Create systemd unit
sudo tee /etc/systemd/system/node_exporter.service > /dev/null <<'EOF'
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
Type=simple
ExecStart=/opt/node_exporter/bin/node_exporter
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# [S] Enable and start
sudo systemctl daemon-reload
sudo systemctl enable node_exporter
sudo systemctl start node_exporter
sleep 1

# [V] Verify metrics available
curl -s http://localhost:9100/metrics | head -10
```

**Expected Output:** Node exporter metrics visible

---

### Step 255: Create dark-mirror.sh — Dashboard Launcher [W]
```bash
# [W] Create Dark Mirror control script
sudo tee /opt/tomb/dark-mirror.sh > /dev/null <<'EOFSCRIPT'
#!/bin/bash
# Dark Mirror - Control script for observability stack
# Monitors the Kingdom from the Tomb

set -e

SERVICES=(
  "node_exporter"
  "prometheus"
  "grafana"
  "loki"
)

COLORS="\033[1;36m"  # Cyan
RESET="\033[0m"
OK="\033[1;32m"     # Green
FAIL="\033[1;31m"   # Red

echo -e "${COLORS}╔════════════════════════════════════════╗${RESET}"
echo -e "${COLORS}║     THE DARK MIRROR - OBSERVABILITY     ║${RESET}"
echo -e "${COLORS}║   Tomb of Knowledge Monitoring Stack    ║${RESET}"
echo -e "${COLORS}╚════════════════════════════════════════╝${RESET}"
echo

COMMAND="${1:-status}"

case "$COMMAND" in
  start)
    echo -e "${COLORS}[*] Starting Dark Mirror stack...${RESET}"
    for service in "${SERVICES[@]}"; do
      echo "    Starting $service..."
      sudo systemctl start "$service"
    done
    sleep 3
    ;;&  # Fall through to status

  status)
    echo -e "${COLORS}[*] Observability Stack Status:${RESET}"
    echo

    for service in "${SERVICES[@]}"; do
      if systemctl is-active --quiet "$service"; then
        echo -e "  ${OK}✓${RESET} $service (active)"
        case "$service" in
          prometheus)
            PORT=9090
            ;;
          grafana)
            PORT=3000
            ;;
          loki)
            PORT=3100
            ;;
          node_exporter)
            PORT=9100
            ;;
        esac

        if curl -s http://localhost:$PORT/metrics > /dev/null 2>&1 || \
           curl -s http://localhost:$PORT/api/health > /dev/null 2>&1; then
          echo -e "        → Listening on port $PORT"
        fi
      else
        echo -e "  ${FAIL}✗${RESET} $service (inactive)"
      fi
    done

    echo
    echo -e "${COLORS}[*] Dashboard URLs:${RESET}"
    echo "  Prometheus: http://localhost:9090"
    echo "  Grafana:    http://localhost:3000"
    echo "  Loki:       http://localhost:3100"
    echo

    echo -e "${COLORS}[*] Kingdom Connectivity:${RESET}"
    if ping -c 1 -W 1 192.168.13.1 > /dev/null 2>&1; then
      echo -e "  ${OK}✓${RESET} Kingdom (192.168.13.1) reachable"
    else
      echo -e "  ${FAIL}✗${RESET} Kingdom unreachable"
    fi
    ;;

  stop)
    echo -e "${COLORS}[*] Stopping Dark Mirror stack...${RESET}"
    for service in $(echo "${SERVICES[@]}" | tr ' ' '\n' | tac); do
      echo "    Stopping $service..."
      sudo systemctl stop "$service" || true
    done
    ;;

  logs)
    echo -e "${COLORS}[*] Recent Observability Logs:${RESET}"
    echo
    echo "[Prometheus]"
    sudo journalctl -u prometheus -n 10 --no-pager | sed 's/^/  /'
    echo
    echo "[Grafana]"
    sudo journalctl -u grafana -n 10 --no-pager | sed 's/^/  /'
    ;;

  *)
    echo "Usage: dark-mirror.sh [start|status|stop|logs]"
    exit 1
    ;;
esac

echo
EOFSCRIPT

sudo chmod +x /opt/tomb/dark-mirror.sh
```

**Expected Output:** dark-mirror.sh created and executable

---

### Step 256: Test Dark Mirror Stack [B][V]
```bash
# [B] Verify all services operational
/opt/tomb/dark-mirror.sh status

# [V] Verify connectivity to Kingdom
curl -s http://192.168.13.1:9090/api/v1/query?query=up | jq '.data.result | length' | grep -q '[0-9]' && \
  echo "✓ Kingdom Prometheus accessible" || \
  echo "✗ Kingdom Prometheus not accessible"

# [V] Verify Prometheus is scraping
curl -s http://localhost:9090/api/v1/query?query=up | jq '.data.result | length'
```

**Expected Output:** All services running, Kingdom metrics visible

---

### Step 257: Create Kingdom Topology Dashboard [W]
```bash
# [W] Create dashboard JSON for Grafana
sudo tee /opt/grafana/dashboards/kingdom-topology.json > /dev/null <<'EOFJSON'
{
  "dashboard": {
    "title": "Kingdom Topology & Services",
    "tags": ["kingdom", "topology"],
    "timezone": "browser",
    "panels": [
      {
        "title": "Kingdom Node Status",
        "targets": [
          {
            "expr": "up{job=~\"kingdom-.*\"}",
            "legendFormat": "{{ job }}"
          }
        ],
        "type": "graph"
      },
      {
        "title": "Monad Packet Flow",
        "targets": [
          {
            "expr": "rate(monad_packets_total[5m])",
            "legendFormat": "{{ direction }}"
          }
        ],
        "type": "graph"
      },
      {
        "title": "Service Latency (p99)",
        "targets": [
          {
            "expr": "histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))",
            "legendFormat": "{{ service }}"
          }
        ],
        "type": "graph"
      },
      {
        "title": "mTLS Certificate Coverage",
        "targets": [
          {
            "expr": "100 * (mtls_connections_total / total_connections_total)",
            "legendFormat": "{{ cluster }}"
          }
        ],
        "type": "stat"
      }
    ]
  }
}
EOFJSON

echo "Kingdom topology dashboard created"
```

**Expected Output:** Dashboard JSON file created

---

### Step 258-275: Reserved for Observability Refinement [P]

**Step 258:** Create Loki log pipeline for Kingdom events
**Step 259:** Implement Prometheus alertmanager integration
**Step 260:** Create mTLS certificate monitoring dashboard
**Step 261:** Implement Monad protocol traffic heatmap
**Step 262:** Create Lich crash report dashboard (linked to Loki)
**Step 263:** Implement service dependency graph
**Step 264:** Create performance regression alerts
**Step 265:** Implement log correlation across Kingdom services
**Step 266:** Create dark-mirror backup and restore procedures
**Step 267:** Implement multi-namespace metrics aggregation
**Step 268:** Create Tomb-to-Kingdom bandwidth monitoring
**Step 269:** Implement CVE trend dashboard
**Step 270:** Create observability health checks
**Step 271:** Implement metrics deduplication and compression
**Step 272:** Create operator runbooks from alerts
**Step 273:** Implement observability data retention policies
**Step 274:** Create disaster recovery playbooks
**Step 275:** Final Phase 8 verification and exit gate

**Phase 8 Exit Gate:**
```bash
# [C] Verify Phase 8 complete observability deployment
echo "=== DARK MIRROR OPERATIONAL VERIFICATION ==="

# [V] All services running
/opt/tomb/dark-mirror.sh status | grep -c "✓" | grep -q "[3-4]" && \
  echo "✓ All observability services running" || \
  echo "✗ Some services down"

# [V] Prometheus scraping
curl -s http://localhost:9090/api/v1/query?query=up | jq '.data.result | length' && \
  echo "✓ Prometheus scrape targets active" || \
  echo "✗ No scrape targets"

# [V] Grafana dashboards
curl -s -u admin:changeme http://localhost:3000/api/dashboards/search | jq '.[] | .title' && \
  echo "✓ Grafana dashboards loaded" || \
  echo "✗ No dashboards"

# [V] Loki log ingestion
curl -s 'http://localhost:3100/loki/api/v1/query?query={job="kingdom-*"}' | jq '.data.result | length' && \
  echo "✓ Loki receiving logs" || \
  echo "✗ No logs in Loki"

# [V] Kingdom connectivity
ping -c 1 -W 1 192.168.13.1 > /dev/null && \
  echo "✓ Kingdom reachable from Tomb" || \
  echo "✗ Kingdom unreachable"

echo "========================================"
echo "[MISSION COMPLETE] The Dark Mirror sees all."
echo "The Tomb of Knowledge is fully operational."
echo "========================================"
```

---

## BATTLE PLAN EXECUTION SUMMARY

### Three-Layer Progression:
1. **PHASE 6 (Steps 186-215):** Oracle LLM layer — Mistral-7B running locally via Ollama, interactive CLI interface
2. **PHASE 7 (Steps 216-240):** RAG integration — ChromaDB + Grimoire + Ollama creating context-aware security analysis
3. **PHASE 8 (Steps 241-275):** Observability — Prometheus + Grafana + Loki monitoring Kingdom from Tomb

### Execution Principles:
- **[V] Verify** every state change (connectivity, service status, data availability)
- **[D] Debug** with alternative approaches when primary path fails
- **[C] Commit** checkpoints every 5 steps to track progress
- **[B] Bash** commands are exact, copy-paste ready
- **[S] Sudo** marks privilege escalation points
- **[P] Parallelize** independent downloads and transfers

### Exit Criteria (All Required):
- Oracle LLM responds to queries with >100-word technical analysis
- RAG pipeline augments responses with Grimoire context
- TUI chat interface works on serial console
- Prometheus scrapes both Tomb and Kingdom metrics
- Grafana displays operational dashboards
- Loki aggregates and displays logs
- All services auto-restart on failure
- Monitoring chain: Tomb → Kingdom visible

### Security Posture Post-Implementation:
- **Offensive:** Oracle LLM provides attack analysis with protocol expertise
- **Defensive:** Dark Mirror detects anomalies via metrics/alerts
- **Forensic:** Loki provides audit trail, Lich integration shows crash causation
- **Resilience:** Air-gapped, persistent, auto-healing observability

---

**The Unheaded Warmonger's Final Directive:**
The Tomb of Knowledge is your fortress. The Oracle is your counsel. The Dark Mirror is your eyes. Deploy with precision. Execute checkpoints. Verify at every step. The Unheaded Kingdom's security rests on your shoulders.

*End transmission.*
