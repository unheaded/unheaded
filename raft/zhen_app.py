#!/usr/bin/env python3
"""
Zhen Web App — RAG UI for Unheaded Infrastructure
Port: 20103 (zhen-ui in Doom Range)

WAVE15 REWIRE (2026-05-02): backends retargeted from Mistral-7B / FAISS
to qwen-coder-7b / vor. The 1.76M-vector FAISS pipeline is retired;
retrieval flows through vor (cs serve at :9876). Inference is the same
llama-server qwen-coder-7b binary the Go agent uses (:8081). Conversation
memory still persists to The Well (zhen_memories + zhen_conversations).

Memory recall is now display-only (T1 closure): cached matches surface
in the response as a side-channel ("matched_memory") but never bypass
the live LLM call. /api/v1/corpus/stats and /api/v1/teach return 410-
shaped deprecation responses — vor is the substrate now, not FAISS.

Architecture spec:    ~/.claude/plans/synthetic-stirring-pudding.md
Battle plan:          docs/battle-plans/WAVE15-ZHENAI-REWIRE.md
H0 baseline:          eval/coding-gate/baseline-direct-cmd-zhen-rag-2026-05-02.md
Auxiliary roadmap:    docs/adr/ADR-056-pgvector-auxiliary-corpus-sharding.md
                      docs/adr/ADR-057-unheaded-source-code-indexing.md

Features (post-rewire):
- Local inference via llama.cpp at :8081 (qwen-coder-7b q4_k_m, ROCm GPU)
- Retrieval via vor at :9876 (1847+ Unheaded sheets, source-trust labeled)
- Conversation memory in zhen_memories (cosine recall, display-only)
- Conversation history in zhen_conversations (full-text + semantic)
- Runbook execution via /api/v1/runbooks/<name>/execute (Phase 2: gates
  through cmd/zhen-agentd for Champion enforcement)
- File-read sandbox via /api/v1/champion/read
- Skills + runbook listing
"""
import io
import json
import logging
import os
import sys
import time
import uuid
from pathlib import Path

import yaml
from flask import Flask, jsonify, request, send_file
from flask_cors import CORS

# Add scripts dir to path for RAG import
sys.path.insert(0, str(Path(__file__).parent / 'scripts'))
from zhen_rag import RAGPipeline

app = Flask(__name__, static_folder='static', static_url_path='/static')
CORS(app)
app.secret_key = os.environ.get('FLASK_SECRET_KEY', 'zhen-session-key-dev')  # override in production

# Initialize RAG pipeline.
#
# WAVE15 REWIRE: index_dir / corpus_file are vestigial — RAGPipeline accepts
# them as legacy positional args and ignores them. Retrieval is via vor on
# :9876; inference is qwen-coder via llama-server on :8081. Both URLs are
# overridable via env (VOR_URL, ZHEN_INFERENCE_URL); defaults match the
# Go-stack production deployment.
index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
corpus_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
corpus_file = corpus_dir / 'ring_all.jsonl'
if not corpus_file.exists():
    corpus_file = corpus_dir / 'ring1.jsonl'  # fallback

rag = None
startup_error = None

try:
    # WAVE15 Phase 2 amended: chat path stays DIRECT to llama-server.
    # Routing chat through cmd/zhen-agentd /api/v1/agent/ask regressed
    # H0 (the agent's JSON-shaped tool-call prompt truncated chat-style
    # answers — see eval/coding-gate/results-via-webui-phase2-2026-05-02
    # final pre-rollback grades and the "Phase 2 amendment" note in
    # docs/battle-plans/WAVE15-ZHENAI-REWIRE.md). The chat path's T6 was
    # theoretical (LLM answers displayed, never executed) so the real
    # T6 closure target is mutation paths — runbook_execute, etc. —
    # which use the new /api/v1/tool/exec endpoint via Champion.Dispatch
    # in zhen_app.py:execute_runbook (Phase 2b).
    #
    # Set ZHEN_PROXY_VIA_AGENTD=true to opt back into chat proxy for
    # debugging; default is false. The proxy_via_agentd code path is
    # KEPT for future experimentation (e.g., a chat-shape daemon
    # endpoint, ADR-058+).
    _proxy_env = os.environ.get('ZHEN_PROXY_VIA_AGENTD', 'false').strip().lower()
    _proxy = _proxy_env in ('true', '1', 'yes', 'on')
    rag = RAGPipeline(
        index_dir, corpus_file,
        vor_url=os.environ.get('VOR_URL', 'http://localhost:9876'),
        inference_url=os.environ.get('ZHEN_INFERENCE_URL', 'http://localhost:8081'),
        agentd_url=os.environ.get('ZHEN_AGENTD_URL', 'http://localhost:20105'),
        # Model name is metadata in the OpenAI-compat protocol — llama-server
        # serves whatever GGUF is loaded regardless of this string. Setting
        # ZHEN_MODEL keeps the response.model field accurate for the UI/logs.
        # See scripts/switch-model.sh for the canonical model-swap path.
        model_name=os.environ.get('ZHEN_MODEL', 'qwen2.5-coder-7b-instruct'),
        proxy_via_agentd=_proxy,
    )
except Exception as e:
    startup_error = str(e)
    print(f"WARNING: RAG not ready: {e}")

# ---------------------------------------------------------------------------
# PostgreSQL — optional conversation logging (The Well)
# ---------------------------------------------------------------------------
pg_conn = None
_session_id = str(uuid.uuid4())

# In-memory conversation history per client session (max 10 turns per session, max 100 sessions)
_conversation_histories = {}  # session_id -> [{'role': 'user'|'assistant', 'content': '...'}, ...]
_MAX_HISTORY_TURNS = 10
_MAX_SESSIONS = 100

def _get_history(session_id):
    """Get conversation history for a session."""
    return _conversation_histories.get(session_id, [])

def _add_to_history(session_id, role, content):
    """Add a turn to conversation history, maintaining size limits."""
    if session_id not in _conversation_histories:
        # Evict oldest session if at capacity
        if len(_conversation_histories) >= _MAX_SESSIONS:
            oldest = next(iter(_conversation_histories))
            del _conversation_histories[oldest]
        _conversation_histories[session_id] = []
    history = _conversation_histories[session_id]
    history.append({'role': role, 'content': content})
    # Keep only the last N turns
    if len(history) > _MAX_HISTORY_TURNS * 2:  # *2 because user+assistant = 2 entries per turn
        _conversation_histories[session_id] = history[-_MAX_HISTORY_TURNS * 2:]

def _pg_connect():
    """Try connecting to Postgres. Returns connection or None."""
    try:
        import psycopg2
        conn = psycopg2.connect(
            dbname=os.environ.get('ZHEN_DB_NAME', 'unheaded_app'),
            user=os.environ.get('ZHEN_DB_USER', 'app_zhen'),
            password=os.environ.get('ZHEN_DB_PASSWORD', ''),
            host=os.environ.get('ZHEN_DB_HOST', 'localhost'),
            port=int(os.environ.get('ZHEN_DB_PORT', '5432')),
            connect_timeout=3,
        )
        conn.autocommit = True
        logging.info("[zhen] Connected to The Well (PostgreSQL)")
        return conn
    except Exception as e:
        logging.warning(f"[zhen] The Well not available — conversations will not be persisted: {e}")
        return None

pg_conn = _pg_connect()

# Ensure zhen_memories table exists
def _ensure_memories_table():
    global pg_conn
    if pg_conn is None:
        return
    try:
        cur = pg_conn.cursor()
        cur.execute("""
            CREATE TABLE IF NOT EXISTS zhen_memories (
                id BIGSERIAL PRIMARY KEY,
                question TEXT NOT NULL,
                answer TEXT NOT NULL,
                embedding BYTEA,
                source VARCHAR(100) DEFAULT 'user',
                model VARCHAR(50),
                created_at TIMESTAMPTZ DEFAULT NOW()
            )
        """)
        cur.close()
    except Exception as e:
        logging.warning(f"[zhen] Failed to create zhen_memories table: {e}")

_ensure_memories_table()


def _pg_log(role, content, sources='[]', model='', tokens_input=0, tokens_output=0, elapsed_ms=0):
    """Insert a conversation row. Silently skips if Postgres is unavailable."""
    global pg_conn
    if pg_conn is None:
        return
    try:
        cur = pg_conn.cursor()
        cur.execute(
            """INSERT INTO zhen_conversations
               (session_id, role, content, sources, model, tokens_input, tokens_output, elapsed_ms)
               VALUES (%s, %s, %s, %s::jsonb, %s, %s, %s, %s)""",
            (_session_id, role, content, sources, model, tokens_input, tokens_output, elapsed_ms),
        )
        cur.close()
    except Exception as e:
        logging.warning(f"[zhen] Failed to log conversation: {e}")
        # Attempt reconnect on next call
        try:
            pg_conn.close()
        except Exception:
            pass
        pg_conn = _pg_connect()


def _recall_semantic(search_term, days=30, k=20):
    """Semantic search fallback for recall — embed the query, search conversations via cosine similarity."""
    global pg_conn
    if pg_conn is None or rag is None:
        return []
    try:
        import numpy as np
        # Embed the search term
        q_emb = rag.embedding_model.encode(search_term, convert_to_numpy=True).astype('float32')

        # Fetch recent conversations from The Well
        cur = pg_conn.cursor()
        cur.execute(
            """SELECT role, content, model, created_at
               FROM zhen_conversations
               WHERE created_at > NOW() - INTERVAL '%s days'
                 AND length(content) > 20
               ORDER BY created_at DESC
               LIMIT 500""",
            (days,),
        )
        rows = cur.fetchall()
        cur.close()

        if not rows:
            return []

        # Embed all conversation content and rank by similarity
        contents = [r[1] for r in rows]
        c_embs = rag.embedding_model.encode(contents, convert_to_numpy=True, batch_size=64).astype('float32')

        # Cosine similarity
        q_norm = q_emb / (np.linalg.norm(q_emb) + 1e-10)
        c_norms = c_embs / (np.linalg.norm(c_embs, axis=1, keepdims=True) + 1e-10)
        scores = c_norms @ q_norm

        # Top-k by similarity (threshold 0.3)
        top_idx = np.argsort(scores)[::-1][:k]
        results = []
        for idx in top_idx:
            if scores[idx] > 0.3:
                results.append(rows[idx])
        return results
    except Exception as e:
        logging.warning(f"[zhen] Semantic recall failed: {e}")
        return []


def _search_memories(question, threshold=0.9):
    """Search zhen_memories for a cached answer using embedding similarity."""
    global pg_conn
    if pg_conn is None or rag is None:
        return None
    try:
        cur = pg_conn.cursor()
        cur.execute("SELECT id, question, answer, embedding, model FROM zhen_memories ORDER BY created_at DESC LIMIT 200")
        rows = cur.fetchall()
        cur.close()
        if not rows:
            return None

        # Embed the query
        import numpy as np
        q_emb = rag.embedding_model.encode(question, convert_to_numpy=True).astype('float32')

        best_match = None
        best_sim = 0.0
        for row in rows:
            mem_id, mem_q, mem_a, mem_emb_bytes, mem_model = row
            if mem_emb_bytes is None:
                continue
            mem_emb = np.frombuffer(mem_emb_bytes, dtype='float32')
            # Cosine similarity
            sim = float(np.dot(q_emb, mem_emb) / (np.linalg.norm(q_emb) * np.linalg.norm(mem_emb) + 1e-8))
            if sim > best_sim:
                best_sim = sim
                best_match = {'id': mem_id, 'question': mem_q, 'answer': mem_a, 'model': mem_model, 'similarity': sim}

        if best_match and best_match['similarity'] >= threshold:
            return best_match
        return None
    except Exception as e:
        logging.warning(f"[zhen] Memory search failed: {e}")
        return None


# ──────────────────────────────────────────────────────────────────────
# Live-state context injection.
#
# vor only indexes filesystem-resident markdown — not live PG state.
# Without injection, the model hallucinates CLI commands like
# "kanban list --status ..." when asked about kanban tasks. With
# injection, it sees the actual zhen_actions / kanban_tasks rows and
# answers from real data.
#
# Detection is keyword-based per intent. Conservative — false positives
# cost a few hundred tokens of context (negligible at max_tokens=600);
# false negatives leave the model in pre-fix hallucination mode.
# ──────────────────────────────────────────────────────────────────────

_LIVE_INTENTS = {
    'kanban':    ('kanban', 'task', 'tasks', 'in progress', 'in-progress',
                   'todo', 'to-do', 'to do', 'backlog', 'pending', 'queue',
                   'review', 'done', 'completed', 'progress', 'sprint',
                   'column', 'columns', 'board', 'how many'),
    'audit':     ('audit', 'recent action', 'recent actions', 'audit log',
                   'last action', 'who ran', 'history'),
    'runbooks':  ('runbook', 'runbooks', 'execution', 'executions', 'runbook history',
                   'health sweep', 'recent runbook'),
    'health':    ('health', 'status', 'running', 'up', 'online', 'down',
                   'reachable', 'system state', 'kingdom state'),
}


def _detect_live_intents(question):
    """Return the set of context kinds this question needs."""
    q = (question or '').lower()
    intents = set()
    for kind, keywords in _LIVE_INTENTS.items():
        for kw in keywords:
            if kw in q:
                intents.add(kind)
                break
    return intents


def _build_live_context(question, max_chars=4096):
    """Pre-fetch live system state matching the question's intent.

    Returns a string suitable for prepending to the LLM's user message,
    or None when no live intent is detected. Capped at `max_chars` so
    we don't blow the context window on a bulk dump.
    """
    intents = _detect_live_intents(question)
    if not intents:
        return None

    parts = []

    if 'kanban' in intents and pg_conn is not None:
        try:
            import psycopg2
            kconn = psycopg2.connect(
                dbname='unheaded',
                user=os.environ.get('ZHEN_DB_USER', 'unheaded'),
                password=os.environ.get('ZHEN_DB_PASSWORD', ''),
                host=os.environ.get('ZHEN_DB_HOST', 'localhost'),
                port=int(os.environ.get('ZHEN_DB_PORT', '5432')),
                connect_timeout=2,
            )
            kconn.autocommit = True
            cur = kconn.cursor()
            cur.execute("""
                SELECT status, COUNT(*) FROM kanban_tasks
                 WHERE deleted_at IS NULL AND archived_at IS NULL
                 GROUP BY status ORDER BY status
            """)
            counts = dict(cur.fetchall())
            # Per-status listings (top N by recency) so the model never has to
            # infer counts from a truncated cross-status sample.
            status_lists = {}
            for status_key, total in counts.items():
                cur.execute("""
                    SELECT id, title, owner, progress
                      FROM kanban_tasks
                     WHERE deleted_at IS NULL AND archived_at IS NULL
                       AND status = %s
                     ORDER BY updated_at DESC NULLS LAST
                     LIMIT 12
                """, (status_key,))
                status_lists[status_key] = cur.fetchall()
            cur.close()
            kconn.close()
            total_active = sum(counts.values())
            status_keys_csv = ', '.join(f"'{s}'" for s in sorted(counts.keys()))
            lines = [
                '## Kanban tasks (live from PG, AUTHORITATIVE)',
                f'Total active (excludes deleted/archived): {total_active}',
                f'Canonical status vocabulary on this board: {status_keys_csv}.'
                ' (There is no separate "backlog" column — unstarted work lives'
                ' in `todo`. "Completed" maps to `done`.)',
                '',
                'EXACT COUNTS BY STATUS (use these numbers — the per-status'
                ' listings below are samples, not the full set):',
            ]
            for s in sorted(counts.keys()):
                lines.append(f'  - {s}: {counts[s]}')
            lines.append('')
            for s in sorted(counts.keys()):
                rows = status_lists.get(s, [])
                lines.append(f'### {s} ({counts[s]} total, showing up to 12 most recent)')
                for tid, title, owner, progress in rows:
                    lines.append(f'  - {tid}: {title}'
                                 + (f' (owner: {owner})' if owner else '')
                                 + (f' [{progress}%]' if progress else ''))
                if counts[s] > len(rows):
                    lines.append(f'  ... and {counts[s] - len(rows)} more in {s}')
                lines.append('')
            parts.append('\n'.join(lines).rstrip())
        except Exception as e:
            parts.append(f'## Kanban tasks: unavailable ({e})')

    if 'audit' in intents and pg_conn is not None:
        try:
            cur = pg_conn.cursor()
            cur.execute("""
                SELECT id, action_type, status, intent, triggered_by,
                       to_char(planned_at, 'YYYY-MM-DD HH24:MI:SS')
                  FROM zhen_actions
                 ORDER BY id DESC LIMIT 15
            """)
            rows = cur.fetchall()
            cur.close()
            lines = ['## Recent audit (zhen_actions, live from PG)']
            for r in rows:
                lines.append(f'  #{r[0]} [{r[5]}] {r[1]} → {r[2]}'
                             + (f' (by {r[4]})' if r[4] else '')
                             + (f': {r[3]}' if r[3] else ''))
            parts.append('\n'.join(lines))
        except Exception as e:
            parts.append(f'## Recent audit: unavailable ({e})')

    if 'runbooks' in intents:
        # Two parts: list of available runbooks (cached static) +
        # recent executions (live from PG).
        try:
            import glob as g
            rb_files = sorted(
                g.glob(os.path.join(RUNBOOKS_DIR, '**', '*.yaml'),
                       recursive=True))
            names = [os.path.relpath(f, RUNBOOKS_DIR).replace('.yaml', '')
                     for f in rb_files]
            lines = [f'## Runbooks available ({len(names)} total)']
            for n in names[:25]:
                lines.append(f'  - {n}')
            if len(names) > 25:
                lines.append(f'  ... and {len(names) - 25} more')
            parts.append('\n'.join(lines))
        except Exception:
            pass

        if pg_conn is not None:
            try:
                cur = pg_conn.cursor()
                cur.execute("""
                    SELECT id, status, intent, elapsed_ms,
                           to_char(planned_at, 'YYYY-MM-DD HH24:MI:SS')
                      FROM zhen_actions
                     WHERE action_type = 'runbook.execute'
                     ORDER BY id DESC LIMIT 10
                """)
                rows = cur.fetchall()
                cur.close()
                if rows:
                    lines = ['## Recent runbook executions (live from PG)']
                    for r in rows:
                        rid, status, intent, elapsed_ms, planned = r
                        lines.append(f'  #{rid} [{planned}] {status} '
                                     f'({elapsed_ms or 0}ms): {intent or ""}')
                    parts.append('\n'.join(lines))
            except Exception:
                pass

    if 'health' in intents:
        # Quick probes; same shape as /api/v1/system/state.
        import urllib.request as _ur
        probes = []
        for label, url in [
            ('vor (retrieval)',     f'{rag.vor_url}/api/health' if rag else ''),
            ('llama-server (LLM)',  f'{rag.inference_url}/health' if rag else ''),
            ('zhen-agentd (gate)',  os.environ.get('ZHEN_AGENTD_URL', 'http://localhost:20105') + '/health'),
            ('kanban-app',          'http://127.0.0.1:20001/health'),
            ('wiki',                'http://127.0.0.1:20002/health'),
            ('dashboard-backend',   'http://127.0.0.1:20000/health'),
            ('wotan (msg-bus)',     'http://127.0.0.1:18000/health'),
        ]:
            if not url:
                continue
            try:
                with _ur.urlopen(url, timeout=2) as r:
                    probes.append(f'  - {label}: ok')
            except Exception:
                probes.append(f'  - {label}: DOWN')
        if pg_conn is not None:
            probes.append('  - the well (PG): connected')
        else:
            probes.append('  - the well (PG): disconnected')
        parts.append('## Backend health (live)\n' + '\n'.join(probes))

    if not parts:
        return None
    blob = '\n\n'.join(parts)
    if len(blob) > max_chars:
        blob = blob[:max_chars] + '\n\n[...live context truncated]'
    return blob


@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        'status': 'ok' if rag else 'degraded',
        'rag_ready': rag is not None,
        'well_connected': pg_conn is not None,
        'session_id': _session_id,
        'error': startup_error,
    })


def _try_command(question):
    """Check if question is a direct command Zhenai can handle without LLM inference.
    Returns (response_dict, True) if handled, (None, False) if not a command."""
    q = question.lower().strip()

    # Help — list all commands
    if q in ('help', 'commands', 'list commands', '?'):
        return {'answer': """**Zhenai Commands** (no LLM needed):

**Operations:**
- `kingdom status` — full dashboard (services, memory, disk, GPU, training, EAST)
- `health` / `status` — service health sweep
- `drift check` / `bailiff` — compare desired vs actual state
- `trust level` — show current trust level

**Runbooks:**
- `list runbooks` — show all 51 runbooks by category
- `run <name>` — execute a runbook
- `run --dry <name>` — dry-run validation

**Scheduling:**
- `schedule <runbook> every <interval>` — schedule recurring execution
- `unschedule <runbook>` — remove schedule
- `list schedules` — show all scheduled jobs
- `emergency stop` — kill all jobs and schedules

**Memory:**
- `recall <topic>` — search past conversations
- `what did we decide about <X>` — find decisions

**Config:**
- `generate nginx|systemd|haproxy <service> [port]` — generate config templates

Type anything else to ask Zhenai via RAG + Mistral-7B inference.""", 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Kingdom Status — comprehensive dashboard
    if q in ('kingdom status', 'kingdom', 'full status', 'dashboard'):
        import subprocess as sp
        import urllib.request as ur
        lines = ['**Kingdom Status Report**\n']

        # Services
        services = {
            'wotan': 18000, 'timeguru': 19000, 'captain': 19002,
            'architect': 19001, 'micromanager': 19003,
            'dashboard': 20000, 'kanban': 16668,
            'zhenai': 20103, 'inference': 20100, 'akira': 16699,
        }
        healthy = 0
        for name, port in services.items():
            try:
                ur.urlopen(f'http://localhost:{port}/health', timeout=2)
                healthy += 1
            except: pass
        lines.append(f'**Services:** {healthy}/{len(services)} healthy')

        # Memory
        try:
            with open('/proc/meminfo') as f:
                mem = {}
                for line in f:
                    parts = line.split()
                    if len(parts) >= 2: mem[parts[0].rstrip(':')] = int(parts[1])
            total = mem.get('MemTotal', 0) // 1024
            avail = mem.get('MemAvailable', 0) // 1024
            swap_used = (mem.get('SwapTotal', 0) - mem.get('SwapFree', 0)) // 1024
            lines.append(f'**Memory:** {total - avail}MB / {total}MB (swap: {swap_used}MB)')
        except: pass

        # Disk
        try:
            result = sp.run(['df', '-h', '/'], capture_output=True, text=True, timeout=5, check=False)
            for line in result.stdout.strip().split('\n')[1:]:
                parts = line.split()
                lines.append(f'**Disk /:** {parts[2]} used / {parts[1]} ({parts[4]})')
        except: pass

        # GPU
        try:
            result = sp.run(['rocm-smi'], capture_output=True, text=True, timeout=5, check=False)
            for line in result.stdout.split('\n'):
                if 'Temp' in line and '°C' in line:
                    lines.append(f'**GPU:** {line.strip()}')
                    break
        except: pass

        # Training
        try:
            with open('/tmp/forge-production.log') as f:
                last = [l for l in f if 'Loss' in l]
                if last:
                    lines.append(f'**Training:** {last[-1].strip()}')
        except: pass

        # Packages
        try:
            result = sp.run(['dpkg', '-l'], capture_output=True, text=True, timeout=5, check=False)
            pkg_count = len([l for l in result.stdout.split('\n') if 'unheaded' in l])
            lines.append(f'**Packages:** {pkg_count} installed')
        except: pass

        # EAST
        try:
            result = sp.run(['ssh', 'govan@east', 'uptime'], capture_output=True, text=True, timeout=5, check=False)
            if result.returncode == 0:
                lines.append(f'**EAST:** {result.stdout.strip()}')
            else:
                lines.append('**EAST:** unreachable')
        except:
            lines.append('**EAST:** unreachable')

        return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Runbook commands
    if q in ('list runbooks', 'show runbooks', 'runbooks'):
        import glob as g
        rbs = sorted(g.glob(os.path.join(RUNBOOKS_DIR, '**', '*.yaml'), recursive=True))
        lines = [f"**{len(rbs)} runbooks available:**\n"]
        by_cat = {}
        for rb in rbs:
            rel = os.path.relpath(rb, RUNBOOKS_DIR)
            cat = rel.split(os.sep)[0]
            name = os.path.splitext(os.path.basename(rb))[0]
            by_cat.setdefault(cat, []).append(name)
        for cat in sorted(by_cat):
            lines.append(f"\n**{cat}/** ({len(by_cat[cat])})")
            for name in sorted(by_cat[cat]):
                lines.append(f"  - {name}")
        lines.append("\nTo run: type `run <name>` or `run --dry <name>`")
        return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Run runbook
    if q.startswith('run ') or q.startswith('execute '):
        import subprocess as sp
        parts = q.split(None, 2)
        dry_run = '--dry' in q or '--dry-run' in q
        name = parts[-1].replace('--dry', '').replace('--dry-run', '').strip()
        import glob as g
        matches = g.glob(os.path.join(RUNBOOKS_DIR, '**', f'{name}.yaml'), recursive=True)
        if not matches:
            return {'answer': f'Runbook not found: `{name}`', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        runner = os.path.expanduser('~/tmp/unheaded/scripts/run-runbook.py')
        cmd = ['python3', runner]
        if dry_run:
            cmd.append('--dry-run')
        cmd.append(matches[0])
        try:
            result = sp.run(cmd, capture_output=True, text=True, timeout=120, check=False)
            status = 'SUCCESS' if result.returncode == 0 else 'FAILED'
            output = result.stdout[-3000:]
            return {'answer': f'**Runbook {name}: {status}**\n\n```\n{output}\n```', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except sp.TimeoutExpired:
            return {'answer': f'Runbook `{name}` timed out (120s)', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Service health
    if q in ('health', 'service health', 'check health', 'status'):
        import urllib.request as ur
        services = {
            'wotan': 18000, 'timeguru': 19000, 'captain': 19002,
            'architect': 19001, 'micromanager': 19003, 'monad': 19004,
            'sophia': 19005, 'dashboard': 20000, 'kanban': 16668,
            'zhenai': 20103, 'inference': 20100,
        }
        lines = ['**Kingdom Service Health:**\n']
        healthy = 0
        for name, port in services.items():
            try:
                ur.urlopen(f'http://localhost:{port}/health', timeout=2)
                lines.append(f'  ✓ {name} (:{port})')
                healthy += 1
            except Exception:
                lines.append(f'  ✗ {name} (:{port})')
        lines.append(f'\n**{healthy}/{len(services)} healthy**')
        return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Bailiff — drift detection: compare desired state vs actual state
    if q in ('drift check', 'bailiff', 'check drift', 'drift'):
        import urllib.request as ur
        desired_services = {
            'wotan': 18000, 'timeguru': 19000, 'captain': 19002,
            'architect': 19001, 'micromanager': 19003, 'monad': 19004,
            'sophia': 19005, 'dashboard': 20000, 'kanban': 16668,
            'zhenai': 20103, 'inference': 20100, 'akira': 16699,
        }
        lines = ['**Bailiff Drift Report:**\n']
        drifts = 0
        for name, port in desired_services.items():
            try:
                ur.urlopen(f'http://localhost:{port}/health', timeout=2)
                lines.append(f'  ✓ {name} (:{port}) — running')
            except Exception:
                lines.append(f'  ✗ {name} (:{port}) — **DRIFT: should be running but is DOWN**')
                drifts += 1

        # Check systemd units
        import subprocess as sp
        try:
            result = sp.run(['systemctl', 'list-units', '--type=service', '--state=failed', '--no-pager', '-q'],
                          capture_output=True, text=True, timeout=5, check=False)
            failed_units = [l.strip().split()[0] for l in result.stdout.strip().split('\n') if l.strip() and 'unheaded' in l]
            for unit in failed_units:
                lines.append(f'  ⚠ {unit} — **DRIFT: systemd unit FAILED**')
                drifts += 1
        except Exception:
            pass

        if drifts == 0:
            lines.append('\n**No drift detected.** All services aligned with desired state.')
        else:
            lines.append(f'\n**{drifts} drift(s) detected.** Run `run service-health-sweep` for details.')

        return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Config generation: generate nginx|systemd|haproxy <service> <port>
    if q.startswith('generate '):
        parts = q.split()
        if len(parts) < 3:
            return {'answer': 'Usage: `generate nginx|systemd|haproxy <service> [port]`', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

        config_type = parts[1]
        svc_name = parts[2]
        port = int(parts[3]) if len(parts) > 3 else 8080

        if config_type == 'nginx':
            config = f"""# Generated by Zhenai — nginx config for {svc_name}
upstream {svc_name}_backend {{
    server 127.0.0.1:{port};
}}

server {{
    listen 80;
    server_name {svc_name}.unheaded.local;

    location / {{
        proxy_pass http://{svc_name}_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}

    location /health {{
        proxy_pass http://{svc_name}_backend/health;
    }}
}}"""
        elif config_type == 'systemd':
            config = f"""# Generated by Zhenai — systemd unit for {svc_name}
[Unit]
Description=Unheaded {svc_name}
After=network.target unheaded-wotan.service
Wants=unheaded-wotan.service

[Service]
Type=simple
User=unheaded
Group=unheaded
ExecStart=/opt/unheaded/bin/{svc_name}
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/unheaded /var/log/unheaded
Environment=PORT={port}

[Install]
WantedBy=multi-user.target"""
        elif config_type == 'haproxy':
            config = f"""# Generated by Zhenai — haproxy backend for {svc_name}
backend {svc_name}_back
    mode http
    balance roundrobin
    option httpchk GET /health
    http-check expect status 200
    server {svc_name}1 127.0.0.1:{port} check inter 10s fall 3 rise 2"""
        else:
            return {'answer': f'Unknown config type: `{config_type}`. Use nginx, systemd, or haproxy.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

        return {'answer': f'**Generated {config_type} config for {svc_name} (port {port}):**\n\n```\n{config}\n```', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Schedule commands
    if q.startswith('schedule ') and 'every' in q:
        # schedule service-health-sweep every 30m
        parts = q.split()
        name = parts[1] if len(parts) > 1 else ''
        interval = parts[-1] if len(parts) > 3 else '30m'
        try:
            from zhen_scheduler import add_schedule
            entry = add_schedule(name, interval)
            return {'answer': f'Scheduled `{name}` every {interval}.\n\nTo start the scheduler daemon: `python3 zhen_scheduler.py`', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except Exception as e:
            return {'answer': f'Schedule error: {e}', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    if q.startswith('unschedule '):
        name = q.split(None, 1)[1].strip() if len(q.split()) > 1 else ''
        try:
            from zhen_scheduler import remove_schedule
            remove_schedule(name)
            return {'answer': f'Unscheduled `{name}`.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except Exception as e:
            return {'answer': f'Unschedule error: {e}', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    if q in ('list schedules', 'schedules', 'show schedules'):
        try:
            from zhen_scheduler import load_schedules
            schedules = load_schedules()
            lines = [f'**Scheduled Jobs ({len(schedules)}):**\n']
            for s in schedules:
                status = 'ENABLED' if s.get('enabled') else 'disabled'
                lines.append(f'  {"●" if s.get("enabled") else "○"} `{s["name"]}` — {s.get("interval", "?")} [{status}]')
            lines.append('\nScheduler daemon: `python3 zhen_scheduler.py`')
            return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except Exception as e:
            return {'answer': f'Schedule list error: {e}', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Emergency stop
    if q in ('emergency stop', 'stop all', 'kill all'):
        import subprocess as sp
        sp.run(['pkill', '-f', 'zhen_scheduler'], capture_output=True, check=False)
        sp.run(['pkill', '-f', 'run-runbook'], capture_output=True, check=False)
        return {'answer': '**EMERGENCY STOP** — All scheduled jobs killed. Runbook executions terminated.\n\nManual-only mode until scheduler is restarted.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Trust level
    if 'trust level' in q or 'trust' == q:
        return {'answer': f'**Zhenai Trust Level: {TRUST_LEVEL}**\n\nLevel 1: Read-only\nLevel 2: Write (high/critical need approval) ← current\nLevel 3: Autonomous', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # Recall — search past conversations
    if q.startswith('recall '):
        search_term = question[7:].strip()
        if not search_term:
            return {'answer': 'Usage: `recall <topic>` or `recall last week`', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

        # Handle time-based recalls
        days = 30
        if 'last week' in search_term:
            days = 7
            search_term = search_term.replace('last week', '').strip() or 'conversation'
        elif 'yesterday' in search_term:
            days = 1
            search_term = search_term.replace('yesterday', '').strip() or 'conversation'
        elif 'last month' in search_term:
            days = 30
            search_term = search_term.replace('last month', '').strip() or 'conversation'

        global pg_conn
        if pg_conn is None:
            return {'answer': 'The Well is not connected — conversation history unavailable.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

        try:
            cur = pg_conn.cursor()
            # Try full-text search first
            cur.execute(
                """SELECT role, content, model, created_at
                   FROM zhen_conversations
                   WHERE created_at > NOW() - INTERVAL '%s days'
                     AND (content ILIKE %s OR
                          (search_vector IS NOT NULL AND search_vector @@ websearch_to_tsquery('english', %s)))
                   ORDER BY created_at DESC
                   LIMIT 20""",
                (days, f'%{search_term}%', search_term),
            )
            rows = cur.fetchall()
            cur.close()

            if not rows:
                # B3: Semantic search fallback — use embedding similarity on conversation content
                rows = _recall_semantic(search_term, days)
                if not rows:
                    return {'answer': f'No conversations found matching "{search_term}" in the last {days} days.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
                lines = [f'**Found {len(rows)} messages via semantic search for "{search_term}" (last {days} days):**\n']
            else:
                lines = [f'**Found {len(rows)} messages matching "{search_term}" (last {days} days):**\n']

            for role, content, model, created_at in rows:
                ts = created_at.strftime('%Y-%m-%d %H:%M') if created_at else '?'
                prefix = '**You:**' if role == 'user' else f'**Zhenai ({model or "?"}):**'
                lines.append(f'`{ts}` {prefix} {content[:200]}{"..." if len(content) > 200 else ""}\n')

            return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except Exception as e:
            return {'answer': f'Recall search error: {e}', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    # "what did we decide about X" — search conversation history for decisions
    if q.startswith('what did we decide') or q.startswith('what was decided'):
        topic = q.split('about', 1)[-1].strip().strip('?') if 'about' in q else q.split('decide', 1)[-1].strip().strip('?')
        if not topic:
            return {'answer': 'Usage: `what did we decide about <topic>?`', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        if pg_conn is None:
            return {'answer': 'The Well is not connected — conversation history unavailable.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

        try:
            cur = pg_conn.cursor()
            # Search for decision-like patterns in conversations
            cur.execute(
                """SELECT role, content, model, created_at
                   FROM zhen_conversations
                   WHERE (content ILIKE %s OR
                          (search_vector IS NOT NULL AND search_vector @@ websearch_to_tsquery('english', %s)))
                     AND (content ILIKE '%%decided%%' OR content ILIKE '%%decision%%' OR content ILIKE '%%chose%%'
                          OR content ILIKE '%%accepted%%' OR content ILIKE '%%approved%%' OR content ILIKE '%%agreed%%'
                          OR role = 'assistant')
                   ORDER BY created_at DESC
                   LIMIT 10""",
                (f'%{topic}%', topic),
            )
            rows = cur.fetchall()
            cur.close()

            if not rows:
                # Semantic fallback for decisions
                rows = _recall_semantic(f"decided {topic}", days=90, k=10)
                if not rows:
                    return {'answer': f'No decisions found about "{topic}" in conversation history.\n\nTry `recall {topic}` for a broader search.', 'model': 'command', 'tokens_used': 0, 'sources': []}, True
                lines = [f'**Decisions about "{topic}" (via semantic search):**\n']
            else:
                lines = [f'**Decisions about "{topic}":**\n']
            for role, content, model, created_at in rows:
                ts = created_at.strftime('%Y-%m-%d %H:%M') if created_at else '?'
                prefix = '**You:**' if role == 'user' else '**Zhenai:**'
                lines.append(f'`{ts}` {prefix} {content[:300]}{"..." if len(content) > 300 else ""}\n')

            return {'answer': '\n'.join(lines), 'model': 'command', 'tokens_used': 0, 'sources': []}, True
        except Exception as e:
            return {'answer': f'Decision search error: {e}', 'model': 'command', 'tokens_used': 0, 'sources': []}, True

    return None, False


@app.route('/api/v1/query', methods=['POST'])
def query():
    if not rag:
        return jsonify({'error': f'RAG not initialized: {startup_error}'}), 503

    data = request.json
    question = data.get('question', '').strip()
    if not question:
        return jsonify({'error': 'Question required'}), 400

    # Try direct command handling first (no LLM needed)
    cmd_result, handled = _try_command(question)
    if handled:
        return jsonify({
            'question': question,
            **cmd_result,
            'elapsed_seconds': 0.0,
            'from_command': True,
        })

    file_content = data.get('file_content', None)
    client_session = data.get('session_id', 'default')

    try:
        # WAVE15: memory recall is DISPLAY-ONLY (T1 closure).
        #
        # Pre-rewire behavior: cosine similarity >= 0.9 returned the cached
        # answer and BYPASSED the live LLM. That was T1 (stored prompt
        # injection via memory replay) by construction — a poisoned prior
        # turn could be cached and replayed without going through the gate.
        #
        # Post-rewire behavior: cached match is fetched as a side-channel
        # ("matched_memory" field on the response), but rag.query() ALWAYS
        # runs the full RAG path. The UI can render the matched memory next
        # to the live answer; it never replaces it.
        memory = _search_memories(question)

        history = _get_history(client_session)

        # Live-state context injection: if the question asks about live
        # system data (kanban tasks, runbook history, audit feed, daemon
        # health), pre-fetch the relevant authoritative state and pass
        # it to the LLM. Without this, the model hallucinates CLI commands
        # ("kanban list --status ...") instead of reading actual rows.
        live_ctx = _build_live_context(question)

        # Identity block — ALWAYS prepended, regardless of question
        # intent. Without this, the model hallucinates "I'm Claude" when
        # asked what model it is (qwen-coder is fine-tuned on a lot of
        # Anthropic-style text). The system prompt instructs the model
        # to read inference_model from the LIVE SYSTEM STATE block;
        # this is what populates that key. Cheap to compute (single
        # /v1/models call) and small (~50 chars), so unconditional.
        identity_block = ''
        try:
            import urllib.request as _ur
            with _ur.urlopen(f'{rag.inference_url}/v1/models', timeout=2) as _r:
                _data = json.loads(_r.read().decode())
                _loaded = (_data.get('data') or [{}])[0].get('id', '')
                if _loaded:
                    _loaded = _loaded.removesuffix('.gguf')
                    identity_block = (
                        f"## Identity (authoritative)\n"
                        f"inference_model: {_loaded}\n"
                        f"served_by: local llama-server on AMD RX 7700 XT (ROCm)\n"
                        f"persona: zhen (真爱), chat surface of the Unheaded Kingdom\n"
                    )
        except Exception:
            pass
        if identity_block:
            live_ctx = identity_block + ('\n\n' + live_ctx if live_ctx else '')

        start = time.time()
        result = rag.query(question, file_content=file_content, history=history,
                          live_context=live_ctx)
        elapsed = time.time() - start
        elapsed_ms = int(elapsed * 1000)

        sources_list = [
            {
                'id': c['id'],
                'source': c['source'],
                'type': c['type'],
                'preview': c['content'][:200],
                'distance': c['distance'],
            }
            for c in result['retrieved'][:5]
        ]

        result_model = result.get('model', 'qwen2.5-coder-7b-instruct')

        # Track conversation history for follow-ups
        _add_to_history(client_session, 'user', question)
        _add_to_history(client_session, 'assistant', result['answer'])

        # Log to The Well (optional)
        _pg_log('user', question)
        _pg_log('assistant', result['answer'],
                sources=json.dumps(sources_list),
                model=result_model,
                tokens_output=result['tokens_used'],
                elapsed_ms=elapsed_ms)

        return jsonify({
            'question': result['question'],
            'answer': result['answer'],
            'sources': sources_list,
            'tokens_used': result['tokens_used'],
            'elapsed_seconds': round(elapsed, 2),
            'model': result_model,
            # WAVE15 side-channel: matched memory shown in the UI alongside
            # the live answer; never replaces it. None when no match.
            'matched_memory': memory,
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/search', methods=['POST'])
def search():
    """Semantic search only — no generation"""
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    data = request.json
    query_text = data.get('query', '').strip()
    k = min(data.get('k', 10), 50)

    if not query_text:
        return jsonify({'error': 'Query required'}), 400

    results = rag.retrieve(query_text, k=k)
    return jsonify({
        'query': query_text,
        'results': [
            {
                'id': r['id'],
                'source': r['source'],
                'type': r['type'],
                'content': r['content'][:500],
                'distance': r['distance'],
            }
            for r in results
        ],
        'total': len(results),
    })


# ──────────────────────────────────────────────────────────────────────
# /api/v1/models — list + switch (ADR-060)
#
# GET  /api/v1/models           → list allowlisted model keys (parsed
#                                  once at boot from scripts/switch-model.sh
#                                  MODEL_FILE entries; cached in process)
# POST /api/v1/models/switch    → proxy to cmd/zhen-agentd /api/v1/tool/exec
#                                  with tool=model_switch + direct-user
#                                  justification. The daemon's
#                                  pkg/champion ModelSwap enforces:
#                                    - allowlist (defense-in-depth here too)
#                                    - single-flight (T13)
#                                    - script-hash TOCTOU (T15)
#                                    - 6-min subprocess cap (T19)
#                                  Returns the daemon's body verbatim plus
#                                  status code, so the UI can surface
#                                  exactly what happened.
# ──────────────────────────────────────────────────────────────────────


_MODEL_KEYS_CACHE = None
_MODEL_FILE_TO_KEY_CACHE = None


def _load_model_keys():
    """Parse scripts/switch-model.sh once and cache MODEL_FILE keys + filename map.

    Returns the keys in declaration order. Also populates
    _MODEL_FILE_TO_KEY_CACHE: gguf-basename → key, so we can back-map
    llama-server's /v1/models response to a canonical sidebar-dropdown
    key (the key the UI just sent in the swap request).

    Mirrors the Go-side parseModelKeys() in pkg/champion/modelswap.go.
    """
    global _MODEL_KEYS_CACHE, _MODEL_FILE_TO_KEY_CACHE
    if _MODEL_KEYS_CACHE is not None:
        return _MODEL_KEYS_CACHE

    project_root = os.environ.get('UNHEADED_ROOT', '/home/govan/tmp/unheaded')
    script_path = os.path.join(project_root, 'scripts', 'switch-model.sh')
    keys = []
    file_to_key = {}
    seen = set()
    try:
        import re as _re
        key_re = _re.compile(r'^MODEL_FILE\[([a-z0-9][a-z0-9-]*)\]="([^"]+)"')
        with open(script_path, 'r') as f:
            for line in f:
                m = key_re.match(line)
                if m and m.group(1) not in seen:
                    key = m.group(1)
                    fname = m.group(2)
                    keys.append(key)
                    seen.add(key)
                    # Index both with and without .gguf extension for
                    # tolerant matching against llama-server output
                    # (which sometimes strips, sometimes keeps the .gguf).
                    file_to_key[fname] = key
                    if fname.endswith('.gguf'):
                        file_to_key[fname[:-5]] = key
    except Exception as e:
        app.logger.warning(f'load_model_keys: {e}')
        keys = []

    _MODEL_KEYS_CACHE = keys
    _MODEL_FILE_TO_KEY_CACHE = file_to_key
    return keys


def _live_loaded_key():
    """Return the dropdown key that matches llama-server's currently-loaded
    model file, or None if llama-server is unreachable / serving an
    unregistered file. Source-of-truth for the UI dropdown's "current"
    label after a swap.
    """
    _load_model_keys()  # populate _MODEL_FILE_TO_KEY_CACHE if needed
    if not rag:
        return None
    try:
        import urllib.request as _ur
        with _ur.urlopen(f'{rag.inference_url}/v1/models', timeout=2) as resp:
            data = json.loads(resp.read().decode())
            loaded = (data.get('data') or [{}])[0].get('id', '')
            return _MODEL_FILE_TO_KEY_CACHE.get(loaded)
    except Exception:
        return None


@app.route('/api/v1/models', methods=['GET'])
def list_models():
    """List the allowlisted model keys the operator can swap to.

    The keys come from scripts/switch-model.sh (MODEL_FILE bash array).
    The daemon's pkg/champion enforces the same allowlist, so this list
    can be trusted to drive the UI dropdown.
    """
    keys = _load_model_keys()
    # `current` is the *sidebar key* (e.g. "qwen-coder-14b") matching
    # what llama-server is actually serving, NOT rag.model_name (which
    # is the env-set display label captured at boot). Sidebar dropdown
    # checks current === selected to decide when a swap is complete.
    current_key = _live_loaded_key()
    return jsonify({
        'keys':    keys,
        'current': current_key,
        # Display label for the sidebar status line — convenience for UIs
        # that want to render the friendly name. Comes from llama-server
        # fresh; stale env value is the fallback.
        'current_label': rag.model_name if (rag and current_key is None) else None,
        'count':   len(keys),
    })


@app.route('/api/v1/models/switch', methods=['POST'])
def switch_model():
    """Proxy to cmd/zhen-agentd /api/v1/tool/exec with tool=model_switch.

    Body: {"key": "<allowlisted-key>"}
    Returns the daemon's body verbatim with the same HTTP status. UI
    polls /api/v1/stats for the model name to update once the swap
    completes (45s-3min depending on which model).
    """
    if not request.is_json:
        return jsonify({'error': 'expected JSON body'}), 400

    body = request.get_json(silent=True) or {}
    key = body.get('key', '').strip()
    if not key:
        return jsonify({'error': 'missing required field: key'}), 400

    # Defense in depth: pre-allowlist check on the Python side too. The
    # daemon will reject if we miss something, but failing fast here
    # gives the UI a tighter loop.
    keys = _load_model_keys()
    if key not in keys:
        return jsonify({
            'error':   f'unknown model key: {key}',
            'allowed': keys,
        }), 400

    import urllib.error as _ue
    import urllib.request as _ur
    agentd_url = os.environ.get('ZHEN_AGENTD_URL', 'http://localhost:20105')
    payload = {
        'tool': 'model_switch',
        'args': {'key': key},
        'session_id':    f'modelswap-{key}-{int(time.time())}',
        'emitted_by':    'zhen-web-ui',
        'justification': [{
            'topic':        'user-clicked-model-switch',
            'category':     'user-action',
            'source_kind':  'user-action',
            'source_trust': 'direct-user',
            'source_label': '/api/v1/models/switch',
        }],
    }
    req_body = json.dumps(payload).encode('utf-8')
    req = _ur.Request(
        f'{agentd_url}/api/v1/tool/exec',
        data=req_body,
        headers={'Content-Type': 'application/json'},
        method='POST',
    )
    # 7-minute cap matches the daemon's 6-min subprocess limit + buffer
    # for round-trip + chat-template init on cold-cache loads.
    try:
        with _ur.urlopen(req, timeout=420) as resp:
            return jsonify(json.loads(resp.read().decode('utf-8'))), resp.status
    except _ue.HTTPError as e:
        try:
            return jsonify(json.loads(e.read().decode('utf-8'))), e.code
        except Exception:
            return jsonify({'status': 'error', 'reason': str(e)}), e.code or 502
    except _ue.URLError as e:
        return jsonify({
            'status': 'error',
            'error':  f'zhen-agentd unreachable at {agentd_url}: {e}',
            'note':   'Start the daemon with `bin/zhen-agentd -port 20105` (and WELL_DSN env).',
        }), 503
    except Exception as e:
        return jsonify({'status': 'error', 'error': str(e)}), 500


@app.route('/api/v1/stats', methods=['GET'])
def stats():
    """Backend + memory-embedder stats — WAVE15 rewire shape.

    Pre-rewire reported FAISS index size and per-ring chunk counts.
    Post-rewire vor is the substrate; the only local index we keep is
    the small all-MiniLM-L6-v2 model used for memory-cache cosine recall.
    """
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    # Probe vor for canonical retrieval-index size (best-effort — vor
    # may not always answer in time; treat the stats endpoint as
    # informational, not gating).
    vor_health = None
    try:
        import urllib.request as _ur
        with _ur.urlopen(f'{rag.vor_url}/api/health', timeout=2) as resp:
            vor_health = json.loads(resp.read().decode())
    except Exception:
        vor_health = None

    # Live model name from llama-server's /v1/models — this is the source
    # of truth after a swap (ADR-060). rag.model_name is the env-set
    # display label captured at boot; if Stevie swaps via the sidebar
    # dropdown, llama-server moves to the new GGUF but rag.model_name
    # would otherwise stay stale, causing the UI's poll-after-swap to
    # never confirm completion. Read live; fall back to the env label
    # when llama-server is unreachable.
    inference_model = rag.model_name
    try:
        import urllib.request as _ur
        with _ur.urlopen(f'{rag.inference_url}/v1/models', timeout=2) as resp:
            data = json.loads(resp.read().decode())
            loaded = (data.get('data') or [{}])[0].get('id', '')
            if loaded:
                # llama-server returns the GGUF basename (e.g.
                # "Qwen2.5-Coder-14B-Instruct-Q4_K_M.gguf"). Strip the
                # .gguf suffix to get a friendlier label without losing
                # the file-identity match the UI poll needs.
                loaded = loaded.removesuffix('.gguf')
                inference_model = loaded
    except Exception:
        pass

    return jsonify({
        'backend':           'vor',
        'vor_url':           rag.vor_url,
        'vor_health':        vor_health,
        'inference_url':     rag.inference_url,
        'inference_model':   inference_model,
        'memory_embedder':   'all-MiniLM-L6-v2',
        'memory_dim':        384,
        'local_max_tokens':  rag.local_max_tokens,
        'memory_recall':     'display-only (T1 closed)',
    })


@app.route('/api/v1/corpus/stats', methods=['GET'])
def corpus_stats():
    """Corpus statistics — DEPRECATED post-WAVE15 rewire.

    Pre-rewire this scanned every JSONL file in raft/corpus/ counting lines.
    The scan over the 2.3 GB ring_all.jsonl was the proximate cause of the
    Werkzeug dev-server deadlock observed 2026-05-02 (one request held the
    GIL for 6+ minutes while subsequent requests piled up in CLOSE-WAIT).

    Post-rewire vor is the retrieval substrate; per-ring chunk counts no
    longer exist as a concept. Refer the caller to vor's /api/health for
    the canonical index size.
    """
    return jsonify({
        'status':       'deprecated',
        'reason':       'WAVE15 rewire: vor is now the retrieval substrate; per-ring chunk counts no longer apply.',
        'backend':      'vor',
        'vor_health':   f'{rag.vor_url if rag else "?"}/api/health',
        'note':         'see docs/battle-plans/WAVE15-ZHENAI-REWIRE.md §Phase 1 step 1.5',
    })


# ──────────────────────────────────────────────────────────────────────
# Operator interaction surface (post-WAVE15) — endpoints that drive the
# inline-action / dashboard / sidebar UI views. All read-only here;
# mutations go through cmd/zhen-agentd /api/v1/tool/exec (Champion gate).
# ──────────────────────────────────────────────────────────────────────


@app.route('/api/v1/system/state', methods=['GET'])
def system_state():
    """Live aggregator: backend health + counts for the operator views.

    Returned shape is intentionally flat + JSON-friendly so the UI can
    render any subset without conditional unpacking. Each backend probe
    is bounded by a 2 s timeout; if a backend is down its slot is null
    and the operator sees that clearly in the UI.
    """
    import urllib.request as _ur
    state = {
        'timestamp': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        'webui':     {'status': 'ok', 'rag_ready': rag is not None,
                      'well_connected': pg_conn is not None},
    }

    # vor
    try:
        with _ur.urlopen(f'{rag.vor_url}/api/health' if rag else '', timeout=2) as r:
            state['vor'] = json.loads(r.read())
    except Exception:
        state['vor'] = None

    # llama-server inference
    try:
        with _ur.urlopen(f'{rag.inference_url}/v1/models' if rag else '', timeout=2) as r:
            d = json.loads(r.read())
            models = d.get('models') or d.get('data') or []
            state['llama_server'] = {
                'status': 'ok',
                'model':  (models[0].get('name') or models[0].get('id') if models else None),
            }
    except Exception:
        state['llama_server'] = None

    # zhen-agentd (mutation gate)
    try:
        agentd_url = os.environ.get('ZHEN_AGENTD_URL', 'http://localhost:20105')
        with _ur.urlopen(f'{agentd_url}/health', timeout=2) as r:
            state['agentd'] = {'status': 'ok', 'url': agentd_url}
    except Exception:
        state['agentd'] = None

    # kanban-app (operator front-door)
    try:
        with _ur.urlopen('http://127.0.0.1:20001/health', timeout=2) as r:
            d = json.loads(r.read())
            state['kanban'] = {
                'status':         d.get('status'),
                'wotan_enabled':  d.get('wotan_enabled'),
                'url':            'http://localhost:20001',
            }
    except Exception:
        state['kanban'] = None

    # wiki (read-only)
    try:
        with _ur.urlopen('http://127.0.0.1:20002/health', timeout=2) as r:
            state['wiki'] = {'status': 'ok', 'url': 'http://localhost:20002'}
    except Exception:
        state['wiki'] = None

    # dashboard-backend (Prometheus + WebSocket metrics surface)
    try:
        with _ur.urlopen('http://127.0.0.1:20000/health', timeout=2) as r:
            d = json.loads(r.read())
            state['dashboard'] = {
                'status': d.get('status', 'ok'),
                'url':    'http://localhost:20000',
            }
    except Exception:
        state['dashboard'] = None

    # wotan (message bus — HTTP control + gRPC streaming)
    try:
        with _ur.urlopen('http://127.0.0.1:18000/health', timeout=2) as r:
            d = json.loads(r.read())
            state['wotan'] = {
                'status':         d.get('status', 'ok'),
                'rooms':          d.get('rooms', 0),
                'total_members':  d.get('total_members', 0),
                'url':            'http://localhost:18000',
                'grpc':           'localhost:18001',
            }
    except Exception:
        state['wotan'] = None

    return jsonify(state)


@app.route('/api/v1/audit/recent', methods=['GET'])
def audit_recent():
    """Recent zhen_actions audit rows. Operator-visible audit trail.

    Query params:
      limit (int, default 25, max 200) — how many rows to return
      action_type (optional) — filter by action_type prefix (e.g. 'runbook')
    """
    limit = min(int(request.args.get('limit', 25)), 200)
    action_type_filter = request.args.get('action_type', '').strip()

    if pg_conn is None:
        return jsonify({'rows': [], 'error': 'The Well not connected'}), 503
    try:
        cur = pg_conn.cursor()
        if action_type_filter:
            cur.execute("""
                SELECT id, action_type, status, intent, parameters, result_summary,
                       error, triggered_by, planned_at, completed_at, elapsed_ms
                  FROM zhen_actions
                 WHERE action_type LIKE %s
                 ORDER BY id DESC
                 LIMIT %s
            """, (action_type_filter + '%', limit))
        else:
            cur.execute("""
                SELECT id, action_type, status, intent, parameters, result_summary,
                       error, triggered_by, planned_at, completed_at, elapsed_ms
                  FROM zhen_actions
                 ORDER BY id DESC
                 LIMIT %s
            """, (limit,))
        rows = []
        for r in cur.fetchall():
            rows.append({
                'id':              r[0],
                'action_type':     r[1],
                'status':          r[2],
                'intent':          r[3],
                'parameters':      r[4],
                'result_summary':  r[5],
                'error':           r[6],
                'triggered_by':    r[7],
                'planned_at':      r[8].isoformat() if r[8] else None,
                'completed_at':    r[9].isoformat() if r[9] else None,
                'elapsed_ms':      r[10],
            })
        cur.close()
        return jsonify({'rows': rows, 'limit': limit})
    except Exception as e:
        return jsonify({'rows': [], 'error': str(e)}), 500


@app.route('/api/v1/kanban/summary', methods=['GET'])
def kanban_summary():
    """Per-column kanban task counts + a sample of recent task titles.

    Reads directly from the `unheaded.kanban_tasks` table (the kanban-app
    canonical store; same one cmd/kanban-app writes to). Avoids an HTTP
    round-trip to kanban-app's own /api endpoint and keeps this responsive
    even when kanban-app is restarting.
    """
    if pg_conn is None:
        return jsonify({'error': 'The Well not connected'}), 503
    try:
        # The kanban_tasks table lives in the 'unheaded' database, not
        # 'unheaded_app' that pg_conn is bound to. Use a separate connection.
        import psycopg2
        kconn = psycopg2.connect(
            dbname='unheaded',
            user=os.environ.get('ZHEN_DB_USER', 'unheaded'),
            password=os.environ.get('ZHEN_DB_PASSWORD', ''),
            host=os.environ.get('ZHEN_DB_HOST', 'localhost'),
            port=int(os.environ.get('ZHEN_DB_PORT', '5432')),
            connect_timeout=2,
        )
        kconn.autocommit = True
        cur = kconn.cursor()
        cur.execute("""
            SELECT status, COUNT(*)
              FROM kanban_tasks
             WHERE deleted_at IS NULL AND archived_at IS NULL
             GROUP BY status
             ORDER BY status
        """)
        counts = {row[0]: row[1] for row in cur.fetchall()}
        cur.execute("""
            SELECT id, title, status, owner, progress, updated_at
              FROM kanban_tasks
             WHERE deleted_at IS NULL AND archived_at IS NULL
             ORDER BY updated_at DESC NULLS LAST
             LIMIT 10
        """)
        recent = [
            {
                'id':          r[0],
                'title':       r[1],
                'status':      r[2],
                'owner':       r[3],
                'progress':    r[4],
                'updated_at':  r[5].isoformat() if r[5] else None,
            }
            for r in cur.fetchall()
        ]
        cur.close()
        kconn.close()
        return jsonify({
            'counts':          counts,
            'total':           sum(counts.values()),
            'recent_tasks':    recent,
            'kanban_url':      'http://localhost:20001',
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/runbooks/recent', methods=['GET'])
def runbooks_recent():
    """Recent runbook executions from zhen_actions audit.

    Filters action_type='runbook.execute' and returns last N. Used by the
    dashboard tab + sidebar's "Recent runbook outputs" surface.
    """
    limit = min(int(request.args.get('limit', 10)), 50)
    if pg_conn is None:
        return jsonify({'rows': [], 'error': 'The Well not connected'}), 503
    try:
        cur = pg_conn.cursor()
        cur.execute("""
            SELECT id, status, intent, parameters, result_summary,
                   error, planned_at, completed_at, elapsed_ms
              FROM zhen_actions
             WHERE action_type = 'runbook.execute'
             ORDER BY id DESC
             LIMIT %s
        """, (limit,))
        rows = []
        for r in cur.fetchall():
            rows.append({
                'id':              r[0],
                'status':          r[1],
                'intent':          r[2],
                'parameters':      r[3],
                'result_summary':  r[4],
                'error':           r[5],
                'planned_at':      r[6].isoformat() if r[6] else None,
                'completed_at':    r[7].isoformat() if r[7] else None,
                'elapsed_ms':      r[8],
            })
        cur.close()
        return jsonify({'rows': rows})
    except Exception as e:
        return jsonify({'rows': [], 'error': str(e)}), 500


# ──────────────────────────────────────────────────────────────────────
# end operator interaction surface
# ──────────────────────────────────────────────────────────────────────


@app.route('/api/v1/context', methods=['POST'])
def get_context():
    """Claude Code calls this to get relevant context before working on a task."""
    if not rag:
        return jsonify({'error': f'RAG not initialized: {startup_error}'}), 503

    data = request.json or {}
    task = data.get('task', '').strip()
    if not task:
        return jsonify({'error': 'task is required'}), 400

    k = min(data.get('k', 10), 50)

    try:
        results = rag.retrieve(task, k=k)
        max_dist = max((r['distance'] for r in results), default=1.0) or 1.0
        context = []
        for r in results:
            context.append({
                'source': r['source'],
                'type': r['type'],
                'content': r['content'],
                'relevance': round(1.0 - (r['distance'] / (max_dist * 1.5)), 4),
            })
        return jsonify({'task': task, 'k': k, 'context': context})
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/skills', methods=['GET'])
def list_skills():
    """List all Kingdom skills Zhen knows about."""
    skills_dir = Path.home() / 'tmp' / 'unheaded' / 'skills'
    skills = []

    for skill_file in sorted(skills_dir.glob('*.skill')):
        name = skill_file.stem
        description = ""
        triggers = []
        try:
            import zipfile
            with zipfile.ZipFile(skill_file, 'r') as zf:
                for zname in zf.namelist():
                    if zname.endswith('SKILL.md'):
                        text = zf.read(zname).decode('utf-8', errors='ignore')
                        if text.startswith('---'):
                            end = text.find('---', 3)
                            if end > 0:
                                front = text[3:end]
                                in_desc = False
                                desc_lines = []
                                for line in front.split('\n'):
                                    if line.strip().startswith('description:'):
                                        val = line.split(':', 1)[1].strip()
                                        if val and val != '|':
                                            desc_lines.append(val)
                                        in_desc = True
                                    elif in_desc and (line.startswith('  ') or line.startswith('\t')):
                                        desc_lines.append(line.strip())
                                    elif in_desc:
                                        in_desc = False
                                    if 'Triggers:' in line or 'triggers:' in line.lower():
                                        trig_text = line.split(':', 1)[1].strip() if ':' in line else ''
                                        triggers = [t.strip().rstrip('.') for t in trig_text.split(',') if t.strip()]
                                description = ' '.join(desc_lines).strip()
                        break
        except Exception:
            pass

        skills.append({
            'name': name,
            'description': description[:300] if description else f"Kingdom skill: {name}",
            'triggers': triggers[:20],
            'file': str(skill_file.name),
        })

    return jsonify({'skills': skills, 'total': len(skills)})


@app.route('/api/v1/skill/<name>', methods=['GET'])
def get_skill(name):
    """Return the full content of a specific skill."""
    skills_dir = Path.home() / 'tmp' / 'unheaded' / 'skills'

    skill_zip = skills_dir / f'{name}.skill'
    if skill_zip.exists():
        try:
            import zipfile
            with zipfile.ZipFile(skill_zip, 'r') as zf:
                for zname in zf.namelist():
                    if zname.endswith('SKILL.md'):
                        content = zf.read(zname).decode('utf-8', errors='ignore')
                        return jsonify({
                            'name': name,
                            'source': f'skills/{name}.skill',
                            'content': content,
                            'size': len(content),
                        })
        except Exception as e:
            return jsonify({'error': f'Failed to read skill zip: {e}'}), 500

    skill_dir = skills_dir / name
    skill_md = skill_dir / 'SKILL.md'
    if skill_md.exists():
        content = skill_md.read_text(encoding='utf-8', errors='ignore')
        return jsonify({
            'name': name,
            'source': f'skills/{name}/SKILL.md',
            'content': content,
            'size': len(content),
        })

    files_zip = skill_dir / 'files.zip'
    if files_zip.exists():
        try:
            import zipfile
            with zipfile.ZipFile(files_zip, 'r') as zf:
                for zname in zf.namelist():
                    if zname.endswith('SKILL.md'):
                        content = zf.read(zname).decode('utf-8', errors='ignore')
                        return jsonify({
                            'name': name,
                            'source': f'skills/{name}/files.zip:SKILL.md',
                            'content': content,
                            'size': len(content),
                        })
        except Exception as e:
            return jsonify({'error': f'Failed to read files.zip: {e}'}), 500

    return jsonify({'error': f'Skill not found: {name}'}), 404


@app.route('/api/v1/conversations', methods=['GET'])
def list_conversations():
    """List recent conversations from The Well."""
    global pg_conn
    if pg_conn is None:
        return jsonify({'error': 'The Well is not connected', 'conversations': []}), 200

    limit = min(int(request.args.get('limit', 50)), 200)
    try:
        cur = pg_conn.cursor()
        cur.execute(
            """SELECT id, session_id, role, content, sources, model,
                      tokens_input, tokens_output, elapsed_ms, created_at
               FROM zhen_conversations
               ORDER BY created_at DESC
               LIMIT %s""",
            (limit,),
        )
        rows = cur.fetchall()
        cur.close()
        conversations = []
        for r in rows:
            conversations.append({
                'id': r[0],
                'session_id': r[1],
                'role': r[2],
                'content': r[3],
                'sources': r[4] if r[4] else [],
                'model': r[5],
                'tokens_input': r[6],
                'tokens_output': r[7],
                'elapsed_ms': r[8],
                'created_at': r[9].isoformat() if r[9] else None,
            })
        return jsonify({'conversations': conversations, 'total': len(conversations)})
    except Exception as e:
        logging.warning(f"[zhen] Failed to list conversations: {e}")
        return jsonify({'error': str(e), 'conversations': []}), 500


@app.route('/api/v1/conversations/search', methods=['GET'])
def search_conversations():
    """Full-text search over conversations using PostgreSQL tsvector."""
    global pg_conn
    if pg_conn is None:
        return jsonify({'error': 'The Well is not connected', 'results': []}), 200

    q = request.args.get('q', '').strip()
    if not q:
        return jsonify({'error': 'q parameter required', 'results': []}), 400

    limit = min(int(request.args.get('limit', 20)), 100)
    try:
        cur = pg_conn.cursor()
        cur.execute(
            """SELECT id, session_id, role, content, sources, model,
                      tokens_input, tokens_output, elapsed_ms, created_at,
                      ts_rank(search_vector, websearch_to_tsquery('english', %s)) AS rank
               FROM zhen_conversations
               WHERE search_vector @@ websearch_to_tsquery('english', %s)
               ORDER BY rank DESC, created_at DESC
               LIMIT %s""",
            (q, q, limit),
        )
        rows = cur.fetchall()
        cur.close()
        results = []
        for r in rows:
            results.append({
                'id': r[0],
                'session_id': r[1],
                'role': r[2],
                'content': r[3],
                'sources': r[4] if r[4] else [],
                'model': r[5],
                'tokens_input': r[6],
                'tokens_output': r[7],
                'elapsed_ms': r[8],
                'created_at': r[9].isoformat() if r[9] else None,
                'rank': float(r[10]),
            })
        return jsonify({'query': q, 'results': results, 'total': len(results)})
    except Exception as e:
        logging.warning(f"[zhen] Failed to search conversations: {e}")
        return jsonify({'error': str(e), 'results': []}), 500


# ---------------------------------------------------------------------------
# New endpoints: Remember, Forget, Teach, Generate-File
# ---------------------------------------------------------------------------

@app.route('/api/v1/remember', methods=['POST'])
def remember():
    """Mark an answer as worth remembering for future queries."""
    global pg_conn
    if pg_conn is None:
        return jsonify({'error': 'The Well is not connected — cannot persist memories'}), 503

    data = request.json or {}
    question = data.get('question', '').strip()
    answer = data.get('answer', '').strip()
    model = data.get('model', '')

    if not question or not answer:
        return jsonify({'error': 'question and answer required'}), 400

    try:
        # Embed the question for future similarity matching
        embedding = rag.embedding_model.encode(question, convert_to_numpy=True).astype('float32')
        emb_bytes = embedding.tobytes()

        cur = pg_conn.cursor()
        cur.execute(
            """INSERT INTO zhen_memories (question, answer, embedding, source, model)
               VALUES (%s, %s, %s, 'user', %s)
               RETURNING id""",
            (question, answer, emb_bytes, model),
        )
        mem_id = cur.fetchone()[0]
        cur.close()

        return jsonify({'status': 'remembered', 'memory_id': mem_id})
    except Exception as e:
        logging.warning(f"[zhen] Failed to remember: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/forget', methods=['POST'])
def forget():
    """Remove a memory by ID."""
    global pg_conn
    if pg_conn is None:
        return jsonify({'error': 'The Well is not connected'}), 503

    data = request.json or {}
    memory_id = data.get('memory_id')
    if not memory_id:
        return jsonify({'error': 'memory_id required'}), 400

    try:
        cur = pg_conn.cursor()
        cur.execute("DELETE FROM zhen_memories WHERE id = %s", (memory_id,))
        cur.close()
        return jsonify({'status': 'forgotten', 'memory_id': memory_id})
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/teach', methods=['POST'])
def teach():
    """Submit text for Zhen to learn — DEPRECATED post-WAVE15 rewire.

    Pre-rewire this appended chunks to the live FAISS index + ring_all.jsonl.
    With vor as the retrieval substrate, "teaching" means writing a markdown
    file under ~/.config/cs/sources/<source>/ which cs picks up on its next
    index sweep — that's a different workflow than this endpoint exposes.

    Returning HTTP 410 (Gone) so callers know to migrate. A future change can
    wire this to ~/.config/cs/sources/zhen-taught/ and re-enable.
    """
    return jsonify({
        'status': 'deprecated',
        'reason': 'WAVE15 rewire: live-index teaching is no longer wired. Drop a markdown file under ~/.config/cs/sources/<your-source>/ and cs will index it.',
        'note':   'see docs/battle-plans/WAVE15-ZHENAI-REWIRE.md §Phase 1 step 1.5',
    }), 410


@app.route('/api/v1/generate-file', methods=['POST'])
def generate_file():
    """Generate a downloadable file from content.

    Request: {"filename": "hello.go", "content": "package main..."}
    Response: file download
    """
    data = request.json or {}
    filename = data.get('filename', 'output.txt').strip()
    content = data.get('content', '')

    if not content:
        return jsonify({'error': 'content required'}), 400

    # Sanitize filename (no path traversal)
    filename = Path(filename).name
    if not filename:
        filename = 'output.txt'

    buf = io.BytesIO(content.encode('utf-8'))
    buf.seek(0)

    return send_file(
        buf,
        as_attachment=True,
        download_name=filename,
        mimetype='text/plain',
    )


# ──────────────────────────────────────────────────────────────────────
# Champion API — Trust Level 1 (read-only operations, all audited)
# ADR-019: Zhen Champion Agent
# ──────────────────────────────────────────────────────────────────────

CHAMPION_ALLOWED = [
    os.path.expanduser('~/tmp/unheaded/'),
    '/var/zhen/',
]
CHAMPION_DENIED = ['.ssh', '.env', 'credentials', 'secrets', 'shadow', 'id_rsa', 'id_ed25519']

def champion_validate_path(path):
    """Validate path is within sandbox. Returns (abs_path, error)."""
    if '..' in path:
        return None, 'Path traversal blocked'
    abs_path = os.path.abspath(os.path.expanduser(path))
    for denied in CHAMPION_DENIED:
        if denied in abs_path:
            return None, f'Path contains denied pattern: {denied}'
    for allowed in CHAMPION_ALLOWED:
        if abs_path.startswith(os.path.abspath(allowed)):
            return abs_path, None
    return None, 'Path outside sandbox'


@app.route('/api/v1/champion/read', methods=['POST'])
def champion_read():
    """Read a file within the sandbox. Logged to action history."""
    data = request.get_json(force=True)
    path = data.get('path', '')
    if not path:
        return jsonify({'error': 'path required'}), 400

    abs_path, err = champion_validate_path(path)
    if err:
        return jsonify({'error': err, 'path': path}), 403

    try:
        with open(abs_path, 'r', errors='replace') as f:
            content = f.read(1_000_000)  # 1MB limit
        return jsonify({
            'path': abs_path,
            'content': content,
            'size': len(content),
        })
    except FileNotFoundError:
        return jsonify({'error': 'File not found', 'path': abs_path}), 404
    except Exception as e:
        return jsonify({'error': str(e)}), 500


@app.route('/api/v1/champion/health', methods=['GET'])
def champion_health():
    """Check health of all Kingdom services."""
    import urllib.request
    services = {
        'wotan': 18000, 'timeguru': 19000, 'captain': 19002,
        'architect': 19001, 'micromanager': 19003, 'monad': 19004,
        'sophia': 19005, 'dashboard': 20000, 'kanban': 16668,
        'zhen': 20103, 'inference': 20100,
    }
    results = []
    for name, port in services.items():
        try:
            req = urllib.request.Request(f'http://localhost:{port}/health', method='GET')
            with urllib.request.urlopen(req, timeout=2) as resp:
                results.append({'name': name, 'port': port, 'status': 'healthy', 'code': resp.status})
        except Exception:
            results.append({'name': name, 'port': port, 'status': 'unhealthy', 'code': 0})
    return jsonify({'services': results, 'healthy': sum(1 for r in results if r['status'] == 'healthy'), 'total': len(results)})


# ──────────────────────────────────────────────────────────────────────
# Runbook API — Standalone execution (no Claude Code needed)
# ADR-024: Zhen Runbook Automation
# ──────────────────────────────────────────────────────────────────────

RUNBOOKS_DIR = os.path.expanduser('~/tmp/unheaded/runbooks')

# Trust levels — controls what Zhenai can do autonomously vs what needs approval
# Mirrors real NOC/sysadmin permission model:
#   Level 1: Read — always allowed (health checks, file reads, corpus search)
#   Level 2: Write with snapshot — allowed for low-risk runbooks (dry-run always allowed)
#   Level 3: Destructive/sudo — requires human approval (high/critical risk runbooks)
TRUST_LEVEL = int(os.environ.get('ZHENAI_TRUST_LEVEL', '2'))

# Risk levels that require approval at each trust level
RISK_REQUIRES_APPROVAL = {
    1: ['low', 'medium', 'high', 'critical'],  # Level 1: everything needs approval except reads
    2: ['high', 'critical'],                      # Level 2: high/critical need approval
    3: ['critical'],                               # Level 3: only critical needs approval
}

@app.route('/api/v1/runbooks', methods=['GET'])
def list_runbooks():
    """List all available runbooks."""
    import glob as g
    runbooks = sorted(g.glob(os.path.join(RUNBOOKS_DIR, '**', '*.yaml'), recursive=True))
    result = []
    for rb in runbooks:
        rel = os.path.relpath(rb, RUNBOOKS_DIR)
        try:
            with open(rb) as f:
                data = yaml.safe_load(f) if 'yaml' in sys.modules else {}
            meta = data.get('metadata', {}) if data else {}
            result.append({
                'name': meta.get('name', os.path.splitext(os.path.basename(rb))[0]),
                'path': rel,
                'description': meta.get('description', ''),
                'risk': meta.get('risk', 'unknown'),
                'estimated_duration': meta.get('estimated_duration', '?'),
            })
        except Exception:
            result.append({'name': os.path.basename(rb), 'path': rel, 'description': 'parse error'})
    return jsonify({'runbooks': result, 'total': len(result)})


@app.route('/api/v1/runbooks/<name>', methods=['GET'])
def get_runbook(name):
    """Get runbook details by name."""
    import glob as g
    matches = g.glob(os.path.join(RUNBOOKS_DIR, '**', f'{name}.yaml'), recursive=True)
    if not matches:
        return jsonify({'error': f'Runbook not found: {name}'}), 404
    with open(matches[0]) as f:
        content = f.read()
    try:
        data = yaml.safe_load(content) if 'yaml' in sys.modules else {}
    except Exception:
        data = {}
    return jsonify({'name': name, 'path': matches[0], 'content': content, 'parsed': data})


@app.route('/api/v1/trust', methods=['GET'])
def get_trust_level():
    """Show current Zhenai trust level and what it controls."""
    return jsonify({
        'trust_level': TRUST_LEVEL,
        'description': {
            1: 'Read-only — all executions require approval',
            2: 'Write with snapshot — high/critical risk require approval',
            3: 'Autonomous — only critical risk requires approval',
        }.get(TRUST_LEVEL, 'Unknown'),
        'requires_approval': RISK_REQUIRES_APPROVAL.get(TRUST_LEVEL, []),
    })


@app.route('/api/v1/runbooks/<name>/execute', methods=['POST'])
def execute_runbook(name):
    """Execute a runbook through cmd/zhen-agentd's gate (WAVE15 Phase 2b).

    Pre-rewire this subprocessed scripts/run-runbook.py directly. Post-
    rewire the request flows: web UI → zhen-agentd /api/v1/tool/exec
    → pkg/champion.Dispatch (3 rules + audit) → Champion.RunbookExecute
    → subprocess. Closes T6 for the kingdom-mutation surface — every
    runbook execution now produces a row in zhen_actions and is gated
    by Champion's trust + destructive-verb rules.

    Pass {"dry_run": true} for validation only. The local trust-level
    check (ZHENAI_TRUST_LEVEL) is preserved as a courtesy gate BEFORE
    the daemon call so the user gets a clear "approval required"
    message without round-tripping. Champion's gate is the
    authoritative defense; this front-end check just shapes UX.

    Returns the daemon's tool-exec response shape:
        status:        "ok" | "denied" | "pending_confirmation" | "error"
        result:        runbook execution output (when status=ok)
        reason:        explanation (when not-ok)
        pending_token: redeem at /api/v1/agent/confirm (when pending)
    """
    import glob as g
    import urllib.error as _ue
    import urllib.request as _ur

    data = request.get_json(force=True) if request.is_json else {}
    dry_run = bool(data.get('dry_run', False))
    force = bool(data.get('force', False))

    matches = g.glob(os.path.join(RUNBOOKS_DIR, '**', f'{name}.yaml'), recursive=True)
    if not matches:
        return jsonify({'error': f'Runbook not found: {name}'}), 404

    # Front-end trust-level check (UX shaping; Champion is the real gate).
    if not dry_run and not force:
        try:
            with open(matches[0]) as f:
                rb_data = yaml.safe_load(f)
            risk = rb_data.get('metadata', {}).get('risk', 'medium')
            needs_approval = RISK_REQUIRES_APPROVAL.get(TRUST_LEVEL, [])
            if risk in needs_approval:
                return jsonify({
                    'error': f'Runbook "{name}" has risk level "{risk}" which requires approval at trust level {TRUST_LEVEL}',
                    'action': 'Set force=true to execute with explicit approval, or increase ZHENAI_TRUST_LEVEL',
                    'trust_level': TRUST_LEVEL,
                    'risk': risk,
                }), 403
        except Exception:
            pass  # parse failure is non-blocking — let the daemon's gate decide

    # Compute the runbook name argument the way Champion.RunbookExecute
    # expects it: "<category>/<runbook>" relative to the runbooks/ dir,
    # without the .yaml extension. e.g. "observe/service-health-sweep".
    rb_path = matches[0]
    runbooks_root = os.path.realpath(RUNBOOKS_DIR)
    rb_real = os.path.realpath(rb_path)
    if not rb_real.startswith(runbooks_root + os.sep):
        return jsonify({'error': 'Runbook path escapes runbooks/ tree'}), 400
    rel = os.path.relpath(rb_real, runbooks_root)
    rel = rel.removesuffix('.yaml')

    # Hit the daemon's /api/v1/tool/exec endpoint. Default justification
    # is direct-user (the operator clicked this in the browser); Champion's
    # Rule 2 (untrusted-justification) is escaped by the direct-user trust
    # label. Rules 1 (path) + 3 (destructive verb) still apply — Champion
    # rejects runbook names with ".." or absolute paths regardless.
    agentd_url = os.environ.get('ZHEN_AGENTD_URL', 'http://localhost:20105')
    payload = {
        'tool': 'runbook_execute',
        'args': {
            'name':    rel,
            'dry_run': dry_run,
        },
        'session_id': f'runbook-{name}-{int(time.time())}',
        'emitted_by': 'zhen-web-ui',
        'justification': [{
            'topic':        'user-clicked-runbook-execute',
            'category':     'user-action',
            'source_kind':  'user-action',
            'source_trust': 'direct-user',
            'source_label': '/api/v1/runbooks/<name>/execute',
        }],
    }
    body = json.dumps(payload).encode('utf-8')
    req = _ur.Request(
        f'{agentd_url}/api/v1/tool/exec',
        data=body,
        headers={'Content-Type': 'application/json'},
        method='POST',
    )

    # Match the daemon's RunbookExecute 10-min subprocess cap, plus a
    # small buffer for round-trip + agent-loop overhead.
    try:
        with _ur.urlopen(req, timeout=620) as resp:
            status_code = resp.status
            daemon_body = json.loads(resp.read().decode('utf-8'))
    except _ue.HTTPError as e:
        # Daemon returned a non-2xx: read body and surface (gate refusals
        # come back as 403; runbook subprocess failure as 500).
        try:
            daemon_body = json.loads(e.read().decode('utf-8'))
        except Exception:
            daemon_body = {'status': 'error', 'reason': str(e)}
        status_code = e.code
    except _ue.URLError as e:
        return jsonify({
            'name':   name,
            'status': 'error',
            'error':  f'zhen-agentd unreachable at {agentd_url}: {e}',
            'note':   'WAVE15 Phase 2b requires the daemon. Start it with `bin/zhen-agentd -port 20105`.',
        }), 503
    except Exception as e:
        return jsonify({'name': name, 'status': 'error', 'error': str(e)}), 500

    # Surface the daemon's response shape, augmented with our metadata.
    daemon_status = daemon_body.get('status', 'unknown')
    out = {
        'name':    name,
        'dry_run': dry_run,
        'status':  daemon_status,
        'gated_via': 'cmd/zhen-agentd /api/v1/tool/exec (Champion gate)',
    }
    if 'result' in daemon_body:
        out['result'] = daemon_body['result']
    if 'reason' in daemon_body:
        out['reason'] = daemon_body['reason']
    if daemon_body.get('pending_token'):
        out['pending_token'] = daemon_body['pending_token']
        out['confirm_endpoint'] = f'{agentd_url}/api/v1/agent/confirm'
    return jsonify(out), status_code


# ──────────────────────────────────────────────────────────────────────
# Approval Queue — pending high-risk operations awaiting human approval
# ──────────────────────────────────────────────────────────────────────

_approval_queue = []  # In-memory for now; The Well integration later
_approval_counter = 0

@app.route('/api/v1/approvals', methods=['GET'])
def list_approvals():
    """List pending approval requests."""
    pending = [a for a in _approval_queue if a['status'] == 'pending']
    return jsonify({'approvals': pending, 'total': len(pending)})


@app.route('/api/v1/approvals', methods=['POST'])
def request_approval():
    """Submit an operation for approval."""
    global _approval_counter
    data = request.get_json(force=True)
    _approval_counter += 1
    approval = {
        'id': _approval_counter,
        'operation': data.get('operation', ''),
        'runbook': data.get('runbook', ''),
        'risk': data.get('risk', 'high'),
        'reason': data.get('reason', ''),
        'status': 'pending',
        'requested_at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        'decided_at': None,
        'decided_by': None,
    }
    _approval_queue.append(approval)
    return jsonify(approval), 201


@app.route('/api/v1/approvals/<int:approval_id>/approve', methods=['POST'])
def approve_request(approval_id):
    """Approve a pending request (human operator action)."""
    for a in _approval_queue:
        if a['id'] == approval_id and a['status'] == 'pending':
            a['status'] = 'approved'
            a['decided_at'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
            a['decided_by'] = 'human'
            return jsonify(a)
    return jsonify({'error': 'Approval not found or already decided'}), 404


@app.route('/api/v1/approvals/<int:approval_id>/deny', methods=['POST'])
def deny_request(approval_id):
    """Deny a pending request."""
    for a in _approval_queue:
        if a['id'] == approval_id and a['status'] == 'pending':
            a['status'] = 'denied'
            a['decided_at'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
            a['decided_by'] = 'human'
            return jsonify(a)
    return jsonify({'error': 'Approval not found or already decided'}), 404


# ──────────────────────────────────────────────────────────────────────
# Source Viewer — view full source content in a new tab
# ──────────────────────────────────────────────────────────────────────

@app.route('/api/v1/source', methods=['GET'])
def view_source():
    """Retrieve full content of a vor topic by ID or source path.

    Pre-rewire this iterated rag.corpus (the in-memory corpus dict).
    Post-rewire vor is the substrate; we look up the topic via vor's
    /api/topics/<id> endpoint. Source-path lookups use the search API
    and filter by the returned source_path field.
    """
    source_id = request.args.get('id', '').strip()
    source_path = request.args.get('path', '').strip()

    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503
    if not (source_id or source_path):
        return jsonify({'error': 'id or path required'}), 400

    import urllib.parse as _up
    import urllib.request as _ur

    # Direct topic lookup by ID is the fast path.
    if source_id:
        try:
            url = f'{rag.vor_url}/api/topics/{_up.quote(source_id, safe="")}'
            with _ur.urlopen(url, timeout=5) as resp:
                t = json.loads(resp.read().decode())
            return jsonify({
                'id':      source_id,
                'source':  t.get('source_path', ''),
                'type':    t.get('source_kind', ''),
                'trust':   t.get('source_trust', 'canonical'),
                'label':   t.get('source_label', ''),
                'content': t.get('content', ''),
                'total_matches': 1,
            })
        except Exception as exc:
            return jsonify({'error': f'vor topic lookup failed: {exc}'}), 404

    # Path-based lookup: search vor and filter by source_path.
    try:
        url = f'{rag.vor_url}/api/search?{_up.urlencode({"q": source_path})}'
        with _ur.urlopen(url, timeout=5) as resp:
            hits = json.loads(resp.read().decode())
        for h in hits:
            if h.get('source_path') == source_path:
                # Fetch full content for the matching topic.
                topic = h.get('topic', '')
                if not topic:
                    continue
                turl = f'{rag.vor_url}/api/topics/{_up.quote(topic, safe="")}'
                with _ur.urlopen(turl, timeout=5) as tr:
                    t = json.loads(tr.read().decode())
                return jsonify({
                    'id':      topic,
                    'source':  source_path,
                    'type':    t.get('source_kind', ''),
                    'trust':   t.get('source_trust', 'canonical'),
                    'label':   t.get('source_label', ''),
                    'content': t.get('content', ''),
                    'total_matches': 1,
                })
        return jsonify({'error': f'Source not found: {source_path}'}), 404
    except Exception as exc:
        return jsonify({'error': f'vor search failed: {exc}'}), 500


@app.route('/view/source')
def source_viewer_page():
    """HTML page that displays a source document. Opened in a new tab."""
    source_path = request.args.get('path', '')
    source_id = request.args.get('id', '')
    return f'''<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Source: {source_path or source_id} — Zhenai</title>
    <style>
        body {{ background: #0a0a0a; color: #c9c9c9; font-family: 'JetBrains Mono', monospace; padding: 20px; max-width: 900px; margin: 0 auto; }}
        h1 {{ color: #ff5c00; font-size: 1.2em; border-bottom: 1px solid #333; padding-bottom: 8px; }}
        .meta {{ color: #666; font-size: 0.8em; margin-bottom: 16px; }}
        pre {{ background: #111; padding: 16px; border-radius: 4px; overflow-x: auto; white-space: pre-wrap; word-wrap: break-word; line-height: 1.5; }}
        .error {{ color: #ff4444; }}
        a {{ color: #ff5c00; }}
    </style>
</head>
<body>
    <h1 id="title">Loading...</h1>
    <div class="meta" id="meta"></div>
    <pre id="content">Loading source content...</pre>
    <script>
        const params = new URLSearchParams(window.location.search);
        const id = params.get('id') || '';
        const path = params.get('path') || '';
        fetch('/api/v1/source?' + (id ? 'id=' + encodeURIComponent(id) : 'path=' + encodeURIComponent(path)))
            .then(r => r.json())
            .then(d => {{
                if (d.error) {{
                    document.getElementById('title').textContent = 'Source Not Found';
                    document.getElementById('content').className = 'error';
                    document.getElementById('content').textContent = d.error;
                    return;
                }}
                document.getElementById('title').textContent = d.source || d.id;
                document.getElementById('meta').textContent = 'Type: ' + (d.type || 'unknown') + ' | ID: ' + (d.id || '?');
                document.title = 'Source: ' + (d.source || d.id) + ' — Zhenai';
                document.getElementById('content').textContent = d.content;
            }})
            .catch(e => {{
                document.getElementById('content').className = 'error';
                document.getElementById('content').textContent = 'Error loading source: ' + e;
            }});
    </script>
</body>
</html>'''


@app.route('/')
def index():
    return app.send_static_file('index.html')


if __name__ == '__main__':
    print("Zhen Web UI starting on port 20103...")
    print(f"RAG status: {'ready' if rag else 'NOT READY — ' + str(startup_error)}")
    print(f"Local context window: {rag.local_max_tokens if rag else 'N/A'} tokens")
    app.run(host='0.0.0.0', port=20103, debug=False)
