// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"unheaded/pkg/database"
	"unheaded/pkg/logger"
)

// wellJournal keeps the revert journal in The Well (unheaded_ops
// chaos_journal + chaos_journal_transitions, migration 039; S-WELL-SPRING
// Phase 2, M-072). Each host reads only its own entries: west and east
// share one Well and each controller reverts only its own faults.
type wellJournal struct {
	store *database.ChaosStore
	host  string
}

// NewWellJournal keeps the journal in The Well as the connected role
// (ops_writer), for host (KINGDOM_NODE or the hostname). It fails when
// migration 039 is missing.
func NewWellJournal(ctx context.Context, db *sql.DB, host string) (JournalStore, error) {
	if host == "" {
		return nil, errors.New("chaos journal: the Well journal needs a host name")
	}
	s := database.NewChaosStore(db)
	if err := s.CheckSchema(ctx); err != nil {
		return nil, err
	}
	return NewWellJournalFromStore(s, host)
}

// NewWellJournalFromStore is NewWellJournal over a store whose schema the
// caller has checked.
func NewWellJournalFromStore(s *database.ChaosStore, host string) (JournalStore, error) {
	if s == nil || host == "" {
		return nil, errors.New("chaos journal: the Well journal needs a store and a host name")
	}
	return &wellJournal{store: s, host: host}, nil
}

func (w *wellJournal) Mode() string { return "well" }

func (w *wellJournal) Load(ctx context.Context) ([]*JournalEntry, error) {
	rows, err := w.store.Open(ctx, w.host, database.ChaosKindExperiment)
	if err != nil {
		return nil, err
	}
	out := make([]*JournalEntry, 0, len(rows))
	for _, r := range rows {
		e := &JournalEntry{ID: r.ID, Type: ExperimentType(r.Type), Created: r.CreatedAt, Expires: r.ExpiresAt}
		if err := json.Unmarshal(r.Target, &e.Target); err != nil {
			return nil, fmt.Errorf("entry %s: target: %w", r.ID, err)
		}
		if err := json.Unmarshal(r.Recipe, &e.Recipe); err != nil {
			return nil, fmt.Errorf("entry %s: recipe: %w", r.ID, err)
		}
		out = append(out, e)
	}
	return out, nil
}

func (w *wellJournal) Add(ctx context.Context, e *JournalEntry, _ []*JournalEntry) error {
	target, err := json.Marshal(e.Target)
	if err != nil {
		return err
	}
	recipe, err := json.Marshal(e.Recipe)
	if err != nil {
		return err
	}
	if e.Recipe == nil {
		recipe = []byte(`{}`)
	}
	created, expires := e.Created, e.Expires
	if created.IsZero() {
		created = time.Now().UTC()
	}
	if expires.Before(created) {
		expires = created
	}
	return w.store.Append(ctx, database.ChaosEntry{
		ID: e.ID, Host: w.host, Kind: database.ChaosKindExperiment, Type: string(e.Type),
		Target: target, Recipe: recipe, CreatedAt: created, ExpiresAt: expires,
	})
}

func (w *wellJournal) Remove(ctx context.Context, id, why string, _ []*JournalEntry) error {
	return w.store.MarkReverted(ctx, id, why)
}

// importFileJournal moves dir's journal.json entries into store when a
// controller is switched from the file to The Well: a fault journaled in
// the file must not be forgotten. The Well is written first, then the file
// is emptied; an entry already in the Well (a crash between the two) is
// skipped, so a fault reverted since is not brought back.
func importFileJournal(dir string, store JournalStore, log *logger.Logger) error {
	f := NewFileJournal(dir)
	if f == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), journalTimeout)
	defer cancel()
	entries, err := f.Load(ctx)
	if err != nil {
		return fmt.Errorf("chaos journal: reading %s to move it into the Well: %w", journalFileName, err)
	}
	if len(entries) == 0 {
		return nil
	}
	for _, e := range entries {
		if e == nil || e.ID == "" {
			return errors.New("chaos journal: an entry without an id in " + journalFileName)
		}
		if err := store.Add(ctx, e, nil); err != nil && !errors.Is(err, database.ErrDuplicate) {
			return fmt.Errorf("chaos journal: moving %s into the Well: %w", e.ID, err)
		}
	}
	if err := f.(*fileJournalStore).save(nil); err != nil {
		return fmt.Errorf("chaos journal: %d entries are in the Well, but %s could not be emptied: %w", len(entries), journalFileName, err)
	}
	if log != nil {
		log.Info().Int("entries", len(entries)).Str("from", dir).Msg("chaos: file journal moved into the Well")
	}
	return nil
}
