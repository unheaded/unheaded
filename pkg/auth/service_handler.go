// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"errors"
	"net/http"
	"strings"
)

// ServiceRoutes says which of a service's routes stay open with auth on,
// and which RBAC policy guards the rest (S-IRON-GATE Phase 6).
type ServiceRoutes struct {
	// OpenPaths are served without authentication for every method: the
	// exact path or anything under path + "/". /health, /ready and
	// /metrics are always open.
	OpenPaths []string
	// OpenReads are served without authentication for GET and HEAD only,
	// with the same matching; "/" opens every read. Writes under them
	// still need a token.
	OpenReads []string
	// RBAC replaces DefaultServicePolicies(cfg.ServiceName) when set.
	RBAC *RBACConfig
	// Authenticators are tried before the configured ones (pkg/bff's
	// session cookie, ADR-103 §4). With any set, auth on with nothing else
	// configured is not ErrNoCredentials.
	Authenticators []Authenticator
}

// writeMethods are what a write role guards wherever no policy is more
// specific.
var writeMethods = []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

// NewServiceHandler wraps h with a service's auth: authentication
// (NewServiceAuthenticator: EdDSA service tokens from trust.d checked
// against AUTH_ALLOW_CALLERS, JWKS user tokens, API keys), then RBAC. With
// auth off it returns h unchanged. An error means auth is on but the
// configuration is unusable; the service must refuse to start rather than
// serve open (SetupMiddleware's refuse-everything fallback is for callers
// that cannot exit).
//
// RBAC is routes.RBAC or DefaultServicePolicies, plus one closing rule: a
// write (POST, PUT, DELETE, PATCH) on any path no policy names needs
// admin, operator or service, so a read-only identity cannot write through
// a route outside /api/v1/.
func NewServiceHandler(h http.Handler, cfg ServiceAuthConfig, routes ServiceRoutes) (http.Handler, error) {
	mw, err := NewServiceChain(cfg, routes)
	if err != nil {
		return nil, err
	}
	if mw == nil {
		return h, nil
	}
	return mw(h), nil
}

// NewServiceChain is NewServiceHandler as a middleware, for callers that
// wrap their handler later: (nil, nil) with auth off, the middleware with
// auth on, or the configuration error.
func NewServiceChain(cfg ServiceAuthConfig, routes ServiceRoutes) (func(http.Handler) http.Handler, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	list := append([]Authenticator(nil), routes.Authenticators...)
	a, err := NewServiceAuthenticator(cfg)
	switch {
	case err == nil:
		list = append(list, a)
	case errors.Is(err, ErrNoCredentials) && len(list) > 0:
	default:
		return nil, err
	}
	authn := SkipAuthPaths(Middleware(&MultiAuthenticator{Authenticators: list}), "/health", "/ready", "/metrics")
	rbacCfg := DefaultServicePolicies(cfg.ServiceName)
	if routes.RBAC != nil {
		rbacCfg = *routes.RBAC
	}
	policies := make([]EndpointPolicy, 0, len(rbacCfg.Policies)+1)
	policies = append(policies, rbacCfg.Policies...)
	policies = append(policies, EndpointPolicy{PathPrefix: "/", Methods: writeMethods,
		RequiredRoles: []string{RoleAdmin, RoleOperator, RoleService}})
	rbacCfg.Policies = policies
	rbac := RBACMiddleware(rbacCfg)

	open := append([]string(nil), routes.OpenPaths...)
	reads := append([]string(nil), routes.OpenReads...)
	return func(h http.Handler) http.Handler {
		guarded := authn(rbac(h))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if canonicalPath(r.URL) {
				if underAny(r.URL.Path, open) {
					h.ServeHTTP(w, r)
					return
				}
				if (r.Method == http.MethodGet || r.Method == http.MethodHead) && underAny(r.URL.Path, reads) {
					h.ServeHTTP(w, r)
					return
				}
			}
			guarded.ServeHTTP(w, r)
		})
	}, nil
}

// underAny reports whether p is one of prefixes or below one ("/" covers
// every path).
func underAny(p string, prefixes []string) bool {
	for _, pre := range prefixes {
		if pre == "/" || p == pre || strings.HasPrefix(p, strings.TrimSuffix(pre, "/")+"/") {
			return true
		}
	}
	return false
}
