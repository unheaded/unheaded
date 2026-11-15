// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package retrieval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"unheaded/pkg/codeindex"
)

// A shard whose CURRENT names a slot file that is gone is a broken shard, and
// the backend says so. It used to read as "no index": zero hits and no error,
// which the federation counts as a backend that answered.
func TestCodeBackend_MissingSlotIsAnError(t *testing.T) {
	dir := saveIndex(t, chunk("a.go::Open", "func Open() { unlock() }"))
	if err := os.Remove(filepath.Join(dir, "index-a.json")); err != nil {
		t.Fatal(err)
	}
	refs, contents, err := NewCodeBackend(codeindex.NewStore(dir)).Retrieve(context.Background(), "unlock", 3)
	if err == nil {
		t.Fatalf("a missing slot answered: refs %+v contents %+v", refs, contents)
	}
	if !errors.Is(err, codeindex.ErrSlotMissing) {
		t.Errorf("err = %v, want codeindex.ErrSlotMissing", err)
	}
	if len(refs) != 0 || len(contents) != 0 {
		t.Errorf("an error came with hits: refs %+v contents %+v", refs, contents)
	}
}

// No index built at all stays what it was: no hits, no error.
func TestCodeBackend_NoIndexIsNoHits(t *testing.T) {
	refs, contents, err := NewCodeBackend(codeindex.NewStore(t.TempDir())).Retrieve(context.Background(), "unlock", 3)
	if err != nil || len(refs) != 0 || len(contents) != 0 {
		t.Fatalf("no index: refs %+v contents %+v err %v, want nothing and nil", refs, contents, err)
	}
}

// The backend passes its context to the scan: a cancelled request ends with
// context.Canceled, not with hits found after nobody was waiting.
func TestCodeBackend_StopsAtItsContext(t *testing.T) {
	b := NewCodeBackend(codeindex.NewStore(saveIndex(t, chunk("a.go::Open", "func Open() { unlock() }"))))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	refs, contents, err := b.Retrieve(ctx, "unlock", 3)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(refs) != 0 || len(contents) != 0 {
		t.Fatalf("a cancelled search came with hits: refs %+v contents %+v", refs, contents)
	}
	// Control: the same backend answers a live context.
	if refs, _, err := b.Retrieve(context.Background(), "unlock", 3); err != nil || len(refs) != 1 {
		t.Fatalf("live context: refs %+v err %v, want one hit", refs, err)
	}
}
