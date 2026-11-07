// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/zhenagent"
)

// H4. POST /api/v1/tasks/{id}/zhen-opt-in hands a card to Zhen's worker and
// DELETE takes it back. This route is the only writer of kanban_zhen_optin
// (migration 049): no card save reaches that table, and nothing on Wotan calls
// this. A POST names the card text its sender read, by zhenagent.CardSHA256, and
// is refused unless that is the text the store holds now: what is recorded is
// what the operator saw, and any later edit needs a fresh opt-in. Only a
// person's login with an operator's role may: a service token is refused even
// when it carries one, whoever it acts for, and so is an API key, which any
// program can hold.

const (
	zhenOptInSuffix  = "/zhen-opt-in"
	maxZhenOptInBody = 1 << 10
)

// ZhenOptIn is a card's kanban_zhen_optin row, as the modal shows it.
type ZhenOptIn struct {
	CardSHA256 string    `json:"card_sha256"`
	By         string    `json:"by"`
	At         time.Time `json:"at"`
	// Current says the card's title and description are still the ones opted in.
	Current bool `json:"current"`
}

// zhenOptInSource is a store with kanban_zhen_optin: only the Well has one.
type zhenOptInSource interface {
	ZhenOptInFor(ctx context.Context, cardID string) (*ZhenOptIn, error)
	SetZhenOptIn(ctx context.Context, cardID, cardSHA256, by string) error
	ClearZhenOptIn(ctx context.Context, cardID string) error
}

// ZhenOptInFor implements zhenOptInSource: nil, nil when nobody opted the card in.
func (p *PgStore) ZhenOptInFor(ctx context.Context, cardID string) (*ZhenOptIn, error) {
	var o ZhenOptIn
	err := p.db.QueryRowContext(ctx, `
		SELECT card_sha256, opted_in_by, opted_in_at
		  FROM kanban_zhen_optin WHERE card_id = $1`, cardID).Scan(&o.CardSHA256, &o.By, &o.At)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SetZhenOptIn implements zhenOptInSource.
func (p *PgStore) SetZhenOptIn(ctx context.Context, cardID, cardSHA256, by string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO kanban_zhen_optin (card_id, card_sha256, opted_in_by) VALUES ($1, $2, $3)
		ON CONFLICT (card_id) DO UPDATE SET card_sha256 = EXCLUDED.card_sha256,
		       opted_in_by = EXCLUDED.opted_in_by, opted_in_at = now()`, cardID, cardSHA256, by)
	return err
}

// ClearZhenOptIn implements zhenOptInSource.
func (p *PgStore) ClearZhenOptIn(ctx context.Context, cardID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM kanban_zhen_optin WHERE card_id = $1`, cardID)
	return err
}

// humanOperator is the person a request acts as, when they may opt a card in.
func humanOperator(r *http.Request) (string, bool) {
	id := auth.IdentityFromContext(r.Context())
	if id == nil || id.Subject == "" || id.HasRole(auth.RoleService) || id.Extra["auth_method"] == "apikey" {
		return "", false
	}
	return id.Subject, id.HasRole(auth.RoleOperator) || id.HasRole(auth.RoleAdmin)
}

func (s *Server) handleZhenOptIn(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	by, ok := humanOperator(r)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only an operator's own login opts a card in"})
		return
	}
	src, ok := s.store.(zhenOptInSource)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "this board has no Well, so no opt-in"})
		return
	}
	task, err := s.store.GetTask(cardID)
	if errors.Is(err, ErrStoreNotFound) || (err == nil && task == nil) {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		storeReadFailed(w, cardID, err)
		return
	}
	if r.Method == http.MethodDelete {
		if err := src.ClearZhenOptIn(r.Context(), cardID); err != nil {
			log.Error().Err(err).Str("task_id", cardID).Msg("zhen opt-in: withdraw failed")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		log.Info().Str("task_id", cardID).Str("user", by).Msg("zhen opt-in withdrawn")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if task.Owner != zhenagent.Owner {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the card's owner is not " + zhenagent.Owner})
		return
	}
	var read struct {
		CardSHA256 string `json:"card_sha256"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxZhenOptInBody)).Decode(&read); err != nil || read.CardSHA256 == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "send card_sha256, the card as you read it"})
		return
	}
	sum := zhenagent.CardSHA256(task.Title, task.Description)
	if read.CardSHA256 != sum {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "the card changed after you read it"})
		return
	}
	if err := src.SetZhenOptIn(r.Context(), cardID, sum, by); err != nil {
		log.Error().Err(err).Str("task_id", cardID).Msg("zhen opt-in: write failed")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info().Str("task_id", cardID).Str("user", by).Msg("zhen opt-in")
	writeJSON(w, http.StatusOK, ZhenOptIn{CardSHA256: sum, By: by, At: time.Now().UTC(), Current: true})
}

// zhenOptInFor is the card's opt-in for its detail view; nil on a board with no
// Well, with none, or when it cannot be read: the modal then offers one.
func (s *Server) zhenOptInFor(ctx context.Context, t *Task) *ZhenOptIn {
	src, ok := s.store.(zhenOptInSource)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, agentRunLookupLimit)
	defer cancel()
	o, err := src.ZhenOptInFor(ctx, t.ID)
	if err != nil {
		log.Warn().Err(err).Str("task_id", t.ID).Msg("zhen opt-in: read failed")
		return nil
	}
	if o != nil {
		o.Current = o.CardSHA256 == zhenagent.CardSHA256(t.Title, t.Description)
	}
	return o
}
