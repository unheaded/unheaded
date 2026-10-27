// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"unheaded/pkg/auth"
	"unheaded/pkg/bff"
)

// Auth with AUTH_ENABLED=true (ADR-103, S-IRON-GATE Phase 6): the dashboard
// is the browser's backend-for-frontend. A person logs in at /auth/login
// (pkg/bff, against AUTH_ISSUER_URL) and the browser holds only an HttpOnly
// session cookie; services and scripts present EdDSA service tokens
// (AUTH_ALLOW_CALLERS) or uhk_ API keys.
//
//   - The UI itself (pages, scripts, styles: every GET outside /api/ and
//     /ws) and /health, /ready, /metrics, security.txt, /auth/* are open.
//   - Every /api/ call and every WebSocket needs an identity. Reads take any
//     of admin, operator, observer, viewer, service; writes take admin,
//     operator or service; the compliance profile switch takes admin or
//     operator. Logs and service details are not public.
//   - A WebSocket handshake from a foreign Origin is refused (403) before
//     authentication, then the cookie (or a token) is checked.

// errAuthConfig wraps every auth configuration refusal from NewServer.
var errAuthConfig = errors.New("auth configuration")

var (
	dashboardReaders = []string{auth.RoleAdmin, auth.RoleOperator, auth.RoleObserver, auth.RoleViewer, auth.RoleService}
	dashboardWriters = []string{auth.RoleAdmin, auth.RoleOperator, auth.RoleService}
	profileSwitchers = []string{auth.RoleAdmin, auth.RoleOperator}
)

func dashboardRBAC() *auth.RBACConfig {
	writes := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	return &auth.RBACConfig{
		ServiceName: "dashboard-backend",
		Policies: []auth.EndpointPolicy{
			{PathPrefix: complianceProfileSwitchPath, RequiredRoles: profileSwitchers},
			// A query is a read sent as POST (it carries a body).
			{PathPrefix: "/api/v1/metrics/query", Methods: []string{http.MethodPost}, RequiredRoles: dashboardReaders},
			{PathPrefix: "/api/", Methods: writes, RequiredRoles: dashboardWriters},
		},
		DefaultPolicy: dashboardReaders,
	}
}

// wrapAuth puts authentication and RBAC in front of next. With auth off it
// returns next, and /auth/auth.js (mounted by the caller) finds no
// /auth/me, so the page shows no login.
func (s *Server) wrapAuth(cfg auth.ServiceAuthConfig, next http.Handler) (http.Handler, error) {
	if !cfg.Enabled {
		return next, nil
	}
	bcfg := bff.ConfigFromEnv(cfg.ServiceName)
	if s.config.DevPlaintext {
		bcfg.DevPlaintext = true
	}
	var extra []auth.Authenticator
	if bcfg.Enabled() {
		b, err := bff.New(bcfg)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errAuthConfig, err)
		}
		s.bff = b
		s.mux.Handle("/auth/", b.Handler())
		extra = append(extra, b.Authenticator())
	} else {
		s.log.Warn().Msg("auth on without AUTH_ISSUER_URL: no browser login; the UI's API calls and WebSockets need a token")
	}
	chain, err := auth.NewServiceChain(cfg, auth.ServiceRoutes{
		OpenPaths:      []string{"/auth", securityTxtPath},
		RBAC:           dashboardRBAC(),
		Authenticators: extra,
	})
	if err != nil {
		if s.bff != nil {
			s.bff.Close()
		}
		return nil, fmt.Errorf("%w: %w", errAuthConfig, err)
	}
	guarded := chain(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUIRead(r) {
			next.ServeHTTP(w, r)
			return
		}
		if isWebSocketUpgrade(r) && !s.wsServer.OriginPermitted(r) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		guarded.ServeHTTP(w, r)
	}), nil
}

// isUIRead is a GET or HEAD of the UI itself: a canonical path outside
// /api/ and /ws. A path with dot segments or escapes goes through auth.
func isUIRead(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if !canonical(r.URL) {
		return false
	}
	p := r.URL.Path
	return !(p == "/api" || strings.HasPrefix(p, "/api/") || p == "/ws" || strings.HasPrefix(p, "/ws/"))
}

func canonical(u *url.URL) bool {
	if u.RawPath != "" {
		return false
	}
	c := path.Clean(u.Path)
	return u.Path == c || u.Path == c+"/"
}

func isWebSocketUpgrade(r *http.Request) bool {
	for _, v := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(v), "upgrade") {
			return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		}
	}
	return false
}
