// SPDX-License-Identifier: GPL-3.0-or-later
// Host selector labels, from /api/v1/hosts (east :20000, 2026-10-23: both
// hosts are read through their huginn; the dashboard runs on east too).
// Run: node hosts.cjs ../static/dashboard.js
const h = require('./harness.cjs');
h.run({
  routes: {
    '/api/v1/hosts': () => ({ hosts: [{ id: 'west', addr: '192.168.13.2', online: true }, { id: 'east', addr: '192.168.13.1', online: false }] }),
  },
  steps: async ({ check, prop }) => {
    const labels = (prop('host-selector', 'children') || []).map(o => o.textContent);
    check('west is not labelled local', labels[0], 'west'); // was "west (local)"
    check('offline host says so', labels[1], 'east [offline]');
  }
});
