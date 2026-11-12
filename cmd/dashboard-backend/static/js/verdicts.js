// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//
// Third-party verdicts page (ADR-113, read-only first step). Renders
// /api/v1/verdicts: the dated third-party inventory, one table per section,
// with counts by verdict, a verdict filter and a text filter. Everything is
// written with textContent; nothing from the inventory is read as markup.
// The page never writes: a verdict is given in the record, in git.
'use strict';

(function () {
    const CLASSES = ['undecided', 'keep', 'replace', 'cut', 'other'];
    let inventory = null;
    let filter = 'all';
    let query = '';
    let shown = false;

    function el(tag, opts, children) {
        const e = document.createElement(tag);
        if (opts) {
            if (opts.text !== undefined) e.textContent = String(opts.text);
            if (opts.cls) e.className = opts.cls;
            if (opts.title) e.title = opts.title;
        }
        (children || []).forEach(c => { if (c) e.appendChild(c); });
        return e;
    }

    function clear(node) {
        while (node.firstChild) node.removeChild(node.firstChild);
    }

    function klass(item) {
        return CLASSES.indexOf(item.class) >= 0 ? item.class : 'other';
    }

    // An item is shown when its verdict passes the filter and every word of
    // the query is somewhere in its name, its verdict or one of its fields.
    function matches(item) {
        if (filter !== 'all' && klass(item) !== filter) return false;
        if (!query) return true;
        const hay = [item.name, item.verdict].concat((item.fields || []).map(f => f.value)).join('\n').toLowerCase();
        return query.split(/\s+/).every(w => hay.indexOf(w) >= 0);
    }

    function renderCounts(counts) {
        const total = counts.total || 0;
        document.getElementById('vd-n-total').textContent = String(total);
        const bar = document.getElementById('vd-bar');
        clear(bar);
        CLASSES.forEach(c => {
            const n = counts[c] || 0;
            document.getElementById('vd-n-' + c).textContent = String(n);
            // Undecided is the bar's own background: only rulings are drawn.
            if (c !== 'undecided' && n > 0 && total > 0) {
                const seg = el('span', { cls: 'b-' + c, title: n + ' ' + c });
                seg.style.width = Math.max(1, (100 * n) / total) + '%';
                bar.appendChild(seg);
            }
        });
    }

    function renderSections() {
        const host = document.getElementById('vd-sections');
        clear(host);
        let visible = 0;
        (inventory.sections || []).forEach(sec => {
            const items = (sec.items || []).filter(matches);
            if (!items.length) return;
            visible += items.length;
            const cols = sec.columns || [];
            const head = el('tr', null, [el('th', { text: 'Item' })]
                .concat(cols.map(c => el('th', { text: c })))
                .concat([el('th', { text: 'Verdict' })]));
            const body = el('tbody');
            items.forEach(it => {
                const byCol = {};
                (it.fields || []).forEach(f => { byCol[f.column] = f.value; });
                const c = klass(it);
                const verdict = el('td', null, [el('span', { text: it.verdict || 'undecided', cls: 'badge c-' + c })]);
                body.appendChild(el('tr', null, [el('td', { text: it.name, cls: 'name' })]
                    .concat(cols.map(col => el('td', { text: byCol[col] || '', cls: byCol[col] ? '' : 'muted' })))
                    .concat([verdict])));
            });
            const all = (sec.items || []).length;
            const title = el('h2', { text: sec.name }, []);
            title.appendChild(el('span', { text: items.length === all ? String(all) : items.length + ' of ' + all, cls: 'vd-count' }));
            host.appendChild(el('section', { cls: 'vd-card' }, [title,
                el('table', { cls: 'vd' }, [el('thead', null, [head]), body])]));
        });
        const total = (inventory.counts && inventory.counts.total) || 0;
        document.getElementById('vd-shown').textContent = visible === total
            ? total + ' shown'
            : visible + ' of ' + total + ' shown';
    }

    function setFilter(f) {
        filter = f;
        ['all'].concat(CLASSES).forEach(c => {
            document.getElementById('vd-f-' + c).className = c === f ? 'on' : '';
        });
        if (inventory) renderSections();
    }

    function waiting(text, failed) {
        if (shown) return;
        document.getElementById('vd-wait').classList.toggle('vd-failed', failed);
        document.getElementById('vd-wait-text').textContent = text;
    }

    function reveal() {
        shown = true;
        document.getElementById('vd-page').classList.remove('vd-loading');
    }

    async function load() {
        const status = document.getElementById('vd-status');
        waiting('Loading the inventory', false);
        let res;
        try {
            res = await fetch('/api/v1/verdicts');
        } catch (_e) {
            status.textContent = 'unreachable';
            waiting('The inventory is unreachable. Reload to try again.', true);
            return;
        }
        if (!res.ok) {
            status.textContent = 'unavailable (' + res.status + ')';
            waiting(res.status === 401
                ? 'Log in to see the inventory.'
                : 'The inventory is unavailable (' + res.status + '). Reload to try again.', true);
            return;
        }
        let inv;
        try {
            inv = await res.json();
        } catch (_e) {
            status.textContent = 'unreadable';
            waiting('The inventory is unreadable. Reload to try again.', true);
            return;
        }
        if (!inv || !Array.isArray(inv.sections) || !inv.counts) {
            status.textContent = 'unreadable';
            waiting('The inventory is unreadable. Reload to try again.', true);
            return;
        }
        inventory = inv;
        renderCounts(inv.counts);
        renderSections();
        document.getElementById('vd-source').textContent = inv.source || '';
        const decided = (inv.counts.total || 0) - (inv.counts.undecided || 0);
        status.textContent = decided + ' of ' + (inv.counts.total || 0) + ' ruled on';
        reveal();
    }

    ['all'].concat(CLASSES).forEach(c => {
        document.getElementById('vd-f-' + c).onclick = function () { setFilter(c); };
    });
    const q = document.getElementById('vd-q');
    q.oninput = function () {
        query = String(q.value || '').trim().toLowerCase();
        if (inventory) renderSections();
    };

    load();
})();
