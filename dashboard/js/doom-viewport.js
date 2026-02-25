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
 *
 * CPU Trace Overlay (D-013):
 *   - Toggleable with F3 key (separate from game keyboard input)
 *   - Shows live CPU state: PC, registers, flags, IPS, cache hit rate, stall count
 *   - Highlights changed registers in gold, flags in green (set) or gray (clear)
 *   - Semi-transparent dark panel in top-left corner
 */
(function () {
    'use strict';

    // Event type constants (from Anamnesis)
    var EventComputeHop   = 0x10;
    var EventCacheMiss    = 0x11;
    var EventMemWrite     = 0x12;
    var EventMemStaged    = 0x13;
    var EventScreenWrite  = 0x14;
    var EventKeyRead      = 0x15;
    var EventComputeHalt  = 0x16;
    var EventComputeStall = 0x17;

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
    var cpuTrace = null; // CPU trace overlay (D-013)

    // JavaScript keyCode → Doom internal keycode translation
    // Doom keycodes: https://github.com/ozkl/doomgeneric doomkeys.h
    var JS_TO_DOOM_KEY = {
        // Arrow keys
        38: 0xAD,   // ArrowUp → KEY_UPARROW
        40: 0xAF,   // ArrowDown → KEY_DOWNARROW
        37: 0xAC,   // ArrowLeft → KEY_LEFTARROW
        39: 0xAE,   // ArrowRight → KEY_RIGHTARROW
        // WASD → arrow equivalents
        87: 0xAD,   // W → KEY_UPARROW
        83: 0xAF,   // S → KEY_DOWNARROW
        65: 0xAC,   // A → KEY_LEFTARROW
        68: 0xAE,   // D → KEY_RIGHTARROW
        // Action keys
        17: 0xA3,   // Ctrl → KEY_FIRE
        32: 0xA2,   // Space → KEY_USE
        16: 0xB6,   // Shift → KEY_RSHIFT (run)
        18: 0xA4,   // Alt → KEY_STRAFE
        // Weapon selection
        49: 0x31,   // 1
        50: 0x32,   // 2
        51: 0x33,   // 3
        52: 0x34,   // 4
        53: 0x35,   // 5
        54: 0x36,   // 6
        55: 0x37,   // 7
        56: 0x38,   // 8
        // Menu
        27: 0x1B,   // Escape
        13: 0x0D,   // Enter
        9:  0x09,   // Tab → KEY_TAB (automap)
        // Extra
        188: 0xA4,  // , (comma) → KEY_STRAFE
        190: 0xAE   // . (period) → KEY_RIGHTARROW (strafe right)
    };

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

        // Initialize CPU trace overlay (D-013)
        cpuTrace = new CpuTraceOverlay(canvas);

        // Keyboard capture (forward to Wotan compute.input topic).
        // NOTE: F3 is reserved for CPU trace overlay toggle and won't be sent to game
        canvas.addEventListener('keydown', function (e) {
            if (e.code !== 'F3') {  // Don't send F3 to game
                sendKeyEvent(e.keyCode, true);
                e.preventDefault();
            }
        });
        canvas.addEventListener('keyup', function (e) {
            if (e.code !== 'F3') {  // Don't send F3 to game
                sendKeyEvent(e.keyCode, false);
                e.preventDefault();
            }
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

    // ── CPU Trace Overlay (D-013) ────────────────────────────────────────────

    /**
     * CpuTraceOverlay: Semi-transparent CPU debugger overlay showing:
     *   - PC (program counter) with decoded instruction
     *   - Registers r0-r15 (hex, highlight changes in gold)
     *   - Flags: Z/N/C (green when set, gray when clear)
     *   - Instructions/sec (rolling 1-second window)
     *   - Cache hit rate % (hits / (hits+misses) over last 1 second)
     *   - Stall count (per second)
     *   - Hop ID of last instruction
     *
     * Toggle with F3 key (separate from game input).
     */
    function CpuTraceOverlay(canvas) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.visible = false;
        this.lastEvent = null;
        this.prevRegs = new Array(16).fill(0);

        // Rolling stats window (1-second = 1e9 nanoseconds)
        this.recentEvents = [];  // { ts: timestamp_ns, hit: cache_hit, stall: boolean }
        this.statsWindowNs = 1000000000; // 1 second in nanoseconds

        // F3 toggle (separate from game keyboard to avoid conflicts)
        var self = this;
        document.addEventListener('keydown', function (e) {
            if (e.code === 'F3') {
                e.preventDefault();
                self.toggle();
            }
        });
    }

    CpuTraceOverlay.prototype.toggle = function () {
        this.visible = !this.visible;
    };

    CpuTraceOverlay.prototype.onComputeEvent = function (evt) {
        if (evt.event_type !== EventComputeHop) return;

        // Track register changes
        this.prevRegs = this.lastEvent ? this.lastEvent.regs.slice() : new Array(16).fill(0);
        this.lastEvent = evt;

        // Add to rolling window
        var isStall = evt.event_type === EventComputeStall;
        this.recentEvents.push({
            ts: evt.timestamp_ns,
            hit: evt.cache_hit ? 1 : 0,
            stall: isStall
        });

        // Prune events older than 1 second
        var cutoffTs = evt.timestamp_ns - this.statsWindowNs;
        this.recentEvents = this.recentEvents.filter(function (e) {
            return e.ts >= cutoffTs;
        });
    };

    CpuTraceOverlay.prototype.getStats = function () {
        var n = this.recentEvents.length;
        if (n === 0) return { ips: 0, hitRate: 0, stallCount: 0 };

        var hits = 0;
        var stalls = 0;
        for (var i = 0; i < this.recentEvents.length; i++) {
            hits += this.recentEvents[i].hit;
            if (this.recentEvents[i].stall) stalls++;
        }

        // IPS: events per second based on window duration
        var ips = 0;
        if (n > 1) {
            var oldest = this.recentEvents[0].ts;
            var newest = this.recentEvents[n - 1].ts;
            var durationS = (newest - oldest) / 1000000000; // nanoseconds to seconds
            ips = durationS > 0 ? Math.round(n / durationS) : 0;
        }

        var hitRate = Math.round((hits / n) * 100);
        return { ips: ips, hitRate: hitRate, stallCount: stalls };
    };

    CpuTraceOverlay.prototype.decodeOpcode = function (insn) {
        var opcode = (insn >>> 24) & 0xFF;
        var opcodeNames = {
            0x01: 'ADD', 0x02: 'SUB', 0x03: 'MUL', 0x04: 'DIV', 0x05: 'MOD', 0x06: 'NEG',
            0x07: 'AND', 0x08: 'OR', 0x09: 'XOR', 0x0A: 'NOT', 0x0B: 'SHL', 0x0C: 'SHR',
            0x0D: 'SAR', 0x0E: 'CMP',
            0x20: 'JMP', 0x21: 'JZ', 0x22: 'JNZ', 0x23: 'JN', 0x24: 'JP', 0x25: 'JC',
            0x26: 'JNC', 0x27: 'CALL', 0x28: 'RET', 0x29: 'MOV', 0x2A: 'MOVI',
            0x30: 'LD', 0x31: 'ST', 0x32: 'LDB', 0x33: 'STB', 0x34: 'LDH', 0x35: 'STH',
            0x40: 'SYSCALL', 0xFF: 'HALT'
        };

        var name = opcodeNames[opcode] || ('0x' + opcode.toString(16).toUpperCase());

        // Special handling for SYSCALL
        if (opcode === 0x40) {
            var imm = insn & 0xFFFF;
            var syscallNames = { 1: 'DRAW_FRAME', 2: 'GET_KEY', 3: 'GET_TICKS', 4: 'SLEEP', 0xFF: 'HALT' };
            var scName = syscallNames[imm] || ('0x' + imm.toString(16).toUpperCase());
            return 'SYSCALL ' + scName;
        }

        return name;
    };

    CpuTraceOverlay.prototype.render = function () {
        // Guard: no-op if hidden or no event yet
        if (!this.visible || !this.lastEvent) return;

        var evt = this.lastEvent;
        var ctx = this.ctx;
        var stats = this.getStats();

        // Overlay panel dimensions
        var x = 8, y = 8;
        var w = 300;
        var lineH = 13;
        var headerLines = 3;  // Title, PC+insn, FLAGS
        var regLines = 4;     // 4 rows of 4 registers each
        var statsLines = 1;   // IPS + cache + stall + hop
        var footerLines = 1;  // F3 hint
        var totalLines = headerLines + regLines + statsLines + footerLines;
        var h = totalLines * lineH + 16;

        // Save canvas state
        ctx.save();

        // Semi-transparent dark panel background
        ctx.fillStyle = 'rgba(0, 0, 0, 0.75)';
        ctx.fillRect(x, y, w, h);

        // Green border
        ctx.strokeStyle = 'rgba(0, 200, 0, 0.5)';
        ctx.lineWidth = 1;
        ctx.strokeRect(x, y, w, h);

        // Font setup
        ctx.font = '11px monospace';
        ctx.textBaseline = 'top';
        var ly = y + 10;

        // Title
        ctx.fillStyle = '#00FF00';
        ctx.fillText('MONAD CPU TRACE', x + 4, ly);
        ly += lineH;

        // PC + opcode name
        ctx.fillStyle = '#FFFF00';
        var pcHex = evt.pc.toString(16).padStart(6, '0').toUpperCase();
        var opname = this.decodeOpcode(evt.instruction);
        ctx.fillText('PC: 0x' + pcHex + '  ' + opname, x + 4, ly);
        ly += lineH;

        // Flags: Z, N, C
        var flags = evt.flags || 0;
        var Z = (flags & 1) ? '#00FF00' : '#444444';
        var N = (flags & 2) ? '#00FF00' : '#444444';
        var C = (flags & 4) ? '#00FF00' : '#444444';

        ctx.fillStyle = '#CCCCCC';
        ctx.fillText('FLAGS:', x + 4, ly);
        ctx.fillStyle = Z;
        ctx.fillText('Z', x + 52, ly);
        ctx.fillStyle = N;
        ctx.fillText('N', x + 64, ly);
        ctx.fillStyle = C;
        ctx.fillText('C', x + 76, ly);
        ly += lineH;

        // Registers r0-r15 in 4 columns x 4 rows
        for (var row = 0; row < 4; row++) {
            var tx = x + 4;
            for (var col = 0; col < 4; col++) {
                var ri = row * 4 + col;
                var val = evt.regs[ri];
                var changed = val !== this.prevRegs[ri];

                // Changed registers in gold, unchanged in gray
                ctx.fillStyle = changed ? '#FFD700' : '#AAAAAA';
                var regHex = val.toString(16).padStart(8, '0').toUpperCase();
                ctx.fillText('r' + ri.toString().padStart(2, '0') + ':' + regHex, tx, ly);
                tx += 75;
            }
            ly += lineH;
        }

        // Stats line: IPS, cache hit rate, stall count, hop ID
        ctx.fillStyle = '#00FFFF';
        var statsText = 'IPS: ' + stats.ips.toLocaleString() + '  HIT: ' + stats.hitRate + '%  ' +
                        'STALL: ' + stats.stallCount + '  HOP: ' + evt.hop_id;
        ctx.fillText(statsText, x + 4, ly);
        ly += lineH;

        // Footer hint
        ctx.fillStyle = '#FF6600';
        ctx.fillText('[F3] Toggle', x + 4, ly);

        ctx.restore();
    };

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
            // Subscribe to compute screen updates and CPU trace events (D-013)
            ws.send(JSON.stringify({
                type: 'subscribe',
                channels: ['compute_screen', 'compute_stats', 'compute_hop']
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
        // Support two formats:
        // 1. Tagged RGBA from doom-bridge: [0x01 tag] + [256000 bytes RGBA]
        // 2. Raw palette data (legacy): 64000 bytes palette indices
        if (data.length > 1 && data[0] === 0x01) {
            // Tagged RGBA frame from doom-bridge
            var rgba = data.subarray(1);
            var expectedLen = SCREEN_W * SCREEN_H * 4;
            if (rgba.length >= expectedLen && imageData) {
                // Write RGBA directly to imageData (skip palette conversion)
                imageData.data.set(rgba.subarray(0, expectedLen));
                // We still need to update screenBuffer for demos/overlay compatibility
                for (var i = 0; i < SCREEN_W * SCREEN_H; i++) {
                    screenBuffer[i] = rgba[i * 4]; // Store R channel as rough index
                }
                stats.frameCount++;
                updateFPS();
            }
        } else if (data.length >= SCREEN_W * SCREEN_H) {
            // Legacy: raw 320x200 palette indices
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

            case 'stats':
                // Stats message from doom-bridge service
                if (msg.pc !== undefined) stats.pc = msg.pc;
                if (msg.insns !== undefined) stats.ips = msg.insns;
                if (msg.packets !== undefined) stats.frameCount = msg.packets;
                break;

            case 'compute_stats':
                if (msg.pc !== undefined) stats.pc = msg.pc;
                if (msg.ips !== undefined) stats.ips = msg.ips;
                if (msg.cache_hit_rate !== undefined) stats.cacheHitRate = msg.cache_hit_rate;
                break;

            case 'compute_hop':
                // D-013: CPU trace event with ComputeHopEvent structure
                if (msg.data && cpuTrace) {
                    cpuTrace.onComputeEvent(msg.data);
                }
                break;

            case 'pong':
                break;
        }
    }

    function sendKeyEvent(keyCode, pressed) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            var doomKey = JS_TO_DOOM_KEY[keyCode];
            if (doomKey === undefined) return;  // Unmapped key, ignore
            var buf = new ArrayBuffer(4);
            var view = new DataView(buf);
            view.setUint8(0, 0x02);                     // tagKbd
            view.setUint16(1, doomKey, true);            // little-endian Doom keycode
            view.setUint8(3, pressed ? 1 : 0);          // pressed flag
            ws.send(buf);
        }
    }

    // ── Rendering ───────────────────────────────────────────────────────────

    function renderLoop() {
        renderFrame();
        updateOverlay();
        // D-013: Render CPU trace overlay on top of frame (only if visible, no-op otherwise)
        if (cpuTrace) {
            cpuTrace.render();
        }
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
        getScreenBuffer: function () { return new Uint8Array(screenBuffer); },
        // D-013: CPU trace overlay control
        toggleCpuTrace: function () {
            if (cpuTrace) cpuTrace.toggle();
        },
        getCpuTraceVisible: function () {
            return cpuTrace ? cpuTrace.visible : false;
        }
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
