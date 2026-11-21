// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// The chat page (/, pages/chat.html). The page's Content-Security-Policy
// allows no inline script, style or handler, so every handler is attached
// here (see "Handlers" at the end) and markup built here carries classes
// from static/chat.css, never a style or on* attribute. Text that comes from
// a model, a sheet or the server goes in through textContent or escapeHtml.

const API = '';  // Same origin

// CSRF interim (threat model T8): echo the zhen_csrf cookie (SameSite=Strict,
// set by GET /) in X-Zhen-CSRF on every same-origin state-changing request.
// Never sent to another origin.
(function () {
    const realFetch = window.fetch.bind(window);
    window.fetch = function (input, init) {
        init = init || {};
        const method = (init.method || 'GET').toUpperCase();
        const url = new URL(typeof input === 'string' ? input : input.url, location.href);
        if (method !== 'GET' && method !== 'HEAD' && url.origin === location.origin) {
            const m = document.cookie.match(/(?:^|; )zhen_csrf=([^;]+)/);
            if (m) {
                const headers = new Headers(init.headers || {});
                headers.set('X-Zhen-CSRF', decodeURIComponent(m[1]));
                init = Object.assign({}, init, { headers: headers });
            }
        }
        return realFetch(input, init).then(function (resp) {
            if (resp.status === 401 && url.origin === location.origin && !url.pathname.startsWith('/auth/')) {
                showLogin();
            }
            return resp;
        });
    };
})();

// Login (F4, H5): the issuer's, through this UI. The browser holds a session
// cookie it cannot read, and never a token.
// True when this page load found a live session: the page is then drawn for
// a person, and a later login (the session ended) keeps it as it is.
let loadedLoggedIn = false;

function showLogin() {
    document.getElementById('login-panel').hidden = false;
    document.getElementById('login-user').focus();
}

function showWho(me) {
    document.getElementById('login-panel').hidden = true;
    document.getElementById('login-who-name').textContent = me.sub;
    document.getElementById('login-who').hidden = false;
}

document.getElementById('login-form').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    const errEl = document.getElementById('login-error');
    const pw = document.getElementById('login-password');
    const btn = document.getElementById('login-submit');
    errEl.textContent = '';
    btn.disabled = true;
    let r;
    try {
        r = await fetch(API + '/auth/login', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({username: document.getElementById('login-user').value, password: pw.value}),
        });
    } catch (_e) {
        pw.value = '';
        btn.disabled = false;
        errEl.textContent = 'login is unreachable (network error)';
        return;
    }
    pw.value = '';
    btn.disabled = false;
    const body = await r.json().catch(() => ({}));
    if (r.ok) {
        if (body.password_below_floor) {
            alert('Your password is shorter than the 15 characters now asked for. It still works: please have it replaced.');
        }
        if (!loadedLoggedIn) {
            // The page was drawn for a stranger: draw it again for this person.
            location.reload();
            return;
        }
        // The session ended while the page was in use (a login lasts 15
        // minutes). Keep the page and what is typed in it, and fetch again
        // what needed a login.
        fetch(API + '/auth/me').then(m => m.ok ? m.json().then(showWho) : showLogin()).catch(showLogin);
        refreshStats();
        return;
    }
    const wait = r.headers.get('Retry-After');
    errEl.textContent = (body.error || 'login failed') + (wait ? ` (try again in ${wait} s)` : '');
});

document.getElementById('login-logout').addEventListener('click', async () => {
    await fetch(API + '/auth/logout', {method: 'POST'});
    location.reload();
});

fetch(API + '/auth/me').then(r => r.ok ? r.json().then(me => { loadedLoggedIn = true; showWho(me); }) : showLogin()).catch(showLogin);

// A held call comes back with a token bound to the person logged in. It is
// spent here, on this origin, as that person.
async function redeemPending(token) {
    const r = await fetch(API + '/api/v1/agent/confirm', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({token: token}),
    });
    return {ok: r.ok, status: r.status, body: await r.json().catch(() => ({}))};
}

// offerConfirm puts a Confirm button in el: nothing runs until it is pressed.
function offerConfirm(el, token, label, done) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'action-btn';
    b.textContent = 'Confirm: ' + label;
    b.addEventListener('click', async () => {
        b.disabled = true;
        b.textContent = 'running: ' + label;
        const out = await redeemPending(token);
        b.textContent = (out.ok ? 'done: ' : 'refused: ') + label;
        done(out);
    });
    el.appendChild(b);
}
const msgs = document.getElementById('messages');
const qInput = document.getElementById('q');
const btn = document.getElementById('btn');
const historyEl = document.getElementById('history');
const searchResultsEl = document.getElementById('search-results');

let queryCount = 0;
let totalTime = 0;
const queryHistory = [];
let currentTab = 'chat';
let uploadedFile = null;  // {name, content}
const sessionId = (crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(36).slice(2) + Date.now().toString(36));

// --- File upload ---
document.getElementById('file-input').addEventListener('change', function(e) {
    const file = e.target.files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = function() {
        uploadedFile = { name: file.name, content: reader.result };
        document.getElementById('file-name').textContent = file.name;
        document.getElementById('file-size').textContent = `(${(file.size / 1024).toFixed(1)} KB)`;
        document.getElementById('file-upload-area').classList.add('active');
    };
    reader.readAsText(file);
    e.target.value = '';  // Reset so same file can be re-uploaded
});

function removeFile() {
    uploadedFile = null;
    document.getElementById('file-upload-area').classList.remove('active');
}

// --- Drag and drop ---
const dropOverlay = document.getElementById('drop-overlay');
let dragCounter = 0;

document.addEventListener('dragenter', function(e) {
    e.preventDefault();
    dragCounter++;
    dropOverlay.classList.add('active');
});
document.addEventListener('dragleave', function(e) {
    e.preventDefault();
    dragCounter--;
    if (dragCounter <= 0) {
        dragCounter = 0;
        dropOverlay.classList.remove('active');
    }
});
document.addEventListener('dragover', function(e) { e.preventDefault(); });
document.addEventListener('drop', function(e) {
    e.preventDefault();
    dragCounter = 0;
    dropOverlay.classList.remove('active');
    const file = e.dataTransfer.files[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = function() {
        uploadedFile = { name: file.name, content: reader.result };
        document.getElementById('file-name').textContent = file.name;
        document.getElementById('file-size').textContent = `(${(file.size / 1024).toFixed(1)} KB)`;
        document.getElementById('file-upload-area').classList.add('active');
    };
    reader.readAsText(file);
});

// --- Tab switching ---
function switchTab(tab) {
    currentTab = tab;
    document.getElementById('tab-chat').className = tab === 'chat' ? 'active' : '';
    document.getElementById('tab-dashboard').className = tab === 'dashboard' ? 'active' : '';
    document.getElementById('tab-search').className = tab === 'search' ? 'active' : '';
    document.getElementById('tab-history').className = tab === 'history' ? 'active' : '';
    for (const view of ['chat', 'dashboard', 'search', 'history']) {
        document.getElementById(view + '-view').hidden = view !== tab;
    }
    if (tab === 'dashboard') refreshDashboard();

    if (tab === 'chat') {
        qInput.placeholder = 'Ask Zhen about Unheaded — drag a file in, or click + to attach';
        btn.textContent = 'Ask';
    } else if (tab === 'search') {
        qInput.placeholder = 'Search the codebase...';
        btn.textContent = 'Search';
    } else if (tab === 'history') {
        qInput.placeholder = 'Search conversations...';
        btn.textContent = 'Search';
        loadHistory();
    }
    qInput.focus();
}

function handleSubmit() {
    if (currentTab === 'chat') {
        ask();
    } else if (currentTab === 'history') {
        loadHistory();
    } else {
        doSearch();
    }
}

async function loadHistory() {
    const list = document.getElementById('history-list');
    const q = document.getElementById('history-search').value.trim();
    list.innerHTML = '<div class="history-note">Loading...</div>';

    try {
        const url = q
            ? `/api/v1/conversations/search?q=${encodeURIComponent(q)}&limit=100`
            : `/api/v1/conversations?limit=100`;
        const resp = await fetch(url);
        const data = await resp.json();
        const convos = data.conversations || [];

        if (convos.length === 0) {
            list.innerHTML = '<div class="history-note">No conversations found.</div>';
            return;
        }

        // Group by session
        const sessions = {};
        convos.forEach(c => {
            const sid = c.session_id || 'unknown';
            if (!sessions[sid]) sessions[sid] = [];
            sessions[sid].push(c);
        });

        let html = '';
        for (const [sid, msgs] of Object.entries(sessions)) {
            const first = msgs[msgs.length - 1];
            const date = first.created_at ? new Date(first.created_at).toLocaleString() : '?';
            html += `<div class="history-session">`;
            html += `<div class="history-session-head">${escapeHtml(date)} &mdash; Session ${escapeHtml(sid.slice(0,8))}</div>`;
            msgs.reverse().forEach(m => {
                const content = m.content.length > 300 ? m.content.slice(0,300) + '...' : m.content;
                const who = m.role === 'user' ? ' from-user' : '';
                html += `<div class="history-turn${who}"><b>${m.role === 'user' ? 'You' : 'Zhenai'}:</b> ${escapeHtml(content)}</div>`;
            });
            html += '</div>';
        }
        list.innerHTML = html;
    } catch (e) {
        list.innerHTML = `<div class="history-note history-error">Error loading history: ${escapeHtml(e.message)}</div>`;
    }
}

// --- Markdown rendering (lightweight) ---
// Code (fenced and inline) is cut out before the prose rules run, so
// **kwargs, *ptr and *.go inside code are never turned into bold/italic/<br>.
// Fenced blocks keep their info string ("go", "c++", "file:x.go") in
// data-lang for highlightCode().
function renderMarkdown(text) {
    const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    const kept = [];
    // A kept piece is marked by its index between two NULs. The text's own
    // NULs are dropped first, so at the end the NULs in html are exactly
    // the marks, and splitting on NUL gives text, index, text, index, ...
    const MARK = '\u0000';
    const keep = html => MARK + (kept.push(html) - 1) + MARK;
    let html = text.split(MARK).join('');
    html = html.replace(/```([^\n`]*)\n([\s\S]*?)```/g, (_, info, code) => {
        info = info.trim();
        const label = /^(file|download):/.test(info) ? `<div class="code-label">${esc(info.replace(/^\w+:/, ''))}</div>` : '';
        const lang = info ? ` data-lang="${esc(info).replace(/"/g, '&quot;')}"` : '';
        return keep(`${label}<pre><code${lang}>${esc(code.replace(/\s+$/, ''))}</code></pre>`);
    });
    html = html.replace(/`([^`\n]+)`/g, (_, code) => keep(`<code>${esc(code)}</code>`));
    html = esc(html);
    // Bold: **...**
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    // Italic: *...*
    html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
    // Line breaks
    html = html.replace(/\n/g, '<br>');
    return html.split(MARK).map((part, i) => (i % 2 ? kept[+part] : part)).join('');
}

// Swap in server-side highlighting (POST /api/v1/highlight, Pygments) for
// the fenced blocks under el that name a language. Any failure leaves the
// plain rendering, which already shows all the text.
async function highlightCode(el) {
    const codes = [...el.querySelectorAll('pre > code[data-lang]')];
    if (!codes.length) return;
    try {
        const r = await fetch(API + '/api/v1/highlight', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({blocks: codes.map(c => ({lang: c.dataset.lang, code: c.textContent}))}),
        });
        if (!r.ok) return;
        const data = await r.json();
        codes.forEach((c, i) => {
            if (typeof data.html?.[i] === 'string') c.innerHTML = data.html[i];
        });
    } catch (_e) { /* plain rendering stays */ }
}

// --- Detect downloadable file blocks ---
function extractDownloadBlocks(text) {
    const regex = /```download:([^\n]+)\n([\s\S]*?)```/g;
    const blocks = [];
    let match;
    while ((match = regex.exec(text)) !== null) {
        blocks.push({ filename: match[1].trim(), content: match[2].trim() });
    }
    return blocks;
}

// --- Model badge helper ---
function modelBadge(model) {
    if (!model) return '';
    const m = model.toLowerCase();
    if (m.includes('opus')) return '<span class="model-badge opus">OPUS</span>';
    if (m.includes('sonnet')) return '<span class="model-badge sonnet">SONNET</span>';
    if (m.includes('haiku')) return '<span class="model-badge haiku">HAIKU</span>';
    if (m.includes('memory')) return '<span class="model-badge memory">MEMORY</span>';
    if (m.includes('mistral')) return '<span class="model-badge mistral">MISTRAL</span>';
    return `<span class="model-badge mistral">${escapeHtml(model.toUpperCase())}</span>`;
}

// --- Model CSS class for message styling ---
function modelMsgClass(model) {
    if (!model) return '';
    const m = model.toLowerCase();
    if (m.includes('opus')) return ' claude-opus';
    if (m.includes('sonnet')) return ' claude-sonnet';
    if (m.includes('haiku')) return ' claude-haiku';
    if (m.includes('memory')) return ' from-memory';
    return '';
}

// ──────────────────────────────────────────────────────────────────
// Inline action buttons (Stage 2 of the operator interaction surface)
//
// Scans LLM answers for references to runbook names + kanban actions,
// renders confirm-buttons inline. Click → POST to the Python webapp's
// /api/v1/runbooks/<n>/execute or /api/v1/tool/exec, which proxies
// through cmd/zhen-agentd /api/v1/tool/exec → pkg/champion.Dispatch.
// T6b closure inherited automatically.
//
// Runbook names are loaded once on page boot from /api/v1/runbooks
// and cached. Kanban-create suggestions are matched on simple natural-
// language patterns ("create a kanban task to X") with a best-effort
// title extraction.
// ──────────────────────────────────────────────────────────────────

const RUNBOOK_NAME_CACHE = new Set();
fetch(API + '/api/v1/runbooks').then(r => r.json()).then(d => {
    const list = d.runbooks || (Array.isArray(d) ? d : []);
    list.forEach(rb => {
        const name = (rb.name || rb).toString();
        // Cache the bare name AND the path-prefixed name so detection
        // matches whichever the model emits ('service-health-sweep'
        // OR 'observe/service-health-sweep').
        RUNBOOK_NAME_CACHE.add(name);
    });
}).catch(() => { /* empty cache → no inline runbook actions; harmless */ });

// detectActions returns a list of action descriptors found in answer text.
// Each descriptor: {kind: 'runbook'|'kanban_create', name, label, payload}
function detectActions(answer) {
    if (!answer) return [];
    const found = [];
    const seen = new Set();

    // 1. Runbook references — scan for any cached runbook name with
    //    word-boundary on both sides. Avoid duplicates.
    for (const name of RUNBOOK_NAME_CACHE) {
        // Word-boundary check: name surrounded by whitespace, code-fences,
        // backticks, or punctuation typical in prose.
        const re = new RegExp('(?:^|[\\s`/\\\\(\\[\\.,;:])' +
            name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') +
            '(?:[\\s`/\\\\)\\]\\.,;:!?]|$)');
        if (re.test(answer) && !seen.has(name)) {
            seen.add(name);
            found.push({
                kind:  'runbook',
                name:  name,
                label: '▶ Run ' + name + ' (dry-run)',
                payload: { dry_run: true },
            });
        }
    }

    // 2. Kanban-create suggestions — heuristic NL match. The model
    //    commonly says "you could create a task ...", "make a kanban
    //    task to ...", "open a task for ...". Extract a best-effort
    //    title (first sentence after the trigger phrase, capped at 80c).
    const kanbanTriggers = [
        /(?:create|make|open|add)\s+(?:a\s+)?(?:kanban\s+)?task\s+(?:to|for|titled)\s+([^.\n]{6,80})/i,
        /file\s+(?:a\s+)?(?:kanban\s+)?task\s+(?:to|for|titled)\s+([^.\n]{6,80})/i,
    ];
    for (const re of kanbanTriggers) {
        const m = answer.match(re);
        if (m && m[1]) {
            const title = m[1].trim().replace(/[`*"']/g, '').slice(0, 80);
            if (!seen.has('kanban:' + title)) {
                seen.add('kanban:' + title);
                found.push({
                    kind:  'kanban_create',
                    label: '＋ Create kanban: ' + title,
                    payload: { title: title, status: 'todo' },
                });
                break;  // one suggestion per answer; the model rarely names two distinct tasks
            }
        }
    }

    return found;
}

// Render the action buttons + result panel for a list of detected actions.
// Returns an HTMLElement to append below the message body.
function renderActionButtons(actions) {
    if (!actions.length) return null;
    const wrap = document.createElement('div');
    wrap.className = 'inline-actions';
    actions.forEach(action => {
        const row = document.createElement('div');
        row.className = 'action-row';
        const btn = document.createElement('button');
        btn.className = 'action-btn ' + action.kind;
        btn.textContent = action.label;
        const result = document.createElement('div');
        result.className = 'action-result';
        btn.addEventListener('click', async () => {
            btn.disabled = true;
            btn.textContent = btn.textContent.replace(/^[▶＋]/, '⏳');
            result.innerHTML = '<span class="action-pending">running through Champion gate…</span>';
            try {
                let resp;
                if (action.kind === 'runbook') {
                    resp = await fetch(API + '/api/v1/runbooks/' +
                        encodeURIComponent(action.name.split('/').pop()) +
                        '/execute', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify(action.payload),
                    });
                } else if (action.kind === 'kanban_create') {
                    // Through the web app, which calls the daemon's
                    // /tool/exec with its API key and a direct-user
                    // justification. Champion's gate enforces the rules;
                    // pgKanbanStore writes the row.
                    const taskId = 'task-zhen-' + Date.now();
                    resp = await fetch(API + '/api/v1/kanban/create', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({
                            task: { id: taskId, ...action.payload },
                        }),
                    });
                }
                const body = await resp.json();
                if (resp.ok && (body.status === 'ok' || body.status === 'success')) {
                    btn.textContent = btn.textContent.replace(/^⏳/, '✓');
                    btn.classList.add('action-ok');
                    result.innerHTML = '<pre class="action-output">' +
                        escapeHtml(JSON.stringify(body.result || body, null, 2).slice(0, 2000)) +
                        '</pre>';
                } else if (body.status === 'pending_confirmation' && body.pending_token) {
                    btn.textContent = '? Confirm: ' + action.name;
                    btn.classList.add('action-pending');
                    result.innerHTML = '<span class="action-warn">Held for you: ' +
                        escapeHtml(body.reason || 'this needs your confirmation') + '</span> ';
                    offerConfirm(result, body.pending_token, action.name, (out) => {
                        const pre = document.createElement('pre');
                        pre.className = 'action-output';
                        pre.textContent = JSON.stringify(out.body.result || out.body, null, 2).slice(0, 2000);
                        result.appendChild(pre);
                    });
                } else {
                    btn.textContent = btn.textContent.replace(/^⏳/, '✗');
                    btn.classList.add('action-fail');
                    result.innerHTML = '<span class="action-fail">' +
                        escapeHtml(body.reason || body.error || JSON.stringify(body).slice(0, 500)) +
                        '</span>';
                }
            } catch (e) {
                btn.textContent = btn.textContent.replace(/^⏳/, '✗');
                btn.classList.add('action-fail');
                result.innerHTML = '<span class="action-fail">' + escapeHtml(String(e)) + '</span>';
            }
        });
        row.appendChild(btn);
        row.appendChild(result);
        wrap.appendChild(row);
    });
    return wrap;
}

// escapeHtml makes s safe as element text and inside a quoted attribute.
function escapeHtml(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// --- Chat ---
function addMsg(text, cls, meta, extra) {
    const d = document.createElement('div');
    d.className = 'msg ' + cls;
    if (cls.includes('zhen') && text === '__typing__') {
        d.innerHTML = '<div class="typing-indicator"><span></span><span></span><span></span></div>';
    } else if (cls.includes('zhen')) {
        d.innerHTML = renderMarkdown(text);
        highlightCode(d);
    } else {
        d.textContent = text;
    }
    if (meta) {
        const m = document.createElement('div');
        m.className = 'meta';
        m.innerHTML = meta;
        d.appendChild(m);
    }
    if (extra) {
        d.appendChild(extra);
    }
    msgs.appendChild(d);
    msgs.scrollTop = msgs.scrollHeight;
    return d;
}

function updateStats(elapsed) {
    queryCount++;
    totalTime += elapsed;
    document.getElementById('stat-queries').textContent = queryCount;
    document.getElementById('stat-avg-time').textContent = (totalTime / queryCount).toFixed(1);
}

function addHistory(q) {
    queryHistory.unshift(q);
    if (queryHistory.length > 20) queryHistory.pop();
    // What was typed goes in as text, and the click is a listener: the page's
    // policy refuses an onclick attribute, and a question is not markup.
    historyEl.replaceChildren(...queryHistory.map(h => {
        const item = document.createElement('div');
        item.className = 'history-item';
        item.textContent = h;
        item.addEventListener('click', () => ask(h));
        return item;
    }));
}

// --- Download file ---
async function downloadFile(filename, content) {
    try {
        const r = await fetch(API + '/api/v1/generate-file', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ filename, content }),
        });
        if (!r.ok) throw new Error('Download failed');
        const blob = await r.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        a.click();
        URL.revokeObjectURL(url);
    } catch(e) {
        alert('Download failed: ' + e.message);
    }
}

// --- Remember answer ---
async function rememberAnswer(question, answer, model, btnEl) {
    try {
        const r = await fetch(API + '/api/v1/remember', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ question, answer, model }),
        });
        const data = await r.json();
        if (data.status === 'remembered') {
            btnEl.textContent = 'Remembered';
            btnEl.classList.add('remembered');
            btnEl.disabled = true;
        } else {
            alert('Failed: ' + (data.error || 'unknown'));
        }
    } catch(e) {
        alert('Remember failed: ' + e.message);
    }
}

async function ask(preset) {
    const question = preset || qInput.value.trim();
    if (!question) return;
    if (currentTab !== 'chat') switchTab('chat');
    qInput.value = '';
    btn.disabled = true;

    // Build file content string if file is attached
    let fileContent = null;
    if (uploadedFile) {
        fileContent = `--- Uploaded file: ${uploadedFile.name} ---\n${uploadedFile.content}\n---`;
        addMsg(question + `\n\n[Attached: ${uploadedFile.name}]`, 'user');
        removeFile();
    } else {
        addMsg(question, 'user');
    }
    addHistory(question);
    const thinking = addMsg('__typing__', 'zhen');

    try {
        const body = { question, session_id: sessionId };
        if (fileContent) body.file_content = fileContent;

        const r = await fetch(API + '/api/v1/query', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(body),
        });
        if (r.status === 401) {
            // The session has ended and the fetch wrapper has opened the
            // login panel. Put the question back and send nothing again:
            // the person sends it once they are logged in.
            msgs.removeChild(thinking);
            qInput.value = question;
            addMsg('Your login ended before this was sent. Log in, then send it again: the question is still in the box.', 'zhen');
            btn.disabled = false;
            return;
        }
        const data = await r.json();
        msgs.removeChild(thinking);

        if (data.error) {
            addMsg('Error: ' + data.error, 'zhen');
        } else {
            const model = data.model || 'mistral-7b';
            const badge = modelBadge(model);
            const extraCls = modelMsgClass(model);

            const sources = (data.sources || []).map(s => {
                const viewUrl = '/view/source?' + (s.id ? 'id=' + encodeURIComponent(s.id) : 'path=' + encodeURIComponent(s.source));
                return `<a href="${viewUrl}" target="_blank" rel="noopener" title="${escapeHtml(s.preview || '')}">${escapeHtml(s.source)}</a>`;
            }).join(' | ');

            // Check for download blocks
            const downloads = extractDownloadBlocks(data.answer);

            // Build action buttons
            const actionsDiv = document.createElement('div');
            actionsDiv.className = 'msg-actions';

            // Remember button (skip if from memory)
            if (!data.from_memory) {
                const remBtn = document.createElement('button');
                remBtn.textContent = 'Remember';
                remBtn.onclick = function() { rememberAnswer(question, data.answer, model, remBtn); };
                actionsDiv.appendChild(remBtn);
            }

            // Download buttons
            for (const dl of downloads) {
                const dlBtn = document.createElement('button');
                dlBtn.textContent = 'Download: ' + dl.filename;
                dlBtn.onclick = function() { downloadFile(dl.filename, dl.content); };
                actionsDiv.appendChild(dlBtn);
            }

            // Inline action buttons (Stage 2): detect runbook/kanban refs
            // in the answer; render Champion-gated confirm-buttons.
            const detectedActions = detectActions(data.answer);
            const inlineActions = renderActionButtons(detectedActions);
            if (inlineActions) {
                actionsDiv.appendChild(inlineActions);
            }

            // No sheet reached the model: say so, so the answer is not
            // read as grounded. Commands and recalled answers are not
            // model answers and carry no such line.
            const unsourced = !sources && model !== 'command' && !data.from_memory;
            const metaHtml = `${badge} ${escapeHtml(data.elapsed_seconds)}s | ${escapeHtml(data.tokens_used)} tok` +
                (sources ? `<div class="sources">Sources: ${sources}</div>` : '') +
                (unsourced ? '<div class="sources unsourced">No source: no sheet matched this question. The answer is the model\'s own and is unchecked.</div>' : '');

            addMsg(data.answer, 'zhen' + extraCls, metaHtml, actionsDiv);
            updateStats(data.elapsed_seconds);
        }
    } catch(e) {
        msgs.removeChild(thinking);
        addMsg('Connection error: ' + e.message, 'zhen');
    }
    btn.disabled = false;
    qInput.focus();
}

// --- Search ---
async function doSearch(preset) {
    const query = preset || qInput.value.trim();
    if (!query) return;
    qInput.value = '';
    btn.disabled = true;
    addHistory(query);

    searchResultsEl.innerHTML = '<div class="search-empty">Searching...</div>';

    try {
        const r = await fetch(API + '/api/v1/search', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({query, k: 15}),
        });
        const data = await r.json();

        if (data.error) {
            searchResultsEl.innerHTML = `<div class="search-empty">Error: ${escapeHtml(data.error)}</div>`;
        } else if (!data.results || data.results.length === 0) {
            searchResultsEl.innerHTML = '<div class="search-empty">No results found.</div>';
        } else {
            searchResultsEl.innerHTML = data.results.map((r, i) => {
                const preview = escapeHtml((r.content || '').substring(0, 200));
                const dist = typeof r.distance === 'number' ? r.distance.toFixed(4) : '—';
                const ringMatch = (r.source || '').match(/ring(\d+)/i);
                const ringHtml = ringMatch
                    ? `<span class="sr-ring">Ring ${escapeHtml(ringMatch[1])}</span>`
                    : '';
                return `<div class="search-result">
                    <div class="sr-source">${escapeHtml(r.source || '')}</div>
                    <div class="sr-preview">${preview}</div>
                    <div class="sr-meta">
                        <span class="sr-score">Distance: ${dist}</span>
                        <span>Type: ${escapeHtml(r.type || '—')}</span>
                        ${ringHtml}
                        <span>#${i + 1} of ${escapeHtml(data.total)}</span>
                    </div>
                </div>`;
            }).join('');
        }
    } catch(e) {
        searchResultsEl.innerHTML = `<div class="search-empty">Connection error: ${escapeHtml(e.message)}</div>`;
    }
    btn.disabled = false;
    qInput.focus();
}

// --- Input handler ---
qInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !btn.disabled) handleSubmit();
});

// Health check: the probe is open before login.
async function checkHealth() {
    try {
        const health = await fetch(API + '/health').then(r => r.json());
        const dot = document.getElementById('status-dot');
        const text = document.getElementById('status-text');
        if (health.rag_ready) {
            dot.className = 'status-dot';
            text.textContent = 'Online';
        } else {
            dot.className = 'status-dot offline';
            // The probe gives a fixed word; the reason is behind the login.
            text.textContent = 'Not ready';
        }
    } catch(_e) {
        document.getElementById('status-dot').className = 'status-dot offline';
        document.getElementById('status-text').textContent = 'Offline';
    }
    refreshStats();
}

// Stats need a login. This is the one place they are fetched: at page load,
// with every health check, and from the login handler, so the sidebar fills
// in after a login. Resolves to the stats, {login_required: true} before
// login, or {} when they cannot be had.
async function refreshStats() {
    const sv = document.getElementById('stat-vectors');
    const sc = document.getElementById('stat-chunks');
    const info = document.getElementById('model-info');
    let stats;
    try {
        const r = await fetch(API + '/api/v1/stats');
        if (r.status === 401) {
            for (const el of [sv, sc]) {
                if (el) { el.textContent = '—'; el.title = 'log in to see'; }
            }
            if (info) info.textContent = 'log in to see the corpus and model';
            return {login_required: true};
        }
        stats = await r.json();
    } catch(_e) {
        return {};
    }
    if (!stats || stats.error) return {};
    // Post-WAVE15 rewire: stats shape changed. vor_health.sheets +
    // inference_model are the new fields; index_vectors/corpus_chunks
    // were FAISS-era and no longer populate.
    const sheets = stats.vor_health ? stats.vor_health.sheets : null;
    const cats   = stats.vor_health ? stats.vor_health.categories : null;
    const model  = stats.inference_model || stats.model || 'qwen-coder-7b';
    if (sv) { sv.textContent = sheets != null ? sheets.toLocaleString() : '—'; sv.title = sheets != null ? '' : 'vor did not answer'; }
    if (sc) { sc.textContent = cats   != null ? cats.toLocaleString()   : '—'; sc.title = cats   != null ? '' : 'vor did not answer'; }
    if (info) {
        info.textContent = `${model} | ${sheets != null ? sheets.toLocaleString() + ' sheets' : 'vor'}`;
    }
    return stats;
}
checkHealth();
setInterval(checkHealth, 30000);

// ──────────────────────────────────────────────────────────────────
// Operator Dashboard (Stage 3) — populates the #dashboard-view from
// the /api/v1/system/state, /api/v1/audit/recent, /api/v1/kanban/summary,
// /api/v1/runbooks/recent endpoints. All read-only. Auto-refresh
// every 10 s when the dashboard tab is active.
// ──────────────────────────────────────────────────────────────────

async function refreshDashboard() {
    const updated = document.getElementById('dash-updated');
    updated.textContent = 'refreshing…';
    try {
        const [state, audit, kanban, runbooks] = await Promise.all([
            fetch(API + '/api/v1/system/state').then(r => r.json()).catch(() => ({})),
            fetch(API + '/api/v1/audit/recent?limit=15').then(r => r.json()).catch(() => ({rows:[]})),
            fetch(API + '/api/v1/kanban/summary').then(r => r.json()).catch(() => ({})),
            fetch(API + '/api/v1/runbooks/recent?limit=10').then(r => r.json()).catch(() => ({rows:[]})),
        ]);
        renderDashSystem(state);
        renderDashKanban(kanban);
        renderDashAudit(audit.rows || []);
        renderDashRunbooks(runbooks.rows || []);
        const t = new Date(state.timestamp || Date.now()).toLocaleTimeString();
        updated.textContent = 'updated ' + t;
    } catch (e) {
        updated.textContent = 'refresh failed: ' + e.message;
    }
}

function renderDashSystem(s) {
    const el = document.getElementById('dash-system');
    if (!el) return;
    const probes = [
        ['vor (retrieval)',     s.vor,          v => v ? `${v.sheets || '?'} sheets / ${v.categories || '?'} categories` : 'unreachable'],
        ['llama-server',        s.llama_server, v => v ? (v.model || 'qwen-coder-7b') : 'unreachable'],
        ['zhen-agentd (gate)',  s.agentd,       v => v ? 'gating ON' : 'down — mutations disabled'],
        ['kanban-app',          s.kanban,       v => v ? `wotan=${v.wotan_enabled ? 'on' : 'off'}` : 'unreachable'],
        ['wiki',                s.wiki,         v => v ? 'serving' : 'unreachable'],
        ['dashboard-backend',   s.dashboard,    v => v ? 'metrics + WS' : 'unreachable'],
        ['wotan (msg-bus)',     s.wotan,        v => v ? `${v.rooms || 0} rooms / ${v.total_members || 0} members` : 'unreachable'],
        ['the well (PG)',       s.webui,        v => v && v.well_connected ? 'connected' : 'disconnected'],
    ];
    el.innerHTML = probes.map(([label, val, fmt]) => {
        const cls = val ? 'dash-probe-ok' : 'dash-probe-down';
        return `<div class="dash-probe ${cls}">
            <div class="dash-probe-label">${label}</div>
            <div class="dash-probe-value">${escapeHtml(fmt(val))}</div>
        </div>`;
    }).join('');
}

function renderDashKanban(k) {
    const grid = document.getElementById('dash-kanban');
    const tot = document.getElementById('dash-kanban-total');
    if (!grid || !tot) return;
    if (k.error) {
        tot.textContent = 'unavailable';
        grid.innerHTML = `<div class="dash-meta">${escapeHtml(k.error)}</div>`;
        return;
    }
    tot.textContent = `${k.total || 0} active tasks`;
    const cols = k.counts || {};
    const order = ['todo', 'in-progress', 'review', 'done'];
    const colHtml = order.filter(c => cols[c] != null).map(c =>
        `<div class="dash-kanban-col">
            <div class="dash-kanban-num">${escapeHtml(cols[c])}</div>
            <div class="dash-kanban-label">${c}</div>
        </div>`
    ).join('');
    const recent = (k.recent_tasks || []).slice(0, 6).map(t =>
        `<div class="dash-kanban-task">
            <span class="dash-kanban-status ${escapeHtml(t.status)}">${escapeHtml(t.status)}</span>
            <span class="dash-kanban-title">${escapeHtml(t.title || '')}</span>
        </div>`
    ).join('');
    grid.innerHTML = `<div class="dash-kanban-cols">${colHtml}</div>` +
                     `<div class="dash-kanban-recent">${recent}</div>`;
}

function renderDashAudit(rows) {
    const el = document.getElementById('dash-audit');
    if (!el) return;
    if (!rows.length) {
        el.innerHTML = '<div class="dash-meta">no audit rows yet — run a tool through cmd/zhen-agentd /api/v1/tool/exec to populate</div>';
        return;
    }
    el.innerHTML = rows.map(r =>
        `<div class="dash-audit-row dash-status-${escapeHtml(r.status || 'unknown')}">
            <div class="dash-audit-id">#${escapeHtml(r.id)}</div>
            <div class="dash-audit-type">${escapeHtml(r.action_type || '')}</div>
            <div class="dash-audit-status">${escapeHtml(r.status || '')}</div>
            <div class="dash-audit-intent">${escapeHtml((r.intent || '').slice(0, 80))}</div>
            <div class="dash-audit-time">${escapeHtml((r.completed_at || r.planned_at || '').slice(11, 19))}</div>
        </div>`
    ).join('');
}

function renderDashRunbooks(rows) {
    const el = document.getElementById('dash-runbooks');
    if (!el) return;
    if (!rows.length) {
        el.innerHTML = '<div class="dash-meta">no runbook executions in zhen_actions yet — execute a runbook from the Chat tab to populate</div>';
        return;
    }
    el.innerHTML = rows.map(r => {
        const ok = r.status === 'completed';
        return `<div class="dash-runbook-row dash-status-${escapeHtml(r.status || 'unknown')}">
            <div class="dash-runbook-id">#${escapeHtml(r.id)}</div>
            <div class="dash-runbook-status">${ok ? '✓' : '✗'} ${escapeHtml(r.status || '')}</div>
            <div class="dash-runbook-intent">${escapeHtml((r.intent || '').slice(0, 100))}</div>
            <div class="dash-runbook-elapsed">${escapeHtml(r.elapsed_ms || 0)}ms</div>
        </div>`;
    }).join('');
}

// Auto-refresh: 10 s while the dashboard tab is active. Pauses
// otherwise — no point polling system state when the user is chatting.
function dashAutoTick() {
    if (currentTab === 'dashboard' && document.getElementById('dash-autorefresh').checked) {
        refreshDashboard();
    }
}
setInterval(dashAutoTick, 10000);

// ──────────────────────────────────────────────────────────────────
// Operator actions sidebar (Stage 4) — always-visible direct-action
// buttons. Each handler hits an /api/v1/* endpoint and renders the
// result in #op-result. Mutating actions (runbook execute) go through
// the Champion gate via the same path Stage 2's inline buttons use.
// ──────────────────────────────────────────────────────────────────

function opShow(html) {
    const el = document.getElementById('op-result');
    if (!el) return;
    el.innerHTML = html;
    el.scrollTop = 0;
    el.classList.add('visible');
}

function opPending(label) {
    opShow(`<div class="op-pending">⏳ ${escapeHtml(label)}…</div>`);
}

// ─────────────────────── Model selector (ADR-060) ───────────────────────
//
// On-load: GET /api/v1/models to populate the dropdown + mark current.
// On-change: POST /api/v1/models/switch with the selected key. Then poll
// /api/v1/stats every 3 s until inference_model matches the selected
// key, with a 6-min hard timeout. The dropdown is disabled during the
// in-flight swap so the operator can't double-fire.

async function loadModelList() {
    const sel = document.getElementById('model-select');
    const status = document.getElementById('model-swap-status');
    if (!sel) return;
    try {
        const r = await fetch(API + '/api/v1/models');
        const body = await r.json();
        sel.innerHTML = '';
        const keys = body.keys || [];
        const current = body.current || '';
        for (const k of keys) {
            const opt = document.createElement('option');
            opt.value = k;
            opt.textContent = k + (k === current ? '  (loaded)' : '');
            if (k === current) opt.selected = true;
            sel.appendChild(opt);
        }
        if (keys.length === 0) {
            sel.innerHTML = '<option value="">(no models registered)</option>';
        }
        sel.disabled = false;
        status.className = 'model-status';
        status.textContent = current ? `${keys.length} models · ${current} loaded` : `${keys.length} models`;
    } catch (e) {
        status.className = 'model-status error';
        status.textContent = 'failed to load model list: ' + String(e);
    }
}

async function switchModel(key) {
    const sel = document.getElementById('model-select');
    const status = document.getElementById('model-swap-status');
    sel.disabled = true;
    status.className = 'model-status swapping';
    status.textContent = `swapping to ${key}… (45 s – 3 min)`;

    let resp;
    try {
        const r = await fetch(API + '/api/v1/models/switch', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({key: key}),
        });
        resp = {status: r.status, body: await r.json()};
    } catch (e) {
        status.className = 'model-status error';
        status.textContent = 'switch error: ' + String(e);
        sel.disabled = false;
        return;
    }

    // A swap is held for the person logged in: confirm it, then the swap
    // runs and the answer is what /tool/exec would have said.
    if (resp.body.status === 'pending_confirmation' && resp.body.pending_token) {
        status.className = 'model-status swapping';
        status.textContent = `swap to ${key} is held for you `;
        const answer = await new Promise((resolve) => {
            offerConfirm(status, resp.body.pending_token, key, resolve);
        });
        resp = {status: answer.status, body: answer.body};
        status.textContent = `swapping to ${key}… (45 s – 3 min)`;
    }

    // Daemon returns 200 + full ModelSwapResult on success; 4xx/5xx on
    // failure with a body that includes a 'reason' or 'error' field.
    if (resp.status >= 400) {
        status.className = 'model-status error';
        status.textContent = `${resp.status}: ${resp.body.reason || resp.body.error || JSON.stringify(resp.body)}`;
        sel.disabled = false;
        return;
    }

    // Poll /api/v1/models until current === key, with timeout. The
    // backend reads llama-server's /v1/models live and back-maps the
    // gguf basename to a sidebar key — so this is an exact-key match,
    // not a substring search.
    const deadline = Date.now() + 6 * 60 * 1000; // 6 min hard cap
    const t0 = Date.now();
    while (Date.now() < deadline) {
        await new Promise(r => setTimeout(r, 3000));
        try {
            const sr = await fetch(API + '/api/v1/models');
            const sb = await sr.json();
            const elapsed = ((Date.now() - t0) / 1000).toFixed(0);
            if (sb.current === key) {
                status.className = 'model-status ok';
                status.textContent = `✓ swapped to ${key} in ${elapsed}s`;
                await loadModelList();
                // Drop a system note into the chat thread so the
                // conversation context is honest about which model
                // is now answering. Without this the welcome message
                // (set at page-load) is the only model-identity cue
                // and goes stale silently after a swap.
                try {
                    const sr2 = await fetch(API + '/api/v1/stats');
                    const sb2 = await sr2.json();
                    const liveName = prettyModelName(sb2.inference_model);
                    addMsg(`⚙ Switched inference to ${liveName} (sidebar key: ${key}; loaded in ${elapsed}s). Subsequent answers come from this model.`, 'zhen');
                } catch (_e) {
                    addMsg(`⚙ Switched inference to ${key} (loaded in ${elapsed}s). Subsequent answers come from this model.`, 'zhen');
                }
                return;
            }
            status.textContent = `swapping to ${key}… ${elapsed}s elapsed (current: ${sb.current || '?'})`;
        } catch (_e) {
            // Polling errors are transient (zhen_app may be busy); keep trying.
        }
    }

    // Timeout: the swap may still complete in the background, but the UI
    // gives up the lock so the operator can intervene.
    status.className = 'model-status error';
    status.textContent = `timeout after 6 min — model may still be loading; refresh to verify`;
    sel.disabled = false;
}

// Wire the on-change handler once on page load.
window.addEventListener('DOMContentLoaded', () => {
    const sel = document.getElementById('model-select');
    if (sel) {
        sel.addEventListener('change', (e) => {
            const key = e.target.value;
            if (key) switchModel(key);
        });
        loadModelList();
    }
});

async function opHealthSweep() {
    opPending('running service-health-sweep (dry-run, gated)');
    try {
        const r = await fetch(API + '/api/v1/runbooks/service-health-sweep/execute', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ dry_run: true }),
        });
        const body = await r.json();
        if (r.ok && (body.status === 'ok' || body.status === 'success')) {
            const out = body.result ? body.result.output || JSON.stringify(body.result, null, 2) : '(empty)';
            opShow(`<div class="op-section">
                <div class="op-section-head"><b>✓ service-health-sweep</b> · gated via ${escapeHtml(body.gated_via || '?')}</div>
                <pre class="op-output">${escapeHtml(out.slice(0, 3000))}</pre>
            </div>`);
        } else if (body.status === 'pending_confirmation' && body.pending_token) {
            opShow(`<div class="op-section op-warn">
                <div class="op-section-head">? Held for you</div>
                <div class="op-meta">${escapeHtml(body.reason || '')}</div>
                <div class="op-meta" id="op-confirm"></div>
            </div>`);
            offerConfirm(document.getElementById('op-confirm'), body.pending_token, 'service-health-sweep', (out) => {
                opShow(`<pre class="op-output">${escapeHtml(JSON.stringify(out.body.result || out.body, null, 2).slice(0, 3000))}</pre>`);
            });
        } else {
            opShow(`<div class="op-section op-fail">
                <div class="op-section-head">✗ ${escapeHtml(body.status || 'error')}</div>
                <div class="op-meta">${escapeHtml(body.reason || body.error || JSON.stringify(body))}</div>
            </div>`);
        }
    } catch (e) {
        opShow(`<div class="op-section op-fail">✗ ${escapeHtml(String(e))}</div>`);
    }
}

async function opRecentAudit() {
    opPending('fetching last 10 audit rows');
    try {
        const r = await fetch(API + '/api/v1/audit/recent?limit=10');
        const body = await r.json();
        const rows = body.rows || [];
        if (!rows.length) {
            opShow(`<div class="op-section">
                <div class="op-section-head">audit feed</div>
                <div class="op-meta">no rows in zhen_actions yet${body.error ? ' — ' + escapeHtml(body.error) : ''}</div>
            </div>`);
            return;
        }
        const html = rows.map(r =>
            `<div class="op-audit-row">
                <span class="op-audit-id">#${escapeHtml(r.id)}</span>
                <span class="op-audit-type">${escapeHtml(r.action_type || '')}</span>
                <span class="op-audit-status op-status-${escapeHtml(r.status || '')}">${escapeHtml(r.status || '')}</span>
                <span class="op-audit-intent">${escapeHtml((r.intent || '').slice(0, 60))}</span>
            </div>`
        ).join('');
        opShow(`<div class="op-section">
            <div class="op-section-head">audit feed — last ${rows.length}</div>
            ${html}
        </div>`);
    } catch (e) {
        opShow(`<div class="op-section op-fail">✗ ${escapeHtml(String(e))}</div>`);
    }
}

async function opRecentRunbooks() {
    opPending('fetching recent runbook executions');
    try {
        const r = await fetch(API + '/api/v1/runbooks/recent?limit=10');
        const body = await r.json();
        const rows = body.rows || [];
        if (!rows.length) {
            opShow(`<div class="op-section">
                <div class="op-section-head">recent runbook executions</div>
                <div class="op-meta">no rows yet${body.error ? ' — ' + escapeHtml(body.error) : ''}. Click the Health sweep button above to populate.</div>
            </div>`);
            return;
        }
        const html = rows.map(r => {
            const ok = r.status === 'completed';
            return `<div class="op-runbook-row">
                <span class="op-audit-id">#${escapeHtml(r.id)}</span>
                <span class="op-audit-status op-status-${escapeHtml(r.status || '')}">${ok ? '✓' : '✗'} ${escapeHtml(r.status || '')}</span>
                <span class="op-audit-intent">${escapeHtml((r.intent || '').slice(0, 70))}</span>
                <span class="op-meta">${escapeHtml(r.elapsed_ms || 0)}ms</span>
            </div>`;
        }).join('');
        opShow(`<div class="op-section">
            <div class="op-section-head">recent runbook executions</div>
            ${html}
        </div>`);
    } catch (e) {
        opShow(`<div class="op-section op-fail">✗ ${escapeHtml(String(e))}</div>`);
    }
}

function opSearchConversations() {
    // Jump to the History tab and focus its search box. The History tab
    // already implements full-text search via /api/v1/conversations/search.
    switchTab('history');
    const el = document.getElementById('history-search');
    if (el) { el.focus(); el.select && el.select(); }
    opShow(`<div class="op-section"><div class="op-meta">→ switched to History tab. Type a search term and press Enter.</div></div>`);
}

function opRefreshStats() {
    opPending('refreshing stats + system probes');
    Promise.all([
        checkHealth(),
        refreshDashboard().catch(() => null),
    ]).then(() => {
        opShow(`<div class="op-section"><div class="op-meta">✓ checkHealth + dashboard refreshed.</div></div>`);
    }).catch(e => {
        opShow(`<div class="op-section op-fail">✗ ${escapeHtml(String(e))}</div>`);
    });
}

// Format the live model identifier into a human-friendly label.
// llama-server reports the GGUF basename (e.g.
// "Qwen2.5-Coder-14B-Instruct-Q4_K_M") which is too noisy for the
// welcome — strip the quant suffix and lowercase. Fall back to the
// raw value if the suffix isn't recognised.
function prettyModelName(raw) {
    if (!raw) return 'qwen-coder-7b';
    let name = String(raw);
    if (name.endsWith('.gguf')) name = name.slice(0, -5);
    name = name.replace(/-(Q[0-9]+_K_[MS]|Q[0-9]+_[0-9]+|IQ[0-9]+_XS|f16|fp16|F16)$/i, '');
    return name;
}

// Welcome — post-WAVE15 rewire shape (vor + qwen-coder backends).
// Fetches live stats from /api/v1/stats (sheets/categories/model) and the
// runbook list count, then composes a current-state opener. Falls back to
// a static line on any HTTP error so the UI degrades cleanly.
Promise.all([
    refreshStats(),
    fetch(API + '/api/v1/runbooks').then(r => r.json()).then(d => d.runbooks ? d.runbooks.length : (Array.isArray(d) ? d.length : 0)).catch(() => 0),
]).then(([s, rbCount]) => {
    const vh = s.vor_health || {};
    // Before login the counts are not ours to show; say so rather than "?".
    const corpus = s.login_required
        ? 'vor (log in to see the sheet and category counts)'
        : (vh.sheets != null && vh.categories != null
            ? `vor with ${vh.sheets} sheets across ${vh.categories} categories`
            : 'vor');
    const model = prettyModelName(s.inference_model);
    const memBack = (s.memory_recall && s.memory_recall.includes('T1')) ? ' Memory recall is display-only (T1 closed).' : '';
    const rbLine = rbCount > 0 ? ` ${rbCount} operational runbooks available.` : '';
    const swapHint = ' Swap models any time via the sidebar dropdown.';
    addMsg(
        `I am Zhenai (真爱), champion of the Unheaded Kingdom.\n\n` +
        `Retrieval: ${corpus} — Unheaded markdown, ADRs, battle plans, embedded cheatsheets. Source-trust labeled (canonical / local / external).\n\n` +
        `Inference: ${model} via local llama-server on AMD RX 7700 XT (ROCm).${rbLine}${memBack}${swapHint}\n\n` +
        `📎 Need a code review or want me to read a specific file? Drag the file straight into this window, or click the + button next to the input box. The whole file goes into my context.\n\n` +
        `Ask me anything.`,
        'zhen'
    );
}).catch(() => {
    addMsg(
        "I am Zhenai (真爱), champion of the Unheaded Kingdom.\n\n" +
        "Local qwen-coder-7b inference + vor retrieval.\n\n" +
        "📎 Drag a file into this window (or click + next to the input) to include its full contents in your query — best path for code review or specific-file questions.\n\n" +
        "Ask me anything.",
        'zhen'
    );
});

// ──────────────────────────────────────────────────────────────────
// Handlers. The page's markup carries none (its policy refuses an on*
// attribute): a button says what it is for in a data attribute or an id,
// and the listener is attached here.
// ──────────────────────────────────────────────────────────────────

function onClick(id, fn) {
    document.getElementById(id).addEventListener('click', fn);
}

for (const tabBtn of document.querySelectorAll('.tab-bar button[data-tab]')) {
    tabBtn.addEventListener('click', () => switchTab(tabBtn.dataset.tab));
}
for (const askBtn of document.querySelectorAll('#suggestions button[data-ask]')) {
    askBtn.addEventListener('click', () => ask(askBtn.dataset.ask));
}

document.getElementById('history-search').addEventListener('keyup', (e) => {
    if (e.key === 'Enter') loadHistory();
});
onClick('history-go', () => loadHistory());
onClick('history-all', () => {
    document.getElementById('history-search').value = '';
    loadHistory();
});
onClick('dash-refresh', () => refreshDashboard());
onClick('remove-file', () => removeFile());
onClick('upload-btn', () => document.getElementById('file-input').click());
onClick('btn', () => handleSubmit());

const OP_ACTIONS = {
    'health-sweep': opHealthSweep,
    'recent-audit': opRecentAudit,
    'recent-runbooks': opRecentRunbooks,
    'search-conversations': opSearchConversations,
    'refresh-stats': opRefreshStats,
};
for (const opBtn of document.querySelectorAll('.op-actions button[data-op]')) {
    opBtn.addEventListener('click', () => OP_ACTIONS[opBtn.dataset.op]());
}
