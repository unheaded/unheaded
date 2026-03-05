/**
 * failover-panel.js - Failover Management Panel (App 08 - Monad CPU)
 * Endpoint health monitoring, failover timeline, and blast radius visualization.
 *
 * Features:
 *   - Endpoint health grid: cards with score (0-100), color coded (green>75, yellow 50-75, red<50)
 *     Each card: circuit state badge, active flow count, health trend sparkline
 *   - Failover timeline: vertical event stream with failover + recovery events
 *   - Blast radius visualization: affected flow percentage per failover
 *   - Backup roster manager: table of primary -> backup mappings, editable
 *   - Circuit breaker summary: counts of CLOSED/OPEN/HALF_OPEN
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var FailoverPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxTimelineEvents: 100,
        sparklinePoints: 20,

        healthColors: function(score) {
            if (score > 75) return '#00d26a';
            if (score > 50) return '#ffd700';
            return '#ff4757';
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
            cardBg: '#12121f',
            failoverEvent: '#ff4757',
            recoveryEvent: '#00d26a',
            blastRadius: 'rgba(255, 71, 87, 0.3)',
            sparkline: '#4ecdc4',
            sparklineFill: 'rgba(78, 205, 196, 0.15)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,

        endpoints: [],       // [{ id, name, score, circuit_state, active_flows, health_trend: [scores] }]
        timeline: [],        // [{ timestamp, type: 'failover'|'recovery', from, to, reason, blast_pct }]
        backupRoster: [],    // [{ primary, backup, priority, editable }]
        circuitSummary: { CLOSED: 0, OPEN: 0, HALF_OPEN: 0 },

        healthGrid: null,
        timelineList: null,
        rosterBody: null,
        cbSummary: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[FailoverPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);

        state.initialized = true;
        console.log('[FailoverPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Circuit breaker summary bar
        state.cbSummary = document.createElement('div');
        state.cbSummary.style.cssText = 'display:flex;gap:16px;padding:12px 16px;background:' +
            CONFIG.colors.headerBg + ';border:1px solid ' + CONFIG.colors.border +
            ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        state.cbSummary.innerHTML =
            '<span style="font-weight:bold;color:' + CONFIG.colors.accent + '">Circuit Breakers:</span>' +
            '<span>CLOSED: <strong id="fo-cb-closed" style="color:#00d26a">--</strong></span>' +
            '<span>OPEN: <strong id="fo-cb-open" style="color:#ff4757">--</strong></span>' +
            '<span>HALF_OPEN: <strong id="fo-cb-half" style="color:#ff9800">--</strong></span>';
        parent.appendChild(state.cbSummary);

        // Health grid
        var healthSection = createSection('Endpoint Health');
        state.healthGrid = document.createElement('div');
        state.healthGrid.style.cssText = 'display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:10px;';
        healthSection.body.appendChild(state.healthGrid);
        parent.appendChild(healthSection.el);

        // Mid: timeline + blast radius
        var midGrid = document.createElement('div');
        midGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var timeSection = createSection('Failover Timeline');
        state.timelineList = document.createElement('div');
        state.timelineList.style.cssText = 'max-height:300px;overflow-y:auto;padding-left:20px;' +
            'border-left:2px solid ' + CONFIG.colors.border + ';';
        timeSection.body.appendChild(state.timelineList);
        midGrid.appendChild(timeSection.el);

        var rosterSection = createSection('Backup Roster (Primary -> Backup)');
        buildRosterTable(rosterSection.body);
        midGrid.appendChild(rosterSection.el);

        parent.appendChild(midGrid);
    }

    function buildRosterTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:300px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Primary</th>' +
            '<th style="padding:6px 8px;text-align:left">Backup</th>' +
            '<th style="padding:6px 8px;text-align:center">Priority</th>' +
            '<th style="padding:6px 8px;text-align:center">Action</th></tr></thead>';
        state.rosterBody = document.createElement('tbody');
        table.appendChild(state.rosterBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
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

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(failoverData) {
        if (!state.initialized || !failoverData) return;

        if (failoverData.endpoints) {
            state.endpoints = failoverData.endpoints;
            renderHealthGrid();
        }
        if (failoverData.timeline) {
            state.timeline = failoverData.timeline.slice(0, CONFIG.maxTimelineEvents);
            renderTimeline();
        }
        if (failoverData.backup_roster) {
            state.backupRoster = failoverData.backup_roster;
            renderRoster();
        }
        if (failoverData.circuit_summary) {
            state.circuitSummary = failoverData.circuit_summary;
            updateCBSummary();
        }
    }

    function updateCBSummary() {
        var s = state.circuitSummary;
        setTextById('fo-cb-closed', String(s.CLOSED || 0));
        setTextById('fo-cb-open', String(s.OPEN || 0));
        setTextById('fo-cb-half', String(s.HALF_OPEN || 0));
    }

    // ========================================================================
    // Rendering - Health Grid
    // ========================================================================
    function renderHealthGrid() {
        if (!state.healthGrid) return;
        state.healthGrid.innerHTML = '';

        for (var i = 0; i < state.endpoints.length; i++) {
            var ep = state.endpoints[i];
            var score = ep.score || 0;
            var scoreColor = CONFIG.healthColors(score);
            var cbColor = CONFIG.circuitColors[ep.circuit_state] || CONFIG.colors.textMuted;

            var card = document.createElement('div');
            card.style.cssText = 'background:' + CONFIG.colors.cardBg + ';border:1px solid ' +
                CONFIG.colors.border + ';border-radius:6px;padding:12px;position:relative;';

            // Header row
            var header = document.createElement('div');
            header.style.cssText = 'display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;';
            header.innerHTML =
                '<span style="font-weight:bold;color:' + CONFIG.colors.accent + ';font-size:12px">' +
                (ep.name || ep.id || 'Endpoint ' + i) + '</span>' +
                '<span style="display:inline-block;padding:2px 6px;background:' + cbColor +
                ';color:#0f0f1a;font-size:9px;font-weight:bold;border-radius:3px">' +
                (ep.circuit_state || '--') + '</span>';
            card.appendChild(header);

            // Score gauge
            var scoreRow = document.createElement('div');
            scoreRow.style.cssText = 'display:flex;align-items:center;gap:8px;margin-bottom:8px;';
            scoreRow.innerHTML =
                '<div style="font-size:24px;font-weight:bold;color:' + scoreColor + '">' + score + '</div>' +
                '<div style="flex:1">' +
                '<div style="height:6px;background:rgba(255,255,255,0.05);border-radius:3px;overflow:hidden">' +
                '<div style="width:' + score + '%;height:100%;background:' + scoreColor +
                ';border-radius:3px;transition:width 0.5s"></div></div>' +
                '<div style="font-size:10px;color:' + CONFIG.colors.textMuted + ';margin-top:2px">Health Score</div></div>';
            card.appendChild(scoreRow);

            // Flows
            var flowsRow = document.createElement('div');
            flowsRow.style.cssText = 'font-size:10px;color:' + CONFIG.colors.textMuted + ';margin-bottom:8px;';
            flowsRow.textContent = 'Active Flows: ' + formatNumber(ep.active_flows || 0);
            card.appendChild(flowsRow);

            // Sparkline
            if (ep.health_trend && ep.health_trend.length > 1) {
                var sparkCanvas = document.createElement('canvas');
                sparkCanvas.width = 180;
                sparkCanvas.height = 30;
                sparkCanvas.style.cssText = 'width:100%;height:30px;';
                card.appendChild(sparkCanvas);
                drawSparkline(sparkCanvas, ep.health_trend);
            }

            state.healthGrid.appendChild(card);
        }
    }

    function drawSparkline(canvas, data) {
        var ctx = canvas.getContext('2d');
        var w = canvas.width;
        var h = canvas.height;

        var max = 100;
        var min = 0;

        // Fill
        ctx.fillStyle = CONFIG.colors.sparklineFill;
        ctx.beginPath();
        ctx.moveTo(0, h);
        for (var i = 0; i < data.length; i++) {
            var x = (i / (data.length - 1)) * w;
            var y = h - ((data[i] - min) / (max - min)) * h;
            ctx.lineTo(x, y);
        }
        ctx.lineTo(w, h);
        ctx.closePath();
        ctx.fill();

        // Line
        ctx.strokeStyle = CONFIG.colors.sparkline;
        ctx.lineWidth = 1.5;
        ctx.lineJoin = 'round';
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var lx = (j / (data.length - 1)) * w;
            var ly = h - ((data[j] - min) / (max - min)) * h;
            if (j === 0) ctx.moveTo(lx, ly); else ctx.lineTo(lx, ly);
        }
        ctx.stroke();
    }

    // ========================================================================
    // Rendering - Timeline
    // ========================================================================
    function renderTimeline() {
        if (!state.timelineList) return;
        state.timelineList.innerHTML = '';

        if (state.timeline.length === 0) {
            state.timelineList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:20px;font-size:12px">No failover events</div>';
            return;
        }

        for (var i = 0; i < state.timeline.length; i++) {
            var ev = state.timeline[i];
            var isFailover = ev.type === 'failover';
            var dotColor = isFailover ? CONFIG.colors.failoverEvent : CONFIG.colors.recoveryEvent;

            var entry = document.createElement('div');
            entry.style.cssText = 'position:relative;padding:8px 0 8px 24px;font-size:11px;' +
                'border-bottom:1px solid ' + CONFIG.colors.border + ';';

            // Dot on the timeline line
            entry.innerHTML =
                '<span style="position:absolute;left:-6px;top:10px;width:10px;height:10px;border-radius:50%;' +
                'background:' + dotColor + ';border:2px solid ' + CONFIG.colors.background + '"></span>' +
                '<div style="display:flex;justify-content:space-between;align-items:center">' +
                '<span style="color:' + dotColor + ';font-weight:bold;text-transform:uppercase;font-size:10px">' +
                (ev.type || '--') + '</span>' +
                '<span style="color:' + CONFIG.colors.textMuted + '">' + formatTime(ev.timestamp) + '</span></div>' +
                '<div style="margin-top:4px">' + (ev.from || '--') + ' &rarr; ' + (ev.to || '--') + '</div>' +
                (ev.reason ? '<div style="color:' + CONFIG.colors.textMuted + ';margin-top:2px;font-size:10px">' +
                    ev.reason + '</div>' : '') +
                (ev.blast_pct != null ?
                    '<div style="margin-top:4px">' +
                    '<div style="height:4px;background:rgba(255,255,255,0.05);border-radius:2px;overflow:hidden">' +
                    '<div style="width:' + ev.blast_pct + '%;height:100%;background:' + CONFIG.colors.blastRadius +
                    ';border-radius:2px"></div></div>' +
                    '<span style="font-size:9px;color:' + CONFIG.colors.textMuted + '">Blast radius: ' +
                    ev.blast_pct.toFixed(1) + '% flows affected</span></div>' : '');

            state.timelineList.appendChild(entry);
        }
    }

    // ========================================================================
    // Rendering - Backup Roster
    // ========================================================================
    function renderRoster() {
        if (!state.rosterBody) return;
        state.rosterBody.innerHTML = '';

        for (var i = 0; i < state.backupRoster.length; i++) {
            var r = state.backupRoster[i];
            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px;font-weight:bold">' + (r.primary || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.sparkline + '">' + (r.backup || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:center">' + (r.priority || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:center">' +
                (r.editable ? '<button style="padding:2px 8px;background:transparent;color:' + CONFIG.colors.accent +
                    ';border:1px solid ' + CONFIG.colors.border + ';border-radius:3px;cursor:pointer;font-size:10px;' +
                    'font-family:inherit">Edit</button>' : '<span style="color:' + CONFIG.colors.textMuted + '">--</span>') +
                '</td>';
            state.rosterBody.appendChild(tr);
        }
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

    // ========================================================================
    // Cleanup
    // ========================================================================
    function destroy() {
        if (state.container) state.container.innerHTML = '';
        state.endpoints = [];
        state.timeline = [];
        state.backupRoster = [];
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

window.FailoverPanel = FailoverPanel;
console.log('failover-panel.js loaded');
