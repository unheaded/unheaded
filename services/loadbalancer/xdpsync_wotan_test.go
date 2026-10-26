// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
)

// xdpsync publishes lb.rebalance after every table rebuild. The client
// refuses a publish on a topic it has not joined, and Wotan approves joins
// by display name (configs/wotan.yaml auto_approve), so the service must
// join lb.rebalance before the first publish, under its own name
// (S-REAL-STEEL Step 92; the name used to be xdpsync-service, which no
// allowlist knows).
func TestXDPSync_JoinsBeforePublish(t *testing.T) {
	var mu sync.Mutex
	var events []string // "join <topic> as <name>" and "publish <topic>", in order
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		topic := strings.TrimPrefix(r.URL.Path, "/api/v1/topics/")
		switch {
		case strings.HasSuffix(r.URL.Path, "/subscribe"):
			var body struct {
				DisplayName string `json:"display_name"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			events = append(events, "join "+strings.TrimSuffix(topic, "/subscribe")+" as "+body.DisplayName)
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"subscriber":{"subscriber_id":"s1","status":"approved"}}`))
		case strings.HasSuffix(r.URL.Path, "/publish"):
			mu.Lock()
			events = append(events, "publish "+strings.TrimSuffix(topic, "/publish"))
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"message":{"message_id":"m1","seq":1}}`))
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"messages":[],"last_seq":0}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client, err := wotanClient.NewClient(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	svc := NewXDPSync(logger.New(nil), client)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Stop() })
	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy})

	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	join, publish := -1, -1
	for i, e := range got {
		if e == "join lb.rebalance as loadbalancer" && join < 0 {
			join = i
		}
		if e == "publish lb.rebalance" && publish < 0 {
			publish = i
		}
		if strings.Contains(e, "xdpsync-service") {
			t.Errorf("joined under the old name: %q", e)
		}
	}
	if publish < 0 {
		t.Fatalf("lb.rebalance was never published (events %v)", got)
	}
	if join < 0 || join > publish {
		t.Fatalf("lb.rebalance published before it was joined as loadbalancer (events %v)", got)
	}
	if svc.WotanDrops() != 0 {
		t.Errorf("wotan drops = %d, want 0", svc.WotanDrops())
	}
}
