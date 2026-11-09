// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Stevie Bellis. All rights reserved.

package store

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"unheaded/pkg/storage/wal"
)

// WALStore implements MessageStore backed by a Write-Ahead Log on local
// disk. It is Wotan's only persistent store and an opt-in (--store-type
// wal); memory is the default (ADR-005). The WAL is also the replication
// substrate for cluster mode (ADR-035).
type WALStore struct {
	wal      *wal.WAL
	capacity int
	closed   bool
	mu       sync.RWMutex

	// In-memory index: msgID → WAL sequence
	index map[uuid.UUID]uint64
	// Hot cache: recent messages for fast reads
	hot     []*Message
	hotSize int
}

// NewWALStore creates a WAL-backed message store in walDir, replaying any
// existing segments to rebuild the index and hot cache.
func NewWALStore(walDir string, capacity int) (*WALStore, error) {
	cfg := wal.DefaultConfig()
	cfg.Path = walDir

	w, err := wal.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("wal_store: create wal: %w", err)
	}

	s := &WALStore{
		wal:      w,
		capacity: capacity,
		index:    make(map[uuid.UUID]uint64),
		hot:      make([]*Message, 0, capacity),
		hotSize:  capacity,
	}

	// Recover state from WAL
	if err := s.recover(); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("wal_store: recovery: %w", err)
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

// Push writes a message to the WAL and adds it to the hot cache.
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

	return msg.ID, false, nil
}

// Get retrieves a message by ID from the hot cache, else from the WAL.
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

	// Read it back from the WAL (slow, last resort)
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

// Close closes the WAL. It holds the store lock so no Push is mid-write,
// and a second Close returns ErrStoreClosed, as MemoryStore's does.
func (s *WALStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}
	s.closed = true

	return s.wal.Close()
}
