// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package loadbalancer

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/wotantest"
)

// An xdpsync that starts before Wotan joins its lb.* topics and streams
// alerts.critical once Wotan comes up, and its lb.rebalance publishes then
// go through. Start used to subscribe once per topic and give up, so every
// later rebalance publish was refused "not subscribed".
func TestLateWotan_XDPSync(t *testing.T) {
	fake := wotantest.Reserve(t)
	client, err := wotanClient.NewClient(fake.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	svc := NewXDPSync(logger.New(nil), client)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = svc.Stop()
	})

	start := time.Now()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with Wotan down: %v", err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Start blocked %v with Wotan down", took)
	}
	time.Sleep(100 * time.Millisecond)

	fake.Start(t)

	joined := []string{"lb.backends.update", "lb.health.update", "lb.rebalance", "alerts.critical"}
	ok := wotantest.WaitFor(10*time.Second, func() bool {
		for _, topic := range joined {
			if len(fake.Joins(topic)) == 0 {
				return false
			}
		}
		return fake.Polls("alerts.critical") > 0 && client.IsApproved("lb.rebalance")
	})
	if !ok {
		for _, topic := range joined {
			t.Logf("%s: joins=%v polls=%d", topic, fake.Joins(topic), fake.Polls(topic))
		}
		t.Fatal("xdpsync did not join after Wotan came up")
	}
	for _, topic := range joined {
		for _, name := range fake.Joins(topic) {
			if name != wotanDisplayName {
				t.Errorf("%s joined as %q, want %q", topic, name, wotanDisplayName)
			}
		}
	}

	svc.AddBackend(&Backend{ID: "b1", Address: "10.0.0.1", Port: 8080, Weight: 1, Health: HealthHealthy})
	if !wotantest.WaitFor(2*time.Second, func() bool { return len(fake.Published("lb.rebalance")) > 0 }) {
		t.Error("lb.rebalance not published after the late join")
	}
}
