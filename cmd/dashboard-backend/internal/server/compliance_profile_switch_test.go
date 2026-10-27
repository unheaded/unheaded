// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.

package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"unheaded/pkg/compliance/remediation"
	"unheaded/pkg/compliance/well"
)

// The compliance profile switch (ADR-099 §5, S-IRON-GATE Step 197): an
// operator login may bind a host to a profile, and the Well row names who.

type recordingStore struct {
	mu      sync.Mutex
	changes []well.Change
	err     error
}

func (s *recordingStore) Source() well.Source                                     { return well.SourceWell }
func (s *recordingStore) Model() *remediation.Model                               { return nil }
func (s *recordingStore) Bindings(context.Context) ([]remediation.Binding, error) { return nil, nil }
func (s *recordingStore) Profiles(context.Context) (*remediation.ProfileSet, error) {
	return &remediation.ProfileSet{}, nil
}
func (s *recordingStore) SetBinding(_ context.Context, c well.Change) error {
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changes = append(s.changes, c)
	return nil
}

const switchBody = `{"host":"west","profile":"lab","mode":"audit","reason":"lab baseline after review"}`

func TestProfileSwitch_OperatorOnlyAndRecordsTheActor(t *testing.T) {
	e := newBFFEnv(t, nil)
	st := &recordingStore{}
	e.srv.compliance.writeStore = func(context.Context) (well.Store, error) { return st, nil }

	if code := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody, xrw).Code; code != http.StatusUnauthorized {
		t.Fatalf("anonymous switch: %d, want 401", code)
	}
	viewer := e.login(t, "watcher", "viewer")
	if code := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody, xrw, viewer).Code; code != http.StatusForbidden {
		t.Fatalf("viewer switch: %d, want 403", code)
	}
	// A service token passes the generic write rule but not this one.
	if code := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody,
		map[string]string{"Authorization": e.f.Bearer("unheaded-cli", "dashboard-backend")}).Code; code != http.StatusForbidden {
		t.Fatalf("service token switch: %d, want 403", code)
	}
	if len(st.changes) != 0 {
		t.Fatalf("a refused switch reached the store: %+v", st.changes)
	}

	op := e.login(t, "dev", "operator")
	rec := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody, xrw, op)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator switch: %d %s", rec.Code, rec.Body)
	}
	if len(st.changes) != 1 {
		t.Fatalf("store got %d changes", len(st.changes))
	}
	got := st.changes[0]
	if got.Actor != "dev" || got.Host != "west" || got.Profile != "lab" || got.Mode != "audit" || got.Reason != "lab baseline after review" {
		t.Fatalf("change written: %+v", got)
	}
	if !strings.Contains(rec.Body.String(), `"actor":"dev"`) {
		t.Fatalf("response does not name the actor: %s", rec.Body)
	}
}

func TestProfileSwitch_Errors(t *testing.T) {
	e := newBFFEnv(t, nil)
	st := &recordingStore{}
	e.srv.compliance.writeStore = func(context.Context) (well.Store, error) { return st, nil }
	op := e.login(t, "dev", "operator")
	for name, tc := range map[string]struct {
		body string
		err  error
		want int
	}{
		"unknown field":  {`{"host":"west","profile":"lab","mode":"audit","reason":"r","actor":"someone-else"}`, nil, http.StatusBadRequest},
		"not json":       {`host=west`, nil, http.StatusBadRequest},
		"invalid change": {switchBody, fmt.Errorf("%w: no such profile", well.ErrInvalidChange), http.StatusBadRequest},
		"yaml fallback":  {switchBody, well.ErrReadOnlyFallback, http.StatusConflict},
		"enforcing gate": {switchBody, remediation.ErrEnforcingGate, http.StatusConflict},
		"oversized":      {`{"host":"west","profile":"lab","mode":"audit","reason":"` + strings.Repeat("r", 8192) + `"}`, nil, http.StatusRequestEntityTooLarge},
	} {
		t.Run(name, func(t *testing.T) {
			st.err = tc.err
			if code := e.do(http.MethodPost, complianceProfileSwitchPath, tc.body, xrw, op).Code; code != tc.want {
				t.Fatalf("got %d, want %d", code, tc.want)
			}
		})
	}
	st.err = nil
	e.srv.compliance.writeStore = func(context.Context) (well.Store, error) {
		return nil, fmt.Errorf("no password: set WELL_CONFIG_ADMIN_PASSWORD")
	}
	if code := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody, xrw, op).Code; code != http.StatusServiceUnavailable {
		t.Fatalf("no Well writer: %d, want 503", code)
	}
	if code := e.do(http.MethodGet, complianceProfileSwitchPath, "", nil, op).Code; code != http.StatusMethodNotAllowed {
		t.Fatalf("GET switch: %d, want 405", code)
	}
}

func TestProfileSwitch_AuthOffRefuses(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "")
	config := DefaultConfig()
	config.WotanAddr = "localhost:19999"
	config.ComplianceRoot = t.TempDir()
	srv, err := NewServer(config, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	st := &recordingStore{}
	srv.compliance.writeStore = func(context.Context) (well.Store, error) { return st, nil }
	e := &bffEnv{srv: srv, h: srv.httpServer.Handler}
	// With no logins there is no actor to record: never an anonymous switch.
	if code := e.do(http.MethodPost, complianceProfileSwitchPath, switchBody, xrw).Code; code != http.StatusUnauthorized {
		t.Fatalf("switch with auth off: %d, want 401", code)
	}
	if len(st.changes) != 0 {
		t.Fatal("an anonymous switch reached the store")
	}
}
