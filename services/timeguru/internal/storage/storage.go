// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package storage holds timeguru's timeline: in memory, built from
// references/timeline.md at start and on change, with the Well's
// timeline_milestones (migration 050, ADR-147) as its one home. A milestone
// changed through the API after the file's modification time keeps that
// change over the file at start. Without the Well timeguru serves the file
// read-only and waits; there is no local SQLite copy (it lived on a tmpfs,
// so every restart lost the API's changes).
package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"unheaded/pkg/database"
	"unheaded/services/timeguru/internal/timeline"
)

var (
	ErrNilTimeline       = errors.New("timeline cannot be nil")
	ErrNotFound          = errors.New("timeline not found")
	ErrMilestoneNotFound = errors.New("milestone not found")
	ErrEmptyMilestoneID  = errors.New("milestone ID cannot be empty")
	ErrInvalidProgress   = errors.New("progress must be between 0 and 100")
	ErrInvalidStatus     = errors.New("invalid status")
	// ErrWellUnavailable: a write while the Well is not connected.
	ErrWellUnavailable = errors.New("the Well is unavailable")
	// ErrWellLimit: a milestone timeline_milestones (migration 050) cannot
	// hold; the timeline is refused before memory or the Well change.
	ErrWellLimit = errors.New("milestone does not fit the Well")
)

// wellTimeout bounds one call to the Well; the store's lock is held across
// it, so a Well that never answers must not hold reads for ever.
var wellTimeout = 15 * time.Second

// Backend is timeline_milestones (pkg/database.TimelineStore).
type Backend interface {
	ReplaceMilestones(ctx context.Context, rows []database.MilestoneRow) error
	UpdateMilestone(ctx context.Context, key string, progress int, status string) error
	NewerThan(ctx context.Context, t time.Time) (map[string]database.MilestoneRow, error)
}

// Store is the timeline in memory and, once connected, in the Well.
type Store struct {
	mu      sync.RWMutex
	tl      *timeline.Timeline
	fileMod time.Time
	backend Backend
	closer  func() error
}

// New returns a store with no timeline and no Well yet.
func New() *Store { return &Store{} }

// WellState is "connected" or "waiting".
func (s *Store) WellState() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.backend == nil {
		return "waiting"
	}
	return "connected"
}

// SetCloser is called by Close (the Well connection).
func (s *Store) SetCloser(fn func() error) {
	s.mu.Lock()
	s.closer = fn
	s.mu.Unlock()
}

// Attach connects the Well: the timeline in memory, with the rows changed
// after the file's modification time overlaid, is written to it, within
// wellTimeout.
func (s *Store) Attach(ctx context.Context, b Backend) error {
	if b == nil {
		return errors.New("attach: nil backend")
	}
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tl != nil {
		if err := s.syncLocked(ctx, b, s.tl, s.fileMod); err != nil {
			return err
		}
	}
	s.backend = b
	return nil
}

// Connect calls connect until it returns a backend that attaches, waiting
// min after the first failure and doubling to max, or until ctx ends. A
// backend that is an io.Closer (the connection pool) is closed when it
// fails to attach, so a retry never leaves the last pool open, and is
// closed by Close once attached.
func (s *Store) Connect(ctx context.Context, connect func(context.Context) (Backend, error), minWait, maxWait time.Duration) {
	wait := minWait
	for {
		b, err := connect(ctx)
		if err == nil {
			err = s.Attach(ctx, b)
			c, closes := b.(io.Closer)
			if err == nil {
				if closes {
					s.SetCloser(c.Close)
				}
				return
			}
			if closes {
				_ = c.Close()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(wait*2, maxWait)
	}
}

// Load takes a timeline parsed from the file (modified at fileMod) as the
// source. With the Well connected, rows changed after fileMod keep their
// status and progress, and the result is written to it; without the Well
// the file is served as it is. A failed Well write is returned; the
// timeline in memory is the file's either way.
func (s *Store) Load(ctx context.Context, tl *timeline.Timeline, fileMod time.Time) error {
	if tl == nil {
		return ErrNilTimeline
	}
	if err := validate(tl); err != nil {
		return err
	}
	c, err := clone(tl)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tl, s.fileMod = c, fileMod
	if s.backend == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	return s.syncLocked(ctx, s.backend, s.tl, fileMod)
}

// Limits of timeline_milestones (db/migrations/050): milestone_key
// VARCHAR(64), name up to 500 characters. Status and risk are checked by
// timeline.Validate.
const (
	maxKeyChars  = 64
	maxNameChars = 500
)

// validate checks the timeline and that the Well can hold every milestone.
func validate(tl *timeline.Timeline) error {
	if err := tl.Validate(); err != nil {
		return fmt.Errorf("validate timeline: %w", err)
	}
	for _, m := range tl.Milestones {
		if n := utf8.RuneCountInString(m.ID); n > maxKeyChars {
			return fmt.Errorf("%w: id %.20q... is %d characters, the Well holds %d", ErrWellLimit, m.ID, n, maxKeyChars)
		}
		if n := utf8.RuneCountInString(m.Name); n > maxNameChars {
			return fmt.Errorf("%w: milestone %q has a %d-character name, the Well holds %d", ErrWellLimit, m.ID, n, maxNameChars)
		}
	}
	return nil
}

// syncLocked overlays rows newer than fileMod on tl and writes tl to b.
func (s *Store) syncLocked(ctx context.Context, b Backend, tl *timeline.Timeline, fileMod time.Time) error {
	newer, err := b.NewerThan(ctx, fileMod)
	if err != nil {
		return fmt.Errorf("read the Well's newer rows: %w", err)
	}
	for _, m := range tl.Milestones {
		if r, ok := newer[m.ID]; ok {
			m.Status, m.Progress = r.Status, r.ProgressPct
		}
	}
	if err := b.ReplaceMilestones(ctx, rows(tl)); err != nil {
		return fmt.Errorf("write the timeline to the Well: %w", err)
	}
	return nil
}

// SaveTimeline replaces the timeline from an import: a write, so it needs
// the Well.
func (s *Store) SaveTimeline(ctx context.Context, tl *timeline.Timeline) error {
	if tl == nil {
		return ErrNilTimeline
	}
	if err := validate(tl); err != nil {
		return err
	}
	c, err := clone(tl)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return ErrWellUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	if err := s.backend.ReplaceMilestones(ctx, rows(c)); err != nil {
		return fmt.Errorf("write the timeline to the Well: %w", err)
	}
	s.tl = c
	return nil
}

// GetTimeline returns a copy of the current timeline.
func (s *Store) GetTimeline(_ context.Context) (*timeline.Timeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.tl == nil {
		return nil, ErrNotFound
	}
	return clone(s.tl)
}

// UpdateMilestone sets a milestone's progress and status, in the Well
// first: without it the update is refused.
func (s *Store) UpdateMilestone(ctx context.Context, milestoneID string, progress int, status string) error {
	if milestoneID == "" {
		return ErrEmptyMilestoneID
	}
	if progress < 0 || progress > 100 {
		return fmt.Errorf("%w: %d", ErrInvalidProgress, progress)
	}
	switch status {
	case "pending", "in_progress", "completed", "blocked":
	default:
		return fmt.Errorf("%w: %q", ErrInvalidStatus, status)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backend == nil {
		return ErrWellUnavailable
	}
	if s.tl == nil {
		return ErrNotFound
	}
	m, found := s.tl.GetMilestoneByID(milestoneID)
	if !found {
		return fmt.Errorf("%w: %s", ErrMilestoneNotFound, milestoneID)
	}
	ctx, cancel := context.WithTimeout(ctx, wellTimeout)
	defer cancel()
	if err := s.backend.UpdateMilestone(ctx, milestoneID, progress, status); err != nil {
		if errors.Is(err, database.ErrMilestoneNotFound) {
			return fmt.Errorf("%w: %s", ErrMilestoneNotFound, milestoneID)
		}
		return fmt.Errorf("update milestone in the Well: %w", err)
	}
	m.Progress, m.Status = progress, status
	s.tl.LastUpdated = time.Now().UTC()
	return nil
}

// Close closes the Well connection, if any (idempotent).
func (s *Store) Close() error {
	s.mu.Lock()
	fn := s.closer
	s.closer, s.backend = nil, nil
	s.mu.Unlock()
	if fn != nil {
		return fn()
	}
	return nil
}

// rows maps the timeline's milestones to timeline_milestones rows.
func rows(tl *timeline.Timeline) []database.MilestoneRow {
	out := make([]database.MilestoneRow, 0, len(tl.Milestones))
	for _, m := range tl.Milestones {
		out = append(out, database.MilestoneRow{
			Key: m.ID, Name: m.Name, Status: m.Status, ProgressPct: m.Progress, ETA: m.ETA,
			Owner: m.Owner, Risk: m.Risk, Description: m.Description,
		})
	}
	return out
}

// clone deep-copies a timeline (callers may change what they get).
func clone(tl *timeline.Timeline) (*timeline.Timeline, error) {
	b, err := json.Marshal(tl)
	if err != nil {
		return nil, fmt.Errorf("copy timeline: %w", err)
	}
	var c timeline.Timeline
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("copy timeline: %w", err)
	}
	return &c, nil
}
