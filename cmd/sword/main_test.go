// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"unheaded/pkg/ports"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadConfig_RefusesWithoutCuirassKey(t *testing.T) {
	_, err := loadConfig(env(map[string]string{"CUIRASS_URL": "http://127.0.0.1:19006"}))
	if err == nil || !strings.Contains(err.Error(), "CUIRASS_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	c, err := loadConfig(env(map[string]string{"CUIRASS_API_KEY": "k"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.addr != ports.DefaultAddr(ports.Sword) || ports.Sword != 19011 {
		t.Fatalf("addr %q", c.addr)
	}
	if c.cuirassURL != "http://127.0.0.1:19006" {
		t.Fatalf("cuirass url %q", c.cuirassURL)
	}
}

func TestBuild_GuardNeedsAReadablePolicy(t *testing.T) {
	log := zerolog.Nop()
	c := config{cuirassURL: "http://127.0.0.1:19006", cuirassKey: "k", vmURL: "http://127.0.0.1:8428",
		policy: filepath.Join(t.TempDir(), "missing.yaml")}
	if _, err := build(c, &log); err == nil {
		t.Fatal("VM_URL with an unreadable policy started anyway")
	}
	p := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(p, []byte("slos:\n  - name: wotan\n    objective: 99.5\n    page: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.policy = p
	s, err := build(c, &log)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
}

// Without AUTH_ENABLED a write is refused by Sword itself; with it and a
// key, pkg/auth stands in front: no key, no deployment.
func TestHandler_AuthInFront(t *testing.T) {
	log := zerolog.Nop()
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_API_KEYS", "sword-test-key")
	s, err := build(config{cuirassURL: "http://127.0.0.1:19006", cuirassKey: "k", dataDir: t.TempDir()}, &log)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := handler(s)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/deployments", strings.NewReader("{}"))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("POST without a key = %d, want 401", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), `unheaded_http_requests_total`) {
		t.Fatal("/metrics lacks the house HTTP series")
	}
}
