// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wal provides a Write-Ahead Log implementation for the Kingdom.
// The WAL ensures durability by writing all changes to a log before applying them.
package wal

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"unheaded/pkg/logger"
	"unheaded/pkg/metrics"
	"unheaded/pkg/storage"
)

// Common errors.
var (
	// ErrCorruptedEntry indicates a corrupted log entry.
	ErrCorruptedEntry = errors.New("wal: corrupted entry")

	// ErrSequenceNotFound indicates the sequence number was not found.
	ErrSequenceNotFound = errors.New("wal: sequence not found")
)

// Config holds WAL configuration.
type Config struct {
	// Path is the directory for WAL files.
	Path string `json:"path" yaml:"path"`

	// SegmentSize is the maximum size of a segment file.
	SegmentSize int64 `json:"segment_size" yaml:"segment_size"`

	// SyncWrites enables fsync after each write.
	SyncWrites bool `json:"sync_writes" yaml:"sync_writes"`

	// MaxSegments is the maximum number of segments to keep.
	MaxSegments int `json:"max_segments" yaml:"max_segments"`

	// Logger is the logger instance.
	Logger *logger.Logger

	// Metrics is the metrics registry.
	Metrics *metrics.Registry
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Path:        "/var/lib/unheaded/wal",
		SegmentSize: 64 * 1024 * 1024, // 64MB
		SyncWrites:  true,
		MaxSegments: 10,
	}
}

// Metrics for WAL operations.
var (
	walWritesTotal = metrics.NewCounterVec(
		"unheaded_wal_writes_total",
		"Total WAL writes",
		nil,
		[]string{"status"},
	)

	walWriteDuration = metrics.NewHistogram(metrics.HistogramOpts{
		Name:    "unheaded_wal_write_duration_seconds",
		Help:    "WAL write duration",
		Buckets: metrics.ExponentialBuckets(0.0001, 2, 12),
	})

	walBytesWritten = metrics.NewCounter(
		"unheaded_wal_bytes_written_total",
		"Total bytes written to WAL",
		nil,
	)

	walSegmentsTotal = metrics.NewGauge(
		"unheaded_wal_segments_total",
		"Total number of WAL segments",
		nil,
	)

	walSequence = metrics.NewGauge(
		"unheaded_wal_sequence",
		"Current WAL sequence number",
		nil,
	)
)

// WAL is a Write-Ahead Log implementation.
type WAL struct {
	cfg      Config
	log      *logger.Logger
	mu       sync.Mutex
	segments []*Segment
	current  *Segment
	sequence uint64
	closed   bool
}

// New creates a new WAL.
func New(cfg Config) (*WAL, error) {
	log := cfg.Logger
	if log == nil {
		log = logger.New(os.Stdout)
	}

	// Create directory
	if err := os.MkdirAll(cfg.Path, 0750); err != nil {
		return nil, fmt.Errorf("wal: create directory: %w", err)
	}

	wal := &WAL{
		cfg:      cfg,
		log:      log.With().Str("component", "wal").Logger(),
		segments: make([]*Segment, 0),
	}

	// Load existing segments
	if err := wal.loadSegments(); err != nil {
		return nil, fmt.Errorf("wal: load segments: %w", err)
	}

	// Create initial segment if none exist
	if len(wal.segments) == 0 {
		if err := wal.createSegment(); err != nil {
			return nil, fmt.Errorf("wal: create initial segment: %w", err)
		}
	} else {
		wal.current = wal.segments[len(wal.segments)-1]
		// Recover sequence from last segment
		if err := wal.recoverSequence(); err != nil {
			return nil, fmt.Errorf("wal: recover sequence: %w", err)
		}
	}

	walSegmentsTotal.Set(float64(len(wal.segments)))
	walSequence.Set(float64(wal.sequence))

	wal.log.Info().
		Str("path", cfg.Path).
		Int("segments", len(wal.segments)).
		Uint64("sequence", wal.sequence).
		Msg("wal initialized")

	return wal, nil
}

// loadSegments loads existing segment files.
func (w *WAL) loadSegments() error {
	entries, err := os.ReadDir(w.cfg.Path)
	if err != nil {
		return err
	}

	var segmentFiles []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".wal") {
			segmentFiles = append(segmentFiles, entry.Name())
		}
	}

	sort.Strings(segmentFiles)

	for _, name := range segmentFiles {
		path := filepath.Join(w.cfg.Path, name)
		segment, err := OpenSegment(path, w.cfg.SyncWrites)
		if err != nil {
			w.log.Warn().Err(err).Str("file", name).Msg("failed to open segment, skipping")
			continue
		}
		w.segments = append(w.segments, segment)
	}

	return nil
}

// recoverSequence recovers the last sequence number.
func (w *WAL) recoverSequence() error {
	if w.current == nil {
		return nil
	}

	var lastSeq uint64
	err := w.current.Scan(0, func(seq uint64, data []byte) error {
		lastSeq = seq
		return nil
	})
	if err != nil && err != io.EOF {
		return err
	}

	w.sequence = lastSeq
	return nil
}

// createSegment creates a new segment.
func (w *WAL) createSegment() error {
	// Generate segment name based on time
	name := fmt.Sprintf("%016d.wal", time.Now().UnixNano())
	path := filepath.Join(w.cfg.Path, name)

	segment, err := CreateSegment(path, w.cfg.SyncWrites)
	if err != nil {
		return err
	}

	w.segments = append(w.segments, segment)
	w.current = segment

	walSegmentsTotal.Set(float64(len(w.segments)))
	w.log.Debug().Str("segment", name).Msg("created new segment")

	return nil
}

// Write appends an entry to the log.
func (w *WAL) Write(ctx context.Context, data []byte) (uint64, error) {
	start := time.Now()

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		walWritesTotal.WithLabels(metrics.Labels{"status": "error"}).Inc()
		return 0, storage.ErrClosed
	}

	// Check if we need a new segment
	if w.current.Size() >= w.cfg.SegmentSize {
		if err := w.current.Close(); err != nil {
			w.log.Warn().Err(err).Msg("failed to close current segment")
		}
		if err := w.createSegment(); err != nil {
			walWritesTotal.WithLabels(metrics.Labels{"status": "error"}).Inc()
			return 0, fmt.Errorf("wal: create segment: %w", err)
		}
	}

	// Increment sequence
	seq := atomic.AddUint64(&w.sequence, 1)

	// Write to segment
	if err := w.current.Write(seq, data); err != nil {
		walWritesTotal.WithLabels(metrics.Labels{"status": "error"}).Inc()
		return 0, fmt.Errorf("wal: write entry: %w", err)
	}

	duration := time.Since(start).Seconds()
	walWritesTotal.WithLabels(metrics.Labels{"status": "success"}).Inc()
	walWriteDuration.Observe(duration)
	walBytesWritten.Add(float64(len(data)))
	walSequence.Set(float64(seq))

	return seq, nil
}

// Read reads an entry by sequence number.
func (w *WAL) Read(ctx context.Context, seq uint64) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil, storage.ErrClosed
	}

	// Search segments in reverse order (most likely to find in recent segments)
	for i := len(w.segments) - 1; i >= 0; i-- {
		segment := w.segments[i]
		data, err := segment.Read(seq)
		if err == ErrSequenceNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		return data, nil
	}

	return nil, ErrSequenceNotFound
}

// Scan iterates over entries starting from a sequence number.
func (w *WAL) Scan(ctx context.Context, startSeq uint64, fn func(seq uint64, data []byte) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return storage.ErrClosed
	}

	for _, segment := range w.segments {
		err := segment.Scan(startSeq, func(seq uint64, data []byte) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return fn(seq, data)
		})
		if err != nil && err != io.EOF {
			return err
		}
	}

	return nil
}

// Truncate removes entries before the given sequence number.
func (w *WAL) Truncate(ctx context.Context, beforeSeq uint64) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return storage.ErrClosed
	}

	// Find segments to remove
	var toRemove []*Segment
	var toKeep []*Segment

	for _, segment := range w.segments {
		lastSeq := segment.LastSequence()
		if lastSeq < beforeSeq && segment != w.current {
			toRemove = append(toRemove, segment)
		} else {
			toKeep = append(toKeep, segment)
		}
	}

	// Remove old segments
	for _, segment := range toRemove {
		if err := segment.Close(); err != nil {
			w.log.Warn().Err(err).Str("path", segment.Path()).Msg("failed to close segment")
		}
		if err := os.Remove(segment.Path()); err != nil {
			w.log.Warn().Err(err).Str("path", segment.Path()).Msg("failed to remove segment")
		}
		w.log.Debug().Str("path", segment.Path()).Msg("removed old segment")
	}

	w.segments = toKeep
	walSegmentsTotal.Set(float64(len(w.segments)))

	return nil
}

// Sync ensures all entries are flushed to disk.
func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return storage.ErrClosed
	}

	if w.current != nil {
		return w.current.Sync()
	}
	return nil
}

// LastSequence returns the last written sequence number.
func (w *WAL) LastSequence() uint64 {
	return atomic.LoadUint64(&w.sequence)
}

// Close releases resources.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}

	w.closed = true

	for _, segment := range w.segments {
		if err := segment.Close(); err != nil {
			w.log.Warn().Err(err).Str("path", segment.Path()).Msg("failed to close segment")
		}
	}

	w.log.Info().Msg("wal closed")
	return nil
}

// Checkpoint creates a checkpoint and truncates old entries.
func (w *WAL) Checkpoint(ctx context.Context) error {
	// Get current sequence
	seq := w.LastSequence()

	// Sync current segment
	if err := w.Sync(); err != nil {
		return fmt.Errorf("wal: sync failed: %w", err)
	}

	// Truncate old segments if we have too many
	w.mu.Lock()
	numSegments := len(w.segments)
	w.mu.Unlock()

	if numSegments > w.cfg.MaxSegments {
		// Find the sequence of the oldest segment to keep
		w.mu.Lock()
		oldestToKeep := w.segments[numSegments-w.cfg.MaxSegments]
		truncateSeq := oldestToKeep.FirstSequence()
		w.mu.Unlock()

		if err := w.Truncate(ctx, truncateSeq); err != nil {
			return fmt.Errorf("wal: truncate failed: %w", err)
		}
	}

	w.log.Info().Uint64("sequence", seq).Msg("checkpoint completed")
	return nil
}

// Segments returns the number of segments.
func (w *WAL) Segments() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.segments)
}

// Size returns the total size of all segments.
func (w *WAL) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	var total int64
	for _, segment := range w.segments {
		total += segment.Size()
	}
	return total
}

// Entry represents a WAL entry.
type Entry struct {
	Sequence uint64
	Data     []byte
}

// EntryHeader is the binary header for an entry.
// Format: [sequence:8][length:4][crc:4][data:length]
const entryHeaderSize = 16

// encodeEntry encodes an entry for storage.
func encodeEntry(seq uint64, data []byte) []byte {
	// Calculate CRC
	crc := crc32.ChecksumIEEE(data)

	// Allocate buffer
	buf := make([]byte, entryHeaderSize+len(data))

	// Write header
	binary.BigEndian.PutUint64(buf[0:8], seq)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(data)))
	binary.BigEndian.PutUint32(buf[12:16], crc)

	// Write data
	copy(buf[16:], data)

	return buf
}
