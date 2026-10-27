// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package server

import (
	"context"
	"net/http"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/well"
)

// Control profiles (ADR-099 §5): which fixes each profile turns on and
// which profile each host is bound to (GET /api/v1/compliance/profiles),
// and the switch (POST /api/v1/compliance/profile, in
// compliance_profile_switch.go): an admin or operator login only, recorded
// in The Well with who and why.

type complianceFixView struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Risk     string   `json:"risk"`
	Class    string   `json:"class"`
	Controls []string `json:"controls"`
	Note     string   `json:"note,omitempty"`
}

type complianceProfileView struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Fixes       []string `json:"fixes"`
}

type complianceProfiles struct {
	Source   string                  `json:"source"`
	Reason   string                  `json:"reason,omitempty"`
	Fixes    []complianceFixView     `json:"fixes"`
	Profiles []complianceProfileView `json:"profiles"`
	Bindings []remediation.Binding   `json:"bindings"`
}

// profileStore is The Well's store when reachable, else the YAML fallback,
// with the reason.
func (c *complianceSource) profileStore(ctx context.Context) (well.Store, string, error) {
	model, err := well.LoadModel(c.root)
	if err != nil {
		return nil, "", err
	}
	why := "The Well is not configured"
	if c.config != nil {
		db, err := c.config.DB(ctx)
		if err == nil {
			s := well.NewPGStore(db, model)
			if _, err := s.Profiles(ctx); err == nil {
				return s, "", nil
			} else {
				why = "profiles: " + err.Error()
			}
		} else {
			why = err.Error()
		}
	}
	y, err := well.NewYAMLStore(c.root)
	if err != nil {
		return nil, "", err
	}
	return y, why, nil
}

func (c *complianceSource) handleProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if c == nil {
		http.Error(w, "compliance catalog not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), wellTimeout)
	defer cancel()
	store, why, err := c.profileStore(ctx)
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	ps, err := store.Profiles(ctx)
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), http.StatusInternalServerError)
		return
	}
	bs, err := store.Bindings(ctx)
	if err != nil {
		http.Error(w, "compliance: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out := complianceProfiles{Source: string(store.Source()), Reason: why, Bindings: bs}
	for _, f := range store.Model().Fixes {
		out.Fixes = append(out.Fixes, complianceFixView{ID: f.ID, Title: f.Title, Risk: f.Risk, Class: f.Class, Controls: f.Controls, Note: f.Note})
	}
	for _, p := range ps.Profiles {
		out.Profiles = append(out.Profiles, complianceProfileView{Name: p.Name, Description: p.Description, Fixes: ps.Resolve(p.Name)})
	}
	writeComplianceJSON(w, out)
}
