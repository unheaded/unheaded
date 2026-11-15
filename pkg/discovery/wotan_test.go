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
	start(ctx, f, ServiceEntry{Name: "timeguru", Address: "west", Port: 19000}, time.Millisecond, 4*time.Millisecond, time.Hour)

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
	if got.Name != "timeguru" || got.Address != "west" || got.Port != 19000 || got.Health != "healthy" {
		t.Errorf("announced %+v, want timeguru at west:19000, healthy", got)
	}
}

// announceOnly is an Announcer and nothing more: Start cannot open a
// stream on it, so a stream cannot compile (ADR-147 Phase 7, Developer D3).
type announceOnly struct{ f *fakeTopics }

func (a announceOnly) Subscribe(ctx context.Context, topic, name string) (*wotanClient.Subscriber, error) {
	return a.f.Subscribe(ctx, topic, name)
}

func (a announceOnly) Publish(ctx context.Context, topic string, payload []byte) error {
	return a.f.Publish(ctx, topic, payload)
}

// TestStartAnnouncesAndDoesNotStream: announcing a service opens no stream
// and builds no registry; it publishes once on join and then every interval.
func TestStartAnnouncesAndDoesNotStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTopics{up: true}
	start(ctx, announceOnly{f}, ServiceEntry{Name: "sword", Address: "west", Port: 19011}, time.Millisecond, 4*time.Millisecond, 20*time.Millisecond)

	waitFor(t, "two announcements", func() bool { _, p := f.snapshot(); return len(p) >= 2 })
	f.mu.Lock()
	streams := len(f.streams)
	f.mu.Unlock()
	if streams != 0 {
		t.Fatalf("Start opened %d streams; announcing must open none", streams)
	}
}

// TestStart_NilClientIsQuiet: a process without a Wotan client announces
// nothing and does not panic.
func TestStart_NilClientIsQuiet(t *testing.T) {
	Start(context.Background(), nil, "kev-poller", 19120)
	var none *wotanClient.Client
	Start(context.Background(), none, "kev-poller", 19120)
}

// TestStart_PendingJoinRetries: a display name missing from Wotan's
// topics.auto_approve leaves the join pending; that is not a registration.
func TestStart_PendingJoinRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTopics{up: true, pending: true}
	start(ctx, f, ServiceEntry{Name: "chaos-controller", Address: "west", Port: 19300}, time.Millisecond, 4*time.Millisecond, time.Hour)

	waitFor(t, "repeated pending joins", func() bool { j, _ := f.snapshot(); return j >= 3 })
	if _, p := f.snapshot(); len(p) != 0 {
		t.Fatalf("published %d announcements on a pending membership", len(p))
	}
}

func TestStart_StopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &fakeTopics{}
	start(ctx, f, ServiceEntry{Name: "timeguru", Address: "west", Port: 19000}, time.Millisecond, 4*time.Millisecond, time.Hour)
	waitFor(t, "a join attempt", func() bool { j, _ := f.snapshot(); return j >= 1 })
	cancel()
	time.Sleep(20 * time.Millisecond)
	before, _ := f.snapshot()
	time.Sleep(30 * time.Millisecond)
	if after, _ := f.snapshot(); after != before {
		t.Errorf("join attempts continued after cancel: %d -> %d", before, after)
	}
}

// A new stream starts at seq 0 and replays system.discovery's retained
// history (up to 10,000 announcements, 100 per poll). Each replayed
// announcement was stamped LastSeen=now on arrival, so a service that died
// hours ago resolved as live for as long as the replay took plus the TTL.
// An announcement older than the TTL by Wotan's timestamp is history, not
// a sighting.
func TestStart_ReplayedAnnouncementsAreNotSightings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &fakeTopics{up: true}
	reg := New(&wotanTopics{ctx: ctx, c: f})
	if err := reg.Start(ctx); err != nil {
		t.Fatal(err)
	}

	ghost, _ := json.Marshal(ServiceEntry{Name: "ghost", Address: "east", Port: 19002})
	live, _ := json.Marshal(ServiceEntry{Name: "live", Address: "east", Port: 19003})
	f.mu.Lock()
	stream := f.streams[0]
	f.mu.Unlock()
	stream <- &wotanClient.Message{Payload: string(ghost), CreatedAt: time.Now().Add(-time.Hour)}
	stream <- &wotanClient.Message{Payload: string(live), CreatedAt: time.Now()}

	waitFor(t, "the live announcement", func() bool { _, err := reg.Resolve("live"); return err == nil })
	if e, err := reg.Resolve("ghost"); err == nil {
		t.Fatalf("an hour-old announcement resolved as live: %+v", e)
	}
}
