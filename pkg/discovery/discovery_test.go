// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Mock WotanClient
// ---------------------------------------------------------------------------

// mockWotan is a minimal in-process Wotan mock that supports Publish/Subscribe.
type mockWotan struct {
	mu       sync.Mutex
	handlers map[string][]func([]byte)

	// publishErr can be set to simulate Publish failures.
	publishErr error
	// published records all messages sent via Publish.
	published []publishedMsg
}

type publishedMsg struct {
	Topic string
	Data  []byte
}

func newMockWotan() *mockWotan {
	return &mockWotan{
		handlers: make(map[string][]func([]byte)),
	}
}

func (m *mockWotan) Publish(topic string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.publishErr != nil {
		return m.publishErr
	}

	m.published = append(m.published, publishedMsg{Topic: topic, Data: data})

	// Fan-out to subscribers.
	for _, h := range m.handlers[topic] {
		go h(data)
	}
	return nil
}

func (m *mockWotan) Subscribe(topic string, handler func([]byte)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[topic] = append(m.handlers[topic], handler)
	return nil
}

// inject simulates an external message arriving on a topic.
func (m *mockWotan) inject(topic string, data []byte) {
	m.mu.Lock()
	handlers := append([]func([]byte){}, m.handlers[topic]...)
	m.mu.Unlock()

	for _, h := range handlers {
		h(data)
	}
}

func (m *mockWotan) publishedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestRegisterAndResolve(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	entry := ServiceEntry{
		Name:    "timeguru",
		Address: "10.10.10.20",
		Port:    8000,
		Health:  "healthy",
		Metadata: map[string]string{
			"version": "v1",
		},
	}

	if err := reg.Register(entry); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := reg.Resolve("timeguru")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if got.Name != "timeguru" {
		t.Errorf("expected name timeguru, got %s", got.Name)
	}
	if got.Address != "10.10.10.20" {
		t.Errorf("expected address 10.10.10.20, got %s", got.Address)
	}
	if got.Port != 8000 {
		t.Errorf("expected port 8000, got %d", got.Port)
	}
	if got.Health != "healthy" {
		t.Errorf("expected health healthy, got %s", got.Health)
	}
	if got.Metadata["version"] != "v1" {
		t.Errorf("expected metadata version=v1, got %s", got.Metadata["version"])
	}
	if got.LastSeen.IsZero() {
		t.Error("expected LastSeen to be set")
	}

	// Verify a Wotan publish happened.
	if w.publishedCount() == 0 {
		t.Error("expected at least one Wotan publish for the announcement")
	}
}

func TestRegisterDefaultHealth(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	// Register without explicitly setting Health.
	err := reg.Register(ServiceEntry{
		Name:    "captain",
		Address: "10.10.10.21",
		Port:    8001,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := reg.Resolve("captain")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Health != "healthy" {
		t.Errorf("expected default health 'healthy', got %s", got.Health)
	}
}

func TestResolveUnknownService(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown service")
	}
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound, got %v", err)
	}
}

func TestStaleServiceExpiration(t *testing.T) {
	w := newMockWotan()
	reg := New(w)
	// Use a very short TTL for the test.
	reg.ttl = 50 * time.Millisecond

	// Inject a remote service via the handler.
	announcement, _ := json.Marshal(ServiceEntry{
		Name:    "remote-svc",
		Address: "10.10.10.99",
		Port:    9000,
		Health:  "healthy",
	})
	reg.handleAnnouncement(announcement)

	// Should be resolvable immediately.
	_, err := reg.Resolve("remote-svc")
	if err != nil {
		t.Fatalf("expected remote-svc to be resolvable: %v", err)
	}

	// Wait for TTL to expire, then reap.
	time.Sleep(100 * time.Millisecond)
	reg.reapStale()

	// Should now be gone.
	_, err = reg.Resolve("remote-svc")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected ErrServiceNotFound after TTL, got %v", err)
	}
}

func TestSelfNotReaped(t *testing.T) {
	w := newMockWotan()
	reg := New(w)
	reg.ttl = 10 * time.Millisecond

	err := reg.Register(ServiceEntry{
		Name:    "myself",
		Address: "127.0.0.1",
		Port:    8080,
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Wait past TTL and reap.
	time.Sleep(50 * time.Millisecond)
	reg.reapStale()

	// Self should still be present.
	_, err = reg.Resolve("myself")
	if err != nil {
		t.Fatalf("self should not be reaped: %v", err)
	}
}

func TestMultipleServices(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	services := []ServiceEntry{
		{Name: "timeguru", Address: "10.10.10.20", Port: 8000, Health: "healthy"},
		{Name: "captain", Address: "10.10.10.21", Port: 8001, Health: "healthy"},
		{Name: "architect", Address: "10.10.10.22", Port: 8002, Health: "degraded"},
	}

	for _, svc := range services {
		if err := reg.Register(svc); err != nil {
			t.Fatalf("Register %s failed: %v", svc.Name, err)
		}
	}

	for _, svc := range services {
		got, err := reg.Resolve(svc.Name)
		if err != nil {
			t.Errorf("Resolve %s failed: %v", svc.Name, err)
			continue
		}
		if got.Port != svc.Port {
			t.Errorf("expected port %d for %s, got %d", svc.Port, svc.Name, got.Port)
		}
	}
}

func TestResolveAllOnlyHealthy(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	_ = reg.Register(ServiceEntry{Name: "svc-a", Address: "1.1.1.1", Port: 1, Health: "healthy"})
	_ = reg.Register(ServiceEntry{Name: "svc-b", Address: "2.2.2.2", Port: 2, Health: "degraded"})
	_ = reg.Register(ServiceEntry{Name: "svc-c", Address: "3.3.3.3", Port: 3, Health: "healthy"})
	_ = reg.Register(ServiceEntry{Name: "svc-d", Address: "4.4.4.4", Port: 4, Health: "unhealthy"})

	all := reg.ResolveAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 healthy services, got %d", len(all))
	}

	names := make(map[string]bool)
	for _, e := range all {
		names[e.Name] = true
	}
	if !names["svc-a"] || !names["svc-c"] {
		t.Errorf("expected svc-a and svc-c, got %v", names)
	}
}

func TestHealthStatusUpdate(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	_ = reg.Register(ServiceEntry{Name: "captain", Address: "10.10.10.21", Port: 8001, Health: "healthy"})

	// Simulate a health update arriving via Wotan.
	update, _ := json.Marshal(ServiceEntry{
		Name:    "captain",
		Address: "10.10.10.21",
		Port:    8001,
		Health:  "degraded",
	})
	reg.handleAnnouncement(update)

	got, err := reg.Resolve("captain")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if got.Health != "degraded" {
		t.Errorf("expected health degraded, got %s", got.Health)
	}
}

func TestWatchCallbackFires(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	received := make(chan ServiceEntry, 1)
	reg.Watch(func(e ServiceEntry) {
		received <- e
	})

	_ = reg.Register(ServiceEntry{Name: "monad", Address: "10.10.10.23", Port: 8004, Health: "healthy"})

	select {
	case e := <-received:
		if e.Name != "monad" {
			t.Errorf("expected monad, got %s", e.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for watch callback")
	}
}

func TestWatchOnRemoteAnnouncement(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	received := make(chan ServiceEntry, 1)
	reg.Watch(func(e ServiceEntry) {
		received <- e
	})

	// Simulate a remote announcement.
	announcement, _ := json.Marshal(ServiceEntry{
		Name:    "sophia",
		Address: "10.10.10.24",
		Port:    8005,
		Health:  "healthy",
	})
	reg.handleAnnouncement(announcement)

	select {
	case e := <-received:
		if e.Name != "sophia" {
			t.Errorf("expected sophia, got %s", e.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for remote watch callback")
	}
}

func TestReAnnounceKeepsAlive(t *testing.T) {
	w := newMockWotan()
	reg := New(w)
	reg.ttl = 80 * time.Millisecond

	// Inject a remote service.
	announcement, _ := json.Marshal(ServiceEntry{
		Name:    "remote-svc",
		Address: "10.10.10.50",
		Port:    7000,
		Health:  "healthy",
	})
	reg.handleAnnouncement(announcement)

	// Wait partway through the TTL, then re-announce.
	time.Sleep(50 * time.Millisecond)
	reg.handleAnnouncement(announcement) // refreshes LastSeen

	// Wait a bit more (total ~80ms from re-announce) and reap.
	time.Sleep(50 * time.Millisecond)
	reg.reapStale()

	// Should still be alive because the re-announce reset LastSeen.
	_, err := reg.Resolve("remote-svc")
	if err != nil {
		t.Fatalf("expected service to survive re-announce, got: %v", err)
	}
}

func TestStartAndStop(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reg.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify subscribe was called.
	w.mu.Lock()
	subCount := len(w.handlers[DiscoveryTopic])
	w.mu.Unlock()
	if subCount == 0 {
		t.Error("expected subscription to discovery topic")
	}

	// Stop should not panic when called multiple times.
	reg.Stop()
	reg.Stop()
}

func TestStartSubscribeError(t *testing.T) {
	w := &failingWotan{subscribeErr: errors.New("connection refused")}
	reg := New(w)

	ctx := context.Background()
	err := reg.Start(ctx)
	if err == nil {
		t.Fatal("expected error from Start when Subscribe fails")
	}
}

func TestPublishError(t *testing.T) {
	w := newMockWotan()
	w.publishErr = errors.New("publish failed")

	reg := New(w)
	err := reg.Register(ServiceEntry{Name: "broken", Address: "1.2.3.4", Port: 1234})
	if err == nil {
		t.Fatal("expected error from Register when Publish fails")
	}
}

func TestHandleMalformedAnnouncement(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	// Should not panic.
	reg.handleAnnouncement([]byte("not valid json"))
	reg.handleAnnouncement([]byte(`{"name": ""}`)) // empty name

	_, err := reg.Resolve("")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("expected not found for empty name, got %v", err)
	}
}

func TestAnnouncementViaWotan(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reg.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Simulate a message arriving on the discovery topic.
	announcement, _ := json.Marshal(ServiceEntry{
		Name:    "external-svc",
		Address: "192.168.1.10",
		Port:    3000,
		Health:  "healthy",
	})
	w.inject(DiscoveryTopic, announcement)

	// Give the handler a moment to process.
	time.Sleep(50 * time.Millisecond)

	got, err := reg.Resolve("external-svc")
	if err != nil {
		t.Fatalf("expected to resolve external-svc: %v", err)
	}
	if got.Address != "192.168.1.10" {
		t.Errorf("expected 192.168.1.10, got %s", got.Address)
	}
}

func TestConcurrentAccess(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "svc"
			_ = reg.Register(ServiceEntry{Name: name, Address: "1.1.1.1", Port: 8000 + idx, Health: "healthy"})
			reg.Resolve(name)
			reg.ResolveAll()
		}(i)
	}
	wg.Wait()
}

func TestResolveReturnsCopy(t *testing.T) {
	w := newMockWotan()
	reg := New(w)

	_ = reg.Register(ServiceEntry{Name: "svc", Address: "1.1.1.1", Port: 8000, Health: "healthy"})

	got, _ := reg.Resolve("svc")
	got.Address = "mutated" // mutate the copy

	got2, _ := reg.Resolve("svc")
	if got2.Address == "mutated" {
		t.Error("Resolve should return a copy, but original was mutated")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// failingWotan is a WotanClient that always returns errors.
type failingWotan struct {
	subscribeErr error
}

func (f *failingWotan) Publish(topic string, data []byte) error {
	return nil
}

func (f *failingWotan) Subscribe(topic string, handler func([]byte)) error {
	return f.subscribeErr
}
