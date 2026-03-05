/**
 * pqc-map-utilization.js - Sophia BPF Map Utilization Gauges
 * Five gauge meters for sig_map, key_map, policy_map, sovereign_map, kem_map.
 *
 * Features:
 *   - Arc gauge meters with percentage fill
 *   - Color transitions: green (0-70%) to yellow (70-90%) to red (90-100%)
 *   - Current count / max capacity label
 *   - Warning indicator pulsing at 90%+ utilization
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var PqcMapUtilization = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maps: [
            { key: 'sig_map',       label: 'sig_map' },
            { key: 'key_map',       label: 'key_map' },
            { key: 'policy_map',    label: 'policy_map' },
            { key: 'sovereign_map', label: 'sovereign_map' },
            { key: 'kem_map',       label: 'kem_map' }
        ],
        gaugeArcStart: Math.PI * 0.75,
        gaugeArcEnd: Math.PI * 2.25,
        thresholds: {
            green: 0.70,
            yellow: 0.90
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            cardBg: '#16162b',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            trackBg: 'rgba(255, 255, 255, 0.06)',
            green: '#00d26a',
            yellow: '#ffd700',
            red: '#ff4757',
            warningPulse: 'rgba(255, 71, 87, 0.6)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvases: {},           // keyed by map key
        ctxs: {},
        mapData: {},            // { sig_map: { current: N, max: N }, ... }
        animationFrame: null,
        resizeHandler: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[PqcMapUtilization] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        // Initialize map data
        for (var i = 0; i < CONFIG.maps.length; i++) {
            state.mapData[CONFIG.maps[i].key] = { current: 0, max: 10000 };
        }

        buildLayout(container);

        state.resizeHandler = debounce(resizeCanvases, 150);
        window.addEventListener('resize', state.resizeHandler);
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[PqcMapUtilization] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent +
            ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;border:1px solid ' +
            CONFIG.colors.border + ';border-radius:6px 6px 0 0;';
        hdr.textContent = 'Sophia BPF Map Utilization';
        parent.appendChild(hdr);

        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(5, 1fr);gap:10px;padding:12px;' +
            'background:' + CONFIG.colors.background + ';border:1px solid ' + CONFIG.colors.border +
            ';border-top:none;border-radius:0 0 6px 6px;';

        for (var i = 0; i < CONFIG.maps.length; i++) {
            var map = CONFIG.maps[i];
            var card = document.createElement('div');
            card.id = 'pqc-map-card-' + map.key;
            card.style.cssText = 'border:1px solid ' + CONFIG.colors.border +
                ';border-radius:6px;overflow:hidden;background:' + CONFIG.colors.cardBg + ';';

            var canvas = document.createElement('canvas');
            canvas.style.cssText = 'display:block;width:100%;';
            card.appendChild(canvas);

            state.canvases[map.key] = canvas;
            state.ctxs[map.key] = canvas.getContext('2d');

            grid.appendChild(card);
        }

        parent.appendChild(grid);
    }

    // ========================================================================
    // Canvas Management
    // ========================================================================
    function resizeCanvases() {
        for (var i = 0; i < CONFIG.maps.length; i++) {
            var key = CONFIG.maps[i].key;
            var canvas = state.canvases[key];
            var ctx = state.ctxs[key];
            if (!canvas || !canvas.parentElement) continue;

            var dpr = window.devicePixelRatio || 1;
            var rect = canvas.parentElement.getBoundingClientRect();
            var w = rect.width || 120;
            var h = 170;
            canvas.width = w * dpr;
            canvas.height = h * dpr;
            canvas.style.width = w + 'px';
            canvas.style.height = h + 'px';
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(mapUtilData) {
        if (!state.initialized || !mapUtilData) return;

        if (mapUtilData.maps) {
            for (var key in mapUtilData.maps) {
                if (mapUtilData.maps.hasOwnProperty(key) && state.mapData.hasOwnProperty(key)) {
                    var d = mapUtilData.maps[key];
                    if (d.current != null) state.mapData[key].current = d.current;
                    if (d.max != null) state.mapData[key].max = d.max;
                }
            }
        }
    }

    // ========================================================================
    // Rendering
    // ========================================================================
    function render() {
        for (var i = 0; i < CONFIG.maps.length; i++) {
            renderGauge(CONFIG.maps[i]);
        }
    }

    function renderGauge(mapInfo) {
        var ctx = state.ctxs[mapInfo.key];
        var canvas = state.canvases[mapInfo.key];
        if (!ctx || !canvas) return;

        var dpr = window.devicePixelRatio || 1;
        var w = canvas.width / dpr;
        var h = canvas.height / dpr;
        var data = state.mapData[mapInfo.key];
        var pct = data.max > 0 ? data.current / data.max : 0;
        if (pct > 1) pct = 1;

        ctx.fillStyle = CONFIG.colors.cardBg;
        ctx.fillRect(0, 0, w, h);

        // Gauge geometry
        var cx = w / 2;
        var cy = h * 0.48;
        var radius = Math.min(w, h) * 0.32;
        var lineW = Math.max(radius * 0.22, 6);
        var arcStart = CONFIG.gaugeArcStart;
        var arcEnd = CONFIG.gaugeArcEnd;
        var arcRange = arcEnd - arcStart;

        // Background track
        ctx.beginPath();
        ctx.arc(cx, cy, radius, arcStart, arcEnd);
        ctx.strokeStyle = CONFIG.colors.trackBg;
        ctx.lineWidth = lineW;
        ctx.lineCap = 'round';
        ctx.stroke();

        // Filled arc
        var fillEnd = arcStart + pct * arcRange;
        var gaugeColor = getGaugeColor(pct);
        ctx.beginPath();
        ctx.arc(cx, cy, radius, arcStart, fillEnd);
        ctx.strokeStyle = gaugeColor;
        ctx.lineWidth = lineW;
        ctx.lineCap = 'round';
        ctx.stroke();

        // Tick marks at 70% and 90% thresholds
        drawThresholdTick(ctx, cx, cy, radius, lineW, arcStart + 0.70 * arcRange, CONFIG.colors.yellow);
        drawThresholdTick(ctx, cx, cy, radius, lineW, arcStart + 0.90 * arcRange, CONFIG.colors.red);

        // Warning pulse at 90%+
        if (pct >= 0.90) {
            var pulseAlpha = 0.3 + 0.3 * Math.sin(Date.now() / 300);
            ctx.beginPath();
            ctx.arc(cx, cy, radius + lineW / 2 + 4, arcStart, fillEnd);
            ctx.strokeStyle = CONFIG.colors.warningPulse;
            ctx.globalAlpha = pulseAlpha;
            ctx.lineWidth = 3;
            ctx.lineCap = 'round';
            ctx.stroke();
            ctx.globalAlpha = 1;
        }

        // Percentage text in center
        ctx.font = 'bold 20px -apple-system, sans-serif';
        ctx.fillStyle = gaugeColor;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText((pct * 100).toFixed(1) + '%', cx, cy);

        // Map label
        ctx.font = 'bold 10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.accent;
        ctx.textBaseline = 'alphabetic';
        ctx.fillText(mapInfo.label, cx, 18);

        // Count / max
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText(formatNumber(data.current) + ' / ' + formatNumber(data.max), cx, h - 28);

        // Status label
        var statusLabel = 'OK';
        var statusColor = CONFIG.colors.green;
        if (pct >= 0.90) { statusLabel = 'CRITICAL'; statusColor = CONFIG.colors.red; }
        else if (pct >= 0.70) { statusLabel = 'WARNING'; statusColor = CONFIG.colors.yellow; }

        ctx.font = 'bold 9px "SF Mono", monospace';
        ctx.fillStyle = statusColor;
        ctx.fillText(statusLabel, cx, h - 12);
    }

    function drawThresholdTick(ctx, cx, cy, radius, lineW, angle, color) {
        var innerR = radius - lineW / 2 - 2;
        var outerR = radius + lineW / 2 + 2;
        var x1 = cx + Math.cos(angle) * innerR;
        var y1 = cy + Math.sin(angle) * innerR;
        var x2 = cx + Math.cos(angle) * outerR;
        var y2 = cy + Math.sin(angle) * outerR;

        ctx.beginPath();
        ctx.moveTo(x1, y1);
        ctx.lineTo(x2, y2);
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.5;
        ctx.lineCap = 'butt';
        ctx.stroke();
    }

    function getGaugeColor(pct) {
        if (pct >= CONFIG.thresholds.yellow) return CONFIG.colors.red;
        if (pct >= CONFIG.thresholds.green) return CONFIG.colors.yellow;
        return CONFIG.colors.green;
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
        state.canvases = {};
        state.ctxs = {};
        state.mapData = {};
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

window.PqcMapUtilization = PqcMapUtilization;
console.log('pqc-map-utilization.js loaded');
