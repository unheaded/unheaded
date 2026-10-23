// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cuirassdriver

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

const testKey = "test-cuirass-key"

// fakeCuirass is an in-memory Cuirass: desired specs from PUT/DELETE,
// actual state that follows desired on every reconcile tick (an instance
// turns running and then healthy after healthyAfter ticks). It refuses
// writes without the key and a second owner of one proxy listen address,
// as Cuirass does.
type fakeCuirass struct {
	t            *testing.T
	mu           sync.Mutex
	desired      map[string]*containerSpec
	actual       map[string]*containerState
	age          map[string]int
	healthyAfter int // ticks; <0 never healthy
	reconciles   int
	log          []string
	// hooks
	onTick func(f *fakeCuirass)
	// failPut makes PUT of that id answer 500.
	failPut map[string]bool
}

func newFakeCuirass(t *testing.T) (*fakeCuirass, *httptest.Server) {
	f := &fakeCuirass{
		t:            t,
		desired:      map[string]*containerSpec{},
		actual:       map[string]*containerState{},
		age:          map[string]int{},
		healthyAfter: 1,
		failPut:      map[string]bool{},
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, srv
}

// seed adds a running, healthy instance as if an earlier deploy made it.
func (f *fakeCuirass) seed(spec containerSpec) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := spec
	f.desired[s.ID] = &s
	f.actual[s.ID] = &containerState{ID: s.ID, Status: "running", Health: "healthy", Managed: true,
		IP: fmt.Sprintf("10.10.10.%d", 50+len(f.actual)), Labels: s.Labels, Proxy: s.Proxy}
	f.age[s.ID] = 100
}

// tick moves actual one step towards desired; the caller holds mu.
func (f *fakeCuirass) tick() {
	for id, spec := range f.desired {
		a, ok := f.actual[id]
		if !ok {
			a = &containerState{ID: id, Status: "running", Health: "unknown", Managed: true,
				IP: fmt.Sprintf("10.10.10.%d", 100+len(f.age)), Labels: spec.Labels}
			f.actual[id] = a
			f.age[id] = 0
		}
		f.age[id]++
		if f.healthyAfter >= 0 && f.age[id] > f.healthyAfter && a.Status == "running" {
			a.Health = "healthy"
		}
		a.Proxy = spec.Proxy
	}
	for id := range f.actual {
		if _, ok := f.desired[id]; !ok && f.actual[id].Managed {
			delete(f.actual, id)
		}
	}
	if f.onTick != nil {
		f.onTick(f)
	}
}

func (f *fakeCuirass) record(s string) { f.log = append(f.log, s) }

func (f *fakeCuirass) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write := r.Method != http.MethodGet
	if write && r.Header.Get("X-API-Key") != testKey {
		http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state/desired":
		_ = json.NewEncoder(w).Encode(f.desired)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/state/actual":
		f.tick()
		_ = json.NewEncoder(w).Encode(f.actual)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/reconcile":
		f.reconciles++
		f.record("POST reconcile")
		f.tick()
		_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{}})
	case strings.HasPrefix(r.URL.Path, "/api/v1/containers/"):
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/containers/")
		switch r.Method {
		case http.MethodPut:
			if f.failPut[id] {
				http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
				return
			}
			var spec containerSpec
			b, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(b, &spec); err != nil {
				http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
				return
			}
			if spec.ID != id {
				http.Error(w, `{"error":"id mismatch"}`, http.StatusBadRequest)
				return
			}
			if spec.Proxy != nil {
				for other, s := range f.desired {
					if other != id && s.Proxy != nil && s.Proxy.Listen == spec.Proxy.Listen {
						http.Error(w, `{"error":"proxy listen taken"}`, http.StatusConflict)
						return
					}
				}
			}
			f.desired[id] = &spec
			f.record(fmt.Sprintf("PUT %s proxy=%t", id, spec.Proxy != nil))
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
		case http.MethodDelete:
			if _, ok := f.desired[id]; !ok {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			delete(f.desired, id)
			f.record("DELETE " + id)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id})
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	default:
		http.NotFound(w, r)
	}
}

// desiredIDs returns the sorted desired IDs.
func (f *fakeCuirass) desiredIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id := range f.desired {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// proxyHolders returns the desired IDs that carry a proxy.
func (f *fakeCuirass) proxyHolders() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id, s := range f.desired {
		if s.Proxy != nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (f *fakeCuirass) logSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.log...)
}
