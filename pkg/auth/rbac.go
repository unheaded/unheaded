// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Standard RBAC role constants.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleObserver = "observer"
	RoleService  = "service"
	RoleGuest    = "guest"
)

// EndpointPolicy defines the auth requirements for a set of endpoints.
type EndpointPolicy struct {
	// PathPrefix is the URL prefix this policy applies to (e.g. "/api/v1/deploy").
	PathPrefix string
	// Methods restricts the policy to these HTTP methods. Empty = all methods.
	Methods []string
	// RequiredRoles lists roles that may access this endpoint (any match = allow).
	RequiredRoles []string
}

// RBACConfig defines role-based access control rules for a service.
type RBACConfig struct {
	// ServiceName identifies the service for audit/logging.
	ServiceName string
	// Policies maps endpoint patterns to required roles.
	Policies []EndpointPolicy
	// DefaultPolicy is the required role when no policy matches.
	// If empty, unauthenticated requests to unmatched paths are allowed through.
	DefaultPolicy []string
	// AuditFunc is called on every RBAC decision (optional).
	AuditFunc func(event AuditEvent)
}

// RBACMiddleware enforces role-based access control on HTTP handlers.
// It expects an Identity in the request context (set by auth Middleware).
// If no identity is present and the endpoint requires a role, returns 401.
// If the identity lacks the required role, returns 403.
func RBACMiddleware(cfg RBACConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			required := findRequiredRoles(cfg, r)
			if len(required) == 0 {
				// No role requirement — allow through.
				next.ServeHTTP(w, r)
				return
			}

			identity := IdentityFromContext(r.Context())
			if identity == nil {
				if cfg.AuditFunc != nil {
					cfg.AuditFunc(AuditEvent{
						EventType: AuditRBACDenied,
						Service:   cfg.ServiceName,
						Endpoint:  r.URL.Path,
						Method:    r.Method,
						Result:    "denied",
						Reason:    "no identity in context",
					})
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !identityHasAnyRole(identity, required) {
				if cfg.AuditFunc != nil {
					cfg.AuditFunc(AuditEvent{
						EventType: AuditRBACDenied,
						Service:   cfg.ServiceName,
						Endpoint:  r.URL.Path,
						Method:    r.Method,
						Subject:   identity.Subject,
						Result:    "denied",
						Reason:    "insufficient role",
					})
				}
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			if cfg.AuditFunc != nil {
				cfg.AuditFunc(AuditEvent{
					EventType: AuditRBACAllowed,
					Service:   cfg.ServiceName,
					Endpoint:  r.URL.Path,
					Method:    r.Method,
					Subject:   identity.Subject,
					Result:    "allowed",
				})
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns a middleware that requires the given role.
// Simpler than full RBAC policy for single-endpoint protection.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := IdentityFromContext(r.Context())
			if identity == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if !identityHasAnyRole(identity, roles) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// DefaultServicePolicies returns a standard RBAC policy set for internal services.
// Public: /health, /ready, /metrics (guest)
// Admin: /api/v1/admin/* (admin only)
// Operator: /api/v1/deploy*, /api/v1/config* (operator+admin)
// Observer: /api/v1/* read-only (observer+operator+admin)
// Service: inter-service endpoints (service role)
func DefaultServicePolicies(serviceName string) RBACConfig {
	return RBACConfig{
		ServiceName: serviceName,
		Policies: []EndpointPolicy{
			{PathPrefix: "/health", RequiredRoles: nil},
			{PathPrefix: "/ready", RequiredRoles: nil},
			{PathPrefix: "/metrics", RequiredRoles: nil},
			{PathPrefix: "/api/v1/admin", RequiredRoles: []string{RoleAdmin}},
			{PathPrefix: "/api/v1/deploy", RequiredRoles: []string{RoleAdmin, RoleOperator}},
			{PathPrefix: "/api/v1/config", RequiredRoles: []string{RoleAdmin, RoleOperator}},
			{PathPrefix: "/api/v1/", Methods: []string{"POST", "PUT", "DELETE", "PATCH"},
				RequiredRoles: []string{RoleAdmin, RoleOperator, RoleService}},
			{PathPrefix: "/api/v1/", Methods: []string{"GET"},
				RequiredRoles: []string{RoleAdmin, RoleOperator, RoleObserver, RoleService}},
		},
		DefaultPolicy: []string{RoleAdmin, RoleOperator, RoleObserver, RoleService},
	}
}

func findRequiredRoles(cfg RBACConfig, r *http.Request) []string {
	for _, p := range cfg.Policies {
		if !strings.HasPrefix(r.URL.Path, p.PathPrefix) {
			continue
		}
		if len(p.Methods) > 0 && !containsMethod(p.Methods, r.Method) {
			continue
		}
		return p.RequiredRoles
	}
	return cfg.DefaultPolicy
}

func containsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func identityHasAnyRole(id *Identity, roles []string) bool {
	for _, required := range roles {
		if id.HasRole(required) {
			return true
		}
	}
	return false
}

// RoleRegistry maintains a mapping of known roles and their descriptions.
type RoleRegistry struct {
	mu    sync.RWMutex
	roles map[string]RoleDefinition
}

// RoleDefinition describes a role.
type RoleDefinition struct {
	Name        string
	Description string
}

// NewRoleRegistry returns a registry pre-populated with the standard roles.
func NewRoleRegistry() *RoleRegistry {
	r := &RoleRegistry{roles: make(map[string]RoleDefinition)}
	r.Register(RoleDefinition{RoleAdmin, "Full access to all operations"})
	r.Register(RoleDefinition{RoleOperator, "Deploy, scale, restart (no secrets)"})
	r.Register(RoleDefinition{RoleObserver, "Read-only access (logs, metrics, state)"})
	r.Register(RoleDefinition{RoleService, "Service-to-service operations"})
	r.Register(RoleDefinition{RoleGuest, "Public endpoints only (health, status)"})
	return r
}

// Register adds or updates a role definition.
func (r *RoleRegistry) Register(def RoleDefinition) {
	r.mu.Lock()
	r.roles[def.Name] = def
	r.mu.Unlock()
}

// Get returns a role definition by name.
func (r *RoleRegistry) Get(name string) (RoleDefinition, bool) {
	r.mu.RLock()
	def, ok := r.roles[name]
	r.mu.RUnlock()
	return def, ok
}

// List returns all registered role definitions.
func (r *RoleRegistry) List() []RoleDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]RoleDefinition, 0, len(r.roles))
	for _, d := range r.roles {
		defs = append(defs, d)
	}
	return defs
}

// ContextWithRole adds a role to the identity in context (for testing / internal use).
func ContextWithRole(ctx context.Context, role string) context.Context {
	id := IdentityFromContext(ctx)
	if id == nil {
		id = &Identity{Subject: "internal", Roles: []string{role}}
	} else {
		// Clone to avoid mutating shared identity.
		clone := *id
		clone.Roles = append(append([]string{}, id.Roles...), role)
		id = &clone
	}
	return context.WithValue(ctx, identityKey, id)
}
