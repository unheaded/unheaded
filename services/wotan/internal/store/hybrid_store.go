// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// HybridStoreType for the factory.
const HybridStoreType StoreType = "hybrid"

// HybridStore combines WAL (speed) + PostgreSQL (durability).
// Writes go to WAL first (fast), then async flush to PG.
// Reads check WAL hot cache first, fall back to PG.
// This is the recommended production configuration.
func NewHybridStore(walDir string, pgConnStr string, capacity int) (*WALStore, error) {
	pg, err := NewPGStore(pgConnStr, capacity)
	if err != nil {
		return nil, fmt.Errorf("hybrid_store: pg: %w", err)
	}

	store, err := NewWALStore(walDir, capacity, pg)
	if err != nil {
		_ = pg.Close()
		return nil, fmt.Errorf("hybrid_store: wal: %w", err)
	}

	return store, nil
}

// PersistentSequenceCounter wraps PGStore.NextSeq for use by TopicService.
// Sequences survive restarts because they're stored in PostgreSQL.
type PersistentSequenceCounter struct {
	pg *PGStore
}

// NewPersistentSequenceCounter creates a PG-backed sequence counter.
func NewPersistentSequenceCounter(pg *PGStore) *PersistentSequenceCounter {
	return &PersistentSequenceCounter{pg: pg}
}

// Next atomically increments and returns the next sequence for a topic.
func (c *PersistentSequenceCounter) Next(ctx context.Context, topic string) (int64, error) {
	return c.pg.NextSeq(ctx, topic)
}

// Last returns the current sequence for a topic without incrementing.
func (c *PersistentSequenceCounter) Last(ctx context.Context, topic string) (int64, error) {
	return c.pg.LastSeq(ctx, topic)
}

// GetAll returns all topic sequences (for replication/recovery).
func (c *PersistentSequenceCounter) GetAll(ctx context.Context) (map[string]int64, error) {
	rows, err := c.pg.db.QueryContext(ctx, `SELECT topic, seq FROM wotan_sequences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var topic string
		var seq int64
		if err := rows.Scan(&topic, &seq); err != nil {
			return nil, err
		}
		result[topic] = seq
	}
	return result, rows.Err()
}

// EpochManager tracks cluster epoch in PostgreSQL for split-brain prevention.
type EpochManager struct {
	pg *PGStore
}

// NewEpochManager creates a PG-backed epoch tracker.
func NewEpochManager(pg *PGStore) *EpochManager {
	return &EpochManager{pg: pg}
}

// GetEpoch returns the current cluster epoch.
func (e *EpochManager) GetEpoch(ctx context.Context) (int64, error) {
	var val string
	err := e.pg.db.QueryRowContext(ctx,
		`SELECT value FROM wotan_cluster WHERE key = 'epoch'`).Scan(&val)
	if err != nil {
		// No epoch set yet — return 0
		return 0, nil
	}
	var epoch int64
	_, _ = fmt.Sscanf(val, "%d", &epoch)
	return epoch, nil
}

// IncrementEpoch atomically increments the epoch (used during failover).
func (e *EpochManager) IncrementEpoch(ctx context.Context) (int64, error) {
	var epoch int64
	err := e.pg.db.QueryRowContext(ctx,
		`INSERT INTO wotan_cluster (key, value, updated_at) VALUES ('epoch', '1', NOW())
		 ON CONFLICT (key) DO UPDATE SET value = (CAST(wotan_cluster.value AS BIGINT) + 1)::TEXT, updated_at = NOW()
		 RETURNING CAST(value AS BIGINT)`).Scan(&epoch)
	return epoch, err
}

// Ensure WALStore satisfies MessageStore at compile time.
var _ MessageStore = (*WALStore)(nil)
var _ MessageStore = (*PGStore)(nil)

// Ensure uuid is used (compile guard).
var _ = uuid.Nil
