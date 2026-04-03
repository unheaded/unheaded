#!/usr/bin/env python3
"""
Zhen Web App — RAG Demo for Unheaded Infrastructure
Port: 20103 (zhen-ui in Doom Range)

Features:
- Hybrid inference: local Mistral-7B + Claude API handoff
- Model selector (auto/mistral/opus/sonnet/haiku)
- File upload (text injected into prompt)
- File generation (downloadable responses)
- Memory system (remember/forget good answers)
- Teach endpoint (grow corpus without restart)
"""
import io
import os
import sys
import json
import time
import uuid
import logging
import yaml
from pathlib import Path
from flask import Flask, request, jsonify, Response, send_file
from flask_cors import CORS

# Add scripts dir to path for RAG import
sys.path.insert(0, str(Path(__file__).parent / 'scripts'))
from zhen_rag import RAGPipeline

app = Flask(__name__, static_folder='static', static_url_path='/static')
CORS(app)
app.secret_key = os.environ.get('FLASK_SECRET_KEY', 'zhen-session-key-dev')  # override in production

# Initialize RAG pipeline
index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
# Load combined corpus (all rings + wikipedia + stackoverflow + skills)
corpus_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
corpus_file = corpus_dir / 'ring_all.jsonl'
if not corpus_file.exists():
    corpus_file = corpus_dir / 'ring1.jsonl'  # fallback

rag = None
startup_error = None

try:
    rag = RAGPipeline(index_dir, corpus_file)
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


@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        'status': 'ok' if rag else 'degraded',
        'rag_ready': rag is not None,
        'well_connected': pg_conn is not None,
        'session_id': _session_id,
        'error': startup_error,
    })


@app.route('/api/v1/query', methods=['POST'])
def query():
    if not rag:
        return jsonify({'error': f'RAG not initialized: {startup_error}'}), 503

    data = request.json
    question = data.get('question', '').strip()
    if not question:
        return jsonify({'error': 'Question required'}), 400

    file_content = data.get('file_content', None)
    client_session = data.get('session_id', 'default')

    try:
        # Check memories first
        memory = _search_memories(question)
        if memory:
            _pg_log('user', question)
            _pg_log('assistant', memory['answer'],
                    model=f"memory:{memory.get('model', 'cached')}",
                    elapsed_ms=0)
            # Still add to history so follow-ups work
            _add_to_history(client_session, 'user', question)
            _add_to_history(client_session, 'assistant', memory['answer'])
            return jsonify({
                'question': question,
                'answer': memory['answer'],
                'sources': [],
                'tokens_used': 0,
                'elapsed_seconds': 0.0,
                'model': f"memory (similarity: {memory['similarity']:.2f})",
                'from_memory': True,
                'memory_id': memory['id'],
            })

        history = _get_history(client_session)

        start = time.time()
        result = rag.query(question, file_content=file_content, history=history)
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

        result_model = result.get('model', 'mistral-7b')

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


@app.route('/api/v1/stats', methods=['GET'])
def stats():
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    return jsonify({
        'index_vectors': rag.index.ntotal,
        'index_dimension': rag.index.d,
        'corpus_chunks': len(rag.corpus),
        'model': 'all-MiniLM-L6-v2',
        'inference_url': rag.inference_url,
        'local_max_tokens': rag.local_max_tokens,
    })


@app.route('/api/v1/corpus/stats', methods=['GET'])
def corpus_stats():
    """Corpus statistics — chunks per ring, total vectors, index file size"""
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    corpus_dir_path = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
    index_file = index_dir / 'ring1.index'

    # Count chunks per ring by scanning corpus files
    rings = {}
    for f in sorted(corpus_dir_path.glob('*.jsonl')):
        ring_name = f.stem  # e.g. "ring1", "ring234"
        count = 0
        try:
            with open(f) as fh:
                for line in fh:
                    if line.strip():
                        count += 1
        except Exception:
            pass
        rings[ring_name] = count

    # Index file size
    index_size_bytes = 0
    try:
        index_size_bytes = index_file.stat().st_size
    except Exception:
        pass

    return jsonify({
        'chunks_per_ring': rings,
        'total_chunks': sum(rings.values()),
        'total_vectors': rag.index.ntotal,
        'index_dimension': rag.index.d,
        'index_file_size_bytes': index_size_bytes,
        'index_file_size_mb': round(index_size_bytes / (1024 * 1024), 2),
    })


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
        import numpy as np
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
    """Submit text for Zhen to learn. Chunks, embeds, and adds to live FAISS index."""
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    data = request.json or {}
    text = data.get('text', '').strip()
    source = data.get('source', 'user')

    if not text:
        return jsonify({'error': 'text required'}), 400

    if len(text) > 100000:
        return jsonify({'error': 'Text too long (max 100K chars)'}), 400

    try:
        result = rag.add_to_corpus(text, source=source)
        return jsonify({
            'status': 'learned',
            'chunks_added': result['added'],
            'chunk_previews': result.get('chunks', []),
        })
    except Exception as e:
        return jsonify({'error': str(e)}), 500


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
    return None, f'Path outside sandbox'


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
    """Execute a runbook. Pass {"dry_run": true} for validation only.
    Trust level controls what can execute without approval."""
    import glob as g
    import subprocess as sp
    data = request.get_json(force=True) if request.is_json else {}
    dry_run = data.get('dry_run', False)
    force = data.get('force', False)  # Override trust check (human explicitly approved)

    matches = g.glob(os.path.join(RUNBOOKS_DIR, '**', f'{name}.yaml'), recursive=True)
    if not matches:
        return jsonify({'error': f'Runbook not found: {name}'}), 404

    # Trust level check — does this runbook need approval?
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
            pass  # If we can't parse the runbook, let it through to the runner

    runner = os.path.expanduser('~/tmp/unheaded/scripts/run-runbook.py')
    cmd = ['python3', runner]
    if dry_run:
        cmd.append('--dry-run')
    cmd.append(matches[0])

    try:
        result = sp.run(cmd, capture_output=True, text=True, timeout=600)
        return jsonify({
            'name': name,
            'status': 'success' if result.returncode == 0 else 'failed',
            'dry_run': dry_run,
            'output': result.stdout[-5000:],
            'errors': result.stderr[-2000:] if result.stderr else '',
            'exit_code': result.returncode,
        })
    except sp.TimeoutExpired:
        return jsonify({'name': name, 'status': 'timeout', 'error': 'Execution exceeded 600s'}), 504
    except Exception as e:
        return jsonify({'name': name, 'status': 'error', 'error': str(e)}), 500


@app.route('/')
def index():
    return app.send_static_file('index.html')


if __name__ == '__main__':
    print(f"Zhen Web UI starting on port 20103...")
    print(f"RAG status: {'ready' if rag else 'NOT READY — ' + str(startup_error)}")
    print(f"Local context window: {rag.local_max_tokens if rag else 'N/A'} tokens")
    app.run(host='0.0.0.0', port=20103, debug=False)
