# Zhen Demo Script

**Duration**: 10 minutes
**Audience**: Job fair / technical hiring managers
**Setup**: Zhen running on localhost:20103, inference on localhost:20100

---

## Opening (30 seconds)

> "This is Zhen, an AI assistant that knows every line of a 385,000-line production codebase. It runs a Mistral-7B model locally on GPU with retrieval-augmented generation over 21,000 indexed code chunks. No cloud APIs. No data leaves the machine."

**Action**: Show the Zhen UI at localhost:20103. Point out the header ("ZHEN — Unheaded AI Champion") and the "Online" status indicator.

---

## Live Demo (5 minutes)

### Question 1: Big Picture (60s)

**Ask**: "What is Unheaded?"

**Talk through**: Show the answer populating. Point out:
- Answer quality — it synthesizes from multiple sources
- Source attribution at the bottom
- Response time and token count in the metadata

### Question 2: Architecture Depth (60s)

**Ask**: "What are the 6 layers of the Unheaded architecture?"

**Talk through**: Zhen retrieves architectural docs and returns structured layer information. Highlight that this is coming from actual codebase documentation, not a general LLM hallucination.

### Question 3: Specific Technical Detail (60s)

**Ask**: "What port does Wotan use?"

**Talk through**: Show it returns exact port numbers (18000 HTTP, 18001 gRPC). This demonstrates retrieval precision — the model is grounded in real code.

### Question 4: Protocol Knowledge (60s)

**Ask**: "What is the Monad wire format size?"

**Talk through**: Answer should mention 20 bytes, version 0x01, frozen. This is deep protocol knowledge from spec documents indexed into the vector store.

### Question 5: Cross-Cutting (60s)

**Ask**: "How does eBPF tracing work in Unheaded?"

**Talk through**: Show it pulls together information from multiple files — Rust eBPF programs, Go userspace, trace collector service. Highlight the breadth of retrieval.

---

## Technical Highlights (2 minutes)

Switch to a brief architecture overview. Key points:

1. **Local inference**: Mistral-7B running via llama.cpp on GPU (port 20100). No API keys, no cloud dependency, no data exfiltration.

2. **RAG pipeline**: Corpus prepared from codebase -> sentence-transformers embeddings (all-MiniLM-L6-v2) -> FAISS vector index (21,110 vectors). Top-k retrieval feeds context into the LLM prompt.

3. **Full stack**: Python Flask app serves both the web UI and the RAG API. Three API endpoints — /api/v1/query (RAG), /api/v1/search (semantic search), /api/v1/stats (index info).

4. **Built in a day**: Corpus preparation, embedding generation, FAISS indexing, inference server setup, Flask app, web UI — all built and deployed in one session.

5. **Production codebase**: The indexed corpus is from Unheaded, a real 385K-LOC infrastructure platform with 10 microservices, eBPF tracing, and a custom wire protocol.

---

## Closing (1 minute)

> "What you've seen is baseline RAG — retrieve and generate. The next step is RAFT training: we have a 503K-entry training corpus ready to fine-tune the model specifically on this codebase. That will move Zhen from 'search and summarize' to 'reason about the code.'"

> "The full system — inference server, vector index, web UI, and integration tests — is reproducible from the scripts in the raft/ directory."

**If asked about next steps**:
- RAFT fine-tuning with the 503K corpus
- Multi-model support (swap in different LLMs)
- Streaming responses for better UX
- Integration with the Unheaded dashboard

---

## Fallback Plan

If the inference server is slow or unresponsive:
- Show the /api/v1/search endpoint directly (no LLM needed, just vector search)
- Show the /api/v1/stats endpoint to prove the index is loaded
- Show the integration test results as evidence of system health
