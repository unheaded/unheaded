// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"unheaded/pkg/storage/wal"
)

// WALStore implements MessageStore backed by a Write-Ahead Log with optional
// async flush to a PGStore for durability. The WAL provides fast writes (<1ms),
// while PG provides persistence across restarts.
type WALStore struct {
	wal      *wal.WAL
	pg       *PGStore // optional — if nil, WAL-only mode
	capacity int
	closed   bool
	mu       sync.RWMutex

	// In-memory index: msgID → WAL sequence
	index map[uuid.UUID]uint64
	// Hot cache: recent messages for fast reads
	hot     []*Message
	hotSize int

	// Async flush
	flushCh   chan struct{}
	flushDone chan struct{}
}

// NewWALStore creates a WAL-backed message store.
// If pgStore is non-nil, messages are asynchronously flushed to PostgreSQL.
func NewWALStore(walDir string, capacity int, pgStore *PGStore) (*WALStore, error) {
	cfg := wal.DefaultConfig()
	cfg.Path = walDir

	w, err := wal.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("wal_store: create wal: %w", err)
	}

	s := &WALStore{
		wal:       w,
		pg:        pgStore,
		capacity:  capacity,
		index:     make(map[uuid.UUID]uint64),
		hot:       make([]*Message, 0, capacity),
		hotSize:   capacity,
		flushCh:   make(chan struct{}, 1),
		flushDone: make(chan struct{}),
	}

	// Recover state from WAL
	if err := s.recover(); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("wal_store: recovery: %w", err)
	}

	// Start async flusher if PG is available
	if pgStore != nil {
		go s.flusher()
	}

	return s, nil
}

// recover replays WAL entries to rebuild the in-memory index and hot cache.
func (s *WALStore) recover() error {
	ctx := context.Background()
	count := 0
	return s.wal.Scan(ctx, 0, func(seq uint64, data []byte) error {
		msg, err := DecodeMessage(data)
		if err != nil {
			return nil // Skip corrupt entries
		}
		s.index[msg.ID] = seq
		s.addToHot(msg)
		count++
		return nil
	})
}

// addToHot adds a message to the hot cache, evicting oldest if at capacity.
func (s *WALStore) addToHot(msg *Message) {
	if len(s.hot) >= s.hotSize {
		// Evict oldest
		s.hot = s.hot[1:]
	}
	s.hot = append(s.hot, msg)
}

// Push writes a message to the WAL and signals the async flusher.
func (s *WALStore) Push(ctx context.Context, msg *Message) (uuid.UUID, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return uuid.Nil, false, ErrStoreClosed
	}
	if err := msg.Validate(); err != nil {
		return uuid.Nil, false, err
	}

	data, err := EncodeMessage(msg)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("wal_store: encode: %w", err)
	}

	seq, err := s.wal.Write(ctx, data)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("wal_store: write: %w", err)
	}

	s.index[msg.ID] = seq
	s.addToHot(msg)

	// Signal flusher (non-blocking)
	select {
	case s.flushCh <- struct{}{}:
	default:
	}

	return msg.ID, false, nil
}

// Get retrieves a message by ID from hot cache or PG fallback.
func (s *WALStore) Get(ctx context.Context, id uuid.UUID) (*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	// Check hot cache first
	for _, m := range s.hot {
		if m.ID == id {
			return m, nil
		}
	}

	// Fall back to PG if available
	if s.pg != nil {
		return s.pg.Get(ctx, id)
	}

	// Try WAL scan (slow, last resort)
	seq, ok := s.index[id]
	if !ok {
		return nil, ErrNotFound
	}
	data, err := s.wal.Read(ctx, seq)
	if err != nil {
		return nil, fmt.Errorf("wal_store: read: %w", err)
	}
	return DecodeMessage(data)
}

// Delete removes a message.
func (s *WALStore) Delete(ctx context.Context, id uuid.UUID, creatorID uuid.UUID, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	// Remove from hot cache
	for i, m := range s.hot {
		if m.ID == id && m.CreatorID == creatorID && m.Content == content {
			s.hot = append(s.hot[:i], s.hot[i+1:]...)
			delete(s.index, id)

			// Also delete from PG
			if s.pg != nil {
				_ = s.pg.Delete(ctx, id, creatorID, content)
			}
			return nil
		}
	}
	return ErrNotFound
}

// Count returns the number of messages in the index.
func (s *WALStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return 0, ErrStoreClosed
	}
	return len(s.index), nil
}

// Capacity returns the configured capacity.
func (s *WALStore) Capacity() int {
	return s.capacity
}

// Close flushes remaining WAL entries to PG and closes both.
func (s *WALStore) Close() error {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	// Stop flusher
	close(s.flushCh)
	if s.pg != nil {
		<-s.flushDone // Wait for final flush
	}

	return s.wal.Close()
}

// flusher runs in background, batching WAL entries to PG.
func (s *WALStore) flusher() {
	defer close(s.flushDone)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case _, ok := <-s.flushCh:
			if !ok {
				// Channel closed — final flush
				s.flushToPG()
				return
			}
			s.flushToPG()
		case <-ticker.C:
			s.flushToPG()
		}
	}
}

// flushToPG writes unflushed messages from hot cache to PG.
func (s *WALStore) flushToPG() {
	if s.pg == nil {
		return
	}

	s.mu.RLock()
	messages := make([]*Message, len(s.hot))
	copy(messages, s.hot)
	s.mu.RUnlock()

	ctx := context.Background()
	for _, msg := range messages {
		_, _, _ = s.pg.Push(ctx, msg) // Idempotent (ON CONFLICT DO NOTHING)
	}
}
