/**
 * Kingdom Dashboard - Frontend JavaScript
 * Real-time monitoring dashboard for Unheaded infrastructure
 *
 * Features:
 * - WebSocket connection with auto-reconnect
 * - Real-time metrics display with charts and gauges
 * - Service health status cards
 * - Packet flow visualization
 * - Event feed with filtering
 * - Toast notifications
 */

(function() {
    'use strict';

    // ==========================================================================
    // Configuration
    // ==========================================================================
    const CONFIG = {
        // WebSocket
        wsEndpoint: '/ws',
        streamEndpoint: '/api/v1/stream',
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 30000,
        reconnectMultiplier: 1.5,
        heartbeatInterval: 30000,

        // API endpoints
        api: {
            health: '/api/v1/health',
            services: '/api/v1/services',
            metrics: '/api/v1/metrics',
            events: '/api/v1/events',
            stats: '/api/v1/stats',
            flows: '/api/v1/flows'
        },

        // Data refresh intervals (ms)
        refreshIntervals: {
            health: 15000,
            services: 10000,
            metrics: 5000,
            events: 10000,
            stats: 5000
        },

        // Chart settings
        charts: {
            maxDataPoints: 60,
            animationDuration: 300
        },

        // Flow visualization
        flow: {
            maxFlows: 50,
            particleSpeed: 2,
            trailLength: 10
        },

        // Events
        events: {
            maxEvents: 100
        }
    };

    // ==========================================================================
    // State Management
    // ==========================================================================
    const state = {
        // Connection state
        ws: null,
        wsConnected: false,
        reconnectAttempts: 0,
        reconnectTimeout: null,
        lastHeartbeat: null,

        // Data state
        services: {},
        systemHealth: null,
        stats: null,
        events: [],
        flows: [],
        currentFilter: 'all',

        // Metrics history for charts
        metricsHistory: {
            requests: [],
            latency: [],
            timestamps: []
        },

        // Gauges
        gauges: {
            cpu: 0,
            memory: 0,
            goroutines: 0
        },

        // Animation state
        animationFrame: null,
        startTime: Date.now()
    };

    // ==========================================================================
    // DOM Elements Cache
    // ==========================================================================
    const elements = {};

    function cacheElements() {
        // Header elements
        elements.overallStatusIndicator = document.getElementById('overall-status-indicator');
        elements.overallStatusText = document.getElementById('overall-status-text');
        elements.wsIndicator = document.getElementById('ws-indicator');
        elements.wsStatusText = document.getElementById('ws-status-text');
        elements.lastUpdate = document.getElementById('last-update');

        // Overview metrics
        elements.totalServices = document.getElementById('total-services');
        elements.healthyServices = document.getElementById('healthy-services');
        elements.degradedServices = document.getElementById('degraded-services');
        elements.unhealthyServices = document.getElementById('unhealthy-services');
        elements.wsConnections = document.getElementById('ws-connections');
        elements.eventCount = document.getElementById('event-count');

        // Services
        elements.servicesGrid = document.getElementById('services-grid');

        // Flow
        elements.flowCanvas = document.getElementById('flow-canvas');
        elements.flowCount = document.getElementById('flow-count');
        elements.avgLatency = document.getElementById('avg-latency');

        // Charts
        elements.requestsChart = document.getElementById('requests-chart');
        elements.latencyChart = document.getElementById('latency-chart');

        // Gauges
        elements.cpuGauge = document.getElementById('cpu-gauge');
        elements.memoryGauge = document.getElementById('memory-gauge');
        elements.goroutinesGauge = document.getElementById('goroutines-gauge');
        elements.cpuValue = document.getElementById('cpu-value');
        elements.memoryValue = document.getElementById('memory-value');
        elements.goroutinesValue = document.getElementById('goroutines-value');

        // Events
        elements.filterToggle = document.getElementById('filter-toggle');
        elements.eventFilters = document.getElementById('event-filters');
        elements.eventsList = document.getElementById('events-list');

        // Footer
        elements.uptime = document.getElementById('uptime');
        elements.serverTime = document.getElementById('server-time');

        // Toast container
        elements.toastContainer = document.getElementById('toast-container');
    }

    // ==========================================================================
    // WebSocket Connection
    // ==========================================================================
    function connectWebSocket() {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) {
            return;
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}${CONFIG.wsEndpoint}`;

        updateConnectionStatus('connecting');
        console.log('[WS] Connecting to:', wsUrl);

        try {
            state.ws = new WebSocket(wsUrl);

            state.ws.onopen = handleWebSocketOpen;
            state.ws.onclose = handleWebSocketClose;
            state.ws.onerror = handleWebSocketError;
            state.ws.onmessage = handleWebSocketMessage;
        } catch (error) {
            console.error('[WS] Connection error:', error);
            scheduleReconnect();
        }
    }

    function handleWebSocketOpen(event) {
        console.log('[WS] Connected');
        state.wsConnected = true;
        state.reconnectAttempts = 0;
        state.lastHeartbeat = Date.now();
        updateConnectionStatus('connected');
        showToast('success', 'Connected', 'WebSocket connection established');

        // Start heartbeat monitoring
        startHeartbeatMonitor();
    }

    function handleWebSocketClose(event) {
        console.log('[WS] Disconnected:', event.code, event.reason);
        state.wsConnected = false;
        state.ws = null;
        updateConnectionStatus('disconnected');

        if (event.code !== 1000) {
            scheduleReconnect();
        }
    }

    function handleWebSocketError(error) {
        console.error('[WS] Error:', error);
        state.wsConnected = false;
        updateConnectionStatus('disconnected');
    }

    function handleWebSocketMessage(event) {
        state.lastHeartbeat = Date.now();

        try {
            const message = JSON.parse(event.data);
            processMessage(message);
        } catch (error) {
            console.error('[WS] Parse error:', error);
        }
    }

    function processMessage(message) {
        switch (message.type) {
            case 'health_update':
                handleHealthUpdate(message.data);
                break;
            case 'packet_flow':
                handlePacketFlow(message.data);
                break;
            case 'event':
                handleEvent(message.data);
                break;
            case 'metrics':
                handleMetricsUpdate(message.data);
                break;
            case 'pong':
                // Heartbeat response
                break;
            default:
                console.log('[WS] Unknown message type:', message.type);
        }
    }

    function scheduleReconnect() {
        if (state.reconnectTimeout) {
            clearTimeout(state.reconnectTimeout);
        }

        const delay = Math.min(
            CONFIG.reconnectBaseDelay * Math.pow(CONFIG.reconnectMultiplier, state.reconnectAttempts),
            CONFIG.reconnectMaxDelay
        );

        state.reconnectAttempts++;
        console.log(`[WS] Reconnecting in ${delay}ms (attempt ${state.reconnectAttempts})`);

        state.reconnectTimeout = setTimeout(() => {
            connectWebSocket();
        }, delay);
    }

    function startHeartbeatMonitor() {
        setInterval(() => {
            if (state.wsConnected && state.ws) {
                // Send ping
                try {
                    state.ws.send(JSON.stringify({ type: 'ping' }));
                } catch (e) {
                    console.error('[WS] Ping failed:', e);
                }

                // Check for stale connection
                const timeSinceHeartbeat = Date.now() - state.lastHeartbeat;
                if (timeSinceHeartbeat > CONFIG.heartbeatInterval * 2) {
                    console.warn('[WS] Connection appears stale, reconnecting...');
                    state.ws.close();
                    connectWebSocket();
                }
            }
        }, CONFIG.heartbeatInterval);
    }

    function updateConnectionStatus(status) {
        const indicator = elements.wsIndicator;
        const text = elements.wsStatusText;

        indicator.className = 'connection-indicator';

        switch (status) {
            case 'connected':
                indicator.classList.add('connected');
                text.textContent = 'Connected';
                break;
            case 'connecting':
                indicator.classList.add('connecting');
                text.textContent = 'Connecting...';
                break;
            case 'disconnected':
            default:
                indicator.classList.add('disconnected');
                text.textContent = 'Disconnected';
                break;
        }
    }

    // ==========================================================================
    // API Fetching
    // ==========================================================================
    async function fetchJSON(url) {
        try {
            const response = await fetch(url);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            return await response.json();
        } catch (error) {
            console.error(`[API] Fetch error for ${url}:`, error);
            return null;
        }
    }

    async function refreshHealth() {
        const data = await fetchJSON(CONFIG.api.health);
        if (data) {
            state.systemHealth = data;
            updateSystemHealthUI(data);
        }
    }

    async function refreshServices() {
        const data = await fetchJSON(CONFIG.api.services);
        if (data) {
            state.services = data.services || [];
            updateServicesUI(data);
        }
    }

    async function refreshEvents() {
        const data = await fetchJSON(CONFIG.api.events + '?limit=50');
        if (data && data.events) {
            state.events = data.events;
            updateEventsUI();
        }
    }

    async function refreshStats() {
        const data = await fetchJSON(CONFIG.api.stats);
        if (data) {
            state.stats = data;
            updateStatsUI(data);
        }
    }

    // ==========================================================================
    // UI Updates
    // ==========================================================================
    function updateSystemHealthUI(health) {
        // Update overall status
        const indicator = elements.overallStatusIndicator;
        const text = elements.overallStatusText;

        indicator.className = 'status-indicator';
        indicator.classList.add(health.status);
        text.textContent = health.message || health.status;

        // Update counts
        elements.totalServices.textContent = health.total_services || 0;
        elements.healthyServices.textContent = health.healthy_count || 0;
        elements.degradedServices.textContent = health.degraded_count || 0;
        elements.unhealthyServices.textContent = health.unhealthy_count || 0;

        // Update timestamp
        elements.lastUpdate.textContent = new Date().toLocaleTimeString();
    }

    function updateServicesUI(data) {
        const grid = elements.servicesGrid;
        const services = data.services || [];

        // Clear placeholder
        grid.innerHTML = '';

        if (services.length === 0) {
            grid.innerHTML = '<div class="service-card placeholder"><div class="service-skeleton"></div></div>';
            return;
        }

        services.forEach(service => {
            const card = createServiceCard(service);
            grid.appendChild(card);
        });
    }

    function createServiceCard(service) {
        const card = document.createElement('div');
        card.className = `service-card ${service.status}`;
        card.dataset.service = service.name;

        const uptime = service.uptime_percent || 0;
        const latency = service.average_latency_ms || 0;

        card.innerHTML = `
            <div class="service-header">
                <span class="service-name">${escapeHtml(service.name)}</span>
                <span class="service-status-badge ${service.status}">${service.status}</span>
            </div>
            <div class="service-metrics">
                <div class="service-metric">
                    <span>Uptime:</span>
                    <span class="service-metric-value">${uptime.toFixed(1)}%</span>
                </div>
                <div class="service-metric">
                    <span>Latency:</span>
                    <span class="service-metric-value">${latency}ms</span>
                </div>
                <div class="service-metric">
                    <span>Checks:</span>
                    <span class="service-metric-value">${service.consecutive_successes || 0}/${service.consecutive_failures || 0}</span>
                </div>
            </div>
            <div class="service-uptime-bar">
                <div class="service-uptime-fill" style="width: ${Math.min(uptime, 100)}%"></div>
            </div>
        `;

        card.addEventListener('click', () => {
            showServiceDetails(service);
        });

        return card;
    }

    function updateEventsUI() {
        const list = elements.eventsList;
        const events = filterEvents(state.events);

        list.innerHTML = '';

        if (events.length === 0) {
            list.innerHTML = '<div class="event-item info"><div class="event-header"><span class="event-title">No events</span></div></div>';
            return;
        }

        events.slice(0, CONFIG.events.maxEvents).forEach(event => {
            const item = createEventItem(event);
            list.appendChild(item);
        });

        elements.eventCount.textContent = state.events.length;
    }

    function createEventItem(event) {
        const item = document.createElement('div');
        item.className = `event-item ${event.severity || 'info'}`;

        const time = new Date(event.timestamp).toLocaleTimeString();

        item.innerHTML = `
            <div class="event-header">
                <span class="event-title">${escapeHtml(event.title || event.type)}</span>
                <span class="event-time">${time}</span>
            </div>
            <div class="event-body">
                <span class="event-message">${escapeHtml(event.message || '')}</span>
                <span class="event-source">${escapeHtml(event.source || 'system')}</span>
            </div>
        `;

        return item;
    }

    function filterEvents(events) {
        if (state.currentFilter === 'all') {
            return events;
        }
        return events.filter(e => e.severity === state.currentFilter);
    }

    function updateStatsUI(stats) {
        if (stats.server) {
            elements.wsConnections.textContent = stats.server.ws_connections || 0;
        }

        // Update footer
        if (stats.server && stats.server.started) {
            const uptime = formatUptime(Date.now() - state.startTime);
            elements.uptime.textContent = `Uptime: ${uptime}`;
        }

        elements.serverTime.textContent = `Server: ${new Date().toLocaleTimeString()}`;

        // Simulate gauge updates (in production, these would come from actual metrics)
        updateGaugeValues({
            cpu: Math.random() * 40 + 20,
            memory: Math.random() * 30 + 40,
            goroutines: Math.floor(Math.random() * 50 + 100)
        });
    }

    function updateGaugeValues(values) {
        state.gauges = values;

        drawGauge(elements.cpuGauge, values.cpu, 100, getGaugeColor(values.cpu, 100));
        drawGauge(elements.memoryGauge, values.memory, 100, getGaugeColor(values.memory, 100));
        drawGauge(elements.goroutinesGauge, values.goroutines, 500, '#ffd700');

        elements.cpuValue.textContent = `${values.cpu.toFixed(1)}%`;
        elements.memoryValue.textContent = `${values.memory.toFixed(1)}%`;
        elements.goroutinesValue.textContent = values.goroutines;
    }

    function getGaugeColor(value, max) {
        const percent = value / max;
        if (percent < 0.5) return '#00d26a';
        if (percent < 0.75) return '#ff9800';
        return '#ff4757';
    }

    // ==========================================================================
    // Event Handlers
    // ==========================================================================
    function handleHealthUpdate(data) {
        if (data.service) {
            // Update specific service card
            const card = document.querySelector(`.service-card[data-service="${data.service}"]`);
            if (card) {
                card.className = `service-card ${data.new_status}`;
                const badge = card.querySelector('.service-status-badge');
                if (badge) {
                    badge.className = `service-status-badge ${data.new_status}`;
                    badge.textContent = data.new_status;
                }
            }

            // Show toast for status changes
            if (data.old_status !== data.new_status) {
                const severity = data.new_status === 'unhealthy' ? 'error' :
                                 data.new_status === 'degraded' ? 'warning' : 'success';
                showToast(severity, `${data.service} Status Change`,
                    `${data.old_status} -> ${data.new_status}`);
            }
        }

        // Refresh health data
        refreshHealth();
    }

    function handlePacketFlow(flow) {
        // Add to flows array
        state.flows.push(flow);
        if (state.flows.length > CONFIG.flow.maxFlows) {
            state.flows.shift();
        }

        // Update flow stats
        elements.flowCount.textContent = state.flows.length;

        const avgLatency = state.flows.reduce((sum, f) => sum + (f.total_time / 1e6), 0) / state.flows.length;
        elements.avgLatency.textContent = `${avgLatency.toFixed(2)}ms`;

        // Update metrics history
        updateMetricsHistory(flow);
    }

    function handleEvent(event) {
        // Add to events array
        state.events.unshift(event);
        if (state.events.length > CONFIG.events.maxEvents) {
            state.events.pop();
        }

        // Update UI
        updateEventsUI();

        // Show toast for critical/error events
        if (event.severity === 'critical' || event.severity === 'error') {
            showToast(event.severity, event.title || 'Alert', event.message || '');
        }
    }

    function handleMetricsUpdate(data) {
        // Update gauges if metrics contain relevant data
        if (data.cpu_percent !== undefined) {
            state.gauges.cpu = data.cpu_percent;
        }
        if (data.memory_percent !== undefined) {
            state.gauges.memory = data.memory_percent;
        }
        if (data.goroutines !== undefined) {
            state.gauges.goroutines = data.goroutines;
        }

        updateGaugeValues(state.gauges);
    }

    // ==========================================================================
    // Charts
    // ==========================================================================
    function updateMetricsHistory(flow) {
        const now = Date.now();

        // Calculate request rate (flows per second)
        const recentFlows = state.flows.filter(f =>
            new Date(f.timestamp).getTime() > now - 60000
        ).length;

        state.metricsHistory.requests.push(recentFlows);
        state.metricsHistory.latency.push(flow.total_time / 1e6);
        state.metricsHistory.timestamps.push(now);

        // Keep last N data points
        const maxPoints = CONFIG.charts.maxDataPoints;
        if (state.metricsHistory.requests.length > maxPoints) {
            state.metricsHistory.requests.shift();
            state.metricsHistory.latency.shift();
            state.metricsHistory.timestamps.shift();
        }

        // Redraw charts
        drawLineChart(elements.requestsChart, state.metricsHistory.requests, '#ffd700');
        drawLineChart(elements.latencyChart, state.metricsHistory.latency, '#00bcd4');
    }

    function drawLineChart(canvas, data, color) {
        if (!canvas || data.length < 2) return;

        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const padding = 10;

        // Clear canvas
        ctx.fillStyle = '#1e2746';
        ctx.fillRect(0, 0, width, height);

        // Calculate scales
        const maxValue = Math.max(...data, 1);
        const minValue = Math.min(...data, 0);
        const range = maxValue - minValue || 1;

        const xStep = (width - padding * 2) / (data.length - 1);
        const yScale = (height - padding * 2) / range;

        // Draw grid lines
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
        ctx.lineWidth = 1;
        for (let i = 0; i < 5; i++) {
            const y = padding + (height - padding * 2) * i / 4;
            ctx.beginPath();
            ctx.moveTo(padding, y);
            ctx.lineTo(width - padding, y);
            ctx.stroke();
        }

        // Draw line
        ctx.strokeStyle = color;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';

        ctx.beginPath();
        data.forEach((value, i) => {
            const x = padding + i * xStep;
            const y = height - padding - (value - minValue) * yScale;

            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        });
        ctx.stroke();

        // Draw gradient fill
        const gradient = ctx.createLinearGradient(0, 0, 0, height);
        gradient.addColorStop(0, color.replace(')', ', 0.3)').replace('rgb', 'rgba'));
        gradient.addColorStop(1, color.replace(')', ', 0)').replace('rgb', 'rgba'));

        ctx.fillStyle = gradient;
        ctx.lineTo(width - padding, height - padding);
        ctx.lineTo(padding, height - padding);
        ctx.closePath();
        ctx.fill();

        // Draw current value
        if (data.length > 0) {
            const currentValue = data[data.length - 1];
            ctx.fillStyle = '#f8f9fa';
            ctx.font = '12px monospace';
            ctx.textAlign = 'right';
            ctx.fillText(currentValue.toFixed(1), width - padding, padding + 12);
        }
    }

    function drawGauge(canvas, value, max, color) {
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;
        const centerX = width / 2;
        const centerY = height / 2;
        const radius = Math.min(width, height) / 2 - 10;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        // Draw background arc
        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, Math.PI * 0.75, Math.PI * 2.25, false);
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
        ctx.lineWidth = 10;
        ctx.lineCap = 'round';
        ctx.stroke();

        // Draw value arc
        const percentage = Math.min(value / max, 1);
        const endAngle = Math.PI * 0.75 + (Math.PI * 1.5 * percentage);

        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, Math.PI * 0.75, endAngle, false);
        ctx.strokeStyle = color;
        ctx.lineWidth = 10;
        ctx.lineCap = 'round';
        ctx.stroke();

        // Add glow effect
        ctx.shadowColor = color;
        ctx.shadowBlur = 10;
        ctx.beginPath();
        ctx.arc(centerX, centerY, radius, endAngle - 0.1, endAngle, false);
        ctx.stroke();
        ctx.shadowBlur = 0;
    }

    // ==========================================================================
    // Packet Flow Visualization
    // ==========================================================================
    const flowViz = {
        particles: [],
        nodes: [
            { name: 'XDP', x: 0.1, y: 0.5, color: '#ff6b6b' },
            { name: 'Gateway', x: 0.3, y: 0.3, color: '#ffd700' },
            { name: 'Busboy', x: 0.5, y: 0.5, color: '#4ecdc4' },
            { name: 'Service', x: 0.7, y: 0.3, color: '#45b7d1' },
            { name: 'Collector', x: 0.9, y: 0.5, color: '#a29bfe' }
        ]
    };

    function initFlowVisualization() {
        const canvas = elements.flowCanvas;
        if (!canvas) return;

        // Set canvas size
        resizeFlowCanvas();
        window.addEventListener('resize', resizeFlowCanvas);

        // Start animation loop
        animateFlow();
    }

    function resizeFlowCanvas() {
        const canvas = elements.flowCanvas;
        const wrapper = canvas.parentElement;
        canvas.width = wrapper.clientWidth;
        canvas.height = wrapper.clientHeight;
    }

    function animateFlow() {
        const canvas = elements.flowCanvas;
        const ctx = canvas.getContext('2d');
        const width = canvas.width;
        const height = canvas.height;

        // Clear canvas
        ctx.fillStyle = '#1e2746';
        ctx.fillRect(0, 0, width, height);

        // Draw connections
        drawFlowConnections(ctx, width, height);

        // Draw nodes
        drawFlowNodes(ctx, width, height);

        // Update and draw particles
        updateFlowParticles();
        drawFlowParticles(ctx, width, height);

        // Add new particles from recent flows
        if (state.flows.length > 0 && Math.random() < 0.3) {
            addFlowParticle();
        }

        state.animationFrame = requestAnimationFrame(animateFlow);
    }

    function drawFlowConnections(ctx, width, height) {
        ctx.strokeStyle = 'rgba(255, 215, 0, 0.2)';
        ctx.lineWidth = 2;

        for (let i = 0; i < flowViz.nodes.length - 1; i++) {
            const from = flowViz.nodes[i];
            const to = flowViz.nodes[i + 1];

            ctx.beginPath();
            ctx.moveTo(from.x * width, from.y * height);

            // Bezier curve for smooth connections
            const cp1x = from.x * width + (to.x - from.x) * width * 0.5;
            const cp1y = from.y * height;
            const cp2x = from.x * width + (to.x - from.x) * width * 0.5;
            const cp2y = to.y * height;

            ctx.bezierCurveTo(cp1x, cp1y, cp2x, cp2y, to.x * width, to.y * height);
            ctx.stroke();
        }
    }

    function drawFlowNodes(ctx, width, height) {
        flowViz.nodes.forEach(node => {
            const x = node.x * width;
            const y = node.y * height;
            const radius = 20;

            // Draw glow
            const gradient = ctx.createRadialGradient(x, y, 0, x, y, radius * 2);
            gradient.addColorStop(0, node.color + '40');
            gradient.addColorStop(1, 'transparent');
            ctx.fillStyle = gradient;
            ctx.beginPath();
            ctx.arc(x, y, radius * 2, 0, Math.PI * 2);
            ctx.fill();

            // Draw node
            ctx.fillStyle = node.color;
            ctx.beginPath();
            ctx.arc(x, y, radius, 0, Math.PI * 2);
            ctx.fill();

            // Draw border
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
            ctx.lineWidth = 2;
            ctx.stroke();

            // Draw label
            ctx.fillStyle = '#f8f9fa';
            ctx.font = '10px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText(node.name, x, y + radius + 15);
        });
    }

    function addFlowParticle() {
        const flow = state.flows[Math.floor(Math.random() * state.flows.length)];
        const isError = flow && flow.status_code >= 400;

        flowViz.particles.push({
            progress: 0,
            speed: 0.01 + Math.random() * 0.01,
            color: isError ? '#ff4757' : '#ffd700',
            size: 3 + Math.random() * 2,
            trail: []
        });
    }

    function updateFlowParticles() {
        flowViz.particles = flowViz.particles.filter(particle => {
            particle.progress += particle.speed;

            // Store trail
            if (particle.trail.length > CONFIG.flow.trailLength) {
                particle.trail.shift();
            }
            particle.trail.push(particle.progress);

            return particle.progress < 1;
        });
    }

    function drawFlowParticles(ctx, width, height) {
        flowViz.particles.forEach(particle => {
            const pos = getParticlePosition(particle.progress, width, height);

            // Draw trail
            ctx.strokeStyle = particle.color + '40';
            ctx.lineWidth = particle.size / 2;
            ctx.beginPath();
            particle.trail.forEach((p, i) => {
                const trailPos = getParticlePosition(p, width, height);
                if (i === 0) {
                    ctx.moveTo(trailPos.x, trailPos.y);
                } else {
                    ctx.lineTo(trailPos.x, trailPos.y);
                }
            });
            ctx.stroke();

            // Draw particle
            ctx.fillStyle = particle.color;
            ctx.shadowColor = particle.color;
            ctx.shadowBlur = 10;
            ctx.beginPath();
            ctx.arc(pos.x, pos.y, particle.size, 0, Math.PI * 2);
            ctx.fill();
            ctx.shadowBlur = 0;
        });
    }

    function getParticlePosition(progress, width, height) {
        const nodes = flowViz.nodes;
        const totalSegments = nodes.length - 1;
        const segment = Math.min(Math.floor(progress * totalSegments), totalSegments - 1);
        const segmentProgress = (progress * totalSegments) - segment;

        const from = nodes[segment];
        const to = nodes[segment + 1];

        // Bezier curve interpolation
        const t = segmentProgress;
        const cp1x = from.x + (to.x - from.x) * 0.5;
        const cp1y = from.y;
        const cp2x = from.x + (to.x - from.x) * 0.5;
        const cp2y = to.y;

        const x = Math.pow(1-t, 3) * from.x +
                  3 * Math.pow(1-t, 2) * t * cp1x +
                  3 * (1-t) * Math.pow(t, 2) * cp2x +
                  Math.pow(t, 3) * to.x;

        const y = Math.pow(1-t, 3) * from.y +
                  3 * Math.pow(1-t, 2) * t * cp1y +
                  3 * (1-t) * Math.pow(t, 2) * cp2y +
                  Math.pow(t, 3) * to.y;

        return { x: x * width, y: y * height };
    }

    // ==========================================================================
    // Toast Notifications
    // ==========================================================================
    function showToast(type, title, message, duration = 5000) {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;

        const icons = {
            success: '&#x2705;',
            error: '&#x274C;',
            warning: '&#x26A0;',
            info: '&#x2139;'
        };

        toast.innerHTML = `
            <span class="toast-icon">${icons[type] || icons.info}</span>
            <div class="toast-content">
                <div class="toast-title">${escapeHtml(title)}</div>
                <div class="toast-message">${escapeHtml(message)}</div>
            </div>
            <button class="toast-close" onclick="this.parentElement.remove()">&times;</button>
        `;

        elements.toastContainer.appendChild(toast);

        // Auto remove after duration
        setTimeout(() => {
            toast.classList.add('hiding');
            setTimeout(() => toast.remove(), 300);
        }, duration);
    }

    // ==========================================================================
    // Event Filtering
    // ==========================================================================
    function setupEventFiltering() {
        elements.filterToggle.addEventListener('click', () => {
            elements.eventFilters.classList.toggle('show');
        });

        document.querySelectorAll('.filter-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                state.currentFilter = btn.dataset.severity;
                updateEventsUI();
            });
        });
    }

    // ==========================================================================
    // Service Details Modal
    // ==========================================================================
    function showServiceDetails(service) {
        // For now, just log - could be extended to show a modal
        console.log('Service details:', service);
        showToast('info', service.name, `Status: ${service.status}, Uptime: ${(service.uptime_percent || 0).toFixed(1)}%`);
    }

    // ==========================================================================
    // Utility Functions
    // ==========================================================================
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    function formatUptime(ms) {
        const seconds = Math.floor(ms / 1000);
        const minutes = Math.floor(seconds / 60);
        const hours = Math.floor(minutes / 60);
        const days = Math.floor(hours / 24);

        if (days > 0) return `${days}d ${hours % 24}h`;
        if (hours > 0) return `${hours}h ${minutes % 60}m`;
        if (minutes > 0) return `${minutes}m ${seconds % 60}s`;
        return `${seconds}s`;
    }

    // ==========================================================================
    // Initialization
    // ==========================================================================
    function init() {
        console.log('[Dashboard] Initializing...');

        // Cache DOM elements
        cacheElements();

        // Setup event filtering
        setupEventFiltering();

        // Connect WebSocket
        connectWebSocket();

        // Initialize visualizations
        initFlowVisualization();

        // Initial data fetch
        refreshHealth();
        refreshServices();
        refreshEvents();
        refreshStats();

        // Setup refresh intervals
        setInterval(refreshHealth, CONFIG.refreshIntervals.health);
        setInterval(refreshServices, CONFIG.refreshIntervals.services);
        setInterval(refreshEvents, CONFIG.refreshIntervals.events);
        setInterval(refreshStats, CONFIG.refreshIntervals.stats);

        // Initialize charts with empty data
        drawLineChart(elements.requestsChart, [0], '#ffd700');
        drawLineChart(elements.latencyChart, [0], '#00bcd4');

        // Initialize gauges
        updateGaugeValues({ cpu: 0, memory: 0, goroutines: 0 });

        console.log('[Dashboard] Initialized');
    }

    // Start when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    // Cleanup on page unload
    window.addEventListener('beforeunload', () => {
        if (state.ws) {
            state.ws.close(1000, 'Page unload');
        }
        if (state.animationFrame) {
            cancelAnimationFrame(state.animationFrame);
        }
    });

})();
