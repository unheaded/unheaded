// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeMemoryStore is an in-memory MemoryStore.
type fakeMemoryStore struct {
	mu   sync.Mutex
	rows []MemoryRecord
}

func (f *fakeMemoryStore) Remember(_ context.Context, m MemoryRecord) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.ID = int64(len(f.rows) + 1)
	f.rows = append(f.rows, m)
	return m.ID, nil
}

func (f *fakeMemoryStore) Recall(context.Context, string, int) ([]MemoryRecord, error) {
	return nil, nil
}

// S-CHAMPION-HARDENING Step 209: memory_write is a mutating tool. Through
// Dispatch it needs a trusted justification (Rule 2), and a destructive verb
// in the text is refused like any other argument (Rule 3).
func TestMemoryWrite_IsMutatingAndDispatched(t *testing.T) {
	if !MutatingTools["memory_write"] || ReadOnlyTools["memory_write"] {
		t.Fatal("memory_write must be registered mutating")
	}
	c, store := newTestChampion(t)
	mem := &fakeMemoryStore{}
	c.WithMemoryStore(mem)
	args := map[string]any{"question": "how do I list runbooks?", "answer": "/runbook list", "model": "qwen-7b"}

	// No justification: pending confirmation, nothing stored.
	_, err := c.Dispatch(context.Background(), ToolCall{Name: "memory_write", Args: args, Principal: "ui:stevie"})
	var ge *GateError
	if !errors.As(err, &ge) || !ge.PendingConfirmation() {
		t.Fatalf("unjustified memory_write: %v, want pending confirmation", err)
	}
	if len(mem.rows) != 0 {
		t.Fatalf("stored %d rows before confirmation", len(mem.rows))
	}

	// direct-user: stored, with the principal as provenance.
	out, err := c.Dispatch(context.Background(), ToolCall{Name: "memory_write", Args: args, Principal: "ui:stevie",
		Justification: []Reference{{SourceTrust: "direct-user"}}})
	if err != nil {
		t.Fatalf("direct-user memory_write: %v", err)
	}
	res, ok := out.(*MemoryWriteResult)
	if !ok || res.ID != 1 {
		t.Fatalf("result %#v, want *MemoryWriteResult{ID:1}", out)
	}
	if got := mem.rows[0]; got.Source != "ui:stevie" || got.Question != args["question"] || got.Model != "qwen-7b" {
		t.Fatalf("row %+v", got)
	}
	found := false
	for _, a := range store.actions {
		if a.ActionType == "memory_write" {
			found = true
		}
	}
	if !found {
		t.Error("no memory_write audit row")
	}

	// Rule 3 applies to the remembered text.
	_, err = c.Dispatch(context.Background(), ToolCall{Name: "memory_write", Principal: "ui:stevie",
		Args:          map[string]any{"question": "cleanup", "answer": "run rm -rf / to clean up"},
		Justification: []Reference{{SourceTrust: "direct-user"}}})
	if err == nil || !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("destructive memory: %v, want refusal", err)
	}
}

func TestMemoryWrite_ConfirmedTokenStores(t *testing.T) {
	c, _ := newTestChampion(t)
	mem := &fakeMemoryStore{}
	c.WithMemoryStore(mem)
	tc := ToolCall{Name: "memory_write", Principal: "ui:stevie",
		Args: map[string]any{"question": "q", "answer": "a"}}
	tok, err := c.IssuePendingConfirmation(tc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ConfirmPendingToolCall(context.Background(), tok, "ui:stevie"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(mem.rows) != 1 || mem.rows[0].Source != "ui:stevie" {
		t.Fatalf("rows %+v", mem.rows)
	}
}

func TestMemoryWrite_Refusals(t *testing.T) {
	ctx := context.Background()
	c, _ := newTestChampion(t)
	if _, err := c.RememberMemory(ctx, MemoryRecord{Question: "q", Answer: "a"}); err == nil {
		t.Error("no memory store: want an error")
	}
	c.WithMemoryStore(&fakeMemoryStore{})
	for name, m := range map[string]MemoryRecord{
		"empty answer":     {Question: "q"},
		"answer too big":   {Question: "q", Answer: strings.Repeat("a", MaxMemoryFieldBytes+1)},
		"question too big": {Question: strings.Repeat("q", MaxMemoryFieldBytes+1), Answer: "a"},
		"invalid utf8":     {Question: "q", Answer: "\xff\xfe"},
	} {
		if _, err := c.RememberMemory(ctx, m); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
