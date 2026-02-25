// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package store provides storage abstraction interfaces for the message bus.
// This allows pluggable backends: in-memory ring buffer, WAL-backed, persistent, etc.
package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by store implementations.
var (
	// ErrNotFound indicates the requested message does not exist.
	ErrNotFound = errors.New("store: message not found")

	// ErrStoreEmpty indicates no messages exist in the store.
	ErrStoreEmpty = errors.New("store: no messages")

	// ErrStoreClosed indicates the store has been closed.
	ErrStoreClosed = errors.New("store: store is closed")

	// ErrIteratorClosed indicates the iterator has been closed.
	ErrIteratorClosed = errors.New("store: iterator is closed")

	// ErrInvalidMessage indicates the message failed validation.
	ErrInvalidMessage = errors.New("store: invalid message")

	// ErrCapacityExceeded indicates the store capacity limit was reached.
	ErrCapacityExceeded = errors.New("store: capacity exceeded")
)

// Message represents a single message in the store.
// This is the canonical message type used across all store implementations.
type Message struct {
	ID        uuid.UUID `json:"id"`
	CreatorID uuid.UUID `json:"creator_id"`
	RoomID    string    `json:"room_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Validate checks if the message has all required fields.
func (m *Message) Validate() error {
	if m == nil {
		return ErrInvalidMessage
	}
	if m.ID == uuid.Nil {
		return ErrInvalidMessage
	}
	if m.CreatorID == uuid.Nil {
		return ErrInvalidMessage
	}
	if m.RoomID == "" {
		return ErrInvalidMessage
	}
	if m.Content == "" {
		return ErrInvalidMessage
	}
	if m.Timestamp.IsZero() {
		return ErrInvalidMessage
	}
	return nil
}

// MessageStore is the primary interface for message storage backends.
// Implementations must be safe for concurrent access.
type MessageStore interface {
	// Push adds a message to the store.
	// Returns the message ID and whether an old message was overwritten (for ring buffers).
	// Returns ErrStoreClosed if the store has been closed.
	// Returns ErrInvalidMessage if the message fails validation.
	Push(ctx context.Context, msg *Message) (uuid.UUID, bool, error)

	// Get retrieves a specific message by ID.
	// Returns ErrNotFound if the message does not exist.
	// Returns ErrStoreClosed if the store has been closed.
	Get(ctx context.Context, id uuid.UUID) (*Message, error)

	// Delete removes a message by ID with triple verification (ID + CreatorID + Content).
	// Returns true if the message was found and deleted.
	// Returns ErrNotFound if the message does not exist or verification fails.
	// Returns ErrStoreClosed if the store has been closed.
	Delete(ctx context.Context, id uuid.UUID, creatorID uuid.UUID, content string) error

	// Count returns the current number of messages in the store.
	Count(ctx context.Context) (int, error)

	// Capacity returns the maximum number of messages the store can hold.
	// Returns -1 for unlimited capacity stores.
	Capacity() int

	// Close releases any resources held by the store.
	// After Close, all operations return ErrStoreClosed.
	Close() error
}

// QueryOptions specifies filters and pagination for message queries.
type QueryOptions struct {
	// RoomID filters messages by room (empty = all rooms).
	RoomID string

	// Since filters messages after this timestamp.
	Since time.Time

	// Until filters messages before this timestamp.
	Until time.Time

	// Limit maximum number of messages to return (0 = no limit).
	Limit int

	// Offset number of messages to skip (for pagination).
	Offset int

	// Ascending returns messages in chronological order if true.
	// If false, returns in reverse chronological order.
	Ascending bool
}

// Queryable extends MessageStore with advanced query capabilities.
// Not all store implementations support this interface.
type Queryable interface {
	MessageStore

	// Query retrieves messages matching the specified options.
	// Returns an iterator over the matching messages.
	Query(ctx context.Context, opts QueryOptions) (MessageIterator, error)
}

// MessageIterator provides iteration over query results.
// Implementations must be safe for concurrent access from a single goroutine.
// Callers must call Close when done to release resources.
type MessageIterator interface {
	// Next advances to the next message.
	// Returns false when iteration is complete or an error occurred.
	Next(ctx context.Context) bool

	// Message returns the current message.
	// Only valid after Next returns true.
	// Returns nil if iteration is complete or not started.
	Message() *Message

	// Err returns any error encountered during iteration.
	// Should be checked after Next returns false.
	Err() error

	// Close releases resources held by the iterator.
	// After Close, all operations return ErrIteratorClosed.
	Close() error
}

// StoreStats provides metrics about store state.
type StoreStats struct {
	// Count is the current number of messages.
	Count int

	// Capacity is the maximum number of messages (-1 for unlimited).
	Capacity int

	// OldestTimestamp is the timestamp of the oldest message.
	OldestTimestamp time.Time

	// NewestTimestamp is the timestamp of the newest message.
	NewestTimestamp time.Time

	// IsWrapping indicates if a ring buffer is overwriting old messages.
	IsWrapping bool

	// BytesUsed is the approximate memory usage (if available).
	BytesUsed int64
}

// Statable extends MessageStore with statistics capabilities.
type Statable interface {
	MessageStore

	// Stats returns current statistics about the store.
	Stats(ctx context.Context) (StoreStats, error)
}

// Compactable extends MessageStore with compaction capabilities.
// Used for stores that accumulate dead space over time.
type Compactable interface {
	MessageStore

	// Compact reclaims unused space and optimizes storage.
	Compact(ctx context.Context) error
}

// Snapshotable extends MessageStore with snapshot capabilities.
// Used for creating consistent backups.
type Snapshotable interface {
	MessageStore

	// Snapshot creates a consistent point-in-time snapshot.
	// Returns a reader for the snapshot data.
	Snapshot(ctx context.Context) (SnapshotReader, error)

	// Restore loads data from a snapshot.
	Restore(ctx context.Context, reader SnapshotReader) error
}

// SnapshotReader provides access to snapshot data.
type SnapshotReader interface {
	// Iterator returns an iterator over snapshot messages.
	Iterator() (MessageIterator, error)

	// Metadata returns snapshot metadata (creation time, count, etc).
	Metadata() SnapshotMetadata

	// Close releases snapshot resources.
	Close() error
}

// SnapshotMetadata describes a snapshot.
type SnapshotMetadata struct {
	// ID is the unique identifier for this snapshot.
	ID string

	// CreatedAt is when the snapshot was taken.
	CreatedAt time.Time

	// MessageCount is the number of messages in the snapshot.
	MessageCount int

	// ByteSize is the size of the snapshot data.
	ByteSize int64
}
