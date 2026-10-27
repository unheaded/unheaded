// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package gateway

import (
	"context"
	"testing"
	"time"

	"unheaded/pkg/wotan-client/wotantest"
	"unheaded/services/gateway/config"
)

// A gateway that starts before Wotan joins its events topic and streams
// alerts.critical once Wotan comes up. Start used to subscribe once (10 s
// each) and give up.
func TestLateWotan_Gateway(t *testing.T) {
	fake := wotantest.Reserve(t)

	cfg := config.DefaultConfig()
	cfg.Server.HTTPPort = 0 // ephemeral
	cfg.Auth.Enabled = false
	cfg.Wotan.Enabled = true
	cfg.Wotan.URL = fake.Addr
	gw, err := New(cfg, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- gw.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("gateway did not shut down")
		}
	})

	if !wotantest.WaitFor(2*time.Second, gw.IsRunning) {
		t.Fatal("gateway not running 2 s after Start with Wotan down")
	}
	time.Sleep(100 * time.Millisecond)

	fake.Start(t)

	topic := cfg.Wotan.Topic
	ok := wotantest.WaitFor(10*time.Second, func() bool {
		return len(fake.Joins(topic)) > 0 &&
			len(fake.Joins("alerts.critical")) > 0 &&
			fake.Polls("alerts.critical") > 0
	})
	if !ok {
		t.Fatalf("gateway did not join after Wotan came up: %s=%v alerts=%v polls=%d",
			topic, fake.Joins(topic), fake.Joins("alerts.critical"), fake.Polls("alerts.critical"))
	}
	for _, name := range append(fake.Joins(topic), fake.Joins("alerts.critical")...) {
		if name != "gateway-service" {
			t.Errorf("joined as %q, want gateway-service", name)
		}
	}
}
