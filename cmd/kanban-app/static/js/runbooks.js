/**
 * Runbooks view and the card modal's runbook / agent-run blocks
 * (S-CHAMPION-HARDENING Phase 12).
 *
 * - Runbooks: GET /api/v1/runbooks (runbooks/MANIFEST.sha256 entries), one
 *   "Create task" button each: POST /api/v1/tasks/from-runbook writes the
 *   card to the board's store (The Well); the board never invents a card.
 * - Progress: SSE "runbook.progress" (run-runbook.py on Wotan
 *   runbooks.progress) updates "step k/n" in the list and an open modal.
 * - Card modal: GET /api/v1/tasks/{id} adds agent_run (zhen_agent_runs:
 *   branch, review note, review reason) and, for a runbook card, runbook.
 *
 * Everything is rendered with textContent; styles are classes (CSP).
 */
// These pages load several <script> tags sharing one global scope; the names
// below are defined by sibling files: Board, WebSocketClient. The linter, which
// reads one file at a time, is told about them per file in .oxlintrc.json.

const RunbookView = (function() {
    'use strict';

    const progress = new Map();   // runbook ref -> latest progress event
    let shownTask = null;         // { id, runbook } of the open modal

    function progressText(p) {
        if (!p) return '';
        let text;
        if (p.state === 'done') {
            text = 'done ' + p.total + '/' + p.total;
        } else {
            text = 'step ' + p.step + '/' + p.total + ' ' + p.state + (p.name ? ': ' + p.name : '');
        }
        if (p.dry_run) text += ' (dry run)';
        return text;
    }

    function progressClass(p) {
        if (!p) return 'runbook-progress';
        return 'runbook-progress runbook-progress-' + (p.state === 'failed' || p.state === 'stopped' ? 'bad' :
            (p.state === 'done' || p.state === 'ok' ? 'good' : 'busy'));
    }

    function el(tag, className, text) {
        const node = document.createElement(tag);
        if (className) node.className = className;
        if (text !== undefined && text !== null) node.textContent = String(text);
        return node;
    }

    /* ---------- Runbooks view ---------- */

    async function loadRunbooks() {
        const list = document.getElementById('runbooksList');
        const note = document.getElementById('runbooksNote');
        if (!list) return;
        list.replaceChildren(el('div', 'archive-empty', 'Loading...'));
        try {
            const resp = await fetch('/api/v1/runbooks');
            const data = await resp.json().catch(() => ({}));
            if (!resp.ok) {
                list.replaceChildren(el('div', 'archive-empty', data.error || ('HTTP ' + resp.status)));
                return;
            }
            const runbooks = data.runbooks || [];
            if (note) note.textContent = runbooks.length + ' in runbooks/MANIFEST.sha256';
            if (runbooks.length === 0) {
                list.replaceChildren(el('div', 'archive-empty', 'No runbooks.'));
                return;
            }
            list.replaceChildren(...runbooks.map(renderRunbook));
        } catch (error) {
            list.replaceChildren(el('div', 'archive-empty', 'Runbooks unavailable: ' + error.message));
        }
    }

    function renderRunbook(rb) {
        if (rb.progress) progress.set(rb.ref, rb.progress);
        const row = el('div', 'runbook-item');
        row.dataset.ref = rb.ref;

        const info = el('div', 'runbook-info');
        info.appendChild(el('code', 'runbook-ref', rb.ref));
        const meta = [rb.status || 'no status', 'risk ' + (rb.risk || '-'), rb.steps + ' steps'];
        if (!rb.manifest_ok) meta.push('changed since the manifest');
        info.appendChild(el('span', 'runbook-meta' + (rb.manifest_ok ? '' : ' runbook-meta-warn'), meta.join(' · ')));
        if (rb.description) info.appendChild(el('div', 'runbook-desc', rb.description));
        const p = progress.get(rb.ref);
        info.appendChild(el('span', progressClass(p), progressText(p)));
        row.appendChild(info);

        const btn = el('button', 'btn btn-secondary runbook-create', 'Create task');
        btn.type = 'button';
        btn.addEventListener('click', () => createTask(rb.ref, btn));
        row.appendChild(btn);
        return row;
    }

    async function createTask(ref, btn) {
        btn.disabled = true;
        try {
            const resp = await fetch('/api/v1/tasks/from-runbook', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ runbook: ref })
            });
            if (!resp.ok) {
                const text = await resp.text();
                Board.showToast('error', 'Card not created', text.slice(0, 200));
                return;
            }
            const data = await resp.json();
            if (data.task && !Board.getTask(data.task.id)) Board.addTask(data.task);
            Board.showToast('success', 'Card created', ref);
        } catch (error) {
            Board.showToast('error', 'Card not created', error.message);
        } finally {
            btn.disabled = false;
        }
    }

    function onProgress(p) {
        if (!p || typeof p.runbook !== 'string') return;
        progress.set(p.runbook, p);
        document.querySelectorAll('#runbooksList .runbook-item').forEach((row) => {
            if (row.dataset.ref !== p.runbook) return;
            const span = row.querySelector('.runbook-progress');
            if (span) {
                span.className = progressClass(p);
                span.textContent = progressText(p);
            }
        });
        if (shownTask && shownTask.runbook === p.runbook) renderRunbookBlock(p.runbook, p);
    }

    /* ---------- Card modal ---------- */

    function renderRunbookBlock(ref, p) {
        const section = document.getElementById('detailRunbookSection');
        if (!section) return;
        if (!ref) {
            section.hidden = true;
            return;
        }
        document.getElementById('detailRunbookRef').textContent = ref;
        const span = document.getElementById('detailRunbookProgress');
        span.className = progressClass(p);
        span.textContent = p ? progressText(p) : 'no progress seen';
        section.hidden = false;
    }

    function renderAgentRun(run) {
        const section = document.getElementById('detailAgentRunSection');
        const dl = document.getElementById('detailAgentRun');
        if (!section || !dl) return;
        if (!run) {
            section.hidden = true;
            dl.replaceChildren();
            return;
        }
        const rows = [
            ['Run', run.run_id + ' (' + run.mode + ', ' + run.family + ')'],
            ['Status', run.status + (run.check ? ', check ' + run.check : '')],
            ['Branch', run.branch || '(none: a scheduled run leaves no branch)'],
        ];
        if (run.branch) rows.push(['Inspect', 'git log --oneline ' + run.branch]);
        if (run.head_commit) rows.push(['Head', run.head_commit]);
        if (run.review_note) rows.push(['Review note', run.review_note]);
        if (run.review_reason) rows.push(['Review', run.review_reason]);
        if (run.claimed_at) rows.push(['Claimed', new Date(run.claimed_at).toLocaleString()]);
        if (run.reviewed_at) rows.push(['Reviewed', new Date(run.reviewed_at).toLocaleString()]);
        const nodes = [];
        rows.forEach(([label, value]) => {
            nodes.push(el('dt', null, label));
            const dd = el('dd');
            const code = el('code', label === 'Inspect' ? 'agent-run-copy' : null, value);
            if (label === 'Inspect') {
                code.title = 'click to copy';
                code.addEventListener('click', () => navigator.clipboard.writeText(value));
            }
            dd.appendChild(code);
            nodes.push(dd);
        });
        dl.replaceChildren(...nodes);
        section.hidden = false;
    }

    /** Called by App.openDetailModal: fetch the card's agent run and runbook. */
    async function showTaskExtras(task) {
        renderAgentRun(null);
        renderRunbookBlock(null);
        shownTask = null;
        if (!task || !task.id) return;
        shownTask = { id: task.id, runbook: '' };
        try {
            const resp = await fetch('/api/v1/tasks/' + encodeURIComponent(task.id));
            if (!resp.ok) return;
            const detail = await resp.json();
            if (!shownTask || shownTask.id !== task.id) return; // another card opened meanwhile
            shownTask.runbook = detail.runbook || '';
            renderAgentRun(detail.agent_run || null);
            renderRunbookBlock(shownTask.runbook, progress.get(shownTask.runbook) || detail.runbook_progress);
        } catch (error) {
            console.warn('[Runbooks] card details unavailable:', error);
        }
    }

    document.addEventListener('DOMContentLoaded', () => {
        const btn = document.getElementById('runbooksViewBtn');
        const view = document.getElementById('runbooksView');
        if (btn && view) {
            btn.addEventListener('click', () => {
                const visible = view.classList.toggle('visible');
                btn.textContent = visible ? 'Hide Runbooks' : 'Runbooks';
                if (visible) loadRunbooks();
            });
        }
        WebSocketClient.on('runbook.progress', onProgress);
    });

    return { showTaskExtras, loadRunbooks };
})();

window.RunbookView = RunbookView;
