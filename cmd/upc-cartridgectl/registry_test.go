// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testRecord(name string, raw []byte) Record {
	return Record{Name: name, SHA256: sha256Hex(raw), Stage: "xdp", Surface: SurfaceVersion, MaxInsn: 64, KeyID: "abcd", Namespace: Namespace, Size: len(raw)}
}

func TestFileRegistry_RoundTripAndPerms(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "reg")
	r := &FileRegistry{Dir: dir}
	raw := testImage(t, 0xFF000000)
	seal := []byte(`{"seal":1}`)
	if err := r.Put(testRecord("pass", raw), raw, seal); err != nil {
		t.Fatal(err)
	}
	rec, gotRaw, gotSeal, err := r.Get("pass")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "pass" || string(gotRaw) != string(raw) || string(gotSeal) != string(seal) {
		t.Fatalf("roundtrip %+v", rec)
	}
	for path, want := range map[string]os.FileMode{
		dir:                           0o700,
		filepath.Join(dir, "records"): 0o700,
		filepath.Join(dir, "records", "pass.json"):      0o600,
		filepath.Join(dir, "blobs"):                     0o700,
		filepath.Join(dir, "blobs", rec.SHA256+".cart"): 0o600,
	} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s mode %v, want %v", path, fi.Mode().Perm(), want)
		}
	}
	// slots.json (written by load) sits in the root and is never read as a
	// record (live-lab bug, S5 Phase 12).
	if err := writeSlots(dir, map[int]SlotState{0: {Name: "pass"}}); err != nil {
		t.Fatal(err)
	}
	list, err := r.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v %v", list, err)
	}
	// A blob altered on disk is refused on Get (the record pins its sha).
	if err := os.WriteFile(filepath.Join(dir, "blobs", rec.SHA256+".cart"), []byte("swapped"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := r.Get("pass"); !errors.Is(err, ErrBlob) {
		t.Fatalf("swapped blob: %v", err)
	}
}

func TestRegistry_NameValidation(t *testing.T) {
	t.Parallel()
	r := &FileRegistry{Dir: t.TempDir()}
	raw := testImage(t, 0xFF000000)
	for _, bad := range []string{"", "../x", "a/b", "UPPER", ".hidden", strings.Repeat("a", 64), "sp ace"} {
		if err := r.Put(testRecord(bad, raw), raw, nil); !errors.Is(err, ErrName) {
			t.Errorf("name %q: %v", bad, err)
		}
		if _, _, _, err := r.Get(bad); !errors.Is(err, ErrName) {
			t.Errorf("get %q: %v", bad, err)
		}
	}
}
