// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestL7ProxyBackendView pins what a backend receives: the client's Host,
// the client IP appended to an incoming X-Forwarded-For (not replacing it),
// configured request headers added and removed, and hop-by-hop headers gone.
func TestL7ProxyBackendView(t *testing.T) {
	var got *http.Request
	be := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Clone(r.Context())
		w.Header().Set("X-Backend", "yes")
		_, _ = w.Write([]byte("ok"))
	}))
	defer be.Close()

	cfg := DefaultConfig()
	cfg.Backends = []BackendConfig{{Name: "b1", Address: strings.TrimPrefix(be.URL, "http://"), Weight: 1, Enabled: true}}
	cfg.Headers.AddRequestHeaders = map[string]string{"X-Added": "1"}
	cfg.Headers.RemoveRequestHeaders = []string{"X-Secret"}
	pool, err := NewBackendPool(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewL7Proxy(cfg, pool, nil)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "http://app.example/path?q=1", nil)
	req.RemoteAddr = "198.51.100.7:4000"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.Header.Set("X-Secret", "drop me")
	req.Header.Set("Connection", "keep-alive, X-Hop")
	req.Header.Set("X-Hop", "drop me too")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" || rec.Header().Get("X-Backend") != "yes" {
		t.Fatalf("response %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	if got == nil {
		t.Fatal("backend not reached")
	}
	for _, c := range []struct{ what, got, want string }{
		{"Host", got.Host, "app.example"},
		{"path", got.URL.RequestURI(), "/path?q=1"},
		{"X-Forwarded-For", got.Header.Get("X-Forwarded-For"), "203.0.113.50, 198.51.100.7"},
		{"X-Added", got.Header.Get("X-Added"), "1"},
		{"X-Secret", got.Header.Get("X-Secret"), ""},
		{"X-Hop", got.Header.Get("X-Hop"), ""},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.what, c.got, c.want)
		}
	}
}
