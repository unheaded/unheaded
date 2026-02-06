// System metrics display and management
// Fetches and displays dashboard-backend metrics with animations

(function() {
    'use strict';

    // Configuration
    const CONFIG = {
        apiBaseUrl: '/api/v1',
        refreshInterval: 5000,
        animationDuration: 300,
        retryDelay: 3000,
        maxRetries: 3
    };

    // State
    let metrics = null;
    let services = [];
    let refreshTimer = null;
    let retryCount = 0;
    let isConnected = false;

    // DOM elements
    let metricsGrid = null;
    let statusIndicator = null;

    // Initialize metrics module
    function init() {
        metricsGrid = document.getElementById('metrics-grid');
        if (!metricsGrid) {
            console.error('metrics.js: #metrics-grid not found');
            return;
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
        statusIndicator.innerHTML = `
            <span class="status-dot"></span>
            <span class="status-text">Connecting to metrics API...</span>
        `;

        const metricsSection = document.getElementById('metrics');
        if (metricsSection) {
            const header = metricsSection.querySelector('h2');
            if (header) {
                header.appendChild(statusIndicator);
            }
        }
    }

    // Create metrics layout
    function createMetricsLayout() {
        metricsGrid.innerHTML = `
            <div class="metrics-row primary-metrics">
                <div class="metric-card" id="metric-request-rate" role="group" aria-label="Request Rate metric">
                    <div class="metric-icon" aria-hidden="true">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>
                        </svg>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="requestRate" aria-live="polite">--</span>
                        <span class="metric-unit">req/s</span>
                    </div>
                    <span class="metric-label">Request Rate</span>
                </div>

                <div class="metric-card" id="metric-error-rate" role="group" aria-label="Error Rate metric">
                    <div class="metric-icon error" aria-hidden="true">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <circle cx="12" cy="12" r="10"></circle>
                            <line x1="15" y1="9" x2="9" y2="15"></line>
                            <line x1="9" y1="9" x2="15" y2="15"></line>
                        </svg>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="errorRate" aria-live="polite">--</span>
                        <span class="metric-unit">%</span>
                    </div>
                    <span class="metric-label">Error Rate</span>
                </div>

                <div class="metric-card" id="metric-connections" role="group" aria-label="Active Connections metric">
                    <div class="metric-icon" aria-hidden="true">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"></path>
                            <circle cx="9" cy="7" r="4"></circle>
                            <path d="M23 21v-2a4 4 0 0 0-3-3.87"></path>
                            <path d="M16 3.13a4 4 0 0 1 0 7.75"></path>
                        </svg>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="activeConnections" aria-live="polite">--</span>
                    </div>
                    <span class="metric-label">Active Connections</span>
                </div>

                <div class="metric-card" id="metric-uptime" role="group" aria-label="Uptime metric">
                    <div class="metric-icon success" aria-hidden="true">
                        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                            <circle cx="12" cy="12" r="10"></circle>
                            <polyline points="12 6 12 12 16 14"></polyline>
                        </svg>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="uptime" aria-live="polite">--</span>
                    </div>
                    <span class="metric-label">Uptime</span>
                </div>
            </div>

            <div class="metrics-row latency-metrics">
                <div class="metric-card latency" id="metric-latency-p50">
                    <div class="metric-header">
                        <span class="metric-label">Latency P50</span>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="latencyP50">--</span>
                        <span class="metric-unit">ms</span>
                    </div>
                    <div class="latency-bar">
                        <div class="latency-fill p50" data-bar="p50"></div>
                    </div>
                </div>

                <div class="metric-card latency" id="metric-latency-p95">
                    <div class="metric-header">
                        <span class="metric-label">Latency P95</span>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="latencyP95">--</span>
                        <span class="metric-unit">ms</span>
                    </div>
                    <div class="latency-bar">
                        <div class="latency-fill p95" data-bar="p95"></div>
                    </div>
                </div>

                <div class="metric-card latency" id="metric-latency-p99">
                    <div class="metric-header">
                        <span class="metric-label">Latency P99</span>
                    </div>
                    <div class="metric-content">
                        <span class="metric-value" data-metric="latencyP99">--</span>
                        <span class="metric-unit">ms</span>
                    </div>
                    <div class="latency-bar">
                        <div class="latency-fill p99" data-bar="p99"></div>
                    </div>
                </div>
            </div>

            <div class="metrics-row services-health">
                <div class="services-header">
                    <h3>Service Health</h3>
                    <span class="services-count" id="services-count">0 services</span>
                </div>
                <div class="services-grid" id="services-grid">
                    <!-- Services will be rendered here -->
                </div>
            </div>
        `;
    }

    // Show loading skeleton state
    function showLoading() {
        const valueElements = metricsGrid.querySelectorAll('.metric-value');
        valueElements.forEach(el => {
            el.classList.add('loading');
            el.setAttribute('aria-busy', 'true');
            el.textContent = '';
            // Add skeleton placeholder
            const skeleton = document.createElement('span');
            skeleton.className = 'skeleton skeleton-value';
            skeleton.setAttribute('aria-hidden', 'true');
            el.appendChild(skeleton);
        });

        const servicesGrid = document.getElementById('services-grid');
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

        statusIndicator.className = `metrics-status ${status}`;
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

    // Fetch metrics from API
    async function fetchMetrics() {
        try {
            // Fetch metrics and health in parallel
            const [metricsResp, healthResp] = await Promise.allSettled([
                fetch(`${CONFIG.apiBaseUrl}/metrics`, {
                    method: 'GET',
                    headers: { 'Accept': 'application/json' }
                }),
                fetch(`${CONFIG.apiBaseUrl}/health`, {
                    method: 'GET',
                    headers: { 'Accept': 'application/json' }
                })
            ]);

            let data = {};

            if (metricsResp.status === 'fulfilled' && metricsResp.value.ok) {
                data = await metricsResp.value.json();
            } else {
                throw new Error('Metrics endpoint unavailable');
            }

            // Merge service health data if available
            if (healthResp.status === 'fulfilled' && healthResp.value.ok) {
                const healthData = await healthResp.value.json();
                if (healthData.services && !data.services) {
                    data.services = Object.entries(healthData.services).map(function([name, info]) {
                        return {
                            name: name,
                            status: info.status || 'unknown',
                            latency: info.average_latency ? parseInt(info.average_latency) / 1000000 : null
                        };
                    });
                }
            }

            retryCount = 0;
            updateStatus('connected', 'Live');
            processMetrics(data);

        } catch (error) {
            handleFetchError(error);
        }
    }

    // Handle fetch errors - display user-friendly messages
    function handleFetchError(error) {
        console.error('metrics.js: Failed to fetch metrics:', error.message);

        retryCount++;
        if (retryCount <= CONFIG.maxRetries) {
            updateStatus('reconnecting', 'Reconnecting (' + retryCount + '/' + CONFIG.maxRetries + ')...');
        } else {
            updateStatus('disconnected', 'Disconnected');
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
    function processMetrics(data) {
        const previousMetrics = metrics;
        metrics = normalizeMetrics(data);

        updateMetricValue('requestRate', formatNumber(metrics.requestRate, 1), previousMetrics?.requestRate);
        updateMetricValue('errorRate', formatNumber(metrics.errorRate, 2), previousMetrics?.errorRate);
        updateMetricValue('activeConnections', formatNumber(metrics.activeConnections, 0), previousMetrics?.activeConnections);
        updateMetricValue('uptime', formatUptime(metrics.uptime), null);

        // Latency metrics
        updateMetricValue('latencyP50', formatNumber(metrics.latency?.p50 || 0, 1), previousMetrics?.latency?.p50);
        updateMetricValue('latencyP95', formatNumber(metrics.latency?.p95 || 0, 1), previousMetrics?.latency?.p95);
        updateMetricValue('latencyP99', formatNumber(metrics.latency?.p99 || 0, 1), previousMetrics?.latency?.p99);

        // Update latency bars
        updateLatencyBars(metrics.latency);

        // Update services
        if (data.services) {
            updateServices(data.services);
        }

        // Clear loading/error states and ARIA busy
        const valueElements = metricsGrid.querySelectorAll('.metric-value');
        valueElements.forEach(el => {
            el.classList.remove('loading', 'error');
            el.removeAttribute('aria-busy');
            el.removeAttribute('aria-invalid');
            // Remove any skeleton placeholders
            var skel = el.querySelector('.skeleton');
            if (skel) skel.remove();
        });

        var servicesGridEl = document.getElementById('services-grid');
        if (servicesGridEl) {
            servicesGridEl.removeAttribute('aria-busy');
        }
    }

    // Normalize metrics data
    function normalizeMetrics(data) {
        return {
            requestRate: data.request_rate || data.requestRate || 0,
            errorRate: data.error_rate || data.errorRate || 0,
            activeConnections: data.active_connections || data.activeConnections || 0,
            uptime: data.uptime || data.uptime_seconds || 0,
            latency: {
                p50: data.latency?.p50 || data.latency_p50 || 0,
                p95: data.latency?.p95 || data.latency_p95 || 0,
                p99: data.latency?.p99 || data.latency_p99 || 0
            }
        };
    }

    // Update a single metric value with animation
    function updateMetricValue(metricKey, newValue, previousValue) {
        const element = metricsGrid.querySelector(`[data-metric="${metricKey}"]`);
        if (!element) return;

        const currentValue = element.textContent;
        if (currentValue === newValue) return;

        // Animate the change
        element.classList.add('updating');

        // Determine direction for animation
        if (previousValue !== null && previousValue !== undefined) {
            const numCurrent = parseFloat(previousValue);
            const numNew = parseFloat(newValue);
            if (!isNaN(numCurrent) && !isNaN(numNew)) {
                if (numNew > numCurrent) {
                    element.classList.add('increasing');
                } else if (numNew < numCurrent) {
                    element.classList.add('decreasing');
                }
            }
        }

        element.textContent = newValue;

        setTimeout(() => {
            element.classList.remove('updating', 'increasing', 'decreasing');
        }, CONFIG.animationDuration);
    }

    // Update latency bars
    function updateLatencyBars(latency) {
        if (!latency) return;

        // Calculate max for scaling (use p99 as baseline, minimum 100ms)
        const maxLatency = Math.max(latency.p99 || 100, 100);

        const bars = {
            p50: latency.p50 || 0,
            p95: latency.p95 || 0,
            p99: latency.p99 || 0
        };

        Object.entries(bars).forEach(([key, value]) => {
            const bar = metricsGrid.querySelector(`[data-bar="${key}"]`);
            if (bar) {
                const percentage = Math.min((value / maxLatency) * 100, 100);
                bar.style.width = `${percentage}%`;

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
        });
    }

    // Update services grid
    function updateServices(serviceData) {
        const servicesGrid = document.getElementById('services-grid');
        const servicesCount = document.getElementById('services-count');
        if (!servicesGrid) return;

        // Normalize service data
        const serviceList = Array.isArray(serviceData) ? serviceData : Object.entries(serviceData).map(([name, status]) => ({
            name,
            status: typeof status === 'object' ? status.status : status,
            latency: typeof status === 'object' ? status.latency : null,
            uptime: typeof status === 'object' ? status.uptime : null
        }));

        services = serviceList;

        // Update count
        if (servicesCount) {
            const healthyCount = serviceList.filter(s => isHealthy(s.status)).length;
            servicesCount.textContent = `${healthyCount}/${serviceList.length} healthy`;
        }

        // Render services
        servicesGrid.innerHTML = serviceList.map(service => createServiceCard(service)).join('');
    }

    // Create service card HTML
    function createServiceCard(service) {
        const status = normalizeServiceStatus(service.status);
        const statusClass = getStatusClass(status);
        const statusIcon = getStatusIcon(status);

        return `
            <div class="service-card ${statusClass}" data-service="${escapeHtml(service.name)}">
                <div class="service-header">
                    <span class="service-name">${escapeHtml(service.name)}</span>
                    <span class="service-status ${statusClass}">
                        ${statusIcon}
                        ${escapeHtml(status)}
                    </span>
                </div>
                ${service.latency ? `
                <div class="service-meta">
                    <span class="service-latency">${formatNumber(service.latency, 1)}ms</span>
                </div>
                ` : ''}
            </div>
        `;
    }

    // Normalize service status
    function normalizeServiceStatus(status) {
        if (!status) return 'unknown';
        const s = String(status).toLowerCase();
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
        const classes = {
            healthy: 'status-healthy',
            degraded: 'status-degraded',
            unhealthy: 'status-unhealthy',
            unknown: 'status-unknown'
        };
        return classes[status] || 'status-unknown';
    }

    // Get icon for status
    function getStatusIcon(status) {
        const icons = {
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
        const num = parseFloat(value);
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

        const days = Math.floor(seconds / 86400);
        const hours = Math.floor((seconds % 86400) / 3600);
        const minutes = Math.floor((seconds % 3600) / 60);

        if (days > 0) {
            return `${days}d ${hours}h`;
        }
        if (hours > 0) {
            return `${hours}h ${minutes}m`;
        }
        return `${minutes}m`;
    }

    // Escape HTML to prevent XSS
    function escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // Force refresh metrics
    function refresh() {
        retryCount = 0;
        fetchMetrics();
    }

    // Get current metrics
    function getMetrics() {
        return metrics ? { ...metrics } : null;
    }

    // Get services list
    function getServices() {
        return [...services];
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
        init,
        refresh,
        startPolling,
        stopPolling,
        getMetrics,
        getServices,
        isConnected: isConnectedStatus,
        setApiBaseUrl,
        setRefreshInterval
    };

    // Auto-init when DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('metrics.js loaded');
})();
