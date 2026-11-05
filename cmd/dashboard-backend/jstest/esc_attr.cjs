// SPDX-License-Identifier: GPL-3.0-or-later
// L9: esc() output is safe inside a quoted attribute as well as in text.
// Run: node esc_attr.cjs ../static/dashboard.js
const h = require('./harness.cjs');
const fs = 'x" onmouseover="y';
h.run({
  routes: {
    '/api/v1/hosts': () => ({ hosts: [{ id: 'west', addr: '192.168.13.2', online: true, disks: [{ mount: '/', filesystem: fs, use_percent: 1 }] }] }),
  },
  steps: async ({ check, html }) => {
    const out = html('sys-disks');
    check('the disks rendered', out.includes('disk-device'), true);
    check('a quote cannot end the title attribute', out.includes('" onmouseover='), false);
    check('the name is still shown', out.includes('x&quot; onmouseover=&quot;y'), true);
  }
});
