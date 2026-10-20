// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monad

import (
	"context"
	"testing"
)

// Every query, mutation and effect was kept in s.operations (with its
// client-supplied input), every transaction in s.transactions and every
// state change in s.stateChanges, forever: memory grew with ordinary traffic
// inside a 128 MiB container. The newest are kept, the oldest go.
func TestRetention_OperationsTransactionsStateChanges(t *testing.T) {
	s := newTestService()
	s.maxOperations, s.maxTransactions, s.maxStateChanges = 5, 3, 4
	s.RegisterQuery("q", func(context.Context, interface{}) (interface{}, error) { return 1, nil })
	s.RegisterMutation("m", func(context.Context, interface{}) (interface{}, []StateChange, error) {
		return 1, []StateChange{{Entity: "e"}, {Entity: "f"}}, nil
	})
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		s.Query(ctx, "q", i)
		s.Mutate(ctx, "m", i)
		s.BeginTransaction(ctx)
	}
	if n := len(s.operations); n != 5 {
		t.Errorf("operations kept = %d, want 5", n)
	}
	if n := len(s.transactions); n != 3 {
		t.Errorf("transactions kept = %d, want 3", n)
	}
	if n := len(s.stateChanges); n != 4 {
		t.Errorf("state changes kept = %d, want 4", n)
	}
	newest := s.ListOperations(1)
	if len(newest) != 1 {
		t.Fatalf("ListOperations(1) = %d ops", len(newest))
	}
	if in, _ := newest[0].Input.(int); in != 19 {
		t.Errorf("newest operation's input = %v, want 19", newest[0].Input)
	}
}

// ListOperations returns most recent first.
func TestListOperations_NewestFirst(t *testing.T) {
	s := newTestService()
	s.RegisterQuery("q", func(context.Context, interface{}) (interface{}, error) { return 1, nil })
	for i := 0; i < 50; i++ {
		s.Query(context.Background(), "q", i)
	}
	ops := s.ListOperations(0)
	for i := 1; i < len(ops); i++ {
		if ops[i-1].StartedAt.Before(ops[i].StartedAt) {
			t.Fatalf("ops[%d] older than ops[%d]", i-1, i)
		}
	}
}
