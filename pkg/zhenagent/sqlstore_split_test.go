// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package zhenagent

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// The worker's store needs both pools: the board (unheaded_app) and the
// runs (unheaded_zhen) are two databases since ADR-149.
func TestNewSQLStore_NeedsBothPools(t *testing.T) {
	db := &sql.DB{} // never dialled: only nil-ness is checked
	if _, err := NewSQLStore(nil, db); err == nil {
		t.Error("NewSQLStore(nil, runs) accepted")
	}
	if _, err := NewSQLStore(db, nil); err == nil {
		t.Error("NewSQLStore(board, nil) accepted")
	}
	s, err := NewSQLStore(db, db)
	if err != nil || s.Board != db || s.Runs != db {
		t.Fatalf("NewSQLStore(board, runs) = %+v, %v", s, err)
	}
}

// A runs-only store (zhen-claude review) refuses the board's methods with
// ErrNoBoard instead of touching a nil pool.
func TestSQLStore_RunsOnlyRefusesTheBoard(t *testing.T) {
	s := SQLStore{Runs: &sql.DB{}}
	ctx := context.Background()
	if _, err := s.Candidates(ctx, 5); !errors.Is(err, ErrNoBoard) {
		t.Errorf("Candidates: %v", err)
	}
	if err := s.Claim(ctx, Run{ID: NewRunID(), CardID: "c"}); !errors.Is(err, ErrNoBoard) {
		t.Errorf("Claim: %v", err)
	}
	if err := s.MoveCard(ctx, "c", "review", KeepProgress, nil); !errors.Is(err, ErrNoBoard) {
		t.Errorf("MoveCard: %v", err)
	}
}
