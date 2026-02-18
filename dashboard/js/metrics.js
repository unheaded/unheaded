// System metrics display and management
// Fetches and displays dashboard-backend metrics with periodic polling.
//
// Data sources:
//   - GET /api/v1/metrics   (dashboard-backend, port 8080) - aggregated metrics
//   - GET /api/v1/health    (dashboard-backend, port 8080) - service health
//   - GET /api/v1/services  (dashboard-backend, port 8080) - service list with status
//
// Fallback: shows "--" / "No data" when the backend is unreachable, retains
// last-known values for a configurable number of failures before clearing.

(function() {
    'use strict';

    // Configuration
    var CONFIG = {
        // Base URL for the dashboard-backend REST API (port 8080)
        apiBaseUrl: '/api/v1',
        refreshInterval: 5000,      // Poll every 5 seconds
        animationDuration: 300,
        retryDelay: 3000,
        maxRetries: 3,
        // Number of consecutive failures before clearing last-known values
        staleThreshold: 6
    };

    // State
    var metrics = null;
    var lastKnownMetrics = null;
    var services = [];
    var refreshTimer = null;
    var retryCount = 0;
    var isConnected = false;
    var consecutiveFailures = 0;

    // DOM elements
    var metricsGrid = null;
    var statusIndicator = null;

    // Initialize metrics module
    function init() {
        metricsGrid = document.getElementById('metrics-grid');
        if (!metricsGrid) {
            console.error('metrics.js: #metrics-grid not found');
            return;
        }

        // Determine API base URL from globals or current page
        if (window.METRICS_API_BASE_URL) {
            CONFIG.apiBaseUrl = window.METRICS_API_BASE_URL;
        } else if (window.location.port !== '8080') {
            // If the dashboard is served from a different port, target port 8080 explicitly
            CONFIG.apiBaseUrl = window.location.protocol + '//' + window.location.hostname + ':8080/api/v1';
        }

        createStatusIndicator();
        createMetricsLayout();
        showLoading();
        startPolling();
    }

    // Create status indicator
    function createStatusIndicator() {
        statusIndicator = document.createElement('div');
        statusIndicator.className = 'metrics-status connecting';
        statusIndicator.id = 'metrics-status';
        statusIndicator.innerHTML =
            '<span class="status-dot"></span>' +
            '<span class="status-text">Connecting to metrics API...</span>';

        var metricsSection = document.getElementById('metrics');
        if (metricsSection) {
            var header = metricsSection.querySelector('h2');
            if (header) {
                header.appendChild(statusIndicator);
            }
        }
    }

    // Create metrics layout
    function createMetricsLayout() {
        metricsGrid.innerHTML =
            '<div class="metrics-row primary-metrics">' +
                '<div class="metric-card" id="metric-request-rate" role="group" aria-label="Request Rate metric">' +
                    '<div class="metric-icon" aria-hidden="true">' +
                        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
                            '<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>' +
                        '</svg>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="requestRate" aria-live="polite">--</span>' +
                        '<span class="metric-unit">req/s</span>' +
                    '</div>' +
                    '<span class="metric-label">Request Rate</span>' +
                '</div>' +

                '<div class="metric-card" id="metric-error-rate" role="group" aria-label="Error Rate metric">' +
                    '<div class="metric-icon error" aria-hidden="true">' +
                        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
                            '<circle cx="12" cy="12" r="10"></circle>' +
                            '<line x1="15" y1="9" x2="9" y2="15"></line>' +
                            '<line x1="9" y1="9" x2="15" y2="15"></line>' +
                        '</svg>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="errorRate" aria-live="polite">--</span>' +
                        '<span class="metric-unit">%</span>' +
                    '</div>' +
                    '<span class="metric-label">Error Rate</span>' +
                '</div>' +

                '<div class="metric-card" id="metric-connections" role="group" aria-label="Active Connections metric">' +
                    '<div class="metric-icon" aria-hidden="true">' +
                        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
                            '<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path>' +
                            '<circle cx="9" cy="7" r="4"></circle>' +
                            '<path d="M23 21v-2a4 4 0 0 0-3-3.87"></path>' +
                            '<path d="M16 3.13a4 4 0 0 1 0 7.75"></path>' +
                        '</svg>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="activeConnections" aria-live="polite">--</span>' +
                    '</div>' +
                    '<span class="metric-label">Active Connections</span>' +
                '</div>' +

                '<div class="metric-card" id="metric-uptime" role="group" aria-label="Uptime metric">' +
                    '<div class="metric-icon success" aria-hidden="true">' +
                        '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
                            '<circle cx="12" cy="12" r="10"></circle>' +
                            '<polyline points="12 6 12 12 16 14"></polyline>' +
                        '</svg>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="uptime" aria-live="polite">--</span>' +
                    '</div>' +
                    '<span class="metric-label">Uptime</span>' +
                '</div>' +
            '</div>' +

            '<div class="metrics-row latency-metrics">' +
                '<div class="metric-card latency" id="metric-latency-p50">' +
                    '<div class="metric-header">' +
                        '<span class="metric-label">Latency P50</span>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="latencyP50">--</span>' +
                        '<span class="metric-unit">ms</span>' +
                    '</div>' +
                    '<div class="latency-bar">' +
                        '<div class="latency-fill p50" data-bar="p50"></div>' +
                    '</div>' +
                '</div>' +

                '<div class="metric-card latency" id="metric-latency-p95">' +
                    '<div class="metric-header">' +
                        '<span class="metric-label">Latency P95</span>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="latencyP95">--</span>' +
                        '<span class="metric-unit">ms</span>' +
                    '</div>' +
                    '<div class="latency-bar">' +
                        '<div class="latency-fill p95" data-bar="p95"></div>' +
                    '</div>' +
                '</div>' +

                '<div class="metric-card latency" id="metric-latency-p99">' +
                    '<div class="metric-header">' +
                        '<span class="metric-label">Latency P99</span>' +
                    '</div>' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="latencyP99">--</span>' +
                        '<span class="metric-unit">ms</span>' +
                    '</div>' +
                    '<div class="latency-bar">' +
                        '<div class="latency-fill p99" data-bar="p99"></div>' +
                    '</div>' +
                '</div>' +
            '</div>' +

            '<div class="metrics-row bpf-metrics">' +
                '<div class="metric-card" id="metric-bpf-flows">' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="bpfFlowRate">--</span>' +
                    '</div>' +
                    '<span class="metric-label">BPF Flows</span>' +
                '</div>' +
                '<div class="metric-card" id="metric-bpf-events">' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="bpfEventRate">--</span>' +
                    '</div>' +
                    '<span class="metric-label">BPF Events</span>' +
                '</div>' +
                '<div class="metric-card" id="metric-bpf-anomalies">' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="bpfAnomalyCount">--</span>' +
                    '</div>' +
                    '<span class="metric-label">Anomalies</span>' +
                '</div>' +
                '<div class="metric-card" id="metric-bpf-latency">' +
                    '<div class="metric-content">' +
                        '<span class="metric-value" data-metric="bpfLatency">--</span>' +
                    '</div>' +
                    '<span class="metric-label">BPF Latency</span>' +
                '</div>' +
            '</div>' +

            '<div class="metrics-row services-health">' +
                '<div class="services-header">' +
                    '<h3>Service Health</h3>' +
                    '<span class="services-count" id="services-count">0 services</span>' +
                '</div>' +
                '<div class="services-grid" id="services-grid">' +
                    '<!-- Services will be rendered here -->' +
                '</div>' +
            '</div>';
    }

    // Show loading skeleton state
    function showLoading() {
        var valueElements = metricsGrid.querySelectorAll('.metric-value');
        valueElements.forEach(function(el) {
            el.classList.add('loading');
            el.setAttribute('aria-busy', 'true');
            el.textContent = '';
            // Add skeleton placeholder
            var skeleton = document.createElement('span');
            skeleton.className = 'skeleton skeleton-value';
            skeleton.setAttribute('aria-hidden', 'true');
            el.appendChild(skeleton);
        });

        var servicesGrid = document.getElementById('services-grid');
        if (servicesGrid) {
            servicesGrid.setAttribute('aria-busy', 'true');
            var skeletonHTML = '';
            for (var i = 0; i < 4; i++) {
                skeletonHTML += '<div class="skeleton-service">' +
                    '<div class="skeleton skeleton-line medium"></div>' +
                    '<div class="skeleton skeleton-line short"></div>' +
                '</div>';
            }
            servicesGrid.innerHTML = skeletonHTML;
        }
    }

    // Update status indicator
    function updateStatus(status, text) {
        if (!statusIndicator) return;

        statusIndicator.className = 'metrics-status ' + status;
        statusIndicator.querySelector('.status-text').textContent = text;
        isConnected = status === 'connected';
    }

    // Start polling for metrics
    function startPolling() {
        fetchMetrics();
        refreshTimer = setInterval(fetchMetrics, CONFIG.refreshInterval);
    }

    // Stop polling
    function stopPolling() {
        if (refreshTimer) {
            clearInterval(refreshTimer);
            refreshTimer = null;
        }
    }

    // Fetch metrics from the real dashboard-backend API
    function fetchMetrics() {
        // Fetch metrics, health, and services in parallel
        var metricsUrl = CONFIG.apiBaseUrl + '/metrics';
        var healthUrl  = CONFIG.apiBaseUrl + '/health';
        var servicesUrl = CONFIG.apiBaseUrl + '/services';

        Promise.allSettled([
            fetch(metricsUrl, { method: 'GET', headers: { 'Accept': 'application/json' } }),
            fetch(healthUrl,  { method: 'GET', headers: { 'Accept': 'application/json' } }),
            fetch(servicesUrl, { method: 'GET', headers: { 'Accept': 'application/json' } })
        ]).then(function(results) {
            var metricsResp = results[0];
            var healthResp  = results[1];
            var servicesResp = results[2];

            // We need at least the metrics endpoint to succeed
            if (metricsResp.status !== 'fulfilled' || !metricsResp.value.ok) {
                throw new Error('Metrics endpoint unavailable');
            }

            return metricsResp.value.json().then(function(metricsData) {
                var data = metricsData;

                // Parse health response if available
                var healthPromise;
                if (healthResp.status === 'fulfilled' && healthResp.value.ok) {
                    healthPromise = healthResp.value.json();
                } else {
                    healthPromise = Promise.resolve(null);
                }

                // Parse services response if available
                var servicesPromise;
                if (servicesResp.status === 'fulfilled' && servicesResp.value.ok) {
                    servicesPromise = servicesResp.value.json();
                } else {
                    servicesPromise = Promise.resolve(null);
                }

                return Promise.all([healthPromise, servicesPromise]).then(function(extras) {
                    var healthData = extras[0];
                    var servicesData = extras[1];

                    // Merge service health data from /api/v1/health
                    if (healthData && healthData.services && !data.services) {
                        data.services = {};
                        var svcNames = Object.keys(healthData.services);
                        for (var i = 0; i < svcNames.length; i++) {
                            var name = svcNames[i];
                            var info = healthData.services[name];
                            data.services[name] = {
                                name: name,
                                status: info.status || 'unknown',
                                latency: info.average_latency ? parseInt(info.average_latency) / 1000000 : null
                            };
                        }
                    }

                    // Merge richer service data from /api/v1/services
                    if (servicesData && servicesData.services && Array.isArray(servicesData.services)) {
                        data._servicesList = servicesData.services;
                    }

                    // Extract aggregate metrics from the AggregatedMetrics structure
                    // The /api/v1/metrics endpoint returns:
                    // { timestamp, services: { name: { metrics: { ... } } }, total_metrics, total_series }
                    data = extractAggregateValues(data);

                    retryCount = 0;
                    consecutiveFailures = 0;
                    updateStatus('connected', 'Live');
                    processMetrics(data);
                });
            });
        }).catch(function(error) {
            handleFetchError(error);
        });
    }

    /**
     * Extract meaningful aggregate values from the AggregatedMetrics structure.
     *
     * The dashboard-backend /api/v1/metrics returns:
     * {
     *   "timestamp": "...",
     *   "services": {
     *     "timeguru": { "name":"timeguru", "status":"up", "metrics": { "unheaded_http_requests_total": { "value": 42 }, ... } },
     *     ...
     *   },
     *   "total_metrics": 120,
     *   "total_series": 45
     * }
     *
     * We aggregate request rates, error rates, latencies, and connection counts
     * from the per-service Prometheus metrics.
     */
    function extractAggregateValues(data) {
        if (!data || !data.services) return data;

        var totalRequests = 0;
        var totalErrors = 0;
        var latencies = [];
        var activeConns = 0;
        var uptime = 0;
        var serviceList = [];

        var svcNames = Object.keys(data.services);
        for (var i = 0; i < svcNames.length; i++) {
            var svcName = svcNames[i];
            var svc = data.services[svcName];
            var svcMetrics = svc.metrics || {};

            // Build service list for health display
            serviceList.push({
                name: svcName,
                status: svc.status || 'unknown',
                latency: null
            });

            // Sum up HTTP request totals across services
            var reqMetric = svcMetrics['unheaded_http_requests_total'] ||
                            svcMetrics['http_requests_total'];
            if (reqMetric) {
                totalRequests += reqMetric.value || 0;
            }

            // Sum up errors (requests with status >= 400)
            // The counter is usually broken out by labels; if we have a single value,
            // use error_count metrics if available
            var errMetric = svcMetrics['unheaded_http_errors_total'] ||
                            svcMetrics['http_errors_total'];
            if (errMetric) {
                totalErrors += errMetric.value || 0;
            }

            // Collect latency values
            var latMetric = svcMetrics['unheaded_http_request_duration_seconds'] ||
                            svcMetrics['http_request_duration_seconds'];
            if (latMetric && latMetric.value) {
                // Convert seconds to milliseconds
                latencies.push(latMetric.value * 1000);
            }

            // Active connections gauge
            var connMetric = svcMetrics['unheaded_active_connections'] ||
                             svcMetrics['active_connections'] ||
                             svcMetrics['dashboard_websocket_connections'];
            if (connMetric) {
                activeConns += connMetric.value || 0;
            }

            // Uptime - take the maximum across services
            var uptimeMetric = svcMetrics['unheaded_uptime_seconds'] ||
                               svcMetrics['process_uptime_seconds'] ||
                               svcMetrics['uptime_seconds'];
            if (uptimeMetric && uptimeMetric.value > uptime) {
                uptime = uptimeMetric.value;
            }
        }

        // Calculate derived values
        // request_rate is approximated from total requests / scrape interval
        var requestRate = totalRequests > 0 ? totalRequests / (CONFIG.refreshInterval / 1000) : 0;
        var errorRate = totalRequests > 0 ? (totalErrors / totalRequests) * 100 : 0;

        // Sort latencies for percentile calculation
        latencies.sort(function(a, b) { return a - b; });
        var p50 = percentile(latencies, 0.50);
        var p95 = percentile(latencies, 0.95);
        var p99 = percentile(latencies, 0.99);

        // Prefer the richer services list from /api/v1/services if available
        if (data._servicesList && data._servicesList.length > 0) {
            serviceList = data._servicesList;
        }

        // Overlay extracted values onto data for processMetrics
        data.request_rate = data.request_rate || requestRate;
        data.error_rate = data.error_rate || errorRate;
        data.active_connections = data.active_connections || activeConns;
        data.uptime = data.uptime || data.uptime_seconds || uptime;
        if (!data.latency) {
            data.latency = { p50: p50, p95: p95, p99: p99 };
        }
        if (!data.services_list) {
            data.services_list = serviceList;
        }

        return data;
    }

    /** Calculate a percentile from a sorted array. */
    function percentile(sortedArr, p) {
        if (!sortedArr || sortedArr.length === 0) return 0;
        var idx = Math.ceil(p * sortedArr.length) - 1;
        if (idx < 0) idx = 0;
        return sortedArr[idx];
    }

    // Handle fetch errors - display user-friendly messages, show last-known values
    function handleFetchError(error) {
        console.error('metrics.js: Failed to fetch metrics:', error.message || error);

        retryCount++;
        consecutiveFailures++;

        if (retryCount <= CONFIG.maxRetries) {
            updateStatus('reconnecting', 'Reconnecting (' + retryCount + '/' + CONFIG.maxRetries + ')...');
        } else {
            updateStatus('disconnected', 'Disconnected');
        }

        // If we have last-known metrics and haven't exceeded the stale threshold,
        // keep displaying them with a "stale" indicator
        if (lastKnownMetrics && consecutiveFailures <= CONFIG.staleThreshold) {
            showStaleMetrics();
            return;
        }

        // Show user-friendly error state on metrics
        var friendlyMessage = getFriendlyErrorMessage(error);
        var valueElements = metricsGrid.querySelectorAll('.metric-value');
        valueElements.forEach(function(el) {
            el.classList.add('error');
            el.setAttribute('aria-invalid', 'true');
            el.textContent = '--';
        });

        // Show error state in services grid
        var servicesGrid = document.getElementById('services-grid');
        if (servicesGrid && retryCount > CONFIG.maxRetries) {
            servicesGrid.innerHTML = '<div class="error-state">' +
                '<div class="error-state-icon" aria-hidden="true">&#x26A0;</div>' +
                '<div class="error-state-title">Unable to load services</div>' +
                '<div class="error-state-message">' + escapeHtml(friendlyMessage) + '</div>' +
                '<button class="error-state-action" onclick="window.DashboardMetrics.refresh()" aria-label="Retry loading metrics">' +
                    'Retry' +
                '</button>' +
            '</div>';
        }
    }

    /** Show last-known values with a stale indicator. */
    function showStaleMetrics() {
        if (!lastKnownMetrics) return;

        // Re-render with last known, but add stale class
        processMetrics(lastKnownMetrics, true);
    }

    // Convert raw errors to user-friendly messages
    function getFriendlyErrorMessage(error) {
        var msg = error.message || String(error);
        if (msg.includes('Failed to fetch') || msg.includes('NetworkError')) {
            return 'Cannot reach the metrics server. Please check your network connection.';
        }
        if (msg.includes('timeout') || msg.includes('Timeout')) {
            return 'The server took too long to respond. It may be under heavy load.';
        }
        if (msg.includes('500') || msg.includes('Internal Server')) {
            return 'The server encountered an internal error. The team has been notified.';
        }
        if (msg.includes('503') || msg.includes('Service Unavailable')) {
            return 'The metrics service is temporarily unavailable. Please try again shortly.';
        }
        if (msg.includes('401') || msg.includes('403') || msg.includes('Unauthorized') || msg.includes('Forbidden')) {
            return 'Authentication required. Please check your credentials.';
        }
        if (msg.includes('unavailable')) {
            return 'The metrics endpoint is not available. The service may be starting up.';
        }
        return 'An unexpected error occurred while loading metrics. Please try again.';
    }

    // Process and update metrics
    function processMetrics(data, isStale) {
        var previousMetrics = metrics;
        metrics = normalizeMetrics(data);

        // Store as last-known for fallback
        if (!isStale) {
            lastKnownMetrics = data;
        }

        updateMetricValue('requestRate', formatNumber(metrics.requestRate, 1), previousMetrics ? previousMetrics.requestRate : null);
        updateMetricValue('errorRate', formatNumber(metrics.errorRate, 2), previousMetrics ? previousMetrics.errorRate : null);
        updateMetricValue('activeConnections', formatNumber(metrics.activeConnections, 0), previousMetrics ? previousMetrics.activeConnections : null);
        updateMetricValue('uptime', formatUptime(metrics.uptime), null);

        // Latency metrics
        var prevLat = previousMetrics ? previousMetrics.latency : null;
        updateMetricValue('latencyP50', formatNumber(metrics.latency ? metrics.latency.p50 : 0, 1), prevLat ? prevLat.p50 : null);
        updateMetricValue('latencyP95', formatNumber(metrics.latency ? metrics.latency.p95 : 0, 1), prevLat ? prevLat.p95 : null);
        updateMetricValue('latencyP99', formatNumber(metrics.latency ? metrics.latency.p99 : 0, 1), prevLat ? prevLat.p99 : null);

        // Update latency bars
        updateLatencyBars(metrics.latency);

        // BPF event metrics (from packet-flow.js state, if available)
        var pf = window.PacketFlow;
        if (pf && pf.getState) {
            var pfState = pf.getState();
            updateMetricValue('bpfFlowRate', formatNumber(pfState.bpfFlowCount || 0, 0), null);
            updateMetricValue('bpfEventRate', formatNumber(pfState.bpfEventCount || 0, 0), null);
            updateMetricValue('bpfAnomalyCount', formatNumber(pfState.bpfAnomalyCount || 0, 0), null);
            if (pfState.bpfLastLatencyNs) {
                updateMetricValue('bpfLatency', formatNumber(pfState.bpfLastLatencyNs / 1000, 1) + 'µs', null);
            }
        }

        // Update services from the services_list or services object
        var serviceData = data.services_list || data.services;
        if (serviceData) {
            updateServices(serviceData);
        }

        // Clear loading/error states and ARIA busy
        var valueElements = metricsGrid.querySelectorAll('.metric-value');
        valueElements.forEach(function(el) {
            el.classList.remove('loading', 'error');
            el.removeAttribute('aria-busy');
            el.removeAttribute('aria-invalid');
            // Remove any skeleton placeholders
            var skel = el.querySelector('.skeleton');
            if (skel) skel.remove();

            // Add stale indicator if showing last-known data
            if (isStale) {
                el.classList.add('stale');
            } else {
                el.classList.remove('stale');
            }
        });

        var servicesGridEl = document.getElementById('services-grid');
        if (servicesGridEl) {
            servicesGridEl.removeAttribute('aria-busy');
        }
    }

    // Normalize metrics data from various possible response formats
    function normalizeMetrics(data) {
        return {
            requestRate: data.request_rate || data.requestRate || 0,
            errorRate: data.error_rate || data.errorRate || 0,
            activeConnections: data.active_connections || data.activeConnections || 0,
            uptime: data.uptime || data.uptime_seconds || 0,
            latency: {
                p50: (data.latency ? data.latency.p50 : null) || data.latency_p50 || 0,
                p95: (data.latency ? data.latency.p95 : null) || data.latency_p95 || 0,
                p99: (data.latency ? data.latency.p99 : null) || data.latency_p99 || 0
            }
        };
    }

    // Update a single metric value with animation
    function updateMetricValue(metricKey, newValue, previousValue) {
        var element = metricsGrid.querySelector('[data-metric="' + metricKey + '"]');
        if (!element) return;

        var currentValue = element.textContent;
        if (currentValue === newValue) return;

        // Animate the change
        element.classList.add('updating');

        // Determine direction for animation
        if (previousValue !== null && previousValue !== undefined) {
            var numCurrent = parseFloat(previousValue);
            var numNew = parseFloat(newValue);
            if (!isNaN(numCurrent) && !isNaN(numNew)) {
                if (numNew > numCurrent) {
                    element.classList.add('increasing');
                } else if (numNew < numCurrent) {
                    element.classList.add('decreasing');
                }
            }
        }

        element.textContent = newValue;

        setTimeout(function() {
            element.classList.remove('updating', 'increasing', 'decreasing');
        }, CONFIG.animationDuration);
    }

    // Update latency bars
    function updateLatencyBars(latency) {
        if (!latency) return;

        // Calculate max for scaling (use p99 as baseline, minimum 100ms)
        var maxLatency = Math.max(latency.p99 || 100, 100);

        var bars = {
            p50: latency.p50 || 0,
            p95: latency.p95 || 0,
            p99: latency.p99 || 0
        };

        var barKeys = Object.keys(bars);
        for (var i = 0; i < barKeys.length; i++) {
            var key = barKeys[i];
            var value = bars[key];
            var bar = metricsGrid.querySelector('[data-bar="' + key + '"]');
            if (bar) {
                var pct = Math.min((value / maxLatency) * 100, 100);
                bar.style.width = pct + '%';

                // Color coding based on latency thresholds
                bar.classList.remove('good', 'warning', 'critical');
                if (value < 50) {
                    bar.classList.add('good');
                } else if (value < 100) {
                    bar.classList.add('warning');
                } else {
                    bar.classList.add('critical');
                }
            }
        }
    }

    // Update services grid
    function updateServices(serviceData) {
        var servicesGrid = document.getElementById('services-grid');
        var servicesCount = document.getElementById('services-count');
        if (!servicesGrid) return;

        // Normalize service data - handle both array and object formats
        var serviceList;
        if (Array.isArray(serviceData)) {
            serviceList = serviceData;
        } else {
            serviceList = [];
            var names = Object.keys(serviceData);
            for (var i = 0; i < names.length; i++) {
                var name = names[i];
                var status = serviceData[name];
                serviceList.push({
                    name: name,
                    status: typeof status === 'object' ? status.status : status,
                    latency: typeof status === 'object' ? status.latency : null,
                    uptime: typeof status === 'object' ? status.uptime : null,
                    average_latency_ms: typeof status === 'object' ? status.average_latency_ms : null
                });
            }
        }

        services = serviceList;

        // Update count
        if (servicesCount) {
            var healthyCount = serviceList.filter(function(s) { return isHealthy(s.status); }).length;
            servicesCount.textContent = healthyCount + '/' + serviceList.length + ' healthy';
        }

        // Render services
        var html = '';
        for (var j = 0; j < serviceList.length; j++) {
            html += createServiceCard(serviceList[j]);
        }
        servicesGrid.innerHTML = html;
    }

    // Create service card HTML
    function createServiceCard(service) {
        var status = normalizeServiceStatus(service.status);
        var statusClass = getStatusClass(status);
        var statusIcon = getStatusIcon(status);

        // Use average_latency_ms from /api/v1/services if available
        var latencyValue = service.average_latency_ms || service.latency;

        return '<div class="service-card ' + statusClass + '" data-service="' + escapeHtml(service.name) + '">' +
            '<div class="service-header">' +
                '<span class="service-name">' + escapeHtml(service.name) + '</span>' +
                '<span class="service-status ' + statusClass + '">' +
                    statusIcon +
                    escapeHtml(status) +
                '</span>' +
            '</div>' +
            (latencyValue ? '<div class="service-meta">' +
                '<span class="service-latency">' + formatNumber(latencyValue, 1) + 'ms</span>' +
            '</div>' : '') +
        '</div>';
    }

    // Normalize service status
    function normalizeServiceStatus(status) {
        if (!status) return 'unknown';
        var s = String(status).toLowerCase();
        if (s === 'healthy' || s === 'ok' || s === 'up' || s === 'running') return 'healthy';
        if (s === 'degraded' || s === 'warning') return 'degraded';
        if (s === 'unhealthy' || s === 'down' || s === 'error' || s === 'failed') return 'unhealthy';
        return 'unknown';
    }

    // Check if service is healthy
    function isHealthy(status) {
        return normalizeServiceStatus(status) === 'healthy';
    }

    // Get CSS class for status
    function getStatusClass(status) {
        var classes = {
            healthy: 'status-healthy',
            degraded: 'status-degraded',
            unhealthy: 'status-unhealthy',
            unknown: 'status-unknown'
        };
        return classes[status] || 'status-unknown';
    }

    // Get icon for status
    function getStatusIcon(status) {
        var icons = {
            healthy: '<svg class="status-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="20 6 9 17 4 12"></polyline></svg>',
            degraded: '<svg class="status-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>',
            unhealthy: '<svg class="status-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="15" y1="9" x2="9" y2="15"></line><line x1="9" y1="9" x2="15" y2="15"></line></svg>',
            unknown: '<svg class="status-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><line x1="12" y1="16" x2="12" y2="12"></line><line x1="12" y1="8" x2="12.01" y2="8"></line></svg>'
        };
        return icons[status] || icons.unknown;
    }

    // Format number with specified decimal places
    function formatNumber(value, decimals) {
        if (value === null || value === undefined || isNaN(value)) return '--';
        var num = parseFloat(value);
        if (num >= 1000000) {
            return (num / 1000000).toFixed(1) + 'M';
        }
        if (num >= 1000) {
            return (num / 1000).toFixed(1) + 'K';
        }
        return num.toFixed(decimals);
    }

    // Format uptime duration
    function formatUptime(seconds) {
        if (!seconds || isNaN(seconds)) return '--';

        var days = Math.floor(seconds / 86400);
        var hours = Math.floor((seconds % 86400) / 3600);
        var minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return days + 'd ' + hours + 'h';
        }
        if (hours > 0) {
            return hours + 'h ' + minutes + 'm';
        }
        return minutes + 'm';
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        if (!text) return '';
        var div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Force refresh metrics
    function refresh() {
        retryCount = 0;
        consecutiveFailures = 0;
        fetchMetrics();
    }

    // Get current metrics
    function getMetrics() {
        if (!metrics) return null;
        return {
            requestRate: metrics.requestRate,
            errorRate: metrics.errorRate,
            activeConnections: metrics.activeConnections,
            uptime: metrics.uptime,
            latency: metrics.latency ? {
                p50: metrics.latency.p50,
                p95: metrics.latency.p95,
                p99: metrics.latency.p99
            } : null
        };
    }

    // Get services list
    function getServices() {
        return services.slice();
    }

    // Check connection status
    function isConnectedStatus() {
        return isConnected;
    }

    // Set custom API base URL
    function setApiBaseUrl(url) {
        CONFIG.apiBaseUrl = url;
    }

    // Set refresh interval
    function setRefreshInterval(interval) {
        CONFIG.refreshInterval = interval;
        if (refreshTimer) {
            stopPolling();
            startPolling();
        }
    }

    // Export for external use
    window.DashboardMetrics = {
        init: init,
        refresh: refresh,
        startPolling: startPolling,
        stopPolling: stopPolling,
        getMetrics: getMetrics,
        getServices: getServices,
        isConnected: isConnectedStatus,
        setApiBaseUrl: setApiBaseUrl,
        setRefreshInterval: setRefreshInterval
    };

    // Auto-init when DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('metrics.js loaded (wired to real dashboard-backend endpoints)');
})();
