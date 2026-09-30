// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.
//
// Compliance crosswalk page (ADR-097). Renders /api/v1/compliance/summary
// and, on demand, /api/v1/compliance/frameworks/{id}. All data is written
// with textContent; only http(s) evidence details become links.
'use strict';

(function () {
    const REQ_STATUSES = ['EVIDENCED', 'FAILING', 'INCOMPLETE', 'UNMAPPED'];

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

    function badge(status) {
        return el('span', { text: status, cls: 'badge s-' + status });
    }

    function safeLink(detail) {
        if (typeof detail === 'string' && /^https?:\/\//.test(detail)) {
            const a = el('a', { text: 'details' });
            a.href = detail;
            a.rel = 'noopener noreferrer';
            a.target = '_blank';
            return a;
        }
        return detail ? el('span', { text: detail, cls: 'muted' }) : null;
    }

    function age(iso) {
        const t = Date.parse(iso);
        if (isNaN(t)) return '';
        const h = (Date.now() - t) / 3.6e6;
        if (h < 1) return Math.max(1, Math.round(h * 60)) + 'm ago';
        if (h < 48) return Math.round(h) + 'h ago';
        return Math.round(h / 24) + 'd ago';
    }

    function renderFrameworks(fws) {
        const body = document.querySelector('#fw-table tbody');
        body.textContent = '';
        fws.forEach(fw => {
            const bar = el('div', { cls: 'bar' });
            ['EVIDENCED', 'FAILING', 'INCOMPLETE'].forEach(s => {
                const n = fw.counts[s] || 0;
                if (n > 0 && fw.total > 0) {
                    const seg = el('span', { cls: 'b-' + s, title: n + ' ' + s.toLowerCase() });
                    seg.style.width = Math.max(1, (100 * n) / fw.total) + '%';
                    bar.appendChild(seg);
                }
            });
            const name = el('td', null, [
                el('div', { text: fw.name + ' ' + fw.version }),
                fw.derived_from ? el('div', { text: 'mappings from ' + fw.derived_from, cls: 'muted' }) : null,
            ]);
            const row = el('tr', { cls: 'fw-row', title: 'show every requirement' }, [
                name,
                el('td', { text: fw.granularity, cls: 'muted' }),
                el('td', null, [bar]),
                el('td', { text: fw.counts.EVIDENCED || 0, cls: 'num s-EVIDENCED' }),
                el('td', { text: fw.counts.FAILING || 0, cls: 'num s-FAILING' }),
                el('td', { text: fw.counts.INCOMPLETE || 0, cls: 'num s-INCOMPLETE' }),
                el('td', { text: fw.counts.UNMAPPED || 0, cls: 'num s-UNMAPPED' }),
                el('td', { text: fw.total, cls: 'num' }),
            ]);
            row.addEventListener('click', () => showFramework(fw.id));
            body.appendChild(row);
        });
    }

    function renderMatrix(controls, fws) {
        const head = document.querySelector('#matrix thead');
        const body = document.querySelector('#matrix tbody');
        head.textContent = '';
        body.textContent = '';
        head.appendChild(el('tr', null, [el('th', { text: 'Control' })].concat(
            fws.map(fw => el('th', { text: fw.id, cls: 'rot', title: fw.name + ' ' + fw.version })))));
        controls.forEach(c => {
            const cells = fws.map(fw => {
                const n = ((c.mappings || {})[fw.id] || []).length;
                return el('td', { text: n || '·', cls: 'cell' + (n ? '' : ' zero'), title: ((c.mappings || {})[fw.id] || []).join(', ') });
            });
            body.appendChild(el('tr', null, [el('td', { text: c.id, cls: 'mono', title: c.title })].concat(cells)));
        });
    }

    function renderControls(controls) {
        const body = document.querySelector('#ctl-table tbody');
        body.textContent = '';
        controls.forEach(c => {
            const ev = el('td');
            (c.evidence || []).forEach(e => {
                const line = el('span', { cls: 'ev' });
                line.appendChild(el('span', { text: e.kind + ' ', cls: 'muted' }));
                if (e.kind === 'attestation') {
                    // A signed human statement, not machine evidence: say so.
                    line.appendChild(el('span', { text: 'self-attested ', cls: 'badge s-STALE', title: 'A signed, expiring statement in compliance/attestations/, not an automated check' }));
                }
                line.appendChild(el('span', { text: e.ref + ' ' }));
                if (e.latest) {
                    line.appendChild(el('span', { text: e.latest.verdict.toUpperCase() + ' ', cls: e.latest.verdict === 'pass' ? 's-PASS' : 's-FAIL' }));
                    line.appendChild(el('span', { text: age(e.latest.observed_at) + ' ', cls: 'muted', title: e.latest.observed_at + (e.latest.commit ? ' @ ' + e.latest.commit : '') }));
                    const link = safeLink(e.latest.detail);
                    if (link) line.appendChild(link);
                } else {
                    line.appendChild(el('span', { text: 'no evidence', cls: 's-NOT_ASSESSED' }));
                }
                ev.appendChild(line);
            });
            body.appendChild(el('tr', null, [
                el('td', null, [el('div', { text: c.id, cls: 'mono' }), el('div', { text: c.title }), el('div', { text: c.statement, cls: 'muted' })]),
                el('td', null, [badge(c.status)]),
                ev,
                el('td', { text: c.rationale || '', cls: 'muted' }),
            ]));
        });
    }

    function renderFindings(controls) {
        const body = document.querySelector('#find-table tbody');
        body.textContent = '';
        let n = 0;
        controls.forEach(c => (c.evidence || []).forEach(e => {
            if (!e.latest || e.latest.verdict !== 'fail') return;
            n++;
            const detail = el('td');
            detail.appendChild(el('span', { text: e.latest.detail || '' }));
            body.appendChild(el('tr', null, [
                el('td', { text: c.id, cls: 'mono', title: c.title }),
                el('td', { text: e.kind + ' ' + e.ref, cls: 'mono' }),
                el('td', { text: age(e.latest.observed_at), cls: 'muted', title: e.latest.observed_at }),
                detail,
            ]));
        }));
        document.getElementById('findings-title').textContent =
            n === 0 ? 'Open findings: none among assessed checks' : 'Open findings (' + n + ' failing checks)';
    }

    let currentFramework = null;
    let currentFilter = null;

    function renderRequirements() {
        const body = document.querySelector('#req-table tbody');
        body.textContent = '';
        currentFramework.requirements
            .filter(r => !currentFilter || r.status === currentFilter)
            .forEach(r => body.appendChild(el('tr', null, [
                el('td', { text: r.id, cls: 'mono' }),
                el('td', { text: r.title || '', cls: 'muted' }),
                el('td', null, [badge(r.status)]),
                el('td', { text: (r.controls || []).join(', '), cls: 'mono' }),
            ])));
    }

    function renderFilters() {
        const box = document.getElementById('fw-filters');
        box.textContent = '';
        [null].concat(REQ_STATUSES).forEach(s => {
            const n = s ? (currentFramework.counts[s] || 0) : currentFramework.total;
            const b = el('button', { text: (s || 'ALL') + ' ' + n, cls: currentFilter === s ? 'on' : '' });
            b.addEventListener('click', () => { currentFilter = s; renderFilters(); renderRequirements(); });
            box.appendChild(b);
        });
    }

    async function showFramework(id) {
        const res = await fetch('/api/v1/compliance/frameworks/' + encodeURIComponent(id));
        if (!res.ok) return;
        currentFramework = await res.json();
        currentFilter = null;
        document.getElementById('fw-detail').style.display = 'block';
        document.getElementById('fw-csv').href = '/api/v1/compliance/frameworks/' + encodeURIComponent(id) + '?format=csv';
        document.getElementById('fw-detail-title').textContent =
            currentFramework.name + ' ' + currentFramework.version + ' (' + currentFramework.granularity + ')';
        renderFilters();
        renderRequirements();
        document.getElementById('fw-detail').scrollIntoView({ behavior: 'smooth' });
    }

    async function load() {
        const status = document.getElementById('cx-status');
        let res;
        try {
            res = await fetch('/api/v1/compliance/summary');
        } catch (_e) {
            status.textContent = 'unreachable';
            return;
        }
        if (!res.ok) {
            status.textContent = 'unavailable (' + res.status + ')';
            return;
        }
        const s = await res.json();
        status.textContent = s.evidence_available
            ? s.evidence_records + ' evidence records'
            : 'no evidence collected yet: every control NOT_ASSESSED';
        renderFindings(s.controls || []);
        renderFrameworks(s.frameworks || []);
        renderMatrix(s.controls || [], s.frameworks || []);
        renderControls(s.controls || []);
    }

    load();
    setInterval(load, 60000);
})();
