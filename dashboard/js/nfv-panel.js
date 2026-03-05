/**
 * nfv-panel.js - Network Function Virtualization Panel (App 10 - Monad CPU)
 * Chain execution visualization, per-function metrics, and chain management.
 *
 * Features:
 *   - Chain execution flow: horizontal pipeline diagram (nodes per function, edges for flow)
 *     Each node: function name, latency bar, decision count, color (green/yellow/red)
 *   - Per-chain stats table: chain ID, name, functions, total latency, packets/s, drop rate
 *   - Function heatmap: Y=function (1-10), X=time, color=latency (green=fast, red=slow)
 *   - Circuit breaker monitor: AUTH state distribution gauge (NORMAL/PRIORITY/EXPERIMENTAL)
 *   - Create chain form: name, add functions from registry, save
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var NFVPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxFunctions: 10,
        heatmapTimePoints: 60,
        pipelineNodeW: 100,
        pipelineNodeH: 60,
        pipelineGap: 30,

        latencyColors: function(ms) {
            if (ms < 1) return '#00d26a';
            if (ms < 5) return '#ffd700';
            if (ms < 10) return '#ff9800';
            return '#ff4757';
        },
        authColors: {
            'NORMAL':       '#00d26a',
            'PRIORITY':     '#ffd700',
            'EXPERIMENTAL': '#45b7d1'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            cardBg: '#12121f',
            pipelineEdge: 'rgba(78, 205, 196, 0.5)',
            pipelineArrow: '#4ecdc4',
            nodeStroke: 'rgba(255, 215, 0, 0.3)',
            nodeFill: '#12121f',
            heatGreen: [0, 210, 106],
            heatRed: [255, 71, 87],
            gridLine: 'rgba(255, 215, 0, 0.06)',
            gaugeTrack: 'rgba(255, 255, 255, 0.05)'
        },
        padding: { top: 20, right: 16, bottom: 30, left: 60 }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        pipelineCanvas: null,
        pipelineCtx: null,
        heatmapCanvas: null,
        heatmapCtx: null,
        gaugeCanvas: null,
        gaugeCtx: null,

        // Data
        chains: [],          // [{ id, name, functions: [{name, latency_ms, decisions, health}], total_latency, pps, drop_rate }]
        selectedChain: 0,    // index into chains
        functionHeatmap: [], // [funcIdx][timeIdx] = latency_ms
        functionNames: [],   // [name, ...]
        authDistribution: { NORMAL: 0, PRIORITY: 0, EXPERIMENTAL: 0 },
        functionRegistry: [], // available functions for chain creation

        chainTable: null,
        formContainer: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[NFVPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[NFVPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Pipeline visualization
        var pipeSection = createSection('Chain Execution Flow');
        state.pipelineCanvas = document.createElement('canvas');
        state.pipelineCanvas.style.cssText = 'display:block;width:100%;';
        pipeSection.body.appendChild(state.pipelineCanvas);
        state.pipelineCtx = state.pipelineCanvas.getContext('2d');
        parent.appendChild(pipeSection.el);

        // Mid: chain stats + circuit breaker
        var midGrid = document.createElement('div');
        midGrid.style.cssText = 'display:grid;grid-template-columns:2fr 1fr;gap:12px;margin-bottom:12px;';

        // Chain stats table
        var tableSection = createSection('Per-Chain Statistics');
        buildChainTable(tableSection.body);
        midGrid.appendChild(tableSection.el);

        // Circuit breaker gauge
        var cbSection = createSection('AUTH State Distribution');
        state.gaugeCanvas = document.createElement('canvas');
        state.gaugeCanvas.style.cssText = 'display:block;margin:0 auto;';
        cbSection.body.appendChild(state.gaugeCanvas);
        state.gaugeCtx = state.gaugeCanvas.getContext('2d');
        midGrid.appendChild(cbSection.el);

        parent.appendChild(midGrid);

        // Bottom: heatmap + create form
        var botGrid = document.createElement('div');
        botGrid.style.cssText = 'display:grid;grid-template-columns:2fr 1fr;gap:12px;margin-bottom:12px;';

        var heatSection = createSection('Function Latency Heatmap');
        state.heatmapCanvas = document.createElement('canvas');
        state.heatmapCanvas.style.cssText = 'display:block;width:100%;';
        heatSection.body.appendChild(state.heatmapCanvas);
        state.heatmapCtx = state.heatmapCanvas.getContext('2d');
        botGrid.appendChild(heatSection.el);

        var formSection = createSection('Create Chain');
        state.formContainer = formSection.body;
        buildCreateForm(formSection.body);
        botGrid.appendChild(formSection.el);

        parent.appendChild(botGrid);
    }

    function buildChainTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:250px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Chain</th>' +
            '<th style="padding:6px 8px;text-align:left">Name</th>' +
            '<th style="padding:6px 8px;text-align:right">Functions</th>' +
            '<th style="padding:6px 8px;text-align:right">Latency (ms)</th>' +
            '<th style="padding:6px 8px;text-align:right">Packets/s</th>' +
            '<th style="padding:6px 8px;text-align:right">Drop %</th></tr></thead>';
        state.chainTable = document.createElement('tbody');
        table.appendChild(state.chainTable);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
    }

    function buildCreateForm(parent) {
        var form = document.createElement('div');
        form.style.cssText = 'display:flex;flex-direction:column;gap:8px;';

        var nameInput = document.createElement('input');
        nameInput.type = 'text';
        nameInput.id = 'nfv-chain-name';
        nameInput.placeholder = 'Chain Name';
        nameInput.style.cssText = inputStyle();
        form.appendChild(nameInput);

        // Function selection list
        var funcList = document.createElement('div');
        funcList.id = 'nfv-func-list';
        funcList.style.cssText = 'max-height:120px;overflow-y:auto;border:1px solid ' + CONFIG.colors.border +
            ';border-radius:4px;padding:4px;';
        funcList.innerHTML = '<div style="color:' + CONFIG.colors.textMuted + ';padding:8px;font-size:11px;text-align:center">' +
            'Function registry will appear here</div>';
        form.appendChild(funcList);

        var saveBtn = document.createElement('button');
        saveBtn.textContent = 'Save Chain';
        saveBtn.style.cssText = 'padding:8px;background:' + CONFIG.colors.accent +
            ';color:#0f0f1a;border:none;border-radius:4px;cursor:pointer;font-weight:bold;font-size:12px;font-family:inherit;';
        form.appendChild(saveBtn);

        parent.appendChild(form);
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

    function inputStyle() {
        return 'padding:6px 10px;background:#0f0f1a;color:#e0e0e0;border:1px solid ' + CONFIG.colors.border +
            ';border-radius:4px;font-size:12px;font-family:"SF Mono",monospace;';
    }

    function resizeCanvases() {
        resizeCanvas(state.pipelineCanvas, state.pipelineCtx, 120);
        resizeCanvas(state.heatmapCanvas, state.heatmapCtx, 200);

        if (state.gaugeCanvas) {
            var dpr = window.devicePixelRatio || 1;
            state.gaugeCanvas.width = 180 * dpr;
            state.gaugeCanvas.height = 180 * dpr;
            state.gaugeCanvas.style.width = '180px';
            state.gaugeCanvas.style.height = '180px';
            state.gaugeCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
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
    function update(nfvData) {
        if (!state.initialized || !nfvData) return;

        if (nfvData.chains) {
            state.chains = nfvData.chains;
            renderChainTable();
        }
        if (nfvData.selected_chain != null) state.selectedChain = nfvData.selected_chain;
        if (nfvData.function_heatmap) state.functionHeatmap = nfvData.function_heatmap;
        if (nfvData.function_names) state.functionNames = nfvData.function_names;
        if (nfvData.auth_distribution) state.authDistribution = nfvData.auth_distribution;
        if (nfvData.function_registry) {
            state.functionRegistry = nfvData.function_registry;
            renderFuncRegistry();
        }
    }

    // ========================================================================
    // Rendering - Pipeline
    // ========================================================================
    function renderPipeline() {
        var ctx = state.pipelineCtx;
        if (!ctx || !state.pipelineCanvas) return;
        var w = state.pipelineCanvas.width / (window.devicePixelRatio || 1);
        var h = state.pipelineCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var chain = state.chains[state.selectedChain];
        if (!chain || !chain.functions || chain.functions.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Select a chain to visualize', w / 2, h / 2);
            return;
        }

        var funcs = chain.functions;
        var nodeW = CONFIG.pipelineNodeW;
        var nodeH = CONFIG.pipelineNodeH;
        var gap = CONFIG.pipelineGap;
        var totalW = funcs.length * nodeW + (funcs.length - 1) * gap;
        var startX = (w - totalW) / 2;
        var nodeY = (h - nodeH) / 2;

        for (var i = 0; i < funcs.length; i++) {
            var fn = funcs[i];
            var x = startX + i * (nodeW + gap);

            // Edge (arrow)
            if (i > 0) {
                var fromX = startX + (i - 1) * (nodeW + gap) + nodeW;
                ctx.strokeStyle = CONFIG.colors.pipelineEdge;
                ctx.lineWidth = 2;
                ctx.beginPath();
                ctx.moveTo(fromX, nodeY + nodeH / 2);
                ctx.lineTo(x, nodeY + nodeH / 2);
                ctx.stroke();

                // Arrow
                ctx.fillStyle = CONFIG.colors.pipelineArrow;
                ctx.beginPath();
                ctx.moveTo(x, nodeY + nodeH / 2);
                ctx.lineTo(x - 8, nodeY + nodeH / 2 - 4);
                ctx.lineTo(x - 8, nodeY + nodeH / 2 + 4);
                ctx.closePath();
                ctx.fill();
            }

            // Node
            var nodeColor = CONFIG.latencyColors(fn.latency_ms || 0);
            ctx.fillStyle = CONFIG.colors.nodeFill;
            ctx.strokeStyle = nodeColor;
            ctx.lineWidth = 2;
            roundRect(ctx, x, nodeY, nodeW, nodeH, 6);

            // Function name
            ctx.fillStyle = CONFIG.colors.text;
            ctx.font = '10px "SF Mono", monospace';
            ctx.textAlign = 'center';
            var displayName = (fn.name || 'fn' + i);
            if (displayName.length > 12) displayName = displayName.substring(0, 12);
            ctx.fillText(displayName, x + nodeW / 2, nodeY + 16);

            // Latency bar
            var barW = nodeW - 16;
            var barH = 6;
            var barX = x + 8;
            var barY = nodeY + 24;
            ctx.fillStyle = 'rgba(255,255,255,0.05)';
            ctx.fillRect(barX, barY, barW, barH);
            var pct = Math.min((fn.latency_ms || 0) / 10, 1);
            ctx.fillStyle = nodeColor;
            ctx.fillRect(barX, barY, barW * pct, barH);

            // Latency text
            ctx.fillStyle = nodeColor;
            ctx.font = '9px "SF Mono", monospace';
            ctx.fillText((fn.latency_ms || 0).toFixed(1) + 'ms', x + nodeW / 2, nodeY + 42);

            // Decision count
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '9px "SF Mono", monospace';
            ctx.fillText((fn.decisions || 0) + ' dec', x + nodeW / 2, nodeY + 54);
        }
    }

    function roundRect(ctx, x, y, w, h, r) {
        ctx.beginPath();
        ctx.moveTo(x + r, y);
        ctx.lineTo(x + w - r, y);
        ctx.quadraticCurveTo(x + w, y, x + w, y + r);
        ctx.lineTo(x + w, y + h - r);
        ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
        ctx.lineTo(x + r, y + h);
        ctx.quadraticCurveTo(x, y + h, x, y + h - r);
        ctx.lineTo(x, y + r);
        ctx.quadraticCurveTo(x, y, x + r, y);
        ctx.closePath();
        ctx.fill();
        ctx.stroke();
    }

    // ========================================================================
    // Rendering - Function Heatmap
    // ========================================================================
    function renderHeatmap() {
        var ctx = state.heatmapCtx;
        if (!ctx || !state.heatmapCanvas) return;
        var w = state.heatmapCanvas.width / (window.devicePixelRatio || 1);
        var h = state.heatmapCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.functionHeatmap;
        var names = state.functionNames;
        if (!data || data.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No function heatmap data', w / 2, h / 2);
            return;
        }

        var numFuncs = data.length;
        var numTime = data[0] ? data[0].length : 0;
        if (numTime === 0) return;

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var cellW = plotW / numTime;
        var cellH = plotH / numFuncs;

        for (var f = 0; f < numFuncs; f++) {
            for (var t = 0; t < numTime; t++) {
                var val = data[f][t] || 0;
                var ratio = Math.min(val / 10, 1);
                var rgb = interpColor(CONFIG.colors.heatGreen, CONFIG.colors.heatRed, ratio);
                ctx.fillStyle = val > 0 ? 'rgb(' + rgb[0] + ',' + rgb[1] + ',' + rgb[2] + ')' :
                    'rgba(255,255,255,0.02)';
                ctx.fillRect(pad.left + t * cellW + 0.5, pad.top + f * cellH + 0.5,
                    Math.max(cellW - 1, 0.5), Math.max(cellH - 1, 0.5));
            }
        }

        // Y labels
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var y = 0; y < numFuncs; y++) {
            var label = (names && names[y]) ? names[y] : 'fn' + y;
            if (label.length > 8) label = label.substring(0, 8);
            ctx.fillText(label, pad.left - 4, pad.top + y * cellH + cellH / 2 + 3);
        }

        // X label
        ctx.textAlign = 'center';
        ctx.fillText('Time', pad.left + plotW / 2, pad.top + plotH + 20);
    }

    function interpColor(c1, c2, t) {
        return [
            Math.round(c1[0] + (c2[0] - c1[0]) * t),
            Math.round(c1[1] + (c2[1] - c1[1]) * t),
            Math.round(c1[2] + (c2[2] - c1[2]) * t)
        ];
    }

    // ========================================================================
    // Rendering - AUTH Gauge
    // ========================================================================
    function renderGauge() {
        var ctx = state.gaugeCtx;
        if (!ctx || !state.gaugeCanvas) return;
        var size = 180;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, size, size);

        var dist = state.authDistribution;
        var total = (dist.NORMAL || 0) + (dist.PRIORITY || 0) + (dist.EXPERIMENTAL || 0);
        if (total === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No AUTH data', size / 2, size / 2);
            return;
        }

        var cx = size / 2;
        var cy = size / 2;
        var radius = 65;
        var startAngle = -Math.PI / 2;
        var keys = ['NORMAL', 'PRIORITY', 'EXPERIMENTAL'];

        for (var i = 0; i < keys.length; i++) {
            var val = dist[keys[i]] || 0;
            var slice = val / total;
            var endAngle = startAngle + slice * Math.PI * 2;
            ctx.fillStyle = CONFIG.authColors[keys[i]] || CONFIG.colors.textMuted;
            ctx.beginPath();
            ctx.moveTo(cx, cy);
            ctx.arc(cx, cy, radius, startAngle, endAngle);
            ctx.closePath();
            ctx.fill();
            startAngle = endAngle;
        }

        // Donut
        ctx.fillStyle = CONFIG.colors.background;
        ctx.beginPath();
        ctx.arc(cx, cy, radius * 0.55, 0, Math.PI * 2);
        ctx.fill();

        // Center text
        ctx.fillStyle = CONFIG.colors.text;
        ctx.font = 'bold 16px -apple-system, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(String(total), cx, cy + 6);

        // Legend
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'left';
        for (var j = 0; j < keys.length; j++) {
            var ly = size - 40 + j * 14;
            ctx.fillStyle = CONFIG.authColors[keys[j]];
            ctx.fillRect(10, ly - 5, 8, 8);
            ctx.fillStyle = CONFIG.colors.text;
            ctx.fillText(keys[j] + ': ' + ((dist[keys[j]] || 0) / total * 100).toFixed(1) + '%', 24, ly + 2);
        }
    }

    // ========================================================================
    // Rendering - Chain Table & Registry
    // ========================================================================
    function renderChainTable() {
        if (!state.chainTable) return;
        state.chainTable.innerHTML = '';

        for (var i = 0; i < state.chains.length; i++) {
            var c = state.chains[i];
            var isSelected = (i === state.selectedChain);
            var tr = document.createElement('tr');
            tr.dataset.index = i;
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';cursor:pointer;' +
                (isSelected ? 'background:rgba(255,215,0,0.05);' : '');
            tr.addEventListener('click', function() {
                state.selectedChain = parseInt(this.dataset.index, 10);
                renderChainTable();
            });

            var dropColor = (c.drop_rate || 0) > 0.01 ? '#ff4757' : CONFIG.colors.text;
            tr.innerHTML =
                '<td style="padding:4px 8px;color:' + (isSelected ? CONFIG.colors.accent : CONFIG.colors.textMuted) + '">' +
                (c.id || i) + '</td>' +
                '<td style="padding:4px 8px;font-weight:' + (isSelected ? 'bold' : 'normal') + '">' +
                (c.name || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (c.functions ? c.functions.length : 0) + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (c.total_latency || 0).toFixed(1) + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + formatNumber(c.pps || 0) + '</td>' +
                '<td style="padding:4px 8px;text-align:right;color:' + dropColor + '">' +
                ((c.drop_rate || 0) * 100).toFixed(2) + '%</td>';
            state.chainTable.appendChild(tr);
        }
    }

    function renderFuncRegistry() {
        var list = document.getElementById('nfv-func-list');
        if (!list) return;
        list.innerHTML = '';

        for (var i = 0; i < state.functionRegistry.length; i++) {
            var fn = state.functionRegistry[i];
            var item = document.createElement('div');
            item.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:4px 6px;' +
                'border-bottom:1px solid ' + CONFIG.colors.border + ';font-size:10px;';
            item.innerHTML =
                '<span>' + (fn.name || fn) + '</span>' +
                '<button style="padding:1px 6px;background:transparent;color:' + CONFIG.colors.accent +
                ';border:1px solid ' + CONFIG.colors.border + ';border-radius:2px;cursor:pointer;font-size:9px">+</button>';
            list.appendChild(item);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderPipeline();
            renderHeatmap();
            renderGauge();
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
        if (state.container) state.container.innerHTML = '';
        state.chains = [];
        state.functionHeatmap = [];
        state.functionNames = [];
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

window.NFVPanel = NFVPanel;
console.log('nfv-panel.js loaded');
