// SPDX-License-Identifier: GPL-3.0-or-later
// Events feed summaries for live eBPF WebSocket events, in the shapes the
// ingestor sends (internal/ebpf/types.go; records from east 2026-10-23).
// Run: node ebpf_summary.cjs ../static/dashboard.js
const h = require('./harness.cjs');
h.run({
  routes: {},
  steps: async ({ check, ws, advance, clickTab, prop }) => {
    clickTab('events');
    const last = () => { const items = prop('event-stream-list', 'children') || []; const m = String(items[0] && items[0].innerHTML).match(/event-stream-message">([^<]*)</); return m ? m[1] : '(none)'; };
    let seq = 0;
    const send = (type, data) => { advance(2000); ws().onmessage({ data: JSON.stringify({ type: 'ebpf_' + type, seq: ++seq, data }) }); };
    const key = { src_addr: '127.0.0.1', dst_addr: '127.0.0.1', src_port: 19005, dst_port: 51738, protocol: 6 };

    send('flow', { timestamp_ns: 857754687850080, flow_key: key, flow_state: { state: 'established', packets_in: 3 }, event_type: 'new' });
    check('flow summary', last(), '127.0.0.1:19005 → 127.0.0.1:51738 TCP [established] new'); // was "?: → ?: []"

    send('latency', { timestamp_ns: 1, latency_ns: 98879, operation: 'tcp_send', pid: 0, tid: 0 });
    check('latency summary', last(), 'tcp_send 98.9μs'); // was "tcp_send 0μs"

    send('syscall', { timestamp_ns: 1, pid: 4242, comm: 'curl', syscall_nr: 59 });
    check('syscall summary', last(), 'curl syscall 59 pid=4242'); // was "? pid=4242"

    // Packets are sampled 1 in 1000 for the feed.
    for (let i = 0; i < 1000; i++) send('packet', { timestamp_ns: 888379526520896, flow_key: { ...key, protocol: 17 }, packet_len: 96, action: 'pass', direction: 'ingress' });
    check('packet summary', last(), '127.0.0.1:19005 → 127.0.0.1:51738 UDP 96B'); // was "? → ? "
  }
});
