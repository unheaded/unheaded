/**
 * Kingdom Dashboard - Frontend JavaScript (Campaign 2.3)
 * Real-time monitoring dashboard with 4 pages:
 *   1. Overview — system metrics, eBPF stats, service health
 *   2. Flow Graph — real-time network topology canvas
 *   3. Latency — P50/P90/P99 bar charts per operation
 *   4. Events — live scrolling event stream with topic filtering
 */
(function() {
    'use strict';

    // ======================================================================
    // Configuration
    // ======================================================================
    var CONFIG = {
        wsEndpoint: '/ws',
        reconnectBaseDelay: 1000,
        reconnectMaxDelay: 30000,
        reconnectMultiplier: 1.5,
        heartbeatInterval: 30000,

        api: {
            health: '/api/v1/health',
            services: '/api/v1/services',
            metrics: '/api/v1/metrics',
            events: '/api/v1/events',
            stats: '/api/v1/stats',
            flows: '/api/v1/flows',
            latency: '/api/v1/latency',
            ebpfStats: '/api/v1/ebpf/stats',
            ebpfEvents: '/api/v1/ebpf/events'
        },

        refreshIntervals: {
            health: 15000,
            services: 10000,
            stats: 5000,
            flows: 3000,
            latency: 3000,
            ebpfStats: 5000,
            ebpfEvents: 2000
        },

        charts: { maxDataPoints: 60 },
        flow: { maxNodes: 40, maxFlows: 80 },
        events: { maxItems: 200 }
    };

    // ======================================================================
    // State
    // ======================================================================
    var state = {
        ws: null,
        wsConnected: false,
        reconnectAttempts: 0,
        reconnectTimeout: null,
        lastHeartbeat: null,
        activePage: 'overview',

        services: {},
        systemHealth: null,
        stats: null,
        events: [],
        flows: [],
        flowNodes: [],
        flowSource: 'unknown',
        latencyData: {},
        ebpfStats: {},
        ebpfEvents: [],
        ebpfActive: false,

        eventStreamPaused: false,
        eventTopicFilter: '',
        eventTypeFilter: 'all',
        eventStreamTotal: 0,

        latencyHistory: [],
        metricsHistory: { requests: [], latency: [], timestamps: [] },
        gauges: { cpu: 0, memory: 0, goroutines: 0 },

        animationFrame: null,
        startTime: Date.now()
    };

    // ======================================================================
    // DOM Element Cache
    // ======================================================================
    var el = {};

    function cacheElements() {
        el.wsIndicator = document.getElementById('ws-indicator');
        el.wsStatusText = document.getElementById('ws-status-text');
        el.lastUpdate = document.getElementById('last-update');

        el.totalServices = document.getElementById('total-services');
        el.healthyServices = document.getElementById('healthy-services');
        el.degradedServices = document.getElementById('degraded-services');
        el.unhealthyServices = document.getElementById('unhealthy-services');
        el.ebpfEventsCount = document.getElementById('ebpf-events-count');
        el.activeFlowsCount = document.getElementById('active-flows-count');
        el.servicesGrid = document.getElementById('services-grid');

        el.statPackets = document.getElementById('stat-packets');
        el.statFlows = document.getElementById('stat-flows');
        el.statLatencySamples = document.getElementById('stat-latency-samples');
        el.statErrors = document.getElementById('stat-errors');
        el.statCompute = document.getElementById('stat-compute');
        el.statAnamnesis = document.getElementById('stat-anamnesis');
        el.statEps = document.getElementById('stat-eps');
        el.statUptime = document.getElementById('stat-uptime');

        el.cpuGauge = document.getElementById('cpu-gauge');
        el.memoryGauge = document.getElementById('memory-gauge');
        el.goroutinesGauge = document.getElementById('goroutines-gauge');
        el.cpuValue = document.getElementById('cpu-value');
        el.memoryValue = document.getElementById('memory-value');
        el.goroutinesValue = document.getElementById('goroutines-value');

        el.flowCanvas = document.getElementById('flow-canvas');
        el.flowGraphCount = document.getElementById('flow-graph-count');
        el.flowBytesSec = document.getElementById('flow-bytes-sec');
        el.flowPktsSec = document.getElementById('flow-pkts-sec');
        el.flowTableBody = document.getElementById('flow-table-body');

        el.latencySummaryGrid = document.getElementById('latency-summary-grid');
        el.latencyHistoryCanvas = document.getElementById('latency-history-canvas');

        el.eventTopicInput = document.getElementById('event-topic-input');
        el.eventPauseBtn = document.getElementById('event-pause-btn');
        el.eventStreamList = document.getElementById('event-stream-list');
        el.eventStreamTotal = document.getElementById('event-stream-total');
        el.eventStreamVisible = document.getElementById('event-stream-visible');
        el.eventStreamRate = document.getElementById('event-stream-rate');

        el.uptime = document.getElementById('uptime');
        el.serverTime = document.getElementById('server-time');
        el.toastContainer = document.getElementById('toast-container');
    }

    // ======================================================================
    // Navigation
    // ======================================================================
    function setupNavigation() {
        document.querySelectorAll('.nav-tab').forEach(function(tab) {
            tab.addEventListener('click', function() {
                switchPage(this.dataset.page);
            });
        });
    }

    function switchPage(page) {
        state.activePage = page;
        document.querySelectorAll('.nav-tab').forEach(function(tab) {
            tab.classList.toggle('active', tab.dataset.page === page);
        });
        document.querySelectorAll('.page-content').forEach(function(p) {
            p.classList.toggle('active', p.id === 'page-' + page);
        });
        if (page === 'flows') resizeFlowCanvas();
    }

    // ======================================================================
    // WebSocket
    // ======================================================================
    function connectWebSocket() {
        if (state.ws && state.ws.readyState === WebSocket.OPEN) return;
        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var wsUrl = protocol + '//' + window.location.host + CONFIG.wsEndpoint;
        updateConnectionStatus('connecting');

        try {
            state.ws = new WebSocket(wsUrl);
            state.ws.onopen = function() {
                state.wsConnected = true;
                state.reconnectAttempts = 0;
                state.lastHeartbeat = Date.now();
                updateConnectionStatus('connected');
                showToast('success', 'Connected', 'WebSocket connection established');
            };
            state.ws.onclose = function() {
                state.wsConnected = false;
                updateConnectionStatus('disconnected');
                scheduleReconnect();
            };
            state.ws.onerror = function() {
                state.wsConnected = false;
                updateConnectionStatus('disconnected');
            };
            state.ws.onmessage = handleWebSocketMessage;
        } catch (e) {
            scheduleReconnect();
        }
    }

    function scheduleReconnect() {
        if (state.reconnectTimeout) return;
        var delay = Math.min(
            CONFIG.reconnectBaseDelay * Math.pow(CONFIG.reconnectMultiplier, state.reconnectAttempts),
            CONFIG.reconnectMaxDelay
        );
        state.reconnectAttempts++;
        state.reconnectTimeout = setTimeout(function() {
            state.reconnectTimeout = null;
            connectWebSocket();
        }, delay);
    }

    function handleWebSocketMessage(event) {
        state.lastHeartbeat = Date.now();
        try {
            var msg = JSON.parse(event.data);
            var type = msg.type || '';
            var data = msg.data || msg;
            // Map backend WS message types to handlers
            if (type === 'health' || type === 'health_update') updateHealthData(data);
            else if (type === 'services') updateServicesData(data);
            else if (type === 'metrics') updateMetricsData(data);
            else if (type === 'stats') updateStatsData(data);
            else if (type === 'flows' || type === 'packet_flow') {
                // packet_flow is a single flow event — wrap for updateFlowsData
                if (type === 'packet_flow') addFlowEvent(data);
                else updateFlowsData(data);
            }
            else if (type === 'event' || type === 'events') addEvent(data);
            else if (type.indexOf('ebpf_') === 0) addEBPFEvent(type, data);
        } catch (e) { /* ignore parse errors */ }
    }

    function updateConnectionStatus(status) {
        if (!el.wsIndicator) return;
        el.wsIndicator.className = 'connection-indicator ' + status;
        var labels = { connected: 'Connected', disconnected: 'Disconnected', connecting: 'Connecting...' };
        el.wsStatusText.textContent = labels[status] || status;
    }

    // ======================================================================
    // API Fetching
    // ======================================================================
    function fetchJSON(url, callback) {
        fetch(url).then(function(r) {
            if (!r.ok) throw new Error(r.status);
            return r.json();
        }).then(callback).catch(function() {});
    }

    function refreshHealth()     { fetchJSON(CONFIG.api.health,     updateHealthData); }
    function refreshServices()   { fetchJSON(CONFIG.api.services,   updateServicesData); }
    function refreshStats()      { fetchJSON(CONFIG.api.stats,      updateStatsData); }
    function refreshFlows()      { fetchJSON(CONFIG.api.flows,      updateFlowsData); }
    function refreshLatency()    { fetchJSON(CONFIG.api.latency,    updateLatencyData); }
    function refreshEBPFStats()  { fetchJSON(CONFIG.api.ebpfStats,  updateEBPFStats); }
    function refreshEBPFEvents() { fetchJSON(CONFIG.api.ebpfEvents, updateEBPFEvents); }

    // ======================================================================
    // Data Handlers
    // ======================================================================
    function updateHealthData(data) {
        state.systemHealth = data;
        // Backend returns healthy_count/degraded_count/unhealthy_count/total_services
        var h = data.healthy_count || data.healthy || 0;
        var d = data.degraded_count || data.degraded || 0;
        var u = data.unhealthy_count || data.unhealthy || 0;
        var total = data.total_services || data.total || (h + d + u);
        setText(el.totalServices, total);
        setText(el.healthyServices, h);
        setText(el.degradedServices, d);
        setText(el.unhealthyServices, u);
        updateTimestamp();
    }

    function updateServicesData(data) {
        var services = data.services || data;
        if (!Array.isArray(services)) return;
        state.services = {};
        services.forEach(function(s) { state.services[s.name] = s; });
        renderServicesGrid(services);
    }

    function updateStatsData(data) {
        state.stats = data;
        // Backend /api/v1/stats returns nested: {server:{ws_connections}, health:{healthy, ...}, scraper:{...}}
        // Extract what we can for gauges
        var h = data.health || {};
        var srv = data.server || {};
        // Use health ratio as a pseudo-CPU metric (% healthy)
        var totalSvc = h.total_services || 0;
        var healthySvc = h.healthy || 0;
        if (totalSvc > 0) state.gauges.cpu = (healthySvc / totalSvc) * 100;
        // Use scraper series count as pseudo-memory metric
        var sc = data.scraper || {};
        state.gauges.memory = Math.min((sc.series_count || 0) / 10, 100);
        // WebSocket connections for goroutines gauge
        state.gauges.goroutines = srv.ws_connections || 0;
        // Direct flat fields override nested (for WS messages with flat data)
        if (data.cpu !== undefined) state.gauges.cpu = data.cpu;
        if (data.memory !== undefined) state.gauges.memory = data.memory;
        if (data.goroutines !== undefined) state.gauges.goroutines = data.goroutines;
        updateGauges();
    }

    function updateFlowsData(data) {
        // Backend returns {source: "ebpf"|"synthetic", active_flows: [...], stats: {...}}
        var flows = data.active_flows || data.flows || data;
        if (!Array.isArray(flows)) flows = [];
        state.flows = flows;
        state.flowSource = data.source || 'unknown';
        var stats = data.stats || {};
        setText(el.flowGraphCount, flows.length);
        setText(el.activeFlowsCount, flows.length);
        setText(el.flowBytesSec, formatBytes(stats.bytes_per_sec || 0));
        setText(el.flowPktsSec, formatNumber(stats.packets_per_sec || 0));
        buildFlowNodes(flows);
        if (state.activePage === 'flows') {
            renderFlowGraph();
            renderFlowTable(flows);
        }
    }

    function updateLatencyData(data) {
        // Backend returns {percentiles: {...}, stats: {...}} or {message: "...", data: null}
        // Normalize: wrap percentiles as "operations" for rendering
        if (data.percentiles && !data.operations) {
            data.operations = data.percentiles;
        }
        state.latencyData = data;
        if (state.activePage === 'latency') {
            renderLatencySummary(data);
            renderLatencyCharts(data);
            renderLatencyHistory(data);
        }
    }

    function updateEBPFStats(data) {
        // Backend returns {active: bool, stats: {...}} or {active: false, message: "..."}
        var stats = data.stats || data;
        state.ebpfStats = stats;
        setText(el.statPackets, formatNumber(stats.packets_ingested || 0));
        setText(el.statFlows, formatNumber(stats.flows_tracked || 0));
        setText(el.statLatencySamples, formatNumber(stats.latency_samples || 0));
        setText(el.statErrors, formatNumber(stats.parse_errors || 0));
        setText(el.statCompute, formatNumber(stats.compute_ingested || 0));
        setText(el.statAnamnesis, formatNumber(stats.anamnesis_ingested || 0));
        setText(el.statEps, formatNumber(stats.events_per_sec || 0));
        setText(el.statUptime, formatUptime(stats.uptime_ms || 0));
        setText(el.ebpfEventsCount, formatNumber(stats.total_events || 0));
        // Update eBPF active indicator
        state.ebpfActive = data.active === true;
    }

    function updateEBPFEvents(data) {
        var events = data.events || data;
        if (!Array.isArray(events) || events.length === 0) return;
        // Normalize events for the event stream
        var normalized = events.map(function(ev) {
            return {
                type: ev.type || ev.event_type || 'packet',
                event_type: ev.type || ev.event_type || 'packet',
                topic: ev.topic || 'ebpf.events',
                data: ev,
                timestamp: ev.timestamp || new Date().toISOString(),
                summary: ev.message || ev.summary || JSON.stringify(ev).slice(0, 120)
            };
        });
        state.ebpfEvents = normalized.concat(state.ebpfEvents).slice(0, CONFIG.events.maxItems);
        state.eventStreamTotal += events.length;
        if (!state.eventStreamPaused && state.activePage === 'events') {
            appendEventStreamItems(normalized);
        }
    }

    function updateMetricsData(data) {
        if (data.request_rate !== undefined) {
            state.metricsHistory.requests.push(data.request_rate);
            if (state.metricsHistory.requests.length > CONFIG.charts.maxDataPoints)
                state.metricsHistory.requests.shift();
        }
        if (data.avg_latency !== undefined) {
            state.metricsHistory.latency.push(data.avg_latency);
            if (state.metricsHistory.latency.length > CONFIG.charts.maxDataPoints)
                state.metricsHistory.latency.shift();
        }
    }

    function addEvent(data) {
        state.events.unshift(data);
        if (state.events.length > CONFIG.events.maxItems) state.events.pop();
    }

    function addFlowEvent(flowData) {
        // Single flow event from WS — merge into state.flows
        if (!flowData) return;
        state.flows.push(flowData);
        if (state.flows.length > CONFIG.flow.maxFlows) state.flows.shift();
        buildFlowNodes(state.flows);
        setText(el.flowGraphCount, state.flows.length);
        setText(el.activeFlowsCount, state.flows.length);
        if (state.activePage === 'flows') {
            renderFlowGraph();
            renderFlowTable(state.flows);
        }
    }

    function addEBPFEvent(type, data) {
        // eBPF events from WS — add to event stream and update counters
        var evType = type.replace('ebpf_', '');
        var ev = {
            type: evType,
            event_type: evType,
            topic: 'ebpf.' + evType + '.events',
            data: data,
            timestamp: new Date().toISOString(),
            summary: formatEBPFSummary(evType, data)
        };
        state.ebpfEvents.unshift(ev);
        if (state.ebpfEvents.length > CONFIG.events.maxItems) state.ebpfEvents.pop();
        state.eventStreamTotal++;
        if (!state.eventStreamPaused && state.activePage === 'events') {
            appendEventStreamItems([ev]);
        }
    }

    function formatEBPFSummary(type, data) {
        if (!data) return type + ' event';
        if (type === 'packet') return (data.src_ip || '?') + ' → ' + (data.dst_ip || '?') + ' ' + (data.protocol || '');
        if (type === 'flow') return (data.src_ip || '?') + ':' + (data.src_port || '') + ' → ' + (data.dst_ip || '?') + ':' + (data.dst_port || '') + ' [' + (data.state || '') + ']';
        if (type === 'latency') return (data.operation || '?') + ' ' + (data.latency_us || 0) + 'μs';
        if (type === 'syscall') return (data.syscall || '?') + ' pid=' + (data.pid || 0);
        return type + ' event';
    }

    // ======================================================================
    // Overview — Service Health Grid
    // ======================================================================
    function renderServicesGrid(services) {
        if (!el.servicesGrid) return;
        el.servicesGrid.innerHTML = '';
        if (services.length === 0) {
            el.servicesGrid.innerHTML = '<div class="service-card placeholder"><div class="service-skeleton"></div></div>';
            return;
        }
        services.forEach(function(svc) {
            var status = (svc.status || 'unknown').toLowerCase();
            var card = document.createElement('div');
            card.className = 'service-card ' + status;
            // Backend returns average_latency_ms (or avg_latency_ms) and uptime_percent
            var latency = svc.average_latency_ms != null ? svc.average_latency_ms : svc.avg_latency_ms;
            var uptime = svc.uptime_percent != null ? svc.uptime_percent : null;
            card.innerHTML =
                '<div class="service-header">' +
                    '<span class="service-name">' + esc(svc.name) + '</span>' +
                    '<span class="service-status-badge ' + status + '">' + status + '</span>' +
                '</div>' +
                '<div class="service-metrics">' +
                    '<div class="service-metric">Uptime <span class="service-metric-value">' +
                        (uptime != null ? Number(uptime).toFixed(1) + '%' : '--') +
                    '</span></div>' +
                    '<div class="service-metric">Latency <span class="service-metric-value">' +
                        (latency != null ? Number(latency).toFixed(0) + 'ms' : '--') +
                    '</span></div>' +
                '</div>' +
                '<div class="service-uptime-bar"><div class="service-uptime-fill" style="width:' +
                    (uptime || 0) + '%"></div></div>';
            el.servicesGrid.appendChild(card);
        });
    }

    // ======================================================================
    // Overview — Gauges
    // ======================================================================
    function updateGauges() {
        drawGauge(el.cpuGauge, state.gauges.cpu / 100, gaugeColor(state.gauges.cpu));
        setText(el.cpuValue, state.gauges.cpu.toFixed(0) + '%');
        drawGauge(el.memoryGauge, state.gauges.memory / 100, gaugeColor(state.gauges.memory));
        setText(el.memoryValue, state.gauges.memory.toFixed(0) + '%');
        var gNorm = Math.min(state.gauges.goroutines / 500, 1);
        drawGauge(el.goroutinesGauge, gNorm, '#ffd700');
        setText(el.goroutinesValue, state.gauges.goroutines);
    }

    function drawGauge(canvas, fraction, color) {
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var w = canvas.width, h = canvas.height;
        var cx = w / 2, cy = h / 2, r = Math.min(w, h) / 2 - 10;
        var startA = 0.75 * Math.PI, endA = 2.25 * Math.PI;
        ctx.clearRect(0, 0, w, h);
        ctx.beginPath(); ctx.arc(cx, cy, r, startA, endA);
        ctx.strokeStyle = '#1e2746'; ctx.lineWidth = 8; ctx.lineCap = 'round'; ctx.stroke();
        var valA = startA + (endA - startA) * Math.max(0, Math.min(1, fraction));
        ctx.beginPath(); ctx.arc(cx, cy, r, startA, valA);
        ctx.strokeStyle = color; ctx.lineWidth = 8; ctx.lineCap = 'round'; ctx.stroke();
    }

    function gaugeColor(pct) {
        if (pct > 80) return '#ff4757';
        if (pct > 60) return '#ff9800';
        return '#00d26a';
    }

    // ======================================================================
    // Flow Graph — Canvas
    // ======================================================================
    function buildFlowNodes(flows) {
        var nodeMap = {};
        flows.forEach(function(f) {
            var src = (f.src_ip || f.source || '') + (f.src_port ? ':' + f.src_port : '');
            var dst = (f.dst_ip || f.destination || '') + (f.dst_port ? ':' + f.dst_port : '');
            if (src && !nodeMap[src]) nodeMap[src] = { id: src, label: src, conns: 0 };
            if (dst && !nodeMap[dst]) nodeMap[dst] = { id: dst, label: dst, conns: 0 };
            if (src) nodeMap[src].conns++;
            if (dst) nodeMap[dst].conns++;
        });
        state.flowNodes = Object.values(nodeMap).slice(0, CONFIG.flow.maxNodes);
    }

    function resizeFlowCanvas() {
        if (!el.flowCanvas) return;
        var wrapper = el.flowCanvas.parentElement;
        el.flowCanvas.width = wrapper.clientWidth;
        el.flowCanvas.height = wrapper.clientHeight || 500;
        renderFlowGraph();
    }

    function renderFlowGraph() {
        var canvas = el.flowCanvas;
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var W = canvas.width, H = canvas.height;
        ctx.clearRect(0, 0, W, H);

        var nodes = state.flowNodes;
        var flows = state.flows;
        if (nodes.length === 0) {
            ctx.fillStyle = '#6c757d'; ctx.font = '16px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            if (state.flowSource === 'synthetic') {
                ctx.fillText('Synthetic mode — start trace-collector for real eBPF flows', W / 2, H / 2 - 12);
                ctx.font = '12px -apple-system, sans-serif';
                ctx.fillText('Waiting for real network flow data...', W / 2, H / 2 + 12);
            } else {
                ctx.fillText('No active flows', W / 2, H / 2);
            }
            return;
        }

        // Circular layout
        var cx = W / 2, cy = H / 2, radius = Math.min(W, H) * 0.35;
        var pos = {};
        nodes.forEach(function(n, i) {
            var a = (2 * Math.PI * i / nodes.length) - Math.PI / 2;
            pos[n.id] = { x: cx + radius * Math.cos(a), y: cy + radius * Math.sin(a) };
        });

        // Edges
        flows.forEach(function(f) {
            var src = (f.src_ip || f.source || '') + (f.src_port ? ':' + f.src_port : '');
            var dst = (f.dst_ip || f.destination || '') + (f.dst_port ? ':' + f.dst_port : '');
            var p1 = pos[src], p2 = pos[dst];
            if (!p1 || !p2) return;

            var proto = (f.protocol || 'tcp').toLowerCase();
            var st = (f.state || '').toLowerCase();
            ctx.beginPath(); ctx.moveTo(p1.x, p1.y); ctx.lineTo(p2.x, p2.y);

            if (st === 'new') ctx.strokeStyle = '#ffd700';
            else if (st === 'closing' || st === 'closed') ctx.strokeStyle = '#6c757d';
            else if (st === 'error') ctx.strokeStyle = '#ff6b6b';
            else if (proto === 'udp') ctx.strokeStyle = '#00d26a';
            else ctx.strokeStyle = '#4ecdc4';

            var bytes = f.bytes || f.byte_count || 1;
            ctx.lineWidth = Math.max(1, Math.min(6, Math.log2(bytes / 100)));
            ctx.globalAlpha = 0.6; ctx.stroke(); ctx.globalAlpha = 1.0;
        });

        // Nodes
        nodes.forEach(function(n) {
            var p = pos[n.id]; if (!p) return;
            var r = Math.max(6, Math.min(16, 4 + n.conns * 2));
            ctx.beginPath(); ctx.arc(p.x, p.y, r, 0, 2 * Math.PI);
            ctx.fillStyle = '#1e2746'; ctx.fill();
            ctx.strokeStyle = '#ffd700'; ctx.lineWidth = 2; ctx.stroke();
            ctx.fillStyle = '#adb5bd'; ctx.font = '10px monospace'; ctx.textAlign = 'center';
            var label = n.label.length > 20 ? n.label.slice(0, 18) + '..' : n.label;
            ctx.fillText(label, p.x, p.y + r + 14);
        });
    }

    function renderFlowTable(flows) {
        if (!el.flowTableBody) return;
        var html = '';
        flows.slice(0, 50).forEach(function(f) {
            var src = (f.src_ip || f.source || '?') + (f.src_port ? ':' + f.src_port : '');
            var dst = (f.dst_ip || f.destination || '?') + (f.dst_port ? ':' + f.dst_port : '');
            html += '<tr>' +
                '<td>' + esc(src) + '</td>' +
                '<td>' + esc(dst) + '</td>' +
                '<td>' + esc((f.protocol || 'tcp').toUpperCase()) + '</td>' +
                '<td>' + formatNumber(f.packets || f.packet_count || 0) + '</td>' +
                '<td>' + formatBytes(f.bytes || f.byte_count || 0) + '</td>' +
                '<td>' + esc(f.state || 'active') + '</td>' +
                '<td>' + (f.age_ms ? formatDuration(f.age_ms) : '--') + '</td>' +
                '</tr>';
        });
        el.flowTableBody.innerHTML = html;
    }

    // ======================================================================
    // Latency Page
    // ======================================================================
    function renderLatencySummary(data) {
        if (!el.latencySummaryGrid) return;
        // Handle "not active" response
        if (data.message && !data.operations) {
            el.latencySummaryGrid.innerHTML =
                '<div class="latency-chart-card" style="grid-column:1/-1;text-align:center;color:var(--text-muted);padding:var(--spacing-xl)">' +
                '<p>' + esc(data.message) + '</p></div>';
            return;
        }
        var ops = data.operations || data;
        if (typeof ops !== 'object') return;
        var keys = Object.keys(ops);
        if (keys.length === 0) {
            el.latencySummaryGrid.innerHTML =
                '<div class="latency-chart-card" style="grid-column:1/-1;text-align:center;color:var(--text-muted);padding:var(--spacing-xl)">' +
                '<p>No latency data yet. Waiting for eBPF events...</p></div>';
            return;
        }
        var html = '';
        keys.forEach(function(name) {
            var op = ops[name];
            var u = op.unit || 'ms';
            html += '<div class="latency-chart-card">' +
                '<h3 class="chart-title">' + esc(name) + '</h3>' +
                '<div class="latency-percentiles">' +
                    '<span class="badge p50">P50: ' + (op.p50 || 0).toFixed(2) + u + '</span>' +
                    '<span class="badge p90">P90: ' + (op.p90 || 0).toFixed(2) + u + '</span>' +
                    '<span class="badge p99">P99: ' + (op.p99 || 0).toFixed(2) + u + '</span>' +
                '</div></div>';
        });
        el.latencySummaryGrid.innerHTML = html;
    }

    function renderLatencyCharts(data) {
        var ops = data.operations || data;
        if (typeof ops !== 'object') return;
        var mapping = {
            'tcp_connect': 'latency-chart-connect',
            'tcp_send': 'latency-chart-send',
            'tcp_recv': 'latency-chart-recv',
            'http_request': 'latency-chart-http'
        };
        Object.keys(mapping).forEach(function(opName) {
            var canvas = document.getElementById(mapping[opName]);
            if (!canvas) return;
            var op = ops[opName];
            if (!op) { drawEmpty(canvas, opName); return; }
            drawBarChart(canvas, op);
            var pEl = document.getElementById('percentiles-' + mapping[opName].split('-').pop());
            if (pEl) {
                var u = op.unit || 'ms';
                pEl.innerHTML =
                    '<span class="badge p50">P50: ' + (op.p50 || 0).toFixed(2) + u + '</span>' +
                    '<span class="badge p90">P90: ' + (op.p90 || 0).toFixed(2) + u + '</span>' +
                    '<span class="badge p99">P99: ' + (op.p99 || 0).toFixed(2) + u + '</span>';
            }
        });
    }

    function drawBarChart(canvas, op) {
        var ctx = canvas.getContext('2d');
        var W = canvas.width, H = canvas.height;
        ctx.clearRect(0, 0, W, H);

        var buckets = op.histogram || op.buckets;
        if (buckets && Array.isArray(buckets) && buckets.length > 0) {
            var maxC = Math.max.apply(null, buckets.map(function(b) { return b.count || 0; })) || 1;
            var bw = (W - 20) / buckets.length;
            buckets.forEach(function(b, i) {
                var bh = ((b.count || 0) / maxC) * (H - 35);
                ctx.fillStyle = '#4ecdc4';
                ctx.fillRect(10 + i * bw + 1, H - 25 - bh, bw - 2, bh);
            });
            return;
        }

        // Fallback: draw P50/P90/P99 as bars
        var vals = [
            { label: 'P50', value: op.p50 || 0, color: '#00d26a' },
            { label: 'P90', value: op.p90 || 0, color: '#ffd700' },
            { label: 'P99', value: op.p99 || 0, color: '#ff4757' }
        ];
        var maxV = Math.max.apply(null, vals.map(function(v) { return v.value; })) || 1;
        var bw = W / 7;
        vals.forEach(function(v, i) {
            var bh = (v.value / maxV) * (H - 50);
            var x = bw + i * bw * 2;
            ctx.fillStyle = v.color;
            ctx.fillRect(x, H - 30 - bh, bw, bh);
            ctx.fillStyle = '#f8f9fa'; ctx.font = '11px monospace'; ctx.textAlign = 'center';
            ctx.fillText(v.value.toFixed(1), x + bw / 2, H - 34 - bh);
            ctx.fillStyle = '#adb5bd';
            ctx.fillText(v.label, x + bw / 2, H - 8);
        });
    }

    function drawEmpty(canvas, label) {
        var ctx = canvas.getContext('2d');
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        ctx.fillStyle = '#6c757d'; ctx.font = '12px -apple-system, sans-serif';
        ctx.textAlign = 'center'; ctx.fillText('No data for ' + label, canvas.width / 2, canvas.height / 2);
    }

    function renderLatencyHistory(data) {
        if (!el.latencyHistoryCanvas) return;
        var ops = data.operations || data;
        if (typeof ops !== 'object') return;
        var avg = 0, cnt = 0;
        Object.keys(ops).forEach(function(k) { if (ops[k].p50) { avg += ops[k].p50; cnt++; } });
        if (cnt > 0) avg /= cnt;
        state.latencyHistory.push(avg);
        if (state.latencyHistory.length > CONFIG.charts.maxDataPoints) state.latencyHistory.shift();
        drawSparkline(el.latencyHistoryCanvas, state.latencyHistory, '#ffd700');
    }

    function drawSparkline(canvas, data, lineColor) {
        if (!canvas || data.length < 2) return;
        var ctx = canvas.getContext('2d');
        var W = canvas.width, H = canvas.height, pad = 20;
        ctx.clearRect(0, 0, W, H);
        var maxV = Math.max.apply(null, data) || 1;
        var minV = Math.min.apply(null, data);
        var range = maxV - minV || 1;

        // Grid
        ctx.strokeStyle = 'rgba(255,215,0,0.1)'; ctx.lineWidth = 1;
        for (var g = 0; g < 4; g++) {
            var gy = pad + g * ((H - 2 * pad) / 3);
            ctx.beginPath(); ctx.moveTo(0, gy); ctx.lineTo(W, gy); ctx.stroke();
        }

        // Line
        ctx.beginPath(); ctx.strokeStyle = lineColor; ctx.lineWidth = 2;
        data.forEach(function(v, i) {
            var x = (i / (data.length - 1)) * (W - 2 * pad) + pad;
            var y = H - pad - ((v - minV) / range) * (H - 2 * pad);
            if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        });
        ctx.stroke();

        ctx.fillStyle = '#adb5bd'; ctx.font = '10px monospace'; ctx.textAlign = 'right';
        ctx.fillText(maxV.toFixed(1) + 'ms', W - 4, pad + 10);
        ctx.fillText(minV.toFixed(1) + 'ms', W - 4, H - pad);
    }

    // ======================================================================
    // Event Stream Page
    // ======================================================================
    function setupEventStream() {
        if (el.eventPauseBtn) {
            el.eventPauseBtn.addEventListener('click', function() {
                state.eventStreamPaused = !state.eventStreamPaused;
                this.textContent = state.eventStreamPaused ? 'Resume' : 'Pause';
                this.classList.toggle('paused', state.eventStreamPaused);
            });
        }
        if (el.eventTopicInput) {
            el.eventTopicInput.addEventListener('input', function() {
                state.eventTopicFilter = this.value.trim();
            });
        }
        document.querySelectorAll('.event-type-filters button').forEach(function(btn) {
            btn.addEventListener('click', function() {
                document.querySelectorAll('.event-type-filters button').forEach(function(b) { b.classList.remove('active'); });
                this.classList.add('active');
                state.eventTypeFilter = this.dataset.type;
            });
        });
    }

    function appendEventStreamItems(events) {
        if (!el.eventStreamList) return;
        var filtered = events.filter(function(ev) {
            var type = ev.type || ev.event_type || '';
            if (state.eventTypeFilter !== 'all' && type !== state.eventTypeFilter) return false;
            if (state.eventTopicFilter) return matchTopic(state.eventTopicFilter, ev.topic || '');
            return true;
        });
        filtered.forEach(function(ev) {
            var type = ev.type || ev.event_type || 'packet';
            var time = ev.timestamp ? new Date(ev.timestamp).toLocaleTimeString() : new Date().toLocaleTimeString();
            var msg = ev.message || ev.summary || JSON.stringify(ev.data || ev).slice(0, 120);
            var item = document.createElement('div');
            item.className = 'event-stream-item type-' + type;
            item.innerHTML =
                '<span class="event-stream-time">' + time + '</span>' +
                '<span class="event-stream-type">' + esc(type) + '</span>' +
                '<span class="event-stream-message">' + esc(msg) + '</span>';
            el.eventStreamList.insertBefore(item, el.eventStreamList.firstChild);
        });
        while (el.eventStreamList.children.length > CONFIG.events.maxItems)
            el.eventStreamList.removeChild(el.eventStreamList.lastChild);
        setText(el.eventStreamTotal, state.eventStreamTotal);
        setText(el.eventStreamVisible, el.eventStreamList.children.length);
    }

    function matchTopic(pattern, topic) {
        if (!pattern || pattern === '*') return true;
        var pp = pattern.split('.'), tp = topic.split('.');
        for (var i = 0; i < pp.length; i++) {
            if (pp[i] === '#') return true;
            if (i >= tp.length) return false;
            if (pp[i] !== '*' && pp[i] !== tp[i]) return false;
        }
        return pp.length === tp.length;
    }

    // ======================================================================
    // Toast Notifications
    // ======================================================================
    function showToast(type, title, message, duration) {
        if (!el.toastContainer) return;
        duration = duration || 3000;
        var toast = document.createElement('div');
        toast.className = 'toast ' + type;
        var icons = { success: '&#x2705;', error: '&#x274C;', warning: '&#x26A0;', info: '&#x2139;' };
        toast.innerHTML =
            '<span class="toast-icon">' + (icons[type] || icons.info) + '</span>' +
            '<div class="toast-content">' +
                '<div class="toast-title">' + esc(title) + '</div>' +
                '<div class="toast-message">' + esc(message) + '</div>' +
            '</div>' +
            '<button class="toast-close" onclick="this.parentElement.remove()">&times;</button>';
        el.toastContainer.appendChild(toast);
        setTimeout(function() {
            toast.classList.add('hiding');
            setTimeout(function() { toast.remove(); }, 300);
        }, duration);
    }

    // ======================================================================
    // Utilities
    // ======================================================================
    function esc(text) {
        if (!text) return '';
        var d = document.createElement('div');
        d.textContent = String(text);
        return d.innerHTML;
    }

    function setText(element, value) { if (element) element.textContent = value; }

    function formatNumber(n) {
        if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
        if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
        return String(n);
    }

    function formatBytes(b) {
        if (b >= 1e9) return (b / 1e9).toFixed(1) + 'GB';
        if (b >= 1e6) return (b / 1e6).toFixed(1) + 'MB';
        if (b >= 1e3) return (b / 1e3).toFixed(1) + 'KB';
        return b + 'B';
    }

    function formatUptime(ms) {
        var s = Math.floor(ms / 1000), m = Math.floor(s / 60), h = Math.floor(m / 60), d = Math.floor(h / 24);
        if (d > 0) return d + 'd ' + (h % 24) + 'h';
        if (h > 0) return h + 'h ' + (m % 60) + 'm';
        if (m > 0) return m + 'm ' + (s % 60) + 's';
        return s + 's';
    }

    function formatDuration(ms) {
        if (ms >= 60000) return (ms / 60000).toFixed(0) + 'm';
        if (ms >= 1000) return (ms / 1000).toFixed(1) + 's';
        return ms + 'ms';
    }

    function updateTimestamp() {
        if (el.lastUpdate) el.lastUpdate.textContent = new Date().toLocaleTimeString();
        if (el.serverTime) el.serverTime.textContent = 'Server: ' + new Date().toLocaleTimeString();
        if (el.uptime) el.uptime.textContent = 'Uptime: ' + formatUptime(Date.now() - state.startTime);
    }

    // ======================================================================
    // Init
    // ======================================================================
    function init() {
        cacheElements();
        setupNavigation();
        setupEventStream();
        connectWebSocket();

        refreshHealth();
        refreshServices();
        refreshStats();
        refreshFlows();
        refreshLatency();
        refreshEBPFStats();
        refreshEBPFEvents();

        setInterval(refreshHealth, CONFIG.refreshIntervals.health);
        setInterval(refreshServices, CONFIG.refreshIntervals.services);
        setInterval(refreshStats, CONFIG.refreshIntervals.stats);
        setInterval(refreshFlows, CONFIG.refreshIntervals.flows);
        setInterval(refreshLatency, CONFIG.refreshIntervals.latency);
        setInterval(refreshEBPFStats, CONFIG.refreshIntervals.ebpfStats);
        setInterval(refreshEBPFEvents, CONFIG.refreshIntervals.ebpfEvents);
        setInterval(updateTimestamp, 1000);

        updateGauges();

        window.addEventListener('resize', function() {
            if (state.activePage === 'flows') resizeFlowCanvas();
        });

        // Event rate counter
        var lastTotal = 0;
        setInterval(function() {
            var rate = state.eventStreamTotal - lastTotal;
            lastTotal = state.eventStreamTotal;
            setText(el.eventStreamRate, rate);
        }, 1000);
    }

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    window.addEventListener('beforeunload', function() {
        if (state.ws) state.ws.close(1000, 'Page unload');
    });

})();
