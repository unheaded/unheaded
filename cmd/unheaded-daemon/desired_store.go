// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// desiredStore keeps Cuirass's desired state on disk. It used to live only
// in memory: a restart forgot every desired container, and with pruning
// on, the next reconcile would delete every managed one as orphaned.
type desiredStore struct {
	path string
}

const desiredFile = "desired.json"

func newDesiredStore(dir string) *desiredStore {
	return &desiredStore{path: filepath.Join(dir, desiredFile)}
}

// load reads the desired state. No file is an empty state; a file that
// does not parse, or names an invalid container, is an error: starting
// empty from a damaged file is how the prune would delete everything.
func (s *desiredStore) load() (map[string]*ContainerSpec, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]*ContainerSpec{}, nil
	}
	if err != nil {
		return nil, err
	}
	desired := map[string]*ContainerSpec{}
	if err := json.Unmarshal(data, &desired); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path, err)
	}
	for id, spec := range desired {
		if spec == nil || spec.ID != id {
			return nil, fmt.Errorf("%s: entry %q does not carry its own id", s.path, id)
		}
		if err := validLXDName(id); err != nil {
			return nil, fmt.Errorf("%s: %w", s.path, err)
		}
	}
	return desired, nil
}

// save replaces the file atomically: a crash leaves the old state or the
// new one, never half of either.
func (s *desiredStore) save(desired map[string]*ContainerSpec) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(desired, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, desiredFile+".*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // fails harmlessly after the rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return err
	}
	d, err := os.Open(dir) // #nosec G304 -- the operator-configured state dir
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
