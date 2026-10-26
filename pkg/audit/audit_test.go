// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package audit_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/audit/storage"
)

// stubExporter is the smallest audit.Exporter; the registry test only needs
// something to register and look up by format.
type stubExporter struct{}

func (stubExporter) Export(_ context.Context, _ []*audit.AuditEvent, _ *audit.AuditFilter) (io.Reader, error) {
	return bytes.NewReader(nil), nil
}

func (stubExporter) Format() string { return "json" }

func TestAuditEvent(t *testing.T) {
	t.Run("validate event", func(t *testing.T) {
		event := &audit.AuditEvent{
			ID:        "test-1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Actor:     &audit.Actor{ID: "user-1"},
		}

		if err := event.Validate(); err != nil {
			t.Errorf("expected valid event, got error: %v", err)
		}
	})

	t.Run("validate event missing ID", func(t *testing.T) {
		event := &audit.AuditEvent{
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Actor:     &audit.Actor{ID: "user-1"},
		}

		if err := event.Validate(); err == nil {
			t.Error("expected error for missing ID")
		}
	})

	t.Run("compute hash", func(t *testing.T) {
		event := &audit.AuditEvent{
			ID:        "test-1",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
		}

		hash := event.ComputeHash()
		if hash == "" {
			t.Error("expected non-empty hash")
		}

		// Same event should produce same hash
		hash2 := event.ComputeHash()
		if hash != hash2 {
			t.Error("expected consistent hash")
		}
	})

	t.Run("verify hash", func(t *testing.T) {
		event := &audit.AuditEvent{
			ID:        "test-1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
		}

		event.Hash = event.ComputeHash()

		if !event.VerifyHash() {
			t.Error("expected hash verification to succeed")
		}

		// Modify event and verify fails
		event.Action = "modified"
		if event.VerifyHash() {
			t.Error("expected hash verification to fail after modification")
		}
	})
}

func TestEventBuilder(t *testing.T) {
	event := audit.NewEventBuilder().
		WithID("test-1").
		WithType(audit.EventTypeAuthentication).
		WithSeverity(audit.SeverityInfo).
		WithAction("login").
		WithOutcome(audit.OutcomeSuccess).
		WithActor(&audit.Actor{ID: "user-1", Type: "user", Name: "Test User"}).
		WithResource(&audit.Resource{ID: "app-1", Type: "application"}).
		WithSource(&audit.Source{IP: "192.168.1.1", Service: "auth-service"}).
		WithDetail("method", "password").
		Build()

	if event.ID != "test-1" {
		t.Errorf("expected ID 'test-1', got '%s'", event.ID)
	}
	if event.Type != audit.EventTypeAuthentication {
		t.Errorf("expected type '%s', got '%s'", audit.EventTypeAuthentication, event.Type)
	}
	if event.Actor == nil || event.Actor.ID != "user-1" {
		t.Error("expected actor with ID 'user-1'")
	}
	if event.Details["method"] != "password" {
		t.Error("expected detail 'method' to be 'password'")
	}
}

func TestMemoryStorage(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	t.Run("store and get", func(t *testing.T) {
		event := &audit.AuditEvent{
			ID:        "test-1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
			Actor:     &audit.Actor{ID: "user-1"},
			Hash:      "testhash",
		}

		if err := store.Store(ctx, event); err != nil {
			t.Fatalf("failed to store event: %v", err)
		}

		retrieved, err := store.Get(ctx, "test-1")
		if err != nil {
			t.Fatalf("failed to get event: %v", err)
		}

		if retrieved.ID != event.ID {
			t.Errorf("expected ID '%s', got '%s'", event.ID, retrieved.ID)
		}
	})

	t.Run("query by type", func(t *testing.T) {
		event1 := &audit.AuditEvent{
			ID:        "test-2",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthorization,
			Action:    "check",
			Actor:     &audit.Actor{ID: "user-1"},
			Hash:      "hash2",
		}
		event2 := &audit.AuditEvent{
			ID:        "test-3",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "logout",
			Actor:     &audit.Actor{ID: "user-2"},
			Hash:      "hash3",
		}

		store.Store(ctx, event1)
		store.Store(ctx, event2)

		q := &audit.AuditQuery{
			Types: []audit.EventType{audit.EventTypeAuthentication},
		}

		results, err := store.Query(ctx, q)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		for _, r := range results {
			if r.Type != audit.EventTypeAuthentication {
				t.Errorf("expected only authentication events, got %s", r.Type)
			}
		}
	})

	t.Run("count", func(t *testing.T) {
		count, err := store.Count(ctx, &audit.AuditQuery{})
		if err != nil {
			t.Fatalf("failed to count: %v", err)
		}

		if count == 0 {
			t.Error("expected count > 0")
		}
	})

	t.Run("get last hash", func(t *testing.T) {
		hash, err := store.GetLastHash(ctx)
		if err != nil {
			t.Fatalf("failed to get last hash: %v", err)
		}

		if hash == "" {
			t.Error("expected non-empty last hash")
		}
	})
}

func TestRegistry(t *testing.T) {
	registry := audit.NewRegistry()

	t.Run("register and get storage", func(t *testing.T) {
		registry.RegisterStorage("memory", func(config map[string]interface{}) (audit.Storage, error) {
			return storage.NewMemoryStorage(), nil
		})

		store, err := registry.GetStorage("memory", nil)
		if err != nil {
			t.Fatalf("failed to get storage: %v", err)
		}

		if store == nil {
			t.Error("expected non-nil storage")
		}
	})

	t.Run("get unknown storage", func(t *testing.T) {
		_, err := registry.GetStorage("unknown", nil)
		if err == nil {
			t.Error("expected error for unknown storage")
		}
	})

	t.Run("register and get exporter", func(t *testing.T) {
		registry.RegisterExporter(stubExporter{})

		exp, ok := registry.GetExporter("json")
		if !ok {
			t.Error("expected to find json exporter")
		}

		if exp.Format() != "json" {
			t.Errorf("expected format 'json', got '%s'", exp.Format())
		}
	})
}

func BenchmarkEventHash(b *testing.B) {
	event := &audit.AuditEvent{
		ID:        "bench-1",
		Timestamp: time.Now(),
		Type:      audit.EventTypeAuthentication,
		Action:    "login",
		Outcome:   audit.OutcomeSuccess,
		Details: map[string]interface{}{
			"method": "password",
			"client": "web",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event.ComputeHash()
	}
}

func BenchmarkMemoryStorageWrite(b *testing.B) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	event := &audit.AuditEvent{
		ID:        "bench-1",
		Timestamp: time.Now(),
		Type:      audit.EventTypeAuthentication,
		Action:    "login",
		Outcome:   audit.OutcomeSuccess,
		Actor:     &audit.Actor{ID: "user-1"},
		Hash:      "benchhash",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		event.ID = fmt.Sprintf("bench-%d", i)
		store.Store(ctx, event)
	}
}

func BenchmarkMemoryStorageQuery(b *testing.B) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	// Pre-populate with events
	for i := 0; i < 1000; i++ {
		event := &audit.AuditEvent{
			ID:        fmt.Sprintf("bench-%d", i),
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Actor:     &audit.Actor{ID: fmt.Sprintf("user-%d", i%10)},
			Hash:      fmt.Sprintf("hash-%d", i),
		}
		store.Store(ctx, event)
	}

	query := &audit.AuditQuery{
		Types: []audit.EventType{audit.EventTypeAuthentication},
		Limit: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Query(ctx, query)
	}
}
