// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package micromanager

import (
	"context"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
	"unheaded/pkg/wotan-client/wotantest"
)

// A micromanager that starts before Wotan still joins its topics and opens
// its alerts stream once Wotan comes up. Start used to subscribe once and,
// when that failed, log "continuing without subscription" for the life of
// the process.
func TestLateWotan_Micromanager(t *testing.T) {
	fake := wotantest.Reserve(t)
	client, err := wotanClient.NewClient(fake.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	service := NewService(NewStore(), client)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	start := time.Now()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start with Wotan down: %v", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("Start blocked %v with Wotan down", took)
	}
	time.Sleep(100 * time.Millisecond) // first join attempts are refused

	fake.Start(t)

	topics := []string{"alerts.critical", "tasks.created", "tasks.updated", "tasks.completed"}
	ok := wotantest.WaitFor(10*time.Second, func() bool {
		for _, topic := range topics {
			if len(fake.Joins(topic)) == 0 {
				return false
			}
		}
		return fake.Polls("alerts.critical") > 0
	})
	if !ok {
		for _, topic := range topics {
			t.Logf("%s: joins=%v polls=%d", topic, fake.Joins(topic), fake.Polls(topic))
		}
		t.Fatal("micromanager did not join its topics after Wotan came up")
	}
	for _, topic := range topics {
		for _, name := range fake.Joins(topic) {
			if name != "micromanager-service" {
				t.Errorf("%s joined as %q", topic, name)
			}
		}
	}
	if !wotantest.WaitFor(2*time.Second, func() bool { return service.HealthStatus()["subscriptions"] == len(topics) }) {
		t.Errorf("subscriptions = %v, want %d", service.HealthStatus()["subscriptions"], len(topics))
	}

	// Publishing works once joined.
	if err := service.PublishTaskCreated("task-1", NewTask("task-1", "Test", "owner")); err != nil {
		t.Fatalf("PublishTaskCreated after late join: %v", err)
	}
	if got := fake.Published("tasks.created"); len(got) != 1 {
		t.Errorf("published on tasks.created: %v, want one event", got)
	}
}
