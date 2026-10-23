// SPDX-License-Identifier: GPL-3.0-or-later
// No eBPF ingestor: /api/v1/ebpf/stats says active:false and /api/v1/flows
// answers 503 (bodies as east :20000 sent them 2026-10-23). The page must
// say there is no eBPF data, not show a row of zeros.
// Run: node ebpf_inactive.cjs ../static/dashboard.js
const h = require('./harness.cjs');
let stats = { active: false, message: 'eBPF ingestor not active. Start trace-collector to get real stats.' };
let flows = { __status: 503, error: 'ebpf ingestor not active', detail: 'Start trace-collector with --wotan-grpc-addr to enable eBPF flow data.' };
h.run({
  routes: {
    '/api/v1/ebpf/stats': () => stats,
    '/api/v1/flows': () => flows,
  },
  steps: async ({ intervals, text, check, settle, prop }) => {
    check('packets is not 0', text('stat-packets'), '--'); // was "0"
    check('eBPF events card is not 0', text('ebpf-events-count'), '--'); // was "0"
    check('events/sec is not 0', text('stat-eps'), '--');
    check('events page total is not 0', text('event-stream-total'), '--'); // was "0"
    check('reason on hover', prop('stat-packets', 'title'), stats.message);
    check('active flows is not 0', text('active-flows-count'), '--'); // was left at the page's "0"
    check('flow graph count is not 0', text('flow-graph-count'), '--');
    check('flow reason on hover', /ebpf ingestor not active — Start trace-collector/.test(prop('active-flows-count', 'title')), true);

    // The ingestor comes up: real numbers replace the dashes.
    stats = { active: true, stats: { packets_ingested: 1000, uptime_seconds: 5, flow_stats: { active_flows: 2 } } };
    flows = { source: 'ebpf', active_flows: [{ key: { src_addr: 'a', dst_addr: 'b' } }, { key: { src_addr: 'c', dst_addr: 'd' } }], stats: { total_packets: 1, total_bytes: 1 } };
    intervals.refreshEBPFStats[0](); intervals.refreshFlows[0](); await settle();
    check('packets once active', text('stat-packets'), '1.0K');
    check('active flows once active', text('active-flows-count'), '2');
    check('flow reason cleared', prop('active-flows-count', 'title'), '');
  }
});
