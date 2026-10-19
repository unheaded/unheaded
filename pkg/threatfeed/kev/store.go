// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package kev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is the metadata kept next to latest.json: what was fetched, when,
// and the validators for the next conditional GET.
type State struct {
	SHA256         string    `json:"sha256"`
	CatalogVersion string    `json:"catalog_version"`
	Count          int       `json:"count"`
	FetchedAt      time.Time `json:"fetched_at"`
	ETag           string    `json:"etag,omitempty"`
	LastModified   string    `json:"last_modified,omitempty"`
}

// Store keeps the last good catalog on disk:
//
//	<dir>/latest.json              raw bytes as published, validated
//	<dir>/state.json               State
//	<dir>/snapshots/<UTC stamp>-<sha8>.json  one per content change
//
// Every file is written to a temporary name in the same directory and
// renamed, so a crash leaves the previous version, never half a file.
type Store struct {
	Dir string
}

const (
	latestName   = "latest.json"
	stateName    = "state.json"
	snapshotsDir = "snapshots"
	fileMode     = 0o640
	dirMode      = 0o750
)

// Load returns the stored catalog and its state. A store that has never been
// written returns (nil, nil, nil). A latest.json that no longer validates is
// an error: the caller must not diff against it.
func (s Store) Load() (*Catalog, *State, error) {
	raw, err := os.ReadFile(filepath.Join(s.Dir, latestName)) // #nosec G304 -- operator-configured state directory
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	c, err := Parse(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("stored %s: %w", latestName, err)
	}
	st := &State{SHA256: Sum(raw), CatalogVersion: c.CatalogVersion, Count: c.Count}
	if b, err := os.ReadFile(filepath.Join(s.Dir, stateName)); err == nil { // #nosec G304 -- as above
		var saved State
		if json.Unmarshal(b, &saved) == nil && saved.SHA256 == st.SHA256 {
			st = &saved
		}
	}
	return c, st, nil
}

// Save writes raw (already parsed into c) as the new latest, records st, and
// keeps a snapshot when the content differs from the previous latest.
func (s Store) Save(raw []byte, c *Catalog, st State, now time.Time) error {
	if err := os.MkdirAll(filepath.Join(s.Dir, snapshotsDir), dirMode); err != nil {
		return err
	}
	st.SHA256 = Sum(raw)
	st.CatalogVersion = c.CatalogVersion
	st.Count = c.Count
	snap := fmt.Sprintf("%s-%s.json", now.UTC().Format("20060102T150405Z"), st.SHA256[:8])
	if err := writeAtomic(filepath.Join(s.Dir, snapshotsDir, snap), raw); err != nil {
		return err
	}
	if err := writeAtomic(filepath.Join(s.Dir, latestName), raw); err != nil {
		return err
	}
	return s.SaveState(st)
}

// SaveState rewrites state.json only (a 304 refreshes FetchedAt).
func (s Store) SaveState(st State) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, dirMode); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.Dir, stateName), append(b, '\n'))
}

// Prune deletes snapshots older than keep, judged by the timestamp in the
// name, but never the newest one. It returns how many it removed.
func (s Store) Prune(keep time.Duration, now time.Time) (int, error) {
	dir := filepath.Join(s.Dir, snapshotsDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	type snap struct {
		name string
		at   time.Time
	}
	var snaps []snap
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		stamp, _, ok := strings.Cut(e.Name(), "-")
		if !ok {
			continue
		}
		at, err := time.Parse("20060102T150405Z", stamp)
		if err != nil {
			continue
		}
		snaps = append(snaps, snap{e.Name(), at})
	}
	newest := -1
	for i, sn := range snaps {
		if newest < 0 || sn.at.After(snaps[newest].at) {
			newest = i
		}
	}
	removed := 0
	for i, sn := range snaps {
		if i == newest || now.Sub(sn.at) <= keep {
			continue
		}
		if err := os.Remove(filepath.Join(dir, sn.name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// Sum is the hex SHA-256 of b.
func Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }() // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Chmod(fileMode); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Sync(); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
