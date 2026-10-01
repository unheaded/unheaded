// SPDX-License-Identifier: GPL-3.0-or-later
// script: ../static/js/compliance.js
// Compliance page (ADR-097/098/099): the Profiles panel and the banner that
// names any part served from the repo files instead of The Well.
// Run: node compliance.cjs ../static/js/compliance.js
const h = require('./harness.cjs');
const fallback = { evidence: 'file-fallback', register: 'yaml-fallback', reason: 'set WELL_OPS_PASSWORD' };
let summary = { evidence_available: true, evidence_records: 3, frameworks: [], controls: [], source: { evidence: 'file-fallback', reason: 'set WELL_OPS_PASSWORD' } };
let findings = { open: [], counts: {}, resolved: [], unobserved: [], stale: [], zero_claimable: true, source: fallback };
let profiles = {
  source: 'yaml-fallback', reason: 'set WELL_CONFIG_PASSWORD',
  fixes: [{ id: 'kmod-cis', title: 'k', risk: 'none', class: 'kmod', controls: [] }, { id: 'sshd-safe', title: 's', risk: 'high', class: 'sshd', controls: [] }],
  profiles: [{ name: 'none', description: '', fixes: [] }, { name: 'lab', description: '', fixes: ['kmod-cis'] }, { name: 'all', description: '', fixes: ['kmod-cis', 'sshd-safe'] }],
  bindings: [{ host: 'west', profile: 'lab', mode: 'audit' }, { host: 'east', profile: 'lab', mode: 'audit' }],
};
h.run({
  routes: {
    '/api/v1/compliance/summary': () => summary,
    '/api/v1/compliance/findings': () => findings,
    '/api/v1/compliance/profiles': () => profiles,
  },
  steps: async ({ intervals, check, settle, rows, text, prop }) => {
    await settle();
    check('one row per host', rows('#host-table tbody'), 2);
    check('one row per fix', rows('#prof-table tbody'), 2);
    check('banner shown on fallback', prop('cx-source', 'hidden'), false);
    const banner = text('cx-source-text');
    check('banner names each fallback part', /evidence: file-fallback/.test(banner) && /findings triage: yaml-fallback/.test(banner) && /profiles: yaml-fallback/.test(banner), true);
    check('banner gives the reasons', /WELL_OPS_PASSWORD/.test(banner) && /WELL_CONFIG_PASSWORD/.test(banner), true);
    check('profiles say their source', text('profiles-source'), 'Source: repository files (yaml-fallback).');

    // Everything from The Well: no banner.
    summary = Object.assign({}, summary, { source: { evidence: 'well' } });
    findings = Object.assign({}, findings, { source: { evidence: 'well', register: 'well' } });
    profiles = Object.assign({}, profiles, { source: 'well', reason: '' });
    intervals.load[0](); await settle();
    check('banner hidden when all from The Well', prop('cx-source', 'hidden'), true);
    check('profiles from The Well', text('profiles-source'), 'Source: The Well.');
  }
});
