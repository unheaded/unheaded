// SPDX-License-Identifier: GPL-3.0-or-later
// Stub DOM, fetch, WebSocket, timers and clock for running the real
// static/dashboard.js under node. No browser, no dependencies.
const fs = require('fs');
const src = fs.readFileSync(process.argv[2], 'utf8');
let now = 1_000_000;
const RealDate = Date;
global.Date = class extends RealDate { constructor(...a) { a.length ? super(...a) : super(now); } static now() { return now; } };
const els = {};
function mkEl(id) {
  const target = { id, textContent: '', innerHTML: '', style: {}, dataset: {}, classList: { add(){}, remove(){}, toggle(){}, contains(){ return false; } },
    children: [], value: '', width: 800, height: 600 };
  return new Proxy(target, { get(t, k) {
    if (k in t) return t[k];
    if (k === 'getContext') return () => new Proxy({}, { get: () => () => ({ addColorStop(){} }) , set: () => true });
    if (k === 'getBoundingClientRect') return () => ({ width: 800, height: 600, left: 0, top: 0 });
    return typeof k === 'string' ? (() => mkEl(id + '.' + k)) : undefined;
  }, set(t, k, v) {
    t[k] = v;
    // As in a browser: textContent in, escaped innerHTML out (esc() relies on it).
    if (k === 'textContent') t.innerHTML = String(v).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return true;
  } });
}
// One fake nav tab; clickTab(page) runs its click handler as that page's tab.
const tabHandlers = [];
const navTab = { dataset: { page: '' }, classList: { toggle(){}, add(){}, remove(){} },
  addEventListener: (ev, fn) => { if (ev === 'click') tabHandlers.push(fn); } };
global.document = { readyState: 'complete',
  getElementById: id => (els[id] ||= mkEl(id)),
  querySelector: s => mkEl(s),
  querySelectorAll: s => s === '.nav-tab' ? [navTab] : [],
  createElement: t => mkEl(t),
  addEventListener(){}, body: mkEl('body'), documentElement: mkEl('html') };
global.window = { addEventListener(){}, location: { protocol: 'http:', host: 'x', hash: '' }, devicePixelRatio: 1,
  requestAnimationFrame(){}, matchMedia: () => ({ matches: false, addEventListener(){} }) };
global.location = window.location; global.requestAnimationFrame = () => 0; global.localStorage = { getItem(){ return null; }, setItem(){} };
global.navigator = {};
const intervals = {}; const fetched = [];
global.setInterval = (fn, ms) => { (intervals[fn.name || ('anon' + ms)] ||= []).push(fn); return 1; };
global.setTimeout = () => 1; global.clearTimeout = () => {}; global.clearInterval = () => {};
let ws; global.WebSocket = class { constructor(u) { ws = this; } send(){} close(){} };
let routes = {};
global.fetch = url => { fetched.push(url);
  const key = Object.keys(routes).find(k => url.startsWith(k));
  const body = key ? routes[key](url) : {};
  return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(body) }); };
const tick = () => new Promise(r => setImmediate(r));
async function settle() { for (let i = 0; i < 5; i++) await tick(); }
const text = id => (els[id] ? String(els[id].textContent) : '(none)');
let fails = 0;
const check = (name, got, want) => { const ok = got === want; if (!ok) fails++; console.log((ok ? 'ok   ' : 'FAIL ') + name + ': ' + JSON.stringify(got) + (ok ? '' : ' want ' + JSON.stringify(want))); };
module.exports = { run: async (setup) => { routes = setup.routes; eval(src); await settle(); await setup.steps({ intervals, fetched, text, check, settle, ws: () => ws, advance: ms => { now += ms; }, setRoute: (k, f) => { routes[k] = f; },
    clickTab: page => tabHandlers.forEach(fn => fn.call({ dataset: { page } })), html: id => (els[id] ? String(els[id].innerHTML) : '(none)') }); console.log(fails ? `${fails} FAILED` : 'ALL OK'); process.exit(fails ? 1 : 0); } };
