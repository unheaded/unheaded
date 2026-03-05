/**
 * lb-panel.js - Load Balancer Panel (App 04 - Monad CPU)
 * Visualization of load balancer backend health, traffic distribution, and failover events.
 *
 * Features:
 *   - Backend health grid: cards with status (green/yellow/red), connections, weight
 *   - Traffic distribution pie chart: per-backend traffic percentage
 *   - Failover timeline: event list with timestamps
 *   - Maglev table visualization: heat strip showing backend distribution
 *   - Stats: total backends, healthy %, total connections, rebalances
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var LoadBalancerPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxFailoverEvents: 50,
        maglevSlots: 128,

        healthColors: {
            healthy:   '#00d26a',
            degraded:  '#ff9800',
            unhealthy: '#ff4757',
            unknown:   '#6b6b7b'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            cardBg: '#12121f',
            eventRecovery: '#00d26a',
            eventFailover: '#ff4757'
        },
        backendColors: [
            '#4ecdc4', '#ffd700', '#ff6b6b', '#45b7d1', '#96ceb4',
            '#ff9800', '#e056fd', '#22a6b3', '#f9ca24', '#eb4d4b',
            '#6ab04c', '#30336b', '#be2edd', '#7ed6df', '#badc58'
        ]
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        pieCanvas: null,
        pieCtx: null,

        backends: [],          // [{ id, name, health, connections, weight, traffic_pct }]
        failoverEvents: [],    // [{ timestamp, type, from, to, reason }]
        maglevTable: [],       // [backend_id, ...] length = maglevSlots
        stats: { total: 0, healthy_pct: 0, total_connections: 0, rebalances: 0 },

        healthGrid: null,
        eventList: null,
        maglevStrip: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[LoadBalancerPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildStatsBar(container);
        buildLayout(container);

        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[LoadBalancerPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildStatsBar(parent) {
        var bar = document.createElement('div');
        bar.id = 'lb-stats-bar';
        bar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        bar.innerHTML =
            '<span>Backends: <strong id="lb-stat-total">--</strong></span>' +
            '<span>Healthy: <strong id="lb-stat-healthy" style="color:#00d26a">--%</strong></span>' +
            '<span>Connections: <strong id="lb-stat-conns">--</strong></span>' +
            '<span>Rebalances: <strong id="lb-stat-rebal" style="color:#ffd700">--</strong></span>';
        parent.appendChild(bar);
    }

    function buildLayout(parent) {
        var topGrid = document.createElement('div');
        topGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        // Health grid
        var healthSection = createSection('Backend Health');
        state.healthGrid = document.createElement('div');
        state.healthGrid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:8px;padding:4px;';
        healthSection.querySelector('.lb-body').appendChild(state.healthGrid);
        topGrid.appendChild(healthSection);

        // Pie chart
        var pieSection = createSection('Traffic Distribution');
        state.pieCanvas = document.createElement('canvas');
        state.pieCanvas.style.cssText = 'display:block;width:100%;';
        pieSection.querySelector('.lb-body').appendChild(state.pieCanvas);
        state.pieCtx = state.pieCanvas.getContext('2d');
        topGrid.appendChild(pieSection);

        parent.appendChild(topGrid);

        // Maglev visualization
        var magSection = createSection('Maglev Hash Table Distribution');
        state.maglevStrip = document.createElement('canvas');
        state.maglevStrip.style.cssText = 'display:block;width:100%;height:40px;';
        magSection.querySelector('.lb-body').appendChild(state.maglevStrip);
        parent.appendChild(magSection);

        // Failover timeline
        var failSection = createSection('Failover Timeline');
        state.eventList = document.createElement('div');
        state.eventList.style.cssText = 'max-height:250px;overflow-y:auto;';
        failSection.querySelector('.lb-body').appendChild(state.eventList);
        parent.appendChild(failSection);
    }

    function createSection(title) {
        var sec = document.createElement('div');
        sec.style.cssText = 'border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;overflow:hidden;margin-bottom:12px;';

        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent + ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;';
        hdr.textContent = title;
        sec.appendChild(hdr);

        var body = document.createElement('div');
        body.className = 'lb-body';
        body.style.cssText = 'padding:12px;background:' + CONFIG.colors.background + ';';
        sec.appendChild(body);

        return sec;
    }

    function resizeCanvases() {
        if (state.pieCanvas && state.pieCanvas.parentElement) {
            var dpr = window.devicePixelRatio || 1;
            var rect = state.pieCanvas.parentElement.getBoundingClientRect();
            var w = rect.width || 350;
            var h = 220;
            state.pieCanvas.width = w * dpr;
            state.pieCanvas.height = h * dpr;
            state.pieCanvas.style.width = w + 'px';
            state.pieCanvas.style.height = h + 'px';
            state.pieCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
        resizeMaglevStrip();
    }

    function resizeMaglevStrip() {
        if (!state.maglevStrip || !state.maglevStrip.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = state.maglevStrip.parentElement.getBoundingClientRect();
        var w = rect.width || 600;
        state.maglevStrip.width = w * dpr;
        state.maglevStrip.height = 40 * dpr;
        state.maglevStrip.style.width = w + 'px';
        state.maglevStrip.style.height = '40px';
        state.maglevStrip.getContext('2d').setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(lbData) {
        if (!state.initialized || !lbData) return;

        if (lbData.backends) state.backends = lbData.backends;
        if (lbData.failover_events) {
            state.failoverEvents = lbData.failover_events.slice(0, CONFIG.maxFailoverEvents);
        }
        if (lbData.maglev_table) state.maglevTable = lbData.maglev_table;
        if (lbData.stats) {
            state.stats = lbData.stats;
            updateStats();
        }

        renderHealthGrid();
        renderFailoverTimeline();
        renderMaglevStrip();
    }

    function updateStats() {
        var s = state.stats;
        setTextById('lb-stat-total', String(s.total || 0));
        setTextById('lb-stat-healthy', (s.healthy_pct || 0).toFixed(1) + '%');
        setTextById('lb-stat-conns', formatNumber(s.total_connections || 0));
        setTextById('lb-stat-rebal', String(s.rebalances || 0));
    }

    // ========================================================================
    // Rendering - Health Grid
    // ========================================================================
    function renderHealthGrid() {
        if (!state.healthGrid) return;
        state.healthGrid.innerHTML = '';

        for (var i = 0; i < state.backends.length; i++) {
            var b = state.backends[i];
            var healthColor = CONFIG.healthColors[b.health] || CONFIG.healthColors.unknown;
            var card = document.createElement('div');
            card.style.cssText = 'background:' + CONFIG.colors.cardBg + ';border:1px solid ' +
                CONFIG.colors.border + ';border-left:3px solid ' + healthColor +
                ';border-radius:4px;padding:10px;font-size:11px;';

            card.innerHTML =
                '<div style="font-weight:bold;color:' + CONFIG.colors.accent + ';margin-bottom:6px">' +
                (b.name || b.id || 'Backend ' + i) + '</div>' +
                '<div style="display:flex;justify-content:space-between;margin-bottom:4px">' +
                '<span style="color:' + CONFIG.colors.textMuted + '">Status</span>' +
                '<span style="color:' + healthColor + ';text-transform:uppercase;font-weight:bold;font-size:10px">' +
                (b.health || 'unknown') + '</span></div>' +
                '<div style="display:flex;justify-content:space-between;margin-bottom:4px">' +
                '<span style="color:' + CONFIG.colors.textMuted + '">Conns</span>' +
                '<span>' + formatNumber(b.connections || 0) + '</span></div>' +
                '<div style="display:flex;justify-content:space-between">' +
                '<span style="color:' + CONFIG.colors.textMuted + '">Weight</span>' +
                '<span>' + (b.weight || 0) + '</span></div>';

            state.healthGrid.appendChild(card);
        }
    }

    // ========================================================================
    // Rendering - Pie Chart
    // ========================================================================
    function renderPie() {
        var ctx = state.pieCtx;
        if (!ctx || !state.pieCanvas) return;
        var w = state.pieCanvas.width / (window.devicePixelRatio || 1);
        var h = state.pieCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        if (state.backends.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No backend data', w / 2, h / 2);
            return;
        }

        var total = 0;
        for (var i = 0; i < state.backends.length; i++) total += (state.backends[i].traffic_pct || 0);
        if (total === 0) total = 1;

        var cx = w * 0.38;
        var cy = h / 2;
        var radius = Math.min(cx - 16, cy - 16);
        var startAngle = -Math.PI / 2;

        for (var j = 0; j < state.backends.length; j++) {
            var slice = (state.backends[j].traffic_pct || 0) / total;
            var endAngle = startAngle + slice * Math.PI * 2;
            ctx.fillStyle = CONFIG.backendColors[j % CONFIG.backendColors.length];
            ctx.beginPath();
            ctx.moveTo(cx, cy);
            ctx.arc(cx, cy, radius, startAngle, endAngle);
            ctx.closePath();
            ctx.fill();
            startAngle = endAngle;
        }

        // Donut hole
        ctx.fillStyle = CONFIG.colors.background;
        ctx.beginPath();
        ctx.arc(cx, cy, radius * 0.5, 0, Math.PI * 2);
        ctx.fill();

        // Legend
        ctx.font = '10px "SF Mono", monospace';
        ctx.textAlign = 'left';
        var lx = w * 0.68;
        var ly = 16;
        for (var k = 0; k < state.backends.length && k < 12; k++) {
            ctx.fillStyle = CONFIG.backendColors[k % CONFIG.backendColors.length];
            ctx.fillRect(lx, ly + k * 16, 8, 8);
            ctx.fillStyle = CONFIG.colors.text;
            ctx.fillText((state.backends[k].name || 'B' + k) + ' ' +
                (state.backends[k].traffic_pct || 0).toFixed(1) + '%', lx + 14, ly + k * 16 + 8);
        }
    }

    // ========================================================================
    // Rendering - Maglev Strip
    // ========================================================================
    function renderMaglevStrip() {
        if (!state.maglevStrip) return;
        var ctx = state.maglevStrip.getContext('2d');
        var dpr = window.devicePixelRatio || 1;
        var w = state.maglevStrip.width / dpr;
        var h = state.maglevStrip.height / dpr;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var slots = state.maglevTable.length || CONFIG.maglevSlots;
        var slotW = w / slots;

        for (var i = 0; i < state.maglevTable.length; i++) {
            var backendIdx = state.maglevTable[i];
            ctx.fillStyle = CONFIG.backendColors[backendIdx % CONFIG.backendColors.length];
            ctx.fillRect(i * slotW, 4, Math.max(slotW - 0.5, 0.5), h - 8);
        }

        if (state.maglevTable.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '11px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No Maglev table data', w / 2, h / 2 + 4);
        }
    }

    // ========================================================================
    // Rendering - Failover Timeline
    // ========================================================================
    function renderFailoverTimeline() {
        if (!state.eventList) return;
        state.eventList.innerHTML = '';

        if (state.failoverEvents.length === 0) {
            state.eventList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:20px;font-size:12px">No failover events</div>';
            return;
        }

        for (var i = 0; i < state.failoverEvents.length; i++) {
            var ev = state.failoverEvents[i];
            var isRecovery = ev.type === 'recovery';
            var dotColor = isRecovery ? CONFIG.colors.eventRecovery : CONFIG.colors.eventFailover;

            var row = document.createElement('div');
            row.style.cssText = 'display:flex;align-items:center;gap:10px;padding:6px 8px;' +
                'border-bottom:1px solid ' + CONFIG.colors.border + ';font-size:11px;';

            row.innerHTML =
                '<span style="width:8px;height:8px;border-radius:50%;background:' + dotColor +
                ';flex-shrink:0;"></span>' +
                '<span style="width:70px;color:' + CONFIG.colors.textMuted + ';flex-shrink:0">' +
                formatTime(ev.timestamp) + '</span>' +
                '<span style="color:' + dotColor + ';width:70px;flex-shrink:0;text-transform:uppercase;font-weight:bold;font-size:10px">' +
                (ev.type || '--') + '</span>' +
                '<span style="flex:1">' + (ev.from || '--') + ' &rarr; ' + (ev.to || '--') + '</span>' +
                '<span style="color:' + CONFIG.colors.textMuted + ';flex-shrink:0">' +
                (ev.reason || '') + '</span>';

            state.eventList.appendChild(row);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() { renderPie(); state.animationFrame = requestAnimationFrame(tick); }
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

    function formatTime(ts) {
        if (!ts) return '--:--:--';
        var d = new Date(typeof ts === 'number' ? ts : Date.parse(ts));
        return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
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
        state.backends = [];
        state.failoverEvents = [];
        state.maglevTable = [];
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

window.LoadBalancerPanel = LoadBalancerPanel;
console.log('lb-panel.js loaded');
