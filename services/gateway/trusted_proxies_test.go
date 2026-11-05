// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"unheaded/pkg/logger"
	"unheaded/services/gateway/config"
)

// L8: rate_limit.trusted_proxies decides for the backends too, so the
// limiter and the forwarded address agree on who the client is.
func TestConfiguredTrustedProxiesReachTheRoutes(t *testing.T) {
	var got string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := config.DefaultConfig()
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = false
	cfg.Routes = []config.RouteConfig{{Name: "app", PathPrefix: "/app", Backends: []string{backend.URL}, Timeout: 5 * time.Second}}
	cfg.RateLimit.TrustedProxies = []string{"192.0.2.1"}
	gw, err := New(cfg, logger.New(io.Discard))
	if err != nil {
		t.Fatal(err)
	}
	route := gw.GetRouter().GetRoute("app")
	if route == nil {
		t.Fatal("route not added")
	}
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.RemoteAddr = "192.0.2.1:4000"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	route.Proxy.ServeHTTP(httptest.NewRecorder(), req)
	if got != "1.2.3.4" {
		t.Fatalf("backend told %q, want the address the trusted proxy forwarded", got)
	}
}
