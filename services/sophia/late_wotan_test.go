// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"context"
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/wotantest"
)

// A sophia that starts before Wotan joins sophia.wisdom and knowledge.shared
// and streams alerts.critical once Wotan comes up, and its events then
// publish. Start used to subscribe once and give up.
func TestLateWotan_Sophia(t *testing.T) {
	fake := wotantest.Reserve(t)
	client, err := wotanClient.NewClient(fake.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	cfg := DefaultConfig()
	cfg.EnableInference = false
	svc := NewService(logger.New(os.Stderr), client, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	start := time.Now()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start with Wotan down: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("Start blocked %v with Wotan down", took)
	}
	time.Sleep(100 * time.Millisecond)

	fake.Start(t)

	ok := wotantest.WaitFor(10*time.Second, func() bool {
		return len(fake.Joins("sophia.wisdom")) > 0 &&
			len(fake.Joins("knowledge.shared")) > 0 &&
			len(fake.Joins("alerts.critical")) > 0 &&
			fake.Polls("alerts.critical") > 0 &&
			client.IsApproved("sophia.wisdom")
	})
	if !ok {
		t.Fatalf("sophia did not join after Wotan came up: wisdom=%v shared=%v alerts=%v polls=%d",
			fake.Joins("sophia.wisdom"), fake.Joins("knowledge.shared"), fake.Joins("alerts.critical"), fake.Polls("alerts.critical"))
	}

	if err := svc.Learn(ctx, &Knowledge{Subject: "wotan", Predicate: "came_up", Object: "late"}); err != nil {
		t.Fatal(err)
	}
	if !wotantest.WaitFor(2*time.Second, func() bool { return len(fake.Published("sophia.wisdom")) > 0 }) {
		t.Error("knowledge event not published after the late join")
	}
}
