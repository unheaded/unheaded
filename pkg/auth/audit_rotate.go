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
// is dropped. A write is never split across files. Rotation cannot lose an
// event: if it fails the current file stays open, the failure goes to
// stderr, and the next attempt waits for another max bytes. Not safe for
// concurrent use; AuditLogger holds its lock around Write and Close.
type rotatingFile struct {
	path  string
	max   int64
	keep  int
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
	return &rotatingFile{path: path, max: maxBytes, keep: keep, f: f, size: info.Size(), limit: maxBytes}, nil
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

// rotate shifts the generations and swaps in a new file. The open file
// follows its rename, so a failure at any step leaves events still landing
// in it.
func (r *rotatingFile) rotate() error {
	for i := r.keep - 1; i >= 1; i-- {
		err := os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil {
		return err
	}
	f, err := openAuditFile(r.path)
	if err != nil {
		return err
	}
	_ = r.f.Close()
	r.f, r.size, r.limit = f, 0, r.max
	return nil
}

// Close implements io.Closer.
func (r *rotatingFile) Close() error {
	return r.f.Close()
}
