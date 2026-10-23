// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cuirasstest is an in-memory Cuirass for tests of code that
// deploys through pkg/deploy/cuirassdriver (Sword). It keeps desired specs
// from PUT/DELETE and an actual state that follows them on every read of
// /api/v1/state/actual or POST /api/v1/reconcile: an instance appears
// running with health "unknown" and turns "healthy" after HealthyAfter
// ticks, unless its image is in NeverHealthy. Like Cuirass, it refuses
// writes without the key (X-API-Key) and a second desired owner of one
// proxy listen address.
//
// Test helper only: it is never linked into a binary.
package cuirasstest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Key is the API key the fake accepts.
const Key = "cuirasstest-key"

// Health and Proxy mirror Cuirass's spec fields.
type Health struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

// Proxy is the unheaded-proxy device.
type Proxy struct {
	Listen  string `json:"listen"`
	Connect string `json:"connect"`
}

// Spec is a desired container.
type Spec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	Health *Health           `json:"health,omitempty"`
	Proxy  *Proxy            `json:"proxy,omitempty"`
}

// State is an actual container.
type State struct {
	ID      string            `json:"id"`
	Status  string            `json:"status"`
	IP      string            `json:"ip"`
	Health  string            `json:"health"`
	Managed bool              `json:"managed"`
	Proxy   *Proxy            `json:"proxy,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// Fake is the in-memory Cuirass.
type Fake struct {
	Mu           sync.Mutex
	Desired      map[string]*Spec
	Actual       map[string]*State
	HealthyAfter int             // ticks before a new instance is healthy
	NeverHealthy map[string]bool // images that never turn healthy
	Reachable    bool            // false: every request answers 503

	age  map[string]int
	ips  int
	log  []string
	Srv  *httptest.Server
	URL  string
	Info map[string]any // GET /api/v1/info body
}

// New starts a fake Cuirass; it stops with the test.
func New(t testing.TB) *Fake {
	f := &Fake{
		Desired:      map[string]*Spec{},
		Actual:       map[string]*State{},
		HealthyAfter: 1,
		NeverHealthy: map[string]bool{},
		Reachable:    true,
		age:          map[string]int{},
		Info:         map[string]any{"lxd_backend": "mock", "enforce": true, "chaos": false},
	}
	f.Srv = httptest.NewServer(f)
	f.URL = f.Srv.URL
	t.Cleanup(f.Srv.Close)
	return f
}

// Seed adds a running, healthy instance.
func (f *Fake) Seed(s Spec) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	cp := s
	f.Desired[s.ID] = &cp
	f.ips++
	f.Actual[s.ID] = &State{ID: s.ID, Status: "running", Health: "healthy", Managed: true,
		IP: fmt.Sprintf("10.10.10.%d", 50+f.ips), Labels: s.Labels, Proxy: s.Proxy}
	f.age[s.ID] = 1000
}

func (f *Fake) tick() {
	for id, spec := range f.Desired {
		a, ok := f.Actual[id]
		if !ok {
			f.ips++
			a = &State{ID: id, Status: "running", Health: "unknown", Managed: true,
				IP: fmt.Sprintf("10.10.10.%d", 50+f.ips), Labels: spec.Labels}
			f.Actual[id] = a
			f.age[id] = 0
		}
		f.age[id]++
		if !f.NeverHealthy[spec.Image] && f.age[id] > f.HealthyAfter && a.Status == "running" {
			a.Health = "healthy"
		}
		a.Proxy = spec.Proxy
	}
	for id, a := range f.Actual {
		if _, ok := f.Desired[id]; !ok && a.Managed {
			delete(f.Actual, id)
		}
	}
}

// ServeHTTP implements the subset of Cuirass's API the driver uses.
func (f *Fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if !f.Reachable {
		http.Error(w, `{"error":"down"}`, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet && r.Header.Get("X-API-Key") != Key {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/info":
		_ = json.NewEncoder(w).Encode(f.Info)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state/desired":
		_ = json.NewEncoder(w).Encode(f.Desired)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state/actual":
		f.tick()
		_ = json.NewEncoder(w).Encode(f.Actual)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/reconcile":
		f.log = append(f.log, "POST reconcile")
		f.tick()
		_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/containers/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/containers/")
		switch r.Method {
		case http.MethodPut:
			var s Spec
			b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err := json.Unmarshal(b, &s); err != nil || s.ID != id {
				http.Error(w, `{"error":"bad spec"}`, http.StatusBadRequest)
				return
			}
			if s.Proxy != nil {
				for other, o := range f.Desired {
					if other != id && o.Proxy != nil && o.Proxy.Listen == s.Proxy.Listen {
						http.Error(w, `{"error":"proxy listen taken"}`, http.StatusConflict)
						return
					}
				}
			}
			f.Desired[id] = &s
			f.log = append(f.log, fmt.Sprintf("PUT %s proxy=%t", id, s.Proxy != nil))
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
		case http.MethodDelete:
			if _, ok := f.Desired[id]; !ok {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			delete(f.Desired, id)
			f.log = append(f.log, "DELETE "+id)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

// DesiredIDs returns the sorted desired IDs.
func (f *Fake) DesiredIDs() []string {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	ids := make([]string, 0, len(f.Desired))
	for id := range f.Desired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ProxyHolders returns the desired IDs that carry the proxy.
func (f *Fake) ProxyHolders() []string {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	var ids []string
	for id, s := range f.Desired {
		if s.Proxy != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// HealthyVersions returns, sorted, the ver label of every instance Cuirass
// itself reports running and healthy.
func (f *Fake) HealthyVersions() []string {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	var vs []string
	for id, a := range f.Actual {
		if a.Managed && a.Status == "running" && a.Health == "healthy" && f.Desired[id] != nil {
			vs = append(vs, f.Desired[id].Labels["ver"])
		}
	}
	sort.Strings(vs)
	return vs
}

// Log returns the writes seen so far.
func (f *Fake) Log() []string {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return append([]string(nil), f.log...)
}
