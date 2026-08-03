/**
 * Log Viewer Component — Unheaded Dashboard
 *
 * Provides service log viewing with:
 * - Service selector dropdown
 * - Level filter (all/debug/info/warn/error/fatal)
 * - Full-text search with debounce
 * - Time range picker (last 5m/15m/1h/24h)
 * - Auto-scroll toggle (tail -f mode)
 * - Color-coded log levels
 * - SSE live tail via /ws/logs
 */

const LogViewer = {
    // State
    service: '',
    level: '',
    search: '',
    timeRange: '15m',
    autoScroll: true,
    entries: [],
    eventSource: null,

    // Level colors
    levelColors: {
        trace: '#888',
        debug: '#aaa',
        info: '#4fc3f7',
        warn: '#ffd54f',
        error: '#ef5350',
        fatal: '#e040fb',
        panic: '#e040fb'
    },

    /**
     * Initialize the log viewer
     */
    init() {
        this.output = document.getElementById('log-output');
        this.serviceSelect = document.getElementById('log-service');
        this.levelSelect = document.getElementById('log-level');
        this.searchInput = document.getElementById('log-search');
        this.autoScrollToggle = document.getElementById('log-autoscroll');
        this.timeRangeSelect = document.getElementById('log-timerange');
        this.statusEl = document.getElementById('log-status');
        this.statusBarEl = document.getElementById('log-status-bar');
        this.liveDot = document.getElementById('live-dot');
        this.liveDotBar = document.getElementById('live-dot-bar');
        this.countEl = document.getElementById('log-count');

        if (!this.output) return;

        // Event listeners
        if (this.serviceSelect) {
            this.serviceSelect.addEventListener('change', () => {
                this.service = this.serviceSelect.value;
                this.refresh();
            });
        }
        if (this.levelSelect) {
            this.levelSelect.addEventListener('change', () => {
                this.level = this.levelSelect.value;
                this.refresh();
            });
        }
        if (this.searchInput) {
            let debounceTimer;
            this.searchInput.addEventListener('input', () => {
                clearTimeout(debounceTimer);
                debounceTimer = setTimeout(() => {
                    this.search = this.searchInput.value;
                    this.refresh();
                }, 300);
            });
        }
        if (this.autoScrollToggle) {
            this.autoScrollToggle.addEventListener('change', () => {
                this.autoScroll = this.autoScrollToggle.checked;
            });
        }
        if (this.timeRangeSelect) {
            this.timeRangeSelect.addEventListener('change', () => {
                this.timeRange = this.timeRangeSelect.value;
                this.refresh();
            });
        }

        // Initial load
        this.refresh();
        this.connectSSE();
    },

    /**
     * Fetch logs from REST API
     */
    async refresh() {
        const params = new URLSearchParams();
        if (this.service) params.set('service', this.service);
        if (this.level) params.set('level', this.level);
        if (this.search) params.set('search', this.search);
        params.set('limit', '500');

        // Time range
        if (this.timeRange) {
            const now = new Date();
            const ms = { '5m': 300000, '15m': 900000, '1h': 3600000, '24h': 86400000 };
            if (ms[this.timeRange]) {
                params.set('from', new Date(now - ms[this.timeRange]).toISOString());
            }
        }

        try {
            const resp = await fetch('/api/v1/logs?' + params.toString());
            const entries = await resp.json();
            this.entries = entries || [];
            this.render();
            this.updateStatus('connected');
        } catch (err) {
            this.updateStatus('error: ' + err.message);
        }
    },

    /**
     * Connect to SSE stream for live tail
     */
    connectSSE() {
        if (this.eventSource) {
            this.eventSource.close();
        }

        const params = new URLSearchParams();
        if (this.service) params.set('service', this.service);
        if (this.level) params.set('level', this.level);

        this.eventSource = new EventSource('/ws/logs?' + params.toString());

        this.eventSource.onmessage = (event) => {
            try {
                const entry = JSON.parse(event.data);
                this.entries.push(entry);
                // Keep last 1000 entries in UI
                if (this.entries.length > 1000) {
                    this.entries = this.entries.slice(-1000);
                }
                this.appendEntry(entry);
                if (this.countEl) {
                    this.countEl.textContent = this.entries.length + ' entries';
                }
            } catch {
                // skip malformed
            }
        };

        this.eventSource.onerror = () => {
            this.updateStatus('disconnected');
            // Reconnect after 5s
            setTimeout(() => this.connectSSE(), 5000);
        };

        this.eventSource.onopen = () => {
            this.updateStatus('streaming');
        };
    },

    /**
     * Render all entries
     */
    render() {
        if (!this.output) return;
        this.output.innerHTML = '';
        for (const entry of this.entries) {
            this.appendEntry(entry);
        }
        if (this.countEl) {
            this.countEl.textContent = this.entries.length + ' entries';
        }
    },

    /**
     * Append a single log entry to the output
     */
    appendEntry(entry) {
        if (!this.output) return;

        const line = document.createElement('div');
        var lvl = entry.level || 'info';
        line.className = 'log-line level-' + lvl;

        const ts = entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : '--:--:--';
        const color = this.levelColors[entry.level] || '#ccc';
        const level = (entry.level || 'info').toUpperCase().padEnd(5);

        line.innerHTML =
            '<span class="log-ts">' + ts + '</span> ' +
            '<span class="log-level" style="color:' + color + '">' + level + '</span> ' +
            '<span class="log-service">[' + (entry.service || '?') + ']</span> ' +
            '<span class="log-msg">' + this.escapeHTML(entry.message || '') + '</span>';

        // Add fields if present
        if (entry.fields && Object.keys(entry.fields).length > 0) {
            const fields = document.createElement('span');
            fields.className = 'log-fields';
            fields.textContent = ' ' + JSON.stringify(entry.fields);
            line.appendChild(fields);
        }

        // Click to copy
        line.addEventListener('click', () => {
            navigator.clipboard.writeText(JSON.stringify(entry, null, 2));
        });

        this.output.appendChild(line);

        if (this.autoScroll) {
            this.output.scrollTop = this.output.scrollHeight;
        }
    },

    /**
     * Update connection status display
     */
    updateStatus(status) {
        var key = status.split(':')[0];
        if (this.statusEl) {
            this.statusEl.textContent = status;
            this.statusEl.className = 'log-status-' + key;
        }
        if (this.statusBarEl) {
            this.statusBarEl.textContent = status;
            this.statusBarEl.className = 'log-status-' + key;
        }
        if (this.liveDot) {
            this.liveDot.className = 'live-dot ' + key;
        }
        if (this.liveDotBar) {
            this.liveDotBar.className = 'live-dot ' + key;
        }
    },

    /**
     * Escape HTML entities
     */
    escapeHTML(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }
};

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => LogViewer.init());
} else {
    LogViewer.init();
}
