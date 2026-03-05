/**
 * int-heatmap.js - In-Network Telemetry Heatmap (App 02 - Monad CPU)
 * Canvas-based heatmap for hop-level latency visualization.
 *
 * Features:
 *   - Heatmap: X=hop index (0-15), Y=time (10 min span, 1s buckets)
 *   - Color scale: 0-1ms green, 5ms yellow, 10ms orange, 100ms+ red
 *   - Hover tooltip: hop_id, latency_us, queue_depth, link_util
 *   - Click cell to drill down to specific trace
 *   - Legend with color scale
 *   - Path visualization: directed graph overlay for active paths
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var INTHeatmap = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxHops: 16,
        timeSpanSeconds: 600,    // 10 minutes
        bucketSeconds: 1,
        cellPadding: 1,
        padding: { top: 40, right: 30, bottom: 50, left: 60 },
        fps: 10,

        // Color scale thresholds in ms
        colorStops: [
            { threshold: 0,   color: [0, 180, 80] },     // green
            { threshold: 1,   color: [80, 210, 60] },     // light green
            { threshold: 5,   color: [255, 220, 0] },     // yellow
            { threshold: 10,  color: [255, 152, 0] },     // orange
            { threshold: 50,  color: [255, 80, 40] },     // red-orange
            { threshold: 100, color: [200, 20, 20] }      // deep red
        ],

        colors: {
            background: '#0f0f1a',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            axisLabel: '#6b6b7b',
            axisLine: 'rgba(255, 215, 0, 0.2)',
            tooltipBg: 'rgba(15, 15, 26, 0.95)',
            tooltipBorder: '#ffd700',
            tooltipText: '#e0e0e0',
            pathLine: 'rgba(78, 205, 196, 0.7)',
            pathArrow: '#4ecdc4',
            emptyCell: 'rgba(255, 255, 255, 0.02)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvas: null,
        ctx: null,
        width: 0,
        height: 0,
        dpr: 1,

        // Data: grid[timeIndex][hopIndex] = { latency_us, queue_depth, link_util, hop_id, trace_id }
        grid: [],
        timeBase: 0,      // timestamp of earliest row
        activePaths: [],   // array of { hops: [hopIdx, ...], label: string }

        tooltip: { visible: false, x: 0, y: 0, data: null },
        animationFrame: null,
        onDrillDown: null, // callback(trace_id)
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[INTHeatmap] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.position = 'relative';

        // Canvas
        state.canvas = document.createElement('canvas');
        state.canvas.style.cssText = 'display:block;';
        state.canvas.setAttribute('role', 'img');
        state.canvas.setAttribute('aria-label', 'In-Network Telemetry heatmap showing hop latencies over time');
        container.appendChild(state.canvas);

        state.ctx = state.canvas.getContext('2d');
        state.dpr = window.devicePixelRatio || 1;

        // Legend
        buildLegend(container);

        // Tooltip element
        var tip = document.createElement('div');
        tip.id = 'int-heatmap-tooltip';
        tip.style.cssText = 'position:absolute;display:none;padding:8px 12px;background:' +
            CONFIG.colors.tooltipBg + ';border:1px solid ' + CONFIG.colors.tooltipBorder +
            ';border-radius:4px;font-size:11px;font-family:"SF Mono",monospace;color:' +
            CONFIG.colors.tooltipText + ';pointer-events:none;z-index:10;white-space:nowrap;';
        container.appendChild(tip);

        // Events
        state.canvas.addEventListener('mousemove', onMouseMove);
        state.canvas.addEventListener('mouseleave', onMouseLeave);
        state.canvas.addEventListener('click', onCanvasClick);
        window.addEventListener('resize', debounce(resizeCanvas, 150));

        resizeCanvas();
        initGrid();
        startAnimation();

        state.initialized = true;
        console.log('[INTHeatmap] Initialized');
        return true;
    }

    function buildLegend(parent) {
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;align-items:center;gap:8px;padding:8px 12px;font-size:11px;color:' +
            CONFIG.colors.axisLabel + ';font-family:"SF Mono",monospace;';

        legend.innerHTML = '<span>Latency:</span>';

        var labels = ['0ms', '1ms', '5ms', '10ms', '50ms', '100ms+'];
        for (var i = 0; i < CONFIG.colorStops.length; i++) {
            var c = CONFIG.colorStops[i].color;
            var swatch = '<span style="display:inline-block;width:16px;height:12px;background:rgb(' +
                c[0] + ',' + c[1] + ',' + c[2] + ');border-radius:2px;margin-right:2px;"></span>';
            legend.innerHTML += swatch + '<span style="margin-right:8px;">' + labels[i] + '</span>';
        }

        parent.appendChild(legend);
    }

    function resizeCanvas() {
        if (!state.canvas || !state.container) return;
        var rect = state.container.getBoundingClientRect();
        state.width = rect.width || 800;
        state.height = Math.max(300, Math.min(500, rect.height - 60)) || 400;

        state.canvas.width = state.width * state.dpr;
        state.canvas.height = state.height * state.dpr;
        state.canvas.style.width = state.width + 'px';
        state.canvas.style.height = state.height + 'px';
        state.ctx.setTransform(state.dpr, 0, 0, state.dpr, 0, 0);
    }

    function initGrid() {
        state.grid = [];
        state.timeBase = Date.now() - CONFIG.timeSpanSeconds * 1000;
        for (var t = 0; t < CONFIG.timeSpanSeconds; t++) {
            var row = [];
            for (var h = 0; h < CONFIG.maxHops; h++) {
                row.push(null);
            }
            state.grid.push(row);
        }
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(hopMetrics) {
        if (!state.initialized) return;
        if (!hopMetrics) return;

        // hopMetrics: { timestamp, hops: [{ hop_index, latency_us, queue_depth, link_util, hop_id, trace_id }] }
        if (Array.isArray(hopMetrics)) {
            for (var i = 0; i < hopMetrics.length; i++) ingestMetric(hopMetrics[i]);
        } else {
            ingestMetric(hopMetrics);
        }
    }

    function ingestMetric(metric) {
        if (!metric || !metric.hops) return;
        var ts = metric.timestamp || Date.now();
        var tIdx = Math.floor((ts - state.timeBase) / (CONFIG.bucketSeconds * 1000));

        // Shift grid if needed
        while (tIdx >= state.grid.length) {
            state.grid.shift();
            var newRow = [];
            for (var h = 0; h < CONFIG.maxHops; h++) newRow.push(null);
            state.grid.push(newRow);
            state.timeBase += CONFIG.bucketSeconds * 1000;
            tIdx--;
        }

        if (tIdx < 0) return;

        for (var i = 0; i < metric.hops.length; i++) {
            var hop = metric.hops[i];
            var hIdx = hop.hop_index;
            if (hIdx >= 0 && hIdx < CONFIG.maxHops && tIdx < state.grid.length) {
                state.grid[tIdx][hIdx] = {
                    latency_us: hop.latency_us || 0,
                    queue_depth: hop.queue_depth || 0,
                    link_util: hop.link_util || 0,
                    hop_id: hop.hop_id || '',
                    trace_id: hop.trace_id || ''
                };
            }
        }
    }

    function setActivePaths(paths) {
        state.activePaths = paths || [];
    }

    // ========================================================================
    // Animation & Rendering
    // ========================================================================
    function startAnimation() {
        function tick() {
            render();
            state.animationFrame = requestAnimationFrame(tick);
        }
        state.animationFrame = requestAnimationFrame(tick);
    }

    function stopAnimation() {
        if (state.animationFrame) {
            cancelAnimationFrame(state.animationFrame);
            state.animationFrame = null;
        }
    }

    function render() {
        var ctx = state.ctx;
        var w = state.width;
        var h = state.height;
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var cellW = plotW / CONFIG.maxHops;
        var rows = state.grid.length;
        var cellH = rows > 0 ? plotH / rows : 1;

        // Draw cells
        for (var t = 0; t < rows; t++) {
            for (var hop = 0; hop < CONFIG.maxHops; hop++) {
                var cell = state.grid[t][hop];
                var x = pad.left + hop * cellW + CONFIG.cellPadding;
                var y = pad.top + t * cellH + CONFIG.cellPadding;
                var cw = cellW - CONFIG.cellPadding * 2;
                var ch = cellH - CONFIG.cellPadding * 2;

                if (cell && cell.latency_us > 0) {
                    var ms = cell.latency_us / 1000;
                    var rgb = interpolateColor(ms);
                    ctx.fillStyle = 'rgb(' + rgb[0] + ',' + rgb[1] + ',' + rgb[2] + ')';
                } else {
                    ctx.fillStyle = CONFIG.colors.emptyCell;
                }
                ctx.fillRect(x, y, Math.max(cw, 0.5), Math.max(ch, 0.5));
            }
        }

        // Axes
        drawAxes(ctx, pad, plotW, plotH, rows);

        // Path overlay
        drawPaths(ctx, pad, plotW, plotH, cellW);
    }

    function drawAxes(ctx, pad, plotW, plotH, rows) {
        ctx.strokeStyle = CONFIG.colors.axisLine;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top);
        ctx.lineTo(pad.left, pad.top + plotH);
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.stroke();

        // X labels (hop index)
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.axisLabel;
        ctx.textAlign = 'center';
        var cellW = plotW / CONFIG.maxHops;
        for (var i = 0; i < CONFIG.maxHops; i++) {
            ctx.fillText(String(i), pad.left + i * cellW + cellW / 2, pad.top + plotH + 14);
        }
        ctx.fillText('Hop Index', pad.left + plotW / 2, pad.top + plotH + 30);

        // Y labels (time)
        ctx.textAlign = 'right';
        var ySteps = 5;
        for (var j = 0; j <= ySteps; j++) {
            var rowIdx = Math.floor(j * (rows - 1) / ySteps);
            var ts = state.timeBase + rowIdx * CONFIG.bucketSeconds * 1000;
            var d = new Date(ts);
            var label = d.toLocaleTimeString('en-US', { hour12: false, minute: '2-digit', second: '2-digit' });
            var yy = pad.top + (rowIdx / Math.max(rows - 1, 1)) * plotH;
            ctx.fillText(label, pad.left - 4, yy + 4);
        }

        // Title
        ctx.textAlign = 'center';
        ctx.fillStyle = CONFIG.colors.axisLabel;
        ctx.font = '11px -apple-system, sans-serif';
        ctx.fillText('INT Heatmap - Hop Latency Over Time', pad.left + plotW / 2, pad.top - 12);
    }

    function drawPaths(ctx, pad, plotW, plotH, cellW) {
        if (!state.activePaths || state.activePaths.length === 0) return;

        ctx.strokeStyle = CONFIG.colors.pathLine;
        ctx.lineWidth = 2;
        ctx.setLineDash([4, 3]);

        for (var p = 0; p < state.activePaths.length; p++) {
            var path = state.activePaths[p];
            if (!path.hops || path.hops.length < 2) continue;

            ctx.beginPath();
            var midY = pad.top + plotH / 2;
            for (var h = 0; h < path.hops.length; h++) {
                var x = pad.left + path.hops[h] * cellW + cellW / 2;
                if (h === 0) ctx.moveTo(x, midY);
                else ctx.lineTo(x, midY);
            }
            ctx.stroke();

            // Arrow at end
            var lastX = pad.left + path.hops[path.hops.length - 1] * cellW + cellW / 2;
            ctx.fillStyle = CONFIG.colors.pathArrow;
            ctx.beginPath();
            ctx.moveTo(lastX + 6, midY);
            ctx.lineTo(lastX - 2, midY - 4);
            ctx.lineTo(lastX - 2, midY + 4);
            ctx.closePath();
            ctx.fill();
        }
        ctx.setLineDash([]);
    }

    // ========================================================================
    // Color Interpolation
    // ========================================================================
    function interpolateColor(ms) {
        var stops = CONFIG.colorStops;
        if (ms <= stops[0].threshold) return stops[0].color;
        if (ms >= stops[stops.length - 1].threshold) return stops[stops.length - 1].color;

        for (var i = 1; i < stops.length; i++) {
            if (ms <= stops[i].threshold) {
                var t = (ms - stops[i - 1].threshold) / (stops[i].threshold - stops[i - 1].threshold);
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
    // Mouse Events
    // ========================================================================
    function onMouseMove(e) {
        var rect = state.canvas.getBoundingClientRect();
        var mx = e.clientX - rect.left;
        var my = e.clientY - rect.top;
        var pad = CONFIG.padding;
        var plotW = state.width - pad.left - pad.right;
        var plotH = state.height - pad.top - pad.bottom;

        var hopIdx = Math.floor((mx - pad.left) / (plotW / CONFIG.maxHops));
        var timeIdx = Math.floor((my - pad.top) / (plotH / state.grid.length));

        if (hopIdx < 0 || hopIdx >= CONFIG.maxHops || timeIdx < 0 || timeIdx >= state.grid.length) {
            hideTooltip();
            return;
        }

        var cell = state.grid[timeIdx][hopIdx];
        if (!cell) { hideTooltip(); return; }

        var tip = document.getElementById('int-heatmap-tooltip');
        if (tip) {
            tip.style.display = 'block';
            tip.style.left = (mx + 12) + 'px';
            tip.style.top = (my - 10) + 'px';
            tip.innerHTML = '<div style="color:#ffd700;margin-bottom:4px">Hop ' + hopIdx + '</div>' +
                'ID: ' + (cell.hop_id || '--') + '<br>' +
                'Latency: ' + cell.latency_us + ' us<br>' +
                'Queue: ' + cell.queue_depth + '<br>' +
                'Link Util: ' + ((cell.link_util || 0) * 100).toFixed(1) + '%';
        }
    }

    function onMouseLeave() { hideTooltip(); }

    function hideTooltip() {
        var tip = document.getElementById('int-heatmap-tooltip');
        if (tip) tip.style.display = 'none';
    }

    function onCanvasClick(e) {
        var rect = state.canvas.getBoundingClientRect();
        var mx = e.clientX - rect.left;
        var my = e.clientY - rect.top;
        var pad = CONFIG.padding;
        var plotW = state.width - pad.left - pad.right;
        var plotH = state.height - pad.top - pad.bottom;

        var hopIdx = Math.floor((mx - pad.left) / (plotW / CONFIG.maxHops));
        var timeIdx = Math.floor((my - pad.top) / (plotH / state.grid.length));

        if (hopIdx >= 0 && hopIdx < CONFIG.maxHops && timeIdx >= 0 && timeIdx < state.grid.length) {
            var cell = state.grid[timeIdx][hopIdx];
            if (cell && cell.trace_id && state.onDrillDown) {
                state.onDrillDown(cell.trace_id);
            }
        }
    }

    // ========================================================================
    // Utilities
    // ========================================================================
    function debounce(fn, wait) {
        var t;
        return function() { var a = arguments, c = this; clearTimeout(t); t = setTimeout(function() { fn.apply(c, a); }, wait); };
    }

    // ========================================================================
    // Cleanup
    // ========================================================================
    function destroy() {
        stopAnimation();
        if (state.canvas) {
            state.canvas.removeEventListener('mousemove', onMouseMove);
            state.canvas.removeEventListener('mouseleave', onMouseLeave);
            state.canvas.removeEventListener('click', onCanvasClick);
        }
        if (state.container) state.container.innerHTML = '';
        state.grid = [];
        state.activePaths = [];
        state.initialized = false;
    }

    // ========================================================================
    // Public API
    // ========================================================================
    return {
        init: init,
        update: update,
        setActivePaths: setActivePaths,
        setDrillDownCallback: function(cb) { state.onDrillDown = cb; },
        destroy: destroy,
        isInitialized: function() { return state.initialized; }
    };
})();

window.INTHeatmap = INTHeatmap;
console.log('int-heatmap.js loaded');
