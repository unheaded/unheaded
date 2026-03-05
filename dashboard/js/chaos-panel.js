/**
 * chaos-panel.js - Chaos Engineering Panel (App 06 - Monad CPU)
 * Manage and visualize chaos injection rules, game days, and audit trails.
 *
 * Features:
 *   - Rules table: service pair, fault type, duration, drop rate, created_by, status
 *   - Start injection form: src_svc, dst_svc, fault_type dropdown, parameters
 *   - Recent events list: last 100 chaos events with timestamps
 *   - GameDay control: dropdown select scenario, start/stop buttons, progress bar
 *   - Circuit breaker kill-switch: big red button with confirmation
 *   - Audit trail: filterable table (user, action, timestamp, rule details)
 *   - Metrics overlay: chaos events annotated on service latency graph
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var ChaosPanel = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        maxEvents: 100,
        maxAudit: 200,
        faultTypes: ['LATENCY', 'DROP', 'CORRUPT', 'PARTITION', 'THROTTLE', 'ABORT'],
        statusColors: {
            'ACTIVE':    '#ff4757',
            'PAUSED':    '#ff9800',
            'COMPLETED': '#00d26a',
            'PENDING':   '#6b6b7b'
        },
        colors: {
            background: '#0f0f1a',
            headerBg: '#1a1a2e',
            text: '#e0e0e0',
            textMuted: '#6b6b7b',
            accent: '#ffd700',
            border: 'rgba(255, 215, 0, 0.1)',
            danger: '#ff4757',
            dangerHover: '#ff6b6b',
            success: '#00d26a',
            warning: '#ff9800',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            latencyLine: '#4ecdc4',
            chaosMarker: 'rgba(255, 71, 87, 0.3)',
            cardBg: '#12121f'
        },
        padding: { top: 20, right: 16, bottom: 30, left: 50 }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        container: null,
        latencyCanvas: null,
        latencyCtx: null,

        rules: [],            // [{ id, src_svc, dst_svc, fault_type, duration, drop_rate, created_by, status }]
        events: [],           // [{ timestamp, type, src_svc, dst_svc, detail }]
        auditTrail: [],       // [{ user, action, timestamp, rule_details }]
        gameDay: { scenario: null, running: false, progress: 0 },
        latencyData: [],      // [{ timestamp, latency_ms }]
        chaosAnnotations: [], // [{ timestamp, label }]

        rulesBody: null,
        eventList: null,
        auditBody: null,
        auditFilter: '',
        animationFrame: null,
        onInject: null,       // callback for injection form submit
        onKillSwitch: null,   // callback for kill switch
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(container) {
        if (typeof container === 'string') container = document.getElementById(container);
        if (!container) { console.error('[ChaosPanel] Container not found'); return false; }

        state.container = container;
        container.innerHTML = '';
        container.style.cssText = 'position:relative;font-family:"SF Mono",monospace;color:' + CONFIG.colors.text + ';';

        buildLayout(container);
        window.addEventListener('resize', debounce(resizeCanvases, 150));
        resizeCanvases();
        startAnimation();

        state.initialized = true;
        console.log('[ChaosPanel] Initialized');
        return true;
    }

    // ========================================================================
    // UI Construction
    // ========================================================================
    function buildLayout(parent) {
        // Top row: injection form + kill switch + game day
        var topRow = document.createElement('div');
        topRow.style.cssText = 'display:grid;grid-template-columns:2fr 1fr;gap:12px;margin-bottom:12px;';

        // Injection form
        var formSection = createSection('Start Chaos Injection');
        buildInjectionForm(formSection.body);
        topRow.appendChild(formSection.el);

        // Game day + kill switch
        var controlSection = createSection('GameDay Control');
        buildGameDayControl(controlSection.body);
        topRow.appendChild(controlSection.el);

        parent.appendChild(topRow);

        // Rules table
        var rulesSection = createSection('Active Rules');
        buildRulesTable(rulesSection.body);
        parent.appendChild(rulesSection.el);

        // Mid row: events + latency overlay
        var midRow = document.createElement('div');
        midRow.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:12px;';

        var evSection = createSection('Recent Chaos Events (Last 100)');
        state.eventList = document.createElement('div');
        state.eventList.style.cssText = 'max-height:250px;overflow-y:auto;';
        evSection.body.appendChild(state.eventList);
        midRow.appendChild(evSection.el);

        var latSection = createSection('Service Latency with Chaos Annotations');
        state.latencyCanvas = document.createElement('canvas');
        state.latencyCanvas.style.cssText = 'display:block;width:100%;';
        latSection.body.appendChild(state.latencyCanvas);
        state.latencyCtx = state.latencyCanvas.getContext('2d');
        midRow.appendChild(latSection.el);

        parent.appendChild(midRow);

        // Audit trail
        var auditSection = createSection('Audit Trail');
        buildAuditTable(auditSection.body);
        parent.appendChild(auditSection.el);
    }

    function buildInjectionForm(parent) {
        var form = document.createElement('div');
        form.style.cssText = 'display:grid;grid-template-columns:1fr 1fr;gap:8px;';

        var fields = [
            { key: 'src_svc', placeholder: 'Source Service', type: 'text' },
            { key: 'dst_svc', placeholder: 'Dest Service', type: 'text' }
        ];

        for (var i = 0; i < fields.length; i++) {
            var input = document.createElement('input');
            input.type = fields[i].type;
            input.placeholder = fields[i].placeholder;
            input.id = 'chaos-' + fields[i].key;
            input.style.cssText = inputStyle();
            form.appendChild(input);
        }

        // Fault type dropdown
        var select = document.createElement('select');
        select.id = 'chaos-fault-type';
        select.style.cssText = inputStyle();
        var defaultOpt = document.createElement('option');
        defaultOpt.value = '';
        defaultOpt.textContent = 'Fault Type...';
        select.appendChild(defaultOpt);
        for (var f = 0; f < CONFIG.faultTypes.length; f++) {
            var opt = document.createElement('option');
            opt.value = CONFIG.faultTypes[f];
            opt.textContent = CONFIG.faultTypes[f];
            select.appendChild(opt);
        }
        form.appendChild(select);

        // Parameters
        var params = document.createElement('input');
        params.type = 'text';
        params.placeholder = 'Parameters (JSON)';
        params.id = 'chaos-params';
        params.style.cssText = inputStyle();
        form.appendChild(params);

        // Submit button
        var btn = document.createElement('button');
        btn.textContent = 'Inject Fault';
        btn.style.cssText = 'grid-column:1/3;padding:8px;background:' + CONFIG.colors.danger +
            ';color:#fff;border:none;border-radius:4px;cursor:pointer;font-weight:bold;font-size:12px;' +
            'font-family:"SF Mono",monospace;';
        btn.addEventListener('click', onInjectClick);
        form.appendChild(btn);

        parent.appendChild(form);
    }

    function buildGameDayControl(parent) {
        // Scenario selector
        var selectRow = document.createElement('div');
        selectRow.style.cssText = 'display:flex;gap:8px;margin-bottom:10px;';
        var scenarioSelect = document.createElement('select');
        scenarioSelect.id = 'chaos-scenario';
        scenarioSelect.style.cssText = inputStyle() + 'flex:1;';
        scenarioSelect.innerHTML = '<option value="">Select Scenario...</option>' +
            '<option value="network_partition">Network Partition</option>' +
            '<option value="cascading_failure">Cascading Failure</option>' +
            '<option value="latency_spike">Latency Spike</option>' +
            '<option value="resource_exhaustion">Resource Exhaustion</option>';
        selectRow.appendChild(scenarioSelect);

        var startBtn = document.createElement('button');
        startBtn.id = 'chaos-gameday-start';
        startBtn.textContent = 'Start';
        startBtn.style.cssText = 'padding:6px 14px;background:' + CONFIG.colors.success +
            ';color:#0f0f1a;border:none;border-radius:4px;cursor:pointer;font-weight:bold;font-size:11px;';
        startBtn.addEventListener('click', function() { toggleGameDay(true); });
        selectRow.appendChild(startBtn);

        var stopBtn = document.createElement('button');
        stopBtn.textContent = 'Stop';
        stopBtn.style.cssText = 'padding:6px 14px;background:' + CONFIG.colors.warning +
            ';color:#0f0f1a;border:none;border-radius:4px;cursor:pointer;font-weight:bold;font-size:11px;';
        stopBtn.addEventListener('click', function() { toggleGameDay(false); });
        selectRow.appendChild(stopBtn);

        parent.appendChild(selectRow);

        // Progress bar
        var progressWrapper = document.createElement('div');
        progressWrapper.style.cssText = 'background:rgba(255,255,255,0.05);border-radius:4px;height:20px;overflow:hidden;margin-bottom:16px;';
        var progressBar = document.createElement('div');
        progressBar.id = 'chaos-progress';
        progressBar.style.cssText = 'width:0%;height:100%;background:' + CONFIG.colors.accent +
            ';transition:width 0.5s;border-radius:4px;';
        progressWrapper.appendChild(progressBar);
        parent.appendChild(progressWrapper);

        // Kill switch
        var killSwitch = document.createElement('button');
        killSwitch.id = 'chaos-kill-switch';
        killSwitch.textContent = 'KILL SWITCH - STOP ALL CHAOS';
        killSwitch.style.cssText = 'width:100%;padding:14px;background:' + CONFIG.colors.danger +
            ';color:#fff;border:2px solid ' + CONFIG.colors.danger +
            ';border-radius:6px;cursor:pointer;font-weight:bold;font-size:14px;text-transform:uppercase;' +
            'letter-spacing:1px;font-family:"SF Mono",monospace;';
        killSwitch.addEventListener('click', onKillSwitchClick);
        parent.appendChild(killSwitch);
    }

    function buildRulesTable(parent) {
        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:250px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">Source</th>' +
            '<th style="padding:6px 8px;text-align:left">Dest</th>' +
            '<th style="padding:6px 8px;text-align:left">Fault</th>' +
            '<th style="padding:6px 8px;text-align:right">Duration</th>' +
            '<th style="padding:6px 8px;text-align:right">Drop %</th>' +
            '<th style="padding:6px 8px;text-align:left">Created By</th>' +
            '<th style="padding:6px 8px;text-align:left">Status</th></tr></thead>';
        state.rulesBody = document.createElement('tbody');
        table.appendChild(state.rulesBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
    }

    function buildAuditTable(parent) {
        var filterRow = document.createElement('div');
        filterRow.style.cssText = 'margin-bottom:8px;';
        var filterInput = document.createElement('input');
        filterInput.type = 'text';
        filterInput.placeholder = 'Filter audit trail...';
        filterInput.style.cssText = inputStyle() + 'width:300px;';
        filterInput.addEventListener('input', function(e) { state.auditFilter = e.target.value; renderAuditTrail(); });
        filterRow.appendChild(filterInput);
        parent.appendChild(filterRow);

        var wrapper = document.createElement('div');
        wrapper.style.cssText = 'max-height:200px;overflow-y:auto;';
        var table = document.createElement('table');
        table.style.cssText = 'width:100%;border-collapse:collapse;font-size:11px;';
        table.innerHTML = '<thead><tr style="color:' + CONFIG.colors.accent + ';position:sticky;top:0;background:' +
            CONFIG.colors.headerBg + '">' +
            '<th style="padding:6px 8px;text-align:left">User</th>' +
            '<th style="padding:6px 8px;text-align:left">Action</th>' +
            '<th style="padding:6px 8px;text-align:left">Timestamp</th>' +
            '<th style="padding:6px 8px;text-align:left">Details</th></tr></thead>';
        state.auditBody = document.createElement('tbody');
        table.appendChild(state.auditBody);
        wrapper.appendChild(table);
        parent.appendChild(wrapper);
    }

    function createSection(title) {
        var el = document.createElement('div');
        el.style.cssText = 'border:1px solid ' + CONFIG.colors.border + ';border-radius:6px;overflow:hidden;margin-bottom:12px;';
        var hdr = document.createElement('div');
        hdr.style.cssText = 'padding:8px 12px;background:' + CONFIG.colors.headerBg +
            ';font-size:11px;color:' + CONFIG.colors.accent + ';font-weight:bold;text-transform:uppercase;letter-spacing:0.5px;';
        hdr.textContent = title;
        el.appendChild(hdr);
        var body = document.createElement('div');
        body.style.cssText = 'padding:12px;background:' + CONFIG.colors.background + ';';
        el.appendChild(body);
        return { el: el, body: body };
    }

    function inputStyle() {
        return 'padding:6px 10px;background:#0f0f1a;color:#e0e0e0;border:1px solid ' + CONFIG.colors.border +
            ';border-radius:4px;font-size:12px;font-family:"SF Mono",monospace;';
    }

    function resizeCanvases() {
        if (!state.latencyCanvas || !state.latencyCanvas.parentElement) return;
        var dpr = window.devicePixelRatio || 1;
        var rect = state.latencyCanvas.parentElement.getBoundingClientRect();
        var w = rect.width || 400;
        var h = 220;
        state.latencyCanvas.width = w * dpr;
        state.latencyCanvas.height = h * dpr;
        state.latencyCanvas.style.width = w + 'px';
        state.latencyCanvas.style.height = h + 'px';
        state.latencyCtx.setTransform(dpr, 0, 0, dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================
    function update(chaosData) {
        if (!state.initialized || !chaosData) return;

        if (chaosData.rules) { state.rules = chaosData.rules; renderRules(); }
        if (chaosData.events) { state.events = chaosData.events.slice(0, CONFIG.maxEvents); renderEvents(); }
        if (chaosData.audit_trail) { state.auditTrail = chaosData.audit_trail.slice(0, CONFIG.maxAudit); renderAuditTrail(); }
        if (chaosData.game_day) {
            state.gameDay = chaosData.game_day;
            var pb = document.getElementById('chaos-progress');
            if (pb) pb.style.width = (state.gameDay.progress || 0) + '%';
        }
        if (chaosData.latency_data) state.latencyData = chaosData.latency_data;
        if (chaosData.chaos_annotations) state.chaosAnnotations = chaosData.chaos_annotations;
    }

    // ========================================================================
    // Rendering - Rules Table
    // ========================================================================
    function renderRules() {
        if (!state.rulesBody) return;
        state.rulesBody.innerHTML = '';

        for (var i = 0; i < state.rules.length; i++) {
            var r = state.rules[i];
            var statusColor = CONFIG.statusColors[r.status] || CONFIG.colors.textMuted;
            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px">' + (r.src_svc || '--') + '</td>' +
                '<td style="padding:4px 8px">' + (r.dst_svc || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.danger + '">' + (r.fault_type || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + (r.duration || '--') + '</td>' +
                '<td style="padding:4px 8px;text-align:right">' + ((r.drop_rate || 0) * 100).toFixed(1) + '%</td>' +
                '<td style="padding:4px 8px">' + (r.created_by || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + statusColor + ';font-weight:bold;font-size:10px">' +
                (r.status || '--') + '</td>';
            state.rulesBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Rendering - Events
    // ========================================================================
    function renderEvents() {
        if (!state.eventList) return;
        state.eventList.innerHTML = '';

        for (var i = 0; i < state.events.length; i++) {
            var ev = state.events[i];
            var row = document.createElement('div');
            row.style.cssText = 'display:flex;gap:8px;padding:4px 8px;font-size:11px;border-bottom:1px solid ' +
                CONFIG.colors.border + ';';
            row.innerHTML =
                '<span style="width:70px;color:' + CONFIG.colors.textMuted + '">' + formatTime(ev.timestamp) + '</span>' +
                '<span style="color:' + CONFIG.colors.danger + ';width:80px;font-weight:bold;font-size:10px">' +
                (ev.type || '--') + '</span>' +
                '<span>' + (ev.src_svc || '--') + ' &rarr; ' + (ev.dst_svc || '--') + '</span>' +
                '<span style="color:' + CONFIG.colors.textMuted + ';margin-left:auto">' + (ev.detail || '') + '</span>';
            state.eventList.appendChild(row);
        }
    }

    // ========================================================================
    // Rendering - Audit Trail
    // ========================================================================
    function renderAuditTrail() {
        if (!state.auditBody) return;
        state.auditBody.innerHTML = '';

        var filter = state.auditFilter.toLowerCase();
        for (var i = 0; i < state.auditTrail.length; i++) {
            var a = state.auditTrail[i];
            var text = (a.user || '') + ' ' + (a.action || '') + ' ' + (a.rule_details || '');
            if (filter && text.toLowerCase().indexOf(filter) === -1) continue;

            var tr = document.createElement('tr');
            tr.style.cssText = 'border-bottom:1px solid ' + CONFIG.colors.border + ';';
            tr.innerHTML =
                '<td style="padding:4px 8px">' + (a.user || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.accent + '">' + (a.action || '--') + '</td>' +
                '<td style="padding:4px 8px;color:' + CONFIG.colors.textMuted + '">' + formatTime(a.timestamp) + '</td>' +
                '<td style="padding:4px 8px;font-size:10px">' + (a.rule_details || '--') + '</td>';
            state.auditBody.appendChild(tr);
        }
    }

    // ========================================================================
    // Rendering - Latency Overlay
    // ========================================================================
    function renderLatencyOverlay() {
        var ctx = state.latencyCtx;
        if (!ctx || !state.latencyCanvas) return;
        var w = state.latencyCanvas.width / (window.devicePixelRatio || 1);
        var h = state.latencyCanvas.height / (window.devicePixelRatio || 1);
        var pad = CONFIG.padding;

        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var data = state.latencyData;
        if (data.length < 2) {
            ctx.fillStyle = CONFIG.colors.textMuted;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for latency data...', w / 2, h / 2);
            return;
        }

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        var yMax = 0;
        for (var i = 0; i < data.length; i++) {
            if (data[i].latency_ms > yMax) yMax = data[i].latency_ms;
        }
        yMax = Math.ceil(yMax * 1.2) || 50;

        // Chaos annotation bands
        for (var a = 0; a < state.chaosAnnotations.length; a++) {
            var ann = state.chaosAnnotations[a];
            // Find closest data index
            var idx = findClosestIndex(data, ann.timestamp);
            var ax = pad.left + (idx / (data.length - 1)) * plotW;
            ctx.fillStyle = CONFIG.colors.chaosMarker;
            ctx.fillRect(ax - 2, pad.top, 4, plotH);
            ctx.fillStyle = CONFIG.colors.danger;
            ctx.font = '9px "SF Mono", monospace';
            ctx.textAlign = 'center';
            ctx.fillText(ann.label || 'chaos', ax, pad.top - 4);
        }

        // Grid
        ctx.strokeStyle = CONFIG.colors.gridLine;
        ctx.lineWidth = 1;
        for (var g = 1; g < 4; g++) {
            var gy = pad.top + plotH * (1 - g / 4);
            ctx.beginPath(); ctx.moveTo(pad.left, gy); ctx.lineTo(pad.left + plotW, gy); ctx.stroke();
        }

        // Latency line
        ctx.strokeStyle = CONFIG.colors.latencyLine;
        ctx.lineWidth = 2;
        ctx.lineJoin = 'round';
        ctx.beginPath();
        for (var j = 0; j < data.length; j++) {
            var x = pad.left + (j / (data.length - 1)) * plotW;
            var y = pad.top + plotH - (data[j].latency_ms / yMax) * plotH;
            if (j === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
        }
        ctx.stroke();
    }

    function findClosestIndex(data, ts) {
        var closest = 0;
        var minDist = Infinity;
        for (var i = 0; i < data.length; i++) {
            var dist = Math.abs((data[i].timestamp || 0) - ts);
            if (dist < minDist) { minDist = dist; closest = i; }
        }
        return closest;
    }

    // ========================================================================
    // Event Handlers
    // ========================================================================
    function onInjectClick() {
        var src = elVal('chaos-src_svc');
        var dst = elVal('chaos-dst_svc');
        var faultType = elVal('chaos-fault-type');
        var params = elVal('chaos-params');

        if (!src || !dst || !faultType) {
            alert('Please fill in Source, Destination, and Fault Type.');
            return;
        }

        if (state.onInject) {
            state.onInject({ src_svc: src, dst_svc: dst, fault_type: faultType, params: params });
        }
    }

    function onKillSwitchClick() {
        if (confirm('DANGER: This will stop ALL active chaos injections. Are you sure?')) {
            if (state.onKillSwitch) state.onKillSwitch();
        }
    }

    function toggleGameDay(start) {
        var scenario = elVal('chaos-scenario');
        if (start && !scenario) {
            alert('Select a scenario first.');
            return;
        }
        state.gameDay.running = start;
        state.gameDay.scenario = scenario;
        if (!start) {
            state.gameDay.progress = 0;
            var pb = document.getElementById('chaos-progress');
            if (pb) pb.style.width = '0%';
        }
    }

    function elVal(id) {
        var el = document.getElementById(id);
        return el ? el.value : '';
    }

    // ========================================================================
    // Animation
    // ========================================================================
    function startAnimation() {
        function tick() { renderLatencyOverlay(); state.animationFrame = requestAnimationFrame(tick); }
        state.animationFrame = requestAnimationFrame(tick);
    }

    function stopAnimation() {
        if (state.animationFrame) { cancelAnimationFrame(state.animationFrame); state.animationFrame = null; }
    }

    // ========================================================================
    // Utilities
    // ========================================================================
    function formatTime(ts) {
        if (!ts) return '--:--:--';
        var d = new Date(typeof ts === 'number' ? ts : Date.parse(ts));
        return d.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }

    function debounce(fn, wait) {
        var t;
        return function() { var a = arguments, c = this; clearTimeout(t); t = setTimeout(function() { fn.apply(c, a); }, wait); };
    }

    // ========================================================================
    // Cleanup
    // ========================================================================
    function destroy() {
        stopAnimation();
        if (state.container) state.container.innerHTML = '';
        state.rules = [];
        state.events = [];
        state.auditTrail = [];
        state.latencyData = [];
        state.chaosAnnotations = [];
        state.initialized = false;
    }

    // ========================================================================
    // Public API
    // ========================================================================
    return {
        init: init,
        update: update,
        destroy: destroy,
        setInjectCallback: function(cb) { state.onInject = cb; },
        setKillSwitchCallback: function(cb) { state.onKillSwitch = cb; },
        isInitialized: function() { return state.initialized; }
    };
})();

window.ChaosPanel = ChaosPanel;
console.log('chaos-panel.js loaded');
