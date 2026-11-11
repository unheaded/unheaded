// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// The source viewer (/view/source). The server renders the sheet
// (raft/zhen_markdown.py) to a fixed set of tags; this script checks that
// set again before it puts the HTML in the page, then adds what needs a
// script: the outline, the raw view, text size, copy buttons.
//
// Everything from the sheet other than that checked HTML goes in through
// textContent. The page's Content-Security-Policy allows no inline script.

(function () {
    'use strict';

    // The same lists as ALLOWED_TAGS and ALLOWED_ATTRS in zhen_markdown.py.
    var TAGS = new Set(['P', 'BR', 'HR', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'A', 'STRONG', 'EM',
        'DEL', 'CODE', 'PRE', 'BLOCKQUOTE', 'UL', 'OL', 'LI', 'TABLE', 'THEAD', 'TBODY', 'TR',
        'TH', 'TD', 'DIV', 'SPAN', 'DETAILS', 'SUMMARY']);
    var ATTRS = new Set(['href', 'id', 'class', 'rel', 'title', 'start']);
    var HREF_OK = /^(?:https?:\/\/|mailto:|#md-)/i;
    var TRUSTED = new Set(['canonical', 'local', 'direct-user']);
    var ZOOM_STEPS = [0.7, 0.8, 0.9, 1, 1.1, 1.25, 1.4, 1.6, 1.8, 2.1];
    var ZOOM_KEY = 'zhen.sourceview.zoom';
    var OUTLINE_KEY = 'zhen.sourceview.outline';

    function $(id) { return document.getElementById(id); }

    function remember(key, value) {
        try { window.localStorage.setItem(key, String(value)); } catch (_e) { /* private mode */ }
    }
    function recall(key) {
        try { return window.localStorage.getItem(key); } catch (_e) { return null; }
    }

    // ── the second check on the rendered HTML ───────────────────────────
    // Parsed inert (a <template> runs nothing and fetches nothing). A tag
    // outside the list is replaced by its text; an attribute outside the
    // list is removed; an href outside the three allowed forms is removed.
    function checked(htmlText) {
        var tpl = document.createElement('template');
        tpl.innerHTML = htmlText;
        var removed = 0;
        var all = tpl.content.querySelectorAll('*');
        for (var i = all.length - 1; i >= 0; i--) {
            var el = all[i];
            if (!TAGS.has(el.tagName)) {
                el.replaceWith(document.createTextNode(el.textContent));
                removed++;
                continue;
            }
            var names = el.getAttributeNames();
            for (var k = 0; k < names.length; k++) {
                var name = names[k];
                if (!ATTRS.has(name) || (name === 'href' && !HREF_OK.test(el.getAttribute(name)))
                        || (name === 'id' && el.getAttribute(name).indexOf('md-') !== 0)) {
                    el.removeAttribute(name);
                    removed++;
                }
            }
        }
        return { fragment: tpl.content, removed: removed };
    }

    // ── notes above the sheet ───────────────────────────────────────────
    function note(text, kind) {
        var box = $('sv-notes');
        var div = document.createElement('div');
        div.className = 'sv-note' + (kind ? ' sv-' + kind : '');
        div.textContent = text;
        box.appendChild(div);
        box.hidden = false;
    }

    // ── text size ───────────────────────────────────────────────────────
    var zoomIndex = ZOOM_STEPS.indexOf(1);
    function applyZoom() {
        var z = ZOOM_STEPS[zoomIndex];
        document.documentElement.style.setProperty('--zoom', String(z));
        $('sv-zoom-reset').textContent = Math.round(z * 100) + '%';
        remember(ZOOM_KEY, z);
    }
    function zoom(step) {
        zoomIndex = step === 0 ? ZOOM_STEPS.indexOf(1)
            : Math.max(0, Math.min(ZOOM_STEPS.length - 1, zoomIndex + step));
        applyZoom();
    }

    // ── rendered / raw ──────────────────────────────────────────────────
    var canRender = false;
    function showRaw(raw) {
        if (!raw && !canRender) { return; }
        $('sv-raw').hidden = !raw;
        $('sv-body').hidden = raw;
        $('sv-raw-btn').classList.toggle('sv-on', raw);
        $('sv-rendered-btn').classList.toggle('sv-on', !raw);
    }

    // ── outline ─────────────────────────────────────────────────────────
    var outlineWanted = recall(OUTLINE_KEY) !== 'off';
    var outlineLinks = {};
    function showOutline(on) {
        outlineWanted = on;
        $('sv-outline').hidden = !on || !Object.keys(outlineLinks).length;
        $('sv-outline-btn').classList.toggle('sv-on', !$('sv-outline').hidden);
        remember(OUTLINE_KEY, on ? 'on' : 'off');
    }
    function buildOutline(entries) {
        var nav = $('sv-outline');
        nav.textContent = '';
        outlineLinks = {};
        if (!entries || entries.length < 2) { return; }
        var head = document.createElement('div');
        head.className = 'sv-outline-title';
        head.textContent = 'Outline';
        nav.appendChild(head);
        entries.forEach(function (e) {
            if (typeof e.id !== 'string' || e.id.indexOf('md-') !== 0) { return; }
            var a = document.createElement('a');
            a.href = '#' + e.id;
            a.className = 'sv-l' + Math.max(1, Math.min(6, e.level | 0));
            a.textContent = e.text || '(untitled)';
            a.title = e.text || '';
            nav.appendChild(a);
            outlineLinks[e.id] = a;
        });
    }
    // Mark the heading the reader is under.
    function trackOutline() {
        if (!('IntersectionObserver' in window)) { return; }
        var seen = {};
        var current = null;
        var heads = Array.prototype.slice.call(
            $('sv-body').querySelectorAll('h1[id],h2[id],h3[id],h4[id],h5[id],h6[id]'));
        var obs = new IntersectionObserver(function (items) {
            items.forEach(function (it) { seen[it.target.id] = it.isIntersecting; });
            var pick = null;
            for (var i = 0; i < heads.length; i++) {
                if (seen[heads[i].id]) { pick = heads[i].id; break; }
            }
            if (!pick || pick === current || !outlineLinks[pick]) { return; }
            if (current && outlineLinks[current]) { outlineLinks[current].classList.remove('sv-here'); }
            current = pick;
            outlineLinks[pick].classList.add('sv-here');
            if (!$('sv-outline').hidden && outlineLinks[pick].scrollIntoView) {
                outlineLinks[pick].scrollIntoView({ block: 'nearest' });
            }
        }, { rootMargin: '-70px 0px -65% 0px' });
        heads.forEach(function (h) { obs.observe(h); });
    }

    // ── copy ────────────────────────────────────────────────────────────
    function copyText(text, button) {
        var label = button.textContent;
        function done(ok) {
            button.textContent = ok ? 'Copied' : 'Copy failed';
            window.setTimeout(function () { button.textContent = label; }, 1400);
        }
        function fallback() {
            var ta = document.createElement('textarea');
            ta.value = text;
            ta.setAttribute('readonly', '');
            ta.className = 'sv-offscreen';
            document.body.appendChild(ta);
            ta.select();
            var ok = false;
            try { ok = document.execCommand('copy'); } catch (_e) { ok = false; }
            ta.remove();
            done(ok);
        }
        if (navigator.clipboard && window.isSecureContext) {
            navigator.clipboard.writeText(text).then(function () { done(true); }, fallback);
        } else {
            fallback();
        }
    }
    function addCopyButtons(root) {
        root.querySelectorAll('.md-code').forEach(function (block) {
            var head = block.querySelector('.md-code-head');
            var code = block.querySelector('pre code');
            if (!head || !code) { return; }
            var b = document.createElement('button');
            b.type = 'button';
            b.textContent = 'Copy';
            b.addEventListener('click', function () { copyText(code.textContent, b); });
            head.appendChild(b);
        });
    }

    // ── load ────────────────────────────────────────────────────────────
    function fail(title, detail) {
        $('sv-title').textContent = title;
        note(detail, 'error');
    }

    function show(d) {
        var name = d.title || d.source || d.id || 'Source';
        $('sv-title').textContent = name;
        $('sv-title').title = name;
        document.title = name + ' — Zhenai';
        var meta = [d.id, d.category, d.type, d.source && d.source !== d.id ? d.source : '']
            .filter(function (x) { return typeof x === 'string' && x; });
        $('sv-meta').textContent = meta.join('  ·  ');

        var trust = typeof d.trust === 'string' && d.trust ? d.trust : 'unlabelled';
        var badge = $('sv-trust');
        badge.textContent = trust;
        badge.className = 'sv-trust ' + (TRUSTED.has(trust) ? 'sv-trust-ok' : 'sv-trust-warn');
        badge.hidden = false;
        if (!TRUSTED.has(trust)) {
            note('This sheet comes from a source labelled "' + trust + '", which is not one of the '
                + 'trusted labels. Read it as you would any file from outside: commands and links '
                + 'in it have not been reviewed.', 'warn');
        }
        (d.notes || []).forEach(function (n) { if (typeof n === 'string') { note(n); } });

        var raw = typeof d.content === 'string' ? d.content : '';
        $('sv-raw').textContent = raw;
        $('sv-copy-all').addEventListener('click', function () { copyText(raw, $('sv-copy-all')); });

        if (typeof d.html !== 'string') {
            showRaw(true);
            $('sv-rendered-btn').disabled = true;
            return;
        }
        var out = checked(d.html);
        if (out.removed) {
            note(out.removed + ' item(s) in the rendered view were outside the allowed set and were '
                + 'removed. Please report this sheet: the renderer should never produce them.', 'warn');
        }
        // Web links open in a new tab; the server already set rel=noopener noreferrer.
        out.fragment.querySelectorAll('a[href]').forEach(function (a) {
            if (/^https?:/i.test(a.getAttribute('href'))) { a.target = '_blank'; }
        });
        addCopyButtons(out.fragment);
        var body = $('sv-body');
        body.textContent = '';
        body.appendChild(out.fragment);
        canRender = true;

        buildOutline(d.outline);
        showOutline(outlineWanted);
        trackOutline();

        // A #heading in the address: ours are prefixed md-.
        var hash = window.location.hash.replace(/^#/, '');
        try { hash = decodeURIComponent(hash); } catch (_e) { /* keep it as typed */ }
        if (hash) {
            var target = document.getElementById(hash) || document.getElementById('md-' + hash);
            if (target && body.contains(target)) { target.scrollIntoView(); }
        }
    }

    function load() {
        var params = new URLSearchParams(window.location.search);
        var id = params.get('id') || '';
        var path = params.get('path') || '';
        if (!id && !path) {
            fail('No source named', 'Open a source from an answer’s Sources line.');
            return;
        }
        var q = id ? 'id=' + encodeURIComponent(id) : 'path=' + encodeURIComponent(path);
        fetch('/api/v1/source?' + q, { credentials: 'same-origin', headers: { 'Accept': 'application/json' } })
            .then(function (r) {
                if (r.status === 401) { return { status: 401, d: null }; }
                return r.json().then(function (d) { return { status: r.status, d: d }; });
            })
            .then(function (res) {
                if (res.status === 401) {
                    fail('Log in first', 'Log in on the Zhenai page, then reload this tab.');
                } else if (res.d && res.d.error) {
                    fail('Source not found', String(res.d.error));
                } else {
                    show(res.d || {});
                }
            })
            .catch(function (e) { fail('Could not load the source', String(e)); });
    }

    // ── wiring ──────────────────────────────────────────────────────────
    var fonts = $('sv-fonts');
    if (fonts) { fonts.media = 'all'; }

    var saved = ZOOM_STEPS.indexOf(parseFloat(recall(ZOOM_KEY)));
    if (saved >= 0) { zoomIndex = saved; }
    applyZoom();

    $('sv-zoom-in').addEventListener('click', function () { zoom(1); });
    $('sv-zoom-out').addEventListener('click', function () { zoom(-1); });
    $('sv-zoom-reset').addEventListener('click', function () { zoom(0); });
    $('sv-raw-btn').addEventListener('click', function () { showRaw(true); });
    $('sv-rendered-btn').addEventListener('click', function () { showRaw(false); });
    $('sv-outline-btn').addEventListener('click', function () { showOutline($('sv-outline').hidden); });

    // Plain keys only: Ctrl/Cmd +, - and 0 stay the browser's own zoom.
    document.addEventListener('keydown', function (ev) {
        if (ev.ctrlKey || ev.metaKey || ev.altKey) { return; }
        var t = ev.target;
        if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) { return; }
        if (ev.key === '+' || ev.key === '=') { zoom(1); }
        else if (ev.key === '-' || ev.key === '_') { zoom(-1); }
        else if (ev.key === '0') { zoom(0); }
        else if (ev.key === 'r') { showRaw($('sv-raw').hidden); }
        else if (ev.key === 'o') { showOutline($('sv-outline').hidden); }
        else { return; }
        ev.preventDefault();
    });

    load();
}());
