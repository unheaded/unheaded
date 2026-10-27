// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"unheaded/pkg/compliance/remediation"
)

// The host's apply journal (ADR-099): <state>/journal.jsonl, one
// hash-chained entry per apply and undo, which the collector pulls into
// The Well. Before writing, the whole file is verified: a broken chain is
// evidence of tampering, so the executor refuses to work on top of it
// rather than extend it. Moving the file aside does not help: The Well
// refuses a journal shorter than what it already ingested.

const journalName = "journal.jsonl"

// undoMeta ties a backup directory to the fix it undoes, for the journal.
type undoMeta struct {
	Fix       string `json:"fix"`
	FixSHA256 string `json:"fix_sha256"`
	Host      string `json:"host"`
}

func (e *Executor) journalPath() string { return filepath.Join(e.o.State, journalName) }

// lastEntry verifies the journal and returns its last entry (nil if empty).
func (e *Executor) lastEntry() (*remediation.JournalEntry, error) {
	b, err := os.ReadFile(e.journalPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := remediation.ReadJournal(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", remediation.ErrChainBroken, e.journalPath(), err)
	}
	if err := remediation.VerifyChain(entries, remediation.GenesisHash); err != nil {
		return nil, fmt.Errorf("%s: %w", e.journalPath(), err)
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[len(entries)-1], nil
}

// checkJournal is run before any change: no apply on a broken journal.
func (e *Executor) checkJournal() error {
	_, err := e.lastEntry()
	if err != nil {
		return fmt.Errorf("%w: the apply journal does not verify, so nothing was changed: %v", ErrRefused, err)
	}
	return nil
}

// appendJournal seals and appends one entry, synced to disk.
func (e *Executor) appendJournal(fixID, fixSHA, action, outcome, dir string) error {
	return e.appendJournalEntry(remediation.JournalEntry{FixID: fixID, FixSHA256: fixSHA, Action: action,
		Outcome: outcome, BackupDir: dir, Operator: e.o.Operator})
}

// appendJournalEntry fills in host, sequence, model commit and time, then
// seals and appends the entry.
func (e *Executor) appendJournalEntry(entry remediation.JournalEntry) error {
	last, err := e.lastEntry()
	if err != nil {
		return err
	}
	entry.Host, entry.Seq, entry.ModelCommit, entry.At = e.o.Host, 1, e.o.ModelCommit, e.o.Now()
	prev := remediation.GenesisHash
	if last != nil {
		entry.Seq, prev = last.Seq+1, last.Hash
	}
	if err := entry.Seal(prev); err != nil {
		return err
	}
	f, err := os.OpenFile(e.journalPath(), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304 -- <state>/journal.jsonl
	if err != nil {
		return err
	}
	if err := entry.WriteJSONL(f); err != nil {
		return errors.Join(err, f.Close())
	}
	return errors.Join(f.Sync(), f.Close())
}

func outcomeOf(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, errRestoreFailed):
		return "failure"
	case errors.Is(err, ErrRefused):
		return "refused"
	default:
		return "restored"
	}
}

func writeMeta(dir string, m undoMeta) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "fix.json"), b, 0o600, -1, -1, false)
}

// UndoDir undoes one apply (its backup directory) and journals it.
func (e *Executor) UndoDir(ctx context.Context, dir string) error {
	if err := e.checkJournal(); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, "fix.json")) // #nosec G304 -- an operator-named backup directory
	if err != nil {
		return fmt.Errorf("%s is not a backup directory of this executor: %w", dir, err)
	}
	var m undoMeta
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.Host != e.o.Host {
		return fmt.Errorf("%w: %s was applied on %s, this is %s", ErrRefused, dir, m.Host, e.o.Host)
	}
	uerr := Undo(ctx, dir, e.o.Out, e.o.AllowNonRoot)
	outcome := "success"
	if uerr != nil {
		outcome = "failure"
	}
	return errors.Join(uerr, e.appendJournal(m.Fix, m.FixSHA256, "undo", outcome, dir))
}
