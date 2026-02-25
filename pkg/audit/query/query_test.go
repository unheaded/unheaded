// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package query

import (
	"context"
	"fmt"
	"testing"
	"time"

	"unheaded/pkg/audit"
)

// --- mock storage ---

type mockStorage struct {
	events []*audit.AuditEvent
}

func newMockStorage(events []*audit.AuditEvent) *mockStorage {
	return &mockStorage{events: events}
}

func (m *mockStorage) Store(ctx context.Context, event *audit.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockStorage) StoreBatch(ctx context.Context, events []*audit.AuditEvent) error {
	m.events = append(m.events, events...)
	return nil
}

func (m *mockStorage) Get(ctx context.Context, id string) (*audit.AuditEvent, error) {
	for _, e := range m.events {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", id)
}

func (m *mockStorage) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	var results []*audit.AuditEvent
	for _, e := range m.events {
		if !query.StartTime.IsZero() && e.Timestamp.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && e.Timestamp.After(query.EndTime) {
			continue
		}
		if len(query.Types) > 0 {
			found := false
			for _, t := range query.Types {
				if e.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(query.Severities) > 0 {
			found := false
			for _, s := range query.Severities {
				if e.Severity == s {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(query.ActorIDs) > 0 && e.Actor != nil {
			found := false
			for _, id := range query.ActorIDs {
				if e.Actor.ID == id {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(query.Actions) > 0 {
			found := false
			for _, a := range query.Actions {
				if e.Action == a {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(query.Outcomes) > 0 {
			found := false
			for _, o := range query.Outcomes {
				if e.Outcome == o {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(query.ResourceTypes) > 0 && e.Resource != nil {
			found := false
			for _, rt := range query.ResourceTypes {
				if e.Resource.Type == rt {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		results = append(results, e)
	}
	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	}
	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}
	return results, nil
}

func (m *mockStorage) GetLastHash(ctx context.Context) (string, error) {
	return "", nil
}

func (m *mockStorage) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	// Count ignores Limit/Offset to return true total matching count
	countQuery := *query
	countQuery.Limit = 0
	countQuery.Offset = 0
	events, err := m.Query(ctx, &countQuery)
	if err != nil {
		return 0, err
	}
	return int64(len(events)), nil
}

func (m *mockStorage) Close() error {
	return nil
}

// --- helpers ---

func makeEvent(id string, ts time.Time, et audit.EventType, sev audit.Severity, action string, outcome audit.Outcome) *audit.AuditEvent {
	return &audit.AuditEvent{
		ID:        id,
		Timestamp: ts,
		Type:      et,
		Severity:  sev,
		Action:    action,
		Outcome:   outcome,
		Actor:     &audit.Actor{ID: "actor-" + id, Type: "user", Name: "User-" + id, Roles: []string{"admin"}},
		Resource:  &audit.Resource{ID: "res-" + id, Type: "document", Name: "doc-" + id, Path: "/docs/" + id},
		Source:    &audit.Source{IP: "10.0.0.1", Service: "gateway", RequestID: "req-" + id},
		Details:   map[string]interface{}{"key": "value-" + id},
		Hash:      "hash-" + id,
	}
}

func sampleEvents() []*audit.AuditEvent {
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	return []*audit.AuditEvent{
		makeEvent("e1", base, audit.EventTypeAuthentication, audit.SeverityInfo, "login", audit.OutcomeSuccess),
		makeEvent("e2", base.Add(time.Hour), audit.EventTypeAuthentication, audit.SeverityWarning, "login", audit.OutcomeFailure),
		makeEvent("e3", base.Add(2*time.Hour), audit.EventTypeSystem, audit.SeverityCritical, "restart", audit.OutcomeSuccess),
		makeEvent("e4", base.Add(3*time.Hour), audit.EventTypeConfiguration, audit.SeverityInfo, "config.update", audit.OutcomeSuccess),
		makeEvent("e5", base.Add(4*time.Hour), audit.EventTypeDataAccess, audit.SeverityError, "data.read", audit.OutcomeFailure),
	}
}

// ===================== QueryBuilder Tests =====================

func TestNewQueryBuilder(t *testing.T) {
	b := NewQueryBuilder()
	q := b.Build()
	if q == nil {
		t.Fatal("Build returned nil")
	}
}

func TestQueryBuilder_Since(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	q := NewQueryBuilder().Since(ts).Build()
	if !q.StartTime.Equal(ts) {
		t.Errorf("expected start time %v, got %v", ts, q.StartTime)
	}
}

func TestQueryBuilder_Until(t *testing.T) {
	ts := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	q := NewQueryBuilder().Until(ts).Build()
	if !q.EndTime.Equal(ts) {
		t.Errorf("expected end time %v, got %v", ts, q.EndTime)
	}
}

func TestQueryBuilder_LastHours(t *testing.T) {
	before := time.Now().UTC()
	q := NewQueryBuilder().LastHours(24).Build()
	after := time.Now().UTC()

	if q.EndTime.Before(before) || q.EndTime.After(after) {
		t.Error("end time not in expected range")
	}
	expectedStart := q.EndTime.Add(-24 * time.Hour)
	diff := q.StartTime.Sub(expectedStart)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("start time off by %v", diff)
	}
}

func TestQueryBuilder_LastDays(t *testing.T) {
	q := NewQueryBuilder().LastDays(7).Build()
	dur := q.EndTime.Sub(q.StartTime)
	expected := 7 * 24 * time.Hour
	if dur < expected-time.Second || dur > expected+time.Second {
		t.Errorf("expected duration ~%v, got %v", expected, dur)
	}
}

func TestQueryBuilder_WithTypes(t *testing.T) {
	q := NewQueryBuilder().
		WithTypes(audit.EventTypeAuthentication, audit.EventTypeSystem).
		Build()
	if len(q.Types) != 2 {
		t.Errorf("expected 2 types, got %d", len(q.Types))
	}
}

func TestQueryBuilder_WithSeverities(t *testing.T) {
	q := NewQueryBuilder().
		WithSeverities(audit.SeverityCritical, audit.SeverityError).
		Build()
	if len(q.Severities) != 2 {
		t.Errorf("expected 2 severities, got %d", len(q.Severities))
	}
}

func TestQueryBuilder_WithActors(t *testing.T) {
	q := NewQueryBuilder().WithActors("alice", "bob").Build()
	if len(q.ActorIDs) != 2 {
		t.Errorf("expected 2 actor IDs, got %d", len(q.ActorIDs))
	}
}

func TestQueryBuilder_WithResourceTypes(t *testing.T) {
	q := NewQueryBuilder().WithResourceTypes("document", "config").Build()
	if len(q.ResourceTypes) != 2 {
		t.Errorf("expected 2 resource types, got %d", len(q.ResourceTypes))
	}
}

func TestQueryBuilder_WithActions(t *testing.T) {
	q := NewQueryBuilder().WithActions("login", "logout").Build()
	if len(q.Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(q.Actions))
	}
}

func TestQueryBuilder_WithOutcomes(t *testing.T) {
	q := NewQueryBuilder().WithOutcomes(audit.OutcomeSuccess, audit.OutcomeFailure).Build()
	if len(q.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(q.Outcomes))
	}
}

func TestQueryBuilder_Limit(t *testing.T) {
	q := NewQueryBuilder().Limit(50).Build()
	if q.Limit != 50 {
		t.Errorf("expected limit 50, got %d", q.Limit)
	}
}

func TestQueryBuilder_Offset(t *testing.T) {
	q := NewQueryBuilder().Offset(10).Build()
	if q.Offset != 10 {
		t.Errorf("expected offset 10, got %d", q.Offset)
	}
}

func TestQueryBuilder_OrderBy(t *testing.T) {
	q := NewQueryBuilder().OrderBy("timestamp").Build()
	if q.OrderBy != "timestamp" {
		t.Errorf("expected order by timestamp, got %s", q.OrderBy)
	}
}

func TestQueryBuilder_Descending(t *testing.T) {
	q := NewQueryBuilder().Descending().Build()
	if !q.Descending {
		t.Error("expected descending true")
	}
}

func TestQueryBuilder_Ascending(t *testing.T) {
	q := NewQueryBuilder().Descending().Ascending().Build()
	if q.Descending {
		t.Error("expected descending false after Ascending()")
	}
}

func TestQueryBuilder_Chaining(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	q := NewQueryBuilder().
		Since(base).
		Until(base.Add(24 * time.Hour)).
		WithTypes(audit.EventTypeAuthentication).
		WithSeverities(audit.SeverityCritical).
		WithActors("alice").
		WithActions("login").
		WithOutcomes(audit.OutcomeFailure).
		Limit(100).
		Offset(0).
		OrderBy("timestamp").
		Descending().
		Build()

	if q.StartTime.IsZero() {
		t.Error("start time not set")
	}
	if len(q.Types) != 1 {
		t.Error("types not set")
	}
	if q.Limit != 100 {
		t.Error("limit not set")
	}
	if !q.Descending {
		t.Error("descending not set")
	}
}

func TestQueryBuilder_Execute(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)

	results, err := NewQueryBuilder().
		WithTypes(audit.EventTypeAuthentication).
		Execute(context.Background(), storage)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 auth events, got %d", len(results))
	}
}

// ===================== QueryExecutor Tests =====================

func TestQueryExecutor_Query(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	results, err := exec.Query(context.Background(), &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5, got %d", len(results))
	}
}

func TestQueryExecutor_Count(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	count, err := exec.Count(context.Background(), &audit.AuditQuery{})
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestQueryExecutor_GetByID(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	ev, err := exec.GetByID(context.Background(), "e1")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if ev.ID != "e1" {
		t.Errorf("expected e1, got %s", ev.ID)
	}

	_, err = exec.GetByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestQueryExecutor_GetAuthenticationEvents(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	results, err := exec.GetAuthenticationEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("GetAuthenticationEvents failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}
}

func TestQueryExecutor_GetFailedAttempts(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	results, err := exec.GetFailedAttempts(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("GetFailedAttempts failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 failures, got %d", len(results))
	}
}

func TestQueryExecutor_GetCriticalEvents(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	results, err := exec.GetCriticalEvents(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("GetCriticalEvents failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 critical, got %d", len(results))
	}
}

func TestQueryExecutor_GetActorActivity(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	results, err := exec.GetActorActivity(context.Background(), "actor-e1", time.Time{})
	if err != nil {
		t.Fatalf("GetActorActivity failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}

func TestQueryExecutor_QueryWithPagination(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	exec := NewQueryExecutor(storage)

	result, err := exec.QueryWithPagination(context.Background(), &audit.AuditQuery{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("QueryWithPagination failed: %v", err)
	}
	if len(result.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(result.Events))
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
	if !result.HasMore {
		t.Error("expected HasMore true")
	}
	if result.NextOffset != 2 {
		t.Errorf("expected next offset 2, got %d", result.NextOffset)
	}

	// Last page
	result, err = exec.QueryWithPagination(context.Background(), &audit.AuditQuery{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("QueryWithPagination failed: %v", err)
	}
	if result.HasMore {
		t.Error("expected HasMore false for last page")
	}
}

// ===================== Aggregator Tests =====================

func TestAggregator_CountByType(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	counts, err := agg.CountByType(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("CountByType failed: %v", err)
	}
	if counts[audit.EventTypeAuthentication] != 2 {
		t.Errorf("expected 2 auth events, got %d", counts[audit.EventTypeAuthentication])
	}
	if counts[audit.EventTypeSystem] != 1 {
		t.Errorf("expected 1 system event, got %d", counts[audit.EventTypeSystem])
	}
}

func TestAggregator_CountBySeverity(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	counts, err := agg.CountBySeverity(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("CountBySeverity failed: %v", err)
	}
	if counts[audit.SeverityInfo] != 2 {
		t.Errorf("expected 2 info, got %d", counts[audit.SeverityInfo])
	}
	if counts[audit.SeverityCritical] != 1 {
		t.Errorf("expected 1 critical, got %d", counts[audit.SeverityCritical])
	}
}

func TestAggregator_CountByOutcome(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	counts, err := agg.CountByOutcome(context.Background(), time.Time{})
	if err != nil {
		t.Fatalf("CountByOutcome failed: %v", err)
	}
	if counts[audit.OutcomeSuccess] != 3 {
		t.Errorf("expected 3 success, got %d", counts[audit.OutcomeSuccess])
	}
	if counts[audit.OutcomeFailure] != 2 {
		t.Errorf("expected 2 failure, got %d", counts[audit.OutcomeFailure])
	}
}

func TestAggregator_TopActors(t *testing.T) {
	events := sampleEvents()
	// Add more events for one actor
	extra := makeEvent("extra1", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	extra.Actor = &audit.Actor{ID: "actor-e1"}
	events = append(events, extra)

	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	top, err := agg.TopActors(context.Background(), time.Time{}, 3)
	if err != nil {
		t.Fatalf("TopActors failed: %v", err)
	}
	if len(top) != 3 {
		t.Errorf("expected 3, got %d", len(top))
	}
	// actor-e1 should be first with 2 events
	if top[0].ActorID != "actor-e1" {
		t.Errorf("expected actor-e1 first, got %s", top[0].ActorID)
	}
	if top[0].Count != 2 {
		t.Errorf("expected count 2 for top actor, got %d", top[0].Count)
	}
}

func TestAggregator_TopActors_NoLimit(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	top, err := agg.TopActors(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("TopActors failed: %v", err)
	}
	if len(top) != 5 {
		t.Errorf("expected 5 actors (no limit), got %d", len(top))
	}
}

func TestAggregator_EventTimeSeries(t *testing.T) {
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	events := []*audit.AuditEvent{
		makeEvent("ts1", base.Add(10*time.Minute), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess),
		makeEvent("ts2", base.Add(20*time.Minute), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess),
		makeEvent("ts3", base.Add(70*time.Minute), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess),
	}

	storage := newMockStorage(events)
	agg := NewAggregator(storage)

	series, err := agg.EventTimeSeries(context.Background(), base, base.Add(2*time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("EventTimeSeries failed: %v", err)
	}
	if len(series) != 2 {
		t.Errorf("expected 2 buckets, got %d", len(series))
	}

	// First bucket: 2 events
	if series[0].Count != 2 {
		t.Errorf("expected 2 in first bucket, got %d", series[0].Count)
	}
	// Second bucket: 1 event
	if series[1].Count != 1 {
		t.Errorf("expected 1 in second bucket, got %d", series[1].Count)
	}
}

// ===================== Searcher Tests =====================

func TestSearcher_Search_SimpleText(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query: "login",
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'login', got %d", len(results))
	}
}

func TestSearcher_Search_CaseInsensitive(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:         "LOGIN",
		CaseSensitive: false,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 case-insensitive results, got %d", len(results))
	}
}

func TestSearcher_Search_CaseSensitive(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:         "LOGIN",
		CaseSensitive: true,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 case-sensitive results for 'LOGIN', got %d", len(results))
	}
}

func TestSearcher_Search_Regex(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:    "log(in|out)",
		UseRegex: true,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 regex results, got %d", len(results))
	}
}

func TestSearcher_Search_InvalidRegex(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	_, err := s.Search(context.Background(), &SearchOptions{
		Query:    "[invalid",
		UseRegex: true,
	})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestSearcher_Search_ByFields(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	// Search only in action field
	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "login",
		Fields: []string{"action"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}

	// Search in id field
	results, err = s.Search(context.Background(), &SearchOptions{
		Query:  "e1",
		Fields: []string{"id"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}

func TestSearcher_Search_ByActorField(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "actor-e1",
		Fields: []string{"actor"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}

func TestSearcher_Search_ByResourceField(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "/docs/e3",
		Fields: []string{"resource"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}

func TestSearcher_Search_BySourceField(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "gateway",
		Fields: []string{"source"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5, got %d", len(results))
	}
}

func TestSearcher_Search_ByDetailsField(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "value-e1",
		Fields: []string{"details"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}
}

func TestSearcher_Search_ByTypeField(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "authentication",
		Fields: []string{"type"},
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}
}

func TestSearcher_Search_Pagination(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:  "10.0.0.1",
		Limit:  2,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 (paginated), got %d", len(results))
	}
}

func TestSearcher_SearchWithMetadata(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	result, err := s.SearchWithMetadata(context.Background(), &SearchOptions{
		Query:  "10.0.0.1",
		Limit:  2,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("SearchWithMetadata failed: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
	if len(result.Events) != 2 {
		t.Errorf("expected 2 events, got %d", len(result.Events))
	}
	if !result.HasMore {
		t.Error("expected HasMore true")
	}
	if result.NextOffset != 2 {
		t.Errorf("expected next offset 2, got %d", result.NextOffset)
	}
	if result.Query != "10.0.0.1" {
		t.Errorf("expected query '10.0.0.1', got %q", result.Query)
	}
	if result.SearchTime <= 0 {
		t.Error("expected positive search time")
	}
}

func TestSearcher_SearchWithMetadata_NoMore(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	result, err := s.SearchWithMetadata(context.Background(), &SearchOptions{
		Query: "10.0.0.1",
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("SearchWithMetadata failed: %v", err)
	}
	if result.HasMore {
		t.Error("expected HasMore false")
	}
}

func TestSearcher_Search_NestedDetails(t *testing.T) {
	ev := makeEvent("nested", time.Now().UTC(), audit.EventTypeSystem, audit.SeverityInfo, "test", audit.OutcomeSuccess)
	ev.Details = map[string]interface{}{
		"outer": map[string]interface{}{
			"inner": "deep-value",
		},
		"list": []interface{}{"item1", "item2"},
	}

	storage := newMockStorage([]*audit.AuditEvent{ev})
	s := NewSearcher(storage)

	results, _ := s.Search(context.Background(), &SearchOptions{
		Query:  "deep-value",
		Fields: []string{"details"},
	})
	if len(results) != 1 {
		t.Errorf("expected 1 for nested value, got %d", len(results))
	}

	results, _ = s.Search(context.Background(), &SearchOptions{
		Query:  "item2",
		Fields: []string{"details"},
	})
	if len(results) != 1 {
		t.Errorf("expected 1 for list value, got %d", len(results))
	}
}

// ===================== FilterBuilder Tests =====================

func TestFilterBuilder_NoFilters(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder()
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected all 5 events, got %d", len(result))
	}
}

func TestFilterBuilder_WithActorRole(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithActorRole("admin")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5 (all have admin role), got %d", len(result))
	}

	fb2 := NewFilterBuilder().WithActorRole("viewer")
	result2 := fb2.Apply(events)
	if len(result2) != 0 {
		t.Errorf("expected 0, got %d", len(result2))
	}
}

func TestFilterBuilder_WithActorRole_NilActor(t *testing.T) {
	ev := &audit.AuditEvent{ID: "no-actor"}
	fb := NewFilterBuilder().WithActorRole("admin")
	result := fb.Apply([]*audit.AuditEvent{ev})
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}
}

func TestFilterBuilder_WithSourceIP(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithSourceIP("10.0.0.1")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5, got %d", len(result))
	}

	fb2 := NewFilterBuilder().WithSourceIP("192.168.1.1")
	result2 := fb2.Apply(events)
	if len(result2) != 0 {
		t.Errorf("expected 0, got %d", len(result2))
	}
}

func TestFilterBuilder_WithSourceService(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithSourceService("gateway")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5, got %d", len(result))
	}
}

func TestFilterBuilder_WithResourcePath(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithResourcePath("/docs/")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5, got %d", len(result))
	}

	fb2 := NewFilterBuilder().WithResourcePath("/other/")
	result2 := fb2.Apply(events)
	if len(result2) != 0 {
		t.Errorf("expected 0, got %d", len(result2))
	}
}

func TestFilterBuilder_WithDetailKey(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithDetailKey("key")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5, got %d", len(result))
	}

	fb2 := NewFilterBuilder().WithDetailKey("nonexistent")
	result2 := fb2.Apply(events)
	if len(result2) != 0 {
		t.Errorf("expected 0, got %d", len(result2))
	}
}

func TestFilterBuilder_WithDetailValue(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().WithDetailValue("key", "value-e1")
	result := fb.Apply(events)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestFilterBuilder_Custom(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().Custom(func(e *audit.AuditEvent) bool {
		return e.Severity == audit.SeverityCritical
	})
	result := fb.Apply(events)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestFilterBuilder_MultipleFilters(t *testing.T) {
	events := sampleEvents()
	fb := NewFilterBuilder().
		WithSourceIP("10.0.0.1").
		WithActorRole("admin").
		WithDetailKey("key")
	result := fb.Apply(events)
	if len(result) != 5 {
		t.Errorf("expected 5, got %d", len(result))
	}
}

// ===================== IndexedSearcher Tests =====================

func TestIndexedSearcher_Index(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	is := NewIndexedSearcher(storage)

	if err := is.Index(context.Background()); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	if len(is.byActor) != 5 {
		t.Errorf("expected 5 actor entries, got %d", len(is.byActor))
	}
	if len(is.byIP) != 1 {
		t.Errorf("expected 1 IP entry, got %d", len(is.byIP))
	}
}

func TestIndexedSearcher_FindByActor(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	is := NewIndexedSearcher(storage)
	_ = is.Index(context.Background())

	results, err := is.FindByActor(context.Background(), "actor-e1")
	if err != nil {
		t.Fatalf("FindByActor failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1, got %d", len(results))
	}

	results, _ = is.FindByActor(context.Background(), "nonexistent")
	if results != nil {
		t.Errorf("expected nil for nonexistent actor, got %d results", len(results))
	}
}

func TestIndexedSearcher_FindByIP(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	is := NewIndexedSearcher(storage)
	_ = is.Index(context.Background())

	results, err := is.FindByIP(context.Background(), "10.0.0.1")
	if err != nil {
		t.Fatalf("FindByIP failed: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5, got %d", len(results))
	}

	results, _ = is.FindByIP(context.Background(), "192.168.1.1")
	if results != nil {
		t.Errorf("expected nil for nonexistent IP, got %d results", len(results))
	}
}

func TestIndexedSearcher_FindByType(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	is := NewIndexedSearcher(storage)
	_ = is.Index(context.Background())

	results, err := is.FindByType(context.Background(), audit.EventTypeAuthentication)
	if err != nil {
		t.Fatalf("FindByType failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2, got %d", len(results))
	}

	results, _ = is.FindByType(context.Background(), audit.EventType("nonexistent"))
	if results != nil {
		t.Errorf("expected nil for nonexistent type, got %d results", len(results))
	}
}

// ===================== searchDetails / searchValue Tests =====================

func TestSearcher_SearchDetails_KeyMatch(t *testing.T) {
	s := NewSearcher(nil)
	details := map[string]interface{}{
		"matching_key": "irrelevant",
	}
	if !s.searchDetails(details, func(s string) bool { return s == "matching_key" }) {
		t.Error("expected key match")
	}
}

func TestSearcher_SearchDetails_NilDetails(t *testing.T) {
	s := NewSearcher(nil)
	if s.searchDetails(nil, func(s string) bool { return true }) {
		t.Error("nil details should return false")
	}
}

func TestSearcher_SearchValue_Types(t *testing.T) {
	s := NewSearcher(nil)
	matcher := func(str string) bool { return str == "target" }

	// String match
	if !s.searchValue("target", matcher) {
		t.Error("expected string match")
	}

	// String no match
	if s.searchValue("other", matcher) {
		t.Error("expected no match for 'other'")
	}

	// Nested map
	if !s.searchValue(map[string]interface{}{"k": "target"}, matcher) {
		t.Error("expected nested map match")
	}

	// Slice
	if !s.searchValue([]interface{}{"a", "target", "b"}, matcher) {
		t.Error("expected slice match")
	}

	// Non-string value
	if s.searchValue(42, matcher) {
		t.Error("expected no match for int")
	}
}

func TestSearcher_Search_RegexCaseSensitive(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	results, err := s.Search(context.Background(), &SearchOptions{
		Query:         "Login",
		UseRegex:      true,
		CaseSensitive: true,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	// "Login" with capital L should not match "login"
	if len(results) != 0 {
		t.Errorf("expected 0 for case-sensitive regex, got %d", len(results))
	}
}

func TestSearcher_Search_NilActorResourceSource(t *testing.T) {
	ev := &audit.AuditEvent{
		ID:        "bare",
		Timestamp: time.Now().UTC(),
		Type:      audit.EventTypeSystem,
		Severity:  audit.SeverityInfo,
		Action:    "test",
		Outcome:   audit.OutcomeSuccess,
		Hash:      "h",
	}
	storage := newMockStorage([]*audit.AuditEvent{ev})
	s := NewSearcher(storage)

	results, _ := s.Search(context.Background(), &SearchOptions{
		Query:  "nonexistent",
		Fields: []string{"actor", "resource", "source", "details"},
	})
	if len(results) != 0 {
		t.Errorf("expected 0, got %d", len(results))
	}
}

func TestSearcher_SearchWithMetadata_HighOffset(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	result, err := s.SearchWithMetadata(context.Background(), &SearchOptions{
		Query:  "10.0.0.1",
		Limit:  10,
		Offset: 100,
	})
	if err != nil {
		t.Fatalf("SearchWithMetadata failed: %v", err)
	}
	if len(result.Events) != 0 {
		t.Errorf("expected 0 events with high offset, got %d", len(result.Events))
	}
	if result.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Total)
	}
}

func TestSearcher_SearchWithMetadata_NoLimit(t *testing.T) {
	events := sampleEvents()
	storage := newMockStorage(events)
	s := NewSearcher(storage)

	result, err := s.SearchWithMetadata(context.Background(), &SearchOptions{
		Query: "10.0.0.1",
		Limit: 0,
	})
	if err != nil {
		t.Fatalf("SearchWithMetadata failed: %v", err)
	}
	if len(result.Events) != 5 {
		t.Errorf("expected all 5 events with no limit, got %d", len(result.Events))
	}
}

func TestFilterBuilder_NilSourceAndResource(t *testing.T) {
	ev := &audit.AuditEvent{
		ID:      "no-source",
		Details: map[string]interface{}{},
	}

	fb := NewFilterBuilder().WithSourceIP("10.0.0.1")
	result := fb.Apply([]*audit.AuditEvent{ev})
	if len(result) != 0 {
		t.Error("expected 0 for nil source with IP filter")
	}

	fb2 := NewFilterBuilder().WithResourcePath("/docs/")
	result2 := fb2.Apply([]*audit.AuditEvent{ev})
	if len(result2) != 0 {
		t.Error("expected 0 for nil resource with path filter")
	}

	fb3 := NewFilterBuilder().WithSourceService("gw")
	result3 := fb3.Apply([]*audit.AuditEvent{ev})
	if len(result3) != 0 {
		t.Error("expected 0 for nil source with service filter")
	}
}

func TestFilterBuilder_WithDetailValue_MissingKey(t *testing.T) {
	ev := &audit.AuditEvent{
		ID:      "empty-details",
		Details: map[string]interface{}{},
	}
	fb := NewFilterBuilder().WithDetailValue("missing", "val")
	result := fb.Apply([]*audit.AuditEvent{ev})
	if len(result) != 0 {
		t.Error("expected 0 for missing detail key")
	}
}
