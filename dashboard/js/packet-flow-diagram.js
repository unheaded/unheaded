/**
 * packet-flow-diagram.js - SVG Network Topology View
 * SVG-based node graph showing services as circles with animated traffic lines.
 *
 * Features:
 *   - Services rendered as circles in a force-directed layout
 *   - Animated lines between nodes representing packet flow
 *   - Line thickness proportional to traffic volume
 *   - Color-coded by latency
 *   - Click node to see service health details
 *   - Simple force-directed auto-layout
 *
 * No external dependencies. Vanilla JS + inline SVG.
 */

var PacketFlowDiagram = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        nodeRadius: 30,
        labelOffset: 42,
        animationInterval: 50,    // ms between layout ticks
        layoutIterations: 100,
        forceStrength: 0.02,
        repulsionStrength: 8000,
        linkDistance: 180,
        damping: 0.85,
        centerGravity: 0.01,

        // Colors
        colors: {
            nodeFill: '#16213e',
            nodeStroke: 'rgba(255, 215, 0, 0.5)',
            nodeStrokeHover: '#ffd700',
            nodeText: '#e0e0e0',
            linkDefault: 'rgba(255, 215, 0, 0.15)',
            linkActive: 'rgba(255, 215, 0, 0.6)',
            background: 'transparent',
            latencyGood: '#00d26a',
            latencyWarning: '#ff9800',
            latencyCritical: '#ff4757',
            particleColor: '#ffd700'
        },

        // Service definitions (mirrors packet-flow.js)
        services: {
            gateway:      { name: 'Gateway',      color: '#ffd700', icon: 'G' },
            wotan:        { name: 'Wotan',         color: '#4ecdc4', icon: 'W' },
            timeguru:     { name: 'Timeguru',      color: '#45b7d1', icon: 'T' },
            captain:      { name: 'Captain',       color: '#a29bfe', icon: 'C' },
            architect:    { name: 'Architect',     color: '#fd79a8', icon: 'A' },
            micromanager: { name: 'Micromanager',  color: '#6c5ce7', icon: 'M' },
            monad:        { name: 'Monad',         color: '#00cec9', icon: 'Mo' },
            sophia:       { name: 'Sophia',        color: '#e17055', icon: 'S' }
        },

        // Connection topology
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
    var state = {
        svg: null,
        container: null,
        width: 0,
        height: 0,
        nodes: [],          // { id, x, y, vx, vy, service }
        links: [],          // { source, target, traffic, latency }
        particles: [],      // animated dots along links
        selectedNode: null,
        detailPanel: null,
        layoutTimer: null,
        animTimer: null,
        trafficCounts: {},  // key -> count (recent traffic volume)
        latencyMap: {},     // key -> avg latency
        initialized: false
    };

    // SVG namespace
    var SVG_NS = 'http://www.w3.org/2000/svg';

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(containerId) {
        var container = document.getElementById(containerId || 'packet-flow-diagram-container');
        if (!container) {
            console.error('[PacketFlowDiagram] Container not found:', containerId);
            return false;
        }

        state.container = container;
        container.innerHTML = '';
        container.style.position = 'relative';

        var rect = container.getBoundingClientRect();
        state.width = rect.width || 800;
        state.height = rect.height || 500;

        // Create SVG element
        state.svg = document.createElementNS(SVG_NS, 'svg');
        state.svg.setAttribute('width', '100%');
        state.svg.setAttribute('height', '100%');
        state.svg.setAttribute('viewBox', '0 0 ' + state.width + ' ' + state.height);
        state.svg.setAttribute('class', 'pfd-svg');
        state.svg.setAttribute('role', 'img');
        state.svg.setAttribute('aria-label', 'Network topology diagram showing service connections');
        container.appendChild(state.svg);

        // Add gradient defs
        addSvgDefs();

        // Create link group (behind nodes)
        var linkGroup = document.createElementNS(SVG_NS, 'g');
        linkGroup.setAttribute('class', 'pfd-links');
        linkGroup.id = 'pfd-links';
        state.svg.appendChild(linkGroup);

        // Create particle group
        var particleGroup = document.createElementNS(SVG_NS, 'g');
        particleGroup.setAttribute('class', 'pfd-particles');
        particleGroup.id = 'pfd-particles';
        state.svg.appendChild(particleGroup);

        // Create node group (on top)
        var nodeGroup = document.createElementNS(SVG_NS, 'g');
        nodeGroup.setAttribute('class', 'pfd-nodes');
        nodeGroup.id = 'pfd-nodes';
        state.svg.appendChild(nodeGroup);

        // Create detail panel
        createDetailPanel(container);

        // Initialize nodes
        initNodes();

        // Initialize links
        initLinks();

        // Run force layout
        runLayout();

        // Render
        renderNodes();
        renderLinks();

        // Start particle animation
        state.animTimer = setInterval(animateParticles, CONFIG.animationInterval);

        // Handle resize
        window.addEventListener('resize', debounce(handleResize, 200));

        state.initialized = true;
        console.log('[PacketFlowDiagram] Initialized');
        return true;
    }

    function addSvgDefs() {
        var defs = document.createElementNS(SVG_NS, 'defs');

        // Glow filter
        var filter = document.createElementNS(SVG_NS, 'filter');
        filter.setAttribute('id', 'pfd-glow');
        filter.setAttribute('x', '-50%');
        filter.setAttribute('y', '-50%');
        filter.setAttribute('width', '200%');
        filter.setAttribute('height', '200%');

        var blur = document.createElementNS(SVG_NS, 'feGaussianBlur');
        blur.setAttribute('stdDeviation', '4');
        blur.setAttribute('result', 'coloredBlur');
        filter.appendChild(blur);

        var merge = document.createElementNS(SVG_NS, 'feMerge');
        var mergeNode1 = document.createElementNS(SVG_NS, 'feMergeNode');
        mergeNode1.setAttribute('in', 'coloredBlur');
        merge.appendChild(mergeNode1);
        var mergeNode2 = document.createElementNS(SVG_NS, 'feMergeNode');
        mergeNode2.setAttribute('in', 'SourceGraphic');
        merge.appendChild(mergeNode2);
        filter.appendChild(merge);

        defs.appendChild(filter);
        state.svg.appendChild(defs);
    }

    function createDetailPanel(container) {
        var panel = document.createElement('div');
        panel.className = 'pfd-detail-panel';
        panel.id = 'pfd-detail-panel';
        panel.setAttribute('role', 'dialog');
        panel.setAttribute('aria-label', 'Service detail');
        panel.setAttribute('aria-hidden', 'true');
        panel.innerHTML =
            '<div class="pfd-detail-header">' +
                '<h4 class="pfd-detail-title" id="pfd-detail-title">Service</h4>' +
                '<button class="pfd-detail-close" id="pfd-detail-close" aria-label="Close">&times;</button>' +
            '</div>' +
            '<div class="pfd-detail-body" id="pfd-detail-body"></div>';
        container.appendChild(panel);
        state.detailPanel = panel;

        var closeBtn = document.getElementById('pfd-detail-close');
        if (closeBtn) {
            closeBtn.addEventListener('click', function() {
                closeDetailPanel();
            });
        }
    }

    // ========================================================================
    // Node / Link Setup
    // ========================================================================

    function initNodes() {
        state.nodes = [];
        var keys = Object.keys(CONFIG.services);
        var cx = state.width / 2;
        var cy = state.height / 2;
        var radius = Math.min(state.width, state.height) * 0.35;

        for (var i = 0; i < keys.length; i++) {
            var id = keys[i];
            var service = CONFIG.services[id];
            // Arrange in a circle initially
            var angle = (2 * Math.PI * i) / keys.length - Math.PI / 2;
            state.nodes.push({
                id: id,
                service: service,
                x: cx + radius * Math.cos(angle),
                y: cy + radius * Math.sin(angle),
                vx: 0,
                vy: 0
            });
        }
    }

    function initLinks() {
        state.links = [];
        for (var i = 0; i < CONFIG.connections.length; i++) {
            var conn = CONFIG.connections[i];
            state.links.push({
                source: conn[0],
                target: conn[1],
                traffic: 0,
                latency: 0
            });
        }
    }

    function getNode(id) {
        for (var i = 0; i < state.nodes.length; i++) {
            if (state.nodes[i].id === id) return state.nodes[i];
        }
        return null;
    }

    // ========================================================================
    // Force-Directed Layout
    // ========================================================================

    function runLayout() {
        for (var iter = 0; iter < CONFIG.layoutIterations; iter++) {
            layoutTick();
        }
    }

    function layoutTick() {
        var nodes = state.nodes;
        var links = state.links;
        var width = state.width;
        var height = state.height;

        // Repulsion between all node pairs
        for (var i = 0; i < nodes.length; i++) {
            for (var j = i + 1; j < nodes.length; j++) {
                var dx = nodes[j].x - nodes[i].x;
                var dy = nodes[j].y - nodes[i].y;
                var dist = Math.sqrt(dx * dx + dy * dy) || 1;
                var force = CONFIG.repulsionStrength / (dist * dist);

                var fx = (dx / dist) * force;
                var fy = (dy / dist) * force;

                nodes[i].vx -= fx;
                nodes[i].vy -= fy;
                nodes[j].vx += fx;
                nodes[j].vy += fy;
            }
        }

        // Attraction along links
        for (var k = 0; k < links.length; k++) {
            var sourceNode = getNode(links[k].source);
            var targetNode = getNode(links[k].target);
            if (!sourceNode || !targetNode) continue;

            var dx2 = targetNode.x - sourceNode.x;
            var dy2 = targetNode.y - sourceNode.y;
            var dist2 = Math.sqrt(dx2 * dx2 + dy2 * dy2) || 1;
            var springForce = CONFIG.forceStrength * (dist2 - CONFIG.linkDistance);

            var fx2 = (dx2 / dist2) * springForce;
            var fy2 = (dy2 / dist2) * springForce;

            sourceNode.vx += fx2;
            sourceNode.vy += fy2;
            targetNode.vx -= fx2;
            targetNode.vy -= fy2;
        }

        // Center gravity
        var cx = width / 2;
        var cy = height / 2;
        for (var n = 0; n < nodes.length; n++) {
            nodes[n].vx += (cx - nodes[n].x) * CONFIG.centerGravity;
            nodes[n].vy += (cy - nodes[n].y) * CONFIG.centerGravity;
        }

        // Apply velocity with damping
        var padding = CONFIG.nodeRadius + 10;
        for (var m = 0; m < nodes.length; m++) {
            nodes[m].vx *= CONFIG.damping;
            nodes[m].vy *= CONFIG.damping;

            nodes[m].x += nodes[m].vx;
            nodes[m].y += nodes[m].vy;

            // Keep within bounds
            nodes[m].x = Math.max(padding, Math.min(width - padding, nodes[m].x));
            nodes[m].y = Math.max(padding, Math.min(height - padding, nodes[m].y));
        }
    }

    // ========================================================================
    // Rendering
    // ========================================================================

    function renderNodes() {
        var nodeGroup = document.getElementById('pfd-nodes');
        if (!nodeGroup) return;
        nodeGroup.innerHTML = '';

        for (var i = 0; i < state.nodes.length; i++) {
            var node = state.nodes[i];
            var g = document.createElementNS(SVG_NS, 'g');
            g.setAttribute('class', 'pfd-node');
            g.setAttribute('data-service', node.id);
            g.setAttribute('transform', 'translate(' + node.x + ',' + node.y + ')');
            g.setAttribute('tabindex', '0');
            g.setAttribute('role', 'button');
            g.setAttribute('aria-label', node.service.name + ' service node');

            // Outer glow circle
            var outerGlow = document.createElementNS(SVG_NS, 'circle');
            outerGlow.setAttribute('r', CONFIG.nodeRadius + 8);
            outerGlow.setAttribute('fill', node.service.color);
            outerGlow.setAttribute('opacity', '0.08');
            g.appendChild(outerGlow);

            // Main circle
            var circle = document.createElementNS(SVG_NS, 'circle');
            circle.setAttribute('r', CONFIG.nodeRadius);
            circle.setAttribute('fill', CONFIG.colors.nodeFill);
            circle.setAttribute('stroke', node.service.color);
            circle.setAttribute('stroke-width', '2.5');
            circle.setAttribute('class', 'pfd-node-circle');
            g.appendChild(circle);

            // Icon text
            var iconText = document.createElementNS(SVG_NS, 'text');
            iconText.setAttribute('text-anchor', 'middle');
            iconText.setAttribute('dominant-baseline', 'central');
            iconText.setAttribute('fill', node.service.color);
            iconText.setAttribute('font-size', '14');
            iconText.setAttribute('font-weight', 'bold');
            iconText.setAttribute('font-family', '-apple-system, sans-serif');
            iconText.textContent = node.service.icon || node.service.name.charAt(0);
            g.appendChild(iconText);

            // Label
            var label = document.createElementNS(SVG_NS, 'text');
            label.setAttribute('text-anchor', 'middle');
            label.setAttribute('y', CONFIG.labelOffset);
            label.setAttribute('fill', CONFIG.colors.nodeText);
            label.setAttribute('font-size', '11');
            label.setAttribute('font-family', '"SF Mono", monospace');
            label.setAttribute('class', 'pfd-node-label');
            label.textContent = node.service.name;
            g.appendChild(label);

            // Click handler
            (function(nodeRef) {
                g.addEventListener('click', function() {
                    selectNode(nodeRef);
                });
                g.addEventListener('keydown', function(e) {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        selectNode(nodeRef);
                    }
                });
            })(node);

            nodeGroup.appendChild(g);
        }
    }

    function renderLinks() {
        var linkGroup = document.getElementById('pfd-links');
        if (!linkGroup) return;
        linkGroup.innerHTML = '';

        for (var i = 0; i < state.links.length; i++) {
            var link = state.links[i];
            var sourceNode = getNode(link.source);
            var targetNode = getNode(link.target);
            if (!sourceNode || !targetNode) continue;

            var key = link.source + '-' + link.target;
            var traffic = state.trafficCounts[key] || 0;
            var latency = state.latencyMap[key] || 0;

            // Line thickness proportional to traffic (min 1, max 6)
            var strokeWidth = Math.max(1, Math.min(6, 1 + traffic * 0.5));
            var strokeColor = getLatencyColor(latency, traffic > 0);

            var line = document.createElementNS(SVG_NS, 'line');
            line.setAttribute('x1', sourceNode.x);
            line.setAttribute('y1', sourceNode.y);
            line.setAttribute('x2', targetNode.x);
            line.setAttribute('y2', targetNode.y);
            line.setAttribute('stroke', strokeColor);
            line.setAttribute('stroke-width', strokeWidth);
            line.setAttribute('class', 'pfd-link' + (traffic > 0 ? ' active' : ''));
            line.setAttribute('data-link', key);

            linkGroup.appendChild(line);
        }
    }

    function getLatencyColor(latency, isActive) {
        if (!isActive) return CONFIG.colors.linkDefault;
        if (latency <= 0) return CONFIG.colors.linkActive;
        if (latency < 1) return CONFIG.colors.latencyGood;
        if (latency < 10) return CONFIG.colors.latencyWarning;
        return CONFIG.colors.latencyCritical;
    }

    // ========================================================================
    // Particle Animation
    // ========================================================================

    function animateParticles() {
        var particleGroup = document.getElementById('pfd-particles');
        if (!particleGroup) return;

        // Update existing particles
        var toRemove = [];
        for (var i = 0; i < state.particles.length; i++) {
            var p = state.particles[i];
            p.progress += p.speed;

            if (p.progress >= 1) {
                toRemove.push(i);
                continue;
            }

            // Interpolate position
            var x = p.x1 + (p.x2 - p.x1) * p.progress;
            var y = p.y1 + (p.y2 - p.y1) * p.progress;

            if (p.element) {
                p.element.setAttribute('cx', x);
                p.element.setAttribute('cy', y);
                p.element.setAttribute('opacity', Math.sin(p.progress * Math.PI));
            }
        }

        // Remove completed particles (reverse order)
        for (var j = toRemove.length - 1; j >= 0; j--) {
            var idx = toRemove[j];
            if (state.particles[idx].element && state.particles[idx].element.parentNode) {
                state.particles[idx].element.parentNode.removeChild(state.particles[idx].element);
            }
            state.particles.splice(idx, 1);
        }

        // Decay traffic counts
        var keys = Object.keys(state.trafficCounts);
        for (var k = 0; k < keys.length; k++) {
            state.trafficCounts[keys[k]] *= 0.97;
            if (state.trafficCounts[keys[k]] < 0.01) {
                delete state.trafficCounts[keys[k]];
            }
        }

        // Update link styles in-place (no DOM rebuild)
        updateLinkStyles();
    }

    // Throttled link style update — modifies existing SVG attributes instead
    // of rebuilding all SVG elements (was renderLinks() every 50ms = DOM thrashing)
    function updateLinkStyles() {
        var linkGroup = document.getElementById('pfd-links');
        if (!linkGroup) return;

        var lines = linkGroup.querySelectorAll('.pfd-link');
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            var key = line.getAttribute('data-link');
            if (!key) continue;

            var traffic = state.trafficCounts[key] || 0;
            var latency = state.latencyMap[key] || 0;
            var strokeWidth = Math.max(1, Math.min(6, 1 + traffic * 0.5));
            var strokeColor = getLatencyColor(latency, traffic > 0);

            line.setAttribute('stroke', strokeColor);
            line.setAttribute('stroke-width', strokeWidth);

            if (traffic > 0) {
                line.classList.add('active');
            } else {
                line.classList.remove('active');
            }
        }
    }

    function spawnParticle(sourceId, targetId, color) {
        var sourceNode = getNode(sourceId);
        var targetNode = getNode(targetId);
        if (!sourceNode || !targetNode) return;

        var particleGroup = document.getElementById('pfd-particles');
        if (!particleGroup) return;

        var circle = document.createElementNS(SVG_NS, 'circle');
        circle.setAttribute('r', '4');
        circle.setAttribute('fill', color || CONFIG.colors.particleColor);
        circle.setAttribute('cx', sourceNode.x);
        circle.setAttribute('cy', sourceNode.y);
        circle.setAttribute('opacity', '0');
        circle.setAttribute('class', 'pfd-particle');
        particleGroup.appendChild(circle);

        state.particles.push({
            x1: sourceNode.x,
            y1: sourceNode.y,
            x2: targetNode.x,
            y2: targetNode.y,
            progress: 0,
            speed: 0.02 + Math.random() * 0.02,
            element: circle
        });

        // Limit particles
        if (state.particles.length > 100) {
            var old = state.particles.shift();
            if (old.element && old.element.parentNode) {
                old.element.parentNode.removeChild(old.element);
            }
        }
    }

    // ========================================================================
    // Service Detail
    // ========================================================================

    function selectNode(node) {
        state.selectedNode = node;

        // Highlight node
        var allNodes = state.svg.querySelectorAll('.pfd-node');
        for (var i = 0; i < allNodes.length; i++) {
            allNodes[i].classList.remove('selected');
            if (allNodes[i].getAttribute('data-service') === node.id) {
                allNodes[i].classList.add('selected');
            }
        }

        openDetailPanel(node);
    }

    function openDetailPanel(node) {
        if (!state.detailPanel) return;

        var titleEl = document.getElementById('pfd-detail-title');
        var bodyEl = document.getElementById('pfd-detail-body');
        if (titleEl) titleEl.textContent = node.service.name;

        // Get health info from DashboardMetrics if available
        var healthInfo = 'Unknown';
        var latencyInfo = 'N/A';
        if (window.DashboardMetrics && window.DashboardMetrics.getServices) {
            var services = window.DashboardMetrics.getServices();
            for (var i = 0; i < services.length; i++) {
                if (services[i].name === node.id || services[i].name === node.service.name) {
                    healthInfo = services[i].status || 'Unknown';
                    if (services[i].latency) latencyInfo = services[i].latency.toFixed(2) + 'ms';
                    if (services[i].average_latency_ms) latencyInfo = services[i].average_latency_ms.toFixed(2) + 'ms';
                    break;
                }
            }
        }

        // Count connections
        var connectionCount = 0;
        for (var j = 0; j < CONFIG.connections.length; j++) {
            if (CONFIG.connections[j][0] === node.id || CONFIG.connections[j][1] === node.id) {
                connectionCount++;
            }
        }

        // Recent traffic volume
        var trafficVol = 0;
        var trafficKeys = Object.keys(state.trafficCounts);
        for (var k = 0; k < trafficKeys.length; k++) {
            if (trafficKeys[k].indexOf(node.id) !== -1) {
                trafficVol += state.trafficCounts[trafficKeys[k]];
            }
        }

        if (bodyEl) {
            bodyEl.innerHTML =
                '<div class="pfd-detail-grid">' +
                    '<span class="pfd-detail-label">Service ID</span>' +
                    '<span class="pfd-detail-value font-mono">' + escapeHtml(node.id) + '</span>' +
                    '<span class="pfd-detail-label">Health</span>' +
                    '<span class="pfd-detail-value">' + escapeHtml(healthInfo) + '</span>' +
                    '<span class="pfd-detail-label">Avg Latency</span>' +
                    '<span class="pfd-detail-value">' + latencyInfo + '</span>' +
                    '<span class="pfd-detail-label">Connections</span>' +
                    '<span class="pfd-detail-value">' + connectionCount + '</span>' +
                    '<span class="pfd-detail-label">Traffic Volume</span>' +
                    '<span class="pfd-detail-value">' + trafficVol.toFixed(1) + '</span>' +
                    '<span class="pfd-detail-label">Node Color</span>' +
                    '<span class="pfd-detail-value"><span style="display:inline-block;width:12px;height:12px;border-radius:50%;background:' + node.service.color + ';margin-right:6px;vertical-align:middle;"></span>' + node.service.color + '</span>' +
                '</div>';
        }

        state.detailPanel.classList.add('open');
        state.detailPanel.setAttribute('aria-hidden', 'false');
    }

    function closeDetailPanel() {
        if (state.detailPanel) {
            state.detailPanel.classList.remove('open');
            state.detailPanel.setAttribute('aria-hidden', 'true');
        }
        state.selectedNode = null;

        var allNodes = state.svg.querySelectorAll('.pfd-node');
        for (var i = 0; i < allNodes.length; i++) {
            allNodes[i].classList.remove('selected');
        }
    }

    // ========================================================================
    // External Data Input
    // ========================================================================

    /**
     * Register a traffic event between two services.
     * Called from the WebSocket message handler for trace events.
     */
    function addTraffic(sourceId, targetId, latencyMs) {
        var key = sourceId + '-' + targetId;

        // Update traffic counts
        state.trafficCounts[key] = (state.trafficCounts[key] || 0) + 1;

        // Update latency map (running average)
        if (latencyMs > 0) {
            var oldLat = state.latencyMap[key] || 0;
            state.latencyMap[key] = oldLat > 0 ? (oldLat * 0.7 + latencyMs * 0.3) : latencyMs;
        }

        // Spawn animated particle
        var color = getLatencyColor(latencyMs, true);
        spawnParticle(sourceId, targetId, color);
    }

    /**
     * Process a flow event with hops.
     */
    function processFlow(flow) {
        if (!flow || !flow.hops || flow.hops.length < 2) return;

        var latencyMs = flow.total_time ? flow.total_time / 1e6 : 0;

        for (var i = 0; i < flow.hops.length - 1; i++) {
            var src = mapComponent(flow.hops[i].component);
            var dst = mapComponent(flow.hops[i + 1].component);
            if (src && dst && src !== dst) {
                addTraffic(src, dst, latencyMs);
            }
        }
    }

    function mapComponent(component) {
        var mapping = {
            'gateway': 'gateway',
            'wotan': 'wotan',
            'timeguru': 'timeguru',
            'captain': 'captain',
            'architect': 'architect',
            'micromanager': 'micromanager',
            'monad': 'monad',
            'sophia': 'sophia',
            'xdp_packet_marker': 'gateway',
            'trace-collector': 'wotan'
        };
        return mapping[component] || null;
    }

    // ========================================================================
    // Resize
    // ========================================================================

    function handleResize() {
        if (!state.container) return;
        var rect = state.container.getBoundingClientRect();
        state.width = rect.width || 800;
        state.height = rect.height || 500;

        if (state.svg) {
            state.svg.setAttribute('viewBox', '0 0 ' + state.width + ' ' + state.height);
        }

        // Re-run layout with new dimensions
        runLayout();
        renderNodes();
        renderLinks();
    }

    // ========================================================================
    // Utilities
    // ========================================================================

    function escapeHtml(text) {
        if (!text) return '';
        var div = document.createElement('div');
        div.textContent = String(text);
        return div.innerHTML;
    }

    function debounce(func, wait) {
        var timeout;
        return function() {
            var args = arguments;
            var context = this;
            clearTimeout(timeout);
            timeout = setTimeout(function() { func.apply(context, args); }, wait);
        };
    }

    // ========================================================================
    // Cleanup
    // ========================================================================

    function destroy() {
        if (state.animTimer) clearInterval(state.animTimer);
        if (state.layoutTimer) clearInterval(state.layoutTimer);
        if (state.container) state.container.innerHTML = '';
        state.initialized = false;
    }

    // ========================================================================
    // Public API
    // ========================================================================
    return {
        init: init,
        addTraffic: addTraffic,
        processFlow: processFlow,
        destroy: destroy,
        getNodes: function() { return state.nodes.slice(); },
        getTrafficCounts: function() { return Object.assign({}, state.trafficCounts); },
        isInitialized: function() { return state.initialized; }
    };
})();

// Make available globally
window.PacketFlowDiagram = PacketFlowDiagram;

console.log('packet-flow-diagram.js loaded');
