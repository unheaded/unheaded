// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRBACMiddleware_NoPolicy(t *testing.T) {
	cfg := RBACConfig{ServiceName: "test"}
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/anything", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRBACMiddleware_RequiresRole_NoIdentity(t *testing.T) {
	cfg := RBACConfig{
		ServiceName:   "test",
		DefaultPolicy: []string{RoleAdmin},
	}
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRBACMiddleware_RequiresRole_WrongRole(t *testing.T) {
	cfg := RBACConfig{
		ServiceName:   "test",
		DefaultPolicy: []string{RoleAdmin},
	}
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	ctx := context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "observer-user",
		Roles:   []string{RoleObserver},
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRBACMiddleware_RequiresRole_CorrectRole(t *testing.T) {
	cfg := RBACConfig{
		ServiceName:   "test",
		DefaultPolicy: []string{RoleAdmin},
	}
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	ctx := context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "admin-user",
		Roles:   []string{RoleAdmin},
	})
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRBACMiddleware_PolicyMatching(t *testing.T) {
	cfg := DefaultServicePolicies("test")
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Health endpoint — no auth required.
	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/health: expected 200, got %d", rr.Code)
	}

	// Admin endpoint — requires admin.
	req = httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	ctx := context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "observer",
		Roles:   []string{RoleObserver},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("/api/v1/admin as observer: expected 403, got %d", rr.Code)
	}

	// Admin endpoint — admin gets through.
	req = httptest.NewRequest("GET", "/api/v1/admin/users", nil)
	ctx = context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "admin",
		Roles:   []string{RoleAdmin},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/api/v1/admin as admin: expected 200, got %d", rr.Code)
	}

	// GET /api/v1/data — observer allowed.
	req = httptest.NewRequest("GET", "/api/v1/data", nil)
	ctx = context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "observer",
		Roles:   []string{RoleObserver},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("GET /api/v1/data as observer: expected 200, got %d", rr.Code)
	}

	// POST /api/v1/data — observer denied (needs operator+).
	req = httptest.NewRequest("POST", "/api/v1/data", nil)
	ctx = context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "observer",
		Roles:   []string{RoleObserver},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("POST /api/v1/data as observer: expected 403, got %d", rr.Code)
	}
}

func TestRequireRole(t *testing.T) {
	mw := RequireRole(RoleAdmin, RoleOperator)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No identity → 401.
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no identity: expected 401, got %d", rr.Code)
	}

	// Wrong role → 403.
	req = httptest.NewRequest("GET", "/", nil)
	ctx := context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "guest",
		Roles:   []string{RoleGuest},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("guest role: expected 403, got %d", rr.Code)
	}

	// Correct role → 200.
	req = httptest.NewRequest("GET", "/", nil)
	ctx = context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "operator",
		Roles:   []string{RoleOperator},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("operator role: expected 200, got %d", rr.Code)
	}
}

func TestRoleRegistry(t *testing.T) {
	reg := NewRoleRegistry()

	roles := reg.List()
	if len(roles) != 5 {
		t.Errorf("expected 5 default roles, got %d", len(roles))
	}

	def, ok := reg.Get(RoleAdmin)
	if !ok || def.Name != RoleAdmin {
		t.Errorf("admin role not found")
	}

	reg.Register(RoleDefinition{Name: "custom", Description: "custom role"})
	if _, ok := reg.Get("custom"); !ok {
		t.Error("custom role not registered")
	}
}

func TestContextWithRole(t *testing.T) {
	ctx := context.Background()

	// No existing identity.
	ctx = ContextWithRole(ctx, RoleAdmin)
	id := IdentityFromContext(ctx)
	if id == nil || !id.HasRole(RoleAdmin) {
		t.Error("expected admin role in new identity")
	}

	// Existing identity — should clone and add.
	ctx = ContextWithRole(ctx, RoleOperator)
	id = IdentityFromContext(ctx)
	if !id.HasRole(RoleAdmin) || !id.HasRole(RoleOperator) {
		t.Error("expected both admin and operator roles")
	}
}

func TestRBACMiddleware_AuditFunc(t *testing.T) {
	var events []AuditEvent
	cfg := RBACConfig{
		ServiceName:   "test",
		DefaultPolicy: []string{RoleAdmin},
		AuditFunc:     func(e AuditEvent) { events = append(events, e) },
	}
	mw := RBACMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Denied (no identity).
	req := httptest.NewRequest("GET", "/api/v1/data", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if len(events) != 1 || events[0].EventType != AuditRBACDenied {
		t.Errorf("expected 1 denied event, got %d events", len(events))
	}

	// Allowed (admin).
	events = nil
	req = httptest.NewRequest("GET", "/api/v1/data", nil)
	ctx := context.WithValue(req.Context(), identityKey, &Identity{
		Subject: "admin",
		Roles:   []string{RoleAdmin},
	})
	req = req.WithContext(ctx)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if len(events) != 1 || events[0].EventType != AuditRBACAllowed {
		t.Errorf("expected 1 allowed event, got %d events", len(events))
	}
}
