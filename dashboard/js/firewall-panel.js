/**
 * firewall-panel.js - Firewall Monitoring Panel (App 03 - Monad CPU)
 * Visualization of firewall block rates, connection tracking, and policy violations.
 *
 * Features:
 *   - Block rate area chart (packets/sec blocked, last 1 hour)
 *   - Connection table: src/dst IP:port, protocol, state, duration, bytes (top 100)
 *   - Policy violation bar chart: top 20 blocked service pairs
 *   - State machine breakdown: pie chart (NEW, SYN_SENT, ESTABLISHED, CLOSING %)
 *   - Stats banner: total connections, blocked %, cache hit rate
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var FirewallPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxConnections: 100,
        blockRateHistory: 3600,  // 1 hour in seconds
        blockRateBuckets: 120,   // 120 buckets (30s each)
        topViolations: 20,
        padding: { top: 20, right: 16, bottom: 30, left: 50 },

        stateColors: {
            'NEW':         '#4ecdc4',
            'SYN_SENT':    '#ffd700',
            'ESTABLISHED': '#00d26a',
            'CLOSING':     '#ff9800',
            'CLOSED':      '#6b6b7b',
            'TIME_WAIT':   '#ff4757'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            blockArea: 'rgba(255, 71, 87, 0.3)',
            blockLine: '#ff4757',
            barFill: '#ff4757',
            barBg: 'rgba(255, 255, 255, 0.05)',
            gridLine: 'rgba(255, 215, 0, 0.06)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        blockRateCanvas: null,
        blockRateCtx: null,
        pieCanvas: null,
        pieCtx: null,

        blockRateData: [],    // array of { timestamp, blocked_pps }
        connections: [],       // top connections
        violations: [],        // { pair: 'svcA->svcB', count: N }
        stateCounts: {},       // { NEW: N, SYN_SENT: N, ... }
        stats: { total_connections: 0, blocked_pct: 0, cache_hit_rate: 0 },

        connTableBody: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[FirewallPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildStatsBar(container);
        buildLayout(container);

        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[FirewallPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildStatsBar(parent) {
        var bar = document.createElement('div');
        bar.id = 'fw-stats-bar';
        bar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        bar.innerHTML =
            '<span>Total Connections: <strong id="fw-stat-total">--</strong></span>' +
            '<span>Blocked: <strong id="fw-stat-blocked" style="color:#ff4757">--%</strong></span>' +
            '<span>Cache Hit Rate: <strong id="fw-stat-cache" style="color:#00d26a">--%</strong></span>';
        parent.appendChild(bar);
    }

    function buildLayout(parent) {
        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;';

        // Block rate chart
        var blockSection = createSection('Block Rate (packets/sec)', 'fw-block-section');
        state.blockRateCanvas = document.createElement('canvas');
        state.blockRateCanvas.style.cssText = 'display:block;width:100%;';
        blockSection.querySelector('.fw-section-body').appendChild(state.blockRateCanvas);
        state.blockRateCtx = state.blockRateCanvas.getContext('2d');
        grid.appendChild(blockSection);

        // State pie chart
        var pieSection = createSection('Connection State Distribution', 'fw-pie-section');
        state.pieCanvas = document.createElement('canvas');
        state.pieCanvas.style.cssText = 'display:block;width:100%;';
        pieSection.querySelector('.fw-section-body').appendChild(state.pieCanvas);
        state.pieCtx = state.pieCanvas.getContext('2d');
        grid.appendChild(pieSection);

        parent.appendChild(grid);

        // Violations bar chart
        var violationSection = createSection('Top Policy Violations (Blocked Service Pairs)', 'fw-violations');
        violationSection.querySelector('.fw-section-body').id = 'fw-violations-body';
        parent.appendChild(violationSection);

        // Connections table
        var connSection = createSection('Active Connections (Top 100)', 'fw-connections');
        var tableWrapper = document.createElement('div');
        tableWrapper.style.cssText = 'max-height:300px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Source</th>' +
            '<th style="padding:6px 8px;text-align:left">Destination</th>' +
            '<th style="padding:6px 8px;text-align:left">Proto</th>' +
            '<th style="padding:6px 8px;text-align:left">State</th>' +
            '<th style="padding:6px 8px;text-align:right">Duration</th>' +
            '<th style="padding:6px 8px;text-align:right">Bytes</th></tr></thead>';
        state.connTableBody = document.createElement('tbody');
        table.appendChild(state.connTableBody);
        tableWrapper.appendChild(table);
        connSection.querySelector('.fw-section-body').appendChild(tableWrapper);
        parent.appendChild(connSection);
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
        body.className = 'fw-section-body';
        body.style.cssText = 'padding:12px;background:' + CONFIG.colors.background + ';';
        sec.appendChild(body);

        return sec;
    }

    function resizeCanvases() {
        resizeCanvas(state.blockRateCanvas, state.blockRateCtx, 250);
        resizeCanvas(state.pieCanvas, state.pieCtx, 250);
    }

    function resizeCanvas(canvas, ctx, minH) {
        if (!canvas || !canvas.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = canvas.parentElement.getBoundingClientRect();
        var w = rect.width || 400;
        var h = Math.max(minH, 200);
        canvas.width = w * dpr;
        canvas.height = h * dpr;
        canvas.style.width = w + 'px';
        canvas.style.height = h + 'px';
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(firewallData) {
        if (!state.initialized || !firewallData) return;

        if (firewallData.block_rate != null) {
            state.blockRateData.push({
                timestamp: firewallData.timestamp || Date.now(),
                blocked_pps: firewallData.block_rate
            });
            if (state.blockRateData.length > CONFIG.blockRateBuckets) state.blockRateData.shift();
        }

        if (firewallData.connections) {
            state.connections = firewallData.connections.slice(0, CONFIG.maxConnections);
        }
        if (firewallData.violations) {
            state.violations = firewallData.violations.slice(0, CONFIG.topViolations);
        }
        if (firewallData.state_counts) {
            state.stateCounts = firewallData.state_counts;
        }
        if (firewallData.stats) {
            state.stats = firewallData.stats;
            updateStats();
        }

        renderConnections();
        renderViolations();
    }

    function updateStats() {
        var s = state.stats;
        setTextById('fw-stat-total', formatNumber(s.total_connections || 0));
        setTextById('fw-stat-blocked', (s.blocked_pct || 0).toFixed(1) + '%');
        setTextById('fw-stat-cache', (s.cache_hit_rate || 0).toFixed(1) + '%');
    }

    // ========================================================================
    // Rendering - Block Rate Chart
    // ========================================================================
    function renderBlockRate() {
        var ctx = state.blockRateCtx;
        if (!ctx || !state.blockRateCanvas) return;
        var w = state.blockRateCanvas.width / (window.devicePixelRatio || 1);
        var h = state.blockRateCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.blockRateData;
        if (data.length < 2) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for block rate data...', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = 0;
        for (var i = 0; i < data.length; i++) {
            if (data[i].blocked_pps > yMax) yMax = data[i].blocked_pps;
        }
        yMax = Math.ceil(yMax * 1.2) || 100;

        // Grid
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var g = 1; g < 5; g++) {
            var gy = pad.top + plotH * (1 - g / 5);
            ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + plotW, gy); ctx.stroke();
        }

        // Area fill
        ctx.fillStyle = CONFIG.colors.blockArea;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top + plotH);
        for (var j = 0; j < data.length; j++) {
            var x = pad.left + (j / (data.length - 1)) * plotW;
            var y = pad.top + plotH - (data[j].blocked_pps / yMax) * plotH;
            ctx.lineTo(x, y);
        }
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.closePath();
        ctx.fill();

        // Line
        ctx.strokeStyle = CONFIG.colors.blockLine;
        ctx.lineWidth = 2;
        ctx.beginPath();
        for (var k = 0; k < data.length; k++) {
            var lx = pad.left + (k / (data.length - 1)) * plotW;
            var ly = pad.top + plotH - (data[k].blocked_pps / yMax) * plotH;
            if (k === 0) ctx.moveTo(lx, ly); else ctx.lineTo(lx, ly);
        }
        ctx.stroke();

        // Axis labels
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var a = 0; a <= 5; a++) {
            var aVal = (yMax * a / 5).toFixed(0);
            var ay = pad.top + plotH * (1 - a / 5);
            ctx.fillText(aVal, pad.left - 4, ay + 4);
        }
        ctx.textAlign = 'center';
        ctx.fillText('pps', pad.left + plotW / 2, pad.top + plotH + 20);
    }

    // ========================================================================
    // Rendering - Pie Chart (State Machine)
    // ========================================================================
    function renderPie() {
        var ctx = state.pieCtx;
        if (!ctx || !state.pieCanvas) return;
        var w = state.pieCanvas.width / (window.devicePixelRatio || 1);
        var h = state.pieCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var counts = state.stateCounts;
        var keys = Object.keys(counts);
        if (keys.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No state data', w / 2, h / 2);
            return;
        }

        var total = 0;
        for (var i = 0; i < keys.length; i++) total += counts[keys[i]];
        if (total === 0) return;

        var cx = w * 0.4;
        var cy = h / 2;
        var radius = Math.min(cx - 20, cy - 20);
        var startAngle = -Math.PI / 2;

        for (var j = 0; j < keys.length; j++) {
            var slice = counts[keys[j]] / total;
            var endAngle = startAngle + slice * Math.PI * 2;

            ctx.fillStyle = CONFIG.stateColors[keys[j]] || '#6b6b7b';
            ctx.beginPath();
            ctx.moveTo(cx, cy);
            ctx.arc(cx, cy, radius, startAngle, endAngle);
            ctx.closePath();
            ctx.fill();

            startAngle = endAngle;
        }

        // Inner circle (donut)
        ctx.fillStyle = CONFIG.colors.background;
        ctx.beginPath();
        ctx.arc(cx, cy, radius * 0.55, 0, Math.PI * 2);
        ctx.fill();

        // Total in center
        ctx.fillStyle = CONFIG.colors.text;
        ctx.font = 'bold 16px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(formatNumber(total), cx, cy + 6);

        // Legend
        ctx.font = '10px "SF Mono", monospace';
        ctx.textAlign = 'left';
        var legendX = w * 0.7;
        var legendY = 30;
        for (var k = 0; k < keys.length; k++) {
            var pct = ((counts[keys[k]] / total) * 100).toFixed(1);
            ctx.fillStyle = CONFIG.stateColors[keys[k]] || '#6b6b7b';
            ctx.fillRect(legendX, legendY + k * 20, 10, 10);
            ctx.fillStyle = CONFIG.colors.text;
            ctx.fillText(keys[k] + ' (' + pct + '%)', legendX + 16, legendY + k * 20 + 9);
        }
    }

    // ========================================================================
    // Rendering - Connections Table
    // ========================================================================
    function renderConnections() {
        if (!state.connTableBody) return;
        state.connTableBody.innerHTML = '';

        for (var i = 0; i < state.connections.length; i++) {
            var c = state.connections[i];
            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';

            var stateColor = CONFIG.stateColors[c.state] || CONFIG.colors.textMuted;
            tr.innerHTML =
                '<td style="padding:4px 8px">' + (c.src || '--') + '</td>' +
                '<td style="padding:4px 8px">' + (c.dst || '--') + '</td>' +
                '<td style="padding:4px 8px">' + (c.protocol || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + stateColor + '">' + (c.state || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + formatDuration(c.duration) + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + formatBytes(c.bytes) + '</td>';
            state.connTableBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Rendering - Violations Bar Chart
    // ========================================================================
    function renderViolations() {
        var body = document.getElementById('fw-violations-body');
        if (!body) return;
        body.innerHTML = '';

        if (state.violations.length === 0) {
            body.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:20px;font-size:12px">No violations recorded</div>';
            return;
        }

        var maxCount = 0;
        for (var i = 0; i < state.violations.length; i++) {
            if (state.violations[i].count > maxCount) maxCount = state.violations[i].count;
        }

        for (var j = 0; j < state.violations.length; j++) {
            var v = state.violations[j];
            var pct = maxCount > 0 ? (v.count / maxCount * 100) : 0;
            var row = document.createElement('div');
            row.style.cssText = 'display:flex;align-items:center;gap:8px;padding:3px 0;font-size:11px;';
            row.innerHTML =
                '<span style="width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:' +
                CONFIG.colors.text + '">' + (v.pair || '--') + '</span>' +
                '<div style="flex:1;height:14px;background:' + CONFIG.colors.barBg + ';border-radius:2px;overflow:hidden">' +
                '<div style="width:' + pct + '%;height:100%;background:' + CONFIG.colors.barFill +
                ';border-radius:2px;transition:width 0.3s"></div></div>' +
                '<span style="width:50px;text-align:right;color:' + CONFIG.colors.textMuted + '">' +
                formatNumber(v.count) + '</span>';
            body.appendChild(row);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderBlockRate();
            renderPie();
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

    function formatBytes(b) {
        if (b == null) return '--';
        if (b >= 1073741824) return (b / 1073741824).toFixed(1) + ' GB';
        if (b >= 1048576) return (b / 1048576).toFixed(1) + ' MB';
        if (b >= 1024) return (b / 1024).toFixed(1) + ' KB';
        return b + ' B';
    }

    function formatDuration(ms) {
        if (ms == null) return '--';
        if (ms >= 3600000) return (ms / 3600000).toFixed(1) + 'h';
        if (ms >= 60000) return (ms / 60000).toFixed(1) + 'm';
        if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
        return ms + 'ms';
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
        state.blockRateData = [];
        state.connections = [];
        state.violations = [];
        state.stateCounts = {};
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

window.FirewallPanel = FirewallPanel;
console.log('firewall-panel.js loaded');
