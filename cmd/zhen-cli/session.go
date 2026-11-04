// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S-ZHEN-POLISH Phase 11 (DP-311): the [memory hit] line and session saving.
// The hit line is a side channel (ADR-059 T1): it is printed before the model
// call and never reaches the prompt. A session is the interactive REPL only.

const (
	maxSavedContent   = 32 << 10
	maxSavedTurns     = 50
	memoryHintTimeout = 2 * time.Second
	saveTimeout       = 10 * time.Second
)

type savedTurn struct {
	Role      string   `json:"role"`
	Content   string   `json:"content"`
	Sources   []string `json:"sources,omitempty"`
	ElapsedMs int      `json:"elapsed_ms,omitempty"`
}

// newSessionID returns "cli-20261104T120000Z-" plus 8 hex from crypto/rand.
func newSessionID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return "cli-" + time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

// memoryHint prints "[memory hit: <score>]" for the best recalled row. Any
// failure, a non-200 or no rows is silent.
func (s *session) memoryHint(q string) {
	if !s.memoryRecall {
		return
	}
	if len(q) > 512 {
		q = strings.ToValidUTF8(q[:512], "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), memoryHintTimeout)
	defer cancel()
	var resp struct {
		Memories []memoryRow `json:"memories"`
	}
	code, err := s.agentdJSON(ctx, s.client, http.MethodGet, "/api/v1/memory/recall?q="+url.QueryEscape(q)+"&k=1", nil, &resp)
	if err != nil || code != http.StatusOK || len(resp.Memories) == 0 {
		return
	}
	s.dim(fmt.Sprintf("[memory hit: %.2f]", resp.Memories[0].Score))
}

// logTurn records one turn for persistSession. No-op unless s.persist.
func (s *session) logTurn(role, content string, sources []string, elapsedMs int) {
	if !s.persist {
		return
	}
	if len(content) > maxSavedContent {
		content = strings.ToValidUTF8(content[:maxSavedContent], "")
	}
	s.saved = append(s.saved, savedTurn{Role: role, Content: content, Sources: sources, ElapsedMs: elapsedMs})
}

// persistSession posts the saved turns once, in batches of maxSavedTurns.
// A failure is one dim line; it never changes the exit status.
func (s *session) persistSession() {
	s.saveOnce.Do(func() {
		if !s.persist || len(s.saved) == 0 {
			return
		}
		for i := 0; i < len(s.saved); i += maxSavedTurns {
			end := i + maxSavedTurns
			if end > len(s.saved) {
				end = len(s.saved)
			}
			body := map[string]any{"session_id": s.sessionID, "model": s.model, "turns": s.saved[i:end]}
			ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
			var resp map[string]any
			code, err := s.agentdJSON(ctx, s.client, http.MethodPost, "/api/v1/conversations", body, &resp)
			cancel()
			if err != nil {
				s.dim("zhen-cli: session not saved: " + err.Error())
				return
			}
			if code != http.StatusOK {
				s.dim("zhen-cli: session not saved: status " + http.StatusText(code))
				return
			}
		}
	})
}
