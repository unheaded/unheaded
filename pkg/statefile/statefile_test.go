// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package statefile

import (
	"os"
	"path/filepath"
	"testing"
)

type state struct {
	Items map[string]int `json:"items"`
}

func TestRoundTrip(t *testing.T) {
	f := In(t.TempDir(), "state.json")
	var empty state
	if found, err := f.Load(&empty); found || err != nil {
		t.Fatalf("missing file: found=%v err=%v", found, err)
	}
	if err := f.Save(state{Items: map[string]int{"a": 1}}); err != nil {
		t.Fatal(err)
	}
	var got state
	if found, err := f.Load(&got); !found || err != nil || got.Items["a"] != 1 {
		t.Fatalf("load: found=%v err=%v got=%+v", found, err, got)
	}
	fi, err := os.Stat(f.Path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v (%v), want 0600", fi.Mode().Perm(), err)
	}
	if left, _ := filepath.Glob(f.Path + ".*"); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
}

func TestDamagedFileIsAnError(t *testing.T) {
	f := In(t.TempDir(), "state.json")
	if err := os.WriteFile(f.Path, []byte(`{"items":`), 0o600); err != nil {
		t.Fatal(err)
	}
	var got state
	if _, err := f.Load(&got); err == nil {
		t.Fatal("damaged file loaded")
	}
}

func TestNoDir(t *testing.T) {
	if In("", "state.json") != nil {
		t.Fatal("empty dir should give no file")
	}
}

func TestSaveFailsWhenDirCannotExist(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := In(filepath.Join(blocker, "sub"), "state.json").Save(state{}); err == nil {
		t.Fatal("save under a file succeeded")
	}
}
