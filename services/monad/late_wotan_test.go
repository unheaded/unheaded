// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package monad

import (
	"context"
	"os"
	"testing"
	"time"

	"unheaded/pkg/logger"
	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/wotantest"
)

// A monad that starts before Wotan joins monad.operations and streams
// alerts.critical once Wotan comes up, and its operation events then
// publish. Start used to subscribe once and give up.
func TestLateWotan_Monad(t *testing.T) {
	fake := wotantest.Reserve(t)
	client, err := wotanClient.NewClient(fake.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	svc := NewService(logger.New(os.Stderr), client)
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
		return len(fake.Joins("monad.operations")) > 0 &&
			len(fake.Joins("alerts.critical")) > 0 &&
			fake.Polls("alerts.critical") > 0 &&
			client.IsApproved("monad.operations")
	})
	if !ok {
		t.Fatalf("monad did not join after Wotan came up: operations=%v alerts=%v polls=%d",
			fake.Joins("monad.operations"), fake.Joins("alerts.critical"), fake.Polls("alerts.critical"))
	}

	svc.RegisterMutation("late", func(ctx context.Context, input interface{}) (interface{}, []StateChange, error) {
		return "ok", nil, nil
	})
	if r := svc.Mutate(ctx, "late", nil); !r.IsOk() {
		t.Fatalf("Mutate: %v", r.Error())
	}
	if !wotantest.WaitFor(2*time.Second, func() bool { return len(fake.Published("monad.operations")) > 0 }) {
		t.Error("operation event not published after the late join")
	}
}
