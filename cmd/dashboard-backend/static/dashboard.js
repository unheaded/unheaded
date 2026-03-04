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
            ebpfEvents: '/api/v1/ebpf/events',
            hosts: '/api/v1/hosts'
        },

        refreshIntervals: {
            health: 15000,
            services: 10000,
            stats: 5000,
            flows: 3000,
            latency: 3000,
            ebpfStats: 5000,
            ebpfEvents: 2000,
            hosts: 5000
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
        hosts: [],
        selectedHost: 'west',

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

        el.hostSelector = document.getElementById('host-selector');
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

        el.sysDetailsSection = document.getElementById('section-system-details');
        el.sysHostInfo = document.getElementById('sys-host-info');
        el.sysLoad = document.getElementById('sys-load');
        el.sysSwap = document.getElementById('sys-swap');
        el.sysNetConns = document.getElementById('sys-netconns');
        el.sysProcesses = document.getElementById('sys-processes');
        el.sysUptime = document.getElementById('sys-uptime');
        el.sysDisks = document.getElementById('sys-disks');

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
        if (page === 'latency' && state.latencyData) {
            renderLatencySummary(state.latencyData);
            renderLatencyCharts(state.latencyData);
            renderLatencyHistory(state.latencyData);
        }
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
    function refreshHosts()      { fetchJSON(CONFIG.api.hosts,      updateHostsData); }

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
        // Gauges are now driven by /api/v1/hosts (refreshHosts), not stats.
        // Direct flat fields from WS messages can still override.
        if (data.cpu !== undefined) state.gauges.cpu = data.cpu;
        if (data.memory !== undefined) state.gauges.memory = data.memory;
        if (data.goroutines !== undefined) state.gauges.goroutines = data.goroutines;
        if (data.cpu !== undefined || data.memory !== undefined || data.goroutines !== undefined) {
            updateGauges();
        }
    }

    function updateHostsData(data) {
        var hosts = data.hosts || [];
        state.hosts = hosts;
        // Populate host selector dropdown
        if (el.hostSelector && hosts.length > 0) {
            var current = el.hostSelector.value || state.selectedHost;
            el.hostSelector.innerHTML = '';
            hosts.forEach(function(h) {
                var opt = document.createElement('option');
                opt.value = h.id;
                opt.textContent = h.id + (h.id === 'west' ? ' (local)' : '') + (h.online ? '' : ' [offline]');
                el.hostSelector.appendChild(opt);
            });
            el.hostSelector.value = current;
            if (!el.hostSelector.value && hosts.length > 0) {
                el.hostSelector.value = hosts[0].id;
            }
            state.selectedHost = el.hostSelector.value;
        }
        // Update gauges from selected host
        var selected = null;
        for (var i = 0; i < hosts.length; i++) {
            if (hosts[i].id === state.selectedHost) { selected = hosts[i]; break; }
        }
        if (selected && selected.online) {
            state.gauges.cpu = selected.cpu_percent || 0;
            state.gauges.memory = selected.memory_percent || 0;
            state.gauges.goroutines = selected.goroutines || 0;
        }
        updateGauges();
        renderSystemDetails(selected);
    }

    function renderSystemDetails(host) {
        if (!el.sysDetailsSection) return;
        if (!host || !host.online) {
            el.sysDetailsSection.style.display = 'none';
            return;
        }
        el.sysDetailsSection.style.display = '';

        // Host info
        var hostLines = host.hostname || host.id;
        if (host.kernel) hostLines += '\n' + host.kernel;
        if (host.addr) hostLines += '\n' + host.addr;
        setText(el.sysHostInfo, hostLines);

        // Load average
        var l1 = (host.load_1m || 0).toFixed(2);
        var l5 = (host.load_5m || 0).toFixed(2);
        var l15 = (host.load_15m || 0).toFixed(2);
        setText(el.sysLoad, l1 + ' / ' + l5 + ' / ' + l15 + '\n(1 / 5 / 15 min)');

        // Swap
        if (host.swap_total > 0) {
            var swapUsedH = formatBytesLong(host.swap_used || 0);
            var swapTotalH = formatBytesLong(host.swap_total);
            var swapPct = (host.swap_percent || 0).toFixed(1);
            setText(el.sysSwap, swapUsedH + ' / ' + swapTotalH + '\n' + swapPct + '% used');
        } else {
            setText(el.sysSwap, 'No swap');
        }

        // Network connections
        var nc = host.net_connections || {};
        setText(el.sysNetConns,
            'ESTAB: ' + (nc.established || 0) +
            '\nTIME_WAIT: ' + (nc.time_wait || 0) +
            '\nCLOSE_WAIT: ' + (nc.close_wait || 0));

        // Processes
        var zLabel = (host.process_zombie || 0) > 0 ? ' (' + host.process_zombie + ' zombie!)' : '';
        setText(el.sysProcesses, (host.process_total || 0) + ' total' + zLabel);

        // Uptime
        setText(el.sysUptime, formatUptime((host.uptime_seconds || 0) * 1000));

        // Disks
        if (el.sysDisks) {
            var disks = host.disks || [];
            if (disks.length === 0) {
                el.sysDisks.innerHTML = '<span style="color:#8b949e;font-size:0.75rem;">No disk data</span>';
            } else {
                var html = '';
                disks.forEach(function(d) {
                    var pct = (d.use_percent || 0).toFixed(1);
                    var color = pct > 89 ? '#ff4757' : pct > 75 ? '#ff9800' : '#00d26a';
                    html += '<div class="disk-bar-wrap">' +
                        '<span class="disk-mount">' + esc(d.mount) + '</span>' +
                        '<div class="disk-bar-track"><div class="disk-bar-fill" style="width:' + pct + '%;background:' + color + ';"></div></div>' +
                        '<span class="disk-pct">' + pct + '%</span>' +
                        '<span style="color:#8b949e;min-width:70px;">' + formatBytesLong(d.used_bytes || 0) + '/' + formatBytesLong(d.size_bytes || 0) + '</span>' +
                        '</div>';
                });
                el.sysDisks.innerHTML = html;
            }
        }
    }

    function formatBytesLong(b) {
        if (b >= 1e12) return (b / 1e12).toFixed(1) + 'T';
        if (b >= 1e9) return (b / 1e9).toFixed(1) + 'G';
        if (b >= 1e6) return (b / 1e6).toFixed(1) + 'M';
        if (b >= 1e3) return (b / 1e3).toFixed(1) + 'K';
        return b + 'B';
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
        // Each operation in percentiles is an ARRAY of window objects:
        //   [{window, sample_count, p50_ns, p90_ns, p99_ns, min_ns, max_ns, mean_ns}, ...]
        // The frontend expects a flat object per operation with p50/p90/p99 in ms.
        // Pick the 60s window (last entry) and convert ns → ms.
        if (data.percentiles && !data.operations) {
            var ops = {};
            Object.keys(data.percentiles).forEach(function(name) {
                var windows = data.percentiles[name];
                if (!Array.isArray(windows) || windows.length === 0) return;
                // Use the widest window (last = 60s)
                var w = windows[windows.length - 1];
                ops[name] = {
                    p50: (w.p50_ns || 0) / 1e6,
                    p90: (w.p90_ns || 0) / 1e6,
                    p99: (w.p99_ns || 0) / 1e6,
                    min: (w.min_ns || 0) / 1e6,
                    max: (w.max_ns || 0) / 1e6,
                    mean: (w.mean_ns || 0) / 1e6,
                    sample_count: w.sample_count || 0,
                    unit: 'ms'
                };
            });
            data.operations = ops;
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
        setText(el.statFlows, formatNumber(stats.flows_ingested || 0));
        setText(el.statLatencySamples, formatNumber(stats.latency_ingested || 0));
        setText(el.statErrors, formatNumber(stats.parse_errors || 0));
        setText(el.statCompute, formatNumber(stats.compute_ingested || 0));
        setText(el.statAnamnesis, formatNumber(stats.anamnesis_ingested || 0));
        // Total ingested = sum of all *_ingested counters
        var totalIngested = (stats.packets_ingested || 0) + (stats.flows_ingested || 0) +
            (stats.latency_ingested || 0) + (stats.syscall_ingested || 0) +
            (stats.compute_ingested || 0) + (stats.anamnesis_ingested || 0);
        // Events/sec: compute from total ingested over uptime
        var uptimeS = (Date.now() - state.startTime) / 1000;
        var eps = uptimeS > 0 ? (totalIngested / uptimeS).toFixed(1) : 0;
        setText(el.statEps, eps);
        setText(el.statUptime, formatUptime(Date.now() - state.startTime));
        setText(el.ebpfEventsCount, formatNumber(totalIngested));
        // Update active flows from flow_stats if available
        var fs = stats.flow_stats || {};
        if (fs.active_flows !== undefined) {
            setText(el.activeFlowsCount, formatNumber(fs.active_flows));
        }
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
    // Flow Graph — Canvas (real eBPF data from /api/v1/flows)
    // ======================================================================
    var HOST_LABELS = {
        '192.168.13.1': 'east', '192.168.13.2': 'west',
        '127.0.0.1': 'localhost', '::1': 'localhost'
    };
    var PORT_LABELS = {
        22: 'ssh', 18000: 'wotan-http', 18001: 'wotan-grpc',
        20000: 'dashboard', 16670: 'trace-col', 16671: 'trace-metrics',
        17000: 'daemon', 17001: 'daemon-grpc'
    };
    var STATE_COLORS = {
        established: '#4a7a4a', syn_sent: '#3a5a8a', syn_recv: '#3a5a8a',
        fin_wait1: '#7a6a3a', fin_wait2: '#7a6a3a', close_wait: '#7a5a3a',
        last_ack: '#7a5a3a', time_wait: '#555', closing: '#7a3a3a', closed: '#6c757d'
    };
    function stateColor(s) { return STATE_COLORS[s] || '#4ecdc4'; }
    function hostLabel(ip) { return HOST_LABELS[ip] || ip; }
    function portLabel(p) { return PORT_LABELS[p] || p; }

    // Extract flow key fields (handles nested API format)
    function fk(f) {
        var k = f.key || {};
        return {
            src: k.src_addr || f.src_ip || f.source || '',
            dst: k.dst_addr || f.dst_ip || f.destination || '',
            sp: k.src_port || f.src_port || 0,
            dp: k.dst_port || f.dst_port || 0,
            proto: k.protocol || f.protocol || 6
        };
    }
    function fBytes(f) { return (f.bytes_in || 0) + (f.bytes_out || 0) + (f.bytes || 0); }
    function fPkts(f) { return (f.packets_in || 0) + (f.packets_out || 0) + (f.packets || 0); }

    function buildFlowNodes(flows) {
        // Group by host IP (not port) for meaningful topology
        var nodeMap = {};
        flows.forEach(function(f) {
            var k = fk(f);
            if (k.src && !nodeMap[k.src]) nodeMap[k.src] = { id: k.src, label: hostLabel(k.src), conns: 0, bytes: 0, established: false };
            if (k.dst && !nodeMap[k.dst]) nodeMap[k.dst] = { id: k.dst, label: hostLabel(k.dst), conns: 0, bytes: 0, established: false };
            if (k.src) { nodeMap[k.src].conns++; nodeMap[k.src].bytes += fBytes(f); }
            if (k.dst) { nodeMap[k.dst].conns++; nodeMap[k.dst].bytes += fBytes(f); }
            if (f.state === 'established') {
                if (k.src) nodeMap[k.src].established = true;
                if (k.dst) nodeMap[k.dst].established = true;
            }
        });
        state.flowNodes = Object.values(nodeMap);
    }

    function resizeFlowCanvas() {
        if (!el.flowCanvas) return;
        var wrapper = el.flowCanvas.parentElement;
        el.flowCanvas.width = wrapper.clientWidth;
        el.flowCanvas.height = wrapper.clientHeight || 600;
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
            ctx.fillStyle = '#6c757d'; ctx.font = '14px monospace';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for eBPF flow data...', W / 2, H / 2);
            ctx.font = '11px monospace'; ctx.fillStyle = '#555';
            ctx.fillText('trace-collector -> wotan -> dashboard-backend', W / 2, H / 2 + 24);
            return;
        }

        // Aggregate edges by host-pair + destination port
        var edges = {};
        flows.forEach(function(f) {
            var k = fk(f);
            var ek = k.src + '->' + k.dst + ':' + k.dp;
            if (!edges[ek]) edges[ek] = { src: k.src, dst: k.dst, port: k.dp, count: 0, bytes: 0, pkts: 0, states: {} };
            var e = edges[ek];
            e.count++;
            e.bytes += fBytes(f);
            e.pkts += fPkts(f);
            e.states[f.state || 'unknown'] = (e.states[f.state || 'unknown'] || 0) + 1;
        });
        var edgeArr = Object.values(edges).sort(function(a,b) { return b.bytes - a.bytes; });
        var maxEdgeBytes = Math.max(1, edgeArr.length > 0 ? edgeArr[0].bytes : 1);

        // Layout: position nodes
        var cx = W / 2, cy = H / 2;
        var pos = {};
        if (nodes.length <= 2) {
            nodes.forEach(function(n, i) { pos[n.id] = { x: W * 0.25 + i * W * 0.5, y: cy }; });
        } else {
            var radius = Math.min(W, H) * 0.32;
            nodes.forEach(function(n, i) {
                var a = (2 * Math.PI * i / nodes.length) - Math.PI / 2;
                pos[n.id] = { x: cx + radius * Math.cos(a), y: cy + radius * Math.sin(a) };
            });
        }

        var nodeR = Math.max(28, Math.min(45, 40 - nodes.length));

        // Draw edges (grouped by host-pair, offset parallel edges by port)
        var pairOffsets = {};
        edgeArr.slice(0, 30).forEach(function(e) {
            var p1 = pos[e.src], p2 = pos[e.dst];
            if (!p1 || !p2) return;

            var pairKey = [e.src, e.dst].sort().join('<>');
            if (!pairOffsets[pairKey]) pairOffsets[pairKey] = 0;
            var off = pairOffsets[pairKey]++;

            // Dominant state
            var domState = '', domCount = 0;
            for (var s in e.states) { if (e.states[s] > domCount) { domState = s; domCount = e.states[s]; } }
            var color = stateColor(domState);
            var isActive = domState === 'established' || domState === 'syn_sent' || domState === 'syn_recv';

            // Edge width by bytes (1.5 - 7px)
            var thick = 1.5 + 5.5 * (e.bytes / maxEdgeBytes);

            // Perpendicular offset for parallel edges
            var dx = p2.x - p1.x, dy = p2.y - p1.y;
            var len = Math.sqrt(dx*dx + dy*dy) || 1;
            var nx = -dy/len, ny = dx/len;
            var shift = (off - 1) * 12;

            // Shorten to not overlap nodes
            var shrink = nodeR + 10;
            var r = shrink / len;
            var x1 = p1.x + dx * r + nx * shift;
            var y1 = p1.y + dy * r + ny * shift;
            var x2 = p2.x - dx * r + nx * shift;
            var y2 = p2.y - dy * r + ny * shift;

            // Draw line
            ctx.save();
            ctx.beginPath(); ctx.moveTo(x1, y1); ctx.lineTo(x2, y2);
            ctx.strokeStyle = color;
            ctx.lineWidth = thick;
            ctx.globalAlpha = isActive ? 0.9 : 0.45;
            if (isActive) ctx.setLineDash([8, 4]);
            ctx.stroke();
            ctx.restore();

            // Arrow
            var aLen = 8;
            var ax = dx / len, ay = dy / len;
            ctx.save();
            ctx.beginPath();
            ctx.moveTo(x2, y2);
            ctx.lineTo(x2 - aLen * ax + aLen * 0.4 * nx, y2 - aLen * ay + aLen * 0.4 * ny);
            ctx.lineTo(x2 - aLen * ax - aLen * 0.4 * nx, y2 - aLen * ay - aLen * 0.4 * ny);
            ctx.closePath();
            ctx.fillStyle = color;
            ctx.globalAlpha = isActive ? 0.9 : 0.5;
            ctx.fill();
            ctx.restore();

            // Edge label
            var mx = (x1 + x2) / 2, my = (y1 + y2) / 2;
            ctx.save();
            ctx.font = '10px monospace';
            ctx.textAlign = 'center';
            ctx.fillStyle = '#777';
            ctx.fillText(':' + portLabel(e.port) + ' (' + e.count + ')', mx, my - 5);
            ctx.restore();
        });

        // Draw nodes
        nodes.forEach(function(n) {
            var p = pos[n.id]; if (!p) return;

            // Glow for active hosts
            if (n.established) {
                ctx.save();
                ctx.beginPath(); ctx.arc(p.x, p.y, nodeR + 6, 0, 2 * Math.PI);
                ctx.fillStyle = 'rgba(74,122,74,0.15)';
                ctx.fill();
                ctx.restore();
            }

            // Node circle
            ctx.beginPath(); ctx.arc(p.x, p.y, nodeR, 0, 2 * Math.PI);
            ctx.fillStyle = '#111'; ctx.fill();
            ctx.strokeStyle = n.established ? '#4a7a4a' : '#333';
            ctx.lineWidth = n.established ? 2.5 : 1.5;
            ctx.stroke();

            // Host label
            ctx.fillStyle = '#ddd'; ctx.font = 'bold 13px monospace';
            ctx.textAlign = 'center'; ctx.textBaseline = 'middle';
            ctx.fillText(n.label, p.x, p.y - 5);

            // IP sublabel
            if (n.label !== n.id) {
                ctx.fillStyle = '#555'; ctx.font = '9px monospace';
                ctx.fillText(n.id, p.x, p.y + 9);
            }

            // Flow count below node
            ctx.fillStyle = '#666'; ctx.font = '10px monospace';
            ctx.fillText(n.conns + ' flows | ' + formatBytes(n.bytes), p.x, p.y + nodeR + 14);
        });

        // Source indicator
        ctx.save();
        ctx.font = '10px monospace'; ctx.fillStyle = '#444'; ctx.textAlign = 'left';
        ctx.fillText('source: ' + state.flowSource + ' | ' + flows.length + ' active flows', 10, H - 10);
        ctx.restore();
    }

    function renderFlowTable(flows) {
        if (!el.flowTableBody) return;
        // Sort by bytes descending
        var sorted = flows.slice().sort(function(a,b) { return fBytes(b) - fBytes(a); });
        var html = '';
        sorted.slice(0, 50).forEach(function(f) {
            var k = fk(f);
            var st = f.state || 'unknown';
            var age = '--';
            if (f.last_seen) {
                var s = Math.floor((Date.now() - new Date(f.last_seen).getTime()) / 1000);
                age = s < 5 ? 'now' : s < 60 ? s + 's' : Math.floor(s/60) + 'm';
            }
            html += '<tr>' +
                '<td>' + esc(k.src + ':' + k.sp) + '</td>' +
                '<td>' + esc(k.dst + ':' + portLabel(k.dp)) + '</td>' +
                '<td>' + esc(k.proto === 6 ? 'TCP' : k.proto === 17 ? 'UDP' : 'P' + k.proto) + '</td>' +
                '<td>' + formatNumber(fPkts(f)) + '</td>' +
                '<td>' + formatBytes(fBytes(f)) + '</td>' +
                '<td style="color:' + stateColor(st) + '">' + esc(st) + '</td>' +
                '<td>' + age + '</td>' +
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
            'tcp_accept': 'latency-chart-http'
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

        // Host selector change handler
        if (el.hostSelector) {
            el.hostSelector.addEventListener('change', function() {
                state.selectedHost = this.value;
                // Re-apply gauges from cached host data
                updateHostsData({ hosts: state.hosts });
            });
        }

        refreshHealth();
        refreshServices();
        refreshStats();
        refreshFlows();
        refreshLatency();
        refreshEBPFStats();
        refreshEBPFEvents();
        refreshHosts();

        setInterval(refreshHealth, CONFIG.refreshIntervals.health);
        setInterval(refreshServices, CONFIG.refreshIntervals.services);
        setInterval(refreshStats, CONFIG.refreshIntervals.stats);
        setInterval(refreshFlows, CONFIG.refreshIntervals.flows);
        setInterval(refreshLatency, CONFIG.refreshIntervals.latency);
        setInterval(refreshEBPFStats, CONFIG.refreshIntervals.ebpfStats);
        setInterval(refreshEBPFEvents, CONFIG.refreshIntervals.ebpfEvents);
        setInterval(refreshHosts, CONFIG.refreshIntervals.hosts);
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
