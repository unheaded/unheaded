// SPDX-License-Identifier: GPL-3.0-or-later
// Overview service counts across a live health_update WebSocket message,
// which carries one service's change (internal/health HealthEvent), not the
// system counts.
// Run: node health.cjs ../static/dashboard.js
const h = require('./harness.cjs');
let health = { status: 'unhealthy', total_services: 6, healthy_count: 5, degraded_count: 0, unhealthy_count: 1, unknown_count: 0 };
h.run({
  routes: {
    '/api/v1/health': () => health,
    '/api/v1/services': () => ({ services: [] }),
  },
  steps: async ({ text, check, settle, ws, fetched }) => {
    check('total from /health', text('total-services'), '6');
    check('unhealthy from /health', text('unhealthy-services'), '1');
    // gateway recovers: the server's counts move with it
    health = { ...health, healthy_count: 6, unhealthy_count: 0 };
    const before = fetched.filter(u => u === '/api/v1/health').length;
    ws().onmessage({ data: JSON.stringify({ type: 'health_update', data: { service: 'gateway', old_status: 'unhealthy', new_status: 'healthy', timestamp: '2026-10-23T06:15:18Z' } }) });
    await settle();
    check('health_update refetches the counts', fetched.filter(u => u === '/api/v1/health').length, before + 1);
    check('total is not zeroed', text('total-services'), '6'); // was "0"
    check('healthy is the server count', text('healthy-services'), '6'); // was "0"
    check('unhealthy is the server count', text('unhealthy-services'), '0');
  }
});
