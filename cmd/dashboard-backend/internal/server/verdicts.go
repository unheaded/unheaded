// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package server

import (
	"net/http"

	"unheaded/cmd/dashboard-backend/internal/verdicts"
)

// handleVerdicts serves the third-party inventory with its verdicts
// (GET /api/v1/verdicts, the /verdicts page; ADR-113, read-only first step,
// Stevie 2026-11-12). The record is a dated file in git and the copy
// embedded in this binary is checked against it by a test, so the answer
// is the same for every reader and nothing here can be written to. Like
// every /api/ read it needs a login when auth is on.
func (s *Server) handleVerdicts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(verdicts.JSON())
}
