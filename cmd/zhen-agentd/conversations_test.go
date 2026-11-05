// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
)

type fakeConvStore struct {
	client, session, model string
	turns                  []conversationTurn
}

func (f *fakeConvStore) AppendTurns(_ context.Context, client, session, model string, turns []conversationTurn) (int, error) {
	f.client, f.session, f.model, f.turns = client, session, model, turns
	return len(turns), nil
}

func TestConversations_StoresRedactedTurns(t *testing.T) {
	st := &fakeConvStore{}
	srv := memServer(t, &fakeMemStore{})
	srv.conversations = st
	body := conversationRequest{SessionID: "cli-1", Model: "qwen", Turns: []conversationTurn{
		{Role: "user", Content: "password=hunter2hunter2"},
		{Role: "assistant", Content: "fine", ElapsedMs: 12, Sources: []string{"wiki/a password=hunter2hunter2"}},
	}}
	rec := as("stevie", srv.handleConversations, http.MethodPost, "/api/v1/conversations", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out conversationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Stored != 2 || out.Redacted != 2 {
		t.Fatalf("%+v", out)
	}
	if st.client != "cli" || st.session != "cli-1" || strings.Contains(st.turns[0].Content, "hunter2") || strings.Contains(st.turns[1].Sources[0], "hunter2") {
		t.Fatalf("%+v", st)
	}
}

func TestConversations_Refusals(t *testing.T) {
	srv := memServer(t, &fakeMemStore{})
	srv.conversations = &fakeConvStore{}
	ok := conversationTurn{Role: "user", Content: "x"}
	many := make([]conversationTurn, maxConversationTurns+1)
	for i := range many {
		many[i] = ok
	}
	cases := []struct {
		name string
		req  conversationRequest
		want int
	}{
		{"no session", conversationRequest{Turns: []conversationTurn{ok}}, http.StatusBadRequest},
		{"bad session", conversationRequest{SessionID: "a b", Turns: []conversationTurn{ok}}, http.StatusBadRequest},
		{"long session", conversationRequest{SessionID: strings.Repeat("a", 65), Turns: []conversationTurn{ok}}, http.StatusBadRequest},
		{"no turns", conversationRequest{SessionID: "s"}, http.StatusBadRequest},
		{"too many", conversationRequest{SessionID: "s", Turns: many}, http.StatusBadRequest},
		{"bad role", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "system", Content: "x"}}}, http.StatusBadRequest},
		{"empty content", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user"}}}, http.StatusBadRequest},
		{"long content", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: strings.Repeat("x", maxConversationContent+1)}}}, http.StatusBadRequest},
		{"nul in content", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "a\x00b"}}}, http.StatusBadRequest},
		{"too many sources", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x", Sources: make([]string, maxConversationSources+1)}}}, http.StatusBadRequest},
		{"long source", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x", Sources: []string{strings.Repeat("s", maxConversationSourceBytes+1)}}}}, http.StatusBadRequest},
		{"nul in source", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x", Sources: []string{"a\x00"}}}}, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := as("stevie", srv.handleConversations, http.MethodPost, "/api/v1/conversations", c.req); rec.Code != c.want {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
		})
	}
	if rec := as("stevie", srv.handleConversations, http.MethodGet, "/api/v1/conversations", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", rec.Code)
	}
	srv.conversations = nil
	if rec := as("stevie", srv.handleConversations, http.MethodPost, "/api/v1/conversations", conversationRequest{SessionID: "s", Turns: []conversationTurn{ok}}); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no store: %d", rec.Code)
	}
}

// A -no-auth daemon has no principal to log the session under.
func TestConversations_AnonymousRefused(t *testing.T) {
	st := &fakeConvStore{}
	srv := memServer(t, &fakeMemStore{})
	srv.conversations = st
	body := conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x"}}}
	rec := as("", srv.handleConversations, http.MethodPost, "/api/v1/conversations", body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if st.turns != nil {
		t.Fatalf("stored for an anonymous caller: %+v", st)
	}
}

// model is free text from the caller and lands in the row, so it gets what
// content gets. Redaction runs before the clip: a secret cut in half at byte
// 64 would no longer match its pattern.
func TestConversations_ModelIsRedactedThenClipped(t *testing.T) {
	st := &fakeConvStore{}
	srv := memServer(t, &fakeMemStore{})
	srv.conversations = st
	turns := []conversationTurn{{Role: "user", Content: "x"}}
	post := func(model string) conversationResponse {
		t.Helper()
		rec := as("stevie", srv.handleConversations, http.MethodPost, "/api/v1/conversations",
			conversationRequest{SessionID: "s", Model: model, Turns: turns})
		if rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		var out conversationResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	out := post(strings.Repeat("m", 50) + " password=hunter2hunter2hunter2")
	if out.Redacted != 1 || strings.Contains(st.model, "hunter2") {
		t.Fatalf("%+v %q", out, st.model)
	}
	if len(st.model) > 64 {
		t.Fatalf("not clipped: %d bytes", len(st.model))
	}

	// Byte 64 falls inside the last rune: a cut there would leave invalid
	// UTF-8, which Postgres refuses, losing every turn in the transaction.
	post(strings.Repeat("a", 63) + "é")
	if st.model != strings.Repeat("a", 63) {
		t.Fatalf("%q", st.model)
	}
}

// elapsed_ms is INTEGER and model reaches Postgres too: either one would fail
// the transaction as a 500 and lose the whole session.
func TestConversations_RefusesWhatPostgresWouldRefuse(t *testing.T) {
	srv := memServer(t, &fakeMemStore{})
	srv.conversations = &fakeConvStore{}
	ok := conversationTurn{Role: "user", Content: "x"}
	cases := []struct {
		name string
		req  conversationRequest
	}{
		{"nul in model", conversationRequest{SessionID: "s", Model: "a\x00", Turns: []conversationTurn{ok}}},
		{"negative elapsed", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x", ElapsedMs: -1}}}},
		{"elapsed over int4", conversationRequest{SessionID: "s", Turns: []conversationTurn{{Role: "user", Content: "x", ElapsedMs: math.MaxInt32 + 1}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := as("stevie", srv.handleConversations, http.MethodPost, "/api/v1/conversations", c.req); rec.Code != http.StatusBadRequest {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
		})
	}
}
