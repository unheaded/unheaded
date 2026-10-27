// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// fakeSophia mimics cmd/sophia's /api/v1/knowledge envelope.
type fakeSophia struct {
	mu   sync.Mutex
	rows []map[string]any
	auth string
}

func (f *fakeSophia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = r.Header.Get("Authorization")
	if r.URL.Path != "/api/v1/knowledge" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		b, _ := io.ReadAll(r.Body)
		var row map[string]any
		_ = json.Unmarshal(b, &row)
		row["created_at"] = "2026-10-23T00:00:0" + string(rune('0'+len(f.rows))) + "Z"
		f.rows = append(f.rows, row)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"knowledge": row}})
	case http.MethodGet:
		var out []map[string]any
		for _, row := range f.rows {
			if row["subject"] == r.URL.Query().Get("subject") && row["predicate"] == r.URL.Query().Get("predicate") {
				out = append(out, row)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"knowledge": out, "count": len(out)}})
	}
}

func TestSophiaRegistry_RoundTrip(t *testing.T) {
	t.Parallel()
	fs := &fakeSophia{}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	r := &SophiaRegistry{BaseURL: srv.URL, Token: "tok", Blobs: &FileRegistry{Dir: t.TempDir()}, Client: srv.Client()}
	raw := testImage(t, 0xFF000000)
	if err := r.Put(testRecord("ratelimit", raw), raw, []byte("seal")); err != nil {
		t.Fatal(err)
	}
	// A second registration of the same name wins (newest record).
	raw2 := testImage(t, 0x00000000)
	if err := r.Put(testRecord("ratelimit", raw2), raw2, []byte("seal2")); err != nil {
		t.Fatal(err)
	}
	rec, gotRaw, gotSeal, err := r.Get("ratelimit")
	if err != nil {
		t.Fatal(err)
	}
	if rec.SHA256 != sha256Hex(raw2) || string(gotRaw) != string(raw2) || string(gotSeal) != "seal2" {
		t.Fatalf("got %+v", rec)
	}
	if fs.auth != "Bearer tok" {
		t.Fatalf("auth header %q", fs.auth)
	}
	if fs.rows[0]["subject"] != "cartridge/ratelimit" || fs.rows[0]["predicate"] != "cartridge.record" {
		t.Fatalf("row %v", fs.rows[0])
	}
	if _, _, _, err := r.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

// S-TRUE-WIRE Steps 33-35: the RFC Editor assigned root key 0x15, category
// "cartridge". Records are written under "cartridge/<name>" only; a record
// written before the assignment under "x-cartridge/<name>" is still read
// (sophia-dictionary-03: MAY be accepted on read, MUST NOT be written).
func TestSophiaRegistry_ReadsLegacyNamespace(t *testing.T) {
	t.Parallel()
	if Namespace != "cartridge" || LegacyNamespace != "x-cartridge" || CartridgeRootKey != 0x15 {
		t.Fatalf("namespace %q legacy %q root key %#x", Namespace, LegacyNamespace, CartridgeRootKey)
	}
	fs := &fakeSophia{}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	r := &SophiaRegistry{BaseURL: srv.URL, Blobs: &FileRegistry{Dir: t.TempDir()}, Client: srv.Client()}
	raw := testImage(t, 0xFF000000)
	if err := r.Put(testRecord("old", raw), raw, []byte("seal")); err != nil {
		t.Fatal(err)
	}
	// Rewrite the stored row as a pre-assignment record.
	fs.mu.Lock()
	fs.rows[0]["subject"] = "x-cartridge/old"
	fs.mu.Unlock()
	rec, gotRaw, _, err := r.Get("old")
	if err != nil {
		t.Fatalf("legacy record not read: %v", err)
	}
	if rec.SHA256 != sha256Hex(raw) || string(gotRaw) != string(raw) {
		t.Fatalf("legacy record %+v", rec)
	}
	for _, row := range fs.rows {
		if row["subject"] != "x-cartridge/old" {
			t.Fatalf("a read wrote a row: %v", row)
		}
	}
}
