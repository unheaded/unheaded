// SPDX-License-Identifier: GPL-3.0-or-later
// Overview numbers: eBPF event paging, Events/sec, uptime, Active Flows.
// Run: node overview.cjs ../static/dashboard.js
const h = require('./harness.cjs');
const ev = s => ({ seq: s, topic: 'ebpf.flow.events', type: 'flow', timestamp: '2026-09-29T00:00:00Z', data: {} });
let lastSeq = 3, ignoreCursor = process.env.OLD_SERVER === '1';
let stats = { packets_ingested: 1000, uptime_seconds: 100, flow_stats: { active_flows: 2 } };
h.run({
  routes: {
    '/api/v1/ebpf/events': url => {
      const after = +(new URL('http://x' + url).searchParams.get('after_seq') || 0);
      const all = []; for (let s = lastSeq; s >= 1; s--) all.push(ev(s));
      const events = ignoreCursor ? all : all.filter(e => e.seq > after);
      return { events, count: events.length, last_seq: lastSeq };
    },
    '/api/v1/ebpf/stats': () => ({ active: true, stats }),
    '/api/v1/flows': () => ({ source: 'ebpf', active_flows: [{ src_ip: 'a', dst_ip: 'b' }, { src_ip: 'c', dst_ip: 'd' }], stats: { active_flows: 2 } }),
  },
  steps: async ({ intervals, fetched, text, check, settle, ws, advance, clickTab, rows }) => {
    // Feed rows on the Events page: each event must appear once, whichever
    // path (poll or WS) brings it. Advance the clock so the feed's
    // rows-per-second cap never hides a row.
    clickTab('events');
    const feed = () => rows('event-stream-list');
    const poll = async () => { advance(2000); intervals.refreshEBPFEvents[0](); await settle(); };
    for (let i = 0; i < 5; i++) await poll();
    check('5 repeat polls add no rows', feed(), 0);
    check('poll cursor sent', fetched.filter(u => u.startsWith('/api/v1/ebpf/events')).pop(), '/api/v1/ebpf/events?limit=1000&after_seq=3');
    const msg = (s) => { advance(2000); ws().onmessage({ data: JSON.stringify({ type: 'ebpf_flow', seq: s, data: {} }) }); };
    msg(3); check('WS event already polled is not shown again', feed(), 0);
    msg(4); check('new WS event shown once', feed(), 1);
    lastSeq = 4; await poll(); check('poll after WS adds nothing', feed(), 1);
    // backend restart: seqs start again
    lastSeq = 1; await poll();
    await poll();
    check('after rewind polls from 0', fetched.filter(u => u.startsWith('/api/v1/ebpf/events')).pop(), '/api/v1/ebpf/events?limit=1000&after_seq=0');
    check('post-restart event shown', feed(), 2);

    // Events/sec + uptime
    check('first stats poll eps unknown', text('stat-eps'), '--');
    check('uptime from backend', text('stat-uptime'), '1m 40s');
    intervals.refreshEBPFStats[0](); await settle(); // fresh baseline after the clock moves above
    advance(2000); stats = { ...stats, packets_ingested: 1010, uptime_seconds: 102 };
    intervals.refreshEBPFStats[0](); await settle();
    check('eps is delta over time', text('stat-eps'), '5.0');
    // Every browser reads the same Events-page numbers: the server's.
    check('events total is the server total', text('event-stream-total'), '1.0K');
    check('events rate is the server rate', text('event-stream-rate'), '5.0');
    intervals.updateTimestamp[0]();
    check('header uptime is backend uptime', text('uptime'), 'Uptime: 1m 42s');
    check('header clock is labelled as the browser clock', text('server-time').startsWith('Local: '), true);

    // Active flows card ignores WS hop list
    intervals.refreshFlows[0](); await settle();
    check('active flows from backend', text('active-flows-count'), '2');
    // A stray packet_flow (old backend) must not turn packets into flows.
    for (let i = 0; i < 5; i++) ws().onmessage({ data: JSON.stringify({ type: 'packet_flow', data: { source: 'a', destination: 'b', size: 60 } }) });
    check('packet_flow leaves the card alone', text('active-flows-count'), '2');
    check('packet_flow leaves the flow graph count alone', text('flow-graph-count'), '2');
  }
});
