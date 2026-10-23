// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"context"
	"testing"
)

// An infer query answers with what the knowledge base supports. The
// built-in rules have no conditions, and a rule with no conditions counted
// as met, so every infer query (GET /api/v1/knowledge?type=infer) returned
// two invented "inference" facts, with an empty knowledge base too.
func TestQueryInfer_NoConditionsNoInference(t *testing.T) {
	s := limitedService(func(*Config) {})
	s.registerDefaultRules()
	ctx := context.Background()

	res, err := s.Query(ctx, &Query{Type: QueryInfer, Subject: "anything"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Errorf("empty knowledge base: %d results (%+v), want none", len(res), res[0])
	}

	if err := s.Learn(ctx, &Knowledge{Subject: "go", Predicate: "is_a", Object: "language"}); err != nil {
		t.Fatal(err)
	}
	res, _ = s.Query(ctx, &Query{Type: QueryInfer, Subject: "go"})
	for _, k := range res {
		if k.Type == KnowledgeInference {
			t.Errorf("inferred %q from a rule with no conditions", k.Object)
		}
	}
	if len(res) != 1 {
		t.Errorf("%d results, want the one learned fact", len(res))
	}
}
