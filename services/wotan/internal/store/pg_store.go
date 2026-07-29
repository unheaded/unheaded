// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// PGStore implements MessageStore backed by PostgreSQL (The Well).
type PGStore struct {
	db       *sql.DB
	capacity int
	closed   bool
	mu       sync.RWMutex
}

// NewPGStore creates a PostgreSQL-backed message store.
// connStr format: "postgres://user:pass@host:port/dbname?sslmode=disable"
func NewPGStore(connStr string, capacity int) (*PGStore, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("pg_store: open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close() // #nosec G104 -- cleanup on an error path; the function is already returning the failure that matters
		return nil, fmt.Errorf("pg_store: ping: %w", err)
	}

	return &PGStore{db: db, capacity: capacity}, nil
}

// Push inserts a message into PostgreSQL.
func (s *PGStore) Push(ctx context.Context, msg *Message) (uuid.UUID, bool, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return uuid.Nil, false, ErrStoreClosed
	}
	s.mu.RUnlock()

	if err := msg.Validate(); err != nil {
		return uuid.Nil, false, err
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wotan_messages (id, room, sender_id, content, seq, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		msg.ID, msg.RoomID, msg.CreatorID, []byte(msg.Content), 0, msg.Timestamp,
	)
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("pg_store: push: %w", err)
	}

	return msg.ID, false, nil
}

// PushWithSeq inserts a message with a specific sequence number.
func (s *PGStore) PushWithSeq(ctx context.Context, msg *Message, seq int64) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStoreClosed
	}
	s.mu.RUnlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wotan_messages (id, room, sender_id, content, seq, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		msg.ID, msg.RoomID, msg.CreatorID, []byte(msg.Content), seq, msg.Timestamp,
	)
	return err
}

// Get retrieves a message by ID.
func (s *PGStore) Get(ctx context.Context, id uuid.UUID) (*Message, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	s.mu.RUnlock()

	var msg Message
	var content []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id, room, sender_id, content, created_at FROM wotan_messages WHERE id = $1`, id,
	).Scan(&msg.ID, &msg.RoomID, &msg.CreatorID, &content, &msg.Timestamp)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pg_store: get: %w", err)
	}
	msg.Content = string(content)
	return &msg, nil
}

// GetSince returns messages in a room after a sequence number.
func (s *PGStore) GetSince(ctx context.Context, roomID string, sinceSeq int64, limit int) ([]*Message, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	s.mu.RUnlock()

	if limit <= 0 {
		limit = 1000
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, room, sender_id, content, created_at FROM wotan_messages
		 WHERE room = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`,
		roomID, sinceSeq, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("pg_store: get_since: %w", err)
	}
	defer rows.Close()

	var msgs []*Message
	for rows.Next() {
		var msg Message
		var content []byte
		if err := rows.Scan(&msg.ID, &msg.RoomID, &msg.CreatorID, &content, &msg.Timestamp); err != nil {
			return nil, fmt.Errorf("pg_store: scan: %w", err)
		}
		msg.Content = string(content)
		msgs = append(msgs, &msg)
	}
	return msgs, rows.Err()
}

// Delete removes a message with triple verification.
func (s *PGStore) Delete(ctx context.Context, id uuid.UUID, creatorID uuid.UUID, content string) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStoreClosed
	}
	s.mu.RUnlock()

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM wotan_messages WHERE id = $1 AND sender_id = $2 AND content = $3`,
		id, creatorID, []byte(content),
	)
	if err != nil {
		return fmt.Errorf("pg_store: delete: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the total number of messages.
func (s *PGStore) Count(ctx context.Context) (int, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return 0, ErrStoreClosed
	}
	s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wotan_messages`).Scan(&count)
	return count, err
}

// Capacity returns the configured capacity.
func (s *PGStore) Capacity() int {
	return s.capacity
}

// Close closes the database connection.
func (s *PGStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.db.Close()
}

// NextSeq atomically increments and returns the next sequence number for a topic.
func (s *PGStore) NextSeq(ctx context.Context, topic string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO wotan_sequences (topic, seq, updated_at) VALUES ($1, 1, NOW())
		 ON CONFLICT (topic) DO UPDATE SET seq = wotan_sequences.seq + 1, updated_at = NOW()
		 RETURNING seq`,
		topic,
	).Scan(&seq)
	return seq, err
}

// LastSeq returns the current sequence number for a topic.
func (s *PGStore) LastSeq(ctx context.Context, topic string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx,
		`SELECT seq FROM wotan_sequences WHERE topic = $1`, topic,
	).Scan(&seq)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return seq, err
}
