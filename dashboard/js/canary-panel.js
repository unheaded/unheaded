/**
 * canary-panel.js - Canary Deployment Panel (App 05 - Monad CPU)
 * Visualization of canary deployment traffic splits, error rates, and rollout status.
 *
 * Features:
 *   - Traffic split waterfall: stacked area (X: time, Y: traffic %)
 *     Colors: red=ROLLBACK, yellow=RAMP, green=STABLE, blue=PROBING
 *   - Error rate comparison: canary vs stable line chart with SLO threshold
 *   - Latency P99 timeline: canary vs stable with confidence band
 *   - Promotion/Rollback events: diamonds (promotion), X (rollback)
 *   - Circuit breaker status: gauge per service (OPEN/CLOSED)
 *   - Active canaries table: service, canary version, stable version, split %, state
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var CanaryPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        timeWindowMinutes: 30,
        maxDataPoints: 180,    // 10s intervals over 30 min
        sloThreshold: 0.01,   // 1% error rate SLO
        padding: { top: 20, right: 16, bottom: 30, left: 50 },

        stateColors: {
            'ROLLBACK': '#ff4757',
            'RAMP':     '#ffd700',
            'STABLE':   '#00d26a',
            'PROBING':  '#45b7d1'
        },
        circuitColors: {
            'CLOSED':    '#00d26a',
            'OPEN':      '#ff4757',
            'HALF_OPEN': '#ff9800'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            canaryLine: '#ff6b6b',
            stableLine: '#00d26a',
            sloLine: '#ff4757',
            confidenceBand: 'rgba(78, 205, 196, 0.1)',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            promotionMarker: '#00d26a',
            rollbackMarker: '#ff4757'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,

        // Canvases
        trafficCanvas: null, trafficCtx: null,
        errorCanvas: null, errorCtx: null,
        latencyCanvas: null, latencyCtx: null,

        // Data
        trafficSplits: [],    // [{ timestamp, ROLLBACK:%, RAMP:%, STABLE:%, PROBING:% }]
        errorRates: [],       // [{ timestamp, canary: rate, stable: rate }]
        latencyP99: [],       // [{ timestamp, canary: ms, stable: ms, canary_ci_low, canary_ci_high }]
        events: [],           // [{ timestamp, type: 'promotion'|'rollback', service, version }]
        circuitBreakers: [],  // [{ service, state: OPEN|CLOSED|HALF_OPEN }]
        activeCanaries: [],   // [{ service, canary_version, stable_version, split_pct, state }]

        canaryTable: null,
        circuitGrid: null,
        eventList: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[CanaryPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[CanaryPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Top row: traffic split + error rate
        var topGrid = document.createElement('div');
        topGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var trafficSection = createSection('Traffic Split Waterfall');
        state.trafficCanvas = document.createElement('canvas');
        state.trafficCanvas.style.cssText = 'display:block;width:100%;';
        trafficSection.body.appendChild(state.trafficCanvas);
        state.trafficCtx = state.trafficCanvas.getContext('2d');

        // Legend for traffic split
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:12px;padding:6px 0;font-size:10px;';
        var states = ['STABLE', 'RAMP', 'PROBING', 'ROLLBACK'];
        for (var s = 0; s < states.length; s++) {
            legend.innerHTML += '<span><span style="display:inline-block;width:10px;height:10px;background:' +
                CONFIG.stateColors[states[s]] + ';border-radius:2px;margin-right:4px"></span>' + states[s] + '</span>';
        }
        trafficSection.body.appendChild(legend);
        topGrid.appendChild(trafficSection.el);

        var errorSection = createSection('Error Rate: Canary vs Stable');
        state.errorCanvas = document.createElement('canvas');
        state.errorCanvas.style.cssText = 'display:block;width:100%;';
        errorSection.body.appendChild(state.errorCanvas);
        state.errorCtx = state.errorCanvas.getContext('2d');
        topGrid.appendChild(errorSection.el);

        parent.appendChild(topGrid);

        // Second row: latency + circuit breakers
        var midGrid = document.createElement('div');
        midGrid.style.cssText = 'display:grid;grid-template-columns:2fr 1fr;gap:12px;margin-bottom:12px;';

        var latencySection = createSection('Latency P99: Canary vs Stable');
        state.latencyCanvas = document.createElement('canvas');
        state.latencyCanvas.style.cssText = 'display:block;width:100%;';
        latencySection.body.appendChild(state.latencyCanvas);
        state.latencyCtx = state.latencyCanvas.getContext('2d');
        midGrid.appendChild(latencySection.el);

        var cbSection = createSection('Circuit Breaker Status');
        state.circuitGrid = document.createElement('div');
        state.circuitGrid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(120px,1fr));gap:6px;';
        cbSection.body.appendChild(state.circuitGrid);
        midGrid.appendChild(cbSection.el);

        parent.appendChild(midGrid);

        // Events timeline
        var eventSection = createSection('Promotion / Rollback Events');
        state.eventList = document.createElement('div');
        state.eventList.style.cssText = 'max-height:150px;overflow-y:auto;';
        eventSection.body.appendChild(state.eventList);
        parent.appendChild(eventSection.el);

        // Active canaries table
        var tableSection = createSection('Active Canaries');
        var tableWrapper = document.createElement('div');
        tableWrapper.style.cssText = 'max-height:200px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Service</th>' +
            '<th style="padding:6px 8px;text-align:left">Canary Ver</th>' +
            '<th style="padding:6px 8px;text-align:left">Stable Ver</th>' +
            '<th style="padding:6px 8px;text-align:right">Split %</th>' +
            '<th style="padding:6px 8px;text-align:left">State</th></tr></thead>';
        state.canaryTable = document.createElement('tbody');
        table.appendChild(state.canaryTable);
        tableWrapper.appendChild(table);
        tableSection.body.appendChild(tableWrapper);
        parent.appendChild(tableSection.el);
    }

    function createSection(title) {
        var el = document.createElement('div');
        el.style.cssText = 'border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;overflow:hidden;margin-bottom:12px;';

        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent + ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;';
        hdr.textContent = title;
        el.appendChild(hdr);

        var body = document.createElement('div');
        body.style.cssText = 'padding:12px;background:' + CONFIG.colors.background + ';';
        el.appendChild(body);

        return { el: el, body: body };
    }

    function resizeCanvases() {
        resizeCanvas(state.trafficCanvas, state.trafficCtx, 200);
        resizeCanvas(state.errorCanvas, state.errorCtx, 200);
        resizeCanvas(state.latencyCanvas, state.latencyCtx, 200);
    }

    function resizeCanvas(canvas, ctx, minH) {
        if (!canvas || !canvas.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = canvas.parentElement.getBoundingClientRect();
        var w = rect.width || 400;
        var h = Math.max(minH, 180);
        canvas.width = w * dpr;
        canvas.height = h * dpr;
        canvas.style.width = w + 'px';
        canvas.style.height = h + 'px';
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(canaryData) {
        if (!state.initialized || !canaryData) return;

        if (canaryData.traffic_splits) {
            state.trafficSplits = canaryData.traffic_splits.slice(-CONFIG.maxDataPoints);
        }
        if (canaryData.error_rates) {
            state.errorRates = canaryData.error_rates.slice(-CONFIG.maxDataPoints);
        }
        if (canaryData.latency_p99) {
            state.latencyP99 = canaryData.latency_p99.slice(-CONFIG.maxDataPoints);
        }
        if (canaryData.events) {
            state.events = canaryData.events;
        }
        if (canaryData.circuit_breakers) {
            state.circuitBreakers = canaryData.circuit_breakers;
            renderCircuitBreakers();
        }
        if (canaryData.active_canaries) {
            state.activeCanaries = canaryData.active_canaries;
            renderCanaryTable();
        }

        renderEvents();
    }

    // ========================================================================
    // Rendering - Traffic Split Waterfall (Stacked Area)
    // ========================================================================
    function renderTrafficSplit() {
        var ctx = state.trafficCtx;
        if (!ctx || !state.trafficCanvas) return;
        var w = canvasW(state.trafficCanvas);
        var h = canvasH(state.trafficCanvas);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.trafficSplits;
        if (data.length < 2) {
            emptyText(ctx, w, h, 'Waiting for traffic split data...');
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var stateKeys = ['STABLE', 'RAMP', 'PROBING', 'ROLLBACK'];

        // Stacked area: bottom-up
        for (var s = stateKeys.length - 1; s >= 0; s--) {
            ctx.fillStyle = CONFIG.stateColors[stateKeys[s]];
            ctx.globalAlpha = 0.6;
            ctx.beginPath();
            ctx.moveTo(pad.left, pad.top + plotH);

            for (var i = 0; i < data.length; i++) {
                var x = pad.left + (i / (data.length - 1)) * plotW;
                var cumulative = 0;
                for (var k = 0; k <= s; k++) cumulative += (data[i][stateKeys[k]] || 0);
                var y = pad.top + plotH - (cumulative / 100) * plotH;
                ctx.lineTo(x, y);
            }

            ctx.lineTo(pad.left + plotW, pad.top + plotH);
            ctx.closePath();
            ctx.fill();
        }
        ctx.globalAlpha = 1.0;

        // Axes
        drawSimpleAxes(ctx, pad, plotW, plotH, '0%', '100%', 'Traffic %');
    }

    // ========================================================================
    // Rendering - Error Rate Chart
    // ========================================================================
    function renderErrorRate() {
        var ctx = state.errorCtx;
        if (!ctx || !state.errorCanvas) return;
        var w = canvasW(state.errorCanvas);
        var h = canvasH(state.errorCanvas);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.errorRates;
        if (data.length < 2) {
            emptyText(ctx, w, h, 'Waiting for error rate data...');
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = CONFIG.sloThreshold * 3;
        for (var i = 0; i < data.length; i++) {
            if (data[i].canary > yMax) yMax = data[i].canary;
            if (data[i].stable > yMax) yMax = data[i].stable;
        }
        yMax = Math.ceil(yMax * 100) / 100 || 0.05;

        // Grid
        drawGrid(ctx, pad, plotW, plotH, 4);

        // SLO threshold line
        var sloY = pad.top + plotH - (CONFIG.sloThreshold / yMax) * plotH;
        ctx.setLineDash([6, 3]);
        ctx.strokeStyle = CONFIG.colors.sloLine;
        ctx.lineWidth = 1;
        ctx.beginPath(); ctx.moveTo(pad.left, sloY); ctx.lineTo(pad.left + plotW, sloY); ctx.stroke();
        ctx.setLineDash([]);
        ctx.fillStyle = CONFIG.colors.sloLine;
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'left';
        ctx.fillText('SLO ' + (CONFIG.sloThreshold * 100) + '%', pad.left + 4, sloY - 4);

        // Canary line
        drawLine(ctx, data, 'canary', pad, plotW, plotH, yMax, CONFIG.colors.canaryLine, 2);
        // Stable line
        drawLine(ctx, data, 'stable', pad, plotW, plotH, yMax, CONFIG.colors.stableLine, 2);

        // Legend
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.canaryLine;
        ctx.fillRect(pad.left + plotW - 100, pad.top + 4, 10, 3);
        ctx.fillStyle = CONFIG.colors.text;
        ctx.fillText('Canary', pad.left + plotW - 86, pad.top + 10);
        ctx.fillStyle = CONFIG.colors.stableLine;
        ctx.fillRect(pad.left + plotW - 100, pad.top + 18, 10, 3);
        ctx.fillStyle = CONFIG.colors.text;
        ctx.fillText('Stable', pad.left + plotW - 86, pad.top + 24);
    }

    // ========================================================================
    // Rendering - Latency P99
    // ========================================================================
    function renderLatency() {
        var ctx = state.latencyCtx;
        if (!ctx || !state.latencyCanvas) return;
        var w = canvasW(state.latencyCanvas);
        var h = canvasH(state.latencyCanvas);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.latencyP99;
        if (data.length < 2) {
            emptyText(ctx, w, h, 'Waiting for latency data...');
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = 0;
        for (var i = 0; i < data.length; i++) {
            var hi = data[i].canary_ci_high || data[i].canary || 0;
            if (hi > yMax) yMax = hi;
            if ((data[i].stable || 0) > yMax) yMax = data[i].stable;
        }
        yMax = Math.ceil(yMax * 1.2) || 50;

        drawGrid(ctx, pad, plotW, plotH, 4);

        // Confidence band
        ctx.fillStyle = CONFIG.colors.confidenceBand;
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var x = pad.left + (j / (data.length - 1)) * plotW;
            var yLow = pad.top + plotH - ((data[j].canary_ci_low || data[j].canary * 0.9 || 0) / yMax) * plotH;
            if (j === 0) ctx.moveTo(x, yLow); else ctx.lineTo(x, yLow);
        }
        for (var k = data.length - 1; k >= 0; k--) {
            var x2 = pad.left + (k / (data.length - 1)) * plotW;
            var yHigh = pad.top + plotH - ((data[k].canary_ci_high || data[k].canary * 1.1 || 0) / yMax) * plotH;
            ctx.lineTo(x2, yHigh);
        }
        ctx.closePath();
        ctx.fill();

        drawLine(ctx, data, 'canary', pad, plotW, plotH, yMax, CONFIG.colors.canaryLine, 2);
        drawLine(ctx, data, 'stable', pad, plotW, plotH, yMax, CONFIG.colors.stableLine, 2);

        // Event markers
        for (var e = 0; e < state.events.length; e++) {
            var ev = state.events[e];
            var evTs = ev.timestamp;
            // Find closest data point
            var closestIdx = 0;
            var closestDist = Infinity;
            for (var d = 0; d < data.length; d++) {
                var dist = Math.abs((data[d].timestamp || 0) - evTs);
                if (dist < closestDist) { closestDist = dist; closestIdx = d; }
            }
            var ex = pad.left + (closestIdx / (data.length - 1)) * plotW;
            var ey = pad.top + 10;

            if (ev.type === 'promotion') {
                drawDiamond(ctx, ex, ey, 6, CONFIG.colors.promotionMarker);
            } else {
                drawCross(ctx, ex, ey, 5, CONFIG.colors.rollbackMarker);
            }
        }
    }

    // ========================================================================
    // Rendering - Circuit Breakers
    // ========================================================================
    function renderCircuitBreakers() {
        if (!state.circuitGrid) return;
        state.circuitGrid.innerHTML = '';

        for (var i = 0; i < state.circuitBreakers.length; i++) {
            var cb = state.circuitBreakers[i];
            var color = CONFIG.circuitColors[cb.state] || CONFIG.colors.textMuted;
            var card = document.createElement('div');
            card.style.cssText = 'padding:8px;background:#12121f;border:1px solid ' + CONFIG.colors.border +
                ';border-radius:4px;text-align:center;';
            card.innerHTML =
                '<div style="font-size:10px;color:' + CONFIG.colors.textMuted + ';margin-bottom:4px">' +
                (cb.service || '--') + '</div>' +
                '<div style="display:inline-block;padding:2px 8px;background:' + color +
                ';color:#0f0f1a;font-size:10px;font-weight:bold;border-radius:3px">' +
                (cb.state || '--') + '</div>';
            state.circuitGrid.appendChild(card);
        }
    }

    // ========================================================================
    // Rendering - Events & Table
    // ========================================================================
    function renderEvents() {
        if (!state.eventList) return;
        state.eventList.innerHTML = '';

        if (state.events.length === 0) {
            state.eventList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:16px;font-size:12px">No events</div>';
            return;
        }

        for (var i = 0; i < state.events.length; i++) {
            var ev = state.events[i];
            var isPromo = ev.type === 'promotion';
            var markerColor = isPromo ? CONFIG.colors.promotionMarker : CONFIG.colors.rollbackMarker;
            var markerSymbol = isPromo ? '&#9670;' : '&#10005;';

            var row = document.createElement('div');
            row.style.cssText = 'display:flex;gap:8px;padding:4px 8px;font-size:11px;border-bottom:1px solid ' +
                CONFIG.colors.border + ';align-items:center;';
            row.innerHTML =
                '<span style="color:' + markerColor + ';font-size:14px">' + markerSymbol + '</span>' +
                '<span style="width:70px;color:' + CONFIG.colors.textMuted + '">' + formatTime(ev.timestamp) + '</span>' +
                '<span style="color:' + markerColor + ';width:80px;text-transform:uppercase;font-size:10px;font-weight:bold">' +
                ev.type + '</span>' +
                '<span>' + (ev.service || '--') + ' v' + (ev.version || '--') + '</span>';
            state.eventList.appendChild(row);
        }
    }

    function renderCanaryTable() {
        if (!state.canaryTable) return;
        state.canaryTable.innerHTML = '';

        for (var i = 0; i < state.activeCanaries.length; i++) {
            var c = state.activeCanaries[i];
            var stColor = CONFIG.stateColors[c.state] || CONFIG.colors.textMuted;
            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px">' + (c.service || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.canaryLine + '">' + (c.canary_version || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.stableLine + '">' + (c.stable_version || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (c.split_pct || 0).toFixed(1) + '%</td>' +
                '<td style="padding:4px 8px;color:' + stColor + ';font-weight:bold;font-size:10px">' +
                (c.state || '--') + '</td>';
            state.canaryTable.appendChild(tr);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderTrafficSplit();
            renderErrorRate();
            renderLatency();
            state.animationFrame = requestAnimationFrame(tick);
        }
        state.animationFrame = requestAnimationFrame(tick);
    }

    function stopAnimation() {
        if (state.animationFrame) { cancelAnimationFrame(state.animationFrame); state.animationFrame = null; }
    }

    // ========================================================================
    // Drawing Helpers
    // ========================================================================
    function drawLine(ctx, data, key, pad, plotW, plotH, yMax, color, lineWidth) {
        ctx.strokeStyle = color;
        ctx.lineWidth = lineWidth;
        ctx.lineJoin = 'round';
        ctx.beginPath();
        for (var i = 0; i < data.length; i++) {
            var x = pad.left + (i / (data.length - 1)) * plotW;
            var y = pad.top + plotH - ((data[i][key] || 0) / yMax) * plotH;
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
    }

    function drawGrid(ctx, pad, plotW, plotH, steps) {
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var i = 1; i < steps; i++) {
            var y = pad.top + (plotH * i / steps);
            ctx.beginPath(); ctx.moveTo(pad.left, y); ctx.lineTo(pad.left + plotW, y); ctx.stroke();
        }
    }

    function drawSimpleAxes(ctx, pad, plotW, plotH, yMinLabel, yMaxLabel, _yTitle) {
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top);
        ctx.lineTo(pad.left, pad.top + plotH);
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.stroke();

        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        ctx.fillText(yMaxLabel, pad.left - 4, pad.top + 10);
        ctx.fillText(yMinLabel, pad.left - 4, pad.top + plotH + 4);
    }

    function drawDiamond(ctx, x, y, size, color) {
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.moveTo(x, y - size);
        ctx.lineTo(x + size, y);
        ctx.lineTo(x, y + size);
        ctx.lineTo(x - size, y);
        ctx.closePath();
        ctx.fill();
    }

    function drawCross(ctx, x, y, size, color) {
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(x - size, y - size); ctx.lineTo(x + size, y + size);
        ctx.moveTo(x + size, y - size); ctx.lineTo(x - size, y + size);
        ctx.stroke();
    }

    function emptyText(ctx, w, h, msg) {
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.font = '12px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(msg, w / 2, h / 2);
    }

    function canvasW(c) { return c.width / (window.devicePixelRatio || 1); }
    function canvasH(c) { return c.height / (window.devicePixelRatio || 1); }

    function formatTime(ts) {
        if (!ts) return '--:--:--';
        var d = new Date(typeof ts === 'number' ? ts : Date.parse(ts));
        return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
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
        if (state.container) state.container.innerHTML = '';
        state.trafficSplits = [];
        state.errorRates = [];
        state.latencyP99 = [];
        state.events = [];
        state.circuitBreakers = [];
        state.activeCanaries = [];
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

window.CanaryPanel = CanaryPanel;
console.log('canary-panel.js loaded');
