// SPDX-License-Identifier: GPL-3.0-or-later
// script: ../static/js/log-viewer.js
// Logs page live tail (/ws/logs SSE) follows the page's filters: service and
// level reopen the stream with them, the search box applies to live lines.
// Run: node logs.cjs ../static/js/log-viewer.js
const h = require('./harness.cjs');
const sources = [];
global.EventSource = class { constructor(url) { this.url = url; sources.push(this); } close() { this.closed = true; } };
global.setTimeout = fn => { fn(); return 1; }; // run the search debounce now
const on = {};
for (const id of ['log-service', 'log-level', 'log-search']) {
  document.getElementById(id).addEventListener = (ev, fn) => { on[id + ':' + ev] = fn; };
}
h.run({
  routes: { '/api/v1/logs': () => [{ timestamp: '2026-10-23T06:00:00Z', service: 'wotan', level: 'info', message: 'health check OK' }] },
  steps: async ({ check, settle, rows, text }) => {
    const live = () => sources[sources.length - 1];
    check('tail opens unfiltered', live().url, '/ws/logs?');
    check('a REST answer does not claim the tail is connected', text('log-status') === 'connected', false);
    document.getElementById('log-service').value = 'wotan';
    on['log-service:change'](); await settle();
    check('service change reopens the tail filtered', live().url, '/ws/logs?service=wotan'); // was still "/ws/logs?"
    check('old tail closed', sources[0].closed, true);
    document.getElementById('log-level').value = 'error';
    on['log-level:change'](); await settle();
    check('level change reopens the tail filtered', live().url, '/ws/logs?service=wotan&level=error');

    document.getElementById('log-search').value = 'BOOM';
    on['log-search:input'](); await settle();
    const before = rows('log-output');
    live().onmessage({ data: JSON.stringify({ service: 'wotan', level: 'error', message: 'disk fine' }) });
    check('live line not matching the search is not shown', rows('log-output'), before); // was before+1
    live().onmessage({ data: JSON.stringify({ service: 'wotan', level: 'error', message: 'x', fields: { err: 'boom: eof' } }) });
    check('live line matching the search is shown', rows('log-output'), before + 1);
  }
});
