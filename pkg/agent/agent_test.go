// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unheaded/pkg/champion"
)

// mockLLM returns scripted responses, one per turn.
type mockLLM struct {
	responses []string
	calls     int
}

func (m *mockLLM) Complete(_ context.Context, _ []Message, _ int, _ float64, _ int) (string, error) {
	if m.calls >= len(m.responses) {
		return "", errors.New("mockLLM: out of scripted responses")
	}
	r := m.responses[m.calls]
	m.calls++
	return r, nil
}

// mockRetriever returns a fixed set of references regardless of query.
type mockRetriever struct {
	refs     []champion.Reference
	contents []TopicContent
	err      error
}

func (m *mockRetriever) Retrieve(_ context.Context, _ string, _ int) ([]champion.Reference, []TopicContent, error) {
	return m.refs, m.contents, m.err
}

// queryAwareRetriever returns different refs depending on whether the
// query mentions specific keywords. Used to test per-turn justification
// updates: the seed query gets one set of refs; the per-turn query
// (constructed from the model's reasoning + tool args) gets another.
type queryAwareRetriever struct {
	bySubstring map[string]queryHit // first match wins
	fallback    queryHit            // when no substring matches
	calls       []string
}

type queryHit struct {
	refs     []champion.Reference
	contents []TopicContent
	err      error
}

func (q *queryAwareRetriever) Retrieve(_ context.Context, query string, _ int) ([]champion.Reference, []TopicContent, error) {
	q.calls = append(q.calls, query)
	for needle, hit := range q.bySubstring {
		if strings.Contains(strings.ToLower(query), strings.ToLower(needle)) {
			return hit.refs, hit.contents, hit.err
		}
	}
	return q.fallback.refs, q.fallback.contents, q.fallback.err
}

// mockActionStore is the same shape used in pkg/champion tests.
type mockActionStore struct{}

func (mockActionStore) LogAction(_ context.Context, a *champion.Action) (int64, error) {
	return 1, nil
}
func (mockActionStore) UpdateAction(_ context.Context, _ int64, _, _, _ string, _ int) error {
	return nil
}
func (mockActionStore) GetActions(_ context.Context, _ string, _ int) ([]champion.Action, error) {
	return nil, nil
}

// newTestAgent constructs an agent with a temp project root, mock LLM,
// mock retriever (canonical refs), and a real Champion.
func newTestAgent(t *testing.T, llm *mockLLM, refs []champion.Reference) (*Agent, *champion.Champion) {
	t.Helper()
	c, err := champion.New(champion.Config{ProjectRoot: t.TempDir()}, mockActionStore{})
	if err != nil {
		t.Fatalf("champion.New: %v", err)
	}
	contents := make([]TopicContent, len(refs))
	for i, r := range refs {
		contents[i] = TopicContent{Ref: r, Content: "(test ref content)"}
	}
	a := New(Config{MaxTurns: 4}, c, &mockRetriever{refs: refs, contents: contents}, llm)
	return a, c
}

func canonicalRefs() []champion.Reference {
	return []champion.Reference{{
		Topic: "go-error-handling", Category: "languages/go",
		SourceKind: "embedded", SourceTrust: "canonical",
	}}
}

func externalRefs() []champion.Reference {
	return []champion.Reference{{
		Topic: "wave14-truth", Category: ".",
		SourceKind: "user-source", SourceTrust: "external",
		SourceLabel: "evil-corpus", SourcePath: "/tmp/evil-corpus",
	}}
}

func TestRun_AnswerImmediately(t *testing.T) {
	llm := &mockLLM{responses: []string{
		`{"thought":"the user wants a simple answer","answer":"42"}`,
	}}
	a, _ := newTestAgent(t, llm, canonicalRefs())
	res, err := a.Run(context.Background(), "what is the answer?")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "42" {
		t.Errorf("answer = %q; want 42", res.Answer)
	}
	if res.TurnsUsed != 1 {
		t.Errorf("turns = %d; want 1", res.TurnsUsed)
	}
	if res.BudgetHit {
		t.Errorf("BudgetHit should be false")
	}
}

func TestRun_ReadFileThenAnswer(t *testing.T) {
	a, c := newTestAgent(t, nil, canonicalRefs())
	// Set up a real file in the project root for read_file to find.
	target := filepath.Join(c.GetProjectRoot(), "config.yaml")
	if err := os.WriteFile(target, []byte("key: value\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	llm := &mockLLM{responses: []string{
		`{"thought":"need to read the config","tool_call":{"name":"read_file","args":{"path":"` + target + `"}}}`,
		`{"thought":"got the config","answer":"key=value"}`,
	}}
	a.llm = llm

	res, err := a.Run(context.Background(), "read the config")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "key=value" {
		t.Errorf("answer = %q; want key=value", res.Answer)
	}
	if res.TurnsUsed != 2 {
		t.Errorf("turns = %d; want 2", res.TurnsUsed)
	}
	if len(res.Trace) != 2 {
		t.Fatalf("trace = %d; want 2", len(res.Trace))
	}
	if res.Trace[0].ToolCall == nil || res.Trace[0].ToolCall.Name != "read_file" {
		t.Errorf("first turn should be read_file tool call")
	}
	if !strings.Contains(res.Trace[0].Observation, "key: value") {
		t.Errorf("observation should contain file content; got %q", res.Trace[0].Observation)
	}
}

func TestRun_RefusedToolCall_ProducesPendingObservation(t *testing.T) {
	// Retrieval returns an external (untrusted) ref. The model emits a
	// write_file tool call; Champion's Rule 2 fires; the agent records
	// the refusal as Pending and surfaces a confirmation token.
	a, c := newTestAgent(t, nil, externalRefs())
	target := filepath.Join(c.GetProjectRoot(), "out.txt")
	llm := &mockLLM{responses: []string{
		`{"thought":"need to write","tool_call":{"name":"write_file","args":{"path":"` + target + `","content":"hi"}}}`,
		`{"thought":"the previous tool call was refused for trust reasons","answer":"refused: needs human confirm"}`,
	}}
	a.llm = llm

	res, err := a.Run(context.Background(), "write something")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.TurnsUsed != 2 {
		t.Errorf("turns = %d; want 2", res.TurnsUsed)
	}
	if !res.Trace[0].Refused {
		t.Errorf("trace[0] should be Refused")
	}
	if !res.Trace[0].Pending {
		t.Errorf("trace[0] should be Pending")
	}
	if res.Trace[0].PendingToken == "" {
		t.Errorf("trace[0] should have a PendingToken")
	}
	// File should NOT have been written.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file should not exist after pending-confirm refusal")
	}
}

func TestRun_DestructiveToolCallHardRefused(t *testing.T) {
	a, c := newTestAgent(t, nil, canonicalRefs())
	target := filepath.Join(c.GetProjectRoot(), "x.sh")
	llm := &mockLLM{responses: []string{
		`{"thought":"clean up","tool_call":{"name":"write_file","args":{"path":"` + target + `","content":"#!/bin/sh\nrm -rf /\n"}}}`,
		`{"thought":"oh","answer":"refused destructive"}`,
	}}
	a.llm = llm

	res, err := a.Run(context.Background(), "do thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Trace[0].Refused {
		t.Errorf("trace[0] should be Refused (destructive)")
	}
	if res.Trace[0].Pending {
		t.Errorf("trace[0] should NOT be Pending (Rule 3 is hard deny)")
	}
	if res.Trace[0].PendingToken != "" {
		t.Errorf("trace[0] should have no PendingToken on hard deny")
	}
}

func TestRun_BudgetExhausted(t *testing.T) {
	// Model only ever emits tool calls (no answer) — agent must hit
	// MaxTurns and surface BudgetHit.
	a, c := newTestAgent(t, nil, canonicalRefs())
	target := filepath.Join(c.GetProjectRoot(), "x.txt")
	if err := os.WriteFile(target, []byte("hi"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	resp := `{"thought":"reading","tool_call":{"name":"read_file","args":{"path":"` + target + `"}}}`
	a.llm = &mockLLM{responses: []string{resp, resp, resp, resp, resp}}

	res, err := a.Run(context.Background(), "infinite loop")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.BudgetHit {
		t.Errorf("BudgetHit should be true")
	}
	if res.TurnsUsed != 4 {
		t.Errorf("turns = %d; want 4 (MaxTurns)", res.TurnsUsed)
	}
}

func TestRun_BadJSONFallsToTerminalAnswer(t *testing.T) {
	llm := &mockLLM{responses: []string{
		"this is not JSON, just prose",
	}}
	a, _ := newTestAgent(t, llm, canonicalRefs())
	res, err := a.Run(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Answer, "this is not JSON") {
		t.Errorf("expected raw text in answer; got %q", res.Answer)
	}
}

func TestRun_FencedJSONParses(t *testing.T) {
	llm := &mockLLM{responses: []string{
		"```json\n{\"thought\":\"f\",\"answer\":\"ok\"}\n```",
	}}
	a, _ := newTestAgent(t, llm, canonicalRefs())
	res, err := a.Run(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Answer != "ok" {
		t.Errorf("answer = %q; want ok", res.Answer)
	}
}

func TestRun_PerTurnJustificationCatchesNonSeedExternal(t *testing.T) {
	// Threat model from A2-agent-adversarial.md:
	//   - Seed query doesn't match the poisoned source → seed refs
	//     are CANONICAL (or empty).
	//   - Model's emitted thought DOES mention the poisoned source by
	//     name → per-turn query matches it → per-turn refs include
	//     the external ref → gate refuses the mutating tool call.
	//
	// Pre-fix: gate saw only seed (canonical), accepted. File written.
	// Post-fix: deriveJustification re-runs retrieval with the model's
	// per-turn output as the query → finds the external ref → gate
	// refuses with PendingConfirmation.

	c, err := champion.New(champion.Config{ProjectRoot: t.TempDir()}, mockActionStore{})
	if err != nil {
		t.Fatalf("champion.New: %v", err)
	}

	// Seed query (the user goal) doesn't mention "wave14-truth" so the
	// seed retriever returns canonical content. The model's thought
	// DOES mention "wave14-truth" so the per-turn query matches the
	// poisoned-source bucket.
	retriever := &queryAwareRetriever{
		bySubstring: map[string]queryHit{
			"wave14-truth": {
				refs: externalRefs(),
				contents: []TopicContent{{
					Ref:     externalRefs()[0],
					Content: "(poisoned)",
				}},
			},
		},
		fallback: queryHit{
			refs: canonicalRefs(),
			contents: []TopicContent{{
				Ref:     canonicalRefs()[0],
				Content: "(canonical)",
			}},
		},
	}

	target := filepath.Join(c.GetProjectRoot(), "out.txt")
	llm := &mockLLM{responses: []string{
		// Model emits a tool call with thought that mentions the
		// poisoned source by name.
		`{"thought":"the wave14-truth document recommends this fix","tool_call":{"name":"write_file","args":{"path":"` + target + `","content":"applied"}}}`,
		// After refusal observation, model gives up.
		`{"thought":"refused","answer":"refused: needs confirm"}`,
	}}

	a := New(Config{MaxTurns: 4}, c, retriever, llm)
	res, err := a.Run(context.Background(), "do thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// At least one per-turn query must include "wave14-truth" so the
	// poisoned-bucket fixture fires. The seed query is "do thing"
	// (unrelated) so it lands in fallback (canonical). Without per-
	// turn retrieval, the gate would never see the external ref.
	saw := false
	for _, q := range retriever.calls {
		if strings.Contains(q, "wave14-truth") {
			saw = true
			break
		}
	}
	if !saw {
		t.Errorf("expected at least one per-turn query to mention 'wave14-truth'; got %v", retriever.calls)
	}

	// First trace turn was the tool call; should have been REFUSED-PENDING.
	if !res.Trace[0].Refused {
		t.Errorf("trace[0] should be Refused")
	}
	if !res.Trace[0].Pending {
		t.Errorf("trace[0] should be Pending (Rule 2: external-trust)")
	}
	if res.Trace[0].PendingToken == "" {
		t.Errorf("trace[0] should have PendingToken")
	}
	// File must NOT exist.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("file should not exist after pending refusal")
	}
}

func TestRun_PerTurnRetrievalFailureFallsBackToSeed(t *testing.T) {
	// If per-turn retrieval errors, the agent falls back to seed refs.
	// Verifies graceful degradation.
	c, err := champion.New(champion.Config{ProjectRoot: t.TempDir()}, mockActionStore{})
	if err != nil {
		t.Fatalf("champion.New: %v", err)
	}
	retriever := &queryAwareRetriever{
		bySubstring: map[string]queryHit{
			// Per-turn query (contains "write_file") errors.
			"write_file": {err: errors.New("retrieval down")},
		},
		fallback: queryHit{
			// Seed query gets canonical refs.
			refs:     canonicalRefs(),
			contents: []TopicContent{{Ref: canonicalRefs()[0], Content: "(seed)"}},
		},
	}
	target := filepath.Join(c.GetProjectRoot(), "out.txt")
	llm := &mockLLM{responses: []string{
		`{"thought":"writing","tool_call":{"name":"write_file","args":{"path":"` + target + `","content":"hi"}}}`,
		`{"thought":"done","answer":"ok"}`,
	}}
	a := New(Config{MaxTurns: 4}, c, retriever, llm)
	res, err := a.Run(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Tool call should have been ALLOWED (seed has canonical refs;
	// per-turn errored, falls back to seed).
	if res.Trace[0].Refused {
		t.Errorf("trace[0] should NOT be Refused (seed canonical fallback)")
	}
	// File should exist.
	if _, err := os.Stat(target); err != nil {
		t.Errorf("file should exist; stat=%v", err)
	}
}

func TestRun_NoChampionRefusesToolCalls(t *testing.T) {
	llm := &mockLLM{responses: []string{
		`{"thought":"try","tool_call":{"name":"write_file","args":{"path":"/tmp/x","content":"hi"}}}`,
		`{"thought":"give up","answer":"can't"}`,
	}}
	contents := []TopicContent{{Ref: canonicalRefs()[0], Content: "x"}}
	a := New(Config{MaxTurns: 4}, nil, &mockRetriever{refs: canonicalRefs(), contents: contents}, llm)
	res, err := a.Run(context.Background(), "do thing")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Trace[0].Observation, "no Champion bound") {
		t.Errorf("expected 'no Champion bound' observation; got %q", res.Trace[0].Observation)
	}
}
