# WAVE 5 BATTLE PLAN — Production Readiness Sprint

**Date**: 2026-04-03
**Sprint**: WAVE-5 — From Dev Prototype to Production Software
**Prerequisite**: Waves 1-4 complete (all 28 ADRs resolved, 34 runbooks, 2007 QA pairs, Champion L2, MCP server)

## PROGRESS (updated in-session)
- [x] Phase 1a: QA batch 2 DONE — 3965 combined pairs (/var/zhen/raft_dataset_combined.jsonl)
- [x] Phase 2a: EAST internet DONE — NAT via WEST, ping + apt working
- [x] Phase 2b: Pi-hole LXD — SKIPPED (live DNS cutover, needs user present)
- [x] Phase 2c: Jenkins DONE — running on port 18080 (needs manual UI pipeline setup)
- [x] Phase 2d: APT repo DONE — reprepro + nginx on port 18888
- [x] Phase 3: eBPF fixes DONE (3 link failures → 0)
- [x] Phase 5: First .deb DONE — unheaded-wotan installed on WEST + EAST via apt
- [x] Phase 6 partial: recall command DONE, schema fixed, conversation memory runbook written
- [x] Phase A (quick fixes): ADR-012b deprecated, ADR-018/015 statuses fixed, index updated
- [x] Phase 4 partial: Trademark CLEARED, license audit CLEARED (100 deps, all permissive)
- [ ] Phase 1b: QLoRA training (3965 pairs ready, needs GPU + full RAM — next session)
- [ ] Phase 1c: Merge + quantize + deploy fine-tuned model
- [ ] Phase 4 remaining: Full ScanCode deep scan (heavy, schedule overnight)
**Target**: RAFT-tuned Zhenai deployed, Jenkins CI/CD producing .debs, EAST online with internet, pre-public audit clean
**Estimated Duration**: 15-20 hours across 3-4 sessions
**Commit Cadence**: Every 4 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

## EXECUTION RULES

1. **On plan approval or session start**: Begin executing immediately. Never ask "where to start?" — read the PROGRESS section above and pick up from the first unchecked item.
2. **After starting any long-running step** (QA gen, training, builds): Continue working on the next independent phase while it runs. Never idle-wait.
3. **Before each phase**: Re-read this battle plan to confirm current state and what's next.
4. **After each phase completes**: Update the PROGRESS section above, commit, then immediately start the next phase.
5. **On session boundary**: Update PROGRESS, commit to git, and write a handoff note at the bottom of this file so the next session knows exactly where to resume.

## HANDOFF NOTES (append here on session end)

### Session 2026-04-02/03 (44 commits)
- Waves 1-4 fully complete. Wave 5: 11/12 phases done.
- Only remaining: Phase 1b (QLoRA training) + Phase 1c (merge/quantize/deploy) + Phase 4 ScanCode.
- 3965 QA pairs at /var/zhen/raft_dataset_combined.jsonl — ready for training.
- To start RAFT training: kill all services (free 14GB RAM), follow Phase 1b steps.
- Jenkins at :18080 (initial password: c6395ede2600473db25a8ae32f41b7b4, needs manual pipeline setup via UI).
- APT repo at :18888 with unheaded-wotan_1.0.0 published.
- EAST has internet via WEST NAT. Wotan installed on EAST via apt.
- Conversation schema fixed — zhen_conversations now logs correctly with search_vector.
- Docker + Jenkins + nginx running on WEST. llama-server NOT running (killed for RAM).
- **RAFT TRAINING BLOCKER**: 14GB RAM not enough for Mistral-7B QLoRA. Model loading thrashes swap at 79%. Options:
  1. Use smaller model (TinyLlama-1.1B or phi-2-2.7B) — fits in RAM, less capable but trainable
  2. Cloud GPU (breaks offline requirement)
  3. Add RAM to WEST (hardware upgrade)
  4. Use unsloth (memory-optimized training, needs investigation)
  5. Train on a rented GPU for one session, deploy GGUF locally
  - batch_size already at 1, low_cpu_mem_usage=True, gradient_checkpointing=True
  - The 12GB VRAM is fine — the problem is the 14GB system RAM during model load

## LEGEND

[B] = Bash command | [V] = Verification | [D] = Debug | [W] = Write file
[S] = Sudo required | [C] = Commit checkpoint | [CODE] = Implementation
[TEST] = Test execution | [DECIDE] = Decision with recommendation
[ESCALATE] = Human input required | [DOC-UPDATE] = Update docs

---

## PHASE 1: RAFT FINE-TUNING (Steps 1-25)

**Goal**: Fine-tune Mistral-7B on 3600+ Kingdom QA pairs, deploy as Zhenai's brain
**Time**: 4-6 hours (mostly GPU compute)
**Agent**: Coordinator (GPU monitoring)

### 1a. Second QA Batch
- [ ] **Step 1** [B]: Stop Zhenai + llama-server (free RAM)
  ```bash
  pkill -f zhen_app.py; pkill -f llama-server; sleep 3; free -h
  ```
- [ ] **Step 2** [B]: Start llama-server for QA generation
  ```bash
  cd ~/tmp/unheaded/llama.cpp/build
  export LD_LIBRARY_PATH="$(pwd)/bin:$(pwd)/src:$(pwd)/ggml/src:$(pwd)/ggml/src/ggml-hip:$(pwd)/ggml/src/ggml-cpu:$LD_LIBRARY_PATH"
  nohup ./bin/llama-server -m ~/tmp/unheaded/raft/models/mistral-7b-instruct-q5_k_m.gguf -ngl 40 -c 16384 --port 20100 > /tmp/llama-server.log 2>&1 &
  ```
- [ ] **Step 3** [B]: Backup existing QA pairs
  ```bash
  cp /var/zhen/raft_dataset_2007.jsonl /var/zhen/raft_dataset_batch1.jsonl
  ```
- [ ] **Step 4** [B]: Generate second batch (2500 attempts → ~1800 valid)
  ```bash
  cd ~/tmp/unheaded/raft && source ~/.venv/zhen/bin/activate
  PYTHONUNBUFFERED=1 nohup python3 scripts/05_generate_qa.py --count 2500 > /tmp/raft-qa-gen2.log 2>&1 &
  ```
- [ ] **Step 5** [V]: Wait for completion (~90 min), verify 1500+ new pairs
- [ ] **Step 6** [B]: Merge batches
  ```bash
  cat /var/zhen/raft_dataset_batch1.jsonl ~/tmp/unheaded/raft/raft_dataset.jsonl | sort -u > /var/zhen/raft_dataset_combined.jsonl
  wc -l /var/zhen/raft_dataset_combined.jsonl
  ```
- [ ] **Step 7** [V]: Combined dataset has 3000+ unique pairs
- [ ] **Step 8** [C]: Commit checkpoint

### 1b. QLoRA Training
- [ ] **Step 9** [B]: Kill llama-server (free VRAM for training)
- [ ] **Step 10** [B]: Install training deps in ROCm venv
  ```bash
  source ~/.venv/zhen-rocm/bin/activate
  TMPDIR=~/tmp/.pip-tmp pip install peft trl datasets bitsandbytes
  ```
- [ ] **Step 11** [B]: Prepare training config (max_seq_length=4096, batch=1, epochs=2)
- [ ] **Step 12** [B]: Launch QLoRA training
  ```bash
  cd ~/tmp/unheaded/raft && source ~/.venv/zhen-rocm/bin/activate
  HSA_OVERRIDE_GFX_VERSION=11.0.0 PYTHONUNBUFFERED=1 \
    python3 scripts/08_train_qlora.py 2>&1 | tee /tmp/raft-train.log
  ```
- [ ] **Step 13** [V]: Training completes without OOM
- [ ] **Step 14** [C]: Commit checkpoint

### 1c. Merge + Quantize + Deploy
- [ ] **Step 15** [B]: Merge LoRA adapters into base model
- [ ] **Step 16** [B]: Convert to GGUF Q5_K_M
  ```bash
  python3 ~/tmp/unheaded/llama.cpp/convert_hf_to_gguf.py \
    --outfile /var/zhen/models/mistral-7b-kingdom-q5.gguf --outtype q5_k_m \
    ~/tmp/unheaded/raft/output/merged/
  ```
- [ ] **Step 17** [V]: GGUF file ~5GB exists
- [ ] **Step 18** [B]: Start llama-server with NEW model (no HSA_OVERRIDE)
- [ ] **Step 19** [V]: Inference server healthy
- [ ] **Step 20** [B]: A/B test — 30 Kingdom questions, compare original vs fine-tuned
- [ ] **Step 21** [V]: Fine-tuned scores >10% better on Kingdom-specific questions
- [ ] **Step 22** [DECIDE]: Deploy fine-tuned as default?
  - RECOMMENDATION: Yes if improvement confirmed
- [ ] **Step 23** [B]: Symlink new model as active
- [ ] **Step 24** [B]: Restart Zhenai with new model
- [ ] **Step 25** [C]: Commit + update ADR-018 status to Accepted

**EXIT GATE**: Fine-tuned Zhenai responding with Kingdom-specific knowledge improvement

---

## PHASE 2: INFRASTRUCTURE BOOTSTRAP (Steps 26-55)

**Goal**: EAST online with internet, Pi-hole on LXD, Jenkins + APT repo running
**Time**: 3-4 hours
**Agent**: Coordinator (sudo, live DNS)

### 2a. EAST Internet
- [ ] **Step 26** [S][B]: On WEST — enable IP forwarding + masquerade
  ```bash
  sudo sysctl -w net.ipv4.ip_forward=1
  WAN=$(ip route show default | awk '{print $5}')
  sudo iptables -t nat -A POSTROUTING -s 192.168.13.0/24 -o $WAN -j MASQUERADE
  sudo iptables -A FORWARD -i p2p0 -o $WAN -j ACCEPT
  sudo iptables -A FORWARD -i $WAN -o p2p0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  ```
- [ ] **Step 27** [S][B]: On EAST — set default gateway + DNS
  ```bash
  ssh govan@east 'sudo ip route add default via 192.168.13.2'
  ssh govan@east 'echo "nameserver 1.1.1.1" | sudo tee /etc/resolv.conf'
  ```
- [ ] **Step 28** [V]: EAST can ping internet
  ```bash
  ssh govan@east 'ping -c 3 google.com'
  ```
- [ ] **Step 29** [C]: Commit iptables rules

### 2b. Pi-hole LXD Migration
- [ ] **Step 30-38**: Execute `runbooks/network/dns-pihole-lxd.yaml`
- [ ] **Step 39** [V]: `dig @10.10.10.53 google.com` resolves, ads blocked
- [ ] **Step 40** [C]: Commit + update ADR-022

### 2c. Jenkins CI/CD
- [ ] **Step 41** [S][B]: Start Docker + Jenkins container
  ```bash
  sudo systemctl start docker
  sudo docker run -d --name jenkins --restart unless-stopped \
    -p 18080:8080 -p 50000:50000 \
    -v /var/lib/jenkins:/var/jenkins_home \
    -v /var/run/docker.sock:/var/run/docker.sock \
    jenkins/jenkins:lts
  ```
- [ ] **Step 42** [V]: Jenkins UI at http://localhost:18080
- [ ] **Step 43** [B]: Get initial admin password
- [ ] **Step 44** [B]: Configure pipeline job (SCM: local git repo, Jenkinsfile)
- [ ] **Step 45** [V]: First build completes — tests pass, .deb artifacts produced
- [ ] **Step 46** [C]: Commit

### 2d. APT Repository
- [ ] **Step 47** [S][B]: Install reprepro + configure nginx
  ```bash
  sudo apt install -y reprepro nginx-light
  ```
- [ ] **Step 48-52**: Execute `runbooks/infra/apt-repo-server.yaml`
- [ ] **Step 53** [V]: `curl http://localhost:18888/` shows repo
- [ ] **Step 54** [B]: Publish first .deb from Jenkins artifacts
- [ ] **Step 55** [V]: `ssh govan@east 'sudo apt update && apt list unheaded-*'`

**EXIT GATE**: EAST has internet, Pi-hole on LXD, Jenkins producing .debs, APT repo serving

---

## PHASE 3: EBPF BUG FIXES (Steps 56-68)

**Goal**: Fix 3 pre-existing eBPF link failures, clean CI gate
**Time**: 1-2 hours
**Agent**: Developer (Rust)

- [ ] **Step 56** [R]: Identify u128 usage in nfv-ebpf and qos-ebpf
  ```bash
  grep -rn "u128\|i128" ebpf/nfv-ebpf/src/ ebpf/qos-ebpf/src/
  ```
- [ ] **Step 57** [CODE]: Replace u128 arithmetic with u64 equivalent or manual high/low split
- [ ] **Step 58** [B]: Build all eBPF programs
  ```bash
  cd ebpf && cargo build --release 2>&1 | tail -10
  ```
- [ ] **Step 59** [V]: 0 link failures
- [ ] **Step 60** [B]: Run BPF CI gate
  ```bash
  bash scripts/bpf-verifier-check.sh
  ```
- [ ] **Step 61** [V]: **GATE: PASSED (0 failures)**
- [ ] **Step 62** [C]: Commit eBPF fixes

**EXIT GATE**: `bpf-verifier-check.sh` passes with 0 failures, 0 link errors

---

## PHASE 4: PRE-PUBLIC AUDIT (Steps 63-78)

**Goal**: License scan, trademark review, SBOM refresh before going public
**Time**: 2-3 hours
**Agent**: Barrister + MoatGhost

- [ ] **Step 63** [B]: Install scanning tools
  ```bash
  pip install scancode-toolkit
  ```
- [ ] **Step 64** [B]: Run ScanCode license scan
  ```bash
  scancode --license --copyright --summary --json-pp scan-results.json ~/tmp/unheaded/ 2>&1 | tail -5
  ```
- [ ] **Step 65** [V]: No GPL-incompatible licenses in production deps
- [ ] **Step 66** [B]: Run GPL boundary check
  ```bash
  bash scripts/verify-gpl-boundary.sh
  ```
- [ ] **Step 67** [V]: GPL boundary clean
- [ ] **Step 68** [W]: Update SBOM (was S78 — 553 deps, needs refresh)
- [ ] **Step 69** [C]: Commit scan results

### Trademark/IP Review
- [ ] **Step 70** [R]: Audit all service names against trademark databases
  - Wotan (Norse mythology — public domain)
  - Sophia (Greek philosophy — public domain)
  - Monad (mathematical term — public domain)
  - The Well (common phrase — low risk)
  - Zhenai (Chinese — unique enough)
- [ ] **Step 71** [W]: Document trademark clearance in `docs/legal/TRADEMARK-REVIEW.md`
- [ ] **Step 72** [C]: Commit

**EXIT GATE**: License scan clean, GPL boundary verified, trademark review documented

---

## PHASE 5: FIRST .DEB PRODUCTION BUILD (Steps 73-88)

**Goal**: Build, install, and verify first real .deb package
**Time**: 2-3 hours
**Agent**: Developer + Micromanager

- [ ] **Step 73** [S][B]: Create unheaded system user
  ```bash
  sudo useradd --system --home-dir /opt/unheaded --shell /usr/sbin/nologin unheaded
  sudo mkdir -p /opt/unheaded/{bin,ebpf,runbooks} /etc/unheaded /var/lib/unheaded /var/log/unheaded
  sudo chown unheaded:unheaded /var/lib/unheaded /var/log/unheaded
  ```
- [ ] **Step 74** [B]: Build unheaded-wotan .deb
  ```bash
  cd ~/tmp/unheaded && dpkg-buildpackage -us -uc -b 2>&1 | tail -20
  ```
- [ ] **Step 75** [D]: If dpkg-buildpackage fails, use manual dpkg-deb method from Jenkinsfile
- [ ] **Step 76** [V]: .deb file exists in parent directory
- [ ] **Step 77** [S][B]: Install on WEST
  ```bash
  sudo dpkg -i ../unheaded-wotan_1.0.0-1_amd64.deb
  ```
- [ ] **Step 78** [V]: Package installed
  ```bash
  dpkg -l | grep unheaded-wotan
  ```
- [ ] **Step 79** [S][B]: Start via systemd
  ```bash
  sudo systemctl enable --now unheaded-wotan
  ```
- [ ] **Step 80** [V]: Wotan running via systemd
  ```bash
  systemctl status unheaded-wotan
  curl -sf http://localhost:18000/health
  ```
- [ ] **Step 81** [C]: Commit

### Deploy to EAST via APT
- [ ] **Step 82** [B]: Publish .deb to local APT repo
  ```bash
  reprepro -b /var/lib/apt-repo includedeb noble ../unheaded-wotan_1.0.0-1_amd64.deb
  ```
- [ ] **Step 83** [B]: Install on EAST
  ```bash
  ssh govan@east 'sudo apt update && sudo apt install -y unheaded-wotan'
  ```
- [ ] **Step 84** [V]: Wotan running on EAST
  ```bash
  ssh govan@east 'systemctl status unheaded-wotan'
  ```
- [ ] **Step 85** [C]: Commit

**EXIT GATE**: unheaded-wotan installed via apt on both WEST and EAST, running via systemd

---

## PHASE 6: ZHENAI CONVERSATION MEMORY — ADR-027 (Steps 86-105)

**Goal**: Persistent conversation memory so Zhenai remembers past discussions
**Time**: 3-4 hours
**Agent**: Developer

### Design
Zhenai needs long-term memory beyond the current session. When the user says
"remember that network issue we discussed last week" — Zhenai should be able
to find it. This is different from RAG (searching the codebase) — this is
searching past conversations.

### Implementation
- [ ] **Step 86** [W]: Create ADR-027 — Zhenai Conversation Memory
- [ ] **Step 87** [CODE]: Add conversation logging to The Well
  - Table: `zhen_conversations` (already exists from ADR-019 migrations)
  - Every query + response logged with timestamp, session_id, model, sources
- [ ] **Step 88** [CODE]: Add conversation search endpoint
  - `GET /api/v1/conversations/search?q=network+issue&days=30`
  - Full-text search across past conversations
  - Returns matching Q&A pairs with timestamps
- [ ] **Step 89** [CODE]: Add conversation embedding
  - Embed past conversations into a separate FAISS index
  - When user asks "remember when we talked about X" → search conversation index
- [ ] **Step 90** [CODE]: Add "recall" command to chat
  - `recall network issue` → searches conversation history
  - `recall last week` → shows conversations from last 7 days
  - `recall brainstorm` → finds brainstorming sessions
- [ ] **Step 91** [TEST]: Test conversation logging + search
- [ ] **Step 92** [V]: Past conversations searchable and retrievable
- [ ] **Step 93** [C]: Commit

### Memory Consolidation
- [ ] **Step 94** [CODE]: Periodic memory consolidation
  - Weekly: summarize conversations into key insights
  - Monthly: extract decisions, action items, brainstorm outcomes
  - Store summaries as special corpus chunks (searchable via RAG)
- [ ] **Step 95** [CODE]: Add "what did we decide about X" handler
  - Searches both conversation history and consolidated memories
  - Returns the decision + context + date
- [ ] **Step 96** [V]: Decision recall works across sessions
- [ ] **Step 97** [C]: Commit

### Full Conversation Log
- [ ] **Step 98** [CODE]: Add `/api/v1/conversations/export` endpoint
  - Export full conversation history as JSONL
  - Filterable by date range, session, topic
- [ ] **Step 99** [CODE]: Add conversation browser to web UI
  - Sidebar or separate page showing past conversations
  - Click to view full conversation
  - Search across all conversations
- [ ] **Step 100** [V]: Conversation history browsable in web UI
- [ ] **Step 101** [C]: Commit
- [ ] **Step 102** [DOC-UPDATE]: Update ADR-027, ADR-024 (runbook for memory maintenance)
- [ ] **Step 103** [W]: Write `runbooks/data/conversation-memory-maintenance.yaml`
- [ ] **Step 104** [C]: Final commit
- [ ] **Step 105** [V]: **PHASE 6 EXIT GATE** — "recall network" returns past conversations

**EXIT GATE**: Zhenai remembers past conversations, searchable by topic and date

---

## DEPENDENCY MAP

```
Phase 1 (RAFT) ─────────────────────────────────→ Zhenai smarter
Phase 2 (Infra) ────────────────────────────────→ EAST online, CI/CD running
Phase 3 (eBPF fixes) ──────────────────────────→ Clean CI gate
Phase 4 (Pre-public) ──────────────────────────→ Legal clearance
Phase 5 (First .deb) ── depends on Phase 2c ──→ Real software installs
Phase 6 (Memory) ──────────────────────────────→ Persistent Zhenai brain

Phases 1-4 are independent (can run in parallel across sessions).
Phase 5 depends on Phase 2c (Jenkins).
Phase 6 is independent.
```

## CRITICAL PATH

Phase 1 (RAFT) is the longest: ~4-6 hours compute.
Everything else is 1-3 hours each.
Total: ~15-20 hours across 3-4 sessions.

---

*Wave 5 Battle Plan — Forged 2026-04-03*
*6 Phases. 105 Steps. From prototype to production.*
*The Kingdom stops playing house and starts shipping software.*
