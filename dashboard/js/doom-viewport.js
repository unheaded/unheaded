/**
 * Doom Viewport — Real-time MBC framebuffer renderer for the Unheaded Dashboard.
 *
 * Renders the 320x200 8-bit palette screen from the Monad CPU BPF program.
 * Data arrives via WebSocket as `compute_screen` messages containing the
 * raw pixel buffer (or as periodic polls of the SCREEN_MAP BPF array).
 *
 * Architecture:
 *   BPF SCREEN_MAP (64000 bytes) → dashboard-backend → WebSocket → Canvas
 *
 * The viewport also shows an overlay with CPU trace info:
 *   PC, register values, IPS (instructions per second), cache hit rate, FPS.
 */
(function () {
    'use strict';

    // Screen dimensions (MBC framebuffer)
    var SCREEN_W = 320;
    var SCREEN_H = 200;
    var SCALE = 2; // 2x upscale for visibility (640x400 actual canvas)

    // Doom palette (subset — classic 256-color VGA palette approximation).
    // Full palette would be loaded from WAD; this is a reasonable default.
    var PALETTE = buildDefaultPalette();

    // Module state
    var canvas = null;
    var ctx = null;
    var imageData = null;
    var pixels = null; // Uint8ClampedArray backing imageData
    var screenBuffer = new Uint8Array(SCREEN_W * SCREEN_H); // latest frame

    var overlayEnabled = true;
    var stats = {
        fps: 0,
        ips: 0,
        pc: 0,
        cacheHitRate: 0,
        frameCount: 0,
        lastFrameTime: 0,
        fpsAccum: 0,
        fpsFrames: 0
    };

    var ws = null;
    var wsUrl = 'ws://localhost:8080/ws';
    var reconnectTimer = null;
    var animFrameId = null;
    var statusCallbacks = [];

    // ── Initialization ──────────────────────────────────────────────────────

    function init(containerId, options) {
        options = options || {};
        if (options.wsUrl) wsUrl = options.wsUrl;
        if (options.scale) SCALE = options.scale;

        var container = document.getElementById(containerId);
        if (!container) {
            console.error('[DoomViewport] Container not found:', containerId);
            return;
        }

        // Create canvas
        canvas = document.createElement('canvas');
        canvas.width = SCREEN_W * SCALE;
        canvas.height = SCREEN_H * SCALE;
        canvas.className = 'doom-canvas';
        canvas.setAttribute('tabindex', '0');
        canvas.setAttribute('role', 'img');
        canvas.setAttribute('aria-label', 'MBC framebuffer output (320x200)');
        container.appendChild(canvas);

        ctx = canvas.getContext('2d');
        ctx.imageSmoothingEnabled = false; // crispy pixels

        // Create an offscreen image at native resolution.
        imageData = ctx.createImageData(SCREEN_W, SCREEN_H);
        pixels = imageData.data;

        // Fill with startup pattern (dark gradient).
        fillStartupPattern();
        renderFrame();

        // Create overlay container
        var overlay = document.createElement('div');
        overlay.className = 'doom-overlay';
        overlay.id = 'doom-overlay';
        overlay.innerHTML = [
            '<div class="doom-stat"><span class="doom-stat-label">FPS</span><span class="doom-stat-value" id="doom-fps">0</span></div>',
            '<div class="doom-stat"><span class="doom-stat-label">IPS</span><span class="doom-stat-value" id="doom-ips">0</span></div>',
            '<div class="doom-stat"><span class="doom-stat-label">PC</span><span class="doom-stat-value" id="doom-pc">0x0000</span></div>',
            '<div class="doom-stat"><span class="doom-stat-label">Cache</span><span class="doom-stat-value" id="doom-cache">--%</span></div>',
        ].join('');
        container.appendChild(overlay);

        // Keyboard capture (forward to Wotan compute.input topic).
        canvas.addEventListener('keydown', function (e) {
            sendKeyEvent(e.keyCode, true);
            e.preventDefault();
        });
        canvas.addEventListener('keyup', function (e) {
            sendKeyEvent(e.keyCode, false);
            e.preventDefault();
        });

        // Connect WebSocket (reuse existing dashboard WS or create new).
        connectWebSocket();

        // Start render loop
        animFrameId = requestAnimationFrame(renderLoop);

        console.log('[DoomViewport] Initialized (%dx%d, scale=%d)', SCREEN_W, SCREEN_H, SCALE);
    }

    function destroy() {
        if (animFrameId) cancelAnimationFrame(animFrameId);
        if (reconnectTimer) clearTimeout(reconnectTimer);
        if (ws) ws.close();
        if (canvas && canvas.parentElement) canvas.parentElement.removeChild(canvas);
        canvas = null;
        ctx = null;
    }

    // ── WebSocket ───────────────────────────────────────────────────────────

    function connectWebSocket() {
        if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
            return;
        }

        try {
            ws = new WebSocket(wsUrl);
        } catch (e) {
            scheduleReconnect();
            return;
        }

        ws.binaryType = 'arraybuffer';

        ws.onopen = function () {
            console.log('[DoomViewport] WebSocket connected');
            notifyStatus('connected');
            // Subscribe to compute screen updates
            ws.send(JSON.stringify({
                type: 'subscribe',
                channels: ['compute_screen', 'compute_stats']
            }));
        };

        ws.onmessage = function (event) {
            if (event.data instanceof ArrayBuffer) {
                // Binary frame: raw 320x200 pixel buffer (64000 bytes).
                handleBinaryFrame(new Uint8Array(event.data));
            } else {
                // JSON message
                try {
                    var msg = JSON.parse(event.data);
                    handleMessage(msg);
                } catch (e) {
                    // ignore parse errors
                }
            }
        };

        ws.onerror = function () {
            notifyStatus('error');
        };

        ws.onclose = function () {
            notifyStatus('disconnected');
            scheduleReconnect();
        };
    }

    function scheduleReconnect() {
        if (reconnectTimer) return;
        reconnectTimer = setTimeout(function () {
            reconnectTimer = null;
            connectWebSocket();
        }, 3000);
    }

    function handleBinaryFrame(data) {
        if (data.length >= SCREEN_W * SCREEN_H) {
            screenBuffer.set(data.subarray(0, SCREEN_W * SCREEN_H));
            stats.frameCount++;
            updateFPS();
        }
    }

    function handleMessage(msg) {
        switch (msg.type) {
            case 'compute_screen':
                // Base64-encoded screen buffer.
                if (msg.pixels) {
                    var decoded = atob(msg.pixels);
                    for (var i = 0; i < decoded.length && i < screenBuffer.length; i++) {
                        screenBuffer[i] = decoded.charCodeAt(i);
                    }
                    stats.frameCount++;
                    updateFPS();
                }
                break;

            case 'compute_stats':
                if (msg.pc !== undefined) stats.pc = msg.pc;
                if (msg.ips !== undefined) stats.ips = msg.ips;
                if (msg.cache_hit_rate !== undefined) stats.cacheHitRate = msg.cache_hit_rate;
                break;

            case 'pong':
                break;
        }
    }

    function sendKeyEvent(keyCode, pressed) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({
                type: 'compute_input',
                key_code: keyCode,
                pressed: pressed
            }));
        }
    }

    // ── Rendering ───────────────────────────────────────────────────────────

    function renderLoop() {
        renderFrame();
        updateOverlay();
        animFrameId = requestAnimationFrame(renderLoop);
    }

    function renderFrame() {
        if (!ctx || !imageData) return;

        // Convert 8-bit palette indices to RGBA.
        for (var i = 0; i < SCREEN_W * SCREEN_H; i++) {
            var palIdx = screenBuffer[i];
            var rgb = PALETTE[palIdx];
            var p = i * 4;
            pixels[p] = rgb[0];     // R
            pixels[p + 1] = rgb[1]; // G
            pixels[p + 2] = rgb[2]; // B
            pixels[p + 3] = 255;    // A
        }

        // Draw at native resolution, then let CSS scale (or manual scale).
        // For crispy pixels, we draw to an offscreen canvas then scale.
        var offscreen = document.createElement('canvas');
        offscreen.width = SCREEN_W;
        offscreen.height = SCREEN_H;
        var offCtx = offscreen.getContext('2d');
        offCtx.putImageData(imageData, 0, 0);

        ctx.clearRect(0, 0, canvas.width, canvas.height);
        ctx.drawImage(offscreen, 0, 0, canvas.width, canvas.height);
    }

    function updateOverlay() {
        if (!overlayEnabled) return;

        var fpsEl = document.getElementById('doom-fps');
        var ipsEl = document.getElementById('doom-ips');
        var pcEl = document.getElementById('doom-pc');
        var cacheEl = document.getElementById('doom-cache');

        if (fpsEl) fpsEl.textContent = stats.fps.toFixed(1);
        if (ipsEl) ipsEl.textContent = formatNumber(stats.ips);
        if (pcEl) pcEl.textContent = '0x' + stats.pc.toString(16).toUpperCase().padStart(4, '0');
        if (cacheEl) cacheEl.textContent = (stats.cacheHitRate * 100).toFixed(0) + '%';
    }

    function updateFPS() {
        var now = performance.now();
        if (stats.lastFrameTime > 0) {
            var dt = now - stats.lastFrameTime;
            stats.fpsAccum += dt;
            stats.fpsFrames++;
            if (stats.fpsAccum >= 1000) {
                stats.fps = (stats.fpsFrames / stats.fpsAccum) * 1000;
                stats.fpsAccum = 0;
                stats.fpsFrames = 0;
            }
        }
        stats.lastFrameTime = now;
    }

    // ── Startup Pattern ─────────────────────────────────────────────────────

    function fillStartupPattern() {
        // Fill with a dark gradient + "MONAD CPU" text placeholder.
        for (var y = 0; y < SCREEN_H; y++) {
            for (var x = 0; x < SCREEN_W; x++) {
                // Dark blue gradient
                screenBuffer[y * SCREEN_W + x] = Math.floor((y / SCREEN_H) * 16);
            }
        }

        // Draw a centered border rectangle.
        var bx = 80, by = 70, bw = 160, bh = 60;
        for (var x = bx; x < bx + bw; x++) {
            screenBuffer[by * SCREEN_W + x] = 44;         // top
            screenBuffer[(by + bh) * SCREEN_W + x] = 44;  // bottom
        }
        for (var y = by; y <= by + bh; y++) {
            screenBuffer[y * SCREEN_W + bx] = 44;         // left
            screenBuffer[y * SCREEN_W + bx + bw] = 44;    // right
        }
    }

    // ── Demo: Load a test pattern from an MBC program ───────────────────────

    /**
     * Simulate running a gradient demo locally (fills screen buffer directly).
     * Useful when no BPF backend is connected.
     */
    function runDemoGradient() {
        for (var y = 0; y < SCREEN_H; y++) {
            for (var x = 0; x < SCREEN_W; x++) {
                screenBuffer[y * SCREEN_W + x] = (x + y) & 0xFF;
            }
        }
        stats.frameCount++;
        updateFPS();
    }

    function runDemoCheckerboard() {
        for (var y = 0; y < SCREEN_H; y++) {
            for (var x = 0; x < SCREEN_W; x++) {
                var tileX = Math.floor(x / 8);
                var tileY = Math.floor(y / 8);
                var checker = (tileX + tileY) & 1;
                screenBuffer[y * SCREEN_W + x] = checker ? 15 : 1;
            }
        }
        stats.frameCount++;
        updateFPS();
    }

    // ── Palette ─────────────────────────────────────────────────────────────

    function buildDefaultPalette() {
        // Build a reasonable 256-color palette similar to VGA default.
        // Index 0 = black, 1 = dark blue, ..., 15 = white, then gradients.
        var pal = new Array(256);

        // CGA-like first 16 colors
        var cga16 = [
            [0, 0, 0],       // 0: black
            [0, 0, 170],     // 1: blue
            [0, 170, 0],     // 2: green
            [0, 170, 170],   // 3: cyan
            [170, 0, 0],     // 4: red
            [170, 0, 170],   // 5: magenta
            [170, 85, 0],    // 6: brown
            [170, 170, 170], // 7: light gray
            [85, 85, 85],    // 8: dark gray
            [85, 85, 255],   // 9: light blue
            [85, 255, 85],   // 10: light green
            [85, 255, 255],  // 11: light cyan
            [255, 85, 85],   // 12: light red
            [255, 85, 255],  // 13: light magenta
            [255, 255, 85],  // 14: yellow
            [255, 255, 255]  // 15: white
        ];

        for (var i = 0; i < 16; i++) {
            pal[i] = cga16[i];
        }

        // Fill 16-255 with a smooth gradient (HSV sweep).
        for (var i = 16; i < 256; i++) {
            var t = (i - 16) / 240.0;
            var h = t * 360;
            var s = 0.8;
            var v = 0.5 + t * 0.5;
            pal[i] = hsvToRgb(h, s, v);
        }

        return pal;
    }

    function hsvToRgb(h, s, v) {
        var c = v * s;
        var x = c * (1 - Math.abs(((h / 60) % 2) - 1));
        var m = v - c;
        var r, g, b;

        if (h < 60) { r = c; g = x; b = 0; }
        else if (h < 120) { r = x; g = c; b = 0; }
        else if (h < 180) { r = 0; g = c; b = x; }
        else if (h < 240) { r = 0; g = x; b = c; }
        else if (h < 300) { r = x; g = 0; b = c; }
        else { r = c; g = 0; b = x; }

        return [
            Math.round((r + m) * 255),
            Math.round((g + m) * 255),
            Math.round((b + m) * 255)
        ];
    }

    // ── Utilities ────────────────────────────────────────────────────────────

    function formatNumber(n) {
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
        return n.toString();
    }

    function notifyStatus(status) {
        for (var i = 0; i < statusCallbacks.length; i++) {
            statusCallbacks[i](status);
        }
    }

    function onStatusChange(cb) {
        statusCallbacks.push(cb);
    }

    // ── Public API ──────────────────────────────────────────────────────────

    window.DoomViewport = {
        init: init,
        destroy: destroy,
        onStatusChange: onStatusChange,
        runDemoGradient: runDemoGradient,
        runDemoCheckerboard: runDemoCheckerboard,
        getStats: function () { return Object.assign({}, stats); },
        toggleOverlay: function () {
            overlayEnabled = !overlayEnabled;
            var el = document.getElementById('doom-overlay');
            if (el) el.style.display = overlayEnabled ? '' : 'none';
        },
        getScreenBuffer: function () { return new Uint8Array(screenBuffer); }
    };

    // Auto-init when DOM is ready.
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', function () {
            var el = document.getElementById('doom-container');
            if (el) init('doom-container');
        });
    } else {
        var el = document.getElementById('doom-container');
        if (el) init('doom-container');
    }
})();
