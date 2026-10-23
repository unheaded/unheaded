// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// genesisFixture is the repository's Genesis JSON (the source of
// migration 034); tests that need a realistic board load it.
const genesisFixture = "../../db/seed/kanban-genesis.json"

func loadGenesisFixture(t *testing.T) []Task {
	t.Helper()
	tasks, err := readGenesisFile(genesisFixture)
	if err != nil {
		t.Fatalf("load %s: %v", genesisFixture, err)
	}
	return tasks
}

func TestGenesisFixture_IsTheSeed(t *testing.T) {
	tasks := loadGenesisFixture(t)
	if len(tasks) != 65 {
		t.Errorf("cards = %d, want 65", len(tasks))
	}
	ids := map[string]bool{}
	for _, x := range tasks {
		ids[x.ID] = true
		if x.CreatedAt.After(x.UpdatedAt) || x.CreatedAt.Year() != 2026 {
			t.Errorf("%s: dates %v .. %v", x.ID, x.CreatedAt, x.UpdatedAt)
		}
	}
	for _, id := range []string{"ms-phase0", "svc-wotan", "debt-seed-reconcile"} {
		if !ids[id] {
			t.Errorf("%s missing", id)
		}
	}
}

func TestSeedFromFile_EmptyStoreOnly(t *testing.T) {
	store := createTestStore(t)
	n, err := store.SeedFromFile(genesisFixture)
	if err != nil || n != 65 {
		t.Fatalf("first seed: %d %v", n, err)
	}
	got, err := store.GetTask("ms-phase0")
	if err != nil || got.CreatedAt.IsZero() || got.Title == "" {
		t.Fatalf("seeded card: %+v %v", got, err)
	}
	// A store with tasks is never re-seeded or merged.
	if err := store.DeleteTask("ms-phase0"); err != nil {
		t.Fatal(err)
	}
	n, err = store.SeedFromFile(genesisFixture)
	if err != nil || n != 64 {
		t.Errorf("second seed: %d %v (want the existing 64)", n, err)
	}
	if _, err := store.GetTask("ms-phase0"); err == nil {
		t.Error("a deleted card came back from the Genesis file")
	}
}

func TestSeedFromFile_RefusesBadFiles(t *testing.T) {
	d := t.TempDir()
	for name, body := range map[string]string{
		"notjson":   "{",
		"unknown":   `[{"id":"a","title":"t","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","priority":1}]`,
		"duplicate": `[{"id":"a","title":"t","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},{"id":"a","title":"u","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
		"noid":      `[{"title":"t","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
		"progress":  `[{"id":"a","title":"t","progress":101,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}]`,
		"nodates":   `[{"id":"a","title":"t"}]`,
	} {
		p := filepath.Join(d, name+".json")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		store := createTestStore(t)
		if _, err := store.SeedFromFile(p); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if n, _ := store.TaskCount(); n != 0 {
			t.Errorf("%s: %d rows after a refused seed", name, n)
		}
	}
	if _, err := createTestStore(t).SeedFromFile(filepath.Join(d, "missing.json")); err == nil {
		t.Error("a missing file was accepted")
	}
}

// Without KANBAN_GENESIS_FILE a new SQLite board starts empty; with it,
// the file seeds it; a bad file means no store at all.
func TestOpenSQLiteStore_GenesisOnlyFromTheEnvFile(t *testing.T) {
	t.Setenv(GenesisFileEnv, "")
	s, err := openSQLiteStore(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.TaskCount(); n != 0 {
		t.Errorf("no env: %d cards, want an empty board", n)
	}
	_ = s.Close()

	t.Setenv(GenesisFileEnv, genesisFixture)
	s, err = openSQLiteStore(filepath.Join(t.TempDir(), "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := s.TaskCount(); n != 65 {
		t.Errorf("env set: %d cards, want 65", n)
	}
	_ = s.Close()

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(GenesisFileEnv, bad)
	if _, err := openSQLiteStore(filepath.Join(t.TempDir(), "c.db")); err == nil || !strings.Contains(err.Error(), GenesisFileEnv) {
		t.Errorf("bad genesis file: %v", err)
	}
}

// The binary carries no seed text (S-TWO-BRAINS Step 428): no source file
// of the package holds a Genesis card title.
func TestNoGenesisSeedInSource(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f) // #nosec G304 -- this package's own sources
		if err != nil {
			t.Fatal(err)
		}
		for _, needle := range []string{"The Foundation " + "Stone", "getInitial" + "Tasks", "SeedIf" + "Empty"} {
			if strings.Contains(string(b), needle) {
				t.Errorf("%s still holds %q", f, needle)
			}
		}
	}
}
