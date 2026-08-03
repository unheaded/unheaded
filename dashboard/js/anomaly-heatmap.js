/**
 * anomaly-heatmap.js - Anomaly Detection Heatmap (App 12 - Monad CPU)
 * Flow anomaly visualization, suspicious flow analysis, and model confidence monitoring.
 *
 * Features:
 *   - Flow anomaly grid: [src_svc x dst_svc x score] matrix
 *     Color gradient: green (0-20), yellow (20-50), orange (50-75), red (75-100)
 *   - Top 20 suspicious flows table: flow key, score, service, packet count, trend
 *     Click row drill down: packet size, entropy, flags, inter-arrival time
 *   - Model confidence histogram: per-service inference correctness distribution
 *   - Adaptive threshold overlay: current threshold line + rolling baseline mean +/- stddev
 *   - Alert timeline: recent anomaly alerts with severity colors
 *   - Baseline comparison: current vs learned baseline stats per service
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var AnomalyHeatmap = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxAlerts: 100,
        topFlows: 20,

        scoreColors: [
            { threshold: 0,  color: [0, 210, 106] },   // green
            { threshold: 20, color: [80, 220, 80] },    // light green
            { threshold: 50, color: [255, 215, 0] },    // yellow
            { threshold: 75, color: [255, 152, 0] },    // orange
            { threshold: 100, color: [255, 71, 87] }    // red
        ],
        severityColors: {
            'LOW':      '#ffd700',
            'MEDIUM':   '#ff9800',
            'HIGH':     '#ff4757',
            'CRITICAL': '#ff0000'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            cardBg: '#12121f',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            thresholdLine: '#ff4757',
            baselineMean: '#4ecdc4',
            baselineBand: 'rgba(78, 205, 196, 0.1)',
            barFill: '#4ecdc4',
            barBg: 'rgba(255, 255, 255, 0.05)',
            drillBg: '#12121f',
            rowHover: 'rgba(255, 215, 0, 0.08)',
            trendUp: '#ff4757',
            trendDown: '#00d26a',
            trendFlat: '#6b6b7b'
        },
        padding: { top: 20, right: 16, bottom: 30, left: 70 }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        gridCanvas: null,
        gridCtx: null,
        confidenceCanvas: null,
        confidenceCtx: null,
        thresholdCanvas: null,
        thresholdCtx: null,

        // Data
        anomalyMatrix: null,    // { services: [names], matrix: [[score, ...]] }
        suspiciousFlows: [],    // [{ flow_key, score, service, packet_count, trend, detail: {...} }]
        modelConfidence: [],    // [{ service, buckets: [count, count, ...] }] for histogram
        thresholdData: null,    // { data: [{ timestamp, value }], threshold, baseline_mean, baseline_stddev }
        alerts: [],             // [{ timestamp, severity, service, message }]
        baselineComparison: [], // [{ service, current_mean, baseline_mean, deviation_pct }]

        flowsBody: null,
        alertList: null,
        baselineBody: null,
        expandedFlowIdx: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[AnomalyHeatmap] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[AnomalyHeatmap] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Top: anomaly grid + confidence histogram
        var topGrid = document.createElement('div');
        topGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var gridSection = createSection('Flow Anomaly Score Matrix (src x dst)');
        state.gridCanvas = document.createElement('canvas');
        state.gridCanvas.style.cssText = 'display:block;width:100%;';
        gridSection.body.appendChild(state.gridCanvas);
        state.gridCtx = state.gridCanvas.getContext('2d');

        // Legend
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:4px;padding:6px 0;font-size:10px;align-items:center;';
        legend.innerHTML = '<span style="color:' + CONFIG.colors.textMuted + '">Score: </span>';
        var legendLabels = ['0', '20', '50', '75', '100'];
        for (var i = 0; i < CONFIG.scoreColors.length; i++) {
            var c = CONFIG.scoreColors[i].color;
            legend.innerHTML += '<span style="display:inline-block;width:24px;height:10px;background:rgb(' +
                c[0] + ',' + c[1] + ',' + c[2] + ');"></span>';
            if (i < legendLabels.length) {
                legend.innerHTML += '<span style="color:' + CONFIG.colors.textMuted + ';font-size:9px;margin-right:2px">' +
                    legendLabels[i] + '</span>';
            }
        }
        gridSection.body.appendChild(legend);
        topGrid.appendChild(gridSection.el);

        var confSection = createSection('Model Confidence Distribution');
        state.confidenceCanvas = document.createElement('canvas');
        state.confidenceCanvas.style.cssText = 'display:block;width:100%;';
        confSection.body.appendChild(state.confidenceCanvas);
        state.confidenceCtx = state.confidenceCanvas.getContext('2d');
        topGrid.appendChild(confSection.el);

        parent.appendChild(topGrid);

        // Mid: suspicious flows table + threshold chart
        var midGrid = document.createElement('div');
        midGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var flowSection = createSection('Top 20 Suspicious Flows');
        buildFlowsTable(flowSection.body);
        midGrid.appendChild(flowSection.el);

        var threshSection = createSection('Adaptive Threshold Overlay');
        state.thresholdCanvas = document.createElement('canvas');
        state.thresholdCanvas.style.cssText = 'display:block;width:100%;';
        threshSection.body.appendChild(state.thresholdCanvas);
        state.thresholdCtx = state.thresholdCanvas.getContext('2d');
        midGrid.appendChild(threshSection.el);

        parent.appendChild(midGrid);

        // Bottom: alerts + baseline comparison
        var botGrid = document.createElement('div');
        botGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var alertSection = createSection('Recent Anomaly Alerts');
        state.alertList = document.createElement('div');
        state.alertList.style.cssText = 'max-height:200px;overflow-y:auto;';
        alertSection.body.appendChild(state.alertList);
        botGrid.appendChild(alertSection.el);

        var baseSection = createSection('Current vs Baseline Comparison');
        buildBaselineTable(baseSection.body);
        botGrid.appendChild(baseSection.el);

        parent.appendChild(botGrid);
    }

    function buildFlowsTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:300px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 6px;text-align:left">Flow Key</th>' +
            '<th style="padding:6px 6px;text-align:right">Score</th>' +
            '<th style="padding:6px 6px;text-align:left">Service</th>' +
            '<th style="padding:6px 6px;text-align:right">Packets</th>' +
            '<th style="padding:6px 6px;text-align:center">Trend</th></tr></thead>';
        state.flowsBody = document.createElement('tbody');
        table.appendChild(state.flowsBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
    }

    function buildBaselineTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:200px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Service</th>' +
            '<th style="padding:6px 8px;text-align:right">Current Mean</th>' +
            '<th style="padding:6px 8px;text-align:right">Baseline Mean</th>' +
            '<th style="padding:6px 8px;text-align:right">Deviation %</th></tr></thead>';
        state.baselineBody = document.createElement('tbody');
        table.appendChild(state.baselineBody);
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
        resizeCanvas(state.gridCanvas, state.gridCtx, 250);
        resizeCanvas(state.confidenceCanvas, state.confidenceCtx, 250);
        resizeCanvas(state.thresholdCanvas, state.thresholdCtx, 220);
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
    function update(anomalyData) {
        if (!state.initialized || !anomalyData) return;

        if (anomalyData.anomaly_matrix) state.anomalyMatrix = anomalyData.anomaly_matrix;
        if (anomalyData.suspicious_flows) {
            state.suspiciousFlows = anomalyData.suspicious_flows.slice(0, CONFIG.topFlows);
            renderFlowsTable();
        }
        if (anomalyData.model_confidence) state.modelConfidence = anomalyData.model_confidence;
        if (anomalyData.threshold_data) state.thresholdData = anomalyData.threshold_data;
        if (anomalyData.alerts) {
            state.alerts = anomalyData.alerts.slice(0, CONFIG.maxAlerts);
            renderAlerts();
        }
        if (anomalyData.baseline_comparison) {
            state.baselineComparison = anomalyData.baseline_comparison;
            renderBaseline();
        }
    }

    // ========================================================================
    // Rendering - Anomaly Grid
    // ========================================================================
    function renderAnomalyGrid() {
        var ctx = state.gridCtx;
        if (!ctx || !state.gridCanvas || !state.anomalyMatrix) return;
        var w = state.gridCanvas.width / (window.devicePixelRatio || 1);
        var h = state.gridCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var am = state.anomalyMatrix;
        var services = am.services || [];
        var matrix = am.matrix || [];
        if (services.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No anomaly data', w / 2, h / 2);
            return;
        }

        var labelW = 70;
        var labelH = 30;
        var plotW = w - labelW - 10;
        var plotH = h - labelH - 20;
        var cellW = plotW / services.length;
        var cellH = plotH / services.length;

        for (var row = 0; row < services.length; row++) {
            for (var col = 0; col < services.length; col++) {
                var score = (matrix[row] && matrix[row][col]) || 0;
                var rgb = scoreToColor(score);
                ctx.fillStyle = 'rgb(' + rgb[0] + ',' + rgb[1] + ',' + rgb[2] + ')';
                ctx.fillRect(labelW + col * cellW + 0.5, labelH + row * cellH + 0.5, cellW - 1, cellH - 1);

                // Score text
                if (cellW > 22 && cellH > 14 && score > 0) {
                    ctx.fillStyle = score > 50 ? '#fff' : CONFIG.colors.text;
                    ctx.font = '8px "SF Mono", monospace';
                    ctx.textAlign = 'center';
                    ctx.fillText(String(Math.round(score)), labelW + col * cellW + cellW / 2,
                        labelH + row * cellH + cellH / 2 + 3);
                }
            }
        }

        // Labels
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var rl = 0; rl < services.length; rl++) {
            var sLabel = services[rl].length > 8 ? services[rl].substring(0, 8) : services[rl];
            ctx.fillText(sLabel, labelW - 4, labelH + rl * cellH + cellH / 2 + 3);
        }
        ctx.textAlign = 'center';
        for (var cl = 0; cl < services.length; cl++) {
            var cLabel = services[cl].length > 5 ? services[cl].substring(0, 5) : services[cl];
            ctx.fillText(cLabel, labelW + cl * cellW + cellW / 2, labelH - 6);
        }
    }

    function scoreToColor(score) {
        var stops = CONFIG.scoreColors;
        if (score <= stops[0].threshold) return stops[0].color;
        if (score >= stops[stops.length - 1].threshold) return stops[stops.length - 1].color;

        for (var i = 1; i < stops.length; i++) {
            if (score <= stops[i].threshold) {
                var t = (score - stops[i - 1].threshold) / (stops[i].threshold - stops[i - 1].threshold);
                var c1 = stops[i - 1].color;
                var c2 = stops[i].color;
                return [
                    Math.round(c1[0] + (c2[0] - c1[0]) * t),
                    Math.round(c1[1] + (c2[1] - c1[1]) * t),
                    Math.round(c1[2] + (c2[2] - c1[2]) * t)
                ];
            }
        }
        return stops[stops.length - 1].color;
    }

    // ========================================================================
    // Rendering - Confidence Histogram
    // ========================================================================
    function renderConfidence() {
        var ctx = state.confidenceCtx;
        if (!ctx || !state.confidenceCanvas) return;
        var w = state.confidenceCanvas.width / (window.devicePixelRatio || 1);
        var h = state.confidenceCanvas.height / (window.devicePixelRatio || 1);
        var pad = { top: 20, right: 16, bottom: 40, left: 50 };

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.modelConfidence;
        if (!data || data.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No model confidence data', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var barGroupW = plotW / data.length;
        var gap = barGroupW * 0.2;

        // Find max across all buckets
        var maxVal = 0;
        for (var s = 0; s < data.length; s++) {
            var buckets = data[s].buckets || [];
            for (var b = 0; b < buckets.length; b++) {
                if (buckets[b] > maxVal) maxVal = buckets[b];
            }
        }
        maxVal = maxVal || 1;

        // Color per bucket index
        var bucketColors = ['#ff4757', '#ff9800', '#ffd700', '#00d26a', '#4ecdc4'];

        for (var si = 0; si < data.length; si++) {
            const buckets = data[si].buckets || [];
            var numBuckets = buckets.length;
            var subBarW = numBuckets > 0 ? (barGroupW - gap) / numBuckets : barGroupW - gap;

            for (var bi = 0; bi < numBuckets; bi++) {
                var barH = (buckets[bi] / maxVal) * plotH;
                var x = pad.left + si * barGroupW + gap / 2 + bi * subBarW;
                var y = pad.top + plotH - barH;

                ctx.fillStyle = bucketColors[bi % bucketColors.length];
                ctx.globalAlpha = 0.8;
                ctx.fillRect(x, y, Math.max(subBarW - 1, 1), barH);
            }

            // Service label
            ctx.globalAlpha = 1.0;
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '8px "SF Mono", monospace';
            ctx.textAlign = 'center';
            var svcLabel = (data[si].service || 'svc' + si);
            if (svcLabel.length > 8) svcLabel = svcLabel.substring(0, 8);
            ctx.save();
            ctx.translate(pad.left + si * barGroupW + barGroupW / 2, pad.top + plotH + 10);
            ctx.rotate(Math.PI / 6);
            ctx.fillText(svcLabel, 0, 0);
            ctx.restore();
        }
        ctx.globalAlpha = 1.0;
    }

    // ========================================================================
    // Rendering - Threshold Overlay
    // ========================================================================
    function renderThreshold() {
        var ctx = state.thresholdCtx;
        if (!ctx || !state.thresholdCanvas) return;
        var w = state.thresholdCanvas.width / (window.devicePixelRatio || 1);
        var h = state.thresholdCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var td = state.thresholdData;
        if (!td || !td.data || td.data.length < 2) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No threshold data', w / 2, h / 2);
            return;
        }

        var data = td.data;
        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = td.threshold || 0;
        for (var i = 0; i < data.length; i++) {
            if (data[i].value > yMax) yMax = data[i].value;
        }
        var bmHigh = (td.baseline_mean || 0) + (td.baseline_stddev || 0);
        if (bmHigh > yMax) yMax = bmHigh;
        yMax = Math.ceil(yMax * 1.3) || 100;

        // Grid
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var g = 1; g < 4; g++) {
            var gy = pad.top + plotH * (1 - g / 4);
            ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + plotW, gy); ctx.stroke();
        }

        // Baseline band (mean +/- stddev)
        if (td.baseline_mean != null && td.baseline_stddev != null) {
            var bandLow = Math.max(0, td.baseline_mean - td.baseline_stddev);
            var bandHigh = td.baseline_mean + td.baseline_stddev;
            var yBandTop = pad.top + plotH - (bandHigh / yMax) * plotH;
            var yBandBot = pad.top + plotH - (bandLow / yMax) * plotH;

            ctx.fillStyle = CONFIG.colors.baselineBand;
            ctx.fillRect(pad.left, yBandTop, plotW, yBandBot - yBandTop);

            // Baseline mean line
            var meanY = pad.top + plotH - (td.baseline_mean / yMax) * plotH;
            ctx.setLineDash([4, 4]);
            ctx.strokeStyle = CONFIG.colors.baselineMean;
            ctx.lineWidth = 1;
            ctx.beginPath(); ctx.moveTo(pad.left, meanY); ctx.lineTo(pad.left + plotW, meanY); ctx.stroke();
            ctx.setLineDash([]);
        }

        // Threshold line
        if (td.threshold) {
            var ty = pad.top + plotH - (td.threshold / yMax) * plotH;
            ctx.setLineDash([6, 3]);
            ctx.strokeStyle = CONFIG.colors.thresholdLine;
            ctx.lineWidth = 1.5;
            ctx.beginPath(); ctx.moveTo(pad.left, ty); ctx.lineTo(pad.left + plotW, ty); ctx.stroke();
            ctx.setLineDash([]);

            ctx.fillStyle = CONFIG.colors.thresholdLine;
            ctx.font = '9px "SF Mono", monospace';
            ctx.textAlign = 'left';
            ctx.fillText('threshold: ' + td.threshold, pad.left + 4, ty - 4);
        }

        // Data line
        ctx.strokeStyle = CONFIG.colors.accent;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var x = pad.left + (j / (data.length - 1)) * plotW;
            var y = pad.top + plotH - (data[j].value / yMax) * plotH;
            if (j === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();

        // Anomaly points (above threshold)
        if (td.threshold) {
            ctx.fillStyle = CONFIG.colors.thresholdLine;
            for (var k = 0; k < data.length; k++) {
                if (data[k].value > td.threshold) {
                    var ax = pad.left + (k / (data.length - 1)) * plotW;
                    var ay = pad.top + plotH - (data[k].value / yMax) * plotH;
                    ctx.beginPath();
                    ctx.arc(ax, ay, 4, 0, Math.PI * 2);
                    ctx.fill();
                }
            }
        }

        // Legend
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'right';
        ctx.fillStyle = CONFIG.colors.baselineMean;
        ctx.fillText('baseline mean +/- stddev', pad.left + plotW, pad.top + 10);
    }

    // ========================================================================
    // Rendering - Flows Table
    // ========================================================================
    function renderFlowsTable() {
        if (!state.flowsBody) return;
        state.flowsBody.innerHTML = '';

        for (var i = 0; i < state.suspiciousFlows.length; i++) {
            var f = state.suspiciousFlows[i];
            var scoreRgb = scoreToColor(f.score || 0);
            var trendColor = f.trend === 'up' ? CONFIG.colors.trendUp :
                (f.trend === 'down' ? CONFIG.colors.trendDown : CONFIG.colors.trendFlat);
            var trendSymbol = f.trend === 'up' ? '&#9650;' :
                (f.trend === 'down' ? '&#9660;' : '&#9644;');

            var tr = document.createElement('tr');
            tr.dataset.index = i;
            tr.style.cssText = 'cursor:pointer;border-bottom:1px solid ' + CONFIG.colors.border + ';transition:background 0.15s;';
            tr.addEventListener('mouseenter', function() { this.style.background = CONFIG.colors.rowHover; });
            tr.addEventListener('mouseleave', function() { this.style.background = 'transparent'; });
            tr.addEventListener('click', onFlowClick);

            tr.innerHTML =
                '<td style="padding:4px 6px;font-size:10px">' + ((f.flow_key || '--').substring(0, 20)) + '</td>' +
                '<td style="padding:4px 6px;text-align:right;color:rgb(' + scoreRgb[0] + ',' + scoreRgb[1] + ',' +
                scoreRgb[2] + ');font-weight:bold">' + (f.score || 0) + '</td>' +
                '<td style="padding:4px 6px">' + (f.service || '--') + '</td>' +
                '<td style="padding:4px 6px;text-align:right">' + formatNumber(f.packet_count || 0) + '</td>' +
                '<td style="padding:4px 6px;text-align:center;color:' + trendColor + '">' + trendSymbol + '</td>';
            state.flowsBody.appendChild(tr);
        }
    }

    function onFlowClick(e) {
        var tr = e.currentTarget;
        var idx = parseInt(tr.dataset.index, 10);

        // Remove existing detail
        var existing = state.flowsBody.querySelector('.anomaly-detail-row');
        if (existing) {
            var prevIdx = parseInt(existing.dataset.detailFor, 10);
            existing.remove();
            if (prevIdx === idx) { state.expandedFlowIdx = null; return; }
        }

        state.expandedFlowIdx = idx;
        var flow = state.suspiciousFlows[idx];
        if (!flow || !flow.detail) return;

        var detailRow = document.createElement('tr');
        detailRow.className = 'anomaly-detail-row';
        detailRow.dataset.detailFor = idx;
        var td = document.createElement('td');
        td.colSpan = 5;
        td.style.cssText = 'padding:0;background:' + CONFIG.colors.drillBg + ';';

        var inner = document.createElement('div');
        inner.style.cssText = 'padding:10px 16px;font-size:10px;';
        inner.innerHTML =
            '<div style="color:' + CONFIG.colors.accent + ';font-weight:bold;margin-bottom:6px">FLOW DETAIL</div>' +
            '<div style="display:grid;grid-template-columns:1fr 1fr 1fr 1fr;gap:8px;">' +
            '<div>Avg Packet Size: <strong>' + (flow.detail.avg_packet_size || '--') + ' B</strong></div>' +
            '<div>Entropy: <strong>' + ((flow.detail.entropy || 0).toFixed(3)) + '</strong></div>' +
            '<div>Flags: <strong>' + (flow.detail.flags || '--') + '</strong></div>' +
            '<div>Inter-arrival: <strong>' + ((flow.detail.inter_arrival_ms || 0).toFixed(2)) + ' ms</strong></div></div>';

        td.appendChild(inner);
        detailRow.appendChild(td);
        tr.parentNode.insertBefore(detailRow, tr.nextSibling);
    }

    // ========================================================================
    // Rendering - Alerts
    // ========================================================================
    function renderAlerts() {
        if (!state.alertList) return;
        state.alertList.innerHTML = '';

        if (state.alerts.length === 0) {
            state.alertList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:16px;font-size:12px">No recent alerts</div>';
            return;
        }

        for (var i = 0; i < state.alerts.length; i++) {
            var a = state.alerts[i];
            var sevColor = CONFIG.severityColors[a.severity] || CONFIG.colors.textMuted;

            var row = document.createElement('div');
            row.style.cssText = 'display:flex;gap:8px;align-items:center;padding:4px 8px;font-size:11px;' +
                'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            row.innerHTML =
                '<span style="width:6px;height:6px;border-radius:50%;background:' + sevColor + ';flex-shrink:0"></span>' +
                '<span style="width:60px;color:' + CONFIG.colors.textMuted + ';flex-shrink:0">' +
                formatTime(a.timestamp) + '</span>' +
                '<span style="width:60px;color:' + sevColor + ';font-weight:bold;font-size:9px;flex-shrink:0;text-transform:uppercase">' +
                (a.severity || '--') + '</span>' +
                '<span style="flex:1">' + (a.message || '--') + '</span>' +
                '<span style="color:' + CONFIG.colors.textMuted + ';flex-shrink:0">' + (a.service || '') + '</span>';
            state.alertList.appendChild(row);
        }
    }

    // ========================================================================
    // Rendering - Baseline Comparison
    // ========================================================================
    function renderBaseline() {
        if (!state.baselineBody) return;
        state.baselineBody.innerHTML = '';

        for (var i = 0; i < state.baselineComparison.length; i++) {
            var b = state.baselineComparison[i];
            var dev = b.deviation_pct || 0;
            var devColor = Math.abs(dev) > 20 ? '#ff4757' : (Math.abs(dev) > 10 ? '#ff9800' : '#00d26a');

            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px">' + (b.service || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (b.current_mean || 0).toFixed(2) + '</td>' +
                '<td style="padding:4px 8px;text-align:right;color:' + CONFIG.colors.textMuted + '">' +
                (b.baseline_mean || 0).toFixed(2) + '</td>' +
                '<td style="padding:4px 8px;text-align:right;color:' + devColor + ';font-weight:bold">' +
                (dev > 0 ? '+' : '') + dev.toFixed(1) + '%</td>';
            state.baselineBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderAnomalyGrid();
            renderConfidence();
            renderThreshold();
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
        state.anomalyMatrix = null;
        state.suspiciousFlows = [];
        state.modelConfidence = [];
        state.thresholdData = null;
        state.alerts = [];
        state.baselineComparison = [];
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

window.AnomalyHeatmap = AnomalyHeatmap;
console.log('anomaly-heatmap.js loaded');
