/**
 * pqc-algo-distribution.js - PQC Algorithm Usage Pie Chart
 * Donut chart showing percentage of verifications by algorithm family.
 *
 * Features:
 *   - Donut pie chart with algorithm breakdown (SLH-DSA, ML-DSA, FN-DSA)
 *   - Color-coded slices per algorithm family
 *   - Interactive legend with counts and percentages
 *   - Total count displayed in center
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var PqcAlgoDistribution = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        algorithms: ['SLH-DSA', 'ML-DSA', 'FN-DSA'],
        algoColors: {
            'SLH-DSA': '#00d26a',   // green
            'ML-DSA':  '#4ecdc4',   // teal
            'FN-DSA':  '#ffd700'    // gold
        },
        padding: { top: 16, right: 16, bottom: 16, left: 16 },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvas: null,
        ctx: null,
        counts: {},             // { 'SLH-DSA': N, 'ML-DSA': N, 'FN-DSA': N }
        animationFrame: null,
        resizeHandler: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[PqcAlgoDistribution] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        // Initialize counts
        for (var i = 0; i < CONFIG.algorithms.length; i++) {
            state.counts[CONFIG.algorithms[i]] = 0;
        }

        buildChart(container);

        state.resizeHandler = debounce(resizeCanvas, 150);
        window.addEventListener('resize', state.resizeHandler);
        resizeCanvas();
        startAnimation();

        state.initialized = true;
        console.log('[PqcAlgoDistribution] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildChart(parent) {
        var section = createSection('PQC Algorithm Distribution', 'pqc-algo-section');
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
        var h = Math.max(280, 200);
        state.canvas.width = w * dpr;
        state.canvas.height = h * dpr;
        state.canvas.style.width = w + 'px';
        state.canvas.style.height = h + 'px';
        state.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(algoData) {
        if (!state.initialized || !algoData) return;

        if (algoData.counts) {
            for (var key in algoData.counts) {
                if (algoData.counts.hasOwnProperty(key)) {
                    state.counts[key] = algoData.counts[key];
                }
            }
        }
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

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var keys = CONFIG.algorithms;
        var total = 0;
        for (var i = 0; i < keys.length; i++) total += (state.counts[keys[i]] || 0);

        if (total === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No algorithm data', w / 2, h / 2);
            return;
        }

        // Pie geometry
        var cx = w * 0.38;
        var cy = h / 2;
        var radius = Math.min(cx - 30, cy - 30, 110);
        var innerRadius = radius * 0.55;
        var startAngle = -Math.PI / 2;

        // Draw slices
        for (var j = 0; j < keys.length; j++) {
            var count = state.counts[keys[j]] || 0;
            if (count === 0) continue;
            var slice = count / total;
            var endAngle = startAngle + slice * Math.PI * 2;

            ctx.fillStyle = CONFIG.algoColors[keys[j]] || '#6b6b7b';
            ctx.beginPath();
            ctx.moveTo(cx + Math.cos(startAngle) * innerRadius, cy + Math.sin(startAngle) * innerRadius);
            ctx.arc(cx, cy, radius, startAngle, endAngle);
            ctx.arc(cx, cy, innerRadius, endAngle, startAngle, true);
            ctx.closePath();
            ctx.fill();

            // Slice separator line
            ctx.strokeStyle = CONFIG.colors.background;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.moveTo(cx + Math.cos(startAngle) * innerRadius, cy + Math.sin(startAngle) * innerRadius);
            ctx.lineTo(cx + Math.cos(startAngle) * radius, cy + Math.sin(startAngle) * radius);
            ctx.stroke();

            // Percentage label on slice
            var midAngle = startAngle + (slice * Math.PI);
            var labelRadius = radius * 0.8;
            var lx = cx + Math.cos(midAngle) * labelRadius;
            var ly = cy + Math.sin(midAngle) * labelRadius;
            if (slice > 0.06) {
                ctx.fillStyle = '#000000';
                ctx.font = 'bold 11px "SF Mono", monospace';
                ctx.textAlign = 'center';
                ctx.textBaseline = 'middle';
                ctx.fillText(((count / total) * 100).toFixed(0) + '%', lx, ly);
            }

            startAngle = endAngle;
        }

        // Center text
        ctx.fillStyle = CONFIG.colors.text;
        ctx.font = 'bold 18px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(formatNumber(total), cx, cy - 6);
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText('total', cx, cy + 12);

        // Legend
        ctx.textBaseline = 'alphabetic';
        var legendX = w * 0.68;
        var legendY = cy - (keys.length * 28) / 2 + 10;
        ctx.font = '11px "SF Mono", monospace';
        ctx.textAlign = 'left';

        for (var k = 0; k < keys.length; k++) {
            var kCount = state.counts[keys[k]] || 0;
            var pct = ((kCount / total) * 100).toFixed(1);
            var ly2 = legendY + k * 28;

            // Color swatch
            ctx.fillStyle = CONFIG.algoColors[keys[k]] || '#6b6b7b';
            ctx.beginPath();
            ctx.roundRect(legendX, ly2 - 6, 14, 14, 3);
            ctx.fill();

            // Algorithm name
            ctx.fillStyle = CONFIG.colors.text;
            ctx.fillText(keys[k], legendX + 20, ly2 + 5);

            // Count and percentage
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.fillText(formatNumber(kCount) + ' (' + pct + '%)', legendX + 20, ly2 + 19);
        }
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
        state.counts = {};
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

window.PqcAlgoDistribution = PqcAlgoDistribution;
console.log('pqc-algo-distribution.js loaded');
