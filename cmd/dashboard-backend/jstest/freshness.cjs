// SPDX-License-Identifier: GPL-3.0-or-later
// Header "last update": the time the backend last answered, marked stale
// when it stops answering; not the browser clock ticking every second.
// Run: node freshness.cjs ../static/dashboard.js
const h = require('./harness.cjs');
let down = false;
const t0 = new Date(1_000_000).toLocaleTimeString();
h.run({
  routes: {
    '/api/v1/health': () => (down ? { __status: 502 } : { total_services: 1, healthy_count: 1 }),
    '/api/v1/': () => (down ? { __status: 502 } : {}),
  },
  steps: async ({ intervals, text, check, advance, settle, ws }) => {
    check('time of the first answer', text('last-update'), t0);
    down = true;
    advance(60000);
    for (const k of Object.keys(intervals)) if (k !== 'updateTimestamp') intervals[k].forEach(fn => fn());
    await settle();
    intervals.updateTimestamp[0]();
    check('errors do not refresh it, and it says stale', text('last-update'), t0 + ' (stale)'); // was the current time
    ws().onmessage({ data: JSON.stringify({ type: 'event', data: {} }) });
    intervals.updateTimestamp[0]();
    check('a WS message is data', text('last-update'), new Date(1_060_000).toLocaleTimeString());
  }
});
