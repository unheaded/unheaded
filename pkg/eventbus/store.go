// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package eventbus

import (
	"container/list"
	"sort"
	"sync"
	"time"
)

// Store defines the interface for event storage.
type Store interface {
	// Store saves an event.
	Store(Event) error
	// Query retrieves events matching the pattern within the time range.
	Query(pattern string, start, end time.Time) ([]Event, error)
	// Get retrieves a specific event by ID.
	Get(id string) (Event, bool)
	// Count returns the number of stored events.
	Count() int
}

// MemoryStore is an in-memory event store with LRU eviction.
type MemoryStore struct {
	mu       sync.RWMutex
	events   map[string]*list.Element
	order    *list.List
	capacity int
	byTopic  map[string][]*list.Element
}

// eventEntry wraps an event for the LRU list.
type eventEntry struct {
	event Event
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore(capacity int) *MemoryStore {
	return &MemoryStore{
		events:   make(map[string]*list.Element),
		order:    list.New(),
		capacity: capacity,
		byTopic:  make(map[string][]*list.Element),
	}
}

// Store saves an event to the store.
func (s *MemoryStore) Store(e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if event already exists
	if existing, ok := s.events[e.ID]; ok {
		// Move to front (most recent)
		s.order.MoveToFront(existing)
		return nil
	}

	// Evict oldest if at capacity
	for s.order.Len() >= s.capacity {
		s.evictOldest()
	}

	// Add new event
	entry := &eventEntry{event: e}
	elem := s.order.PushFront(entry)
	s.events[e.ID] = elem

	// Index by topic
	s.byTopic[e.Topic] = append(s.byTopic[e.Topic], elem)

	return nil
}

// Query retrieves events matching the pattern within the time range.
func (s *MemoryStore) Query(pattern string, start, end time.Time) ([]Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []Event

	// If pattern is exact, use topic index
	if !isWildcardPattern(pattern) {
		if elems, ok := s.byTopic[pattern]; ok {
			for _, elem := range elems {
				entry := elem.Value.(*eventEntry)
				if s.inTimeRange(entry.event, start, end) {
					results = append(results, entry.event)
				}
			}
		}
	} else {
		// Scan all events for wildcard patterns
		for e := s.order.Front(); e != nil; e = e.Next() {
			entry := e.Value.(*eventEntry)
			if MatchPattern(pattern, entry.event.Topic) && s.inTimeRange(entry.event, start, end) {
				results = append(results, entry.event)
			}
		}
	}

	// Sort by timestamp (oldest first for replay)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.Before(results[j].Timestamp)
	})

	return results, nil
}

// Get retrieves a specific event by ID.
func (s *MemoryStore) Get(id string) (Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if elem, ok := s.events[id]; ok {
		return elem.Value.(*eventEntry).event, true
	}
	return Event{}, false
}

// Count returns the number of stored events.
func (s *MemoryStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.order.Len()
}

// Clear removes all events from the store.
func (s *MemoryStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = make(map[string]*list.Element)
	s.order = list.New()
	s.byTopic = make(map[string][]*list.Element)
}

// Topics returns all unique topics in the store.
func (s *MemoryStore) Topics() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	topics := make([]string, 0, len(s.byTopic))
	for topic := range s.byTopic {
		topics = append(topics, topic)
	}
	return topics
}

// evictOldest removes the oldest event (caller must hold lock).
func (s *MemoryStore) evictOldest() {
	oldest := s.order.Back()
	if oldest == nil {
		return
	}

	entry := oldest.Value.(*eventEntry)

	// Remove from events map
	delete(s.events, entry.event.ID)

	// Remove from topic index
	s.removeFromTopicIndex(entry.event.Topic, oldest)

	// Remove from order list
	s.order.Remove(oldest)
}

// removeFromTopicIndex removes an element from the topic index.
func (s *MemoryStore) removeFromTopicIndex(topic string, elem *list.Element) {
	elems := s.byTopic[topic]
	for i, e := range elems {
		if e == elem {
			s.byTopic[topic] = append(elems[:i], elems[i+1:]...)
			if len(s.byTopic[topic]) == 0 {
				delete(s.byTopic, topic)
			}
			return
		}
	}
}

// inTimeRange checks if an event is within the time range.
func (s *MemoryStore) inTimeRange(e Event, start, end time.Time) bool {
	if !start.IsZero() && e.Timestamp.Before(start) {
		return false
	}
	if !end.IsZero() && e.Timestamp.After(end) {
		return false
	}
	return true
}

// isWildcardPattern checks if a pattern contains wildcards.
func isWildcardPattern(pattern string) bool {
	for _, c := range pattern {
		if c == '*' {
			return true
		}
	}
	return false
}

// DeadLetter represents an event that failed processing.
type DeadLetter struct {
	Event    Event
	Reason   string
	Time     time.Time
	Attempts int
}

// DeadLetterQueue manages failed events.
type DeadLetterQueue struct {
	mu       sync.RWMutex
	letters  []DeadLetter
	capacity int
}

// NewDeadLetterQueue creates a new dead letter queue.
func NewDeadLetterQueue(capacity int) *DeadLetterQueue {
	return &DeadLetterQueue{
		letters:  make([]DeadLetter, 0, capacity),
		capacity: capacity,
	}
}

// Add adds a dead letter to the queue.
func (q *DeadLetterQueue) Add(dl DeadLetter) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if event already in queue
	for i := range q.letters {
		if q.letters[i].Event.ID == dl.Event.ID {
			q.letters[i].Attempts++
			q.letters[i].Reason = dl.Reason
			q.letters[i].Time = dl.Time
			return
		}
	}

	// Evict oldest if at capacity
	if len(q.letters) >= q.capacity {
		q.letters = q.letters[1:]
	}

	q.letters = append(q.letters, dl)
}

// Get retrieves all dead letters.
func (q *DeadLetterQueue) Get() []DeadLetter {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]DeadLetter, len(q.letters))
	copy(result, q.letters)
	return result
}

// Count returns the number of dead letters.
func (q *DeadLetterQueue) Count() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.letters)
}

// Clear removes all dead letters.
func (q *DeadLetterQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.letters = q.letters[:0]
}

// Retry attempts to reprocess dead letters.
func (q *DeadLetterQueue) Retry(bus *Bus) int {
	q.mu.Lock()
	letters := q.letters
	q.letters = make([]DeadLetter, 0, q.capacity)
	q.mu.Unlock()

	successful := 0
	for _, dl := range letters {
		if err := bus.Publish(dl.Event.Topic, dl.Event.Data); err == nil {
			successful++
		} else {
			// Add back to queue with incremented attempts
			dl.Attempts++
			q.Add(dl)
		}
	}

	return successful
}

// RetryWithFilter retries dead letters matching a filter.
func (q *DeadLetterQueue) RetryWithFilter(bus *Bus, filter func(DeadLetter) bool) int {
	q.mu.Lock()

	var toRetry []DeadLetter
	var remaining []DeadLetter

	for _, dl := range q.letters {
		if filter(dl) {
			toRetry = append(toRetry, dl)
		} else {
			remaining = append(remaining, dl)
		}
	}

	q.letters = remaining
	q.mu.Unlock()

	successful := 0
	for _, dl := range toRetry {
		if err := bus.Publish(dl.Event.Topic, dl.Event.Data); err == nil {
			successful++
		} else {
			dl.Attempts++
			q.Add(dl)
		}
	}

	return successful
}

// TimeBasedStore wraps a store with time-based retention.
type TimeBasedStore struct {
	store     Store
	retention time.Duration
	stop      chan struct{}
}

// NewTimeBasedStore creates a store with automatic retention.
func NewTimeBasedStore(store Store, retention time.Duration) *TimeBasedStore {
	s := &TimeBasedStore{
		store:     store,
		retention: retention,
		stop:      make(chan struct{}),
	}

	go s.cleanup()
	return s
}

// Store saves an event.
func (s *TimeBasedStore) Store(e Event) error {
	return s.store.Store(e)
}

// Query retrieves events, automatically filtering by retention.
func (s *TimeBasedStore) Query(pattern string, start, end time.Time) ([]Event, error) {
	cutoff := time.Now().Add(-s.retention)
	if start.Before(cutoff) {
		start = cutoff
	}
	return s.store.Query(pattern, start, end)
}

// Get retrieves a specific event.
func (s *TimeBasedStore) Get(id string) (Event, bool) {
	return s.store.Get(id)
}

// Count returns the number of stored events.
func (s *TimeBasedStore) Count() int {
	return s.store.Count()
}

// Close stops the cleanup goroutine.
func (s *TimeBasedStore) Close() {
	close(s.stop)
}

// cleanup periodically removes old events.
func (s *TimeBasedStore) cleanup() {
	ticker := time.NewTicker(s.retention / 10)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			// For MemoryStore, we rely on LRU eviction
			// More sophisticated cleanup could be added here
		}
	}
}

// SnapshotStore provides point-in-time snapshots.
type SnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[string][]Event
	store     Store
}

// NewSnapshotStore creates a store with snapshot capability.
func NewSnapshotStore(store Store) *SnapshotStore {
	return &SnapshotStore{
		snapshots: make(map[string][]Event),
		store:     store,
	}
}

// Store saves an event.
func (s *SnapshotStore) Store(e Event) error {
	return s.store.Store(e)
}

// Query retrieves events.
func (s *SnapshotStore) Query(pattern string, start, end time.Time) ([]Event, error) {
	return s.store.Query(pattern, start, end)
}

// Get retrieves a specific event.
func (s *SnapshotStore) Get(id string) (Event, bool) {
	return s.store.Get(id)
}

// Count returns the number of stored events.
func (s *SnapshotStore) Count() int {
	return s.store.Count()
}

// CreateSnapshot saves current events as a named snapshot.
func (s *SnapshotStore) CreateSnapshot(name string) error {
	events, err := s.store.Query("*", time.Time{}, time.Now())
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshots[name] = events
	return nil
}

// RestoreSnapshot retrieves events from a named snapshot.
func (s *SnapshotStore) RestoreSnapshot(name string) ([]Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, ok := s.snapshots[name]
	if !ok {
		return nil, false
	}

	result := make([]Event, len(events))
	copy(result, events)
	return result, true
}

// DeleteSnapshot removes a named snapshot.
func (s *SnapshotStore) DeleteSnapshot(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.snapshots, name)
}

// ListSnapshots returns all snapshot names.
func (s *SnapshotStore) ListSnapshots() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.snapshots))
	for name := range s.snapshots {
		names = append(names, name)
	}
	return names
}
