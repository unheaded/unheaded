// SPDX-License-Identifier: GPL-3.0-or-later
// script: ../static/js/log-viewer.js
// L9: every field of a log line lands on the page as text, never as markup.
// Run: node log_escape.cjs ../static/js/log-viewer.js
const h = require('./harness.cjs');
const sources = [];
global.EventSource = class { constructor(url) { this.url = url; sources.push(this); } close() { this.closed = true; } };
h.run({
  routes: { '/api/v1/logs': () => [] },
  steps: async ({ check, prop }) => {
    const mark = '<form id="x">';
    const tail = sources[sources.length - 1];
    tail.onmessage({ data: JSON.stringify({ service: mark, level: mark, message: mark }) });
    const lines = prop('log-output', 'children');
    const html = String(lines[lines.length - 1].innerHTML);
    check('no field reaches the page as markup', html.includes('<form'), false); // service and level did
    check('the service is still shown', html.includes('[&lt;form'), true);
    check('quotes are escaped too', html.includes('id=&quot;x&quot;'), true);
  }
});
