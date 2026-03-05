/**
 * pqc-key-rotation.js - PQC Key Rotation Schedule
 * Timeline visualization of key lifecycle: creation, rotation, and expiration.
 *
 * Features:
 *   - Horizontal timeline showing key creation, rotation, and expiration events
 *   - Grace period visualization (overlap bar between old and new keys)
 *   - Next rotation countdown timer
 *   - Active / rotating / expired key counts in stats banner
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var PqcKeyRotation = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxEvents: 20,
        padding: { top: 20, right: 20, bottom: 30, left: 80 },
        rowHeight: 28,
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            active: '#00d26a',
            rotating: '#ffd700',
            expired: '#ff4757',
            grace: 'rgba(255, 215, 0, 0.2)',
            graceStroke: 'rgba(255, 215, 0, 0.5)',
            barBg: 'rgba(255, 255, 255, 0.05)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvas: null,
        ctx: null,
        keys: [],               // { id, algorithm, status, createdAt, rotatedAt, expiresAt, graceEnd }
        nextRotation: null,     // timestamp of next scheduled rotation
        stats: { active: 0, rotating: 0, expired: 0 },
        animationFrame: null,
        resizeHandler: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[PqcKeyRotation] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildStatsBar(container);
        buildChart(container);

        state.resizeHandler = debounce(resizeCanvas, 150);
        window.addEventListener('resize', state.resizeHandler);
        resizeCanvas();
        startAnimation();

        state.initialized = true;
        console.log('[PqcKeyRotation] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildStatsBar(parent) {
        var bar = document.createElement('div');
        bar.id = 'pqc-kr-stats-bar';
        bar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        bar.innerHTML =
            '<span>Active Keys: <strong id="pqc-kr-active" style="color:' + CONFIG.colors.active + '">--</strong></span>' +
            '<span>Rotating: <strong id="pqc-kr-rotating" style="color:' + CONFIG.colors.rotating + '">--</strong></span>' +
            '<span>Expired: <strong id="pqc-kr-expired" style="color:' + CONFIG.colors.expired + '">--</strong></span>' +
            '<span>Next Rotation: <strong id="pqc-kr-countdown" style="color:' + CONFIG.colors.accent + '">--:--:--</strong></span>';
        parent.appendChild(bar);
    }

    function buildChart(parent) {
        var section = createSection('Key Rotation Timeline', 'pqc-kr-chart');
        state.canvas = document.createElement('canvas');
        state.canvas.style.cssText = 'display:block;width:100%;';
        section.querySelector('.pqc-section-body').appendChild(state.canvas);
        state.ctx = state.canvas.getContext('2d');
        parent.appendChild(section);
    }

    function createSection(title, id) {
        var sec = document.createElement('div');
        sec.id = id;
        sec.style.cssText = 'border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;overflow:hidden;';

        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent + ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;';
        hdr.textContent = title;
        sec.appendChild(hdr);

        var body = document.createElement('div');
        body.className = 'pqc-section-body';
        body.style.cssText = 'padding:12px;background:' + CONFIG.colors.background + ';';
        sec.appendChild(body);

        return sec;
    }

    // ========================================================================
    // Canvas Management
    // ========================================================================
    function resizeCanvas() {
        if (!state.canvas || !state.canvas.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = state.canvas.parentElement.getBoundingClientRect();
        var w = rect.width || 400;
        var rows = Math.max(state.keys.length, 3);
        var h = Math.max(CONFIG.padding.top + rows * CONFIG.rowHeight + CONFIG.padding.bottom, 160);
        state.canvas.width = w * dpr;
        state.canvas.height = h * dpr;
        state.canvas.style.width = w + 'px';
        state.canvas.style.height = h + 'px';
        state.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(rotationData) {
        if (!state.initialized || !rotationData) return;

        if (rotationData.keys) {
            state.keys = rotationData.keys.slice(0, CONFIG.maxEvents);
            resizeCanvas();
        }

        if (rotationData.nextRotation != null) {
            state.nextRotation = rotationData.nextRotation;
        }

        if (rotationData.stats) {
            state.stats = rotationData.stats;
        }

        updateStatsBar();
    }

    function updateStatsBar() {
        var s = state.stats;
        setTextById('pqc-kr-active', String(s.active || 0));
        setTextById('pqc-kr-rotating', String(s.rotating || 0));
        setTextById('pqc-kr-expired', String(s.expired || 0));
    }

    // ========================================================================
    // Rendering
    // ========================================================================
    function render() {
        var ctx = state.ctx;
        if (!ctx || !state.canvas) return;
        var dpr = window.devicePixelRatio || 1;
        var w = state.canvas.width / dpr;
        var h = state.canvas.height / dpr;
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        // Update countdown
        updateCountdown();

        var keys = state.keys;
        if (keys.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No key rotation data', w / 2, h / 2);
            return;
        }

        // Compute time range across all keys
        var tMin = Infinity, tMax = -Infinity;
        for (var i = 0; i < keys.length; i++) {
            var k = keys[i];
            if (k.createdAt < tMin) tMin = k.createdAt;
            if (k.expiresAt > tMax) tMax = k.expiresAt;
            if (k.graceEnd && k.graceEnd > tMax) tMax = k.graceEnd;
        }
        var tRange = tMax - tMin || 1;

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        // Draw time grid lines
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        var gridCount = 6;
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'center';
        for (var g = 0; g <= gridCount; g++) {
            var gx = pad.left + (g / gridCount) * plotW;
            ctx.beginPath(); ctx.moveTo(gx, pad.top); ctx.lineTo(gx, pad.top + plotH); ctx.stroke();
            var gTime = tMin + (g / gridCount) * tRange;
            ctx.fillText(formatShortTime(gTime), gx, pad.top + plotH + 16);
        }

        // Draw key bars
        for (var j = 0; j < keys.length; j++) {
            var key = keys[j];
            var rowY = pad.top + j * CONFIG.rowHeight;
            var barH = CONFIG.rowHeight - 6;

            // Key label (left side)
            ctx.font = '9px "SF Mono", monospace';
            ctx.textAlign = 'right';
            ctx.fillStyle = CONFIG.colors.text;
            var label = key.id ? key.id.substring(0, 8) : 'key-' + j;
            ctx.fillText(label, pad.left - 8, rowY + barH / 2 + 3);

            // Background bar
            ctx.fillStyle = CONFIG.colors.barBg;
            ctx.fillRect(pad.left, rowY, plotW, barH);

            // Active lifetime bar
            var startX = pad.left + ((key.createdAt - tMin) / tRange) * plotW;
            var endX = pad.left + ((key.expiresAt - tMin) / tRange) * plotW;
            var barW = Math.max(endX - startX, 2);

            var statusColor = CONFIG.colors.active;
            if (key.status === 'rotating') statusColor = CONFIG.colors.rotating;
            else if (key.status === 'expired') statusColor = CONFIG.colors.expired;

            ctx.fillStyle = statusColor;
            ctx.globalAlpha = 0.6;
            ctx.fillRect(startX, rowY + 1, barW, barH - 2);
            ctx.globalAlpha = 1;

            // Grace period overlay
            if (key.rotatedAt && key.graceEnd) {
                var graceStartX = pad.left + ((key.rotatedAt - tMin) / tRange) * plotW;
                var graceEndX = pad.left + ((key.graceEnd - tMin) / tRange) * plotW;
                var graceW = Math.max(graceEndX - graceStartX, 2);

                ctx.fillStyle = CONFIG.colors.grace;
                ctx.fillRect(graceStartX, rowY + 1, graceW, barH - 2);

                // Dashed border for grace period
                ctx.strokeStyle = CONFIG.colors.graceStroke;
                ctx.lineWidth = 1;
                ctx.setLineDash([3, 3]);
                ctx.strokeRect(graceStartX, rowY + 1, graceW, barH - 2);
                ctx.setLineDash([]);
            }

            // Event markers
            drawEventDot(ctx, startX, rowY + barH / 2, CONFIG.colors.active, 'C');
            if (key.rotatedAt) {
                var rotX = pad.left + ((key.rotatedAt - tMin) / tRange) * plotW;
                drawEventDot(ctx, rotX, rowY + barH / 2, CONFIG.colors.rotating, 'R');
            }
            drawEventDot(ctx, endX, rowY + barH / 2, CONFIG.colors.expired, 'E');

            // Algorithm tag
            if (key.algorithm) {
                ctx.font = '7px "SF Mono", monospace';
                ctx.textAlign = 'left';
                ctx.fillStyle = CONFIG.colors.textMuted;
                ctx.fillText(key.algorithm, startX + 4, rowY + barH - 3);
            }
        }

        // Legend
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'left';
        var lx = pad.left;
        var ly = pad.top + keys.length * CONFIG.rowHeight + 26;
        if (ly < h - 6) {
            var items = [
                { color: CONFIG.colors.active,   label: 'C = Created' },
                { color: CONFIG.colors.rotating,  label: 'R = Rotated' },
                { color: CONFIG.colors.expired,   label: 'E = Expires' },
                { color: CONFIG.colors.graceStroke, label: '--- Grace Period' }
            ];
            for (var m = 0; m < items.length; m++) {
                ctx.fillStyle = items[m].color;
                ctx.fillRect(lx + m * 110, ly, 8, 8);
                ctx.fillStyle = CONFIG.colors.textMuted;
                ctx.fillText(items[m].label, lx + m * 110 + 12, ly + 8);
            }
        }
    }

    function drawEventDot(ctx, x, y, color, letter) {
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.arc(x, y, 5, 0, Math.PI * 2);
        ctx.fill();

        ctx.fillStyle = '#000';
        ctx.font = 'bold 7px "SF Mono", monospace';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(letter, x, y);
        ctx.textBaseline = 'alphabetic';
    }

    function updateCountdown() {
        if (!state.nextRotation) return;
        var remaining = state.nextRotation - Date.now();
        if (remaining < 0) remaining = 0;
        var hours = Math.floor(remaining / 3600000);
        var mins = Math.floor((remaining % 3600000) / 60000);
        var secs = Math.floor((remaining % 60000) / 1000);
        var str = pad2(hours) + ':' + pad2(mins) + ':' + pad2(secs);
        setTextById('pqc-kr-countdown', str);
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            render();
            state.animationFrame = requestAnimationFrame(tick);
        }
        state.animationFrame = requestAnimationFrame(tick);
    }

    function stopAnimation() {
        if (state.animationFrame) { cancelAnimationFrame(state.animationFrame); state.animationFrame = null; }
    }

    // ========================================================================
    // Utilities
    // ========================================================================
    function formatShortTime(ts) {
        var d = new Date(ts);
        return pad2(d.getHours()) + ':' + pad2(d.getMinutes());
    }

    function pad2(n) {
        return n < 10 ? '0' + n : String(n);
    }

    function setTextById(id, text) {
        var el = document.getElementById(id);
        if (el) el.textContent = text;
    }

    function debounce(fn, wait) {
        var t;
        return function() { var a = arguments, c = this; clearTimeout(t); t = setTimeout(function() { fn.apply(c, a); }, wait); };
    }

    // ========================================================================
    // Cleanup
    // ========================================================================
    function destroy() {
        stopAnimation();
        if (state.resizeHandler) window.removeEventListener('resize', state.resizeHandler);
        if (state.container) state.container.innerHTML = '';
        state.keys = [];
        state.nextRotation = null;
        state.stats = { active: 0, rotating: 0, expired: 0 };
        state.initialized = false;
    }

    // ========================================================================
    // Public API
    // ========================================================================
    return {
        init: init,
        update: update,
        destroy: destroy,
        isInitialized: function() { return state.initialized; }
    };
})();

window.PqcKeyRotation = PqcKeyRotation;
console.log('pqc-key-rotation.js loaded');
