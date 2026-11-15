// SPDX-License-Identifier: GPL-3.0-or-later
// A logged-out page stops: every API answers 401 (the session ended), and
// the page made one round of nine fetches and then nothing, however long
// it stays open; its socket is closed and not reopened, and the status says
// why. It polled on: 197,379 answers 401 of 351,818 in a week (ADR-147,
// Developer D13).
// Run: node logged_out.cjs ../static/dashboard.js
const h = require('./harness.cjs');
h.run({
  routes: { '/api/v1/': () => ({ __status: 401 }) },
  steps: async ({ fetched, text, check, advance, settle, ws, fireIntervals, liveIntervals }) => {
    const first = fetched.length;
    check('the first round', first, 9);
    for (let i = 0; i < 2; i++) { // past the longest refresh interval (15 s), twice
      advance(16000);
      fireIntervals();
      await settle();
    }
    check('no fetch after the 401s', fetched.length, first);
    check('every interval cleared', liveIntervals(), 0);
    check('the socket closed', ws().closed, true);
    check('the status says so', text('ws-status-text'), 'Logged out: log in and reload');
  }
});
