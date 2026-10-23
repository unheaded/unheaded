// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

// Package statefile keeps a service's state as one JSON file, replaced
// atomically: a crash leaves the old state or the new one, never half of
// either. For small state only; it rewrites the whole file on every save.
package statefile

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrNoDir is returned when a service has no state directory configured:
// it must then refuse writes, not keep them in memory until a restart.
var ErrNoDir = errors.New("no state directory: writes cannot be persisted, so they are refused")

// File is one JSON state file.
type File struct {
	Path string
}

// In returns the file name under dir, or nil if dir is empty.
func In(dir, name string) *File {
	if dir == "" {
		return nil
	}
	return &File{Path: filepath.Join(dir, name)}
}

// Load decodes the file into v. A missing file leaves v untouched and
// reports found=false; a file that does not decode is an error, so a
// damaged file is never read as empty state.
func (f *File) Load(v any) (found bool, err error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return true, fmt.Errorf("%s: %w", f.Path, err)
	}
	return true, nil
}

// Save writes v: temp file in the same directory, fsync, rename, fsync the
// directory. The file is 0600 and the directory, if created, 0700.
func (f *File) Save(v any) error {
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(f.Path)+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly after the rename
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.Path); err != nil {
		return err
	}
	d, err := os.Open(dir) // #nosec G304 -- the operator-configured state dir
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
