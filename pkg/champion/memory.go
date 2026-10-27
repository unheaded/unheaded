// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// MaxMemoryFieldBytes caps the question and the answer of one remembered
// row (RT-05's write cap, applied to memories).
const MaxMemoryFieldBytes = 16 << 10

// MemoryRecord is one zhen_memories row (db/migrations/008). Source is the
// provenance: the principal that wrote it ("ui:<subject>"), or "user" for
// rows the Python web app wrote. Score is set by Recall only.
type MemoryRecord struct {
	ID        int64     `json:"id"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Source    string    `json:"source"`
	Model     string    `json:"model,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Score     float64   `json:"score,omitempty"`
}

// MemoryStore persists remembered answers. cmd/zhen-agentd implements it
// over The Well; the CLI reaches it only through zhen-agentd (ADR-059
// Phase 2b: no database driver in the CLI).
type MemoryStore interface {
	// Remember stores one row and returns its id.
	Remember(ctx context.Context, m MemoryRecord) (int64, error)
	// Recall returns up to k rows ranked by relevance to query.
	Recall(ctx context.Context, query string, k int) ([]MemoryRecord, error)
}

// MemoryWriteResult is what memory_write returns.
type MemoryWriteResult struct {
	ID int64 `json:"id"`
}

// ErrNoMemoryStore is returned by memory_write when no store is set.
var ErrNoMemoryStore = errors.New("memory_write: no memory store configured")

// WithMemoryStore sets the store memory_write writes to.
func (c *Champion) WithMemoryStore(s MemoryStore) *Champion {
	c.memoryStore = s
	return c
}

// RememberMemory stores one memory. It is the memory_write tool's body;
// callers outside the gate (none today) would skip Rules 2 and 3, so the
// HTTP path always goes through Dispatch.
func (c *Champion) RememberMemory(ctx context.Context, m MemoryRecord) (*MemoryWriteResult, error) {
	if c.memoryStore == nil {
		return nil, ErrNoMemoryStore
	}
	switch {
	case m.Answer == "":
		return nil, errors.New("memory_write: empty answer")
	case len(m.Question) > MaxMemoryFieldBytes || len(m.Answer) > MaxMemoryFieldBytes:
		return nil, fmt.Errorf("memory_write: question and answer are capped at %d bytes each", MaxMemoryFieldBytes)
	case !utf8.ValidString(m.Question) || !utf8.ValidString(m.Answer) || !utf8.ValidString(m.Model):
		return nil, errors.New("memory_write: text is not valid UTF-8")
	}
	if len(m.Source) > 100 { // zhen_memories.source is VARCHAR(100)
		m.Source = m.Source[:100]
	}
	if len(m.Model) > 50 { // zhen_memories.model is VARCHAR(50)
		m.Model = m.Model[:50]
	}
	start := time.Now()
	actionID, err := c.logMutation(ctx, "memory_write", fmt.Sprintf("Remember answer (%d bytes) from %s", len(m.Answer), m.Source), m.Source)
	if err != nil {
		return nil, err
	}
	id, err := c.memoryStore.Remember(ctx, m)
	if err != nil {
		c.completeAction(ctx, actionID, "failed", "", err.Error(), time.Since(start))
		return nil, fmt.Errorf("memory_write: %w", err)
	}
	c.completeAction(ctx, actionID, "completed", fmt.Sprintf("memory %d", id), "", time.Since(start))
	return &MemoryWriteResult{ID: id}, nil
}

// memoryWriteCall turns memory_write's args into a record; the principal
// that made the call is its provenance.
func memoryWriteCall(tc ToolCall) (MemoryRecord, error) {
	question, err := requireString(tc.Args, "question")
	if err != nil {
		return MemoryRecord{}, err
	}
	answer, err := requireString(tc.Args, "answer")
	if err != nil {
		return MemoryRecord{}, err
	}
	model, _ := tc.Args["model"].(string)
	source := tc.Principal
	if source == "" {
		source = tc.EmittedBy
	}
	return MemoryRecord{Question: question, Answer: answer, Model: model, Source: source}, nil
}
