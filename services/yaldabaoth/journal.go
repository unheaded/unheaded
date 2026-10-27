// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package yaldabaoth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"unheaded/pkg/statefile"
)

// The revert journal (S-REAL-STEEL D-19). Before an injector acts, its
// revert recipe is written to the journal store: The Well
// (unheaded_ops.chaos_journal, migration 039; journal_well.go) when the
// controller runs with WELL_DB, otherwise CHAOS_STATE_DIR/journal.json
// (0600, replaced atomically). The fault is reverted at the experiment's end, on any
// abort, by its TTL (duration + RevertGrace) even if the experiment's own
// loop is wedged, at the next start of the controller, and by
// `chaos-controller revert-all` (the unit's ExecStopPost, which runs after
// a crash or SIGKILL too). An entry is removed only after its injector
// verifies the fault is gone; otherwise it stays, chaos.revert_failed is
// published, and every later revert pass tries again. In The Well removing
// an entry appends a 'reverted' row; the entry itself is never changed.

const journalFileName = "journal.json"

// journalTimeout bounds one journal write or read.
const journalTimeout = 10 * time.Second

// JournalStore keeps the revert journal. Add and Remove get the open
// entries after the change because the file store rewrites them all; the
// Well store appends one row.
type JournalStore interface {
	// Mode is "well" or "file".
	Mode() string
	// Load returns the open entries (faults that may still be in place).
	Load(ctx context.Context) ([]*JournalEntry, error)
	// Add journals e before its fault lands.
	Add(ctx context.Context, e *JournalEntry, open []*JournalEntry) error
	// Remove closes entry id once its fault is verified gone.
	Remove(ctx context.Context, id, why string, open []*JournalEntry) error
}

// fileJournalStore is CHAOS_STATE_DIR/journal.json.
type fileJournalStore struct{ file *statefile.File }

// NewFileJournal keeps the journal in dir/journal.json; nil for an empty dir.
func NewFileJournal(dir string) JournalStore {
	f := statefile.In(dir, journalFileName)
	if f == nil {
		return nil
	}
	return &fileJournalStore{file: f}
}

func (f *fileJournalStore) Mode() string { return "file" }

func (f *fileJournalStore) Load(context.Context) ([]*JournalEntry, error) {
	var jf journalFile
	if _, err := f.file.Load(&jf); err != nil {
		return nil, err
	}
	return jf.Entries, nil
}

func (f *fileJournalStore) save(open []*JournalEntry) error {
	return f.file.Save(journalFile{Version: 1, Entries: open})
}

func (f *fileJournalStore) Add(_ context.Context, _ *JournalEntry, open []*JournalEntry) error {
	return f.save(open)
}

func (f *fileJournalStore) Remove(_ context.Context, _, _ string, open []*JournalEntry) error {
	return f.save(open)
}

// JournalEntry is one fault in place.
type JournalEntry struct {
	ID      string         `json:"id"`
	Type    ExperimentType `json:"type"`
	Target  ResolvedTarget `json:"target"`
	Recipe  Recipe         `json:"recipe"`
	Created time.Time      `json:"created"`
	Expires time.Time      `json:"expires"`
}

type journalFile struct {
	Version int             `json:"version"`
	Entries []*JournalEntry `json:"entries"`
}

type journal struct {
	store   JournalStore
	mu      sync.Mutex
	entries map[string]*JournalEntry
}

func openJournal(store JournalStore) (*journal, error) {
	j := &journal{store: store, entries: map[string]*JournalEntry{}}
	ctx, cancel := context.WithTimeout(context.Background(), journalTimeout)
	defer cancel()
	entries, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("chaos journal (%s): %w", store.Mode(), err)
	}
	for _, e := range entries {
		if e == nil || e.ID == "" || j.entries[e.ID] != nil {
			return nil, errors.New("chaos journal: an entry without a unique id")
		}
		j.entries[e.ID] = e
	}
	return j, nil
}

func (j *journal) openLocked() []*JournalEntry {
	out := make([]*JournalEntry, 0, len(j.entries))
	for _, id := range j.idsLocked() {
		out = append(out, j.entries[id])
	}
	return out
}

func (j *journal) idsLocked() []string {
	ids := make([]string, 0, len(j.entries))
	for id := range j.entries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// add persists e; on failure the journal is unchanged.
func (j *journal) add(e *JournalEntry) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, dup := j.entries[e.ID]; dup {
		return fmt.Errorf("journal already holds %s", e.ID)
	}
	j.entries[e.ID] = e
	ctx, cancel := context.WithTimeout(context.Background(), journalTimeout)
	defer cancel()
	if err := j.store.Add(ctx, e, j.openLocked()); err != nil {
		delete(j.entries, e.ID)
		return err
	}
	return nil
}

func (j *journal) remove(id, why string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[id]
	if !ok {
		return nil
	}
	delete(j.entries, id)
	ctx, cancel := context.WithTimeout(context.Background(), journalTimeout)
	defer cancel()
	if err := j.store.Remove(ctx, id, why, j.openLocked()); err != nil {
		j.entries[id] = e
		return err
	}
	return nil
}

func (j *journal) get(id string) *JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e, ok := j.entries[id]; ok {
		cp := *e
		return &cp
	}
	return nil
}

func (j *journal) list() []JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]JournalEntry, 0, len(j.entries))
	for _, id := range j.idsLocked() {
		out = append(out, *j.entries[id])
	}
	return out
}

// JournalEntries returns the faults in place now.
func (s *Service) JournalEntries() []JournalEntry {
	if s.journal == nil {
		return nil
	}
	return s.journal.list()
}

const revertTimeout = 30 * time.Second

// revertEntry reverts and verifies one journaled fault. It reports whether
// the fault is gone, and a High finding when it is not. Safe to call again
// and from several places (end of run, TTL, abort, revert-all).
func (s *Service) revertEntry(id, why string) (bool, *Finding) {
	s.revertMu.Lock()
	defer s.revertMu.Unlock()
	if s.journal == nil {
		return true, nil
	}
	e := s.journal.get(id)
	if e == nil {
		return true, nil // already reverted
	}
	s.mu.RLock()
	inj := s.injectors[e.Type]
	s.mu.RUnlock()
	fail := func(cause string) (bool, *Finding) {
		s.log.Error().Str("id", id).Str("why", why).Str("cause", cause).Msg("chaos: revert not verified; the journal entry stays")
		s.audit.Record(AuditEntry{Timestamp: time.Now().UTC(), Action: "revert", RuleID: id, Result: "failed: " + cause})
		s.publish(TopicRevertFailed, map[string]any{"experiment_id": id, "type": e.Type, "target": e.Target, "cause": cause})
		return false, &Finding{Severity: SeverityHigh, Title: "Revert not verified",
			Description: fmt.Sprintf("%s on %s/%s: %s", e.Type, e.Target.Kind, e.Target.ID, cause),
			Remediation: "Appendix A1 of the S-REAL-STEEL battle plan: touch the abort file, run chaos-controller revert-all, then remove the fault by hand"}
	}
	if inj == nil {
		return fail("no injector registered for " + string(e.Type))
	}
	ctx, cancel := context.WithTimeout(context.Background(), revertTimeout)
	defer cancel()
	if err := inj.Revert(ctx, e.Recipe); err != nil {
		return fail("revert: " + err.Error())
	}
	gone, err := inj.Reverted(ctx, e.Recipe)
	if err != nil {
		return fail("verify: " + err.Error())
	}
	if !gone {
		return fail("the fault is still in place after its revert")
	}
	if err := s.journal.remove(id, why); err != nil {
		return fail("reverted, but the journal could not be updated: " + err.Error())
	}
	s.disarmTTL(id)
	s.updateActive()
	s.audit.Record(AuditEntry{Timestamp: time.Now().UTC(), Action: "revert", RuleID: id, Result: "verified (" + why + ")"})
	return true, nil
}

// RevertAll reverts every journaled fault and returns how many are still
// in place. revert-all exits 0 only when that is zero; a second run is a
// no-op.
func (s *Service) RevertAll(_ context.Context) (int, error) {
	if s.journal == nil {
		return 0, nil
	}
	var errs []error
	for _, e := range s.journal.list() {
		if ok, f := s.revertEntry(e.ID, "revert-all"); !ok && f != nil {
			errs = append(errs, errors.New(f.Description))
		}
	}
	return len(s.journal.list()), errors.Join(errs...)
}

// armTTL reverts the entry when it expires and aborts its experiment.
func (s *Service) armTTL(e *JournalEntry) {
	d := time.Until(e.Expires)
	if d < 0 {
		d = 0
	}
	t := time.AfterFunc(d, func() {
		s.abortOne(e.ID, "ttl", "fault TTL reached")
		s.revertEntry(e.ID, "ttl")
	})
	s.ttlMu.Lock()
	s.ttl[e.ID] = t
	s.ttlMu.Unlock()
}

func (s *Service) disarmTTL(id string) {
	s.ttlMu.Lock()
	defer s.ttlMu.Unlock()
	if t, ok := s.ttl[id]; ok {
		t.Stop()
		delete(s.ttl, id)
	}
}
