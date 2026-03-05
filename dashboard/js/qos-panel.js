/**
 * qos-panel.js - Quality of Service Panel (App 07 - Monad CPU)
 * Visualization of QoS priority classes, congestion state, and CoDel metrics.
 *
 * Features:
 *   - Priority class grid: 8 columns (class 0-7), weight, packet count, drop rate, queue depth, P50/P99/P999
 *   - Congestion map: service list with circuit state (OK/WARN/CONGESTED/PANIC) badges
 *   - Rate limit vs actual: line chart (target rate vs actual throughput, last 5 min)
 *   - CoDel sojourn time histogram: latency buckets with target line at 5ms
 *   - Stats: total classes, packets/s, drop rate %, congested services count
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var QoSPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        priorityClasses: 8,
        rateHistoryPoints: 60,   // 5 min at 5s intervals
        codelBuckets: ['0-1ms', '1-5ms', '5-10ms', '10ms+'],
        codelTarget: 5,          // 5ms target

        congestionColors: {
            'OK':        '#00d26a',
            'WARN':      '#ffd700',
            'CONGESTED': '#ff9800',
            'PANIC':     '#ff4757'
        },
        classColors: [
            '#ff4757', '#ff9800', '#ffd700', '#f9ca24',
            '#00d26a', '#4ecdc4', '#45b7d1', '#6c5ce7'
        ],
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            targetLine: '#4ecdc4',
            actualLine: '#ffd700',
            targetArea: 'rgba(78, 205, 196, 0.1)',
            barFill: '#4ecdc4',
            barBg: 'rgba(255, 255, 255, 0.05)',
            codelTarget: '#ff4757',
            cardBg: '#12121f'
        },
        padding: { top: 20, right: 16, bottom: 30, left: 50 }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        rateCanvas: null,
        rateCtx: null,
        codelCanvas: null,
        codelCtx: null,

        // Data
        priorityData: [],     // [{ class: 0-7, weight, packet_count, drop_rate, queue_depth, p50, p99, p999 }]
        congestionMap: [],    // [{ service, state: OK|WARN|CONGESTED|PANIC }]
        rateData: [],         // [{ timestamp, target_rate, actual_rate }]
        codelHistogram: [],   // [count, count, count, count] matching codelBuckets
        stats: { total_classes: 0, packets_per_sec: 0, drop_rate_pct: 0, congested_count: 0 },

        priorityGrid: null,
        congestionList: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[QoSPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildStatsBar(container);
        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[QoSPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildStatsBar(parent) {
        var bar = document.createElement('div');
        bar.id = 'qos-stats-bar';
        bar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        bar.innerHTML =
            '<span>Classes: <strong id="qos-stat-classes">--</strong></span>' +
            '<span>Packets/s: <strong id="qos-stat-pps">--</strong></span>' +
            '<span>Drop Rate: <strong id="qos-stat-drop" style="color:#ff4757">--%</strong></span>' +
            '<span>Congested: <strong id="qos-stat-congested" style="color:#ff9800">--</strong></span>';
        parent.appendChild(bar);
    }

    function buildLayout(parent) {
        // Priority class grid
        var priSection = createSection('Priority Classes (0-7)');
        state.priorityGrid = document.createElement('div');
        state.priorityGrid.style.cssText = 'overflow-x:auto;';
        priSection.body.appendChild(state.priorityGrid);
        parent.appendChild(priSection.el);

        // Mid row: congestion map + rate chart
        var midGrid = document.createElement('div');
        midGrid.style.cssText = 'display:grid;grid-template-columns:1fr 2fr;gap:12px;margin-bottom:12px;';

        var congSection = createSection('Congestion Map');
        state.congestionList = document.createElement('div');
        state.congestionList.style.cssText = 'max-height:250px;overflow-y:auto;';
        congSection.body.appendChild(state.congestionList);
        midGrid.appendChild(congSection.el);

        var rateSection = createSection('Rate Limit vs Actual Throughput (5 min)');
        state.rateCanvas = document.createElement('canvas');
        state.rateCanvas.style.cssText = 'display:block;width:100%;';
        rateSection.body.appendChild(state.rateCanvas);
        state.rateCtx = state.rateCanvas.getContext('2d');
        midGrid.appendChild(rateSection.el);

        parent.appendChild(midGrid);

        // CoDel histogram
        var codelSection = createSection('CoDel Sojourn Time Distribution');
        state.codelCanvas = document.createElement('canvas');
        state.codelCanvas.style.cssText = 'display:block;width:100%;';
        codelSection.body.appendChild(state.codelCanvas);
        state.codelCtx = state.codelCanvas.getContext('2d');
        parent.appendChild(codelSection.el);
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
        resizeCanvas(state.rateCanvas, state.rateCtx, 200);
        resizeCanvas(state.codelCanvas, state.codelCtx, 180);
    }

    function resizeCanvas(canvas, ctx, minH) {
        if (!canvas || !canvas.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = canvas.parentElement.getBoundingClientRect();
        var w = rect.width || 400;
        var h = Math.max(minH, 160);
        canvas.width = w * dpr;
        canvas.height = h * dpr;
        canvas.style.width = w + 'px';
        canvas.style.height = h + 'px';
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(qosData) {
        if (!state.initialized || !qosData) return;

        if (qosData.priority_classes) {
            state.priorityData = qosData.priority_classes;
            renderPriorityGrid();
        }
        if (qosData.congestion_map) {
            state.congestionMap = qosData.congestion_map;
            renderCongestionMap();
        }
        if (qosData.rate_data) {
            state.rateData = qosData.rate_data.slice(-CONFIG.rateHistoryPoints);
        }
        if (qosData.codel_histogram) {
            state.codelHistogram = qosData.codel_histogram;
        }
        if (qosData.stats) {
            state.stats = qosData.stats;
            updateStats();
        }
    }

    function updateStats() {
        var s = state.stats;
        setTextById('qos-stat-classes', String(s.total_classes || 0));
        setTextById('qos-stat-pps', formatNumber(s.packets_per_sec || 0));
        setTextById('qos-stat-drop', (s.drop_rate_pct || 0).toFixed(2) + '%');
        setTextById('qos-stat-congested', String(s.congested_count || 0));
    }

    // ========================================================================
    // Rendering - Priority Class Grid
    // ========================================================================
    function renderPriorityGrid() {
        if (!state.priorityGrid) return;

        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';

        // Header
        var hdr = '<thead><tr style="color:' + CONFIG.colors.accent + '">' +
            '<th style="padding:6px 6px;text-align:center">Class</th>' +
            '<th style="padding:6px 6px;text-align:right">Weight</th>' +
            '<th style="padding:6px 6px;text-align:right">Packets</th>' +
            '<th style="padding:6px 6px;text-align:right">Drop %</th>' +
            '<th style="padding:6px 6px;text-align:right">Queue</th>' +
            '<th style="padding:6px 6px;text-align:right">P50</th>' +
            '<th style="padding:6px 6px;text-align:right">P99</th>' +
            '<th style="padding:6px 6px;text-align:right">P999</th></tr></thead>';
        table.innerHTML = hdr;

        var tbody = document.createElement('tbody');
        for (var i = 0; i < state.priorityData.length; i++) {
            var p = state.priorityData[i];
            var classColor = CONFIG.classColors[p.class_id || i] || CONFIG.colors.text;
            var dropColor = (p.drop_rate || 0) > 0.01 ? '#ff4757' : CONFIG.colors.text;

            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 6px;text-align:center"><span style="display:inline-block;width:20px;height:14px;' +
                'background:' + classColor + ';border-radius:2px;vertical-align:middle;margin-right:4px"></span>' +
                (p.class_id != null ? p.class_id : i) + '</td>' +
                '<td style="padding:4px 6px;text-align:right">' + (p.weight || 0) + '</td>' +
                '<td style="padding:4px 6px;text-align:right">' + formatNumber(p.packet_count || 0) + '</td>' +
                '<td style="padding:4px 6px;text-align:right;color:' + dropColor + '">' +
                ((p.drop_rate || 0) * 100).toFixed(2) + '%</td>' +
                '<td style="padding:4px 6px;text-align:right">' + (p.queue_depth || 0) + '</td>' +
                '<td style="padding:4px 6px;text-align:right">' + (p.p50 || 0).toFixed(1) + 'ms</td>' +
                '<td style="padding:4px 6px;text-align:right">' + (p.p99 || 0).toFixed(1) + 'ms</td>' +
                '<td style="padding:4px 6px;text-align:right">' + (p.p999 || 0).toFixed(1) + 'ms</td>';
            tbody.appendChild(tr);
        }
        table.appendChild(tbody);
        state.priorityGrid.innerHTML = '';
        state.priorityGrid.appendChild(table);
    }

    // ========================================================================
    // Rendering - Congestion Map
    // ========================================================================
    function renderCongestionMap() {
        if (!state.congestionList) return;
        state.congestionList.innerHTML = '';

        if (state.congestionMap.length === 0) {
            state.congestionList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:20px;font-size:12px">No services</div>';
            return;
        }

        for (var i = 0; i < state.congestionMap.length; i++) {
            var svc = state.congestionMap[i];
            var badgeColor = CONFIG.congestionColors[svc.state] || CONFIG.colors.textMuted;

            var row = document.createElement('div');
            row.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:6px 8px;' +
                'border-bottom:1px solid ' + CONFIG.colors.border + ';font-size:11px;';
            row.innerHTML =
                '<span>' + (svc.service || '--') + '</span>' +
                '<span style="display:inline-block;padding:2px 8px;background:' + badgeColor +
                ';color:#0f0f1a;font-size:9px;font-weight:bold;border-radius:3px;text-transform:uppercase">' +
                (svc.state || '--') + '</span>';
            state.congestionList.appendChild(row);
        }
    }

    // ========================================================================
    // Rendering - Rate Chart
    // ========================================================================
    function renderRateChart() {
        var ctx = state.rateCtx;
        if (!ctx || !state.rateCanvas) return;
        var w = state.rateCanvas.width / (window.devicePixelRatio || 1);
        var h = state.rateCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.rateData;
        if (data.length < 2) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for rate data...', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = 0;
        for (var i = 0; i < data.length; i++) {
            if ((data[i].target_rate || 0) > yMax) yMax = data[i].target_rate;
            if ((data[i].actual_rate || 0) > yMax) yMax = data[i].actual_rate;
        }
        yMax = Math.ceil(yMax * 1.2) || 1000;

        // Grid
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var g = 1; g < 4; g++) {
            var gy = pad.top + plotH * (1 - g / 4);
            ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + plotW, gy); ctx.stroke();
        }

        // Target area
        ctx.fillStyle = CONFIG.colors.targetArea;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top + plotH);
        for (var t = 0; t < data.length; t++) {
            var tx = pad.left + (t / (data.length - 1)) * plotW;
            var ty = pad.top + plotH - ((data[t].target_rate || 0) / yMax) * plotH;
            ctx.lineTo(tx, ty);
        }
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.closePath();
        ctx.fill();

        // Target line
        drawDataLine(ctx, data, 'target_rate', pad, plotW, plotH, yMax, CONFIG.colors.targetLine, 1.5, [4, 3]);
        // Actual line
        drawDataLine(ctx, data, 'actual_rate', pad, plotW, plotH, yMax, CONFIG.colors.actualLine, 2, []);

        // Legend
        ctx.font = '10px "SF Mono", monospace';
        ctx.textAlign = 'right';
        ctx.setLineDash([4, 3]);
        ctx.strokeStyle = CONFIG.colors.targetLine;
        ctx.beginPath(); ctx.moveTo(w - 120, pad.top + 8); ctx.lineTo(w - 100, pad.top + 8); ctx.stroke();
        ctx.setLineDash([]);
        ctx.fillStyle = CONFIG.colors.text;
        ctx.fillText('Target', w - pad.right, pad.top + 12);

        ctx.strokeStyle = CONFIG.colors.actualLine;
        ctx.lineWidth = 2;
        ctx.beginPath(); ctx.moveTo(w - 120, pad.top + 22); ctx.lineTo(w - 100, pad.top + 22); ctx.stroke();
        ctx.fillText('Actual', w - pad.right, pad.top + 26);
    }

    function drawDataLine(ctx, data, key, pad, plotW, plotH, yMax, color, lineWidth, dash) {
        ctx.strokeStyle = color;
        ctx.lineWidth = lineWidth;
        ctx.setLineDash(dash);
        ctx.lineJoin = 'round';
        ctx.beginPath();
        for (var i = 0; i < data.length; i++) {
            var x = pad.left + (i / (data.length - 1)) * plotW;
            var y = pad.top + plotH - ((data[i][key] || 0) / yMax) * plotH;
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
        ctx.setLineDash([]);
    }

    // ========================================================================
    // Rendering - CoDel Histogram
    // ========================================================================
    function renderCodelHistogram() {
        var ctx = state.codelCtx;
        if (!ctx || !state.codelCanvas) return;
        var w = state.codelCanvas.width / (window.devicePixelRatio || 1);
        var h = state.codelCanvas.height / (window.devicePixelRatio || 1);
        var pad = { top: 20, right: 20, bottom: 36, left: 50 };

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var buckets = CONFIG.codelBuckets;
        var data = state.codelHistogram;
        if (!data || data.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for CoDel data...', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var barCount = Math.min(data.length, buckets.length);
        var barW = plotW / barCount;
        var gap = barW * 0.2;

        var maxVal = 0;
        for (var i = 0; i < barCount; i++) {
            if (data[i] > maxVal) maxVal = data[i];
        }
        maxVal = maxVal || 1;

        // Bars
        for (var j = 0; j < barCount; j++) {
            var barH = (data[j] / maxVal) * plotH;
            var x = pad.left + j * barW + gap / 2;
            var y = pad.top + plotH - barH;

            // Background
            ctx.fillStyle = CONFIG.colors.barBg;
            ctx.fillRect(x, pad.top, barW - gap, plotH);

            // Bar
            var isOverTarget = (j >= 2); // 5-10ms and 10ms+ buckets
            ctx.fillStyle = isOverTarget ? '#ff4757' : CONFIG.colors.barFill;
            ctx.fillRect(x, y, barW - gap, barH);

            // Label
            ctx.fillStyle = CONFIG.colors.text;
            ctx.font = '10px "SF Mono", monospace';
            ctx.textAlign = 'center';
            ctx.fillText(formatNumber(data[j]), x + (barW - gap) / 2, y - 4);

            // Bucket label
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.fillText(buckets[j], x + (barW - gap) / 2, pad.top + plotH + 16);
        }

        // Target line annotation
        ctx.fillStyle = CONFIG.colors.codelTarget;
        ctx.font = '10px "SF Mono", monospace';
        ctx.textAlign = 'right';
        ctx.fillText('CoDel target: ' + CONFIG.codelTarget + 'ms', w - pad.right, pad.top + 12);
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderRateChart();
            renderCodelHistogram();
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
        if (state.container) state.container.innerHTML = '';
        state.priorityData = [];
        state.congestionMap = [];
        state.rateData = [];
        state.codelHistogram = [];
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

window.QoSPanel = QoSPanel;
console.log('qos-panel.js loaded');
