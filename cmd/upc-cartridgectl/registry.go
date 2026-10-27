// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The cartridge registry (ADR-108 D-C4). A record names an image by its
// sha256; the image bytes and seal are stored content-addressed on disk.
// Where the record lives is the registry backend: Sophia (category
// "cartridge", root key 0x15, assigned by the RFC Editor in S-TRUE-WIRE
// Phase 1; sophia-dictionary-03) or a file-backed directory with the same
// schema while the stack is down. Records written before the assignment
// under "x-cartridge" are still read, never written.
// The storage location is not trusted either way: every load re-verifies
// the seal on the bytes it reads.

const (
	// Namespace is the Sophia category records are written under.
	Namespace = "cartridge"
	// LegacyNamespace is the provisional pre-assignment category: read as an
	// alias (MAY be accepted on read), never written.
	LegacyNamespace = "x-cartridge"
	// CartridgeRootKey is the root dictionary key assigned to the category.
	CartridgeRootKey uint8 = 0x15
	SurfaceVersion         = "cartridge/v0"
	predicate              = "cartridge.record"
	maxBlob                = 1 << 20
)

var (
	ErrName     = errors.New("registry: cartridge name must match [a-z0-9][a-z0-9._-]{0,62}")
	ErrNotFound = errors.New("registry: no such cartridge")
	ErrBlob     = errors.New("registry: stored image does not match its record")
	nameRE      = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
)

// Record is one registry entry.
type Record struct {
	Name         string    `json:"name"`
	SHA256       string    `json:"sha256"`
	ImageSHA256  string    `json:"image_sha256"`
	KeyID        string    `json:"seal_key_id"`
	Stage        string    `json:"stage"`
	Surface      string    `json:"surface_version"`
	MaxInsn      int       `json:"max_insn"`
	Slot         int       `json:"slot"`
	Size         int       `json:"size"`
	Lab          bool      `json:"lab"`
	Namespace    string    `json:"namespace"`
	RegisteredAt time.Time `json:"registered_at"`
}

// Registry stores records and their image bytes.
type Registry interface {
	Put(rec Record, raw, seal []byte) error
	Get(name string) (Record, []byte, []byte, error)
	List() ([]Record, error)
}

// FileRegistry keeps <dir>/<name>.json and <dir>/blobs/<sha>.cart[.seal]
// (directories 0700, files 0600).
type FileRegistry struct{ Dir string }

func (r *FileRegistry) blobDir() string { return filepath.Join(r.Dir, "blobs") }

// recordDir holds <name>.json, apart from slots.json and blobs/ in the root.
func (r *FileRegistry) recordDir() string { return filepath.Join(r.Dir, "records") }

func readCapped(path string) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- inside the operator's registry dir, name validated
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBlob+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBlob {
		return nil, fmt.Errorf("%s: larger than %d bytes", path, maxBlob)
	}
	return b, nil
}

func (r *FileRegistry) putBlobs(sha string, raw, seal []byte) error {
	if err := os.MkdirAll(r.blobDir(), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(r.Dir, 0o700); err != nil { // #nosec G302 -- a directory needs x; 0700 is owner-only
		return err
	}
	if err := os.WriteFile(filepath.Join(r.blobDir(), sha+".cart"), raw, 0o600); err != nil {
		return err
	}
	if seal != nil {
		return os.WriteFile(filepath.Join(r.blobDir(), sha+".cart.seal"), seal, 0o600)
	}
	return nil
}

func (r *FileRegistry) getBlobs(sha string) ([]byte, []byte, error) {
	raw, err := readCapped(filepath.Join(r.blobDir(), sha+".cart"))
	if err != nil {
		return nil, nil, err
	}
	if sha256Hex(raw) != sha {
		return nil, nil, ErrBlob
	}
	seal, err := readCapped(filepath.Join(r.blobDir(), sha+".cart.seal"))
	if errors.Is(err, os.ErrNotExist) {
		seal, err = nil, nil
	}
	return raw, seal, err
}

// Put stores the record and its bytes.
func (r *FileRegistry) Put(rec Record, raw, seal []byte) error {
	if !nameRE.MatchString(rec.Name) {
		return ErrName
	}
	if err := r.putBlobs(rec.SHA256, raw, seal); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(r.recordDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.recordDir(), rec.Name+".json"), append(b, '\n'), 0o600)
}

// Get returns the record, its image bytes and seal (nil if unsigned).
func (r *FileRegistry) Get(name string) (Record, []byte, []byte, error) {
	var rec Record
	if !nameRE.MatchString(name) {
		return rec, nil, nil, ErrName
	}
	b, err := readCapped(filepath.Join(r.recordDir(), name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return rec, nil, nil, ErrNotFound
	}
	if err != nil {
		return rec, nil, nil, err
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return rec, nil, nil, err
	}
	raw, seal, err := r.getBlobs(rec.SHA256)
	return rec, raw, seal, err
}

// List returns every record, by name.
func (r *FileRegistry) List() ([]Record, error) {
	ents, err := os.ReadDir(r.recordDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, e := range ents {
		n, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !nameRE.MatchString(n) {
			continue
		}
		rec, _, _, err := r.Get(n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out = append(out, rec)
	}
	return out, nil
}

// SophiaRegistry stores records as Sophia knowledge entries
// (subject "cartridge/<name>", predicate "cartridge.record", object the
// sha256, the record in metadata) and the bytes in Blobs.
type SophiaRegistry struct {
	BaseURL string
	Token   string // bearer token for Sophia's service auth ("" = none)
	Blobs   *FileRegistry
	Client  *http.Client
}

func (r *SophiaRegistry) do(req *http.Request) ([]byte, error) {
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	c := r.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("sophia: %s: %s", resp.Status, bytes.TrimSpace(b))
	}
	return b, nil
}

// Put stores the bytes, then the record in Sophia.
func (r *SophiaRegistry) Put(rec Record, raw, seal []byte) error {
	if !nameRE.MatchString(rec.Name) {
		return ErrName
	}
	// The local store keeps the bytes and a mirror of the record (for List);
	// Get answers from Sophia.
	if err := r.Blobs.Put(rec, raw, seal); err != nil {
		return err
	}
	var meta map[string]any
	rb, _ := json.Marshal(rec)
	_ = json.Unmarshal(rb, &meta)
	body, err := json.Marshal(map[string]any{
		"type": "fact", "subject": Namespace + "/" + rec.Name, "predicate": predicate,
		"object": rec.SHA256, "source": "upc-cartridgectl", "confidence": 1.0, "metadata": meta,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(r.BaseURL, "/")+"/api/v1/knowledge", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = r.do(req)
	return err
}

type sophiaRow struct {
	Object    string          `json:"object"`
	CreatedAt string          `json:"created_at"`
	Metadata  json.RawMessage `json:"metadata"`
}

func (r *SophiaRegistry) query(subject string) ([]sophiaRow, error) {
	q := url.Values{"subject": {subject}, "predicate": {predicate}, "limit": {"1000"}}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(r.BaseURL, "/")+"/api/v1/knowledge?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	b, err := r.do(req)
	if err != nil {
		return nil, err
	}
	var env struct {
		Data struct {
			Knowledge []sophiaRow `json:"knowledge"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, fmt.Errorf("sophia: %w", err)
	}
	return env.Data.Knowledge, nil
}

// Get returns the newest record for name.
func (r *SophiaRegistry) Get(name string) (Record, []byte, []byte, error) {
	var rec Record
	if !nameRE.MatchString(name) {
		return rec, nil, nil, ErrName
	}
	rows, err := r.query(Namespace + "/" + name)
	if err != nil {
		return rec, nil, nil, err
	}
	if len(rows) == 0 {
		// A record registered before the root key was assigned.
		if rows, err = r.query(LegacyNamespace + "/" + name); err != nil {
			return rec, nil, nil, err
		}
	}
	if len(rows) == 0 {
		return rec, nil, nil, ErrNotFound
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt < rows[j].CreatedAt })
	newest := rows[len(rows)-1]
	if err := json.Unmarshal(newest.Metadata, &rec); err != nil {
		return rec, nil, nil, fmt.Errorf("sophia record: %w", err)
	}
	if rec.SHA256 != newest.Object || rec.Name != name {
		return rec, nil, nil, ErrBlob
	}
	raw, seal, err := r.Blobs.getBlobs(rec.SHA256)
	return rec, raw, seal, err
}

// List returns the records the blob store knows names for (Sophia's query
// API matches exact subjects only, so the file store's index is used).
func (r *SophiaRegistry) List() ([]Record, error) { return r.Blobs.List() }
