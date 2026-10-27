// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"

	"unheaded/pkg/auth"
	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/well"
	"unheaded/pkg/database"
)

// complianceProfileSwitchPath binds one host to a profile and mode
// (ADR-099 §5, S-IRON-GATE Step 197).
const complianceProfileSwitchPath = "/api/v1/compliance/profile"

const maxProfileSwitchBody = 4 << 10

type profileSwitchRequest struct {
	Host    string `json:"host"`
	Profile string `json:"profile"`
	Mode    string `json:"mode"`
	Reason  string `json:"reason"`
}

// complianceAdminConfig is the unheaded_config connection the switch
// writes with: config_admin (WELL_CONFIG_ADMIN_USER, ..._PASSWORD), the
// role 013_compliance_config.sql grants INSERT/UPDATE on
// compliance_host_bindings. No password, no connection: switching answers
// 503 and every read is unaffected.
func complianceAdminConfig() database.Config {
	c := well.ConfigFromEnv()
	c.User = "config_admin"
	if v, ok := os.LookupEnv("WELL_CONFIG_ADMIN_USER"); ok {
		c.User = v
	}
	c.Password = os.Getenv("WELL_CONFIG_ADMIN_PASSWORD")
	return c
}

// adminStore is The Well's compliance store as config_admin.
func (c *complianceSource) adminStore(ctx context.Context) (well.Store, error) {
	if c.writeStore != nil {
		return c.writeStore(ctx)
	}
	model, err := well.LoadModel(c.root)
	if err != nil {
		return nil, err
	}
	db, err := c.admin.DB(ctx)
	if err != nil {
		return nil, err
	}
	return well.NewPGStore(db, model), nil
}

// handleProfileSwitch: POST {host, profile, mode, reason}. The caller must
// be a logged-in admin or operator (RBAC checks the route; this checks
// again so the handler is never open, auth on or off), and its subject is
// the change's actor: The Well's trigger writes the history row (who, why,
// old and new) in the same statement. ADR-099 §5 also asks for a signed
// record; the history row is not signed yet.
func (c *complianceSource) handleProfileSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := auth.IdentityFromContext(r.Context())
	if id == nil {
		http.Error(w, "profile switching needs a login (AUTH_ENABLED=true and an operator session)", http.StatusUnauthorized)
		return
	}
	if !id.HasRole(auth.RoleAdmin) && !id.HasRole(auth.RoleOperator) {
		http.Error(w, "profile switching needs the operator or admin role", http.StatusForbidden)
		return
	}
	if c == nil {
		http.Error(w, "compliance catalog not configured", http.StatusServiceUnavailable)
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		http.Error(w, "content type must be application/json", http.StatusBadRequest)
		return
	}
	var req profileSwitchRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProfileSwitchBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "malformed request: want {host, profile, mode, reason}", http.StatusBadRequest)
		return
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		http.Error(w, "malformed request: one JSON object", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), wellTimeout)
	defer cancel()
	store, err := c.adminStore(ctx)
	if err != nil {
		http.Error(w, "profile switching needs The Well as config_admin: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	change := well.Change{Host: req.Host, Profile: req.Profile, Mode: req.Mode, Actor: id.Subject, Reason: req.Reason}
	if err := store.SetBinding(ctx, change); err != nil {
		switch {
		case errors.Is(err, well.ErrReadOnlyFallback), errors.Is(err, remediation.ErrEnforcingGate):
			http.Error(w, "compliance: "+err.Error(), http.StatusConflict)
		case errors.Is(err, well.ErrInvalidChange):
			http.Error(w, "compliance: "+err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "compliance: profile switch failed", http.StatusInternalServerError)
		}
		return
	}
	writeComplianceJSON(w, map[string]string{
		"host": change.Host, "profile": change.Profile, "mode": change.Mode, "actor": change.Actor, "source": string(store.Source()),
	})
}
