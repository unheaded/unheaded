// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	_ "embed"
	"net/http"
)

// securityTxt is the RFC 9116 file served at /.well-known/security.txt.
// Embedded here (not in static/) so the route works whatever StaticFS the
// server was given. TestSecurityTxt_NotExpired fails before Expires passes.
//
//go:embed security.txt
var securityTxt string

// securityTxtPath is public by definition: NewServer skips it in the auth
// middleware.
const securityTxtPath = "/.well-known/security.txt"

// handleSecurityTxt serves the embedded file to GET and HEAD; anything else
// is 405 (checked here: the "/" catch-all would otherwise take other methods).
func handleSecurityTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(securityTxt)) // #nosec G104 -- client went away; nothing to recover
}
