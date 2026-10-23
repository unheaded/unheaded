// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"context"
	"testing"
	"time"
)

// Relearning an ID replaces the fact. Its old index entries used to stay, so
// an exact query on the subject listed it once per relearn, a fact that had
// moved to another subject still sat in the old subject's index, the
// indexes grew without bound under repeated POSTs of one ID, and created_at
// moved to the latest relearn.
func TestLearn_RelearningAnIDReplacesIt(t *testing.T) {
	s := limitedService(func(*Config) {})
	ctx := context.Background()
	first := time.Now().Add(-time.Hour)
	if err := s.Learn(ctx, &Knowledge{ID: "k1", Subject: "go", Predicate: "is", Object: "fast", CreatedAt: first}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if err := s.Learn(ctx, &Knowledge{ID: "k1", Subject: "go", Predicate: "is", Object: "fast"}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := s.Query(ctx, &Query{Type: QueryExact, Subject: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Errorf("exact query on subject: %d results, want 1", len(res))
	}
	res, _ = s.Query(ctx, &Query{Type: QueryExact, Predicate: "is"})
	if len(res) != 1 {
		t.Errorf("exact query on predicate: %d results, want 1", len(res))
	}
	if n := len(s.subjectIndex["go"]) + len(s.predicateIndex["is"]) + len(s.typeIndex[""]); n != 3 {
		t.Errorf("index entries for one fact = %d, want 3", n)
	}
	if k, _ := s.GetKnowledge("k1"); !k.CreatedAt.Equal(first) {
		t.Errorf("created_at = %v, want the first learn's %v", k.CreatedAt, first)
	}

	// Moved to another subject: the old subject no longer lists it.
	if err := s.Learn(ctx, &Knowledge{ID: "k1", Subject: "rust", Predicate: "is", Object: "fast"}); err != nil {
		t.Fatal(err)
	}
	if ids := s.subjectIndex["go"]; len(ids) != 0 {
		t.Errorf("old subject still indexes %v", ids)
	}
	if st := s.Stats(); st["indexed_subjects"] != 1 {
		t.Errorf("indexed_subjects = %v, want 1", st["indexed_subjects"])
	}
}
