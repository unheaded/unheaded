// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2025-2026 Steven Bellis. All rights reserved.

package main

import (
	_ "embed"
	"net/http"
)

// openapiSpec is the OpenAPI 3.0 description of the daemon's HTTP
// surface. Embedded so the binary serves it without a sidecar config.
//
// Updated whenever endpoints/shapes change. Smoke-test: pipe through
// `python3 -c "import json,sys; json.load(sys.stdin)"` after editing.
//
//go:embed openapi.json
var openapiSpec []byte

// handleOpenAPI serves the embedded spec at /api/v1/openapi.json.
// Bypasses auth (intentionally — clients need this to learn how to
// authenticate). Bypasses rate-limit only at the path level — the
// rate-limit middleware doesn't yet skip /api/v1/openapi.json, so
// this counts against the per-IP bucket.
func (s *server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openapiSpec)
}
