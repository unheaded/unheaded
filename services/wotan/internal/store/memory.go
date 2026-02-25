// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory implementation of MessageStore using a ring buffer.
// When the buffer is full, oldest messages are overwritten.
// This is the default store for development and testing.
type MemoryStore struct {
	mu       sync.RWMutex
	buffer   []*Message
	size     int
	head     int  // next write position
	count    int  // current number of messages
	wrapping bool // has the buffer wrapped around?
	closed   bool
}

// Compile-time interface checks
var (
	_ MessageStore = (*MemoryStore)(nil)
	_ Queryable    = (*MemoryStore)(nil)
	_ Statable     = (*MemoryStore)(nil)
)

// NewMemoryStore creates a new in-memory ring buffer store with the specified capacity.
// If size is <= 0, a default size of 10000 is used.
func NewMemoryStore(size int) *MemoryStore {
	if size <= 0 {
		size = 10000 // default size
	}
	return &MemoryStore{
		buffer: make([]*Message, size),
		size:   size,
		head:   0,
		count:  0,
	}
}

// Push adds a message to the store.
// Returns the message ID and whether an old message was overwritten.
func (s *MemoryStore) Push(ctx context.Context, msg *Message) (uuid.UUID, bool, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, false, err
	}

	if err := msg.Validate(); err != nil {
		return uuid.Nil, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return uuid.Nil, false, ErrStoreClosed
	}

	overwritten := false
	if s.count == s.size {
		overwritten = true
		s.wrapping = true
	}

	// Deep copy the message to prevent external mutation
	msgCopy := &Message{
		ID:        msg.ID,
		CreatorID: msg.CreatorID,
		RoomID:    msg.RoomID,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
	}

	s.buffer[s.head] = msgCopy
	s.head = (s.head + 1) % s.size

	if s.count < s.size {
		s.count++
	}

	return msg.ID, overwritten, nil
}

// Get retrieves a specific message by ID.
func (s *MemoryStore) Get(ctx context.Context, id uuid.UUID) (*Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if id == uuid.Nil {
		return nil, ErrNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	if s.count == 0 {
		return nil, ErrNotFound
	}

	msg := s.getByIDLocked(id)
	if msg == nil {
		return nil, ErrNotFound
	}

	// Return a copy to prevent external mutation
	return s.copyMessage(msg), nil
}

// getByIDLocked finds a message by ID. Caller must hold the read lock.
func (s *MemoryStore) getByIDLocked(id uuid.UUID) *Message {
	if !s.wrapping {
		for i := 0; i < s.count; i++ {
			if s.buffer[i] != nil && s.buffer[i].ID == id {
				return s.buffer[i]
			}
		}
	} else {
		oldestIdx := s.head
		for i := 0; i < s.count; i++ {
			msg := s.buffer[(oldestIdx+i)%s.size]
			if msg != nil && msg.ID == id {
				return msg
			}
		}
	}
	return nil
}

// Delete removes a message by ID with triple verification.
func (s *MemoryStore) Delete(ctx context.Context, id uuid.UUID, creatorID uuid.UUID, content string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if id == uuid.Nil || creatorID == uuid.Nil || content == "" {
		return ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if s.count == 0 {
		return ErrNotFound
	}

	// Triple verification: ID + CreatorID + Content
	if !s.wrapping {
		for i := 0; i < s.count; i++ {
			msg := s.buffer[i]
			if msg != nil && msg.ID == id && msg.CreatorID == creatorID && msg.Content == content {
				// Shift messages to fill the gap
				copy(s.buffer[i:], s.buffer[i+1:s.count])
				s.buffer[s.count-1] = nil
				s.count--
				s.head = s.count
				return nil
			}
		}
	} else {
		oldestIdx := s.head
		for i := 0; i < s.count; i++ {
			actualIdx := (oldestIdx + i) % s.size
			msg := s.buffer[actualIdx]
			if msg != nil && msg.ID == id && msg.CreatorID == creatorID && msg.Content == content {
				// Remove by shifting in circular buffer
				for j := i; j < s.count-1; j++ {
					fromIdx := (oldestIdx + j + 1) % s.size
					toIdx := (oldestIdx + j) % s.size
					s.buffer[toIdx] = s.buffer[fromIdx]
				}
				lastIdx := (oldestIdx + s.count - 1) % s.size
				s.buffer[lastIdx] = nil
				s.count--
				s.head = (s.head - 1 + s.size) % s.size
				if s.count < s.size {
					s.wrapping = false
				}
				return nil
			}
		}
	}

	return ErrNotFound
}

// Count returns the current number of messages in the store.
func (s *MemoryStore) Count(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, ErrStoreClosed
	}

	return s.count, nil
}

// Capacity returns the maximum number of messages the store can hold.
func (s *MemoryStore) Capacity() int {
	return s.size
}

// Close releases resources held by the store.
func (s *MemoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	s.closed = true
	// Clear the buffer to allow GC
	for i := range s.buffer {
		s.buffer[i] = nil
	}
	s.buffer = nil
	s.count = 0

	return nil
}

// Query retrieves messages matching the specified options.
func (s *MemoryStore) Query(ctx context.Context, opts QueryOptions) (MessageIterator, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Collect all matching messages into a slice
	var messages []*Message

	if !s.wrapping {
		for i := 0; i < s.count; i++ {
			if s.matchesOpts(s.buffer[i], opts) {
				messages = append(messages, s.copyMessage(s.buffer[i]))
			}
		}
	} else {
		oldestIdx := s.head
		for i := 0; i < s.count; i++ {
			msg := s.buffer[(oldestIdx+i)%s.size]
			if s.matchesOpts(msg, opts) {
				messages = append(messages, s.copyMessage(msg))
			}
		}
	}

	// Apply ascending/descending order
	if !opts.Ascending {
		// Reverse for descending order (newest first)
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	// Apply offset and limit
	if opts.Offset > 0 {
		if opts.Offset >= len(messages) {
			messages = nil
		} else {
			messages = messages[opts.Offset:]
		}
	}

	if opts.Limit > 0 && opts.Limit < len(messages) {
		messages = messages[:opts.Limit]
	}

	return &sliceIterator{
		messages: messages,
		index:    -1,
	}, nil
}

// matchesOpts checks if a message matches the query options.
func (s *MemoryStore) matchesOpts(msg *Message, opts QueryOptions) bool {
	if msg == nil {
		return false
	}

	if opts.RoomID != "" && msg.RoomID != opts.RoomID {
		return false
	}

	if !opts.Since.IsZero() && !msg.Timestamp.After(opts.Since) {
		return false
	}

	if !opts.Until.IsZero() && !msg.Timestamp.Before(opts.Until) {
		return false
	}

	return true
}

// Stats returns current statistics about the store.
func (s *MemoryStore) Stats(ctx context.Context) (StoreStats, error) {
	if err := ctx.Err(); err != nil {
		return StoreStats{}, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return StoreStats{}, ErrStoreClosed
	}

	stats := StoreStats{
		Count:      s.count,
		Capacity:   s.size,
		IsWrapping: s.wrapping,
	}

	if s.count > 0 {
		// Find oldest and newest timestamps
		if !s.wrapping {
			stats.OldestTimestamp = s.buffer[0].Timestamp
			stats.NewestTimestamp = s.buffer[s.count-1].Timestamp
		} else {
			oldestIdx := s.head
			newestIdx := (s.head - 1 + s.size) % s.size
			stats.OldestTimestamp = s.buffer[oldestIdx].Timestamp
			stats.NewestTimestamp = s.buffer[newestIdx].Timestamp
		}
	}

	// Estimate bytes used (rough approximation)
	// Message overhead: ~100 bytes per message average
	stats.BytesUsed = int64(s.count * 100)

	return stats, nil
}

// copyMessage creates a deep copy of a message.
func (s *MemoryStore) copyMessage(msg *Message) *Message {
	if msg == nil {
		return nil
	}
	return &Message{
		ID:        msg.ID,
		CreatorID: msg.CreatorID,
		RoomID:    msg.RoomID,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
	}
}

// sliceIterator provides iteration over a slice of messages.
type sliceIterator struct {
	messages []*Message
	index    int
	closed   bool
	err      error
}

func (it *sliceIterator) Next(ctx context.Context) bool {
	if it.closed {
		return false
	}

	if err := ctx.Err(); err != nil {
		it.err = err
		return false
	}

	it.index++
	return it.index < len(it.messages)
}

func (it *sliceIterator) Message() *Message {
	if it.index < 0 || it.index >= len(it.messages) {
		return nil
	}
	return it.messages[it.index]
}

func (it *sliceIterator) Err() error {
	return it.err
}

func (it *sliceIterator) Close() error {
	if it.closed {
		return ErrIteratorClosed
	}
	it.closed = true
	it.messages = nil
	return nil
}
