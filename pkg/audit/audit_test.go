// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package audit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"unheaded/pkg/audit"
	"unheaded/pkg/audit/export"
	"unheaded/pkg/audit/logger"
	"unheaded/pkg/audit/query"
	"unheaded/pkg/audit/storage"
)

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

func TestQueryBuilder(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	// Add test events
	now := time.Now()
	events := []*audit.AuditEvent{
		{
			ID:        "q1",
			Timestamp: now.Add(-2 * time.Hour),
			Type:      audit.EventTypeAuthentication,
			Severity:  audit.SeverityInfo,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
			Actor:     &audit.Actor{ID: "user-1"},
			Hash:      "h1",
		},
		{
			ID:        "q2",
			Timestamp: now.Add(-1 * time.Hour),
			Type:      audit.EventTypeAuthorization,
			Severity:  audit.SeverityWarning,
			Action:    "access_denied",
			Outcome:   audit.OutcomeFailure,
			Actor:     &audit.Actor{ID: "user-2"},
			Hash:      "h2",
		},
		{
			ID:        "q3",
			Timestamp: now,
			Type:      audit.EventTypeConfiguration,
			Severity:  audit.SeverityCritical,
			Action:    "config_change",
			Outcome:   audit.OutcomeSuccess,
			Actor:     &audit.Actor{ID: "admin-1"},
			Hash:      "h3",
		},
	}

	for _, e := range events {
		store.Store(ctx, e)
	}

	t.Run("query last hours", func(t *testing.T) {
		q := query.NewQueryBuilder().
			LastHours(3).
			Build()

		results, err := store.Query(ctx, q)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("query by severity", func(t *testing.T) {
		q := query.NewQueryBuilder().
			WithSeverities(audit.SeverityCritical).
			Build()

		results, err := store.Query(ctx, q)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 critical result, got %d", len(results))
		}
	})

	t.Run("query with pagination", func(t *testing.T) {
		q := query.NewQueryBuilder().
			Limit(2).
			Build()

		results, err := store.Query(ctx, q)
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})
}

func TestSearcher(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	// Add test events
	events := []*audit.AuditEvent{
		{
			ID:        "s1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "user_login",
			Actor:     &audit.Actor{ID: "alice", Name: "Alice Smith"},
			Hash:      "hs1",
		},
		{
			ID:        "s2",
			Timestamp: time.Now(),
			Type:      audit.EventTypeDataAccess,
			Action:    "file_download",
			Actor:     &audit.Actor{ID: "bob", Name: "Bob Jones"},
			Resource:  &audit.Resource{ID: "doc-1", Path: "/files/secret.pdf"},
			Hash:      "hs2",
		},
	}

	for _, e := range events {
		store.Store(ctx, e)
	}

	searcher := query.NewSearcher(store)

	t.Run("search by actor name", func(t *testing.T) {
		opts := &query.SearchOptions{
			Query:  "Alice",
			Fields: []string{"actor"},
		}

		results, err := searcher.Search(ctx, opts)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("search by resource path", func(t *testing.T) {
		opts := &query.SearchOptions{
			Query:  "secret",
			Fields: []string{"resource"},
		}

		results, err := searcher.Search(ctx, opts)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})

	t.Run("case insensitive search", func(t *testing.T) {
		opts := &query.SearchOptions{
			Query:         "alice",
			CaseSensitive: false,
		}

		results, err := searcher.Search(ctx, opts)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
	})
}

func TestJSONExporter(t *testing.T) {
	ctx := context.Background()
	exporter := export.NewJSONExporter()

	events := []*audit.AuditEvent{
		{
			ID:        "e1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
			Actor:     &audit.Actor{ID: "user-1"},
		},
	}

	reader, err := exporter.Export(ctx, events, nil)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(reader)

	var exported []*audit.AuditEvent
	if err := json.Unmarshal(buf.Bytes(), &exported); err != nil {
		t.Fatalf("failed to unmarshal exported JSON: %v", err)
	}

	if len(exported) != 1 {
		t.Errorf("expected 1 exported event, got %d", len(exported))
	}
}

func TestCSVExporter(t *testing.T) {
	ctx := context.Background()
	exporter := export.NewCSVExporter()

	events := []*audit.AuditEvent{
		{
			ID:        "e1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Action:    "login",
			Outcome:   audit.OutcomeSuccess,
			Actor:     &audit.Actor{ID: "user-1", Type: "user", Name: "Test"},
			Hash:      "testhash",
		},
	}

	reader, err := exporter.Export(ctx, events, nil)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(reader)

	// Check that output contains expected headers and data
	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("id,timestamp")) {
		t.Error("expected CSV headers in output")
	}
	if !bytes.Contains([]byte(output), []byte("e1")) {
		t.Error("expected event ID in output")
	}
}

func TestCEFExporter(t *testing.T) {
	ctx := context.Background()
	exporter := export.NewCEFExporter("Unheaded", "AuditSystem", "1.0")

	events := []*audit.AuditEvent{
		{
			ID:        "e1",
			Timestamp: time.Now(),
			Type:      audit.EventTypeAuthentication,
			Severity:  audit.SeverityWarning,
			Action:    "login_failed",
			Outcome:   audit.OutcomeFailure,
			Actor:     &audit.Actor{ID: "user-1"},
			Source:    &audit.Source{IP: "192.168.1.1"},
			Hash:      "testhash",
		},
	}

	reader, err := exporter.Export(ctx, events, nil)
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(reader)

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("CEF:0|Unheaded|AuditSystem|1.0")) {
		t.Error("expected CEF header in output")
	}
}

func TestLogger(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()

	config := &audit.Config{
		BufferSize:            10,
		FlushInterval:         100 * time.Millisecond,
		AsyncWorkers:          2,
		EnableTamperDetection: true,
	}

	log, err := logger.New(config, store, nil)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	defer log.Close()

	t.Run("log event", func(t *testing.T) {
		event := audit.NewEventBuilder().
			WithType(audit.EventTypeAuthentication).
			WithSeverity(audit.SeverityInfo).
			WithAction("login").
			WithOutcome(audit.OutcomeSuccess).
			WithActor(&audit.Actor{ID: "user-1"}).
			Build()

		if err := log.Log(ctx, event); err != nil {
			t.Fatalf("failed to log event: %v", err)
		}

		// Wait for async processing
		time.Sleep(200 * time.Millisecond)

		// Query the event
		events, err := log.Query(ctx, &audit.AuditQuery{})
		if err != nil {
			t.Fatalf("failed to query events: %v", err)
		}

		if len(events) == 0 {
			t.Error("expected at least one event")
		}
	})

	t.Run("tamper detection", func(t *testing.T) {
		// Log multiple events
		for i := 0; i < 5; i++ {
			event := audit.NewEventBuilder().
				WithType(audit.EventTypeSystem).
				WithSeverity(audit.SeverityInfo).
				WithAction("test_action").
				WithOutcome(audit.OutcomeSuccess).
				WithActor(&audit.Actor{ID: "system"}).
				Build()

			if err := log.Log(ctx, event); err != nil {
				t.Fatalf("failed to log event: %v", err)
			}
		}

		// Wait for processing
		time.Sleep(200 * time.Millisecond)

		// Query all events
		events, err := log.Query(ctx, &audit.AuditQuery{})
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		// Verify hash chain
		if len(events) >= 2 {
			firstID := events[0].ID
			lastID := events[len(events)-1].ID

			valid, err := log.VerifyIntegrity(ctx, firstID, lastID)
			if err != nil {
				t.Fatalf("integrity check failed: %v", err)
			}

			if !valid {
				t.Error("expected hash chain to be valid")
			}
		}
	})
}

func TestAggregator(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	defer store.Close()

	// Add test events
	events := []*audit.AuditEvent{
		{ID: "a1", Timestamp: time.Now(), Type: audit.EventTypeAuthentication, Severity: audit.SeverityInfo, Actor: &audit.Actor{ID: "user-1"}, Hash: "h1"},
		{ID: "a2", Timestamp: time.Now(), Type: audit.EventTypeAuthentication, Severity: audit.SeverityWarning, Actor: &audit.Actor{ID: "user-1"}, Hash: "h2"},
		{ID: "a3", Timestamp: time.Now(), Type: audit.EventTypeAuthorization, Severity: audit.SeverityInfo, Actor: &audit.Actor{ID: "user-2"}, Hash: "h3"},
		{ID: "a4", Timestamp: time.Now(), Type: audit.EventTypeConfiguration, Severity: audit.SeverityCritical, Actor: &audit.Actor{ID: "admin-1"}, Hash: "h4"},
	}

	for _, e := range events {
		store.Store(ctx, e)
	}

	agg := query.NewAggregator(store)

	t.Run("count by type", func(t *testing.T) {
		counts, err := agg.CountByType(ctx, time.Now().Add(-1*time.Hour))
		if err != nil {
			t.Fatalf("aggregation failed: %v", err)
		}

		if counts[audit.EventTypeAuthentication] != 2 {
			t.Errorf("expected 2 auth events, got %d", counts[audit.EventTypeAuthentication])
		}
	})

	t.Run("count by severity", func(t *testing.T) {
		counts, err := agg.CountBySeverity(ctx, time.Now().Add(-1*time.Hour))
		if err != nil {
			t.Fatalf("aggregation failed: %v", err)
		}

		if counts[audit.SeverityInfo] != 2 {
			t.Errorf("expected 2 info events, got %d", counts[audit.SeverityInfo])
		}
	})

	t.Run("top actors", func(t *testing.T) {
		topActors, err := agg.TopActors(ctx, time.Now().Add(-1*time.Hour), 2)
		if err != nil {
			t.Fatalf("aggregation failed: %v", err)
		}

		if len(topActors) == 0 {
			t.Error("expected at least one actor")
		}

		if topActors[0].ActorID != "user-1" {
			t.Errorf("expected user-1 as top actor, got %s", topActors[0].ActorID)
		}
	})
}

func TestFilterBuilder(t *testing.T) {
	events := []*audit.AuditEvent{
		{
			ID:     "f1",
			Actor:  &audit.Actor{ID: "user-1", Roles: []string{"admin", "user"}},
			Source: &audit.Source{IP: "192.168.1.1", Service: "web"},
		},
		{
			ID:       "f2",
			Actor:    &audit.Actor{ID: "user-2", Roles: []string{"user"}},
			Source:   &audit.Source{IP: "10.0.0.1", Service: "api"},
			Resource: &audit.Resource{Path: "/api/v1/users"},
		},
		{
			ID:     "f3",
			Actor:  &audit.Actor{ID: "service-1", Roles: []string{"service"}},
			Source: &audit.Source{IP: "127.0.0.1", Service: "internal"},
		},
	}

	t.Run("filter by role", func(t *testing.T) {
		filter := query.NewFilterBuilder().
			WithActorRole("admin").
			Apply(events)

		if len(filter) != 1 {
			t.Errorf("expected 1 event with admin role, got %d", len(filter))
		}
	})

	t.Run("filter by IP", func(t *testing.T) {
		filter := query.NewFilterBuilder().
			WithSourceIP("192.168.1.1").
			Apply(events)

		if len(filter) != 1 {
			t.Errorf("expected 1 event from IP, got %d", len(filter))
		}
	})

	t.Run("filter by service", func(t *testing.T) {
		filter := query.NewFilterBuilder().
			WithSourceService("api").
			Apply(events)

		if len(filter) != 1 {
			t.Errorf("expected 1 event from api service, got %d", len(filter))
		}
	})

	t.Run("filter by resource path", func(t *testing.T) {
		filter := query.NewFilterBuilder().
			WithResourcePath("/api/").
			Apply(events)

		if len(filter) != 1 {
			t.Errorf("expected 1 event with api path, got %d", len(filter))
		}
	})

	t.Run("combined filters", func(t *testing.T) {
		filter := query.NewFilterBuilder().
			WithActorRole("user").
			WithSourceService("web").
			Apply(events)

		if len(filter) != 1 {
			t.Errorf("expected 1 event matching both conditions, got %d", len(filter))
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
		registry.RegisterExporter(export.NewJSONExporter())

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
