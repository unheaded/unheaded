// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package architect

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// lateWotan reserves an address with nothing listening; start brings an HTTP
// Wotan up there and records each join as "<topic> as <name>".
func lateWotan(t *testing.T) (string, func(), func() []string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	_ = lis.Close()
	var mu sync.Mutex
	var joins []string
	start := func() {
		l, err := net.Listen("tcp", addr)
		if err != nil {
			t.Fatalf("relisten %s: %v", addr, err)
		}
		srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/subscribe"):
				var body struct {
					DisplayName string `json:"display_name"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				topic := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/topics/"), "/subscribe")
				mu.Lock()
				joins = append(joins, topic+" as "+body.DisplayName)
				mu.Unlock()
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"s1","status":"approved"}}`))
			case strings.HasSuffix(r.URL.Path, "/messages"):
				_, _ = w.Write([]byte(`{"messages":[],"last_seq":0}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		srv.Listener = l
		srv.Start()
		t.Cleanup(srv.Close)
	}
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), joins...)
	}
	return addr, start, got
}

// architect started before Wotan joins alerts.critical and
// architecture.updates once Wotan is up. Start used to try once, return the
// error, and leave the service without Wotan until restarted.
func TestArchitect_SubscribesWhenWotanStartsLater(t *testing.T) {
	addr, start, joins := lateWotan(t)
	client, err := wotanClient.NewClient(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	svc := NewWithWotan(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with Wotan down: %v (it retries in the background)", err)
	}

	time.Sleep(1200 * time.Millisecond)
	start()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j := strings.Join(joins(), ",")
		if strings.Contains(j, "alerts.critical as architect-service") && strings.Contains(j, "architecture.updates as architect-service") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("joins %v 5 s after Wotan came up, want alerts.critical and architecture.updates", joins())
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitUp := time.Now().Add(3 * time.Second)
	for !svc.WotanHealth().Streams["alerts.critical"] {
		if time.Now().After(waitUp) {
			t.Fatalf("alerts.critical stream not up: %+v", svc.WotanHealth())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// /health stays 200 and carries the Wotan detail.
func TestArchitect_HealthIncludesWotanDetail(t *testing.T) {
	client, err := wotanClient.NewClient("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	h := NewHTTPHandler(NewWithWotan(client))
	w := httptest.NewRecorder()
	h.Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", w.Code)
	}
	var body struct {
		Wotan *struct {
			Transport string `json:"transport"`
		} `json:"wotan"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Wotan == nil || body.Wotan.Transport != "HTTP" {
		t.Errorf("wotan detail %+v, want transport HTTP", body.Wotan)
	}
}
