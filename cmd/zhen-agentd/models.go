// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"net/http"
)

// GET /api/v1/models: the default root's scripts/switch-model.sh --json
// --status, parsed (champion.ModelStatus): which key llama-server serves
// and the allowlisted keys with their documented load times. zhen-cli
// /model lists from here; a swap goes through /api/v1/tool/exec
// model_switch, never the script directly. Read-only: the script's status
// mode reads ss and /proc and takes no GPU lease.
func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	champ, err := s.pool.get(s.defaultRoot)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "champion init: %v", err)
		return
	}
	st, err := champ.ModelStatus(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}
