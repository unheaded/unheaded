# Zhen Demo Script

**Duration**: 10 minutes
**Audience**: anyone technical
**Setup**: `raft/DEMO_CHECKLIST.md` passes. Web UI on localhost:20103, retrieval
(vor) on 20110, zhen-agentd on 20111, local model on 8081.

---

## Opening (30 seconds)

> "This is Zhen, an assistant that answers from the Unheaded source tree. The
> model runs on this machine and retrieval runs on this machine, so nothing here
> needs a cloud API and nothing leaves the host."

**Action**: Log in at localhost:20103 before the audience arrives: the page asks
first, and nothing but the page and `/health` answers without a session. Show
the Zhen UI and point out the "Online" status in the header.

---

## Live Demo (5 minutes)

### Question 1: Big Picture (60s)

**Ask**: "What is Unheaded?"

**Talk through**: Show the answer arriving. Point out:
- Answer quality: it draws on more than one source
- Source attribution at the bottom
- Response time and token count in the metadata

### Question 2: Architecture Depth (60s)

**Ask**: "What are the 6 layers of the Unheaded architecture?"

**Talk through**: Zhen retrieves the architecture docs and returns the layers.

### Question 3: Specific Technical Detail (60s)

**Ask**: "What port does Wotan use?"

**Talk through**: It returns the exact ports (18000 HTTP, 18001 gRPC), which
shows retrieval precision.

### Question 4: Protocol Knowledge (60s)

**Ask**: "What is the Monad wire format size?"

**Talk through**: The answer should say 20 bytes, version 0x01, frozen.

### Question 5: Cross-Cutting (60s)

**Ask**: "How does eBPF tracing work in Unheaded?"

**Talk through**: It pulls together Rust eBPF programs, Go userspace and the
trace collector. Point out the breadth of retrieval.

---

## How It Fits Together (2 minutes)

1. **Local inference**: a GGUF model under llama-server on port 8081. No API
   keys and no outside dependency. `/api/v1/stats` names the model loaded.

2. **Retrieval**: vor on port 20110 answers from the indexed tree; the top
   results go into the prompt.

3. **The web app**: `raft/zhen_app.py` serves the UI and the API, including
   `/api/v1/query` (retrieval and generation), `/api/v1/search` (retrieval
   alone) and `/api/v1/stats`. Chat goes through zhen-agentd on 20111.

---

## Closing (1 minute)

> "What you've seen is retrieval and generation on one machine. The same assistant
> can run reviewed runbooks, with an approval step in front of anything that
> changes a host."

---

## Fallback Plan

If the inference server is slow or unresponsive, go to a terminal holding the
checklist's `zhen.cookies`. None of these needs the model.

- Show `/health` for the system state. It needs no session.
- Show the backends reporting:
  ```bash
  curl -s -b zhen.cookies http://localhost:20103/api/v1/stats | python3 -m json.tool
  ```
- Show retrieval alone. Search is a POST, so it also sends back the CSRF cookie,
  which only the page sets:
  ```bash
  curl -s -b zhen.cookies -c zhen.cookies -o /dev/null http://localhost:20103/
  curl -s -b zhen.cookies -X POST http://localhost:20103/api/v1/search \
    -H "Origin: http://localhost:20103" \
    -H "X-Zhen-CSRF: $(awk '$6 == "zhen_csrf" {print $7}' zhen.cookies)" \
    -H "Content-Type: application/json" \
    -d '{"query": "What is Unheaded?"}' | python3 -m json.tool
  ```
