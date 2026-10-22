// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	wotanClient "unheaded/pkg/wotan-client"
)

// fakeTopics stands in for Wotan's HTTP client: joins fail until up is set,
// and published messages are delivered to every stream on the topic.
type fakeTopics struct {
	mu        sync.Mutex
	up        bool
	pending   bool
	joins     int
	published [][]byte
	streams   []chan *wotanClient.Message
}

func (f *fakeTopics) Subscribe(_ context.Context, topic, name string) (*wotanClient.Subscriber, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.joins++
	if !f.up {
		return nil, errors.New("connection refused")
	}
	status := "approved"
	if f.pending {
		status = "pending"
	}
	return &wotanClient.Subscriber{Topic: topic, DisplayName: name, Status: status}, nil
}

func (f *fakeTopics) Publish(_ context.Context, _ string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.up {
		return errors.New("connection refused")
	}
	f.published = append(f.published, payload)
	for _, ch := range f.streams {
		ch <- &wotanClient.Message{Payload: string(payload)}
	}
	return nil
}

func (f *fakeTopics) StreamMessages(_ context.Context, _ string) (<-chan *wotanClient.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan *wotanClient.Message, 16)
	f.streams = append(f.streams, ch)
	return ch, nil
}

func (f *fakeTopics) set(up bool) {
	f.mu.Lock()
	f.up = up
	f.mu.Unlock()
}

func (f *fakeTopics) snapshot() (joins int, published [][]byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.joins, append([][]byte(nil), f.published...)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestStart_RegistersOnceWotanAnswers: every service called discovery with
// a nil connection, so nothing ever registered. A service that starts
// before Wotan must still register when Wotan comes up.
func TestStart_RegistersOnceWotanAnswers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTopics{}
	reg := start(ctx, f, ServiceEntry{Name: "timeguru", Address: "west", Port: 19000}, time.Millisecond, 4*time.Millisecond)

	waitFor(t, "join retries while Wotan is down", func() bool { j, _ := f.snapshot(); return j >= 3 })
	if _, p := f.snapshot(); len(p) != 0 {
		t.Fatalf("published %d announcements with Wotan down", len(p))
	}

	f.set(true)
	waitFor(t, "the first announcement", func() bool { _, p := f.snapshot(); return len(p) > 0 })
	_, p := f.snapshot()
	var got ServiceEntry
	if err := json.Unmarshal(p[0], &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "timeguru" || got.Address != "west" || got.Port != 19000 {
		t.Errorf("announced %+v, want timeguru at west:19000", got)
	}

	// A peer's announcement lands in the registry through the stream.
	peer, _ := json.Marshal(ServiceEntry{Name: "captain", Address: "west", Port: 19002})
	if err := f.Publish(ctx, DiscoveryTopic, peer); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the peer to resolve", func() bool { _, err := reg.Resolve("captain"); return err == nil })
}

// TestStart_PendingJoinRetries: a display name missing from Wotan's
// topics.auto_approve leaves the join pending; that is not a registration.
func TestStart_PendingJoinRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTopics{up: true, pending: true}
	start(ctx, f, ServiceEntry{Name: "chaos-controller", Address: "west", Port: 19300}, time.Millisecond, 4*time.Millisecond)

	waitFor(t, "repeated pending joins", func() bool { j, _ := f.snapshot(); return j >= 3 })
	if _, p := f.snapshot(); len(p) != 0 {
		t.Fatalf("published %d announcements on a pending membership", len(p))
	}
}

func TestStart_StopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeTopics{}
	start(ctx, f, ServiceEntry{Name: "monad", Address: "west", Port: 19004}, time.Millisecond, 4*time.Millisecond)
	waitFor(t, "a join attempt", func() bool { j, _ := f.snapshot(); return j >= 1 })
	cancel()
	time.Sleep(20 * time.Millisecond)
	before, _ := f.snapshot()
	time.Sleep(30 * time.Millisecond)
	if after, _ := f.snapshot(); after != before {
		t.Errorf("join attempts continued after cancel: %d -> %d", before, after)
	}
}
