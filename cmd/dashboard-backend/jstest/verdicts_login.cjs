// SPDX-License-Identifier: GPL-3.0-or-later
// script: ../static/js/verdicts.js
// Verdicts page without a login, and with an answer that is not the
// inventory: it says so and draws nothing.
// Run: node verdicts_login.cjs ../static/js/verdicts.js
const h = require('./harness.cjs');
h.run({
  routes: { '/api/v1/verdicts': () => ({ __status: 401, error: 'unauthorized' }) },
  steps: async ({ check, settle, text }) => {
    await settle();
    check('says to log in', text('vd-wait-text'), 'Log in to see the inventory.');
    check('status', text('vd-status'), 'unavailable (401)');
    check('nothing drawn', global.document.getElementById('vd-sections').children.length, 0);
    check('counts never written', text('vd-n-total'), '(none)');
  }
});
