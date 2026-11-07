// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"unheaded/pkg/auth"
	"unheaded/pkg/zhenagent"
)

// H4. POST /api/v1/tasks/{id}/zhen-opt-in hands a card to Zhen's worker and
// DELETE takes it back. This route is the only writer of kanban_zhen_optin
// (migration 049): no card save reaches that table, and nothing on Wotan calls
// this. It records the description as the store has it now, so an edit needs a
// fresh opt-in. Only a person with an operator's role may: a service token is
// refused even when it carries one, whoever it acts for.

const zhenOptInSuffix = "/zhen-opt-in"

// ZhenOptIn is a card's kanban_zhen_optin row, as the modal shows it.
type ZhenOptIn struct {
	DescriptionSHA256 string    `json:"description_sha256"`
	By                string    `json:"by"`
	At                time.Time `json:"at"`
	// Current says the card's description is still the one opted in.
	Current bool `json:"current"`
}

// zhenOptInSource is a store with kanban_zhen_optin: only the Well has one.
type zhenOptInSource interface {
	ZhenOptInFor(ctx context.Context, cardID string) (*ZhenOptIn, error)
	SetZhenOptIn(ctx context.Context, cardID, descriptionSHA256, by string) error
	ClearZhenOptIn(ctx context.Context, cardID string) error
}

// ZhenOptInFor implements zhenOptInSource: nil, nil when nobody opted the card in.
func (p *PgStore) ZhenOptInFor(ctx context.Context, cardID string) (*ZhenOptIn, error) {
	var o ZhenOptIn
	err := p.db.QueryRowContext(ctx, `
		SELECT description_sha256, opted_in_by, opted_in_at
		  FROM kanban_zhen_optin WHERE card_id = $1`, cardID).Scan(&o.DescriptionSHA256, &o.By, &o.At)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SetZhenOptIn implements zhenOptInSource.
func (p *PgStore) SetZhenOptIn(ctx context.Context, cardID, descriptionSHA256, by string) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO kanban_zhen_optin (card_id, description_sha256, opted_in_by) VALUES ($1, $2, $3)
		ON CONFLICT (card_id) DO UPDATE SET description_sha256 = EXCLUDED.description_sha256,
		       opted_in_by = EXCLUDED.opted_in_by, opted_in_at = now()`, cardID, descriptionSHA256, by)
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
	if id == nil || id.Subject == "" || id.HasRole(auth.RoleService) {
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
	sum := zhenagent.DescriptionSHA256(task.Description)
	if err := src.SetZhenOptIn(r.Context(), cardID, sum, by); err != nil {
		log.Error().Err(err).Str("task_id", cardID).Msg("zhen opt-in: write failed")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Info().Str("task_id", cardID).Str("user", by).Msg("zhen opt-in")
	writeJSON(w, http.StatusOK, ZhenOptIn{DescriptionSHA256: sum, By: by, At: time.Now().UTC(), Current: true})
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
		o.Current = o.DescriptionSHA256 == zhenagent.DescriptionSHA256(t.Description)
	}
	return o
}
