/**
 * trace-viewer.js - Real-time Trace Table
 * Live-updating table of recent trace events received via WebSocket.
 *
 * Features:
 *   - Columns: Timestamp, Source, Destination, Protocol, RTT, Flow State, Trace ID
 *   - Click-to-expand: full trace detail in a drawer/popup
 *   - Filter controls: by source, destination, protocol, RTT threshold
 *   - Sort by any column
 *   - Max 200 rows displayed (auto-scroll)
 *   - Color-coded rows: green (<1ms RTT), yellow (<10ms), red (>10ms)
 *
 * Data source: WebSocket messages with type "trace" from dashboard-backend.
 * Falls back to synthetic data from PacketFlowViz.recentFlows when available.
 *
 * No external dependencies. Vanilla JS only.
 */

var TraceViewer = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxRows: 200,
        autoScroll: true,
        defaultSortColumn: 'timestamp',
        defaultSortOrder: 'desc',
        rttThresholds: {
            good: 1,       // < 1ms -> green
            warning: 10    // < 10ms -> yellow, >= 10ms -> red
        },
        dateFormat: {
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit',
            fractionalSecondDigits: 3
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        tableBody: null,
        detailDrawer: null,
        traces: [],            // All stored trace events
        filteredTraces: [],    // After filters applied
        filters: {
            source: '',
            destination: '',
            protocol: '',
            rttThreshold: 0
        },
        sort: {
            column: CONFIG.defaultSortColumn,
            order: CONFIG.defaultSortOrder
        },
        selectedTrace: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(containerId) {
        var container = document.getElementById(containerId || 'trace-viewer-container');
        if (!container) {
            console.error('[TraceViewer] Container not found:', containerId);
            return false;
        }

        state.container = container;
        buildUI(container);
        state.initialized = true;
        console.log('[TraceViewer] Initialized');
        return true;
    }

    function buildUI(container) {
        container.innerHTML = '';

        // Filter bar
        var filterBar = document.createElement('div');
        filterBar.className = 'trace-filter-bar';
        filterBar.setAttribute('role', 'search');
        filterBar.setAttribute('aria-label', 'Trace event filters');
        filterBar.innerHTML =
            '<div class="trace-filter-group">' +
                '<label for="trace-filter-source">Source</label>' +
                '<input type="text" id="trace-filter-source" class="trace-filter-input" placeholder="e.g. gateway" autocomplete="off">' +
            '</div>' +
            '<div class="trace-filter-group">' +
                '<label for="trace-filter-dest">Destination</label>' +
                '<input type="text" id="trace-filter-dest" class="trace-filter-input" placeholder="e.g. wotan" autocomplete="off">' +
            '</div>' +
            '<div class="trace-filter-group">' +
                '<label for="trace-filter-protocol">Protocol</label>' +
                '<select id="trace-filter-protocol" class="trace-filter-select">' +
                    '<option value="">All</option>' +
                    '<option value="HTTP">HTTP</option>' +
                    '<option value="gRPC">gRPC</option>' +
                    '<option value="BPF">BPF</option>' +
                    '<option value="WS">WebSocket</option>' +
                '</select>' +
            '</div>' +
            '<div class="trace-filter-group">' +
                '<label for="trace-filter-rtt">Max RTT (ms)</label>' +
                '<input type="number" id="trace-filter-rtt" class="trace-filter-input trace-filter-rtt" placeholder="0" min="0" step="0.1">' +
            '</div>' +
            '<button class="trace-filter-clear" id="trace-filter-clear" aria-label="Clear all filters">Clear</button>' +
            '<div class="trace-count" id="trace-count" aria-live="polite">0 traces</div>';
        container.appendChild(filterBar);

        // Table wrapper
        var tableWrapper = document.createElement('div');
        tableWrapper.className = 'trace-table-wrapper';
        tableWrapper.id = 'trace-table-wrapper';

        var table = document.createElement('table');
        table.className = 'trace-table';
        table.setAttribute('role', 'grid');
        table.setAttribute('aria-label', 'Trace events');

        // Table header
        var thead = document.createElement('thead');
        thead.innerHTML =
            '<tr>' +
                '<th class="trace-th sortable" data-column="timestamp" aria-sort="descending">' +
                    'Timestamp <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="source">' +
                    'Source <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="destination">' +
                    'Destination <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="protocol">' +
                    'Protocol <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="rtt">' +
                    'RTT <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="flowState">' +
                    'Flow State <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
                '<th class="trace-th sortable" data-column="traceId">' +
                    'Trace ID <span class="sort-indicator" aria-hidden="true"></span>' +
                '</th>' +
            '</tr>';
        table.appendChild(thead);

        // Table body
        var tbody = document.createElement('tbody');
        tbody.id = 'trace-table-body';
        table.appendChild(tbody);
        state.tableBody = tbody;

        tableWrapper.appendChild(table);
        container.appendChild(tableWrapper);

        // Detail drawer (hidden by default)
        var drawer = document.createElement('div');
        drawer.className = 'trace-detail-drawer';
        drawer.id = 'trace-detail-drawer';
        drawer.setAttribute('role', 'dialog');
        drawer.setAttribute('aria-label', 'Trace detail');
        drawer.setAttribute('aria-hidden', 'true');
        drawer.innerHTML =
            '<div class="trace-detail-header">' +
                '<h3 class="trace-detail-title">Trace Detail</h3>' +
                '<button class="trace-detail-close" id="trace-detail-close" aria-label="Close detail panel">&times;</button>' +
            '</div>' +
            '<div class="trace-detail-body" id="trace-detail-body"></div>';
        container.appendChild(drawer);
        state.detailDrawer = drawer;

        // Empty state
        var emptyState = document.createElement('div');
        emptyState.className = 'trace-empty-state';
        emptyState.id = 'trace-empty-state';
        emptyState.innerHTML =
            '<div class="empty-state-icon" aria-hidden="true">&#x1F50D;</div>' +
            '<div class="empty-state-title">No trace events yet</div>' +
            '<div class="empty-state-description">Trace events will appear here as they flow through the system.</div>';
        tableWrapper.appendChild(emptyState);

        // Wire up event handlers
        bindEvents(container);
    }

    function bindEvents(container) {
        // Filter inputs
        var sourceInput = document.getElementById('trace-filter-source');
        var destInput = document.getElementById('trace-filter-dest');
        var protocolSelect = document.getElementById('trace-filter-protocol');
        var rttInput = document.getElementById('trace-filter-rtt');
        var clearBtn = document.getElementById('trace-filter-clear');

        if (sourceInput) {
            sourceInput.addEventListener('input', function() {
                state.filters.source = this.value.toLowerCase().trim();
                applyFiltersAndRender();
            });
        }
        if (destInput) {
            destInput.addEventListener('input', function() {
                state.filters.destination = this.value.toLowerCase().trim();
                applyFiltersAndRender();
            });
        }
        if (protocolSelect) {
            protocolSelect.addEventListener('change', function() {
                state.filters.protocol = this.value;
                applyFiltersAndRender();
            });
        }
        if (rttInput) {
            rttInput.addEventListener('input', function() {
                state.filters.rttThreshold = parseFloat(this.value) || 0;
                applyFiltersAndRender();
            });
        }
        if (clearBtn) {
            clearBtn.addEventListener('click', function() {
                state.filters = { source: '', destination: '', protocol: '', rttThreshold: 0 };
                if (sourceInput) sourceInput.value = '';
                if (destInput) destInput.value = '';
                if (protocolSelect) protocolSelect.value = '';
                if (rttInput) rttInput.value = '';
                applyFiltersAndRender();
            });
        }

        // Sort headers
        var headers = container.querySelectorAll('.trace-th.sortable');
        for (var i = 0; i < headers.length; i++) {
            headers[i].addEventListener('click', handleSortClick);
        }

        // Detail drawer close
        var closeBtn = document.getElementById('trace-detail-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', closeDetail);
        }

        // Close drawer on Escape
        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape' && state.selectedTrace) {
                closeDetail();
            }
        });
    }

    // ========================================================================
    // Data Handling
    // ========================================================================

    /**
     * Add a trace event. Called from the WebSocket message handler.
     *
     * Expected trace object shape:
     *   {
     *     trace_id: string,
     *     timestamp: number (ms epoch) or string,
     *     source: string,
     *     destination: string,
     *     protocol: string (HTTP|gRPC|BPF|WS),
     *     rtt: number (milliseconds),
     *     flow_state: string (active|complete|error|timeout),
     *     status_code: number,
     *     method: string,
     *     path: string,
     *     hops: array of { component, timestamp_ns }
     *   }
     */
    function addTrace(trace) {
        if (!state.initialized) return;
        if (!trace || !trace.trace_id) return;

        // Normalize
        var normalized = {
            traceId: trace.trace_id || trace.traceId || generateId(),
            timestamp: trace.timestamp ? new Date(trace.timestamp).getTime() : Date.now(),
            source: trace.source || (trace.hops && trace.hops.length > 0 ? trace.hops[0].component : 'unknown'),
            destination: trace.destination || (trace.hops && trace.hops.length > 1 ? trace.hops[trace.hops.length - 1].component : 'unknown'),
            protocol: trace.protocol || trace.method || 'HTTP',
            rtt: typeof trace.rtt === 'number' ? trace.rtt : (trace.total_time ? trace.total_time / 1e6 : 0),
            flowState: trace.flow_state || trace.flowState || getFlowState(trace.status_code),
            statusCode: trace.status_code || trace.statusCode || 200,
            method: trace.method || '',
            path: trace.path || '',
            hops: trace.hops || [],
            raw: trace
        };

        state.traces.unshift(normalized);

        // Enforce max
        if (state.traces.length > CONFIG.maxRows) {
            state.traces.length = CONFIG.maxRows;
        }

        applyFiltersAndRender();
    }

    /**
     * Add multiple traces at once (batch).
     */
    function addTraces(traces) {
        if (!Array.isArray(traces)) return;
        for (var i = 0; i < traces.length; i++) {
            addTrace(traces[i]);
        }
    }

    function getFlowState(statusCode) {
        if (!statusCode) return 'active';
        if (statusCode >= 500) return 'error';
        if (statusCode >= 400) return 'error';
        if (statusCode >= 200 && statusCode < 300) return 'complete';
        return 'active';
    }

    // ========================================================================
    // Filtering and Sorting
    // ========================================================================

    function applyFiltersAndRender() {
        state.filteredTraces = state.traces.filter(function(trace) {
            // Source filter
            if (state.filters.source && trace.source.toLowerCase().indexOf(state.filters.source) === -1) {
                return false;
            }
            // Destination filter
            if (state.filters.destination && trace.destination.toLowerCase().indexOf(state.filters.destination) === -1) {
                return false;
            }
            // Protocol filter
            if (state.filters.protocol && trace.protocol !== state.filters.protocol) {
                return false;
            }
            // RTT threshold filter
            if (state.filters.rttThreshold > 0 && trace.rtt > state.filters.rttThreshold) {
                return false;
            }
            return true;
        });

        // Apply sort
        sortTraces();

        // Render
        renderTable();
        updateCount();
    }

    function sortTraces() {
        var column = state.sort.column;
        var order = state.sort.order;
        var multiplier = order === 'asc' ? 1 : -1;

        state.filteredTraces.sort(function(a, b) {
            var valA = a[column];
            var valB = b[column];

            if (typeof valA === 'string') {
                return multiplier * valA.localeCompare(valB);
            }
            if (typeof valA === 'number') {
                return multiplier * (valA - valB);
            }
            return 0;
        });
    }

    function handleSortClick(e) {
        var th = e.currentTarget;
        var column = th.getAttribute('data-column');
        if (!column) return;

        // Toggle sort order
        if (state.sort.column === column) {
            state.sort.order = state.sort.order === 'asc' ? 'desc' : 'asc';
        } else {
            state.sort.column = column;
            state.sort.order = 'asc';
        }

        // Update header aria-sort attributes
        var allHeaders = state.container.querySelectorAll('.trace-th.sortable');
        for (var i = 0; i < allHeaders.length; i++) {
            allHeaders[i].removeAttribute('aria-sort');
            var indicator = allHeaders[i].querySelector('.sort-indicator');
            if (indicator) indicator.textContent = '';
        }

        th.setAttribute('aria-sort', state.sort.order === 'asc' ? 'ascending' : 'descending');
        var currentIndicator = th.querySelector('.sort-indicator');
        if (currentIndicator) {
            currentIndicator.textContent = state.sort.order === 'asc' ? ' \u25B2' : ' \u25BC';
        }

        applyFiltersAndRender();
    }

    // ========================================================================
    // Rendering
    // ========================================================================

    function renderTable() {
        if (!state.tableBody) return;

        var emptyState = document.getElementById('trace-empty-state');

        if (state.filteredTraces.length === 0) {
            state.tableBody.innerHTML = '';
            if (emptyState) emptyState.style.display = '';
            return;
        }

        if (emptyState) emptyState.style.display = 'none';

        // Build rows with document fragment for performance
        var fragment = document.createDocumentFragment();

        for (var i = 0; i < state.filteredTraces.length; i++) {
            var trace = state.filteredTraces[i];
            var row = createTraceRow(trace);
            fragment.appendChild(row);
        }

        state.tableBody.innerHTML = '';
        state.tableBody.appendChild(fragment);

        // Auto-scroll to top if sorting by timestamp descending
        if (CONFIG.autoScroll && state.sort.column === 'timestamp' && state.sort.order === 'desc') {
            var wrapper = document.getElementById('trace-table-wrapper');
            if (wrapper) wrapper.scrollTop = 0;
        }
    }

    function createTraceRow(trace) {
        var row = document.createElement('tr');
        row.className = 'trace-row ' + getRttClass(trace.rtt) + ' flow-' + trace.flowState;
        row.setAttribute('data-trace-id', trace.traceId);
        row.setAttribute('tabindex', '0');
        row.setAttribute('role', 'row');
        row.setAttribute('aria-label', 'Trace ' + trace.traceId.substring(0, 8));

        var timeStr = formatTimestamp(trace.timestamp);
        var rttStr = trace.rtt > 0 ? trace.rtt.toFixed(2) + 'ms' : '--';

        row.innerHTML =
            '<td class="trace-td trace-td-timestamp">' + escapeHtml(timeStr) + '</td>' +
            '<td class="trace-td trace-td-source">' + escapeHtml(trace.source) + '</td>' +
            '<td class="trace-td trace-td-dest">' + escapeHtml(trace.destination) + '</td>' +
            '<td class="trace-td trace-td-protocol"><span class="trace-protocol-badge">' + escapeHtml(trace.protocol) + '</span></td>' +
            '<td class="trace-td trace-td-rtt ' + getRttClass(trace.rtt) + '">' + rttStr + '</td>' +
            '<td class="trace-td trace-td-state"><span class="trace-state-badge state-' + escapeHtml(trace.flowState) + '">' + escapeHtml(trace.flowState) + '</span></td>' +
            '<td class="trace-td trace-td-traceid font-mono">' + escapeHtml(trace.traceId.substring(0, 16)) + '</td>';

        // Click to expand
        row.addEventListener('click', function() {
            openDetail(trace);
        });
        row.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                openDetail(trace);
            }
        });

        return row;
    }

    function getRttClass(rtt) {
        if (rtt <= 0) return 'rtt-unknown';
        if (rtt < CONFIG.rttThresholds.good) return 'rtt-good';
        if (rtt < CONFIG.rttThresholds.warning) return 'rtt-warning';
        return 'rtt-critical';
    }

    function formatTimestamp(ts) {
        try {
            var d = new Date(ts);
            return d.toLocaleTimeString('en-US', {
                hour12: false,
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            }) + '.' + String(d.getMilliseconds()).padStart(3, '0');
        } catch {
            return '--:--:--';
        }
    }

    function updateCount() {
        var countEl = document.getElementById('trace-count');
        if (countEl) {
            var total = state.traces.length;
            var filtered = state.filteredTraces.length;
            if (total === filtered) {
                countEl.textContent = total + ' trace' + (total !== 1 ? 's' : '');
            } else {
                countEl.textContent = filtered + ' of ' + total + ' traces';
            }
        }
    }

    // ========================================================================
    // Detail Drawer
    // ========================================================================

    function openDetail(trace) {
        state.selectedTrace = trace;
        var drawer = state.detailDrawer;
        if (!drawer) return;

        var body = document.getElementById('trace-detail-body');
        if (!body) return;

        var rttStr = trace.rtt > 0 ? trace.rtt.toFixed(3) + 'ms' : 'N/A';
        var timeStr = new Date(trace.timestamp).toISOString();

        var hopsHtml = '';
        if (trace.hops && trace.hops.length > 0) {
            hopsHtml = '<div class="trace-detail-section">' +
                '<h4>Hops (' + trace.hops.length + ')</h4>' +
                '<ol class="trace-hops-list">';
            for (var i = 0; i < trace.hops.length; i++) {
                var hop = trace.hops[i];
                var hopTs = hop.timestamp_ns ? (hop.timestamp_ns / 1e6).toFixed(3) + 'ms' : '--';
                hopsHtml += '<li class="trace-hop-item">' +
                    '<span class="trace-hop-component">' + escapeHtml(hop.component || 'unknown') + '</span>' +
                    '<span class="trace-hop-time">' + hopTs + '</span>' +
                '</li>';
            }
            hopsHtml += '</ol></div>';
        }

        body.innerHTML =
            '<div class="trace-detail-section">' +
                '<h4>Overview</h4>' +
                '<div class="trace-detail-grid">' +
                    '<span class="trace-detail-label">Trace ID</span>' +
                    '<span class="trace-detail-value font-mono">' + escapeHtml(trace.traceId) + '</span>' +
                    '<span class="trace-detail-label">Timestamp</span>' +
                    '<span class="trace-detail-value">' + escapeHtml(timeStr) + '</span>' +
                    '<span class="trace-detail-label">Source</span>' +
                    '<span class="trace-detail-value">' + escapeHtml(trace.source) + '</span>' +
                    '<span class="trace-detail-label">Destination</span>' +
                    '<span class="trace-detail-value">' + escapeHtml(trace.destination) + '</span>' +
                    '<span class="trace-detail-label">Protocol</span>' +
                    '<span class="trace-detail-value">' + escapeHtml(trace.protocol) + '</span>' +
                    '<span class="trace-detail-label">RTT</span>' +
                    '<span class="trace-detail-value ' + getRttClass(trace.rtt) + '">' + rttStr + '</span>' +
                    '<span class="trace-detail-label">Flow State</span>' +
                    '<span class="trace-detail-value"><span class="trace-state-badge state-' + escapeHtml(trace.flowState) + '">' + escapeHtml(trace.flowState) + '</span></span>' +
                    '<span class="trace-detail-label">Status Code</span>' +
                    '<span class="trace-detail-value">' + trace.statusCode + '</span>' +
                    '<span class="trace-detail-label">Method</span>' +
                    '<span class="trace-detail-value">' + escapeHtml(trace.method || 'N/A') + '</span>' +
                    '<span class="trace-detail-label">Path</span>' +
                    '<span class="trace-detail-value font-mono">' + escapeHtml(trace.path || 'N/A') + '</span>' +
                '</div>' +
            '</div>' +
            hopsHtml;

        drawer.classList.add('open');
        drawer.setAttribute('aria-hidden', 'false');

        // Highlight selected row
        var rows = state.tableBody.querySelectorAll('.trace-row');
        for (var j = 0; j < rows.length; j++) {
            rows[j].classList.remove('selected');
            if (rows[j].getAttribute('data-trace-id') === trace.traceId) {
                rows[j].classList.add('selected');
            }
        }
    }

    function closeDetail() {
        state.selectedTrace = null;
        if (state.detailDrawer) {
            state.detailDrawer.classList.remove('open');
            state.detailDrawer.setAttribute('aria-hidden', 'true');
        }
        // Remove selected highlight
        if (state.tableBody) {
            var rows = state.tableBody.querySelectorAll('.trace-row.selected');
            for (var i = 0; i < rows.length; i++) {
                rows[i].classList.remove('selected');
            }
        }
    }

    // ========================================================================
    // Utilities
    // ========================================================================

    function generateId() {
        return Math.random().toString(36).substring(2, 10);
    }

    function escapeHtml(text) {
        if (!text) return '';
        var div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }

    // ========================================================================
    // Public API
    // ========================================================================

    function clear() {
        state.traces = [];
        state.filteredTraces = [];
        state.selectedTrace = null;
        if (state.tableBody) state.tableBody.innerHTML = '';
        updateCount();
        closeDetail();
        var emptyState = document.getElementById('trace-empty-state');
        if (emptyState) emptyState.style.display = '';
    }

    function getTraces() {
        return state.traces.slice();
    }

    function getFilteredTraces() {
        return state.filteredTraces.slice();
    }

    function isInitialized() {
        return state.initialized;
    }

    return {
        init: init,
        addTrace: addTrace,
        addTraces: addTraces,
        clear: clear,
        getTraces: getTraces,
        getFilteredTraces: getFilteredTraces,
        isInitialized: isInitialized
    };
})();

// Make available globally
window.TraceViewer = TraceViewer;

console.log('trace-viewer.js loaded');
