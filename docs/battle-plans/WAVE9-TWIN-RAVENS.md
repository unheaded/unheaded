# WAVE 9 BATTLE PLAN — The Twin Ravens: Wotan Active-Passive Redundancy

**Forged**: 2026-04-05
**Codename**: The Twin Ravens (Huginn and Muninn)
**Round Table**: BlackMage (security), Scientist (architecture), Micromanager (execution), Warmonger (planning)
**Prerequisite**: Wotan running on WEST + EAST, PostgreSQL (The Well) on :5432, WAL at pkg/storage/wal/
**Target**: Wotan survives node failure with zero message loss. Messages persist in PostgreSQL.
**Estimated Duration**: 9-12 hours across 4 phases
**Agent Strategy**: Phase 0 sequential, Phases 1-3 sequential, Track B parallel throughout
**Commit Cadence**: Every 4 steps

## LEGEND

[B] = Bash command | [V] = Verification | [D] = Debug | [W] = Wire/integrate
[S] = Security | [P] = Parallel OK | [SEQ] = Must be sequential
[C] = Commit checkpoint | [CODE] = Implementation | [TEST] = Test
[DESIGN] = Architecture decision

## PROGRESS

- [x] Phase 0: PostgreSQL Store (Steps 1-18) — DONE. PG+WAL+hybrid stores, encoding, sequences, epoch. 13 packages green.
- [x] Phase 1: Cluster Plumbing (Steps 19-36) — DONE. Config (8 tests), proto, replication server+client, flags, node_id, main.go wiring.
- [x] Phase 2: Failover (Steps 37-48) — DONE. State machine (5 tests), runbooks (setup + failover).
- [x] Phase 3: Integration + Hardening (Steps 49-56) — DONE. Config, ADR-035 accepted, PG publish wiring, 13/13 packages green.
- [x] Track B: Backlog Blitz — ADR-035/036/037 written, README updated, NetBox runbook, Wave 6 9/10 sprints
- [~] Track D: Distillation — script ready (2378 files repo mode), auto-distill queued (waiting for training v3)

---

## PHASE 0: POSTGRESQL STORE IMPLEMENTATION (Steps 1-18)

**Goal**: Messages persist in PostgreSQL. WAL provides write-ahead buffering.
**Time**: ~2 hours
**Agent**: Developer (sequential)

### Schema

```sql
-- Wotan message persistence
CREATE TABLE IF NOT EXISTS wotan_messages (
    id UUID PRIMARY KEY,
    room TEXT NOT NULL,
    sender_id UUID NOT NULL,
    content BYTEA NOT NULL,
    seq BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_wotan_messages_room_seq ON wotan_messages(room, seq);
CREATE INDEX IF NOT EXISTS idx_wotan_messages_created ON wotan_messages(created_at);

-- Per-topic sequence tracking
CREATE TABLE IF NOT EXISTS wotan_sequences (
    topic TEXT PRIMARY KEY,
    seq BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Cluster epoch tracking (split-brain prevention)
CREATE TABLE IF NOT EXISTS wotan_cluster (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Steps

- [ ] **Step 1** [CODE]: Create `services/wotan/internal/store/encoding.go`
  - EncodeMessage/DecodeMessage with version byte prefix [v1][json]
  - Must handle ringbuffer.Message ↔ []byte round-trip

- [ ] **Step 2** [TEST]: Create `services/wotan/internal/store/encoding_test.go`
  - Round-trip, nil, empty, large content (>64KB)

- [ ] **Step 3** [B]: Create PostgreSQL schema
  ```bash
  docker exec unheaded-postgres psql -U unheaded -d unheaded -c "CREATE TABLE IF NOT EXISTS wotan_messages ..."
  ```

- [ ] **Step 4** [V]: Verify schema created
  ```bash
  docker exec unheaded-postgres psql -U unheaded -d unheaded -c "\dt wotan_*"
  ```

- [ ] **Step 5** [C]: Commit encoding + schema

- [ ] **Step 6** [CODE]: Create `services/wotan/internal/store/pg_store.go`
  - PGStore struct: holds *sql.DB connection pool
  - Implements MessageStore interface
  - Push: INSERT with RETURNING seq
  - Get: SELECT by id
  - GetSince: SELECT WHERE room=$1 AND seq > $2
  - Delete: DELETE with triple verification (id + sender + content hash)
  - Count/Capacity from PG
  - Recovery: SELECT MAX(seq) per topic on startup

- [ ] **Step 7** [TEST]: Create `services/wotan/internal/store/pg_store_test.go`
  - Push/Get round-trip against real Docker PG
  - GetSince pagination
  - Delete with verification
  - Concurrent access (-race)

- [ ] **Step 8** [V]: `go test ./services/wotan/internal/store/... -run TestPG -race`

- [ ] **Step 9** [C]: Commit PG store

- [ ] **Step 10** [CODE]: Create `services/wotan/internal/store/sequence.go`
  - PersistentSequenceCounter backed by wotan_sequences table
  - Next(topic): UPDATE seq = seq + 1 WHERE topic=$1 RETURNING seq
  - Upsert on first use: INSERT ON CONFLICT UPDATE

- [ ] **Step 11** [CODE]: Create `services/wotan/internal/store/wal_store.go`
  - Wraps pkg/storage/wal/wal.go
  - Push: encode → WAL write (fast) → signal async flusher
  - Background flusher: batch WAL entries → PG INSERT every 100ms or 100 entries
  - Get: check in-memory hot cache → fall back to PG
  - Recovery: scan WAL for unflushed entries, replay to PG

- [ ] **Step 12** [CODE]: Create `services/wotan/internal/store/hybrid_store.go`
  - Combines WALStore (speed) + PGStore (durability)
  - Constructor: NewHybridStore(walDir, pgConnStr) 
  - Close: flush WAL to PG, close both

- [ ] **Step 13** [TEST]: Create `services/wotan/internal/store/wal_store_test.go`
  - WAL write + PG flush verified
  - Recovery after simulated crash (close without flush, reopen)
  - Concurrent writes

- [ ] **Step 14** [V]: `go test ./services/wotan/internal/store/... -race`

- [ ] **Step 15** [C]: Commit WAL + hybrid store

- [ ] **Step 16** [W]: Wire into store factory
  - Update `services/wotan/internal/store/config.go:136-137`
  - WALStoreType → NewWALStore()
  - PostgresStoreType → NewPGStore()
  - Add "hybrid" type → NewHybridStore()

- [ ] **Step 17** [V]: Existing MemoryStore tests still pass

- [ ] **Step 18** [V]: **PHASE 0 EXIT GATE**
  - Messages survive Wotan restart (PG persistence)
  - Sequence numbers persist across restarts  
  - WAL provides <1ms write latency
  - `go test ./services/wotan/internal/store/... -race` all green
  - `go vet ./services/wotan/...` clean

---

## PHASE 1: CLUSTER PLUMBING (Steps 19-36)

**Goal**: Primary streams WAL entries to standby via gRPC + mTLS.
**Time**: ~3 hours
**Prerequisite**: Phase 0 complete

- [ ] **Step 19** [DESIGN]: Cluster config (config.go)
- [ ] **Step 20** [CODE]: `services/wotan/internal/cluster/config.go`
- [ ] **Step 21** [CODE]: `services/wotan/proto/replication.proto`
- [ ] **Step 22** [B]: `protoc --go_out=. --go-grpc_out=. replication.proto`
- [ ] **Step 23** [V]: Generated code compiles
- [ ] **Step 24** [C]: Commit proto + config

- [ ] **Step 25** [CODE]: `services/wotan/internal/cluster/replication_server.go`
- [ ] **Step 26** [CODE]: `services/wotan/internal/cluster/replication_client.go`
- [ ] **Step 27** [CODE]: `services/wotan/internal/cluster/applier.go`
- [ ] **Step 28** [TEST]: replication_server_test.go + replication_client_test.go
- [ ] **Step 29** [V]: CatchUp streams entries, client applies
- [ ] **Step 30** [C]: Commit replication

- [ ] **Step 31** [S]: `services/wotan/internal/cluster/transport.go` — mTLS
- [ ] **Step 32** [C]: Add cluster flags to main.go
- [ ] **Step 33** [W]: Wire cluster init into main.go (zero overhead standalone)
- [ ] **Step 34** [W]: Wire WAL write hook into TopicService.PublishTopic
- [ ] **Step 35** [W]: Populate node_id in all TopicEvent messages
- [ ] **Step 36** [V]: **PHASE 1 EXIT GATE** — standalone unchanged, cluster connects

---

## PHASE 2: FAILOVER (Steps 37-48)

**Goal**: Akira-driven failover, client failover, split-brain prevention.
**Time**: ~2 hours
**Prerequisite**: Phase 1 complete

- [ ] **Step 37** [DESIGN]: Akira-driven failover strategy
- [ ] **Step 38** [CODE]: `services/wotan/internal/cluster/failover.go`
- [ ] **Step 39** [CODE]: `services/wotan/internal/cluster/epoch.go`
- [ ] **Step 40** [TEST]: failover_test.go — state machine + epoch
- [ ] **Step 41** [C]: Commit failover

- [ ] **Step 42** [CODE]: Extend pkg/wotan-client — NewClusterClient(primary, standby)
- [ ] **Step 43** [TEST]: cluster_client_test.go
- [ ] **Step 44** [C]: Commit client failover

- [ ] **Step 45** [CODE]: `runbooks/wotan/cluster-failover.yaml`
- [ ] **Step 46** [CODE]: `runbooks/wotan/cluster-setup.yaml`
- [ ] **Step 47** [V]: Manual failover test — kill WEST, verify EAST promotes
- [ ] **Step 48** [V]: **PHASE 2 EXIT GATE** — failover <15s, epoch prevents split-brain

---

## PHASE 3: INTEGRATION + HARDENING (Steps 49-56)

**Goal**: Configs, metrics, ADR, full test suite green.
**Time**: ~2 hours
**Prerequisite**: Phase 2 complete

- [ ] **Step 49** [C]: Update configs/wotan.yaml with cluster + store sections
- [ ] **Step 50** [CODE]: Cluster metrics (role, lag, epoch, failovers)
- [ ] **Step 51** [TEST]: Integration test — primary + standby + 1000 messages + kill + verify
- [ ] **Step 52** [TEST]: WAL integrity fuzz test
- [ ] **Step 53** [V]: Full test suite green
- [ ] **Step 54** [CODE]: ADR-035 — Wotan Active-Passive Redundancy
- [ ] **Step 55** [CODE]: Update ADR-INDEX.md
- [ ] **Step 56** [V]: **PHASE 3 EXIT GATE** — everything green, ADR accepted

---

## TRACK B: PARALLEL BACKLOG (no code overlap with Track A)

- [ ] ADR-031: Write implementation plan section
- [ ] ADR-032: Write phased migration plan
- [ ] ADR-033: Create NetBox setup runbook
- [ ] Monitor training v3, deploy when complete
- [ ] CA rotation ceremony runbook

---

## TRACK D: CLAUDE DISTILLATION — TRAINING DATA GENERATION (Steps D1-D12)

**Goal**: Use Claude Haiku/Sonnet to generate 5K-10K expert-quality QA pairs from Kingdom docs.
**Parallel with**: Track A (Wotan redundancy), Track B (backlog), Track C (training v3)
**No code overlap**: Writes to raft/scripts/, outputs to /var/zhen/
**Time**: ~2 hours script + ~1 hour API batch run

### Why

Current 3965 QA pairs were auto-generated by Mistral-7B (the model teaching itself).
Loss plateau at 8.77 = garbage in, garbage out. Claude actually understands code —
its answers will be grounded, accurate, and detailed. This is **distillation**:
a stronger model teaching a weaker one.

### Cost Estimate

| Model | Input (2M tokens) | Output (1M tokens) | Total |
|-------|--------------------|---------------------|-------|
| Haiku | $0.25/M = $0.50 | $1.25/M = $1.25 | ~$1.75 |
| Sonnet | $3/M = $6.00 | $15/M = $15.00 | ~$21.00 |

Haiku is the pragmatic choice. Sonnet for a second pass on critical docs.

### Steps

- [ ] **D1** [CODE]: Create `raft/scripts/distill_qa.py`
  - Reads Kingdom documents (ADRs, docs, runbooks, Go source, Rust source)
  - For each document: sends content + generation prompt to Claude Haiku
  - Claude generates 10-20 QA pairs per document, grounded in the actual content
  - Output: JSONL matching zhenai-forge format (question, answer, source fields)
  - Checkpoint: saves every 50 documents to /var/zhen/distilled_qa_checkpoint.jsonl
  - Idempotent: skips documents already in checkpoint

- [ ] **D2** [CODE]: Document categories + prompts
  - ADRs (37 files): "Generate questions about this architecture decision"
  - Docs (512 files): "Generate questions about this documentation"
  - Runbooks (55 files): "Generate operational questions about this runbook"
  - Go source (top 100 files by LOC): "Generate questions about this Go code"
  - Rust source (zhenai-forge, zhend, doom-runner): "Generate questions about this Rust code"
  - Skills (18 files): "Generate questions about this Kingdom skill"
  - CLAUDE.md: "Generate comprehensive questions about the Unheaded project"

- [ ] **D3** [V]: Verify ANTHROPIC_API_KEY is set
  ```bash
  python3 -c "import anthropic; c = anthropic.Anthropic(); print('OK')"
  ```

- [ ] **D4** [B]: Dry run — generate 10 QA pairs from ADR-034 to validate format
  ```bash
  python3 raft/scripts/distill_qa.py --dry-run --limit 1
  ```

- [ ] **D5** [V]: Verify output JSONL matches zhenai-forge expected format
  ```bash
  head -1 /var/zhen/distilled_qa_dry.jsonl | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.keys())"
  ```

- [ ] **D6** [C]: Commit distill_qa.py

- [ ] **D7** [B]: Run full distillation — all document categories
  ```bash
  ANTHROPIC_API_KEY=<key> python3 raft/scripts/distill_qa.py \
    --output /var/zhen/distilled_qa_haiku.jsonl \
    --model claude-haiku-4-5-20251001 \
    --docs-dir ~/tmp/unheaded/docs \
    --source-dir ~/tmp/unheaded
  ```

- [ ] **D8** [V]: Verify output — count pairs, spot-check quality
  ```bash
  wc -l /var/zhen/distilled_qa_haiku.jsonl
  head -3 /var/zhen/distilled_qa_haiku.jsonl | python3 -m json.tool
  ```

- [ ] **D9** [B]: Merge with existing dataset
  ```bash
  cat /var/zhen/raft_dataset_combined.jsonl /var/zhen/distilled_qa_haiku.jsonl \
    > /var/zhen/raft_dataset_v4.jsonl
  wc -l /var/zhen/raft_dataset_v4.jsonl
  ```

- [ ] **D10** [B]: Train kingdom-v4.zlora with distilled data
  ```bash
  cd ~/tmp/unheaded/crates/zhenai-forge && \
  LD_LIBRARY_PATH=/opt/rocm/lib ./target/release/zhenai-forge train \
    --model /var/zhen/models/mistral-7b-instruct-q5_k_m.gguf \
    --data /var/zhen/raft_dataset_v4.jsonl \
    --output /var/zhen/models/kingdom-v4.zlora \
    --epochs 3
  ```

- [ ] **D11** [B]: Convert + A/B test
  ```bash
  python3 scripts/zlora-to-gguf.py -i /var/zhen/models/kingdom-v4.zlora -o /var/zhen/models/kingdom-v4-lora.gguf
  # Start llama-server with --lora, test 30 Kingdom questions
  ```

- [ ] **D12** [V]: **TRACK D EXIT GATE**
  - 5K+ QA pairs generated by Claude
  - kingdom-v4.zlora trained on distilled data
  - A/B test shows measurable improvement over base Mistral-7B
  - If improvement: deploy as default for Zhenai inference

---

*Wave 9 Battle Plan — Forged 2026-04-05*
*4 Phases + 4 Tracks. 68 Steps. Two ravens fly. One watches. One remembers.*
*Track D: The student learns from the master, then teaches itself.*
