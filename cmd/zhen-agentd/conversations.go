// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"unheaded/pkg/zhencapture"
)

// POST /api/v1/conversations: zhen-cli saves a finished session (DP-311,
// ADR-059 Q3). The CLI has no database driver; this does what /remember
// does for one answer. Text is redacted here, once, for every caller, and
// this is not a Champion tool: it changes no Kingdom state, it keeps a log.
const (
	maxConversationTurns       = 50
	maxConversationContent     = 32 << 10
	maxConversationBody        = 2 << 20
	maxConversationSources     = 20
	maxConversationSourceBytes = 256
	maxConversationModel       = 64 // 010's VARCHAR(64), the tighter of 003 and 010
)

var conversationSessionRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type conversationTurn struct {
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Sources   []string `json:"sources,omitempty"`
	ElapsedMs int      `json:"elapsed_ms,omitempty"`
}

type conversationRequest struct {
	SessionID string             `json:"session_id"`
	Model     string             `json:"model,omitempty"`
	Turns     []conversationTurn `json:"turns"`
}

type conversationResponse struct {
	Stored   int `json:"stored"`
	Redacted int `json:"redacted"`
}

type conversationStore interface {
	AppendTurns(ctx context.Context, client, session, model string, turns []conversationTurn) (int, error)
}

func (s *server) handleConversations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.conversations == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "no session store (needs -memory-store pg and migration 045)")
		return
	}
	if requestPrincipal(r) == anonymousPrincipal {
		writeJSON(w, http.StatusForbidden, toolExecResponse{
			Status: "denied",
			Reason: "saving a session needs an authenticated principal",
		})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxConversationBody))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	var req conversationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	if err := req.validate(); err != nil {
		writeJSONError(w, http.StatusBadRequest, "%v", err)
		return
	}

	redacted := 0
	redact := func(s string) string {
		out, n := zhencapture.Redact(s)
		redacted += n
		return out
	}
	for i := range req.Turns {
		t := &req.Turns[i]
		t.Content = redact(t.Content)
		for j := range t.Sources {
			t.Sources[j] = redact(t.Sources[j])
		}
	}
	// Redact first: a secret cut in half by the clip would stop matching.
	req.Model = clipModel(redact(req.Model))

	n, err := s.conversations.AppendTurns(r.Context(), "cli", req.SessionID, req.Model, req.Turns)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "store session: %v", err)
		return
	}
	writeJSON(w, http.StatusOK, conversationResponse{Stored: n, Redacted: redacted})
}

// validate refuses what the schema or Postgres would refuse, here, as a 400:
// inside AppendTurns' transaction one bad turn would lose the whole session.
func (c *conversationRequest) validate() error {
	if !conversationSessionRE.MatchString(c.SessionID) {
		return fmt.Errorf("session_id must match %s", conversationSessionRE)
	}
	if len(c.Turns) == 0 || len(c.Turns) > maxConversationTurns {
		return fmt.Errorf("turns must be 1-%d", maxConversationTurns)
	}
	if !storableText(c.Model) {
		return fmt.Errorf("model must be UTF-8 without NUL")
	}
	for i, t := range c.Turns {
		switch {
		case t.Role != "user" && t.Role != "assistant":
			return fmt.Errorf("turn %d: role must be user or assistant", i)
		case t.Content == "" || len(t.Content) > maxConversationContent:
			return fmt.Errorf("turn %d: content must be 1-%d bytes", i, maxConversationContent)
		case !storableText(t.Content):
			return fmt.Errorf("turn %d: content must be UTF-8 without NUL", i)
		case len(t.Sources) > maxConversationSources:
			return fmt.Errorf("turn %d: at most %d sources", i, maxConversationSources)
		case t.ElapsedMs < 0 || t.ElapsedMs > math.MaxInt32:
			return fmt.Errorf("turn %d: elapsed_ms out of range", i)
		}
		for _, src := range t.Sources {
			if len(src) > maxConversationSourceBytes || !storableText(src) {
				return fmt.Errorf("turn %d: a source must be at most %d bytes of UTF-8 without NUL", i, maxConversationSourceBytes)
			}
		}
	}
	return nil
}

// storableText reports whether Postgres TEXT and JSONB can hold s.
func storableText(s string) bool {
	return utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

// clipModel cuts s to maxConversationModel bytes on a rune boundary.
func clipModel(s string) string {
	if len(s) <= maxConversationModel {
		return s
	}
	return strings.ToValidUTF8(s[:maxConversationModel], "")
}

// AppendTurns writes a session in one transaction: a half-saved session
// would read as a finished one.
func (p *pgMemoryStore) AppendTurns(ctx context.Context, client, session, model string, turns []conversationTurn) (int, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, t := range turns {
		sources := t.Sources
		if sources == nil {
			sources = []string{}
		}
		raw, err := json.Marshal(sources)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO zhen_conversations (session_id, role, content, sources, model, elapsed_ms, client)
			 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)`,
			session, t.Role, t.Content, string(raw), model, t.ElapsedMs, client); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(turns), nil
}

// conversationsReady probes for migration 045 once at startup; without the
// client column session saving stays off and the endpoint answers 503.
func (p *pgMemoryStore) conversationsReady(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx, `SELECT client FROM zhen_conversations LIMIT 0`)
	return err
}
