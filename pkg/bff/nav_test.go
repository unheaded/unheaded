// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package bff

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeNavJS(t *testing.T) {
	w := httptest.NewRecorder()
	ServeNavJS(w, httptest.NewRequest(http.MethodGet, "/auth/nav.js", nil))
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("GET /auth/nav.js = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	for _, want := range []string{
		`"id":"dashboard"`, `"port":20000`,
		`"id":"kanban"`, `"port":20001`,
		`"id":"wiki"`, `"port":20002`,
		`"id":"zhenai"`, `"port":20103`, `"scheme":"https"`,
		`"id":"grafana"`, `"port":3001`, `"scheme":"http"`,
		"data-kingdom-nav", "location.hostname",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("nav.js lacks %s", want)
		}
	}
	if strings.Contains(body, "__NAV__") || strings.Contains(body, "192.168") {
		t.Error("nav.js still holds the placeholder or an address")
	}
	// Every app's id is unique, and the order is the pages' order.
	apps := KingdomNav()
	seen := map[string]bool{}
	for _, a := range apps {
		if seen[a.ID] || a.Port <= 0 || a.Name == "" || (a.Scheme != "http" && a.Scheme != "https") {
			t.Errorf("bad entry %+v", a)
		}
		seen[a.ID] = true
	}
	if apps[0].ID != "dashboard" || apps[len(apps)-1].ID != "zhenai" {
		t.Errorf("order: %+v", apps)
	}

	for _, m := range []string{http.MethodPost, http.MethodPut} {
		w := httptest.NewRecorder()
		ServeNavJS(w, httptest.NewRequest(m, "/auth/nav.js", nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /auth/nav.js = %d, want 405", m, w.Code)
		}
	}
}
