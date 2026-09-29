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
  steps: async ({ intervals, fetched, text, check, settle, ws, advance }) => {
    const rate = () => { intervals.anon1000[0](); return text('event-stream-rate'); };
    rate(); // absorb the initial poll's 3 events
    const poll = async () => { intervals.refreshEBPFEvents[0](); await settle(); };
    for (let i = 0; i < 5; i++) await poll();
    check('5 repeat polls add no events', rate(), '0');
    check('poll cursor sent', fetched.filter(u => u.startsWith('/api/v1/ebpf/events')).pop(), '/api/v1/ebpf/events?limit=1000&after_seq=3');
    const msg = (s) => ws().onmessage({ data: JSON.stringify({ type: 'ebpf_flow', seq: s, data: {} }) });
    msg(3); check('WS event already polled is not recounted', rate(), '0');
    msg(4); check('new WS event counted once', rate(), '1');
    lastSeq = 4; await poll(); check('poll after WS adds nothing', rate(), '0');
    // backend restart: seqs start again
    lastSeq = 1; await poll();
    check('restart rewinds cursor', fetched.filter(u => u.startsWith('/api/v1/ebpf/events')).length > 0, true);
    await poll();
    check('after rewind polls from 0', fetched.filter(u => u.startsWith('/api/v1/ebpf/events')).pop(), '/api/v1/ebpf/events?limit=1000&after_seq=0');
    check('post-restart event counted', rate(), '1');

    // Events/sec + uptime
    check('first stats poll eps unknown', text('stat-eps'), '--');
    check('uptime from backend', text('stat-uptime'), '1m 40s');
    advance(2000); stats = { ...stats, packets_ingested: 1010, uptime_seconds: 102 };
    intervals.refreshEBPFStats[0](); await settle();
    check('eps is delta over time', text('stat-eps'), '5.0');
    intervals.updateTimestamp[0]();
    check('header uptime is backend uptime', text('uptime'), 'Uptime: 1m 42s');
    check('header clock is labelled as the browser clock', text('server-time').startsWith('Local: '), true);

    // Active flows card ignores WS hop list
    intervals.refreshFlows[0](); await settle();
    check('active flows from backend', text('active-flows-count'), '2');
    for (let i = 0; i < 5; i++) ws().onmessage({ data: JSON.stringify({ type: 'packet_flow', data: { hops: [{ component: 'a' }, { component: 'b' }, { component: 'c' }] } }) });
    check('WS packet_flow leaves the card alone', text('active-flows-count'), '2');
  }
});
