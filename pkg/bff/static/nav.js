// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.
//
// The Kingdom's menu, one copy for every page (pkg/bff.ServeNavJS serves it
// at /auth/nav.js on the dashboard, Kanban, the wiki and the Zhen front).
// The apps all live on the host the page was loaded from, each on its own
// port, so a link is built from location.hostname and the port registry
// (pkg/ports): no page carries an address. Fills every element with a
// data-kingdom-nav attribute, whose value is the current app's id, with a
// link to each other app:
//
//   <span data-kingdom-nav="kanban"></span>
//   <div data-kingdom-nav="wiki" data-kingdom-nav-class="" data-kingdom-nav-sep=" | "></div>
//
// data-kingdom-nav-class is the class of each link (default "nav-tab
// nav-link"); data-kingdom-nav-sep is text put between links. DOM calls and
// textContent only; nothing from the page is interpreted as markup.
(function () {
  'use strict';
  var apps = __NAV__;
  var host = location.hostname;
  if (!host) {
    return;
  }
  function fill(el) {
    var here = el.getAttribute('data-kingdom-nav');
    var cls = el.hasAttribute('data-kingdom-nav-class') ? el.getAttribute('data-kingdom-nav-class') : 'nav-tab nav-link';
    var sep = el.getAttribute('data-kingdom-nav-sep') || '';
    var frag = document.createDocumentFragment();
    apps.forEach(function (a) {
      if (a.id === here) {
        return;
      }
      if (sep && frag.childNodes.length) {
        frag.appendChild(document.createTextNode(sep));
      }
      var link = document.createElement('a');
      if (cls) {
        link.className = cls;
      }
      link.href = a.scheme + '://' + host + ':' + a.port + '/';
      link.textContent = a.name;
      frag.appendChild(link);
    });
    while (el.firstChild) {
      el.removeChild(el.firstChild);
    }
    el.appendChild(frag);
    // The links sit in the page's own flex row as if they were its children.
    if (el.tagName === 'SPAN') {
      el.style.display = 'contents';
    }
  }
  function run() {
    var els = document.querySelectorAll('[data-kingdom-nav]');
    for (var i = 0; i < els.length; i++) {
      fill(els[i]);
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', run);
  } else {
    run();
  }
})();
