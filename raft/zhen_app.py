#!/usr/bin/env python3
"""
Zhen Web App — RAG Demo for Unheaded Infrastructure
Port: 20103 (zhen-ui in Doom Range)
"""
import sys
import json
import time
from pathlib import Path
from flask import Flask, request, jsonify, Response
from flask_cors import CORS

# Add scripts dir to path for RAG import
sys.path.insert(0, str(Path(__file__).parent / 'scripts'))
from zhen_rag import RAGPipeline

app = Flask(__name__, static_folder='static', static_url_path='/static')
CORS(app)

# Initialize RAG pipeline
index_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'index'
corpus_file = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus' / 'ring1.jsonl'

rag = None
startup_error = None

try:
    rag = RAGPipeline(index_dir, corpus_file)
except Exception as e:
    startup_error = str(e)
    print(f"WARNING: RAG not ready: {e}")


@app.route('/health', methods=['GET'])
def health():
    return jsonify({
        'status': 'ok' if rag else 'degraded',
        'rag_ready': rag is not None,
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

    try:
        start = time.time()
        result = rag.query(question)
        elapsed = time.time() - start

        return jsonify({
            'question': result['question'],
            'answer': result['answer'],
            'sources': [
                {
                    'id': c['id'],
                    'source': c['source'],
                    'type': c['type'],
                    'preview': c['content'][:200],
                    'distance': c['distance'],
                }
                for c in result['retrieved'][:5]
            ],
            'tokens_used': result['tokens_used'],
            'elapsed_seconds': round(elapsed, 2),
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
    })


@app.route('/api/v1/corpus/stats', methods=['GET'])
def corpus_stats():
    """Corpus statistics — chunks per ring, total vectors, index file size"""
    if not rag:
        return jsonify({'error': 'RAG not initialized'}), 503

    corpus_dir = Path.home() / 'tmp' / 'unheaded' / 'raft' / 'corpus'
    index_file = index_dir / 'ring1.index'

    # Count chunks per ring by scanning corpus files
    rings = {}
    for f in sorted(corpus_dir.glob('*.jsonl')):
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


@app.route('/')
def index():
    return app.send_static_file('index.html')


HTML_UI = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Zhen — Unheaded AI Champion</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;700&display=swap" rel="stylesheet">
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'JetBrains Mono', monospace;
            background: linear-gradient(135deg, #0a0a14 0%, #0f0f1e 50%, #1a1a2e 100%);
            color: #c8c8d4;
            min-height: 100vh;
            display: flex;
            flex-direction: column;
        }
        header {
            background: rgba(0,0,0,0.6);
            padding: 18px 24px;
            border-bottom: 2px solid #ff5c00;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        .logo { display: flex; align-items: center; gap: 12px; }
        .logo h1 { color: #ff5c00; font-size: 1.8em; letter-spacing: 2px; }
        .logo .subtitle { color: #666; font-size: 0.75em; }
        .status { font-size: 0.7em; color: #4a4; }
        .status.offline { color: #a44; }
        main {
            flex: 1;
            display: flex;
            flex-direction: column;
            padding: 16px;
            max-width: 960px;
            margin: 0 auto;
            width: 100%;
        }
        .chat {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: rgba(0,0,0,0.35);
            border: 1px solid #333;
            border-radius: 8px;
            overflow: hidden;
        }
        .messages {
            flex: 1;
            overflow-y: auto;
            padding: 16px;
            display: flex;
            flex-direction: column;
            gap: 12px;
        }
        .msg {
            padding: 12px 16px;
            border-radius: 6px;
            max-width: 85%;
            line-height: 1.5;
            font-size: 0.85em;
            white-space: pre-wrap;
            word-wrap: break-word;
        }
        .msg.user {
            background: rgba(255,92,0,0.12);
            border-left: 3px solid #ff5c00;
            align-self: flex-end;
            text-align: right;
        }
        .msg.zhen {
            background: rgba(0,140,255,0.08);
            border-left: 3px solid #0096ff;
            align-self: flex-start;
        }
        .msg .meta {
            font-size: 0.7em;
            color: #555;
            margin-top: 8px;
        }
        .msg .sources {
            font-size: 0.7em;
            color: #666;
            margin-top: 6px;
            border-top: 1px solid #222;
            padding-top: 6px;
        }
        .msg .sources span { color: #ff5c00; }
        .input-bar {
            display: flex;
            gap: 8px;
            padding: 12px 16px;
            border-top: 1px solid #333;
            background: rgba(0,0,0,0.3);
        }
        input[type=text] {
            flex: 1;
            padding: 10px 14px;
            background: rgba(0,0,0,0.5);
            border: 1px solid #444;
            border-radius: 5px;
            color: #e0e0e0;
            font-family: inherit;
            font-size: 0.85em;
        }
        input:focus { outline: none; border-color: #ff5c00; }
        button {
            padding: 10px 24px;
            background: #ff5c00;
            color: #000;
            border: none;
            border-radius: 5px;
            font-weight: 700;
            font-family: inherit;
            cursor: pointer;
            transition: background 0.2s;
        }
        button:hover { background: #ff7733; }
        button:disabled { background: #444; color: #888; cursor: wait; }
        .typing { color: #666; font-style: italic; }
        .suggestions {
            display: flex;
            flex-wrap: wrap;
            gap: 6px;
            padding: 8px 16px;
            border-top: 1px solid #222;
            background: rgba(0,0,0,0.2);
        }
        .suggestions button {
            padding: 4px 10px;
            font-size: 0.7em;
            background: rgba(255,92,0,0.15);
            color: #ff5c00;
            border: 1px solid #ff5c0044;
        }
        .suggestions button:hover { background: rgba(255,92,0,0.3); }
    </style>
</head>
<body>
    <header>
        <div class="logo">
            <h1>ZHEN</h1>
            <div>
                <div class="subtitle">Unheaded AI Champion</div>
                <div class="status" id="status">Connecting...</div>
            </div>
        </div>
    </header>
    <main>
        <div class="chat">
            <div class="messages" id="messages"></div>
            <div class="suggestions" id="suggestions">
                <button onclick="ask('What is Unheaded?')">What is Unheaded?</button>
                <button onclick="ask('What are the core services?')">Core services</button>
                <button onclick="ask('How does eBPF tracing work?')">eBPF tracing</button>
                <button onclick="ask('What is the Wotan message bus?')">Wotan</button>
                <button onclick="ask('How does the Monad wire format work?')">Monad protocol</button>
                <button onclick="ask('What is the Meta Moment?')">Meta Moment</button>
            </div>
            <div class="input-bar">
                <input type="text" id="q" placeholder="Ask Zhen about Unheaded..." autocomplete="off" />
                <button id="btn" onclick="ask()">Ask</button>
            </div>
        </div>
    </main>
<script>
const msgs = document.getElementById('messages');
const qInput = document.getElementById('q');
const btn = document.getElementById('btn');
const statusEl = document.getElementById('status');

function addMsg(text, cls, meta) {
    const d = document.createElement('div');
    d.className = 'msg ' + cls;
    d.textContent = text;
    if (meta) {
        const m = document.createElement('div');
        m.className = 'meta';
        m.innerHTML = meta;
        d.appendChild(m);
    }
    msgs.appendChild(d);
    msgs.scrollTop = msgs.scrollHeight;
    return d;
}

async function ask(preset) {
    const question = preset || qInput.value.trim();
    if (!question) return;
    qInput.value = '';
    btn.disabled = true;

    addMsg(question, 'user');
    const thinking = addMsg('Thinking...', 'zhen typing');

    try {
        const r = await fetch('/api/v1/query', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({question}),
        });
        const data = await r.json();
        msgs.removeChild(thinking);

        if (data.error) {
            addMsg('Error: ' + data.error, 'zhen');
        } else {
            let sourcesHtml = data.sources.map(s =>
                '<span>' + s.source + '</span>'
            ).join(', ');
            addMsg(data.answer, 'zhen',
                `${data.elapsed_seconds}s | ${data.tokens_used} tokens<div class="sources">Sources: ${sourcesHtml}</div>`
            );
        }
    } catch(e) {
        msgs.removeChild(thinking);
        addMsg('Connection error: ' + e.message, 'zhen');
    }
    btn.disabled = false;
    qInput.focus();
}

qInput.addEventListener('keydown', e => { if (e.key === 'Enter') ask(); });

// Health check
fetch('/health').then(r => r.json()).then(d => {
    if (d.rag_ready) {
        statusEl.textContent = 'Online';
        statusEl.className = 'status';
    } else {
        statusEl.textContent = 'RAG: ' + (d.error || 'not ready');
        statusEl.className = 'status offline';
    }
}).catch(() => {
    statusEl.textContent = 'Offline';
    statusEl.className = 'status offline';
});

// Welcome
addMsg("I am Zhen, champion of the Unheaded Kingdom.\\n\\nI know every line of the 385K-LOC codebase — architecture, protocols, services, eBPF traces, and Kingdom lore.\\n\\nAsk me anything.", 'zhen');
</script>
</body>
</html>'''


if __name__ == '__main__':
    print(f"Zhen Web UI starting on port 20103...")
    print(f"RAG status: {'ready' if rag else 'NOT READY — ' + str(startup_error)}")
    app.run(host='0.0.0.0', port=20103, debug=False)
