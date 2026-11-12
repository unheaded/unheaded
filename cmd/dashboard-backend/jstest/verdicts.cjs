// SPDX-License-Identifier: GPL-3.0-or-later
// script: ../static/js/verdicts.js
// Verdicts page (ADR-113, read-only): counts, one table per section, the
// verdict filter, the text filter, and inventory text shown as text.
// Run: node verdicts.cjs ../static/js/verdicts.js
const h = require('./harness.cjs');
const hostile = '<img src=x onerror=alert(1)>';
const item = (name, verdict, cls, where) => ({ name, verdict, class: cls, fields: [{ column: 'Version / tag', value: 'v1' }, { column: 'Where', value: where }] });
const inventory = {
  source: 'docs/policy/third-party-inventory-2026-10-29.md', title: 'Third-party inventory (2026-10-29)',
  counts: { total: 5, undecided: 2, keep: 1, replace: 1, cut: 1, other: 0 },
  sections: [
    { name: 'Go modules (direct)', columns: ['Version / tag', 'Where'], items: [
      item('github.com/rs/zerolog', 'keep', 'keep', 'go.mod'),
      item('modernc.org/sqlite', 'replace with our own', 'replace', 'go.mod'),
      item(hostile, '', 'undecided', 'go.mod') ] },
    { name: 'Container images', columns: ['Version / tag', 'Where'], items: [
      item('grafana/loki', 'cut', 'cut', 'monitoring/docker-compose.yml'),
      item('clickhouse/clickhouse-server', '', 'undecided', 'docker-compose.yml') ] },
  ],
};
const doc = global.document;
const sections = () => doc.getElementById('vd-sections').children;
const rowsOf = sec => sec.children[1].children[1].children; // section > table > tbody > rows
h.run({
  routes: { '/api/v1/verdicts': () => inventory },
  steps: async ({ check, settle, text, prop }) => {
    await settle();
    check('total', text('vd-n-total'), '5');
    check('undecided', text('vd-n-undecided'), '2');
    check('keep', text('vd-n-keep'), '1');
    check('replace', text('vd-n-replace'), '1');
    check('cut', text('vd-n-cut'), '1');
    check('status counts rulings', text('vd-status'), '3 of 5 ruled on');
    check('source named', text('vd-source'), 'docs/policy/third-party-inventory-2026-10-29.md');
    check('one table per section', sections().length, 2);
    check('every row shown', text('vd-shown'), '5 shown');
    check('bar draws rulings only', doc.getElementById('vd-bar').children.length, 3);
    const first = rowsOf(sections()[0]);
    check('rows of the first section', first.length, 3);
    check('a row is name, each column, verdict', first[0].children.length, 4);
    check('an empty verdict reads undecided', first[2].children[3].children[0].textContent, 'undecided');
    check('inventory text stays text', first[2].children[0].textContent, hostile);
    check('and is escaped as markup', /&lt;img/.test(first[2].children[0].innerHTML), true);

    // The verdict filter.
    prop('vd-f-undecided', 'onclick')();
    check('undecided: both sections still have one', sections().length, 2);
    check('undecided: count', text('vd-shown'), '2 of 5 shown');
    check('undecided: button lit', prop('vd-f-undecided', 'className'), 'on');
    check('undecided: all unlit', prop('vd-f-all', 'className'), '');
    prop('vd-f-cut', 'onclick')();
    check('cut: a section with no match is not drawn', sections().length, 1);
    check('cut: the row', rowsOf(sections()[0])[0].children[0].textContent, 'grafana/loki');
    prop('vd-f-other', 'onclick')();
    check('other: nothing', sections().length, 0);
    check('other: count', text('vd-shown'), '0 of 5 shown');

    // The text filter: every word, anywhere in the row, any case.
    prop('vd-f-all', 'onclick')();
    doc.getElementById('vd-q').value = '  SQLITE go.mod ';
    prop('vd-q', 'oninput')();
    check('query: one row', text('vd-shown'), '1 of 5 shown');
    check('query: the row', rowsOf(sections()[0])[0].children[0].textContent, 'modernc.org/sqlite');
    doc.getElementById('vd-q').value = 'compose';
    prop('vd-q', 'oninput')();
    check('query matches a field', text('vd-shown'), '2 of 5 shown');
    prop('vd-f-cut', 'onclick')();
    check('query and verdict together', text('vd-shown'), '1 of 5 shown');
    doc.getElementById('vd-q').value = '';
    prop('vd-q', 'oninput')();
    prop('vd-f-all', 'onclick')();
    check('cleared', text('vd-shown'), '5 shown');
  }
});
