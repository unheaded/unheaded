// SPDX-License-Identifier: GPL-3.0-or-later
// Latency page numbers: global percentiles, empty operations.
// Run: node latency.cjs ../static/dashboard.js
const h = require('./harness.cjs');
const win = (n, p50, p99) => [1e9, 1e10, 6e10].map(w => ({ window: w, sample_count: n, p50_ns: p50 * 1e6, p90_ns: p99 * 1e6, p99_ns: p99 * 1e6, min_ns: p50 * 1e6, max_ns: p99 * 1e6, mean_ns: p50 * 1e6 }));
let latency = {
  percentiles: { tcp_send: win(99, 1, 1), tcp_connect: win(1, 100, 100), tcp_accept: win(0, 0, 0) },
  combined: [1e9, 1e10, 6e10].map(w => ({ window: w, sample_count: 100, p50_ns: 1e6, p99_ns: 1e6 })),
};
h.run({
  routes: { '/api/v1/latency': () => latency },
  steps: async ({ intervals, check, settle, clickTab, html }) => {
    clickTab('latency');
    intervals.refreshLatency[0](); await settle();
    const tile = label => { const m = html('latency-summary-grid').match(new RegExp(label + '</div><div class="lat-kpi-value"[^>]*>([^<]*)<')); return m ? m[1] : '(missing)'; };
    // 99 samples at 1 ms and 1 at 100 ms: p50 and p99 are both 1 ms.
    check('global p50 is a percentile of all samples', tile('Global p50'), '1.00 ms'); // was 1.99 (weighted mean of op p50s)
    check('global p99 is a percentile of all samples', tile('Global p99'), '1.00 ms'); // was 100.00 (max op p99)
    check('an op with no samples is not active', tile('Active ops'), '2');
    check('ops over SLO counts only ops with samples', /\/ 2$/.test(tile('Ops over SLO')), true);
    check('hero is the worst op with samples', /tcp_connect/.test(html('latency-hero')), true);

    latency = { percentiles: { tcp_send: win(0, 0, 0), tcp_recv: win(0, 0, 0) }, combined: [] };
    intervals.refreshLatency[0](); await settle();
    check('no samples anywhere says so', /No latency data yet/.test(html('latency-hero')), true); // was "0.00 ms"
  }
});
