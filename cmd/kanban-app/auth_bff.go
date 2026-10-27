// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package main

import (
	"fmt"
	"net/http"

	"unheaded/pkg/auth"
	"unheaded/pkg/bff"
)

// The board's login (ADR-103 §4, S-IRON-GATE Steps 211-216). kanban-app
// is its own backend-for-frontend: cookies are per origin and the board
// (20001) is not the dashboard (20000), so it runs pkg/bff itself against
// the same issuer (AUTH_ISSUER_URL, audience kanban-app). Reads stay public
// (The Meta Moment); a write needs an operator session cookie with
// X-Requested-With, or a service token from AUTH_ALLOW_CALLERS.

// kanbanAuthRoutes mounts /auth/auth.js always and, with auth on and an
// issuer configured, the login routes; it returns the routes for
// auth.NewServiceHandler. The BFF's eviction loop stops with the server.
func (s *Server) kanbanAuthRoutes(mux *http.ServeMux, cfg auth.ServiceAuthConfig) (auth.ServiceRoutes, error) {
	mux.HandleFunc("/auth/auth.js", bff.ServeAuthJS)
	routes := auth.ServiceRoutes{OpenReads: []string{"/"}, OpenPaths: []string{"/auth"}}
	if !cfg.Enabled {
		return routes, nil
	}
	bcfg := bff.ConfigFromEnv(cfg.ServiceName)
	if !bcfg.Enabled() {
		return routes, nil
	}
	b, err := bff.New(bcfg)
	if err != nil {
		return routes, fmt.Errorf("board login: %w", err)
	}
	go func() {
		<-s.ctx.Done()
		b.Close()
	}()
	mux.Handle("/auth/", b.Handler())
	routes.Authenticators = []auth.Authenticator{b.Authenticator()}
	return routes, nil
}
