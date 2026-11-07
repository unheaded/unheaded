// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"unheaded/pkg/auth"
)

type fixedIdentity struct{ id *auth.Identity }

func (f fixedIdentity) Authenticate(context.Context, *http.Request) (*auth.Identity, error) {
	return f.id, nil
}

func whoamiRequest(t *testing.T, id *auth.Identity, method string) *httptest.ResponseRecorder {
	t.Helper()
	var h http.Handler = http.HandlerFunc((&server{}).handleWhoami)
	if id != nil {
		h = auth.Middleware(fixedIdentity{id})(h)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/whoami", nil))
	return rec
}

// F4, H5: the web UI verifies nothing itself. It asks here who a login's
// token belongs to, so the answer is pkg/auth's and the principal is the one
// a pending confirmation binds to.
func TestWhoami_NamesTheVerifiedCaller(t *testing.T) {
	rec := whoamiRequest(t, &auth.Identity{Subject: "stevie", Roles: []string{auth.RoleOperator}}, http.MethodGet)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got whoamiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Subject != "stevie" || got.Principal != "ui:stevie" || len(got.Roles) != 1 || got.Roles[0] != auth.RoleOperator {
		t.Fatalf("whoami = %+v", got)
	}
}

// A -no-auth daemon has nobody to name, so a login through it fails.
func TestWhoami_NoIdentityIsRefused(t *testing.T) {
	if rec := whoamiRequest(t, nil, http.MethodGet); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
	if rec := whoamiRequest(t, &auth.Identity{}, http.MethodGet); rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty subject: status %d, want 401", rec.Code)
	}
}

func TestWhoami_GetOnly(t *testing.T) {
	id := &auth.Identity{Subject: "stevie"}
	if rec := whoamiRequest(t, id, http.MethodPost); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", rec.Code)
	}
}
