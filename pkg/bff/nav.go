// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package bff

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"unheaded/pkg/ports"
	pqtls "unheaded/pkg/tls"
)

//go:embed static/nav.js
var navJS string

// NavApp is one entry of the Kingdom's menu: an app on this host, on its
// own port. The browser builds the link from the host it loaded the page
// from, so no page and no binary carries an address (next.md 2026-11-11
// todos 5, 6 and 8d: every menu was a hardcoded wireless address).
type NavApp struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Scheme string `json:"scheme"`
	Port   int    `json:"port"`
}

// grafanaPort is where docker-compose.yml publishes Grafana. It is third
// party and speaks plain HTTP until ADR-114's gate (N8) sits in front of it;
// it is not in pkg/ports, which registers the Kingdom's own listeners.
const grafanaPort = 3001

// KingdomNav is the menu, in the order the pages show it. The first-party
// apps take this process's own scheme (UNHEADED_TLS_ENABLED: every page in
// a deployment is TLS or none is); the Zhen front is TLS always (ADR-138).
func KingdomNav() []NavApp {
	s := pqtls.Scheme()
	return []NavApp{
		{ID: "dashboard", Name: "Dashboard", Scheme: s, Port: ports.DashboardBackend},
		{ID: "grafana", Name: "Grafana", Scheme: "http", Port: grafanaPort},
		{ID: "kanban", Name: "Kanban", Scheme: s, Port: ports.KanbanApp},
		{ID: "wiki", Name: "Wiki", Scheme: s, Port: ports.WikiServer},
		{ID: "zhenai", Name: "Zhenai", Scheme: "https", Port: ports.ZhenWebUI},
	}
}

var (
	navOnce sync.Once
	navBody []byte
)

// ServeNavJS serves nav.js, the menu script, with the menu rendered into it
// once per process. A service mounts it at /auth/nav.js beside auth.js.
func ServeNavJS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	navOnce.Do(func() {
		apps, err := json.Marshal(KingdomNav())
		if err != nil {
			apps = []byte("[]")
		}
		navBody = []byte(strings.Replace(navJS, "__NAV__", string(apps), 1))
	})
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(navBody)
}
