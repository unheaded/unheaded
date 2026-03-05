/**
 * compliance-panel.js - Compliance Monitoring Panel (App 09 - Monad CPU)
 * Data sovereignty, PII egress tracking, and policy violation monitoring.
 *
 * Features:
 *   - Compliance score gauge: large circular gauge (0-100%)
 *   - Policy violation heatmap: service matrix (rows=src, cols=dst), cell color = blocked count
 *   - Geographic zone map: simplified zone visualization with PII egress attempts
 *   - Real-time violations table: timestamp, src_svc, dst_svc, reason, action, classification
 *   - Audit trail search: filter by trace_id, time range, violation type
 *   - Data classification legend: PUBLIC, INTERNAL, SENSITIVE, SOVEREIGN_PII
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var CompliancePanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxViolations: 200,
        maxAudit: 500,

        classificationColors: {
            'PUBLIC':        '#6b6b7b',
            'INTERNAL':      '#45b7d1',
            'SENSITIVE':     '#ffd700',
            'SOVEREIGN_PII': '#ff4757'
        },
        actionColors: {
            'BLOCKED':  '#ff4757',
            'ALLOWED':  '#00d26a',
            'FLAGGED':  '#ff9800',
            'AUDITED':  '#45b7d1'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            cardBg: '#12121f',
            gaugeTrack: 'rgba(255, 255, 255, 0.05)',
            gaugeGood: '#00d26a',
            gaugeWarn: '#ffd700',
            gaugeBad: '#ff4757',
            heatmapLow: [15, 15, 30],
            heatmapHigh: [255, 71, 87],
            zoneOk: 'rgba(0, 210, 106, 0.2)',
            zoneAlert: 'rgba(255, 71, 87, 0.3)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        gaugeCanvas: null,
        gaugeCtx: null,
        heatmapCanvas: null,
        heatmapCtx: null,

        complianceScore: 0,    // 0-100
        violationMatrix: null, // { services: [names], matrix: [[counts]] }
        zones: [],             // [{ name, status: 'ok'|'alert', pii_attempts }]
        violations: [],        // [{ timestamp, src_svc, dst_svc, reason, action, classification }]
        auditTrail: [],        // extended violations with trace_id
        auditFilter: { trace_id: '', violation_type: '', time_start: '', time_end: '' },

        violationsBody: null,
        auditBody: null,
        zonesGrid: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[CompliancePanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[CompliancePanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Classification legend
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:16px;padding:10px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:11px;';
        legend.innerHTML = '<span style="color:' + CONFIG.colors.accent + ';font-weight:bold">DATA CLASSIFICATION:</span>';
        var classes = ['PUBLIC', 'INTERNAL', 'SENSITIVE', 'SOVEREIGN_PII'];
        for (var c = 0; c < classes.length; c++) {
            legend.innerHTML +=
                '<span><span style="display:inline-block;width:12px;height:12px;background:' +
                CONFIG.classificationColors[classes[c]] + ';border-radius:2px;margin-right:4px;vertical-align:middle"></span>' +
                classes[c] + '</span>';
        }
        parent.appendChild(legend);

        // Top: gauge + heatmap + zones
        var topGrid = document.createElement('div');
        topGrid.style.cssText = 'display:grid;grid-template-columns:200px 1fr 250px;gap:12px;margin-bottom:12px;';

        // Gauge
        var gaugeSection = createSection('Compliance Score');
        state.gaugeCanvas = document.createElement('canvas');
        state.gaugeCanvas.style.cssText = 'display:block;margin:0 auto;';
        gaugeSection.body.appendChild(state.gaugeCanvas);
        state.gaugeCtx = state.gaugeCanvas.getContext('2d');
        topGrid.appendChild(gaugeSection.el);

        // Heatmap
        var heatSection = createSection('Policy Violation Matrix (src x dst)');
        state.heatmapCanvas = document.createElement('canvas');
        state.heatmapCanvas.style.cssText = 'display:block;width:100%;';
        heatSection.body.appendChild(state.heatmapCanvas);
        state.heatmapCtx = state.heatmapCanvas.getContext('2d');
        topGrid.appendChild(heatSection.el);

        // Zone map
        var zoneSection = createSection('Geographic Zones');
        state.zonesGrid = document.createElement('div');
        state.zonesGrid.style.cssText = 'display:flex;flex-direction:column;gap:6px;';
        zoneSection.body.appendChild(state.zonesGrid);
        topGrid.appendChild(zoneSection.el);

        parent.appendChild(topGrid);

        // Violations table
        var violSection = createSection('Real-Time Policy Violations');
        buildViolationsTable(violSection.body);
        parent.appendChild(violSection.el);

        // Audit trail search
        var auditSection = createSection('Audit Trail Search');
        buildAuditSearch(auditSection.body);
        parent.appendChild(auditSection.el);
    }

    function buildViolationsTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:250px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Timestamp</th>' +
            '<th style="padding:6px 8px;text-align:left">Source</th>' +
            '<th style="padding:6px 8px;text-align:left">Destination</th>' +
            '<th style="padding:6px 8px;text-align:left">Reason</th>' +
            '<th style="padding:6px 8px;text-align:left">Action</th>' +
            '<th style="padding:6px 8px;text-align:left">Class</th></tr></thead>';
        state.violationsBody = document.createElement('tbody');
        table.appendChild(state.violationsBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
    }

    function buildAuditSearch(parent) {
        var filterRow = document.createElement('div');
        filterRow.style.cssText = 'display:flex;gap:8px;margin-bottom:8px;flex-wrap:wrap;';

        var fields = [
            { key: 'trace_id', placeholder: 'Trace ID' },
            { key: 'violation_type', placeholder: 'Violation Type' },
            { key: 'time_start', placeholder: 'Start (ISO)' },
            { key: 'time_end', placeholder: 'End (ISO)' }
        ];

        for (var i = 0; i < fields.length; i++) {
            var input = document.createElement('input');
            input.type = 'text';
            input.placeholder = fields[i].placeholder;
            input.dataset.auditKey = fields[i].key;
            input.style.cssText = 'flex:1;min-width:120px;padding:6px 10px;background:#0f0f1a;color:#e0e0e0;' +
                'border:1px solid ' + CONFIG.colors.border + ';border-radius:4px;font-size:11px;font-family:"SF Mono",monospace;';
            input.addEventListener('input', function(e) {
                state.auditFilter[e.target.dataset.auditKey] = e.target.value;
                renderAuditTrail();
            });
            filterRow.appendChild(input);
        }
        parent.appendChild(filterRow);

        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:200px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Trace ID</th>' +
            '<th style="padding:6px 8px;text-align:left">Timestamp</th>' +
            '<th style="padding:6px 8px;text-align:left">Type</th>' +
            '<th style="padding:6px 8px;text-align:left">Source</th>' +
            '<th style="padding:6px 8px;text-align:left">Action</th></tr></thead>';
        state.auditBody = document.createElement('tbody');
        table.appendChild(state.auditBody);
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

    function resizeCanvases() {
        // Gauge
        if (state.gaugeCanvas) {
            var dpr = window.devicePixelRatio || 1;
            state.gaugeCanvas.width = 160 * dpr;
            state.gaugeCanvas.height = 160 * dpr;
            state.gaugeCanvas.style.width = '160px';
            state.gaugeCanvas.style.height = '160px';
            state.gaugeCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
        // Heatmap
        if (state.heatmapCanvas && state.heatmapCanvas.parentElement) {
            var dpr2 = window.devicePixelRatio || 1;
            var rect = state.heatmapCanvas.parentElement.getBoundingClientRect();
            var w = rect.width || 400;
            var h = 250;
            state.heatmapCanvas.width = w * dpr2;
            state.heatmapCanvas.height = h * dpr2;
            state.heatmapCanvas.style.width = w + 'px';
            state.heatmapCanvas.style.height = h + 'px';
            state.heatmapCtx.setTransform(dpr2, 0, 0, dpr2, 0, 0);
        }
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(complianceData) {
        if (!state.initialized || !complianceData) return;

        if (complianceData.score != null) state.complianceScore = complianceData.score;
        if (complianceData.violation_matrix) state.violationMatrix = complianceData.violation_matrix;
        if (complianceData.zones) { state.zones = complianceData.zones; renderZones(); }
        if (complianceData.violations) {
            state.violations = complianceData.violations.slice(0, CONFIG.maxViolations);
            renderViolations();
        }
        if (complianceData.audit_trail) {
            state.auditTrail = complianceData.audit_trail.slice(0, CONFIG.maxAudit);
            renderAuditTrail();
        }
    }

    // ========================================================================
    // Rendering - Gauge
    // ========================================================================
    function renderGauge() {
        var ctx = state.gaugeCtx;
        if (!ctx || !state.gaugeCanvas) return;
        var size = 160;
        var cx = size / 2;
        var cy = size / 2;
        var radius = 60;
        var lineWidth = 10;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, size, size);

        // Track
        ctx.strokeStyle = CONFIG.colors.gaugeTrack;
        ctx.lineWidth = lineWidth;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.arc(cx, cy, radius, 0.75 * Math.PI, 2.25 * Math.PI);
        ctx.stroke();

        // Score arc
        var score = state.complianceScore;
        var gaugeColor = score > 75 ? CONFIG.colors.gaugeGood :
            (score > 50 ? CONFIG.colors.gaugeWarn : CONFIG.colors.gaugeBad);
        var scoreAngle = 0.75 * Math.PI + (score / 100) * 1.5 * Math.PI;

        ctx.strokeStyle = gaugeColor;
        ctx.lineWidth = lineWidth;
        ctx.lineCap = 'round';
        ctx.beginPath();
        ctx.arc(cx, cy, radius, 0.75 * Math.PI, scoreAngle);
        ctx.stroke();

        // Score text
        ctx.fillStyle = gaugeColor;
        ctx.font = 'bold 28px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(score + '%', cx, cy + 8);

        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillText('COMPLIANCE', cx, cy + 24);
    }

    // ========================================================================
    // Rendering - Heatmap
    // ========================================================================
    function renderHeatmap() {
        var ctx = state.heatmapCtx;
        if (!ctx || !state.heatmapCanvas || !state.violationMatrix) return;
        var w = state.heatmapCanvas.width / (window.devicePixelRatio || 1);
        var h = state.heatmapCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var services = state.violationMatrix.services || [];
        var matrix = state.violationMatrix.matrix || [];
        if (services.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No violation data', w / 2, h / 2);
            return;
        }

        var labelW = 80;
        var labelH = 30;
        var plotW = w - labelW - 10;
        var plotH = h - labelH - 10;
        var cellW = plotW / services.length;
        var cellH = plotH / services.length;

        // Find max
        var maxCount = 0;
        for (var r = 0; r < matrix.length; r++) {
            for (var c = 0; c < matrix[r].length; c++) {
                if (matrix[r][c] > maxCount) maxCount = matrix[r][c];
            }
        }
        maxCount = maxCount || 1;

        // Draw cells
        for (var row = 0; row < services.length; row++) {
            for (var col = 0; col < services.length; col++) {
                var val = (matrix[row] && matrix[row][col]) || 0;
                var t = val / maxCount;
                var rgb = interpolate(CONFIG.colors.heatmapLow, CONFIG.colors.heatmapHigh, t);
                ctx.fillStyle = 'rgb(' + rgb[0] + ',' + rgb[1] + ',' + rgb[2] + ')';
                ctx.fillRect(labelW + col * cellW + 1, labelH + row * cellH + 1, cellW - 2, cellH - 2);

                // Count text for large cells
                if (cellW > 25 && cellH > 15 && val > 0) {
                    ctx.fillStyle = t > 0.5 ? '#fff' : CONFIG.colors.textMuted;
                    ctx.font = '9px "SF Mono", monospace';
                    ctx.textAlign = 'center';
                    ctx.fillText(String(val), labelW + col * cellW + cellW / 2, labelH + row * cellH + cellH / 2 + 3);
                }
            }
        }

        // Row labels (src)
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'right';
        ctx.fillStyle = CONFIG.colors.textMuted;
        for (var rl = 0; rl < services.length; rl++) {
            var label = services[rl].length > 10 ? services[rl].substring(0, 10) : services[rl];
            ctx.fillText(label, labelW - 4, labelH + rl * cellH + cellH / 2 + 3);
        }

        // Col labels (dst)
        ctx.textAlign = 'center';
        for (var cl = 0; cl < services.length; cl++) {
            var cLabel = services[cl].length > 6 ? services[cl].substring(0, 6) : services[cl];
            ctx.fillText(cLabel, labelW + cl * cellW + cellW / 2, labelH - 6);
        }
    }

    function interpolate(c1, c2, t) {
        return [
            Math.round(c1[0] + (c2[0] - c1[0]) * t),
            Math.round(c1[1] + (c2[1] - c1[1]) * t),
            Math.round(c1[2] + (c2[2] - c1[2]) * t)
        ];
    }

    // ========================================================================
    // Rendering - Zones
    // ========================================================================
    function renderZones() {
        if (!state.zonesGrid) return;
        state.zonesGrid.innerHTML = '';

        for (var i = 0; i < state.zones.length; i++) {
            var z = state.zones[i];
            var isAlert = z.status === 'alert';
            var bgColor = isAlert ? CONFIG.colors.zoneAlert : CONFIG.colors.zoneOk;
            var borderColor = isAlert ? '#ff4757' : '#00d26a';

            var card = document.createElement('div');
            card.style.cssText = 'padding:8px 12px;background:' + bgColor +
                ';border:1px solid ' + borderColor + ';border-radius:4px;font-size:11px;' +
                'display:flex;justify-content:space-between;align-items:center;';
            card.innerHTML =
                '<span style="font-weight:bold">' + (z.name || '--') + '</span>' +
                '<span>' + (isAlert ? '<span style="color:#ff4757;font-weight:bold">' +
                    (z.pii_attempts || 0) + ' PII attempts</span>' :
                    '<span style="color:#00d26a">OK</span>') + '</span>';
            state.zonesGrid.appendChild(card);
        }
    }

    // ========================================================================
    // Rendering - Violations
    // ========================================================================
    function renderViolations() {
        if (!state.violationsBody) return;
        state.violationsBody.innerHTML = '';

        for (var i = 0; i < state.violations.length; i++) {
            var v = state.violations[i];
            var actionColor = CONFIG.actionColors[v.action] || CONFIG.colors.textMuted;
            var classColor = CONFIG.classificationColors[v.classification] || CONFIG.colors.textMuted;

            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px;color:' + CONFIG.colors.textMuted + '">' + formatTime(v.timestamp) + '</td>' +
                '<td style="padding:4px 8px">' + (v.src_svc || '--') + '</td>' +
                '<td style="padding:4px 8px">' + (v.dst_svc || '--') + '</td>' +
                '<td style="padding:4px 8px;font-size:10px">' + (v.reason || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + actionColor + ';font-weight:bold;font-size:10px">' +
                (v.action || '--') + '</td>' +
                '<td style="padding:4px 8px"><span style="display:inline-block;padding:1px 6px;background:' +
                classColor + ';color:#0f0f1a;font-size:9px;font-weight:bold;border-radius:2px">' +
                (v.classification || '--') + '</span></td>';
            state.violationsBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Rendering - Audit Trail
    // ========================================================================
    function renderAuditTrail() {
        if (!state.auditBody) return;
        state.auditBody.innerHTML = '';

        var f = state.auditFilter;
        for (var i = 0; i < state.auditTrail.length; i++) {
            var a = state.auditTrail[i];
            if (f.trace_id && (a.trace_id || '').indexOf(f.trace_id) === -1) continue;
            if (f.violation_type && (a.type || '').indexOf(f.violation_type) === -1) continue;

            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px;font-size:10px">' + ((a.trace_id || '--').substring(0, 12)) + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.textMuted + '">' + formatTime(a.timestamp) + '</td>' +
                '<td style="padding:4px 8px">' + (a.type || '--') + '</td>' +
                '<td style="padding:4px 8px">' + (a.src_svc || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + (CONFIG.actionColors[a.action] || CONFIG.colors.textMuted) +
                '">' + (a.action || '--') + '</td>';
            state.auditBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderGauge();
            renderHeatmap();
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
        state.complianceScore = 0;
        state.violationMatrix = null;
        state.zones = [];
        state.violations = [];
        state.auditTrail = [];
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

window.CompliancePanel = CompliancePanel;
console.log('compliance-panel.js loaded');
