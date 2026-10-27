// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// Browser half of the backend-for-frontend login (ADR-103 §4, pkg/bff).
// Served by the backend at /auth/auth.js; one copy for every page that
// loads it (dashboard, kanban).
//
// The page never sees a bearer token: the backend keeps it and the browser
// holds only an HttpOnly session cookie that JS cannot read. This file:
//   - adds "X-Requested-With: unheaded" to every same-origin fetch (the
//     backend refuses a cookie-authenticated write without it: CSRF);
//   - shows a login panel when an API call answers 401;
//   - shows who is logged in, with a logout button (from /auth/me).
// /auth/me answering 404 means auth is off on this service: nothing shows.
(function () {
  'use strict';

  var HEADER = 'X-Requested-With';
  var VALUE = 'unheaded';
  var nativeFetch = window.fetch.bind(window);

  function sameOrigin(input) {
    var raw = typeof input === 'string' ? input : (input && input.url) || String(input);
    try {
      return new URL(raw, location.href).origin === location.origin;
    } catch (_e) {
      return false;
    }
  }

  function pathOf(input) {
    var raw = typeof input === 'string' ? input : (input && input.url) || String(input);
    try {
      return new URL(raw, location.href).pathname;
    } catch (_e) {
      return '';
    }
  }

  function authFetch(input, init) {
    if (!sameOrigin(input)) {
      return nativeFetch(input, init);
    }
    var opts = Object.assign({}, init || {});
    var headers = new Headers(opts.headers || (input && input.headers) || undefined);
    if (!headers.has(HEADER)) {
      headers.set(HEADER, VALUE);
    }
    opts.headers = headers;
    if (!opts.credentials) {
      opts.credentials = 'same-origin';
    }
    return nativeFetch(input, opts).then(function (resp) {
      if (resp.status === 401 && pathOf(input).indexOf('/api/') === 0) {
        showLogin();
      }
      return resp;
    });
  }
  window.fetch = authFetch;

  // --- UI (built with DOM calls and textContent only) ----------------------

  var panel = null;
  var badge = null;

  function el(tag, style, text) {
    var n = document.createElement(tag);
    if (style) {
      Object.keys(style).forEach(function (k) { n.style[k] = style[k]; });
    }
    if (text) {
      n.textContent = text;
    }
    return n;
  }

  var inputStyle = {
    display: 'block', width: '100%', boxSizing: 'border-box', margin: '6px 0 12px',
    padding: '8px', borderRadius: '6px', border: '1px solid #556', background: '#0d1117', color: '#e6edf3'
  };
  var buttonStyle = {
    padding: '6px 14px', borderRadius: '6px', border: '1px solid #556',
    background: '#1f6feb', color: '#fff', cursor: 'pointer', font: 'inherit'
  };

  function showLogin(message) {
    if (panel) {
      panel.style.display = 'flex';
      return;
    }
    panel = el('div', {
      position: 'fixed', inset: '0', background: 'rgba(0,0,0,0.6)', zIndex: '10000',
      display: 'flex', alignItems: 'center', justifyContent: 'center'
    });
    var form = el('form', {
      background: '#161b22', color: '#e6edf3', padding: '24px', borderRadius: '10px',
      width: '300px', font: '14px system-ui, sans-serif', boxShadow: '0 8px 32px rgba(0,0,0,0.5)'
    });
    form.appendChild(el('h2', { margin: '0 0 12px', fontSize: '18px' }, 'Log in'));
    var status = el('p', { color: '#f85149', minHeight: '1em', margin: '0 0 8px' }, message || '');
    var user = el('input', inputStyle);
    user.name = 'username';
    user.autocomplete = 'username';
    user.required = true;
    user.placeholder = 'username';
    var pass = el('input', inputStyle);
    pass.type = 'password';
    pass.name = 'password';
    pass.autocomplete = 'current-password';
    pass.required = true;
    pass.placeholder = 'password';
    var row = el('div', { display: 'flex', gap: '8px', justifyContent: 'flex-end' });
    var cancel = el('button', Object.assign({}, buttonStyle, { background: 'transparent' }), 'Cancel');
    cancel.type = 'button';
    cancel.addEventListener('click', function () { panel.style.display = 'none'; });
    var submit = el('button', buttonStyle, 'Log in');
    submit.type = 'submit';
    row.appendChild(cancel);
    row.appendChild(submit);
    form.appendChild(status);
    form.appendChild(user);
    form.appendChild(pass);
    form.appendChild(row);
    form.addEventListener('submit', function (ev) {
      ev.preventDefault();
      submit.disabled = true;
      status.textContent = '';
      var body = JSON.stringify({ username: user.value, password: pass.value });
      pass.value = '';
      nativeFetch('/auth/login', {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', 'X-Requested-With': VALUE },
        body: body
      }).then(function (resp) {
        submit.disabled = false;
        if (resp.ok) {
          location.reload();
          return;
        }
        if (resp.status === 401) {
          status.textContent = 'Wrong username or password.';
        } else if (resp.status === 429) {
          status.textContent = 'Too many attempts. Wait a minute and try again.';
        } else {
          status.textContent = 'Login is unavailable (HTTP ' + resp.status + ').';
        }
      }).catch(function () {
        submit.disabled = false;
        status.textContent = 'Login is unavailable (network error).';
      });
    });
    panel.appendChild(form);
    document.body.appendChild(panel);
    user.focus();
  }

  function logout() {
    nativeFetch('/auth/logout', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'X-Requested-With': VALUE }
    }).finally(function () { location.reload(); });
  }

  function renderBadge(me) {
    if (!badge) {
      badge = el('div', {
        position: 'fixed', top: '8px', right: '8px', zIndex: '9999', display: 'flex', gap: '8px',
        alignItems: 'center', padding: '4px 10px', borderRadius: '8px', background: 'rgba(22,27,34,0.85)',
        color: '#e6edf3', font: '12px system-ui, sans-serif'
      });
      document.body.appendChild(badge);
    }
    while (badge.firstChild) {
      badge.removeChild(badge.firstChild);
    }
    if (me) {
      badge.appendChild(el('span', null, me.sub + ' (' + (me.roles || []).join(', ') + ')'));
      var out = el('button', Object.assign({}, buttonStyle, { padding: '2px 8px' }), 'Log out');
      out.type = 'button';
      out.addEventListener('click', logout);
      badge.appendChild(out);
    } else {
      var login = el('button', Object.assign({}, buttonStyle, { padding: '2px 8px' }), 'Log in');
      login.type = 'button';
      login.addEventListener('click', function () { showLogin(); });
      badge.appendChild(login);
    }
  }

  function init() {
    nativeFetch('/auth/me', { credentials: 'same-origin' }).then(function (resp) {
      if (resp.status === 404) {
        return; // auth is off on this service
      }
      if (!resp.ok) {
        renderBadge(null);
        return;
      }
      return resp.json().then(renderBadge);
    }).catch(function () { /* backend unreachable: the page shows its own errors */ });
  }

  window.unheadedAuth = { showLogin: showLogin, logout: logout };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
