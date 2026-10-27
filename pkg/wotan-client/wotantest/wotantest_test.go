// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis. All rights reserved.

package wotantest

import (
	"context"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// Reserve refuses connections until Start; then a real client joins,
// publishes and streams through the fake.
func TestFake_LateStartRoundTrip(t *testing.T) {
	f := Reserve(t)
	c, err := wotanClient.NewClient(f.Addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := c.Subscribe(ctx, "t.one", "svc"); err == nil {
		t.Fatal("Subscribe before Start succeeded; want connection refused")
	}

	f.Start(t)
	if _, err := c.Subscribe(ctx, "t.one", "svc"); err != nil {
		t.Fatalf("Subscribe after Start: %v", err)
	}
	if got := f.Joins("t.one"); len(got) != 1 || got[0] != "svc" {
		t.Errorf("Joins = %v", got)
	}
	if err := c.Publish(ctx, "t.one", []byte("hi")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got := f.Published("t.one"); len(got) != 1 || got[0] != "hi" {
		t.Errorf("Published = %v", got)
	}

	f.Enqueue("t.one", "pushed")
	ch, err := c.StreamMessages(ctx, "t.one")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-ch:
		if m == nil || m.Payload != "pushed" {
			t.Errorf("streamed %+v, want payload pushed", m)
		}
	case <-ctx.Done():
		t.Fatal("no message streamed")
	}
	if f.Polls("t.one") == 0 {
		t.Error("Polls = 0 after a stream delivered")
	}
}
