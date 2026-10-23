// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

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
)

// lateWotan reserves an address with nothing listening; start brings an HTTP
// Wotan up there and records which topics were joined, by whom.
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

// timeguru started before Wotan subscribes once Wotan is up. initWotan used
// to subscribe once at start and, on failure, run without Wotan for good.
func TestTimeguru_SubscribesWhenWotanStartsLater(t *testing.T) {
	addr, start, joins := lateWotan(t)
	client, err := initWotan(addr, "")
	if err != nil {
		t.Fatalf("initWotan with Wotan down: %v (it must not need Wotan to be up)", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startWotanSubscriptions(ctx, client, nil)

	time.Sleep(1200 * time.Millisecond) // timeguru runs a while with Wotan down
	start()
	deadline := time.Now().Add(5 * time.Second)
	for {
		j := strings.Join(joins(), ",")
		if strings.Contains(j, "timeline.updates as "+wotanDisplayName) && strings.Contains(j, "alerts.critical as "+wotanDisplayName) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("joins %v 5 s after Wotan came up, want timeline.updates and alerts.critical", joins())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !client.IsApproved(wotanTopic) {
		t.Error("client does not hold an approved timeline.updates subscription")
	}
}
