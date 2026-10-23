// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package mock

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// Message IDs are unique. They were the current second plus one of 26
// letters, so the 27th message in a second reused the first one's ID.
func TestPublish_MessageIDsAreUnique(t *testing.T) {
	m := NewMockClient(WithAutoApprove())
	ctx := context.Background()
	if _, err := m.Subscribe(ctx, "t", "svc"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if err := m.Publish(ctx, "t", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	msgs, _ := m.GetMessages(ctx, "t", 0, 0)
	seen := map[string]bool{}
	for _, msg := range msgs {
		if seen[msg.MessageID] {
			t.Fatalf("message ID %s repeated", msg.MessageID)
		}
		seen[msg.MessageID] = true
	}
}

// Injected errors can be set while the client is in use (run with -race).
func TestSetError_WhileInUse(t *testing.T) {
	m := NewMockClient(WithAutoApprove())
	ctx := context.Background()
	if _, err := m.Subscribe(ctx, "t", "svc"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = m.Publish(ctx, "t", []byte("x"))
			_, _ = m.GetMessages(ctx, "t", 0, 1)
		}
	}()
	for i := 0; i < 200; i++ {
		m.SetError("publish", errors.New("down"))
		m.ClearErrors()
	}
	wg.Wait()
}
