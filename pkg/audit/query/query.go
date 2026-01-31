// Package query provides audit event query and search capabilities.
package query

import (
	"context"
	"fmt"
	"time"

	"github.com/unheaded/unheaded/pkg/audit"
)

// QueryBuilder provides a fluent interface for building audit queries.
type QueryBuilder struct {
	query *audit.AuditQuery
}

// NewQueryBuilder creates a new QueryBuilder.
func NewQueryBuilder() *QueryBuilder {
	return &QueryBuilder{
		query: &audit.AuditQuery{},
	}
}

// Since sets the start time.
func (b *QueryBuilder) Since(t time.Time) *QueryBuilder {
	b.query.StartTime = t
	return b
}

// Until sets the end time.
func (b *QueryBuilder) Until(t time.Time) *QueryBuilder {
	b.query.EndTime = t
	return b
}

// LastHours sets the time range to the last N hours.
func (b *QueryBuilder) LastHours(hours int) *QueryBuilder {
	b.query.EndTime = time.Now().UTC()
	b.query.StartTime = b.query.EndTime.Add(-time.Duration(hours) * time.Hour)
	return b
}

// LastDays sets the time range to the last N days.
func (b *QueryBuilder) LastDays(days int) *QueryBuilder {
	b.query.EndTime = time.Now().UTC()
	b.query.StartTime = b.query.EndTime.Add(-time.Duration(days) * 24 * time.Hour)
	return b
}

// WithTypes filters by event types.
func (b *QueryBuilder) WithTypes(types ...audit.EventType) *QueryBuilder {
	b.query.Types = append(b.query.Types, types...)
	return b
}

// WithSeverities filters by severities.
func (b *QueryBuilder) WithSeverities(severities ...audit.Severity) *QueryBuilder {
	b.query.Severities = append(b.query.Severities, severities...)
	return b
}

// WithActors filters by actor IDs.
func (b *QueryBuilder) WithActors(actorIDs ...string) *QueryBuilder {
	b.query.ActorIDs = append(b.query.ActorIDs, actorIDs...)
	return b
}

// WithResourceTypes filters by resource types.
func (b *QueryBuilder) WithResourceTypes(resourceTypes ...string) *QueryBuilder {
	b.query.ResourceTypes = append(b.query.ResourceTypes, resourceTypes...)
	return b
}

// WithActions filters by actions.
func (b *QueryBuilder) WithActions(actions ...string) *QueryBuilder {
	b.query.Actions = append(b.query.Actions, actions...)
	return b
}

// WithOutcomes filters by outcomes.
func (b *QueryBuilder) WithOutcomes(outcomes ...audit.Outcome) *QueryBuilder {
	b.query.Outcomes = append(b.query.Outcomes, outcomes...)
	return b
}

// Limit sets the maximum number of results.
func (b *QueryBuilder) Limit(n int) *QueryBuilder {
	b.query.Limit = n
	return b
}

// Offset sets the pagination offset.
func (b *QueryBuilder) Offset(n int) *QueryBuilder {
	b.query.Offset = n
	return b
}

// OrderBy sets the sort field.
func (b *QueryBuilder) OrderBy(field string) *QueryBuilder {
	b.query.OrderBy = field
	return b
}

// Descending sets descending sort order.
func (b *QueryBuilder) Descending() *QueryBuilder {
	b.query.Descending = true
	return b
}

// Ascending sets ascending sort order.
func (b *QueryBuilder) Ascending() *QueryBuilder {
	b.query.Descending = false
	return b
}

// Build returns the constructed query.
func (b *QueryBuilder) Build() *audit.AuditQuery {
	return b.query
}

// Execute runs the query against the given storage.
func (b *QueryBuilder) Execute(ctx context.Context, storage audit.Storage) ([]*audit.AuditEvent, error) {
	return storage.Query(ctx, b.query)
}

// QueryExecutor wraps storage for convenient querying.
type QueryExecutor struct {
	storage audit.Storage
}

// NewQueryExecutor creates a new QueryExecutor.
func NewQueryExecutor(storage audit.Storage) *QueryExecutor {
	return &QueryExecutor{storage: storage}
}

// Query executes a query.
func (e *QueryExecutor) Query(ctx context.Context, query *audit.AuditQuery) ([]*audit.AuditEvent, error) {
	return e.storage.Query(ctx, query)
}

// Count returns the count of matching events.
func (e *QueryExecutor) Count(ctx context.Context, query *audit.AuditQuery) (int64, error) {
	return e.storage.Count(ctx, query)
}

// GetByID retrieves a single event by ID.
func (e *QueryExecutor) GetByID(ctx context.Context, id string) (*audit.AuditEvent, error) {
	return e.storage.Get(ctx, id)
}

// GetAuthenticationEvents returns authentication events.
func (e *QueryExecutor) GetAuthenticationEvents(ctx context.Context, since time.Time) ([]*audit.AuditEvent, error) {
	query := NewQueryBuilder().
		Since(since).
		WithTypes(audit.EventTypeAuthentication).
		Descending().
		Build()
	return e.storage.Query(ctx, query)
}

// GetFailedAttempts returns failed events.
func (e *QueryExecutor) GetFailedAttempts(ctx context.Context, since time.Time) ([]*audit.AuditEvent, error) {
	query := NewQueryBuilder().
		Since(since).
		WithOutcomes(audit.OutcomeFailure).
		Descending().
		Build()
	return e.storage.Query(ctx, query)
}

// GetCriticalEvents returns critical severity events.
func (e *QueryExecutor) GetCriticalEvents(ctx context.Context, since time.Time) ([]*audit.AuditEvent, error) {
	query := NewQueryBuilder().
		Since(since).
		WithSeverities(audit.SeverityCritical).
		Descending().
		Build()
	return e.storage.Query(ctx, query)
}

// GetActorActivity returns events for a specific actor.
func (e *QueryExecutor) GetActorActivity(ctx context.Context, actorID string, since time.Time) ([]*audit.AuditEvent, error) {
	query := NewQueryBuilder().
		Since(since).
		WithActors(actorID).
		Descending().
		Build()
	return e.storage.Query(ctx, query)
}

// QueryResult holds query results with pagination info.
type QueryResult struct {
	Events     []*audit.AuditEvent `json:"events"`
	Total      int64               `json:"total"`
	Limit      int                 `json:"limit"`
	Offset     int                 `json:"offset"`
	HasMore    bool                `json:"has_more"`
	NextOffset int                 `json:"next_offset,omitempty"`
}

// QueryWithPagination executes a query with pagination info.
func (e *QueryExecutor) QueryWithPagination(ctx context.Context, query *audit.AuditQuery) (*QueryResult, error) {
	total, err := e.storage.Count(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to count events: %w", err)
	}

	events, err := e.storage.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	result := &QueryResult{
		Events: events,
		Total:  total,
		Limit:  query.Limit,
		Offset: query.Offset,
	}

	if query.Limit > 0 && int64(query.Offset+len(events)) < total {
		result.HasMore = true
		result.NextOffset = query.Offset + len(events)
	}

	return result, nil
}

// Aggregator provides event aggregation capabilities.
type Aggregator struct {
	storage audit.Storage
}

// NewAggregator creates a new Aggregator.
func NewAggregator(storage audit.Storage) *Aggregator {
	return &Aggregator{storage: storage}
}

// CountByType counts events grouped by type.
func (a *Aggregator) CountByType(ctx context.Context, since time.Time) (map[audit.EventType]int64, error) {
	query := NewQueryBuilder().Since(since).Build()
	events, err := a.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	counts := make(map[audit.EventType]int64)
	for _, event := range events {
		counts[event.Type]++
	}
	return counts, nil
}

// CountBySeverity counts events grouped by severity.
func (a *Aggregator) CountBySeverity(ctx context.Context, since time.Time) (map[audit.Severity]int64, error) {
	query := NewQueryBuilder().Since(since).Build()
	events, err := a.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	counts := make(map[audit.Severity]int64)
	for _, event := range events {
		counts[event.Severity]++
	}
	return counts, nil
}

// CountByOutcome counts events grouped by outcome.
func (a *Aggregator) CountByOutcome(ctx context.Context, since time.Time) (map[audit.Outcome]int64, error) {
	query := NewQueryBuilder().Since(since).Build()
	events, err := a.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	counts := make(map[audit.Outcome]int64)
	for _, event := range events {
		counts[event.Outcome]++
	}
	return counts, nil
}

// TopActors returns the most active actors.
func (a *Aggregator) TopActors(ctx context.Context, since time.Time, limit int) ([]ActorCount, error) {
	query := NewQueryBuilder().Since(since).Build()
	events, err := a.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, event := range events {
		if event.Actor != nil {
			counts[event.Actor.ID]++
		}
	}

	// Convert to slice and sort
	result := make([]ActorCount, 0, len(counts))
	for id, count := range counts {
		result = append(result, ActorCount{ActorID: id, Count: count})
	}

	// Simple bubble sort for top N
	for i := 0; i < len(result)-1; i++ {
		for j := 0; j < len(result)-i-1; j++ {
			if result[j].Count < result[j+1].Count {
				result[j], result[j+1] = result[j+1], result[j]
			}
		}
	}

	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	return result, nil
}

// ActorCount holds actor activity count.
type ActorCount struct {
	ActorID string `json:"actor_id"`
	Count   int64  `json:"count"`
}

// TimeSeriesPoint represents a point in a time series.
type TimeSeriesPoint struct {
	Time  time.Time `json:"time"`
	Count int64     `json:"count"`
}

// EventTimeSeries returns event counts over time.
func (a *Aggregator) EventTimeSeries(ctx context.Context, since, until time.Time, interval time.Duration) ([]TimeSeriesPoint, error) {
	query := NewQueryBuilder().Since(since).Until(until).Build()
	events, err := a.storage.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	// Create buckets
	buckets := make(map[time.Time]int64)
	current := since.Truncate(interval)
	for current.Before(until) {
		buckets[current] = 0
		current = current.Add(interval)
	}

	// Count events into buckets
	for _, event := range events {
		bucket := event.Timestamp.Truncate(interval)
		buckets[bucket]++
	}

	// Convert to sorted slice
	result := make([]TimeSeriesPoint, 0, len(buckets))
	current = since.Truncate(interval)
	for current.Before(until) {
		result = append(result, TimeSeriesPoint{
			Time:  current,
			Count: buckets[current],
		})
		current = current.Add(interval)
	}

	return result, nil
}
