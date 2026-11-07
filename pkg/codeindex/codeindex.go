// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package codeindex is a lexical, file-backed index of the repository's
// source: chunks per declaration, a name-matched call graph, word search and
// a store whose readers swap atomically. No vectors, no embeddings.
package codeindex

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// MaxChunkBytes clips Chunk.Content.
const MaxChunkBytes = 2048

// Chunk is one declaration.
type Chunk struct {
	ID        string `json:"id"` // "pkg/x/a.go::Box.Open"
	Path      string `json:"path"`
	Language  string `json:"language"` // go, rust, python
	Kind      string `json:"kind"`     // func, method, type, var, const; fn, struct, enum, trait, impl; def, class
	Symbol    string `json:"symbol"`
	Parent    string `json:"parent,omitempty"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	Content   string `json:"content"` // clipped to MaxChunkBytes
	BlobSHA   string `json:"blob_sha"`
	IsTest    bool   `json:"is_test,omitempty"`
}

// Call is one call site, matched by bare name.
type Call struct {
	Caller string `json:"caller"` // a Chunk ID
	Callee string `json:"callee"` // bare name: helper, Sprint, twice
	Path   string `json:"path"`
	Line   int    `json:"line"`
}

// Index is the whole shard.
type Index struct {
	Head   string            `json:"head"`
	Files  map[string]string `json:"files"` // every indexed path to its blob, chunks or none
	Chunks []Chunk           `json:"chunks"`
	Calls  []Call            `json:"calls"`
}

// Hit is one search result.
type Hit struct {
	Chunk Chunk
	Score float64
}

// Stats reports one Refresh.
type Stats struct{ Files, Parsed, Reused, Deleted, Chunks, Calls int }

// Included reports whether a repo path belongs in the shard.
func Included(path string) bool {
	if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".rs") && !strings.HasSuffix(path, ".py") {
		return false
	}
	if strings.HasSuffix(path, ".pb.go") || strings.HasSuffix(path, ".gen.go") {
		return false
	}
	parts := strings.Split(path, "/")
	switch parts[0] {
	case "cmd", "pkg", "services", "crates", "raft":
	default:
		return false
	}
	for _, p := range parts {
		l := strings.ToLower(p)
		if strings.Contains(l, "doom") || strings.Contains(l, "lich") || strings.Contains(l, "grimoire") {
			return false
		}
		switch l {
		case "target", "_legacy", "node_modules", "upstream":
			return false
		}
	}
	return true
}

func isTestPath(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, "/tests/") ||
		(strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py")) ||
		strings.HasSuffix(base, "_test.py")
}

// Definition returns chunks defining symbol (bare or Parent.Symbol),
// non-test first, then by ID.
func (ix *Index) Definition(symbol string, limit int) []Chunk {
	var out []Chunk
	for _, c := range ix.Chunks {
		if c.Symbol == symbol || (c.Parent != "" && c.Parent+"."+c.Symbol == symbol) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].IsTest != out[j].IsTest {
			return !out[i].IsTest
		}
		return out[i].ID < out[j].ID
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Callers returns call sites whose callee is the last part of symbol.
func (ix *Index) Callers(symbol string, limit int) []Call {
	name := symbol
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		name = symbol[i+1:]
	}
	var out []Call
	for _, c := range ix.Calls {
		if c.Callee == name {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	if limit >= 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Search scores chunks by the share of query words they hold.
func (ix *Index) Search(query string, k int) []Hit {
	if k <= 0 {
		return nil
	}
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return nil
	}
	type scored struct {
		h    Hit
		name int
	}
	var all []scored
	for _, c := range ix.Chunks {
		text := strings.ToLower(c.Symbol + " " + c.Parent + " " + c.Path + " " + c.Content)
		nameText := strings.ToLower(c.Symbol + " " + c.Parent)
		found, inName := 0, 0
		for _, w := range words {
			if strings.Contains(text, w) {
				found++
			}
			if strings.Contains(nameText, w) {
				inName++
			}
		}
		if found == 0 {
			continue
		}
		all = append(all, scored{Hit{Chunk: c, Score: float64(found) / float64(len(words))}, inName})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].h.Score != all[j].h.Score {
			return all[i].h.Score > all[j].h.Score
		}
		if all[i].name != all[j].name {
			return all[i].name > all[j].name
		}
		return all[i].h.Chunk.ID < all[j].h.Chunk.ID
	})
	if len(all) > k {
		all = all[:k]
	}
	out := make([]Hit, len(all))
	for i, s := range all {
		out[i] = s.h
	}
	return out
}

func slotFile(dir, slot string) string { return filepath.Join(dir, "index-"+slot+".json") }

func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	// #nosec G306 -- repo code and tracked text only; readers need 0644
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readSlot(dir string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "CURRENT")) // #nosec G304 -- caller-chosen index dir
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s != "a" && s != "b" {
		return "", errors.New("codeindex: bad CURRENT")
	}
	return s, nil
}

// Save writes ix into the slot that is not current, then points CURRENT at it.
func Save(dir string, ix *Index) error {
	// #nosec G301 -- index of tracked text, readable by the services
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	slot := "a"
	if cur, err := readSlot(dir); err == nil && cur == "a" {
		slot = "b"
	}
	data, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := writeAtomic(slotFile(dir, slot), data); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "CURRENT"), []byte(slot+"\n"))
}

// Load reads the current slot.
func Load(dir string) (*Index, error) {
	slot, err := readSlot(dir)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(slotFile(dir, slot)) // #nosec G304 -- caller-chosen index dir
	if err != nil {
		return nil, err
	}
	var ix Index
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, err
	}
	return &ix, nil
}

// Store hands readers the current index and swaps when CURRENT changes.
type Store struct {
	dir     string
	mu      sync.Mutex
	loaded  *Index
	slot    string
	modTime time.Time
}

// NewStore returns a Store over dir.
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Get returns the current index, reloading if CURRENT changed. A failed
// reload with a good older index returns the older one.
func (s *Store) Get() (*Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, err := readSlot(s.dir)
	var fi os.FileInfo
	if err == nil {
		fi, err = os.Stat(filepath.Join(s.dir, "CURRENT"))
	}
	if err != nil {
		if s.loaded != nil {
			return s.loaded, nil
		}
		return nil, err
	}
	if s.loaded != nil && slot == s.slot && fi.ModTime().Equal(s.modTime) {
		return s.loaded, nil
	}
	ix, err := Load(s.dir)
	if err != nil {
		if s.loaded != nil {
			return s.loaded, nil
		}
		return nil, err
	}
	s.loaded, s.slot, s.modTime = ix, slot, fi.ModTime()
	return ix, nil
}
