# ADR-027: Zhenai Conversation Memory — Persistent Cross-Session Recall

**Status:** Planned
**Date:** 2026-04-03
**Deciders:** Captain, Architect, Developer
**Depends on:** ADR-016 (The Well), ADR-019 (Champion Agent), ADR-018 (RAFT Training)

## Context

Zhenai currently has session-scoped memory — conversation history within a single browser session, stored in Flask's in-memory state. When the session ends or Zhenai restarts, all conversation context is lost.

This creates a real workflow problem: brainstorming sessions, debugging sessions, architecture discussions, and decision-making conversations contain valuable knowledge that should persist. When the user says "remember that network issue we discussed last week" or "what did we decide about the storage layer?" — Zhenai should be able to find and recall those conversations.

This is **different from RAG** (which searches the codebase and docs). This is searching **past conversations** — the record of what was discussed, decided, explored, and brainstormed between the human operator and Zhenai.

## Decision

### Three-Layer Memory Architecture

```
Layer 1: Conversation Log (The Well — PostgreSQL)
  ├── Every query + response stored with full metadata
  ├── Timestamp, session_id, model used, sources cited
  ├── Full-text searchable via PostgreSQL ts_vector
  └── Never deleted (append-only, like zhen_action_snapshots)

Layer 2: Conversation Embeddings (FAISS — Separate Index)
  ├── Each conversation turn embedded via all-MiniLM-L6-v2
  ├── Semantic search: "that thing about firewalls" finds firewall discussions
  ├── Separate from corpus index (conversations vs codebase)
  └── Rebuilt periodically from Layer 1

Layer 3: Consolidated Memory (Summaries + Decisions)
  ├── Weekly: auto-summarize conversations into key insights
  ├── Monthly: extract decisions, action items, brainstorm outcomes
  ├── Stored as special corpus chunks (searchable via main RAG)
  └── "What did we decide about X?" queries this layer
```

### Schema (The Well)

```sql
-- zhen_conversations already exists (ADR-019 migration)
-- Extend with full-text search:

ALTER TABLE zhen_conversations ADD COLUMN IF NOT EXISTS
  search_vector tsvector GENERATED ALWAYS AS (
    to_tsvector('english', coalesce(content, ''))
  ) STORED;

CREATE INDEX IF NOT EXISTS idx_zhen_conv_search
  ON zhen_conversations USING gin(search_vector);

-- Consolidated memories
CREATE TABLE IF NOT EXISTS zhen_memories_consolidated (
    id          BIGSERIAL PRIMARY KEY,
    period      VARCHAR(20) NOT NULL,  -- 'weekly', 'monthly'
    period_start TIMESTAMPTZ NOT NULL,
    period_end  TIMESTAMPTZ NOT NULL,
    summary     TEXT NOT NULL,
    decisions   JSONB DEFAULT '[]',    -- [{topic, decision, date, context}]
    action_items JSONB DEFAULT '[]',   -- [{item, status, date}]
    topics      TEXT[] DEFAULT '{}',   -- searchable topic tags
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
```

### Chat Commands

```
recall <topic>          — Search past conversations by topic
recall last week        — Show conversations from last 7 days
recall decisions        — Show all recorded decisions
remember this           — Flag current conversation as important (boosts in recall)
what did we decide about <X> — Search consolidated decisions
```

### API Endpoints

```
GET  /api/v1/conversations/search?q=<query>&days=30  — Full-text search
GET  /api/v1/conversations/recent?days=7             — Recent conversations
GET  /api/v1/conversations/decisions                  — All consolidated decisions
GET  /api/v1/conversations/export?format=jsonl        — Export full history
POST /api/v1/conversations/consolidate                — Trigger manual consolidation
```

### Conversation → Training Data Pipeline

Every conversation is potential RAFT training data:
1. User asks Kingdom-specific question → Zhenai answers
2. If user says "Remember" (existing feature) → answer is validated
3. Validated Q&A pairs feed into next RAFT training batch
4. Zhenai gets smarter from every conversation

This creates a **flywheel**: conversations → training data → better model → better conversations → more training data.

## Implementation Phases

### Phase 1: Persistent Logging (immediate)
- Log all conversations to The Well (zhen_conversations table)
- Full-text search via PostgreSQL tsvector
- `recall` chat command

### Phase 2: Semantic Search
- Embed conversations into separate FAISS index
- "remember that thing about..." finds semantically similar conversations
- Rebuild conversation index periodically

### Phase 3: Memory Consolidation
- Weekly/monthly auto-summarization via Mistral-7B
- Decision extraction and tagging
- "what did we decide about..." queries consolidated memories

### Phase 4: Training Integration
- Validated conversations → RAFT training pipeline
- Periodic re-training includes conversation-sourced QA pairs
- Zhenai improves from every interaction

## Consequences

### Positive
- Brainstorming sessions persist across days/weeks/months
- Decisions are searchable ("what did we decide about the storage layer?")
- No re-explaining context from prior sessions
- Natural training data generation from real conversations
- Full audit trail of all Zhenai interactions

### Negative
- Storage grows over time (mitigated: conversation text is small compared to corpus)
- Consolidation requires LLM inference (scheduled, not real-time)
- Privacy: all conversations stored permanently (feature, not bug — it's your server)

### Risks
- Stale memories: old decisions may no longer apply
  - Mitigate: consolidation includes "still valid?" check, expired decisions flagged
- Memory pollution: low-quality conversations dilute search
  - Mitigate: "Remember" flag for validated answers, weight flagged memories higher
