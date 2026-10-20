// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophia

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"unheaded/pkg/logger"
)

func limitedService(cfg func(*Config)) *Service {
	c := DefaultConfig()
	c.EnableInference = false
	cfg(c)
	return NewService(logger.New(os.Stderr), nil, c)
}

// MaxKnowledge was configured (and tested to be set) but never enforced:
// /api/v1/knowledge takes unauthenticated POSTs, so the map grew until the
// 128 MiB container was OOM-killed, taking every fact it held with it.
func TestLearn_EnforcesMaxKnowledge(t *testing.T) {
	s := limitedService(func(c *Config) { c.MaxKnowledge = 3 })
	for i := 0; i < 3; i++ {
		if err := s.Learn(context.Background(), &Knowledge{Subject: fmt.Sprint(i), Predicate: "p", Object: "o"}); err != nil {
			t.Fatalf("learn %d: %v", i, err)
		}
	}
	if err := s.Learn(context.Background(), &Knowledge{Subject: "x", Predicate: "p", Object: "o"}); err != ErrKnowledgeFull {
		t.Fatalf("fourth fact: err=%v, want ErrKnowledgeFull", err)
	}
}

// A count alone does not bound memory: one POST may carry 64 KB.
func TestLearn_EnforcesByteBudget(t *testing.T) {
	s := limitedService(func(c *Config) { c.MaxKnowledgeBytes = 100 << 10 })
	big := strings.Repeat("x", 30<<10)
	learned := 0
	for i := 0; i < 10; i++ {
		err := s.Learn(context.Background(), &Knowledge{Subject: fmt.Sprint(i), Predicate: "p", Object: big})
		if err == ErrKnowledgeFull {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		learned++
	}
	if learned != 3 {
		t.Fatalf("learned %d 30 KiB facts under a 100 KiB budget, want 3", learned)
	}
	if s.knowledgeBytes > 100<<10 {
		t.Fatalf("knowledgeBytes=%d over the budget", s.knowledgeBytes)
	}
}

// /api/v1/decide stored every decision forever. The newest MaxDecisions are
// kept, so GetDecision still answers for recent ones.
func TestDecide_KeepsTheNewestMaxDecisions(t *testing.T) {
	s := limitedService(func(c *Config) { c.MaxDecisions = 5 })
	var ids []string
	for i := 0; i < 12; i++ {
		d, err := s.Decide(context.Background(), fmt.Sprint("q", i), []Option{{Name: "a"}, {Name: "b"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	if n := len(s.decisions); n != 5 {
		t.Fatalf("%d decisions kept, want 5", n)
	}
	if _, ok := s.GetDecision(ids[len(ids)-1]); !ok {
		t.Fatal("the newest decision is gone")
	}
	if _, ok := s.GetDecision(ids[0]); ok {
		t.Fatal("the oldest decision is still there")
	}
}
