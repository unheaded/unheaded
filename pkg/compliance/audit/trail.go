// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// TrailEntry represents a single entry in the audit trail.
type TrailEntry struct {
	Sequence      int64     `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	PreviousHash  string    `json:"previous_hash"`
	Hash          string    `json:"hash"`
	Event         *AuditEvent `json:"event"`
}

// AuditTrail provides an immutable, verifiable audit trail.
type AuditTrail struct {
	mu       sync.RWMutex
	entries  []*TrailEntry
	lastHash string
	sequence int64
}

// NewAuditTrail creates a new audit trail.
func NewAuditTrail() *AuditTrail {
	return &AuditTrail{
		entries:  make([]*TrailEntry, 0),
		lastHash: "genesis",
	}
}

// Append adds a new event to the audit trail.
func (t *AuditTrail) Append(ctx context.Context, event *AuditEvent) (*TrailEntry, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sequence++

	entry := &TrailEntry{
		Sequence:     t.sequence,
		Timestamp:    time.Now(),
		PreviousHash: t.lastHash,
		Event:        event,
	}

	// Compute hash of the entry
	entry.Hash = t.computeHash(entry)
	t.lastHash = entry.Hash

	t.entries = append(t.entries, entry)

	return entry, nil
}

// Get retrieves an entry by sequence number.
func (t *AuditTrail) Get(sequence int64) (*TrailEntry, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, entry := range t.entries {
		if entry.Sequence == sequence {
			return entry, nil
		}
	}

	return nil, fmt.Errorf("entry not found: %d", sequence)
}

// GetRange retrieves entries within a sequence range.
func (t *AuditTrail) GetRange(start, end int64) ([]*TrailEntry, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var results []*TrailEntry
	for _, entry := range t.entries {
		if entry.Sequence >= start && entry.Sequence <= end {
			results = append(results, entry)
		}
	}

	return results, nil
}

// GetByTimeRange retrieves entries within a time range.
func (t *AuditTrail) GetByTimeRange(start, end time.Time) ([]*TrailEntry, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var results []*TrailEntry
	for _, entry := range t.entries {
		if entry.Timestamp.After(start) && entry.Timestamp.Before(end) {
			results = append(results, entry)
		}
	}

	return results, nil
}

// Verify verifies the integrity of the audit trail.
func (t *AuditTrail) Verify() (*VerificationResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := &VerificationResult{
		TotalEntries:    len(t.entries),
		VerifiedAt:      time.Now(),
		ValidEntries:    0,
		InvalidEntries:  make([]InvalidEntry, 0),
	}

	previousHash := "genesis"

	for _, entry := range t.entries {
		// Verify previous hash link
		if entry.PreviousHash != previousHash {
			result.InvalidEntries = append(result.InvalidEntries, InvalidEntry{
				Sequence: entry.Sequence,
				Reason:   "previous_hash_mismatch",
				Expected: previousHash,
				Actual:   entry.PreviousHash,
			})
			continue
		}

		// Verify entry hash
		computedHash := t.computeHash(entry)
		if entry.Hash != computedHash {
			result.InvalidEntries = append(result.InvalidEntries, InvalidEntry{
				Sequence: entry.Sequence,
				Reason:   "hash_mismatch",
				Expected: computedHash,
				Actual:   entry.Hash,
			})
			continue
		}

		result.ValidEntries++
		previousHash = entry.Hash
	}

	result.IsValid = len(result.InvalidEntries) == 0

	return result, nil
}

// Export exports the audit trail to JSON.
func (t *AuditTrail) Export(ctx context.Context) ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return json.MarshalIndent(t.entries, "", "  ")
}

// LastSequence returns the last sequence number.
func (t *AuditTrail) LastSequence() int64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.sequence
}

// Count returns the number of entries in the trail.
func (t *AuditTrail) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

func (t *AuditTrail) computeHash(entry *TrailEntry) string {
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		entry.Sequence,
		entry.Timestamp.Format(time.RFC3339Nano),
		entry.PreviousHash,
		entry.Event.ID,
		entry.Event.Type,
		entry.Event.Action,
	)

	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// VerificationResult contains the result of trail verification.
type VerificationResult struct {
	IsValid        bool           `json:"is_valid"`
	TotalEntries   int            `json:"total_entries"`
	ValidEntries   int            `json:"valid_entries"`
	InvalidEntries []InvalidEntry `json:"invalid_entries,omitempty"`
	VerifiedAt     time.Time      `json:"verified_at"`
}

// InvalidEntry represents an invalid entry in the trail.
type InvalidEntry struct {
	Sequence int64  `json:"sequence"`
	Reason   string `json:"reason"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// TrailArchiver handles archiving of audit trails.
type TrailArchiver struct {
	trail        *AuditTrail
	archiveStore ArchiveStore
	retention    time.Duration
}

// ArchiveStore defines the interface for storing archived trails.
type ArchiveStore interface {
	Store(ctx context.Context, entries []*TrailEntry, metadata ArchiveMetadata) (string, error)
	Retrieve(ctx context.Context, archiveID string) ([]*TrailEntry, error)
	List(ctx context.Context) ([]ArchiveMetadata, error)
}

// ArchiveMetadata contains metadata about an archived trail segment.
type ArchiveMetadata struct {
	ArchiveID     string    `json:"archive_id"`
	StartSequence int64     `json:"start_sequence"`
	EndSequence   int64     `json:"end_sequence"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	EntryCount    int       `json:"entry_count"`
	Hash          string    `json:"hash"`
	ArchivedAt    time.Time `json:"archived_at"`
}

// NewTrailArchiver creates a new trail archiver.
func NewTrailArchiver(trail *AuditTrail, store ArchiveStore, retention time.Duration) *TrailArchiver {
	return &TrailArchiver{
		trail:        trail,
		archiveStore: store,
		retention:    retention,
	}
}

// InMemoryArchiveStore implements ArchiveStore with in-memory storage.
type InMemoryArchiveStore struct {
	mu       sync.RWMutex
	archives map[string][]*TrailEntry
	metadata map[string]ArchiveMetadata
}

// NewInMemoryArchiveStore creates a new in-memory archive store.
func NewInMemoryArchiveStore() *InMemoryArchiveStore {
	return &InMemoryArchiveStore{
		archives: make(map[string][]*TrailEntry),
		metadata: make(map[string]ArchiveMetadata),
	}
}

func (s *InMemoryArchiveStore) Store(ctx context.Context, entries []*TrailEntry, metadata ArchiveMetadata) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	archiveID := fmt.Sprintf("ARCH-%d", time.Now().UnixNano())
	metadata.ArchiveID = archiveID
	metadata.ArchivedAt = time.Now()

	s.archives[archiveID] = entries
	s.metadata[archiveID] = metadata

	return archiveID, nil
}

func (s *InMemoryArchiveStore) Retrieve(ctx context.Context, archiveID string) ([]*TrailEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.archives[archiveID]
	if !ok {
		return nil, fmt.Errorf("archive not found: %s", archiveID)
	}

	return entries, nil
}

func (s *InMemoryArchiveStore) List(ctx context.Context) ([]ArchiveMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ArchiveMetadata, 0, len(s.metadata))
	for _, m := range s.metadata {
		result = append(result, m)
	}

	return result, nil
}

// TrailQueryBuilder provides a fluent interface for querying the audit trail.
type TrailQueryBuilder struct {
	trail     *AuditTrail
	startSeq  *int64
	endSeq    *int64
	startTime *time.Time
	endTime   *time.Time
	eventTypes []EventType
	actors    []string
	limit     int
}

// NewTrailQuery creates a new trail query builder.
func NewTrailQuery(trail *AuditTrail) *TrailQueryBuilder {
	return &TrailQueryBuilder{
		trail: trail,
	}
}

// FromSequence sets the starting sequence.
func (q *TrailQueryBuilder) FromSequence(seq int64) *TrailQueryBuilder {
	q.startSeq = &seq
	return q
}

// ToSequence sets the ending sequence.
func (q *TrailQueryBuilder) ToSequence(seq int64) *TrailQueryBuilder {
	q.endSeq = &seq
	return q
}

// FromTime sets the starting time.
func (q *TrailQueryBuilder) FromTime(t time.Time) *TrailQueryBuilder {
	q.startTime = &t
	return q
}

// ToTime sets the ending time.
func (q *TrailQueryBuilder) ToTime(t time.Time) *TrailQueryBuilder {
	q.endTime = &t
	return q
}

// WithEventTypes filters by event types.
func (q *TrailQueryBuilder) WithEventTypes(types ...EventType) *TrailQueryBuilder {
	q.eventTypes = types
	return q
}

// WithActors filters by actors.
func (q *TrailQueryBuilder) WithActors(actors ...string) *TrailQueryBuilder {
	q.actors = actors
	return q
}

// WithLimit sets the maximum number of results.
func (q *TrailQueryBuilder) WithLimit(limit int) *TrailQueryBuilder {
	q.limit = limit
	return q
}

// Execute runs the query and returns matching entries.
func (q *TrailQueryBuilder) Execute() ([]*TrailEntry, error) {
	q.trail.mu.RLock()
	defer q.trail.mu.RUnlock()

	var results []*TrailEntry

	for _, entry := range q.trail.entries {
		if q.matches(entry) {
			results = append(results, entry)
			if q.limit > 0 && len(results) >= q.limit {
				break
			}
		}
	}

	return results, nil
}

func (q *TrailQueryBuilder) matches(entry *TrailEntry) bool {
	if q.startSeq != nil && entry.Sequence < *q.startSeq {
		return false
	}
	if q.endSeq != nil && entry.Sequence > *q.endSeq {
		return false
	}
	if q.startTime != nil && entry.Timestamp.Before(*q.startTime) {
		return false
	}
	if q.endTime != nil && entry.Timestamp.After(*q.endTime) {
		return false
	}

	if len(q.eventTypes) > 0 {
		found := false
		for _, t := range q.eventTypes {
			if entry.Event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if len(q.actors) > 0 {
		found := false
		for _, a := range q.actors {
			if entry.Event.Actor == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
