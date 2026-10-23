// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"fmt"

	"unheaded/pkg/statefile"
)

// desiredStore keeps Cuirass's desired state on disk. It used to live only
// in memory: a restart forgot every desired container, and with pruning
// on, the next reconcile would delete every managed one as orphaned.
type desiredStore struct {
	file *statefile.File
	path string
}

const desiredFile = "desired.json"

func newDesiredStore(dir string) *desiredStore {
	f := statefile.In(dir, desiredFile)
	return &desiredStore{file: f, path: f.Path}
}

// load reads the desired state. No file is an empty state; a file that
// does not parse, or names an invalid container, is an error: starting
// empty from a damaged file is how the prune would delete everything.
func (s *desiredStore) load() (map[string]*ContainerSpec, error) {
	desired := map[string]*ContainerSpec{}
	if _, err := s.file.Load(&desired); err != nil {
		return nil, err
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

// save replaces the file atomically (statefile).
func (s *desiredStore) save(desired map[string]*ContainerSpec) error {
	return s.file.Save(desired)
}
