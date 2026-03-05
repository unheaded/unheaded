/**
 * pqc-tier-status.js - PQC Compliance Tier Status Cards
 * Four tier cards showing NONE/STANDARD/ENHANCED/SOVEREIGN compliance levels.
 *
 * Features:
 *   - 4 cards with active services count, verification mode, last check time
 *   - Color-coded borders (green/yellow/orange/red by tier)
 *   - Current tier highlighted with accent glow
 *   - Canvas-rendered status indicators
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var PqcTierStatus = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        tiers: [
            { key: 'NONE',       label: 'None',       color: '#6b6b7b', borderColor: '#6b6b7b' },
            { key: 'STANDARD',   label: 'Standard',   color: '#00d26a', borderColor: '#00d26a' },
            { key: 'ENHANCED',   label: 'Enhanced',   color: '#ffd700', borderColor: '#ffd700' },
            { key: 'SOVEREIGN',  label: 'Sovereign',  color: '#ff5c00', borderColor: '#ff5c00' }
        ],
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            cardBg: '#16162b',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            activeGlow: 'rgba(255, 215, 0, 0.25)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        canvases: {},           // keyed by tier key
        ctxs: {},
        tierData: {},           // { NONE: { active: N, mode: '', lastCheck: ts }, ... }
        currentTier: 'STANDARD',
        animationFrame: null,
        resizeHandler: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[PqcTierStatus] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        // Initialize tier data
        for (var i = 0; i < CONFIG.tiers.length; i++) {
            state.tierData[CONFIG.tiers[i].key] = { active: 0, mode: '--', lastCheck: null };
        }

        buildLayout(container);

        state.resizeHandler = debounce(resizeCanvases, 150);
        window.addEventListener('resize', state.resizeHandler);
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[PqcTierStatus] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent +
            ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;border:1px solid ' +
            CONFIG.colors.border + ';border-radius:6px 6px 0 0;margin-bottom:0;';
        hdr.textContent = 'PQC Compliance Tiers';
        parent.appendChild(hdr);

        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(4, 1fr);gap:12px;padding:12px;' +
            'background:' + CONFIG.colors.background + ';border:1px solid ' + CONFIG.colors.border +
            ';border-top:none;border-radius:0 0 6px 6px;';

        for (var i = 0; i < CONFIG.tiers.length; i++) {
            var tier = CONFIG.tiers[i];
            var card = document.createElement('div');
            card.id = 'pqc-tier-card-' + tier.key;
            card.style.cssText = 'border:2px solid ' + tier.borderColor + ';border-radius:6px;overflow:hidden;' +
                'background:' + CONFIG.colors.cardBg + ';transition:box-shadow 0.3s;';

            var canvas = document.createElement('canvas');
            canvas.style.cssText = 'display:block;width:100%;';
            card.appendChild(canvas);

            state.canvases[tier.key] = canvas;
            state.ctxs[tier.key] = canvas.getContext('2d');

            grid.appendChild(card);
        }

        parent.appendChild(grid);
    }

    // ========================================================================
    // Canvas Management
    // ========================================================================
    function resizeCanvases() {
        for (var i = 0; i < CONFIG.tiers.length; i++) {
            var key = CONFIG.tiers[i].key;
            var canvas = state.canvases[key];
            var ctx = state.ctxs[key];
            if (!canvas || !canvas.parentElement) continue;

            var dpr = window.devicePixelRatio || 1;
            var rect = canvas.parentElement.getBoundingClientRect();
            var w = rect.width || 150;
            var h = 160;
            canvas.width = w * dpr;
            canvas.height = h * dpr;
            canvas.style.width = w + 'px';
            canvas.style.height = h + 'px';
            ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        }
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(tierData) {
        if (!state.initialized || !tierData) return;

        if (tierData.currentTier) {
            state.currentTier = tierData.currentTier;
        }

        if (tierData.tiers) {
            for (var key in tierData.tiers) {
                if (tierData.tiers.hasOwnProperty(key) && state.tierData.hasOwnProperty(key)) {
                    var d = tierData.tiers[key];
                    if (d.active != null) state.tierData[key].active = d.active;
                    if (d.mode != null) state.tierData[key].mode = d.mode;
                    if (d.lastCheck != null) state.tierData[key].lastCheck = d.lastCheck;
                }
            }
        }

        // Update card highlighting
        for (var i = 0; i < CONFIG.tiers.length; i++) {
            var t = CONFIG.tiers[i];
            var card = document.getElementById('pqc-tier-card-' + t.key);
            if (!card) continue;
            if (t.key === state.currentTier) {
                card.style.boxShadow = '0 0 12px ' + CONFIG.colors.activeGlow + ', inset 0 0 8px ' + CONFIG.colors.activeGlow;
            } else {
                card.style.boxShadow = 'none';
            }
        }
    }

    // ========================================================================
    // Rendering
    // ========================================================================
    function render() {
        for (var i = 0; i < CONFIG.tiers.length; i++) {
            renderCard(CONFIG.tiers[i]);
        }
    }

    function renderCard(tier) {
        var ctx = state.ctxs[tier.key];
        var canvas = state.canvases[tier.key];
        if (!ctx || !canvas) return;

        var dpr = window.devicePixelRatio || 1;
        var w = canvas.width / dpr;
        var h = canvas.height / dpr;
        var data = state.tierData[tier.key];
        var isCurrent = (tier.key === state.currentTier);

        ctx.fillStyle = CONFIG.colors.cardBg;
        ctx.fillRect(0, 0, w, h);

        // Tier label header
        ctx.fillStyle = isCurrent ? tier.color : CONFIG.colors.textMuted;
        ctx.font = 'bold 13px "SF Mono", monospace';
        ctx.textAlign = 'center';
        ctx.fillText(tier.label.toUpperCase(), w / 2, 24);

        // Status dot
        var dotRadius = 5;
        var dotX = w / 2;
        var dotY = 42;
        ctx.beginPath();
        ctx.arc(dotX, dotY, dotRadius, 0, Math.PI * 2);
        ctx.fillStyle = isCurrent ? tier.color : CONFIG.colors.textMuted;
        ctx.fill();
        if (isCurrent) {
            ctx.beginPath();
            ctx.arc(dotX, dotY, dotRadius + 3, 0, Math.PI * 2);
            ctx.strokeStyle = tier.color;
            ctx.lineWidth = 1;
            ctx.globalAlpha = 0.4 + 0.2 * Math.sin(Date.now() / 500);
            ctx.stroke();
            ctx.globalAlpha = 1;
        }

        // Active services count
        ctx.font = 'bold 26px -apple-system, sans-serif';
        ctx.fillStyle = isCurrent ? CONFIG.colors.text : CONFIG.colors.textMuted;
        ctx.textAlign = 'center';
        ctx.fillText(String(data.active), w / 2, 80);

        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText('active services', w / 2, 94);

        // Verification mode
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = isCurrent ? tier.color : CONFIG.colors.textMuted;
        ctx.fillText(data.mode || '--', w / 2, 116);

        ctx.font = '8px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText('verify mode', w / 2, 128);

        // Last check time
        var lastStr = data.lastCheck ? formatTimestamp(data.lastCheck) : '--:--:--';
        ctx.font = '9px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.textMuted;
        ctx.fillText('last: ' + lastStr, w / 2, 150);
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() {
            render();
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
    function formatTimestamp(ts) {
        var d = new Date(ts);
        var hh = String(d.getHours()).padStart(2, '0');
        var mm = String(d.getMinutes()).padStart(2, '0');
        var ss = String(d.getSeconds()).padStart(2, '0');
        return hh + ':' + mm + ':' + ss;
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
        if (state.resizeHandler) window.removeEventListener('resize', state.resizeHandler);
        if (state.container) state.container.innerHTML = '';
        state.canvases = {};
        state.ctxs = {};
        state.tierData = {};
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

window.PqcTierStatus = PqcTierStatus;
console.log('pqc-tier-status.js loaded');
