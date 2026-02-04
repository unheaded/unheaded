/**
 * packet-flow.js - Real-time Packet Flow Visualization
 * Displays network traffic flowing between Unheaded services
 *
 * Uses Canvas API for rendering (no external dependencies)
 * WebSocket connection to dashboard-backend for real-time updates
 */

const PacketFlowViz = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    const CONFIG = {
        // WebSocket settings
        wsUrl: 'ws://localhost:8080/ws/packets',
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 30000,
        reconnectMultiplier: 1.5,
        maxReconnectAttempts: 0, // 0 = infinite

        // Visualization settings
        nodeRadius: 28,
        nodeLabelOffset: 45,
        packetSize: 6,
        packetSpeed: 0.015,
        trailLength: 15,
        maxPackets: 100,
        animationFPS: 60,

        // Colors
        colors: {
            background: '#0a0a0a',
            connectionLine: 'rgba(255, 215, 0, 0.15)',
            connectionLineActive: 'rgba(255, 215, 0, 0.4)',
            nodeStroke: 'rgba(255, 255, 255, 0.3)',
            labelColor: '#e0e0e0',
            traceIdColor: '#ffd700',
            latencyColor: '#4ecdc4',
            errorPacket: '#ff4757',
            successPacket: '#00d26a',
            warningPacket: '#ff9800'
        },

        // Service node definitions
        services: {
            gateway: { name: 'Gateway', color: '#ffd700', x: 0.5, y: 0.08 },
            busboy: { name: 'Busboy', color: '#4ecdc4', x: 0.5, y: 0.35 },
            timeguru: { name: 'Timeguru', color: '#45b7d1', x: 0.15, y: 0.6 },
            captain: { name: 'Captain', color: '#a29bfe', x: 0.35, y: 0.6 },
            architect: { name: 'Architect', color: '#fd79a8', x: 0.65, y: 0.6 },
            micromanager: { name: 'Micromanager', color: '#6c5ce7', x: 0.85, y: 0.6 },
            monad: { name: 'Monad', color: '#00cec9', x: 0.25, y: 0.85 },
            sophia: { name: 'Sophia', color: '#e17055', x: 0.75, y: 0.85 }
        },

        // Connection topology (which services connect to which)
        connections: [
            ['gateway', 'busboy'],
            ['busboy', 'timeguru'],
            ['busboy', 'captain'],
            ['busboy', 'architect'],
            ['busboy', 'micromanager'],
            ['busboy', 'monad'],
            ['busboy', 'sophia'],
            ['timeguru', 'monad'],
            ['captain', 'architect'],
            ['architect', 'sophia']
        ]
    };

    // ========================================================================
    // State
    // ========================================================================
    const state = {
        // WebSocket
        ws: null,
        wsConnected: false,
        reconnectAttempts: 0,
        reconnectTimeout: null,

        // Canvas
        canvas: null,
        ctx: null,
        width: 0,
        height: 0,
        dpr: 1,

        // Animation
        animationFrame: null,
        lastFrameTime: 0,

        // Data
        packets: [],
        activeConnections: new Map(),
        recentFlows: [],
        stats: {
            totalPackets: 0,
            avgLatency: 0,
            errorRate: 0
        },

        // Status callbacks
        statusCallbacks: new Set()
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(containerId, options) {
        const container = document.getElementById(containerId || 'flow-viz');
        if (!container) {
            console.error('[PacketFlow] Container element not found:', containerId);
            return false;
        }

        // Apply options
        if (options) {
            if (options.wsUrl) CONFIG.wsUrl = options.wsUrl;
            if (options.services) Object.assign(CONFIG.services, options.services);
        }

        // Create canvas
        state.canvas = document.createElement('canvas');
        state.canvas.id = 'packet-flow-canvas';
        state.canvas.style.width = '100%';
        state.canvas.style.height = '100%';
        state.canvas.style.display = 'block';
        container.appendChild(state.canvas);

        state.ctx = state.canvas.getContext('2d');
        state.dpr = window.devicePixelRatio || 1;

        // Create status indicator
        createStatusIndicator(container);

        // Create stats panel
        createStatsPanel(container);

        // Handle resize
        resizeCanvas();
        window.addEventListener('resize', debounce(resizeCanvas, 100));

        // Start animation loop
        startAnimation();

        // Connect WebSocket
        connectWebSocket();

        console.log('[PacketFlow] Initialized');
        return true;
    }

    function createStatusIndicator(container) {
        const indicator = document.createElement('div');
        indicator.id = 'packet-flow-status';
        indicator.className = 'packet-flow-status disconnected';
        indicator.innerHTML = '<span class="status-dot"></span><span class="status-text">Disconnected</span>';
        indicator.style.cssText = `
            position: absolute;
            top: 10px;
            right: 10px;
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 6px 12px;
            background: rgba(0, 0, 0, 0.7);
            border-radius: 4px;
            font-size: 12px;
            color: #e0e0e0;
            font-family: 'Courier New', monospace;
            z-index: 10;
        `;
        container.style.position = 'relative';
        container.appendChild(indicator);

        // Add status dot styles
        const style = document.createElement('style');
        style.textContent = `
            .packet-flow-status .status-dot {
                width: 8px;
                height: 8px;
                border-radius: 50%;
                background: #ff4757;
                transition: background 0.3s;
            }
            .packet-flow-status.connected .status-dot {
                background: #00d26a;
                box-shadow: 0 0 8px #00d26a;
            }
            .packet-flow-status.connecting .status-dot {
                background: #ffd700;
                animation: pulse 1s infinite;
            }
            @keyframes pulse {
                0%, 100% { opacity: 1; }
                50% { opacity: 0.5; }
            }
        `;
        document.head.appendChild(style);
    }

    function createStatsPanel(container) {
        const panel = document.createElement('div');
        panel.id = 'packet-flow-stats';
        panel.innerHTML = `
            <div class="stat-item">
                <span class="stat-label">Packets</span>
                <span class="stat-value" id="pf-stat-packets">0</span>
            </div>
            <div class="stat-item">
                <span class="stat-label">Avg Latency</span>
                <span class="stat-value" id="pf-stat-latency">0ms</span>
            </div>
            <div class="stat-item">
                <span class="stat-label">Error Rate</span>
                <span class="stat-value" id="pf-stat-errors">0%</span>
            </div>
        `;
        panel.style.cssText = `
            position: absolute;
            bottom: 10px;
            left: 10px;
            display: flex;
            gap: 20px;
            padding: 10px 15px;
            background: rgba(0, 0, 0, 0.7);
            border-radius: 4px;
            font-family: 'Courier New', monospace;
            z-index: 10;
        `;
        container.appendChild(panel);

        // Add stats styles
        const style = document.createElement('style');
        style.textContent = `
            #packet-flow-stats .stat-item {
                display: flex;
                flex-direction: column;
                align-items: center;
            }
            #packet-flow-stats .stat-label {
                font-size: 10px;
                color: #888;
                text-transform: uppercase;
            }
            #packet-flow-stats .stat-value {
                font-size: 16px;
                color: #ffd700;
                font-weight: bold;
            }
        `;
        document.head.appendChild(style);
    }

    function resizeCanvas() {
        const rect = state.canvas.parentElement.getBoundingClientRect();
        state.width = rect.width;
        state.height = rect.height;

        state.canvas.width = state.width * state.dpr;
        state.canvas.height = state.height * state.dpr;
        state.ctx.scale(state.dpr, state.dpr);
    }

    // ========================================================================
    // WebSocket Connection
    // ========================================================================
    function connectWebSocket() {
        if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
            return;
        }

        updateConnectionStatus('connecting');
        console.log('[PacketFlow] Connecting to:', CONFIG.wsUrl);

        try {
            state.ws = new WebSocket(CONFIG.wsUrl);

            state.ws.onopen = handleWSOpen;
            state.ws.onclose = handleWSClose;
            state.ws.onerror = handleWSError;
            state.ws.onmessage = handleWSMessage;
        } catch (error) {
            console.error('[PacketFlow] WebSocket connection error:', error);
            scheduleReconnect();
        }
    }

    function handleWSOpen() {
        console.log('[PacketFlow] Connected');
        state.wsConnected = true;
        state.reconnectAttempts = 0;
        updateConnectionStatus('connected');

        // Subscribe to packet flow updates
        send({ type: 'subscribe', channel: 'packet_flow' });
    }

    function handleWSClose(event) {
        console.log('[PacketFlow] Disconnected:', event.code, event.reason);
        state.wsConnected = false;
        state.ws = null;
        updateConnectionStatus('disconnected');

        if (event.code !== 1000) {
            scheduleReconnect();
        }
    }

    function handleWSError(error) {
        console.error('[PacketFlow] WebSocket error:', error);
        state.wsConnected = false;
        updateConnectionStatus('disconnected');
    }

    function handleWSMessage(event) {
        try {
            const message = JSON.parse(event.data);
            processMessage(message);
        } catch (error) {
            console.error('[PacketFlow] Message parse error:', error);
        }
    }

    function processMessage(message) {
        switch (message.type) {
            case 'packet_flow':
                handlePacketFlow(message.data);
                break;
            case 'pong':
                // Heartbeat response
                break;
            case 'error':
                console.error('[PacketFlow] Server error:', message.data);
                break;
            default:
                // Handle other message types
                if (message.trace_id) {
                    // Direct packet flow data
                    handlePacketFlow(message);
                }
        }
    }

    function handlePacketFlow(flow) {
        if (!flow) return;

        // Store recent flow
        state.recentFlows.push(flow);
        if (state.recentFlows.length > 100) {
            state.recentFlows.shift();
        }

        // Update stats
        updateStats(flow);

        // Create animated packet from flow
        createPacketFromFlow(flow);
    }

    function createPacketFromFlow(flow) {
        const hops = flow.hops || [];
        if (hops.length < 2) return;

        // Map hop components to service names
        const route = hops.map(hop => mapComponentToService(hop.component)).filter(Boolean);
        if (route.length < 2) return;

        // Create packet object
        const packet = {
            id: flow.trace_id || generateId(),
            traceId: flow.trace_id || 'unknown',
            route: route,
            currentSegment: 0,
            progress: 0,
            speed: CONFIG.packetSpeed * (0.8 + Math.random() * 0.4),
            color: getPacketColor(flow.status_code),
            size: CONFIG.packetSize,
            trail: [],
            latency: flow.total_time ? (flow.total_time / 1e6).toFixed(2) : 0,
            method: flow.method || '',
            path: flow.path || '',
            statusCode: flow.status_code || 200,
            timestamp: Date.now()
        };

        state.packets.push(packet);

        // Limit packets
        if (state.packets.length > CONFIG.maxPackets) {
            state.packets.shift();
        }

        // Mark connection as active
        for (let i = 0; i < route.length - 1; i++) {
            const key = `${route[i]}-${route[i + 1]}`;
            state.activeConnections.set(key, Date.now());
        }
    }

    function mapComponentToService(component) {
        const mapping = {
            'xdp_packet_marker': 'gateway',
            'gateway': 'gateway',
            'busboy': 'busboy',
            'service': 'timeguru', // Default service
            'timeguru': 'timeguru',
            'captain': 'captain',
            'architect': 'architect',
            'micromanager': 'micromanager',
            'monad': 'monad',
            'sophia': 'sophia',
            'trace-collector': 'busboy'
        };
        return mapping[component] || component;
    }

    function getPacketColor(statusCode) {
        if (!statusCode || statusCode < 300) return CONFIG.colors.successPacket;
        if (statusCode < 400) return CONFIG.colors.warningPacket;
        return CONFIG.colors.errorPacket;
    }

    function updateStats(flow) {
        state.stats.totalPackets++;

        // Calculate average latency
        const latencies = state.recentFlows
            .filter(f => f.total_time)
            .map(f => f.total_time / 1e6);
        if (latencies.length > 0) {
            state.stats.avgLatency = latencies.reduce((a, b) => a + b, 0) / latencies.length;
        }

        // Calculate error rate
        const errors = state.recentFlows.filter(f => f.status_code >= 400).length;
        state.stats.errorRate = (errors / state.recentFlows.length) * 100;

        // Update UI
        updateStatsUI();
    }

    function updateStatsUI() {
        const packetsEl = document.getElementById('pf-stat-packets');
        const latencyEl = document.getElementById('pf-stat-latency');
        const errorsEl = document.getElementById('pf-stat-errors');

        if (packetsEl) packetsEl.textContent = state.stats.totalPackets;
        if (latencyEl) latencyEl.textContent = state.stats.avgLatency.toFixed(2) + 'ms';
        if (errorsEl) errorsEl.textContent = state.stats.errorRate.toFixed(1) + '%';
    }

    function scheduleReconnect() {
        if (state.reconnectTimeout) {
            clearTimeout(state.reconnectTimeout);
        }

        if (CONFIG.maxReconnectAttempts > 0 && state.reconnectAttempts >= CONFIG.maxReconnectAttempts) {
            console.error('[PacketFlow] Max reconnect attempts reached');
            return;
        }

        const delay = Math.min(
            CONFIG.reconnectBaseDelay * Math.pow(CONFIG.reconnectMultiplier, state.reconnectAttempts),
            CONFIG.reconnectMaxDelay
        );

        state.reconnectAttempts++;
        console.log('[PacketFlow] Reconnecting in', delay, 'ms (attempt', state.reconnectAttempts + ')');

        state.reconnectTimeout = setTimeout(connectWebSocket, delay);
    }

    function send(message) {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            try {
                state.ws.send(JSON.stringify(message));
                return true;
            } catch (error) {
                console.error('[PacketFlow] Send error:', error);
            }
        }
        return false;
    }

    function updateConnectionStatus(status) {
        const indicator = document.getElementById('packet-flow-status');
        if (!indicator) return;

        indicator.className = 'packet-flow-status ' + status;
        const textEl = indicator.querySelector('.status-text');
        if (textEl) {
            textEl.textContent = status.charAt(0).toUpperCase() + status.slice(1);
        }

        // Notify callbacks
        state.statusCallbacks.forEach(callback => {
            try {
                callback(status);
            } catch (e) {
                console.error('[PacketFlow] Status callback error:', e);
            }
        });
    }

    // ========================================================================
    // Animation & Rendering
    // ========================================================================
    function startAnimation() {
        function animate(timestamp) {
            const deltaTime = timestamp - state.lastFrameTime;

            if (deltaTime >= 1000 / CONFIG.animationFPS) {
                state.lastFrameTime = timestamp;
                update(deltaTime);
                render();
            }

            state.animationFrame = requestAnimationFrame(animate);
        }

        state.animationFrame = requestAnimationFrame(animate);
    }

    function stopAnimation() {
        if (state.animationFrame) {
            cancelAnimationFrame(state.animationFrame);
            state.animationFrame = null;
        }
    }

    function update(deltaTime) {
        const now = Date.now();

        // Update packets
        state.packets = state.packets.filter(packet => {
            packet.progress += packet.speed;

            // Store trail position
            const pos = getPacketPosition(packet);
            if (pos) {
                packet.trail.push({ x: pos.x, y: pos.y, time: now });
                if (packet.trail.length > CONFIG.trailLength) {
                    packet.trail.shift();
                }
            }

            // Check if packet reached next segment
            if (packet.progress >= 1) {
                packet.currentSegment++;
                packet.progress = 0;

                // Remove packet if journey complete
                if (packet.currentSegment >= packet.route.length - 1) {
                    return false;
                }
            }

            return true;
        });

        // Clean up old active connections
        state.activeConnections.forEach((time, key) => {
            if (now - time > 2000) {
                state.activeConnections.delete(key);
            }
        });

        // Simulate packet flow when not connected (demo mode)
        if (!state.wsConnected && Math.random() < 0.02) {
            simulatePacket();
        }
    }

    function simulatePacket() {
        const services = Object.keys(CONFIG.services);
        const startService = 'gateway';
        const endService = services[Math.floor(Math.random() * services.length)];

        if (startService === endService) return;

        // Create simulated flow
        const simulatedFlow = {
            trace_id: 'sim-' + generateId(),
            status_code: Math.random() > 0.1 ? 200 : 500,
            total_time: (5 + Math.random() * 20) * 1e6,
            hops: [
                { component: 'gateway' },
                { component: 'busboy' },
                { component: endService }
            ]
        };

        createPacketFromFlow(simulatedFlow);
    }

    function render() {
        const ctx = state.ctx;
        const width = state.width;
        const height = state.height;

        // Clear canvas
        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, width, height);

        // Draw grid pattern
        drawGrid(ctx, width, height);

        // Draw connections
        drawConnections(ctx, width, height);

        // Draw packets
        drawPackets(ctx, width, height);

        // Draw service nodes
        drawNodes(ctx, width, height);

        // Draw active packet info
        drawPacketInfo(ctx, width, height);
    }

    function drawGrid(ctx, width, height) {
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.03)';
        ctx.lineWidth = 1;

        const gridSize = 40;

        for (let x = 0; x < width; x += gridSize) {
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, height);
            ctx.stroke();
        }

        for (let y = 0; y < height; y += gridSize) {
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(width, y);
            ctx.stroke();
        }
    }

    function drawConnections(ctx, width, height) {
        const now = Date.now();

        CONFIG.connections.forEach(([from, to]) => {
            const fromService = CONFIG.services[from];
            const toService = CONFIG.services[to];

            if (!fromService || !toService) return;

            const x1 = fromService.x * width;
            const y1 = fromService.y * height;
            const x2 = toService.x * width;
            const y2 = toService.y * height;

            // Check if connection is active
            const key = `${from}-${to}`;
            const reverseKey = `${to}-${from}`;
            const isActive = state.activeConnections.has(key) || state.activeConnections.has(reverseKey);

            // Draw connection line
            ctx.beginPath();
            ctx.moveTo(x1, y1);

            // Use bezier curve for smoother connections
            const midX = (x1 + x2) / 2;
            const midY = (y1 + y2) / 2;
            const controlOffset = Math.abs(y2 - y1) * 0.3;

            ctx.quadraticCurveTo(midX, midY - controlOffset, x2, y2);

            ctx.strokeStyle = isActive ? CONFIG.colors.connectionLineActive : CONFIG.colors.connectionLine;
            ctx.lineWidth = isActive ? 2 : 1;
            ctx.stroke();

            // Draw animated dashes for active connections
            if (isActive) {
                drawAnimatedConnection(ctx, x1, y1, x2, y2, now);
            }
        });
    }

    function drawAnimatedConnection(ctx, x1, y1, x2, y2, now) {
        const dashLength = 10;
        const gapLength = 10;
        const animSpeed = 0.05;
        const offset = (now * animSpeed) % (dashLength + gapLength);

        ctx.setLineDash([dashLength, gapLength]);
        ctx.lineDashOffset = -offset;
        ctx.strokeStyle = 'rgba(255, 215, 0, 0.3)';
        ctx.lineWidth = 1;

        ctx.beginPath();
        ctx.moveTo(x1, y1);
        ctx.lineTo(x2, y2);
        ctx.stroke();

        ctx.setLineDash([]);
    }

    function drawNodes(ctx, width, height) {
        Object.entries(CONFIG.services).forEach(([id, service]) => {
            const x = service.x * width;
            const y = service.y * height;
            const radius = CONFIG.nodeRadius;

            // Draw glow
            const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius * 2);
            gradient.addColorStop(0, service.color + '40');
            gradient.addColorStop(1, 'transparent');
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(x, y, radius * 2, 0, Math.PI * 2);
            ctx.fill();

            // Draw node background
            ctx.fillStyle = 'rgba(10, 10, 10, 0.8)';
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw node border
            ctx.strokeStyle = service.color;
            ctx.lineWidth = 2;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();

            // Draw inner glow
            const innerGradient = ctx.createRadialGradient(x, y, radius * 0.5, x, y, radius);
            innerGradient.addColorStop(0, service.color + '30');
            innerGradient.addColorStop(1, 'transparent');
            ctx.fillStyle = innerGradient;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw label
            ctx.fillStyle = CONFIG.colors.labelColor;
            ctx.font = '12px "Courier New", monospace';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'top';
            ctx.fillText(service.name, x, y + CONFIG.nodeLabelOffset);
        });
    }

    function drawPackets(ctx, width, height) {
        state.packets.forEach(packet => {
            const pos = getPacketPosition(packet);
            if (!pos) return;

            // Draw trail
            if (packet.trail.length > 1) {
                ctx.beginPath();
                ctx.moveTo(packet.trail[0].x, packet.trail[0].y);

                for (let i = 1; i < packet.trail.length; i++) {
                    ctx.lineTo(packet.trail[i].x, packet.trail[i].y);
                }

                const gradient = ctx.createLinearGradient(
                    packet.trail[0].x, packet.trail[0].y,
                    pos.x, pos.y
                );
                gradient.addColorStop(0, 'transparent');
                gradient.addColorStop(1, packet.color + '80');

                ctx.strokeStyle = gradient;
                ctx.lineWidth = packet.size / 2;
                ctx.lineCap = 'round';
                ctx.stroke();
            }

            // Draw packet
            ctx.fillStyle = packet.color;
            ctx.shadowColor = packet.color;
            ctx.shadowBlur = 10;
            ctx.beginPath();
            ctx.arc(pos.x, pos.y, packet.size, 0, Math.PI * 2);
            ctx.fill();
            ctx.shadowBlur = 0;

            // Draw trace ID label for larger packets
            if (packet.size >= 6 && packet.traceId) {
                ctx.fillStyle = CONFIG.colors.traceIdColor;
                ctx.font = '9px "Courier New", monospace';
                ctx.textAlign = 'center';
                ctx.fillText(packet.traceId.slice(0, 12), pos.x, pos.y - packet.size - 5);
            }
        });
    }

    function drawPacketInfo(ctx, width, height) {
        // Draw info for the most recent active packet
        const activePacket = state.packets[state.packets.length - 1];
        if (!activePacket) return;

        const infoX = 10;
        const infoY = 10;

        ctx.fillStyle = 'rgba(0, 0, 0, 0.7)';
        ctx.fillRect(infoX, infoY, 200, 60);

        ctx.fillStyle = CONFIG.colors.labelColor;
        ctx.font = '10px "Courier New", monospace';
        ctx.textAlign = 'left';
        ctx.textBaseline = 'top';

        ctx.fillText('Trace: ' + activePacket.traceId.slice(0, 20), infoX + 5, infoY + 5);
        ctx.fillStyle = CONFIG.colors.latencyColor;
        ctx.fillText('Latency: ' + activePacket.latency + 'ms', infoX + 5, infoY + 20);
        ctx.fillStyle = activePacket.statusCode < 400 ? CONFIG.colors.successPacket : CONFIG.colors.errorPacket;
        ctx.fillText('Status: ' + activePacket.statusCode, infoX + 5, infoY + 35);
        ctx.fillStyle = CONFIG.colors.labelColor;
        ctx.fillText(activePacket.method + ' ' + activePacket.path, infoX + 5, infoY + 50);
    }

    function getPacketPosition(packet) {
        if (packet.currentSegment >= packet.route.length - 1) return null;

        const fromService = CONFIG.services[packet.route[packet.currentSegment]];
        const toService = CONFIG.services[packet.route[packet.currentSegment + 1]];

        if (!fromService || !toService) return null;

        const x1 = fromService.x * state.width;
        const y1 = fromService.y * state.height;
        const x2 = toService.x * state.width;
        const y2 = toService.y * state.height;

        // Use quadratic bezier for curved path
        const t = packet.progress;
        const midX = (x1 + x2) / 2;
        const midY = (y1 + y2) / 2;
        const controlY = midY - Math.abs(y2 - y1) * 0.3;

        // Quadratic bezier formula
        const x = Math.pow(1 - t, 2) * x1 + 2 * (1 - t) * t * midX + Math.pow(t, 2) * x2;
        const y = Math.pow(1 - t, 2) * y1 + 2 * (1 - t) * t * controlY + Math.pow(t, 2) * y2;

        return { x, y };
    }

    // ========================================================================
    // Utility Functions
    // ========================================================================
    function generateId() {
        return Math.random().toString(36).substring(2, 10);
    }

    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // ========================================================================
    // Public API
    // ========================================================================
    function disconnect() {
        if (state.reconnectTimeout) {
            clearTimeout(state.reconnectTimeout);
        }
        if (state.ws) {
            state.ws.close(1000, 'Manual disconnect');
            state.ws = null;
        }
        state.wsConnected = false;
        updateConnectionStatus('disconnected');
    }

    function reconnect() {
        disconnect();
        state.reconnectAttempts = 0;
        connectWebSocket();
    }

    function setWsUrl(url) {
        CONFIG.wsUrl = url;
        reconnect();
    }

    function onStatusChange(callback) {
        state.statusCallbacks.add(callback);
        return () => state.statusCallbacks.delete(callback);
    }

    function getStats() {
        return { ...state.stats };
    }

    function isConnected() {
        return state.wsConnected;
    }

    function destroy() {
        stopAnimation();
        disconnect();

        if (state.canvas && state.canvas.parentElement) {
            state.canvas.parentElement.removeChild(state.canvas);
        }

        const status = document.getElementById('packet-flow-status');
        if (status) status.remove();

        const stats = document.getElementById('packet-flow-stats');
        if (stats) stats.remove();
    }

    // Add demo packet for testing
    function addDemoPacket(options) {
        const defaultOptions = {
            trace_id: 'demo-' + generateId(),
            status_code: 200,
            total_time: 15 * 1e6,
            method: 'GET',
            path: '/api/v1/demo',
            hops: [
                { component: 'gateway' },
                { component: 'busboy' },
                { component: 'timeguru' }
            ]
        };

        createPacketFromFlow({ ...defaultOptions, ...options });
    }

    return {
        init,
        disconnect,
        reconnect,
        setWsUrl,
        onStatusChange,
        getStats,
        isConnected,
        destroy,
        addDemoPacket,
        CONFIG
    };
})();

// Auto-initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    // Check if container exists
    const container = document.getElementById('flow-viz');
    if (container) {
        // Determine WebSocket URL
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = window.PACKET_FLOW_WS_URL || `${protocol}//${window.location.host}/ws/packets`;

        PacketFlowViz.init('flow-viz', { wsUrl: wsUrl });
    }
});

// Make available globally
window.PacketFlowViz = PacketFlowViz;

console.log('packet-flow.js loaded');
