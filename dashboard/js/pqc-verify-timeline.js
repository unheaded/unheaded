/**
 * pqc-verify-timeline.js - PQC Verification Success/Fail Timeline
 * Area chart showing verifications per second over 1 hour.
 *
 * Features:
 *   - Dual area chart: green (success) / red (fail) stacked
 *   - Tooltip showing exact counts on hover
 *   - Auto-scrolling real-time display
 *   - Stats banner: total verifications, pass rate %
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var PqcVerifyTimeline = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        historySeconds: 3600,
        buckets: 120,           // 120 buckets (30s each)
        padding: { top: 20, right: 16, bottom: 30, left: 55 },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            successArea: 'rgba(0, 210, 106, 0.35)',
            successLine: '#00d26a',
            failArea: 'rgba(255, 71, 87, 0.35)',
            failLine: '#ff4757',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            tooltip: 'rgba(26, 26, 46, 0.95)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvas: null,
        ctx: null,
        data: [],               // { timestamp, success, fail }
        stats: { total: 0, passRate: 0 },
        mouse: { x: -1, y: -1, inside: false },
        animationFrame: null,
        resizeHandler: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[PqcVerifyTimeline] Container not found'); return false; }

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
        console.log('[PqcVerifyTimeline] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildStatsBar(parent) {
        var bar = document.createElement('div');
        bar.id = 'pqc-vt-stats-bar';
        bar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        bar.innerHTML =
            '<span>Total Verifications: <strong id="pqc-vt-total">--</strong></span>' +
            '<span>Pass Rate: <strong id="pqc-vt-pass" style="color:#00d26a">--%</strong></span>' +
            '<span>Current Rate: <strong id="pqc-vt-rate" style="color:' + CONFIG.colors.accent + '">-- /s</strong></span>';
        parent.appendChild(bar);
    }

    function buildChart(parent) {
        var section = createSection('PQC Verification Timeline (1 Hour)', 'pqc-vt-chart');
        state.canvas = document.createElement('canvas');
        state.canvas.style.cssText = 'display:block;width:100%;cursor:crosshair;';
        section.querySelector('.pqc-section-body').appendChild(state.canvas);
        state.ctx = state.canvas.getContext('2d');
        parent.appendChild(section);

        state.canvas.addEventListener('mousemove', function(e) {
            var rect = state.canvas.getBoundingClientRect();
            state.mouse.x = e.clientX - rect.left;
            state.mouse.y = e.clientY - rect.top;
            state.mouse.inside = true;
        });
        state.canvas.addEventListener('mouseleave', function() {
            state.mouse.inside = false;
        });
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
        var h = Math.max(260, 200);
        state.canvas.width = w * dpr;
        state.canvas.height = h * dpr;
        state.canvas.style.width = w + 'px';
        state.canvas.style.height = h + 'px';
        state.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(verifyData) {
        if (!state.initialized || !verifyData) return;

        if (verifyData.success != null || verifyData.fail != null) {
            state.data.push({
                timestamp: verifyData.timestamp || Date.now(),
                success: verifyData.success || 0,
                fail: verifyData.fail || 0
            });
            if (state.data.length > CONFIG.buckets) state.data.shift();
        }

        if (verifyData.stats) {
            state.stats = verifyData.stats;
            updateStats();
        }
    }

    function updateStats() {
        var s = state.stats;
        setTextById('pqc-vt-total', formatNumber(s.total || 0));
        setTextById('pqc-vt-pass', (s.passRate || 0).toFixed(1) + '%');
        var latest = state.data.length > 0 ? state.data[state.data.length - 1] : null;
        var rate = latest ? (latest.success + latest.fail) : 0;
        setTextById('pqc-vt-rate', formatNumber(rate) + ' /s');
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

        var data = state.data;
        if (data.length < 2) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for verification data...', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        // Compute yMax from combined success+fail
        var yMax = 0;
        for (var i = 0; i < data.length; i++) {
            var combined = data[i].success + data[i].fail;
            if (combined > yMax) yMax = combined;
        }
        yMax = Math.ceil(yMax * 1.2) || 100;

        // Grid lines
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var g = 1; g < 5; g++) {
            var gy = pad.top + plotH * (1 - g / 5);
            ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + plotW, gy); ctx.stroke();
        }

        // Draw fail area (on top, stacked above success)
        drawArea(ctx, data, pad, plotW, plotH, yMax, function(d) { return d.success + d.fail; },
            CONFIG.colors.failArea, CONFIG.colors.failLine);

        // Draw success area (bottom)
        drawArea(ctx, data, pad, plotW, plotH, yMax, function(d) { return d.success; },
            CONFIG.colors.successArea, CONFIG.colors.successLine);

        // Y-axis labels
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var a = 0; a <= 5; a++) {
            var aVal = (yMax * a / 5).toFixed(0);
            var ay = pad.top + plotH * (1 - a / 5);
            ctx.fillText(aVal, pad.left - 4, ay + 4);
        }

        // X-axis label
        ctx.textAlign = 'center';
        ctx.fillText('verifications / sec', pad.left + plotW / 2, pad.top + plotH + 22);

        // Legend
        ctx.font = '10px "SF Mono", monospace';
        var lx = pad.left + plotW - 140;
        ctx.fillStyle = CONFIG.colors.successLine;
        ctx.fillRect(lx, pad.top + 4, 10, 10);
        ctx.fillStyle = CONFIG.colors.text;
        ctx.textAlign = 'left';
        ctx.fillText('Success', lx + 14, pad.top + 13);

        ctx.fillStyle = CONFIG.colors.failLine;
        ctx.fillRect(lx + 76, pad.top + 4, 10, 10);
        ctx.fillStyle = CONFIG.colors.text;
        ctx.fillText('Fail', lx + 90, pad.top + 13);

        // Tooltip
        if (state.mouse.inside) {
            drawTooltip(ctx, data, pad, plotW, plotH, yMax, w, h);
        }
    }

    function drawArea(ctx, data, pad, plotW, plotH, yMax, valueFn, fillColor, lineColor) {
        ctx.fillStyle = fillColor;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top + plotH);
        for (var i = 0; i < data.length; i++) {
            var x = pad.left + (i / (data.length - 1)) * plotW;
            var y = pad.top + plotH - (valueFn(data[i]) / yMax) * plotH;
            ctx.lineTo(x, y);
        }
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.closePath();
        ctx.fill();

        ctx.strokeStyle = lineColor;
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var lx = pad.left + (j / (data.length - 1)) * plotW;
            var ly = pad.top + plotH - (valueFn(data[j]) / yMax) * plotH;
            if (j === 0) ctx.moveTo(lx, ly); else ctx.lineTo(lx, ly);
        }
        ctx.stroke();
    }

    function drawTooltip(ctx, data, pad, plotW, plotH, yMax, w, _h) {
        var mx = state.mouse.x;
        if (mx < pad.left || mx > pad.left + plotW) return;

        var idx = Math.round(((mx - pad.left) / plotW) * (data.length - 1));
        if (idx < 0 || idx >= data.length) return;
        var d = data[idx];
        var x = pad.left + (idx / (data.length - 1)) * plotW;

        // Vertical crosshair
        ctx.strokeStyle = CONFIG.colors.accent;
        ctx.lineWidth = 1;
        ctx.setLineDash([3, 3]);
        ctx.beginPath(); ctx.moveTo(x, pad.top); ctx.lineTo(x, pad.top + plotH); ctx.stroke();
        ctx.setLineDash([]);

        // Tooltip box
        var tw = 150, th = 50;
        var tx = Math.min(x + 10, w - tw - 10);
        var ty = Math.max(state.mouse.y - th - 10, pad.top);

        ctx.fillStyle = CONFIG.colors.tooltip;
        ctx.strokeStyle = CONFIG.colors.border;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.roundRect(tx, ty, tw, th, 4);
        ctx.fill();
        ctx.stroke();

        ctx.font = '10px "SF Mono", monospace';
        ctx.textAlign = 'left';
        ctx.fillStyle = CONFIG.colors.successLine;
        ctx.fillText('Pass: ' + formatNumber(d.success), tx + 8, ty + 18);
        ctx.fillStyle = CONFIG.colors.failLine;
        ctx.fillText('Fail: ' + formatNumber(d.fail), tx + 8, ty + 34);
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText('Total: ' + formatNumber(d.success + d.fail), tx + 80, ty + 18);
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
    function formatNumber(n) {
        if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
        if (n >= 1000) return (n / 1000).toFixed(1) + 'K';
        return String(n);
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
        state.data = [];
        state.stats = { total: 0, passRate: 0 };
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

window.PqcVerifyTimeline = PqcVerifyTimeline;
console.log('pqc-verify-timeline.js loaded');
