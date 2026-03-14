/**
 * packet-flow.js - Real-time Packet Flow Visualization
 * Displays network traffic flowing between Unheaded services
 *
 * Uses Canvas API for rendering (no external dependencies)
 * WebSocket connections to:
 *   - dashboard-backend (Go, port 8080) for packet flow broadcasts
 *   - trace-collector (Rust/eBPF, port 9091) for real eBPF trace events
 * Falls back to demo simulation when no live data is available.
 */

const PacketFlowViz = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    const CONFIG = {
        // WebSocket settings - relative to current host (auto-detects port)
        wsUrl: (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws',
        // Trace-collector WebSocket — falls back to same host /ws if no dedicated collector
        traceCollectorWsUrl: (location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws/traces',
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 30000,
        reconnectMultiplier: 1.5,
        maxReconnectAttempts: 0, // 0 = infinite
        // Enable demo fallback when both WebSocket sources are unavailable
        demoFallback: true,

        // Visualization settings
        nodeRadius: 28,
        nodeLabelOffset: 45,
        packetSize: 6,
        packetSpeed: 0.015,
        trailLength: 15,
        maxPackets: 100,
        animationFPS: 60,

        // Colors (Kingdom theme)
        colors: {
            background: '#0f0f1a',
            connectionLine: 'rgba(255, 215, 0, 0.12)',
            connectionLineActive: 'rgba(255, 215, 0, 0.45)',
            nodeStroke: 'rgba(255, 215, 0, 0.3)',
            nodeFill: '#16213e',
            nodeGlow: 'rgba(255, 215, 0, 0.2)',
            labelColor: '#e0e0e0',
            traceIdColor: '#ffd700',
            latencyColor: '#4ecdc4',
            errorPacket: '#ff4757',
            successPacket: '#00d26a',
            warningPacket: '#ff9800',
            gridColor: 'rgba(255, 215, 0, 0.03)'
        },

        // Service node definitions
        services: {
            gateway: { name: 'Gateway', color: '#ffd700', x: 0.5, y: 0.08 },
            wotan: { name: 'Wotan', color: '#4ecdc4', x: 0.5, y: 0.35 },
            timeguru: { name: 'Timeguru', color: '#45b7d1', x: 0.15, y: 0.6 },
            captain: { name: 'Captain', color: '#a29bfe', x: 0.35, y: 0.6 },
            architect: { name: 'Architect', color: '#fd79a8', x: 0.65, y: 0.6 },
            micromanager: { name: 'Micromanager', color: '#6c5ce7', x: 0.85, y: 0.6 },
            monad: { name: 'Monad', color: '#00cec9', x: 0.25, y: 0.85 },
            sophia: { name: 'Sophia', color: '#e17055', x: 0.75, y: 0.85 }
        },

        // Connection topology (which services connect to which)
        connections: [
            ['gateway', 'wotan'],
            ['wotan', 'timeguru'],
            ['wotan', 'captain'],
            ['wotan', 'architect'],
            ['wotan', 'micromanager'],
            ['wotan', 'monad'],
            ['wotan', 'sophia'],
            ['timeguru', 'monad'],
            ['captain', 'architect'],
            ['architect', 'sophia']
        ]
    };

    // ========================================================================
    // State
    // ========================================================================
    const state = {
        // Dashboard-backend WebSocket (packet_flow broadcasts)
        ws: null,
        wsConnected: false,
        reconnectAttempts: 0,
        reconnectTimeout: null,

        // Trace-collector WebSocket (eBPF trace events)
        traceWs: null,
        traceWsConnected: false,
        traceReconnectAttempts: 0,
        traceReconnectTimeout: null,
        traceSubscriptionId: 'dashboard-all',

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
        var container = document.getElementById(containerId || 'flow-viz');
        if (!container) {
            console.error('[PacketFlow] Container element not found:', containerId);
            return false;
        }

        // Apply options
        if (options) {
            if (options.wsUrl) CONFIG.wsUrl = options.wsUrl;
            if (options.traceCollectorWsUrl) CONFIG.traceCollectorWsUrl = options.traceCollectorWsUrl;
            if (options.services) Object.assign(CONFIG.services, options.services);
            if (typeof options.demoFallback !== 'undefined') CONFIG.demoFallback = options.demoFallback;
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

        // Connect to both WebSocket sources
        connectWebSocket();
        connectTraceCollector();

        console.log('[PacketFlow] Initialized with real data sources');
        return true;
    }

    function createStatusIndicator(container) {
        var indicator = document.createElement('div');
        indicator.id = 'packet-flow-status';
        indicator.className = 'packet-flow-status disconnected';
        indicator.setAttribute('role', 'status');
        indicator.setAttribute('aria-live', 'polite');
        indicator.setAttribute('aria-label', 'WebSocket status: disconnected');
        indicator.innerHTML = '<span class="status-dot" aria-hidden="true"></span><span class="status-text">Disconnected</span>';
        indicator.style.cssText = '\
            position: absolute;\
            top: 12px;\
            right: 12px;\
            display: flex;\
            align-items: center;\
            gap: 8px;\
            padding: 8px 14px;\
            background: rgba(22, 33, 62, 0.92);\
            border: 1px solid rgba(255, 215, 0, 0.2);\
            border-radius: 6px;\
            font-size: 11px;\
            color: #e0e0e0;\
            font-family: "SF Mono", "Cascadia Code", "Fira Code", "Courier New", monospace;\
            z-index: 10;\
            backdrop-filter: blur(8px);\
            transition: all 0.25s ease;\
        ';
        container.style.position = 'relative';
        container.appendChild(indicator);

        // Add status dot styles
        var style = document.createElement('style');
        style.textContent = '\
            .packet-flow-status .status-dot {\
                width: 8px;\
                height: 8px;\
                border-radius: 50%;\
                background: #ff4757;\
                transition: all 0.3s ease;\
            }\
            .packet-flow-status.connected .status-dot {\
                background: #00d26a;\
                box-shadow: 0 0 10px #00d26a;\
            }\
            .packet-flow-status.connecting .status-dot {\
                background: #ffd700;\
                animation: pf-pulse 1s infinite;\
            }\
            .packet-flow-status.disconnected .status-dot {\
                background: #ff4757;\
                box-shadow: 0 0 8px rgba(255, 71, 87, 0.5);\
            }\
            .packet-flow-status.demo .status-dot {\
                background: #ff9800;\
                box-shadow: 0 0 8px rgba(255, 152, 0, 0.5);\
            }\
            .packet-flow-status:hover {\
                border-color: rgba(255, 215, 0, 0.4);\
                box-shadow: 0 0 15px rgba(255, 215, 0, 0.15);\
            }\
            @keyframes pf-pulse {\
                0%, 100% { opacity: 1; transform: scale(1); }\
                50% { opacity: 0.5; transform: scale(1.1); }\
            }\
        ';
        document.head.appendChild(style);
    }

    function createStatsPanel(container) {
        var panel = document.createElement('div');
        panel.id = 'packet-flow-stats';
        panel.setAttribute('role', 'region');
        panel.setAttribute('aria-label', 'Packet flow statistics');
        panel.innerHTML = '\
            <div class="stat-item" role="group" aria-label="Total packets">\
                <span class="stat-label">Packets</span>\
                <span class="stat-value" id="pf-stat-packets" aria-live="polite">0</span>\
            </div>\
            <div class="stat-item" role="group" aria-label="Average latency">\
                <span class="stat-label">Avg Latency</span>\
                <span class="stat-value" id="pf-stat-latency" aria-live="polite">0ms</span>\
            </div>\
            <div class="stat-item" role="group" aria-label="Error rate percentage">\
                <span class="stat-label">Error Rate</span>\
                <span class="stat-value" id="pf-stat-errors" aria-live="polite">0%</span>\
            </div>\
        ';
        panel.style.cssText = '\
            position: absolute;\
            bottom: 12px;\
            left: 12px;\
            display: flex;\
            gap: 24px;\
            padding: 12px 18px;\
            background: rgba(22, 33, 62, 0.92);\
            border: 1px solid rgba(255, 215, 0, 0.2);\
            border-radius: 6px;\
            font-family: "SF Mono", "Cascadia Code", "Fira Code", "Courier New", monospace;\
            z-index: 10;\
            backdrop-filter: blur(8px);\
        ';
        container.appendChild(panel);

        // Add stats styles
        var style = document.createElement('style');
        style.textContent = '\
            #packet-flow-stats .stat-item {\
                display: flex;\
                flex-direction: column;\
                align-items: center;\
                gap: 4px;\
            }\
            #packet-flow-stats .stat-label {\
                font-size: 9px;\
                color: #6b6b7b;\
                text-transform: uppercase;\
                letter-spacing: 0.05em;\
            }\
            #packet-flow-stats .stat-value {\
                font-size: 18px;\
                color: #ffd700;\
                font-weight: 600;\
                text-shadow: 0 0 10px rgba(255, 215, 0, 0.3);\
                transition: all 0.2s ease;\
            }\
            #packet-flow-stats .stat-value:hover {\
                text-shadow: 0 0 15px rgba(255, 215, 0, 0.5);\
            }\
        ';
        document.head.appendChild(style);
    }

    function resizeCanvas() {
        var rect = state.canvas.parentElement.getBoundingClientRect();
        state.width = rect.width;
        state.height = rect.height;

        state.canvas.width = state.width * state.dpr;
        state.canvas.height = state.height * state.dpr;
        state.ctx.scale(state.dpr, state.dpr);
    }

    // ========================================================================
    // WebSocket Connection - Dashboard Backend (packet_flow broadcasts)
    // ========================================================================
    function connectWebSocket() {
        if (state.ws && (state.ws.readyState === WebSocket.OPEN || state.ws.readyState === WebSocket.CONNECTING)) {
            return;
        }

        updateConnectionStatus('connecting');
        console.log('[PacketFlow] Connecting to dashboard-backend:', CONFIG.wsUrl);

        try {
            state.ws = new WebSocket(CONFIG.wsUrl);

            state.ws.onopen = handleWSOpen;
            state.ws.onclose = handleWSClose;
            state.ws.onerror = handleWSError;
            state.ws.onmessage = handleWSMessage;
        } catch (error) {
            console.error('[PacketFlow] Dashboard-backend WebSocket connection error:', error);
            scheduleReconnect();
        }
    }

    function handleWSOpen() {
        console.log('[PacketFlow] Connected to dashboard-backend');
        state.wsConnected = true;
        state.reconnectAttempts = 0;
        updateConnectionStatus('connected');
    }

    function handleWSClose(event) {
        console.log('[PacketFlow] Dashboard-backend disconnected:', event.code, event.reason);
        state.wsConnected = false;
        state.ws = null;
        updateOverallStatus();

        if (event.code !== 1000) {
            scheduleReconnect();
        }
    }

    function handleWSError(error) {
        console.error('[PacketFlow] Dashboard-backend WebSocket error:', error);
        state.wsConnected = false;
        updateOverallStatus();
    }

    function handleWSMessage(event) {
        try {
            var message = JSON.parse(event.data);
            processMessage(message);
        } catch (error) {
            console.error('[PacketFlow] Dashboard-backend message parse error:', error);
        }
    }

    function processMessage(message) {
        switch (message.type) {
            case 'packet_flow':
                // Dashboard-backend broadcasts: { type: "packet_flow", data: PacketFlow }
                handlePacketFlow(message.data);
                break;

            // ── Anamnesis event types (real BPF data) ─────────────────
            case 'flow':
                // Completed Anamnesis flow: BIRTH → HOP* → DEATH
                handleAnamnesisFlow(message.data);
                break;
            case 'hop':
                // Real-time hop marker from BPF
                handleAnamnesisHop(message.data);
                break;
            case 'anomaly':
                // CRC failure or decode error — red flash
                handleAnamnesisAnomaly(message.data, '#ff0000');
                break;
            case 'chaos':
                // Yaldabaoth chaos injection — orange pulse
                handleAnamnesisAnomaly(message.data, '#ff5c00');
                break;

            case 'health_update':
                // Health updates from dashboard-backend - no visualization action
                break;
            case 'event':
                // Events from dashboard-backend - no visualization action
                break;
            case 'pong':
                // Heartbeat response
                break;
            case 'error':
                var friendlyMsg = formatServerError(message.data);
                console.error('[PacketFlow] Server error:', friendlyMsg);
                if (window.showToast) {
                    window.showToast(friendlyMsg, 'error');
                }
                break;
            default:
                // Handle other message types - check if it looks like raw flow data
                if (message.trace_id) {
                    handlePacketFlow(message);
                }
        }
    }

    // ========================================================================
    // Anamnesis Event Handlers (real BPF data from trace-collector-go)
    // ========================================================================

    function handleAnamnesisFlow(flow) {
        if (!flow) return;

        // Convert Anamnesis flow to PacketFlow-compatible format
        var hops = [];
        if (flow.birth) hops.push({ component: 'shield-ingress', hop_id: flow.birth.hop_id, timestamp_ns: flow.birth.timestamp_ns });
        if (flow.hops) {
            flow.hops.forEach(function(h) {
                hops.push({ component: 'hop-' + h.hop_id, hop_id: h.hop_id, timestamp_ns: h.timestamp_ns });
            });
        }
        if (flow.death) hops.push({ component: 'shield-egress', hop_id: flow.death.hop_id, timestamp_ns: flow.death.timestamp_ns });

        var syntheticFlow = {
            trace_id: 'flow-' + flow.flow_label,
            status_code: (flow.anomalies && flow.anomalies.length > 0) ? 500 : 200,
            total_time: flow.latency_ns || 0,
            method: 'BPF',
            path: 'flow/' + flow.flow_label,
            hops: hops.length >= 2 ? hops : [{ component: 'gateway' }, { component: 'wotan' }]
        };

        handlePacketFlow(syntheticFlow);

        // Notify monad decoder if available
        if (window.UnheadedMonad && flow.birth) {
            window.UnheadedMonad.onAnamnesisEvent({
                timestamp_ns: flow.birth.timestamp_ns,
                event_type: 'birth',
                hop_id: flow.birth.hop_id,
                flow_label_lo: flow.flow_label,
                monad: flow.birth.monad
            });
        }

        // Update BPF flow stats
        state.bpfFlowCount = (state.bpfFlowCount || 0) + 1;
        state.bpfLastLatencyNs = flow.latency_ns || 0;

        // Track hop latency for histogram
        if (flow.hop_latency) {
            if (!state.hopLatencies) state.hopLatencies = [];
            flow.hop_latency.forEach(function(hl) {
                state.hopLatencies.push(hl.latency_ns);
                if (state.hopLatencies.length > 1000) state.hopLatencies.shift();
            });
        }
    }

    function handleAnamnesisHop(ev) {
        if (!ev) return;
        // Flash the hop node on the canvas
        var hopKey = 'hop-' + ev.hop_id;
        state.activeConnections.set(hopKey, Date.now());

        // Notify monad decoder
        if (window.UnheadedMonad) {
            window.UnheadedMonad.onAnamnesisEvent(ev);
        }

        state.bpfEventCount = (state.bpfEventCount || 0) + 1;
    }

    function handleAnamnesisAnomaly(ev, flashColor) {
        if (!ev) return;
        // Create a brief flash effect for anomalies
        var flashPacket = {
            id: 'anomaly-' + generateId(),
            traceId: 'anomaly',
            route: ['gateway', 'wotan'],
            currentSegment: 0,
            progress: 0,
            speed: CONFIG.packetSpeed * 2,
            color: flashColor,
            size: CONFIG.packetSize * 1.5,
            trail: [],
            latency: 0,
            method: ev.event_type === 'chaos' ? 'CHAOS' : 'ANOMALY',
            path: 'anomaly/' + ev.hop_id,
            statusCode: 500,
            timestamp: Date.now()
        };
        state.packets.push(flashPacket);

        // Notify monad decoder
        if (window.UnheadedMonad) {
            window.UnheadedMonad.onAnamnesisEvent(ev);
        }

        state.bpfAnomalyCount = (state.bpfAnomalyCount || 0) + 1;
    }

    // ========================================================================
    // WebSocket Connection - Trace Collector (eBPF trace events, port 9091)
    // ========================================================================
    function connectTraceCollector() {
        if (state.traceWs && (state.traceWs.readyState === WebSocket.OPEN || state.traceWs.readyState === WebSocket.CONNECTING)) {
            return;
        }

        console.log('[PacketFlow] Connecting to trace-collector:', CONFIG.traceCollectorWsUrl);

        try {
            state.traceWs = new WebSocket(CONFIG.traceCollectorWsUrl);

            state.traceWs.onopen = handleTraceWSOpen;
            state.traceWs.onclose = handleTraceWSClose;
            state.traceWs.onerror = handleTraceWSError;
            state.traceWs.onmessage = handleTraceWSMessage;
        } catch (error) {
            console.error('[PacketFlow] Trace-collector WebSocket connection error:', error);
            scheduleTraceReconnect();
        }
    }

    function handleTraceWSOpen() {
        console.log('[PacketFlow] Connected to trace-collector');
        state.traceWsConnected = true;
        state.traceReconnectAttempts = 0;
        updateOverallStatus();

        // Subscribe to all trace events using the trace-collector protocol
        // ClientMessage::Subscribe { id, subscription }
        sendTrace({
            type: 'subscribe',
            id: state.traceSubscriptionId,
            subscription: {
                all: true,
                event_types: [],
                services: [],
                trace_ids: [],
                min_latency_ns: null,
                errors_only: false,
                sample_rate: 1.0
            }
        });

        // Send client info
        sendTrace({
            type: 'client_info',
            name: 'unheaded-dashboard',
            version: '1.0.0'
        });
    }

    function handleTraceWSClose(event) {
        console.log('[PacketFlow] Trace-collector disconnected:', event.code, event.reason);
        state.traceWsConnected = false;
        state.traceWs = null;
        updateOverallStatus();

        if (event.code !== 1000) {
            scheduleTraceReconnect();
        }
    }

    function handleTraceWSError(error) {
        console.error('[PacketFlow] Trace-collector WebSocket error:', error);
        state.traceWsConnected = false;
        updateOverallStatus();
    }

    function handleTraceWSMessage(event) {
        try {
            var message = JSON.parse(event.data);
            processTraceMessage(message);
        } catch (error) {
            console.error('[PacketFlow] Trace-collector message parse error:', error);
        }
    }

    function processTraceMessage(message) {
        // ServerMessage from trace-collector is tagged with "type" (snake_case)
        switch (message.type) {
            case 'connected':
                // ServerMessage::Connected { version, connection_id, capabilities }
                console.log('[PacketFlow] Trace-collector version:', message.version,
                    'connection:', message.connection_id,
                    'capabilities:', message.capabilities);
                break;

            case 'subscribed':
                // ServerMessage::Subscribed { id, success, error }
                if (message.success) {
                    console.log('[PacketFlow] Subscribed to traces:', message.id);
                } else {
                    console.error('[PacketFlow] Subscription failed:', message.id, message.error);
                }
                break;

            case 'trace_update':
                // ServerMessage::TraceUpdate { subscription_id, update: TraceUpdate }
                handleTraceUpdate(message.update);
                break;

            case 'trace_update_batch':
                // ServerMessage::TraceUpdateBatch { subscription_id, updates: [TraceUpdate] }
                if (message.updates && Array.isArray(message.updates)) {
                    for (var i = 0; i < message.updates.length; i++) {
                        handleTraceUpdate(message.updates[i]);
                    }
                }
                break;

            case 'stats':
                // ServerMessage::Stats { active_traces, events_per_second, ... }
                // Could use for display; not acting on it now.
                break;

            case 'pong':
                // ServerMessage::Pong { client_timestamp, server_timestamp }
                break;

            case 'error':
                // ServerMessage::Error { code, message, request_id }
                console.error('[PacketFlow] Trace-collector error:', message.code, message.message);
                break;

            default:
                break;
        }
    }

    /**
     * Convert a trace-collector TraceUpdate into a packet flow visualization.
     *
     * TraceUpdate fields:
     *   trace_id, services[], event_type, operation, duration_ns, latency_ns,
     *   status ("ok"|"error"|"unset"), timestamp_ns, source, attributes, is_complete
     */
    function handleTraceUpdate(update) {
        if (!update || !update.trace_id) return;

        // Build a synthetic PacketFlow-like object from the TraceUpdate
        var services = update.services || [];
        if (services.length < 2) {
            // If fewer than 2 services, build a plausible route through gateway
            if (services.length === 1) {
                services = ['gateway', 'wotan', services[0]];
            } else {
                services = ['gateway', 'wotan'];
            }
        }

        var statusCode = 200;
        if (update.status === 'error') {
            statusCode = 500;
        }

        var totalTimeNs = update.duration_ns || update.latency_ns || 0;

        var hops = [];
        for (var i = 0; i < services.length; i++) {
            hops.push({ component: services[i] });
        }

        var syntheticFlow = {
            trace_id: update.trace_id,
            status_code: statusCode,
            total_time: totalTimeNs,
            method: (update.attributes && update.attributes.http_method) || '',
            path: update.operation || '',
            hops: hops
        };

        handlePacketFlow(syntheticFlow);
    }

    function sendTrace(message) {
        if (state.traceWs && state.traceWs.readyState === WebSocket.OPEN) {
            try {
                state.traceWs.send(JSON.stringify(message));
                return true;
            } catch (error) {
                console.error('[PacketFlow] Trace send error:', error);
            }
        }
        return false;
    }

    function scheduleTraceReconnect() {
        if (state.traceReconnectTimeout) {
            clearTimeout(state.traceReconnectTimeout);
        }

        if (CONFIG.maxReconnectAttempts > 0 && state.traceReconnectAttempts >= CONFIG.maxReconnectAttempts) {
            console.error('[PacketFlow] Max trace-collector reconnect attempts reached');
            return;
        }

        var delay = Math.min(
            CONFIG.reconnectBaseDelay * Math.pow(CONFIG.reconnectMultiplier, state.traceReconnectAttempts),
            CONFIG.reconnectMaxDelay
        );

        state.traceReconnectAttempts++;
        console.log('[PacketFlow] Trace-collector reconnecting in', delay, 'ms (attempt', state.traceReconnectAttempts + ')');

        state.traceReconnectTimeout = setTimeout(connectTraceCollector, delay);
    }

    // ========================================================================
    // Status Management
    // ========================================================================

    /** Determine overall connection status from both WebSocket sources. */
    function updateOverallStatus() {
        if (state.wsConnected || state.traceWsConnected) {
            var label = 'Live';
            if (state.wsConnected && state.traceWsConnected) {
                label = 'Live (backend + eBPF)';
            } else if (state.wsConnected) {
                label = 'Live (backend)';
            } else {
                label = 'Live (eBPF)';
            }
            updateConnectionStatus('connected', label);
        } else if (CONFIG.demoFallback) {
            updateConnectionStatus('demo', 'Demo Mode');
        } else {
            updateConnectionStatus('disconnected');
        }
    }

    // Convert server error data to user-friendly message
    function formatServerError(data) {
        if (!data) return 'An unknown server error occurred.';
        if (typeof data === 'string') {
            if (data.includes('rate limit')) return 'Too many requests. Please wait a moment.';
            if (data.includes('unauthorized') || data.includes('forbidden')) return 'Authentication error. Please refresh the page.';
            if (data.includes('not found')) return 'The requested resource was not found.';
            return 'Server error: ' + data.substring(0, 100);
        }
        if (typeof data === 'object') {
            return data.message || data.error || 'An unexpected server error occurred.';
        }
        return 'An unexpected error occurred.';
    }

    // ========================================================================
    // Packet Flow Handling (shared between both data sources)
    // ========================================================================
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
        var hops = flow.hops || [];
        if (hops.length < 2) return;

        // Map hop components to service names
        var route = hops.map(function(hop) { return mapComponentToService(hop.component); }).filter(Boolean);
        if (route.length < 2) return;

        // Create packet object
        var packet = {
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
        for (var i = 0; i < route.length - 1; i++) {
            var key = route[i] + '-' + route[i + 1];
            state.activeConnections.set(key, Date.now());
        }
    }

    function mapComponentToService(component) {
        var mapping = {
            'xdp_packet_marker': 'gateway',
            'gateway': 'gateway',
            'wotan': 'wotan',
            'service': 'timeguru', // Default service
            'timeguru': 'timeguru',
            'captain': 'captain',
            'architect': 'architect',
            'micromanager': 'micromanager',
            'monad': 'monad',
            'sophia': 'sophia',
            'trace-collector': 'wotan'
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
        var latencies = state.recentFlows
            .filter(function(f) { return f.total_time; })
            .map(function(f) { return f.total_time / 1e6; });
        if (latencies.length > 0) {
            state.stats.avgLatency = latencies.reduce(function(a, b) { return a + b; }, 0) / latencies.length;
        }

        // Calculate error rate
        var errors = state.recentFlows.filter(function(f) { return f.status_code >= 400; }).length;
        state.stats.errorRate = (errors / state.recentFlows.length) * 100;

        // Update UI
        updateStatsUI();
    }

    function updateStatsUI() {
        var packetsEl = document.getElementById('pf-stat-packets');
        var latencyEl = document.getElementById('pf-stat-latency');
        var errorsEl = document.getElementById('pf-stat-errors');

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

        var delay = Math.min(
            CONFIG.reconnectBaseDelay * Math.pow(CONFIG.reconnectMultiplier, state.reconnectAttempts),
            CONFIG.reconnectMaxDelay
        );

        state.reconnectAttempts++;
        console.log('[PacketFlow] Dashboard-backend reconnecting in', delay, 'ms (attempt', state.reconnectAttempts + ')');

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

    function updateConnectionStatus(status, labelOverride) {
        var indicator = document.getElementById('packet-flow-status');
        if (indicator) {
            indicator.className = 'packet-flow-status ' + status;
            indicator.setAttribute('aria-label', 'WebSocket status: ' + status);
            var textEl = indicator.querySelector('.status-text');
            if (textEl) {
                textEl.textContent = labelOverride || (status.charAt(0).toUpperCase() + status.slice(1));
            }
        }

        // Update the reconnection banner
        updateReconnectBanner(status);

        // Notify callbacks
        state.statusCallbacks.forEach(function(callback) {
            try {
                callback(status);
            } catch (e) {
                console.error('[PacketFlow] Status callback error:', e);
            }
        });
    }

    // Manage the top reconnection banner
    function updateReconnectBanner(status) {
        var banner = document.getElementById('ws-reconnect-banner');
        if (!banner) return;

        var textEl = banner.querySelector('.reconnect-text');
        var retryBtn = banner.querySelector('#reconnect-retry-btn');

        if (status === 'connected' || status === 'demo') {
            banner.classList.remove('visible', 'error');
            return;
        }

        if (status === 'connecting') {
            banner.classList.add('visible');
            banner.classList.remove('error');
            if (textEl) {
                if (state.reconnectAttempts > 0) {
                    textEl.textContent = 'Reconnecting to server (attempt ' + state.reconnectAttempts + ')...';
                } else {
                    textEl.textContent = 'Connecting to server...';
                }
            }
            return;
        }

        if (status === 'disconnected') {
            banner.classList.add('visible');
            if (state.reconnectAttempts >= 5) {
                banner.classList.add('error');
                if (textEl) textEl.textContent = 'Connection lost. Check network or server status.';
            } else {
                banner.classList.remove('error');
                if (textEl) textEl.textContent = 'Disconnected from server. Attempting to reconnect...';
            }
        }

        // Wire up retry button
        if (retryBtn && !retryBtn._bound) {
            retryBtn._bound = true;
            retryBtn.addEventListener('click', function() {
                reconnect();
            });
        }
    }

    // ========================================================================
    // Animation & Rendering
    // ========================================================================
    function startAnimation() {
        function animate(timestamp) {
            var deltaTime = timestamp - state.lastFrameTime;

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
        var now = Date.now();

        // Update packets
        state.packets = state.packets.filter(function(packet) {
            packet.progress += packet.speed;

            // Store trail position
            var pos = getPacketPosition(packet);
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
        state.activeConnections.forEach(function(time, key) {
            if (now - time > 2000) {
                state.activeConnections.delete(key);
            }
        });

        // Demo mode: simulate packet flow when BOTH sources are disconnected
        // Only active when ?demo=true is in the URL query string
        var isDemoParam = (window.location.search.indexOf('demo=true') !== -1);
        if (!state.wsConnected && !state.traceWsConnected && CONFIG.demoFallback && isDemoParam && Math.random() < 0.02) {
            simulatePacket();
        }
    }

    function simulatePacket() {
        var services = Object.keys(CONFIG.services);
        var startService = 'gateway';
        var endService = services[Math.floor(Math.random() * services.length)];

        if (startService === endService) return;

        // Create simulated flow
        var simulatedFlow = {
            trace_id: 'sim-' + generateId(),
            status_code: Math.random() > 0.1 ? 200 : 500,
            total_time: (5 + Math.random() * 20) * 1e6,
            hops: [
                { component: 'gateway' },
                { component: 'wotan' },
                { component: endService }
            ]
        };

        createPacketFromFlow(simulatedFlow);
    }

    function render() {
        var ctx = state.ctx;
        var width = state.width;
        var height = state.height;

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
        ctx.strokeStyle = CONFIG.colors.gridColor;
        ctx.lineWidth = 1;

        var gridSize = 40;

        // Draw vertical lines
        for (var x = 0; x < width; x += gridSize) {
            ctx.beginPath();
            ctx.moveTo(x, 0);
            ctx.lineTo(x, height);
            ctx.stroke();
        }

        // Draw horizontal lines
        for (var y = 0; y < height; y += gridSize) {
            ctx.beginPath();
            ctx.moveTo(0, y);
            ctx.lineTo(width, y);
            ctx.stroke();
        }

        // Draw subtle radial gradient overlay from center
        var centerX = width / 2;
        var centerY = height / 2;
        var gradient = ctx.createRadialGradient(centerX, centerY, 0, centerX, centerY, Math.max(width, height) / 2);
        gradient.addColorStop(0, 'rgba(255, 215, 0, 0.02)');
        gradient.addColorStop(1, 'transparent');
        ctx.fillStyle = gradient;
        ctx.fillRect(0, 0, width, height);
    }

    function drawConnections(ctx, width, height) {
        var now = Date.now();

        CONFIG.connections.forEach(function(conn) {
            var from = conn[0];
            var to = conn[1];
            var fromService = CONFIG.services[from];
            var toService = CONFIG.services[to];

            if (!fromService || !toService) return;

            var x1 = fromService.x * width;
            var y1 = fromService.y * height;
            var x2 = toService.x * width;
            var y2 = toService.y * height;

            // Check if connection is active
            var key = from + '-' + to;
            var reverseKey = to + '-' + from;
            var isActive = state.activeConnections.has(key) || state.activeConnections.has(reverseKey);

            // Draw connection line
            ctx.beginPath();
            ctx.moveTo(x1, y1);

            // Use bezier curve for smoother connections
            var midX = (x1 + x2) / 2;
            var midY = (y1 + y2) / 2;
            var controlOffset = Math.abs(y2 - y1) * 0.3;

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
        var dashLength = 10;
        var gapLength = 10;
        var animSpeed = 0.05;
        var offset = (now * animSpeed) % (dashLength + gapLength);

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
        Object.entries(CONFIG.services).forEach(function(entry) {
            var id = entry[0];
            var service = entry[1];
            var x = service.x * width;
            var y = service.y * height;
            var radius = CONFIG.nodeRadius;

            // Draw outer glow
            var outerGlow = ctx.createRadialGradient(x, y, 0, x, y, radius * 2.5);
            outerGlow.addColorStop(0, service.color + '25');
            outerGlow.addColorStop(0.5, service.color + '10');
            outerGlow.addColorStop(1, 'transparent');
            ctx.fillStyle = outerGlow;
            ctx.beginPath();
            ctx.arc(x, y, radius * 2.5, 0, Math.PI * 2);
            ctx.fill();

            // Draw node background (Kingdom dark)
            ctx.fillStyle = CONFIG.colors.nodeFill;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw node border with glow effect
            ctx.strokeStyle = service.color;
            ctx.lineWidth = 2.5;
            ctx.shadowColor = service.color;
            ctx.shadowBlur = 12;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.stroke();
            ctx.shadowBlur = 0;

            // Draw inner highlight
            var innerGradient = ctx.createRadialGradient(x - radius * 0.3, y - radius * 0.3, 0, x, y, radius);
            innerGradient.addColorStop(0, 'rgba(255, 255, 255, 0.08)');
            innerGradient.addColorStop(1, 'transparent');
            ctx.fillStyle = innerGradient;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw service icon/initial
            ctx.fillStyle = service.color;
            ctx.font = 'bold 14px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText(service.name.charAt(0).toUpperCase(), x, y);

            // Draw label with background
            var labelY = y + CONFIG.nodeLabelOffset;
            ctx.font = '11px "SF Mono", monospace';
            var labelWidth = ctx.measureText(service.name).width + 12;

            // Label background
            ctx.fillStyle = 'rgba(15, 15, 26, 0.85)';
            ctx.beginPath();
            ctx.roundRect(x - labelWidth / 2, labelY - 8, labelWidth, 18, 4);
            ctx.fill();

            // Label border
            ctx.strokeStyle = 'rgba(255, 215, 0, 0.2)';
            ctx.lineWidth = 1;
            ctx.stroke();

            // Label text
            ctx.fillStyle = CONFIG.colors.labelColor;
            ctx.textAlign = 'center';
            ctx.textBaseline = 'middle';
            ctx.fillText(service.name, x, labelY + 1);
        });
    }

    function drawPackets(ctx, width, height) {
        state.packets.forEach(function(packet) {
            var pos = getPacketPosition(packet);
            if (!pos) return;

            // Draw trail
            if (packet.trail.length > 1) {
                ctx.beginPath();
                ctx.moveTo(packet.trail[0].x, packet.trail[0].y);

                for (var i = 1; i < packet.trail.length; i++) {
                    ctx.lineTo(packet.trail[i].x, packet.trail[i].y);
                }

                var gradient = ctx.createLinearGradient(
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
        var activePacket = state.packets[state.packets.length - 1];
        if (!activePacket) return;

        var infoX = 12;
        var infoY = 12;
        var infoWidth = 220;
        var infoHeight = 70;

        // Draw info panel background with Kingdom styling
        ctx.fillStyle = 'rgba(22, 33, 62, 0.92)';
        ctx.beginPath();
        ctx.roundRect(infoX, infoY, infoWidth, infoHeight, 6);
        ctx.fill();

        // Draw border
        ctx.strokeStyle = 'rgba(255, 215, 0, 0.25)';
        ctx.lineWidth = 1;
        ctx.stroke();

        // Draw top accent line
        ctx.fillStyle = 'rgba(255, 215, 0, 0.6)';
        ctx.fillRect(infoX, infoY, infoWidth, 2);

        ctx.textAlign = 'left';
        ctx.textBaseline = 'top';

        // Trace ID
        ctx.fillStyle = CONFIG.colors.traceIdColor;
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillText('TRACE ' + activePacket.traceId.slice(0, 16), infoX + 8, infoY + 10);

        // Latency
        ctx.fillStyle = CONFIG.colors.latencyColor;
        ctx.fillText(activePacket.latency + 'ms', infoX + 8, infoY + 26);

        // Status code with color
        var statusColor = activePacket.statusCode < 300 ? CONFIG.colors.successPacket :
                       activePacket.statusCode < 400 ? CONFIG.colors.warningPacket :
                       CONFIG.colors.errorPacket;
        ctx.fillStyle = statusColor;
        ctx.fillText(activePacket.statusCode, infoX + 70, infoY + 26);

        // Method and path
        ctx.fillStyle = CONFIG.colors.labelColor;
        ctx.font = '9px "SF Mono", monospace';
        var pathText = (activePacket.method + ' ' + activePacket.path).slice(0, 32);
        ctx.fillText(pathText, infoX + 8, infoY + 44);
    }

    function getPacketPosition(packet) {
        if (packet.currentSegment >= packet.route.length - 1) return null;

        var fromService = CONFIG.services[packet.route[packet.currentSegment]];
        var toService = CONFIG.services[packet.route[packet.currentSegment + 1]];

        if (!fromService || !toService) return null;

        var x1 = fromService.x * state.width;
        var y1 = fromService.y * state.height;
        var x2 = toService.x * state.width;
        var y2 = toService.y * state.height;

        // Use quadratic bezier for curved path
        var t = packet.progress;
        var midX = (x1 + x2) / 2;
        var midY = (y1 + y2) / 2;
        var controlY = midY - Math.abs(y2 - y1) * 0.3;

        // Quadratic bezier formula
        var x = Math.pow(1 - t, 2) * x1 + 2 * (1 - t) * t * midX + Math.pow(t, 2) * x2;
        var y = Math.pow(1 - t, 2) * y1 + 2 * (1 - t) * t * controlY + Math.pow(t, 2) * y2;

        return { x: x, y: y };
    }

    // ========================================================================
    // Utility Functions
    // ========================================================================
    function generateId() {
        return Math.random().toString(36).substring(2, 10);
    }

    function debounce(func, wait) {
        var timeout;
        return function() {
            var args = arguments;
            var context = this;
            var later = function() {
                clearTimeout(timeout);
                func.apply(context, args);
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
        if (state.traceReconnectTimeout) {
            clearTimeout(state.traceReconnectTimeout);
        }
        if (state.ws) {
            state.ws.close(1000, 'Manual disconnect');
            state.ws = null;
        }
        if (state.traceWs) {
            state.traceWs.close(1000, 'Manual disconnect');
            state.traceWs = null;
        }
        state.wsConnected = false;
        state.traceWsConnected = false;
        updateConnectionStatus('disconnected');
    }

    function reconnect() {
        disconnect();
        state.reconnectAttempts = 0;
        state.traceReconnectAttempts = 0;
        connectWebSocket();
        connectTraceCollector();
    }

    function setWsUrl(url) {
        CONFIG.wsUrl = url;
        reconnect();
    }

    function setTraceCollectorWsUrl(url) {
        CONFIG.traceCollectorWsUrl = url;
        if (state.traceWs) {
            state.traceWs.close(1000, 'URL changed');
            state.traceWs = null;
        }
        state.traceReconnectAttempts = 0;
        connectTraceCollector();
    }

    function onStatusChange(callback) {
        state.statusCallbacks.add(callback);
        return function() { state.statusCallbacks.delete(callback); };
    }

    function getStats() {
        return {
            totalPackets: state.stats.totalPackets,
            avgLatency: state.stats.avgLatency,
            errorRate: state.stats.errorRate
        };
    }

    function isConnected() {
        return state.wsConnected || state.traceWsConnected;
    }

    function destroy() {
        stopAnimation();
        disconnect();

        if (state.canvas && state.canvas.parentElement) {
            state.canvas.parentElement.removeChild(state.canvas);
        }

        var status = document.getElementById('packet-flow-status');
        if (status) status.remove();

        var stats = document.getElementById('packet-flow-stats');
        if (stats) stats.remove();
    }

    // Add demo packet for testing
    function addDemoPacket(options) {
        var defaultOptions = {
            trace_id: 'demo-' + generateId(),
            status_code: 200,
            total_time: 15 * 1e6,
            method: 'GET',
            path: '/api/v1/demo',
            hops: [
                { component: 'gateway' },
                { component: 'wotan' },
                { component: 'timeguru' }
            ]
        };

        var merged = {};
        for (var k in defaultOptions) { merged[k] = defaultOptions[k]; }
        if (options) { for (var k2 in options) { merged[k2] = options[k2]; } }
        createPacketFromFlow(merged);
    }

    return {
        init: init,
        disconnect: disconnect,
        reconnect: reconnect,
        setWsUrl: setWsUrl,
        setTraceCollectorWsUrl: setTraceCollectorWsUrl,
        onStatusChange: onStatusChange,
        getStats: getStats,
        isConnected: isConnected,
        destroy: destroy,
        addDemoPacket: addDemoPacket,
        CONFIG: CONFIG
    };
})();

// Auto-initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    // Check if container exists
    var container = document.getElementById('flow-viz');
    if (container) {
        // Determine WebSocket URLs from globals or derive from current page location
        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var wsUrl = window.PACKET_FLOW_WS_URL || (protocol + '//' + window.location.hostname + ':8080/ws');
        var traceWsUrl = window.TRACE_COLLECTOR_WS_URL || (protocol + '//' + window.location.hostname + ':9091');

        PacketFlowViz.init('flow-viz', {
            wsUrl: wsUrl,
            traceCollectorWsUrl: traceWsUrl
        });
    }
});

// Make available globally
window.PacketFlowViz = PacketFlowViz;

console.log('packet-flow.js loaded (wired to real data sources)');
