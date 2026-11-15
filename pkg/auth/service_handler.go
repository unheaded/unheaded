// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
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
	// AnonymousFrom are the callers that may use the routes in SourceGated
	// without a credential, judged by r.RemoteAddr alone (never a header).
	// Empty: everyone, as before (S-WEST-GROUND, ADR-147 decision 16).
	AnonymousFrom []netip.Prefix
	// SourceGated are path prefixes, matched like OpenPaths, that are open
	// (as OpenPaths, OpenReads or /health, /ready, /metrics would make them)
	// only to AnonymousFrom; any other caller needs a credential.
	SourceGated []string
}

// ParseAnonymousFrom reads AUTH_ANONYMOUS_FROM: comma-separated CIDR
// prefixes, spaces trimmed. An empty string is nil; an empty element, an
// address without a length or a length out of range is an error.
func ParseAnonymousFrom(s string) ([]netip.Prefix, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		p, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("auth: AUTH_ANONYMOUS_FROM: %w", err)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}

// remoteIn reports whether remoteAddr (host:port, as net/http fills
// r.RemoteAddr) is inside one of allow. Anything that does not parse is
// outside. An IPv4-mapped IPv6 address is judged as IPv4; a zone is dropped.
func remoteIn(remoteAddr string, allow []netip.Prefix) bool {
	ap, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return false
	}
	a := ap.Addr().Unmap().WithZone("")
	for _, p := range allow {
		if p.Contains(a) {
			return true
		}
	}
	return false
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
	if cfg.anonymousFromErr != nil {
		return nil, cfg.anonymousFromErr
	}
	audit, err := NewServiceAudit(cfg)
	if err != nil {
		return nil, err
	}
	list := append([]Authenticator(nil), routes.Authenticators...)
	a, err := configuredAuthenticator(cfg)
	switch {
	case err == nil:
		list = append(list, a)
	case errors.Is(err, ErrNoCredentials) && len(list) > 0:
		// routes.Authenticators are all this service accepts.
	default:
		return nil, err
	}
	authed := auditedAuthenticator{&MultiAuthenticator{Authenticators: list}, audit}
	authn := SkipAuthPaths(Middleware(authed), "/health", "/ready", "/metrics")
	rbacCfg := DefaultServicePolicies(cfg.ServiceName)
	if routes.RBAC != nil {
		rbacCfg = *routes.RBAC
	}
	policies := make([]EndpointPolicy, 0, len(rbacCfg.Policies)+1)
	policies = append(policies, rbacCfg.Policies...)
	policies = append(policies, EndpointPolicy{PathPrefix: "/", Methods: writeMethods,
		RequiredRoles: []string{RoleAdmin, RoleOperator, RoleService}})
	rbacCfg.Policies = policies
	if rbacCfg.AuditFunc == nil {
		rbacCfg.AuditFunc = audit
	}
	rbac := RBACMiddleware(rbacCfg)

	open := append([]string(nil), routes.OpenPaths...)
	reads := append([]string(nil), routes.OpenReads...)
	anonymous := append([]netip.Prefix(nil), routes.AnonymousFrom...)
	gated := append([]string(nil), routes.SourceGated...)
	strictAuthn := Middleware(authed) // no SkipAuthPaths: /metrics included
	return func(h http.Handler) http.Handler {
		guarded := authn(rbac(h))
		strict := strictAuthn(rbac(h))
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// A source-gated route from outside AnonymousFrom always needs a
			// credential. A non-canonical path is never opened below, so it
			// needs one anyway.
			if len(anonymous) > 0 && canonicalPath(r.URL) && underAny(r.URL.Path, gated) && !remoteIn(r.RemoteAddr, anonymous) {
				strict.ServeHTTP(w, r)
				return
			}
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
