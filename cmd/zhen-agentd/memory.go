// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"unheaded/pkg/champion"
)

// Memory endpoints (ADR-059 Phase 2b, S-CHAMPION-HARDENING Steps 208-209).
// zhen-cli's /recall and /remember come here: zhen-agentd holds The Well's
// DSN, the CLI holds no database driver.
//
//	GET  /api/v1/memory/recall?q=<text>&k=<1..20>  → {"memories":[MemoryRecord...]}
//	POST /api/v1/memory/remember {"question","answer","model","emitted_by"}
//	     → the /api/v1/tool/exec response shapes (memory_write through the gate)
//
// Recall is a read and needs only the listener's authentication. Remember
// is the mutating memory_write tool: an authenticated caller typed it, so
// it carries a direct-user justification (Rule 2's escape) and Rule 3 still
// refuses destructive verbs in the text. A request with no authenticated
// principal (the -no-auth daemon) is refused: a token issued to it could
// never be confirmed (RT-03).

const (
	maxRecallQueryBytes = 512
	defaultRecallK      = 5
	maxRecallK          = 20
)

type memoryRecallResponse struct {
	Memories []champion.MemoryRecord `json:"memories"`
}

type memoryRememberRequest struct {
	Question  string `json:"question"`
	Answer    string `json:"answer"`
	Model     string `json:"model,omitempty"`
	EmittedBy string `json:"emitted_by,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

func (s *server) handleMemoryRecall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.memory == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "no memory store configured (start zhen-agentd with -memory-store pg)")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > maxRecallQueryBytes {
		writeJSONError(w, http.StatusBadRequest, "q must be 1-%d bytes", maxRecallQueryBytes)
		return
	}
	k := defaultRecallK
	if v := r.URL.Query().Get("k"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxRecallK {
			writeJSONError(w, http.StatusBadRequest, "k must be 1-%d", maxRecallK)
			return
		}
		k = n
	}
	rows, err := s.memory.Recall(r.Context(), q, k)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "recall: %v", err)
		return
	}
	if len(rows) > k {
		rows = rows[:k]
	}
	if rows == nil {
		rows = []champion.MemoryRecord{}
	}
	writeJSON(w, http.StatusOK, memoryRecallResponse{Memories: rows})
}

func (s *server) handleMemoryRemember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	if s.memory == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "no memory store configured (start zhen-agentd with -memory-store pg)")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16)) // 64 KiB, as /tool/exec
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "read body: %v", err)
		return
	}
	defer r.Body.Close()
	var req memoryRememberRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "decode body: %v", err)
		return
	}
	if strings.TrimSpace(req.Answer) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing %q", "answer")
		return
	}
	principal := requestPrincipal(r)
	if principal == anonymousPrincipal {
		writeJSON(w, http.StatusForbidden, toolExecResponse{
			Status: "denied",
			Reason: "remember needs an authenticated principal (this daemon serves without auth)",
		})
		return
	}
	emittedBy := strings.TrimSpace(req.EmittedBy)
	if emittedBy == "" || isAgentLoop(emittedBy) {
		// A model loop is never the direct user; this endpoint is for
		// people, so the label is only for the audit trail.
		emittedBy = "direct"
	}
	if req.SessionID == "" {
		req.SessionID = fmt.Sprintf("remember-%s", time.Now().UTC().Format("20060102T150405Z"))
	}
	champ, err := s.pool.get(s.defaultRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}
	tc := champion.ToolCall{
		Name: "memory_write",
		Args: map[string]any{"question": req.Question, "answer": req.Answer, "model": req.Model},
		Justification: []champion.Reference{{
			Topic: "user-typed-remember", Category: "user-action", SourceKind: "user-action",
			SourceTrust: "direct-user", SourceLabel: emittedBy,
		}},
		EmittedBy: emittedBy,
		Principal: principal,
	}
	s.dispatchAndRespond(w, r, champ, tc, req.SessionID)
}

// pgMemoryStore is champion.MemoryStore over unheaded_app.zhen_memories
// (db/migrations/008), as app_zhen. Recall ranks with Postgres full-text
// search over question and answer: the Python web app's embedding recall
// needs its embedder, which zhen-agentd does not load.
type pgMemoryStore struct {
	db *sql.DB
}

func newPGMemoryStore(ctx context.Context, db *sql.DB) (*pgMemoryStore, error) {
	// The Well's migrations own the table (ADR-101); check its shape only.
	if _, err := db.ExecContext(ctx, `SELECT id, question, answer, source, model, created_at FROM zhen_memories LIMIT 0`); err != nil {
		return nil, fmt.Errorf("zhen_memories missing or incomplete (run well-migrate): %w", err)
	}
	return &pgMemoryStore{db: db}, nil
}

func (p *pgMemoryStore) Remember(ctx context.Context, m champion.MemoryRecord) (int64, error) {
	var id int64
	err := p.db.QueryRowContext(ctx,
		`INSERT INTO zhen_memories (question, answer, source, model) VALUES ($1, $2, $3, NULLIF($4, '')) RETURNING id`,
		m.Question, m.Answer, m.Source, m.Model).Scan(&id)
	return id, err
}

func (p *pgMemoryStore) Recall(ctx context.Context, query string, k int) ([]champion.MemoryRecord, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, question, answer, COALESCE(source, ''), COALESCE(model, ''), created_at,
		       ts_rank(to_tsvector('english', question || ' ' || answer), plainto_tsquery('english', $1)) AS score
		  FROM zhen_memories
		 WHERE to_tsvector('english', question || ' ' || answer) @@ plainto_tsquery('english', $1)
		 ORDER BY score DESC, created_at DESC
		 LIMIT $2`, query, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []champion.MemoryRecord
	for rows.Next() {
		var m champion.MemoryRecord
		var score float32
		if err := rows.Scan(&m.ID, &m.Question, &m.Answer, &m.Source, &m.Model, &m.CreatedAt, &score); err != nil {
			return nil, err
		}
		m.Score = float64(score)
		out = append(out, m)
	}
	return out, rows.Err()
}

// buildMemoryStore selects the memory backend per -memory-store: "none"
// (default; /recall and /remember answer 503) or "pg" (WELL_APP_ZHEN_DSN,
// unheaded_app as app_zhen). Asked for and unreachable fails startup.
func buildMemoryStore(ctx context.Context, kind string) (champion.MemoryStore, func(), error) {
	switch kind {
	case "none", "":
		return nil, func() {}, nil
	case "pg":
	default:
		return nil, func() {}, fmt.Errorf("unknown memory-store %q (supported: none, pg)", kind)
	}
	dsn := strings.TrimSpace(os.Getenv("WELL_APP_ZHEN_DSN"))
	if dsn == "" {
		return nil, func() {}, fmt.Errorf("-memory-store pg needs WELL_APP_ZHEN_DSN (unheaded_app as app_zhen)")
	}
	warnIfInsecureDSN(dsn)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, func() {}, err
	}
	db.SetMaxOpenConns(2)
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pctx); err != nil {
		_ = db.Close()
		return nil, func() {}, fmt.Errorf("the Well is unreachable: %w", err)
	}
	st, err := newPGMemoryStore(pctx, db)
	if err != nil {
		_ = db.Close()
		return nil, func() {}, err
	}
	return st, func() { _ = db.Close() }, nil
}
