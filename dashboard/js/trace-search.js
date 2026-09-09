/**
 * trace-search.js - Trace Search Widget (App 01 - Monad CPU)
 * Search, filter, and explore distributed traces in real-time.
 *
 * Features:
 *   - Search form: source service, dest service, min/max RTT, trace ID
 *   - Results table: trace_id, src_svc, dst_svc, hop_count, total_latency, timestamp
 *   - Filter/sort by any column
 *   - Click row to expand hop detail
 *   - WebSocket subscription for live trace events
 *
 * No external dependencies. Vanilla JS only.
 */

var TraceSearch = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxResults: 500,
        pageSize: 50,
        wsReconnectDelay: 3000,
        columns: [
            { key: 'trace_id',      label: 'Trace ID',      width: '18%' },
            { key: 'src_svc',       label: 'Source',         width: '14%' },
            { key: 'dst_svc',       label: 'Destination',    width: '14%' },
            { key: 'hop_count',     label: 'Hops',           width: '8%'  },
            { key: 'total_latency', label: 'Latency (ms)',   width: '12%' },
            { key: 'timestamp',     label: 'Timestamp',      width: '18%' }
        ],
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            rowEven: 'rgba(255, 215, 0, 0.02)',
            rowOdd: 'transparent',
            rowHover: 'rgba(255, 215, 0, 0.08)',
            rowExpanded: 'rgba(78, 205, 196, 0.06)',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            latencyGood: '#00d26a',
            latencyWarn: '#ff9800',
            latencyBad: '#ff4757',
            hopBg: '#12121f'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        traces: [],
        filtered: [],
        filters: { src_svc: '', dst_svc: '', min_rtt: '', max_rtt: '', trace_id: '' },
        sort: { column: 'timestamp', order: 'desc' },
        expandedRow: null,
        ws: null,
        liveMode: false,
        tableBody: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') {
            container = document.getElementById(container);
        }
        if (!container) {
            console.error('[TraceSearch] Container not found');
            return false;
        }

        state.container = container;
        container.innerHTML = '';
        container.style.position = 'relative';

        buildSearchForm(container);
        buildResultsTable(container);

        state.initialized = true;
        console.log('[TraceSearch] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildSearchForm(parent) {
        var form = document.createElement('div');
        form.className = 'ts-search-form';
        form.style.cssText = 'display:flex;flex-wrap:wrap;gap:8px;padding:12px;background:' +
            CONFIG.colors.headerBg + ';border:1px solid ' + CONFIG.colors.border +
            ';border-radius:6px;margin-bottom:12px;';

        var fields = [
            { key: 'src_svc',  placeholder: 'Source service',  type: 'text' },
            { key: 'dst_svc',  placeholder: 'Dest service',    type: 'text' },
            { key: 'min_rtt',  placeholder: 'Min RTT (ms)',    type: 'number' },
            { key: 'max_rtt',  placeholder: 'Max RTT (ms)',    type: 'number' },
            { key: 'trace_id', placeholder: 'Trace ID',        type: 'text' }
        ];

        for (var i = 0; i < fields.length; i++) {
            var input = document.createElement('input');
            input.type = fields[i].type;
            input.placeholder = fields[i].placeholder;
            input.dataset.filter = fields[i].key;
            input.style.cssText = 'flex:1;min-width:120px;padding:6px 10px;background:#0f0f1a;' +
                'color:#e0e0e0;border:1px solid ' + CONFIG.colors.border +
                ';border-radius:4px;font-size:12px;font-family:"SF Mono",monospace;';
            input.addEventListener('input', onFilterChange);
            form.appendChild(input);
        }

        var searchBtn = document.createElement('button');
        searchBtn.textContent = 'Search';
        searchBtn.style.cssText = 'padding:6px 16px;background:' + CONFIG.colors.accent +
            ';color:#0f0f1a;border:none;border-radius:4px;cursor:pointer;font-weight:bold;font-size:12px;';
        searchBtn.addEventListener('click', applyFilters);
        form.appendChild(searchBtn);

        var liveBtn = document.createElement('button');
        liveBtn.textContent = 'Live';
        liveBtn.id = 'ts-live-btn';
        liveBtn.style.cssText = 'padding:6px 12px;background:transparent;color:' +
            CONFIG.colors.latencyGood + ';border:1px solid ' + CONFIG.colors.latencyGood +
            ';border-radius:4px;cursor:pointer;font-size:12px;';
        liveBtn.addEventListener('click', toggleLive);
        form.appendChild(liveBtn);

        parent.appendChild(form);
    }

    function buildResultsTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'overflow-x:auto;border:1px solid ' + CONFIG.colors.border +
            ';border-radius:6px;max-height:500px;overflow-y:auto;';

        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:12px;font-family:"SF Mono",monospace;';

        var thead = document.createElement('thead');
        var headerRow = document.createElement('tr');
        headerRow.style.cssText = 'background:' + CONFIG.colors.headerBg + ';position:sticky;top:0;z-index:1;';

        for (var i = 0; i < CONFIG.columns.length; i++) {
            var th = document.createElement('th');
            th.textContent = CONFIG.columns[i].label;
            th.dataset.column = CONFIG.columns[i].key;
            th.style.cssText = 'padding:8px 10px;text-align:left;color:' + CONFIG.colors.accent +
                ';cursor:pointer;width:' + CONFIG.columns[i].width +
                ';border-bottom:2px solid ' + CONFIG.colors.border +
                ';user-select:none;white-space:nowrap;';
            th.addEventListener('click', onSortClick);
            headerRow.appendChild(th);
        }
        thead.appendChild(headerRow);
        table.appendChild(thead);

        state.tableBody = document.createElement('tbody');
        table.appendChild(state.tableBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);

        renderEmptyState();
    }

    // ========================================================================
    // Rendering
    // ========================================================================
    function renderTable() {
        if (!state.tableBody) return;
        state.tableBody.innerHTML = '';

        if (state.filtered.length === 0) {
            renderEmptyState();
            return;
        }

        var limit = Math.min(state.filtered.length, CONFIG.pageSize);
        for (var i = 0; i < limit; i++) {
            var trace = state.filtered[i];
            var tr = document.createElement('tr');
            tr.dataset.index = i;
            tr.style.cssText = 'cursor:pointer;background:' +
                (i % 2 === 0 ? CONFIG.colors.rowEven : CONFIG.colors.rowOdd) +
                ';transition:background 0.15s;';
            tr.addEventListener('mouseenter', function() { this.style.background = CONFIG.colors.rowHover; });
            tr.addEventListener('mouseleave', function() {
                var idx = parseInt(this.dataset.index, 10);
                this.style.background = idx % 2 === 0 ? CONFIG.colors.rowEven : CONFIG.colors.rowOdd;
            });
            tr.addEventListener('click', onRowClick);

            for (var j = 0; j < CONFIG.columns.length; j++) {
                var td = document.createElement('td');
                td.style.cssText = 'padding:6px 10px;color:' + CONFIG.colors.text +
                    ';border-bottom:1px solid ' + CONFIG.colors.border + ';white-space:nowrap;';
                var key = CONFIG.columns[j].key;
                var val = trace[key];

                if (key === 'timestamp') {
                    td.textContent = formatTimestamp(val);
                } else if (key === 'total_latency') {
                    td.textContent = typeof val === 'number' ? val.toFixed(2) : '--';
                    td.style.color = latencyColor(val);
                } else if (key === 'trace_id') {
                    td.textContent = typeof val === 'string' ? val.substring(0, 12) + '...' : '--';
                    td.title = val || '';
                } else {
                    td.textContent = val != null ? val : '--';
                }
                tr.appendChild(td);
            }
            state.tableBody.appendChild(tr);
        }
    }

    function renderEmptyState() {
        if (!state.tableBody) return;
        state.tableBody.innerHTML = '';
        var tr = document.createElement('tr');
        var td = document.createElement('td');
        td.colSpan = CONFIG.columns.length;
        td.style.cssText = 'padding:40px;text-align:center;color:' + CONFIG.colors.textMuted + ';font-size:13px;';
        td.textContent = 'No traces found. Enter search criteria or enable live mode.';
        tr.appendChild(td);
        state.tableBody.appendChild(tr);
    }

    function renderHopDetail(trace, parentRow) {
        if (!trace.hops || trace.hops.length === 0) return;

        var detailRow = document.createElement('tr');
        detailRow.className = 'ts-detail-row';
        var detailTd = document.createElement('td');
        detailTd.colSpan = CONFIG.columns.length;
        detailTd.style.cssText = 'padding:0;background:' + CONFIG.colors.hopBg + ';';

        var inner = document.createElement('div');
        inner.style.cssText = 'padding:12px 20px;';

        var title = document.createElement('div');
        title.style.cssText = 'color:' + CONFIG.colors.accent + ';font-weight:bold;margin-bottom:8px;font-size:11px;';
        title.textContent = 'HOP DETAIL - ' + (trace.hops.length) + ' hops';
        inner.appendChild(title);

        var hopTable = document.createElement('table');
        hopTable.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        var hdr = '<tr style="color:' + CONFIG.colors.textMuted + '">' +
            '<th style="padding:4px 8px;text-align:left">Hop</th>' +
            '<th style="padding:4px 8px;text-align:left">Node</th>' +
            '<th style="padding:4px 8px;text-align:right">Latency (us)</th>' +
            '<th style="padding:4px 8px;text-align:right">Queue Depth</th>' +
            '<th style="padding:4px 8px;text-align:right">Link Util %</th></tr>';
        hopTable.innerHTML = hdr;

        for (var h = 0; h < trace.hops.length; h++) {
            var hop = trace.hops[h];
            var row = document.createElement('tr');
            row.style.cssText = 'color:' + CONFIG.colors.text + ';';
            row.innerHTML = '<td style="padding:4px 8px">' + h + '</td>' +
                '<td style="padding:4px 8px">' + (hop.node || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right;color:' + latencyColor((hop.latency_us || 0) / 1000) + '">' +
                (hop.latency_us || 0) + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (hop.queue_depth || 0) + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + ((hop.link_util || 0) * 100).toFixed(1) + '</td>';
            hopTable.appendChild(row);
        }

        inner.appendChild(hopTable);
        detailTd.appendChild(inner);
        detailRow.appendChild(detailTd);
        parentRow.parentNode.insertBefore(detailRow, parentRow.nextSibling);
    }

    // ========================================================================
    // Event Handlers
    // ========================================================================
    function onFilterChange(e) {
        var key = e.target.dataset.filter;
        if (key) state.filters[key] = e.target.value;
    }

    function applyFilters() {
        state.filtered = state.traces.filter(function(t) {
            if (state.filters.src_svc && t.src_svc &&
                t.src_svc.toLowerCase().indexOf(state.filters.src_svc.toLowerCase()) === -1) return false;
            if (state.filters.dst_svc && t.dst_svc &&
                t.dst_svc.toLowerCase().indexOf(state.filters.dst_svc.toLowerCase()) === -1) return false;
            if (state.filters.trace_id && t.trace_id &&
                t.trace_id.toLowerCase().indexOf(state.filters.trace_id.toLowerCase()) === -1) return false;
            if (state.filters.min_rtt && t.total_latency < parseFloat(state.filters.min_rtt)) return false;
            if (state.filters.max_rtt && t.total_latency > parseFloat(state.filters.max_rtt)) return false;
            return true;
        });
        sortFiltered();
        renderTable();
    }

    function onSortClick(e) {
        var col = e.target.dataset.column;
        if (!col) return;
        if (state.sort.column === col) {
            state.sort.order = state.sort.order === 'asc' ? 'desc' : 'asc';
        } else {
            state.sort.column = col;
            state.sort.order = 'asc';
        }
        sortFiltered();
        renderTable();
    }

    function sortFiltered() {
        var col = state.sort.column;
        var dir = state.sort.order === 'asc' ? 1 : -1;
        state.filtered.sort(function(a, b) {
            var va = a[col], vb = b[col];
            if (va == null) return 1;
            if (vb == null) return -1;
            if (typeof va === 'number') return (va - vb) * dir;
            return String(va).localeCompare(String(vb)) * dir;
        });
    }

    function onRowClick(e) {
        var tr = e.currentTarget;
        var idx = parseInt(tr.dataset.index, 10);
        var existing = tr.parentNode.querySelector('.ts-detail-row');
        if (existing) { existing.remove(); state.expandedRow = null; return; }
        var prev = state.tableBody.querySelector('.ts-detail-row');
        if (prev) prev.remove();
        state.expandedRow = idx;
        renderHopDetail(state.filtered[idx], tr);
    }

    function toggleLive() {
        state.liveMode = !state.liveMode;
        var btn = document.getElementById('ts-live-btn');
        if (btn) {
            btn.textContent = state.liveMode ? 'Live ON' : 'Live';
            btn.style.background = state.liveMode ? CONFIG.colors.latencyGood : 'transparent';
            btn.style.color = state.liveMode ? '#0f0f1a' : CONFIG.colors.latencyGood;
        }
        if (state.liveMode) connectWS(); else disconnectWS();
    }

    // ========================================================================
    // WebSocket
    // ========================================================================
    function connectWS() {
        var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        var url = protocol + '//' + window.location.host + '/ws/traces';
        try {
            state.ws = new WebSocket(url);
            state.ws.onmessage = function(e) {
                try {
                    var data = JSON.parse(e.data);
                    if (Array.isArray(data)) {
                        for (var i = 0; i < data.length; i++) addTrace(data[i]);
                    } else {
                        addTrace(data);
                    }
                    applyFilters();
                } catch { /* ignore parse errors */ }
            };
            state.ws.onclose = function() {
                if (state.liveMode) {
                    setTimeout(connectWS, CONFIG.wsReconnectDelay);
                }
            };
        } catch (err) {
            console.error('[TraceSearch] WebSocket error:', err);
        }
    }

    function disconnectWS() {
        if (state.ws) { state.ws.close(); state.ws = null; }
    }

    function addTrace(trace) {
        state.traces.unshift(trace);
        if (state.traces.length > CONFIG.maxResults) state.traces.pop();
    }

    // ========================================================================
    // Utilities
    // ========================================================================
    function latencyColor(ms) {
        if (typeof ms !== 'number') return CONFIG.colors.text;
        if (ms < 1) return CONFIG.colors.latencyGood;
        if (ms < 10) return CONFIG.colors.latencyWarn;
        return CONFIG.colors.latencyBad;
    }

    function formatTimestamp(ts) {
        if (!ts) return '--';
        var d = new Date(typeof ts === 'number' ? ts : Date.parse(ts));
        return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' }) +
            '.' + String(d.getMilliseconds()).padStart(3, '0');
    }

    // ========================================================================
    // Public API
    // ========================================================================
    function update(traces) {
        if (!Array.isArray(traces)) return;
        state.traces = traces.slice(0, CONFIG.maxResults);
        applyFilters();
    }

    function destroy() {
        disconnectWS();
        state.liveMode = false;
        if (state.container) state.container.innerHTML = '';
        state.traces = [];
        state.filtered = [];
        state.expandedRow = null;
        state.initialized = false;
    }

    return {
        init: init,
        update: update,
        destroy: destroy,
        addTrace: addTrace,
        isInitialized: function() { return state.initialized; }
    };
})();

window.TraceSearch = TraceSearch;
console.log('trace-search.js loaded');
