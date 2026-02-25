/**
 * latency-chart.js - Real-time Latency Histogram / Time-Series Chart
 * Canvas-based chart displaying RTT distribution over time.
 *
 * Features:
 *   - Time-series display of RTT distribution
 *   - Show p50, p90, p99 percentile lines
 *   - Auto-scrolling X axis (last 60 seconds)
 *   - Highlight anomalies (>3 sigma from mean)
 *   - Canvas-based for performance
 *   - Responsive
 *
 * No external dependencies. Vanilla JS + Canvas API.
 */

var LatencyChart = (function() {
    'use strict';

    // ========================================================================
    // Configuration
    // ========================================================================
    var CONFIG = {
        windowSeconds: 60,      // X axis shows last 60 seconds
        bucketMs: 1000,         // 1 second per bucket
        maxDataPoints: 60,      // 60 data points (1 per second for 60s)
        anomalyThreshold: 3,    // >3 sigma
        padding: { top: 30, right: 20, bottom: 40, left: 60 },
        fps: 30,

        colors: {
            background: '#0f0f1a',
            gridLine: 'rgba(255, 215, 0, 0.06)',
            axisLine: 'rgba(255, 215, 0, 0.2)',
            axisLabel: '#6b6b7b',
            p50Line: '#00d26a',
            p90Line: '#ff9800',
            p99Line: '#ff4757',
            areaFill: 'rgba(255, 215, 0, 0.08)',
            dataPoint: '#ffd700',
            anomalyPoint: '#ff4757',
            anomalyGlow: 'rgba(255, 71, 87, 0.4)',
            legendText: '#a1a1aa',
            meanLine: 'rgba(78, 205, 196, 0.5)',
            sigmaRegion: 'rgba(255, 215, 0, 0.03)'
        }
    };

    // ========================================================================
    // State
    // ========================================================================
    var state = {
        canvas: null,
        ctx: null,
        container: null,
        width: 0,
        height: 0,
        dpr: 1,

        // Data storage: array of buckets, each bucket = { timestamp, values: [rtt,...] }
        buckets: [],
        currentBucket: null,
        currentBucketTime: 0,

        // Computed stats
        percentiles: {
            p50: [],   // one per bucket
            p90: [],
            p99: []
        },
        mean: 0,
        stddev: 0,
        anomalies: [],  // indices of anomaly data points

        animationFrame: null,
        initialized: false
    };

    // ========================================================================
    // Initialization
    // ========================================================================
    function init(containerId) {
        var container = document.getElementById(containerId || 'latency-chart-container');
        if (!container) {
            console.error('[LatencyChart] Container not found:', containerId);
            return false;
        }

        state.container = container;
        container.innerHTML = '';
        container.style.position = 'relative';

        // Create canvas
        state.canvas = document.createElement('canvas');
        state.canvas.className = 'latency-chart-canvas';
        state.canvas.setAttribute('role', 'img');
        state.canvas.setAttribute('aria-label', 'Real-time latency histogram showing RTT distribution over the last 60 seconds');
        container.appendChild(state.canvas);

        state.ctx = state.canvas.getContext('2d');
        state.dpr = window.devicePixelRatio || 1;

        // Legend
        var legend = document.createElement('div');
        legend.className = 'latency-chart-legend';
        legend.innerHTML =
            '<span class="latency-legend-item"><span class="latency-legend-dot" style="background:#00d26a;"></span>p50</span>' +
            '<span class="latency-legend-item"><span class="latency-legend-dot" style="background:#ff9800;"></span>p90</span>' +
            '<span class="latency-legend-item"><span class="latency-legend-dot" style="background:#ff4757;"></span>p99</span>' +
            '<span class="latency-legend-item"><span class="latency-legend-dot" style="background:rgba(78,205,196,0.7);"></span>mean</span>' +
            '<span class="latency-legend-item"><span class="latency-legend-dot latency-legend-anomaly"></span>anomaly (&gt;3&sigma;)</span>';
        container.appendChild(legend);

        // Stats summary
        var statsBar = document.createElement('div');
        statsBar.className = 'latency-chart-stats';
        statsBar.id = 'latency-chart-stats';
        statsBar.innerHTML =
            '<span class="latency-stat" id="lc-stat-p50">p50: --</span>' +
            '<span class="latency-stat" id="lc-stat-p90">p90: --</span>' +
            '<span class="latency-stat" id="lc-stat-p99">p99: --</span>' +
            '<span class="latency-stat" id="lc-stat-mean">mean: --</span>' +
            '<span class="latency-stat" id="lc-stat-samples">samples: 0</span>';
        container.appendChild(statsBar);

        resizeCanvas();
        window.addEventListener('resize', debounce(resizeCanvas, 150));

        // Start render loop
        startAnimation();

        state.initialized = true;
        console.log('[LatencyChart] Initialized');
        return true;
    }

    function resizeCanvas() {
        if (!state.canvas || !state.container) return;
        var rect = state.container.getBoundingClientRect();
        state.width = rect.width || 800;
        state.height = Math.max(250, rect.height - 80) || 300; // leave room for legend/stats

        state.canvas.width = state.width * state.dpr;
        state.canvas.height = state.height * state.dpr;
        state.canvas.style.width = state.width + 'px';
        state.canvas.style.height = state.height + 'px';
        state.ctx.setTransform(state.dpr, 0, 0, state.dpr, 0, 0);
    }

    // ========================================================================
    // Data Input
    // ========================================================================

    /**
     * Add a latency sample (in milliseconds).
     * Called from the WebSocket message handler for each trace event.
     */
    function addSample(rttMs) {
        if (!state.initialized) return;
        if (typeof rttMs !== 'number' || rttMs < 0) return;

        var now = Date.now();
        var bucketTime = Math.floor(now / CONFIG.bucketMs) * CONFIG.bucketMs;

        // Check if we need a new bucket
        if (!state.currentBucket || state.currentBucketTime !== bucketTime) {
            // Finalize previous bucket
            if (state.currentBucket && state.currentBucket.values.length > 0) {
                finalizeBucket(state.currentBucket);
            }

            state.currentBucket = {
                timestamp: bucketTime,
                values: []
            };
            state.currentBucketTime = bucketTime;
        }

        state.currentBucket.values.push(rttMs);
    }

    /**
     * Add multiple samples from an array of trace events.
     */
    function addSamples(rttArray) {
        if (!Array.isArray(rttArray)) return;
        for (var i = 0; i < rttArray.length; i++) {
            addSample(rttArray[i]);
        }
    }

    function finalizeBucket(bucket) {
        if (!bucket || bucket.values.length === 0) return;

        var sorted = bucket.values.slice().sort(function(a, b) { return a - b; });
        var stats = {
            timestamp: bucket.timestamp,
            count: sorted.length,
            p50: percentile(sorted, 0.50),
            p90: percentile(sorted, 0.90),
            p99: percentile(sorted, 0.99),
            mean: mean(sorted),
            max: sorted[sorted.length - 1],
            min: sorted[0]
        };

        state.buckets.push(stats);

        // Trim to window size
        if (state.buckets.length > CONFIG.maxDataPoints) {
            state.buckets.shift();
        }

        // Recompute global stats
        recomputeGlobalStats();
    }

    function recomputeGlobalStats() {
        if (state.buckets.length === 0) return;

        var allP50 = [];
        var allP99 = [];
        var allMeans = [];
        var totalSamples = 0;

        for (var i = 0; i < state.buckets.length; i++) {
            allP50.push(state.buckets[i].p50);
            allP99.push(state.buckets[i].p99);
            allMeans.push(state.buckets[i].mean);
            totalSamples += state.buckets[i].count;
        }

        state.mean = mean(allMeans);
        state.stddev = stddev(allMeans);

        // Detect anomalies
        state.anomalies = [];
        var threshold = state.mean + CONFIG.anomalyThreshold * state.stddev;
        for (var j = 0; j < state.buckets.length; j++) {
            if (state.buckets[j].p99 > threshold && threshold > 0) {
                state.anomalies.push(j);
            }
        }

        // Update stats display
        var latestBucket = state.buckets[state.buckets.length - 1];
        updateStatEl('lc-stat-p50', 'p50: ' + latestBucket.p50.toFixed(2) + 'ms');
        updateStatEl('lc-stat-p90', 'p90: ' + latestBucket.p90.toFixed(2) + 'ms');
        updateStatEl('lc-stat-p99', 'p99: ' + latestBucket.p99.toFixed(2) + 'ms');
        updateStatEl('lc-stat-mean', 'mean: ' + state.mean.toFixed(2) + 'ms');
        updateStatEl('lc-stat-samples', 'samples: ' + totalSamples);
    }

    function updateStatEl(id, text) {
        var el = document.getElementById(id);
        if (el) el.textContent = text;
    }

    // ========================================================================
    // Animation & Rendering
    // ========================================================================

    function startAnimation() {
        function animate() {
            // Finalize current bucket if stale (>1s old)
            if (state.currentBucket && state.currentBucket.values.length > 0) {
                var now = Date.now();
                var bucketTime = Math.floor(now / CONFIG.bucketMs) * CONFIG.bucketMs;
                if (state.currentBucketTime < bucketTime) {
                    finalizeBucket(state.currentBucket);
                    state.currentBucket = null;
                    state.currentBucketTime = 0;
                }
            }

            render();
            state.animationFrame = requestAnimationFrame(animate);
        }

        state.animationFrame = requestAnimationFrame(animate);
    }

    function stopAnimation() {
        if (state.animationFrame) {
            cancelAnimationFrame(state.animationFrame);
            state.animationFrame = null;
        }
    }

    function render() {
        var ctx = state.ctx;
        var w = state.width;
        var h = state.height;
        var pad = CONFIG.padding;

        // Clear
        ctx.fillStyle = CONFIG.colors.background;
        ctx.fillRect(0, 0, w, h);

        var plotW = w - pad.left - pad.right;
        var plotH = h - pad.top - pad.bottom;

        if (state.buckets.length < 2) {
            // Empty state
            ctx.fillStyle = CONFIG.colors.axisLabel;
            ctx.font = '12px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('Waiting for latency data...', w / 2, h / 2);
            drawAxes(ctx, pad, plotW, plotH, 0, 10);
            return;
        }

        // Find Y range (max latency)
        var yMax = 0;
        for (var i = 0; i < state.buckets.length; i++) {
            if (state.buckets[i].p99 > yMax) yMax = state.buckets[i].p99;
            if (state.buckets[i].max > yMax) yMax = state.buckets[i].max;
        }
        yMax = Math.ceil(yMax * 1.2) || 10; // 20% headroom

        // Draw axes and grid
        drawAxes(ctx, pad, plotW, plotH, 0, yMax);

        // Draw sigma region (mean +/- 3 sigma)
        if (state.mean > 0 && state.stddev > 0) {
            var sigmaLow = Math.max(0, state.mean - CONFIG.anomalyThreshold * state.stddev);
            var sigmaHigh = state.mean + CONFIG.anomalyThreshold * state.stddev;
            var sy1 = pad.top + plotH - (sigmaHigh / yMax) * plotH;
            var sy2 = pad.top + plotH - (sigmaLow / yMax) * plotH;

            ctx.fillStyle = CONFIG.colors.sigmaRegion;
            ctx.fillRect(pad.left, Math.max(pad.top, sy1), plotW, Math.min(sy2 - sy1, plotH));
        }

        // Draw mean line
        if (state.mean > 0) {
            var meanY = pad.top + plotH - (state.mean / yMax) * plotH;
            ctx.setLineDash([4, 4]);
            ctx.strokeStyle = CONFIG.colors.meanLine;
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(pad.left, meanY);
            ctx.lineTo(pad.left + plotW, meanY);
            ctx.stroke();
            ctx.setLineDash([]);
        }

        // Draw p50, p90, p99 lines
        drawPercentileLine(ctx, pad, plotW, plotH, yMax, 'p50', CONFIG.colors.p50Line, 1.5);
        drawPercentileLine(ctx, pad, plotW, plotH, yMax, 'p90', CONFIG.colors.p90Line, 1.5);
        drawPercentileLine(ctx, pad, plotW, plotH, yMax, 'p99', CONFIG.colors.p99Line, 2);

        // Draw area fill under p99
        drawAreaFill(ctx, pad, plotW, plotH, yMax, 'p99', CONFIG.colors.areaFill);

        // Draw anomaly markers
        drawAnomalies(ctx, pad, plotW, plotH, yMax);
    }

    function drawAxes(ctx, pad, plotW, plotH, yMin, yMax) {
        // Y axis
        ctx.strokeStyle = CONFIG.colors.axisLine;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top);
        ctx.lineTo(pad.left, pad.top + plotH);
        ctx.stroke();

        // X axis
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top + plotH);
        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.stroke();

        // Y axis labels and grid
        var ySteps = 5;
        ctx.font = '10px "SF Mono", monospace';
        ctx.fillStyle = CONFIG.colors.axisLabel;
        ctx.textAlign = 'right';

        for (var i = 0; i <= ySteps; i++) {
            var yVal = yMin + (yMax - yMin) * (i / ySteps);
            var yPos = pad.top + plotH - (i / ySteps) * plotH;

            // Grid line
            if (i > 0 && i < ySteps) {
                ctx.strokeStyle = CONFIG.colors.gridLine;
                ctx.beginPath();
                ctx.moveTo(pad.left, yPos);
                ctx.lineTo(pad.left + plotW, yPos);
                ctx.stroke();
            }

            // Label
            ctx.fillText(yVal.toFixed(1) + 'ms', pad.left - 6, yPos + 4);
        }

        // X axis labels (time)
        ctx.textAlign = 'center';
        if (state.buckets.length > 1) {
            var xStep = Math.max(1, Math.floor(state.buckets.length / 6));
            for (var j = 0; j < state.buckets.length; j += xStep) {
                var xPos = pad.left + (j / (state.buckets.length - 1)) * plotW;
                var t = new Date(state.buckets[j].timestamp);
                var label = t.toLocaleTimeString('en-US', { hour12: false, minute: '2-digit', second: '2-digit' });
                ctx.fillText(label, xPos, pad.top + plotH + 16);
            }
        }

        // Y axis title
        ctx.save();
        ctx.translate(14, pad.top + plotH / 2);
        ctx.rotate(-Math.PI / 2);
        ctx.textAlign = 'center';
        ctx.fillStyle = CONFIG.colors.axisLabel;
        ctx.font = '10px -apple-system, sans-serif';
        ctx.fillText('Latency (ms)', 0, 0);
        ctx.restore();
    }

    function drawPercentileLine(ctx, pad, plotW, plotH, yMax, key, color, lineWidth) {
        if (state.buckets.length < 2) return;

        ctx.strokeStyle = color;
        ctx.lineWidth = lineWidth;
        ctx.lineJoin = 'round';
        ctx.lineCap = 'round';
        ctx.beginPath();

        for (var i = 0; i < state.buckets.length; i++) {
            var x = pad.left + (i / (state.buckets.length - 1)) * plotW;
            var y = pad.top + plotH - (state.buckets[i][key] / yMax) * plotH;

            if (i === 0) {
                ctx.moveTo(x, y);
            } else {
                ctx.lineTo(x, y);
            }
        }

        ctx.stroke();

        // Draw dots at each data point
        ctx.fillStyle = color;
        for (var j = 0; j < state.buckets.length; j++) {
            var dx = pad.left + (j / (state.buckets.length - 1)) * plotW;
            var dy = pad.top + plotH - (state.buckets[j][key] / yMax) * plotH;
            ctx.beginPath();
            ctx.arc(dx, dy, 2, 0, Math.PI * 2);
            ctx.fill();
        }
    }

    function drawAreaFill(ctx, pad, plotW, plotH, yMax, key, color) {
        if (state.buckets.length < 2) return;

        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.moveTo(pad.left, pad.top + plotH);

        for (var i = 0; i < state.buckets.length; i++) {
            var x = pad.left + (i / (state.buckets.length - 1)) * plotW;
            var y = pad.top + plotH - (state.buckets[i][key] / yMax) * plotH;
            ctx.lineTo(x, y);
        }

        ctx.lineTo(pad.left + plotW, pad.top + plotH);
        ctx.closePath();
        ctx.fill();
    }

    function drawAnomalies(ctx, pad, plotW, plotH, yMax) {
        for (var i = 0; i < state.anomalies.length; i++) {
            var idx = state.anomalies[i];
            if (idx >= state.buckets.length) continue;

            var x = pad.left + (idx / (state.buckets.length - 1)) * plotW;
            var y = pad.top + plotH - (state.buckets[idx].p99 / yMax) * plotH;

            // Glow
            ctx.fillStyle = CONFIG.colors.anomalyGlow;
            ctx.beginPath();
            ctx.arc(x, y, 10, 0, Math.PI * 2);
            ctx.fill();

            // Point
            ctx.fillStyle = CONFIG.colors.anomalyPoint;
            ctx.beginPath();
            ctx.arc(x, y, 5, 0, Math.PI * 2);
            ctx.fill();

            // Exclamation
            ctx.fillStyle = '#fff';
            ctx.font = 'bold 8px -apple-system, sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('!', x, y + 3);
        }
    }

    // ========================================================================
    // Math Utilities
    // ========================================================================

    function percentile(sorted, p) {
        if (!sorted || sorted.length === 0) return 0;
        var idx = Math.ceil(p * sorted.length) - 1;
        if (idx < 0) idx = 0;
        return sorted[idx];
    }

    function mean(arr) {
        if (!arr || arr.length === 0) return 0;
        var sum = 0;
        for (var i = 0; i < arr.length; i++) sum += arr[i];
        return sum / arr.length;
    }

    function stddev(arr) {
        if (!arr || arr.length < 2) return 0;
        var m = mean(arr);
        var sqDiffSum = 0;
        for (var i = 0; i < arr.length; i++) {
            sqDiffSum += (arr[i] - m) * (arr[i] - m);
        }
        return Math.sqrt(sqDiffSum / arr.length);
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
        stopAnimation();
        if (state.container) state.container.innerHTML = '';
        state.buckets = [];
        state.currentBucket = null;
        state.anomalies = [];
        state.initialized = false;
    }

    function clear() {
        state.buckets = [];
        state.currentBucket = null;
        state.currentBucketTime = 0;
        state.anomalies = [];
        state.mean = 0;
        state.stddev = 0;
    }

    // ========================================================================
    // Public API
    // ========================================================================
    return {
        init: init,
        addSample: addSample,
        addSamples: addSamples,
        clear: clear,
        destroy: destroy,
        getBuckets: function() { return state.buckets.slice(); },
        getStats: function() {
            return {
                mean: state.mean,
                stddev: state.stddev,
                anomalyCount: state.anomalies.length,
                bucketCount: state.buckets.length
            };
        },
        isInitialized: function() { return state.initialized; }
    };
})();

// Make available globally
window.LatencyChart = LatencyChart;

console.log('latency-chart.js loaded');
