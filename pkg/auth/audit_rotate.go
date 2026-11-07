// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package auth

import (
	"fmt"
	"os"
)

// AUTH_AUDIT_LOG's bound: the live file and auditRotateKeep generations
// (<path>.1 the newest), each at most auditRotateBytes.
const (
	auditRotateBytes = 64 << 20
	auditRotateKeep  = 7
)

// rotatingFile is an append-only 0600 file that moves aside when the next
// write would take it past max, keeping keep older generations; the oldest
// is dropped. A write is never split across files. A failed rotation loses no
// event and drops no generation that a working one would have kept: the
// current file stays open, the failure goes to stderr, and the next attempt
// waits for another max bytes. Until one works the file outgrows max, because
// keeping the events comes before the bound. Not safe for concurrent use;
// AuditLogger holds its lock around Write and Close.
type rotatingFile struct {
	path  string
	max   int64
	keep  int
	open  func(string) (*os.File, error)
	f     *os.File
	size  int64
	limit int64 // the size that triggers the next rotation
}

func openRotatingFile(path string, maxBytes int64, keep int) (*rotatingFile, error) {
	f, err := openAuditFile(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &rotatingFile{path: path, max: maxBytes, keep: keep, open: openAuditFile, f: f, size: info.Size(), limit: maxBytes}, nil
}

// Write implements io.Writer.
func (r *rotatingFile) Write(p []byte) (int, error) {
	if r.size > 0 && r.size+int64(len(p)) > r.limit {
		if err := r.rotate(); err != nil {
			r.limit = r.size + r.max
			fmt.Fprintf(auditStderr, "auth: cannot rotate %s: %v\n", r.path, err)
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate swaps in a new file. The new file is opened first, under a spare
// name, so the step most likely to fail changes nothing. The open file follows
// its rename, so a failure after that leaves events still landing in it, and a
// later attempt picks up from whatever step was reached. A live file something
// else removed is not a failure: its events are gone already, and the new file
// puts path back.
func (r *rotatingFile) rotate() error {
	spare := r.path + ".next"
	f, err := r.open(spare)
	if err != nil {
		return err
	}
	if err := r.shift(); err != nil {
		_ = f.Close()
		return err
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil && !os.IsNotExist(err) {
		_ = f.Close()
		return err
	}
	if err := os.Rename(spare, r.path); err != nil {
		_ = f.Close()
		return err
	}
	_ = r.f.Close()
	r.f, r.size, r.limit = f, 0, r.max
	return nil
}

// shift frees <path>.1 by moving generations up into the first gap, so the
// oldest goes only when there is no gap. An attempt that stopped halfway left
// one, and the next attempt drops nothing to finish the move.
func (r *rotatingFile) shift() error {
	gap := 1
	for gap < r.keep {
		if _, err := os.Lstat(r.generation(gap)); err != nil {
			break
		}
		gap++
	}
	for i := gap - 1; i >= 1; i-- {
		if err := os.Rename(r.generation(i), r.generation(i+1)); err != nil {
			return err
		}
	}
	return nil
}

func (r *rotatingFile) generation(i int) string {
	return fmt.Sprintf("%s.%d", r.path, i)
}

// Close implements io.Closer.
func (r *rotatingFile) Close() error {
	return r.f.Close()
}
