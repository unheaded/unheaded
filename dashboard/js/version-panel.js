/**
 * version-panel.js - Protocol Version Management Panel (App 11 - Monad CPU)
 * Version distribution tracking, translation stats, and rollout progress.
 *
 * Features:
 *   - Version distribution heatmap: X=time, Y=version byte, color=packet count
 *   - Service version support matrix: grid (services x versions), cell color: green=supported,
 *     yellow=fallback, red=unsupported
 *   - Translation stats: total translations, errors, avg latency
 *   - Rollout progress: per-version deployment percentage bar
 *   - Active rules table: src_ver -> dst_ver, field_mask, CRC recalc
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var VersionPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        heatmapTimePoints: 60,
        maxVersions: 16,

        supportColors: {
            'supported':  '#00d26a',
            'fallback':   '#ffd700',
            'unsupported': '#ff4757'
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
            heatLow: [15, 15, 30],
            heatHigh: [255, 215, 0],
            progressBg: 'rgba(255, 255, 255, 0.05)',
            progressFill: '#4ecdc4'
        },
        padding: { top: 20, right: 16, bottom: 30, left: 50 }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        heatmapCanvas: null,
        heatmapCtx: null,
        matrixCanvas: null,
        matrixCtx: null,

        // Data
        versionHeatmap: [],  // [versionIdx][timeIdx] = packet_count
        versionLabels: [],   // ['0x01', '0x02', ...]
        supportMatrix: null, // { services: [names], versions: [labels], matrix: [['supported',...]] }
        translationStats: { total: 0, errors: 0, avg_latency_ms: 0 },
        rolloutProgress: [], // [{ version, percentage }]
        activeRules: [],     // [{ src_ver, dst_ver, field_mask, crc_recalc }]

        rulesBody: null,
        rolloutContainer: null,
        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[VersionPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[VersionPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Stats bar
        var statsBar = document.createElement('div');
        statsBar.id = 'ver-stats-bar';
        statsBar.style.cssText = 'display:flex;gap:20px;padding:12px 16px;background:' + CONFIG.colors.headerBg +
            ';border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;margin-bottom:12px;font-size:12px;';
        statsBar.innerHTML =
            '<span style="font-weight:bold;color:' + CONFIG.colors.accent + '">TRANSLATION STATS:</span>' +
            '<span>Total: <strong id="ver-stat-total">--</strong></span>' +
            '<span>Errors: <strong id="ver-stat-errors" style="color:#ff4757">--</strong></span>' +
            '<span>Avg Latency: <strong id="ver-stat-latency" style="color:#4ecdc4">-- ms</strong></span>';
        parent.appendChild(statsBar);

        // Top: heatmap + support matrix
        var topGrid = document.createElement('div');
        topGrid.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var heatSection = createSection('Version Distribution Over Time');
        state.heatmapCanvas = document.createElement('canvas');
        state.heatmapCanvas.style.cssText = 'display:block;width:100%;';
        heatSection.body.appendChild(state.heatmapCanvas);
        state.heatmapCtx = state.heatmapCanvas.getContext('2d');
        topGrid.appendChild(heatSection.el);

        var matSection = createSection('Service Version Support Matrix');
        state.matrixCanvas = document.createElement('canvas');
        state.matrixCanvas.style.cssText = 'display:block;width:100%;';
        matSection.body.appendChild(state.matrixCanvas);
        state.matrixCtx = state.matrixCanvas.getContext('2d');
        topGrid.appendChild(matSection.el);

        parent.appendChild(topGrid);

        // Rollout progress
        var rollSection = createSection('Rollout Progress');
        state.rolloutContainer = rollSection.body;
        parent.appendChild(rollSection.el);

        // Active rules table
        var rulesSection = createSection('Active Translation Rules');
        buildRulesTable(rulesSection.body);
        parent.appendChild(rulesSection.el);
    }

    function buildRulesTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:250px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Source Version</th>' +
            '<th style="padding:6px 8px;text-align:center">&rarr;</th>' +
            '<th style="padding:6px 8px;text-align:left">Dest Version</th>' +
            '<th style="padding:6px 8px;text-align:left">Field Mask</th>' +
            '<th style="padding:6px 8px;text-align:center">CRC Recalc</th></tr></thead>';
        state.rulesBody = document.createElement('tbody');
        table.appendChild(state.rulesBody);
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
        resizeCanvas(state.heatmapCanvas, state.heatmapCtx, 220);
        resizeCanvas(state.matrixCanvas, state.matrixCtx, 220);
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
    function update(versionData) {
        if (!state.initialized || !versionData) return;

        if (versionData.version_heatmap) state.versionHeatmap = versionData.version_heatmap;
        if (versionData.version_labels) state.versionLabels = versionData.version_labels;
        if (versionData.support_matrix) state.supportMatrix = versionData.support_matrix;
        if (versionData.translation_stats) {
            state.translationStats = versionData.translation_stats;
            updateStats();
        }
        if (versionData.rollout_progress) {
            state.rolloutProgress = versionData.rollout_progress;
            renderRollout();
        }
        if (versionData.active_rules) {
            state.activeRules = versionData.active_rules;
            renderRules();
        }
    }

    function updateStats() {
        var s = state.translationStats;
        setTextById('ver-stat-total', formatNumber(s.total || 0));
        setTextById('ver-stat-errors', formatNumber(s.errors || 0));
        setTextById('ver-stat-latency', (s.avg_latency_ms || 0).toFixed(2) + ' ms');
    }

    // ========================================================================
    // Rendering - Version Heatmap
    // ========================================================================
    function renderVersionHeatmap() {
        var ctx = state.heatmapCtx;
        if (!ctx || !state.heatmapCanvas) return;
        var w = state.heatmapCanvas.width / (window.devicePixelRatio || 1);
        var h = state.heatmapCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.versionHeatmap;
        if (!data || data.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No version distribution data', w / 2, h / 2);
            return;
        }

        var numVer = data.length;
        var numTime = data[0] ? data[0].length : 0;
        if (numTime === 0) return;

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;
        var cellW = plotW / numTime;
        var cellH = plotH / numVer;

        // Find max
        var maxVal = 0;
        for (var v = 0; v < numVer; v++) {
            for (var t = 0; t < numTime; t++) {
                if (data[v][t] > maxVal) maxVal = data[v][t];
            }
        }
        maxVal = maxVal || 1;

        // Draw cells
        for (var vi = 0; vi < numVer; vi++) {
            for (var ti = 0; ti < numTime; ti++) {
                var val = data[vi][ti] || 0;
                var ratio = val / maxVal;
                var rgb = interpColor(CONFIG.colors.heatLow, CONFIG.colors.heatHigh, ratio);
                ctx.fillStyle = val > 0 ? 'rgb(' + rgb[0] + ',' + rgb[1] + ',' + rgb[2] + ')' :
                    'rgba(255,255,255,0.02)';
                ctx.fillRect(pad.left + ti * cellW + 0.5, pad.top + vi * cellH + 0.5,
                    Math.max(cellW - 1, 0.5), Math.max(cellH - 1, 0.5));
            }
        }

        // Y labels
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var yl = 0; yl < numVer; yl++) {
            var label = (state.versionLabels[yl]) || 'v' + yl;
            ctx.fillText(label, pad.left - 4, pad.top + yl * cellH + cellH / 2 + 3);
        }

        ctx.textAlign = 'center';
        ctx.fillText('Time', pad.left + plotW / 2, pad.top + plotH + 20);

        // Title
        ctx.save();
        ctx.translate(12, pad.top + plotH / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.textAlign = 'center';
        ctx.fillText('Version', 0, 0);
        ctx.restore();
    }

    // ========================================================================
    // Rendering - Support Matrix
    // ========================================================================
    function renderSupportMatrix() {
        var ctx = state.matrixCtx;
        if (!ctx || !state.matrixCanvas || !state.supportMatrix) return;
        var w = state.matrixCanvas.width / (window.devicePixelRatio || 1);
        var h = state.matrixCanvas.height / (window.devicePixelRatio || 1);

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var sm = state.supportMatrix;
        var services = sm.services || [];
        var versions = sm.versions || [];
        var matrix = sm.matrix || [];

        if (services.length === 0 || versions.length === 0) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No support matrix data', w / 2, h / 2);
            return;
        }

        var labelW = 80;
        var labelH = 30;
        var plotW = w - labelW - 10;
        var plotH = h - labelH - 30;
        var cellW = plotW / versions.length;
        var cellH = plotH / services.length;

        // Cells
        for (var s = 0; s < services.length; s++) {
            for (var v = 0; v < versions.length; v++) {
                var val = (matrix[s] && matrix[s][v]) || 'unsupported';
                ctx.fillStyle = CONFIG.supportColors[val] || CONFIG.supportColors.unsupported;
                ctx.globalAlpha = 0.7;
                ctx.fillRect(labelW + v * cellW + 1, labelH + s * cellH + 1, cellW - 2, cellH - 2);
                ctx.globalAlpha = 1.0;
            }
        }

        // Row labels
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.textAlign = 'right';
        for (var rl = 0; rl < services.length; rl++) {
            var sLabel = services[rl].length > 10 ? services[rl].substring(0, 10) : services[rl];
            ctx.fillText(sLabel, labelW - 4, labelH + rl * cellH + cellH / 2 + 3);
        }

        // Col labels
        ctx.textAlign = 'center';
        for (var cl = 0; cl < versions.length; cl++) {
            ctx.fillText(versions[cl] || 'v' + cl, labelW + cl * cellW + cellW / 2, labelH - 6);
        }

        // Legend
        var legendY = h - 16;
        ctx.font = '9px "SF Mono", monospace';
        ctx.textAlign = 'left';
        var legendItems = ['supported', 'fallback', 'unsupported'];
        var lx = labelW;
        for (var li = 0; li < legendItems.length; li++) {
            ctx.fillStyle = CONFIG.supportColors[legendItems[li]];
            ctx.fillRect(lx, legendY - 5, 10, 10);
            ctx.fillStyle = CONFIG.colors.text;
            ctx.fillText(legendItems[li], lx + 14, legendY + 3);
            lx += 90;
        }
    }

    // ========================================================================
    // Rendering - Rollout Progress
    // ========================================================================
    function renderRollout() {
        if (!state.rolloutContainer) return;
        state.rolloutContainer.innerHTML = '';

        if (state.rolloutProgress.length === 0) {
            state.rolloutContainer.innerHTML = '<div style="color:' + CONFIG.colors.textMuted +
                ';text-align:center;padding:16px;font-size:12px">No rollout data</div>';
            return;
        }

        for (var i = 0; i < state.rolloutProgress.length; i++) {
            var r = state.rolloutProgress[i];
            var row = document.createElement('div');
            row.style.cssText = 'display:flex;align-items:center;gap:10px;padding:4px 0;font-size:11px;';

            var pct = (r.percentage || 0);
            var pctColor = pct >= 100 ? '#00d26a' : (pct >= 50 ? '#ffd700' : CONFIG.colors.text);
            row.innerHTML =
                '<span style="width:60px;color:' + CONFIG.colors.accent + ';font-weight:bold">' +
                (r.version || '--') + '</span>' +
                '<div style="flex:1;height:16px;background:' + CONFIG.colors.progressBg +
                ';border-radius:3px;overflow:hidden">' +
                '<div style="width:' + pct + '%;height:100%;background:' + CONFIG.colors.progressFill +
                ';border-radius:3px;transition:width 0.5s"></div></div>' +
                '<span style="width:50px;text-align:right;color:' + pctColor + '">' + pct.toFixed(1) + '%</span>';
            state.rolloutContainer.appendChild(row);
        }
    }

    // ========================================================================
    // Rendering - Rules Table
    // ========================================================================
    function renderRules() {
        if (!state.rulesBody) return;
        state.rulesBody.innerHTML = '';

        for (var i = 0; i < state.activeRules.length; i++) {
            var r = state.activeRules[i];
            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px;color:' + CONFIG.colors.accent + '">' + (r.src_ver || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:center;color:' + CONFIG.colors.textMuted + '">&rarr;</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.accent + '">' + (r.dst_ver || '--') + '</td>' +
                '<td style="padding:4px 8px;font-family:monospace;font-size:10px">' + (r.field_mask || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:center;color:' +
                (r.crc_recalc ? '#00d26a' : CONFIG.colors.textMuted) + '">' +
                (r.crc_recalc ? 'YES' : 'NO') + '</td>';
            state.rulesBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            renderVersionHeatmap();
            renderSupportMatrix();
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
    function interpColor(c1, c2, t) {
        return [
            Math.round(c1[0] + (c2[0] - c1[0]) * t),
            Math.round(c1[1] + (c2[1] - c1[1]) * t),
            Math.round(c1[2] + (c2[2] - c1[2]) * t)
        ];
    }

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
        state.versionHeatmap = [];
        state.supportMatrix = null;
        state.rolloutProgress = [];
        state.activeRules = [];
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

window.VersionPanel = VersionPanel;
console.log('version-panel.js loaded');
